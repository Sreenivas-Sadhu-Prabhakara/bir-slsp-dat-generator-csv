package slsp

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Generate is the whole job in one function: read the CSV, build the records,
// check every one of them, and produce the data file only if nothing fatal was
// found.

// Filer is the taxpayer submitting the list — the one whose return this
// belongs to, named in the header record and repeated on every detail record.
type Filer struct {
	TIN            TIN                  `json:"tin"`
	RegisteredName string               `json:"registeredName,omitempty"`
	Person         PersonName           `json:"person"`
	TradeName      string               `json:"tradeName,omitempty"`
	Address        [AddressLines]string `json:"address"`
	// RDOCode is the revenue district office the filer is registered with,
	// in exactly three digits.
	RDOCode string `json:"rdoCode"`
	// FiscalYearEndMonth is the month the filer's accounting year ends in,
	// 1 to 12, where 12 is a calendar year. Zero leaves the field empty.
	FiscalYearEndMonth int `json:"fiscalYearEndMonth"`
}

// Name is the filer's name in one piece.
func (f Filer) Name() string {
	switch {
	case f.RegisteredName != "":
		return f.RegisteredName
	case !f.Person.Empty():
		return f.Person.String()
	}
	return f.TradeName
}

// cleanFiler puts the filing taxpayer's own details through the same cleaning
// as every other text value, and reports anything it had to change.
func cleanFiler(f Filer) (Filer, []string) {
	var changes []string
	clean := func(what string, v *string) {
		out, ch := CleanText(*v)
		for _, c := range ch {
			changes = append(changes, "in the filer's "+what+": "+c)
		}
		*v = out
	}
	clean("registered name", &f.RegisteredName)
	clean("trade name", &f.TradeName)
	clean("last name", &f.Person.Last)
	clean("first name", &f.Person.First)
	clean("middle name", &f.Person.Middle)
	clean("first address line", &f.Address[0])
	clean("second address line", &f.Address[1])
	f.RDOCode = strings.TrimSpace(f.RDOCode)
	return f, changes
}

// Options are the choices that govern one run.
type Options struct {
	// List is which of the two summary lists to produce.
	List ListKind
	// Period is the month the list covers. A summary list is filed one month
	// at a time, so a quarter is three files.
	Period Period
	// Filer is the taxpayer submitting the list.
	Filer Filer
	// NonCreditableInputVAT is how much of a purchases list's input tax is not
	// creditable this month, in centavos. It is a figure for the month as a
	// whole, which only the taxpayer knows; the creditable amount is the rest.
	NonCreditableInputVAT int64
	// DateOrder is how to read a numeric date in the CSV.
	DateOrder DateOrder
	// VATRate overrides the statutory rate used to check the tax columns.
	// Zero means use VATRatePercent.
	VATRate int
	// AllowNegativeAmounts lets a negative figure through. It is off by
	// default because a summary list has no column for one.
	AllowNegativeAmounts bool
	// NoConsolidate keeps one record per CSV row instead of adding together
	// the rows that name the same taxpayer. A summary list is a list per
	// taxpayer, so consolidating is the default.
	NoConsolidate bool
	// SingleNameCommaMeansIndividual decides how a single name column with a
	// comma in it is read when the CSV does not say what kind of taxpayer it
	// is. On by default.
	SingleNameCommaMeansIndividual bool
}

// DefaultOptions are the settings a run starts from.
func DefaultOptions() Options {
	return Options{
		List:                           Sales,
		DateOrder:                      MonthFirst,
		VATRate:                        VATRatePercent,
		SingleNameCommaMeansIndividual: true,
	}
}

func (o Options) vatRate() int {
	if o.VATRate <= 0 {
		return VATRatePercent
	}
	return o.VATRate
}

// Result is everything one run produced.
type Result struct {
	// List and Period say what was produced.
	List   string `json:"list"`
	Period string `json:"period"`
	// FileName is the name the data file must be saved under.
	FileName string `json:"fileName"`
	// SubmissionFileName is the name of the copy carrying the submission key.
	SubmissionFileName string `json:"submissionFileName"`
	// Entries are the records that were built, in file order.
	Entries []Entry `json:"entries"`
	// Totals are the figures in the header record.
	Totals Totals `json:"totals"`
	// Findings are everything wrong, or worth remarking on, in the records.
	Findings []Finding `json:"findings"`
	// Changes are the values the tool had to alter to fit the format, each
	// against the line it came from.
	Changes []Problem `json:"changes"`
	// Merges records where several rows were added together, and Splits where
	// one TIN turned up under more than one name.
	Merges []MergeNote `json:"merges"`
	Splits []MergeNote `json:"splits"`
	// Notes are remarks about the file as a whole.
	Notes []string `json:"notes"`
	// Columns maps each column the tool wanted to the heading it matched.
	Columns map[string]string `json:"columns"`
	// Unmatched are the CSV's headings that nothing claimed.
	Unmatched []string `json:"unmatched"`
	// RowsRead is how many data rows the CSV had.
	RowsRead int `json:"rowsRead"`
	// Dat is the data file as text. It is empty when the file was refused.
	Dat string `json:"dat"`
	// Submission is the same file with the submission key line appended.
	Submission string `json:"submission"`
	// ValidationKey is the key on that last line.
	ValidationKey string `json:"validationKey"`
	// OK says whether the data file was produced.
	OK bool `json:"ok"`
	// UncertainFields lists the parts of the layout that could not be sourced
	// with confidence, so the interface can repeat the warning.
	UncertainFields []string `json:"uncertainFields"`
}

