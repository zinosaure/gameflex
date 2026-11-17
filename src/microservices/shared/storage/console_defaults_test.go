package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleDefaultsRoundTrip(t *testing.T) {
	root := t.TempDir()

	empty, err := ReadConsoleDefaults(root, "nes")
	if err != nil || empty != (ConsoleDefaults{}) {
		t.Fatalf("sans fichier = (%+v, %v), want rien et pas d'erreur", empty, err)
	}

	want := ConsoleDefaults{Core: "nestopia", Bios: "disksys.rom"}
	if err := WriteConsoleDefaults(root, "nes", want); err != nil {
		t.Fatalf("WriteConsoleDefaults: %v", err)
	}
	got, err := ReadConsoleDefaults(root, "nes")
	if err != nil || got != want {
		t.Fatalf("relu = (%+v, %v), want %+v", got, err, want)
	}
	if _, err := os.Stat(filepath.Join(root, "defaults", "nes.json.part")); !os.IsNotExist(err) {
		t.Errorf("le .part reste après l'écriture (%v)", err)
	}

	// Plus rien d'enregistré : le fichier part.
	if err := WriteConsoleDefaults(root, "nes", ConsoleDefaults{}); err != nil {
		t.Fatalf("WriteConsoleDefaults(vide): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "defaults", "nes.json")); !os.IsNotExist(err) {
		t.Errorf("le fichier reste quand plus rien n'est enregistré (%v)", err)
	}
}

func TestConsoleDefaultsRefusesAPathOutsideItsFolder(t *testing.T) {
	root := t.TempDir()
	for _, consoleID := range []string{"", "../nes", "a/b", ".hidden"} {
		if err := WriteConsoleDefaults(root, consoleID, ConsoleDefaults{Core: "x"}); err == nil {
			t.Errorf("WriteConsoleDefaults(%q) = nil, want une erreur", consoleID)
		}
	}
}

func TestConsoleDefaultsUnreadableIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "defaults"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "defaults", "nes.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadConsoleDefaults(root, "nes"); err == nil || got != (ConsoleDefaults{}) {
		t.Errorf("fichier illisible = (%+v, %v), want rien et une erreur", got, err)
	}
}
