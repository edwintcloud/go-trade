package alpaca

import (
	"context"
	"fmt"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/labstack/gommon/log"
	"github.com/shopspring/decimal"
)

var (
	SideBuy  = alpaca.Buy
	SideSell = alpaca.Sell
)

func (c *Client) GetSymbols() ([]string, error) {
	if cachedSymbols, ok, err := readCache[[]string](symbolsCacheFile, symbolsCacheMaxAge); err == nil && ok {
		return cachedSymbols, nil
	}

	assets, err := c.tradeClient.GetAssets(alpaca.GetAssetsRequest{
		Status:     "active",
		AssetClass: "us_equity",
	})
	if err != nil {
		return nil, fmt.Errorf("get symbols: %w", err)
	}

	filtered := GetFilteredTradeableAssets(assets)

	result := make([]string, len(filtered))
	for i, a := range filtered {
		result[i] = a.Symbol
	}

	_ = writeCache(symbolsCacheFile, result)

	return result, nil
}

func (c *Client) SubmitOrder(symbol string, qty uint64, side alpaca.Side) (*alpaca.Order, error) {
	if qty == 0 {
		return nil, nil
	}

	var limitPrice float64
	quote, err := c.dataClient.GetLatestQuote(symbol, marketdata.GetLatestQuoteRequest{
		Feed: marketdata.SIP,
	})
	if err != nil {
		return nil, fmt.Errorf("get latest quote for %s: %w", symbol, err)
	}

	// reject if spread is too wide to avoid bad fills in illiquid stocks
	spread := quote.AskPrice - quote.BidPrice
	if side == SideBuy && spread/quote.AskPrice > c.config.MaxSpreadPct {
		return nil, fmt.Errorf("spread too wide: %.2f%%", spread/quote.AskPrice*100)
	}

	// base limit price on current quote
	if side == SideSell {
		limitPrice = quote.BidPrice - c.config.LimitOrderSlippageDollars
	} else {
		limitPrice = quote.AskPrice + c.config.LimitOrderSlippageDollars
	}

	decimalQty := decimal.NewFromInt(int64(qty))
	order, err := c.tradeClient.PlaceOrder(alpaca.PlaceOrderRequest{
		Symbol:        symbol,
		Qty:           &decimalQty,
		Side:          side,
		Type:          alpaca.Limit,
		LimitPrice:    alpaca.RoundLimitPrice(decimal.NewFromFloat(limitPrice), side),
		TimeInForce:   alpaca.Day,
		ExtendedHours: true,
	})
	if err != nil {
		return nil, fmt.Errorf("place order for %s: %w", symbol, err)
	}

	log.Infof("Filled %s order for %d shares of %s at average price %.2f", side, qty, symbol, order.FilledAvgPrice.InexactFloat64())
	return order, nil
}

func (c *Client) GetAccount() (*alpaca.Account, error) {
	return c.tradeClient.GetAccount()
}

func (c *Client) GetPositions() ([]alpaca.Position, error) {
	return c.tradeClient.GetPositions()
}

func (c *Client) GetPosition(symbol string) (*alpaca.Position, error) {
	return c.tradeClient.GetPosition(symbol)
}

func (c *Client) StreamTradeUpdatesInBackground(ctx context.Context, handler func(alpaca.TradeUpdate)) {
	c.tradeClient.StreamTradeUpdatesInBackground(ctx, handler)
}
