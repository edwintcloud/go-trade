package portfolio

import (
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/telegram"
)

type Portfolio struct {
	config                       *config.Config
	startingEquity               map[string]float64        // day -> starting equity
	openTrades                   map[string]*domain.Trade  // symbol -> open trade
	closedTrades                 map[string][]domain.Trade // day -> closed trades
	attemptedEntries             map[string]time.Time      // symbol -> last attempted entry time
	mu                           sync.RWMutex
	broker                       *alpaca.Client
	telegram                     *telegram.TelegramNotifier
	lastDailySummaryDay          string
	dailySummarySchedulerStarted bool
}

func NewPortfolio(config *config.Config) *Portfolio {
	var telegramNotifier *telegram.TelegramNotifier
	if config.TelegramBotToken != "" && config.TelegramChatID != "" {
		telegramNotifier = telegram.NewTelegramNotifier(config)
	}
	// TODO: should have some logic here for loading previous equity from file or database, and if not found, use the provided equity as starting equity for the day
	return &Portfolio{
		startingEquity:   make(map[string]float64),
		openTrades:       make(map[string]*domain.Trade),
		closedTrades:     make(map[string][]domain.Trade),
		attemptedEntries: make(map[string]time.Time),
		config:           config,
		telegram:         telegramNotifier,
	}
}

func (p *Portfolio) SetStartingEquity(date time.Time, equity float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := date.In(markethours.Location).Format("2006-01-02")
	p.startingEquity[dateKey] = equity
}

func (p *Portfolio) SetBroker(broker *alpaca.Client) {
	p.mu.Lock()
	p.broker = broker
	p.mu.Unlock()

	now := time.Now().In(markethours.Location)
	if err := p.EnsureStartingEquity(now); err != nil {
		panic("failed to get account information from broker: " + err.Error())
	}
	if err := p.HydrateOpenTradesFromBroker(now); err != nil {
		panic("failed to hydrate broker positions: " + err.Error())
	}
}

func (p *Portfolio) EnsureStartingEquity(date time.Time) error {
	dayKey := date.In(markethours.Location).Format("2006-01-02")

	p.mu.RLock()
	_, exists := p.startingEquity[dayKey]
	broker := p.broker
	p.mu.RUnlock()
	if exists {
		return nil
	}

	if broker != nil {
		acct, err := broker.GetAccount()
		if err != nil {
			return err
		}

		p.mu.Lock()
		if _, exists := p.startingEquity[dayKey]; !exists {
			p.startingEquity[dayKey] = acct.Equity.InexactFloat64()
		}
		p.mu.Unlock()
		return nil
	}

	p.mu.Lock()
	if _, exists := p.startingEquity[dayKey]; !exists {
		p.startingEquity[dayKey] = p.lastKnownEquityLocked()
	}
	p.mu.Unlock()
	return nil
}

func (p *Portfolio) sendDailySummary(date time.Time) bool {
	if p.telegram == nil {
		return false
	}
	dayKey := date.In(markethours.Location).Format("2006-01-02")

	p.mu.RLock()
	startingEquity, exists := p.startingEquity[dayKey]
	if !exists {
		p.mu.RUnlock()
		return false
	}
	closedTrades := p.closedTrades[dayKey]
	dayPnL := p.getPnlByDate(date)
	p.mu.RUnlock()

	p.telegram.NotifyDailySummary(date, startingEquity, dayPnL, len(closedTrades))
	return true
}

func (p *Portfolio) lastKnownEquityLocked() float64 {
	latestDay := ""
	for day := range p.startingEquity {
		if day > latestDay {
			latestDay = day
		}
	}
	if latestDay == "" {
		return 0
	}

	return p.startingEquity[latestDay] + p.pnlByDayKeyLocked(latestDay)
}
