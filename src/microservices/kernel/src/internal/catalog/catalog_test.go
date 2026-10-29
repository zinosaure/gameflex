package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// La référence rétro que ce catalogue accepte doit être EXACTEMENT celle que
// produisent retro_files (kernel) et le catalogue du runtime rétro. Même valeur
// figée dans les trois paquets — voir
// retro_files.TestReferencesMatchTheSharedGoldenValue pour la raison.
func TestRetroReferenceMatchesTheSharedGoldenValue(t *testing.T) {
	const golden = "a16f8b20-e5e2-5991-a712-a4d77ef85a7e"
	if got := retroReference("nes", "Super Mario Bros..nes"); got != golden {
		t.Errorf("retroReference(nes, Super Mario Bros..nes) = %s, want %s", got, golden)
	}
}

// UN JEU PS5 EN ZARCHIVE (#431) : le dossier garde son paquet, que Kyty ne lit
// pas, et se lance par son `.zar` ; un `.extraction` caché, le temps qu'un jeu
// s'archive, n'est pas un jeu.
func TestAPs5GameLaunchesFromItsZarAndHiddenFoldersAreSkipped(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{
		"Final Fantasy VII/Final Fantasy VII.pkg",
		"Final Fantasy VII/Final Fantasy VII.zar",
		"Final Fantasy VII/.extraction/decrypted/eboot.bin",
		"Richman 11/eboot.bin",
	} {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]string{}
	for _, game := range scan("ps5", dir) {
		got[game.Title] = game.RelPath
	}
	want := map[string]string{
		"Final Fantasy VII": filepath.Join("Final Fantasy VII", "Final Fantasy VII.zar"),
		"Richman 11":        "Richman 11",
	}
	if len(got) != len(want) {
		t.Fatalf("jeux : %v, attendu %v", got, want)
	}
	for title, rel := range want {
		if got[title] != rel {
			t.Errorf("%s : %q, attendu %q", title, got[title], rel)
		}
	}
}

// UN DOSSIER QUI NE SE LANCE PAS SE RECONNAÎT (#431) : HoldsFolder le trouve,
// sans casse, et Formats dit ce qu'il aurait fallu.
func TestAFolderWithNothingToLaunchIsStillRecognised(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Final Fantasy VII Rebirth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Final Fantasy VII Rebirth", "Rebirth.pkg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ps5 := New("ps5", dir)
	if _, err := ps5.Resolve("Final Fantasy VII Rebirth"); err == nil {
		t.Fatal("un paquet PS5 seul ne se lance pas")
	}
	if !ps5.HoldsFolder("final fantasy vii rebirth") || ps5.HoldsFolder("Absent") {
		t.Fatal("HoldsFolder")
	}
	if got := Formats("ps5"); got != ".elf, .zar" {
		t.Errorf("Formats(ps5) = %q", got)
	}
	if got := Formats("switch"); got != ".xci, .nsp, .iso, .pkg" {
		t.Errorf("Formats(switch) = %q", got)
	}
}
