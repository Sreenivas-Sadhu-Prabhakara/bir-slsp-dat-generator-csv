package slsp

import (
	"strings"
	"testing"
)

func TestLooksLikeAmount(t *testing.T) {
	cases := []struct {
		in           string
		detail, head bool
	}{
		{"0", true, false}, // a nil detail amount, but not a header total
		{"0.00", true, true},
		{"1234.56", true, true},
		{"0.01", true, true},
		{"-1234.56", true, true},
		{"-0.01", true, true},
		{"123456789012.34", true, true},
		{"", false, false},
		{"1234", false, false},     // no decimal places
		{"1234.5", false, false},   // one decimal place
		{"1234.567", false, false}, // three
		{"1,234.56", false, false}, // a thousands separator
		{"₱1234.56", false, false},
		{"1234.56 ", false, false},
		{" 1234.56", false, false},
		{"--1.00", false, false},
		{"1234.56-", false, false},
		{"(1234.56)", false, false},
		{"abc", false, false},
		{".56", false, false},
		{"1234.", false, false},
		{"00", false, false},
	}
	for _, c := range cases {
		if got := looksLikeAmount(c.in, DetailAmount); got != c.detail {
			t.Errorf("looksLikeAmount(%q, detail) = %v, want %v", c.in, got, c.detail)
		}
		if got := looksLikeAmount(c.in, HeaderAmount); got != c.head {
			t.Errorf("looksLikeAmount(%q, header) = %v, want %v", c.in, got, c.head)
		}
	}
}

func TestEveryAmountTheWriterProducesPassesItsOwnCheck(t *testing.T) {
	// The writer and the validator must agree exactly, or a correct file
	// would be refused.
	for _, v := range []int64{0, 1, 50, 99, 100, 123456, -1, -100, -123456, 999999999999} {
		d := FormatAmount(v)
		if v == 0 {
			d = "0"
		}
		if !looksLikeAmount(d, DetailAmount) {
			t.Errorf("the detail form of %d is %q, which the validator refuses", v, d)
		}
		if !looksLikeAmount(FormatAmount(v), HeaderAmount) {
			t.Errorf("the header form of %d is %q, which the validator refuses", v, FormatAmount(v))
		}
	}
}

