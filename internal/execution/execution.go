package execution

import (
	"context"
	"slices"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
)

type ExecutionEngine struct {
	config             *config.Config
	state              *state.State
	client             *alpaca.Client
	symbolsToSubscribe []string
	subscribedToSymbol map[string]bool
}

func NewExecutionEngine(config *config.Config, state *state.State) *ExecutionEngine {
	return &ExecutionEngine{
		config:             config,
		state:              state,
		symbolsToSubscribe: make([]string, 0, 10),
		subscribedToSymbol: make(map[string]bool),
	}
}

func (e *ExecutionEngine) WithClient(client *alpaca.Client) *ExecutionEngine {
	e.client = client
	return e
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
		hasOpenTrade := e.state.Portfolio.HasOpenTrade(symbol)
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

func (e *ExecutionEngine) handleQuote(quote stream.Quote) {
	domainSymbol, ok := e.state.Symbols.Get(quote.Symbol)
	if !ok {
		log.Warnf("Received quote for unregistered symbol: %s", quote.Symbol)
		return
	}
	domainSymbol.UpdateSymbolWithQuote(quote)

	if !e.state.Portfolio.HasOpenTrade(quote.Symbol) {
		// evaluate for entry
		metrics := domainSymbol.GetMetrics()
		strategyApproved, reason := e.evaluateEntryConditions(quote.BidPrice, metrics)
		if !strategyApproved {
			log.Debugf("Entry conditions not met for %s at price %.2f: %s", quote.Symbol, quote.BidPrice, reason)
			return
		}
		riskApproved, reason := e.state.Portfolio.TryEnterTrade(domainSymbol)
		if !riskApproved {
			log.Debugf("Risk conditions not met for %s at price %.2f: %s", quote.Symbol, quote.BidPrice, reason)
			return
		}
		log.Infof("Entering trade for %s at price %.2f based on entry conditions and risk management approval", quote.Symbol, quote.BidPrice)
	} else {
		// evaluate for exit
		e.state.Portfolio.EvaluateExitConditions(domainSymbol)
	}
}

func (e *ExecutionEngine) subscribeToSymbol(ctx context.Context, symbol string) error {
	log.Debugf("Subscribing to symbol: %s", symbol)
	// subscribe to quote updates for updating price in real time for open trades
	return e.client.SubscribeQuotes(ctx, func(quote stream.Quote) { e.handleQuote(quote) }, symbol)
}

func (e *ExecutionEngine) unsubscribeFromSymbol(symbol string) error {
	log.Debugf("Unsubscribing from symbol: %s", symbol)

	return e.client.UnsubscribeQuotes(symbol)
}

func (e *ExecutionEngine) evaluateEntryConditions(lastPrice float64, metrics domain.Metrics) (bool, string) {
	if metrics.HullMa == 0 || metrics.HullMaRoc == 0 || metrics.VWAPRoc == 0 || metrics.AverageVolume5Min == 0 {
		return false, "not-ready"
	}
	if metrics.VWAPRoc < 0 {
		return false, "vwap-roc-negative"
	}
	if metrics.AverageVolume5Min < float64(e.config.MinAverageVolume5Min) {
		return false, "average-volume-5min-too-low"
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
	if metrics.BidAskSpreadPct > e.config.MaxSpreadPct {
		return false, "bid-ask-spread-too-wide"
	}
	return true, ""
}
