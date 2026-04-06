package markethours

import "time"

var Location, _ = time.LoadLocation("America/New_York")

func IsMarketDay(t time.Time) bool {
	local := t.In(Location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}

	return !isNYSEHoliday(local)
}

func regularSessionBounds(t time.Time) (time.Time, time.Time) {
	local := t.In(Location)
	open := time.Date(local.Year(), local.Month(), local.Day(), 9, 30, 0, 0, Location)
	close := time.Date(local.Year(), local.Month(), local.Day(), 16, 0, 0, 0, Location)
	return open, close
}

func IsRegularSession(t time.Time) bool {
	if !IsMarketDay(t) {
		return false
	}

	local := t.In(Location)
	open, close := regularSessionBounds(local)
	return local.After(open) && local.Before(close)
}

func IsPreMarketSession(t time.Time) bool {
	if !IsMarketDay(t) {
		return false
	}

	local := t.In(Location)
	return local.After(time.Date(local.Year(), local.Month(), local.Day(), 7, 0, 0, 0, Location)) &&
		local.Before(time.Date(local.Year(), local.Month(), local.Day(), 9, 30, 0, 0, Location))
}

func IsAfterHoursSession(t time.Time) bool {
	if !IsMarketDay(t) {
		return false
	}

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

func isNYSEHoliday(t time.Time) bool {
	local := t.In(Location)
	if isObservedFixedHoliday(local, local.Year(), time.January, 1) || isObservedFixedHoliday(local, local.Year()+1, time.January, 1) {
		return true
	}
	if isNthWeekdayOfMonth(local, time.January, time.Monday, 3) {
		return true
	}
	if isNthWeekdayOfMonth(local, time.February, time.Monday, 3) {
		return true
	}
	if isGoodFriday(local) {
		return true
	}
	if isLastWeekdayOfMonth(local, time.May, time.Monday) {
		return true
	}
	if local.Year() >= 2022 && isObservedFixedHoliday(local, local.Year(), time.June, 19) {
		return true
	}
	if isObservedFixedHoliday(local, local.Year(), time.July, 4) {
		return true
	}
	if isNthWeekdayOfMonth(local, time.September, time.Monday, 1) {
		return true
	}
	if isNthWeekdayOfMonth(local, time.November, time.Thursday, 4) {
		return true
	}
	if isObservedFixedHoliday(local, local.Year(), time.December, 25) {
		return true
	}

	return false
}

func isObservedFixedHoliday(t time.Time, year int, month time.Month, day int) bool {
	observed := observedHoliday(year, month, day)
	return sameDay(t, observed)
}

func observedHoliday(year int, month time.Month, day int) time.Time {
	holiday := time.Date(year, month, day, 0, 0, 0, 0, Location)
	switch holiday.Weekday() {
	case time.Saturday:
		return holiday.AddDate(0, 0, -1)
	case time.Sunday:
		return holiday.AddDate(0, 0, 1)
	default:
		return holiday
	}
}

func isNthWeekdayOfMonth(t time.Time, month time.Month, weekday time.Weekday, nth int) bool {
	local := t.In(Location)
	if local.Month() != month || local.Weekday() != weekday {
		return false
	}

	return (local.Day()-1)/7+1 == nth
}

func isLastWeekdayOfMonth(t time.Time, month time.Month, weekday time.Weekday) bool {
	local := t.In(Location)
	if local.Month() != month || local.Weekday() != weekday {
		return false
	}

	return local.AddDate(0, 0, 7).Month() != month
}

func isGoodFriday(t time.Time) bool {
	local := t.In(Location)
	month, day := easterSunday(local.Year())
	easter := time.Date(local.Year(), month, day, 0, 0, 0, 0, Location)
	return sameDay(local, easter.AddDate(0, 0, -2))
}

func sameDay(left, right time.Time) bool {
	left = left.In(Location)
	right = right.In(Location)
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func easterSunday(year int) (time.Month, int) {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Month(month), day
}
