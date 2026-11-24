package slsp

import (
	"errors"
	"strings"
	"testing"
)

func testOptions(t *testing.T, k ListKind) Options {
	t.Helper()
	tin, err := ParseTIN("004-123-456-000")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParsePeriod("10/2026")
	if err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.List = k
	o.Period = p
	o.Filer = Filer{
		TIN: tin, RegisteredName: "ABC Trading Corporation",
		Address: [AddressLines]string{"123 Main St", "Makati City"}, RDOCode: "044", FiscalYearEndMonth: 12,
	}
	return o
}

func generate(t *testing.T, k ListKind, csv string) *Result {
	t.Helper()
	res, err := Generate(strings.NewReader(csv), testOptions(t, k))
	if err != nil {
		t.Fatalf("Generate returned error %v", err)
	}
	return res
}

const minimalSales = "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
	"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n"

func TestGenerateMinimal(t *testing.T) {
	res := generate(t, Sales, minimalSales)
	if !res.OK {
		for _, f := range res.Findings {
			t.Logf("finding: %s", f)
		}
		t.Fatalf("a simple CSV was refused: %d error(s)", res.FatalCount())
	}
	if res.FileName != "004123456S102026.DAT" {
		t.Errorf("FileName = %q", res.FileName)
	}
	if res.Totals.Records != 1 {
		t.Errorf("Records = %d, want 1", res.Totals.Records)
	}
	if res.Totals.Sales.Taxable != 10000000 || res.Totals.Sales.OutputVAT != 1200000 {
		t.Errorf("totals = %+v", res.Totals.Sales)
	}
	lines := strings.Split(strings.TrimSuffix(res.Dat, RecordSeparator), RecordSeparator)
	if len(lines) != 2 {
		t.Fatalf("the file has %d records, want a header and one detail", len(lines))
	}
	if !strings.HasPrefix(lines[0], "H,S,") || !strings.HasPrefix(lines[1], "D,S,") {
		t.Errorf("the records begin %q and %q", lines[0][:4], lines[1][:4])
	}
	if !strings.HasSuffix(res.Dat, RecordSeparator) {
		t.Error("the file does not end with the record separator")
	}
}

func TestGenerateRefusesWithoutTheThingsItMustHave(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Options)
		mentions string
	}{
		{"no period", func(o *Options) { o.Period = Period{} }, "month the list covers"},
		{"no filer TIN", func(o *Options) { o.Filer.TIN = TIN{} }, "TIN of the taxpayer"},
		{"no filer name", func(o *Options) { o.Filer.RegisteredName = ""; o.Filer.TradeName = "" }, "name of the taxpayer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := testOptions(t, Sales)
			c.mutate(&o)
			_, err := Generate(strings.NewReader(minimalSales), o)
			if err == nil {
				t.Fatalf("%s was accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.mentions) {
				t.Errorf("the error %q does not mention %q", err, c.mentions)
			}
		})
	}
}

func TestGenerateRefusesAnUnreadableCSV(t *testing.T) {
	for _, csv := range []string{"", "   ", "a,b,c\n1,2,3\n", "TIN,Registered Name\r\n"} {
		_, err := Generate(strings.NewReader(csv), testOptions(t, Sales))
		if err == nil {
			t.Errorf("Generate accepted %q", csv)
			continue
		}
		if !errors.Is(err, ErrCSV) {
			t.Errorf("the error for %q does not wrap ErrCSV: %v", csv, err)
		}
	}
}

