package portfolio

import (
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/labstack/gommon/log"
)

// TODO: should subscribe to quotes for open positions to check for stop loss hits and update equity in real time
type Trade struct {
	Symbol         string
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	EntryPrice     float64
	CurrentPrice   float64
	CurrentMetrics domain.Metrics
	ExitPrice      float64
	ATR            float64
	StopPrice      float64
	Quantity       uint64
}

func (p *Portfolio) hasOpenTrade(symbol string) bool {
	_, exists := p.openTrades[symbol]
	return exists
}

func (p *Portfolio) UpdateOpenTrade(symbol string, lastPrice float64, metrics domain.Metrics, timestamp time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasOpenTrade(symbol) {
		return
	}
	trade := p.openTrades[symbol]
	trade.CurrentPrice = lastPrice
	trade.CurrentMetrics = metrics
	if lastPrice <= trade.StopPrice {
		p.exitTrade(symbol, timestamp, lastPrice)
	} else {
		p.updateStopPrice(symbol, timestamp, lastPrice, metrics.ATR)
	}
}

func (p *Portfolio) nTradesByDate(date time.Time) int {
	dateKey := date.Format("2006-01-02")
	trades, exists := p.closedTrades[dateKey]
	if !exists {
		return len(p.openTrades)
	}
	return len(trades) + len(p.openTrades)
}

func (p *Portfolio) unrelalizedPnl() float64 {
	unrealized := 0.0
	for _, trade := range p.openTrades {
		unrealized += float64(trade.Quantity) * (trade.CurrentPrice - trade.EntryPrice)
	}
	return unrealized
}

func (p *Portfolio) getPnlByDate(date time.Time) float64 {
	dateKey := date.Format("2006-01-02")
	trades, exists := p.closedTrades[dateKey]
	if !exists {
		return 0
	}
	pnl := 0.0
	for _, trade := range trades {
		if !trade.ExitTimestamp.IsZero() {
			pnl += float64(trade.Quantity) * (trade.ExitPrice - trade.EntryPrice)
		}
	}
	return pnl + p.unrelalizedPnl()
}

func (p *Portfolio) calculateProfitOrLossPctByDate(date time.Time) float64 {
	pnl := p.getPnlByDate(date)
	startingEquity, exists := p.startingEquity[date.Format("2006-01-02")]
	if !exists || startingEquity == 0 {
		return 0
	}
	return pnl / startingEquity
}

func (p *Portfolio) currentEquity(date time.Time) float64 {
	startingEquity, exists := p.startingEquity[date.Format("2006-01-02")]
	if !exists {
		log.Debugf("No starting equity set for %s, defaulting to 0", date.Format("2006-01-02"))
		return 0
	}
	return startingEquity + p.getPnlByDate(date)
}

func (p *Portfolio) currentMarketValue() float64 {
	marketValue := 0.0
	for _, trade := range p.openTrades {
		marketValue += float64(trade.Quantity) * trade.CurrentPrice
	}
	return marketValue
}

func (p *Portfolio) availableBuyingPower(date time.Time) float64 {
	buyingPower := p.currentEquity(date) - p.currentMarketValue()
	if buyingPower < 0 {
		return 0
	}
	return buyingPower
}

func (p *Portfolio) determineTradeQuantityWithBuyingPower(entryTimestamp time.Time, price float64, atr float64, buyingPower float64) uint64 {
	if price <= 0 {
		return 0
	}

	equity := p.currentEquity(entryTimestamp)
	if equity <= 0 || buyingPower <= 0 {
		return 0
	}

	stopPrice := p.stopPriceFor(price, atr)
	riskPerShare := price - stopPrice
	if riskPerShare == 0 {
		return 0
	}

	quantity := uint64(buyingPower / price)
	if quantity < uint64(p.config.MinPositionSizePct)*uint64(equity/price) {
		return 0
	}

	return quantity
}

func (p *Portfolio) symbolOnCooldown(symbol string, entryTimestamp time.Time) bool {
	cooldown := time.Duration(p.config.SameSymbolCooldownMinutes) * time.Minute
	for _, trade := range p.closedTrades[entryTimestamp.Format("2006-01-02")] {
		if trade.Symbol != symbol || trade.ExitTimestamp.IsZero() {
			continue
		}
		if entryTimestamp.Before(trade.ExitTimestamp) {
			continue
		}
		if entryTimestamp.Sub(trade.ExitTimestamp) < cooldown {
			return true
		}
	}
	return false
}

