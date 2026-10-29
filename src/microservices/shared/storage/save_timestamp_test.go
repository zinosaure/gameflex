package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Une copie garde la date de l'original : elle se lit, se pose, et se relit
// telle quelle à la milliseconde (#395).
func TestASavedTimestampIsKept(t *testing.T) {
	if saved, err := ParseSaveTimestamp(""); err != nil || !saved.IsZero() {
		t.Fatalf("sans date : %v %v", saved, err)
	}
	for _, wrong := range []string{"hier", "-5", "0"} {
		if _, err := ParseSaveTimestamp(wrong); err == nil {
			t.Fatalf("%q accepté", wrong)
		}
	}

	saved, err := ParseSaveTimestamp("1760000000123")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "jeu_slot3.state")
	if err := os.WriteFile(path, []byte("état"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := KeepSaveTimestamp(path, saved); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.ModTime().UnixMilli(); got != 1760000000123 {
		t.Fatalf("date relue %d, attendu 1760000000123", got)
	}
	if err := KeepSaveTimestamp(path, time.Time{}); err != nil {
		t.Fatalf("sans date, rien à poser : %v", err)
	}
}
