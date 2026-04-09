package execution

import (
	"context"
	"slices"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
)

type ExecutionEngine struct {
	client             *alpaca.Client
	state              *state.State
	symbolsToSubscribe []string
	subscribedToSymbol map[string]bool
}

func NewExecutionEngine(client *alpaca.Client, state *state.State) *ExecutionEngine {
	return &ExecutionEngine{
		client:             client,
		state:              state,
		symbolsToSubscribe: make([]string, 0, 10),
		subscribedToSymbol: make(map[string]bool),
	}
}

func (e *ExecutionEngine) MonitorCandidates(ctx context.Context, candidates <-chan string) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Infof("Execution engine subscription manager exiting")
				return
			case <-time.After(60 * time.Second):
				e.ensureSubscriptions(ctx)
			}
		}
	}()

	for {
		select {
		case candidate := <-candidates:
			log.Debugf("Received candidate: %s", candidate)
			e.symbolsToSubscribe = append(e.symbolsToSubscribe, candidate)
			if len(e.symbolsToSubscribe) > 10 {
				e.symbolsToSubscribe = e.symbolsToSubscribe[len(e.symbolsToSubscribe)-10:]
			}
		case <-ctx.Done():
			log.Infof("Live trading session ended")
			return nil
		}
	}
}

func (e *ExecutionEngine) ensureSubscriptions(ctx context.Context) {
	for _, symbol := range e.symbolsToSubscribe {
		if !e.subscribedToSymbol[symbol] {
			err := e.subscribeToSymbol(ctx, symbol)
			if err != nil {
				log.Errorf("Failed to subscribe to symbol %s: %v", symbol, err)
				continue
			}
			e.subscribedToSymbol[symbol] = true
		}
	}
	timestamp := time.Now().In(markethours.Location)
	for symbol := range e.subscribedToSymbol {
		hasOpenTrade := e.state.Portfolio.HasOpenTrade(symbol, timestamp)
		if !slices.Contains(e.symbolsToSubscribe, symbol) && !hasOpenTrade {
			err := e.unsubscribeFromSymbol(symbol)
			if err != nil {
				log.Errorf("Failed to unsubscribe from symbol %s: %v", symbol, err)
				continue
			}
			delete(e.subscribedToSymbol, symbol)
		}
	}

	// liquidate all open trades and clear all subscriptions 15 minutes before market close to avoid holding positions overnight
	if e.state.Portfolio.LenOpenTrades() > 0 && markethours.HasReachedTradableSessionCloseBuffer(timestamp, 15*time.Minute) {
		log.Infof("15 minutes until market close, liquidating all open trades and clearing all subscriptions")
		e.state.Portfolio.LiquidateOpenTrades(timestamp)
		e.symbolsToSubscribe = make([]string, 0, 10)
		for symbol := range e.subscribedToSymbol {
			err := e.unsubscribeFromSymbol(symbol)
			if err != nil {
				log.Errorf("Failed to unsubscribe from symbol %s: %v", symbol, err)
				continue
			}
			delete(e.subscribedToSymbol, symbol)
		}
		return
	}
	// TODO: reconcile open trades with current subscriptions and subscribe to any missing symbols for open trades
}

func (e *ExecutionEngine) handleBar(bar stream.Bar) {
	domainSymbol, ok := e.state.Symbols.Get(bar.Symbol)
	if !ok {
		log.Warnf("Received bar for unregistered symbol: %s", bar.Symbol)
		return
	}
	domainSymbol.AddBar(domain.BarFromStreamBar(bar))
	if e.state.Portfolio.HasOpenTrade(bar.Symbol, bar.Timestamp) {
		return
	}
	time.Sleep(100 * time.Millisecond) // slight delay to ensure metrics are updated before evaluating entry conditions
	metrics := domainSymbol.GetMetrics()
	ok, reason := e.evaluateEntryConditions(bar.Close, metrics)
	if ok {
		log.Infof("Entry conditions met for %s at price %.2f", bar.Symbol, bar.Close)
		entryTimestamp := time.Now().In(markethours.Location)
		e.state.Portfolio.TryEnterTrade(domain.Candidate{
			Symbol:    bar.Symbol,
			Timestamp: entryTimestamp,
			LastPrice: bar.Close,
			Metrics:   metrics,
		})
	} else {
		log.Debugf("Entry conditions not met for %s at price %.2f: %s", bar.Symbol, bar.Close, reason)
	}
}

func (e *ExecutionEngine) handleQuote(quote stream.Quote) {
	// domainSymbol, ok := e.state.Symbols.Get(quote.Symbol)
	// if !ok {
	// 	log.Warnf("Received quote for unregistered symbol: %s", quote.Symbol)
	// 	return
	// }
	timestamp := time.Now().In(markethours.Location)
	if !e.state.Portfolio.HasOpenTrade(quote.Symbol, timestamp) {
		return
	}
	// evaluate immediate exit based on metrics
	// if domainSymbol.GetMetrics().HullMaRoc < 0 {
	// 	log.Infof("Exit conditions met for %s at price %.2f", quote.Symbol, quote.BidPrice)
	// 	e.state.Portfolio.ExitTrade(quote.Symbol, timestamp, quote.BidPrice)
	// 	return
	// }
	// evaluate exit based on trailing atr stop loss and update the stop loss price if necessary
	e.state.Portfolio.EvaluateExitConditions(quote.Symbol, quote.BidPrice, timestamp)
}

func (e *ExecutionEngine) subscribeToSymbol(ctx context.Context, symbol string) error {
	log.Debugf("Subscribing to symbol: %s", symbol)
	// subscribe to minute bars for entry signals
	err := e.client.SubscribeToBars(ctx, func(bar stream.Bar) { e.handleBar(bar) }, symbol)
	if err != nil {
		return err
	}
	// subscribe to quote updates for updating price in real time for open trades
	return e.client.SubscribeQuotes(ctx, func(quote stream.Quote) { e.handleQuote(quote) }, symbol)
}

func (e *ExecutionEngine) unsubscribeFromSymbol(symbol string) error {
	log.Debugf("Unsubscribing from symbol: %s", symbol)

	err := e.client.UnsubscribeFromBars(symbol)
	if err != nil {
		return err
	}
	return e.client.UnsubscribeQuotes(symbol)
}

func (e *ExecutionEngine) evaluateEntryConditions(lastPrice float64, metrics domain.Metrics) (bool, string) {
	if metrics.HullMa == 0 || metrics.HullMaRoc == 0 || metrics.VWAPRoc == 0 {
		return false, "not-ready"
	}
	if metrics.VWAPRoc < 0 {
		return false, "vwap-roc-negative"
	}
	if metrics.HullMaRoc < 0 {
		return false, "hull-ma-roc-negative"
	}
	if lastPrice < metrics.HullMa {
		return false, "price-below-hull-ma"
	}
	if lastPrice-metrics.HullMa > 0.02*lastPrice {
		return false, "price-above-hull-ma-by-more-than-2-percent"
	}
	return true, ""
}
