package voice

import (
	"fmt"
	"io"
	"os"
	"strings"

	"byline/internal/rules"
	"byline/internal/textutil"
)

const usage = "usage: byline-mcp voicecheck <draft> | --calibrate <corpus>"

// RunCLI runs the voicecheck mode: scoring one draft, or (with
// --calibrate) deriving baseline ranges from a corpus. An unknown flag is
// rejected with a plain message (M10, gate-p12-p5-review.md) instead of
// being read as the draft or corpus path.
func RunCLI(args []string, set *rules.Set, stdout, stderr io.Writer) int {
	if len(args) >= 1 && strings.HasPrefix(args[0], "--") && args[0] != "--calibrate" {
		fmt.Fprintf(stderr, "voicecheck: unknown flag %q\n", args[0])
		return 2
	}
	if len(args) >= 1 && args[0] == "--calibrate" {
		if len(args) < 2 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return runCalibrate(args[1], set, stdout, stderr)
	}
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runScore(args[0], set, stdout, stderr)
}

func runScore(file string, set *rules.Set, stdout, stderr io.Writer) int {
	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "voicecheck: can't read %s: %v\n", file, err)
		return 2
	}
	r := Score(set, string(b))
	if r.NotMeasured {
		fmt.Fprintln(stdout, "Byline hasn't measured this person's voice yet; setup does that.")
		return 0
	}
	fmt.Fprintf(stdout, "%s  VOICE %s/10  (%d/%d in range, penalty %d)\n",
		file, textutil.FormatNum(r.VoiceScore), r.InRange, r.Total, r.Penalty)
	for _, d := range r.Detail {
		mark := "ok  "
		if !d.OK {
			mark = fmt.Sprintf("%-4s", d.Dir)
		}
		fmt.Fprintf(stdout, "  %s %-34s %6s   [%s..%s]\n",
			mark, d.Name, textutil.FormatNum(d.Value), textutil.FormatNum(d.Lo), textutil.FormatNum(d.Hi))
	}
	fmt.Fprintf(stdout, "  info words=%d sents=%d short%%=%s we/100w=%s so-chain=%s stiff=%d\n",
		r.Metrics.Words, r.Metrics.Sentences, textutil.FormatNum(r.Metrics.ShortSentPct),
		textutil.FormatNum(r.Metrics.WeRate), textutil.FormatNum(r.Metrics.SoChainRate), r.Metrics.FormalityFlags)
	return 0
}

func runCalibrate(file string, set *rules.Set, stdout, stderr io.Writer) int {
	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "voicecheck: can't read %s: %v\n", file, err)
		return 2
	}
	corpus := string(b)
	m := measure(set.SignatureLines, corpus)

	fmt.Fprintf(stdout, "CALIBRATION from %s (%s words, %s sentences)\n", file,
		textutil.FormatNum(float64(m.Words)), textutil.FormatNum(float64(m.Sentences)))
	fmt.Fprintln(stdout)
	for _, kv := range metricPairs(m) {
		fmt.Fprintf(stdout, "  %-20s %s\n", kv.key, textutil.FormatNum(kv.value))
	}

	fmt.Fprintln(stdout)
	ranges := Calibrate(corpus)
	if ranges == nil {
		fmt.Fprintln(stdout, "For ranges, separate at least 3 messages of 40 or more words with lines of ---.")
		return 0
	}
	n := len(qualifyingMessages(corpus))
	fmt.Fprintf(stdout, "Ranges from %d messages:\n", n)
	baseline := ranges["baseline"].(map[string]any)
	for _, sm := range ScoredMetrics {
		r := baseline[sm.Key].(map[string]any)
		fmt.Fprintf(stdout, "  %-20s %s..%s\n", sm.Key, textutil.FormatNum(r["lo"].(float64)), textutil.FormatNum(r["hi"].(float64)))
	}
	return 0
}

type metricPair struct {
	key   string
	value float64
}

// metricPairs lists every measure() field in the order voicecheck.mjs
// returns them, for the --calibrate CLI's full metric dump.
func metricPairs(m Metrics) []metricPair {
	return []metricPair{
		{"words", float64(m.Words)},
		{"sentences", float64(m.Sentences)},
		{"contraction_rate", m.ContractionRate},
		{"first_person_rate", m.FirstPersonRate},
		{"we_rate", m.WeRate},
		{"second_person_rate", m.SecondPersonRate},
		{"sent_len_mean", m.SentLenMean},
		{"sent_len_cv", m.SentLenCV},
		{"short_sent_pct", m.ShortSentPct},
		{"comma_chain_rate", m.CommaChainRate},
		{"so_chain_rate", m.SoChainRate},
		{"formality_flags", float64(m.FormalityFlags)},
	}
}
