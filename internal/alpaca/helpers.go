package alpaca

import (
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/domain"
)

func streamBarToDomainBar(bar stream.Bar) domain.Bar {
	return domain.Bar{
		Symbol:     bar.Symbol,
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

func marketdataBarToDomainBar(symbol string, bar marketdata.Bar) domain.Bar {
	return domain.Bar{
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
