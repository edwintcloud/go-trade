package state

import (
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/portfolio"
)

type State struct {
	config    *config.Config
	Symbols   *domain.Symbols
	Portfolio *portfolio.Portfolio
	pausedAt  time.Time
	mu        sync.RWMutex
}

func NewState(config *config.Config, symbolList []string) *State {
	return &State{
		config:    config,
		Symbols:   domain.NewSymbols(symbolList),
		Portfolio: portfolio.NewPortfolio(config),
	}
}

func (s *State) IsPaused(date time.Time) bool {
	// pausing is effective for the rest of the day, so we check if pausedAt is set and if it's the same day as the provided date
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pausedAt.IsZero() {
		return false
	}
	return date.Format("2006-01-02") == s.pausedAt.Format("2006-01-02")
}

func (s *State) SetPaused(date time.Time, paused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if paused {
		s.pausedAt = date.In(markethours.Location)
	} else {
		s.pausedAt = time.Time{}
	}
}
