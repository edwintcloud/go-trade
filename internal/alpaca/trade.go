package alpaca

import (
	"fmt"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
)

func (c *Client) GetSymbols() ([]string, error) {
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

	return result, nil
}
