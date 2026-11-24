package slsp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrDate is the base of every date or period failure.
var ErrDate = errors.New("not a date")

// DateOrder says how to read a numeric date whose first two parts could be
// either the month or the day. The Philippines writes dates month first, so
// that is the default; a CSV exported by a system configured for day-first
// dates can be read by changing this one setting.
type DateOrder int

const (
	// MonthFirst reads 03/04/2026 as 4 March 2026. This is the default.
	MonthFirst DateOrder = iota
	// DayFirst reads 03/04/2026 as 3 April 2026.
	DayFirst
)

func (o DateOrder) String() string {
	if o == DayFirst {
		return "day first (DD/MM/YYYY)"
	}
	return "month first (MM/DD/YYYY)"
}

// ParseDateOrder turns the setting's name into the setting.
func ParseDateOrder(s string) (DateOrder, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "mdy", "monthfirst", "month-first", "month first", "mm/dd/yyyy":
		return MonthFirst, nil
	case "dmy", "dayfirst", "day-first", "day first", "dd/mm/yyyy":
		return DayFirst, nil
	}
	return MonthFirst, fmt.Errorf("%q is not a date order; use month-first or day-first: %w", s, ErrDate)
}

// monthNames maps the month names and the usual abbreviations onto a month,
// so that a CSV written by a spreadsheet with a text date column still reads.
var monthNames = map[string]time.Month{
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

// ParseDocDate reads the date of one document — an invoice, a receipt, a
// voucher — out of a CSV cell.
//
// These forms are read whatever the date order, because they cannot be
// misread:
//
//	"2026-09-30"    ISO
//	"2026/09/30"    ISO with slashes
//	"30-Sep-2026"   day, month name, year
//	"Sep 30, 2026"  month name, day, year
//	"30 September 2026"
//
// A plain numeric date such as "09/30/2026" is read according to order. Where
// the two readings differ and the stated order gives an impossible month, the
// cell is refused with a message naming the setting, rather than the tool
// quietly swapping the parts and filing the wrong date.
func ParseDocDate(s string, order DateOrder) (time.Time, error) {
	raw := strings.TrimSpace(s)
	t := strings.Map(dropSpaceKeepOne, raw)
	t = strings.TrimSpace(t)
	if t == "" {
		return time.Time{}, fmt.Errorf("the date is blank: %w", ErrDate)
	}
	t = strings.TrimSuffix(t, ",")

	// A month name anywhere makes the date unambiguous.
	if mon, day, year, ok := parseNamedMonth(t); ok {
		return buildDate(year, mon, day, raw)
	}

	parts := strings.FieldsFunc(t, func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == ' '
	})
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("%q is not a date this tool can read: %w", raw, ErrDate)
	}
	for _, p := range parts {
		if p == "" || strings.IndexFunc(p, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return time.Time{}, fmt.Errorf("%q is not a date this tool can read: %w", raw, ErrDate)
		}
	}
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[1])
	c, _ := strconv.Atoi(parts[2])

	// A four-digit first part is an ISO date and needs no guessing.
	if len(parts[0]) == 4 {
		return buildDate(a, time.Month(b), c, raw)
	}
	if len(parts[2]) != 4 {
		if len(parts[2]) != 2 {
			return time.Time{}, fmt.Errorf("%q does not end in a year: %w", raw, ErrDate)
		}
		// A two-digit year is read as 20xx, which covers every period a
		// summary list can be filed for.
		c += 2000
	}
	month, day := a, b
	if order == DayFirst {
		month, day = b, a
	}
	if month < 1 || month > 12 {
		other := "day-first"
		if order == DayFirst {
			other = "month-first"
		}
		return time.Time{}, fmt.Errorf("%q has no month %d when dates are read %s; if this file is %s, change the date order: %w",
			raw, month, order, other, ErrDate)
	}
	return buildDate(c, time.Month(month), day, raw)
}

// parseNamedMonth handles the forms that spell the month out.
func parseNamedMonth(t string) (mon time.Month, day, year int, ok bool) {
	fields := strings.FieldsFunc(strings.ReplaceAll(t, ",", " "), func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == ' '
	})
	if len(fields) != 3 {
		return 0, 0, 0, false
	}
	monIdx := -1
	for i, f := range fields {
		if m, found := monthNames[strings.ToLower(f)]; found {
			monIdx, mon = i, m
			break
		}
	}
	if monIdx < 0 {
		return 0, 0, 0, false
	}
	var nums []int
	for i, f := range fields {
		if i == monIdx {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return 0, 0, 0, false
		}
		nums = append(nums, n)
	}
	if len(nums) != 2 {
		return 0, 0, 0, false
	}
	// Whichever number is bigger than 31, or has four digits, is the year.
	switch {
	case nums[0] > 31:
		year, day = nums[0], nums[1]
	default:
		day, year = nums[0], nums[1]
	}
	if year < 100 {
		year += 2000
	}
	return mon, day, year, true
}

// buildDate rejects a date that does not exist, which is the whole point of
// going through time.Date and checking the result: 31 February and 29 February
// in a common year must not be silently rolled forward into March.
func buildDate(year int, month time.Month, day int, raw string) (time.Time, error) {
	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("%q has no month %d: %w", raw, int(month), ErrDate)
	}
	if day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("%q has no day %d: %w", raw, day, ErrDate)
	}
	if year < 1900 || year > 2999 {
		return time.Time{}, fmt.Errorf("%q has the year %d, which is outside the range this tool accepts: %w", raw, year, ErrDate)
	}
	d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if d.Year() != year || d.Month() != month || d.Day() != day {
		return time.Time{}, fmt.Errorf("%q is not a date that exists — %s %d has %d days: %w",
			raw, month, year, DaysInMonth(year, month), ErrDate)
	}
	return d, nil
}

