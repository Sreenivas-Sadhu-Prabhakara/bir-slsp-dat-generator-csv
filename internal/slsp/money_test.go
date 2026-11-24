package slsp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int64
	}{
		{"empty cell is zero", "", 0},
		{"spaces only is zero", "   ", 0},
		{"bookkeeper dash is zero", "-", 0},
		{"double dash is zero", "--", 0},
		{"plain zero", "0", 0},
		{"zero with centavos", "0.00", 0},
		{"whole pesos", "1234", 123400},
		{"two decimals", "1234.56", 123456},
		{"one decimal is tenths", "1234.5", 123450},
		{"trailing zeros beyond centavos", "1234.5600", 123456},
		{"thousands separator", "1,234.56", 123456},
		{"two thousands separators", "1,234,567.89", 123456789},
		{"grouping with no decimals", "1,234,567", 123456700},
		{"european grouping", "1.234,56", 123456},
		{"european grouping twice", "1.234.567,89", 123456789},
		{"peso sign", "₱1234.56", 123456},
		{"peso sign with space", "₱ 1,234.56", 123456},
		{"PHP prefix", "PHP 1234.56", 123456},
		{"Php prefix", "Php1,234.56", 123456},
		{"bare P prefix", "P1234.56", 123456},
		{"PHP suffix", "1234.56 PHP", 123456},
		{"no-break space grouping", "1\u00a0234.56", 123456},
		{"narrow no-break space", "1\u202f234.56", 123456},
		{"byte order mark", "\ufeff1234.56", 123456},
		{"accounting negative", "(1,234.56)", -123456},
		{"accounting negative with peso", "(₱1234.56)", -123456},
		{"minus negative", "-1234.56", -123456},
		{"minus with space", "- 1234.56", -123456},
		{"explicit plus", "+1234.56", 123456},
		{"negative inside parens stays negative", "(-1234.56)", 123456},
		{"one centavo", "0.01", 1},
		{"half a peso", ".50", 50},
		{"leading zeros", "000123.45", 12345},
		{"rounding boundary 99 centavos", "9999999.99", 999999999},
		{"large amount", "123456789012.34", 12345678901234},
		{"tab inside", "1\t234.56", 123456},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseAmount(c.in)
			if err != nil {
				t.Fatalf("ParseAmount(%q) returned error %v, want %d", c.in, err, c.want)
			}
			if got != c.want {
				t.Errorf("ParseAmount(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestParseAmountRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"letters", "abc"},
		{"amount with letters", "12a4.56"},
		{"two decimal points", "1.2.3"},
		{"two commas used as decimals", "1,2,3"},
		{"three decimal places", "1234.567"},
		{"four decimal places", "1.2345"},
		{"double sign", "--1234"},
		{"stray sign at end", "1234-"},
		{"unclosed paren", "(1234.56"},
		{"percent", "12%"},
		{"range", "100-200"},
		{"nothing but separators", ",,,"},
		{"malformed grouping, short group", "1,2345.67"},
		{"malformed grouping, long first group", "1234,567"},
		{"indian grouping is not this market's", "12,34,567.89"},
		{"grouping separator with nothing before it", ",123"},
		{"doubled grouping separator", "1,,234"},
		{"decimal point with nothing after it", "1234."},
		{"comma with nothing after it", "1234,"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseAmount(c.in)
			if err == nil {
				t.Fatalf("ParseAmount(%q) = %d, want an error", c.in, got)
			}
			if !errors.Is(err, ErrAmount) {
				t.Errorf("ParseAmount(%q) error %v does not wrap ErrAmount", c.in, err)
			}
			if !strings.Contains(err.Error(), strings.TrimSpace(c.in)) && strings.TrimSpace(c.in) != "" {
				t.Errorf("ParseAmount(%q) error %q does not quote the offending text", c.in, err)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0.00"},
		{1, "0.01"},
		{50, "0.50"},
		{99, "0.99"},
		{100, "1.00"},
		{123456, "1234.56"},
		{12345678901234, "123456789012.34"},
		{-1, "-0.01"},
		{-50, "-0.50"},
		{-123456, "-1234.56"},
		{-100, "-1.00"},
	}
	for _, c := range cases {
		if got := FormatAmount(c.in); got != c.want {
			t.Errorf("FormatAmount(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatAmountNeverCarriesSeparatorsOrSymbols(t *testing.T) {
	// The data file must not contain grouping or a currency marker: the
	// validator rejects such a record, so the formatter must never produce one.
	for _, v := range []int64{0, 1, 999, 100000, 123456789, -123456789, 999999999999} {
		s := FormatAmount(v)
		if strings.ContainsAny(s, ",₱ ") || strings.Contains(s, "PHP") {
			t.Errorf("FormatAmount(%d) = %q, which carries grouping or a currency marker", v, s)
		}
		if parts := strings.SplitN(strings.TrimPrefix(s, "-"), ".", 2); len(parts) != 2 || len(parts[1]) != 2 {
			t.Errorf("FormatAmount(%d) = %q, want exactly two decimal places", v, s)
		}
	}
}

func TestParseAmountFormatAmountRoundTrip(t *testing.T) {
	for _, v := range []int64{0, 1, 7, 99, 100, 101, 99999, 123456, -1, -99, -100, -123456, 100000000000} {
		s := FormatAmount(v)
		back, err := ParseAmount(s)
		if err != nil {
			t.Fatalf("ParseAmount(FormatAmount(%d)) = error %v", v, err)
		}
		if back != v {
			t.Errorf("round trip of %d through %q gave %d", v, s, back)
		}
	}
}

func TestFormatPeso(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "₱0.00"},
		{5, "₱0.05"},
		{123456, "₱1,234.56"},
		{100000000, "₱1,000,000.00"},
		{-123456, "-₱1,234.56"},
		{99900, "₱999.00"},
	}
	for _, c := range cases {
		if got := FormatPeso(c.in); got != c.want {
			t.Errorf("FormatPeso(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
