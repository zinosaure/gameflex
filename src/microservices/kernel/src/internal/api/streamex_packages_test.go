package api

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LE CORRECTIF D'UN JEU DEPUIS LA SOURCE OFFICIELLE (chantier 63, P6) : le paquet `streamex-patches` de l'index porte
// plusieurs correctifs ; seul celui demandé se déplie, sous `patches/<console>/<identifiant>/`, et un second
// téléchargement est refusé comme pour les autres sources.
func TestAPatchComesFromTheOfficialSource(t *testing.T) {
	server, _ := listingServer(t)

	var packaged bytes.Buffer
	writer := zip.NewWriter(&packaged)
	for name, content := range map[string]string{
		"ps5/PPSA21567/settings.json": `{"name":"ASTRO BOT"}`,
		"ps5/PPSA99999/settings.json": `{"name":"another game"}`,
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packaged.Bytes())
	var release *httptest.Server
	release = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/components.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"format": 1, "packages": []map[string]any{{
				"kind": "patches", "file": "streamex-patches-test.zip", "sizeBytes": packaged.Len(),
				"sha256": hex.EncodeToString(digest[:]), "url": release.URL + "/streamex-patches-test.zip",
				"items": []map[string]any{
					{"console": "ps5", "name": "PPSA21567", "files": []string{"settings.json"}},
					{"console": "ps5", "name": "PPSA99999", "files": []string{"settings.json"}},
				},
			}}})
		case "/streamex-patches-test.zip":
			_, _ = w.Write(packaged.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer release.Close()
	server.streamexIndex = release.URL + "/components.json"

	fetch := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.handleOfficialPatchImport(response, httptest.NewRequest(http.MethodPost, "/api/v1/retrogaming/storage/official/patches",
			strings.NewReader(`{"consoleId":"ps5","name":"PPSA21567"}`)))

		return response
	}

	if response := fetch(); response.Code != http.StatusOK {
		t.Fatalf("correctif officiel : %d %s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(patchesRoot(), "ps5", "PPSA21567", "settings.json"))
	if err != nil || string(content) != `{"name":"ASTRO BOT"}` {
		t.Fatalf("le correctif rangé : %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(patchesRoot(), "ps5", "PPSA99999")); err == nil {
		t.Fatal("un autre correctif du paquet a été déplié")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(patchesRoot(), "ps5", ".streamex-*")); len(leftovers) > 0 {
		t.Fatalf("des restes du déballage : %v", leftovers)
	}
	if again := fetch(); again.Code != http.StatusConflict {
		t.Fatalf("un second téléchargement : %d %s", again.Code, again.Body.String())
	}
}
