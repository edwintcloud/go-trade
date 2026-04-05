package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/scanner"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
	"github.com/spf13/cobra"
)

func parseDate(value string) time.Time {
	dt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), markethours.Location)
	if err != nil {
		log.Warnf("Invalid date format, using default of now: %v", err)
		return time.Now()
	}
	return dt
}

func BacktestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Run a backtest using historical data",
		Run: func(cmd *cobra.Command, args []string) {
			startDate, _ := cmd.Flags().GetString("start")
			endDate, _ := cmd.Flags().GetString("end")
			startingEquity, _ := cmd.Flags().GetFloat64("starting-equity")
			symbols, _ := cmd.Flags().GetString("symbols")

			startTime := parseDate(startDate).Add(4 * time.Hour)
			endTime := parseDate(endDate).Add(20 * time.Hour)
			runBacktest(startTime, endTime, startingEquity, symbols)
		},
	}
	cmd.Flags().String("start", time.Now().In(markethours.Location).Format("2006-01-02"), "start date for backtest in YYYY-MM-DD format")
	cmd.Flags().String("end", time.Now().In(markethours.Location).Format("2006-01-02"), "end date for backtest in YYYY-MM-DD format")
	cmd.Flags().Float64("starting-equity", 25000.0, "starting equity for backtest")
	cmd.Flags().String("symbols", "PENG", "comma separated list of symbols to include in backtest, all symbols will be included if not specified")
	return cmd
}

func runBacktest(startTime time.Time, endTime time.Time, startingEquity float64, symbolsStr string) {
	log.Infof("Running backtest from %s to %s", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := config.LoadConfig()

	portfolio := domain.NewPortfolio(startTime, startingEquity)
	state := state.NewState(portfolio)

	client := alpaca.NewClient(config)

	// determine symbols to backtest
	var symbolList []string
	if symbolsStr == "*" {
		equitySymbols, err := client.GetSymbols()
		if err != nil {
			log.Errorf("Error getting symbols: %v", err)
			return
		}
		symbolList = equitySymbols
	} else {
		symbolList = strings.Split(symbolsStr, ",")
		log.Infof("Backtesting with symbols: %v", symbolList)
	}
	symbols := domain.NewSymbols(symbolList)

	// stream minute bars
	var err error
	minuteBars := make(chan domain.Bar, config.ChannelBufferSize)

	err = client.StreamHistoricalMinuteBars(ctx, symbolList, startTime, endTime, minuteBars)
	if err != nil {
		log.Errorf("Error streaming minute bars: %v", err)
		return
	}

	// start scanner
	candidates := make(chan domain.Candidate, config.ChannelBufferSize)
	scanner := scanner.NewScanner(config, state, symbols, portfolio)

	err = scanner.Start(ctx, minuteBars, candidates)
	if err != nil {
		log.Errorf("Error starting scanner: %v", err)
		return
	}

	for {
		select {
		case candidate := <-candidates:
			entryTime := candidate.Timestamp
			proposedQuantity := portfolio.GetCurrentEquity() / candidate.LastPrice * 0.8
			stopPrice := max(candidate.LastPrice*0.95, candidate.LastPrice-candidate.Metrics.ATR*2)
			if !portfolio.HasOpenTrade(candidate.Symbol, entryTime) {
				portfolio.EnterTrade(candidate.Symbol, entryTime, candidate.LastPrice, uint(proposedQuantity), stopPrice)
			}
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			log.Info("Stopping scanner after 30 seconds")
			portfolio.GenerateReport(startTime)
			return
		}
	}
}
