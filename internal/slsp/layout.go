package slsp

// This file is the only place in the program that says what a summary-list
// data file looks like. Every other file asks this one: the writer walks the
// field tables to produce a record, the validator walks the same tables to
// check one, the interface renders them so the user can see exactly what will
// be written, and the explainer site prints them. Change a width, a token or
// an order here and the whole program changes with it.
//
// The layout below was taken from the Bureau of Internal Revenue's own
// Reconciliation of Listing for Enforcement data entry module, version 2.3:
// from the routine that writes the file, and cross-checked field by field
// against the routine in the matching validation module that accepts or
// rejects it. Where the two agree, a field is marked Sourced. Where only one
// of them speaks to a point, it is Corroborated. The few points neither
// settles are marked Uncertain and are listed in the documentation.

// Confidence records how well a part of the layout could be sourced.
type Confidence int

const (
	// Sourced means the Bureau's own module both writes this and checks it.
	Sourced Confidence = iota
	// Corroborated means the Bureau's module writes it, or checks it, but not
	// demonstrably both.
	Corroborated
	// Uncertain means this is the best available reading and the user should
	// check it against a file produced by the Bureau's own module before
	// relying on it. The documentation lists every Uncertain field.
	Uncertain
)

func (c Confidence) String() string {
	switch c {
	case Sourced:
		return "sourced"
	case Corroborated:
		return "corroborated"
	}
	return "uncertain"
}

// ---------------------------------------------------------- file mechanics

const (
	// FieldSeparator separates the fields of a record. The module builds each
	// record by concatenating values with a comma, and the validation module
	// reads the file back as a comma-delimited table.
	FieldSeparator = ','

	// TextDelimiter encloses a text field. Numeric fields and dates are
	// written bare.
	TextDelimiter = '"'

	// RecordSeparator ends a record. The module writes each one with a call
	// that appends a carriage return and a line feed.
	RecordSeparator = "\r\n"

	// AmountDecimals is how many decimal places an amount carries.
	AmountDecimals = 2

	// DateLayout is how every date is written: month, day, year, slash
	// separated, zero padded, four-digit year.
	DateLayout = "01/02/2006"

	// MaxFieldsPerRecord and MaxFieldChars are the ceilings the validation
	// module imposes, because it reads the file into a table of that shape.
	// A record wider than this, or a value longer than this, is silently
	// truncated by the module rather than reported, so the tool refuses it.
	MaxFieldsPerRecord = 25
	MaxFieldChars      = 254

	// MaxRecordChars is how much of a line the submission routine reads. A
	// longer line has its tail dropped without a word, which would change the
	// figures, so a record this long is refused.
	MaxRecordChars = 600
)

// ------------------------------------------------------------- field widths
//
// These are the widths of the module's own columns. A value longer than its
// width could not have come out of the module, so it is shortened and the user
// is told.

const (
	// MaxRegisteredNameChars is the width of a registered name, and of the
	// filing taxpayer's trade name.
	MaxRegisteredNameChars = 50
	MaxTradeNameChars      = 50

	// MaxSurnameChars, MaxFirstNameChars and MaxMiddleNameChars are the widths
	// of the three parts of an individual's name.
	MaxSurnameChars    = 30
	MaxFirstNameChars  = 30
	MaxMiddleNameChars = 30

	// MaxPartyAddressChars is the width of one of a customer's or supplier's
	// two address lines.
	MaxPartyAddressChars = 30

	// MaxFilerAddress1Chars and MaxFilerAddress2Chars are the widths of the
	// filing taxpayer's two address lines. The module builds the first from
	// three thirty-character columns joined by spaces and the second from two
	// thirty-character columns and a twelve-character postal code, which is
	// where these two numbers come from.
	MaxFilerAddress1Chars = 92
	MaxFilerAddress2Chars = 74

	// AddressLines is how many address lines a record carries.
	AddressLines = 2

	// RDOCodeChars is the exact length of the district office code in the
	// header. The validation module refuses any other length, so a code of
	// 44 has to be written 044.
	RDOCodeChars = 3

	// TotalsTolerance is how far the header's totals may differ from the sum
	// of the detail records before the validation module objects, in
	// centavos. This tool makes them agree exactly; the figure is here
	// because it is part of the format.
	TotalsTolerance = 200
)

// ---------------------------------------------------------------- the lists

