package slsp

import (
	"strings"
	"testing"
)

func TestRenderQuotesTheTwoWaysTheFormatRequires(t *testing.T) {
	// A header text field keeps its delimiters when empty; a detail text field
	// loses them. Both are the Bureau's module's behaviour, and the two must
	// not be confused.
	header := RecordSpec{What: "test header", Fields: []Field{
		{Name: "a", Kind: TokenValue},
		{Name: "b", Kind: HeaderText},
		{Name: "c", Kind: HeaderText},
		{Name: "d", Kind: HeaderAmount},
		{Name: "e", Kind: BareText},
	}}
	if got := render(header, []string{"H", "NAME", "", "0.00", "044"}); got != `H,"NAME","",0.00,044` {
		t.Errorf("header render = %q", got)
	}
	detail := RecordSpec{What: "test detail", Fields: []Field{
		{Name: "a", Kind: TokenValue},
		{Name: "b", Kind: DetailText},
		{Name: "c", Kind: DetailText},
		{Name: "d", Kind: DetailAmount},
		{Name: "e", Kind: DateValue},
	}}
	if got := render(detail, []string{"D", "NAME", "", "0", "10/31/2026"}); got != `D,"NAME",,0,10/31/2026` {
		t.Errorf("detail render = %q", got)
	}
	// An empty record and a record with more values than fields must not
	// panic.
	if got := render(RecordSpec{}, nil); got != "" {
		t.Errorf("rendering nothing gave %q", got)
	}
	if got := render(detail, []string{"D", "X", "", "0", "10/31/2026", "extra"}); !strings.HasSuffix(got, ",extra") {
		t.Errorf("a record with a value too many rendered as %q", got)
	}
}

func TestPutAmountUsesTheRightFormForEachRecord(t *testing.T) {
	// Nil is a single zero digit on a detail record and 0.00 in a header.
	// Everything else carries two decimal places in both.
	detail := RecordSpec{What: "test detail", Fields: []Field{
		{Name: "nil", Kind: DetailAmount},
		{Name: "some", Kind: DetailAmount},
		{Name: "centavo", Kind: DetailAmount},
	}}
	b := newRecord(detail)
	b.putAmount("nil", 0)
	b.putAmount("some", 123456)
	b.putAmount("centavo", 1)
	vals, mismatch := b.done()
	if len(mismatch) != 0 {
		t.Fatalf("unexpected mismatch: %v", mismatch)
	}
	want := []string{"0", "1234.56", "0.01"}
	for i := range want {
		if vals[i] != want[i] {
			t.Errorf("detail amount %d = %q, want %q", i, vals[i], want[i])
		}
	}

	header := RecordSpec{What: "test header", Fields: []Field{
		{Name: "nil", Kind: HeaderAmount},
		{Name: "some", Kind: HeaderAmount},
	}}
	h := newRecord(header)
	h.putAmount("nil", 0)
	h.putAmount("some", 123456)
	hvals, _ := h.done()
	if hvals[0] != "0.00" {
		t.Errorf("a nil header total = %q, want %q", hvals[0], "0.00")
	}
	if hvals[1] != "1234.56" {
		t.Errorf("a header total = %q, want %q", hvals[1], "1234.56")
	}
}

