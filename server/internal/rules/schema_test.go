package rules

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func synth(t *testing.T) *Core {
	t.Helper()
	c, err := LoadCore(os.DirFS("testdata/synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestObject_KeepsKeyOrder(t *testing.T) {
	in := `{"b":1,"a":{"y":2,"x":3}}`
	var o Object
	if err := json.Unmarshal([]byte(in), &o); err != nil || o[0].Key != "b" || o[1].Key != "a" {
		t.Fatalf("%v %v", err, o)
	}
	inner, _ := DecodeObject(o[1].Value)
	if inner[0].Key != "y" || inner[1].Key != "x" {
		t.Fatalf("inner %v", inner)
	}
	if out, _ := json.Marshal(o); string(out) != in {
		t.Fatalf("round trip %s", out)
	}
}

func TestParseCore_AcceptsTodaysRulesShape(t *testing.T) {
	c, err := ParseCore([]byte(`{"$schema":"x","version":"1.0.0","canonical":true,"meta":{},
	 "families":[{"id":1,"name":"F","test":"T","principle":"P"}],
	 "rules":[{"id":"K1","family":1,"name":"N","ban":"b","strings":[],"fix":"f","bad":"x","good":"y","flagged":null}],
	 "voice_target":{"note":"n"}}`))
	if err != nil || c.Format != 0 || c.Rules[0].Flagged != "" || c.Families[0].Principle != "P" {
		t.Fatalf("%v %+v", err, c)
	}
}

func TestValidateCore_Errors(t *testing.T) {
	for _, c := range []struct {
		edit func(*Core)
		want string
	}{
		{func(c *Core) { c.Format = 0 }, "core rules: format 0, want 1"},
		{func(c *Core) { c.Version = "" }, "core rules: no version"},
		{func(c *Core) { c.Families = append(c.Families, c.Families[0]) }, "core rules: family 1 appears twice"},
		{func(c *Core) { c.Rules = append(c.Rules, c.Rules[0]) }, `core rules: rule "K1" appears twice`},
		{func(c *Core) { c.Rules[0].Family = 9 }, `rule "K1": family 9 does not exist`},
		{func(c *Core) { c.Rules[0].Structural = "vibes" }, `rule "K1": unknown structural check "vibes"`},
		{func(c *Core) { c.Rules[0].Strings = []string{""} }, `rule "K1": empty string`},
		{func(c *Core) { c.Rules[0].Ban = "" }, `rule "K1": no ban`},
		{func(c *Core) { c.Rules[0].ID = "K 1" }, `rule "K 1": id must be a letter followed by letters, digits, _ or -`},
		{func(c *Core) { c.Rules[0].ID = "U1" }, `core rules: rule "U1": ids starting with U are kept for personal rules`},
		{func(c *Core) { c.VoiceTarget = Object{{Key: "contractions", Value: json.RawMessage(`"x"`)}} },
			`voice_target contractions: needs "rule" and "examples"`},
		{func(c *Core) {
			c.VoiceTarget = Object{{Key: "baseline", Value: json.RawMessage(`{"vibes":{"lo":1,"hi":2}}`)}}
		},
			`voice_target baseline: unknown metric "vibes"`},
		// M7 (gate-p12-p5-review.md): "register": 5 passed validation and
		// printed blank, because brief.go's register/person/sentences
		// cases silently drop a non-string value.
		{func(c *Core) { c.VoiceTarget = Object{{Key: "register", Value: json.RawMessage(`5`)}} },
			`voice_target register: must be a string`},
		{func(c *Core) { c.VoiceTarget = Object{{Key: "person", Value: json.RawMessage(`["I"]`)}} },
			`voice_target person: must be a string`},
		{func(c *Core) { c.VoiceTarget = Object{{Key: "sentences", Value: json.RawMessage(`true`)}} },
			`voice_target sentences: must be a string`},
	} {
		core := synth(t)
		c.edit(core)
		if err := ValidateCore(core); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, got %v", c.want, err)
		}
	}
}

func TestLoadCore_Synthetic(t *testing.T) {
	c := synth(t)
	if len(c.Rules) != 15 || len(c.Families) != 6 || c.Version != "0.9.0" {
		t.Fatalf("%d rules, %d families, %q", len(c.Rules), len(c.Families), c.Version)
	}
}

func TestRepoCoreRules_IsValid(t *testing.T) {
	c, err := LoadCore(os.DirFS("../../../rules"))
	if err != nil || len(c.Families) != 6 {
		t.Fatalf("%v", err)
	}
}
