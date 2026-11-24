package slsp

import (
	"fmt"
	"strings"
)

// Nothing is written to a data file until it has been through this file. Every
// record is checked field by field against the layout — width, required,
// characters, number format — and then against the rules the Bureau's own
// validation module applies. A record that fails is named by the line of the
// CSV it came from and by the field at fault, and the file is not written at
// all.
//
// Refusing to write is the point. A data file the validation module rejects
// costs the taxpayer a resubmission and, if the deadline has passed, a
// penalty. It is better to hand back a list of twelve rows to fix than a file
// that looks finished and is not.

// VATRatePercent is the rate of value-added tax, as a whole percentage. It is
// used only to check that the tax column is consistent with the taxable
// column; it is never used to work the tax out, because the figures must come
// from the taxpayer's own books.
//
// Twelve per cent is the rate imposed under the National Internal Revenue
// Code as amended. It is stated here in one place and can be overridden for a
// period when a different rate applied — confirm the current rate with the
// Bureau of Internal Revenue.
const VATRatePercent = 12

// VATCheckTolerance is how far the tax column may differ from the rate applied
// to the taxable column before it is remarked on, in centavos. Line-level
// rounding across many invoices accumulates, so a peso of slack per record
// avoids a warning on every row of a correct file.
const VATCheckTolerance = 100

// CrossFootTolerance is how far a gross column given in the CSV may differ
// from the sum of the columns that are written before it is remarked on, in
// centavos.
const CrossFootTolerance = 1

// Severity says whether a finding stops the file being written.
type Severity int

const (
	// Fatal means the file will not be written.
	Fatal Severity = iota
	// Warning means the file will be written but something is worth checking.
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Finding is one thing wrong with one record, or with the file as a whole.
type Finding struct {
	Severity Severity `json:"severity"`
	// Line is the line of the CSV the record came from, or 0 for a finding
	// about the file rather than a row.
	Line int `json:"line"`
	// Record is which record of the data file is at fault.
	Record string `json:"record,omitempty"`
	// Field is the field of that record at fault.
	Field string `json:"field,omitempty"`
	// Value is what the field held.
	Value string `json:"value,omitempty"`
	// Detail says what is wrong, in words the person filing can act on.
	Detail string `json:"detail"`
}

func (f Finding) String() string {
	var b strings.Builder
	b.WriteString(f.Severity.String())
	if f.Line > 0 {
		fmt.Fprintf(&b, " on line %d", f.Line)
	}
	if f.Record != "" {
		fmt.Fprintf(&b, " in the %s record", f.Record)
	}
	if f.Field != "" {
		fmt.Fprintf(&b, ", field %q", f.Field)
	}
	if f.Value != "" {
		fmt.Fprintf(&b, " (%q)", f.Value)
	}
	b.WriteString(": ")
	b.WriteString(f.Detail)
	return b.String()
}

// anyFatal reports whether any finding in a list stops the file being written.
func anyFatal(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Fatal {
			return true
		}
	}
	return false
}

// ---------------------------------------------- checking one built record

