package slsp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseDocDate(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		order DateOrder
		want  string // YYYY-MM-DD
	}{
		{"month first with slashes", "09/30/2026", MonthFirst, "2026-09-30"},
		{"month first with dashes", "09-30-2026", MonthFirst, "2026-09-30"},
		{"month first without leading zeros", "9/5/2026", MonthFirst, "2026-09-05"},
		{"day first with slashes", "30/09/2026", DayFirst, "2026-09-30"},
		{"day first, ambiguous digits", "03/04/2026", DayFirst, "2026-04-03"},
		{"month first, ambiguous digits", "03/04/2026", MonthFirst, "2026-03-04"},
		{"ISO", "2026-09-30", MonthFirst, "2026-09-30"},
		{"ISO read with day-first order", "2026-09-30", DayFirst, "2026-09-30"},
		{"ISO with slashes", "2026/09/30", MonthFirst, "2026-09-30"},
		{"ISO with dots", "2026.09.30", MonthFirst, "2026-09-30"},
		{"day month-name year", "30-Sep-2026", MonthFirst, "2026-09-30"},
		{"day month-name year, day order", "30-Sep-2026", DayFirst, "2026-09-30"},
		{"month-name day comma year", "Sep 30, 2026", MonthFirst, "2026-09-30"},
		{"full month name", "30 September 2026", MonthFirst, "2026-09-30"},
		{"full month name first", "September 30 2026", MonthFirst, "2026-09-30"},
		{"sept abbreviation", "30-Sept-2026", MonthFirst, "2026-09-30"},
		{"lower case month", "30-sep-2026", MonthFirst, "2026-09-30"},
		{"upper case month", "30-SEP-2026", MonthFirst, "2026-09-30"},
		{"two digit year", "09/30/26", MonthFirst, "2026-09-30"},
		{"two digit year, day first", "30/09/26", DayFirst, "2026-09-30"},
		{"leap day in a leap year", "02/29/2024", MonthFirst, "2024-02-29"},
		{"last day of a short month", "04/30/2026", MonthFirst, "2026-04-30"},
		{"last day of a long month", "01/31/2026", MonthFirst, "2026-01-31"},
		{"new year's day", "01/01/2026", MonthFirst, "2026-01-01"},
		{"new year's eve", "12/31/2026", MonthFirst, "2026-12-31"},
		{"surrounding whitespace", "  09/30/2026  ", MonthFirst, "2026-09-30"},
		{"no-break space inside", "30\u00a0Sep\u00a02026", MonthFirst, "2026-09-30"},
		{"byte order mark", "\ufeff09/30/2026", MonthFirst, "2026-09-30"},
		{"century leap year", "02/29/2000", MonthFirst, "2000-02-29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseDocDate(c.in, c.order)
			if err != nil {
				t.Fatalf("ParseDocDate(%q, %v) returned error %v", c.in, c.order, err)
			}
			if gs := got.Format("2006-01-02"); gs != c.want {
				t.Errorf("ParseDocDate(%q, %v) = %s, want %s", c.in, c.order, gs, c.want)
			}
		})
	}
}

