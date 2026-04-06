package alpaca

import (
	"container/heap"
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
	resultsBySymbol := make(map[string][]marketdata.Bar, len(symbols))
	var fetchErr error
	var resultsMu sync.Mutex
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

				resultsMu.Lock()
				for _, symbol := range batch {
					resultsBySymbol[symbol] = results[symbol]
				}
				resultsMu.Unlock()
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

	return streamHistoricalBarsInOrder(ctx, symbols, resultsBySymbol, out)
}

func batchSymbols(symbols []string, batchSize int) [][]string {
	batches := make([][]string, 0, (len(symbols)+batchSize-1)/batchSize)
	for start := 0; start < len(symbols); start += batchSize {
		end := min(start+batchSize, len(symbols))
		batches = append(batches, symbols[start:end])
	}
	return batches
}

func streamHistoricalBarsInOrder(ctx context.Context, symbols []string, resultsBySymbol map[string][]marketdata.Bar, out chan<- domain.Bar) error {
	h := &historicalBarMinHeap{}
	heap.Init(h)

	for _, symbol := range symbols {
		bars := resultsBySymbol[symbol]
		if len(bars) == 0 {
			continue
		}
		heap.Push(h, historicalBarCursor{
			symbol: symbol,
			bars:   bars,
		})
	}

	for h.Len() > 0 {
		cursor := heap.Pop(h).(historicalBarCursor)
		bar := cursor.bars[cursor.index]

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- marketdataBarToDomainBar(cursor.symbol, bar):
		}

		cursor.index++
		if cursor.index < len(cursor.bars) {
			heap.Push(h, cursor)
		}
	}

	return nil
}

type historicalBarCursor struct {
	symbol string
	bars   []marketdata.Bar
	index  int
}

type historicalBarMinHeap []historicalBarCursor

func (h historicalBarMinHeap) Len() int { return len(h) }

func (h historicalBarMinHeap) Less(i, j int) bool {
	left := h[i].bars[h[i].index]
	right := h[j].bars[h[j].index]

	if !left.Timestamp.Equal(right.Timestamp) {
		return left.Timestamp.Before(right.Timestamp)
	}

	if h[i].symbol != h[j].symbol {
		return h[i].symbol < h[j].symbol
	}

	return h[i].index < h[j].index
}

func (h historicalBarMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *historicalBarMinHeap) Push(x any) {
	*h = append(*h, x.(historicalBarCursor))
}

func (h *historicalBarMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
