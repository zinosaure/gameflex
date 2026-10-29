package storage

import (
	"path/filepath"
	"testing"
)

// LA DISPOSITION DU SERVEUR, PAR COMPTE : le téléphone la suit désormais, et
// un compte ou une console venus d'une requête ne sortent pas du dossier.
func TestSavesAreFiledByAccount(t *testing.T) {
	root := filepath.FromSlash("/lib/saves")
	for _, tc := range []struct {
		got  func() (string, error)
		want string
	}{
		{func() (string, error) {
			return SaveStatePath(root, "gino", "nes", "appareil/nes/Mario.nes", 3, ".state")
		},
			"/lib/saves/gino/states/nes/appareil_nes_Mario.nes_slot3.state"},
		{func() (string, error) { return SaveStatePath(root, "gino", "psp", "Zelda: A?.iso", 0, ".png") },
			"/lib/saves/gino/states/psp/Zelda_ A_.iso_slot0.png"},
		{func() (string, error) { return SaveRAMPath(root, "root", "gba", "Pokemon.gba") },
			"/lib/saves/root/sram/gba/Pokemon.gba.srm"},
		{func() (string, error) { return CoreSaveDir(root, "gino", "desmume") },
			"/lib/saves/gino/cores/desmume"},
	} {
		got, err := tc.got()
		if err != nil || got != filepath.FromSlash(tc.want) {
			t.Errorf("%q, %v ; attendu %q", got, err, tc.want)
		}
	}

	for _, bad := range []string{"", "..", "../databases", "a/b"} {
		if _, err := SaveStatePath(root, bad, "nes", "x", 0, ".state"); err == nil {
			t.Errorf("compte %q accepté", bad)
		}
		if _, err := SaveRAMPath(root, "gino", bad, "x"); err == nil {
			t.Errorf("console %q acceptée", bad)
		}
	}
	if _, err := CoreSaveDir("", "gino", "desmume"); err == nil {
		t.Error("une racine vide est acceptée")
	}

	if game, slot, ok := ParseSaveStateName("appareil_nes_Mario.nes_slot3.state"); !ok || game != "appareil_nes_Mario.nes" || slot != 3 {
		t.Errorf("relu : %q %d %v", game, slot, ok)
	}
	for _, other := range []string{"x_slot3.png", "x.srm", "x_slotA.state"} {
		if _, _, ok := ParseSaveStateName(other); ok {
			t.Errorf("%q pris pour un état", other)
		}
	}
}
