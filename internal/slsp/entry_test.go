package slsp

import (
	"strings"
	"testing"
)

func mustTIN(t *testing.T, s string) TIN {
	t.Helper()
	tin, err := ParseTIN(s)
	if err != nil {
		t.Fatalf("ParseTIN(%q): %v", s, err)
	}
	return tin
}

func TestConsolidateAddsUpTheSameTaxpayer(t *testing.T) {
	a := mustTIN(t, "123456789")
	b := mustTIN(t, "234567890")
	entries := []Entry{
		{Line: 2, Party: Party{TIN: a, RegisteredName: "ACME"}, Sales: SalesAmounts{Taxable: 1000, OutputVAT: 120}},
		{Line: 3, Party: Party{TIN: b, RegisteredName: "BAYAN"}, Sales: SalesAmounts{Taxable: 500, OutputVAT: 60}},
		{Line: 4, Party: Party{TIN: a, RegisteredName: "ACME"}, Sales: SalesAmounts{Taxable: 250, OutputVAT: 30}},
		{Line: 5, Party: Party{TIN: a, RegisteredName: "ACME"}, Sales: SalesAmounts{Exempt: 10}},
	}
	out, merges, splits := Consolidate(entries, Sales)
	if len(out) != 2 {
		t.Fatalf("Consolidate produced %d records, want 2", len(out))
	}
	if len(splits) != 0 {
		t.Errorf("Consolidate reported a name conflict where there is none: %v", splits)
	}
	if len(merges) != 1 {
		t.Fatalf("Consolidate reported %d merges, want 1", len(merges))
	}
	if got := merges[0].Lines; len(got) != 3 || got[0] != 2 || got[1] != 4 || got[2] != 5 {
		t.Errorf("the merge names lines %v, want 2, 4 and 5", got)
	}
	for _, e := range out {
		if e.Party.TIN.Base != a.Base {
			continue
		}
		want := SalesAmounts{Exempt: 10, Taxable: 1250, OutputVAT: 150}
		if e.Sales != want {
			t.Errorf("the consolidated amounts are %+v, want %+v", e.Sales, want)
		}
		if len(e.Lines) != 3 {
			t.Errorf("the consolidated record names %d lines, want 3", len(e.Lines))
		}
		if e.Line != 2 {
			t.Errorf("the consolidated record is reported on line %d, want the first, 2", e.Line)
		}
	}
}

func TestConsolidateKeepsDifferentNamesApart(t *testing.T) {
	// The Bureau's module consolidates on the name as well as the TIN, so two
	// spellings stay two records. That is a trap, so it is reported.
	a := mustTIN(t, "123456789")
	entries := []Entry{
		{Line: 2, Party: Party{TIN: a, RegisteredName: "ACME TRADING CORPORATION"}, Sales: SalesAmounts{Taxable: 100}},
		{Line: 3, Party: Party{TIN: a, RegisteredName: "ACME TRADING CORP"}, Sales: SalesAmounts{Taxable: 200}},
	}
	out, merges, splits := Consolidate(entries, Sales)
	if len(out) != 2 {
		t.Fatalf("Consolidate produced %d records, want 2", len(out))
	}
	if len(merges) != 0 {
		t.Errorf("Consolidate merged two different names: %v", merges)
	}
	if len(splits) != 1 {
		t.Fatalf("Consolidate reported %d name conflicts, want 1: %v", len(splits), splits)
	}
	if !strings.Contains(splits[0].Name, "ACME TRADING CORP") || !strings.Contains(splits[0].Name, "ACME TRADING CORPORATION") {
		t.Errorf("the conflict names %q, want both spellings", splits[0].Name)
	}
}

func TestConsolidateKeepsIndividualsApartFromCompanies(t *testing.T) {
	a := mustTIN(t, "123456789")
	entries := []Entry{
		{Line: 2, Party: Party{TIN: a, RegisteredName: "ACME"}, Sales: SalesAmounts{Taxable: 100}},
		{Line: 3, Party: Party{TIN: a, Person: PersonName{Last: "SANTOS", First: "MARIA"}}, Sales: SalesAmounts{Taxable: 200}},
	}
	out, _, _ := Consolidate(entries, Sales)
	if len(out) != 2 {
		t.Errorf("a company and an individual sharing a TIN were merged into %d record(s)", len(out))
	}
}

