package scanner

import (
	"context"
	"fmt"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/state"
)

type Scanner struct {
	config *config.Config
	state  *state.State
}

func NewScanner(config *config.Config, state *state.State) *Scanner {
	return &Scanner{config: config, state: state}
}

func (s *Scanner) Start(ctx context.Context, in <-chan domain.Bar, out chan<- domain.Candidate) (<-chan struct{}, error) {
	if s.config == nil || s.state == nil || s.state.Symbols == nil || s.state.Portfolio == nil {
		return nil, fmt.Errorf("scanner not initialized")
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		type pendingBar struct {
			bar    domain.Bar
			symbol *domain.Symbol
		}

		flushBatch := func(batch []pendingBar) bool {
			for _, pending := range batch {
				if s.state.IsPaused(pending.bar.Timestamp) ||
					!markethours.IsMarketOpen(pending.bar.Timestamp) {
					continue
				}

				metrics := pending.symbol.GetMetrics()
				lastPrice := pending.symbol.GetLastPrice()

				// Update portfolio first incase we need to exit a position before evaluating new candidates.
				// This ensures we have the most up-to-date information on open trades, which may impact whether
				// we emit a new candidate for the same symbol.
				if s.state.Portfolio.HasOpenTrade(pending.symbol.Name, pending.bar.Timestamp) {
					s.state.Portfolio.UpdateOpenTrade(pending.symbol.Name, lastPrice, metrics, pending.bar.Timestamp)
					continue
				}

				shouldEmit, _ := s.evaluate(pending.symbol.Name, metrics, lastPrice)
				if !shouldEmit {
					continue
				}

				select {
				case out <- domain.Candidate{
					Symbol:    pending.bar.Symbol,
					Timestamp: pending.bar.Timestamp,
					Metrics:   metrics,
					LastPrice: lastPrice,
				}:
				case <-ctx.Done():
					return false
				}
			}

			return true
		}

		batch := make([]pendingBar, 0, s.config.ChannelBufferSize)
		for {
			var bar domain.Bar
			var ok bool

			select {
			case <-ctx.Done():
				return
			case bar, ok = <-in:
				if !ok {
					if len(batch) > 0 {
						flushBatch(batch)
					}
					return
				}
			}

			if len(batch) > 0 && !bar.Timestamp.Equal(batch[0].bar.Timestamp) {
				if !flushBatch(batch) {
					return
				}
				batch = batch[:0]
			}

			if bar.Volume == 0 || bar.Open == 0 || bar.Close == 0 || bar.High == 0 || bar.Low == 0 {
				continue
			}

			symbol, ok := s.state.Symbols.Get(bar.Symbol)
			if !ok {
				fmt.Printf("Received bar for unknown symbol: %s\n", bar.Symbol)
				continue
			}

			symbol.SetLastPrice(bar.Close)
			symbol.AddBar(bar)
			s.state.Symbols.UpdateVolumeLeaders(symbol.Name, symbol.GetDailyVolume(), bar.Timestamp)
			batch = append(batch, pendingBar{bar: bar, symbol: symbol})
		}
	}()

	return done, nil
}

func (s *Scanner) evaluate(symbolName string, metrics domain.Metrics, lastPrice float64) (bool, string) {
	if lastPrice == 0 || metrics.ATR == 0 || metrics.EMA20 == 0 || metrics.MACD == 0 || metrics.MACDSignal == 0 || metrics.Volume5m == 0 || metrics.RSI == 0 {
		return false, "not-enough-data"
	}

	if lastPrice > s.config.MaxPrice || lastPrice < s.config.MinPrice {
		return false, "price-out-of-range"
	}

	if !s.state.Symbols.IsSymbolVolumeLeader(symbolName) {
		return false, "not-volume-leader"
	}

	if lastPrice <= metrics.EMA20 {
		return false, "price-below-moving-average"
	}

	if metrics.MACD <= metrics.MACDSignal {
		return false, "macd-below-signal"
	}

	if metrics.RSI > 60 || metrics.RSI < 30 {
		return false, "rsi-out-of-range"
	}

	if metrics.EMA20Roc < 0.003 {
		// log.Infof("EMA20 ROC below 0 for %s: %.2f", symbolName, metrics.EMA20Roc)
		return false, "ema-roc-below-threshold"
	}

	if lastPrice-metrics.EMA20 > metrics.ATR {
		return false, "price-too-far-above-ema"
	}

	return true, ""
}
