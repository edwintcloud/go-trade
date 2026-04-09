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

const recentBarHistorySize = 30

const (
	breakoutMetricLookback = 5
	pullbackMetricLookback = 3
)

type alignedMetricStreams struct {
	macd       <-chan float64
	macdRoc    <-chan float64
	macdSignal <-chan float64
	atr        <-chan float64
	volume5m   <-chan float64
	rsi        <-chan float64
	ema        <-chan float64
	emaRoc     <-chan float64
	hullMa     <-chan float64
	hullMaRoc  <-chan float64
}

type Symbol struct {
	Name                    string
	bars                    chan Bar
	metrics                 Metrics
	lastPrice               float64
	dailyVolume             uint64
	dailyVolumeAccStartDate time.Time
	sessionVolume           float64
	sessionNotional         float64
	recentBars              []Bar
	metricsInitOnce         sync.Once
	mu                      sync.RWMutex
	cond                    *sync.Cond
	pendingBars             int
}

func NewSymbol(name string) *Symbol {
	s := &Symbol{
		Name: name,
		bars: make(chan Bar, 1000),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *Symbol) ensureMetricsInitialized() {
	s.metricsInitOnce.Do(s.initializeMetrics)
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
	s.ensureMetricsInitialized()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.waitForPendingBarsLocked()

	metrics := s.metrics
	if s.sessionVolume > 0 {
		metrics.SessionVWAP = s.sessionNotional / s.sessionVolume
	}
	metrics.RelativeVolume20 = s.currentToAverageRatioLocked(20, func(bar Bar) float64 {
		return float64(bar.Volume)
	})
	metrics.TradeCountAccel = s.currentToAverageRatioLocked(20, func(bar Bar) float64 {
		return float64(bar.TradeCount)
	})
	s.populateTriggerMetricsLocked(&metrics)

	return metrics
}

func (s *Symbol) GetDailyVolume() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dailyVolume
}

