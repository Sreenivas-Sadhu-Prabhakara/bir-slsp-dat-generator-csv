package slsp

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// Reading a CSV that came out of somebody else's spreadsheet is the part of
// this tool most likely to meet something unexpected, so the reader here
// assumes nothing: it strips a byte-order mark, works out the separator for
// itself, accepts either line ending, finds the header row wherever it is,
// matches column names loosely, and tolerates rows that are shorter or longer
// than the header. Every problem it meets is reported against the line number
// of the file as the user sees it in a text editor, and nothing it meets can
// make it panic.

// maxCSVBytes caps how much of a file is read. A summary list for a small
// business is a few hundred kilobytes; this is room for a very large one.
const maxCSVBytes = 64 << 20

// maxHeaderSearchRows is how many rows from the top are examined for the
// header, so that a title row, a blank row and a company name above the
// headings do not defeat the reader.
const maxHeaderSearchRows = 25

// ErrCSV is the base of every failure that is about the file rather than about
// one row in it.
var ErrCSV = errors.New("the CSV could not be read")

// Problem is one thing wrong with one row, named by the line number the user
// would see in a text editor.
type Problem struct {
	Line   int    `json:"line"`
	Field  string `json:"field,omitempty"`
	Value  string `json:"value,omitempty"`
	Detail string `json:"detail"`
}

func (p Problem) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "line %d", p.Line)
	if p.Field != "" {
		fmt.Fprintf(&b, ", column %q", p.Field)
	}
	b.WriteString(": ")
	b.WriteString(p.Detail)
	return b.String()
}

// Table is a CSV read into rows addressed by canonical column name.
type Table struct {
	// HeaderLine is the line number the header row was found on.
	HeaderLine int
	// Separator is the character the file turned out to be separated by.
	Separator rune
	// Columns maps each canonical name that was found to the heading as it was
	// actually spelled in the file, so the interface can show the user what it
	// matched.
	Columns map[string]string
	// Unmatched holds headings in the file that no canonical name claimed.
	Unmatched []string
	// Rows are the data rows, in file order.
	Rows []Row
	// Notes are remarks about the file as a whole — a skipped title row, a
	// dropped total row, a heading that was not recognised.
	Notes []string
}

// Row is one data row: its line number and its cells by canonical name.
type Row struct {
	Line  int
	cells map[string]string
}

// Get returns the cell for a canonical column name, trimmed, or the empty
// string if the file has no such column. It never panics, including on a Row
// that was never filled in.
func (r Row) Get(name string) string {
	if r.cells == nil {
		return ""
	}
	return r.cells[name]
}

// Has reports whether the row carries a non-empty value for a column.
func (r Row) Has(name string) bool { return r.Get(name) != "" }

// IsBlank reports whether every cell in the row is empty, which is how a
// trailing empty row from a spreadsheet presents itself.
func (r Row) IsBlank() bool {
	for _, v := range r.cells {
		if v != "" {
			return false
		}
	}
	return true
}

// Cells returns a copy of the row's values, for reporting.
func (r Row) Cells() map[string]string {
	out := make(map[string]string, len(r.cells))
	for k, v := range r.cells {
		out[k] = v
	}
	return out
}

// ColumnSpec declares one column the caller wants, with every spelling it is
// willing to accept. The aliases are matched on a reduced form of the heading
// — lower case with everything but letters and digits removed — so "Gross
// Sales", "gross_sales" and "GROSS SALES (VAT incl.)" all reach the same
// column.
type ColumnSpec struct {
	// Name is the canonical name the caller will use with Row.Get.
	Name string
	// Aliases are the headings that mean this column. The canonical name is
	// always accepted as well.
	Aliases []string
	// Required means the file is unusable without this column.
	Required bool
}

