package slsp

import (
	"fmt"
	"sort"
	"strings"
)

// An Entry is one line of a summary list: one customer or supplier, and what
// was sold to or bought from them during the month. The data file writes one
// detail record per entry.

// Party is who the entry is about.
type Party struct {
	TIN TIN `json:"tin"`
	// TINGiven records whether the CSV had anything in the TIN column at all.
	// A cell that was filled in but could not be read is a different thing
	// from an empty one, and only the empty one is a sale to an unregistered
	// buyer, so only the empty one earns that remark.
	TINGiven       bool                 `json:"tinGiven"`
	Kind           TaxpayerKind         `json:"-"`
	KindName       string               `json:"kind"`
	RegisteredName string               `json:"registeredName,omitempty"`
	Person         PersonName           `json:"person"`
	Address        [AddressLines]string `json:"address"`
}

// Name is the party's name in one piece, for the interface and for messages.
func (p Party) Name() string {
	if p.RegisteredName != "" {
		return p.RegisteredName
	}
	if n := p.Person.String(); n != "" {
		return n
	}
	return "(unnamed)"
}

// sortKey is the order the Bureau's module writes records in: alphabetically
// by registered name, then by the three parts of an individual's name, then by
// TIN.
func (p Party) sortKey() string {
	return strings.Join([]string{
		p.RegisteredName, p.Person.Last, p.Person.First, p.Person.Middle, p.TIN.Base,
	}, "\x00")
}

// groupKey is what the module consolidates on: the month, the TIN and all four
// name fields. Two rows with the same TIN under different names stay two
// records, exactly as the module leaves them, and the caller is told.
func (p Party) groupKey() string {
	return strings.Join([]string{
		p.TIN.Base, p.RegisteredName, p.Person.Last, p.Person.First, p.Person.Middle,
	}, "\x00")
}

// SalesAmounts are the money columns of a sales entry, in centavos.
type SalesAmounts struct {
	Exempt    int64 `json:"exempt"`
	ZeroRated int64 `json:"zeroRated"`
	Taxable   int64 `json:"taxable"`
	OutputVAT int64 `json:"outputVat"`
}

// Total is every column added together, which is the gross including the tax.
// The data file has no field for it; it is used only to check a gross column
// the CSV happened to supply.
func (a SalesAmounts) Total() int64 {
	return a.Exempt + a.ZeroRated + a.Taxable + a.OutputVAT
}

// AllZero reports whether the entry has nothing to report.
func (a SalesAmounts) AllZero() bool {
	return a.Exempt == 0 && a.ZeroRated == 0 && a.Taxable == 0 && a.OutputVAT == 0
}

// PurchaseAmounts are the money columns of a purchases entry, in centavos.
type PurchaseAmounts struct {
	Exempt       int64 `json:"exempt"`
	ZeroRated    int64 `json:"zeroRated"`
	Services     int64 `json:"services"`
	CapitalGoods int64 `json:"capitalGoods"`
	OtherGoods   int64 `json:"otherGoods"`
	InputVAT     int64 `json:"inputVat"`
}

// TaxableTotal is every taxable purchase column added together, which is the
// base the input tax is checked against.
func (a PurchaseAmounts) TaxableTotal() int64 {
	return a.Services + a.CapitalGoods + a.OtherGoods
}

// Total is every column added together, including the input tax.
func (a PurchaseAmounts) Total() int64 {
	return a.Exempt + a.ZeroRated + a.TaxableTotal() + a.InputVAT
}

// AllZero reports whether the entry has nothing to report.
func (a PurchaseAmounts) AllZero() bool {
	return a.Exempt == 0 && a.ZeroRated == 0 && a.Services == 0 &&
		a.CapitalGoods == 0 && a.OtherGoods == 0 && a.InputVAT == 0
}

// Entry is one detail line of a summary list.
type Entry struct {
	// Line is the line of the CSV this entry came from, for messages.
	Line int `json:"line"`
	// Lines are every CSV line that went into it, when rows were consolidated.
	Lines []int `json:"lines,omitempty"`
	// Party is the customer or supplier.
	Party Party `json:"party"`
	// Sales is filled in on a sales list.
	Sales SalesAmounts `json:"sales"`
	// Purchases is filled in on a purchases list.
	Purchases PurchaseAmounts `json:"purchases"`
	// GrossGiven and Gross hold a gross column the CSV supplied, which the
	// data file has no field for but which is worth checking against the sum
	// of the columns that are written.
	GrossGiven bool  `json:"grossGiven"`
	Gross      int64 `json:"gross,omitempty"`
}

