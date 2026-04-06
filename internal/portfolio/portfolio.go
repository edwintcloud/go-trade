package portfolio

import (
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/config"
)

type Portfolio struct {
	config         *config.Config
	startingEquity map[string]float64 // day -> starting equity
	openTrades     map[string]*Trade  // symbol -> open trade
	closedTrades   map[string][]Trade // day -> closed trades
	mu             sync.RWMutex
}

func NewPortfolio(config *config.Config) *Portfolio {
	// TODO: should have some logic here for loading previous equity from file or database, and if not found, use the provided equity as starting equity for the day
	return &Portfolio{
		startingEquity: make(map[string]float64),
		openTrades:     make(map[string]*Trade),
		closedTrades:   make(map[string][]Trade),
		config:         config,
	}
}

func (p *Portfolio) SetStartingEquity(date time.Time, equity float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := date.Format("2006-01-02")
	p.startingEquity[dateKey] = equity
}
