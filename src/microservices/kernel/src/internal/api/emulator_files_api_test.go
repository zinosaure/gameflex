package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/storage/listing"
)

// LES FICHIERS DES ÉMULATEURS DE BUREAU (chantier 29) : rangés dans
// `cores/<console>/`, ils apparaissent dans la liste des émulateurs — ceux du
// serveur et ceux du téléphone —, se servent à un ami, et une ancienne version
// se supprime par son nom. Un cœur libretro ne passe pas par cette route.
func TestEmulatorFilesAreListedServedAndRemoved(t *testing.T) {
	server, root := listingServer(t,
		"cores/ps5/KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz",
		"cores/ps5/kyty-2026-10-23+box64-d653d0ebe-forky-r1-aarch64.zip",
		"cores/ps4/shadps4-linux-sdl-0.19.0.zip",
		"cores/nes/fceumm_libretro.so",
	)
	router := chi.NewRouter()
	router.Get("/storage/cores", server.handleStorageCores)
	router.Delete("/storage/cores", server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDelete))
	router.Get("/federation/bundles/{consoleId}/{file}", server.handleFederationBundle)
	call := func(method, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, path, nil))

		return response
	}
	packages := func() map[string]string {
		var listed struct {
			Data struct {
				Consoles []struct {
					ConsoleID string      `json:"consoleId"`
					Items     []storeItem `json:"items"`
				} `json:"consoles"`
			} `json:"data"`
		}
		response := call(http.MethodGet, "/storage/cores")
		if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
			t.Fatalf("liste illisible : %s", response.Body.String())
		}
		found := map[string]string{}
		for _, console := range listed.Data.Consoles {
			for _, item := range console.Items {
				if item.Package != "" {
					found[console.ConsoleID+"/"+item.Name] = item.Package
				}
			}
		}

		return found
	}

	want := map[string]string{
		"ps5/KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz":       "server",
		"ps5/kyty-2026-10-23+box64-d653d0ebe-forky-r1-aarch64.zip": "phone",
		"ps4/shadps4-linux-sdl-0.19.0.zip":                         "server",
	}
	if got := packages(); len(got) != len(want) {
		t.Fatalf("fichiers d'émulateur listés = %v, attendu %v", got, want)
	} else {
		for key, kind := range want {
			if got[key] != kind {
				t.Fatalf("%s = %q, attendu %q", key, got[key], kind)
			}
		}
	}

	if served := call(http.MethodGet, "/federation/bundles/ps4/shadps4-linux-sdl-0.19.0.zip"); served.Code != http.StatusOK || served.Body.String() != "x" {
		t.Fatalf("servi à un ami = %d %q", served.Code, served.Body.String())
	}
	for _, path := range []string{"/federation/bundles/nes/fceumm_libretro.so", "/federation/bundles/ps5/..%2Fnes%2Ffceumm_libretro.so"} {
		if refused := call(http.MethodGet, path); refused.Code == http.StatusOK {
			t.Fatalf("%s a été servi", path)
		}
	}

	if removed := call(http.MethodDelete, "/storage/cores?consoleId=ps5&core=KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz"); removed.Code != http.StatusOK {
		t.Fatalf("supprimer = %d %s", removed.Code, removed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "cores", "ps5", "KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("le fichier est resté : %v", err)
	}
	if _, listed := packages()["ps5/KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz"]; listed {
		t.Fatal("après la suppression, la liste le montre encore")
	}
}

// L'ÉMULATEUR D'UN AMI SE RAPATRIE : son archive arrive telle quelle (un .zip
// de shadPS4 EST le fichier attendu, il ne se déplie pas), jamais par-dessus
// celle qui est là.
func TestAFriendsEmulatorFileIsImportedAsIs(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, _ := writer.Create("Shadps4-sdl.AppImage")
	_, _ = entry.Write([]byte("appimage"))
	_ = writer.Close()
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jeton" || r.URL.Path != "/api/v1/federation/bundles/ps4/shadps4-linux-sdl-0.19.0.zip" {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		_, _ = w.Write(archive.Bytes())
	}))
	t.Cleanup(peer.Close)

	server, root := listingServer(t)
	server.serverRegistry.items = []registeredServer{{ID: 3, Link: peer.URL, Token: "jeton", Hostname: "ami"}}
	server.serverRegistry.fetchedAt = time.Now()
	router := chi.NewRouter()
	router.Post("/servers/{id}/storage/{section}", server.handleRemoteStorageImport)
	importing := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/servers/3/storage/cores",
			strings.NewReader(`{"consoleId":"ps4","name":"shadps4-linux-sdl-0.19.0.zip"}`)))

		return response
	}

	if response := importing(); response.Code != http.StatusOK {
		t.Fatalf("rapatrier = %d %s", response.Code, response.Body.String())
	}
	written, err := os.ReadFile(filepath.Join(root, "cores", "ps4", "shadps4-linux-sdl-0.19.0.zip"))
	if err != nil || !bytes.Equal(written, archive.Bytes()) {
		t.Fatalf("l'archive n'est pas arrivée telle quelle (%v)", err)
	}
	if again := importing(); again.Code != http.StatusConflict {
		t.Fatalf("une seconde fois = %d, attendu 409", again.Code)
	}
}
