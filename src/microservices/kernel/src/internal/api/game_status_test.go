package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/docker"
	"gameflex/storage"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
)

// statusRouter — les routes du chantier 30 derrière la vraie porte : jeton
// signé, rang lu dans le jeton.
func statusRouter(server *Server) http.Handler {
	router := chi.NewRouter()
	guard := func(next http.HandlerFunc) http.HandlerFunc { return auth.Middleware(facadeTestSecret, next) }
	router.Get("/{consoleId}/roms", guard(server.handleRetroRoms))
	router.Get("/{consoleId}/roms/{ref}/status", guard(adminOnly(server.handleGameStatus)))
	router.Post("/{consoleId}/roms/{ref}/status", guard(adminOnly(server.handleReportGame)))
	router.Get("/{consoleId}/status", guard(adminOnly(server.handleGetConsoleStatus)))
	router.Post("/{consoleId}/status", guard(adminOnly(server.handleConsoleStatus)))

	return router
}

func statusToken(t *testing.T, username string, role int) string {
	t.Helper()
	return testToken(t, auth.Claims{Username: username, RoleLevel: role})
}

func statusCall(t *testing.T, router http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	request := httptest.NewRequest(method, path, &payload)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

// listedStatuses — le statut de chaque jeu de la liste, par titre.
func listedStatuses(t *testing.T, router http.Handler, token, consoleID string) map[string]gamestatus.View {
	t.Helper()
	recorder := statusCall(t, router, token, http.MethodGet, "/"+consoleID+"/roms", nil)
	var answer struct {
		Data []struct {
			Title  string           `json:"title"`
			Status *gamestatus.View `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("liste = %d %s", recorder.Code, recorder.Body.String())
	}
	statuses := map[string]gamestatus.View{}
	for _, rom := range answer.Data {
		if rom.Status == nil {
			t.Fatalf("%s n'a pas de statut", rom.Title)
		}
		statuses[rom.Title] = *rom.Status
	}

	return statuses
}

// UN ADMINISTRATEUR SIGNALE UN JEU : la liste le montre aussitôt, avec la
// note ; les autres gardent le défaut d'une console rétro, « jouable » ; le listing que les amis
// recopient porte le signalement, sans la version, et sa date avance.
func TestAnAdministratorReportsAGame(t *testing.T) {
	server, _ := listingServer(t, "roms/ps1/Tekken 3.cue", "roms/ps1/Wipeout.cue", "cores/ps1/pcsx_rearmed_libretro.so")
	router := statusRouter(server)
	admin := statusToken(t, "gino", auth.RoleAdmin)
	if statuses := listedStatuses(t, router, admin, "ps1"); statuses["Tekken 3"].Status != gamestatus.Playable {
		t.Fatalf("avant tout signalement : %+v", statuses)
	}
	before, _ := listing.LoadHeader(listingCacheDir, listing.SelfKey)

	tekken := storage.RomID("ps1", "Tekken 3.cue")
	answer := statusCall(t, router, admin, http.MethodPost, "/ps1/roms/"+tekken+"/status",
		map[string]any{"status": gamestatus.Crashes, "note": "plante au deuxième combat"})
	if answer.Code != http.StatusOK {
		t.Fatalf("signalement = %d %s", answer.Code, answer.Body.String())
	}

	statuses := listedStatuses(t, router, admin, "ps1")
	if statuses["Tekken 3"].Status != gamestatus.Crashes || len(statuses["Tekken 3"].Notes) != 1 ||
		statuses["Tekken 3"].Notes[0].By != "gino" {
		t.Fatalf("Tekken 3 : %+v", statuses["Tekken 3"])
	}
	if statuses["Wipeout"].Status != gamestatus.Playable {
		t.Fatalf("Wipeout a bougé : %+v", statuses["Wipeout"])
	}

	items, _ := listing.LoadSection(listingCacheDir, listing.SelfKey, "ps1", listing.Roms)
	for _, item := range items {
		if item.ID != tekken {
			continue
		}
		if len(item.Reports) != 1 || item.Reports[0].Versions != nil {
			t.Fatalf("le listing doit porter le signalement sans version : %+v", item.Reports)
		}
	}
	after, _ := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("la date du listing n'a pas avancé : %v → %v", before.UpdatedAt, after.UpdatedAt)
	}

	// L'HISTORIQUE, versions comprises, pour l'administrateur.
	history := statusCall(t, router, admin, http.MethodGet, "/ps1/roms/"+tekken+"/status", nil)
	var detail struct {
		Data gameStatusAnswer `json:"data"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &detail); err != nil || history.Code != http.StatusOK {
		t.Fatalf("historique = %d %s", history.Code, history.Body.String())
	}
	if len(detail.Data.Reports) != 1 || detail.Data.Reports[0].Versions["pcsx_rearmed"] == "" {
		t.Fatalf("la version du cœur manque : %+v", detail.Data.Reports)
	}
	if len(detail.Data.Emulators) != 1 || !detail.Data.Emulators[0].Installed {
		t.Fatalf("émulateurs : %+v", detail.Data.Emulators)
	}
}

// TOUTE UNE CONSOLE D'UN GESTE : chaque jeu sans signalement à lui la suit ;
// un jeu signalé garde le sien ; l'en-tête du listing le publie. Avant le
// geste, la console dit son défaut : jouable pour le rétro.
func TestAWholeConsoleInOneGesture(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Metroid.nes", "roms/nes/Zelda.nes")
	router := statusRouter(server)
	admin := statusToken(t, "gino", auth.RoleAdmin)
	listedStatuses(t, router, admin, "nes") // le listing existe
	setting := func(consoleID string) gamestatus.Setting {
		var answer struct {
			Data gamestatus.Setting `json:"data"`
		}
		_ = json.NewDecoder(statusCall(t, router, admin, http.MethodGet, "/"+consoleID+"/status", nil).Body).Decode(&answer)

		return answer.Data
	}
	if nes, ps5 := setting("nes"), setting("ps5"); nes.Status != "" || nes.Default != gamestatus.Playable || ps5.Default != gamestatus.Untested {
		t.Fatalf("défauts : nes %+v, ps5 %+v", nes, ps5)
	}

	zelda := storage.RomID("nes", "Zelda.nes")
	if code := statusCall(t, router, admin, http.MethodPost, "/nes/roms/"+zelda+"/status",
		map[string]any{"status": gamestatus.Unplayable}).Code; code != http.StatusOK {
		t.Fatalf("signalement = %d", code)
	}
	if code := statusCall(t, router, admin, http.MethodPost, "/nes/status",
		map[string]any{"status": gamestatus.Crashes}).Code; code != http.StatusOK {
		t.Fatalf("console = %d", code)
	}
	if nes := setting("nes"); nes.Status != gamestatus.Crashes || nes.By != "gino" {
		t.Fatalf("réglage lu : %+v", nes)
	}

	statuses := listedStatuses(t, router, admin, "nes")
	if statuses["Metroid"].Status != gamestatus.Crashes || statuses["Zelda"].Status != gamestatus.Unplayable {
		t.Fatalf("statuts : %+v", statuses)
	}
	header, _ := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	if header.Statuses["nes"] != gamestatus.Crashes {
		t.Fatalf("l'en-tête ne publie pas la console : %+v", header.Statuses)
	}
}

// UN JOUEUR NE SIGNALE RIEN, et une demande fausse est refusée sans rien
// écrire.
func TestOnlyAnAdministratorReportsAndOnlyValidReports(t *testing.T) {
	server, root := listingServer(t, "roms/nes/Metroid.nes")
	router := statusRouter(server)
	metroid := storage.RomID("nes", "Metroid.nes")

	player := statusToken(t, "jerry", 2)
	if code := statusCall(t, router, player, http.MethodPost, "/nes/roms/"+metroid+"/status",
		map[string]any{"status": gamestatus.Playable}).Code; code != http.StatusForbidden {
		t.Fatalf("un joueur a signalé : %d", code)
	}
	if code := statusCall(t, router, player, http.MethodPost, "/nes/status",
		map[string]any{"status": gamestatus.Playable}).Code; code != http.StatusForbidden {
		t.Fatalf("un joueur a réglé la console : %d", code)
	}

	admin := statusToken(t, "gino", auth.RoleAdmin)
	for _, attempt := range []struct {
		path string
		body map[string]any
		want int
	}{
		{"/nes/roms/" + metroid + "/status", map[string]any{"status": "peut-être"}, http.StatusBadRequest},
		{"/nes/roms/" + metroid + "/status", map[string]any{"status": gamestatus.Playable, "emulators": []string{"snes9x"}}, http.StatusBadRequest},
		{"/nes/roms/inconnu/status", map[string]any{"status": gamestatus.Playable}, http.StatusNotFound},
		{"/console-inconnue/status", map[string]any{"status": gamestatus.Playable}, http.StatusNotFound},
	} {
		if code := statusCall(t, router, admin, http.MethodPost, attempt.path, attempt.body).Code; code != attempt.want {
			t.Fatalf("%s %v = %d, attendu %d", attempt.path, attempt.body, code, attempt.want)
		}
	}
	if console, _ := (gamestatus.Store{Dir: filepath.Join(root, "status")}).Load("nes"); len(console.Games) != 0 || console.Status != "" {
		t.Fatalf("une demande refusée a écrit : %+v", console)
	}
}

// LA VERSION D'UN ÉMULATEUR DE CONSOLE RÉCENTE : ce que l'étoile du motif de
// son image prend dans le nom du fichier installé.
func TestTheVersionOfARecentConsoleEmulator(t *testing.T) {
	server, _ := listingServer(t, "roms/switch/Mini Metro.nsp",
		"cores/switch/Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage")
	server.emulatorsRequired = func(context.Context) []docker.RequiredEmulator {
		return []docker.RequiredEmulator{
			{Console: "switch", Core: "Eden (streamex)", Pattern: "Eden-Linux-*-amd64-clang-pgo.AppImage"},
			{Console: "switch", Core: "Citron (streamex)", Pattern: "citron_nightly-*-linux-x86_64_v3.AppImage"},
		}
	}
	if version := server.emulatorVersion(context.Background(), "switch", "Eden (streamex)"); version != "de7e6e01d4" {
		t.Fatalf("version d'Eden = %q", version)
	}
	if version := server.emulatorVersion(context.Background(), "switch", "Citron (streamex)"); version != "" {
		t.Fatalf("Citron n'est pas installé, version = %q", version)
	}
	if installed := server.installedEmulators(context.Background(), "switch"); len(installed) != 1 || installed[0] != "Eden (streamex)" {
		t.Fatalf("installés : %v", installed)
	}
	for pattern, cases := range map[string]map[string]string{
		"Eden-Linux-*-amd64.AppImage": {"Eden-Linux-abc-amd64.AppImage": "abc", "Autre.AppImage": ""},
		"sans-etoile.zip":             {"sans-etoile.zip": ""},
	} {
		for name, want := range cases {
			if got := wildcardPart(pattern, name); got != want {
				t.Fatalf("wildcardPart(%q, %q) = %q, attendu %q", pattern, name, got, want)
			}
		}
	}
}

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// UN JEU DE DISQUE INTERNE a son statut dans le dossier de ce serveur, sous
// le même identifiant : signalé, il le montre dans la liste du disque.
func TestADiskGameKeepsItsStatusInTheServerFolder(t *testing.T) {
	server, _ := listingServer(t)
	diskRoot := t.TempDir()
	writeTree(t, diskRoot, "roms/snes/F-Zero.sfc")
	server.internalDisks = func(context.Context) []docker.InternalDisk {
		return []docker.InternalDisk{{Name: "Gamebox", Dir: diskRoot, Present: true}}
	}
	router := statusRouter(server)
	router.(*chi.Mux).Get("/disk/{consoleId}", auth.Middleware(facadeTestSecret, func(w http.ResponseWriter, r *http.Request) {
		server.serveDiskRoms(w, r, "Gamebox", chi.URLParam(r, "consoleId"), "")
	}))
	admin := statusToken(t, "gino", auth.RoleAdmin)

	fzero := storage.RomID("snes", "F-Zero.sfc")
	if answer := statusCall(t, router, admin, http.MethodPost, "/snes/roms/"+fzero+"/status",
		map[string]any{"status": gamestatus.Playable}); answer.Code != http.StatusOK {
		t.Fatalf("signalement d'un jeu de disque = %d %s", answer.Code, answer.Body.String())
	}
	recorder := statusCall(t, router, admin, http.MethodGet, "/disk/snes", nil)
	var answer struct {
		Data []struct {
			Status *gamestatus.View `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || len(answer.Data) != 1 ||
		answer.Data[0].Status == nil || answer.Data[0].Status.Status != gamestatus.Playable {
		t.Fatalf("liste du disque = %d %s", recorder.Code, recorder.Body.String())
	}
}
