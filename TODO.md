# simplify
- scanner produces top 10 canidates by volume
- strategy continuously monitors these 10 using minuteBar subscription aggregating metrics

## mean reversion strategy (momentum)
- stock is above vwap
- entry: at vwap
- exit trailing stop of 1.5atr
- early exit indicator: ema10 roc is negative

## mean reversion strategy (reversion/reversal)
- stock is below vwap
- entry: 1.5 stddev below vwap
- exit trailing stop of 1.5atr

## general strategy
- hull ma 30 roc
- when roc is positive, enter
- when roc is negative, exit