func TestConsolidateOrdersAlphabetically(t *testing.T) {
	// The regulation requires the list to be alphabetically arranged, and the
	// module sorts by registered name, then the three personal names, then
	// the TIN. An individual, whose registered name is empty, sorts first.
	entries := []Entry{
		{Line: 2, Party: Party{TIN: mustTIN(t, "111111112"), RegisteredName: "ZENITH"}, Sales: SalesAmounts{Taxable: 1}},
		{Line: 3, Party: Party{TIN: mustTIN(t, "222222223"), RegisteredName: "ALPHA"}, Sales: SalesAmounts{Taxable: 1}},
		{Line: 4, Party: Party{TIN: mustTIN(t, "333333334"), Person: PersonName{Last: "SANTOS", First: "MARIA"}}, Sales: SalesAmounts{Taxable: 1}},
		{Line: 5, Party: Party{TIN: mustTIN(t, "444444445"), RegisteredName: "MANGO"}, Sales: SalesAmounts{Taxable: 1}},
		{Line: 6, Party: Party{TIN: mustTIN(t, "555555556"), Person: PersonName{Last: "ABAD", First: "JOSE"}}, Sales: SalesAmounts{Taxable: 1}},
	}
	out, _, _ := Consolidate(entries, Sales)
	want := []string{"ABAD, JOSE", "SANTOS, MARIA", "ALPHA", "MANGO", "ZENITH"}
	for i, w := range want {
		if got := out[i].Party.Name(); got != w {
			t.Errorf("record %d is %q, want %q", i+1, got, w)
		}
	}
}

func TestConsolidateSameNameDifferentTINsStayApart(t *testing.T) {
	entries := []Entry{
		{Line: 2, Party: Party{TIN: mustTIN(t, "123456789"), RegisteredName: "ACME"}, Sales: SalesAmounts{Taxable: 100}},
		{Line: 3, Party: Party{TIN: mustTIN(t, "234567890"), RegisteredName: "ACME"}, Sales: SalesAmounts{Taxable: 200}},
	}
	out, merges, splits := Consolidate(entries, Sales)
	if len(out) != 2 {
		t.Errorf("two taxpayers with the same name were merged into %d record(s)", len(out))
	}
	if len(merges) != 0 || len(splits) != 0 {
		t.Errorf("merges %v, splits %v; want neither", merges, splits)
	}
	// Sorted by name then TIN, so the lower TIN comes first.
	if out[0].Party.TIN.Base != "123456789" {
		t.Errorf("the records are ordered %q then %q", out[0].Party.TIN.Base, out[1].Party.TIN.Base)
	}
}

func TestConsolidatePurchases(t *testing.T) {
	a := mustTIN(t, "123456789")
	entries := []Entry{
		{Line: 2, Party: Party{TIN: a, RegisteredName: "ACME"}, Purchases: PurchaseAmounts{
			Exempt: 1, ZeroRated: 2, Services: 3, CapitalGoods: 4, OtherGoods: 5, InputVAT: 6}},
		{Line: 3, Party: Party{TIN: a, RegisteredName: "ACME"}, Purchases: PurchaseAmounts{
			Exempt: 10, ZeroRated: 20, Services: 30, CapitalGoods: 40, OtherGoods: 50, InputVAT: 60}},
	}
	out, _, _ := Consolidate(entries, Purchases)
	if len(out) != 1 {
		t.Fatalf("Consolidate produced %d records, want 1", len(out))
	}
	want := PurchaseAmounts{Exempt: 11, ZeroRated: 22, Services: 33, CapitalGoods: 44, OtherGoods: 55, InputVAT: 66}
	if out[0].Purchases != want {
		t.Errorf("the consolidated amounts are %+v, want %+v", out[0].Purchases, want)
	}
	if got := out[0].Purchases.TaxableTotal(); got != 132 {
		t.Errorf("TaxableTotal = %d, want 132", got)
	}
	if got := out[0].Purchases.Total(); got != 231 {
		t.Errorf("Total = %d, want 231", got)
	}
}