func TestLooksLikeDate(t *testing.T) {
	good := []string{"10/31/2026", "01/01/2026", "02/29/2024", "12/31/1999", "02/28/2026", "01/31/1950"}
	bad := []string{
		"", "10/31/26", "2026-10-31", "31/10/2026", "1/1/2026", "10-31-2026",
		"02/29/2026", "02/30/2024", "13/01/2026", "00/15/2026", "10/00/2026",
		"10/32/2026", "10/31/1949", "10/31/2026 ", " 10/31/2026", "aa/bb/cccc",
		"10/31/20266", "10//31/2026",
	}
	for _, s := range good {
		if !looksLikeDate(s) {
			t.Errorf("looksLikeDate(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if looksLikeDate(s) {
			t.Errorf("looksLikeDate(%q) = true, want false", s)
		}
	}
}

func TestOffendingCharacters(t *testing.T) {
	cases := []struct {
		in       string
		hasError bool
	}{
		{"ACME TRADING CORPORATION", false},
		{"SAN MIGUEL CORP.", false},
		{"7-ELEVEN #1234", false},
		{"A/B (REAR) 100%", false},
		{"PEÑA", false}, // the one accented letter the format carries
		{"SMITH & SONS", true},
		{"O'BRIEN", true},
		{"CASH $ CARRY", true},
		{"A,B", true},
		{`A"B`, true},
		{"A\\B", true},
		{"A|B", true},
		{"A`B", true},
		{"A~B", true},
		{"éCLAIR", true},
		{"ñ", true}, // lower case is folded to upper before this point
	}
	for _, c := range cases {
		got := offendingCharacters(c.in)
		if (got != "") != c.hasError {
			t.Errorf("offendingCharacters(%q) = %q, want an error: %v", c.in, got, c.hasError)
		}
	}
}

// --- checkRecord

func testRecordSpec() RecordSpec {
	return RecordSpec{What: "test detail", Token: "D", Fields: []Field{
		{Name: "record type", Kind: TokenValue, Max: 1, Required: true},
		{Name: "name", Kind: DetailText, Max: 10},
		{Name: "owner", Kind: BareText, Max: 9, Required: true},
		{Name: "amount", Kind: DetailAmount, Required: true},
		{Name: "when", Kind: DateValue, Required: true},
		{Name: "month end", Kind: OptionalNumber, Max: 2},
	}}
}

func TestCheckRecord(t *testing.T) {
	spec := testRecordSpec()
	cases := []struct {
		name      string
		values    []string
		wantFatal bool
		mentions  string
	}{
		{"a good record", []string{"D", "ACME", "004123456", "0", "10/31/2026", "12"}, false, ""},
		{"empty optional number", []string{"D", "ACME", "004123456", "0", "10/31/2026", ""}, false, ""},
		{"empty optional text", []string{"D", "", "004123456", "0", "10/31/2026", "12"}, false, ""},
		{"a required field left empty", []string{"D", "ACME", "", "0", "10/31/2026", "12"}, true, "may not be empty"},
		{"a value too long", []string{"D", "ACMETRADINGCORP", "004123456", "0", "10/31/2026", "12"}, true, "at most 10 characters"},
		{"a separator inside a value", []string{"D", "AC,ME", "004123456", "0", "10/31/2026", "12"}, true, "field separator"},
		{"a delimiter inside a value", []string{"D", `AC"ME`, "004123456", "0", "10/31/2026", "12"}, true, "text delimiter"},
		{"a line break inside a value", []string{"D", "AC\nME", "004123456", "0", "10/31/2026", "12"}, true, "line break"},
		{"a refused character", []string{"D", "A&B", "004123456", "0", "10/31/2026", "12"}, true, "refuses these characters"},
		{"letters where digits belong", []string{"D", "ACME", "00412345X", "0", "10/31/2026", "12"}, true, "digits only"},
		{"a badly formed amount", []string{"D", "ACME", "004123456", "1,234.56", "10/31/2026", "12"}, true, "decimal places"},
		{"a badly formed date", []string{"D", "ACME", "004123456", "0", "31/10/2026", "12"}, true, "MM/DD/YYYY"},
		{"a non-numeric month end", []string{"D", "ACME", "004123456", "0", "10/31/2026", "xx"}, true, "whole number"},
		{"a token the format does not use", []string{"X", "ACME", "004123456", "0", "10/31/2026", "12"}, true, "not one of the tokens"},
		{"a multi-letter token", []string{"DSLS", "ACME", "004123456", "0", "10/31/2026", "12"}, true, "not one of the tokens"},
		{"too few fields", []string{"D", "ACME", "004123456"}, true, "this is a fault in the tool"},
		{"too many fields", []string{"D", "ACME", "004123456", "0", "10/31/2026", "12", "x"}, true, "this is a fault in the tool"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkRecord(spec, c.values, 7)
			if anyFatal(got) != c.wantFatal {
				t.Fatalf("checkRecord(%v) gave %v, want a fatal finding: %v", c.values, got, c.wantFatal)
			}
			if !c.wantFatal {
				return
			}
			joined := ""
			for _, f := range got {
				joined += f.String() + "\n"
				if f.Line != 7 {
					t.Errorf("the finding is reported on line %d, want 7", f.Line)
				}
			}
			if c.mentions != "" && !strings.Contains(joined, c.mentions) {
				t.Errorf("the findings %q do not mention %q", joined, c.mentions)
			}
		})
	}
}

func TestCheckRecordRefusesALineTooLongForTheSubmissionRoutine(t *testing.T) {
	// The routine that appends the submission key reads only the first 600
	// characters of a line and drops the rest without a word, which would
	// change the figures. A record that long has to be refused.
	spec := RecordSpec{What: "test", Fields: []Field{
		{Name: "a", Kind: DetailText, Max: MaxFieldChars},
		{Name: "b", Kind: DetailText, Max: MaxFieldChars},
		{Name: "c", Kind: DetailText, Max: MaxFieldChars},
	}}
	long := strings.Repeat("A", 250)
	got := checkRecord(spec, []string{long, long, long}, 3)
	if !anyFatal(got) {
		t.Fatal("a record of over 600 characters was accepted")
	}
	found := false
	for _, f := range got {
		if strings.Contains(f.Detail, "600") {
			found = true
		}
	}
	if !found {
		t.Errorf("the findings do not mention the line-length ceiling: %v", got)
	}
}

// --- checkFiler

func goodFiler(t *testing.T) Filer {
	t.Helper()
	tin, err := ParseTIN("004-123-456-000")
	if err != nil {
		t.Fatal(err)
	}
	return Filer{
		TIN: tin, RegisteredName: "ABC TRADING CORPORATION",
		Address: [AddressLines]string{"123 MAIN ST", "MAKATI CITY"}, RDOCode: "044", FiscalYearEndMonth: 12,
	}
}

