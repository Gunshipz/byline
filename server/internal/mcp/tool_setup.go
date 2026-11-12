package mcp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"byline/internal/paths"
	"byline/internal/rules"
	"byline/internal/state"
	"byline/internal/textutil"
)

// needThreeMessagesMsg is returned, before anything is written, when a
// calibration corpus holds fewer than 3 messages of 40 or more words
// (voice.Calibrate's minCalibrateWords rule).
const needThreeMessagesMsg = "Byline needs at least 3 of your messages of 40 words or more to measure your voice, separated by lines of ---. Add more and try again."

// switchOff is one core rule's string a calibration corpus fires on: a
// candidate for byline_calibrate's preview, and for byline_setup_save to
// silence outright.
type switchOff struct {
	RuleID string `json:"rule_id"`
	String string `json:"string"`
	Count  int    `json:"count"`
}

// hit is one still-active personal rule a calibration corpus fires on,
// reported but never changed.
type hit struct {
	RuleID string `json:"rule_id"`
	Line   int    `json:"line"`
}

// measure scores a calibration corpus, shared by byline_calibrate and
// byline_setup_save: the word count of its messages (the --- separators
// excluded), the voice_target baseline voice.Calibrate derives from it,
// every core string-rule hit (one entry per distinct rule+string, with
// how many lines it hit), and every still-active personal rule hit. err
// is the three-messages sentence when there aren't at least 3 messages.
func measure(corpus string, set *rules.Set) (words int, target map[string]any, coreHits []switchOff, personalHits []hit, err error) {
	target = runVoiceCalibrate(corpus)
	if target == nil {
		return 0, nil, nil, nil, errors.New(needThreeMessagesMsg)
	}
	words = len(strings.Fields(messagesOnly(corpus)))

	coreIDs := map[string]bool{}
	for _, r := range set.Core() {
		coreIDs[r.ID] = true
	}

	violations := runLint(set, corpus)
	type key struct{ id, s string }
	var order []key
	counts := map[key]int{}
	seenPersonal := map[string]bool{}
	for _, v := range violations {
		if v.String == "" {
			continue // a structural hit, not a banned string
		}
		if coreIDs[v.RuleID] {
			k := key{v.RuleID, v.String}
			if _, ok := counts[k]; !ok {
				order = append(order, k)
			}
			counts[k]++
			continue
		}
		if !seenPersonal[v.RuleID] {
			personalHits = append(personalHits, hit{RuleID: v.RuleID, Line: v.Line})
			seenPersonal[v.RuleID] = true
		}
	}
	for _, k := range order {
		coreHits = append(coreHits, switchOff{RuleID: k.id, String: k.s, Count: counts[k]})
	}
	return words, target, coreHits, personalHits, nil
}