// ReadTable reads a CSV into a Table using the given column specifications.
//
// The returned error is only ever about the file as a whole — it could not be
// read at all, or a required column is missing. Everything that is wrong with
// an individual row is left for the caller to find, so that one bad row does
// not hide the other ninety-nine.
func ReadTable(r io.Reader, specs []ColumnSpec) (*Table, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxCSVBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCSV, err)
	}
	if len(raw) > maxCSVBytes {
		return nil, fmt.Errorf("%w: the file is larger than %d megabytes", ErrCSV, maxCSVBytes>>20)
	}
	text := string(raw)
	text = strings.TrimPrefix(text, "\ufeff")     // UTF-8 byte-order mark
	text = strings.ReplaceAll(text, "\r\n", "\n") // Windows line endings
	text = strings.ReplaceAll(text, "\r", "\n")   // classic Mac line endings
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: the file is empty", ErrCSV)
	}

	sep := sniffSeparator(text)
	records, lines, err := readRecords(text, sep)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: the file has no rows", ErrCSV)
	}

	hdrIdx, matched := findHeader(records, specs)
	if hdrIdx < 0 {
		return nil, fmt.Errorf("%w: no header row was found in the first %d lines. Expected column headings such as %s",
			ErrCSV, min(maxHeaderSearchRows, len(records)), exampleHeadings(specs))
	}

	tbl := &Table{
		HeaderLine: lines[hdrIdx],
		Separator:  sep,
		Columns:    make(map[string]string, len(specs)),
	}
	if hdrIdx > 0 {
		tbl.Notes = append(tbl.Notes, fmt.Sprintf("%d line(s) above the headings were skipped.", hdrIdx))
	}

	header := records[hdrIdx]
	// byIndex maps a column position to the canonical name it carries.
	byIndex := make(map[int]string, len(header))
	for i, h := range header {
		key := reduceHeading(h)
		if key == "" {
			continue
		}
		if name, ok := matched[key]; ok {
			if _, taken := tbl.Columns[name]; taken {
				tbl.Notes = append(tbl.Notes, fmt.Sprintf("The column %q repeats a heading already used for %s; the first one was used.", strings.TrimSpace(h), name))
				continue
			}
			byIndex[i] = name
			tbl.Columns[name] = strings.TrimSpace(h)
			continue
		}
		tbl.Unmatched = append(tbl.Unmatched, strings.TrimSpace(h))
	}

	var missing []string
	for _, s := range specs {
		if s.Required {
			if _, ok := tbl.Columns[s.Name]; !ok {
				missing = append(missing, s.Name)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: the heading row on line %d has no column for %s. The headings found were: %s",
			ErrCSV, tbl.HeaderLine, strings.Join(missing, ", "), strings.Join(headingList(header), ", "))
	}
	if len(tbl.Unmatched) > 0 {
		tbl.Notes = append(tbl.Notes, fmt.Sprintf("These columns were not recognised and were ignored: %s.", strings.Join(tbl.Unmatched, ", ")))
	}

	blank, extra := 0, 0
	for i := hdrIdx + 1; i < len(records); i++ {
		rec := records[i]
		row := Row{Line: lines[i], cells: make(map[string]string, len(byIndex))}
		for idx, name := range byIndex {
			if idx < len(rec) {
				row.cells[name] = strings.TrimSpace(strings.Map(dropZeroWidth, rec[idx]))
			} else {
				row.cells[name] = "" // a short row: the missing cells are empty
			}
		}
		if row.IsBlank() {
			blank++
			continue
		}
		if len(rec) > len(header) {
			nonEmpty := false
			for _, c := range rec[len(header):] {
				if strings.TrimSpace(c) != "" {
					nonEmpty = true
					break
				}
			}
			if nonEmpty {
				extra++
			}
		}
		tbl.Rows = append(tbl.Rows, row)
	}
	if blank > 0 {
		tbl.Notes = append(tbl.Notes, fmt.Sprintf("%d blank row(s) were skipped.", blank))
	}
	if extra > 0 {
		tbl.Notes = append(tbl.Notes, fmt.Sprintf("%d row(s) had cells past the last heading; those cells were ignored.", extra))
	}
	if len(tbl.Rows) == 0 {
		return nil, fmt.Errorf("%w: the heading row on line %d is followed by no data", ErrCSV, tbl.HeaderLine)
	}
	return tbl, nil
}

// readRecords parses the text into records, keeping the line number each
// record started on. A quoting error is reported against its line rather than
// abandoning the whole file.
func readRecords(text string, sep rune) (records [][]string, lines []int, err error) {
	cr := csv.NewReader(strings.NewReader(text))
	cr.Comma = sep
	cr.FieldsPerRecord = -1 // ragged rows are the caller's problem, not an error
	cr.LazyQuotes = true    // a stray quote inside a field must not stop the read
	cr.TrimLeadingSpace = true
	cr.ReuseRecord = false

	for {
		rec, rerr := cr.Read()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			var pe *csv.ParseError
			if errors.As(rerr, &pe) {
				return nil, nil, fmt.Errorf("%w: line %d could not be parsed: %v", ErrCSV, pe.Line, pe.Err)
			}
			return nil, nil, fmt.Errorf("%w: %v", ErrCSV, rerr)
		}
		// FieldPos reports the line the record's first field began on, which
		// is the line number the user sees in a text editor. It may only be
		// asked after a successful Read, and only for a field that exists.
		start := len(lines) + 1
		if len(rec) > 0 {
			if line, _ := cr.FieldPos(0); line > 0 {
				start = line
			}
		}
		records = append(records, rec)
		lines = append(lines, start)
	}
	return records, lines, nil
}

