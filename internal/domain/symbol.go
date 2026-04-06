package domain

import (
	"sync"
	"time"

	"github.com/cinar/indicator/v2/helper"
	"github.com/cinar/indicator/v2/momentum"
	"github.com/cinar/indicator/v2/trend"
	"github.com/cinar/indicator/v2/volatility"
	"github.com/edwintcloud/go-trade/internal/markethours"
)

type Symbol struct {
	Name                    string
	bars                    chan Bar
	metrics                 Metrics
	lastPrice               float64
	dailyVolume             uint64
	dailyVolumeAccStartDate time.Time
	mu                      sync.RWMutex
	cond                    *sync.Cond
	pendingBars             int
}

func NewSymbol(name string) *Symbol {
	s := &Symbol{
		Name: name,
		bars: make(chan Bar, 240),
	}
	s.cond = sync.NewCond(&s.mu)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.pendingBars > 0 {
		s.cond.Wait()
	}

	return s.metrics
}

func (s *Symbol) GetDailyVolume() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dailyVolume
}

func (s *Symbol) AddBar(bar Bar) {
	s.mu.Lock()
	s.updateDailyVolume(bar.Timestamp, bar.Volume)
	select {
	case s.bars <- bar:
		s.pendingBars++
	default:
		// channel is full, drain one item to make space
		<-s.bars
		s.bars <- bar
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *Symbol) updateDailyVolume(currentTime time.Time, barVolume uint64) {
	if !markethours.IsSameDay(s.dailyVolumeAccStartDate, currentTime) {
		s.dailyVolume = barVolume
		s.dailyVolumeAccStartDate = currentTime
		return
	}

	s.dailyVolume += barVolume
}

func (s *Symbol) initializeMetrics() {
	macdBars := make(chan Bar)
	stochHighBars := make(chan Bar)
	stochLowBars := make(chan Bar)
	stochCloseBars := make(chan Bar)
	atrHighBars := make(chan Bar)
	atrLowBars := make(chan Bar)
	atrCloseBars := make(chan Bar)
	volumeBars := make(chan Bar)
	rsiBars := make(chan Bar)
	sma20Bars := make(chan Bar)

	go func() {
		for bar := range s.bars {
			macdBars <- bar
			stochHighBars <- bar
			stochLowBars <- bar
			stochCloseBars <- bar
			atrHighBars <- bar
			atrLowBars <- bar
			atrCloseBars <- bar
			volumeBars <- bar
			rsiBars <- bar
			sma20Bars <- bar

			s.mu.Lock()
			if s.pendingBars > 0 {
				s.pendingBars--
			}
			s.cond.Broadcast()
			s.mu.Unlock()
		}
	}()

	// macd
	closesMacd := helper.Map(macdBars, func(b Bar) float64 { return b.Close })
	macdLine, macdSignal := trend.NewMacdWithPeriod[float64](24, 52, 18).Compute(closesMacd)

	// stoch
	highsStoch := helper.Map(stochHighBars, func(b Bar) float64 { return b.High })
	lowsStoch := helper.Map(stochLowBars, func(b Bar) float64 { return b.Low })
	closesStoch := helper.Map(stochCloseBars, func(b Bar) float64 { return b.Close })
	stoch := &momentum.StochasticOscillator[float64]{
		Max: trend.NewMovingMaxWithPeriod[float64](10),
		Min: trend.NewMovingMinWithPeriod[float64](10),
		Sma: trend.NewSmaWithPeriod[float64](10),
	}
	stochK, stochD := stoch.Compute(highsStoch, lowsStoch, closesStoch)

	// atr
	highsAtr := helper.Map(atrHighBars, func(b Bar) float64 { return b.High })
	lowsAtr := helper.Map(atrLowBars, func(b Bar) float64 { return b.Low })
	closesAtr := helper.Map(atrCloseBars, func(b Bar) float64 { return b.Close })
	atr := volatility.NewAtrWithPeriod[float64](14).Compute(highsAtr, lowsAtr, closesAtr)

	// 5 min volume
	volumes5m := helper.Map(volumeBars, func(b Bar) float64 { return float64(b.Volume) })
	volume5m := trend.NewSmaWithPeriod[float64](5).Compute(volumes5m)

	// rsi
	closesRsi := helper.Map(rsiBars, func(b Bar) float64 { return b.Close })
	rsi := momentum.NewRsiWithPeriod[float64](14).Compute(closesRsi)

	// sma20
	closesSma20 := helper.Map(sma20Bars, func(b Bar) float64 { return b.Close })
	sma20 := trend.NewSmaWithPeriod[float64](20).Compute(closesSma20)

	s.consumeMetric(macdLine, func(v float64) {
		s.metrics.MACD = v
	})
	s.consumeMetric(macdSignal, func(v float64) {
		s.metrics.MACDSignal = v
	})
	s.consumeMetric(stochK, func(v float64) {
		s.metrics.StochK = v
	})
	s.consumeMetric(stochD, func(v float64) {
		s.metrics.StochD = v
	})
	s.consumeMetric(atr, func(v float64) {
		s.metrics.ATR = v
	})
	s.consumeMetric(volume5m, func(v float64) {
		s.metrics.Volume5m = v
	})
	s.consumeMetric(rsi, func(v float64) {
		s.metrics.RSI = v
	})
	s.consumeMetric(sma20, func(v float64) {
		s.metrics.SMA20 = v
	})
}

func (s *Symbol) consumeMetric(values <-chan float64, assign func(v float64)) {
	go func() {
		for v := range values {
			s.mu.Lock()
			assign(v)
			s.mu.Unlock()
		}
	}()
}