func TestGenerateIsDefensiveAboutTheCSV(t *testing.T) {
	// Each of these is the same one-row list written the way a different
	// spreadsheet would export it. All of them must produce the same file.
	want := generate(t, Sales, minimalSales).Dat
	variants := map[string]string{
		"a byte-order mark":        "\ufeffTIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n",
		"unix line endings":        "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\n",
		"classic mac line endings": "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r",
		"no trailing newline":      "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00",
		"semicolon separated":      "TIN;Registered Name;Address 1;Taxable Sales;Output Tax\r\n123-456-789-000;Acme Trading Corporation;12 Ayala Avenue;100000.00;12000.00\r\n",
		"tab separated":            "TIN\tRegistered Name\tAddress 1\tTaxable Sales\tOutput Tax\r\n123-456-789-000\tAcme Trading Corporation\t12 Ayala Avenue\t100000.00\t12000.00\r\n",
		"thousands separators":     "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,\"100,000.00\",\"12,000.00\"\r\n",
		"a peso marker":            "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,₱100000.00,₱12000.00\r\n",
		"alternative headings":     "Customer TIN,Company Name,Business Address,Vatable Sales,Output VAT\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n",
		"title rows above":         "MABUHAY TRADING\r\nSummary List of Sales\r\n\r\nTIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n",
		"blank rows below":         minimalSales + ",,,,\r\n\r\n",
		"a short row":              "TIN,Registered Name,Address 1,Taxable Sales,Output Tax,Remarks\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n",
		"columns it does not know": "TIN,Registered Name,Address 1,Taxable Sales,Output Tax,Encoded By,Remarks\r\n123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00,jm,ok\r\n",
		"a nine-digit TIN":         "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123456789,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n",
		"lower-case name":          "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n123-456-789-000,acme trading corporation,12 ayala avenue,100000.00,12000.00\r\n",
		"extra whitespace":         "TIN , Registered Name , Address 1 , Taxable Sales , Output Tax \r\n  123-456-789-000 ,  Acme   Trading   Corporation  ,  12 Ayala Avenue  , 100000.00 , 12000.00 \r\n",
	}
	for name, csv := range variants {
		t.Run(name, func(t *testing.T) {
			res := generate(t, Sales, csv)
			if !res.OK {
				for _, f := range res.Findings {
					t.Logf("finding: %s", f)
				}
				t.Fatalf("%s was refused", name)
			}
			if res.Dat != want {
				t.Errorf("%s produced a different file.\nwant %q\ngot  %q", name, want, res.Dat)
			}
		})
	}
}

func TestGenerateNeverPanicsOnOddInput(t *testing.T) {
	odd := []string{
		"\x00\x01\x02",
		"\"",
		"TIN,Registered Name\r\n\"unclosed,Acme\r\n",
		strings.Repeat(",", 10000),
		"TIN,Registered Name,Taxable Sales\r\n" + strings.Repeat("x", 5000) + ",y,1\r\n",
		minimalSales + strings.Repeat("123-456-789-000,A,B,1,0\r\n", 500),
		"TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n中文,中文,中文,中文,中文\r\n",
		"TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n-,-,-,-,-\r\n",
	}
	for _, csv := range odd {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("Generate panicked on %q: %v", truncate(csv, 50), p)
				}
			}()
			for _, k := range []ListKind{Sales, Purchases} {
				_, _ = Generate(strings.NewReader(csv), testOptions(t, k))
			}
		}()
	}
}

func TestGenerateNamesTheOffendingRowAndField(t *testing.T) {
	csv := "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n" +
		"12345,Bad Tin Corporation,Somewhere,1000.00,120.00\r\n" +
		"234-567-890-000,Bad Amount Corporation,Somewhere,not a number,120.00\r\n"
	res := generate(t, Sales, csv)
	if res.OK {
		t.Fatal("a CSV with two bad rows produced a file")
	}
	if res.Dat != "" {
		t.Error("a refused run still produced a file")
	}
	byLine := map[int][]Finding{}
	for _, f := range res.Findings {
		if f.Severity == Fatal {
			byLine[f.Line] = append(byLine[f.Line], f)
		}
	}
	if len(byLine[3]) == 0 {
		t.Error("the bad TIN on line 3 was not reported against line 3")
	}
	if len(byLine[4]) == 0 {
		t.Error("the bad amount on line 4 was not reported against line 4")
	}
	if len(byLine[2]) != 0 {
		t.Errorf("the good row on line 2 was reported: %v", byLine[2])
	}
	// The field must be named, not just the row.
	found := false
	for _, f := range byLine[4] {
		if f.Field == ColTaxable {
			found = true
		}
	}
	if !found {
		t.Errorf("the finding for line 4 does not name the column: %v", byLine[4])
	}
}

func TestGenerateConsolidatesPerTaxpayer(t *testing.T) {
	csv := "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n" +
		"234-567-890-000,Bayan Supply Inc,Cebu City,50000.00,6000.00\r\n" +
		"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,25000.00,3000.00\r\n"
	res := generate(t, Sales, csv)
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	if res.RowsRead != 3 {
		t.Errorf("RowsRead = %d, want 3", res.RowsRead)
	}
	if res.Totals.Records != 2 {
		t.Errorf("the file holds %d records, want 2 — a summary list is one line per customer", res.Totals.Records)
	}
	if len(res.Merges) != 1 {
		t.Fatalf("Merges = %v, want one note", res.Merges)
	}
	if got := res.Merges[0].Lines; len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Errorf("the merge note names lines %v, want 2 and 4", got)
	}
	// The amounts are added together.
	for _, e := range res.Entries {
		if e.Party.TIN.Base == "123456789" {
			if e.Sales.Taxable != 12500000 || e.Sales.OutputVAT != 1500000 {
				t.Errorf("the consolidated amounts are %+v", e.Sales)
			}
		}
	}
	if res.Totals.Sales.Taxable != 17500000 {
		t.Errorf("total taxable = %d, want 17500000", res.Totals.Sales.Taxable)
	}

	// Turning consolidation off keeps one record per row.
	o := testOptions(t, Sales)
	o.NoConsolidate = true
	raw, err := Generate(strings.NewReader(csv), o)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Totals.Records != 3 {
		t.Errorf("with consolidation off the file holds %d records, want 3", raw.Totals.Records)
	}
}

