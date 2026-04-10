package domain

import (
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

type Bar struct {
	Symbol     string
	Open       float64
	High       float64
	Low        float64
	Close      float64
	Volume     uint64
	Timestamp  time.Time
	TradeCount uint64
	VWAP       float64
}

func BarFromStreamBar(bar stream.Bar) Bar {
	return Bar{
		Symbol:     bar.Symbol,
		Open:       bar.Open,
		High:       bar.High,
		Low:        bar.Low,
		Close:      bar.Close,
		Volume:     bar.Volume,
		Timestamp:  bar.Timestamp,
		TradeCount: bar.TradeCount,
		VWAP:       bar.VWAP,
	}
}

func BarFromMarketdataBar(symbol string, bar marketdata.Bar) Bar {
	return Bar{
		Symbol:     symbol,
		Timestamp:  bar.Timestamp,
		Open:       bar.Open,
		Close:      bar.Close,
		High:       bar.High,
		Low:        bar.Low,
		Volume:     bar.Volume,
		TradeCount: bar.TradeCount,
		VWAP:       bar.VWAP,
	}
}
