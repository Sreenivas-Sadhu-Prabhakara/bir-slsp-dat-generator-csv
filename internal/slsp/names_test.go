package slsp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTaxpayerKind(t *testing.T) {
	cases := []struct {
		in   string
		want TaxpayerKind
	}{
		{"", KindUnknown},
		{"I", Individual},
		{"i", Individual},
		{"Individual", Individual},
		{"individual", Individual},
		{"IND", Individual},
		{"sole proprietor", Individual},
		{"N", NonIndividual},
		{"n", NonIndividual},
		{"C", NonIndividual},
		{"Corporation", NonIndividual},
		{"corp", NonIndividual},
		{"Non-Individual", NonIndividual},
		{"non individual", NonIndividual},
		{"Partnership", NonIndividual},
		{"cooperative", NonIndividual},
		{"Inc.", NonIndividual},
		{"  Individual  ", Individual},
	}
	for _, c := range cases {
		got, err := ParseTaxpayerKind(c.in)
		if err != nil {
			t.Errorf("ParseTaxpayerKind(%q) returned error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseTaxpayerKind(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, s := range []string{"maybe", "both", "x", "123"} {
		if _, err := ParseTaxpayerKind(s); err == nil {
			t.Errorf("ParseTaxpayerKind(%q) was accepted", s)
		} else if !errors.Is(err, ErrName) {
			t.Errorf("ParseTaxpayerKind(%q) error %v does not wrap ErrName", s, err)
		}
	}
}

func TestSplitPersonName(t *testing.T) {
	cases := []struct {
		in                  string
		last, first, middle string
	}{
		{"DELA CRUZ, JUAN P.", "DELA CRUZ", "JUAN", "P."},
		{"Santos, Maria Clara Reyes", "SANTOS", "MARIA CLARA", "REYES"},
		{"Lim, Andrew", "LIM", "ANDREW", ""},
		{"DE LOS SANTOS, Jose Rizal Mercado", "DE LOS SANTOS", "JOSE RIZAL", "MERCADO"},
		{"  reyes  ,  ana  marie  ", "REYES", "ANA", "MARIE"},
		// The tilde letter survives, because the data file carries it; the
		// other accents are folded, because it cannot.
		{"Niño, José María Cruz", "NIÑO", "JOSE MARIA", "CRUZ"},
		// The apostrophe does not survive: the validation module refuses it.
		{"O'Brien, Patrick", "OBRIEN", "PATRICK", ""},
		{"Tan-Lim, Mary Grace Uy", "TAN-LIM", "MARY GRACE", "UY"},
		{"Peña, Maria", "PEÑA", "MARIA", ""},
	}
	for _, c := range cases {
		got, err := SplitPersonName(c.in)
		if err != nil {
			t.Errorf("SplitPersonName(%q) returned error %v", c.in, err)
			continue
		}
		if got.Last != c.last || got.First != c.first || got.Middle != c.middle {
			t.Errorf("SplitPersonName(%q) = {%q %q %q}, want {%q %q %q}",
				c.in, got.Last, got.First, got.Middle, c.last, c.first, c.middle)
		}
	}
}

func TestSplitPersonNameRefusesToGuessTheSurname(t *testing.T) {
	// Without a comma there is no safe way to tell where a Filipino surname
	// ends, so the tool must say so instead of guessing.
	for _, s := range []string{"Juan Dela Cruz", "Jose Rizal", "MARIA CLARA DE LOS SANTOS"} {
		_, err := SplitPersonName(s)
		if err == nil {
			t.Errorf("SplitPersonName(%q) guessed a surname; it should refuse", s)
			continue
		}
		if !strings.Contains(err.Error(), "SURNAME, First Middle") {
			t.Errorf("the error for %q does not tell the user what to write: %v", s, err)
		}
	}
	for _, s := range []string{"", "  ", ", Juan", "Dela Cruz,", "Dela Cruz,   "} {
		if _, err := SplitPersonName(s); err == nil {
			t.Errorf("SplitPersonName(%q) was accepted", s)
		}
	}
}

func TestPersonNameRendering(t *testing.T) {
	n := PersonName{Last: "DELA CRUZ", First: "JUAN", Middle: "P."}
	if got := n.String(); got != "DELA CRUZ, JUAN P." {
		t.Errorf("String() = %q", got)
	}
	if n.Empty() {
		t.Error("a filled-in name reported itself empty")
	}
	var z PersonName
	if !z.Empty() {
		t.Error("the zero name did not report itself empty")
	}
	if got := z.String(); got != "" {
		t.Errorf("the zero name rendered as %q", got)
	}
}

func TestCleanText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme Trading Corporation", "ACME TRADING CORPORATION"},
		{"acme   trading", "ACME TRADING"},
		{"  Acme Trading  ", "ACME TRADING"},
		{"Acme\tTrading\nCorp", "ACME TRADING CORP"},
		{"San Miguel Corp.", "SAN MIGUEL CORP."},
		{"Tan-Lim Hardware", "TAN-LIM HARDWARE"},
		{"7-Eleven #1234", "7-ELEVEN #1234"},
		{"A/B Enterprises", "A/B ENTERPRISES"},
		{"100% Pure Water Refilling", "100% PURE WATER REFILLING"},
		{"J R Trading (Phils.) Inc.", "J R TRADING (PHILS.) INC."},
		{"Café España", "CAFE ESPAÑA"},
		{"Niño Jesús Store", "NIÑO JESUS STORE"},
		{"Peña Water District", "PEÑA WATER DISTRICT"},
		{"Smith — Jones", "SMITH - JONES"},
		{"", ""},
		{"   ", ""},
		{"​Acme​", "ACME"},
		{"Unit 5, Ayala Ave", "UNIT 5 AYALA AVE"},
	}
	for _, c := range cases {
		if got := CleanName(c.in); got != c.want {
			t.Errorf("CleanName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanTextSubstitutesWhatTheValidationModuleRefuses(t *testing.T) {
	// The validation module refuses the ampersand and the apostrophe, which
	// are common in real business names. They must be substituted, and the
	// substitution must be reported rather than made in silence.
	cases := []struct {
		in         string
		want       string
		mustReport string
	}{
		{"Smith & Sons", "SMITH AND SONS", "&"},
		{"O'Brien's Bakery", "OBRIENS BAKERY", "'"},
		{"J&R Trading", "J AND R TRADING", "&"},
		{"A & B & C", "A AND B AND C", "&"},
		{"Cash $ Carry", "CASH CARRY", "$"},
		{"Back\\Slash Inc", "BACKSLASH INC", "\\\\"},
		{"Pipe|Dream", "PIPEDREAM", "|"},
		{"Tick`Tock", "TICKTOCK", "`"},
		{"Acme, Incorporated", "ACME INCORPORATED", ","},
		{`Say "Hello"`, "SAY HELLO", `"`},
	}
	for _, c := range cases {
		got, changes := CleanText(c.in)
		if got != c.want {
			t.Errorf("CleanText(%q) = %q, want %q", c.in, got, c.want)
		}
		if len(changes) == 0 {
			t.Errorf("CleanText(%q) changed the value but reported nothing", c.in)
			continue
		}
		found := false
		for _, ch := range changes {
			if strings.Contains(ch, c.mustReport) {
				found = true
			}
		}
		if !found {
			t.Errorf("CleanText(%q) reported %v, none of which mentions %q", c.in, changes, c.mustReport)
		}
	}
}

func TestCleanTextLeavesAcceptableValuesAlone(t *testing.T) {
	for _, s := range []string{
		"ACME TRADING CORPORATION", "SAN MIGUEL CORP.", "7-ELEVEN #1234",
		"A/B ENTERPRISES", "100% PURE WATER", "PEÑA WATER DISTRICT",
		"UNIT 5 (REAR) AYALA AVE", "ABC+XYZ", "DELA CRUZ",
	} {
		got, changes := CleanText(s)
		if got != s {
			t.Errorf("CleanText(%q) = %q; an acceptable value was altered", s, got)
		}
		if len(changes) != 0 {
			t.Errorf("CleanText(%q) reported changes %v for an acceptable value", s, changes)
		}
	}
}

func TestCleanTextOutputAlwaysPassesTheCharacterCheck(t *testing.T) {
	// Whatever goes in, what comes out must be writable: no separator, no
	// delimiter, no line break, and nothing the validation module refuses.
	inputs := []string{
		"AC" + string(FieldSeparator) + "ME" + string(TextDelimiter) + " CORP",
		"A\nB\rC\tD",
		"\x00\x01\x02 NAME",
		"\u4e2d\u6587 TRADING",
		"EMOJI \U0001f600 CORP",
		"~TILDE~ \u00bd \u00ff \u00a5",
		strings.Repeat("&'\"", 20),
	}
	for _, in := range inputs {
		got := CleanName(in)
		if bad := offendingCharacters(got); bad != "" {
			t.Errorf("CleanName(%q) = %q, which still contains refused characters: %s", in, got, bad)
		}
		if strings.ContainsRune(got, FieldSeparator) || strings.ContainsRune(got, TextDelimiter) {
			t.Errorf("CleanName(%q) = %q, which contains a structural character", in, got)
		}
		if strings.ContainsAny(got, "\r\n\t") {
			t.Errorf("CleanName(%q) = %q, which contains a line break", in, got)
		}
	}
}

func TestNTildeSurvivesAndEncodesAsOneByte(t *testing.T) {
	// The validation module makes a deliberate exception for N with a tilde,
	// and it matters: PEÑA and NIÑO are real surnames. It must survive
	// cleaning and must reach the file as a single byte, not as two.
	got := CleanName("Peña")
	if got != "PEÑA" {
		t.Fatalf("CleanName(%q) = %q, want %q", "Peña", got, "PEÑA")
	}
	b := Encode(got)
	if len(b) != 4 {
		t.Errorf("Encode(%q) produced %d bytes (%v), want 4 — the file is single-byte", got, len(b), b)
	}
	if b[2] != nTilde {
		t.Errorf("Encode(%q) wrote the tilde letter as %#x, want %#x", got, b[2], nTilde)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in      string
		max     int
		want    string
		wantCut bool
	}{
		{"ACME", 10, "ACME", false},
		{"ACME", 4, "ACME", false},
		{"ACME TRADING", 4, "ACME", true},
		{"ACME TRADING CORPORATION", 12, "ACME TRADING", true},
		{"ACMETRADINGCORPORATION", 10, "ACMETRADIN", true},
		{"ACME TRADING CORP.", 14, "ACME TRADING", true},
		{"", 10, "", false},
		{"ACME", 0, "ACME", false},
		{"ACME", -1, "ACME", false},
	}
	for _, c := range cases {
		got, cut := Truncate(c.in, c.max)
		if got != c.want || cut != c.wantCut {
			t.Errorf("Truncate(%q, %d) = %q, %v; want %q, %v", c.in, c.max, got, cut, c.want, c.wantCut)
		}
		if c.max > 0 && len(got) > c.max {
			t.Errorf("Truncate(%q, %d) returned %d characters", c.in, c.max, len(got))
		}
	}
}

func TestFoldToASCII(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"Niño", "Niño"}, // the tilde letter is kept, not folded
		{"Ñ", "Ñ"},
		{"José", "Jose"},
		{"\u20b1100", "PHP100"},
		{"a\u2013b", "a-b"},
		{"\u4e2d\u6587", "  "}, // unknown characters become spaces, not nothing
	}
	for _, c := range cases {
		if got := foldToASCII(c.in, func(string) {}); got != c.want {
			t.Errorf("foldToASCII(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if !isASCII("plain ascii") {
		t.Error("isASCII said a plain string is not ASCII")
	}
	if isASCII("Niño") {
		t.Error("isASCII said an accented string is ASCII")
	}
}

func TestAllowedAndForbiddenCharactersAgree(t *testing.T) {
	allowed, forbidden := AllowedCharacters(), ForbiddenCharacters()
	if allowed == "" || forbidden == "" {
		t.Fatal("the character lists are empty")
	}
	if strings.ContainsAny(allowed, forbidden) {
		t.Errorf("the allowed list %q contains a forbidden character from %q", allowed, forbidden)
	}
	if strings.ContainsRune(allowed, FieldSeparator) {
		t.Error("the allowed list contains the field separator")
	}
	if strings.ContainsRune(allowed, TextDelimiter) {
		t.Error("the allowed list contains the text delimiter")
	}
	for _, r := range forbidden {
		if allowedByte(byte(r)) {
			t.Errorf("%q is listed as forbidden but allowedByte accepts it", string(r))
		}
	}
	for _, r := range allowed {
		if !allowedByte(byte(r)) {
			t.Errorf("%q is listed as allowed but allowedByte refuses it", string(r))
		}
	}
}
