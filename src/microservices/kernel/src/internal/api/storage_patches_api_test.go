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

	"gameflex/emuparams"
	"gameflex/storage/listing"
)

// withColdRoot — une racine froide à ce test : `params/` et `patches/`
// s'écrivent là, jamais dans la vraie bibliothèque.
func withColdRoot(t *testing.T) string {
	t.Helper()
	previous := paramsRoot
	paramsRoot = t.TempDir()
	t.Cleanup(func() { paramsRoot = previous })

	return paramsRoot
}

func writeUnder(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lendingPeer — un ami qui prête : son listing (des paramètres, un correctif),
// sa date, un préréglage et un correctif zippé.
func lendingPeer(t *testing.T) *httptest.Server {
	t.Helper()
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	served := listing.New(listing.SelfSource, "ami")
	served.SetSection(listing.Params, map[string][]listing.Item{
		"ps3": {{Name: "rpcs3", Values: true, Presets: []listing.Preset{{ID: "hd-perso", Name: "HD perso"}}}},
	}, at)
	served.SetSection(listing.Patches, map[string][]listing.Item{
		"wiiu": {{Name: "NewSuperMarioBrosU", Dir: true, Files: []string{"CrashFix"}, Size: 5}},
	}, at)
	var pack bytes.Buffer
	archive := zip.NewWriter(&pack)
	entry, _ := archive.Create("CrashFix/rules.txt")
	_, _ = entry.Write([]byte("12345"))
	_ = archive.Close()

	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jeton" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		switch r.URL.Path {
		case "/api/v1/federation/listing":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": served})
		case "/api/v1/federation/listing/updated":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updatedAt": at}})
		case "/api/v1/federation/params/ps3/rpcs3":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"name": "rpcs3", "system": "PS3", "values": map[string]any{"spuBlockSize": "Mega"},
			}})
		case "/api/v1/federation/params/ps3/rpcs3/presets/hd-perso":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"name": "HD perso", "system": "PS3", "values": map[string]any{"spuBlockSize": "Giga", "inconnu": 1},
			}})
		case "/api/v1/federation/patches/wiiu/NewSuperMarioBrosU":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(pack.Bytes())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(peer.Close)

	return peer
}

// LES CORRECTIFS D'ICI (chantier 15) : listés par le listing, servis à un ami
// — un dossier zippé à la volée —, supprimés ; un nom qui sortirait de
// `patches/` est refusé.
func TestLesCorrectifsDIciSeListentSeServentEtSeSuppriment(t *testing.T) {
	server, _ := listingServer(t)
	writeUnder(t, paramsRoot, "patches/wiiu/NewSuperMarioBrosU/CrashFix/rules.txt", "12345")

	router := chi.NewRouter()
	router.Get("/storage/patches", server.handleStoragePatches)
	router.Delete("/storage/patches", server.refreshing(sectionNamed(listing.Patches), server.handleStoragePatchDelete))
	router.Get("/federation/patches/{consoleId}/{name}", server.handleFederationPatch)
	call := func(method, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, path, nil))

		return response
	}

	var listed struct {
		Data struct {
			Consoles []struct {
				ConsoleID string `json:"consoleId"`
				Items     []struct {
					Name string `json:"name"`
					Dir  bool   `json:"dir"`
				} `json:"items"`
			} `json:"consoles"`
		} `json:"data"`
	}
	response := call(http.MethodGet, "/storage/patches")
	if json.Unmarshal(response.Body.Bytes(), &listed) != nil || len(listed.Data.Consoles) != 1 ||
		listed.Data.Consoles[0].ConsoleID != "wiiu" || !listed.Data.Consoles[0].Items[0].Dir {
		t.Fatalf("correctifs = %d %s", response.Code, response.Body.String())
	}

	served := call(http.MethodGet, "/federation/patches/wiiu/NewSuperMarioBrosU")
	archive, err := zip.NewReader(bytes.NewReader(served.Body.Bytes()), int64(served.Body.Len()))
	if err != nil || len(archive.File) != 1 || archive.File[0].Name != "CrashFix/rules.txt" {
		t.Fatalf("servi à un ami = %d (%v)", served.Code, err)
	}
	if bad := call(http.MethodGet, "/federation/patches/wiiu/..%2F..%2Fparams"); bad.Code == http.StatusOK {
		t.Fatalf("un nom qui sort de patches/ a été servi : %d", bad.Code)
	}

	if bad := call(http.MethodDelete, "/storage/patches?consoleId=wiiu&name=.."); bad.Code != http.StatusBadRequest {
		t.Fatalf("supprimer « .. » = %d, attendu 400", bad.Code)
	}
	if missing := call(http.MethodDelete, "/storage/patches?consoleId=wiiu&name=Absent"); missing.Code != http.StatusNotFound {
		t.Fatalf("supprimer un absent = %d, attendu 404", missing.Code)
	}
	if removed := call(http.MethodDelete, "/storage/patches?consoleId=wiiu&name=NewSuperMarioBrosU"); removed.Code != http.StatusOK {
		t.Fatalf("supprimer = %d %s", removed.Code, removed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(paramsRoot, "patches", "wiiu", "NewSuperMarioBrosU")); !os.IsNotExist(err) {
		t.Fatalf("le dossier est resté : %v", err)
	}
	response = call(http.MethodGet, "/storage/patches")
	if json.Unmarshal(response.Body.Bytes(), &listed) != nil || len(listed.Data.Consoles) != 0 {
		t.Fatalf("après la suppression, le listing le montre encore : %s", response.Body.String())
	}
}