// ListKind is which summary list is being produced.
type ListKind int

const (
	// Sales is the Summary List of Sales.
	Sales ListKind = iota
	// Purchases is the Summary List of Purchases.
	Purchases
)

func (k ListKind) String() string {
	if k == Purchases {
		return "purchases"
	}
	return "sales"
}

// Title is the name of the list, for the interface and the documentation.
func (k ListKind) Title() string {
	if k == Purchases {
		return "Summary List of Purchases"
	}
	return "Summary List of Sales"
}

// Party is what the list calls the other side of the transaction.
func (k ListKind) Party() string {
	if k == Purchases {
		return "supplier"
	}
	return "customer"
}

// ParseListKind reads which list is wanted.
func ParseListKind(s string) (ListKind, bool) {
	switch normaliseKey(s) {
	case "sales", "sale", "sls", "s", "summarylistofsales", "outward", "output":
		return Sales, true
	case "purchases", "purchase", "slp", "p", "summarylistofpurchases", "inward", "input":
		return Purchases, true
	}
	return Sales, false
}

// ------------------------------------------------------------ record tokens

const (
	// RecordTypeHeader and RecordTypeDetail are the tokens in the first field
	// of a record. There is no third kind: the file is one header record
	// followed by its detail records, and the header carries the totals, so
	// there is no trailer.
	RecordTypeHeader = "H"
	RecordTypeDetail = "D"
)

// TransactionCodeSales and TransactionCodePurchases are the tokens in the
// second field of every record, which say which list the file is.
const (
	TransactionCodeSales     = "S"
	TransactionCodePurchases = "P"
)

// TransactionCode is the code for this list.
func (k ListKind) TransactionCode() string {
	if k == Purchases {
		return TransactionCodePurchases
	}
	return TransactionCodeSales
}

// -------------------------------------------------------------- field kinds

// FieldKind says how a value is written into a record, and in particular
// whether it is enclosed in the text delimiter.
type FieldKind int

const (
	// DetailText is a text field of a detail record. It is enclosed in the
	// text delimiter when it has a value and written as nothing at all when
	// it is empty, which is what the module does: two separators side by
	// side, not a pair of delimiters with nothing between them.
	DetailText FieldKind = iota
	// HeaderText is a text field of the header record. It is always enclosed
	// in the text delimiter, empty or not, because the module builds the
	// header by concatenation with the delimiters written in.
	HeaderText
	// DetailAmount is a money field of a detail record: a bare decimal with
	// two places, except that nil is written as a single zero digit.
	DetailAmount
	// HeaderAmount is a total in the header record: a bare decimal with two
	// places always, including nil.
	HeaderAmount
	// BareText is a text field written without the delimiter: the filing
	// taxpayer's TIN where it repeats on a detail record, and the district
	// office code.
	BareText
	// DateValue is a bare date in DateLayout.
	DateValue
	// TokenValue is a fixed token written bare.
	TokenValue
	// OptionalNumber is a bare whole number that may be left empty.
	OptionalNumber
)

func (k FieldKind) String() string {
	switch k {
	case HeaderText:
		return "text"
	case DetailAmount, HeaderAmount:
		return "amount"
	case BareText:
		return "digits"
	case DateValue:
		return "date"
	case TokenValue:
		return "token"
	case OptionalNumber:
		return "number"
	}
	return "text"
}

// Quoted reports whether a field of this kind is enclosed in the text
// delimiter. A DetailText field is quoted only when it has a value, which
// QuotedWhenEmpty distinguishes.
func (k FieldKind) Quoted() bool { return k == DetailText || k == HeaderText }

// QuotedWhenEmpty reports whether an empty value still gets its delimiters.
func (k FieldKind) QuotedWhenEmpty() bool { return k == HeaderText }

// IsAmount reports whether the field holds money.
func (k FieldKind) IsAmount() bool { return k == DetailAmount || k == HeaderAmount }

// Field is one field of one record.
type Field struct {
	// Name is what this field is called in the documentation and in the
	// messages the validator produces.
	Name string
	// Kind says how the value is written.
	Kind FieldKind
	// Max is the greatest number of characters the value may occupy, or 0 for
	// no limit beyond MaxFieldChars. It does not count the delimiters.
	Max int
	// Required means the data file is rejected if this field is empty.
	Required bool
	// Confidence is how well this field could be sourced.
	Confidence Confidence
	// Note explains the field, for the documentation.
	Note string
}

