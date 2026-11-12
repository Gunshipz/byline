package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"byline/internal/paths"
	"byline/internal/rules"
	"byline/internal/state"
	"byline/internal/textutil"
)

// flagRefusal is returned from byline_flag's UpdateMyRules change function
// to abort the save: the proposed rule would fire on the person's own
// writing (fixtures/my_real_voice.txt). The handler turns it into a
// normal (non-error) result instead of a tool error.
type flagRefusal struct {
	lines []map[string]any
}

func (e *flagRefusal) Error() string { return "fires on the person's own writing" }

// uIDRe matches a personal rule id: "U" followed by digits.
var uIDRe = regexp.MustCompile(`^U(\d+)$`)

func flagTool() Tool {
	return Tool{
		Name:        "byline_flag",
		Description: "Adds or extends a writing rule from one bad/good example: a brand-new personal rule, or another example on an existing one. Give the phrase, or for a pattern with no fixed phrase, a one-sentence test for spotting it.",
		Args: []Arg{
			{"phrase", "string", false},
			{"bad", "string", true},
			{"good", "string", true},
			{"rule_id", "string", false},
			{"family", "integer", false},
			{"name", "string", false},
			{"fix", "string", false},
			{"test", "string", false},
		},
		Handler: handleFlag,
	}
}

func handleFlag(args map[string]any) (any, error) {
	phrase, _ := args["phrase"].(string)
	phrase = strings.TrimSpace(phrase)
	test, _ := args["test"].(string)
	test = strings.TrimSpace(test)
	bad := args["bad"].(string)
	good := args["good"].(string)
	ruleID, hasRuleID := args["rule_id"].(string)

	// I4: a pattern with no fixed phrase is saved as a judgment test (a
	// rule with no strings), and only ever as a brand-new rule.
	switch {
	case phrase == "" && test == "":
		return nil, errors.New(`"phrase" or "test" is needed.`)
	case phrase != "" && test != "":
		return nil, errors.New(`Give "phrase" or "test", not both.`)
	case test != "" && hasRuleID:
		return nil, errors.New(`"test" adds a brand-new rule, so give "family" instead of "rule_id".`)
	case phrase != "" && !strings.Contains(strings.ToLower(bad), strings.ToLower(phrase)):
		return nil, fmt.Errorf("%q must contain the phrase %q.", "bad", phrase)
	case phrase != "" && strings.Contains(strings.ToLower(good), strings.ToLower(phrase)):
		return nil, fmt.Errorf("%q must not contain the phrase %q.", "good", phrase)
	}

	var family int
	var isCore bool
	var warning string
	if hasRuleID {
		// Gate finding M11: currentRules degrades an unreadable
		// my-rules.json to a warning and the core-only set, so checking
		// ruleID against that set would wrongly answer "There's no rule"
		// instead of naming the real, unreadable-file problem.
		if _, err := state.LoadMyRules(); err != nil {
			return nil, friendlyStateError(err)
		}
		set, w, err := currentRules()
		if err != nil {
			return nil, friendlyStateError(err)
		}
		warning = w
		core, terr := checkFlagTarget(set, ruleID)
		if terr != nil {
			return nil, terr
		}
		isCore = core
	} else {
		fv, ok := args["family"].(json.Number)
		if !ok {
			return nil, errors.New(`"family" (1 to 6) is needed to add a new rule.`)
		}
		n, _ := fv.Int64()
		family = int(n)
		if family < 1 || family > 6 {
			return nil, errors.New(`"family" must be 1 to 6.`)
		}
	}

	name, _ := args["name"].(string)
	fix, _ := args["fix"].(string)

	var resultID string
	var created, realVoiceChecked bool
	backup, err := state.UpdateMyRules(func(doc map[string]any) error {
		id, isNew := applyFlag(doc, ruleID, hasRuleID, isCore, phrase, test, bad, good, family, name, fix)
		resultID, created = id, isNew

		candidate, merr := json.Marshal(doc)
		if merr != nil {
			return merr
		}
		set, lerr := loadRules(candidate)
		if lerr != nil {
			return lerr
		}

		realVoice, rerr := os.ReadFile(paths.RealVoicePath())
		if rerr != nil {
			return nil
		}
		realVoiceChecked = true
		if lines := realVoiceHits(set, string(realVoice), resultID); len(lines) > 0 {
			return &flagRefusal{lines: lines}
		}
		return nil
	})

	if err != nil {
		var refusal *flagRefusal
		if errors.As(err, &refusal) {
			lines := refusal.lines
			if lines == nil {
				lines = []map[string]any{}
			}
			return withWarning(map[string]any{"saved": false, "reason": "fires_on_your_writing", "rule_id": resultID, "lines": lines}, warning), nil
		}
		return nil, friendlyStateError(err)
	}

	if _, err := rules.Generate(); err != nil {
		return nil, err
	}
	note := "flagged phrase: " + phrase
	if test != "" {
		note = "flagged pattern: " + test
	}
	if err := appendFailureLine(resultID, bad, note, !created); err != nil {
		return nil, err
	}

	return withWarning(map[string]any{
		"saved":              true,
		"rule_id":            resultID,
		"created":            created,
		"real_voice_checked": realVoiceChecked,
		"backup":             backup,
		"brief_stamp":        firstLineOf(paths.BriefPath()),
	}, warning), nil
}

// checkFlagTarget reports whether ruleID names a core rule (true) or a
// personal one (false), refusing with the plain sentence when it doesn't
// exist or is retired.
func checkFlagTarget(set *rules.Set, ruleID string) (isCore bool, err error) {
	for _, r := range set.Core() {
		if r.ID == ruleID {
			if r.Retired {
				return true, fmt.Errorf("Rule %q is retired, so it can't be extended.", ruleID)
			}
			return true, nil
		}
	}
	for _, r := range set.Rules {
		if r.ID == ruleID {
			if r.Retired {
				return false, fmt.Errorf("Rule %q is retired, so it can't be extended.", ruleID)
			}
			return false, nil
		}
	}
	return false, fmt.Errorf("There's no rule %q.", ruleID)
}

