package domain

type Symbols map[string]*Symbol

func (s Symbols) Get(symbolName string) (*Symbol, bool) {
	symbol, ok := s[symbolName]
	return symbol, ok
}

func NewSymbols(symbolNames []string) Symbols {
	symbols := make(Symbols)
	for _, name := range symbolNames {
		symbols[name] = NewSymbol(name)
	}
	return symbols
}