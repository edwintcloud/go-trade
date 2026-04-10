package portfolio

import (
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/edwintcloud/go-trade/internal/domain"
	"github.com/edwintcloud/go-trade/internal/markethours"
	"github.com/labstack/gommon/log"
)

func (p *Portfolio) hasOpenTrade(symbol string) bool {
	_, exists := p.openTrades[symbol]
	return exists
}

func (p *Portfolio) HasOpenTrade(symbol string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasOpenTrade(symbol)
}

func (p *Portfolio) LenOpenTrades() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.openTrades)
}

func (p *Portfolio) EvaluateExitConditions(symbol *domain.Symbol) {
	if !p.hasOpenTrade(symbol.Name) {
		return
	}
	trade := p.openTrades[symbol.Name]
	p.mu.Lock()
	defer p.mu.Unlock()
	if trade.Symbol == nil {
		trade.Symbol = symbol
	}

	if symbol.BidPrice <= trade.StopPrice ||
		markethours.DurationElapsed(time.Duration(p.config.MinutesUntilBreakEvenStop)*time.Minute, trade.EntryTimestamp, time.Now()) {
		p.exitTrade(symbol.Name)
	} else {
		trade.StopPrice = max(trade.StopPrice, p.stopPriceFor(symbol.BidPrice, symbol.GetMetrics().ATR))
	}
}

func (p *Portfolio) ExitTrade(symbol string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exitTrade(symbol)
}

func (p *Portfolio) nTradesByDate(date time.Time) uint64 {
	dateKey := date.Format("2006-01-02")
	trades, exists := p.closedTrades[dateKey]
	if !exists {
		return uint64(len(p.openTrades))
	}
	return uint64(len(trades) + len(p.openTrades))
}

func (p *Portfolio) unrelalizedPnl() float64 {
	unrealized := 0.0
	for _, trade := range p.openTrades {
		unrealized += float64(trade.Quantity) * (trade.Symbol.BidPrice - trade.EntryPrice)
	}
	return unrealized
}

func (p *Portfolio) pnlByDayKeyLocked(dayKey string) float64 {
	trades, exists := p.closedTrades[dayKey]
	if !exists {
		return p.unrelalizedPnl()
	}
	pnl := 0.0
	for _, trade := range trades {
		if !trade.ExitTimestamp.IsZero() {
			pnl += float64(trade.Quantity) * (trade.ExitPrice - trade.EntryPrice)
		}
	}
	return pnl + p.unrelalizedPnl()
}

func (p *Portfolio) getPnlByDate(date time.Time) float64 {
	return p.pnlByDayKeyLocked(date.In(markethours.Location).Format("2006-01-02"))
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
	startingEquity, exists := p.startingEquity[date.In(markethours.Location).Format("2006-01-02")]
	if !exists {
		log.Debugf("No starting equity set for %s, defaulting to 0", date.In(markethours.Location).Format("2006-01-02"))
		return 0
	}
	return startingEquity + p.getPnlByDate(date)
}

func (p *Portfolio) currentMarketValue() float64 {
	marketValue := 0.0
	for _, trade := range p.openTrades {
		marketValue += float64(trade.Quantity) * trade.Symbol.BidPrice
	}
	return marketValue
}

func (p *Portfolio) availableBuyingPower(date time.Time) float64 {
	buyingPower := p.currentEquity(date) - p.currentMarketValue()
	if buyingPower < 0 {
		return 0
	}
	return buyingPower * 0.95 // keep a buffer to avoid margin issues
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

	quantity := uint64(buyingPower / price * p.config.MaxCapitalPerTradePct)
	if quantity < uint64(p.config.MinPositionSizePct)*uint64(equity/price) {
		return 0
	}

	return quantity
}