// RecordSpec is the layout of one kind of record.
type RecordSpec struct {
	// What the record is called in the documentation.
	What string
	// Token is the value of the first field.
	Token string
	// Fields are every field of the record, in the order they are written,
	// including the record-type token itself.
	Fields []Field
}

// Width is how many fields the record has.
func (r RecordSpec) Width() int { return len(r.Fields) }

// FileSpec is the layout of one whole data file.
type FileSpec struct {
	Kind ListKind
	// Header is the single record the file opens with. It names the filing
	// taxpayer and carries the totals of every amount column.
	Header RecordSpec
	// Detail is one record per customer or supplier for the month.
	Detail RecordSpec
}

// Records returns the record specifications in file order.
func (f FileSpec) Records() []RecordSpec { return []RecordSpec{f.Header, f.Detail} }

// UncertainFields returns every field whose layout could not be sourced with
// confidence, so the documentation and the interface can say so.
func (f FileSpec) UncertainFields() []string {
	var out []string
	for _, rec := range f.Records() {
		for _, fl := range rec.Fields {
			if fl.Confidence == Uncertain {
				out = append(out, rec.What+" → "+fl.Name)
			}
		}
	}
	return out
}

// Layout returns the layout of the data file for one of the two lists. It is
// the single entry point to everything in this file.
func Layout(k ListKind) FileSpec {
	if k == Purchases {
		return purchasesFile
	}
	return salesFile
}

// ------------------------------------------------- the two layouts, in full

// filerFields are the first ten fields of a header record: who is filing. The
// same ten open both lists' headers.
func filerFields(code string) []Field {
	return []Field{
		{Name: "record type", Kind: TokenValue, Max: 1, Required: true, Confidence: Sourced,
			Note: "H. A file has exactly one header record and it comes first."},
		{Name: "transaction code", Kind: TokenValue, Max: 1, Required: true, Confidence: Sourced,
			Note: code + " on this list."},
		{Name: "TIN of owner", Kind: HeaderText, Max: TINBaseDigits, Required: true, Confidence: Sourced,
			Note: "The filing taxpayer's own TIN: exactly nine digits, no dashes and no branch code."},
		{Name: "owner registered name", Kind: HeaderText, Max: MaxRegisteredNameChars, Confidence: Sourced,
			Note: "The registered name of a non-individual filer. Empty for an individual."},
		{Name: "owner last name", Kind: HeaderText, Max: MaxSurnameChars, Confidence: Sourced,
			Note: "An individual filer's last name. Empty for a non-individual."},
		{Name: "owner first name", Kind: HeaderText, Max: MaxFirstNameChars, Confidence: Sourced,
			Note: "An individual filer's first name."},
		{Name: "owner middle name", Kind: HeaderText, Max: MaxMiddleNameChars, Confidence: Sourced,
			Note: "An individual filer's middle name."},
		{Name: "owner trade name", Kind: HeaderText, Max: MaxTradeNameChars, Confidence: Sourced,
			Note: "The business name the filer trades under, if different. At least one of the five name fields must have a value."},
		{Name: "owner address line 1", Kind: HeaderText, Max: MaxFilerAddress1Chars, Confidence: Sourced,
			Note: "Unit and street and barangay. At least one of the two address lines must have a value."},
		{Name: "owner address line 2", Kind: HeaderText, Max: MaxFilerAddress2Chars, Confidence: Sourced,
			Note: "District, city or municipality, and postal code."},
	}
}

// closingFields are the last fields of a header record, after the totals.
func closingFields() []Field {
	return []Field{
		{Name: "RDO code", Kind: BareText, Max: RDOCodeChars, Required: true, Confidence: Sourced,
			Note: "The code of the revenue district office the filer is registered with, in exactly three characters — so district 44 is written 044."},
		{Name: "tax month", Kind: DateValue, Required: true, Confidence: Sourced,
			Note: "The last day of the month the list covers, as MM/DD/YYYY. Every record in the file repeats it."},
		{Name: "fiscal or calendar month end", Kind: OptionalNumber, Max: 2, Confidence: Sourced,
			Note: "The month the filer's accounting year ends in, 1 to 12, where 12 is a calendar year. May be left empty."},
	}
}

