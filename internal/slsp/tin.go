package slsp

import (
	"errors"
	"fmt"
	"strings"
)

// A Taxpayer Identification Number issued in the Philippines is a nine-digit
// base number, usually printed with a branch code after it: 123-456-789-000.
//
// The data file carries only the nine digits. It has no field for a branch
// code anywhere — not in the header, not in a detail record, not in the file
// name — so a branch code given in the CSV is kept here for display and for
// telling two rows apart, and is reported as not written rather than being
// dropped in silence.

const (
	// TINBaseDigits is the length of the base TIN, and the exact length the
	// data file requires of every TIN field.
	TINBaseDigits = 9

	// TINBranchWidth and TINBranchWidthWide are the two widths a branch code
	// is written in: three digits on the older registration certificates and
	// five on the current ones. Neither is written into the data file.
	TINBranchWidth     = 3
	TINBranchWidthWide = 5

	// TINHeadOfficeBranch is the branch code of a head office.
	TINHeadOfficeBranch = "000"
)

// ErrTIN is the base of every TIN failure.
var ErrTIN = errors.New("not a Taxpayer Identification Number")

// blockedTINs are the numbers the validation module refuses outright: a
// nine-digit run of one digit. Writing one produces a file that is rejected,
// so they are refused here too.
//
// The module's list leaves out a run of twos, so a run of twos is only
// remarked on. See softBlockedTIN.
var blockedTINs = map[string]bool{
	"000000000": true,
	"111111111": true,
	"333333333": true,
	"444444444": true,
	"555555555": true,
	"666666666": true,
	"777777777": true,
	"888888888": true,
	"999999999": true,
}

// softBlockedTIN is the one run of a single digit the validation module lets
// through. It is almost certainly a placeholder somebody typed, so it earns a
// warning rather than a refusal: refusing it would reject a file the Bureau
// would have accepted.
const softBlockedTIN = "222222222"

// TIN is a parsed Philippine TIN.
type TIN struct {
	// Base is exactly nine digits, no separators. This is what the data file
	// carries.
	Base string `json:"base"`
	// Branch is the branch code as given, or "000" when none was given. It is
	// never written into the data file.
	Branch string `json:"branch"`
	// BranchGiven records whether the CSV actually supplied a branch code, so
	// that the user can be told it is not part of this format.
	BranchGiven bool `json:"branchGiven"`
}

// ParseTIN reads a TIN out of a CSV cell.
//
// It accepts the shapes a bookkeeper or a spreadsheet actually produces:
//
//	"123-456-789"        nine digits, dashed
//	"123456789"          nine digits, run together
//	"123-456-789-000"    with a three-digit branch code
//	"123-456-789-00000"  with a five-digit branch code
//	"123 456 789 000"    spaces instead of dashes
//
// The nine digits are what the file gets. A number of any other length, or
// one carrying anything but digits and separators, is an error that quotes the
// offending text.
func ParseTIN(s string) (TIN, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return TIN{}, fmt.Errorf("the TIN is blank: %w", ErrTIN)
	}
	digits := make([]byte, 0, 16)
	for _, c := range raw {
		switch {
		case c >= '0' && c <= '9':
			digits = append(digits, byte(c))
		case c == '-' || c == ' ' || c == '–' || c == '—' || c == '/' || c == '.' || c == '\t' || c == ' ':
			// a separator; ignore it
		default:
			return TIN{}, fmt.Errorf("%q contains %q, which is not a digit: %w", raw, string(c), ErrTIN)
		}
		if len(digits) > TINBaseDigits+TINBranchWidthWide {
			return TIN{}, fmt.Errorf("%q has more than %d digits: %w", raw, TINBaseDigits+TINBranchWidthWide, ErrTIN)
		}
	}

	t := TIN{Branch: TINHeadOfficeBranch}
	switch len(digits) {
	case TINBaseDigits:
		t.Base = string(digits)
	case TINBaseDigits + TINBranchWidth, TINBaseDigits + TINBranchWidthWide:
		t.Base = string(digits[:TINBaseDigits])
		t.Branch = string(digits[TINBaseDigits:])
		t.BranchGiven = true
	default:
		return TIN{}, fmt.Errorf("%q has %d digits; the data file needs exactly %d, with or without a %d- or %d-digit branch code after them: %w",
			raw, len(digits), TINBaseDigits, TINBranchWidth, TINBranchWidthWide, ErrTIN)
	}
	if blockedTINs[t.Base] {
		return TIN{}, fmt.Errorf("%q is a run of one digit, which the validation module refuses: %w", raw, ErrTIN)
	}
	return t, nil
}

// Suspicious reports whether the number is one the validation module happens
// to allow but that is almost certainly a placeholder.
func (t TIN) Suspicious() bool { return t.Base == softBlockedTIN }

// Digits returns the nine digits the data file carries.
func (t TIN) Digits() string { return t.Base }

// Valid reports whether this is a TIN the data file can carry.
func (t TIN) Valid() bool {
	if len(t.Base) != TINBaseDigits || blockedTINs[t.Base] {
		return false
	}
	for i := 0; i < len(t.Base); i++ {
		if t.Base[i] < '0' || t.Base[i] > '9' {
			return false
		}
	}
	return true
}

// Dashed returns the number the way it is printed on a registration
// certificate: 123-456-789-000.
func (t TIN) Dashed() string {
	if len(t.Base) != TINBaseDigits {
		return ""
	}
	return t.Base[0:3] + "-" + t.Base[3:6] + "-" + t.Base[6:9] + "-" + t.Branch
}

// IsHeadOffice reports whether the branch code given was the head office's.
func (t TIN) IsHeadOffice() bool { return t.Branch == TINHeadOfficeBranch }

// String renders the TIN in its dashed form, for messages.
func (t TIN) String() string { return t.Dashed() }