func (p *Portfolio) symbolOnCooldown(symbol string, entryTimestamp time.Time) bool {
	tradeCooldown := time.Duration(p.config.SameSymbolCooldownMinutes) * time.Minute
	retryCooldown := 5 * time.Minute
	for _, trade := range p.closedTrades[entryTimestamp.In(markethours.Location).Format("2006-01-02")] {
		if trade.Symbol.Name != symbol || trade.ExitTimestamp.IsZero() {
			continue
		}
		if entryTimestamp.Before(trade.ExitTimestamp) {
			continue
		}
		if entryTimestamp.Sub(trade.ExitTimestamp) < tradeCooldown {
			return true
		}
	}
	if lastAttempt, exists := p.attemptedEntries[symbol]; exists {
		if entryTimestamp.Sub(lastAttempt) < retryCooldown {
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

func (p *Portfolio) evaluateOpenTradesForAnOpportunitySwap(symbol string, entryTimestamp time.Time, metrics domain.Metrics) {
	if p.hasOpenTrade(symbol) || len(p.openTrades) == 0 {
		return
	}

	for _, openTrade := range p.openTrades {
		if !openTrade.EntryTimestamp.Before(entryTimestamp) || entryTimestamp.Sub(openTrade.EntryTimestamp) < 10*time.Minute {
			continue
		}

		// exit only if the new trade is rising faster than the existing trade based on hull ma roc
		openTradeMetrics := openTrade.Symbol.GetMetrics()
		exitCondition := openTradeMetrics.HullMaRoc*1.003 < metrics.HullMaRoc
		if exitCondition {
			log.Debugf("Opportunity swap: exiting %s for %s based on hull ma roc improvement (%.4f -> %.4f)", openTrade.Symbol.Name, symbol, openTradeMetrics.HullMaRoc, metrics.HullMaRoc)
			p.exitTrade(openTrade.Symbol.Name)
			return
		}
	}
}

func (p *Portfolio) TryEnterTrade(symbol *domain.Symbol) (bool, string) {
	entryTimestamp := time.Now().In(markethours.Location)
	symbolName := symbol.Name
	entryPrice := symbol.BidPrice
	metrics := symbol.GetMetrics()
	stopPrice := p.stopPriceFor(entryPrice, metrics.ATR)

	p.mu.Lock()
	defer p.mu.Unlock()

	quantity := p.determineTradeQuantityWithBuyingPower(entryTimestamp, entryPrice, metrics.ATR, p.availableBuyingPower(entryTimestamp))
	if float64(quantity)*entryPrice < p.currentEquity(entryTimestamp)*p.config.MinPositionSizePct*0.9 {
		p.evaluateOpenTradesForAnOpportunitySwap(symbolName, entryTimestamp, metrics)
	}

	tradeApproved, reason := p.canEnterTrade(symbolName, entryTimestamp, quantity)
	if !tradeApproved {
		log.Debugf("Cannot enter trade for %s at %s: %s", symbol.Name, entryTimestamp.Format("2006-01-02 15:04"), reason)
		return false, reason
	}

	// enter trade with broker if available
	if p.broker != nil {
		p.attemptedEntries[symbolName] = entryTimestamp
		_, err := p.broker.SubmitOrder(symbolName, quantity, alpaca.Buy)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return false, "broker-order-submit-failure"
		}
		delete(p.attemptedEntries, symbolName)
		// send notification for new trade
		if p.telegram != nil {
			p.telegram.NotifyTradeOpened(
				symbolName,
				quantity,
				entryPrice,
				stopPrice,
			)
		}
	}

	p.openTrades[symbolName] = &domain.Trade{
		Symbol:         symbol,
		EntryTimestamp: entryTimestamp.In(markethours.Location),
		EntryPrice:     entryPrice,
		StopPrice:      stopPrice,
		Quantity:       quantity,
	}

	return true, ""
}

func (p *Portfolio) recordTradeExit(symbol string, exitPrice float64) {
	if !p.hasOpenTrade(symbol) {
		return
	}

	trade := p.openTrades[symbol]
	trade.ExitTimestamp = time.Now().In(markethours.Location)
	trade.ExitPrice = exitPrice
	dateKey := trade.ExitTimestamp.In(markethours.Location).Format("2006-01-02")
	p.closedTrades[dateKey] = append(p.closedTrades[dateKey], *trade)
	delete(p.openTrades, symbol)
}

func (p *Portfolio) exitTrade(symbol string) {
	if !p.hasOpenTrade(symbol) {
		return
	}

	trade := p.openTrades[symbol]

	// exit trade with broker if available
	if p.broker != nil {
		submittedOrder, err := p.broker.SubmitOrder(symbol, trade.Quantity, alpaca.Sell)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return
		}
		// send notification for closed trade
		if p.telegram != nil {
			p.telegram.NotifyTradeClosed(symbol, submittedOrder.LimitPrice)
		}
		p.recordTradeExit(symbol, submittedOrder.LimitPrice)
	}
}

func (p *Portfolio) stopPriceFor(bidPrice float64, lastAtr float64) float64 {
	return max(bidPrice*(1-p.config.TrailingStopPctFallback), bidPrice-lastAtr*p.config.TrailingStopAtrMultiplier)
}

func (p *Portfolio) LiquidateOpenTrades(exitTimestamp time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.openTrades) == 0 {
		return
	}

	n := len(p.openTrades)
	for symbol, _ := range p.openTrades {
		p.exitTrade(symbol)
	}

	log.Infof("Liquidated %d open position(s) ahead of the session close at %s", n, exitTimestamp.In(markethours.Location).Format("15:04"))
}
