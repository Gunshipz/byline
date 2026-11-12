// Package textutil provides the JavaScript-compatible text helpers
// Byline's checkers need to match today's Node scripts exactly: Unicode
// whitespace normalization, UTF-16 code unit offsets, the rounding
// Number.prototype.toFixed does, number formatting, and the lookbehind
// sentence split RE2 can't express as a regex.
package textutil

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Normalize turns CRLF into LF, then turns every character JavaScript's \s
// matches and Go's \s doesn't into a plain space.
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isJSOnlySpace(r) {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isJSOnlySpace(r rune) bool {
	switch r {
	case 0x000B, 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func utf16Width(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// Index16 converts a byte index into s into a UTF-16 code unit index, the
// way the old Node scripts counted offsets (a rune above U+FFFF counts 2).
func Index16(s string, byteIdx int) int {
	units := 0
	for i, r := range s {
		if i >= byteIdx {
			break
		}
		units += utf16Width(r)
	}
	return units
}

// unitToByteFloor returns the byte offset of the rune whose UTF-16 span
// contains code unit position pos, floored to that rune's start so a
// position landing inside a surrogate pair never splits it. A pos at or
// past s's UTF-16 length returns len(s).
func unitToByteFloor(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	u := 0
	for i, r := range s {
		w := utf16Width(r)
		if pos < u+w {
			return i
		}
		u += w
	}
	return len(s)
}

// Slice16 slices s by UTF-16 code unit offsets, the way the old Node
// scripts sliced strings. to is clamped to s's UTF-16 length; a from or to
// that would cut a surrogate pair (a rune above U+FFFF) in half instead
// floors to that rune's start, so the rune is dropped rather than split.
func Slice16(s string, from, to int) string {
	if from < 0 {
		from = 0
	}
	if to < from {
		return ""
	}
	bFrom := unitToByteFloor(s, from)
	bTo := unitToByteFloor(s, to)
	if bTo < bFrom {
		bTo = bFrom
	}
	return s[bFrom:bTo]
}

// ToFixed rounds x to digits decimal places the way
// Number.prototype.toFixed does: take the exact rational value of x's
// float64 bits (not the decimal x was written with), scale by 10^digits,
// round half away from zero, and scale back. Operating on the exact bits
// reproduces toFixed's known quirks on values like 1.005 and 2.675, whose
// nearest double is a hair below the written decimal.
func ToFixed(x float64, digits int) float64 {
	neg := x < 0
	if neg {
		x = -x
	}
	r := new(big.Rat).SetFloat64(x)
	if r == nil { // NaN or Inf: nothing to round
		if neg {
			return -x
		}
		return x
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	if new(big.Int).Mul(rem, big.NewInt(2)).Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	f, _ := new(big.Float).SetInt(q).Float64()
	result := f / math.Pow10(digits)
	if neg {
		result = -result
	}
	return result
}

// FormatNum formats x the way the old Node scripts printed numbers:
// strconv.FormatFloat(x, 'f', -1, 64), except -0 prints as "0".
func FormatNum(x float64) string {
	if x == 0 {
		return "0"
	}
	return strconv.FormatFloat(x, 'f', -1, 64)
}

// closingLines are the generic sign-off openers CutClosingSignature
// recognizes, for a person with no signature_lines of their own configured.
var closingLines = map[string]bool{
	"Thanks,": true, "Thank you,": true, "Best,": true, "Regards,": true,
	"Cheers,": true, "Best regards,": true, "Kind regards,": true,
	"Sincerely,": true, "Thanks!": true, "Thank you!": true,
}

// CutClosingSignature finds a generic closing line (one of "Thanks,",
// "Best,", "Regards," and similar, alone on its own line) that is followed
// by at most 4 further lines, each 6 words or fewer, and nothing else in s
// (trailing blank lines are allowed). It returns the byte offset in s where
// the closing line starts, or -1 if s has no such signature.
func CutClosingSignature(s string) int {
	lines := strings.Split(s, "\n")
	offset := 0
	offsets := make([]int, len(lines))
	for i, l := range lines {
		offsets[i] = offset
		offset += len(l) + 1
	}
	for i, l := range lines {
		if !closingLines[strings.TrimSpace(l)] {
			continue
		}
		// The sign-off block is the run of non-blank lines right after the
		// closing line; a blank line inside it ends the block, and
		// anything past the block must be blank too, or this isn't really
		// the end of the message.
		rest := lines[i+1:]
		blockEnd := 0
		for blockEnd < len(rest) && strings.TrimSpace(rest[blockEnd]) != "" {
			blockEnd++
		}
		block := rest[:blockEnd]
		if len(block) > 4 {
			continue
		}
		ok := true
		for _, r := range block {
			if len(strings.Fields(r)) > 6 {
				ok = false
				break
			}
		}
		for _, r := range rest[blockEnd:] {
			if strings.TrimSpace(r) != "" {
				ok = false
				break
			}
		}
		if ok {
			return offsets[i]
		}
	}
	return -1
}

// Piece is one span SplitSentences cuts s into: its text and the byte
// offset in s where it starts.
type Piece struct {
	Text  string
	Start int
}

func isASCIISpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// SplitSentences splits s the way the old Node scripts did with
// s.split(/(?<=[.!?])\s+|\n\n+/), a lookbehind RE2 can't express: at a run
// of ASCII whitespace whose preceding byte is '.', '!' or '?', or else at a
// run of two or more '\n' anywhere. Empty pieces are kept, as Node keeps
// them.
func SplitSentences(s string) []Piece {
	var pieces []Piece
	pieceStart, i, n := 0, 0, len(s)
	for i < n {
		if i > 0 {
			switch s[i-1] {
			case '.', '!', '?':
				if isASCIISpaceByte(s[i]) {
					j := i
					for j < n && isASCIISpaceByte(s[j]) {
						j++
					}
					pieces = append(pieces, Piece{s[pieceStart:i], pieceStart})
					pieceStart, i = j, j
					continue
				}
			}
		}
		if s[i] == '\n' {
			j := i
			for j < n && s[j] == '\n' {
				j++
			}
			if j-i >= 2 {
				pieces = append(pieces, Piece{s[pieceStart:i], pieceStart})
				pieceStart, i = j, j
				continue
			}
		}
		i++
	}
	pieces = append(pieces, Piece{s[pieceStart:n], pieceStart})
	return pieces
}
