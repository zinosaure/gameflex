package manager

import (
	"os"
	"path/filepath"
	"testing"
)

// LE MOTEUR AU LANCEMENT (chantier 36) : la règle du scan, sur le jeu tel que
// le kernel le voit. Un jeu déballé le dit ; un `.pkg` dans son dossier, ou un
// fichier posé à plat dans celui de la console, n'a rien à montrer.
func TestTheEngineIsReadFromTheGameAtLaunch(t *testing.T) {
	roms := t.TempDir()
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(roms, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("FF XV Pocket Edition HD/eboot.bin")
	write("FF XV Pocket Edition HD/Media/globalgamemanagers")
	write("Octopath Traveler 0/eboot.bin")
	write("Octopath Traveler 0/uecommandline.txt")
	write("Packed/game.pkg")
	write("Loose.pkg")

	for rel, want := range map[string]string{
		"FF XV Pocket Edition HD": "unity",
		"Octopath Traveler 0":     "unreal",
		"Packed/game.pkg":         "",
		"Loose.pkg":               "",
		"Missing":                 "",
	} {
		if got := gameEngine("ps4", filepath.Join(roms, rel), rel); got != want {
			t.Errorf("%s : moteur %q, attendu %q", rel, got, want)
		}
	}
	if got := gameEngine("ps3", filepath.Join(roms, "Octopath Traveler 0"), "Octopath Traveler 0"); got != "" {
		t.Errorf("PS3 : moteur %q, attendu aucun (ses jeux sont des .iso)", got)
	}
}