func TestParseDocDateRejects(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		order DateOrder
	}{
		{"blank", "", MonthFirst},
		{"spaces only", "   ", MonthFirst},
		{"not a date", "last Tuesday", MonthFirst},
		{"two parts only", "09/2026", MonthFirst},
		{"four parts", "09/30/2026/01", MonthFirst},
		{"no month 13", "13/01/2026", MonthFirst},
		{"no month 00", "00/15/2026", MonthFirst},
		{"no day 32", "01/32/2026", MonthFirst},
		{"no day 00", "01/00/2026", MonthFirst},
		{"29 February in a common year", "02/29/2026", MonthFirst},
		{"29 February in a century non-leap year", "02/29/1900", MonthFirst},
		{"31 February", "02/31/2024", MonthFirst},
		{"31 April", "04/31/2026", MonthFirst},
		{"31 June", "06/31/2026", MonthFirst},
		{"31 September", "09/31/2026", MonthFirst},
		{"31 November", "11/31/2026", MonthFirst},
		{"day where the month should be", "30/09/2026", MonthFirst},
		{"month where the day should be", "09/30/2026", DayFirst},
		{"letters in a numeric date", "09/AB/2026", MonthFirst},
		{"year out of range", "09/30/1899", MonthFirst},
		{"three digit year", "09/30/202", MonthFirst},
		{"empty parts", "09//2026", MonthFirst},
		{"unknown month name", "30-Sep2-2026", MonthFirst},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseDocDate(c.in, c.order)
			if err == nil {
				t.Fatalf("ParseDocDate(%q, %v) = %s, want an error", c.in, c.order, got.Format("2006-01-02"))
			}
			if !errors.Is(err, ErrDate) {
				t.Errorf("ParseDocDate(%q) error %v does not wrap ErrDate", c.in, err)
			}
		})
	}
}

func TestSwappedDayAndMonthNamesTheSetting(t *testing.T) {
	// A day-first file read month-first must say so, so the user changes the
	// setting rather than hunting for a bad row.
	_, err := ParseDocDate("30/09/2026", MonthFirst)
	if err == nil {
		t.Fatal("30/09/2026 was accepted as a month-first date")
	}
	if !strings.Contains(err.Error(), "day-first") {
		t.Errorf("the error %q does not point at the date-order setting", err)
	}
}