func TestCheckFiler(t *testing.T) {
	base := goodFiler(t)
	if f := checkFiler(base, Sales); anyFatal(f) {
		t.Fatalf("a good filer was refused: %v", f)
	}

	cases := []struct {
		name     string
		mutate   func(*Filer)
		mentions string
	}{
		{"no TIN", func(f *Filer) { f.TIN = TIN{} }, "exactly 9 digits"},
		{"no name at all", func(f *Filer) { f.RegisteredName = "" }, "has no name"},
		{"both kinds of name", func(f *Filer) { f.Person = PersonName{Last: "SANTOS", First: "MARIA"} }, "one or the other"},
		{"no address", func(f *Filer) { f.Address = [AddressLines]string{} }, "no address"},
		{"no RDO code", func(f *Filer) { f.RDOCode = "" }, "exactly 3 digits"},
		{"a two-digit RDO code", func(f *Filer) { f.RDOCode = "44" }, "written 044"},
		{"a four-digit RDO code", func(f *Filer) { f.RDOCode = "0044" }, "exactly 3 digits"},
		{"a non-numeric RDO code", func(f *Filer) { f.RDOCode = "04A" }, "exactly 3 digits"},
		{"a month end of 13", func(f *Filer) { f.FiscalYearEndMonth = 13 }, "1 to 12"},
		{"a negative month end", func(f *Filer) { f.FiscalYearEndMonth = -1 }, "1 to 12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := base
			c.mutate(&f)
			got := checkFiler(f, Sales)
			if !anyFatal(got) {
				t.Fatalf("%s was accepted", c.name)
			}
			joined := ""
			for _, x := range got {
				joined += x.String() + "\n"
			}
			if !strings.Contains(joined, c.mentions) {
				t.Errorf("the findings %q do not mention %q", joined, c.mentions)
			}
		})
	}

	// A trade name on its own satisfies the name rule, and so does an
	// individual's name. A month end of zero is allowed and leaves the field
	// empty.
	for _, ok := range []func(*Filer){
		func(f *Filer) { f.RegisteredName = ""; f.TradeName = "ABC TRADING" },
		func(f *Filer) { f.RegisteredName = ""; f.Person = PersonName{Last: "SANTOS", First: "MARIA"} },
		func(f *Filer) { f.FiscalYearEndMonth = 0 },
		func(f *Filer) { f.Address = [AddressLines]string{"", "MAKATI CITY"} },
	} {
		f := base
		ok(&f)
		if got := checkFiler(f, Sales); anyFatal(got) {
			t.Errorf("an acceptable filer was refused: %v", got)
		}
	}
}

func TestCheckFilerWarnsAboutADiscardedBranchCode(t *testing.T) {
	f := goodFiler(t)
	tin, err := ParseTIN("004-123-456-005")
	if err != nil {
		t.Fatal(err)
	}
	f.TIN = tin
	got := checkFiler(f, Sales)
	if anyFatal(got) {
		t.Fatalf("a branch TIN was refused: %v", got)
	}
	found := false
	for _, x := range got {
		if x.Severity == Warning && strings.Contains(x.Detail, "no field for one") {
			found = true
		}
	}
	if !found {
		t.Errorf("the discarded branch code was not reported: %v", got)
	}
}

// --- checkEntry

func testEntry(t *testing.T, k ListKind) Entry {
	t.Helper()
	tin, err := ParseTIN("123456789")
	if err != nil {
		t.Fatal(err)
	}
	e := Entry{Line: 4, Party: Party{
		TIN: tin, RegisteredName: "XYZ CORPORATION",
		Address: [AddressLines]string{"456 EDSA", "QUEZON CITY"},
	}}
	if k == Purchases {
		e.Purchases = PurchaseAmounts{Services: 10000000, InputVAT: 1200000}
	} else {
		e.Sales = SalesAmounts{Taxable: 10000000, OutputVAT: 1200000}
	}
	return e
}

