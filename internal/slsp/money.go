package slsp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Amounts are carried as whole centavos in an int64 so that totals are exact
// and never drift. One peso is 100 centavos; the largest value an int64 can
// hold is far beyond any summary list a taxpayer will ever file.

// ErrAmount is the base of every amount-parsing failure, so a caller can test
// for "this was not a number" without matching on the message.
var ErrAmount = errors.New("not an amount")

// currencyMarkers are the ways a spreadsheet or an accounting package writes
// the peso before or after a figure. They are stripped before parsing.
var currencyMarkers = []string{"PHP", "Php", "php", "PhP", "₱", "P"}

// ParseAmount reads one money value out of a CSV cell and returns it in whole
// centavos.
//
// It is deliberately forgiving, because the cell has usually been through a
// spreadsheet first:
//
//	""            -> 0          (a blank cell is zero, not an error)
//	"-"           -> 0          (the dash a bookkeeper writes for nil)
//	"1,234.56"    -> 123456     (thousands separators in any position)
//	"₱ 1 234.56"  -> 123456     (currency marker, ordinary and no-break spaces)
//	"(1,234.56)"  -> -123456    (accounting parentheses for a negative)
//	"1234"        -> 123400     (no decimal point means whole pesos)
//	"1234.5"      -> 123450     (one decimal place is tenths of a peso)
//	"1.234,56"    -> 123456     (European grouping, detected not assumed)
//
// Anything else — letters, two decimal points, a stray sign — is an error
// naming the text that could not be read, never a silent zero and never a
// panic.
func ParseAmount(s string) (int64, error) {
	raw := s
	t := strings.TrimSpace(s)
	t = strings.Map(dropSpace, t)
	if t == "" || t == "-" || t == "--" {
		return 0, nil
	}

	neg := false
	// Accounting parentheses: (1,234.56) is a negative amount.
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg = true
		t = strings.TrimSuffix(strings.TrimPrefix(t, "("), ")")
		t = strings.TrimSpace(t)
	}
	for _, m := range currencyMarkers {
		if strings.HasPrefix(t, m) {
			t = strings.TrimSpace(t[len(m):])
			break
		}
	}
	for _, m := range currencyMarkers {
		if m == "P" { // a trailing bare "P" is more likely a typo than a peso
			continue
		}
		if strings.HasSuffix(t, m) {
			t = strings.TrimSpace(t[:len(t)-len(m)])
			break
		}
	}
	switch {
	case strings.HasPrefix(t, "-"):
		neg = !neg
		t = strings.TrimSpace(t[1:])
	case strings.HasPrefix(t, "+"):
		t = strings.TrimSpace(t[1:])
	}
	if t == "" {
		return 0, fmt.Errorf("%q is %w", raw, ErrAmount)
	}

	intPart, fracPart, err := splitDecimal(t)
	if err != nil {
		return 0, fmt.Errorf("%q is %w", raw, ErrAmount)
	}
	if intPart == "" && fracPart == "" {
		return 0, fmt.Errorf("%q is %w", raw, ErrAmount)
	}
	if intPart == "" {
		intPart = "0"
	}

	// Pad or reject the fraction: centavos are two digits. A third digit that
	// is not zero is a real loss of precision, so say so rather than round it
	// away behind the user's back.
	switch {
	case len(fracPart) == 0:
		fracPart = "00"
	case len(fracPart) == 1:
		fracPart += "0"
	case len(fracPart) > 2:
		for _, c := range fracPart[2:] {
			if c != '0' {
				return 0, fmt.Errorf("%q has more precision than one centavo: %w", raw, ErrAmount)
			}
		}
		fracPart = fracPart[:2]
	}

	pesos, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is %w", raw, ErrAmount)
	}
	centavos, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is %w", raw, ErrAmount)
	}
	const maxPesos = (1<<62 - 1) / 100
	if pesos > maxPesos {
		return 0, fmt.Errorf("%q is too large to be an amount: %w", raw, ErrAmount)
	}
	v := pesos*100 + centavos
	if neg {
		v = -v
	}
	return v, nil
}

