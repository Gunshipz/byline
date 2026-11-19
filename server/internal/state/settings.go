package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"byline/internal/paths"
)

// Settings is the person's saved preferences, under BYLINE_HOME.
//
// There is no telemetry setting (gate review I5): nothing read the one
// that used to live here, so saving it told a person who opted out that
// usage counting was off while it carried on. If this copy counts usage
// at all, its own off switch lives outside these settings; they never
// claim to control it.
type Settings struct {
	Format            int      `json:"format"`
	SendTools         []string `json:"send_tools"`
	ExemptChannels    []string `json:"exempt_channels"`
	ExtraDraftPhrases []string `json:"extra_draft_phrases"`
}

// DefaultSendTools is declared in copy_specific.go: the tools Byline's
// send checks always cover differ by copy.

var (
	toolNameRe  = regexp.MustCompile(`^mcp__[A-Za-z0-9_.-]+__[A-Za-z0-9_.-]+$`)
	channelIDRe = regexp.MustCompile(`^[CDG][A-Z0-9]{8,12}$`)
)

// LoadSettings reads settings.json, applying a default for any missing
// key. SendTools is always DefaultSendTools plus the file's extra entries,
// so no hand edit can drop a default.
func LoadSettings() (Settings, error) {
	doc, err := readSettingsDoc()
	if err != nil {
		return Settings{}, err
	}
	return settingsFromDoc(doc), nil
}

// ValidateSettings rejects a settings value a person could never have
// saved through the checks above: a dropped default send tool, a
// malformed tool name or channel id, or a blank or over-long draft phrase.
func ValidateSettings(s Settings) error {
	for _, d := range DefaultSendTools {
		if !slices.Contains(s.SendTools, d) {
			return fmt.Errorf(`Byline can't remove %q from the send tools; its checks always cover it.`, d)
		}
	}
	for _, t := range s.SendTools {
		if !toolNameRe.MatchString(t) {
			return fmt.Errorf("%q isn't a tool name.", t)
		}
	}
	for _, c := range s.ExemptChannels {
		if !channelIDRe.MatchString(c) {
			return fmt.Errorf("%q isn't a Slack channel id.", c)
		}
	}
	for _, p := range s.ExtraDraftPhrases {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("%q is empty.", p)
		}
		if len(p) > 200 {
			return fmt.Errorf("%q is over 200 characters.", p)
		}
	}
	return nil
}

// settingsRaceHookForTests, when set, runs once, right after SaveSettings
// reads settings.json and before it writes back -- test-only, standing
// in for a second session saving at the worst possible moment (M10).
var settingsRaceHookForTests func()

// SaveSettings sets only the keys in patch, keeps every other key in the
// file, validates the result, backs the file up, then writes it. If the
// file on disk changed between the read and the write -- another
// session saved first, the same race UpdateMyRules already guards (M10)
// -- it starts over, up to 3 tries. It returns the effective settings
// and the backup path.
func SaveSettings(patch map[string]any) (Settings, string, error) {
	path := paths.SettingsPath()
	const maxTries = 3
	for try := 1; ; try++ {
		raw, err := readSettingsRaw(path)
		if err != nil {
			return Settings{}, "", err
		}
		doc, err := decodeSettingsDoc(raw)
		if err != nil {
			return Settings{}, "", err
		}
		if hook := settingsRaceHookForTests; hook != nil {
			settingsRaceHookForTests = nil
			hook()
		}
		for k, v := range patch {
			doc[k] = v
		}
		s := settingsFromDoc(doc)
		if err := ValidateSettings(s); err != nil {
			return Settings{}, "", err
		}
		doc["format"] = Format
		doc["send_tools"] = toAnySlice(s.SendTools)
		doc["exempt_channels"] = toAnySlice(s.ExemptChannels)
		doc["extra_draft_phrases"] = toAnySlice(s.ExtraDraftPhrases)
		delete(doc, "telemetry") // I5: dropped entirely, never written back
		data, err := encodeDoc(doc)
		if err != nil {
			return Settings{}, "", err
		}

		cur, err := readSettingsRaw(path)
		if err != nil {
			return Settings{}, "", err
		}
		if !bytes.Equal(cur, raw) {
			if try >= maxTries {
				return Settings{}, "", errors.New("Your settings kept changing while saving. Try again.")
			}
			continue
		}

		backup, err := Backup(path)
		if err != nil {
			return Settings{}, "", err
		}
		if err := WriteFileAtomic(path, data, 0o600); err != nil {
			return s, backup, err
		}
		return s, backup, nil
	}
}

// readSettingsDoc reads settings.json (an empty doc if it doesn't exist
// yet) with UseNumber, and refuses one saved by a newer version.
func readSettingsDoc() (map[string]any, error) {
	raw, err := readSettingsRaw(paths.SettingsPath())
	if err != nil {
		return nil, err
	}
	return decodeSettingsDoc(raw)
}

// readSettingsRaw reads path's raw bytes, nil (not an error) if it
// doesn't exist yet. Split out from decodeSettingsDoc so SaveSettings
// can compare bytes across its own read and its re-read before writing
// (M10).
func readSettingsRaw(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

// decodeSettingsDoc decodes raw (nil meaning the file doesn't exist yet,
// read as an empty document) with UseNumber, and refuses one saved by a
// newer version.
func decodeSettingsDoc(raw []byte) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	// A Windows editor may save a leading UTF-8 BOM.
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("Byline can't read settings.json (%w).", err)
	}
	if doc == nil {
		// "null", or any other JSON value that isn't an object, decodes
		// with no error but leaves doc nil; writing into a nil map
		// later panics (M1), so it's unreadable here instead.
		return nil, fmt.Errorf("Byline can't read settings.json (not a JSON object).")
	}
	// Content left after the object -- a hand edit that closed it too
	// early -- means the file is unreadable (I2), not just that one
	// read happened to stop early; trailing whitespace is fine.
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("Byline can't read settings.json (content after the first JSON value).")
	}
	if f, ok := doc["format"]; ok {
		n, ok := f.(json.Number)
		if !ok {
			return nil, fmt.Errorf("settings.json's format field isn't a number")
		}
		iv, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("settings.json's format field isn't an integer")
		}
		if int(iv) > Format {
			return nil, ErrNewerFormat
		}
	}
	return doc, nil
}

// settingsFromDoc extracts Settings from a decoded document, applying
// every default for a missing key.
func settingsFromDoc(doc map[string]any) Settings {
	s := Settings{Format: Format}
	s.SendTools = expandSendTools(toStringSlice(doc["send_tools"]))
	s.ExemptChannels = toStringSlice(doc["exempt_channels"])
	s.ExtraDraftPhrases = toStringSlice(doc["extra_draft_phrases"])
	return s
}

// expandSendTools always returns DefaultSendTools plus any entry in extra
// that isn't already a default, so no edit can drop one.
func expandSendTools(extra []string) []string {
	out := append([]string{}, DefaultSendTools...)
	for _, e := range extra {
		if !slices.Contains(DefaultSendTools, e) {
			out = append(out, e)
		}
	}
	return out
}

// toStringSlice always returns a non-nil slice, even for a missing key,
// so a missing list reads as [] rather than encoding back out as JSON
// null (M5).
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return make([]string, 0)
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// encodeDoc marshals a voice-folder document without HTML-escaping, with
// two-space indent.
func encodeDoc(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