// -------------------------------------------------------- the CSV columns

// Canonical column names. The CSV's own headings are matched onto these.
const (
	ColTIN            = "TIN"
	ColKind           = "individual or not"
	ColRegisteredName = "registered name"
	ColName           = "name"
	ColSurname        = "surname"
	ColFirstName      = "first name"
	ColMiddleName     = "middle name"
	ColAddress        = "address line 1"
	ColAddress2       = "address line 2"

	ColExempt    = "exempt"
	ColZeroRated = "zero-rated"
	ColTaxable   = "taxable"
	ColOutputVAT = "output tax"
	ColGross     = "gross"

	ColServices     = "taxable services"
	ColCapitalGoods = "taxable capital goods"
	ColOtherGoods   = "taxable other goods"
	ColInputVAT     = "input tax"
)

// CSVColumns is the set of columns the CSV for one list may carry, with every
// heading spelling the reader accepts. The TIN is the only column that must be
// present; everything else is either optional or has an alternative.
func CSVColumns(k ListKind) []ColumnSpec {
	party := k.Party() // "customer" or "supplier"
	common := []ColumnSpec{
		{Name: ColTIN, Required: true, Aliases: []string{
			"tin", "tin no", "tin number", party + " tin", party + "'s tin",
			"tin of " + party, "vat reg tin", "tin/branch", "tin and branch",
		}},
		{Name: ColKind, Aliases: []string{
			"type", "taxpayer type", "individual", "individual or non-individual",
			"individual/corporation", "i/n", "ind or corp", party + " type", "entity type",
		}},
		{Name: ColRegisteredName, Aliases: []string{
			"registered name", "corporation name", "company name", "business name",
			"non-individual name", "trade name", party + " registered name",
		}},
		{Name: ColName, Aliases: []string{
			"name", party + " name", party, "customer", "supplier", "vendor",
			"payee", "payor", "client", "name of " + party,
		}},
		{Name: ColSurname, Aliases: []string{
			"surname", "last name", "lastname", "family name", "apelyido",
		}},
		{Name: ColFirstName, Aliases: []string{
			"first name", "firstname", "given name", "pangalan",
		}},
		{Name: ColMiddleName, Aliases: []string{
			"middle name", "middlename", "middle initial", "mi", "middle",
		}},
		{Name: ColAddress, Aliases: []string{
			"address", "address1", "address 1", "registered address",
			"business address", party + " address", "street", "unit and street",
		}},
		{Name: ColAddress2, Aliases: []string{
			"address2", "address 2", "city", "city/municipality", "municipality",
			"barangay", "province", "town", "address line two",
		}},
	}
	if k == Purchases {
		return append(common,
			ColumnSpec{Name: ColExempt, Aliases: []string{
				"exempt", "exempt purchases", "vat exempt", "vat-exempt purchases",
				"exempt amount", "vat exempt purchases",
			}},
			ColumnSpec{Name: ColZeroRated, Aliases: []string{
				"zero rated", "zero-rated", "zero rated purchases", "zero-rated purchases",
				"zero rated amount",
			}},
			ColumnSpec{Name: ColServices, Aliases: []string{
				"services", "taxable services", "purchase of services", "purchases of services",
				"taxable purchases of services", "services net of vat", "domestic purchase of services",
			}},
			ColumnSpec{Name: ColCapitalGoods, Aliases: []string{
				"capital goods", "taxable capital goods", "purchase of capital goods",
				"purchases of capital goods", "taxable purchases of capital goods",
				"domestic purchase of capital goods",
			}},
			ColumnSpec{Name: ColOtherGoods, Aliases: []string{
				"other goods", "goods other than capital goods", "taxable other goods",
				"purchase of goods other than capital goods", "taxable purchases of other goods",
				"other than capital goods", "goods", "domestic purchase of goods other than capital goods",
			}},
			ColumnSpec{Name: ColInputVAT, Aliases: []string{
				"input tax", "input vat", "vat", "input tax amount", "creditable input tax",
			}},
			ColumnSpec{Name: ColGross, Aliases: []string{
				"gross", "gross amount", "gross purchases", "total", "total purchases",
				"gross taxable purchases", "amount", "gross amount of purchases",
			}},
		)
	}
	return append(common,
		ColumnSpec{Name: ColExempt, Aliases: []string{
			"exempt", "exempt sales", "vat exempt", "vat-exempt sales", "exempt amount",
			"vat exempt sales",
		}},
		ColumnSpec{Name: ColZeroRated, Aliases: []string{
			"zero rated", "zero-rated", "zero rated sales", "zero-rated sales", "zero rated amount",
		}},
		ColumnSpec{Name: ColTaxable, Aliases: []string{
			"taxable", "taxable sales", "vatable sales", "vatable", "taxable amount",
			"net sales", "sales net of vat", "taxable net sales",
		}},
		ColumnSpec{Name: ColOutputVAT, Aliases: []string{
			"output tax", "output vat", "vat", "output tax amount", "vat amount", "12% vat",
		}},
		ColumnSpec{Name: ColGross, Aliases: []string{
			"gross", "gross amount", "gross sales", "total", "total sales",
			"gross taxable sales", "amount", "gross amount of sales",
		}},
	)
}

