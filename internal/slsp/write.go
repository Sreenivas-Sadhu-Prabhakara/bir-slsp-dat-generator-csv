package slsp

import (
	"fmt"
	"strconv"
	"strings"
)

// The writer turns records into bytes. It never decides what a record
// contains: it walks the field table in layout.go and asks for each field in
// turn, and it refuses to carry on if the value it is given does not belong to
// the field the table expects next.
//
// That check is what keeps the layout in one place. If a field is inserted,
// renamed or moved in layout.go and the code here is not changed to match, the
// builder notices immediately and the generator reports a fault in the tool
// rather than quietly writing a record with its columns shifted by one.

// recordBuilder collects the values of one record in field order, checking
// each against the name the layout expects in that position.
type recordBuilder struct {
	spec     RecordSpec
	values   []string
	mismatch []string
}

func newRecord(spec RecordSpec) *recordBuilder {
	return &recordBuilder{spec: spec, values: make([]string, 0, len(spec.Fields))}
}

// put adds the next field's value. The name is the name the layout gives the
// field in this position; a disagreement is recorded and reported.
func (b *recordBuilder) put(name, value string) {
	i := len(b.values)
	switch {
	case i >= len(b.spec.Fields):
		b.mismatch = append(b.mismatch, fmt.Sprintf("field %d (%q) is past the end of the %s layout, which has %d fields",
			i+1, name, b.spec.What, len(b.spec.Fields)))
	case b.spec.Fields[i].Name != name:
		b.mismatch = append(b.mismatch, fmt.Sprintf("field %d of the %s layout is %q but the writer supplied %q",
			i+1, b.spec.What, b.spec.Fields[i].Name, name))
	}
	b.values = append(b.values, value)
}

// putAmount adds an amount field, rendering centavos in the form the field's
// own kind calls for. The two kinds differ: a detail record writes a nil
// amount as a single zero digit, while a header total always carries its two
// decimal places. That asymmetry is in the Bureau's module, not an oversight
// here, so a file this tool writes looks like one the module wrote.
func (b *recordBuilder) putAmount(name string, centavos int64) {
	i := len(b.values)
	kind := DetailAmount
	if i < len(b.spec.Fields) {
		kind = b.spec.Fields[i].Kind
	}
	if kind == DetailAmount && centavos == 0 {
		b.put(name, "0")
		return
	}
	b.put(name, FormatAmount(centavos))
}

// done returns the finished values, or the reasons the record does not match
// its layout.
func (b *recordBuilder) done() ([]string, []string) {
	if len(b.values) != len(b.spec.Fields) && len(b.mismatch) == 0 {
		b.mismatch = append(b.mismatch, fmt.Sprintf("the %s layout has %d fields but the writer supplied %d",
			b.spec.What, len(b.spec.Fields), len(b.values)))
	}
	return b.values, b.mismatch
}

// render joins one record's values into the line that goes in the file: fields
// separated by FieldSeparator, text fields between TextDelimiter, numbers and
// dates bare.
//
// A text field of a detail record is written bare when it is empty — two
// separators side by side rather than a pair of delimiters with nothing
// between them — while a header text field always carries its delimiters.
// Again, that is what the Bureau's module does.
//
// Nothing is escaped. A value that would need escaping has already been
// refused by the validator, because the record layout has no escape mechanism
// at all: the module strips the delimiter out of a value and the validation
// module refuses a value containing the separator.
func render(spec RecordSpec, values []string) string {
	var b strings.Builder
	for i, v := range values {
		if i > 0 {
			b.WriteRune(FieldSeparator)
		}
		kind := TokenValue
		if i < len(spec.Fields) {
			kind = spec.Fields[i].Kind
		}
		if kind.Quoted() && (v != "" || kind.QuotedWhenEmpty()) {
			b.WriteRune(TextDelimiter)
			b.WriteString(v)
			b.WriteRune(TextDelimiter)
			continue
		}
		b.WriteString(v)
	}
	return b.String()
}

// ------------------------------------------------------------ the records

// buildHeader builds the single header record: who is filing, and the totals
// of every amount column in the file.
func buildHeader(f FileSpec, filer Filer, p Period, t Totals) ([]string, []string) {
	b := newRecord(f.Header)
	b.put("record type", RecordTypeHeader)
	b.put("transaction code", f.Kind.TransactionCode())
	b.put("TIN of owner", filer.TIN.Digits())
	reg, _ := Truncate(filer.RegisteredName, MaxRegisteredNameChars)
	last, _ := Truncate(filer.Person.Last, MaxSurnameChars)
	first, _ := Truncate(filer.Person.First, MaxFirstNameChars)
	middle, _ := Truncate(filer.Person.Middle, MaxMiddleNameChars)
	trade, _ := Truncate(filer.TradeName, MaxTradeNameChars)
	addr1, _ := Truncate(filer.Address[0], MaxFilerAddress1Chars)
	addr2, _ := Truncate(filer.Address[1], MaxFilerAddress2Chars)
	b.put("owner registered name", reg)
	b.put("owner last name", last)
	b.put("owner first name", first)
	b.put("owner middle name", middle)
	b.put("owner trade name", trade)
	b.put("owner address line 1", addr1)
	b.put("owner address line 2", addr2)

	if f.Kind == Purchases {
		a := t.Purchases
		b.putAmount("total exempt purchases", a.Exempt)
		b.putAmount("total zero-rated purchases", a.ZeroRated)
		b.putAmount("total taxable purchases of services", a.Services)
		b.putAmount("total taxable purchases of capital goods", a.CapitalGoods)
		b.putAmount("total taxable purchases of other goods", a.OtherGoods)
		b.putAmount("total input tax", a.InputVAT)
		b.putAmount("creditable input tax", t.CreditableInputVAT)
		b.putAmount("non-creditable input tax", t.NonCreditableInputVAT)
	} else {
		a := t.Sales
		b.putAmount("total exempt sales", a.Exempt)
		b.putAmount("total zero-rated sales", a.ZeroRated)
		b.putAmount("total taxable sales", a.Taxable)
		b.putAmount("total output tax", a.OutputVAT)
	}

	b.put("RDO code", filer.RDOCode)
	b.put("tax month", p.LastDay().Format(DateLayout))
	b.put("fiscal or calendar month end", monthEndValue(filer.FiscalYearEndMonth))
	return b.done()
}