func TestGenerateWarnsWhenOneTINHasTwoNames(t *testing.T) {
	csv := "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00\r\n" +
		"123-456-789-000,Acme Trading Corp,12 Ayala Avenue,25000.00,3000.00\r\n"
	res := generate(t, Sales, csv)
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	// The Bureau's module consolidates on the name as well as the TIN, so
	// these stay two records — and that is worth saying out loud.
	if res.Totals.Records != 2 {
		t.Errorf("the file holds %d records, want 2", res.Totals.Records)
	}
	if len(res.Splits) == 0 {
		t.Error("one TIN under two names was not reported")
	}
	found := false
	for _, f := range res.Findings {
		if f.Severity == Warning && strings.Contains(f.Detail, "more than one name") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning mentions the two names: %v", res.Findings)
	}
}

func TestGenerateRecordsAreInAlphabeticalOrder(t *testing.T) {
	// The Bureau's module writes records sorted by name, which the regulation
	// requires, so the order must not follow the CSV.
	csv := "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"111-111-112-000,Zenith Corporation,A,1000.00,120.00\r\n" +
		"222-222-223-000,Alpha Corporation,B,1000.00,120.00\r\n" +
		"333-333-334-000,Mango Corporation,C,1000.00,120.00\r\n"
	res := generate(t, Sales, csv)
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	var names []string
	for _, e := range res.Entries {
		names = append(names, e.Party.RegisteredName)
	}
	want := []string{"ALPHA CORPORATION", "MANGO CORPORATION", "ZENITH CORPORATION"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("record %d is %q, want %q (the file is written alphabetically)", i+1, names[i], want[i])
		}
	}
}

func TestGenerateIndividualAndCompanyUseDifferentNameFields(t *testing.T) {
	csv := "TIN,Type,Registered Name,Surname,First Name,Middle Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,N,Acme Trading Corporation,,,,12 Ayala Avenue,100000.00,12000.00\r\n" +
		"234-567-890-000,I,,Dela Cruz,Juan,Perez,18 Mabini Street,50000.00,6000.00\r\n"
	res := generate(t, Sales, csv)
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	lines := strings.Split(strings.TrimSuffix(res.Dat, RecordSeparator), RecordSeparator)
	var individual, company string
	for _, l := range lines[1:] {
		if strings.Contains(l, "DELA CRUZ") {
			individual = l
		}
		if strings.Contains(l, "ACME") {
			company = l
		}
	}
	// A company fills field 4 and leaves 5, 6 and 7 empty: ,"NAME",,,,
	if !strings.Contains(company, `"ACME TRADING CORPORATION",,,,`) {
		t.Errorf("the company record does not leave the personal name fields empty: %q", company)
	}
	// An individual leaves field 4 empty and fills 5, 6 and 7.
	if !strings.Contains(individual, `,,"DELA CRUZ","JUAN","PEREZ",`) {
		t.Errorf("the individual record does not use the three name fields: %q", individual)
	}
}

func TestGenerateRefusesAnIndividualNamedBothWays(t *testing.T) {
	csv := "TIN,Type,Registered Name,Surname,First Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,I,Acme Trading Corporation,Dela Cruz,Juan,12 Ayala Avenue,100000.00,12000.00\r\n"
	res := generate(t, Sales, csv)
	if res.OK {
		t.Fatal("a row naming a taxpayer both ways produced a file")
	}
	found := false
	for _, f := range res.Findings {
		if f.Severity == Fatal && strings.Contains(f.Detail, "registered-name column must be empty") {
			found = true
		}
	}
	if !found {
		t.Errorf("the refusal does not explain the rule: %v", res.Findings)
	}
}