func TestRecordBuilderCatchesALayoutThatHasDrifted(t *testing.T) {
	// This is the check that keeps the layout in one place: the writer names
	// each field as it supplies it, and a disagreement with the table is
	// reported instead of producing a record with its columns shifted.
	spec := RecordSpec{What: "test", Fields: []Field{
		{Name: "first", Kind: TokenValue},
		{Name: "second", Kind: TokenValue},
	}}

	wrongName := newRecord(spec)
	wrongName.put("first", "a")
	wrongName.put("third", "b") // the layout says "second"
	if _, mismatch := wrongName.done(); len(mismatch) == 0 {
		t.Error("a field supplied under the wrong name was not reported")
	} else if !strings.Contains(mismatch[0], "second") || !strings.Contains(mismatch[0], "third") {
		t.Errorf("the report %q does not name both the expected and the supplied field", mismatch[0])
	}

	tooFew := newRecord(spec)
	tooFew.put("first", "a")
	if _, mismatch := tooFew.done(); len(mismatch) == 0 {
		t.Error("a record with a field missing was not reported")
	}

	tooMany := newRecord(spec)
	tooMany.put("first", "a")
	tooMany.put("second", "b")
	tooMany.put("third", "c")
	if _, mismatch := tooMany.done(); len(mismatch) == 0 {
		t.Error("a record with a field too many was not reported")
	}

	right := newRecord(spec)
	right.put("first", "a")
	right.put("second", "b")
	if vals, mismatch := right.done(); len(mismatch) != 0 || len(vals) != 2 {
		t.Errorf("a correct record was reported: %v", mismatch)
	}
}

func TestBuildHeaderAndDetailMatchTheirLayouts(t *testing.T) {
	// The real builders, against the real layouts. Any drift between
	// layout.go and write.go shows up here.
	tin, err := ParseTIN("004123456")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParsePeriod("02/2024") // a leap February, to exercise the month end
	if err != nil {
		t.Fatal(err)
	}
	filer := Filer{
		TIN: tin, RegisteredName: "ABC TRADING CORPORATION", TradeName: "ABC",
		Address: [AddressLines]string{"123 MAIN ST", "MAKATI CITY"}, RDOCode: "044", FiscalYearEndMonth: 12,
	}
	party, err := ParseTIN("123456789")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []ListKind{Sales, Purchases} {
		spec := Layout(k)
		e := Entry{Line: 2, Party: Party{
			TIN: party, RegisteredName: "XYZ CORPORATION",
			Address: [AddressLines]string{"456 EDSA", "QUEZON CITY"},
		}}
		e.Sales = SalesAmounts{Taxable: 10000000, OutputVAT: 1200000}
		e.Purchases = PurchaseAmounts{Services: 10000000, InputVAT: 1200000}
		totals := addUp([]Entry{e}, k, 0)

		hv, hm := buildHeader(spec, filer, p, totals)
		if len(hm) != 0 {
			t.Errorf("%s header does not match its layout: %v", k, hm)
		}
		if f := checkRecord(spec.Header, hv, 0); len(f) != 0 {
			for _, x := range f {
				t.Errorf("%s header failed its own check: %s", k, x)
			}
		}
		// The tax month must be the last day of the month, leap year included.
		if got := hv[len(hv)-2]; got != "02/29/2024" {
			t.Errorf("%s header tax month = %q, want 02/29/2024", k, got)
		}

		dv, dm := buildDetail(spec, e, filer, p)
		if len(dm) != 0 {
			t.Errorf("%s detail does not match its layout: %v", k, dm)
		}
		if f := checkRecord(spec.Detail, dv, e.Line); len(f) != 0 {
			for _, x := range f {
				t.Errorf("%s detail failed its own check: %s", k, x)
			}
		}
		if got := dv[len(dv)-1]; got != "02/29/2024" {
			t.Errorf("%s detail tax month = %q, want 02/29/2024", k, got)
		}
		// The filing taxpayer's TIN repeats, unquoted, in the last but one
		// field of every detail record.
		if got := dv[len(dv)-2]; got != "004123456" {
			t.Errorf("%s detail owner TIN = %q, want the filer's nine digits", k, got)
		}
	}
}

