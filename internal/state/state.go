package state

import (
	"sync/atomic"

	"github.com/edwintcloud/go-trade/internal/domain"
)

type State struct {
	paused    atomic.Bool
	portfolio *domain.Portfolio
}

func NewState(portfolio *domain.Portfolio) *State {
	return &State{portfolio: portfolio}
}

func (s *State) IsPaused() bool {
	return s.paused.Load()
}

func (s *State) SetPaused(paused bool) {
	s.paused.Store(paused)
}
