package alpaca

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/domain"
)

const (
	paperBaseURL       = "https://paper-api.alpaca.markets"
	liveBaseURL        = "https://api.alpaca.markets"
	subscribeBatchSize = 500
	subscribeBatchWait = 100 * time.Millisecond
)

type Client struct {
	dataClient      *marketdata.Client
	streamClient    *stream.StocksClient
	streamConnected atomic.Bool
	tradeClient     *alpaca.Client
}

func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		tradeClient: alpaca.NewClient(alpaca.ClientOpts{
			APIKey:    apiKey,
			APISecret: apiSecret,
			BaseURL:   paperBaseURL,
		}),
		dataClient: marketdata.NewClient(marketdata.ClientOpts{
			APIKey:    apiKey,
			APISecret: apiSecret,
		}),
		streamClient: stream.NewStocksClient(marketdata.SIP, stream.WithCredentials(apiKey, apiSecret)),
	}
}

func (c *Client) ensureConnection(ctx context.Context) error {
	if c.streamConnected.Load() {
		return nil
	}
	time.Sleep(1 * time.Second)
	err := c.streamClient.Connect(ctx)
	if err != nil {
		return err
	}
	c.streamConnected.Store(true)
	return nil
}

func (c *Client) StreamLiveMinuteBars(ctx context.Context, symbols []string, out chan<- domain.Bar) error {
	// TODO: when market is closed, stream may return no data. Need to provide some message to indicate.
	if len(symbols) == 0 {
		return nil
	}
	err := c.ensureConnection(ctx)
	if err != nil {
		return err
	}
	onBar := func(bar stream.Bar) {
		select {
		case <-ctx.Done():
			return
		case <-c.streamClient.Terminated():
			return
		case out <- streamBarToDomainBar(bar):
		default:
		}
	}
	totalBatches := (len(symbols) + subscribeBatchSize - 1) / subscribeBatchSize
	for i := 0; i < len(symbols); i += subscribeBatchSize {
		end := i + subscribeBatchSize
		if end > len(symbols) {
			end = len(symbols)
		}
		batch := symbols[i:end]
		batchNum := i/subscribeBatchSize + 1
		fmt.Println(batch)
		err = c.streamClient.SubscribeToBars(onBar, batch...)
		if err != nil {
			return err
		}
		log.Printf("stream: %s request sent for %d symbols (batch %d/%d fields=%v)", "subscribe", len(batch), batchNum, totalBatches, "bars")
		if end < len(symbols) {
			time.Sleep(subscribeBatchWait)
		}
	}
	return nil
}

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

func (c *Client) GetSymbols() ([]string, error) {
	assets, err := c.tradeClient.GetAssets(alpaca.GetAssetsRequest{
		Status:     "active",
		AssetClass: "us_equity",
	})
	if err != nil {
		return nil, fmt.Errorf("get symbols: %w", err)
	}

	filtered := []alpaca.Asset{}
	for _, a := range assets {
		if !a.Tradable {
			continue
		}
		exchange := strings.ToUpper(a.Exchange)
		if exchange == "NASDAQ" || exchange == "NYSE" {
			a.Symbol = strings.ToUpper(a.Symbol)
			filtered = append(filtered, a)
		}
	}

	slices.SortFunc(filtered, func(a, b alpaca.Asset) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	result := make([]string, len(filtered))
	for i, a := range filtered {
		result[i] = a.Symbol
	}

	return result, nil
}
