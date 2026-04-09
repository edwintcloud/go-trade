package portfolio

import (
	"fmt"
	"sort"
	"strings"

	"github.com/edwintcloud/go-trade/internal/markethours"
)

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

func (p *Portfolio) generateDailyReport(date string) float64 {
	trades, exists := p.closedTrades[date]
	if !exists {
		fmt.Printf("No trades for %s\n", date)
		return 0.0
	}
	startingEquity := p.startingEquity[date]
	endingEquity := startingEquity
	for _, trade := range trades {
		if !trade.ExitTimestamp.IsZero() {
			profitLoss := float64(trade.Quantity) * (trade.ExitPrice - trade.EntryPrice)
			endingEquity += profitLoss
		}
	}

	for _, trade := range trades {
		exitTime := "OPEN"
		if !trade.ExitTimestamp.IsZero() {
			exitTime = trade.ExitTimestamp.In(markethours.Location).Format("15:04")
		}
		profitLoss := float64(trade.Quantity) * (trade.ExitPrice - trade.EntryPrice)
		fmt.Printf("%s - %s: Enter: %s ($%.2f), Exit: %s ($%.2f), Qty: %d, P/L: $%.2f, ATR: %.2f\n",
			date, trade.Symbol,
			trade.EntryTimestamp.In(markethours.Location).Format("15:04"), trade.EntryPrice,
			exitTime, trade.ExitPrice,
			trade.Quantity, profitLoss,
			trade.EntryMetrics.ATR,
		)
	}

	returnPct := (endingEquity - startingEquity) / startingEquity * 100
	fmt.Printf("\n\t%d trades, Starting Equity: $%.2f, Ending Equity: $%.2f, Total P/L: $%.2f, Return: %.2f%%, Winrate: %.2f%%\n",
		len(trades), startingEquity, endingEquity, endingEquity-startingEquity, returnPct, p.calculateWinRate()*100)

	return returnPct
}

func (p *Portfolio) GenerateReport() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	dates := make([]string, 0, len(p.closedTrades))
	for date := range p.closedTrades {
		dates = append(dates, date)
	}
	// sort dates
	sort.Strings(dates)

	totalReturnPct := 0.0
	for _, dateKey := range dates {
		fmt.Println(strings.Repeat("-", 150))
		totalReturnPct += p.generateDailyReport(dateKey)
		fmt.Println(strings.Repeat("=", 150))
	}
	fmt.Printf("Overall Return: %.2f%%\n", totalReturnPct)
}
