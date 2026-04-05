package alpaca

import (
	"context"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/edwintcloud/go-trade/internal/domain"
)

func (c *Client) StreamHistoricalMinuteBars(ctx context.Context, symbols []string, start, end time.Time, out chan<- domain.Bar) error {
	results, err := c.dataClient.GetMultiBars(symbols, marketdata.GetBarsRequest{
		TimeFrame:  marketdata.OneMin,
		Start:      start,
		End:        end,
		Feed:       marketdata.SIP,
		Sort:       marketdata.SortAsc,
		Adjustment: marketdata.AdjustmentRaw,
	})
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for symbol, bars := range results {
		wg.Add(1)
		go func(symbol string, bars []marketdata.Bar) {
			defer wg.Done()
			for _, bar := range bars {
				select {
				case <-ctx.Done():
					return
				default:
				}
				out <- marketdataBarToDomainBar(symbol, bar)
			}
		}(symbol, bars)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return nil
}