func TestCheckEntry(t *testing.T) {
	filer := goodFiler(t)
	opts := DefaultOptions()

	for _, k := range []ListKind{Sales, Purchases} {
		if f := checkEntry(testEntry(t, k), k, filer, opts); anyFatal(f) {
			t.Fatalf("%s: a good entry was refused: %v", k, f)
		}
	}

	cases := []struct {
		name     string
		kind     ListKind
		mutate   func(*Entry)
		mentions string
	}{
		{"the filer's own TIN", Sales, func(e *Entry) { e.Party.TIN = filer.TIN }, "own TIN"},
		{"no name", Sales, func(e *Entry) { e.Party.RegisteredName = "" }, "names no customer"},
		{"both kinds of name", Sales, func(e *Entry) { e.Party.Person = PersonName{Last: "SANTOS", First: "MARIA"} }, "one taxpayer, not two"},
		{"every amount nil", Sales, func(e *Entry) { e.Sales = SalesAmounts{} }, "nothing to report"},
		{"a negative amount", Sales, func(e *Entry) { e.Sales.Taxable = -100 }, "no column for a negative"},
		{"a negative tax", Sales, func(e *Entry) { e.Sales.OutputVAT = -100 }, "no column for a negative"},
		{"no supplier TIN", Purchases, func(e *Entry) { e.Party.TIN = TIN{} }, "must give the supplier's TIN"},
		{"no supplier address", Purchases, func(e *Entry) { e.Party.Address = [AddressLines]string{} }, "must give the supplier's address"},
		{"every purchase nil", Purchases, func(e *Entry) { e.Purchases = PurchaseAmounts{} }, "nothing to report"},
		{"a negative purchase", Purchases, func(e *Entry) { e.Purchases.Services = -1 }, "no column for a negative"},
		{"a malformed TIN", Sales, func(e *Entry) { e.Party.TIN = TIN{Base: "12345", Branch: "000"} }, "exactly 9 digits"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := testEntry(t, c.kind)
			c.mutate(&e)
			got := checkEntry(e, c.kind, filer, opts)
			if !anyFatal(got) {
				t.Fatalf("%s was accepted", c.name)
			}
			joined := ""
			for _, x := range got {
				joined += x.String() + "\n"
				if x.Severity == Fatal && x.Line != 4 {
					t.Errorf("the finding is reported on line %d, want 4", x.Line)
				}
			}
			if !strings.Contains(joined, c.mentions) {
				t.Errorf("the findings %q do not mention %q", joined, c.mentions)
			}
		})
	}
}

func TestASaleWithNoTINIsAllowedButRemarkedOn(t *testing.T) {
	// The validation module allows a sales record with no customer TIN, for
	// an unregistered buyer. It must not be refused — but it is worth saying.
	filer := goodFiler(t)
	e := testEntry(t, Sales)
	e.Party.TIN = TIN{}
	got := checkEntry(e, Sales, filer, DefaultOptions())
	if anyFatal(got) {
		t.Fatalf("a sale with no customer TIN was refused: %v", got)
	}
	found := false
	for _, x := range got {
		if x.Severity == Warning && strings.Contains(x.Detail, "end consumer") {
			found = true
		}
	}
	if !found {
		t.Errorf("a sale with no TIN was not remarked on: %v", got)
	}
}

func TestNegativeAmountsCanBeAllowedOnPurpose(t *testing.T) {
	filer := goodFiler(t)
	e := testEntry(t, Sales)
	e.Sales.Taxable = -10000000
	e.Sales.OutputVAT = -1200000
	opts := DefaultOptions()
	opts.AllowNegativeAmounts = true
	if got := checkEntry(e, Sales, filer, opts); anyFatal(got) {
		t.Errorf("negative amounts were refused although they were allowed: %v", got)
	}
}

func TestCheckVAT(t *testing.T) {
	filer := goodFiler(t)
	opts := DefaultOptions()

	// Twelve per cent exactly: nothing to say.
	e := testEntry(t, Sales)
	e.Sales = SalesAmounts{Taxable: 10000000, OutputVAT: 1200000}
	if got := checkEntry(e, Sales, filer, opts); len(got) != 0 {
		t.Errorf("a correct 12%% record was remarked on: %v", got)
	}

	// A peso out is inside the tolerance.
	e.Sales.OutputVAT = 1200000 + VATCheckTolerance
	if got := checkEntry(e, Sales, filer, opts); len(got) != 0 {
		t.Errorf("a record a peso out was remarked on: %v", got)
	}

	// More than that earns a warning, not a refusal.
	e.Sales.OutputVAT = 1200000 + VATCheckTolerance + 1
	got := checkEntry(e, Sales, filer, opts)
	if anyFatal(got) {
		t.Errorf("a tax figure off the rate was refused; it should only be remarked on: %v", got)
	}
	if len(got) == 0 {
		t.Error("a tax figure off the rate was not remarked on")
	}

	// Tax with no taxable amount to carry it.
	e.Sales = SalesAmounts{Exempt: 100000, OutputVAT: 1200}
	got = checkEntry(e, Sales, filer, opts)
	if anyFatal(got) {
		t.Errorf("tax without a taxable amount was refused: %v", got)
	}
	if len(got) == 0 {
		t.Error("tax without a taxable amount was not remarked on")
	}

	// A zero-rated-only record has no tax and needs no warning.
	e.Sales = SalesAmounts{ZeroRated: 7500000}
	if got := checkEntry(e, Sales, filer, opts); len(got) != 0 {
		t.Errorf("a zero-rated record was remarked on: %v", got)
	}

	// The rate can be overridden for a period when another applied.
	e.Sales = SalesAmounts{Taxable: 10000000, OutputVAT: 1000000}
	opts.VATRate = 10
	if got := checkEntry(e, Sales, filer, opts); len(got) != 0 {
		t.Errorf("a 10%% record was remarked on when the rate was set to 10: %v", got)
	}
}

