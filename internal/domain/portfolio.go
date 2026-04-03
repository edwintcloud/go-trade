package domain

import (
	"fmt"
	"sync"
	"time"

	"github.com/edwintcloud/go-trade/internal/markethours"
)

type Position struct {
	Symbol         string
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	EntryPrice     float64
	ExitPrice      float64
	StopPrice      float64
	Quantity       uint
}

type Portfolio struct {
	StartingEquity float64 // initial equity at the begining of the day
	Equity         float64
	Positions      map[string][]Position     // day -> position
	PositionsIndex map[string]map[string]int // day -> symbol -> position index
	mu             sync.Mutex
}

func NewPortfolio(equity float64) *Portfolio {
	return &Portfolio{
		StartingEquity: equity,
		Equity:         equity,
		Positions:      make(map[string][]Position),
		PositionsIndex: make(map[string]map[string]int),
	}
}

func (p *Portfolio) EnterPosition(symbol string, entryTimestamp time.Time, entryPrice float64, quantity uint, stopPrice float64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := entryTimestamp.Format("2006-01-02")
	if p.inPosition(symbol, entryTimestamp) {
		return false
	}
	if p.PositionsIndex[dateKey] == nil {
		p.StartingEquity = p.Equity
		p.PositionsIndex[dateKey] = make(map[string]int)
	}
	p.Positions[dateKey] = append(p.Positions[dateKey], Position{
		Symbol:         symbol,
		EntryTimestamp: entryTimestamp,
		EntryPrice:     entryPrice,
		Quantity:       quantity,
		StopPrice:      stopPrice,
	})
	index := len(p.Positions[dateKey]) - 1
	p.PositionsIndex[dateKey][symbol] = index
	return true
}

func (p *Portfolio) ExitPosition(symbol string, exitTimestamp time.Time, exitPrice float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := exitTimestamp.Format("2006-01-02")
	index, exists := p.PositionsIndex[dateKey][symbol]
	if !exists {
		return
	}
	position := &p.Positions[dateKey][index]
	if !position.ExitTimestamp.IsZero() {
		return
	}
	position.ExitTimestamp = exitTimestamp
	position.ExitPrice = exitPrice
	p.Equity += float64(position.Quantity) * (position.ExitPrice - position.EntryPrice)
}

func (p *Portfolio) HasPosition(symbol string, timestamp time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inPosition(symbol, timestamp)
}

func (p *Portfolio) inPosition(symbol string, timestamp time.Time) bool {
	dateKey := timestamp.Format("2006-01-02")
	index, exists := p.PositionsIndex[dateKey][symbol]
	if !exists {
		return false
	}
	position := p.Positions[dateKey][index]
	return position.ExitTimestamp.IsZero()
}

func (p *Portfolio) UpdateStopPrice(symbol string, timestamp time.Time, stopPrice float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := timestamp.Format("2006-01-02")
	index, exists := p.PositionsIndex[dateKey][symbol]
	if !exists {
		return
	}
	position := &p.Positions[dateKey][index]
	position.StopPrice = stopPrice
}

func (p *Portfolio) GetPosition(symbol string, timestamp time.Time) (Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dateKey := timestamp.Format("2006-01-02")
	index, exists := p.PositionsIndex[dateKey][symbol]
	if !exists {
		return Position{}, false
	}
	return p.Positions[dateKey][index], true
}

func (p *Portfolio) calculateWinRate() float64 {
	winCount := 0
	totalCount := 0
	for _, positions := range p.Positions {
		for _, position := range positions {
			if !position.ExitTimestamp.IsZero() {
				totalCount++
				if position.ExitPrice > position.EntryPrice {
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

func (p *Portfolio) GenerateReport() {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Printf("Final Equity: $%.2f\n", p.Equity)
	fmt.Printf("Total P/L: $%.2f\n", p.Equity-p.StartingEquity)
	fmt.Printf("Return: %.2f%%\n", (p.Equity-p.StartingEquity)/p.StartingEquity*100)
	fmt.Printf("Winrate: %.2f%%\n", p.calculateWinRate()*100)
	fmt.Println("Positions:")
	for date, positions := range p.Positions {
		for _, position := range positions {
			if position.ExitTimestamp.IsZero() {
				fmt.Printf("%s - %s: Enter: %s ($%.2f), Exit: OPEN, Qty: %d\n",
					date, position.Symbol,
					position.EntryTimestamp.In(markethours.Location).Format("15:04"), position.EntryPrice,
					position.Quantity)
				continue
			}

			profitLoss := float64(position.Quantity) * (position.ExitPrice - position.EntryPrice)
			fmt.Printf("%s - %s: Enter: %s ($%.2f), Exit: %s ($%.2f), Qty: %d, P/L: $%.2f\n",
				date, position.Symbol,
				position.EntryTimestamp.In(markethours.Location).Format("15:04"), position.EntryPrice,
				position.ExitTimestamp.In(markethours.Location).Format("15:04"), position.ExitPrice,
				position.Quantity, profitLoss)
		}
	}
}

func (p *Portfolio) TradingBlocked() bool {
	maxLossCondition := p.Equity < p.StartingEquity*0.9     // 10% max daily loss
	profitGoalCondition := p.Equity > p.StartingEquity*1.05 // 5% profit goal
	return maxLossCondition || profitGoalCondition
}
