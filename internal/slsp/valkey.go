package slsp

import (
	"fmt"
	"strings"
)

// A file sent in through the Bureau's electronic submission, rather than
// handed over on a disk, carries one extra line at the end: a twenty-character
// validation key worked out from the file's own name and contents. The
// Bureau's validation module writes that second copy into a folder of its own
// and tells the user that is the one to attach.
//
// The key is derived, not secret: it is a weak checksum that lets the
// receiving end notice a file that has been edited after validation. The
// algorithm below is transcribed from the module that produces it.
//
// Two copies of the data are therefore produced: the plain file, which is what
// the Bureau's own validation module expects to be given, and the submission
// copy with the key line appended. Producing both is what the module does, so
// whichever the submission channel wants is to hand.

const (
	// ValidationKeyLength is how many characters the key line holds.
	ValidationKeyLength = 20

	// SubmissionFolder is the folder the Bureau's module puts the submission
	// copy in, and the name this tool uses for the same thing.
	SubmissionFolder = "eRelief"
)

// ValidationKey works out the submission key for a file.
//
// baseName is the file's name without its extension: nine digits of the
// filing taxpayer's TIN, one letter for the list, two digits of month and four
// of year — sixteen characters in all. records are the header and detail lines
// as they appear in the file, without their line endings.
//
// The key is built character by character from that name, each character
// shifted by either the number of lines in the file or the total number of
// characters in them, and closed with the low byte of the sum of every
// character in the file.
func ValidationKey(baseName string, records []string) (string, error) {
	if len(baseName) < 16 {
		return "", fmt.Errorf("the file name %q is %d characters; the submission key needs the full sixteen", baseName, len(baseName))
	}
	name := baseName[:16]

	var lineCount, charCount, sum int
	for _, rec := range records {
		line := strings.TrimSpace(rec)
		if line == "" {
			continue
		}
		switch line[0] {
		case 'H', 'h', 'D', 'd':
		default:
			// Only the header and detail lines count towards the key.
			continue
		}
		b := Encode(line)
		lineCount++
		charCount += len(b)
		for _, c := range b {
			sum += int(c)
		}
	}
	sum %= 256

	// digit shifts the numeric value of one character of the name and takes
	// the last decimal digit; letter does the same into the range A to Z. A
	// character of the name that is not a digit counts as zero, which is what
	// the module's own conversion does.
	digit := func(i, by int) string { return fmt.Sprintf("%d", (digitAt(name, i)+by)%10) }
	letter := func(i, by int) string { return string(rune((digitAt(name, i)+by)%26 + 'A')) }

	var b strings.Builder
	b.WriteByte('V')
	// The nine digits of the TIN.
	b.WriteString(digit(1, charCount))
	b.WriteString(letter(2, charCount))
	b.WriteString(digit(3, lineCount))
	b.WriteString(letter(4, lineCount))
	b.WriteString(digit(5, charCount))
	b.WriteString(letter(6, charCount))
	b.WriteString(digit(7, lineCount))
	b.WriteString(letter(8, lineCount))
	b.WriteString(digit(9, charCount))
	// The two digits of the month.
	b.WriteString(letter(11, lineCount))
	b.WriteString(digit(12, lineCount))
	// The letter that says which list it is, taken by its character code
	// rather than its numeric value, because it is not a digit.
	b.WriteString(string(rune((int(name[9])+charCount)%26 + 'A')))
	// The four digits of the year.
	b.WriteString(digit(13, charCount))
	b.WriteString(digit(14, lineCount))
	b.WriteString(letter(15, charCount))
	b.WriteString(digit(16, charCount))
	// The low byte of the sum of every character, in three digits.
	fmt.Fprintf(&b, "%03d", sum)

	key := b.String()
	if len(key) != ValidationKeyLength {
		return "", fmt.Errorf("the submission key came out %d characters long instead of %d; this is a fault in the tool", len(key), ValidationKeyLength)
	}
	return key, nil
}

// digitAt returns the numeric value of the i'th character of the name, counted
// from one as the module counts. A character that is not a digit counts as
// zero.
func digitAt(name string, i int) int {
	if i < 1 || i > len(name) {
		return 0
	}
	c := name[i-1]
	if c < '0' || c > '9' {
		return 0
	}
	return int(c - '0')
}
