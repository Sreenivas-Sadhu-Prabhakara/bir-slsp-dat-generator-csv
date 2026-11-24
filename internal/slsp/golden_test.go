package slsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden test is the one that matters most in this program: it pins the
// exact bytes of a generated file. Every other test checks a rule; this one
// checks the result. If a change to any rule changes what reaches the file,
// this test says so in bytes, which is the only form the Bureau's validation
// module cares about.
//
// Run with UPDATE_GOLDEN=1 to rewrite the expected files after a deliberate
// change to the layout, and then read the diff before committing it.

// goldenFiler is the taxpayer the golden files are filed for. The values are
// invented; the TIN is one that passes the format's own checks.
func goldenFiler(t *testing.T) Filer {
	t.Helper()
	tin, err := ParseTIN("004-123-456-000")
	if err != nil {
		t.Fatalf("ParseTIN: %v", err)
	}
	return Filer{
		TIN:                tin,
		RegisteredName:     "ABC Trading Corporation",
		TradeName:          "ABC Trading",
		Address:            [AddressLines]string{"123 Main St Brgy San Antonio", "Makati City 1203"},
		RDOCode:            "044",
		FiscalYearEndMonth: 12,
	}
}

func goldenOptions(t *testing.T, k ListKind) Options {
	t.Helper()
	p, err := ParsePeriod("10/2026")
	if err != nil {
		t.Fatalf("ParsePeriod: %v", err)
	}
	o := DefaultOptions()
	o.List = k
	o.Period = p
	o.Filer = goldenFiler(t)
	if k == Purchases {
		// Some of the month's input tax is not creditable, so the header's
		// creditable and non-creditable split is exercised too.
		o.NonCreditableInputVAT = 60000 // PHP 600.00
	}
	return o
}

func runGolden(t *testing.T, k ListKind) *Result {
	t.Helper()
	in, err := os.ReadFile(filepath.Join("testdata", k.String()+"-input.csv"))
	if err != nil {
		t.Fatalf("reading the input fixture: %v", err)
	}
	res, err := Generate(bytes.NewReader(in), goldenOptions(t, k))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !res.OK {
		for _, f := range res.Findings {
			t.Logf("finding: %s", f)
		}
		t.Fatalf("the golden input was refused: %d error(s)", res.FatalCount())
	}
	return res
}

func checkGoldenFile(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		t.Logf("updated %s (%d bytes)", path, len(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v (run the test with UPDATE_GOLDEN=1 to create it)", path, err)
	}
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("%s does not match, byte for byte.\n--- want (%d bytes) ---\n%s\n--- got (%d bytes) ---\n%s\n%s",
		name, len(want), showBytes(want), len(got), showBytes(got), firstDifference(want, got))
}

// showBytes renders a file so that a difference in line endings or in a
// single-byte accented character is visible in the failure message.
func showBytes(b []byte) string {
	var out strings.Builder
	for _, line := range bytes.Split(b, []byte("\r\n")) {
		if len(line) == 0 {
			continue
		}
		for _, c := range line {
			if c >= 0x20 && c < 0x7f {
				out.WriteByte(c)
				continue
			}
			out.WriteString("<")
			out.WriteString(strings.ToUpper(hexByte(c)))
			out.WriteString(">")
		}
		out.WriteString("<CRLF>\n")
	}
	return out.String()
}

func hexByte(c byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[c>>4], digits[c&0x0f]})
}

