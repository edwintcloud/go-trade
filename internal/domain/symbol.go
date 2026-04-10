package domain

import (
	"sync"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
)

type Symbol struct {
	Name            string
	BidPrice        float64
	AskPrice        float64
	bars            chan Bar
	metrics         Metrics
	metricsInitOnce sync.Once
	mu              sync.RWMutex
}

func NewSymbol(name string) *Symbol {
	s := &Symbol{
		Name: name,
		bars: make(chan Bar, 240), // buffer to hold up to 4 hours of 1-minute bars
	}
	return s
}

func (s *Symbol) UpdateSymbolWithQuote(quote stream.Quote) {
	s.mu.Lock()
	s.metrics.BidAskSpreadPct = (quote.AskPrice - quote.BidPrice) / quote.AskPrice
	s.BidPrice = quote.BidPrice
	s.AskPrice = quote.AskPrice
	s.mu.Unlock()
}

func (s *Symbol) AddBar(bar Bar) {
	s.ensureMetricsInitialized()
	s.mu.Lock()
	select {
	case s.bars <- bar:
	default:
		// channel is full, drain one item to make space
		<-s.bars
		s.bars <- bar
	}
	s.mu.Unlock()
}
