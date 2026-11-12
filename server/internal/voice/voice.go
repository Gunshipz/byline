// Package voice ports voicecheck.mjs: deterministic style metrics scored
// against a baseline, calibrated from a person's own writing.
package voice

import (
	"math"
	"regexp"
	"strings"

	"byline/internal/rules"
	"byline/internal/textutil"
)

// Metrics holds voicecheck's deterministic style measurements for one text.
type Metrics struct {
	Words            int     `json:"words"`
	Sentences        int     `json:"sentences"`
	ContractionRate  float64 `json:"contraction_rate"`
	FirstPersonRate  float64 `json:"first_person_rate"`
	WeRate           float64 `json:"we_rate"`
	SecondPersonRate float64 `json:"second_person_rate"`
	SentLenMean      float64 `json:"sent_len_mean"`
	SentLenCV        float64 `json:"sent_len_cv"`
	ShortSentPct     float64 `json:"short_sent_pct"`
	CommaChainRate   float64 `json:"comma_chain_rate"`
	SoChainRate      float64 `json:"so_chain_rate"`
	FormalityFlags   int     `json:"formality_flags"`
}

// Range is the acceptable [Lo, Hi] band for one metric.
type Range struct {
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

// Baseline maps a metric key to its acceptable Range.
type Baseline map[string]Range

// ScoredMetrics names the metrics Score checks against a Baseline, in
// BASELINE's key order, with the display name voicecheck.mjs gives each.
var ScoredMetrics = []struct{ Key, Name string }{
	{"contraction_rate", "contractions /100w"},
	{"first_person_rate", "I/my /100w"},
	{"sent_len_mean", "mean words/sentence"},
	{"sent_len_cv", "sentence-length CV (rhythm)"},
	{"second_person_rate", "you/your /100w"},
	{"comma_chain_rate", "frac sentences w/ 2+ commas"},
}

// Detail is one scored metric's result against its range.
type Detail struct {
	Metric string  `json:"metric"`
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Lo     float64 `json:"lo"`
	Hi     float64 `json:"hi"`
	OK     bool    `json:"ok"`
	Dir    string  `json:"dir"`
}

// Result is a draft's voice score: its Metrics, the score out of 10, how
// many scored metrics were in range, and the per-metric Detail. NotMeasured
// is true when set carries no voice_target.baseline at all (I3,
// gate-p12-p5-review.md): setup hasn't calibrated this person's voice yet,
// so there is nothing to score against, and VoiceScore, InRange, Total,
// Penalty and Detail stay at their zero values.
type Result struct {
	Metrics     Metrics  `json:"metrics"`
	NotMeasured bool     `json:"not_measured"`
	VoiceScore  float64  `json:"voice_score"`
	InRange     int      `json:"inRange"`
	Total       int      `json:"total"`
	Penalty     int      `json:"penalty"`
	Detail      []Detail `json:"detail"`
}

// Score measures text with set's signature lines, then scores it against
// set's own baseline (set.VoiceTarget()["baseline"]) and penalizes
// formality flags straight off the top. With no baseline, it returns a
// NotMeasured result: there is no shipped fallback to score against
// instead (I3) -- setup must calibrate (or dev split must carry forward)
// this person's own baseline first.
func Score(set *rules.Set, text string) Result {
	m := measure(set.SignatureLines, text)
	baseline, ok := ownBaseline(set)
	if !ok {
		return Result{Metrics: m, NotMeasured: true}
	}

	var inRange, total int
	var detail []Detail
	for _, sm := range ScoredMetrics {
		b, has := baseline[sm.Key]
		if !has {
			continue
		}
		total++
		v := metricValue(m, sm.Key)
		inr := v >= b.Lo && v <= b.Hi
		if inr {
			inRange++
		}
		dir := "ok"
		switch {
		case v < b.Lo:
			dir = "BELOW"
		case v > b.Hi:
			dir = "ABOVE"
		}
		detail = append(detail, Detail{Metric: sm.Key, Name: sm.Name, Value: v, Lo: b.Lo, Hi: b.Hi, OK: inr, Dir: dir})
	}
	if total == 0 {
		return Result{Metrics: m, NotMeasured: true}
	}

	penalty := m.FormalityFlags
	score := textutil.ToFixed(10*float64(inRange)/float64(total)-float64(penalty), 1)
	return Result{Metrics: m, VoiceScore: score, InRange: inRange, Total: total, Penalty: penalty, Detail: detail}
}

// ownBaseline decodes set's own voice_target.baseline, reporting false
// when set carries none (or something that isn't a baseline object): there
// is then nothing to fall back to (I3).
func ownBaseline(set *rules.Set) (Baseline, bool) {
	raw, ok := set.VoiceTarget()["baseline"]
	if !ok {
		return nil, false
	}
	bm, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	out := Baseline{}
	for _, sm := range ScoredMetrics {
		rv, ok := bm[sm.Key]
		if !ok {
			continue
		}
		rm, ok := rv.(map[string]any)
		if !ok {
			continue
		}
		lo, _ := rm["lo"].(float64)
		hi, _ := rm["hi"].(float64)
		out[sm.Key] = Range{Lo: lo, Hi: hi}
	}
	return out, true
}

// metricValue returns m's value for one of ScoredMetrics' keys.
func metricValue(m Metrics, key string) float64 {
	switch key {
	case "contraction_rate":
		return m.ContractionRate
	case "first_person_rate":
		return m.FirstPersonRate
	case "sent_len_mean":
		return m.SentLenMean
	case "sent_len_cv":
		return m.SentLenCV
	case "second_person_rate":
		return m.SecondPersonRate
	case "comma_chain_rate":
		return m.CommaChainRate
	}
	return 0
}

var (
	headerRe  = regexp.MustCompile(`(?im)^(?:[ \t]*(?:To|Cc|CC|Bcc|Subject|From)[ \t]*:.*\n?)+`)
	headingRe = regexp.MustCompile(`(?m)^#{1,6}\s.*$`)
	tableRe   = regexp.MustCompile(`(?m)^\s*\|.*\|\s*$`)
	bulletRe  = regexp.MustCompile(`(?m)^\s*[-*]\s.*$`)
	wordsRe   = regexp.MustCompile(`\s+`)

	contractionsRe = regexp.MustCompile(`(?i)\b(?:I'm|I'd|I've|I'll|we're|we'd|we've|we'll|don't|doesn't|didn't|won't|can't|couldn't|shouldn't|wouldn't|isn't|aren't|wasn't|weren't|it's|that's|there's|here's|what's|let's|you're|you've|you'd|they're|hasn't|haven't|hadn't|nobody's|who's)\b`)
	firstPersonRe  = regexp.MustCompile(`(?i)\b(?:I|I'm|I'd|I've|I'll|my|me)\b`)
	weRe           = regexp.MustCompile(`(?i)\b(?:we|our|us|we're|we'd|we've|we'll)\b`)
	secondPersonRe = regexp.MustCompile(`(?i)\b(?:you|your|you're|you've|you'd)\b`)
	soChainRe      = regexp.MustCompile(`(?i),\s+so\s`)

	stiffRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bin order to\b`),
		regexp.MustCompile(`(?i)\bit is important to\b`),
		regexp.MustCompile(`(?i)\bplease be advised\b`),
		regexp.MustCompile(`(?i)\bshould you have any\b`),
		regexp.MustCompile(`(?i)\bat your earliest convenience\b`),
		regexp.MustCompile(`(?i)\bI am writing to\b`),
		regexp.MustCompile(`(?i)\bkindly\b`),
		regexp.MustCompile(`(?i)\bthe aforementioned\b`),
		regexp.MustCompile(`(?i)\bwith regard to\b`),
		regexp.MustCompile(`(?i)\bpursuant to\b`),
		regexp.MustCompile(`(?i)\bit is recommended that\b`),
	}
)

// stripPlumbing removes header lines, cuts everything from the signature
// on, then removes headings, table rows and bullets, as voicecheck.mjs does.
func stripPlumbing(sig []string, t string) string {
	t = headerRe.ReplaceAllString(t, "")
	t = cutSignature(sig, t)
	t = headingRe.ReplaceAllString(t, "")
	t = tableRe.ReplaceAllString(t, "")
	t = bulletRe.ReplaceAllString(t, "")
	return t
}

// cutSignature drops everything from the first line starting with one of
// sig's lines, "Manager, " or "Sr. Manager, " onward. An empty entry is
// skipped (M6, gate-p12-p5-review.md): a sign-off block often has a blank
// line, and an empty alternative in the regex below would otherwise match
// at the very first newline in the draft.
func cutSignature(sig []string, t string) string {
	alts := make([]string, 0, len(sig)+2)
	for _, s := range sig {
		if s == "" {
			continue
		}
		alts = append(alts, regexp.QuoteMeta(s))
	}
	alts = append(alts, `Manager,[ \t]`, `Sr\.? Manager,[ \t]`)
	re := regexp.MustCompile(`(?m)\n[ \t]*(?:` + strings.Join(alts, "|") + `)`)
	if loc := re.FindStringIndex(t); loc != nil {
		return t[:loc[0]]
	}
	return t
}

// measure is voicecheck.mjs's measure(), unexported since Score, Calibrate
// and the CLI are the only callers besides the tests.
func measure(sig []string, raw string) Metrics {
	t := stripPlumbing(sig, textutil.Normalize(raw))

	w := 0
	for _, s := range wordsRe.Split(t, -1) {
		if s != "" {
			w++
		}
	}
	if w == 0 {
		w = 1
	}
	wf := float64(w)
	per100 := func(n int) float64 { return textutil.ToFixed(100*float64(n)/wf, 2) }

	var sents []string
	var lens []int
	for _, p := range textutil.SplitSentences(t) {
		s := strings.TrimSpace(p.Text)
		ws := wordsRe.Split(s, -1)
		if len(ws) >= 3 {
			sents = append(sents, s)
			lens = append(lens, len(ws))
		}
	}

	var mean float64
	if len(lens) > 0 {
		sum := 0
		for _, l := range lens {
			sum += l
		}
		mean = float64(sum) / float64(len(lens))
	}
	var sd float64
	if len(lens) > 1 {
		var sq float64
		for _, l := range lens {
			d := float64(l) - mean
			sq += d * d
		}
		sd = math.Sqrt(sq / float64(len(lens)-1))
	}
	var cv float64
	if mean != 0 {
		cv = sd / mean
	}

	shortCount := 0
	for _, l := range lens {
		if l < 8 {
			shortCount++
		}
	}
	lensDenom := len(lens)
	if lensDenom == 0 {
		lensDenom = 1
	}

	commaCount := 0
	for _, s := range sents {
		if strings.Count(s, ",") >= 2 {
			commaCount++
		}
	}
	sentsDenom := len(sents)
	if sentsDenom == 0 {
		sentsDenom = 1
	}

	formality := 0
	for _, re := range stiffRes {
		formality += len(re.FindAllString(t, -1))
	}

	return Metrics{
		Words:            w,
		Sentences:        len(sents),
		ContractionRate:  per100(len(contractionsRe.FindAllString(t, -1))),
		FirstPersonRate:  per100(len(firstPersonRe.FindAllString(t, -1))),
		WeRate:           per100(len(weRe.FindAllString(t, -1))),
		SecondPersonRate: per100(len(secondPersonRe.FindAllString(t, -1))),
		SentLenMean:      textutil.ToFixed(mean, 1),
		SentLenCV:        textutil.ToFixed(cv, 2),
		ShortSentPct:     textutil.ToFixed(100*float64(shortCount)/float64(lensDenom), 0),
		CommaChainRate:   textutil.ToFixed(float64(commaCount)/float64(sentsDenom), 2),
		SoChainRate:      per100(len(soChainRe.FindAllString(t, -1))),
		FormalityFlags:   formality,
	}
}
