package domain

type Metrics struct {
	MACD                     float64
	MACDRoc                  float64
	MACDSignal               float64
	ATR                      float64
	Volume5m                 float64
	RSI                      float64
	EMA20                    float64
	EMA20Roc                 float64
	SessionVWAP              float64
	RelativeVolume20         float64
	TradeCountAccel          float64
	CloseStrength            float64
	BreakoutLevel5           float64
	BreakoutOpenedBelowLevel bool
	PullbackLow3             float64
	PullbackReclaimLevel     float64
	PullbackOpenedBelowLevel bool
}
