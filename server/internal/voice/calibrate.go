package voice

import (
	"math"
	"sort"
	"strings"

	"byline/internal/textutil"
)

// minCalibrateWords is I2's floor (gate-p12-p5-review.md): a message under
// this many words is too short to calibrate from, since one short reply
// ("Thanks, will do.") could otherwise drag a whole metric's range to 0.
const minCalibrateWords = 40

// Calibrate derives baseline ranges from a corpus of a person's own
// messages, separated by lines that are exactly "---". It measures with no
// signature lines (Requests, R4), since a calibration corpus has none to
// cut. Only messages of 40 or more words count (minCalibrateWords); it
// returns nil when fewer than 3 of those qualify.
func Calibrate(corpus string) map[string]any {
	measured := qualifyingMessages(corpus)
	if len(measured) < 3 {
		return nil
	}

	baseline := map[string]any{}
	for _, sm := range ScoredMetrics {
		vals := make([]float64, len(measured))
		for i, m := range measured {
			vals[i] = metricValue(m, sm.Key)
		}
		sort.Float64s(vals)
		lo := textutil.ToFixed(percentile(vals, 10)*0.9, 2)
		hi := textutil.ToFixed(percentile(vals, 90)*1.1, 2)
		baseline[sm.Key] = map[string]any{"lo": lo, "hi": hi}
	}
	return map[string]any{"baseline": baseline}
}

// qualifyingMessages splits corpus into its messages and measures the ones
// with minCalibrateWords or more words; the rest are too short to
// calibrate from.
func qualifyingMessages(corpus string) []Metrics {
	var out []Metrics
	for _, msg := range splitMessages(corpus) {
		m := measure(nil, msg)
		if m.Words >= minCalibrateWords {
			out = append(out, m)
		}
	}
	return out
}

// percentile returns the p-th percentile (0-100) of sorted, a slice
// already in ascending order, by linear interpolation between the two
// nearest ranks.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// splitMessages splits corpus into trimmed, non-empty messages on lines
// that are exactly "---" after trimming.
func splitMessages(corpus string) []string {
	normalized := textutil.Normalize(corpus)
	var messages []string
	var cur []string
	flush := func() {
		msg := strings.TrimSpace(strings.Join(cur, "\n"))
		if msg != "" {
			messages = append(messages, msg)
		}
		cur = nil
	}
	for _, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return messages
}
