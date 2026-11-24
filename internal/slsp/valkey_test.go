package slsp

import (
	"strings"
	"testing"
)

func TestValidationKeyShape(t *testing.T) {
	// Twenty characters: a V, then eight pairs of a digit and a letter drawn
	// from the file name, then three digits of checksum. The positions of the
	// digits and the letters are fixed by the algorithm, so they are checked
	// here rather than only the length.
	records := []string{
		`H,S,"004123456","ABC TRADING",,,,"ABC","X","Y",0.00,0.00,100.00,12.00,044,10/31/2026,12`,
		`D,S,"123456789","XYZ CORP",,,,"A","B",0,0,100.00,12.00,004123456,10/31/2026`,
	}
	key, err := ValidationKey("004123456S102026", records)
	if err != nil {
		t.Fatalf("ValidationKey: %v", err)
	}
	if len(key) != ValidationKeyLength {
		t.Fatalf("the key %q is %d characters, want %d", key, len(key), ValidationKeyLength)
	}
	if key[0] != 'V' {
		t.Errorf("the key %q does not begin with V", key)
	}
	// Characters 2 to 17 alternate digit and letter in the pattern the
	// algorithm lays down, and 18 to 20 are the checksum.
	wantDigit := map[int]bool{1: true, 3: true, 5: true, 7: true, 9: true, 11: true, 13: true, 14: true, 16: true, 17: true, 18: true, 19: true}
	for i := 1; i < len(key); i++ {
		c := key[i]
		if wantDigit[i] {
			if c < '0' || c > '9' {
				t.Errorf("character %d of %q is %q, want a digit", i+1, key, string(c))
			}
			continue
		}
		if c < 'A' || c > 'Z' {
			t.Errorf("character %d of %q is %q, want a letter A to Z", i+1, key, string(c))
		}
	}
}

func TestValidationKeyIsDeterministic(t *testing.T) {
	records := []string{"H,S,\"004123456\"", "D,S,\"123456789\""}
	first, err := ValidationKey("004123456S102026", records)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := ValidationKey("004123456S102026", records)
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("the key changed between runs: %q then %q", first, again)
		}
	}
}

func TestValidationKeyChangesWithTheFile(t *testing.T) {
	base := []string{
		`H,S,"004123456","ABC TRADING",,,,"ABC","X","Y",0.00,0.00,100.00,12.00,044,10/31/2026,12`,
		`D,S,"123456789","XYZ CORP",,,,"A","B",0,0,100.00,12.00,004123456,10/31/2026`,
	}
	key, err := ValidationKey("004123456S102026", base)
	if err != nil {
		t.Fatal(err)
	}

	// The key exists to notice an edited file, so each of these must change it.
	variants := map[string][]string{
		"an amount altered": {
			base[0],
			strings.Replace(base[1], "100.00", "900.00", 1),
		},
		"a record added": append(append([]string{}, base...),
			`D,S,"234567890","OTHER CORP",,,,"A","B",0,0,1.00,0.12,004123456,10/31/2026`),
		"a record removed": {base[0]},
		"a name altered": {
			base[0],
			strings.Replace(base[1], "XYZ CORP", "XYZ CORPX", 1),
		},
	}
	for name, recs := range variants {
		got, err := ValidationKey("004123456S102026", recs)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got == key {
			t.Errorf("%s did not change the key (%q)", name, key)
		}
	}

	// And so must a change to the file name.
	for _, other := range []string{"004123456P102026", "004123456S112026", "004123457S102026", "004123456S102027"} {
		got, err := ValidationKey(other, base)
		if err != nil {
			t.Fatal(err)
		}
		if got == key {
			t.Errorf("the name %q gives the same key as 004123456S102026 (%q)", other, key)
		}
	}
}

func TestValidationKeyCountsOnlyHeaderAndDetailLines(t *testing.T) {
	// The routine skips anything that does not begin with H or D, and ignores
	// blank lines, so a stray line must not change the key.
	base := []string{
		`H,S,"004123456","ABC"`,
		`D,S,"123456789","XYZ"`,
	}
	want, err := ValidationKey("004123456S102026", base)
	if err != nil {
		t.Fatal(err)
	}
	withNoise := []string{base[0], "", "   ", "# a comment", base[1], ""}
	got, err := ValidationKey("004123456S102026", withNoise)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("a stray line changed the key: %q then %q", want, got)
	}
}

