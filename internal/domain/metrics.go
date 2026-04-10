package domain

import (
	"github.com/cinar/indicator/v2/helper"
	"github.com/cinar/indicator/v2/trend"
	"github.com/cinar/indicator/v2/volatility"
)

type Metrics struct {
	ATR               float64
	HullMa            float64
	HullMaRoc         float64
	VWAPRoc           float64
	AverageVolume5Min float64
	BidAskSpreadPct   float64
}

func (s *Symbol) GetMetrics() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

func (s *Symbol) initializeMetrics() {
	atrHighBars := make(chan Bar)
	atrLowBars := make(chan Bar)
	atrCloseBars := make(chan Bar)
	hullMaBars := make(chan Bar)
	vwapRocBars := make(chan Bar)
	volumeBars := make(chan Bar)

	go func() {
		for bar := range s.bars {
			atrHighBars <- bar
			atrLowBars <- bar
			atrCloseBars <- bar
			hullMaBars <- bar
			vwapRocBars <- bar
			volumeBars <- bar
		}
	}()

	// atr
	atrIndicator := volatility.NewAtrWithPeriod[float64](14)
	highsAtr := helper.Map(atrHighBars, func(b Bar) float64 { return b.High })
	lowsAtr := helper.Map(atrLowBars, func(b Bar) float64 { return b.Low })
	closesAtr := helper.Map(atrCloseBars, func(b Bar) float64 { return b.Close })
	atr := atrIndicator.Compute(highsAtr, lowsAtr, closesAtr)

	// hull ma
	hullMaIndicator := trend.NewHmaWithPeriod[float64](30)
	closesHullMa := helper.Map(hullMaBars, func(b Bar) float64 { return b.Close })
	hullMaRaw := hullMaIndicator.Compute(closesHullMa)
	hullMaStreams := helper.Duplicate(hullMaRaw, 2)
	hullMaRocIndicator := trend.NewRocWithPeriod[float64](5)
	hullMa := hullMaStreams[0]
	hullMaRoc := hullMaRocIndicator.Compute(hullMaStreams[1])

	// vwap roc
	vwapRocIndicator := trend.NewRocWithPeriod[float64](5)
	vwapRoc := vwapRocIndicator.Compute(helper.Map(vwapRocBars, func(b Bar) float64 { return b.VWAP }))

	// volume sma 5 period
	volumeSmaIndicator := trend.NewSmaWithPeriod[float64](5)
	volumeSma := volumeSmaIndicator.Compute(helper.Map(volumeBars, func(b Bar) float64 { return float64(b.Volume) }))

	s.startMetricStream(atr, func(value float64) { s.metrics.ATR = value })
	s.startMetricStream(hullMa, func(value float64) { s.metrics.HullMa = value })
	s.startMetricStream(hullMaRoc, func(value float64) { s.metrics.HullMaRoc = value })
	s.startMetricStream(vwapRoc, func(value float64) { s.metrics.VWAPRoc = value })
	s.startMetricStream(volumeSma, func(value float64) { s.metrics.AverageVolume5Min = value })

}

func (s *Symbol) startMetricStream(stream <-chan float64, onValue func(value float64)) {
	go func() {
		for value := range stream {
			s.mu.Lock()
			onValue(value)
			s.mu.Unlock()
		}
	}()
}

func (s *Symbol) ensureMetricsInitialized() {
	s.metricsInitOnce.Do(s.initializeMetrics)
}
