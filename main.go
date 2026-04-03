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

var (
	SYMBOLS                     = []string{"PENG"}
	LIVE                        = true
	START_TIME                  = time.Date(2026, 3, 30, 4, 0, 0, 0, markethours.Location)
	END_TIME                    = time.Date(2026, 3, 31, 20, 0, 0, 0, markethours.Location)
	STARTING_EQUITY             = 25000.0
	DEFAULT_CHANNEL_BUFFER_SIZE = 1000
)

func init() {
	_ = godotenv.Load()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	config := config.LoadConfig()
	state := state.NewState()
	portfolio := domain.NewPortfolio(STARTING_EQUITY)

	client := alpaca.NewClient(config.AlpacaAPIKey, config.AlpacaAPISecret)
	if LIVE {
		equitySymbols, err := client.GetSymbols()
		if err != nil {
			fmt.Printf("Error getting symbols: %v\n", err)
			return
		}
		SYMBOLS = equitySymbols
	}
	symbols := domain.NewSymbols(SYMBOLS)

	// stream minute bars
	var err error
	minuteBars := make(chan domain.Bar, DEFAULT_CHANNEL_BUFFER_SIZE)
	if LIVE {
		err = client.StreamLiveMinuteBars(ctx, SYMBOLS, minuteBars)
	} else {
		err = client.StreamHistoricalMinuteBars(ctx, SYMBOLS, START_TIME, END_TIME, minuteBars)
	}
	if err != nil {
		fmt.Printf("Error streaming minute bars: %v\n", err)
		return
	}

	// start scanner
	candidates := make(chan domain.Candidate, DEFAULT_CHANNEL_BUFFER_SIZE)
	scanner := scanner.NewScanner(config, state, symbols, portfolio)

	err = scanner.Start(ctx, minuteBars, candidates)
	if err != nil {
		fmt.Printf("Error starting scanner: %v\n", err)
		return
	}

	for {
		select {
		case candidate := <-candidates:
			if LIVE {
				fmt.Printf("Received candidate: %+v\n", candidate)
			}
			entryTime := candidate.Timestamp
			proposedQuantity := portfolio.StartingEquity / candidate.LastPrice * 0.8
			stopPrice := max(candidate.LastPrice*0.95, candidate.LastPrice-candidate.Metrics.ATR*2)
			if !portfolio.HasPosition(candidate.Symbol, entryTime) {
				portfolio.EnterPosition(candidate.Symbol, entryTime, candidate.LastPrice, uint(proposedQuantity), stopPrice)
			}
		case <-ctx.Done():
			return
		case <-time.After(4 * time.Second):
			if LIVE {
				continue
			}
			fmt.Println("Stopping scanner after 4 seconds")
			portfolio.GenerateReport()
			cancel()
			return
		}
	}
}
