package domain

import (
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/markethours"
)

const volumeLeaderCount = 10

type Symbols struct {
	mapping          map[string]*Symbol
	volumeLeaders    map[string]bool
	volumeLeaderList []SymbolVolumeMapping
	volumeLeaderDay  string
	mu               sync.RWMutex
}

func (s *Symbols) Get(symbolName string) (*Symbol, bool) {
	symbol, ok := s.mapping[symbolName]
	return symbol, ok
}

func NewSymbols(symbolNames []string) *Symbols {
	symbols := &Symbols{
		mapping:       make(map[string]*Symbol),
		volumeLeaders: make(map[string]bool),
	}
	for _, name := range symbolNames {
		symbols.mapping[name] = NewSymbol(name)
	}
	return symbols
}

func (s *Symbols) IsSymbolVolumeLeader(symbolName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.volumeLeaders[symbolName]
	return ok
}

func (s *Symbols) UpdateVolumeLeaders(symbolName string, volume uint64, timestamp time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dayKey := timestamp.In(markethours.Location).Format("2006-01-02")
	if s.volumeLeaderDay != dayKey {
		s.volumeLeaderDay = dayKey
		s.volumeLeaderList = s.volumeLeaderList[:0]
		clear(s.volumeLeaders)
	}

	if idx := s.findVolumeLeader(symbolName); idx >= 0 {
		s.volumeLeaderList[idx].Volume = volume
		s.reorderVolumeLeader(idx)
		return
	}

	if len(s.volumeLeaderList) < volumeLeaderCount {
		s.volumeLeaderList = append(s.volumeLeaderList, SymbolVolumeMapping{
			SymbolName: symbolName,
			Volume:     volume,
		})
		s.reorderVolumeLeader(len(s.volumeLeaderList) - 1)
		s.rebuildVolumeLeaderSet()
		return
	}

	if volume <= s.volumeLeaderList[0].Volume {
		return
	}

	s.volumeLeaderList[0] = SymbolVolumeMapping{
		SymbolName: symbolName,
		Volume:     volume,
	}
	s.reorderVolumeLeader(0)
	s.rebuildVolumeLeaderSet()
}

func (s *Symbols) findVolumeLeader(symbolName string) int {
	for i, leader := range s.volumeLeaderList {
		if leader.SymbolName == symbolName {
			return i
		}
	}

	return -1
}

func (s *Symbols) reorderVolumeLeader(idx int) {
	for idx > 0 && volumeLeaderLess(s.volumeLeaderList[idx], s.volumeLeaderList[idx-1]) {
		s.volumeLeaderList[idx], s.volumeLeaderList[idx-1] = s.volumeLeaderList[idx-1], s.volumeLeaderList[idx]
		idx--
	}

	for idx+1 < len(s.volumeLeaderList) && volumeLeaderLess(s.volumeLeaderList[idx+1], s.volumeLeaderList[idx]) {
		s.volumeLeaderList[idx], s.volumeLeaderList[idx+1] = s.volumeLeaderList[idx+1], s.volumeLeaderList[idx]
		idx++
	}
}

func (s *Symbols) rebuildVolumeLeaderSet() {
	clear(s.volumeLeaders)
	for _, leader := range s.volumeLeaderList {
		s.volumeLeaders[leader.SymbolName] = true
	}
}

func volumeLeaderLess(left, right SymbolVolumeMapping) bool {
	if left.Volume != right.Volume {
		return left.Volume < right.Volume
	}

	return left.SymbolName < right.SymbolName
}