func TestBuildHeaderShortensValuesThatAreTooLong(t *testing.T) {
	tin, _ := ParseTIN("004123456")
	p, _ := ParsePeriod("10/2026")
	long := strings.Repeat("A", 200)
	filer := Filer{
		TIN: tin, RegisteredName: long, TradeName: long,
		Address: [AddressLines]string{long, long}, RDOCode: "044", FiscalYearEndMonth: 12,
	}
	spec := Layout(Sales)
	hv, hm := buildHeader(spec, filer, p, Totals{})
	if len(hm) != 0 {
		t.Fatalf("header does not match its layout: %v", hm)
	}
	if f := checkRecord(spec.Header, hv, 0); len(f) != 0 {
		for _, x := range f {
			t.Errorf("an over-long value was not shortened enough: %s", x)
		}
	}
	if len(hv[3]) != MaxRegisteredNameChars {
		t.Errorf("the registered name came out %d characters, want %d", len(hv[3]), MaxRegisteredNameChars)
	}
	if len(hv[8]) != MaxFilerAddress1Chars {
		t.Errorf("the first address line came out %d characters, want %d", len(hv[8]), MaxFilerAddress1Chars)
	}
}

func TestAddUp(t *testing.T) {
	entries := []Entry{
		{Sales: SalesAmounts{Exempt: 100, ZeroRated: 200, Taxable: 300, OutputVAT: 36}},
		{Sales: SalesAmounts{Exempt: 1, ZeroRated: 2, Taxable: 3, OutputVAT: 1}},
		{Sales: SalesAmounts{}},
	}
	got := addUp(entries, Sales, 0)
	if got.Records != 3 {
		t.Errorf("Records = %d, want 3", got.Records)
	}
	want := SalesAmounts{Exempt: 101, ZeroRated: 202, Taxable: 303, OutputVAT: 37}
	if got.Sales != want {
		t.Errorf("Sales = %+v, want %+v", got.Sales, want)
	}

	pur := []Entry{
		{Purchases: PurchaseAmounts{Exempt: 10, ZeroRated: 20, Services: 30, CapitalGoods: 40, OtherGoods: 50, InputVAT: 14}},
		{Purchases: PurchaseAmounts{Services: 1, InputVAT: 1}},
	}
	gp := addUp(pur, Purchases, 5)
	if gp.Purchases.InputVAT != 15 {
		t.Errorf("total input tax = %d, want 15", gp.Purchases.InputVAT)
	}
	if gp.NonCreditableInputVAT != 5 {
		t.Errorf("non-creditable = %d, want 5", gp.NonCreditableInputVAT)
	}
	if gp.CreditableInputVAT != 10 {
		t.Errorf("creditable = %d, want 10", gp.CreditableInputVAT)
	}
	// The split must always add back up to the total, which the validation
	// module insists on.
	if gp.CreditableInputVAT+gp.NonCreditableInputVAT != gp.Purchases.InputVAT {
		t.Error("the creditable and non-creditable amounts do not add up to the total input tax")
	}
	// No entries at all gives nil totals, not a panic.
	if z := addUp(nil, Sales, 0); z.Records != 0 || z.Sales != (SalesAmounts{}) {
		t.Errorf("addUp(nil) = %+v", z)
	}
}

func TestAddUpIsExactOverManyEntries(t *testing.T) {
	// Centavos are whole numbers, so a thousand records must total exactly,
	// with no drift of the kind a floating-point total would show.
	var entries []Entry
	for i := 0; i < 1000; i++ {
		entries = append(entries, Entry{Sales: SalesAmounts{Taxable: 1, OutputVAT: 1}})
	}
	got := addUp(entries, Sales, 0)
	if got.Sales.Taxable != 1000 || got.Sales.OutputVAT != 1000 {
		t.Errorf("a thousand one-centavo records totalled %d and %d", got.Sales.Taxable, got.Sales.OutputVAT)
	}
}

func TestMonthEndValue(t *testing.T) {
	cases := map[int]string{0: "", 1: "1", 6: "6", 12: "12", -1: "", 13: "", 99: ""}
	for in, want := range cases {
		if got := monthEndValue(in); got != want {
			t.Errorf("monthEndValue(%d) = %q, want %q", in, got, want)
		}
	}
}
