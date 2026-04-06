package portfolio

import (
	"context"
	"time"

	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/labstack/gommon/log"
)

const dailySummaryHour = 20

func (p *Portfolio) StartDailySummaryScheduler(ctx context.Context) {
	p.mu.Lock()
	if p.dailySummarySchedulerStarted || p.telegram == nil {
		p.mu.Unlock()
		return
	}
	p.dailySummarySchedulerStarted = true
	p.mu.Unlock()

	go p.runDailySummaryScheduler(ctx)
}

func (p *Portfolio) runDailySummaryScheduler(ctx context.Context) {
	p.maybeSendDailySummary(time.Now())

	for {
		next := nextDailySummaryRun(time.Now())
		timer := time.NewTimer(time.Until(next))

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case scheduledAt := <-timer.C:
			p.maybeSendDailySummary(scheduledAt)
		}
	}
}

func (p *Portfolio) maybeSendDailySummary(now time.Time) {
	local := now.In(markethours.Location)
	if !markethours.IsMarketDay(local) {
		return
	}

	scheduledAt := time.Date(local.Year(), local.Month(), local.Day(), dailySummaryHour, 0, 0, 0, markethours.Location)
	if local.Before(scheduledAt) {
		return
	}

	if err := p.EnsureStartingEquity(local); err != nil {
		log.Errorf("Failed to prepare daily summary for %s: %v", local.Format("2006-01-02"), err)
		return
	}

	if !p.markDailySummarySent(local) {
		return
	}

	p.sendDailySummary(local)
}

func (p *Portfolio) markDailySummarySent(date time.Time) bool {
	dayKey := date.In(markethours.Location).Format("2006-01-02")

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastDailySummaryDay == dayKey {
		return false
	}

	p.lastDailySummaryDay = dayKey
	return true
}

func nextDailySummaryRun(now time.Time) time.Time {
	local := now.In(markethours.Location)
	next := time.Date(local.Year(), local.Month(), local.Day(), dailySummaryHour, 0, 0, 0, markethours.Location)
	if !local.Before(next) {
		next = next.AddDate(0, 0, 1)
	}

	for !markethours.IsMarketDay(next) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}