// sniffSeparator decides whether the file is separated by commas, semicolons,
// tabs or pipes, by counting which one appears most consistently across the
// first few lines outside quoted fields.
func sniffSeparator(text string) rune {
	candidates := []rune{',', ';', '\t', '|'}
	best, bestScore := ',', -1
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var sample []string
	for sc.Scan() && len(sample) < 10 {
		if strings.TrimSpace(sc.Text()) != "" {
			sample = append(sample, sc.Text())
		}
	}
	for _, c := range candidates {
		counts := make([]int, 0, len(sample))
		for _, line := range sample {
			counts = append(counts, countOutsideQuotes(line, c))
		}
		score := 0
		first := 0
		if len(counts) > 0 {
			first = counts[0]
		}
		if first == 0 {
			continue
		}
		for _, n := range counts {
			if n == first {
				score += first
			}
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

func countOutsideQuotes(line string, sep rune) int {
	n, inQuote := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == sep && !inQuote:
			n++
		}
	}
	return n
}

// findHeader looks for the first row that matches at least one required
// column, or two optional ones, and returns it with the alias table used to
// match it.
func findHeader(records [][]string, specs []ColumnSpec) (int, map[string]string) {
	aliases := buildAliases(specs)
	required := map[string]bool{}
	for _, s := range specs {
		if s.Required {
			required[s.Name] = true
		}
	}
	limit := min(maxHeaderSearchRows, len(records))
	bestIdx, bestScore := -1, 0
	for i := 0; i < limit; i++ {
		reqHits, hits := 0, 0
		seen := map[string]bool{}
		for _, h := range records[i] {
			name, ok := aliases[reduceHeading(h)]
			if !ok || seen[name] {
				continue
			}
			seen[name] = true
			hits++
			if required[name] {
				reqHits++
			}
		}
		// Weight required columns heavily: a row that names them is the header
		// even if a title row above it happens to share a word.
		score := hits + 10*reqHits
		if (reqHits == len(required) && len(required) > 0) || hits >= 2 {
			if score > bestScore {
				bestIdx, bestScore = i, score
			}
			if reqHits == len(required) && len(required) > 0 {
				// A row carrying every required column is the header; stop at
				// the first such row so a repeated heading block lower down
				// does not win.
				return i, aliases
			}
		}
	}
	return bestIdx, aliases
}

func buildAliases(specs []ColumnSpec) map[string]string {
	out := make(map[string]string, len(specs)*4)
	for _, s := range specs {
		if k := reduceHeading(s.Name); k != "" {
			out[k] = s.Name
		}
		for _, a := range s.Aliases {
			if k := reduceHeading(a); k != "" {
				if _, taken := out[k]; !taken {
					out[k] = s.Name
				}
			}
		}
	}
	return out
}

// reduceHeading turns a heading into the form aliases are matched on: lower
// case, with everything that is not a letter or a digit removed. So "Gross
// Sales (VAT incl.)" becomes "grosssalesvatincl".
func reduceHeading(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dropZeroWidth removes the invisible characters a copy-and-paste out of a web
// page leaves behind, which would otherwise make a value that looks right fail
// validation.
func dropZeroWidth(r rune) rune {
	switch r {
	case '\ufeff', '\u200b', '\u200c', '\u200d', '\u2060':
		return -1
	}
	return r
}

func headingList(header []string) []string {
	out := make([]string, 0, len(header))
	for _, h := range header {
		if s := strings.TrimSpace(h); s != "" {
			out = append(out, strconv.Quote(s))
		}
	}
	if len(out) == 0 {
		return []string{"(none)"}
	}
	return out
}

func exampleHeadings(specs []ColumnSpec) string {
	var out []string
	for _, s := range specs {
		if s.Required {
			out = append(out, strconv.Quote(s.Name))
		}
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 {
		for _, s := range specs {
			out = append(out, strconv.Quote(s.Name))
			if len(out) == 3 {
				break
			}
		}
	}
	return strings.Join(out, ", ")
}