func TestValidationKeyUsesTheSingleByteLength(t *testing.T) {
	// The accented letter is one byte in the file, not two, so it must count
	// as one character. Getting this wrong would produce a key the Bureau's
	// own routine disagrees with.
	oneByte := []string{"H,S,\"004123456\",\"PEÑA\""}
	// The same record with the accented letter replaced by a plain one has the
	// same byte length once encoded, so the same character count, and
	// therefore the same line and character counters. Only the checksum
	// differs, by the difference between the two bytes.
	plain := []string{"H,S,\"004123456\",\"PENA\""}
	if len(Encode(oneByte[0])) != len(Encode(plain[0])) {
		t.Fatalf("the two records encode to %d and %d bytes; the test needs them equal",
			len(Encode(oneByte[0])), len(Encode(plain[0])))
	}
	k1, err := ValidationKey("004123456S102026", oneByte)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := ValidationKey("004123456S102026", plain)
	if err != nil {
		t.Fatal(err)
	}
	if k1[:17] != k2[:17] {
		t.Errorf("the accented letter changed the length-derived part of the key: %q and %q", k1, k2)
	}
	if k1 == k2 {
		t.Error("the accented letter did not change the checksum, so it was not counted by its own byte value")
	}
}

func TestValidationKeyRefusesAShortName(t *testing.T) {
	for _, name := range []string{"", "004123456", "004123456S1020", "short"} {
		if _, err := ValidationKey(name, []string{"H,S,x"}); err == nil {
			t.Errorf("ValidationKey accepted the name %q", name)
		}
	}
}

func TestValidationKeyWithNoRecords(t *testing.T) {
	// No records is not this function's business to refuse — the generator
	// has already refused a file with no details — but it must not panic.
	key, err := ValidationKey("004123456S102026", nil)
	if err != nil {
		t.Fatalf("ValidationKey(nil records): %v", err)
	}
	if len(key) != ValidationKeyLength {
		t.Errorf("the key %q is %d characters", key, len(key))
	}
}

func TestDigitAt(t *testing.T) {
	name := "004123456S102026"
	cases := map[int]int{1: 0, 2: 0, 3: 4, 4: 1, 9: 6, 10: 0, 11: 1, 12: 0, 13: 2, 16: 6}
	for i, want := range cases {
		if got := digitAt(name, i); got != want {
			t.Errorf("digitAt(%q, %d) = %d, want %d", name, i, got, want)
		}
	}
	// Out of range and non-digit characters count as zero rather than
	// panicking.
	for _, i := range []int{0, -1, 17, 100} {
		if got := digitAt(name, i); got != 0 {
			t.Errorf("digitAt(%q, %d) = %d, want 0", name, i, got)
		}
	}
	if got := digitAt("", 1); got != 0 {
		t.Errorf("digitAt(\"\", 1) = %d, want 0", got)
	}
}

func TestValidationKeyChecksumIsAByte(t *testing.T) {
	// The checksum is the low byte of a sum, so it is always 0 to 255 and
	// always written in three digits.
	for n := 1; n <= 40; n++ {
		records := make([]string, 0, n)
		for i := 0; i < n; i++ {
			records = append(records, `D,S,"12345678`+string(rune('0'+i%10))+`","NAME",,,,"A","B",0,0,1.00,0.12,004123456,10/31/2026`)
		}
		key, err := ValidationKey("004123456S102026", records)
		if err != nil {
			t.Fatal(err)
		}
		tail := key[len(key)-3:]
		if !allDigits(tail) {
			t.Fatalf("the checksum %q of %q is not three digits", tail, key)
		}
		v := 0
		for _, c := range tail {
			v = v*10 + int(c-'0')
		}
		if v < 0 || v > 255 {
			t.Errorf("the checksum of %q is %d, outside 0 to 255", key, v)
		}
	}
}
