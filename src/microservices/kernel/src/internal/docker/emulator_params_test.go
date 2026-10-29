package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/emuparams"
)

// LE CONTENEUR REÇOIT CE QUE LE FICHIER DIT : rien sans fichier (et l'état
// « absent », pour que streamex rende ce qu'un lancement précédent avait
// posé), les valeurs une fois l'écran passé par là, et rien du tout pour une
// console sans paramètres livrés.
func TestTheSessionGetsTheEmulatorParams(t *testing.T) {
	root := t.TempDir()
	settings := func() emuparams.Settings {
		entry, ok := emulatorParamsEnv(root, "ps3", "", "")
		if !ok || !strings.HasPrefix(entry, "GAMEFLEX_EMULATOR_PARAMS=") {
			t.Fatalf("pas de variable pour la PS3 : %q", entry)
		}
		var decoded emuparams.Settings
		if err := json.Unmarshal([]byte(strings.TrimPrefix(entry, "GAMEFLEX_EMULATOR_PARAMS=")), &decoded); err != nil {
			t.Fatal(err)
		}

		return decoded
	}

	if decoded := settings(); decoded.State != "absent" || decoded.Emulator != "rpcs3" || len(decoded.Entries) != 0 {
		t.Fatalf("sans fichier : %+v", decoded)
	}
	store := emuparams.Store{Root: root}
	if _, err := store.Save(emuparams.Ref{ConsoleID: "ps3", Emulator: "rpcs3"}, "PlayStation 3", nil,
		map[string]any{"spuBlockSize": "Giga"}, "", nil); err != nil {
		t.Fatal(err)
	}
	decoded := settings()
	if decoded.State != "enabled" || len(decoded.Entries) == 0 {
		t.Fatalf("avec le fichier : %+v", decoded)
	}
	found := false
	for _, entry := range decoded.Entries {
		found = found || (entry.Config == "Core > SPU Block Size" && entry.Value == "Giga")
	}
	if !found {
		t.Fatalf("la valeur enregistrée n'est pas transmise : %+v", decoded.Entries)
	}
	if _, err := os.Stat(filepath.Join(root, "params", "ps3", "rpcs3.json")); err != nil {
		t.Fatal(err)
	}

	// UNE CONSOLE SANS PARAMÈTRES LIVRÉS : la NES (la PS5 en a depuis le
	// 2026-10-14, les réglages de Box64 du téléphone, #371).
	if entry, ok := emulatorParamsEnv(root, "nes", "", ""); ok {
		t.Fatalf("une console sans paramètres livrés : %q", entry)
	}
}
