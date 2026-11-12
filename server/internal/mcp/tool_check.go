package mcp

import "encoding/json"

func lintTool() Tool {
	return Tool{
		Name:        "byline_lint",
		Description: "Checks text against Byline's writing rules: banned strings and structural patterns.",
		Args:        []Arg{{"text", "string", true}},
		Handler: func(args map[string]any) (any, error) {
			set, warning, err := currentRules()
			if err != nil {
				return nil, friendlyStateError(err)
			}
			violations := runLint(set, args["text"].(string))
			var out any = violations
			if len(violations) == 0 {
				out = []any{}
			}
			return withWarning(map[string]any{"clean": len(violations) == 0, "violations": out}, warning), nil
		},
	}
}

func factlintTool() Tool {
	return Tool{
		Name:        "byline_factlint",
		Description: "Checks a draft's claims (numbers, counts, quotes, names) against its source, flagging ones the source doesn't support.",
		Args:        []Arg{{"draft", "string", true}, {"source", "string", true}},
		Handler: func(args map[string]any) (any, error) {
			set, warning, err := currentRules()
			if err != nil {
				return nil, friendlyStateError(err)
			}
			flags := runFactlint(set, args["draft"].(string), args["source"].(string))
			var out any = flags
			if len(flags) == 0 {
				out = []any{}
			}
			return withWarning(map[string]any{"clean": len(flags) == 0, "flags": out}, warning), nil
		},
	}
}

func draftcheckTool() Tool {
	return Tool{
		Name:        "byline_draftcheck",
		Description: "Checks a drafted comm against its spec: cut content revived, a section answering no reader question, over budget, or added persuasion.",
		Args:        []Arg{{"draft", "string", true}, {"spec", "object", true}},
		Handler: func(args map[string]any) (any, error) {
			specBytes, err := json.Marshal(args["spec"])
			if err != nil {
				return nil, err
			}
			r, err := runDraftcheck(args["draft"].(string), specBytes)
			if err != nil {
				return nil, err
			}
			checks := make([]map[string]any, len(r.Checks))
			for i, c := range r.Checks {
				checks[i] = map[string]any{"name": c.Name, "pass": c.Pass, "detail": c.Detail}
			}
			return map[string]any{
				"pass":            r.Pass,
				"reader":          r.Reader,
				"words":           r.Words,
				"checks":          checks,
				"receipt_written": r.Pass,
			}, nil
		},
	}
}

// notMeasuredMsg is voice/cli.go's own line for a Result.NotMeasured
// score (I3): never zeros presented as a score, before setup has run.
const notMeasuredMsg = "Byline hasn't measured this person's voice yet; setup does that."

func voicecheckTool() Tool {
	return Tool{
		Name:        "byline_voicecheck",
		Description: "Scores text against the person's voice target: contractions, I/you rate, sentence rhythm, comma chains.",
		Args:        []Arg{{"text", "string", true}},
		Handler: func(args map[string]any) (any, error) {
			set, warning, err := currentRules()
			if err != nil {
				return nil, friendlyStateError(err)
			}
			r := runVoiceScore(set, args["text"].(string))
			if r.NotMeasured {
				return withWarning(map[string]any{"not_measured": true, "message": notMeasuredMsg}, warning), nil
			}
			return withWarningOn(r, warning), nil
		},
	}
}
