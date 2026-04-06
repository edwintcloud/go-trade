package domain

import (
	"time"
)

type Trade struct {
	Symbol         string
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	EntryPrice     float64
	EntryMetrics   Metrics
	CurrentPrice   float64
	CurrentMetrics Metrics
	ExitPrice      float64
	ATR            float64
	StopPrice      float64
	Quantity       uint64
}
