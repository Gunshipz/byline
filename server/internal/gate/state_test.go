package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"byline/internal/paths"
)

func TestSessionSlug(t *testing.T) {
	for in, want := range map[string]string{"abc-1.2_x": "abc-1.2_x", `a/b\c d`: "a_b_c_d", "../../etc": ".._.._etc", "": ""} {
		if got := sessionSlug(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// TestSessionSlug_LongIDIsHashed is M8: a session id long enough that the
// slug alone wouldn't leave room for a ".read-exemplars" suffix on every
// filesystem gets hashed to a short, fixed, deterministic name instead of
// silently failing to arm.
func TestSessionSlug_LongIDIsHashed(t *testing.T) {
	newHome(t)
	long := strings.Repeat("a", 300)
	slug := sessionSlug(long)
	if len(slug) > slugMaxLen {
		t.Fatalf("slug is still %d characters: %q", len(slug), slug)
	}
	if slug == sessionSlug(strings.Repeat("b", 300)) {
		t.Fatal("two different long ids hashed to the same slug")
	}
	if slug != sessionSlug(long) {
		t.Fatal("hashing the same long id twice must be deterministic")
	}
	arm(slug)
	if !isArmed(slug) {
		t.Fatal("a hashed long-id slug must still be armable")
	}
}

func TestState_ArmReadReset(t *testing.T) {
	newHome(t)
	arm("s1")
	recordRead("s1", "brief")
	if !isArmed("s1") || isArmed("s2") || !hasRead("s1", "brief") || hasRead("s1", "profile") {
		t.Fatal("arm or read marks wrong")
	}
	if _, err := os.Stat(filepath.Join(paths.StateDir(), "s1.armed")); err != nil {
		t.Fatal("state must live under paths.StateDir()")
	}
	resetSession("s1")
	if isArmed("s1") || hasRead("s1", "brief") {
		t.Fatal("reset left marks behind")
	}
}

func TestState_PruneRemovesOnlyOldFiles(t *testing.T) {
	newHome(t)
	pruneState(stateMaxAge) // no state folder yet: must not panic
	arm("old")
	arm("new")
	when := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(filepath.Join(paths.StateDir(), "old.armed"), when, when)
	pruneState(stateMaxAge)
	if isArmed("old") || !isArmed("new") {
		t.Fatal("prune removed the wrong file")
	}
}

// TestState_PruneKeepsRulesSaved is M1: pruneState must remove only the
// gate's own session marks (an ".armed" or a ".read-" file), never another
// file something else in Byline put under state/, such as rules-saved.
func TestState_PruneKeepsRulesSaved(t *testing.T) {
	newHome(t)
	saved := paths.RulesSavedPath()
	if err := os.MkdirAll(filepath.Dir(saved), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved, []byte("2020-01-01T00:00:00Z"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(saved, when, when); err != nil {
		t.Fatal(err)
	}
	pruneState(stateMaxAge)
	if _, err := os.Stat(saved); err != nil {
		t.Fatal("prune removed state/rules-saved, which the gate doesn't own")
	}
}

// TestPruneReceipts_RemovesOnlyOldOnes is M7: receipts outlive
// receiptLifetime forever unless something prunes them, and only the
// stale ones should go.
func TestPruneReceipts_RemovesOnlyOldOnes(t *testing.T) {
	newHome(t)
	addReceipt(t, bodyMD, receiptLifetime+time.Hour)
	addReceipt(t, bodyMD, 0)
	if entries, _ := os.ReadDir(paths.ReceiptsDir()); len(entries) != 2 {
		t.Fatalf("fixture must write 2 receipts, got %d", len(entries))
	}
	pruneReceipts(receiptLifetime)
	entries, err := os.ReadDir(paths.ReceiptsDir())
	if err != nil || len(entries) != 1 {
		t.Fatalf("want 1 receipt left, got %d (%v)", len(entries), err)
	}
}
