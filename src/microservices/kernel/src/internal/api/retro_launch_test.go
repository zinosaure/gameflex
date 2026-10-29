package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/kernel/internal/registry"
)

// UN JEU QUI N'EST PAS DANS LA BIBLIOTHÈQUE NE SE LANCE PAS ICI (chantier 14,
// 2026-10-12) : ni celui d'un serveur fédéré, ni celui d'un partage monté, ni
// une référence inconnue — 404, avant tout conteneur. Le mode « hybride » les
// copiait dans un cache pour les jouer ici.
func TestAGameOutsideTheLibraryIsNotLaunchedHere(t *testing.T) {
	root := t.TempDir()
	server := &Server{
		pool:        registry.NewPool(),
		mirrors:     registry.NewMirrors(),
		storageRoot: root,
	}

	for _, reference := range []string{"srv3:abc9539e-371e-584c-85d9-8409f8db02dd", "mnt2:kdrive/nes/Zelda.nes", "inconnue"} {
		body := `{"platform":"retrogaming","consoleId":"nes","reference":"` + reference + `","core":"fceumm","username":"gino"}`
		response := httptest.NewRecorder()
		server.handleRetroLaunch(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "not in the library") {
			t.Errorf("%s : %d %s, attendu 404 « not in the library »", reference, response.Code, response.Body.String())
		}
	}
}
