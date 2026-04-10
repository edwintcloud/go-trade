package alpaca

import (
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

func (c *Client) FetchHistoricalMinuteBars(symbols []string, start, end time.Time) (map[string][]marketdata.Bar, error) {
	request := marketdata.GetBarsRequest{
		TimeFrame:  marketdata.OneMin,
		Start:      start,
		End:        end,
		Feed:       marketdata.SIP,
		Sort:       marketdata.SortAsc,
		Adjustment: marketdata.AdjustmentRaw,
	}
	cachePath, cachePathErr := historicalBarsCachePath(symbols, request)
	if cachePathErr == nil {
		if cachedResults, ok, err := readCache[map[string][]marketdata.Bar](cachePath, 0); err == nil && ok {
			return cachedResults, nil
		}
	}

	results, err := c.dataClient.GetMultiBars(symbols, request)
	if err != nil {
		return nil, err
	}

	if cachePathErr == nil {
		_ = writeCache(cachePath, results)
	}

	return results, nil
}