// ---------------------------------------------------- building one entry

// buildEntry turns one CSV row into an Entry, collecting every problem it
// finds rather than stopping at the first, so that a user fixing a file sees
// everything wrong with a row at once. Changes it had to make to a value, as
// opposed to failures, come back as notes.
func buildEntry(row Row, k ListKind, opts Options) (Entry, []Problem, []Problem) {
	var probs, notes []Problem
	e := Entry{Line: row.Line}
	add := func(field, value, detail string) {
		probs = append(probs, Problem{Line: row.Line, Field: field, Value: value, Detail: detail})
	}
	remark := func(field, value, detail string) {
		notes = append(notes, Problem{Line: row.Line, Field: field, Value: value, Detail: detail})
	}

	// --- the TIN
	tinCell := row.Get(ColTIN)
	switch {
	case tinCell == "" && k == Sales:
		// A sale may be to someone with no TIN. The record carries an empty
		// TIN field, which the validation module allows on a sales file.
	case tinCell == "":
		add(ColTIN, "", "a purchases list must give the supplier's TIN; the validation module refuses a record without one")
	default:
		e.Party.TINGiven = true
		tin, err := ParseTIN(tinCell)
		if err != nil {
			add(ColTIN, tinCell, err.Error())
		} else {
			e.Party.TIN = tin
			if tin.BranchGiven && !tin.IsHeadOffice() {
				remark(ColTIN, tinCell, fmt.Sprintf(
					"the branch code %s was read but is not written: this data file has no field for a branch code anywhere, only the nine digits %s",
					tin.Branch, tin.Base))
			}
			if tin.Suspicious() {
				remark(ColTIN, tinCell, "this TIN is a run of one digit. The validation module happens to allow it, but check it is really the "+k.Party()+"'s number")
			}
		}
	}

	// --- what kind of taxpayer, and the name that goes with it
	kind, kerr := ParseTaxpayerKind(row.Get(ColKind))
	if kerr != nil {
		add(ColKind, row.Get(ColKind), kerr.Error())
	}
	e.Party.RegisteredName, e.Party.Person, kind = resolveName(row, kind, opts, add, remark)
	e.Party.Kind = kind
	e.Party.KindName = kind.String()

	// --- the address
	e.Party.Address = resolveAddress(row, k, add, remark)

	// --- the amounts
	switch k {
	case Purchases:
		e.Purchases = purchaseAmounts(row, add)
	default:
		e.Sales = salesAmounts(row, add)
	}
	if row.Has(ColGross) {
		g, err := ParseAmount(row.Get(ColGross))
		if err != nil {
			add(ColGross, row.Get(ColGross), err.Error())
		} else {
			e.GrossGiven, e.Gross = true, g
		}
	}
	return e, probs, notes
}