// partyFields are the fields of a detail record that identify the other side
// of the transaction. Both lists carry the same seven.
func partyFields(code, party string, tinRequired, addressRequired bool) []Field {
	tinNote := "The " + party + "'s TIN: exactly nine digits, no dashes and no branch code."
	if !tinRequired {
		tinNote += " It may be left empty for a sale to someone who is not registered, such as an end consumer."
	}
	addrNote := "The first of the " + party + "'s two address lines."
	if addressRequired {
		addrNote += " At least one of the two must have a value."
	}
	return []Field{
		{Name: "record type", Kind: TokenValue, Max: 1, Required: true, Confidence: Sourced,
			Note: "D for a detail record."},
		{Name: "transaction code", Kind: TokenValue, Max: 1, Required: true, Confidence: Sourced,
			Note: code + " on this list."},
		{Name: "TIN", Kind: DetailText, Max: TINBaseDigits, Required: tinRequired, Confidence: Sourced,
			Note: tinNote + " It may not be the filing taxpayer's own TIN."},
		{Name: "registered name", Kind: DetailText, Max: MaxRegisteredNameChars, Confidence: Sourced,
			Note: "The registered name of a non-individual " + party + ". At least one of the four name fields must have a value."},
		{Name: "last name", Kind: DetailText, Max: MaxSurnameChars, Confidence: Sourced,
			Note: "An individual " + party + "'s last name."},
		{Name: "first name", Kind: DetailText, Max: MaxFirstNameChars, Confidence: Sourced,
			Note: "An individual " + party + "'s first name."},
		{Name: "middle name", Kind: DetailText, Max: MaxMiddleNameChars, Confidence: Sourced,
			Note: "An individual " + party + "'s middle name."},
		{Name: "address line 1", Kind: DetailText, Max: MaxPartyAddressChars, Required: addressRequired, Confidence: Sourced,
			Note: addrNote},
		{Name: "address line 2", Kind: DetailText, Max: MaxPartyAddressChars, Confidence: Sourced,
			Note: "The second of the " + party + "'s two address lines."},
	}
}

// trailingDetailFields are the last two fields of every detail record.
func trailingDetailFields() []Field {
	return []Field{
		{Name: "TIN of owner", Kind: BareText, Max: TINBaseDigits, Required: true, Confidence: Sourced,
			Note: "The filing taxpayer's own TIN again, written without the text delimiter. It must match the header's."},
		{Name: "tax month", Kind: DateValue, Required: true, Confidence: Sourced,
			Note: "The last day of the month the list covers, the same on every record."},
	}
}

