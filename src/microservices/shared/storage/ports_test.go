package storage

import (
	"os"
	"path/filepath"
	"testing"

	"gameflex/consoles"
)

// UN FICHIER N'EST JAMAIS UN JEU DE PORTS (2026-11-23 : les ports rétro, reconnus à leur ROM, sont retirés).
func TestAFileIsNeverAPortGame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.z64")
	if err := os.WriteFile(path, []byte("a rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := PortOf("n64", path); ok {
		t.Error("un fichier a été pris pour un jeu de ports")
	}
}

// UN JEU-DOSSIER (chantier 72) : une copie PS5 est reconnue à l'identifiant et à la version de son `sce_sys/param.json`
// — la version éprouvée seulement.
func TestAPortRecognisesAGameFolderByTitleAndVersion(t *testing.T) {
	port := consoles.Port{Core: "essai", From: "ps5", Titles: []consoles.PortTitle{{TitleID: "PPSA21567", Version: "01.019.000"}}}
	previous := knownPorts
	knownPorts = func() []consoles.Port { return []consoles.Port{port} }
	t.Cleanup(func() { knownPorts = previous })

	game := func(version string) string {
		dir := filepath.Join(t.TempDir(), "Astro Bot")
		if err := os.MkdirAll(filepath.Join(dir, "sce_sys"), 0o755); err != nil {
			t.Fatal(err)
		}
		param := `{"titleId":"PPSA21567","contentVersion":"` + version + `"}`
		if err := os.WriteFile(filepath.Join(dir, "sce_sys", "param.json"), []byte(param), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	if found, ok := PortOf("ps5", game("01.019.000")); !ok || found.Core != "essai" {
		t.Fatalf("la version éprouvée : %+v %v", found, ok)
	}
	if _, ok := PortOf("ps5", game("01.018.000")); ok {
		t.Fatal("une autre version est acceptée")
	}
	if _, ok := PortOf("n64", game("01.019.000")); ok {
		t.Fatal("un jeu d'une autre console est accepté")
	}
}
