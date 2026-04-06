package scanner

import (
	"context"
	"fmt"
	"time"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
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

		type pendingCandidate struct {
			bar       domain.Bar
			metrics   domain.Metrics
			lastPrice float64
		}

		flushBatch := func(batch []pendingBar) bool {
			batchTimestamp := batch[0].bar.Timestamp
			if err := s.state.Portfolio.EnsureStartingEquity(batchTimestamp); err != nil {
				log.Errorf("Failed to refresh starting equity for %s: %v", batchTimestamp.In(markethours.Location).Format("2006-01-02"), err)
				return true
			}

			if s.state.IsPaused(batchTimestamp) || !markethours.IsMarketOpen(batchTimestamp) {
				return true
			}

			candidates := make([]pendingCandidate, 0, len(batch))
			for _, pending := range batch {
				metrics := pending.symbol.GetMetrics()
				lastPrice := pending.symbol.GetLastPrice()

				// Update portfolio first in case we need to exit a position before evaluating new candidates.
				if s.state.Portfolio.HasOpenTrade(pending.symbol.Name, pending.bar.Timestamp) {
					s.state.Portfolio.UpdateOpenTrade(pending.symbol.Name, lastPrice, metrics, pending.bar.Timestamp)
					continue
				}

				candidates = append(candidates, pendingCandidate{
					bar:       pending.bar,
					metrics:   metrics,
					lastPrice: lastPrice,
				})
			}

			if markethours.HasReachedRegularSessionCloseBuffer(batchTimestamp, 15*time.Minute) {
				s.state.Portfolio.LiquidateOpenTrades(batchTimestamp)
				return true
			}

			if markethours.HasReachedRegularSessionCloseBuffer(batchTimestamp, 30*time.Minute) {
				return true
			}

			for _, candidate := range candidates {
				shouldEmit, _ := s.evaluate(candidate.bar.Symbol, candidate.metrics, candidate.lastPrice)
				if !shouldEmit {
					continue
				}

				select {
				case out <- domain.Candidate{
					Symbol:    candidate.bar.Symbol,
					Timestamp: candidate.bar.Timestamp,
					Metrics:   candidate.metrics,
					LastPrice: candidate.lastPrice,
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
