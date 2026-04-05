package alpaca

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/config"
)

type Client struct {
	config          *config.Config
	dataClient      *marketdata.Client
	streamClient    *stream.StocksClient
	streamConnected atomic.Bool
	tradeClient     *alpaca.Client
}

func NewClient(config *config.Config) *Client {
	return &Client{
		config:      config,
		tradeClient: alpaca.NewClient(alpaca.ClientOpts{
			APIKey:    config.AlpacaAPIKey,
			APISecret: config.AlpacaAPISecret,
			BaseURL:   config.PaperBaseURL,
		}),
		dataClient: marketdata.NewClient(marketdata.ClientOpts{
			APIKey:    config.AlpacaAPIKey,
			APISecret: config.AlpacaAPISecret,
		}),
		streamClient: stream.NewStocksClient(
			marketdata.SIP,
			stream.WithCredentials(config.AlpacaAPIKey, config.AlpacaAPISecret),
		),
	}
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
