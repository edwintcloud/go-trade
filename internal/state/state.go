package state

import (
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/portfolio"
	"github.com/labstack/gommon/log"
)

type State struct {
	client     *alpaca.Client
	config     *config.Config
	SymbolList []string
	Symbols    *domain.Symbols
	Portfolio  *portfolio.Portfolio
	pausedAt   time.Time
	mu         sync.RWMutex
}

func NewState(config *config.Config, symbolList []string) *State {
	return &State{
		config:     config,
		SymbolList: symbolList,
		Symbols:    domain.NewSymbols(symbolList),
		Portfolio:  portfolio.NewPortfolio(config),
	}
}

func (s *State) WithClient(client *alpaca.Client) *State {
	s.client = client
	s.Portfolio.SetBroker(client)
	return s
}

// seeds metrics for all symbols with historical data from the last 4 hours
func (s *State) SeedMetrics() error {
	end := time.Now().In(markethours.Location)
	start := markethours.Add(end, -4*time.Hour)

	log.Infof("Seeding metrics with historical minute bars from %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	barsBySymbol, err := s.client.FetchHistoricalMinuteBars(s.SymbolList, start, end)
	if err != nil {
		return err
	}

	for symbol, bars := range barsBySymbol {
		domainSymbol, ok := s.Symbols.Get(symbol)
		if !ok {
			continue
		}
		for _, bar := range bars {
			domainSymbol.AddBar(domain.Bar{
				Symbol:     symbol,
				Open:       bar.Open,
				High:       bar.High,
				Low:        bar.Low,
				Close:      bar.Close,
				Volume:     bar.Volume,
				Timestamp:  bar.Timestamp,
				TradeCount: bar.TradeCount,
				VWAP:       bar.VWAP,
			})
		}
	}

	log.Infof("Finished seeding metrics for %d symbols", len(barsBySymbol))

	return nil
}

func (s *State) IsPaused(date time.Time) bool {
	// pausing is effective for the rest of the day, so we check if pausedAt is set and if it's the same day as the provided date
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pausedAt.IsZero() {
		return false
	}
	return date.In(markethours.Location).Format("2006-01-02") == s.pausedAt.Format("2006-01-02")
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
