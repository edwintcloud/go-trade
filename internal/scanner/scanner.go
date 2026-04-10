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
	client   *alpaca.Client
	config   *config.Config
	state    *state.State
	volumePq *domain.PriorityQueue
}

func NewScanner(client *alpaca.Client, config *config.Config, state *state.State) *Scanner {
	return &Scanner{
		client:   client,
		config:   config,
		state:    state,
		volumePq: domain.NewPriorityQueue(),
	}
}

func (s *Scanner) handleBar(bar stream.Bar) {
	timestamp := time.Now().In(markethours.Location)
	// log.Debugf("Received bar for %s at %s: open=%.2f, close=%.2f, high=%.2f, low=%.2f, volume=%d", bar.Symbol, timestamp.Format(time.RFC3339), bar.Open, bar.Close, bar.High, bar.Low, bar.Volume)
	if !markethours.IsTradableSession(timestamp) || markethours.HasReachedTradableSessionCloseBuffer(timestamp, 30*time.Minute) {
		log.Debugf("Not processing bar for %s at %s because it's outside of tradable session or within 30 minutes of tradable session close", bar.Symbol, timestamp.Format(time.RFC3339))
		return
	}
	if bar.Close < s.config.MinPrice || bar.Close > s.config.MaxPrice || bar.Symbol == "" || bar.Volume == 0 {
		// log.Debugf("Not processing bar for %s at %s because it doesn't meet price/volume/symbol criteria (close: %.2f, volume: %d)", bar.Symbol, timestamp.Format(time.RFC3339), bar.Close, bar.Volume)
		return
	}
	// accumulate volume for symbol in pq
	s.volumePq.AccumulateOrPush(bar.Symbol, int(bar.Volume), timestamp, 30*time.Minute)

	// update bar for symbol in state
	domainSymbol, ok := s.state.Symbols.Get(bar.Symbol)
	if !ok {
		log.Debugf("Received bar for symbol %s which is not in state.Symbols, skipping state update", bar.Symbol)
		return
	}
	domainSymbol.AddBar(domain.BarFromStreamBar(bar))
}

// emits top 10 volume symbols on an interval of 1 minute
// caller must keep a map of symbols currently subscribed for strategy
func (s *Scanner) Start(ctx context.Context, out chan<- string) error {
	// subscribe to minute bars in batches of 500
	start, end, n := 0, 500, len(s.state.SymbolList)-1
	curBatch, nBatches := 1, n/500
	if n%500 != 0 {
		nBatches += 1
	}
	for start < end {
		err := s.client.SubscribeToBars(ctx, s.handleBar, s.state.SymbolList[start:end]...)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("stream: subscribe bars request sent for %d symbols (batch %d/%d)", end-start, curBatch, nBatches)
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
			curTime := time.Now().In(markethours.Location)
			for _, v := range s.volumePq.PeekN(10) {
				// only emit symbols if the accumulated volume is greater than the threshold to avoid emitting symbols with low liquidity
				// also only emit symbols once there is 5 minutes of data
				if curTime.Sub(v.AccumulationStartTime) < 5*time.Minute || v.Priority < int(s.config.MinTotalVolume5Min) {
					// log.Debugf("Emitting top 10 symbols at %s", curTime.Format(time.RFC3339))
					continue
				}
				out <- v.Value
			}
		}
	}
}
