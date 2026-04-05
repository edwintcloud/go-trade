package scanner

import (
	"context"
	"fmt"

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

func (s *Scanner) Start(ctx context.Context, in <-chan domain.Bar, out chan<- domain.Candidate) error {
	// TODO: multiple workers causes out of order candidates, need to add a timestamp and sort them in the main loop
	n := 1 // the number of workers

	for range n {
		go func() {
			for bar := range in {
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
		}()
	}

	return nil
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
