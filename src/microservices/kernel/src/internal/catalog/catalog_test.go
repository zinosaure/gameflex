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

// UN PORT D'UN JEU-DOSSIER (chantier 72) : la copie PS5 d'Astro Bot se lance par la console des ports, reconnue à
// son identifiant et à sa version (sce_sys/param.json) — le listage la montrait, le lancement ne la trouvait pas.
func TestAPortLaunchesAGameFolderRecognisedByItsTitle(t *testing.T) {
	roms := t.TempDir()
	param := filepath.Join(roms, "ps5", "Astro Bot", "sce_sys", "param.json")
	if err := os.MkdirAll(filepath.Dir(param), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(param, []byte(`{"titleId":"PPSA21567","contentVersion":"01.019.000"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	game, err := For("ports", roms).Resolve("Astro Bot")
	if err != nil {
		t.Fatalf("Resolve(Astro Bot): %v", err)
	}
	if game.RelPath != filepath.Join("ps5", "Astro Bot") || game.Folder != "ps5" {
		t.Errorf("got %+v, want ps5/Astro Bot", game)
	}
}
