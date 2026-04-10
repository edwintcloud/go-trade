package alpaca

import (
	"context"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

func (c *Client) ensureStreamConnected(ctx context.Context) error {
	var err error
	c.streamConnected.Do(func() {
		err = c.streamClient.Connect(ctx)
	})
	return err
}

func (c *Client) SubscribeToDailyBars(ctx context.Context, handler func(stream.Bar), symbols ...string) error {
	if len(symbols) == 0 {
		return nil
	}
	if err := c.ensureStreamConnected(ctx); err != nil {
		return err
	}
	return c.streamClient.SubscribeToDailyBars(handler, symbols...)
}

func (c *Client) SubscribeToBars(ctx context.Context, handler func(stream.Bar), symbols ...string) error {
	if len(symbols) == 0 {
		return nil
	}
	if err := c.ensureStreamConnected(ctx); err != nil {
		return err
	}
	return c.streamClient.SubscribeToBars(handler, symbols...)
}

func (c *Client) UnsubscribeFromBars(symbols ...string) error {
	return c.streamClient.UnsubscribeFromBars(symbols...)
}

func (c *Client) SubscribeQuotes(ctx context.Context, handler func(stream.Quote), symbols ...string) error {
	if len(symbols) == 0 {
		return nil
	}
	if err := c.ensureStreamConnected(ctx); err != nil {
		return err
	}
	return c.streamClient.SubscribeToQuotes(handler, symbols...)
}

func (c *Client) UnsubscribeQuotes(symbols ...string) error {
	return c.streamClient.UnsubscribeFromQuotes(symbols...)
}