// resolveName works out the four name fields from whichever name columns the
// CSV turned out to have, and settles what kind of taxpayer this is when the
// CSV did not say.
func resolveName(row Row, kind TaxpayerKind, opts Options, add, remark func(field, value, detail string)) (string, PersonName, TaxpayerKind) {
	clean := func(col string) string {
		raw := row.Get(col)
		out, changes := CleanText(raw)
		for _, c := range changes {
			remark(col, raw, c)
		}
		return out
	}
	reg := clean(ColRegisteredName)
	person := PersonName{
		Last:   clean(ColSurname),
		First:  clean(ColFirstName),
		Middle: clean(ColMiddleName),
	}
	single := strings.TrimSpace(row.Get(ColName))

	// The columns themselves say what kind of taxpayer this is when the CSV
	// did not: separate surname and first-name columns mean an individual, a
	// registered-name column means a non-individual.
	if kind == KindUnknown {
		switch {
		case person.Last != "" && person.First != "":
			kind = Individual
		case reg != "":
			kind = NonIndividual
		case single != "":
			// A single name column with a comma in it is the individual form;
			// without one it is taken as a registered name.
			if strings.Contains(single, ",") && opts.SingleNameCommaMeansIndividual {
				kind = Individual
			} else {
				kind = NonIndividual
			}
		}
	}

	switch kind {
	case Individual:
		if person.Last == "" && person.First == "" && single != "" {
			p, err := SplitPersonName(single)
			if err != nil {
				add(ColName, single, err.Error())
			} else {
				person = p
			}
		}
		if person.Last == "" && person.First == "" {
			add(ColSurname, "", "this row names an individual but gives no surname or first name; give a surname column, or write the name as \"SURNAME, First Middle\"")
		}
		// Filling both forms on one record is not refused by the validation
		// module, but it describes two different taxpayers, so it is refused
		// here.
		if reg != "" {
			add(ColRegisteredName, reg, "this row names an individual, so the registered-name column must be empty; a record names one taxpayer, not two")
			reg = ""
		}
		return "", person, kind
	case NonIndividual:
		if reg == "" && single != "" {
			out, changes := CleanText(single)
			for _, c := range changes {
				remark(ColName, single, c)
			}
			reg = out
		}
		if reg == "" {
			add(ColRegisteredName, "", "a non-individual needs a registered name")
		}
		if !person.Empty() {
			add(ColSurname, person.String(), "this row names a non-individual, so the surname, first-name and middle-name columns must be empty")
			person = PersonName{}
		}
		return reg, PersonName{}, kind
	}
	add(ColName, "", "this row has no name in any of the name columns, and the validation module refuses a record with all four name fields empty")
	return "", PersonName{}, kind
}

// resolveAddress fills the two address lines the record carries, shortening a
// line that is too long and saying so.
func resolveAddress(row Row, k ListKind, add, remark func(field, value, detail string)) [AddressLines]string {
	var out [AddressLines]string
	cols := [AddressLines]string{ColAddress, ColAddress2}
	for i, c := range cols {
		raw := row.Get(c)
		v, changes := CleanText(raw)
		for _, ch := range changes {
			remark(c, raw, ch)
		}
		cut, did := Truncate(v, MaxPartyAddressChars)
		if did {
			remark(c, v, fmt.Sprintf("the address line is longer than the %d characters the record allows, so it was shortened to %q", MaxPartyAddressChars, cut))
		}
		out[i] = cut
	}
	if k == Purchases && out[0] == "" && out[1] == "" {
		add(ColAddress, "", "a purchases list must give the supplier's address; the validation module refuses a record without one")
	}
	return out
}

func salesAmounts(row Row, add func(field, value, detail string)) SalesAmounts {
	get := amountGetter(row, add)
	return SalesAmounts{
		Exempt:    get(ColExempt),
		ZeroRated: get(ColZeroRated),
		Taxable:   get(ColTaxable),
		OutputVAT: get(ColOutputVAT),
	}
}

func purchaseAmounts(row Row, add func(field, value, detail string)) PurchaseAmounts {
	get := amountGetter(row, add)
	return PurchaseAmounts{
		Exempt:       get(ColExempt),
		ZeroRated:    get(ColZeroRated),
		Services:     get(ColServices),
		CapitalGoods: get(ColCapitalGoods),
		OtherGoods:   get(ColOtherGoods),
		InputVAT:     get(ColInputVAT),
	}
}

// amountGetter reads an amount column, reporting a cell it cannot read against
// its own column name and treating it as zero so the rest of the row is still
// checked.
func amountGetter(row Row, add func(field, value, detail string)) func(string) int64 {
	return func(col string) int64 {
		cell := row.Get(col)
		v, err := ParseAmount(cell)
		if err != nil {
			add(col, cell, err.Error())
			return 0
		}
		return v
	}
}

