package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
)

// TelegramNotifier sends optional Telegram notifications for broker-confirmed trade events.
type TelegramNotifier struct {
	config *config.Config
	client *http.Client
}

// NewTelegramNotifierFromEnv creates a notifier from environment variables.
func NewTelegramNotifier(config *config.Config) *TelegramNotifier {
	return &TelegramNotifier{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyTradeOpened sends a message when an opening fill creates or adds to a position.
func (n *TelegramNotifier) NotifyTradeOpened(trade domain.Trade) {
	text := fmt.Sprintf(
		"🚀 Trade Opened\n\nSymbol: %s\nQty: %d\nPrice: $%.2f\nStop: $%.2f",
		trade.Symbol,
		trade.Quantity,
		trade.EntryPrice,
		trade.StopPrice,
	)
	n.send(text)
}

// NotifyTradeClosed sends a message when a position is fully closed.
func (n *TelegramNotifier) NotifyTradeClosed(trade domain.Trade) {
	text := fmt.Sprintf(
		"🏁 Trade Closed\n\nSymbol: %s\nExit Price: $%.2f",
		trade.Symbol,
		trade.ExitPrice,
	)
	n.send(text)
}

// NotifyDailySummary sends an end-of-day summary for the current trading day.
func (n *TelegramNotifier) NotifyDailySummary(asOf time.Time, startingCapital float64, dayPnL float64, tradesToday int) {
	roiPct := 0.0
	if startingCapital > 0 {
		roiPct = dayPnL / startingCapital * 100
	}

	text := fmt.Sprintf(
		"📊 End of Day Summary\n\nDate: %s\nNet Profit: $%.2f\nROI: %.2f%%\nTrades: %d",
		asOf.Format("2006-01-02"),
		dayPnL,
		roiPct,
		tradesToday,
	)
	n.send(text)
}

func (n *TelegramNotifier) send(text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.config.TelegramBotToken)
	payload, _ := json.Marshal(map[string]string{
		"chat_id": n.config.TelegramChatID,
		"text":    text,
	})
	resp, err := n.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("telemetry: telegram notification failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("telemetry: telegram returned status %d", resp.StatusCode)
	}
}
