package state

import (
	"sync/atomic"

	"github.com/edwintcloud/go-trade/internal/domain"
)

type State struct {
	paused    atomic.Bool
	Portfolio *domain.Portfolio
	Symbols  *domain.Symbols
}

func NewState(portfolio *domain.Portfolio, symbols *domain.Symbols) *State {
	return &State{
		Portfolio: portfolio,
		Symbols:  symbols,
	}
}

func (s *State) IsPaused() bool {
	return s.paused.Load()
}

func (s *State) SetPaused(paused bool) {
	s.paused.Store(paused)
}
