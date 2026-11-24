package slsp

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// A summary list names each customer or supplier in four fields: a registered
// name for a company, and a last, first and middle name for an individual. The
// Bureau's module carries all four on every record and requires at least one
// of them to have a value; it does not have two record shapes. So a company
// fills in the registered name and leaves the three personal fields empty, and
// an individual does the opposite.
//
// Every text field is upper-cased, because the Bureau's own data entry screens
// upper-case what is typed into them, and a file that disagrees with the
// registration record in case alone invites a mismatch.

// ErrName is the base of every name failure.
var ErrName = errors.New("the name cannot be used")

// TaxpayerKind is whether a customer or supplier is an individual or not.
type TaxpayerKind int

const (
	// KindUnknown means the CSV did not say.
	KindUnknown TaxpayerKind = iota
	// Individual is a natural person: a sole proprietor, a professional.
	Individual
	// NonIndividual is a corporation, partnership, cooperative, association
	// or any other juridical person.
	NonIndividual
)

func (k TaxpayerKind) String() string {
	switch k {
	case Individual:
		return "individual"
	case NonIndividual:
		return "non-individual"
	}
	return "unknown"
}

// ParseTaxpayerKind reads the kind out of a CSV cell. It accepts the single
// letters the Bureau's own forms use as well as the words a bookkeeper writes.
func ParseTaxpayerKind(s string) (TaxpayerKind, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.Trim(t, ".")
	switch t {
	case "":
		return KindUnknown, nil
	case "i", "ind", "indiv", "individual", "person", "natural", "sole proprietor", "soleproprietor", "sp", "single proprietor":
		return Individual, nil
	case "n", "c", "corp", "corporation", "non-individual", "nonindividual", "non individual", "juridical",
		"company", "partnership", "cooperative", "coop", "association", "government", "inc", "incorporated":
		return NonIndividual, nil
	}
	return KindUnknown, fmt.Errorf("%q does not say whether the taxpayer is an individual or not; write I or N: %w", s, ErrName)
}

// PersonName is an individual's name in the three parts the data file holds.
type PersonName struct {
	Last   string `json:"last,omitempty"`
	First  string `json:"first,omitempty"`
	Middle string `json:"middle,omitempty"`
}

// Empty reports whether no part of the name is filled in.
func (n PersonName) Empty() bool { return n.Last == "" && n.First == "" && n.Middle == "" }

// String renders the name for a person to read.
func (n PersonName) String() string {
	parts := make([]string, 0, 3)
	if n.Last != "" {
		parts = append(parts, n.Last+",")
	}
	if n.First != "" {
		parts = append(parts, n.First)
	}
	if n.Middle != "" {
		parts = append(parts, n.Middle)
	}
	return strings.Join(parts, " ")
}

// SplitPersonName reads an individual's name out of a single CSV cell.
//
// Only the comma form is accepted, because it is the only one that says
// unambiguously where the surname ends:
//
//	"DELA CRUZ, JUAN P."        -> last "DELA CRUZ", first "JUAN", middle "P."
//	"Santos, Maria Clara Reyes" -> last "SANTOS", first "MARIA CLARA", middle "REYES"
//	"Lim, Andrew"               -> last "LIM", first "ANDREW", no middle name
//
// A name without a comma is refused by name rather than guessed at, because
// Filipino surnames are frequently two or three words — "DELA CRUZ", "DE LOS
// SANTOS" — and taking the last word as the surname would file the return
// against the wrong person. The caller is told to use separate columns or to
// add the comma.
func SplitPersonName(s string) (PersonName, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return PersonName{}, fmt.Errorf("the name is blank: %w", ErrName)
	}
	last, rest, found := strings.Cut(raw, ",")
	if !found {
		return PersonName{}, fmt.Errorf("%q does not say where the surname ends. Write it as \"SURNAME, First Middle\", or give the last, first and middle names in separate columns: %w", raw, ErrName)
	}
	last = CleanName(last)
	if last == "" {
		return PersonName{}, fmt.Errorf("%q has nothing before the comma, so the surname is missing: %w", raw, ErrName)
	}
	words := strings.Fields(CleanName(rest))
	switch len(words) {
	case 0:
		return PersonName{}, fmt.Errorf("%q has nothing after the comma, so the first name is missing: %w", raw, ErrName)
	case 1:
		return PersonName{Last: last, First: words[0]}, nil
	default:
		// The last word is the middle name; everything before it is the first
		// name, so a compound first name stays together.
		return PersonName{
			Last:   last,
			First:  strings.Join(words[:len(words)-1], " "),
			Middle: words[len(words)-1],
		}, nil
	}
}