// LE CORRECTIF D'UN AMI SE RAPATRIE : son dossier arrive zippé et se déplie
// dans `patches/<console>/` ; jamais par-dessus un qui est là ; un nom qu'il
// n'annonce pas est refusé.
func TestLeCorrectifDUnAmiSeRapatrie(t *testing.T) {
	peer := lendingPeer(t)
	server := &Server{}
	freshListing(t)
	withColdRoot(t)
	server.serverRegistry.items = []registeredServer{{ID: 3, Link: peer.URL, Token: "jeton", Hostname: "ami"}}
	server.serverRegistry.fetchedAt = time.Now()

	router := chi.NewRouter()
	router.Post("/servers/{id}/storage/{section}", server.handleRemoteStorageImport)
	router.Get("/servers/{id}/storage/patches", server.handleRemoteStoragePatches)
	importing := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/servers/3/storage/patches", strings.NewReader(body)))

		return response
	}

	if response := importing(`{"consoleId":"wiiu","name":"NewSuperMarioBrosU"}`); response.Code != http.StatusOK {
		t.Fatalf("rapatrier = %d %s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(paramsRoot, "patches", "wiiu", "NewSuperMarioBrosU", "CrashFix", "rules.txt"))
	if err != nil || string(content) != "12345" {
		t.Fatalf("le pack déplié = %q (%v)", content, err)
	}
	if again := importing(`{"consoleId":"wiiu","name":"NewSuperMarioBrosU"}`); again.Code != http.StatusConflict {
		t.Fatalf("une seconde fois = %d, attendu 409", again.Code)
	}
	if unknown := importing(`{"consoleId":"wiiu","name":"Inconnu"}`); unknown.Code != http.StatusNotFound {
		t.Fatalf("un correctif qu'il n'annonce pas = %d, attendu 404", unknown.Code)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/servers/3/storage/patches", nil))
	if !strings.Contains(response.Body.String(), `"installed":true`) {
		t.Fatalf("ses correctifs ne disent pas qu'on l'a aussi : %s", response.Body.String())
	}
}

// LES PARAMÈTRES PRÊTÉS (chantier 15, puis chantier 53) : un préréglage perso
// d'ici se prête ; ce qu'un ami prête se lit d'ici AVEC SES VALEURS, passées par
// les paramètres d'ici — un préréglage de plus pour le formulaire, qui n'écrit
// rien —, et ne s'importe plus.
func TestUnReglagePreteSeLitAvecSesValeursEtNeSImportePlus(t *testing.T) {
	server, _ := listingServer(t)
	rpcs3 := emuparams.Ref{ConsoleID: "ps3", Emulator: "rpcs3"}
	if _, err := paramsStore().Save(rpcs3, "PlayStation 3", nil, map[string]any{"spuBlockSize": "Giga"}, "Mon réglage", nil); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/federation/params/{consoleId}/{emulator}", server.handleFederationParams)
	router.Get("/federation/params/{consoleId}/{emulator}/presets/{preset}", server.handleFederationParams)
	lent := httptest.NewRecorder()
	router.ServeHTTP(lent, httptest.NewRequest(http.MethodGet, "/federation/params/ps3/rpcs3/presets/mon-reglage", nil))
	if lent.Code != http.StatusOK || !strings.Contains(lent.Body.String(), `"Giga"`) {
		t.Fatalf("prêté = %d %s", lent.Code, lent.Body.String())
	}
	shipped := httptest.NewRecorder()
	router.ServeHTTP(shipped, httptest.NewRequest(http.MethodGet, "/federation/params/ps3/rpcs3/presets/default", nil))
	if shipped.Code != http.StatusNotFound {
		t.Fatalf("un préréglage de GameFlex prêté = %d, attendu 404", shipped.Code)
	}

	peer := lendingPeer(t)
	borrower := &Server{}
	freshListing(t)
	withColdRoot(t)
	borrower.serverRegistry.items = []registeredServer{{ID: 3, Link: peer.URL, Token: "jeton", Hostname: "ami"}}
	borrower.serverRegistry.fetchedAt = time.Now()
	borrowing := chi.NewRouter()
	borrowing.Post("/servers/{id}/storage/{section}", borrower.handleRemoteStorageImport)
	borrowing.Get("/servers/{id}/storage/params", borrower.handleRemoteStorageParams)
	read := func(query string) []lentPreset {
		t.Helper()
		response := httptest.NewRecorder()
		borrowing.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/servers/3/storage/params"+query, nil))
		var answer struct {
			Data struct {
				Consoles []struct {
					ConsoleID string       `json:"consoleId"`
					Items     []lentPreset `json:"items"`
				} `json:"consoles"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &answer) != nil {
			t.Fatalf("ce qu'il prête = %d %s", response.Code, response.Body.String())
		}
		items := []lentPreset{}
		for _, section := range answer.Data.Consoles {
			items = append(items, section.Items...)
		}

		return items
	}

	items := read("?console=ps3&emulator=rpcs3")
	if len(items) != 2 {
		t.Fatalf("ses deux prêts (ses valeurs, son préréglage) = %+v", items)
	}
	values, preset := items[0], items[1]
	if values.Preset != "" || values.Name != "rpcs3" || values.Values["spuBlockSize"] != "Mega" || values.Error != "" {
		t.Fatalf("ses valeurs = %+v", values)
	}
	if preset.Preset != "hd-perso" || preset.Name != "HD perso" || preset.Values["spuBlockSize"] != "Giga" ||
		len(preset.Values) != 1 || len(preset.Ignored) != 1 || preset.Ignored[0] != "inconnu" {
		t.Fatalf("son préréglage, passé par les paramètres d'ici = %+v", preset)
	}
	if others := read("?console=ps3&emulator=autre"); len(others) != 0 {
		t.Fatalf("un autre émulateur = %+v, attendu rien", others)
	}
	if all := read(""); len(all) != 2 {
		t.Fatalf("sans filtre = %+v", all)
	}
	if written, _ := filepath.Glob(filepath.Join(paramsRoot, "params", "ps3", "presets", "ami*")); len(written) != 0 {
		t.Fatalf("lire ce qu'il prête a écrit %v", written)
	}

	response := httptest.NewRecorder()
	borrowing.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/servers/3/storage/params",
		strings.NewReader(`{"consoleId":"ps3","emulator":"rpcs3","preset":"hd-perso"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("importer ses paramètres = %d %s, attendu 400", response.Code, response.Body.String())
	}
}
