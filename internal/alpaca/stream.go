package alpaca

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/domain"
)

func (c *Client) ensureStreamConnected(ctx context.Context) error {
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
	err := c.ensureStreamConnected(ctx)
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
	totalBatches := (len(symbols) + c.config.SubscribeBatchSize - 1) / c.config.SubscribeBatchSize
	for i := 0; i < len(symbols); i += c.config.SubscribeBatchSize {
		end := i + c.config.SubscribeBatchSize
		if end > len(symbols) {
			end = len(symbols)
		}
		batch := symbols[i:end]
		batchNum := i/c.config.SubscribeBatchSize + 1
		fmt.Println(batch)
		err = c.streamClient.SubscribeToBars(onBar, batch...)
		if err != nil {
			return err
		}
		log.Printf("stream: %s request sent for %d symbols (batch %d/%d fields=%v)", "subscribe", len(batch), batchNum, totalBatches, "bars")
		if end < len(symbols) {
			time.Sleep(c.config.SubscribeBatchWait)
		}
	}
	return nil
}