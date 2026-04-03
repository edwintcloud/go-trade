package scanner

import (
	"context"
	"fmt"
	"time"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/state"
)

type Scanner struct {
	config  *config.Config
	state   *state.State
	symbols domain.Symbols
}

func NewScanner(config *config.Config, state *state.State, symbols domain.Symbols) *Scanner {
	return &Scanner{config: config, state: state, symbols: symbols}
}

func (s *Scanner) Start(ctx context.Context, in <-chan domain.Bar, out chan<- domain.Candidate) error {
	n := 10 // the number of workers

	for range n {
		go func() {
			for bar := range in {
				symbol, ok := s.symbols.Get(bar.Symbol)
				if !ok {
					fmt.Printf("Received bar for unknown symbol: %s\n", bar.Symbol)
					continue
				}
				symbol.SetLastPrice((bar.High + bar.Low) / 2)
				symbol.AddBar(bar) // updates metrics

				// give the async indicator pipeline time to process the bar
				time.Sleep(5 * time.Millisecond)

				// evaluate
				candidate, shouldEmit, _ := s.evaluate(symbol)
				if !shouldEmit {
					continue
				}
				select {
				case out <- candidate:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return nil
}

func (s *Scanner) evaluate(symbol *domain.Symbol) (domain.Candidate, bool, string) {
	metrics := symbol.GetMetrics()
	lastPrice := symbol.GetLastPrice()
	if lastPrice == 0 || metrics.ATR == 0 || metrics.SMA20 == 0 || metrics.MACD == 0 || metrics.MACDSignal == 0 || metrics.StochK == 0 || metrics.StochD == 0 || metrics.Volume5m == 0 || metrics.RSI == 0 {
		return domain.Candidate{}, false, "not-enough-data"
	}

	if lastPrice <= metrics.SMA20 {
		return domain.Candidate{}, false, "price-below-sma20"
	}
	if metrics.MACD <= metrics.MACDSignal {
		return domain.Candidate{}, false, "macd-below-signal"
	}
	if metrics.StochK <= metrics.StochD {
		return domain.Candidate{}, false, "stoch-k-below-d"
	}
	if metrics.Volume5m <= 1000 {
		return domain.Candidate{}, false, "low-volume"
	}
	if metrics.RSI <= 70 {
		return domain.Candidate{}, false, "rsi-not-overbought"
	}

	return domain.Candidate{
		Symbol: symbol.Name,
		LastPrice: lastPrice,
		Metrics: metrics,
	}, true, ""
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