// ---------------------------------------------------------------- grouping

// Consolidate adds together the rows that name the same taxpayer and puts the
// result into the order the data file wants.
//
// This is what a summary list is: one line per customer or supplier for the
// month, not one line per invoice. The Bureau's module consolidates on the TIN
// together with all four name fields and then sorts alphabetically by name, so
// that is what happens here. Two rows carrying the same TIN under different
// names therefore stay two records — and the caller is told, because that is
// usually a typo.
func Consolidate(entries []Entry, k ListKind) ([]Entry, []MergeNote, []MergeNote) {
	type group struct {
		entry Entry
		lines []int
	}
	order := make([]string, 0, len(entries))
	groups := make(map[string]*group, len(entries))

	for _, e := range entries {
		key := e.Party.groupKey()
		g, ok := groups[key]
		if !ok {
			cp := e
			groups[key] = &group{entry: cp, lines: []int{e.Line}}
			order = append(order, key)
			continue
		}
		g.lines = append(g.lines, e.Line)
		if k == Purchases {
			g.entry.Purchases.Exempt += e.Purchases.Exempt
			g.entry.Purchases.ZeroRated += e.Purchases.ZeroRated
			g.entry.Purchases.Services += e.Purchases.Services
			g.entry.Purchases.CapitalGoods += e.Purchases.CapitalGoods
			g.entry.Purchases.OtherGoods += e.Purchases.OtherGoods
			g.entry.Purchases.InputVAT += e.Purchases.InputVAT
		} else {
			g.entry.Sales.Exempt += e.Sales.Exempt
			g.entry.Sales.ZeroRated += e.Sales.ZeroRated
			g.entry.Sales.Taxable += e.Sales.Taxable
			g.entry.Sales.OutputVAT += e.Sales.OutputVAT
		}
		g.entry.Gross += e.Gross
		g.entry.GrossGiven = g.entry.GrossGiven && e.GrossGiven
	}

	out := make([]Entry, 0, len(order))
	var merges []MergeNote
	for _, key := range order {
		g := groups[key]
		sort.Ints(g.lines)
		g.entry.Lines = g.lines
		out = append(out, g.entry)
		if len(g.lines) > 1 {
			merges = append(merges, MergeNote{
				TIN:   g.entry.Party.TIN.Dashed(),
				Name:  g.entry.Party.Name(),
				Lines: g.lines,
			})
		}
	}

	// One TIN under two names is two records, which is almost always a typo.
	byTIN := map[string]map[string]bool{}
	for _, e := range out {
		if e.Party.TIN.Base == "" {
			continue
		}
		if byTIN[e.Party.TIN.Base] == nil {
			byTIN[e.Party.TIN.Base] = map[string]bool{}
		}
		byTIN[e.Party.TIN.Base][e.Party.Name()] = true
	}
	var splits []MergeNote
	for _, e := range out {
		names := byTIN[e.Party.TIN.Base]
		if len(names) > 1 {
			keys := make([]string, 0, len(names))
			for n := range names {
				keys = append(keys, n)
			}
			sort.Strings(keys)
			note := MergeNote{TIN: e.Party.TIN.Dashed(), Name: strings.Join(keys, " / "), Lines: e.Lines}
			if !containsNote(splits, note.TIN) {
				splits = append(splits, note)
			}
		}
	}

	// The order the file is written in: alphabetically by name, then by TIN.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Party.sortKey() < out[j].Party.sortKey()
	})
	return out, merges, splits
}

func containsNote(notes []MergeNote, tin string) bool {
	for _, n := range notes {
		if n.TIN == tin {
			return true
		}
	}
	return false
}

// MergeNote records that several rows were added together into one record, or
// that one TIN turned up under more than one name.
type MergeNote struct {
	TIN   string `json:"tin"`
	Name  string `json:"name"`
	Lines []int  `json:"lines"`
}

func (m MergeNote) String() string {
	parts := make([]string, 0, len(m.Lines))
	for _, l := range m.Lines {
		parts = append(parts, fmt.Sprint(l))
	}
	return fmt.Sprintf("%s (%s): lines %s", m.Name, m.TIN, strings.Join(parts, ", "))
}
