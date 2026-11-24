package slsp

import (
	"strings"
	"testing"
)

// The layout is the product, so these tests check its shape against the
// Bureau's module independently of the code that writes it: the field counts,
// the tokens, the positions of the fields that are pinned by the file name and
// by the validation module, and the widths.

func TestRecordWidths(t *testing.T) {
	// The field counts come from the Bureau's own module: a sales file is 17
	// fields of header and 15 of detail, a purchases file 21 and 17.
	cases := []struct {
		kind           ListKind
		header, detail int
	}{
		{Sales, 17, 15},
		{Purchases, 21, 17},
	}
	for _, c := range cases {
		f := Layout(c.kind)
		if got := f.Header.Width(); got != c.header {
			t.Errorf("%s header has %d fields, want %d", c.kind, got, c.header)
		}
		if got := f.Detail.Width(); got != c.detail {
			t.Errorf("%s detail has %d fields, want %d", c.kind, got, c.detail)
		}
		for _, rec := range f.Records() {
			if rec.Width() > MaxFieldsPerRecord {
				t.Errorf("%s has %d fields, over the %d the validation module reads", rec.What, rec.Width(), MaxFieldsPerRecord)
			}
		}
	}
}

func TestRecordTokensAndTransactionCodes(t *testing.T) {
	if RecordTypeHeader != "H" || RecordTypeDetail != "D" {
		t.Errorf("the record tokens are %q and %q, want H and D", RecordTypeHeader, RecordTypeDetail)
	}
	if Sales.TransactionCode() != "S" {
		t.Errorf("the sales transaction code is %q, want S", Sales.TransactionCode())
	}
	if Purchases.TransactionCode() != "P" {
		t.Errorf("the purchases transaction code is %q, want P", Purchases.TransactionCode())
	}
	for _, k := range []ListKind{Sales, Purchases} {
		f := Layout(k)
		if f.Header.Token != RecordTypeHeader {
			t.Errorf("%s opens with %q, want %q", f.Header.What, f.Header.Token, RecordTypeHeader)
		}
		if f.Detail.Token != RecordTypeDetail {
			t.Errorf("%s opens with %q, want %q", f.Detail.What, f.Detail.Token, RecordTypeDetail)
		}
		// There is no third record kind: no trailer, no control record.
		if n := len(f.Records()); n != 2 {
			t.Errorf("%s has %d kinds of record, want 2 — the header carries the totals, so there is no trailer", k, n)
		}
	}
}

func TestPinnedFieldPositions(t *testing.T) {
	// These positions are not free: the validation module reads each of them
	// by number, so moving one silently breaks the file.
	cases := []struct {
		kind     ListKind
		record   string
		position int // 1-based
		name     string
	}{
		{Sales, "header", 1, "record type"},
		{Sales, "header", 2, "transaction code"},
		{Sales, "header", 3, "TIN of owner"},
		{Sales, "header", 15, "RDO code"},
		{Sales, "header", 16, "tax month"},
		{Sales, "header", 17, "fiscal or calendar month end"},
		{Sales, "detail", 3, "TIN"},
		{Sales, "detail", 10, "exempt sales"},
		{Sales, "detail", 11, "zero-rated sales"},
		{Sales, "detail", 12, "taxable sales"},
		{Sales, "detail", 13, "output tax"},
		{Sales, "detail", 14, "TIN of owner"},
		{Sales, "detail", 15, "tax month"},
		{Purchases, "header", 16, "total input tax"},
		{Purchases, "header", 17, "creditable input tax"},
		{Purchases, "header", 18, "non-creditable input tax"},
		{Purchases, "header", 19, "RDO code"},
		{Purchases, "header", 20, "tax month"},
		{Purchases, "header", 21, "fiscal or calendar month end"},
		{Purchases, "detail", 3, "TIN"},
		{Purchases, "detail", 10, "exempt purchases"},
		{Purchases, "detail", 12, "taxable purchases of services"},
		{Purchases, "detail", 13, "taxable purchases of capital goods"},
		{Purchases, "detail", 14, "taxable purchases of other goods"},
		{Purchases, "detail", 15, "input tax"},
		{Purchases, "detail", 16, "TIN of owner"},
		{Purchases, "detail", 17, "tax month"},
	}
	for _, c := range cases {
		f := Layout(c.kind)
		rec := f.Detail
		if c.record == "header" {
			rec = f.Header
		}
		if c.position > rec.Width() {
			t.Errorf("%s has only %d fields, so there is no field %d", rec.What, rec.Width(), c.position)
			continue
		}
		if got := rec.Fields[c.position-1].Name; got != c.name {
			t.Errorf("field %d of the %s is %q, want %q", c.position, rec.What, got, c.name)
		}
	}
}