// applyFlag mutates doc for one byline_flag call: a brand-new personal
// rule, extending an existing personal rule, or extending a core rule
// through the overlay keys (add_strings, replace_examples, switch_off).
func applyFlag(doc map[string]any, ruleID string, hasRuleID, isCore bool, phrase, test, bad, good string, family int, name, fix string) (resultID string, created bool) {
	switch {
	case test != "":
		// I4: no strings, so the phrase check never fires on it; the brief
		// prints it as "name: test", the same as a core judgment rule.
		resultID = nextPersonalID(doc)
		created = true
		if name == "" {
			name = "Flagged pattern"
		}
		if fix == "" {
			fix = "Write it the way the good example does."
		}
		rule := map[string]any{"id": resultID, "family": family, "name": name, "ban": test, "test": test,
			"fix": fix, "bad": bad, "good": good, "flagged": time.Now().Format("2006-01-02")}
		list, _ := doc["add_rules"].([]any)
		doc["add_rules"] = append(list, rule)
	case !hasRuleID:
		resultID = nextPersonalID(doc)
		created = true
		rule := map[string]any{
			"id":      resultID,
			"family":  family,
			"name":    nameOrDefault(name, phrase),
			"ban":     phrase,
			"strings": []any{phrase},
			"fix":     fixOrDefault(fix, phrase),
			"bad":     bad,
			"good":    good,
			"flagged": time.Now().Format("2006-01-02"),
		}
		list, _ := doc["add_rules"].([]any)
		doc["add_rules"] = append(list, rule)
	case !isCore:
		resultID = ruleID
		list, _ := doc["add_rules"].([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok || m["id"] != ruleID {
				continue
			}
			strs, _ := m["strings"].([]any)
			if !containsFold(strs, phrase) {
				m["strings"] = append(strs, phrase)
			}
			if s, _ := m["bad"].(string); s == "" {
				m["bad"] = bad
			}
			if s, _ := m["good"].(string); s == "" {
				m["good"] = good
			}
			break
		}
	default:
		resultID = ruleID
		addStrings, _ := doc["add_strings"].(map[string]any)
		if addStrings == nil {
			addStrings = map[string]any{}
		}
		strs, _ := addStrings[ruleID].([]any)
		if !containsFold(strs, phrase) {
			strs = append(strs, phrase)
		}
		addStrings[ruleID] = strs
		doc["add_strings"] = addStrings

		replaceEx, _ := doc["replace_examples"].(map[string]any)
		if replaceEx == nil {
			replaceEx = map[string]any{}
		}
		if _, exists := replaceEx[ruleID]; !exists {
			replaceEx[ruleID] = map[string]any{"bad": bad, "good": good}
		}
		doc["replace_examples"] = replaceEx

		if switchOff, ok := doc["switch_off"].(map[string]any); ok {
			if list, ok := switchOff[ruleID].([]any); ok {
				kept := []any{}
				for _, item := range list {
					m, _ := item.(map[string]any)
					if s, _ := m["string"].(string); strings.EqualFold(s, phrase) {
						continue
					}
					kept = append(kept, item)
				}
				switchOff[ruleID] = kept
				doc["switch_off"] = switchOff
			}
		}
	}
	return resultID, created
}

// nextPersonalID returns "U<n>", n one more than the highest U<digits> id
// already in doc's add_rules (retired ones included).
func nextPersonalID(doc map[string]any) string {
	list, _ := doc["add_rules"].([]any)
	best := 0
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		if mm := uIDRe.FindStringSubmatch(id); mm != nil {
			if n, err := strconv.Atoi(mm[1]); err == nil && n > best {
				best = n
			}
		}
	}
	return fmt.Sprintf("U%d", best+1)
}

func nameOrDefault(name, phrase string) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("Flagged phrase: %q", phrase)
}

func fixOrDefault(fix, phrase string) string {
	if fix != "" {
		return fix
	}
	return fmt.Sprintf("Don't write %q; say the plain thing instead.", phrase)
}

// containsFold reports whether list (a decoded JSON array of strings)
// holds s, case-insensitively.
func containsFold(list []any, s string) bool {
	for _, item := range list {
		if str, ok := item.(string); ok && strings.EqualFold(str, s) {
			return true
		}
	}
	return false
}

// realVoiceHits returns up to 10 distinct lines (1-based, \r trimmed, cut
// at 300 characters) where set flags ruleID in text, in order.
func realVoiceHits(set *rules.Set, text, ruleID string) []map[string]any {
	violations := runLint(set, text)
	lines := strings.Split(textutil.Normalize(text), "\n")
	seen := map[int]bool{}
	var out []map[string]any
	for _, v := range violations {
		if v.RuleID != ruleID || seen[v.Line] {
			continue
		}
		seen[v.Line] = true
		line := ""
		if v.Line >= 1 && v.Line <= len(lines) {
			line = strings.TrimRight(lines[v.Line-1], "\r")
		}
		if len(line) > 300 {
			line = line[:300]
		}
		out = append(out, map[string]any{"line": v.Line, "text": line})
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// appendFailureLine appends one line to failures.jsonl, matching today's
// log shape.
func appendFailureLine(ruleID, text, note string, ruleExists bool) error {
	line := map[string]any{
		"date":        time.Now().Format("2006-01-02"),
		"rule":        ruleID,
		"text":        text,
		"layer":       "lint",
		"rule_exists": ruleExists,
		"note":        note,
	}
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(paths.FailuresPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
