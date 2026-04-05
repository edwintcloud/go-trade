package alpaca

import (
	"fmt"
	"slices"
	"strings"

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