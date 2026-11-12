package gate

import (
	"path/filepath"
	"strings"

	"byline/internal/paths"
)

// requiredFile is one of the voice-folder files the gate insists on before
// it lets a draft go out.
type requiredFile struct {
	key  string
	name string
	path func() string
}

// requiredFiles are the files session-start checks for, in the order their
// names are listed when some are missing.
var requiredFiles = []requiredFile{
	{"brief", "the writing brief", paths.BriefPath},
	{"profile", "the writing profile", paths.ProfilePath},
	{"exemplars", "the sample emails", paths.ExemplarsPath},
}

// requiredKey reports which required file filePath names, tolerating a
// symlinked prefix or drive-letter case the way today's tail match does:
// it lowercases the path and turns "\" into "/", then matches on the voice
// folder's own name plus the file's name, not the full path. "" means
// filePath isn't one of the three.
func requiredKey(filePath string) string {
	norm := strings.ReplaceAll(strings.ToLower(filePath), `\`, "/")
	homeName := strings.ToLower(filepath.Base(paths.Home()))
	for _, f := range requiredFiles {
		suffix := "/" + homeName + "/" + strings.ToLower(filepath.Base(f.path()))
		if strings.HasSuffix(norm, suffix) {
			return f.key
		}
	}
	return ""
}
