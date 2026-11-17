package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
)

func defaultsRouter(server *Server) http.Handler {
	router := chi.NewRouter()
	router.Get("/{consoleId}/defaults", server.handleGetConsoleDefaults)
	router.Put("/{consoleId}/defaults", server.handlePutConsoleDefaults)

	return router
}

func putDefaults(t *testing.T, router http.Handler, consoleID, body string) (int, map[string]consoleDefaults) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/"+consoleID+"/defaults", strings.NewReader(body)))
	var view map[string]consoleDefaults
	if recorder.Code == http.StatusOK {
		var envelope struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
		raw := envelope.Data
		view = map[string]consoleDefaults{}
		for _, key := range []string{"stored", "inUse"} {
			var entry consoleDefaults
			_ = json.Unmarshal(raw[key], &entry)
			view[key] = entry
		}
	}

	return recorder.Code, view
}

// LA VERSION EN SERVICE D'UNE CONSOLE STREAMEX (chantier 53) : choisie, c'est
// elle qu'une partie déballe, même plus ancienne que la dernière déposée ; elle
// fait de son émulateur celui par défaut ; retirée du disque, la console revient
// au dernier déposé au lieu de ne plus rien lancer.
func TestTheVersionInServiceIsTheOneAGameUnpacks(t *testing.T) {
	server, root := requiringKyty(t, "")
	older := "KytyPS5-2026-10-01-1111111-Linux-x86_64.tar.gz"
	place(t, root, older, time.Now().Add(-time.Hour))
	place(t, root, kytyFile, time.Now())
	need, _ := server.requiredEmulatorFor(context.Background(), "ps5", "")
	if file, _, _ := server.emulatorInUse(need); file != kytyFile {
		t.Fatalf("sans choix, le dernier déposé : %q", file)
	}

	router := defaultsRouter(server)
	code, view := putDefaults(t, router, "ps5", `{"emulatorFile":"`+older+`"}`)
	if code != http.StatusOK || view["stored"].EmulatorFile != older || view["stored"].Core != "KytyPS5 (streamex)" || view["inUse"].EmulatorFile != older {
		t.Fatalf("mise en service : %d %+v", code, view)
	}
	if file, _, _ := server.emulatorInUse(need); file != older {
		t.Fatalf("la version en service : %q", file)
	}
	if core := server.streamexCoreFor("ps5", ""); core != "KytyPS5 (streamex)" {
		t.Fatalf("son émulateur par défaut : %q", core)
	}
	if core := server.streamexCoreFor("ps5", "Autre (streamex)"); core != "Autre (streamex)" {
		t.Fatalf("le choix de la fiche passe devant : %q", core)
	}

	if err := os.Remove(filepath.Join(root, "cores", "ps5", older)); err != nil {
		t.Fatal(err)
	}
	if file, _, _ := server.emulatorInUse(need); file != kytyFile {
		t.Fatalf("la version en service retirée du disque : %q", file)
	}
}

// UN NOM VENU DE LA REQUÊTE NE CHOISIT JAMAIS UN FICHIER PRIS AILLEURS : un
// chemin, un fichier absent, un fichier qu'aucune image ne déballe, un BIOS sur
// une console Streamex, une console inconnue — tout est refusé, et rien ne
// s'écrit.
func TestConsoleDefaultsRefuseWhatIsNotInstalledHere(t *testing.T) {
	server, root := requiringKyty(t, "")
	place(t, root, kytyFile, time.Now())
	router := defaultsRouter(server)
	for _, body := range []string{
		`{"emulatorFile":"../../saves/x.tar.gz"}`,
		`{"emulatorFile":"KytyPS5-2099-01-01-0000000-Linux-x86_64.tar.gz"}`,
		`{"core":"RPCS3 (streamex)"}`,
		`{"bios":"PS5UPDAT.PUP"}`,
	} {
		if code, _ := putDefaults(t, router, "ps5", body); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s : %d", body, code)
		}
	}
	if code, _ := putDefaults(t, router, "nope", `{"core":""}`); code != http.StatusNotFound {
		t.Fatalf("console inconnue : %d", code)
	}
	if _, err := os.Stat(filepath.Join(server.storageRoot, "defaults", "ps5.json")); !os.IsNotExist(err) {
		t.Fatalf("rien ne devait s'écrire : %v", err)
	}
}

// L'ÉMULATEUR PAR DÉFAUT D'UNE CONSOLE RÉTRO : enregistré, il part quand la
// fiche ne choisit rien ; la fiche passe devant ; supprimé du disque depuis, il
// est ignoré et le premier du registre repart. Tout effacer retire le fichier.
func TestTheDefaultRetroCoreLaunchesWhenTheSheetChoosesNothing(t *testing.T) {
	consoleID, declared := "", []string(nil)
	for _, candidate := range []string{"snes", "ps1", "gba", "nes", "megadrive"} {
		if cores := consoles.CoresFor(candidate); len(cores) >= 2 && !strings.Contains(cores[1], " ") {
			consoleID, declared = candidate, cores

			break
		}
	}
	if consoleID == "" {
		t.Skip("aucune console rétro à deux cœurs dans le registre")
	}
	second := declared[1]
	server, root := listingServer(t, "cores/"+consoleID+"/"+retro_files.CoreFileName(second))
	router := defaultsRouter(server)

	if core := server.retroCoreFor(consoleID, ""); core != "" {
		t.Fatalf("sans défaut, le runtime prend le premier : %q", core)
	}
	if code, view := putDefaults(t, router, consoleID, `{"core":"`+second+`"}`); code != http.StatusOK || view["inUse"].Core != second {
		t.Fatalf("défaut enregistré : %d %+v", code, view)
	}
	if core := server.retroCoreFor(consoleID, ""); core != second {
		t.Fatalf("le défaut part : %q", core)
	}
	if core := server.retroCoreFor(consoleID, declared[0]); core != declared[0] {
		t.Fatalf("la fiche passe devant : %q", core)
	}
	if code, _ := putDefaults(t, router, consoleID, `{"core":"`+declared[0]+`"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("un cœur qui n'est pas installé ne devient pas le défaut : %d", code)
	}

	if err := os.Remove(filepath.Join(root, "cores", consoleID, retro_files.CoreFileName(second))); err != nil {
		t.Fatal(err)
	}
	if core := server.retroCoreFor(consoleID, ""); core != "" {
		t.Fatalf("un défaut supprimé du disque est ignoré : %q", core)
	}
	if code, _ := putDefaults(t, router, consoleID, `{"core":""}`); code != http.StatusOK {
		t.Fatalf("effacer : %d", code)
	}
	if _, err := os.Stat(filepath.Join(server.storageRoot, "defaults", consoleID+".json")); !os.IsNotExist(err) {
		t.Fatalf("tout effacé, le fichier part : %v", err)
	}
}
