package domain

import (
	"time"
)

type Trade struct {
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	EntryPrice     float64
	Symbol         *Symbol
	ExitPrice      float64
	StopPrice      float64
	Quantity       uint64
}
