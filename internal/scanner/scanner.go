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
	n := 10 // the number of workers

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
				if s.portfolio.TradingBlocked() || s.state.IsPaused() {
					continue
				}

				// update stop price and check exit conditions
				if s.portfolio.HasPosition(bar.Symbol, bar.Timestamp) {
					position, _ := s.portfolio.GetPosition(bar.Symbol, bar.Timestamp)
					if lastPrice < position.StopPrice {
						s.portfolio.ExitPosition(bar.Symbol, bar.Timestamp, lastPrice)
						continue
					}
					newStopPrice := lastPrice - metrics.ATR*1.5
					if newStopPrice > position.StopPrice {
						s.portfolio.UpdateStopPrice(bar.Symbol, bar.Timestamp, newStopPrice)
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

// func (s *Scanner) Scan(portfolio *portfolio.Portfolio, symbols domain.Symbols, minuteBars <-chan stream.Bar) {
// 	for {
// 		select {
// 		case bar := <-minuteBars:
// 			symbol := symbols[bar.Symbol]
// 			symbol.LastPrice = (bar.High + bar.Low) / 2
// 			trySend(symbol.MinuteBars, bar)

// 			time.Sleep(1 * time.Millisecond)

// 			// evaluate entry and exit conditions
// 			if portfolio.TradingBlocked() {
// 				continue
// 			}
// 			inPosition := portfolio.InPosition(bar.Symbol, bar.Timestamp)

// 			if inPosition {
// 				position, _ := portfolio.GetPosition(bar.Symbol, bar.Timestamp)
// 				if symbol.LastPrice < position.StopPrice {
// 					portfolio.ExitPosition(bar.Symbol, bar.Timestamp, symbol.LastPrice)
// 					continue
// 				} else {
// 					// update stop price
// 					newStopPrice := symbol.LastPrice - symbol.Metrics.ATR*1.5
// 					if newStopPrice > position.StopPrice {
// 						portfolio.UpdateStopPrice(bar.Symbol, bar.Timestamp, newStopPrice)
// 					}
// 				}
// 			}

// 			entryCondition := symbol.LastPrice > symbol.Metrics.SMA20 && symbol.Metrics.MACD > symbol.Metrics.MACDSignal && symbol.Metrics.StochK > symbol.Metrics.StochD && symbol.Metrics.Volume5m > 1000 && symbol.Metrics.RSI > 70
// 			if !entryCondition {
// 				continue
// 			}

// 			stopPrice := symbol.LastPrice - symbol.Metrics.ATR*1.5
// 			quantity := uint(portfolio.Equity / symbol.LastPrice * 0.8)
// 			portfolio.EnterPosition(bar.Symbol, bar.Timestamp, symbol.LastPrice, quantity, stopPrice)

// 		case <-time.After(2 * time.Second):
// 			fmt.Println("No new bars received in the last 2 seconds. Exiting.")
// 			portfolio.GenerateReport()
// 			return
// 		}
// 	}
// }

// func trySend[T any](c chan T, v T) {
// 	select {
// 	case c <- v:
// 	default:
// 		// channel is full, drain one item to make space
// 		<-c
// 		c <- v
// 	}
// }
