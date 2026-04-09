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
			e.symbolsToSubscribe = append(e.symbolsToSubscribe[1:], candidate) // keep only the 10 most recent candidates
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
	if markethours.HasReachedRegularSessionCloseBuffer(timestamp, 15*time.Minute) {
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

func (e *ExecutionEngine) handleBar(symbol string, bar stream.Bar) {
	domainSymbol, ok := e.state.Symbols.Get(symbol)
	if !ok {
		log.Warnf("Received bar for unregistered symbol: %s", symbol)
		return
	}
	domainSymbol.AddBar(domain.BarFromStreamBar(bar))
	time.Sleep(1 * time.Second) // slight delay to ensure metrics are updated before evaluating entry conditions
	lastPrice := domainSymbol.GetLastPrice()
	metrics := domainSymbol.GetMetrics()
	if ok, _ := e.evaluateEntryConditions(metrics); ok {
		log.Infof("Entry conditions met for %s at price %.2f", symbol, lastPrice)
		entryTimestamp := time.Now().In(markethours.Location)
		e.state.Portfolio.TryEnterTrade(domain.Candidate{
			Symbol:    symbol,
			Timestamp: entryTimestamp,
			LastPrice: lastPrice,
			Metrics:   metrics,
		})
	}
}

func (e *ExecutionEngine) handleQuote(symbol string, quote stream.Quote) {
	domainSymbol, ok := e.state.Symbols.Get(symbol)
	if !ok {
		log.Warnf("Received quote for unregistered symbol: %s", symbol)
		return
	}
	timestamp := time.Now().In(markethours.Location)
	// evaluate immediate exit based on metrics
	if domainSymbol.GetMetrics().HullMaRoc < 0 {
		log.Infof("Exit conditions met for %s at price %.2f", symbol, quote.BidPrice)
		e.state.Portfolio.ExitTrade(symbol, timestamp, quote.BidPrice)
		return
	}
	// evaluate exit based on trailing atr stop loss and update the stop loss price if necessary
	e.state.Portfolio.EvaluateExitConditions(symbol, quote.BidPrice, timestamp)
}

func (e *ExecutionEngine) subscribeToSymbol(ctx context.Context, symbol string) error {
	// subscribe to minute bars for entry signals
	err := e.client.SubscribeToBars(ctx, func(bar stream.Bar) { e.handleBar(symbol, bar) }, symbol)
	if err != nil {
		return err
	}
	// subscribe to quote updates for updating price in real time for open trades
	return e.client.SubscribeQuotes(ctx, func(quote stream.Quote) { e.handleQuote(symbol, quote) }, symbol)
}

func (e *ExecutionEngine) unsubscribeFromSymbol(symbol string) error {
	err := e.client.UnsubscribeFromBars(symbol)
	if err != nil {
		return err
	}
	return e.client.UnsubscribeQuotes(symbol)
}

func (e *ExecutionEngine) evaluateEntryConditions(metrics domain.Metrics) (bool, string) {
	if metrics.HullMa == 0 || metrics.HullMaRoc == 0 {
		return false, "not-ready"
	}
	if metrics.HullMaRoc > 0 {
		return true, "hull-ma-roc-positive"
	}
	return false, ""
}
