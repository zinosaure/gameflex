package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gameflex/storage/installs"
)

// LE CACHE, VU DE L'ÉCRAN (#448) : un jeu PS4 déballé dit si son .pkg est
// encore dans la bibliothèque ; une entrée se supprime par son nom, et un nom
// hors de la liste est refusé.
func TestTheCacheScreenListsAndEmpties(t *testing.T) {
	root := t.TempDir()
	server := &Server{storageRoot: root}
	for path, size := range map[string]int{
		installs.Dir(root, "ps4", "Sonic", "eboot.bin"):                     10,
		installs.Dir(root, "ps4", "Ancien", "eboot.bin"):                    20,
		filepath.Join(root, "roms", "ps4", "Sonic Mania Plus", "Sonic.pkg"): 5,
		installs.Dir(root, "ps3", "dev_flash", "vsh", "module", "vsh.self"): 30,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	server.handleStorageCache(recorder, httptest.NewRequest(http.MethodGet, "/storage/cache", nil))
	var envelope struct {
		Data struct {
			Consoles  []cacheConsole `json:"consoles"`
			SizeBytes int64          `json:"sizeBytes"`
		} `json:"data"`
	}
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &envelope) != nil {
		t.Fatalf("liste : %d %s", recorder.Code, recorder.Body.String())
	}
	answer := envelope.Data
	sources := map[string]string{}
	for _, console := range answer.Consoles {
		for _, entry := range console.Entries {
			sources[console.ConsoleID+"/"+entry.Name] = entry.Source
		}
	}
	if answer.SizeBytes != 60 || sources["ps4/Sonic"] != "present" || sources["ps4/Ancien"] != "missing" || sources["ps3/dev_flash"] != "" {
		t.Fatalf("le cache : %d octets, sources %v — %s", answer.SizeBytes, sources, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.handleStorageCacheDelete(recorder, httptest.NewRequest(http.MethodDelete, "/storage/cache?consoleId=ps4&name=../../roms", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("un nom hors du cache : %d, attendu 404", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	server.handleStorageCacheDelete(recorder, httptest.NewRequest(http.MethodDelete, "/storage/cache?consoleId=ps4&name=Ancien", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("supprimer Ancien : %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := os.Stat(installs.Dir(root, "ps4", "Ancien")); !os.IsNotExist(err) {
		t.Fatal("Ancien est resté dans le cache")
	}
}
