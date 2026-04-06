package markethours

import "time"

var Location, _ = time.LoadLocation("America/New_York")

func regularSessionBounds(t time.Time) (time.Time, time.Time) {
	local := t.In(Location)
	open := time.Date(local.Year(), local.Month(), local.Day(), 9, 30, 0, 0, Location)
	close := time.Date(local.Year(), local.Month(), local.Day(), 16, 0, 0, 0, Location)
	return open, close
}

func IsRegularSession(t time.Time) bool {
	local := t.In(Location)
	open, close := regularSessionBounds(local)
	return local.After(open) && local.Before(close)
}

func IsPreMarketSession(t time.Time) bool {
	local := t.In(Location)
	return local.After(time.Date(local.Year(), local.Month(), local.Day(), 7, 0, 0, 0, Location)) &&
		local.Before(time.Date(local.Year(), local.Month(), local.Day(), 9, 30, 0, 0, Location))
}

func IsAfterHoursSession(t time.Time) bool {
	local := t.In(Location)
	return local.After(time.Date(local.Year(), local.Month(), local.Day(), 16, 0, 0, 0, Location)) &&
		local.Before(time.Date(local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, Location))
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

func HasReachedRegularSessionCloseBuffer(t time.Time, lead time.Duration) bool {
	local := t.In(Location)
	_, close := regularSessionBounds(local)
	return !local.Before(close.Add(-lead))
}
