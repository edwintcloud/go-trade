package scanner

import (
	"context"
	"fmt"
	"sync"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/state"
)

type Scanner struct {
	config    *config.Config
	state     *state.State
	symbols   domain.Symbols
	portfolio *domain.Portfolio
}

func NewScanner(config *config.Config, state *state.State, symbols domain.Symbols, portfolio *domain.Portfolio) *Scanner {
	return &Scanner{config: config, state: state, symbols: symbols, portfolio: portfolio}
}

func (s *Scanner) Start(ctx context.Context, in <-chan domain.Bar, out chan<- domain.Candidate) (<-chan struct{}, error) {
	// Shard bars by symbol so per-symbol bar order is preserved while workers run in parallel.
	done := make(chan struct{})
	n := 10 // the number of workers
	workerInputs := make([]chan domain.Bar, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		workerInputs[i] = make(chan domain.Bar, s.config.ChannelBufferSize)

		go func(workerIn <-chan domain.Bar) {
			defer wg.Done()

			for {
				var bar domain.Bar
				var ok bool

				select {
				case <-ctx.Done():
					return
				case bar, ok = <-workerIn:
					if !ok {
						return
					}
				}

				if bar.Volume == 0 || bar.Open == 0 || bar.Close == 0 || bar.High == 0 || bar.Low == 0 {
					continue
				}
				symbol, ok := s.symbols.Get(bar.Symbol)
				if !ok {
					fmt.Printf("Received bar for unknown symbol: %s\n", bar.Symbol)
					continue
				}
				symbol.SetLastPrice((bar.High + bar.Low) / 2)
				symbol.AddBar(bar) // updates metrics
				metrics := symbol.GetMetrics()
				lastPrice := symbol.GetLastPrice()

				// check for blocking conditions
				if s.portfolio.IsTradingBlocked() || s.state.IsPaused() {
					continue
				}

				// update stop price and check exit conditions
				if s.portfolio.HasOpenTrade(bar.Symbol, bar.Timestamp) {
					position, _ := s.portfolio.GetTrade(bar.Symbol, bar.Timestamp)
					if lastPrice < position.StopPrice {
						s.portfolio.ExitTrade(bar.Symbol, bar.Timestamp, lastPrice)
						continue
					}
					newStopPrice := max(lastPrice*0.95, lastPrice-metrics.ATR*2)
					if newStopPrice > position.StopPrice {
						s.portfolio.UpdateStopPrice(bar.Symbol, newStopPrice)
					}
				}

				// evaluate
				shouldEmit, _ := s.evaluate(metrics, lastPrice)
				if !shouldEmit {
					continue
				}
				select {
				case out <- domain.Candidate{
					Symbol:    bar.Symbol,
					Timestamp: bar.Timestamp,
					Metrics:   metrics,
					LastPrice: lastPrice,
				}:
				case <-ctx.Done():
					return
				}
			}
		}(workerInputs[i])
	}

	go func() {
		defer func() {
			for _, workerIn := range workerInputs {
				close(workerIn)
			}
		}()

		for {
			var bar domain.Bar
			var ok bool

			select {
			case <-ctx.Done():
				return
			case bar, ok = <-in:
				if !ok {
					return
				}
			}

			workerIn := workerInputs[s.workerIndex(bar.Symbol, n)]

			select {
			case <-ctx.Done():
				return
			case workerIn <- bar:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	return done, nil
}

func (s *Scanner) workerIndex(symbol string, workers int) int {
	hash := uint32(2166136261)
	for i := 0; i < len(symbol); i++ {
		hash ^= uint32(symbol[i])
		hash *= 16777619
	}
	return int(hash % uint32(workers))
}

func (s *Scanner) evaluate(metrics domain.Metrics, lastPrice float64) (bool, string) {
	if lastPrice == 0 || metrics.ATR == 0 || metrics.SMA20 == 0 || metrics.MACD == 0 || metrics.MACDSignal == 0 || metrics.StochK == 0 || metrics.StochD == 0 || metrics.Volume5m == 0 || metrics.RSI == 0 {
		return false, "not-enough-data"
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
