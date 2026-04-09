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

func (c *Client) GetSymbols(ctx context.Context) ([]string, error) {
	if cachedSymbols, ok, err := readCache[[]string](symbolsCacheFile, symbolsCacheMaxAge); err == nil && ok {
		return cachedSymbols, nil
	}
	if _, err := c.floatStore.LoadOrFetchFloatData(ctx); err != nil {
		return nil, fmt.Errorf("float-store: SEC EDGAR fetch error: %w", err)
	}
	log.Printf("float-store: %d symbols with float data", c.floatStore.Len())

	assets, err := c.tradeClient.GetAssets(alpaca.GetAssetsRequest{
		Status:     "active",
		AssetClass: "us_equity",
	})
	if err != nil {
		return nil, fmt.Errorf("get symbols: %w", err)
	}

	filtered := GetFilteredTradeableAssets(assets)

	result := []string{}
	for _, a := range filtered {
		symbolFloat := c.floatStore.Get(a.Symbol)
		if symbolFloat == 0 || symbolFloat < c.config.MinFloat || symbolFloat > c.config.MaxFloat {
			continue
		}

		result = append(result, a.Symbol)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no symbols found with float between %d and %d", c.config.MinFloat, c.config.MaxFloat)
	}

	_ = writeCache(symbolsCacheFile, result)

	return result, nil
}

func (c *Client) SubmitOrder(symbol string, qty uint64, side alpaca.Side) (string, error) {
	if qty == 0 {
		return "", nil
	}

	var limitPrice float64
	quote, err := c.dataClient.GetLatestQuote(symbol, marketdata.GetLatestQuoteRequest{
		Feed: marketdata.SIP,
	})
	if err != nil {
		return "", fmt.Errorf("get latest quote for %s: %w", symbol, err)
	}

	// reject if spread is too wide to avoid bad fills in illiquid stocks
	spread := quote.AskPrice - quote.BidPrice
	if side == SideBuy && spread/quote.AskPrice > c.config.MaxSpreadPct {
		return "", fmt.Errorf("spread too wide: %.2f%%", spread/quote.AskPrice*100)
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
		return "", fmt.Errorf("place order for %s: %w", symbol, err)
	}

	log.Infof("Placed %s order for %d shares of %s at price %.2f with order id %s", side, qty, symbol, limitPrice, order.ID)
	// TODO: should add poll and retry logic here to check avg fill price and ensure order is fully filled
	return order.ID, nil
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
