package alpaca

import (
	"context"
	"sync"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/edwintcloud/go-trade/internal/domain"
)

const (
	historicalSymbolsPerRequest = 250
	historicalFetchWorkers      = 4
)

func (c *Client) StreamHistoricalMinuteBars(ctx context.Context, symbols []string, start, end time.Time, out chan<- domain.Bar) error {
	defer close(out)

	if len(symbols) == 0 {
		return nil
	}

	request := marketdata.GetBarsRequest{
		TimeFrame:  marketdata.OneMin,
		Start:      start,
		End:        end,
		Feed:       marketdata.SIP,
		Sort:       marketdata.SortAsc,
		Adjustment: marketdata.AdjustmentRaw,
	}
	batches := batchSymbols(symbols, historicalSymbolsPerRequest)
	workerCount := min(historicalFetchWorkers, len(batches))
	jobs := make(chan []string)
	stop := make(chan struct{})
	var fetchErr error
	var stopOnce sync.Once
	setErr := func(err error) {
		if err == nil {
			return
		}
		stopOnce.Do(func() {
			fetchErr = err
			close(stop)
		})
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()

			for batch := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-stop:
					return
				default:
				}

				results, err := c.dataClient.GetMultiBars(batch, request)
				if err != nil {
					setErr(err)
					return
				}

				for _, symbol := range batch {
					bars := results[symbol]
					for _, bar := range bars {
						select {
						case <-ctx.Done():
							return
						case <-stop:
							return
						case out <- marketdataBarToDomainBar(symbol, bar):
						}
					}
				}
			}
		}()
	}

	dispatching := true
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			dispatching = false
		case <-stop:
			dispatching = false
		case jobs <- batch:
		}
		if !dispatching {
			break
		}
	}
	close(jobs)
	wg.Wait()

	if fetchErr != nil {
		return fetchErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

func batchSymbols(symbols []string, batchSize int) [][]string {
	batches := make([][]string, 0, (len(symbols)+batchSize-1)/batchSize)
	for start := 0; start < len(symbols); start += batchSize {
		end := min(start+batchSize, len(symbols))
		batches = append(batches, symbols[start:end])
	}
	return batches
}