// messagesOnly strips the --- separator lines from a calibration corpus,
// for an accurate word count.
func messagesOnly(corpus string) string {
	var kept []string
	for _, l := range strings.Split(textutil.Normalize(corpus), "\n") {
		if strings.TrimSpace(l) == "---" {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

// refuseIfNewerFormat reports state.ErrNewerFormat (the lead ruling,
// 2026-11-10) as the same plain sentence state already uses elsewhere
// for it, before byline_calibrate or byline_setup_save ever calls
// currentRules -- which, like loadSet, treats a newer-format my-rules.json
// as just another broken file and quietly degrades to core rules only,
// losing the distinction a refusal needs. Any other error is nil here;
// the caller still goes on to currentRules for that.
func refuseIfNewerFormat() error {
	if _, err := state.LoadMyRules(); errors.Is(err, state.ErrNewerFormat) {
		return friendlyStateError(err)
	}
	return nil
}

// refuseIfRulesUnreadable reports any LoadMyRules error at all (gate
// finding I3), not just a newer format: byline_setup_save must check
// my-rules.json can be read before it writes a single file, unlike
// byline_calibrate's read-only preview (refuseIfNewerFormat above), which
// still falls back like every other checker tool for a plain unreadable
// file.
func refuseIfRulesUnreadable() error {
	if _, err := state.LoadMyRules(); err != nil {
		return friendlyStateError(err)
	}
	return nil
}

func calibrateTool() Tool {
	return Tool{
		Name:        "byline_calibrate",
		Description: "Previews what setup would learn from a sample of the person's own messages, without changing anything.",
		Args:        []Arg{{"corpus", "string", true}},
		Handler: func(args map[string]any) (any, error) {
			if err := refuseIfNewerFormat(); err != nil {
				return nil, err
			}
			set, warning, err := currentRules()
			if err != nil {
				return nil, friendlyStateError(err)
			}
			words, target, coreHits, _, err := measure(args["corpus"].(string), set)
			if err != nil {
				return nil, err
			}
			if coreHits == nil {
				coreHits = []switchOff{}
			}
			return withWarning(map[string]any{"words": words, "voice_target": target, "would_switch_off": coreHits}, warning), nil
		},
	}
}

func setupSaveTool() Tool {
	return Tool{
		Name:        "byline_setup_save",
		Description: "Saves setup's writing profile, sample messages and real-voice sample, and silences any core rule the person's own messages trip.",
		Args: []Arg{
			{"profile", "string", true},
			{"exemplars", "string", true},
			{"real_voice", "string", true},
			{"signature_lines", "array", false},
		},
		Handler: handleSetupSave,
	}
}

func handleSetupSave(args map[string]any) (any, error) {
	if err := refuseIfRulesUnreadable(); err != nil {
		return nil, err
	}

	profile := args["profile"].(string)
	exemplars := args["exemplars"].(string)
	realVoice := args["real_voice"].(string)

	signatureLines, err := parseSignatureLines(args["signature_lines"])
	if err != nil {
		return nil, err
	}

	set, warning, err := currentRules()
	if err != nil {
		return nil, friendlyStateError(err)
	}
	words, target, coreHits, personalHits, err := measure(realVoice, set)
	if err != nil {
		return nil, err
	}

	var backups []string
	for _, wf := range []struct{ path, data string }{
		{paths.ProfilePath(), profile},
		{paths.ExemplarsPath(), exemplars},
		{paths.RealVoicePath(), realVoice},
	} {
		b, werr := state.WriteVoiceFile(wf.path, []byte(wf.data))
		if werr != nil {
			return nil, werr
		}
		if b != "" {
			backups = append(backups, b)
		}
	}

	var switchedOff []switchOffAdded
	rulesBackup, err := state.UpdateMyRules(func(doc map[string]any) error {
		switchedOff = applySetupSave(doc, target, coreHits, signatureLines)
		return nil
	})
	if err != nil {
		return nil, friendlyStateError(err)
	}
	if rulesBackup != "" {
		backups = append(backups, rulesBackup)
	}

	if _, err := rules.Generate(); err != nil {
		return nil, err
	}

	if personalHits == nil {
		personalHits = []hit{}
	}
	if backups == nil {
		backups = []string{}
	}
	if switchedOff == nil {
		switchedOff = []switchOffAdded{}
	}
	return withWarning(map[string]any{
		"words":              words,
		"voice_target":       target,
		"personal_rule_hits": personalHits,
		"backups":            backups,
		"saved":              []string{paths.ProfilePath(), paths.ExemplarsPath(), paths.RealVoicePath(), paths.MyRulesPath()},
		"switched_off":       switchedOff,
		"brief_stamp":        firstLineOf(paths.BriefPath()),
	}, warning), nil
}

// parseSignatureLines validates the optional signature_lines argument:
// absent means "leave it alone" (nil); present means a list of strings,
// each non-empty after trimming.
func parseSignatureLines(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	if !isStringList(v) {
		return nil, errors.New(`"signature_lines" must be a list of strings.`)
	}
	list := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		s := item.(string)
		if strings.TrimSpace(s) == "" {
			return nil, errors.New(`"signature_lines" entries must not be blank.`)
		}
		out = append(out, s)
	}
	return out, nil
}

// switchOffAdded is one switch-off a setup save actually appended to
// my-rules.json, for the save result's switched_off (gate finding I4):
// every core hit the corpus trips, minus the ones already silenced from
// an earlier save.
type switchOffAdded struct {
	RuleID string `json:"rule_id"`
	String string `json:"string"`
}

// applySetupSave sets each key of target into doc's voice_target (other
// keys stay), switches off every core hit not already silenced (today's
// reason), and, when given, replaces doc's signature_lines. It returns
// the switch-offs it actually appended, so the save result (gate finding
// I4) never has to guess from calibrate's possibly-stale preview.
func applySetupSave(doc map[string]any, target map[string]any, coreHits []switchOff, signatureLines []string) []switchOffAdded {
	vt, _ := doc["voice_target"].(map[string]any)
	if vt == nil {
		vt = map[string]any{}
	}
	for k, v := range target {
		vt[k] = v
	}
	doc["voice_target"] = vt

	off, _ := doc["switch_off"].(map[string]any)
	if off == nil {
		off = map[string]any{}
	}
	today := time.Now().Format("2006-01-02")
	var added []switchOffAdded
	for _, h := range coreHits {
		list, _ := off[h.RuleID].([]any)
		if switchOffHasString(list, h.String) {
			continue
		}
		list = append(list, map[string]any{
			"string": h.String,
			"reason": fmt.Sprintf("You write this yourself (found in your own messages at setup, %s).", today),
		})
		off[h.RuleID] = list
		added = append(added, switchOffAdded{RuleID: h.RuleID, String: h.String})
	}
	doc["switch_off"] = off

	if signatureLines != nil {
		arr := make([]any, len(signatureLines))
		for i, s := range signatureLines {
			arr[i] = s
		}
		doc["signature_lines"] = arr
	}
	return added
}

func switchOffHasString(list []any, s string) bool {
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if str, _ := m["string"].(string); strings.EqualFold(str, s) {
			return true
		}
	}
	return false
}
