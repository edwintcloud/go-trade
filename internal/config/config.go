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
	MaxPrice            float64
	MinPrice            float64
	MaxAtrp             float64
	MinRsi              float64
	MaxRsi              float64
	MinMacdRoc          float64
	MinEMA20Roc         float64
	MinRelativeVolume20 float64
	MinTradeCountAccel  float64
	MinCloseStrength    float64
	MinVwapPremiumAtr   float64
	MaxVwapPremiumAtr   float64
	// Execution parameters
	LimitOrderSlippageDollars float64
	MaxSpreadPct              float64
	MaxCapitalPerTradePct     float64
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
		MinRsi:                    30.0,
		MaxRsi:                    80.0,
		MinMacdRoc:                0.003,
		MinEMA20Roc:               0.003,
		MinRelativeVolume20:       2,
		MinTradeCountAccel:        2,
		MinCloseStrength:          0.65,
		MinVwapPremiumAtr:         0.5,
		MaxVwapPremiumAtr:         3,
		LimitOrderSlippageDollars: 0.1,
		MaxSpreadPct:              0.05,
		MaxCapitalPerTradePct:     0.5,
	}
}
