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
		defaultDate := getDefaultDate()
		log.Warnf("Invalid date format, using default of %s: %v", defaultDate, err)
		dt, _ = time.ParseInLocation(defaultDate, "2006-01-02", markethours.Location)
	}
	return dt
}

func getDefaultDate() string {
	// defaults to last market day
	now := time.Now()
	result := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, markethours.Location)
	switch now.Weekday() {
	case time.Monday:
		return result.AddDate(0, 0, -3).Format("2006-01-02")
	case time.Sunday:
		return result.AddDate(0, 0, -2).Format("2006-01-02")
	default:
		return result.AddDate(0, 0, -1).Format("2006-01-02")
	}
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
			var endTime time.Time
			if endDate == "" {
				endTime = startTime.Add(16 * time.Hour)
			} else {
				endTime = parseDate(endDate).Add(20 * time.Hour)
			}
			runBacktest(startTime, endTime, startingEquity, symbols)
		},
	}

	cmd.Flags().String("start", getDefaultDate(), "start date for backtest in YYYY-MM-DD format")
	cmd.Flags().String("end", "", "end date for backtest in YYYY-MM-DD format")
	cmd.Flags().Float64("starting-equity", 25000.0, "starting equity for backtest")
	cmd.Flags().String("symbols", "*", "comma separated list of symbols to include in backtest, all symbols will be included if not specified")
	return cmd
}

func runBacktest(startTime time.Time, endTime time.Time, startingEquity float64, symbolsStr string) {
	log.Infof("Running backtest from %s to %s", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := config.LoadConfig()

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
		log.Infof("Backtesting with %d symbols", len(symbolList))
	} else {
		symbolList = strings.Split(symbolsStr, ",")
		log.Infof("Backtesting with symbols: %v", symbolList)
	}

	state := state.NewState(config, symbolList)
	state.Portfolio.SetStartingEquity(startTime, startingEquity)

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
		state.Portfolio.TryEnterTrade(candidate)
	}

	historyErrCh := make(chan error, 1)
	go func() {
		historyErrCh <- client.StreamHistoricalMinuteBars(ctx, symbolList, startTime, endTime, minuteBars)
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