// checkRecord checks the values of one record against the layout: the right
// number of fields, nothing required left empty, nothing wider than its field,
// no character that would split the record or that the validation module
// refuses, and every number and date in the exact form the layout asks for.
//
// It takes the values already rendered as strings, so that what is checked is
// what will be written — not an intermediate form that might differ.
func checkRecord(spec RecordSpec, values []string, line int) []Finding {
	var out []Finding
	at := func(sev Severity, field, value, detail string) {
		out = append(out, Finding{Severity: sev, Line: line, Record: spec.What, Field: field, Value: value, Detail: detail})
	}
	if len(values) != len(spec.Fields) {
		at(Fatal, "", "", fmt.Sprintf("the %s record has %d fields but the layout has %d; this is a fault in the tool, not in your file",
			spec.What, len(values), len(spec.Fields)))
		return out
	}
	if len(values) > MaxFieldsPerRecord {
		at(Fatal, "", "", fmt.Sprintf("the record has %d fields but the validation module reads only %d and drops the rest without a word",
			len(values), MaxFieldsPerRecord))
	}
	for i, f := range spec.Fields {
		v := values[i]
		if f.Required && v == "" {
			at(Fatal, f.Name, v, "this field may not be empty")
			continue
		}
		limit := f.Max
		if limit == 0 || limit > MaxFieldChars {
			limit = MaxFieldChars
		}
		if len(v) > limit {
			at(Fatal, f.Name, v, fmt.Sprintf("this field holds at most %d characters but the value is %d long", limit, len(v)))
		}
		if strings.ContainsRune(v, FieldSeparator) {
			at(Fatal, f.Name, v, fmt.Sprintf("the value contains the field separator %q, which would split the record in two", string(FieldSeparator)))
		}
		if strings.ContainsRune(v, TextDelimiter) {
			at(Fatal, f.Name, v, fmt.Sprintf("the value contains the text delimiter %q, which would end the field early", string(TextDelimiter)))
		}
		if strings.ContainsAny(v, "\r\n") {
			at(Fatal, f.Name, v, "the value contains a line break, which would split the record across two lines")
		}
		switch f.Kind {
		case DetailText, HeaderText:
			if bad := offendingCharacters(v); bad != "" {
				at(Fatal, f.Name, v, fmt.Sprintf("the validation module refuses these characters in a text field: %s", bad))
			}
		case BareText:
			if !allDigits(v) {
				at(Fatal, f.Name, v, "this field holds digits only")
			}
		case DetailAmount, HeaderAmount:
			if !looksLikeAmount(v, f.Kind) {
				at(Fatal, f.Name, v, amountRule(f.Kind))
			}
		case DateValue:
			if !looksLikeDate(v) {
				at(Fatal, f.Name, v, "a date must be written as MM/DD/YYYY, zero padded, with a four-digit year, and must be a date that exists")
			}
		case OptionalNumber:
			if v != "" && !allDigits(v) {
				at(Fatal, f.Name, v, "this field holds a whole number or nothing at all")
			}
		case TokenValue:
			// The record-type and transaction-code tokens are the two things
			// the validation module looks at first, so a value that is not
			// one of the tokens the layout allows is refused here.
			if !isKnownToken(v) {
				at(Fatal, f.Name, v, fmt.Sprintf("%q is not one of the tokens this format uses (%s, %s, %s, %s)",
					v, RecordTypeHeader, RecordTypeDetail, TransactionCodeSales, TransactionCodePurchases))
			}
		}
	}
	// The whole line has a ceiling too, because the submission routine reads
	// only so much of it and silently drops the rest.
	if n := len(render(spec, values)); n > MaxRecordChars {
		at(Fatal, "", "", fmt.Sprintf("the record is %d characters long, and the submission routine reads only the first %d — the rest would be dropped without a word, changing the figures. Shorten the names and addresses on this row",
			n, MaxRecordChars))
	}
	return out
}

// isKnownToken reports whether a value is one of the four fixed tokens the
// format uses in its first two fields.
func isKnownToken(v string) bool {
	switch v {
	case RecordTypeHeader, RecordTypeDetail, TransactionCodeSales, TransactionCodePurchases:
		return true
	}
	return false
}

// offendingCharacters returns the characters in a value that the validation
// module refuses, as a readable list.
func offendingCharacters(v string) string {
	var bad []string
	seen := map[rune]bool{}
	for _, r := range v {
		if seen[r] {
			continue
		}
		ok := r == 'Ñ' || (r < 0x80 && allowedByte(byte(r)))
		if !ok {
			seen[r] = true
			bad = append(bad, fmt.Sprintf("%q", string(r)))
		}
	}
	return strings.Join(bad, " ")
}

func amountRule(k FieldKind) string {
	if k == DetailAmount {
		return fmt.Sprintf("an amount on a detail record is a plain number with exactly %d decimal places, or the single digit 0 when it is nil — no thousands separator and no currency marker", AmountDecimals)
	}
	return fmt.Sprintf("a total in the header always carries exactly %d decimal places, with no thousands separator and no currency marker", AmountDecimals)
}

