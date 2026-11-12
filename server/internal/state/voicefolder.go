package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"byline/embed"
	"byline/internal/paths"
	"byline/internal/rules"
	"byline/internal/version"
)

// UnreadableError means a voice-folder JSON file can't be understood: it
// isn't valid JSON, isn't a JSON object, or its format field isn't a whole
// number. Byline leaves a file like this untouched rather than risk
// destroying content it can't parse.
type UnreadableError struct {
	Path string
	Err  error
}

func (e *UnreadableError) Error() string { return fmt.Sprintf("can't read %s (%s)", e.Path, e.Err) }
func (e *UnreadableError) Unwrap() error { return e.Err }

// voiceTemplates maps each template embed.Files ships under templates/ to
// where it installs under the voice folder.
var voiceTemplates = []struct {
	name string
	path func() string
}{
	{"profile.md", paths.ProfilePath},
	{"exemplars.md", paths.ExemplarsPath},
	{"my-rules.json", paths.MyRulesPath},
	{"my-stance.md", paths.MyStancePath},
	{"settings.json", paths.SettingsPath},
}

// EnsureVoiceFolder makes the voice folder and its subdirectories, installs
// any template that isn't there yet, and regenerates the composition brief
// when it's stale. Never panics; nil when all is well, else errors.Join of
// plain problem sentences. When the folder itself can't be made, every
// subfolder underneath fails the same way, and installTemplate's raw
// open errors would pile on top of that; rather than let all of them
// stack into unreadable noise, this returns the first folder-creation
// failure immediately, as one plain sentence (gate re-review residual).
func EnsureVoiceFolder() error {
	for _, d := range []string{
		paths.Home(),
		paths.ReceiptsDir(),
		paths.StateDir(),
		paths.BackupsDir(),
		filepath.Dir(paths.RealVoicePath()),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("Byline can't create its folder at %s (%w).", d, err)
		}
	}

	var errs []error
	for _, tpl := range voiceTemplates {
		if err := installTemplate(tpl.name, tpl.path()); err != nil {
			errs = append(errs, err)
		}
	}

	skip, err := checkMyRulesReadable()
	if err != nil {
		errs = append(errs, err)
	}
	if !skip {
		stale, err := briefIsStale()
		if err != nil {
			errs = append(errs, err)
		} else if stale {
			if _, err := rules.Generate(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// installTemplate writes embed.Files' templates/name to path if path
// doesn't exist yet; an existing file, even an empty one, is never
// touched. If the write or close fails partway (M9: a full disk, say),
// the file this call just created (O_EXCL proves nothing else is racing
// it) is removed, not left behind as a zero-byte file that the next
// start would then mistake for one the person made on purpose and skip
// forever.
func installTemplate(name, path string) error {
	data, err := embed.Files.ReadFile("templates/" + name)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// checkMyRulesReadable reports whether my-rules.json can't be used right
// now (unreadable, or saved by a newer version of Byline), in which case
// brief regeneration must be skipped so the old brief stays. The two
// problem sentences it can add are exact.
func checkMyRulesReadable() (skip bool, err error) {
	_, err = readFormatDoc(paths.MyRulesPath())
	if err == nil {
		return false, nil
	}
	var ue *UnreadableError
	if errors.As(err, &ue) {
		return true, fmt.Errorf("Byline can't read your personal rules file at %s (%w). It hasn't changed it; fix it or copy one back from %s.",
			ue.Path, ue.Err, paths.BackupsDir())
	}
	if errors.Is(err, ErrNewerFormat) {
		return true, fmt.Errorf("Your personal rules were saved by a newer version of Byline, so this one won't change them; to update, %s, then quit and reopen the app.",
			version.UpdateStep(os.Getenv))
	}
	return true, err
}

// briefIsStale reports whether the brief needs regenerating: it's missing,
// its first line doesn't carry this version's stamp, or my-rules.json or
// my-stance.md is newer than it.
func briefIsStale() (bool, error) {
	briefInfo, err := os.Stat(paths.BriefPath())
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	first, err := firstLineOf(paths.BriefPath())
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(first, "<!-- byline "+version.Version()+" |") {
		return true, nil
	}
	for _, p := range []string{paths.MyRulesPath(), paths.MyStancePath()} {
		info, err := os.Stat(p)
		if err != nil {
			continue // missing personal file: nothing to compare
		}
		if info.ModTime().After(briefInfo.ModTime()) {
			return true, nil
		}
	}
	return false, nil
}

func firstLineOf(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return string(data[:i]), nil
	}
	return string(data), nil
}

// LoadMyRules reads my-rules.json, decoded with json.Number preserved so a
// rewrite never rounds a number through float64. A file with no format
// field is read as the current format: this version of Byline has never
// shipped an older one, so there's nothing to upgrade. err is
// ErrNewerFormat or *UnreadableError.
func LoadMyRules() (map[string]any, error) {
	return readFormatDoc(paths.MyRulesPath())
}

// UpdateMyRules reads my-rules.json, runs change on the decoded document,
// validates the result with rules.Load, then backs up and writes it. If
// the file on disk changed between the read and the write - another
// session saved first - it starts over, up to 3 tries.
func UpdateMyRules(change func(doc map[string]any) error) (string, error) {
	path := paths.MyRulesPath()
	const maxTries = 3
	for try := 1; ; try++ {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", &UnreadableError{Path: path, Err: err}
		}
		doc, err := decodeFormatDoc(path, raw)
		if err != nil {
			return "", err
		}
		if err := change(doc); err != nil {
			return "", err
		}
		doc["format"] = Format
		data, err := encodeDoc(doc)
		if err != nil {
			return "", err
		}
		if _, err := rules.Load(data); err != nil {
			return "", err
		}

		cur, err := os.ReadFile(path)
		if err != nil {
			return "", &UnreadableError{Path: path, Err: err}
		}
		if !bytes.Equal(cur, raw) {
			if try >= maxTries {
				return "", errors.New("Your personal rules kept changing while saving. Try again.")
			}
			continue
		}

		backup, err := Backup(path)
		if err != nil {
			return "", err
		}
		if err := WriteFileAtomic(path, data, 0o600); err != nil {
			return backup, err
		}
		if err := os.WriteFile(paths.RulesSavedPath(), []byte(time.Now().UTC().Format(time.RFC3339)), 0o600); err != nil {
			return backup, err
		}
		return backup, nil
	}
}

// readFormatDoc reads path and decodes it with decodeFormatDoc.
func readFormatDoc(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &UnreadableError{Path: path, Err: err}
	}
	return decodeFormatDoc(path, raw)
}

// decodeFormatDoc decodes raw as a voice-folder JSON object with
// json.Number preserved, then checks its format field: missing or absent
// is the current Format (nothing predates it); greater than Format is
// ErrNewerFormat; anything that isn't a JSON object, or whose format
// field isn't a whole number, is *UnreadableError. Content left after the
// object -- a hand edit that closed it too early -- is *UnreadableError
// too (I2), so a save never drops it silently; trailing whitespace is
// fine.
func decodeFormatDoc(path string, raw []byte) (map[string]any, error) {
	// A Windows editor may save a leading UTF-8 BOM.
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, &UnreadableError{Path: path, Err: err}
	}
	if doc == nil {
		return nil, &UnreadableError{Path: path, Err: errors.New("not a JSON object")}
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, &UnreadableError{Path: path, Err: errors.New("content after the first JSON value")}
	}
	if f, ok := doc["format"]; ok {
		n, ok := f.(json.Number)
		if !ok {
			return nil, &UnreadableError{Path: path, Err: errors.New("format field isn't a number")}
		}
		iv, err := n.Int64()
		if err != nil {
			return nil, &UnreadableError{Path: path, Err: errors.New("format field isn't an integer")}
		}
		if int(iv) > Format {
			return nil, ErrNewerFormat
		}
	}
	return doc, nil
}
