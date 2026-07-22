package holiday

import "time"

// Holiday represents a single Indonesian national holiday.
type Holiday struct {
	Date time.Time // the calendar date of the holiday
	Day  string    // Indonesian weekday name (e.g. "Kamis")
	Name string    // description of the holiday
}

// mustDate parses a date in "2006-01-02" format and panics on error.
// It is intended for use with the compile-time holiday tables below.
func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Holidays2026 is the list of Indonesian national holidays in 2026.
var Holidays2026 = []Holiday{
	{mustDate("2026-01-01"), "Kamis", "Tahun Baru 2026 Masehi"},
	{mustDate("2026-01-16"), "Jumat", "Isra Mikraj Nabi Muhammad SAW"},
	{mustDate("2026-02-17"), "Selasa", "Tahun Baru Imlek 2577 Kongzili"},
	{mustDate("2026-03-19"), "Kamis", "Hari Suci Nyepi (Tahun Baru Saka 1948)"},
	{mustDate("2026-03-21"), "Sabtu", "Hari Raya Idul Fitri 1447 Hijriah"},
	{mustDate("2026-03-22"), "Minggu", "Hari Raya Idul Fitri 1447 Hijriah"},
	{mustDate("2026-04-03"), "Jumat", "Wafat Yesus Kristus"},
	{mustDate("2026-04-05"), "Minggu", "Kebangkitan Yesus Kristus (Paskah)"},
	{mustDate("2026-05-01"), "Jumat", "Hari Buruh Internasional"},
	{mustDate("2026-05-14"), "Kamis", "Kenaikan Yesus Kristus"},
	{mustDate("2026-05-27"), "Rabu", "Hari Raya Idul Adha 1447 Hijriah"},
	{mustDate("2026-05-31"), "Minggu", "Hari Raya Waisak 2570 BE"},
	{mustDate("2026-06-01"), "Senin", "Hari Lahir Pancasila"},
	{mustDate("2026-06-16"), "Selasa", "Tahun Baru Islam 1448 Hijriah"},
	{mustDate("2026-08-17"), "Senin", "Hari Kemerdekaan Republik Indonesia"},
	{mustDate("2026-08-25"), "Selasa", "Maulid Nabi Muhammad SAW"},
	{mustDate("2026-12-25"), "Jumat", "Hari Raya Natal (Kelahiran Yesus Kristus)"},
}

// IsHoliday reports whether the given date falls on a holiday in Holidays2026.
// Only the year, month, and day are compared; the time of day is ignored.
func IsHoliday(t time.Time) bool {
	y, m, d := t.Date()
	for _, h := range Holidays2026 {
		hy, hm, hd := h.Date.Date()
		if y == hy && m == hm && d == hd {
			return true
		}
	}
	return false
}

// WIB is the Western Indonesia timezone (UTC+7), used for all working-hours math.
var WIB = time.FixedZone("WIB", 7*60*60)

// Working-hours window (in WIB): a working day runs from workStartHour:00 to
// workEndHour:00, i.e. 09:00–18:00.
const (
	workStartHour = 9
	workEndHour   = 18
)

// IsWorkday reports whether t falls on a working day: a weekday (Mon–Fri) that
// is not an Indonesian national holiday. The time of day is ignored.
func IsWorkday(t time.Time) bool {
	t = t.In(WIB)
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !IsHoliday(t)
}

// BusinessDuration returns the elapsed working time between from and to,
// counting only the minutes that fall inside working hours (09:00–18:00 WIB) on
// working days. Weekends, national holidays, and off-hours are skipped, so a bug
// created Friday 17:00 and measured Monday 10:00 accrues just 2 hours.
// Returns 0 if to is not after from.
func BusinessDuration(from, to time.Time) time.Duration {
	from = from.In(WIB)
	to = to.In(WIB)
	if !to.After(from) {
		return 0
	}

	var total time.Duration
	for cursor := from; cursor.Before(to); {
		y, m, d := cursor.Date()
		dayStart := time.Date(y, m, d, workStartHour, 0, 0, 0, WIB)
		dayEnd := time.Date(y, m, d, workEndHour, 0, 0, 0, WIB)

		if IsWorkday(cursor) {
			segStart := maxTime(cursor, dayStart)
			segEnd := minTime(to, dayEnd)
			if segEnd.After(segStart) {
				total += segEnd.Sub(segStart)
			}
		}

		// advance to 00:00 of the next calendar day
		cursor = time.Date(y, m, d+1, 0, 0, 0, 0, WIB)
	}
	return total
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
