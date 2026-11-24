package slsp

import (
	"errors"
	"strings"
	"testing"
)

var testSpecs = []ColumnSpec{
	{Name: "tin", Aliases: []string{"TIN", "Customer TIN", "tin no"}, Required: true},
	{Name: "name", Aliases: []string{"Registered Name", "Customer Name"}, Required: true},
	{Name: "amount", Aliases: []string{"Gross Amount", "Gross Sales", "amount (PHP)"}},
	{Name: "date", Aliases: []string{"Invoice Date", "Document Date"}},
}

func readString(t *testing.T, s string) *Table {
	t.Helper()
	tbl, err := ReadTable(strings.NewReader(s), testSpecs)
	if err != nil {
		t.Fatalf("ReadTable returned error %v for input %q", err, s)
	}
	return tbl
}

func TestReadTableSeparatorsAndLineEndings(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantSep rune
	}{
		{"comma, unix endings", "TIN,Registered Name\n123-456-789-000,Acme\n", ','},
		{"comma, windows endings", "TIN,Registered Name\r\n123-456-789-000,Acme\r\n", ','},
		{"comma, classic mac endings", "TIN,Registered Name\r123-456-789-000,Acme\r", ','},
		{"semicolon", "TIN;Registered Name\n123-456-789-000;Acme\n", ';'},
		{"tab", "TIN\tRegistered Name\n123-456-789-000\tAcme\n", '\t'},
		{"pipe", "TIN|Registered Name\n123-456-789-000|Acme\n", '|'},
		{"byte order mark", "\ufeffTIN,Registered Name\n123-456-789-000,Acme\n", ','},
		{"no trailing newline", "TIN,Registered Name\n123-456-789-000,Acme", ','},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tbl := readString(t, c.in)
			if tbl.Separator != c.wantSep {
				t.Errorf("Separator = %q, want %q", tbl.Separator, c.wantSep)
			}
			if len(tbl.Rows) != 1 {
				t.Fatalf("read %d rows, want 1", len(tbl.Rows))
			}
			if got := tbl.Rows[0].Get("tin"); got != "123-456-789-000" {
				t.Errorf("tin = %q, want %q", got, "123-456-789-000")
			}
			if got := tbl.Rows[0].Get("name"); got != "Acme" {
				t.Errorf("name = %q, want %q", got, "Acme")
			}
		})
	}
}

func TestReadTableHeaderAliases(t *testing.T) {
	cases := []string{
		"TIN,Registered Name\n123-456-789-000,Acme\n",
		"tin,registered name\n123-456-789-000,Acme\n",
		"TIN NO,CUSTOMER NAME\n123-456-789-000,Acme\n",
		"Customer TIN,Customer Name\n123-456-789-000,Acme\n",
		"  TIN  ,  Registered   Name  \n123-456-789-000,Acme\n",
		"tin_no,customer_name\n123-456-789-000,Acme\n",
		"Tin No.,Customer Name:\n123-456-789-000,Acme\n",
	}
	for _, in := range cases {
		tbl := readString(t, in)
		if got := tbl.Rows[0].Get("tin"); got != "123-456-789-000" {
			t.Errorf("for headings %q the tin was %q", strings.SplitN(in, "\n", 2)[0], got)
		}
		if got := tbl.Rows[0].Get("name"); got != "Acme" {
			t.Errorf("for headings %q the name was %q", strings.SplitN(in, "\n", 2)[0], got)
		}
	}
}

func TestReadTableSkipsRowsAboveTheHeader(t *testing.T) {
	in := "Mabuhay Trading Corporation\nSummary List of Sales\nFor the month of September 2026\n\nTIN,Registered Name,Gross Sales\n123-456-789-000,Acme,1000.00\n"
	tbl := readString(t, in)
	if tbl.HeaderLine != 5 {
		t.Errorf("HeaderLine = %d, want 5", tbl.HeaderLine)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("read %d rows, want 1", len(tbl.Rows))
	}
	if tbl.Rows[0].Line != 6 {
		t.Errorf("the data row is reported on line %d, want 6", tbl.Rows[0].Line)
	}
	if !hasNoteContaining(tbl.Notes, "skipped") {
		t.Errorf("no note mentions the skipped lines: %v", tbl.Notes)
	}
}

func TestReadTableLineNumbersSurviveQuotedNewlines(t *testing.T) {
	// A quoted field containing a line break shifts every line number after
	// it; the reported line must still be the one a text editor shows.
	in := "TIN,Registered Name\n" +
		"123-456-789-000,\"Acme\nTrading\"\n" +
		"987-654-321-000,Bayan Supply\n"
	tbl := readString(t, in)
	if len(tbl.Rows) != 2 {
		t.Fatalf("read %d rows, want 2", len(tbl.Rows))
	}
	if tbl.Rows[0].Line != 2 {
		t.Errorf("the first data row is on line %d, want 2", tbl.Rows[0].Line)
	}
	if tbl.Rows[1].Line != 4 {
		t.Errorf("the second data row is on line %d, want 4 (the quoted newline takes a line)", tbl.Rows[1].Line)
	}
	if got := tbl.Rows[0].Get("name"); got != "Acme\nTrading" {
		t.Errorf("the quoted multi-line name read as %q", got)
	}
}

