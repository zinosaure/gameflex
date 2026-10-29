package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/emuparams"
)

// LE SERVEUR SERT LES MÊMES PARAMÈTRES QUE LE TÉLÉPHONE, mais n'en pose
// aucun : sans fichier, l'écran montre la liste livrée et rien n'est appliqué ;
// l'enregistrer le crée. Une valeur hors de son type est refusée, un émulateur
// sans paramètres livrés n'existe pas.
func TestTheServerServesTheEmulatorParams(t *testing.T) {
	previous := paramsRoot
	paramsRoot = t.TempDir()
	t.Cleanup(func() { paramsRoot = previous })
	server := &Server{}

	listed := httptest.NewRecorder()
	server.handleStorageParams(listed, httptest.NewRequest(http.MethodGet, "/storage/params", nil))
	var listing struct {
		Success bool `json:"success"`
		Data    struct {
			Emulators []emuparams.Emulator `json:"emulators"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listing); err != nil || listed.Code != http.StatusOK || len(listing.Data.Emulators) != len(emuparams.Shipped()) {
		t.Fatalf("liste = %d %s", listed.Code, listed.Body.String())
	}
	// Chaque émulateur livré y est (la PS3, puis la Switch, la Wii U et la
	// Xbox depuis #354) ; la PS3 et ses huit préréglages.
	var item emuparams.Emulator
	for _, candidate := range listing.Data.Emulators {
		if candidate.Emulator == "rpcs3" {
			item = candidate
		}
	}
	if item.Exists || item.Enabled || item.Console != "PlayStation 3" || len(item.Params) == 0 || len(item.Presets) != 8 {
		t.Fatalf("sans fichier : %+v", item)
	}
	if _, err := os.Stat(filepath.Join(paramsRoot, "params", "ps3", "rpcs3.json")); !os.IsNotExist(err) {
		t.Fatal("le serveur ne doit poser aucun fichier d'émulateur")
	}

	router := chi.NewRouter()
	router.Put("/storage/params/{consoleId}/{emulator}", server.handleStorageParamsWrite)
	put := func(path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))

		return recorder
	}
	if saved := put("/storage/params/ps3/rpcs3", `{"values":{"spuBlockSize":"Giga"}}`); saved.Code != http.StatusOK ||
		!strings.Contains(saved.Body.String(), `"exists":true`) || !strings.Contains(saved.Body.String(), `"enabled":true`) {
		t.Fatalf("créer = %d %s", saved.Code, saved.Body.String())
	}
	if settings := (emuparams.Store{Root: paramsRoot}).Settings(emuparams.Ref{ConsoleID: "ps3", Emulator: "rpcs3"}); settings.State != "enabled" {
		t.Fatalf("après enregistrement : %+v", settings)
	}
	if refused := put("/storage/params/ps3/rpcs3", `{"values":{"spuBlockSize":"Tera"}}`); refused.Code != http.StatusBadRequest {
		t.Fatalf("valeur hors de son type = %d, attendu 400", refused.Code)
	}
	if missing := put("/storage/params/ps2/pcsx2", `{"values":{}}`); missing.Code != http.StatusNotFound {
		t.Fatalf("émulateur sans paramètres = %d, attendu 404", missing.Code)
	}

	// « SAVE AS PRESET », puis le bouton de suppression : le préréglage de
	// l'admin s'écrit et s'efface ; celui que GameFlex livre, ni l'un ni
	// l'autre.
	router.Delete("/storage/params/{consoleId}/{emulator}/presets/{preset}", server.handleStorageParamsPresetDelete)
	remove := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, path, nil))

		return recorder
	}
	if saved := put("/storage/params/ps3/rpcs3", `{"values":{"spuBlockSize":"Mega"},"preset":"Mon S25"}`); saved.Code != http.StatusOK ||
		!strings.Contains(saved.Body.String(), `"savedPreset":"mon-s25"`) {
		t.Fatalf("enregistrer en préréglage = %d %s", saved.Code, saved.Body.String())
	}
	if refused := put("/storage/params/ps3/rpcs3", `{"values":{},"preset":"Performance"}`); refused.Code != http.StatusBadRequest {
		t.Fatalf("le nom d'un préréglage livré = %d, attendu 400", refused.Code)
	}
	if refused := remove("/storage/params/ps3/rpcs3/presets/performance"); refused.Code != http.StatusBadRequest {
		t.Fatalf("supprimer un préréglage livré = %d, attendu 400", refused.Code)
	}
	if deleted := remove("/storage/params/ps3/rpcs3/presets/mon-s25"); deleted.Code != http.StatusOK ||
		strings.Contains(deleted.Body.String(), `"mon-s25"`) {
		t.Fatalf("supprimer le sien = %d %s", deleted.Code, deleted.Body.String())
	}
	if gone := remove("/storage/params/ps3/rpcs3/presets/mon-s25"); gone.Code != http.StatusNotFound {
		t.Fatalf("un préréglage disparu = %d, attendu 404", gone.Code)
	}
}
