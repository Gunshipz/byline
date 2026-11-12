package textutil

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	if got := Normalize("a\r\nb\u00a0c\u2003d\ufeffe\vf"); got != "a\nb c d e f" {
		t.Fatalf("%q", got)
	}
}

func TestIndex16AndSlice16(t *testing.T) {
	s := "a’b🎉c"
	if Index16(s, strings.Index(s, "c")) != 5 || Slice16(s, 1, 5) != "’b🎉" || Slice16(s, 0, 4) != "a’b" || Slice16(s, 3, 99) != "🎉c" {
		t.Fatal(Index16(s, strings.Index(s, "c")), Slice16(s, 1, 5), Slice16(s, 0, 4))
	}
}

// Values from Node: (x).toFixed(d), then unary plus.
func TestToFixed_RoundsLikeNode(t *testing.T) {
	for _, c := range []struct {
		x    float64
		d    int
		want float64
	}{{12.5, 0, 13}, {0.125, 2, 0.13}, {1.005, 2, 1}, {-2.5, 0, -3}, {2.675, 2, 2.67}, {8.625, 2, 8.63}} {
		if got := ToFixed(c.x, c.d); got != c.want {
			t.Errorf("ToFixed(%v,%d) = %v, want %v", c.x, c.d, got, c.want)
		}
	}
}

func TestFormatNum(t *testing.T) {
	for x, want := range map[float64]string{8.62: "8.62", 10: "10", 0.2: "0.2", 3.3: "3.3", -3: "-3", 0: "0"} {
		if got := FormatNum(x); got != want {
			t.Errorf("%v: %q", x, got)
		}
	}
}

// Gate finding I4: factlint has no producer of signature_lines for anyone
// but the owner, so it needs a generic closing-line fallback.
func TestCutClosingSignature(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want int
	}{
		"closing line alone at the end": {
			"Hi team,\n\nUpdate below.\n\nThanks,", 25,
		},
		"closing line plus a short sign-off": {
			"Update below.\n\nThanks,\nJordan Sample\nSenior Program Manager\nNorth America Operations\n", 15,
		},
		"up to 4 short lines, trailing blank line allowed": {
			"Body.\n\nBest,\na\nb\nc\nd\n\n", 7,
		},
		"5 short lines is too many": {
			"Body.\n\nBest,\na\nb\nc\nd\ne\n", -1,
		},
		"a line over 6 words after the closer fails it": {
			"Body.\n\nBest,\nthis line has way more than six words in it\n", -1,
		},
		"not alone on its line": {
			"Body.\n\nThanks, that's great, see you then.\n", -1,
		},
		"more content after the short lines": {
			"Body.\n\nBest,\na\n\nOne more paragraph that keeps going.\n", -1,
		},
		"no closing line at all": {
			"Just an ordinary note with no sign-off.\n", -1,
		},
	} {
		if got := CutClosingSignature(c.in); got != c.want {
			t.Errorf("%s: CutClosingSignature(%q) = %d, want %d", name, c.in, got, c.want)
		}
	}
}

// Same pieces as Node's s.split(/(?<=[.!?])\s+|\n\n+/).
func TestSplitSentences(t *testing.T) {
	for in, want := range map[string][]Piece{
		"One. Two!  Three?\nFour\n\nFive": {{"One.", 0}, {"Two!", 5}, {"Three?", 11}, {"Four", 18}, {"Five", 24}},
		"no split\nhere. ok":              {{"no split\nhere.", 0}, {"ok", 15}},
		"a\n\n\nb":                        {{"a", 0}, {"b", 4}},
		"Wait...  next":                   {{"Wait...", 0}, {"next", 9}},
		"x \n\ny":                         {{"x ", 0}, {"y", 4}},
	} {
		if got := SplitSentences(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v", in, got)
		}
	}
}