func TestFieldWidthsMatchTheModulesColumns(t *testing.T) {
	cases := []struct {
		kind     ListKind
		record   string
		field    string
		wantWide int
	}{
		{Sales, "detail", "TIN", 9},
		{Sales, "detail", "registered name", 50},
		{Sales, "detail", "last name", 30},
		{Sales, "detail", "first name", 30},
		{Sales, "detail", "middle name", 30},
		{Sales, "detail", "address line 1", 30},
		{Sales, "detail", "address line 2", 30},
		{Sales, "header", "owner registered name", 50},
		{Sales, "header", "owner trade name", 50},
		{Sales, "header", "RDO code", 3},
	}
	for _, c := range cases {
		f := Layout(c.kind)
		rec := f.Detail
		if c.record == "header" {
			rec = f.Header
		}
		found := false
		for _, fl := range rec.Fields {
			if fl.Name == c.field {
				found = true
				if fl.Max != c.wantWide {
					t.Errorf("%s field %q holds %d characters, want %d", rec.What, c.field, fl.Max, c.wantWide)
				}
			}
		}
		if !found {
			t.Errorf("%s has no field %q", rec.What, c.field)
		}
	}
}

func TestEveryFieldIsUsableByTheWriterAndValidator(t *testing.T) {
	for _, k := range []ListKind{Sales, Purchases} {
		for _, rec := range Layout(k).Records() {
			seen := map[string]bool{}
			for i, f := range rec.Fields {
				if f.Name == "" {
					t.Errorf("field %d of the %s has no name", i+1, rec.What)
				}
				// Two fields with the same name in one record would make the
				// writer's position check useless.
				if seen[f.Name] {
					t.Errorf("the %s has two fields called %q", rec.What, f.Name)
				}
				seen[f.Name] = true
				if f.Max < 0 {
					t.Errorf("field %q of the %s has a negative width", f.Name, rec.What)
				}
				if f.Max > MaxFieldChars {
					t.Errorf("field %q of the %s holds %d characters, over the %d the validation module reads",
						f.Name, rec.What, f.Max, MaxFieldChars)
				}
			}
		}
	}
}

func TestNoLayoutFieldIsLeftUncertain(t *testing.T) {
	// Every field was read off the Bureau's own module. If a change
	// introduces one that was not, this test makes it visible rather than
	// letting it ship unannounced — and Describe reports it to the user.
	for _, k := range []ListKind{Sales, Purchases} {
		if u := Layout(k).UncertainFields(); len(u) > 0 {
			t.Logf("%s has fields marked uncertain, which the documentation must list: %v", k, u)
		}
		d := Describe(k)
		if len(d.Uncertain) != len(Layout(k).UncertainFields()) {
			t.Errorf("%s: Describe reports %d uncertain fields but the layout has %d", k, len(d.Uncertain), len(Layout(k).UncertainFields()))
		}
	}
}

func TestQuotingRulesAreTheModulesTwo(t *testing.T) {
	// A header text field always carries its delimiters; a detail text field
	// carries them only when it has a value. Nothing else is quoted.
	for _, k := range []ListKind{Sales, Purchases} {
		f := Layout(k)
		for _, fl := range f.Header.Fields {
			switch fl.Kind {
			case HeaderText:
				if !fl.Kind.Quoted() || !fl.Kind.QuotedWhenEmpty() {
					t.Errorf("%s field %q should always be quoted", f.Header.What, fl.Name)
				}
			case HeaderAmount, DateValue, BareText, TokenValue, OptionalNumber:
				if fl.Kind.Quoted() {
					t.Errorf("%s field %q should not be quoted", f.Header.What, fl.Name)
				}
			default:
				t.Errorf("%s field %q has kind %v, which does not belong in a header", f.Header.What, fl.Name, fl.Kind)
			}
		}
		for _, fl := range f.Detail.Fields {
			switch fl.Kind {
			case DetailText:
				if !fl.Kind.Quoted() || fl.Kind.QuotedWhenEmpty() {
					t.Errorf("%s field %q should be quoted only when it has a value", f.Detail.What, fl.Name)
				}
			case DetailAmount, DateValue, BareText, TokenValue:
				if fl.Kind.Quoted() {
					t.Errorf("%s field %q should not be quoted", f.Detail.What, fl.Name)
				}
			default:
				t.Errorf("%s field %q has kind %v, which does not belong in a detail record", f.Detail.What, fl.Name, fl.Kind)
			}
		}
	}
}