func (s *Symbol) AddBar(bar Bar) {
	s.ensureMetricsInitialized()

	s.mu.Lock()
	s.rollSessionStateLocked(bar.Timestamp)
	s.updateDailyVolume(bar.Timestamp, bar.Volume)
	s.updateSessionVWAPLocked(bar)
	s.appendRecentBarLocked(bar)
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

func (s *Symbol) waitForPendingBarsLocked() {
	for s.pendingBars > 0 {
		s.cond.Wait()
	}
}

func (s *Symbol) rollSessionStateLocked(currentTime time.Time) {
	if markethours.IsSameDay(s.dailyVolumeAccStartDate, currentTime) {
		return
	}

	s.sessionVolume = 0
	s.sessionNotional = 0
	s.recentBars = s.recentBars[:0]
}

func (s *Symbol) updateDailyVolume(currentTime time.Time, barVolume uint64) {
	if !markethours.IsSameDay(s.dailyVolumeAccStartDate, currentTime) {
		s.dailyVolume = barVolume
		s.dailyVolumeAccStartDate = currentTime
		return
	}

	s.dailyVolume += barVolume
}

func (s *Symbol) updateSessionVWAPLocked(bar Bar) {
	price := bar.VWAP
	if price == 0 {
		price = bar.Close
	}

	volume := float64(bar.Volume)
	if volume == 0 {
		return
	}

	s.sessionNotional += price * volume
	s.sessionVolume += volume
}

func (s *Symbol) appendRecentBarLocked(bar Bar) {
	s.recentBars = append(s.recentBars, bar)
	if len(s.recentBars) > recentBarHistorySize {
		s.recentBars = s.recentBars[len(s.recentBars)-recentBarHistorySize:]
	}
}

func (s *Symbol) currentToAverageRatioLocked(lookback int, value func(Bar) float64) float64 {
	if len(s.recentBars) < 2 {
		return 0
	}

	current := value(s.recentBars[len(s.recentBars)-1])
	sum := 0.0
	count := 0
	for i := len(s.recentBars) - 2; i >= 0 && count < lookback; i-- {
		sum += value(s.recentBars[i])
		count++
	}

	if current == 0 || sum == 0 || count == 0 {
		return 0
	}

	return current / (sum / float64(count))
}

func (s *Symbol) populateTriggerMetricsLocked(metrics *Metrics) {
	if len(s.recentBars) == 0 {
		return
	}

	currentBar := s.recentBars[len(s.recentBars)-1]
	metrics.CloseStrength = candleCloseStrength(currentBar)

	priorBars := s.recentBars[:len(s.recentBars)-1]
	if len(priorBars) >= breakoutMetricLookback {
		metrics.BreakoutLevel5 = highestHigh(tailBars(priorBars, breakoutMetricLookback))
		metrics.BreakoutOpenedBelowLevel = currentBar.Open <= metrics.BreakoutLevel5
	}

	if len(priorBars) >= pullbackMetricLookback {
		metrics.PullbackLow3 = lowestLow(tailBars(priorBars, pullbackMetricLookback))
		metrics.PullbackReclaimLevel = priorBars[len(priorBars)-1].High
		metrics.PullbackOpenedBelowLevel = currentBar.Open <= metrics.PullbackReclaimLevel
	}
}

func (s *Symbol) initializeMetrics() {
	macdBars := make(chan Bar)
	atrHighBars := make(chan Bar)
	atrLowBars := make(chan Bar)
	atrCloseBars := make(chan Bar)
	volumeBars := make(chan Bar)
	rsiBars := make(chan Bar)
	emaBars := make(chan Bar)
	currentBars := make(chan Bar)
	hullMaBars := make(chan Bar)
	hullMaRocBars := make(chan Bar)

	go func() {
		for bar := range s.bars {
			macdBars <- bar
			atrHighBars <- bar
			atrLowBars <- bar
			atrCloseBars <- bar
			volumeBars <- bar
			rsiBars <- bar
			emaBars <- bar
			currentBars <- bar
			hullMaBars <- bar
			hullMaRocBars <- bar
		}
	}()

	// macd
	macdIndicator := trend.NewMacdWithPeriod[float64](24, 52, 18)
	closesMacd := helper.Map(macdBars, func(b Bar) float64 { return b.Close })
	macdLineRaw, macdSignalRaw := macdIndicator.Compute(closesMacd)
	macdStreams := helper.Duplicate(macdLineRaw, 2)
	macdRocIndicator := trend.NewRocWithPeriod[float64](5)

	// atr
	atrIndicator := volatility.NewAtrWithPeriod[float64](14)
	highsAtr := helper.Map(atrHighBars, func(b Bar) float64 { return b.High })
	lowsAtr := helper.Map(atrLowBars, func(b Bar) float64 { return b.Low })
	closesAtr := helper.Map(atrCloseBars, func(b Bar) float64 { return b.Close })
	atrRaw := atrIndicator.Compute(highsAtr, lowsAtr, closesAtr)

	// 5 min volume
	volume5mIndicator := trend.NewSmaWithPeriod[float64](5)
	volumes5m := helper.Map(volumeBars, func(b Bar) float64 { return float64(b.Volume) })
	volume5mRaw := volume5mIndicator.Compute(volumes5m)

	// rsi
	rsiIndicator := momentum.NewRsiWithPeriod[float64](14)
	closesRsi := helper.Map(rsiBars, func(b Bar) float64 { return b.Close })
	rsiRaw := rsiIndicator.Compute(closesRsi)

	// EMA20
	emaIndicator := trend.NewEmaWithPeriod[float64](20)
	closesEma := helper.Map(emaBars, func(b Bar) float64 { return b.Close })
	emaRaw := emaIndicator.Compute(closesEma)
	emaStreams := helper.Duplicate(emaRaw, 2)
	emaRocIndicator := trend.NewRocWithPeriod[float64](5)

	// hull ma
	hullMaIndicator := trend.NewHmaWithPeriod[float64](30)
	closesHullMa := helper.Map(hullMaBars, func(b Bar) float64 { return b.Close })
	hullMaRaw := hullMaIndicator.Compute(closesHullMa)
	hullMaStreams := helper.Duplicate(hullMaRaw, 2)
	hullMaRocIndicator := trend.NewRocWithPeriod[float64](5)

	s.consumeMetrics(currentBars, alignedMetricStreams{
		macd:       alignMetricStream(macdStreams[0], macdIndicator.IdlePeriod()),
		macdRoc:    alignMetricStream(macdRocIndicator.Compute(macdStreams[1]), macdIndicator.IdlePeriod()+macdRocIndicator.IdlePeriod()),
		macdSignal: alignMetricStream(macdSignalRaw, macdIndicator.IdlePeriod()),
		atr:        alignMetricStream(atrRaw, atrIndicator.IdlePeriod()),
		volume5m:   alignMetricStream(volume5mRaw, volume5mIndicator.IdlePeriod()),
		rsi:        alignMetricStream(rsiRaw, rsiIndicator.IdlePeriod()),
		ema:        alignMetricStream(emaStreams[0], emaIndicator.IdlePeriod()),
		emaRoc:     alignMetricStream(emaRocIndicator.Compute(emaStreams[1]), emaIndicator.IdlePeriod()+emaRocIndicator.IdlePeriod()),
		hullMa:     alignMetricStream(hullMaStreams[0], hullMaIndicator.IdlePeriod()),
		hullMaRoc:  alignMetricStream(hullMaRocIndicator.Compute(hullMaStreams[1]), hullMaIndicator.IdlePeriod()+hullMaRocIndicator.IdlePeriod()),
	})
}

func (s *Symbol) consumeMetrics(currentBars <-chan Bar, streams alignedMetricStreams) {
	go func() {
		for range currentBars {
			macd, ok := <-streams.macd
			if !ok {
				return
			}
			macdRoc, ok := <-streams.macdRoc
			if !ok {
				return
			}
			macdSignal, ok := <-streams.macdSignal
			if !ok {
				return
			}
			atr, ok := <-streams.atr
			if !ok {
				return
			}
			volume5m, ok := <-streams.volume5m
			if !ok {
				return
			}
			rsi, ok := <-streams.rsi
			if !ok {
				return
			}
			ema, ok := <-streams.ema
			if !ok {
				return
			}
			emaRoc, ok := <-streams.emaRoc
			if !ok {
				return
			}
			hullMa, ok := <-streams.hullMa
			if !ok {
				return
			}
			hullMaRoc, ok := <-streams.hullMaRoc
			if !ok {
				return
			}

			s.mu.Lock()
			s.metrics.MACD = macd
			s.metrics.MACDRoc = macdRoc
			s.metrics.MACDSignal = macdSignal
			s.metrics.ATR = atr
			s.metrics.Volume5m = volume5m
			s.metrics.RSI = rsi
			s.metrics.EMA20 = ema
			s.metrics.EMA20Roc = emaRoc
			s.metrics.HullMa = hullMa
			s.metrics.HullMaRoc = hullMaRoc
			if s.pendingBars > 0 {
				s.pendingBars--
			}
			s.cond.Broadcast()
			s.mu.Unlock()
		}
	}()
}

func alignMetricStream(values <-chan float64, idleCount int) <-chan float64 {
	aligned := make(chan float64, cap(values))

	go func() {
		defer close(aligned)

		for i := 0; i < idleCount; i++ {
			aligned <- 0
		}

		for value := range values {
			aligned <- value
		}
	}()

	return aligned
}

func tailBars(bars []Bar, limit int) []Bar {
	if limit <= 0 || limit >= len(bars) {
		return bars
	}

	return bars[len(bars)-limit:]
}

func highestHigh(bars []Bar) float64 {
	high := 0.0
	for _, bar := range bars {
		if bar.High > high {
			high = bar.High
		}
	}
	return high
}

func lowestLow(bars []Bar) float64 {
	low := 0.0
	for i, bar := range bars {
		if i == 0 || bar.Low < low {
			low = bar.Low
		}
	}
	return low
}

func candleCloseStrength(bar Bar) float64 {
	rangeSize := bar.High - bar.Low
	if rangeSize <= 0 {
		return 0
	}

	return clampFloat((bar.Close-bar.Low)/rangeSize, 0, 1)
}

func clampFloat(value float64, lower float64, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}
