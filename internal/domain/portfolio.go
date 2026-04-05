package domain

import (
	"fmt"
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/labstack/gommon/log"
)

// TODO: should subscribe to quotes for open positions to check for stop loss hits and update equity in real time
type Trade struct {
	Symbol         string
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	EntryPrice     float64
	ExitPrice      float64
	StopPrice      float64
	Quantity       uint
}

type Portfolio struct {
	startingEquity map[string]float64 // day -> starting equity
	currentEquity  float64
	openTrades     map[string]*Trade   // symbol -> open trade
	closedTrades   map[string][]Trade // day -> closed trades
	mu             sync.Mutex
	tradingBlocked   bool
}

func NewPortfolio(date time.Time, equity float64) *Portfolio {
	// TODO: should have some logic here for loading previous equity from file or database, and if not found, use the provided equity as starting equity for the day
	dayKey := date.In(markethours.Location).Format("2006-01-02")
	return &Portfolio{
		startingEquity: map[string]float64{dayKey: equity},
		currentEquity: equity,
		openTrades: make(map[string]*Trade),
		closedTrades: make(map[string][]Trade),
	}
}

func (p *Portfolio) hasOpenTrade(symbol string) bool {
	_, exists := p.openTrades[symbol]
	return exists
}

func (p *Portfolio) EnterTrade(symbol string, entryTimestamp time.Time, entryPrice float64, quantity uint, stopPrice float64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hasOpenTrade(symbol) {
		return false
	}
	p.openTrades[symbol] = &Trade{
		Symbol: symbol,
		EntryTimestamp: entryTimestamp.In(markethours.Location),
		EntryPrice: entryPrice,
		Quantity: quantity,
		StopPrice: stopPrice,
	}
	p.currentEquity -= float64(quantity) * entryPrice
	return true
}

func (p *Portfolio) ExitTrade(symbol string, exitTimestamp time.Time, exitPrice float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := exitTimestamp.Format("2006-01-02")
	if !p.hasOpenTrade(symbol) {
		return
	}
	trade := p.openTrades[symbol]
	trade.ExitTimestamp = exitTimestamp.In(markethours.Location)
	trade.ExitPrice = exitPrice
	p.closedTrades[dateKey] = append(p.closedTrades[dateKey], *trade)
	delete(p.openTrades, symbol)
	p.currentEquity += float64(trade.Quantity) * exitPrice
}

func (p *Portfolio) HasOpenTrade(symbol string, timestamp time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasOpenTrade(symbol)
}

func (p *Portfolio) UpdateStopPrice(symbol string, stopPrice float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasOpenTrade(symbol) {
		return
	}
	p.openTrades[symbol].StopPrice = stopPrice
}

func (p *Portfolio) GetCurrentEquity() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentEquity
}

func (p *Portfolio) GetTrade(symbol string, timestamp time.Time) (Trade, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hasOpenTrade(symbol) {
		return *p.openTrades[symbol], true
	}
	dateKey := timestamp.Format("2006-01-02")
	if trades, exists := p.closedTrades[dateKey]; exists {
		for _, trade := range trades {
			if trade.Symbol == symbol {
				return trade, true
			}
		}
	}
	return Trade{}, false
}

func (p *Portfolio) calculateWinRate() float64 {
	winCount := 0
	totalCount := 0
	for _, trades := range p.closedTrades {
		for _, trade := range trades {
			if !trade.ExitTimestamp.IsZero() {
				totalCount++
				if trade.ExitPrice > trade.EntryPrice {
					winCount++
				}
			}
		}
	}
	if totalCount == 0 {
		return 0
	}
	return float64(winCount) / float64(totalCount)
}

func (p *Portfolio) GenerateReport(timestamp time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := timestamp.Format("2006-01-02")
	startingEquity := p.startingEquity[dateKey]
	if startingEquity == 0 {
		log.Errorf("No starting equity found for date %s", dateKey)
	}
	fmt.Printf("\nFinal Equity: $%.2f\n", startingEquity)
	fmt.Printf("Total P/L: $%.2f\n", p.currentEquity-startingEquity)
	fmt.Printf("Return: %.2f%%\n", (p.currentEquity-startingEquity)/startingEquity*100)
	fmt.Printf("Winrate: %.2f%%\n", p.calculateWinRate()*100)
	fmt.Println("Positions:")
	for date, trades := range p.closedTrades {
		for _, trade := range trades {
			if trade.ExitTimestamp.IsZero() {
				fmt.Printf("%s - %s: Enter: %s ($%.2f), Exit: OPEN, Qty: %d\n",
					date, trade.Symbol,
					trade.EntryTimestamp.In(markethours.Location).Format("15:04"), trade.EntryPrice,
					trade.Quantity)
				continue
			}

			profitLoss := float64(trade.Quantity) * (trade.ExitPrice - trade.EntryPrice)
			fmt.Printf("%s - %s: Enter: %s ($%.2f), Exit: %s ($%.2f), Qty: %d, P/L: $%.2f\n",
				date, trade.Symbol,
				trade.EntryTimestamp.In(markethours.Location).Format("15:04"), trade.EntryPrice,
				trade.ExitTimestamp.In(markethours.Location).Format("15:04"), trade.ExitPrice,
				trade.Quantity, profitLoss)
		}
	}
	fmt.Println()
}

func (p *Portfolio) SetTradingBlocked() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tradingBlocked = true
}

func (p *Portfolio) IsTradingBlocked() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tradingBlocked
}