func TestGenerateReportsWhatItHadToChange(t *testing.T) {
	csv := "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n" +
		"123-456-789-000,Smith & Sons Hardware,12 O'Connell Street,100000.00,12000.00\r\n"
	res := generate(t, Sales, csv)
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	if !strings.Contains(res.Dat, "SMITH AND SONS HARDWARE") {
		t.Errorf("the ampersand was not substituted: %q", res.Dat)
	}
	if !strings.Contains(res.Dat, "12 OCONNELL STREET") {
		t.Errorf("the apostrophe was not removed: %q", res.Dat)
	}
	if len(res.Changes) == 0 {
		t.Fatal("the changes were made but not reported")
	}
	joined := ""
	for _, c := range res.Changes {
		joined += c.String() + "\n"
		if c.Line != 2 {
			t.Errorf("a change is reported on line %d, want 2", c.Line)
		}
	}
	if !strings.Contains(joined, "&") || !strings.Contains(joined, "'") {
		t.Errorf("the report %q does not name both substitutions", joined)
	}
}

func TestGeneratePurchasesNeedsATINAndAnAddress(t *testing.T) {
	// A purchases list is stricter than a sales list: the validation module
	// refuses a record without a supplier TIN or an address.
	cases := []struct {
		name, csv, mentions string
	}{
		{
			"no TIN",
			"TIN,Registered Name,Address 1,Taxable Services,Input Tax\r\n,Acme Trading,12 Ayala Avenue,100000.00,12000.00\r\n",
			"supplier's TIN",
		},
		{
			"no address",
			"TIN,Registered Name,Address 1,Taxable Services,Input Tax\r\n123-456-789-000,Acme Trading,,100000.00,12000.00\r\n",
			"supplier's address",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := generate(t, Purchases, c.csv)
			if res.OK {
				t.Fatalf("a purchases row with %s produced a file", c.name)
			}
			joined := ""
			for _, f := range res.Findings {
				joined += f.Detail + "\n"
			}
			if !strings.Contains(joined, c.mentions) {
				t.Errorf("the refusal %q does not mention %q", joined, c.mentions)
			}
		})
	}
	// The same CSV is fine as a sales list, where a buyer may be unregistered.
	res := generate(t, Sales, "TIN,Registered Name,Address 1,Taxable Sales,Output Tax\r\n,Walk-in Customers,Public Market,100000.00,12000.00\r\n")
	if !res.OK {
		t.Errorf("a sale to an unregistered buyer was refused: %v", res.Findings)
	}
}

func TestGeneratePurchasesCreditableSplit(t *testing.T) {
	csv := "TIN,Registered Name,Address 1,Taxable Services,Input Tax\r\n" +
		"123-456-789-000,Acme Trading,12 Ayala Avenue,100000.00,12000.00\r\n"
	o := testOptions(t, Purchases)
	o.NonCreditableInputVAT = 200000 // PHP 2,000.00
	res, err := Generate(strings.NewReader(csv), o)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("refused: %v", res.Findings)
	}
	if res.Totals.CreditableInputVAT != 1000000 {
		t.Errorf("creditable = %d, want 1000000", res.Totals.CreditableInputVAT)
	}
	if !strings.Contains(res.Dat, ",12000.00,10000.00,2000.00,044,") {
		t.Errorf("the header does not carry the input tax and its split: %q", res.Dat)
	}

	// More non-creditable than there is input tax is refused rather than
	// written as a negative creditable amount.
	o.NonCreditableInputVAT = 9900000
	bad, err := Generate(strings.NewReader(csv), o)
	if err != nil {
		t.Fatal(err)
	}
	if bad.OK {
		t.Error("a non-creditable amount larger than the input tax produced a file")
	}
}

func TestGenerateHeaderTotalsAlwaysMatchTheDetails(t *testing.T) {
	// This is the one cross-check the validation module performs on the file
	// as a whole, so it must hold for every file the tool writes.
	csvs := []string{
		minimalSales,
		minimalSales + "234-567-890-000,Bayan Supply,Cebu,50000.00,6000.00\r\n",
		"TIN,Registered Name,Address 1,Exempt Sales,Zero-Rated Sales,Taxable Sales,Output Tax\r\n" +
			"123-456-789-000,A Corp,X,1.00,2.00,3.00,0.36\r\n" +
			"234-567-890-000,B Corp,Y,0.01,0.02,0.03,0.01\r\n",
	}
	for _, csv := range csvs {
		res := generate(t, Sales, csv)
		if !res.OK {
			t.Fatalf("refused: %v", res.Findings)
		}
		var sum SalesAmounts
		for _, e := range res.Entries {
			sum.Exempt += e.Sales.Exempt
			sum.ZeroRated += e.Sales.ZeroRated
			sum.Taxable += e.Sales.Taxable
			sum.OutputVAT += e.Sales.OutputVAT
		}
		if sum != res.Totals.Sales {
			t.Errorf("the header totals %+v do not match the records' %+v", res.Totals.Sales, sum)
		}
		header := strings.SplitN(res.Dat, RecordSeparator, 2)[0]
		fields := splitDatLine(header)
		want := []string{
			FormatAmount(sum.Exempt), FormatAmount(sum.ZeroRated),
			FormatAmount(sum.Taxable), FormatAmount(sum.OutputVAT),
		}
		for i, w := range want {
			if got := fields[10+i]; got != w {
				t.Errorf("header total %d is %q, want %q", i+1, got, w)
			}
		}
	}
}

