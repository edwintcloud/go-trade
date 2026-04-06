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
	// Risk management parameters
	MaxTradesPerDay           int
	MaxLossPerDayPct          float64
	TrailingStopAtrMultiplier float64
	TrailingStopPctFallback   float64
	SameSymbolCooldownMinutes int
	MinPositionSizePct        float64
	MinutesUntilBreakEvenStop int
	DailyProfitTargetPct      float64
	// Scanning parameters
	MaxPrice float64
	MinPrice float64
	MaxAtrp  float64
	// Execution parameters
	LimitOrderSlippageDollars float64
	MaxSpreadPct              float64
}

func LoadConfig() *Config {
	return &Config{
		AlpacaAPIKey:              os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret:           os.Getenv("ALPACA_API_SECRET"),
		ChannelBufferSize:         DEFAULT_CHANNEL_BUFFER_SIZE,
		PaperBaseURL:              "https://paper-api.alpaca.markets",
		LiveBaseURL:               "https://api.alpaca.markets",
		SubscribeBatchSize:        500,
		SubscribeBatchWait:        100 * time.Millisecond,
		MaxTradesPerDay:           20,
		MaxLossPerDayPct:          0.1,
		TrailingStopAtrMultiplier: 1.5,
		TrailingStopPctFallback:   0.05,
		SameSymbolCooldownMinutes: 30,
		MinPositionSizePct:        0.3,
		MinutesUntilBreakEvenStop: 7,
		DailyProfitTargetPct:      0.05,
		MaxPrice:                  40.0,
		MinPrice:                  3.5,
		MaxAtrp:                   0.04,
		LimitOrderSlippageDollars: 0.1,
		MaxSpreadPct:              0.05,
	}
}