func TestFileName(t *testing.T) {
	p, err := ParsePeriod("10/2026")
	if err != nil {
		t.Fatal(err)
	}
	tin, err := ParseTIN("004-123-456-000")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind ListKind
		want string
	}{
		{Sales, "004123456S102026.DAT"},
		{Purchases, "004123456P102026.DAT"},
	}
	for _, c := range cases {
		if got := FileName(tin, p, c.kind); got != c.want {
			t.Errorf("FileName(%s) = %q, want %q", c.kind, got, c.want)
		}
	}
	// The validation module reads the name back apart by position, so the
	// offsets have to be exact: nine of TIN, one letter, two of month, four
	// of year.
	name := FileName(tin, p, Sales)
	base := strings.TrimSuffix(name, FileExtension)
	if len(base) != 16 {
		t.Fatalf("the name %q has a %d-character base, want 16", name, len(base))
	}
	if base[0:9] != "004123456" {
		t.Errorf("characters 1 to 9 of %q are %q, want the TIN", base, base[0:9])
	}
	if base[9:10] != "S" {
		t.Errorf("character 10 of %q is %q, want the list letter", base, base[9:10])
	}
	if base[10:12] != "10" {
		t.Errorf("characters 11 and 12 of %q are %q, want the month", base, base[10:12])
	}
	if base[12:16] != "2026" {
		t.Errorf("characters 13 to 16 of %q are %q, want the year", base, base[12:16])
	}

	// January, to check the month is zero padded.
	jan, _ := ParsePeriod("01/2026")
	if got := FileName(tin, jan, Sales); got != "004123456S012026.DAT" {
		t.Errorf("FileName for January = %q, want %q", got, "004123456S012026.DAT")
	}
	// Nothing is produced without a usable TIN or period.
	if got := FileName(TIN{}, p, Sales); got != "" {
		t.Errorf("FileName with no TIN = %q, want the empty string", got)
	}
	if got := FileName(tin, Period{}, Sales); got != "" {
		t.Errorf("FileName with no period = %q, want the empty string", got)
	}
}

func TestParseListKind(t *testing.T) {
	for _, s := range []string{"sales", "Sales", "SALES", "s", "SLS", "sale", "Summary List of Sales"} {
		if got, ok := ParseListKind(s); !ok || got != Sales {
			t.Errorf("ParseListKind(%q) = %v, %v; want Sales, true", s, got, ok)
		}
	}
	for _, s := range []string{"purchases", "Purchases", "p", "SLP", "purchase", "Summary List of Purchases"} {
		if got, ok := ParseListKind(s); !ok || got != Purchases {
			t.Errorf("ParseListKind(%q) = %v, %v; want Purchases, true", s, got, ok)
		}
	}
	for _, s := range []string{"", "importations", "both", "x"} {
		if _, ok := ParseListKind(s); ok {
			t.Errorf("ParseListKind(%q) was accepted", s)
		}
	}
}

func TestDescribeCoversEveryField(t *testing.T) {
	for _, k := range []ListKind{Sales, Purchases} {
		d := Describe(k)
		f := Layout(k)
		if len(d.Records) != len(f.Records()) {
			t.Fatalf("%s: Describe has %d records, the layout has %d", k, len(d.Records), len(f.Records()))
		}
		for i, rec := range f.Records() {
			if len(d.Records[i].Fields) != rec.Width() {
				t.Errorf("%s: Describe has %d fields for the %s, the layout has %d",
					k, len(d.Records[i].Fields), rec.What, rec.Width())
			}
			for j, fd := range d.Records[i].Fields {
				if fd.Position != j+1 {
					t.Errorf("%s field %d of the %s is numbered %d", k, j+1, rec.What, fd.Position)
				}
				if fd.Name != rec.Fields[j].Name {
					t.Errorf("%s: Describe calls field %d of the %s %q, the layout calls it %q",
						k, j+1, rec.What, fd.Name, rec.Fields[j].Name)
				}
			}
		}
		if d.FieldSeparator != "," || d.TextDelimiter != "\"" {
			t.Errorf("%s: Describe reports separator %q and delimiter %q", k, d.FieldSeparator, d.TextDelimiter)
		}
		if d.RecordSeparator != "CR LF" {
			t.Errorf("%s: Describe reports record separator %q", k, d.RecordSeparator)
		}
	}
}

func TestConfidenceAndSeverityNames(t *testing.T) {
	for v, want := range map[Confidence]string{Sourced: "sourced", Corroborated: "corroborated", Uncertain: "uncertain"} {
		if got := v.String(); got != want {
			t.Errorf("Confidence(%d).String() = %q, want %q", v, got, want)
		}
	}
	if Fatal.String() != "error" || Warning.String() != "warning" {
		t.Errorf("the severities render as %q and %q", Fatal, Warning)
	}
	if Sales.Party() != "customer" || Purchases.Party() != "supplier" {
		t.Errorf("the parties are %q and %q", Sales.Party(), Purchases.Party())
	}
}