func TestCheckGrossOnlyWarns(t *testing.T) {
	filer := goodFiler(t)
	e := testEntry(t, Sales)
	e.GrossGiven = true
	e.Gross = e.Sales.Total()
	if got := checkEntry(e, Sales, filer, DefaultOptions()); len(got) != 0 {
		t.Errorf("a gross that adds up was remarked on: %v", got)
	}
	e.Gross = e.Sales.Total() + 100000
	got := checkEntry(e, Sales, filer, DefaultOptions())
	if anyFatal(got) {
		t.Errorf("a gross that does not add up was refused; the file has no gross field: %v", got)
	}
	if len(got) == 0 {
		t.Error("a gross that does not add up was not remarked on")
	}
}

// --- checkTotals

func TestCheckTotals(t *testing.T) {
	entries := []Entry{
		{Purchases: PurchaseAmounts{Services: 10000, InputVAT: 1200}},
		{Purchases: PurchaseAmounts{OtherGoods: 5000, InputVAT: 600}},
	}
	good := addUp(entries, Purchases, 0)
	if got := checkTotals(entries, good, Purchases); anyFatal(got) {
		t.Fatalf("correct totals were refused: %v", got)
	}

	// Totals that do not match the records.
	wrong := good
	wrong.Purchases.Services += 1
	if got := checkTotals(entries, wrong, Purchases); !anyFatal(got) {
		t.Error("totals that do not match the records were accepted")
	}

	// A count that does not match.
	badCount := good
	badCount.Records = 5
	if got := checkTotals(entries, badCount, Purchases); !anyFatal(got) {
		t.Error("a record count that does not match was accepted")
	}

	// The creditable split must add back up to the total input tax, which is
	// a rule the validation module enforces.
	split := addUp(entries, Purchases, 600)
	if got := checkTotals(entries, split, Purchases); anyFatal(got) {
		t.Errorf("a correct creditable split was refused: %v", got)
	}
	broken := split
	broken.CreditableInputVAT += 1
	got := checkTotals(entries, broken, Purchases)
	if !anyFatal(got) {
		t.Error("a creditable split that does not add up was accepted")
	}
	joined := ""
	for _, f := range got {
		joined += f.Detail
	}
	if !strings.Contains(joined, "creditable and non-creditable") {
		t.Errorf("the finding %q does not explain the rule", joined)
	}

	// Non-creditable more than the total input tax.
	tooMuch := addUp(entries, Purchases, 999999)
	if got := checkTotals(entries, tooMuch, Purchases); !anyFatal(got) {
		t.Error("a non-creditable amount larger than the total input tax was accepted")
	}

	// A sales file has no creditable split, so none of that applies.
	sales := []Entry{{Sales: SalesAmounts{Taxable: 100, OutputVAT: 12}}}
	if got := checkTotals(sales, addUp(sales, Sales, 0), Sales); anyFatal(got) {
		t.Errorf("correct sales totals were refused: %v", got)
	}
}

func TestFindingStringNamesWhatIsWrong(t *testing.T) {
	f := Finding{Severity: Fatal, Line: 12, Record: "sales detail", Field: "TIN", Value: "abc", Detail: "not a number"}
	s := f.String()
	for _, want := range []string{"error", "line 12", "sales detail", "TIN", "abc", "not a number"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q does not mention %q", s, want)
		}
	}
	// A finding about the file rather than a row does not claim a line.
	g := Finding{Severity: Warning, Detail: "something"}
	if strings.Contains(g.String(), "line") {
		t.Errorf("%q mentions a line it does not have", g.String())
	}
}