func TestGenerateDateOrderSettingIsCarried(t *testing.T) {
	// The summary list has no per-row date, so the setting only matters for a
	// CSV that happens to carry one; what must hold is that it is accepted
	// and reported, not silently ignored.
	o := testOptions(t, Sales)
	o.DateOrder = DayFirst
	res, err := Generate(strings.NewReader(minimalSales), o)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Errorf("a day-first run was refused: %v", res.Findings)
	}
}

func TestGenerateReportsTheColumnsItMatched(t *testing.T) {
	csv := "Customer TIN,Company Name,Business Address,Vatable Sales,Output VAT,Remarks\r\n" +
		"123-456-789-000,Acme Trading Corporation,12 Ayala Avenue,100000.00,12000.00,ok\r\n"
	res := generate(t, Sales, csv)
	if got := res.Columns[ColTIN]; got != "Customer TIN" {
		t.Errorf("the TIN column matched %q", got)
	}
	if got := res.Columns[ColRegisteredName]; got != "Company Name" {
		t.Errorf("the registered-name column matched %q", got)
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0] != "Remarks" {
		t.Errorf("Unmatched = %v, want just Remarks", res.Unmatched)
	}
}

func TestGenerateAlwaysProducesBothCopies(t *testing.T) {
	for _, k := range []ListKind{Sales, Purchases} {
		csv := minimalSales
		if k == Purchases {
			csv = "TIN,Registered Name,Address 1,Taxable Services,Input Tax\r\n" +
				"123-456-789-000,Acme Trading,12 Ayala Avenue,100000.00,12000.00\r\n"
		}
		res := generate(t, k, csv)
		if !res.OK {
			t.Fatalf("%s refused: %v", k, res.Findings)
		}
		if res.Dat == "" || res.Submission == "" {
			t.Errorf("%s: one of the two copies is empty", k)
		}
		if res.ValidationKey == "" {
			t.Errorf("%s: no submission key", k)
		}
		if !strings.HasPrefix(res.SubmissionFileName, SubmissionFolder+"/") {
			t.Errorf("%s: the submission copy is named %q", k, res.SubmissionFileName)
		}
		if len(res.DatBytes()) == 0 || len(res.SubmissionBytes()) == 0 {
			t.Errorf("%s: one of the two copies encoded to nothing", k)
		}
		if len(res.SubmissionBytes()) <= len(res.DatBytes()) {
			t.Errorf("%s: the submission copy is not longer than the data file", k)
		}
	}
}

func TestZeroResultIsSafe(t *testing.T) {
	var r Result
	if r.FatalCount() != 0 || r.WarningCount() != 0 {
		t.Error("the zero Result counted findings")
	}
	if len(r.DatBytes()) != 0 || len(r.SubmissionBytes()) != 0 {
		t.Error("the zero Result produced bytes")
	}
}

func TestSampleCSVGeneratesCleanly(t *testing.T) {
	// The template the tool hands out must itself produce a file, or it is
	// not a template.
	for _, k := range []ListKind{Sales, Purchases} {
		o := testOptions(t, k)
		res, err := Generate(strings.NewReader(SampleCSV(k)), o)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if !res.OK {
			for _, f := range res.Findings {
				if f.Severity == Fatal {
					t.Errorf("%s template: %s", k, f)
				}
			}
			t.Fatalf("the %s template was refused", k)
		}
		if res.Totals.Records != 5 {
			t.Errorf("%s template produced %d records, want 5", k, res.Totals.Records)
		}
		if name := SampleCSVName(k); !strings.HasSuffix(name, ".csv") {
			t.Errorf("%s template is offered as %q", k, name)
		}
	}
}
