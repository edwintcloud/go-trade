package cmd

import (
	"os"

	"github.com/labstack/gommon/log"
	"github.com/spf13/cobra"
)

var logLevel string

var rootCmd = &cobra.Command{
	Use: "go-trade",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set level based on flag
		switch logLevel {
		case "debug":
			log.SetLevel(log.DEBUG)
		case "warn":
			log.SetLevel(log.WARN)
		case "error":
			log.SetLevel(log.ERROR)
		default:
			log.SetLevel(log.INFO)
		}
		log.Infof("Log level set to: %s", logLevel)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(BacktestCommand())
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Set log level (debug, info, warn, error)")
}
