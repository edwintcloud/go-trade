package domain

import (
	"sync"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/cinar/indicator/v2/helper"
	"github.com/cinar/indicator/v2/momentum"
	"github.com/cinar/indicator/v2/trend"
	"github.com/cinar/indicator/v2/volatility"
)

type Symbol struct {
	Name      string
	bars      chan Bar
	metrics   Metrics
	lastPrice float64
	mu        sync.RWMutex
}

func NewSymbol(name string) *Symbol {
	s := &Symbol{
		Name:       name,
		bars: make(chan Bar, 240),
	}
	s.initializeMetrics()
	return s
}

func (s *Symbol) SetLastPrice(newPrice float64) {
	s.mu.Lock()
	s.lastPrice = newPrice
	s.mu.Unlock()
}

func (s *Symbol) GetLastPrice() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastPrice
}

func (s *Symbol) GetMetrics() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

func (s *Symbol) AddBar(bar Bar) {
	select {
	case s.bars <- bar:
	default:
		// channel is full, drain one item to make space
		<-s.bars
		s.bars <- bar
	}
}

func (s *Symbol) initializeMetrics() {
	bars := helper.Duplicate(s.bars, 10)

	// macd
	closesMacd := helper.Map(bars[0], func(b stream.Bar) float64 { return b.Close })
	macdLine, macdSignal := trend.NewMacdWithPeriod[float64](24, 52, 18).Compute(closesMacd)

	// stoch
	highsStoch := helper.Map(bars[1], func(b stream.Bar) float64 { return b.High })
	lowsStoch := helper.Map(bars[2], func(b stream.Bar) float64 { return b.Low })
	closesStoch := helper.Map(bars[3], func(b stream.Bar) float64 { return b.Close })
	stoch := &momentum.StochasticOscillator[float64]{
		Max: trend.NewMovingMaxWithPeriod[float64](10),
		Min: trend.NewMovingMinWithPeriod[float64](10),
		Sma: trend.NewSmaWithPeriod[float64](10),
	}
	stochK, stochD := stoch.Compute(highsStoch, lowsStoch, closesStoch)

	// atr
	highsAtr := helper.Map(bars[4], func(b stream.Bar) float64 { return b.High })
	lowsAtr := helper.Map(bars[5], func(b stream.Bar) float64 { return b.Low })
	closesAtr := helper.Map(bars[6], func(b stream.Bar) float64 { return b.Close })
	atr := volatility.NewAtrWithPeriod[float64](14).Compute(highsAtr, lowsAtr, closesAtr)

	// 5 min volume
	volumes5m := helper.Map(bars[7], func(b stream.Bar) float64 { return float64(b.Volume) })
	volume5m := trend.NewSmaWithPeriod[float64](5).Compute(volumes5m)

	// rsi
	closesRsi := helper.Map(bars[8], func(b stream.Bar) float64 { return b.Close })
	rsi := momentum.NewRsiWithPeriod[float64](14).Compute(closesRsi)

	// sma20
	closesSma20 := helper.Map(bars[9], func(b stream.Bar) float64 { return b.Close })
	sma20 := trend.NewSmaWithPeriod[float64](20).Compute(closesSma20)

	go func() {
		for {
			select {
			case v := <-macdLine:
				s.mu.Lock()
				s.metrics.MACD = v
				s.mu.Unlock()
			case v := <-macdSignal:
				s.mu.Lock()
				s.metrics.MACDSignal = v
				s.mu.Unlock()
			case v := <-stochK:
				s.mu.Lock()
				s.metrics.StochK = v
				s.mu.Unlock()
			case v := <-stochD:
				s.mu.Lock()
				s.metrics.StochD = v
				s.mu.Unlock()
			case v := <-atr:
				s.mu.Lock()
				s.metrics.ATR = v
				s.mu.Unlock()
			case v := <-volume5m:
				s.mu.Lock()
				s.metrics.Volume5m = v
				s.mu.Unlock()
			case v := <-rsi:
				s.mu.Lock()
				s.metrics.RSI = v
				s.mu.Unlock()
			case v := <-sma20:
				s.mu.Lock()
				s.metrics.SMA20 = v
				s.mu.Unlock()
			}
		}
	}()
}