func TestConsolidateWithNothing(t *testing.T) {
	out, merges, splits := Consolidate(nil, Sales)
	if len(out) != 0 || len(merges) != 0 || len(splits) != 0 {
		t.Errorf("Consolidate(nil) = %v, %v, %v", out, merges, splits)
	}
}

func TestAmountSetHelpers(t *testing.T) {
	var s SalesAmounts
	if !s.AllZero() || s.Total() != 0 {
		t.Error("the zero sales amounts do not report themselves nil")
	}
	s.OutputVAT = 1
	if s.AllZero() {
		t.Error("sales amounts with a tax figure reported themselves nil")
	}
	if got := (SalesAmounts{Exempt: 1, ZeroRated: 2, Taxable: 3, OutputVAT: 4}).Total(); got != 10 {
		t.Errorf("SalesAmounts.Total() = %d, want 10", got)
	}

	var p PurchaseAmounts
	if !p.AllZero() || p.Total() != 0 || p.TaxableTotal() != 0 {
		t.Error("the zero purchase amounts do not report themselves nil")
	}
	p.CapitalGoods = 1
	if p.AllZero() {
		t.Error("purchase amounts with a capital-goods figure reported themselves nil")
	}
}

func TestPartyName(t *testing.T) {
	a := mustTIN(t, "123456789")
	if got := (Party{TIN: a, RegisteredName: "ACME"}).Name(); got != "ACME" {
		t.Errorf("Name() = %q", got)
	}
	if got := (Party{TIN: a, Person: PersonName{Last: "SANTOS", First: "MARIA"}}).Name(); got != "SANTOS, MARIA" {
		t.Errorf("Name() = %q", got)
	}
	if got := (Party{TIN: a}).Name(); got != "(unnamed)" {
		t.Errorf("an unnamed party rendered as %q", got)
	}
	var z Party
	if got := z.Name(); got != "(unnamed)" {
		t.Errorf("the zero party rendered as %q", got)
	}
}

func TestCSVColumnsCoverBothLists(t *testing.T) {
	for _, k := range []ListKind{Sales, Purchases} {
		specs := CSVColumns(k)
		names := map[string]bool{}
		required := 0
		for _, s := range specs {
			if names[s.Name] {
				t.Errorf("%s: the column %q is declared twice", k, s.Name)
			}
			names[s.Name] = true
			if s.Required {
				required++
			}
		}
		if !names[ColTIN] {
			t.Errorf("%s: there is no TIN column", k)
		}
		if required != 1 {
			t.Errorf("%s: %d columns are required, want only the TIN", k, required)
		}
		// The amount columns differ between the two lists, and each list must
		// have the ones its records carry.
		if k == Purchases {
			for _, c := range []string{ColServices, ColCapitalGoods, ColOtherGoods, ColInputVAT} {
				if !names[c] {
					t.Errorf("the purchases columns have no %q", c)
				}
			}
			if names[ColTaxable] || names[ColOutputVAT] {
				t.Error("the purchases columns include a sales-only column")
			}
			continue
		}
		for _, c := range []string{ColTaxable, ColOutputVAT} {
			if !names[c] {
				t.Errorf("the sales columns have no %q", c)
			}
		}
		if names[ColServices] || names[ColInputVAT] {
			t.Error("the sales columns include a purchases-only column")
		}
	}
}

func TestProblemString(t *testing.T) {
	p := Problem{Line: 9, Field: "TIN", Value: "x", Detail: "no good"}
	s := p.String()
	for _, want := range []string{"line 9", "TIN", "no good"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q does not mention %q", s, want)
		}
	}
	q := Problem{Line: 3, Detail: "something"}
	if strings.Contains(q.String(), "column") {
		t.Errorf("%q mentions a column it does not have", q.String())
	}
}

func TestMergeNoteString(t *testing.T) {
	m := MergeNote{TIN: "123-456-789-000", Name: "ACME", Lines: []int{2, 4, 7}}
	s := m.String()
	for _, want := range []string{"ACME", "123-456-789-000", "2, 4, 7"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q does not mention %q", s, want)
		}
	}
}
