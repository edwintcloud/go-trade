package cmd

import (
	"context"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
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
	equitySymbols, err := client.GetSymbols()
	if err != nil {
		log.Errorf("Error getting symbols: %v", err)
		return
	}
	symbolList := equitySymbols
	log.Infof("Loaded %d symbols...", len(symbolList))

	state := state.NewState(config, symbolList)

	state.Portfolio.SetBroker(client)

	// start scanner before historical loading so fetching and scanning can overlap.
	minuteBars := make(chan domain.Bar, config.ChannelBufferSize)
	candidates := make(chan domain.Candidate, config.ChannelBufferSize)
	scanner := scanner.NewScanner(config, state)

	done, err := scanner.Start(ctx, minuteBars, candidates)
	if err != nil {
		log.Errorf("Error starting scanner: %v", err)
		return
	}

	applyCandidate := func(candidate domain.Candidate) {
		state.Portfolio.TryEnterTrade(candidate.Symbol, candidate.Timestamp, candidate.LastPrice, candidate.Metrics)
	}

	historyErrCh := make(chan error, 1)
	go func() {
		historyErrCh <- client.StreamLiveMinuteBars(ctx, symbolList, minuteBars)
	}()

	for {
		select {
		case candidate := <-candidates:
			applyCandidate(candidate)
		case err := <-historyErrCh:
			historyErrCh = nil
			if err != nil {
				log.Errorf("Error streaming minute bars: %v", err)
				return
			}
		case <-done:
			if historyErrCh != nil {
				if err := <-historyErrCh; err != nil {
					log.Errorf("Error streaming minute bars: %v", err)
					return
				}
				historyErrCh = nil
			}
			for {
				select {
				case candidate := <-candidates:
					applyCandidate(candidate)
				default:
					state.Portfolio.GenerateReport()
					return
				}
			}
		case <-ctx.Done():
			state.Portfolio.GenerateReport()
			return
		}
	}
}
