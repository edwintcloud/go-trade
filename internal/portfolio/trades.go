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

func (p *Portfolio) LenOpenTrades() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.openTrades)
}

func (p *Portfolio) EvaluateExitConditions(symbol string, lastPrice float64, timestamp time.Time) {
	if !p.hasOpenTrade(symbol) {
		return
	}
	trade := p.openTrades[symbol]
	p.mu.Lock()
	defer p.mu.Unlock()
	trade.CurrentPrice = lastPrice
	if lastPrice <= trade.StopPrice ||
		markethours.DurationElapsed(time.Duration(p.config.MinutesUntilBreakEvenStop)*time.Minute, trade.EntryTimestamp, timestamp) {
		p.exitTrade(symbol, timestamp, lastPrice)
	} else {
		trade.StopPrice = max(trade.StopPrice, p.stopPriceFor(lastPrice, trade.CurrentMetrics.ATR))
	}
}

func (p *Portfolio) ExitTrade(symbol string, exitTimestamp time.Time, exitPrice float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exitTrade(symbol, exitTimestamp, exitPrice)
}

func (p *Portfolio) UpdateOpenTrade(symbol string, lastPrice float64, metrics domain.Metrics, timestamp time.Time) {
	if err := p.EnsureStartingEquity(timestamp); err != nil {
		log.Errorf("Failed to refresh starting equity for %s: %v", timestamp.In(markethours.Location).Format("2006-01-02"), err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasOpenTrade(symbol) {
		return
	}
	trade := p.openTrades[symbol]
	trade.CurrentMetrics = metrics
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
		marketValue += float64(trade.Quantity) * trade.CurrentPrice
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
	cooldown := time.Duration(p.config.SameSymbolCooldownMinutes) * time.Minute
	for _, trade := range p.closedTrades[entryTimestamp.In(markethours.Location).Format("2006-01-02")] {
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

	quantity := p.determineTradeQuantityWithBuyingPower(entryTimestamp, entryPrice, metrics.ATR, p.availableBuyingPower(entryTimestamp))
	if float64(quantity)*entryPrice >= p.currentEquity(entryTimestamp)*p.config.MinPositionSizePct*0.9 {
		return
	}

	for _, trade := range p.openTrades {
		if !trade.EntryTimestamp.Before(entryTimestamp) || entryTimestamp.Before(trade.EntryTimestamp.Add(10*time.Minute)) {
			continue
		}

		// curTradeATRP := trade.CurrentMetrics.ATR / trade.CurrentPrice
		// newTradeATRP := metrics.ATR / entryPrice
		exitCondition := trade.CurrentMetrics.TradeCountAccel < metrics.TradeCountAccel && trade.CurrentMetrics.RelativeVolume20 < metrics.RelativeVolume20
		if exitCondition {
			p.exitTrade(trade.Symbol, entryTimestamp, trade.CurrentPrice)
			return
		}
	}
}

func (p *Portfolio) TryEnterTrade(candidate domain.Candidate) bool {
	symbol := candidate.Symbol
	entryTimestamp := candidate.Timestamp
	entryPrice := candidate.LastPrice
	metrics := candidate.Metrics
	stopPrice := p.stopPriceFor(entryPrice, metrics.ATR)

	if err := p.EnsureStartingEquity(entryTimestamp); err != nil {
		log.Errorf("Failed to refresh starting equity for %s: %v", entryTimestamp.In(markethours.Location).Format("2006-01-02"), err)
		return false
	}

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
		order, err := p.broker.SubmitOrder(symbol, quantity, alpaca.Buy)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return false
		}
		entryPrice = order.FilledAvgPrice.InexactFloat64()
		stopPrice = p.stopPriceFor(entryPrice, metrics.ATR)
		// send notification for new trade
		if p.telegram != nil {
			p.telegram.NotifyTradeOpened(domain.Trade{
				Symbol:     symbol,
				Quantity:   quantity,
				EntryPrice: entryPrice,
				StopPrice:  stopPrice,
			})
		}
	}

	p.openTrades[symbol] = &domain.Trade{
		Symbol:         symbol,
		EntryTimestamp: entryTimestamp.In(markethours.Location),
		EntryPrice:     entryPrice,
		StopPrice:      stopPrice,
		EntryMetrics:   metrics,
		CurrentPrice:   entryPrice,
		CurrentMetrics: metrics,
		Quantity:       quantity,
	}

	return true
}

func (p *Portfolio) recordTradeExit(symbol string, exitTimestamp time.Time, exitPrice float64) {
	if !p.hasOpenTrade(symbol) {
		return
	}

	trade := p.openTrades[symbol]
	trade.ExitTimestamp = exitTimestamp.In(markethours.Location)
	trade.ExitPrice = exitPrice
	dateKey := exitTimestamp.In(markethours.Location).Format("2006-01-02")
	p.closedTrades[dateKey] = append(p.closedTrades[dateKey], *trade)
	delete(p.openTrades, symbol)
}

func (p *Portfolio) exitTrade(symbol string, exitTimestamp time.Time, exitPrice float64) {
	if !p.hasOpenTrade(symbol) {
		return
	}

	trade := p.openTrades[symbol]

	// exit trade with broker if available
	if p.broker != nil {
		order, err := p.broker.SubmitOrder(symbol, trade.Quantity, alpaca.Sell)
		if err != nil {
			log.Errorf("Failed to submit order to broker for %s: %v", symbol, err)
			return
		}
		exitPrice = order.FilledAvgPrice.InexactFloat64()
		// send notification for closed trade
		if p.telegram != nil {
			p.telegram.NotifyTradeClosed(domain.Trade{
				Symbol:    symbol,
				ExitPrice: exitPrice,
			})
		}
	}
	p.recordTradeExit(symbol, exitTimestamp, exitPrice)
}

func (p *Portfolio) stopPriceFor(lastPrice float64, lastAtr float64) float64 {
	return max(lastPrice*(1-p.config.TrailingStopPctFallback), lastPrice-lastAtr*p.config.TrailingStopAtrMultiplier)
}

func (p *Portfolio) HasOpenTrade(symbol string, timestamp time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasOpenTrade(symbol)
}

func (p *Portfolio) LiquidateOpenTrades(exitTimestamp time.Time) {
	if err := p.EnsureStartingEquity(exitTimestamp); err != nil {
		log.Errorf("Failed to refresh starting equity before liquidation for %s: %v", exitTimestamp.In(markethours.Location).Format("2006-01-02"), err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.openTrades) == 0 {
		return
	}

	type pendingExit struct {
		symbol string
		price  float64
	}

	exits := make([]pendingExit, 0, len(p.openTrades))
	for symbol, trade := range p.openTrades {
		exitPrice := trade.CurrentPrice
		if exitPrice == 0 {
			exitPrice = trade.EntryPrice
		}
		exits = append(exits, pendingExit{symbol: symbol, price: exitPrice})
	}

	for _, exit := range exits {
		p.exitTrade(exit.symbol, exitTimestamp, exit.price)
	}

	log.Infof("Liquidated %d open position(s) ahead of the session close at %s", len(exits), exitTimestamp.In(markethours.Location).Format("15:04"))
}

func (p *Portfolio) HydrateOpenTradesFromBroker(snapshotTime time.Time) error {
	p.mu.RLock()
	broker := p.broker
	p.mu.RUnlock()
	if broker == nil {
		return nil
	}

	positions, err := broker.GetPositions()
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	nextOpenTrades := make(map[string]*domain.Trade, len(positions))
	for _, position := range positions {
		if position.Side == "short" {
			log.Warnf("Ignoring unsupported short position for %s during broker sync", position.Symbol)
			continue
		}

		quantity := uint64(position.Qty.Abs().IntPart())
		if quantity == 0 {
			continue
		}

		entryPrice := position.AvgEntryPrice.InexactFloat64()
		currentPrice := entryPrice
		if position.CurrentPrice != nil {
			currentPrice = position.CurrentPrice.InexactFloat64()
		}

		trade, exists := p.openTrades[position.Symbol]
		if !exists {
			trade = &domain.Trade{
				Symbol:         position.Symbol,
				EntryTimestamp: snapshotTime.In(markethours.Location),
				StopPrice:      p.stopPriceFor(currentPrice, 0),
			}
		}

		trade.EntryPrice = entryPrice
		trade.CurrentPrice = currentPrice
		trade.Quantity = quantity
		if trade.StopPrice == 0 {
			trade.StopPrice = p.stopPriceFor(currentPrice, trade.CurrentMetrics.ATR)
		}

		nextOpenTrades[position.Symbol] = trade
	}
	p.openTrades = nextOpenTrades
	return nil
}

func (p *Portfolio) HandleTradeUpdate(update alpaca.TradeUpdate) {
	if update.Order.Symbol == "" {
		return
	}
	if update.Event != "fill" && update.Event != "partial_fill" {
		return
	}

	timestamp := update.At.In(markethours.Location)
	if err := p.EnsureStartingEquity(timestamp); err != nil {
		log.Errorf("Failed to refresh starting equity from trade update for %s: %v", timestamp.Format("2006-01-02"), err)
		return
	}

	if update.PositionQty != nil && update.PositionQty.Abs().Sign() == 0 {
		exitPrice := 0.0
		if update.Price != nil {
			exitPrice = update.Price.InexactFloat64()
		} else if update.Order.FilledAvgPrice != nil {
			exitPrice = update.Order.FilledAvgPrice.InexactFloat64()
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.hasOpenTrade(update.Order.Symbol) {
			return
		}
		if exitPrice == 0 {
			exitPrice = p.openTrades[update.Order.Symbol].CurrentPrice
		}
		p.recordTradeExit(update.Order.Symbol, timestamp, exitPrice)
		return
	}

	p.mu.RLock()
	broker := p.broker
	p.mu.RUnlock()
	if broker == nil {
		return
	}

	position, err := broker.GetPosition(update.Order.Symbol)
	if err != nil {
		log.Errorf("Failed to load broker position for %s after trade update: %v", update.Order.Symbol, err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if position.Side == "short" {
		log.Warnf("Ignoring unsupported short position for %s after trade update", position.Symbol)
		delete(p.openTrades, position.Symbol)
		return
	}

	quantity := uint64(position.Qty.Abs().IntPart())
	if quantity == 0 {
		delete(p.openTrades, position.Symbol)
		return
	}

	entryPrice := position.AvgEntryPrice.InexactFloat64()
	currentPrice := entryPrice
	if position.CurrentPrice != nil {
		currentPrice = position.CurrentPrice.InexactFloat64()
	}

	trade, exists := p.openTrades[position.Symbol]
	if !exists {
		trade = &domain.Trade{
			Symbol:         position.Symbol,
			EntryTimestamp: timestamp,
			StopPrice:      p.stopPriceFor(currentPrice, 0),
		}
		p.openTrades[position.Symbol] = trade
	}

	trade.EntryPrice = entryPrice
	trade.CurrentPrice = currentPrice
	trade.Quantity = quantity
	if trade.StopPrice == 0 {
		trade.StopPrice = p.stopPriceFor(currentPrice, trade.CurrentMetrics.ATR)
	}
}