// looksLikeAmount accepts exactly what the writer produces for that kind of
// field, and nothing else.
func looksLikeAmount(s string, k FieldKind) bool {
	if k == DetailAmount && s == "0" {
		return true
	}
	t := strings.TrimPrefix(s, "-")
	if strings.HasPrefix(t, "-") {
		return false
	}
	whole, frac, found := strings.Cut(t, ".")
	if !found || whole == "" || len(frac) != AmountDecimals {
		return false
	}
	return allDigits(whole) && allDigits(frac)
}

// looksLikeDate accepts exactly MM/DD/YYYY, and only a date that exists in a
// year the validation module allows.
func looksLikeDate(s string) bool {
	if len(s) != 10 || s[2] != '/' || s[5] != '/' {
		return false
	}
	if !allDigits(s[0:2]) || !allDigits(s[3:5]) || !allDigits(s[6:10]) {
		return false
	}
	d, err := ParseDocDate(s, MonthFirst)
	return err == nil && d.Year() >= 1950
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------- the summary list's own rules

// checkFiler checks the taxpayer filing the list, whose details go in the
// header record and are repeated on every detail record.
func checkFiler(f Filer, k ListKind) []Finding {
	var out []Finding
	what := k.String() + " header"
	at := func(sev Severity, field, value, detail string) {
		out = append(out, Finding{Severity: sev, Record: what, Field: field, Value: value, Detail: detail})
	}
	if !f.TIN.Valid() {
		at(Fatal, "TIN of owner", f.TIN.Base, fmt.Sprintf("the filing taxpayer's TIN must be exactly %d digits", TINBaseDigits))
	}
	if f.TIN.BranchGiven && !f.TIN.IsHeadOffice() {
		at(Warning, "TIN of owner", f.TIN.Dashed(), fmt.Sprintf(
			"the branch code %s is not written: this data file has no field for one, so the file is filed under the nine digits %s", f.TIN.Branch, f.TIN.Base))
	}
	// The validation module wants at least one of the five name fields filled.
	if f.RegisteredName == "" && f.Person.Empty() && f.TradeName == "" {
		at(Fatal, "owner registered name", "", "the taxpayer filing the list has no name: give a registered name, or an individual's name, or at least a trade name")
	}
	if f.RegisteredName != "" && !f.Person.Empty() {
		at(Fatal, "owner registered name", f.RegisteredName,
			"the filing taxpayer has both a registered name and an individual's name; give one or the other")
	}
	// And at least one of the two address lines.
	if f.Address[0] == "" && f.Address[1] == "" {
		at(Fatal, "owner address line 1", "", "the taxpayer filing the list has no address, and the validation module refuses a header without one")
	}
	if len(f.RDOCode) != RDOCodeChars || !allDigits(f.RDOCode) {
		at(Fatal, "RDO code", f.RDOCode, fmt.Sprintf(
			"the revenue district office code must be exactly %d digits, so district 44 is written 044", RDOCodeChars))
	}
	if f.FiscalYearEndMonth != 0 && (f.FiscalYearEndMonth < 1 || f.FiscalYearEndMonth > 12) {
		at(Fatal, "fiscal or calendar month end", fmt.Sprint(f.FiscalYearEndMonth),
			"the month an accounting year ends in is 1 to 12, where 12 is a calendar year, or may be left out")
	}
	return out
}

// checkEntry checks one entry against the rules a summary list has to satisfy,
// as opposed to the rules about how a field is written.
func checkEntry(e Entry, k ListKind, filer Filer, opts Options) []Finding {
	var out []Finding
	at := func(sev Severity, field, value, detail string) {
		out = append(out, Finding{Severity: sev, Line: e.Line, Record: k.String() + " detail", Field: field, Value: value, Detail: detail})
	}
	party := k.Party()

	// --- identity
	switch {
	case e.Party.TIN.Base == "" && k == Purchases:
		at(Fatal, "TIN", "", "a purchases list must give the supplier's TIN")
	case e.Party.TIN.Base == "" && !e.Party.TINGiven:
		// A sale with no TIN is allowed. Say so once, because it is the kind
		// of thing a bookkeeper wants to notice.
		at(Warning, "TIN", "", "this sale has no customer TIN. The validation module allows that, but only an unregistered buyer such as an end consumer should be without one")
	case e.Party.TIN.Base == "":
		// The cell was filled in but could not be read, which has already
		// been reported against the column itself. Saying it twice helps
		// nobody.
	case !e.Party.TIN.Valid():
		at(Fatal, "TIN", e.Party.TIN.Base, fmt.Sprintf("the %s's TIN must be exactly %d digits", party, TINBaseDigits))
	case e.Party.TIN.Base == filer.TIN.Base:
		at(Fatal, "TIN", e.Party.TIN.Dashed(), fmt.Sprintf(
			"this is the filing taxpayer's own TIN. A summary list records dealings with other people, and the validation module refuses a %s whose TIN is the owner's", party))
	}

	hasReg := e.Party.RegisteredName != ""
	hasPerson := !e.Party.Person.Empty()
	switch {
	case hasReg && hasPerson:
		at(Fatal, "registered name", e.Party.RegisteredName,
			"this record fills in both the registered name and the individual's name; a record names one taxpayer, not two")
	case !hasReg && !hasPerson:
		at(Fatal, "registered name", "",
			fmt.Sprintf("this record names no %s at all: give a registered name for a company, or a surname and first name for an individual", party))
	}

	if k == Purchases && e.Party.Address[0] == "" && e.Party.Address[1] == "" {
		at(Fatal, "address line 1", "", "a purchases list must give the supplier's address")
	}

	// --- the amounts
	if k == Purchases {
		a := e.Purchases
		named := []namedAmount{
			{"exempt purchases", a.Exempt},
			{"zero-rated purchases", a.ZeroRated},
			{"taxable purchases of services", a.Services},
			{"taxable purchases of capital goods", a.CapitalGoods},
			{"taxable purchases of other goods", a.OtherGoods},
			{"input tax", a.InputVAT},
		}
		out = append(out, checkAmountSet(e, k, named, a.AllZero(), opts)...)
		out = append(out, checkVAT(e, k, a.TaxableTotal(), a.InputVAT, "input tax", opts)...)
		out = append(out, checkGross(e, k, a.Total(), "purchases")...)
	} else {
		a := e.Sales
		named := []namedAmount{
			{"exempt sales", a.Exempt},
			{"zero-rated sales", a.ZeroRated},
			{"taxable sales", a.Taxable},
			{"output tax", a.OutputVAT},
		}
		out = append(out, checkAmountSet(e, k, named, a.AllZero(), opts)...)
		out = append(out, checkVAT(e, k, a.Taxable, a.OutputVAT, "output tax", opts)...)
		out = append(out, checkGross(e, k, a.Total(), "sales")...)
	}
	return out
}

type namedAmount struct {
	name string
	v    int64
}

// checkAmountSet applies the rules that hold for every amount column: none may
// be negative, and a record where every column is nil has nothing to report.
func checkAmountSet(e Entry, k ListKind, named []namedAmount, allZero bool, opts Options) []Finding {
	var out []Finding
	at := func(sev Severity, field, value, detail string) {
		out = append(out, Finding{Severity: sev, Line: e.Line, Record: k.String() + " detail", Field: field, Value: value, Detail: detail})
	}
	for _, n := range named {
		if n.v < 0 && !opts.AllowNegativeAmounts {
			at(Fatal, n.name, FormatAmount(n.v), fmt.Sprintf(
				"a summary list has no column for a negative amount. Net the return, discount or credit memo against the month's figures for this %s and enter the result, or allow negative amounts if you have been told to file them that way",
				k.Party()))
		}
	}
	if allZero {
		at(Fatal, "", "", fmt.Sprintf(
			"every amount column on this row is nil, so there is nothing to report for this %s. Remove the row, or fill in the amounts",
			k.Party()))
	}
	return out
}

// checkVAT remarks on a tax column that does not sit at the statutory rate on
// the taxable column. It is a warning, not a refusal: a party may be partly
// zero-rated, may have a transitional rate, or may be a withholding agent, and
// only the taxpayer knows. The Bureau's own module once insisted on the rate
// and no longer does.
func checkVAT(e Entry, k ListKind, taxable, tax int64, field string, opts Options) []Finding {
	rate := opts.vatRate()
	if taxable == 0 && tax == 0 {
		return nil
	}
	if taxable == 0 && tax != 0 {
		return []Finding{{
			Severity: Warning, Line: e.Line, Record: k.String() + " detail", Field: field,
			Value:  FormatAmount(tax),
			Detail: fmt.Sprintf("there is %s of tax but no taxable amount to carry it", FormatPeso(tax)),
		}}
	}
	// Round half away from zero, as a tax computation does.
	want := (taxable*int64(rate)*2/100 + 1) / 2
	if diff := tax - want; diff > VATCheckTolerance || diff < -VATCheckTolerance {
		return []Finding{{
			Severity: Warning, Line: e.Line, Record: k.String() + " detail", Field: field,
			Value: FormatAmount(tax),
			Detail: fmt.Sprintf("%d%% of the taxable %s is %s, but the %s column says %s. Check the figure if this party is not partly zero-rated or exempt",
				rate, FormatPeso(taxable), FormatPeso(want), field, FormatPeso(tax)),
		}}
	}
	return nil
}

// checkGross compares a gross column the CSV supplied against the sum of the
// columns that are written. The data file has no gross field, so this can only
// ever be a warning — but a gross that does not add up usually means a column
// was mapped to the wrong heading.
func checkGross(e Entry, k ListKind, want int64, word string) []Finding {
	if !e.GrossGiven {
		return nil
	}
	if diff := e.Gross - want; diff > CrossFootTolerance || diff < -CrossFootTolerance {
		return []Finding{{
			Severity: Warning, Line: e.Line, Record: k.String() + " detail", Field: "gross " + word,
			Value: FormatAmount(e.Gross),
			Detail: fmt.Sprintf("the gross column says %s but the columns that are written add up to %s. The data file has no gross field, so this changes nothing in it — but check that each column was read into the right place",
				FormatPeso(e.Gross), FormatPeso(want)),
		}}
	}
	return nil
}

// checkTotals checks the header's totals against the detail records, which is
// the one cross-check the validation module performs on the file as a whole.
func checkTotals(entries []Entry, t Totals, k ListKind) []Finding {
	var out []Finding
	what := k.String() + " header"
	at := func(sev Severity, field, detail string) {
		out = append(out, Finding{Severity: sev, Record: what, Field: field, Detail: detail})
	}
	recomputed := addUp(entries, k, t.NonCreditableInputVAT)
	if recomputed.Sales != t.Sales || recomputed.Purchases != t.Purchases {
		at(Fatal, "", "the header's totals do not match the detail records. This is a fault in the tool; please report it")
	}
	if t.Records != len(entries) {
		at(Fatal, "", fmt.Sprintf("the header was built for %d detail records but the file holds %d. This is a fault in the tool; please report it", t.Records, len(entries)))
	}
	if k != Purchases {
		return out
	}
	if t.CreditableInputVAT+t.NonCreditableInputVAT != t.Purchases.InputVAT {
		at(Fatal, "creditable input tax", fmt.Sprintf(
			"the validation module requires the total input tax to equal the creditable and non-creditable amounts added together: %s + %s is not %s",
			FormatPeso(t.CreditableInputVAT), FormatPeso(t.NonCreditableInputVAT), FormatPeso(t.Purchases.InputVAT)))
	}
	if t.NonCreditableInputVAT < 0 {
		at(Fatal, "non-creditable input tax", "the non-creditable input tax cannot be negative")
	}
	if t.CreditableInputVAT < 0 {
		at(Fatal, "non-creditable input tax", fmt.Sprintf(
			"the non-creditable input tax you gave, %s, is more than the total input tax in the file, %s, which would make the creditable amount negative",
			FormatPeso(t.NonCreditableInputVAT), FormatPeso(t.Purchases.InputVAT)))
	}
	return out
}