func firstDifference(want, got []byte) string {
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			return "first difference at byte " + itoa(i) + ": want 0x" + hexByte(want[i]) + ", got 0x" + hexByte(got[i])
		}
	}
	if len(want) != len(got) {
		return "the files agree for " + itoa(n) + " bytes and then differ in length"
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestGoldenSalesFile(t *testing.T) {
	res := runGolden(t, Sales)
	if res.FileName != "004123456S102026.DAT" {
		t.Errorf("FileName = %q, want %q", res.FileName, "004123456S102026.DAT")
	}
	checkGoldenFile(t, "004123456S102026.DAT", res.DatBytes())
	checkGoldenFile(t, "004123456S102026.submission.DAT", res.SubmissionBytes())
}

func TestGoldenPurchasesFile(t *testing.T) {
	res := runGolden(t, Purchases)
	if res.FileName != "004123456P102026.DAT" {
		t.Errorf("FileName = %q, want %q", res.FileName, "004123456P102026.DAT")
	}
	checkGoldenFile(t, "004123456P102026.DAT", res.DatBytes())
	checkGoldenFile(t, "004123456P102026.submission.DAT", res.SubmissionBytes())
}

// TestGoldenFilesHaveTheShapeTheFormatRequires checks the golden files against
// the format's rules directly, rather than against the code that wrote them.
// Without this, the golden files would only prove the program is consistent
// with itself.
func TestGoldenFilesHaveTheShapeTheFormatRequires(t *testing.T) {
	for _, name := range []string{"004123456S102026.DAT", "004123456P102026.DAT"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			// Every record ends with CR LF, including the last.
			if !bytes.HasSuffix(raw, []byte("\r\n")) {
				t.Error("the file does not end with CR LF")
			}
			if bytes.Contains(bytes.ReplaceAll(raw, []byte("\r\n"), nil), []byte("\n")) {
				t.Error("the file contains a line feed that is not part of a CR LF pair")
			}
			lines := strings.Split(strings.TrimSuffix(string(raw), "\r\n"), "\r\n")

			// Exactly one header, first, and at least one detail record.
			headers := 0
			for i, line := range lines {
				switch {
				case strings.HasPrefix(line, RecordTypeHeader+","):
					headers++
					if i != 0 {
						t.Errorf("line %d is a header record but the header must come first", i+1)
					}
				case strings.HasPrefix(line, RecordTypeDetail+","):
				default:
					t.Errorf("line %d starts with neither %q nor %q: %q", i+1, RecordTypeHeader, RecordTypeDetail, line)
				}
				if n := len(line); n > MaxRecordChars {
					t.Errorf("line %d is %d characters, over the %d the submission routine reads", i+1, n, MaxRecordChars)
				}
			}
			if headers != 1 {
				t.Errorf("the file has %d header records, want exactly 1", headers)
			}
			if len(lines) < 2 {
				t.Error("the file has no detail records")
			}

			// Every record has the number of fields its layout says, every
			// byte is one the validation module accepts, and the tax month is
			// the last day of the month on every record.
			k := Sales
			if strings.Contains(name, "P1") {
				k = Purchases
			}
			spec := Layout(k)
			for i, line := range lines {
				rec := spec.Detail
				if i == 0 {
					rec = spec.Header
				}
				fields := splitDatLine(line)
				if len(fields) != rec.Width() {
					t.Errorf("line %d has %d fields, but the %s layout has %d", i+1, len(fields), rec.What, rec.Width())
					continue
				}
				if got := fields[1]; got != k.TransactionCode() {
					t.Errorf("line %d has transaction code %q, want %q", i+1, got, k.TransactionCode())
				}
				last := fields[len(fields)-1]
				if i == 0 {
					last = fields[len(fields)-2] // the header ends with the month-end
				}
				if last != "10/31/2026" {
					t.Errorf("line %d carries the tax month %q, want the last day of the month, 10/31/2026", i+1, last)
				}
			}
			for i, b := range raw {
				if b == '\r' || b == '\n' || b == FieldSeparator || b == TextDelimiter {
					continue
				}
				if !allowedByte(b) {
					t.Errorf("byte %d of the file is 0x%s, which the validation module refuses", i, hexByte(b))
				}
			}
		})
	}
}

// splitDatLine splits a record into its fields, respecting the text
// delimiters, and strips them. It is a reader written for the test alone, so
// that the test does not use the writer's own code to check the writer.
func splitDatLine(line string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == TextDelimiter:
			inQuote = !inQuote
		case c == FieldSeparator && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	out = append(out, cur.String())
	return out
}

// TestGoldenSubmissionCopyIsTheFilePlusOneKeyLine checks the relationship
// between the two files rather than their contents.
func TestGoldenSubmissionCopyIsTheFilePlusOneKeyLine(t *testing.T) {
	for _, k := range []ListKind{Sales, Purchases} {
		res := runGolden(t, k)
		if !strings.HasPrefix(res.Submission, res.Dat) {
			t.Errorf("%s: the submission copy does not begin with the data file", k)
		}
		extra := strings.TrimSuffix(strings.TrimPrefix(res.Submission, res.Dat), RecordSeparator)
		if extra != res.ValidationKey {
			t.Errorf("%s: the submission copy adds %q, want the key %q", k, extra, res.ValidationKey)
		}
		if len(res.ValidationKey) != ValidationKeyLength {
			t.Errorf("%s: the key %q is %d characters, want %d", k, res.ValidationKey, len(res.ValidationKey), ValidationKeyLength)
		}
		if res.ValidationKey[0] != 'V' {
			t.Errorf("%s: the key %q does not begin with V", k, res.ValidationKey)
		}
	}
}