func (p *Portfolio) canEnterTrade(symbol string, entryTimestamp time.Time, proposedQuantity uint64) (bool, string) {
	if p.hasOpenTrade(symbol) {
		return false, "already have an open trade for this symbol"
	}
	if p.nTradesByDate(entryTimestamp) >= p.config.MaxTradesPerDay {
		return false, "exceeded max trades per day"
	}
	profitOrLossPct := p.calculateProfitOrLossPctByDate(entryTimestamp)
	if profitOrLossPct <= -p.config.MaxLossPerDayPct {
		return false, "exceeded max loss per day"
	}
	if profitOrLossPct >= p.config.DailyProfitTargetPct {
		return false, "exceeded daily profit target per day"
	}
	if proposedQuantity == 0 {
		return false, "proposed quantity is 0 based on risk parameters"
	}
	if p.symbolOnCooldown(symbol, entryTimestamp) {
		return false, "symbol is on cooldown from a recent trade"
	}
	return true, ""
}

// evaluates open trades against new trade to determine a more positive outcome by liquidating the open trade and entering the new trade.
// atrp is used as the basis for comparison since it incorporates both risk and reward potential of the trade. If the new trade has a
// higher atrp than the existing trade, we exit the existing trade and allow the new trade to be entered. This allows us to dynamically
// adjust our positions throughout the day to take advantage of better opportunities as they arise, while still adhering to our risk management parameters.
func (p *Portfolio) evaluateOpenTradesForAnOpportunitySwap(symbol string, entryTimestamp time.Time, entryPrice float64, metrics domain.Metrics) {
	if p.hasOpenTrade(symbol) || len(p.openTrades) == 0 {
		return
	}

	for _, trade := range p.openTrades {
		if !trade.EntryTimestamp.Before(entryTimestamp) {
			continue
		}

		curTradeATRP := trade.CurrentMetrics.ATR / trade.CurrentPrice
		newTradeATRP := metrics.ATR / entryPrice
		if curTradeATRP < newTradeATRP {
			p.exitTrade(trade.Symbol, entryTimestamp, trade.CurrentPrice)
			return
		}
	}
}

func (p *Portfolio) TryEnterTrade(symbol string, entryTimestamp time.Time, entryPrice float64, metrics domain.Metrics) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.evaluateOpenTradesForAnOpportunitySwap(symbol, entryTimestamp, entryPrice, metrics)

	quantity := p.determineTradeQuantityWithBuyingPower(entryTimestamp, entryPrice, metrics.ATR, p.availableBuyingPower(entryTimestamp))
	tradeApproved, reason := p.canEnterTrade(symbol, entryTimestamp, quantity)
	if !tradeApproved {
		log.Debugf("Cannot enter trade for %s at %s: %s", symbol, entryTimestamp.Format("2006-01-02 15:04"), reason)
		return false
	}

	// enter trade with broker if available
	if p.broker != nil {
		err := p.broker.SubmitOrder(symbol, quantity, alpaca.Buy)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return false
		}
	}

	p.openTrades[symbol] = &Trade{
		Symbol:         symbol,
		EntryTimestamp: entryTimestamp.In(markethours.Location),
		EntryPrice:     entryPrice,
		CurrentPrice:   entryPrice,
		CurrentMetrics: metrics,
		ATR:            metrics.ATR,
		Quantity:       quantity,
	}
	p.updateStopPrice(symbol, entryTimestamp, entryPrice, metrics.ATR)
	return true
}

func (p *Portfolio) exitTrade(symbol string, exitTimestamp time.Time, exitPrice float64) {
	if !p.hasOpenTrade(symbol) {
		return
	}

	trade := p.openTrades[symbol]

	// exit trade with broker if available
	if p.broker != nil {
		err := p.broker.SubmitOrder(symbol, trade.Quantity, alpaca.Sell)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return
		}
	}

	trade.ExitTimestamp = exitTimestamp.In(markethours.Location)
	trade.ExitPrice = exitPrice
	dateKey := exitTimestamp.Format("2006-01-02")
	p.closedTrades[dateKey] = append(p.closedTrades[dateKey], *trade)
	delete(p.openTrades, symbol)
}

func (p *Portfolio) stopPriceFor(lastPrice float64, lastAtr float64) float64 {
	return max(lastPrice*(1-p.config.TrailingStopPctFallback), lastPrice-lastAtr*p.config.TrailingStopAtrMultiplier)
}

func (p *Portfolio) updateStopPrice(symbol string, timestamp time.Time, lastPrice float64, lastAtr float64) {
	newStop := p.stopPriceFor(lastPrice, lastAtr)
	if p.openTrades[symbol].EntryTimestamp.Add(time.Duration(p.config.MinutesUntilBreakEvenStop) * time.Minute).Before(timestamp.In(markethours.Location)) {
		newStop = max(newStop, p.openTrades[symbol].EntryPrice)
	}
	if newStop > p.openTrades[symbol].StopPrice {
		p.openTrades[symbol].StopPrice = newStop
	}
}

func (p *Portfolio) HasOpenTrade(symbol string, timestamp time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasOpenTrade(symbol)
}