func TestReadTableShortAndLongRows(t *testing.T) {
	in := "TIN,Registered Name,Gross Sales,Invoice Date\n" +
		"123-456-789-000,Acme\n" + // short: two cells missing
		"987-654-321-000,Bayan,2000.00,09/30/2026,extra,more\n" // long
	tbl := readString(t, in)
	if len(tbl.Rows) != 2 {
		t.Fatalf("read %d rows, want 2", len(tbl.Rows))
	}
	if got := tbl.Rows[0].Get("amount"); got != "" {
		t.Errorf("a missing cell in a short row read as %q, want the empty string", got)
	}
	if got := tbl.Rows[0].Get("date"); got != "" {
		t.Errorf("a missing cell in a short row read as %q, want the empty string", got)
	}
	if got := tbl.Rows[1].Get("date"); got != "09/30/2026" {
		t.Errorf("the long row's date read as %q", got)
	}
	if !hasNoteContaining(tbl.Notes, "past the last heading") {
		t.Errorf("no note mentions the extra cells: %v", tbl.Notes)
	}
}

func TestReadTableSkipsBlankRows(t *testing.T) {
	in := "TIN,Registered Name\n123-456-789-000,Acme\n,\n\n987-654-321-000,Bayan\n,,,\n"
	tbl := readString(t, in)
	if len(tbl.Rows) != 2 {
		t.Fatalf("read %d rows, want 2 (blank rows dropped)", len(tbl.Rows))
	}
	if !hasNoteContaining(tbl.Notes, "blank row") {
		t.Errorf("no note mentions the blank rows: %v", tbl.Notes)
	}
}

func TestReadTableReportsUnrecognisedColumns(t *testing.T) {
	in := "TIN,Registered Name,Remarks,Encoded By\n123-456-789-000,Acme,ok,jm\n"
	tbl := readString(t, in)
	if len(tbl.Unmatched) != 2 {
		t.Errorf("Unmatched = %v, want two entries", tbl.Unmatched)
	}
	if !hasNoteContaining(tbl.Notes, "not recognised") {
		t.Errorf("no note mentions the unrecognised columns: %v", tbl.Notes)
	}
	// The recognised columns still have to be reported with their real spelling.
	if got := tbl.Columns["name"]; got != "Registered Name" {
		t.Errorf("Columns[name] = %q, want %q", got, "Registered Name")
	}
}

func TestReadTableStripsInvisibleCharacters(t *testing.T) {
	in := "TIN,Registered Name\n\u200b123-456-789-000\u200b, Acme \n"
	tbl := readString(t, in)
	if got := tbl.Rows[0].Get("tin"); got != "123-456-789-000" {
		t.Errorf("tin = %q; zero-width characters were not stripped", got)
	}
	if got := tbl.Rows[0].Get("name"); got != "Acme" {
		t.Errorf("name = %q; surrounding spaces were not trimmed", got)
	}
}

func TestReadTableRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty file", "", "empty"},
		{"whitespace only", "   \n\n  \n", "empty"},
		{"no header anywhere", "a,b,c\n1,2,3\n", "no header row"},
		{"header but no data", "TIN,Registered Name\n", "no data"},
		{"header with only blank data", "TIN,Registered Name\n,\n,\n", "no data"},
		{"missing a required column", "TIN,Gross Sales\n123-456-789-000,100.00\n", "no column for name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadTable(strings.NewReader(c.in), testSpecs)
			if err == nil {
				t.Fatalf("ReadTable(%q) succeeded, want an error", c.in)
			}
			if !errors.Is(err, ErrCSV) {
				t.Errorf("error %v does not wrap ErrCSV", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestReadTableMissingColumnErrorListsWhatItFound(t *testing.T) {
	_, err := ReadTable(strings.NewReader("TIN,Gross Sales\n123-456-789-000,100.00\n"), testSpecs)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"name", "TIN", "Gross Sales", "line 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not mention %q", err, want)
		}
	}
}

func TestRowZeroValueIsSafe(t *testing.T) {
	// A Row that was never filled in must answer every question without
	// panicking, because error paths build one.
	var r Row
	if got := r.Get("tin"); got != "" {
		t.Errorf("the zero Row returned %q", got)
	}
	if r.Has("tin") {
		t.Error("the zero Row claims to have a value")
	}
	if !r.IsBlank() {
		t.Error("the zero Row does not report itself blank")
	}
	if len(r.Cells()) != 0 {
		t.Error("the zero Row returned cells")
	}
}

func TestReadTableDoesNotPanicOnOddInput(t *testing.T) {
	// None of these may panic; an error or a Table is both acceptable.
	odd := []string{
		"\x00\x01\x02",
		"\"",
		"TIN,Registered Name\n\"unclosed,Acme\n",
		"TIN,Registered Name\n123,\"quo\"\"ted\"\n",
		"TIN,Registered Name\n123,\"mis\"matched\"\n",
		strings.Repeat(",", 5000),
		strings.Repeat("TIN,Registered Name\n", 200),
		"TIN\n1\n",
		"\ufeff",
		"\n\n\n\n",
	}
	for _, in := range odd {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("ReadTable panicked on %q: %v", truncate(in, 40), p)
				}
			}()
			_, _ = ReadTable(strings.NewReader(in), testSpecs)
		}()
	}
}

func TestReduceHeading(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Gross Sales", "grosssales"},
		{"GROSS_SALES", "grosssales"},
		{"gross-sales", "grosssales"},
		{"Gross Sales (VAT incl.)", "grosssalesvatincl"},
		{"  TIN  ", "tin"},
		{"TIN No.", "tinno"},
		{"", ""},
		{"---", ""},
		{"Amount 2", "amount2"},
	}
	for _, c := range cases {
		if got := reduceHeading(c.in); got != c.want {
			t.Errorf("reduceHeading(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func hasNoteContaining(notes []string, want string) bool {
	for _, n := range notes {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
