package markethours

import "time"

var Location, _ = time.LoadLocation("America/New_York")

func IsRegularSession(t time.Time) bool {
	return t.In(Location).After(time.Date(t.Year(), t.Month(), t.Day(), 9, 30, 0, 0, Location)) &&
		t.In(Location).Before(time.Date(t.Year(), t.Month(), t.Day(), 16, 0, 0, 0, Location))
}

func IsPreMarketSession(t time.Time) bool {
	return t.In(Location).After(time.Date(t.Year(), t.Month(), t.Day(), 4, 0, 0, 0, Location)) &&
		t.In(Location).Before(time.Date(t.Year(), t.Month(), t.Day(), 9, 30, 0, 0, Location))
}

func IsAfterHoursSession(t time.Time) bool {
	return t.In(Location).After(time.Date(t.Year(), t.Month(), t.Day(), 16, 0, 0, 0, Location)) &&
		t.In(Location).Before(time.Date(t.Year(), t.Month(), t.Day(), 20, 0, 0, 0, Location))
}

func IsMarketOpen(t time.Time) bool {
	return IsRegularSession(t) || IsPreMarketSession(t) || IsAfterHoursSession(t)
}

func IsMarketClosed(t time.Time) bool {
	return !IsMarketOpen(t)
}

func IsSameDay(t1, t2 time.Time) bool {
	return t1.In(Location).Year() == t2.In(Location).Year() &&
		t1.In(Location).Month() == t2.In(Location).Month() &&
		t1.In(Location).Day() == t2.In(Location).Day()
}