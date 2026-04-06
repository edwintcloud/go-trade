package alpaca

import (
	"slices"
	"strings"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
)

var scannerETFKeywords = []string{
	" ETF ",
	" ETN ",
	" ETP ",
	" EXCHANGE TRADED ",
	" INDEX FUND ",
}

var scannerETPSponsorKeywords = []string{
	" PROSHARES ",
	" DIREXION ",
	" ISHARES ",
	" SPDR ",
	" GLOBAL X ",
	" VANECK ",
	" INVESCO ",
	" FIRST TRUST ",
	" WISDOMTREE ",
	" GRAYSCALE ",
	" VOLATILITY SHARES ",
	" ROUNDHILL ",
	" YIELDMAX ",
	" DEFIANCE ",
	" BITWISE ",
	" ARK ",
}

var scannerDerivativeKeywords = []string{
	" LEVERAGED ",
	" INVERSE ",
	" ULTRA ",
	" ULTRAPRO ",
	" 2X ",
	" 3X ",
	" -1X ",
	" -2X ",
	" -3X ",
	" SHORT ",
	" BEAR ",
	" BULL ",
	" FUTURES ",
	" VOLATILITY ",
}

func GetFilteredTradeableAssets(assets []alpaca.Asset) []alpaca.Asset {
	filtered := []alpaca.Asset{}
	for _, a := range assets {
		if !a.Tradable || shouldBlock(a.Name, a.Exchange) {
			continue
		}
		a.Symbol = strings.ToUpper(a.Symbol)
		filtered = append(filtered, a)
	}

	slices.SortFunc(filtered, func(a, b alpaca.Asset) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	return filtered
}

func shouldBlock(name string, exchange string) bool {
	normalized := normalizeInstrumentName(name)
	if normalized == "" {
		return false
	}
	if strings.ToUpper(exchange) != "NASDAQ" && strings.ToUpper(exchange) != "NYSE" {
		return true
	}
	for _, keyword := range scannerETFKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	for _, keyword := range scannerETPSponsorKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	for _, keyword := range scannerDerivativeKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func normalizeInstrumentName(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	replacer := strings.NewReplacer(
		".", " ",
		",", " ",
		"/", " ",
		"-", " ",
		"_", " ",
		"&", " ",
	)
	upper = replacer.Replace(upper)
	return " " + strings.Join(strings.Fields(upper), " ") + " "
}
