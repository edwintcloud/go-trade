package domain

type Candidate struct {
	Symbol string
	LastPrice float64
	Metrics Metrics
}

func NewCandidateFromSymbol(symbol *Symbol) *Candidate {
	return &Candidate{
		Symbol: symbol.Name,
		LastPrice: symbol.GetLastPrice(),
		Metrics: symbol.GetMetrics(),
	}
}