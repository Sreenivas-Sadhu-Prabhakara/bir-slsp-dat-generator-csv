package slsp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTIN(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantBase    string
		wantBranch  string
		branchGiven bool
	}{
		{"nine bare digits", "123456789", "123456789", "000", false},
		{"nine digits dashed", "123-456-789", "123456789", "000", false},
		{"dashed head office", "123-456-789-000", "123456789", "000", true},
		{"run together with branch", "123456789000", "123456789", "000", true},
		{"spaces instead of dashes", "123 456 789 000", "123456789", "000", true},
		{"five digit branch, head office", "123-456-789-00000", "123456789", "00000", true},
		{"branch 001", "123-456-789-001", "123456789", "001", true},
		{"branch 00002 widened", "123-456-789-00002", "123456789", "00002", true},
		{"branch 999", "005-123-456-999", "005123456", "999", true},
		{"leading zeros in base", "004-123-456", "004123456", "000", false},
		{"surrounding whitespace", "  123-456-789-000  ", "123456789", "000", true},
		{"en dash separators", "123–456–789", "123456789", "000", false},
		{"slashes as separators", "123/456/789", "123456789", "000", false},
		{"dots as separators", "123.456.789", "123456789", "000", false},
		{"no-break spaces", "123 456 789", "123456789", "000", false},
		{"mixed separators", "123-456 789/000", "123456789", "000", true},
		{"a branch code that would not fit three digits", "123-456-789-10007", "123456789", "10007", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTIN(c.in)
			if err != nil {
				t.Fatalf("ParseTIN(%q) returned error %v", c.in, err)
			}
			if got.Base != c.wantBase {
				t.Errorf("ParseTIN(%q).Base = %q, want %q", c.in, got.Base, c.wantBase)
			}
			if got.Branch != c.wantBranch {
				t.Errorf("ParseTIN(%q).Branch = %q, want %q", c.in, got.Branch, c.wantBranch)
			}
			if got.BranchGiven != c.branchGiven {
				t.Errorf("ParseTIN(%q).BranchGiven = %v, want %v", c.in, got.BranchGiven, c.branchGiven)
			}
			if len(got.Base) != TINBaseDigits {
				t.Errorf("ParseTIN(%q).Base has %d digits, want %d", c.in, len(got.Base), TINBaseDigits)
			}
			if !got.Valid() {
				t.Errorf("ParseTIN(%q) produced a TIN that reports itself invalid", c.in)
			}
			// Whatever the branch code, only the nine digits reach the file.
			if got.Digits() != c.wantBase {
				t.Errorf("ParseTIN(%q).Digits() = %q, want %q", c.in, got.Digits(), c.wantBase)
			}
		})
	}
}

func TestParseTINRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"blank", ""},
		{"spaces only", "   "},
		{"too few digits", "12345678"},
		{"ten digits", "1234567890"},
		{"eleven digits", "12345678901"},
		{"thirteen digits", "1234567890123"},
		{"fifteen digits", "123456789000000"},
		{"letters", "ABC-456-789-000"},
		{"letters at the end", "123-456-789-00A"},
		{"a name instead of a number", "Not Applicable"},
		{"a run of zeros", "000000000"},
		{"a run of zeros with a branch", "000-000-000-000"},
		{"a run of ones", "111111111"},
		{"a run of threes", "333333333"},
		{"a run of nines", "999999999"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTIN(c.in)
			if err == nil {
				t.Fatalf("ParseTIN(%q) = %v, want an error", c.in, got)
			}
			if !errors.Is(err, ErrTIN) {
				t.Errorf("ParseTIN(%q) error %v does not wrap ErrTIN", c.in, err)
			}
		})
	}
}

func TestParseTINKeepsTheOneRunOfDigitsTheModuleAllows(t *testing.T) {
	// The validation module's list of refused numbers leaves out a run of
	// twos, so refusing it here would reject a file the Bureau would accept.
	// It earns a warning instead.
	got, err := ParseTIN("222222222")
	if err != nil {
		t.Fatalf("ParseTIN of a run of twos failed: %v; the validation module allows it", err)
	}
	if !got.Suspicious() {
		t.Error("a run of twos was not flagged as suspicious")
	}
	ordinary, err := ParseTIN("123456789")
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Suspicious() {
		t.Error("an ordinary TIN was flagged as suspicious")
	}
}

func TestTINRendering(t *testing.T) {
	tin, err := ParseTIN("123456789001")
	if err != nil {
		t.Fatalf("ParseTIN: %v", err)
	}
	if got := tin.Digits(); got != "123456789" {
		t.Errorf("Digits() = %q, want %q", got, "123456789")
	}
	if got := tin.Dashed(); got != "123-456-789-001" {
		t.Errorf("Dashed() = %q, want %q", got, "123-456-789-001")
	}
	if got := tin.String(); got != "123-456-789-001" {
		t.Errorf("String() = %q, want %q", got, "123-456-789-001")
	}
	if tin.IsHeadOffice() {
		t.Error("branch 001 reported itself as the head office")
	}

	ho, err := ParseTIN("123-456-789")
	if err != nil {
		t.Fatalf("ParseTIN: %v", err)
	}
	if !ho.IsHeadOffice() {
		t.Error("a TIN with no branch code did not report itself as the head office")
	}
	if got := ho.Dashed(); got != "123-456-789-000" {
		t.Errorf("Dashed() = %q, want %q", got, "123-456-789-000")
	}
}

func TestZeroTINIsSafeAndInvalid(t *testing.T) {
	// The zero value must not panic or produce a half-formed number, and must
	// not pass as a usable TIN.
	var z TIN
	if z.Valid() {
		t.Error("the zero TIN reported itself valid")
	}
	if got := z.Dashed(); got != "" {
		t.Errorf("the zero TIN rendered as %q, want the empty string", got)
	}
	if got := z.Digits(); got != "" {
		t.Errorf("the zero TIN's digits were %q, want the empty string", got)
	}
	if z.Suspicious() {
		t.Error("the zero TIN reported itself suspicious")
	}
}

func TestTINValidRejectsNonDigits(t *testing.T) {
	// Valid is the gate the writer relies on, so it must not trust that Base
	// was built by ParseTIN.
	for _, base := range []string{"12345678A", "12345678 ", "", "1234567890", "000000000"} {
		if (TIN{Base: base, Branch: "000"}).Valid() {
			t.Errorf("a TIN with base %q reported itself valid", base)
		}
	}
	if !(TIN{Base: "004123456", Branch: "000"}).Valid() {
		t.Error("a well-formed TIN reported itself invalid")
	}
}

func TestBranchCodeIsNeverWrittenIntoTheFile(t *testing.T) {
	// The whole file has no field for a branch code. Whatever the CSV says,
	// the nine digits are what reaches every TIN field.
	for _, in := range []string{"004-123-456", "004-123-456-000", "004-123-456-005", "004-123-456-00123"} {
		tin, err := ParseTIN(in)
		if err != nil {
			t.Fatalf("ParseTIN(%q): %v", in, err)
		}
		if got := tin.Digits(); got != "004123456" {
			t.Errorf("ParseTIN(%q).Digits() = %q, want the nine digits only", in, got)
		}
		if strings.Contains(tin.Digits(), "-") || len(tin.Digits()) != TINBaseDigits {
			t.Errorf("ParseTIN(%q).Digits() = %q, which is not nine bare digits", in, tin.Digits())
		}
	}
}
