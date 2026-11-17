package storage

// LE DÉFAUT D'UNE CONSOLE, SUR LE DISQUE (chantier 54, P1, #680) — le fichier
// `defaults/<console>.json` à la racine d'une installation : l'émulateur par
// défaut, la version en service d'un émulateur Streamex, le BIOS par défaut
// d'une console rétro. Le serveur l'a écrit au chantier 53 (P4, #668) ; le
// téléphone le lit de la même façon, d'où sa place ici.
//
// Seuls la forme et l'écriture se partagent. Ce que « installé ici » veut dire
// reste à chaque kernel : les images Docker et `cores/` au serveur, l'inventaire
// de l'application au téléphone.
//
// Le fichier n'est ni listé ni fédéré : `defaults/` n'est pas une section.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConsoleDefaults — ce qui est enregistré pour une console. Un champ vide n'a
// pas de défaut : la règle d'avant s'applique (le premier cœur du registre, le
// fichier déposé en dernier, tout le dossier des BIOS).
type ConsoleDefaults struct {
	Core         string `json:"core,omitempty"`
	EmulatorFile string `json:"emulatorFile,omitempty"`
	Bios         string `json:"bios,omitempty"`
}

// ConsoleDefaultsPath — `<root>/defaults/<console>.json`, ou une erreur pour
// un identifiant qui sortirait du dossier.
func ConsoleDefaultsPath(root, consoleID string) (string, error) {
	if consoleID == "" || consoleID != filepath.Base(consoleID) || strings.HasPrefix(consoleID, ".") {
		return "", fmt.Errorf("invalid console id %q", consoleID)
	}

	return filepath.Join(root, "defaults", consoleID+".json"), nil
}

// ReadConsoleDefaults — ce qui est enregistré ; rien, sans erreur, quand le
// fichier manque.
func ReadConsoleDefaults(root, consoleID string) (ConsoleDefaults, error) {
	var defaults ConsoleDefaults
	path, err := ConsoleDefaultsPath(root, consoleID)
	if err != nil {
		return defaults, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return ConsoleDefaults{}, err
	}

	return defaults, nil
}

// WriteConsoleDefaults — écrit le fichier d'un coup (un `.part` renommé), ou le
// supprime quand plus rien n'est enregistré. Il prend le propriétaire de son
// dossier, comme tout ce que le kernel écrit dans une bibliothèque.
func WriteConsoleDefaults(root, consoleID string, defaults ConsoleDefaults) error {
	path, err := ConsoleDefaultsPath(root, consoleID)
	if err != nil {
		return err
	}
	if defaults == (ConsoleDefaults{}) {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	AdoptParentOwner(filepath.Dir(path))
	encoded, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".part"
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	AdoptParentOwner(path)

	return nil
}
