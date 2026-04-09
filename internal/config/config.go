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
	TelegramBotToken   string
	TelegramChatID     string
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
	MinOneMinuteVolume  uint64
	MinFloat            int64
	MaxFloat            int64
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
	MinVolume5m         float64
	MaxVwapPremiumAtr   float64
	MaxEmaPremiumAtr    float64
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
		SubscribeBatchSize:        500,
		SubscribeBatchWait:        100 * time.Millisecond,
		MaxTradesPerDay:           10,
		MaxLossPerDayPct:          0.03,
		TrailingStopAtrMultiplier: 1.5,
		TrailingStopPctFallback:   0.05,
		SameSymbolCooldownMinutes: 30,
		MinPositionSizePct:        0.3,
		MinutesUntilBreakEvenStop: 10,
		DailyProfitTargetPct:      0.1,
		MinOneMinuteVolume:        1000,
		MinFloat:                  1_000_000,
		MaxFloat:                  50_000_000,
		MinPrice:                  3.5,
		MaxPrice:                  40.0,
		MaxAtrp:                   0.03,
		MinRsi:                    30.0,
		MaxRsi:                    70.0,
		MinMacdRoc:                0.005,
		MinEMA20Roc:               0.009,
		MinRelativeVolume20:       3,
		MinTradeCountAccel:        3,
		MinCloseStrength:          0,
		MinVolume5m:               5000,
		MinVwapPremiumAtr:         0.5,
		MaxVwapPremiumAtr:         3,
		MaxEmaPremiumAtr:          3,
		LimitOrderSlippageDollars: 0.1,
		MaxSpreadPct:              0.05,
		MaxCapitalPerTradePct:     0.5,
	}
}
