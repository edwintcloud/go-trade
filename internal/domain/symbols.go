package domain

type Symbols struct {
	mapping map[string]*Symbol
}

func (s *Symbols) Get(symbolName string) (*Symbol, bool) {
	symbol, ok := s.mapping[symbolName]
	return symbol, ok
}

func NewSymbols(symbolNames []string) *Symbols {
	symbols := &Symbols{
		mapping: make(map[string]*Symbol),
	}
	for _, name := range symbolNames {
		symbols.mapping[name] = NewSymbol(name)
	}
	return symbols
}
