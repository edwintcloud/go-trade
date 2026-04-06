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
				metrics := pending.symbol.GetMetrics()
				lastPrice := pending.symbol.GetLastPrice()

				if s.state.Portfolio.IsTradingBlocked() || s.state.IsPaused() {
					continue
				}

				if s.state.Portfolio.HasOpenTrade(pending.bar.Symbol, pending.bar.Timestamp) {
					position, _ := s.state.Portfolio.GetTrade(pending.bar.Symbol, pending.bar.Timestamp)
					if lastPrice < position.StopPrice {
						s.state.Portfolio.ExitTrade(pending.bar.Symbol, pending.bar.Timestamp, lastPrice)
						continue
					}
					newStopPrice := max(lastPrice*0.95, lastPrice-metrics.ATR*2)
					if newStopPrice > position.StopPrice {
						s.state.Portfolio.UpdateStopPrice(pending.bar.Symbol, newStopPrice)
					}
				}

				if !markethours.IsRegularSession(pending.bar.Timestamp) {
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

			symbol.SetLastPrice((bar.High + bar.Low) / 2)
			symbol.AddBar(bar)
			s.state.Symbols.UpdateVolumeLeaders(symbol.Name, symbol.GetDailyVolume(), bar.Timestamp)
			batch = append(batch, pendingBar{bar: bar, symbol: symbol})
		}
	}()

	return done, nil
}

func (s *Scanner) evaluate(symbolName string, metrics domain.Metrics, lastPrice float64) (bool, string) {
	if lastPrice == 0 || metrics.ATR == 0 || metrics.SMA20 == 0 || metrics.MACD == 0 || metrics.MACDSignal == 0 || metrics.StochK == 0 || metrics.StochD == 0 || metrics.Volume5m == 0 || metrics.RSI == 0 {
		return false, "not-enough-data"
	}

	if !s.state.Symbols.IsSymbolVolumeLeader(symbolName) {
		return false, "not-volume-leader"
	}

	if lastPrice <= metrics.SMA20 {
		return false, "price-below-sma20"
	}
	if metrics.MACD <= metrics.MACDSignal {
		return false, "macd-below-signal"
	}
	if metrics.StochK <= metrics.StochD {
		return false, "stoch-k-below-d"
	}
	if metrics.Volume5m <= 1000 {
		return false, "low-volume"
	}
	if metrics.RSI <= 70 {
		return false, "rsi-not-overbought"
	}

	return true, ""
}
