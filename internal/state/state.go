package state

import (
	"sync/atomic"
)

type State struct {
	paused atomic.Bool
}

func NewState() *State {
	return &State{}
}

func (s *State) IsPaused() bool {
	return s.paused.Load()
}

func (s *State) SetPaused(paused bool) {
	s.paused.Store(paused)
}
