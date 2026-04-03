package alpaca

import (
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

type Client struct {
	dataClient *marketdata.Client
}

func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		dataClient: marketdata.NewClient(marketdata.ClientOpts{
			APIKey:    apiKey,
			APISecret: apiSecret,
		}),
	}
}

func (c *Client) StreamMinuteBars(symbols []string, start, end time.Time) (chan stream.Bar, error) {
	out := make(chan stream.Bar, 1000)
	results, err := c.dataClient.GetMultiBars(symbols, marketdata.GetBarsRequest{
		TimeFrame:  marketdata.OneMin,
		Start:      start,
		End:        end,
		Feed:       marketdata.SIP,
		Sort:       marketdata.SortAsc,
		Adjustment: marketdata.AdjustmentRaw,
	})
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for symbol, bars := range results {
		wg.Add(1)
		go func(symbol string, bars []marketdata.Bar) {
			defer wg.Done()
			for _, bar := range bars {
				out <- stream.Bar{
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
		}(symbol, bars)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out, nil
}
