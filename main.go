package main

import (
	"context"
	"fmt"
	"time"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/scanner"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/joho/godotenv"
)

func init() {
	_ = godotenv.Load()
}

func main() {
	symbolNames := []string{"ARTL"}
	symbols := domain.NewSymbols(symbolNames)

	config := config.LoadConfig()
	state := state.NewState()

	client := alpaca.NewClient(config.AlpacaAPIKey, config.AlpacaAPISecret)

	startTime := time.Date(2026, 3, 30, 4, 0, 0, 0, markethours.Location)
	endTime := time.Date(2026, 3, 31, 20, 0, 0, 0, markethours.Location)

	minuteBars, err := client.StreamMinuteBars(symbolNames, startTime, endTime)
	if err != nil {
		fmt.Printf("Error streaming minute bars: %v\n", err)
		return
	}

	portfolio := domain.NewPortfolio(25000)
	candidates := make(chan domain.Candidate, 1000)

	scanner := scanner.NewScanner(config, state, symbols, portfolio)

	ctx, cancel := context.WithCancel(context.Background())
	err = scanner.Start(ctx, minuteBars, candidates)
	if err != nil {
		fmt.Printf("Error starting scanner: %v\n", err)
		return
	}

	for {
		select {
		case candidate := <-candidates:
			entryTime := time.Now()
			proposedQuantity := portfolio.StartingEquity / candidate.LastPrice * 0.8
			stopPrice := max(candidate.LastPrice*0.95, candidate.LastPrice-candidate.Metrics.ATR*1.5)
			if !portfolio.HasPosition(candidate.Symbol, entryTime) {
				portfolio.EnterPosition(candidate.Symbol, entryTime, candidate.LastPrice, uint(proposedQuantity), stopPrice)
			}
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
			fmt.Println("Stopping scanner after 2 seconds")
			portfolio.GenerateReport()
			cancel()
			return
		}
	}
}
