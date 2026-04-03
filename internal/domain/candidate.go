package domain

import "time"

type Candidate struct {
	Symbol    string
	Timestamp time.Time
	LastPrice float64
	Metrics   Metrics
}
