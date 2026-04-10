package alpaca

import (
	"sync"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/config"
)

type Client struct {
	config          *config.Config
	dataClient      *marketdata.Client
	streamClient    *stream.StocksClient
	tradeClient     *alpaca.Client
	floatStore      *FloatStore
	streamConnected sync.Once
}

func NewClient(config *config.Config) *Client {
	return &Client{
		config: config,
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
		floatStore: NewFloatStore(),
	}
}
