package draftcheck

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"byline/internal/paths"
)

// Receipt is what WriteReceipt writes on a pass, and what the PreToolUse
// gate later reads back to prove this exact draft was checked: the gate
// matches on >=90% of these content words being present in the outgoing
// message, within a 60-minute lifetime, requiring at least 6 content
// words. The words come from MessageText, the same text the gate reads
// from the outgoing message, so markup and a signature never count.
type Receipt struct {
	Draft string   `json:"draft"`
	Spec  string   `json:"spec"`
	At    string   `json:"at"`
	Words []string `json:"words"`
}

// WriteReceipt writes a receipt for text under paths.ReceiptsDir()
// (folder mode 0700, file mode 0600), named
// "<now.UnixMilli()>-<6 random digits>.json", and returns its path.
// draftPath and specPath are empty when Run wrote it, filled in when the
// CLI did.
func WriteReceipt(draftPath, specPath, text string, now time.Time) (string, error) {
	dir := paths.ReceiptsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	rec := Receipt{Draft: draftPath, Spec: specPath, At: now.UTC().Format("2006-01-02T15:04:05.000Z"), Words: ContentWords(MessageText(text))}
	b, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d-%06d.json", now.UnixMilli(), rand.Intn(1000000))
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return "", err
	}
	return p, nil
}
