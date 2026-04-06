package alpaca

import (
	"context"
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
	if len(symbols) == 0 {
		return nil
	}
	for {
		if err := c.ensureStreamConnected(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("stream: connect failed: %v", err)
			c.streamClient = newStocksStreamClient(c.config)
			c.streamConnected.Store(false)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		streamClient := c.streamClient
		onBar := func(bar stream.Bar) {
			select {
			case <-ctx.Done():
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
			if err := streamClient.SubscribeToBars(onBar, batch...); err != nil {
				c.streamClient = newStocksStreamClient(c.config)
				c.streamConnected.Store(false)
				return err
			}
			log.Printf("stream: %s request sent for %d symbols (batch %d/%d fields=%v)", "subscribe", len(batch), batchNum, totalBatches, "bars")
			if end < len(symbols) {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(c.config.SubscribeBatchWait):
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-streamClient.Terminated():
			c.streamClient = newStocksStreamClient(c.config)
			c.streamConnected.Store(false)
			if ctx.Err() != nil {
				return nil
			}
			if ok {
				log.Printf("stream: terminated, reconnecting: %v", err)
			} else {
				log.Printf("stream: terminated, reconnecting")
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}
}
