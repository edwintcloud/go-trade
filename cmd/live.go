package cmd

import (
	"context"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/execution"
	"github.com/edwintcloud/go-trade/internal/scanner"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
	"github.com/spf13/cobra"
)

func LiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Run live trading session",
		Run: func(cmd *cobra.Command, args []string) {
			runLive()
		},
	}
	return cmd
}

func runLive() {
	log.Infof("Running live trading session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := config.LoadConfig()

	client := alpaca.NewClient(config)

	// determine symbols
	equitySymbols, err := client.GetSymbols(ctx)
	if err != nil {
		log.Errorf("Error getting symbols: %v", err)
		return
	}
	symbolList := equitySymbols
	log.Infof("Loaded %d symbols...", len(symbolList))

	state := state.NewState(config, symbolList).WithClient(client)
	err = state.SeedMetrics()
	if err != nil {
		log.Errorf("Error seeding metrics: %v", err)
		return
	}

	state.Portfolio.StartDailySummaryScheduler(ctx)
	client.StreamTradeUpdatesInBackground(ctx, state.Portfolio.HandleTradeUpdate)

	// start scanner to emit candidates based on daily bar data
	canidates := make(chan string, config.ChannelBufferSize)
	scanner := scanner.NewScanner(client, config, state)
	go func() {
		err = scanner.Start(ctx, canidates)
		if err != nil {
			log.Errorf("Error starting scanner: %v", err)
			return
		}
	}()

	// blocks until context is done
	execution := execution.NewExecutionEngine(client, state)
	err = execution.MonitorCandidates(ctx, canidates)
	if err != nil {
		log.Errorf("Error monitoring candidates: %v", err)
		return
	}
}
