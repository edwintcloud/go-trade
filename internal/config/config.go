package config

import (
	"os"
	"time"
)

const (
	DEFAULT_CHANNEL_BUFFER_SIZE = 240
)

type Config struct {
	AlpacaAPIKey       string
	AlpacaAPISecret    string
	ChannelBufferSize  int
	PaperBaseURL       string
	LiveBaseURL        string
	SubscribeBatchSize int
	SubscribeBatchWait time.Duration
}

func LoadConfig() *Config {
	return &Config{
		AlpacaAPIKey:       os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret:    os.Getenv("ALPACA_API_SECRET"),
		ChannelBufferSize:  DEFAULT_CHANNEL_BUFFER_SIZE,
		PaperBaseURL:       "https://paper-api.alpaca.markets",
		LiveBaseURL:        "https://api.alpaca.markets",
		SubscribeBatchSize: 500,
		SubscribeBatchWait: 100 * time.Millisecond,
	}
}
