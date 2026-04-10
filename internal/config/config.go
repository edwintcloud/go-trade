package config

import (
	"os"
)

const (
	DEFAULT_CHANNEL_BUFFER_SIZE = 240
)

type Config struct {
	AlpacaAPIKey      string
	AlpacaAPISecret   string
	TelegramBotToken  string
	TelegramChatID    string
	ChannelBufferSize uint64
	PaperBaseURL      string
	LiveBaseURL       string
	// Risk management parameters
	MaxTradesPerDay           uint64
	SameSymbolCooldownMinutes uint64
	MinutesUntilBreakEvenStop uint64
	MaxLossPerDayPct          float64
	TrailingStopAtrMultiplier float64
	TrailingStopPctFallback   float64
	MinPositionSizePct        float64
	DailyProfitTargetPct      float64
	// Scanning parameters
	MinAverageVolume5Min uint64
	MinFloat             uint64
	MaxFloat             uint64
	MinPrice             float64
	MaxPrice             float64
	// Execution parameters
	LimitOrderSlippageDollars float64
	MaxSpreadPct              float64
	MaxCapitalPerTradePct     float64
}

func LoadConfig() *Config {
	return &Config{
		AlpacaAPIKey:              os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret:           os.Getenv("ALPACA_API_SECRET"),
		TelegramBotToken:          os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:            os.Getenv("TELEGRAM_CHAT_ID"),
		ChannelBufferSize:         DEFAULT_CHANNEL_BUFFER_SIZE,
		PaperBaseURL:              "https://paper-api.alpaca.markets",
		LiveBaseURL:               "https://api.alpaca.markets",
		MaxTradesPerDay:           30,
		SameSymbolCooldownMinutes: 30,
		MinutesUntilBreakEvenStop: 10,
		MaxLossPerDayPct:          0.03,
		TrailingStopAtrMultiplier: 1.5,
		TrailingStopPctFallback:   0.05,
		MinPositionSizePct:        0.3,
		DailyProfitTargetPct:      0.1,
		MinAverageVolume5Min:      5_000,
		MinFloat:                  1_000_000,
		MaxFloat:                  50_000_000,
		MinPrice:                  3.5,
		MaxPrice:                  40.0,
		LimitOrderSlippageDollars: 0.1,
		MaxSpreadPct:              0.01, // 3 cent for a $3 stock, 10 cents for a $10 stock
		MaxCapitalPerTradePct:     0.5,
	}
}