// splitDecimal separates the whole part from the fractional part of a bare
// number, working out for itself which of "." and "," is the decimal point and
// which is grouping. The caller has already stripped signs, spaces and
// currency markers, so only digits, "." and "," may remain.
//
// The rules, in order:
//
//   - Both characters present: the one that appears last is the decimal point,
//     and it must appear exactly once. So "1,234.56" and "1.234,56" both read
//     as 1234.56, and "1.234.567,89" as 1234567.89.
//   - Only "," present: more than one means grouping ("1,234,567"). Exactly one
//     with three digits after it is also grouping, because in this market the
//     comma is a thousands separator ("1,234" is one thousand two hundred and
//     thirty-four). Exactly one with one or two digits after it can only be a
//     decimal comma ("1,5" is one peso fifty).
//   - Only "." present: more than one means grouping ("1.234.567"). Exactly one
//     is always the decimal point, however many digits follow — so "1234.567"
//     is three decimal places, which the caller then rejects as finer than one
//     centavo rather than silently reading it as 1,234,567.
//
// The fractional part is returned as written; it is the caller that decides how
// many places are acceptable.
func splitDecimal(t string) (intPart, fracPart string, err error) {
	for _, c := range t {
		if (c < '0' || c > '9') && c != '.' && c != ',' {
			return "", "", ErrAmount
		}
	}
	dots := strings.Count(t, ".")
	commas := strings.Count(t, ",")

	sep := -1
	switch {
	case dots > 0 && commas > 0:
		lastDot, lastComma := strings.LastIndex(t, "."), strings.LastIndex(t, ",")
		if lastDot > lastComma {
			if dots != 1 {
				return "", "", ErrAmount
			}
			sep = lastDot
		} else {
			if commas != 1 {
				return "", "", ErrAmount
			}
			sep = lastComma
		}
	case commas == 1:
		// Three digits after a lone comma is grouping in this market.
		if n := len(t) - strings.IndexByte(t, ',') - 1; n == 1 || n == 2 {
			sep = strings.IndexByte(t, ',')
		}
	case dots == 1:
		sep = strings.IndexByte(t, '.')
	}
	if sep < 0 {
		if !groupingIsSane(t) {
			return "", "", ErrAmount
		}
		return strings.Map(dropGrouping, t), "", nil
	}
	if !groupingIsSane(t[:sep]) {
		return "", "", ErrAmount
	}
	intPart = strings.Map(dropGrouping, t[:sep])
	fracPart = t[sep+1:]
	if fracPart == "" {
		return "", "", ErrAmount
	}
	for _, c := range fracPart {
		if c < '0' || c > '9' {
			return "", "", ErrAmount
		}
	}
	return intPart, fracPart, nil
}

// groupingIsSane checks that the separators left in a whole-number part really
// are thousands separators: at most three digits before the first, then exactly
// three between each pair. Without this check "1.2.3" would read as 123 and
// "1,2345" as 12345, quietly turning a typo into a plausible-looking peso
// figure. Rejecting the cell is always safer: the row is reported by line
// number and the user fixes it, rather than a wrong amount reaching the file.
func groupingIsSane(whole string) bool {
	if whole == "" {
		return true // ".50" has no whole part, which is fine
	}
	if !strings.ContainsAny(whole, ".,") {
		return len(whole) > 0
	}
	parts := strings.FieldsFunc(whole, func(r rune) bool { return r == '.' || r == ',' })
	// FieldsFunc drops empty fields, so count the separators instead of
	// trusting the slice length: ",123" and "1,,234" must both fail.
	if len(parts) != strings.Count(whole, ".")+strings.Count(whole, ",")+1 {
		return false
	}
	if len(parts[0]) < 1 || len(parts[0]) > 3 {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) != 3 {
			return false
		}
	}
	return true
}

func dropGrouping(r rune) rune {
	if r == ',' || r == '.' {
		return -1
	}
	return r
}

// dropSpace removes every kind of space a spreadsheet can emit, including the
// no-break space Excel uses when it formats a number for display.
func dropSpace(r rune) rune {
	switch r {
	case ' ', '\t', '\u00a0', '\u2007', '\u202f', '\u2009', '\ufeff':
		return -1
	}
	return r
}

// FormatAmount renders centavos the way the data file wants them: a plain
// decimal with exactly two places, no thousands separator, no currency marker,
// and a leading minus for a negative.
//
//	123456  -> "1234.56"
//	-50     -> "-0.50"
//	0       -> "0.00"
func FormatAmount(centavos int64) string {
	neg := centavos < 0
	if neg {
		centavos = -centavos
	}
	s := fmt.Sprintf("%d.%02d", centavos/100, centavos%100)
	if neg {
		return "-" + s
	}
	return s
}

// FormatPeso renders centavos for a person to read, with grouping and a peso
// sign. It is used in the interface and in error messages, never in the data
// file.
func FormatPeso(centavos int64) string {
	neg := centavos < 0
	if neg {
		centavos = -centavos
	}
	whole := strconv.FormatInt(centavos/100, 10)
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := fmt.Sprintf("₱%s.%02d", b.String(), centavos%100)
	if neg {
		return "-" + out
	}
	return out
}
