package config

import (
	"os"
)

const (
	DEFAULT_CHANNEL_BUFFER_SIZE = 240
)

type Config struct {
	AlpacaAPIKey    string
	AlpacaAPISecret string
}

func LoadConfig() *Config {
	return &Config{
		AlpacaAPIKey: os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret: os.Getenv("ALPACA_API_SECRET"),
	}
}
