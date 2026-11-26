package kdrive

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/storage"
)

// L'ARBORESCENCE RÉELLE, celle du dossier d'un serveur : bios/, cores/,
// roms/, et les consoles sous roms/.
//
// Ce test existe parce que la sonde disait « 3 dossiers trouvés » pendant
// qu'aucun jeu n'apparaissait (signalé le 2026-09-02) : regarder UN dossier
// et parcourir un arbre ne sollicitent pas le même code, et seul le second
// remplit la bibliothèque.
func TestScanFindsGamesInTheRealTree(t *testing.T) {
	children := map[string]string{
		// racine du drive
		"": `[{"id":10,"name":"gameflex","type":"dir"}]`,
		// gameflex/ → les trois dossiers que la sonde a comptés
		"10": `[{"id":20,"name":"bios","type":"dir"},
		        {"id":21,"name":"cores","type":"dir"},
		        {"id":22,"name":"roms","type":"dir"}]`,
		"20": `[{"id":40,"name":"scph5501.bin","type":"file","size":524288}]`,
		"21": `[]`,
		// roms/ → les consoles
		"22": `[{"id":30,"name":"nes","type":"dir"},
		        {"id":31,"name":"gba","type":"dir"}]`,
		"30": `[{"id":50,"name":"Metroid.nes","type":"file","size":131072},
		        {"id":51,"name":"Zelda.nes","type":"file","size":131072}]`,
		"31": `[{"id":52,"name":"Fire Emblem.gba","type":"file","size":8388608}]`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); len(parts) == 6 {
			id = parts[4]
		}
		body, known := children[id]
		if !known {
			body = "[]"
		}
		_, _ = io.WriteString(w, fmt.Sprintf(`{"result":"success","data":%s}`, body))
	}))
	defer server.Close()
	baseURL = server.URL

	source, err := New(Config{
		Name: "kdrive", DriveID: "42", Token: "jeton", Folder: "gameflex",
		Client: server.Client(),
	})
	if err != nil {
		t.Fatalf("montage : %v", err)
	}

	games, err := storage.ScanConvention(source, "roms")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}

	byTitle := map[string]storage.Game{}
	for _, game := range games {
		byTitle[game.Title] = game
	}
	for _, wanted := range []struct{ title, console string }{
		{"Metroid", "nes"}, {"Zelda", "nes"}, {"Fire Emblem", "gba"},
	} {
		game, ok := byTitle[wanted.title]
		if !ok {
			t.Fatalf("%q manque — %d jeux trouvés : %+v", wanted.title, len(games), games)
		}
		if game.Console != wanted.console {
			t.Fatalf("%q rattaché à %q, attendu %q", wanted.title, game.Console, wanted.console)
		}
		if game.Kind != "kdrive" {
			t.Fatalf("%q vient de %q, attendu kdrive", wanted.title, game.Kind)
		}
	}
}