// FatalCount and WarningCount are how many of each kind of finding there are.
func (r *Result) FatalCount() int   { return countSeverity(r.Findings, Fatal) }
func (r *Result) WarningCount() int { return countSeverity(r.Findings, Warning) }

// DatBytes is the data file as the bytes that go on disk, in the single-byte
// encoding the format uses.
func (r *Result) DatBytes() []byte { return Encode(r.Dat) }

// SubmissionBytes is the submission copy as the bytes that go on disk.
func (r *Result) SubmissionBytes() []byte { return Encode(r.Submission) }

func countSeverity(fs []Finding, s Severity) int {
	n := 0
	for _, f := range fs {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Generate reads a CSV and produces the data file for one summary list.
//
// The returned error is about the run as a whole — the CSV could not be read,
// the period was not given, the filer was not named. Everything wrong with an
// individual record is in the Result's findings, with the line of the CSV it
// came from, and leaves Result.OK false and Result.Dat empty.
func Generate(csv io.Reader, opts Options) (*Result, error) {
	if !opts.Period.Valid() {
		return nil, fmt.Errorf("the month the list covers was not given")
	}
	if opts.Filer.TIN.Base == "" {
		return nil, fmt.Errorf("the TIN of the taxpayer filing the list was not given")
	}
	if opts.Filer.Name() == "" {
		return nil, fmt.Errorf("the name of the taxpayer filing the list was not given")
	}

	// The filing taxpayer's own details go through the same cleaning as every
	// other value, here rather than in the caller, so that a caller which
	// forgot cannot put a character into the header that the validation
	// module refuses.
	var filerChanges []string
	opts.Filer, filerChanges = cleanFiler(opts.Filer)

	spec := Layout(opts.List)
	res := &Result{
		List:               opts.List.Title(),
		Period:             opts.Period.String(),
		FileName:           FileName(opts.Filer.TIN, opts.Period, opts.List),
		SubmissionFileName: SubmissionFolder + "/" + FileName(opts.Filer.TIN, opts.Period, opts.List),
		Columns:            map[string]string{},
		UncertainFields:    spec.UncertainFields(),
	}

	tbl, err := ReadTable(csv, CSVColumns(opts.List))
	if err != nil {
		return nil, err
	}
	res.Columns = tbl.Columns
	res.Unmatched = tbl.Unmatched
	res.Notes = append(res.Notes, tbl.Notes...)
	res.RowsRead = len(tbl.Rows)
	res.Notes = append(res.Notes, fmt.Sprintf("The headings were found on line %d and the columns are separated by %s.",
		tbl.HeaderLine, separatorName(tbl.Separator)))

	// --- the taxpayer filing the list
	for _, c := range filerChanges {
		res.Findings = append(res.Findings, Finding{
			Severity: Warning, Record: opts.List.String() + " header", Detail: c,
		})
	}
	res.Findings = append(res.Findings, checkFiler(opts.Filer, opts.List)...)

	// --- build an entry per row, collecting the cells that would not read
	entries := make([]Entry, 0, len(tbl.Rows))
	for _, row := range tbl.Rows {
		e, probs, changes := buildEntry(row, opts.List, opts)
		res.Changes = append(res.Changes, changes...)
		for _, p := range probs {
			res.Findings = append(res.Findings, Finding{
				Severity: Fatal, Line: p.Line, Record: opts.List.String() + " detail",
				Field: p.Field, Value: p.Value, Detail: p.Detail,
			})
		}
		for _, c := range changes {
			res.Findings = append(res.Findings, Finding{
				Severity: Warning, Line: c.Line, Record: opts.List.String() + " detail",
				Field: c.Field, Value: c.Value, Detail: c.Detail,
			})
		}
		entries = append(entries, e)
	}

	// --- one record per taxpayer, in the order the file wants
	if !opts.NoConsolidate {
		merged, merges, splits := Consolidate(entries, opts.List)
		entries, res.Merges, res.Splits = merged, merges, splits
		if len(merges) > 0 {
			res.Notes = append(res.Notes, fmt.Sprintf(
				"%d taxpayer(s) appeared on more than one row; their rows were added together, because a summary list carries one line per %s for the month.",
				len(merges), opts.List.Party()))
		}
		for _, s := range res.Splits {
			res.Findings = append(res.Findings, Finding{
				Severity: Warning, Record: opts.List.String() + " detail", Field: "registered name",
				Value: s.TIN,
				Detail: fmt.Sprintf("the TIN %s appears under more than one name (%s), so it is written as that many records. Usually this is a spelling difference worth tidying up",
					s.TIN, s.Name),
			})
		}
	}
	res.Entries = entries
	res.Totals = addUp(entries, opts.List, opts.NonCreditableInputVAT)

	// --- the summary list's own rules
	for _, e := range entries {
		res.Findings = append(res.Findings, checkEntry(e, opts.List, opts.Filer, opts)...)
	}
	res.Findings = append(res.Findings, checkTotals(entries, res.Totals, opts.List)...)
	if len(entries) == 0 {
		res.Findings = append(res.Findings, Finding{
			Severity: Fatal, Detail: "the CSV produced no records, so there is nothing to file. The validation module refuses a file with no detail records",
		})
	}

	// --- build the records and check the bytes that would be written
	lines, buildFindings := buildAll(spec, opts, entries, res.Totals)
	res.Findings = append(res.Findings, buildFindings...)

	// Put the findings in the order somebody fixing the file will work
	// through them: by the line of the CSV, errors before remarks.
	sort.SliceStable(res.Findings, func(i, j int) bool {
		a, b := res.Findings[i], res.Findings[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Severity < b.Severity
	})

	if anyFatal(res.Findings) {
		return res, nil
	}

	res.Dat = strings.Join(lines, RecordSeparator) + RecordSeparator
	base := strings.TrimSuffix(res.FileName, FileExtension)
	key, err := ValidationKey(base, lines)
	if err != nil {
		res.Findings = append(res.Findings, Finding{Severity: Fatal, Detail: err.Error()})
		res.Dat = ""
		return res, nil
	}
	res.ValidationKey = key
	res.Submission = strings.Join(append(lines, key), RecordSeparator) + RecordSeparator
	res.OK = true
	return res, nil
}

// buildAll builds every record of the file and checks each one against the
// layout before any of it is joined together.
func buildAll(spec FileSpec, opts Options, entries []Entry, totals Totals) ([]string, []Finding) {
	var findings []Finding
	lines := make([]string, 0, len(entries)+1)

	addRecord := func(rec RecordSpec, values, mismatch []string, line int) {
		for _, m := range mismatch {
			findings = append(findings, Finding{
				Severity: Fatal, Line: line, Record: rec.What,
				Detail: "the record does not match the layout: " + m + ". This is a fault in the tool; please report it",
			})
		}
		findings = append(findings, checkRecord(rec, values, line)...)
		lines = append(lines, render(rec, values))
	}

	hv, hm := buildHeader(spec, opts.Filer, opts.Period, totals)
	addRecord(spec.Header, hv, hm, 0)

	for _, e := range entries {
		dv, dm := buildDetail(spec, e, opts.Filer, opts.Period)
		addRecord(spec.Detail, dv, dm, e.Line)
	}
	return lines, findings
}

func separatorName(r rune) string {
	switch r {
	case ',':
		return "commas"
	case ';':
		return "semicolons"
	case '\t':
		return "tabs"
	case '|':
		return "pipes"
	}
	return fmt.Sprintf("%q", string(r))
}

// ------------------------------------------------------------- the file name

// The data file's name is not decoration: the validation module reads the
// filing taxpayer's TIN, the list and the period back out of it and checks
// them against the file's contents, and offers to rename a file that does not
// fit. So the name is built the same way the Bureau's module builds it.
const (
	// FileNameTemplate is the shape of the name: nine digits of TIN, one
	// letter for the list, two digits of month, four of year.
	FileNameTemplate = "<TIN 9 digits><S or P><MM><YYYY>.DAT"

	// FileExtension is the extension the data file carries, upper case.
	FileExtension = ".DAT"
)

// FileName returns the name the data file must be saved under.
func FileName(t TIN, p Period, k ListKind) string {
	if len(t.Base) != TINBaseDigits || !p.Valid() {
		return ""
	}
	return t.Base + k.TransactionCode() + p.MMYYYY() + FileExtension
}