func TestParseDateOrder(t *testing.T) {
	for _, s := range []string{"", "mdy", "MDY", "month-first", "MM/DD/YYYY", "monthfirst"} {
		got, err := ParseDateOrder(s)
		if err != nil || got != MonthFirst {
			t.Errorf("ParseDateOrder(%q) = %v, %v; want MonthFirst, nil", s, got, err)
		}
	}
	for _, s := range []string{"dmy", "DMY", "day-first", "DD/MM/YYYY", "dayfirst"} {
		got, err := ParseDateOrder(s)
		if err != nil || got != DayFirst {
			t.Errorf("ParseDateOrder(%q) = %v, %v; want DayFirst, nil", s, got, err)
		}
	}
	if _, err := ParseDateOrder("ymd"); err == nil {
		t.Error("ParseDateOrder accepted an order it does not support")
	}
}

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		year  int
		month time.Month
		want  int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2024, time.February, 29},
		{2000, time.February, 29},
		{1900, time.February, 28},
		{2100, time.February, 28},
		{2026, time.April, 30},
		{2026, time.June, 30},
		{2026, time.September, 30},
		{2026, time.November, 30},
		{2026, time.December, 31},
	}
	for _, c := range cases {
		if got := DaysInMonth(c.year, c.month); got != c.want {
			t.Errorf("DaysInMonth(%d, %s) = %d, want %d", c.year, c.month, got, c.want)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Period
	}{
		{"MM/YYYY", "09/2026", Period{2026, time.September}},
		{"M/YYYY", "9/2026", Period{2026, time.September}},
		{"YYYY-MM", "2026-09", Period{2026, time.September}},
		{"YYYY/MM", "2026/09", Period{2026, time.September}},
		{"MMYYYY", "092026", Period{2026, time.September}},
		{"YYYYMM", "202609", Period{2026, time.September}},
		{"month name and year", "September 2026", Period{2026, time.September}},
		{"short month name", "Sep 2026", Period{2026, time.September}},
		{"month name with a dash", "Sep-2026", Period{2026, time.September}},
		{"year then month name", "2026 September", Period{2026, time.September}},
		{"MM-YYYY", "09-2026", Period{2026, time.September}},
		{"December", "12/2026", Period{2026, time.December}},
		{"January", "01/2026", Period{2026, time.January}},
		{"two digit year", "09/26", Period{2026, time.September}},
		{"whitespace", "  09/2026 ", Period{2026, time.September}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParsePeriod(c.in)
			if err != nil {
				t.Fatalf("ParsePeriod(%q) returned error %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("ParsePeriod(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParsePeriodRejects(t *testing.T) {
	for _, s := range []string{"", "   ", "13/2026", "00/2026", "2026-13", "Smarch 2026", "2026", "09/30/2026", "abc", "09/", "/2026", "9999999"} {
		if got, err := ParsePeriod(s); err == nil {
			t.Errorf("ParsePeriod(%q) = %v, want an error", s, got)
		}
	}
}

func TestPeriodRendering(t *testing.T) {
	p := Period{2026, time.September}
	if got := p.MMYYYY(); got != "092026" {
		t.Errorf("MMYYYY() = %q, want %q", got, "092026")
	}
	if got := p.SlashMMYYYY(); got != "09/2026" {
		t.Errorf("SlashMMYYYY() = %q, want %q", got, "09/2026")
	}
	if got := p.String(); got != "September 2026" {
		t.Errorf("String() = %q, want %q", got, "September 2026")
	}
	if got := (Period{2026, time.January}).MMYYYY(); got != "012026" {
		t.Errorf("MMYYYY() of January = %q, want %q", got, "012026")
	}
	var z Period
	if z.Valid() {
		t.Error("the zero period reported itself valid")
	}
	if got := z.String(); got != "" {
		t.Errorf("the zero period rendered as %q, want the empty string", got)
	}
}

func TestPeriodBoundaries(t *testing.T) {
	cases := []struct {
		p            Period
		first, last  string
		quarter      int
		quarterEnd   string
		containsThis string
	}{
		{Period{2026, time.February}, "2026-02-01", "2026-02-28", 1, "2026-03-31", "2026-02-28"},
		{Period{2024, time.February}, "2024-02-01", "2024-02-29", 1, "2024-03-31", "2024-02-29"},
		{Period{2026, time.January}, "2026-01-01", "2026-01-31", 1, "2026-03-31", "2026-01-15"},
		{Period{2026, time.March}, "2026-03-01", "2026-03-31", 1, "2026-03-31", "2026-03-31"},
		{Period{2026, time.April}, "2026-04-01", "2026-04-30", 2, "2026-06-30", "2026-04-01"},
		{Period{2026, time.September}, "2026-09-01", "2026-09-30", 3, "2026-09-30", "2026-09-30"},
		{Period{2026, time.October}, "2026-10-01", "2026-10-31", 4, "2026-12-31", "2026-10-31"},
		{Period{2026, time.December}, "2026-12-01", "2026-12-31", 4, "2026-12-31", "2026-12-31"},
	}
	for _, c := range cases {
		if got := c.p.FirstDay().Format("2006-01-02"); got != c.first {
			t.Errorf("%v FirstDay() = %s, want %s", c.p, got, c.first)
		}
		if got := c.p.LastDay().Format("2006-01-02"); got != c.last {
			t.Errorf("%v LastDay() = %s, want %s", c.p, got, c.last)
		}
		if got := c.p.Quarter(); got != c.quarter {
			t.Errorf("%v Quarter() = %d, want %d", c.p, got, c.quarter)
		}
		if got := c.p.QuarterEnd().Format("2006-01-02"); got != c.quarterEnd {
			t.Errorf("%v QuarterEnd() = %s, want %s", c.p, got, c.quarterEnd)
		}
		d, err := ParseDocDate(c.containsThis, MonthFirst)
		if err != nil {
			t.Fatalf("ParseDocDate(%q): %v", c.containsThis, err)
		}
		if !c.p.Contains(d) {
			t.Errorf("%v does not contain %s", c.p, c.containsThis)
		}
	}

	// A date one day either side of the period is outside it.
	p := Period{2026, time.September}
	for _, s := range []string{"2026-08-31", "2026-10-01", "2025-09-15"} {
		d, err := ParseDocDate(s, MonthFirst)
		if err != nil {
			t.Fatalf("ParseDocDate(%q): %v", s, err)
		}
		if p.Contains(d) {
			t.Errorf("%v wrongly contains %s", p, s)
		}
	}
}