// DaysInMonth returns the length of a month, taking leap years into account.
func DaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// dropSpaceKeepOne collapses the exotic spaces a spreadsheet emits down to an
// ordinary space, which the field splitter then treats as a separator.
func dropSpaceKeepOne(r rune) rune {
	switch r {
	case '\u00a0', '\u2007', '\u202f', '\u2009', '\t':
		return ' '
	case '\ufeff':
		return -1
	}
	return r
}

// ---------------------------------------------------------------- the period

// Period is the month a summary list covers. Quarterly lists are filed as
// their three months, so a month is the only unit the tool needs.
type Period struct {
	Year  int
	Month time.Month
}

// ParsePeriod reads the period a summary list covers. It accepts
//
//	"09/2026"  "9/2026"  "2026-09"  "2026/09"  "092026"
//	"September 2026"  "Sep 2026"  "Sep-2026"
func ParsePeriod(s string) (Period, error) {
	raw := strings.TrimSpace(s)
	t := strings.TrimSpace(strings.Map(dropSpaceKeepOne, raw))
	if t == "" {
		return Period{}, fmt.Errorf("the period is blank: %w", ErrDate)
	}

	fields := strings.FieldsFunc(t, func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == ' '
	})
	// A month name, in either order.
	if len(fields) == 2 {
		for i, f := range fields {
			if m, ok := monthNames[strings.ToLower(f)]; ok {
				y, err := strconv.Atoi(fields[1-i])
				if err != nil {
					return Period{}, fmt.Errorf("%q does not name a year: %w", raw, ErrDate)
				}
				if y < 100 {
					y += 2000
				}
				return newPeriod(y, m, raw)
			}
		}
	}
	switch len(fields) {
	case 1:
		// MMYYYY or YYYYMM run together.
		d := fields[0]
		if len(d) != 6 || strings.IndexFunc(d, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return Period{}, fmt.Errorf("%q is not a period this tool can read: %w", raw, ErrDate)
		}
		if a, _ := strconv.Atoi(d[:4]); a >= 1900 {
			m, _ := strconv.Atoi(d[4:])
			return newPeriod(a, time.Month(m), raw)
		}
		m, _ := strconv.Atoi(d[:2])
		y, _ := strconv.Atoi(d[2:])
		return newPeriod(y, time.Month(m), raw)
	case 2:
		for _, f := range fields {
			if f == "" || strings.IndexFunc(f, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return Period{}, fmt.Errorf("%q is not a period this tool can read: %w", raw, ErrDate)
			}
		}
		a, _ := strconv.Atoi(fields[0])
		b, _ := strconv.Atoi(fields[1])
		if len(fields[0]) == 4 {
			return newPeriod(a, time.Month(b), raw)
		}
		if b < 100 {
			b += 2000
		}
		return newPeriod(b, time.Month(a), raw)
	}
	return Period{}, fmt.Errorf("%q is not a period this tool can read: %w", raw, ErrDate)
}

func newPeriod(year int, month time.Month, raw string) (Period, error) {
	if month < 1 || month > 12 {
		return Period{}, fmt.Errorf("%q has no month %d: %w", raw, int(month), ErrDate)
	}
	if year < 1900 || year > 2999 {
		return Period{}, fmt.Errorf("%q has the year %d, which is outside the range this tool accepts: %w", raw, year, ErrDate)
	}
	return Period{Year: year, Month: month}, nil
}

// Valid reports whether the period has been set.
func (p Period) Valid() bool { return p.Year >= 1900 && p.Month >= 1 && p.Month <= 12 }

// LastDay is the last day of the period's month, which is the date the data
// file carries as the end of the covered period. Month lengths and leap years
// are handled by the calendar, not by a table.
func (p Period) LastDay() time.Time {
	return time.Date(p.Year, p.Month+1, 0, 0, 0, 0, 0, time.UTC)
}

// FirstDay is the first day of the period's month.
func (p Period) FirstDay() time.Time {
	return time.Date(p.Year, p.Month, 1, 0, 0, 0, 0, time.UTC)
}

// Contains reports whether a document date falls inside the period.
func (p Period) Contains(d time.Time) bool {
	return d.Year() == p.Year && d.Month() == p.Month
}

// MMYYYY renders the period as the six digits the data file's name uses.
func (p Period) MMYYYY() string { return fmt.Sprintf("%02d%04d", int(p.Month), p.Year) }

// SlashMMYYYY renders the period as MM/YYYY.
func (p Period) SlashMMYYYY() string { return fmt.Sprintf("%02d/%04d", int(p.Month), p.Year) }

// String renders the period for a person to read.
func (p Period) String() string {
	if !p.Valid() {
		return ""
	}
	return fmt.Sprintf("%s %d", p.Month, p.Year)
}

// Quarter is the calendar quarter the period falls in, 1 to 4.
func (p Period) Quarter() int { return (int(p.Month)-1)/3 + 1 }

// QuarterEnd is the last day of the calendar quarter the period falls in,
// which is what a quarterly summary list covers up to.
func (p Period) QuarterEnd() time.Time {
	endMonth := time.Month(p.Quarter() * 3)
	return time.Date(p.Year, endMonth+1, 0, 0, 0, 0, 0, time.UTC)
}
