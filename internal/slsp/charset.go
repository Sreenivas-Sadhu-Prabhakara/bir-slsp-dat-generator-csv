package slsp

import "strings"

// The data file is a single-byte text file in the Windows code page the
// Bureau's module was built on, and the validation module checks every text
// field against a fixed list of acceptable byte values. This file holds that
// list and the encoder, so that what is written is what the validation module
// will accept.
//
// Two of the characters the validation module refuses are common in real
// business names: the ampersand and the apostrophe. The module that writes the
// file does not remove them, which means the Bureau's own data entry module
// can produce a file its own validator rejects. This tool substitutes them and
// says so, rather than passing them through and letting the submission fail.

// EncodingName is the character encoding of the data file.
const EncodingName = "Windows-1252 (single-byte)"

const (
	// nTilde and nTildeLower are the two characters outside plain ASCII that
	// the validation module accepts, and they matter: they appear in Filipino
	// surnames such as PEÑA and NIÑO. In this encoding they are single bytes.
	nTilde      = 0xD1 // N with tilde, upper case
	nTildeLower = 0xF1 // n with tilde, lower case
)

// forbiddenASCII are the printable ASCII characters the validation module
// refuses inside a text field. Each is listed with what it is replaced by and
// why, so that the user can be told exactly what changed.
var forbiddenASCII = map[byte]struct {
	with string
	why  string
}{
	'"':  {"", "the text delimiter, which would end the field early"},
	'$':  {"", "refused by the validation module"},
	'&':  {" AND ", "refused by the validation module"},
	'\'': {"", "refused by the validation module"},
	',':  {"", "the field separator, which would split the record"},
	'\\': {"", "refused by the validation module"},
	'`':  {"", "refused by the validation module"},
	'|':  {"", "refused by the validation module"},
}

// allowedByte reports whether a single byte may appear inside a text field.
// The ranges are those the validation module tests against: anything at or
// below a space's predecessor is out, as are a handful of printable
// characters and everything above the ASCII range except the two accented
// letters.
func allowedByte(b byte) bool {
	if b < 0x20 {
		return false
	}
	if b == nTilde || b == nTildeLower {
		return true
	}
	if b > 0x7d {
		return false // 0x7e onwards is refused, apart from the two above
	}
	if _, bad := forbiddenASCII[b]; bad {
		return false
	}
	return true
}

// AllowedCharacters renders the acceptable set for the documentation.
func AllowedCharacters() string {
	var b strings.Builder
	for c := byte(0x20); c <= 0x7d; c++ {
		if allowedByte(c) {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ForbiddenCharacters renders the refused printable characters for the
// documentation, in a stable order.
func ForbiddenCharacters() string {
	var b strings.Builder
	for _, c := range []byte{'"', '$', '&', '\'', ',', '\\', '`', '|'} {
		b.WriteByte(c)
	}
	return b.String()
}

// encodeRune turns one character into the bytes this file carries, or returns
// false if it has no representation here.
func encodeRune(r rune) (string, bool) {
	switch r {
	case 'Ñ': // N with tilde
		return string([]byte{nTilde}), true
	case 'ñ': // n with tilde
		return string([]byte{nTildeLower}), true
	}
	if r < 0x80 && allowedByte(byte(r)) {
		return string([]byte{byte(r)}), true
	}
	return "", false
}

// Encode turns records that CleanText has already passed over into the bytes
// of the data file.
//
// It is given whole records, or a whole file, so the structural characters —
// the separator, the delimiter and the carriage return and line feed that end
// a record — pass straight through. What it is for is the values: every
// character in one has a single-byte representation by the time it gets here,
// and a character that somehow does not is dropped rather than written as two
// bytes, because a multi-byte character in this file is what a rejected
// submission looks like. A value that still held a separator or a line break
// of its own would have been refused by the validator before reaching this
// point.
func Encode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch r {
		case '\r', '\n', FieldSeparator, TextDelimiter:
			out = append(out, byte(r))
			continue
		case 'Ñ':
			out = append(out, nTilde)
			continue
		case 'ñ':
			out = append(out, nTildeLower)
			continue
		}
		if r < 0x80 {
			out = append(out, byte(r))
			continue
		}
		if b, ok := encodeRune(r); ok {
			out = append(out, b...)
		}
	}
	return out
}