// monthEndValue renders the month a filer's accounting year ends in. Zero means
// it was not given, and the field is left empty, which the layout allows.
func monthEndValue(m int) string {
	if m < 1 || m > 12 {
		return ""
	}
	return strconv.Itoa(m)
}

// buildDetail builds one detail record: who the other party is and what was
// sold to or bought from them.
func buildDetail(f FileSpec, e Entry, filer Filer, p Period) ([]string, []string) {
	b := newRecord(f.Detail)
	b.put("record type", RecordTypeDetail)
	b.put("transaction code", f.Kind.TransactionCode())
	b.put("TIN", e.Party.TIN.Digits())
	reg, _ := Truncate(e.Party.RegisteredName, MaxRegisteredNameChars)
	last, _ := Truncate(e.Party.Person.Last, MaxSurnameChars)
	first, _ := Truncate(e.Party.Person.First, MaxFirstNameChars)
	middle, _ := Truncate(e.Party.Person.Middle, MaxMiddleNameChars)
	b.put("registered name", reg)
	b.put("last name", last)
	b.put("first name", first)
	b.put("middle name", middle)
	for i := 0; i < AddressLines; i++ {
		line, _ := Truncate(e.Party.Address[i], MaxPartyAddressChars)
		b.put(fmt.Sprintf("address line %d", i+1), line)
	}
	if f.Kind == Purchases {
		a := e.Purchases
		b.putAmount("exempt purchases", a.Exempt)
		b.putAmount("zero-rated purchases", a.ZeroRated)
		b.putAmount("taxable purchases of services", a.Services)
		b.putAmount("taxable purchases of capital goods", a.CapitalGoods)
		b.putAmount("taxable purchases of other goods", a.OtherGoods)
		b.putAmount("input tax", a.InputVAT)
	} else {
		a := e.Sales
		b.putAmount("exempt sales", a.Exempt)
		b.putAmount("zero-rated sales", a.ZeroRated)
		b.putAmount("taxable sales", a.Taxable)
		b.putAmount("output tax", a.OutputVAT)
	}
	b.put("TIN of owner", filer.TIN.Digits())
	b.put("tax month", p.LastDay().Format(DateLayout))
	return b.done()
}

// ------------------------------------------------------------------ totals

// Totals are the figures the header record carries.
type Totals struct {
	// Records is how many detail records the file holds. The layout has no
	// field for it, but the interface shows it and the submission key depends
	// on it.
	Records   int             `json:"records"`
	Sales     SalesAmounts    `json:"sales"`
	Purchases PurchaseAmounts `json:"purchases"`
	// CreditableInputVAT and NonCreditableInputVAT split the total input tax
	// on a purchases list. They are given for the month as a whole, and must
	// add up to the total input tax.
	CreditableInputVAT    int64 `json:"creditableInputVat"`
	NonCreditableInputVAT int64 `json:"nonCreditableInputVat"`
}

// addUp totals the entries. Every figure is a whole number of centavos, so the
// total of the file is exactly the total of its records with no drift.
//
// The non-creditable input tax is whatever the filer says it is; the creditable
// amount is the rest, which is how the Bureau's module works it out.
func addUp(entries []Entry, k ListKind, nonCreditable int64) Totals {
	t := Totals{Records: len(entries)}
	for _, e := range entries {
		if k == Purchases {
			t.Purchases.Exempt += e.Purchases.Exempt
			t.Purchases.ZeroRated += e.Purchases.ZeroRated
			t.Purchases.Services += e.Purchases.Services
			t.Purchases.CapitalGoods += e.Purchases.CapitalGoods
			t.Purchases.OtherGoods += e.Purchases.OtherGoods
			t.Purchases.InputVAT += e.Purchases.InputVAT
			continue
		}
		t.Sales.Exempt += e.Sales.Exempt
		t.Sales.ZeroRated += e.Sales.ZeroRated
		t.Sales.Taxable += e.Sales.Taxable
		t.Sales.OutputVAT += e.Sales.OutputVAT
	}
	if k == Purchases {
		t.NonCreditableInputVAT = nonCreditable
		t.CreditableInputVAT = t.Purchases.InputVAT - nonCreditable
	}
	return t
}