var salesFile = FileSpec{
	Kind: Sales,
	Header: RecordSpec{
		What:  "sales header",
		Token: RecordTypeHeader,
		Fields: concatFields(
			filerFields(TransactionCodeSales),
			[]Field{
				{Name: "total exempt sales", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the exempt sales on every detail record."},
				{Name: "total zero-rated sales", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the zero-rated sales."},
				{Name: "total taxable sales", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the taxable sales, net of the tax."},
				{Name: "total output tax", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the output tax."},
			},
			closingFields(),
		),
	},
	Detail: RecordSpec{
		What:  "sales detail",
		Token: RecordTypeDetail,
		Fields: concatFields(
			// A sale may be to someone with no TIN, so the customer's TIN is
			// not required and nor is the address.
			partyFields(TransactionCodeSales, "customer", false, false),
			[]Field{
				{Name: "exempt sales", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Sales to this customer that are exempt from value-added tax."},
				{Name: "zero-rated sales", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Sales to this customer subject to value-added tax at nil."},
				{Name: "taxable sales", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Sales to this customer subject to value-added tax, net of the tax."},
				{Name: "output tax", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "The value-added tax on those taxable sales."},
			},
			trailingDetailFields(),
		),
	},
}

var purchasesFile = FileSpec{
	Kind: Purchases,
	Header: RecordSpec{
		What:  "purchases header",
		Token: RecordTypeHeader,
		Fields: concatFields(
			filerFields(TransactionCodePurchases),
			[]Field{
				{Name: "total exempt purchases", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the exempt purchases on every detail record."},
				{Name: "total zero-rated purchases", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the zero-rated purchases."},
				{Name: "total taxable purchases of services", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the taxable purchases of services."},
				{Name: "total taxable purchases of capital goods", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the taxable purchases of capital goods."},
				{Name: "total taxable purchases of other goods", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the taxable purchases of goods other than capital goods."},
				{Name: "total input tax", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The sum of the input tax. It must equal the creditable and non-creditable amounts added together."},
				{Name: "creditable input tax", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "So much of the input tax as is creditable this month. Given for the month as a whole, not per supplier."},
				{Name: "non-creditable input tax", Kind: HeaderAmount, Required: true, Confidence: Sourced,
					Note: "The rest of the input tax. The total input tax must equal this plus the creditable amount."},
			},
			closingFields(),
		),
	},
	Detail: RecordSpec{
		What:  "purchases detail",
		Token: RecordTypeDetail,
		Fields: concatFields(
			// Unlike a sale, a purchase must name a registered supplier and
			// give an address.
			partyFields(TransactionCodePurchases, "supplier", true, true),
			[]Field{
				{Name: "exempt purchases", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Purchases from this supplier that are exempt from value-added tax."},
				{Name: "zero-rated purchases", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Purchases from this supplier subject to value-added tax at nil."},
				{Name: "taxable purchases of services", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Taxable purchases of services, net of the tax."},
				{Name: "taxable purchases of capital goods", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Taxable purchases of capital goods, net of the tax."},
				{Name: "taxable purchases of other goods", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "Taxable purchases of goods other than capital goods, net of the tax."},
				{Name: "input tax", Kind: DetailAmount, Required: true, Confidence: Sourced,
					Note: "The value-added tax on those taxable purchases."},
			},
			trailingDetailFields(),
		),
	},
}

// concatFields joins field lists into one, copying as it goes so that the
// shared helpers above can never be aliased into two layouts at once.
func concatFields(groups ...[]Field) []Field {
	n := 0
	for _, g := range groups {
		n += len(g)
	}
	out := make([]Field, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// normaliseKey reduces a word to lower-case letters and digits, for matching
// the names of settings.
func normaliseKey(s string) string { return reduceHeading(s) }

// ---------------------------------------------- the layout, for display

// FieldDescription is one field of the layout in a form the interface and the
// explainer site can render, so that the same table drives the program, the
// documentation and the screen.
type FieldDescription struct {
	Position   int    `json:"position"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Quoted     string `json:"quoted"`
	Max        int    `json:"max"`
	Required   bool   `json:"required"`
	Confidence string `json:"confidence"`
	Note       string `json:"note,omitempty"`
}

// RecordDescription is one record of the layout, for display.
type RecordDescription struct {
	What   string             `json:"what"`
	Token  string             `json:"token"`
	Fields []FieldDescription `json:"fields"`
}

// LayoutDescription is a whole data file's layout, for display.
type LayoutDescription struct {
	List            string              `json:"list"`
	FieldSeparator  string              `json:"fieldSeparator"`
	TextDelimiter   string              `json:"textDelimiter"`
	RecordSeparator string              `json:"recordSeparator"`
	Encoding        string              `json:"encoding"`
	FileNamePattern string              `json:"fileNamePattern"`
	Records         []RecordDescription `json:"records"`
	Uncertain       []string            `json:"uncertain"`
}

// Describe renders the layout for display.
func Describe(k ListKind) LayoutDescription {
	f := Layout(k)
	out := LayoutDescription{
		List:            f.Kind.Title(),
		FieldSeparator:  string(rune(FieldSeparator)),
		TextDelimiter:   string(rune(TextDelimiter)),
		RecordSeparator: "CR LF",
		Encoding:        EncodingName,
		FileNamePattern: FileNameTemplate,
		Uncertain:       f.UncertainFields(),
	}
	for _, rec := range f.Records() {
		rd := RecordDescription{What: rec.What, Token: rec.Token}
		for i, fl := range rec.Fields {
			rd.Fields = append(rd.Fields, FieldDescription{
				Position:   i + 1,
				Name:       fl.Name,
				Kind:       fl.Kind.String(),
				Quoted:     quotingRule(fl.Kind),
				Max:        fl.Max,
				Required:   fl.Required,
				Confidence: fl.Confidence.String(),
				Note:       fl.Note,
			})
		}
		out.Records = append(out.Records, rd)
	}
	return out
}

func quotingRule(k FieldKind) string {
	switch {
	case k.QuotedWhenEmpty():
		return "always"
	case k.Quoted():
		return "when not empty"
	}
	return "never"
}