// CleanText puts a value into the form the data file wants and reports every
// substitution it had to make, so that nothing is changed behind the user's
// back.
//
// It upper-cases, collapses runs of whitespace to one space, folds accented
// letters to their unaccented form — except the two the validation module
// accepts, Ñ and ñ — and substitutes or removes every character the module
// refuses. The ampersand becomes the word AND, because a company called
// "SMITH & SONS" should stay recognisable; the apostrophe and the rest are
// removed.
func CleanText(s string) (string, []string) {
	var changes []string
	seen := map[string]bool{}
	note := func(what string) {
		if !seen[what] {
			seen[what] = true
			changes = append(changes, what)
		}
	}

	s = strings.Map(dropZeroWidth, s)
	s = foldToASCII(s, note)

	var b strings.Builder
	b.Grow(len(s))
	lastWasSpace := true
	writeRune := func(r rune) {
		b.WriteRune(r)
		lastWasSpace = false
	}
	writeSpace := func() {
		if !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}

	for _, r := range strings.ToUpper(s) {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ' ':
			writeSpace()
			continue
		case r == 'Ñ':
			writeRune(r) // the one accented letter the module keeps
			continue
		}
		if r < 0x80 {
			if allowedByte(byte(r)) {
				writeRune(r)
				continue
			}
			sub, ok := forbiddenASCII[byte(r)]
			if ok {
				note(fmt.Sprintf("%q was %s because it is %s", string(r), replacedWith(sub.with), sub.why))
				if strings.TrimSpace(sub.with) != "" {
					writeSpace()
					for _, c := range strings.TrimSpace(sub.with) {
						writeRune(c)
					}
					writeSpace()
					continue
				}
				continue
			}
			note(fmt.Sprintf("%q was removed because the data file cannot carry it", string(r)))
			continue
		}
		note(fmt.Sprintf("%q was removed because the data file cannot carry it", string(r)))
	}
	sort.Strings(changes)
	return strings.TrimSpace(b.String()), changes
}

func replacedWith(with string) string {
	if strings.TrimSpace(with) == "" {
		return "removed"
	}
	return "replaced with " + strings.TrimSpace(with)
}

// CleanName is CleanText for callers that do not need the list of changes.
func CleanName(s string) string {
	out, _ := CleanText(s)
	return out
}

// foldToASCII replaces the accented letters that turn up in Philippine and
// Spanish names with their unaccented equivalents, and the typographic
// punctuation a word processor substitutes with the plain character. Ñ and ñ
// are left alone, because the data file carries them.
func foldToASCII(s string, note func(string)) string {
	if isASCII(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < unicode.MaxASCII {
			b.WriteRune(r)
			continue
		}
		if r == 'Ñ' || r == 'ñ' {
			b.WriteRune(r)
			continue
		}
		if rep, ok := asciiFolds[r]; ok {
			note(fmt.Sprintf("%q was written as %q, because the data file is a single-byte file", string(r), rep))
			b.WriteString(rep)
			continue
		}
		// An unknown character becomes a space, so the words either side of
		// it do not run together.
		note(fmt.Sprintf("%q was removed because the data file cannot carry it", string(r)))
		b.WriteByte(' ')
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= unicode.MaxASCII {
			return false
		}
	}
	return true
}

var asciiFolds = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
	'Ç': "C", 'ç': "c",
	'Ý': "Y", 'ý': "y", 'ÿ': "y",
	'Æ': "AE", 'æ': "ae",
	'Ø': "O", 'ø': "o",
	'‘': "", '’': "", '‚': "", '‛': "",
	'“': "", '”': "",
	'–': "-", '—': "-", '−': "-", '‐': "-", '‑': "-",
	'…': "...",
	'·': ".", '•': ".",
	'º': ".", 'ª': ".",
	'©': "(C)", '®': "(R)",
	'₱': "PHP",
	' ': " ",
}

// Truncate shortens a value to a maximum number of characters at a word
// boundary where it can, and reports whether it had to cut anything. The
// caller turns that into a note so the user knows a name was shortened rather
// than discovering it in a rejected file.
func Truncate(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > max*2/3 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " .,-"), true
}
