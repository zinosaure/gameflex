package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/storage"
)

// LE DÉBALLÉ D'UN JEU PS4 PART AVEC LUI (chantier 22), et seulement quand
// plus aucun .pkg du même nom ne reste : une copie sur un disque interne le
// garde, puisque streamex la déballerait au même endroit. Un jeu-dossier (le
// jeu et sa mise à jour) emporte ses deux déballés.
func TestDeletingAPS4GameRemovesItsUnpackedCopy(t *testing.T) {
	server, root := listingServer(t, "roms/ps4/Sonic.pkg", "roms/ps4/FF7/FF7.pkg", "roms/ps4/FF7/FF7-patch.pkg")
	disk := diskWith(t, "ssd", map[string]string{"roms/ps4/Sonic.pkg": "x"})
	withDisks(server, disk)
	installs := filepath.Join(root, "cache", "installs", "ps4")
	for _, dir := range []string{"Sonic", "FF7", "FF7-patch", "Autre"} {
		if err := os.MkdirAll(filepath.Join(installs, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(installs, "Sonic.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Delete("/{consoleId}/roms/{ref}", server.handleRetroDeleteRom)
	remove := func(name, query string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/ps4/roms/"+storage.RomID("ps4", name)+query, nil))
		// La suppression part en tâche suivie (202, son identifiant) : la test attend sa fin, comme la page.
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("suppression de %s : %d %s", name, recorder.Code, recorder.Body.String())
		}
		var answer struct {
			Data struct {
				DownloadID string `json:"downloadId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Data.DownloadID == "" {
			t.Fatalf("suppression de %s : pas d'identifiant (%s)", name, recorder.Body.String())
		}
		// Sans jeton, la tâche n'a pas de compte : elle se suit sous « ».
		steps, last, _, known := server.downloads.Subscribe(answer.Data.DownloadID, "")
		if !known {
			t.Fatalf("suppression de %s : tâche inconnue", name)
		}
		for step := range steps {
			last = step
		}
		if last.Type != "done" {
			t.Fatalf("suppression de %s : %+v", name, last)
		}
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(installs, name))
		return err == nil
	}

	remove("Sonic.pkg", "")
	if !exists("Sonic") {
		t.Fatal("le déballé de Sonic est parti alors qu'une copie reste sur le disque ssd")
	}
	remove("Sonic.pkg", "?disk=ssd")
	if exists("Sonic") || exists("Sonic.lock") {
		t.Fatal("le déballé de Sonic reste après la suppression de sa dernière copie")
	}
	remove("FF7", "")
	if exists("FF7") || exists("FF7-patch") {
		t.Fatal("un jeu-dossier n'emporte pas ses deux déballés")
	}
	if !exists("Autre") {
		t.Fatal("le déballé d'un autre jeu est parti")
	}
}
