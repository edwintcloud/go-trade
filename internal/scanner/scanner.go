package scanner

import (
	"context"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/edwintcloud/go-trade/internal/alpaca"
	"github.com/edwintcloud/go-trade/internal/config"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/edwintcloud/go-trade/internal/state"
	"github.com/labstack/gommon/log"
)

type Scanner struct {
	client *alpaca.Client
	config *config.Config
	state  *state.State
}

func NewScanner(client *alpaca.Client, config *config.Config, state *state.State) *Scanner {
	return &Scanner{
		client: client,
		config: config,
		state:  state,
	}
}

// emits top 10 volume symbols on an interval of 1 minute
// caller must keep a map of symbols currently subscribed for strategy
func (s *Scanner) Start(ctx context.Context, out chan<- string) error {
	pq := domain.NewPriorityQueue()
	timestamp := time.Now().In(markethours.Location)

	handler := func(bar stream.Bar) {
		if !markethours.IsTradableSession(timestamp) || markethours.HasReachedTradableSessionCloseBuffer(timestamp, 30*time.Minute) {
			return
		}
		if bar.Close < s.config.MinPrice || bar.Close > s.config.MaxPrice || bar.Symbol == "" || bar.Volume == 0 {
			return
		}
		pq.UpdateOrPush(bar.Symbol, int(bar.Volume))
	}

	// subscribe to daily bars in batches of 500
	start, end, n := 0, 500, len(s.state.SymbolList)-1
	curBatch, nBatches := 1, n/500
	if n%500 != 0 {
		nBatches += 1
	}
	for start < end {
		err := s.client.SubscribeToDailyBars(ctx, handler, s.state.SymbolList[start:end]...)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("stream: subscribe dailyBars request sent for %d symbols (batch %d/%d)", end-start, curBatch, nBatches)
		start += 500
		end += 500
		if end > n {
			end = n + 1
		}
		curBatch += 1
		time.Sleep(100 * time.Millisecond) // avoid hitting rate limits
	}

	// emit top 10 symbols every minute
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(60 * time.Second):
			for _, v := range pq.PeekN(10) {
				out <- v.Value
			}
		}
	}
}
