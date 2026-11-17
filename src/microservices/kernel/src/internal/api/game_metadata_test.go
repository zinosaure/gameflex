package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/auth"
	"gameflex/metadata"
	"gameflex/storage"
)

// LA FICHE D'UN JEU QUE PERSONNE NE CONNAÎT rend son seul `gameId` (une
// console récente sans compte IGDB : aucune requête au-dehors) ; 404 pour un
// jeu absent, 400 pour une console inconnue ; 401 sans jeton.
func TestGameMetadataAnswers(t *testing.T) {
	server, _ := listingServer(t, "roms/ps3/Jeu.iso")
	router := chi.NewRouter()
	router.Get("/{consoleId}/roms/{ref}/metadata", auth.Middleware(facadeTestSecret, server.handleGameMetadata))
	token := statusToken(t, "joueur", 1)
	reference := storage.RomID("ps3", "Jeu.iso")

	recorder := statusCall(t, router, token, http.MethodGet, "/ps3/roms/"+reference+"/metadata", nil)
	var answer struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("fiche = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(answer.Data) != 1 || answer.Data["gameId"] == "" || answer.Data["gameId"] == nil {
		t.Fatalf("un jeu inconnu rend plus que son gameId : %s", recorder.Body.String())
	}

	for path, want := range map[string]int{
		"/ps3/roms/" + storage.RomID("ps3", "Absent.iso") + "/metadata": http.StatusNotFound,
		"/nintendo99/roms/" + reference + "/metadata":                   http.StatusBadRequest,
	} {
		if got := statusCall(t, router, token, http.MethodGet, path, nil).Code; got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}
	if got := statusCall(t, router, "", http.MethodGet, "/ps3/roms/"+reference+"/metadata", nil).Code; got != http.StatusUnauthorized {
		t.Errorf("sans jeton = %d, want 401", got)
	}
}

// LES GESTES DE L'ADMINISTRATEUR SUR LA FICHE, par le vrai routeur :
// réservés aux administrateurs ; le texte manuel passe devant ; une image
// manuelle se dépose et se retire ; candidats et recherche refusés en 409
// sans compte ; une source s'oublie ; noms, sources et références refusés.
func TestGameMetadataAdminRoutes(t *testing.T) {
	server, _ := listingServer(t, "roms/ps3/Jeu.iso")
	server.config.JWTSecret = facadeTestSecret
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	user := statusToken(t, "ami", auth.RoleAdmin+1)
	game := "/ps3/roms/" + storage.RomID("ps3", "Jeu.iso")
	var cover bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 300, 400)))
	const art = "https://images.igdb.com/igdb/image/upload/t_cover_big_2x/co1.jpg"
	const drawing = "https://thumbnails.libretro.com/Sony%20-%20PlayStation%203/Named_Boxarts/Jeu.png"
	fakeChosenImages(t, map[string]any{"igdb " + art: cover.Bytes(), "libretro " + drawing: []byte("<svg/>")})
	picked, svg := string(chosen("igdb", art)), string(chosen("libretro", drawing))

	call := func(token, method, path string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}

	for _, step := range []struct {
		token, method, path string
		body                string
		want                int
		contains            string
	}{
		{"", http.MethodPut, game + "/metadata/manual", `{}`, http.StatusUnauthorized, ""},
		{user, http.MethodPut, game + "/metadata/manual", `{"title":"x"}`, http.StatusForbidden, ""},
		{user, http.MethodPut, game + "/metadata/manual/images/cover", picked, http.StatusForbidden, ""},
		{user, http.MethodGet, game + "/metadata/images/search?kind=cover", "", http.StatusForbidden, ""},
		{user, http.MethodGet, game + "/metadata/candidates?source=igdb", "", http.StatusForbidden, ""},
		{user, http.MethodPost, game + "/metadata/fetch", `{"source":"igdb"}`, http.StatusForbidden, ""},
		{user, http.MethodDelete, game + "/metadata?source=manual", "", http.StatusForbidden, ""},
		{admin, http.MethodPut, game + "/metadata/manual", `{"releaseDate":"1999","genres":["Action"],"title":"Mon jeu"}`, http.StatusOK, `"genres":["Action"]`},
		{user, http.MethodGet, game + "/metadata", "", http.StatusOK, `"title":"Mon jeu"`},
		{admin, http.MethodPut, game + "/metadata/manual", `{"releaseDate":"hier"}`, http.StatusBadRequest, "releaseDate"},
		{admin, http.MethodPut, game + "/metadata/manual", `["title"]`, http.StatusBadRequest, ""},
		{admin, http.MethodPut, game + "/metadata/manual", `pas du JSON`, http.StatusBadRequest, ""},
		{admin, http.MethodPut, game + "/metadata/manual", `{"note":"x"}`, http.StatusBadRequest, "unknown field"},
		{admin, http.MethodPut, game + "/metadata/manual/images/cover", picked, http.StatusOK, ""},
		{user, http.MethodGet, game + "/images/cover?w=256", "", http.StatusOK, ""},
		{admin, http.MethodGet, game + "/metadata", "", http.StatusOK, `"title":"Mon jeu"`},
		{admin, http.MethodPut, game + "/metadata/manual/images/poster", picked, http.StatusBadRequest, ""},
		{admin, http.MethodPut, game + "/metadata/manual/images/cover", svg, http.StatusUnsupportedMediaType, ""},
		// L'envoi d'un fichier est parti : seule une image choisie se pose, et chez l'hôte de sa source.
		{admin, http.MethodPut, game + "/metadata/manual/images/cover", cover.String(), http.StatusBadRequest, "{source, url}"},
		{admin, http.MethodPut, game + "/metadata/manual/images/cover", string(chosen("igdb", "https://ailleurs.invalid/co1.jpg")), http.StatusBadRequest, "not an image of its source"},
		{admin, http.MethodGet, game + "/metadata/images/search?kind=poster", "", http.StatusBadRequest, "unknown image kind"},
		// Une image seule ne se retire plus : elle se remplace (le porteur, 2026-11-14).
		{admin, http.MethodDelete, game + "/metadata/manual/images/cover", "", http.StatusMethodNotAllowed, ""},
		// Ses images seules ne se remettent plus à part : la réinitialisation de la fiche (?source=manual) remet tout.
		{admin, http.MethodDelete, game + "/metadata/manual/images", "", http.StatusNotFound, ""},
		{admin, http.MethodGet, game + "/metadata/candidates?source=igdb", "", http.StatusConflict, "service_not_configured"},
		{admin, http.MethodGet, game + "/metadata/candidates?source=libretro", "", http.StatusBadRequest, ""},
		{admin, http.MethodPost, game + "/metadata/fetch", `{"source":"steamgriddb","externalId":"42"}`, http.StatusConflict, ""},
		{admin, http.MethodPost, game + "/metadata/fetch", `{"source":"manual"}`, http.StatusBadRequest, ""},
		{admin, http.MethodPost, game + "/metadata/fetch", `source=igdb`, http.StatusBadRequest, ""},
		{admin, http.MethodDelete, game + "/metadata?source=../igdb", "", http.StatusBadRequest, ""},
		{admin, http.MethodDelete, game + "/metadata", "", http.StatusBadRequest, ""},
		{admin, http.MethodDelete, game + "/metadata?source=manual", "", http.StatusOK, ""},
		{admin, http.MethodPut, "/ps3/roms/..%2F..%2Fetc/metadata/manual", `{"title":"x"}`, http.StatusNotFound, ""},
		{admin, http.MethodPut, "/nintendo99/roms/x/metadata/manual", `{"title":"x"}`, http.StatusBadRequest, ""},
	} {
		answer := call(step.token, step.method, step.path, []byte(step.body))
		if answer.Code != step.want || !strings.Contains(answer.Body.String(), step.contains) {
			t.Fatalf("%s %s = %d %s, attendu %d %s", step.method, step.path, answer.Code, answer.Body.String(), step.want, step.contains)
		}
	}

	// La fiche réinitialisée (`?source=manual`) ne rend plus que son gameId : ni texte corrigé, ni image remplacée, et
	// jamais un mot de « manual » (le porteur, 2026-11-14).
	answer := call(user, http.MethodGet, game+"/metadata", nil)
	if strings.Contains(answer.Body.String(), "manual") {
		t.Fatalf("la fiche dit encore « manual » : %s", answer.Body.String())
	}
	var sheet struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &sheet); err != nil || len(sheet.Data) != 1 {
		t.Fatalf("après l'oubli de manual : %s", answer.Body.String())
	}
}

// LES MÉTADONNÉES D'UN JEU VIVENT AVEC LUI (le porteur, 2026-11-13) : un jeu
// de la bibliothèque dans le dossier du serveur, un jeu d'un disque interne
// sur ce disque — trouvé seul ou par `?disk=` —, au propriétaire du disque ;
// ce qui est par console reste dans le dossier du serveur.
func TestGameMetadataLivesWithItsGame(t *testing.T) {
	server, root := listingServer(t, "roms/ps3/Bibliotheque.iso")
	server.config.JWTSecret = facadeTestSecret
	disk := testDisk(t, "Gamebox", true, "ps3/Disque.iso")
	withDisks(server, disk)
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	library := "/ps3/roms/" + storage.RomID("ps3", "Bibliotheque.iso")
	onDisk := "/ps3/roms/" + storage.RomID("ps3", "Disque.iso")
	owned := os.Geteuid() == 0
	if owned {
		if err := os.Chown(disk.Dir, 4242, 4243); err != nil {
			t.Fatal(err)
		}
	}
	var cover bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 300, 400)))
	const art = "https://images.igdb.com/igdb/image/upload/t_cover_big_2x/co1.jpg"
	fakeChosenImages(t, map[string]any{"igdb " + art: cover.Bytes()})
	picked := string(chosen("igdb", art))

	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	for _, step := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPut, library + "/metadata/manual", `{"title":"Bibliothèque"}`, http.StatusOK},
		{http.MethodPut, onDisk + "/metadata/manual", `{"title":"Disque"}`, http.StatusOK},
		{http.MethodPut, onDisk + "/metadata/manual/images/cover?disk=Gamebox", picked, http.StatusOK},
		{http.MethodGet, onDisk + "/images/cover?w=256&disk=Gamebox", "", http.StatusOK},
		{http.MethodGet, onDisk + "/metadata?disk=Inconnu", "", http.StatusNotFound},
		{http.MethodGet, library + "/metadata?disk=Gamebox", "", http.StatusNotFound},
		{http.MethodPut, "/consoles/ps3/backgrounds/slots/1", picked, http.StatusOK},
	} {
		if answer := call(step.method, step.path, []byte(step.body)); answer.Code != step.want {
			t.Fatalf("%s %s = %d %s, attendu %d", step.method, step.path, answer.Code, answer.Body.String(), step.want)
		}
	}

	findManual := func(base string) []string {
		found, _ := filepath.Glob(filepath.Join(base, "metadata", "ps3", "*", "manual", "source.json"))
		return found
	}
	if got := findManual(root); len(got) != 1 {
		t.Fatalf("dossier du serveur : %v, attendu la seule fiche du jeu de la bibliothèque", got)
	}
	diskSheets := findManual(disk.Dir)
	if len(diskSheets) != 1 {
		t.Fatalf("disque : %v", diskSheets)
	}
	if content, _ := os.ReadFile(diskSheets[0]); !strings.Contains(string(content), "Disque") {
		t.Fatalf("fiche du disque : %s", content)
	}
	images, _ := filepath.Glob(filepath.Join(disk.Dir, "metadata", "ps3", "*", "manual", "cover*"))
	if len(images) < 2 { // l'image et sa réduction
		t.Fatalf("images du disque : %v", images)
	}
	if backgrounds, _ := filepath.Glob(filepath.Join(root, "metadata", "ps3", "background", "*.png")); len(backgrounds) != 1 {
		t.Fatalf("le fond de la console n'est pas dans le dossier du serveur : %v", backgrounds)
	}
	if _, err := os.Stat(filepath.Join(disk.Dir, "metadata", "ps3", "background")); !os.IsNotExist(err) {
		t.Fatalf("le fond de la console est sur le disque : %v", err)
	}

	// Au propriétaire du disque, du dossier metadata/ jusqu'au fichier.
	if owned {
		for _, path := range append(images, diskSheets[0], filepath.Join(disk.Dir, "metadata"), filepath.Dir(filepath.Dir(diskSheets[0]))) {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if stat := info.Sys().(*syscall.Stat_t); stat.Uid != 4242 || stat.Gid != 4243 {
				t.Errorf("%s appartient à %d:%d", path, stat.Uid, stat.Gid)
			}
		}
	}
}

// L'INTERRUPTEUR « ÉCRIRE LES MÉTADONNÉES SUR LE DISQUE DU JEU » (le porteur, 2026-11-14), lu au service d'identité :
// coupé, la correction d'un jeu d'un disque s'écrit au dossier du serveur et le disque n'est pas touché ; allumé, elle
// s'écrit sur le disque. Le choix est fait en un point (gameMetadataRoots), pour toutes les routes.
func TestTheGameDiskSwitchChoosesTheRoots(t *testing.T) {
	server, root := listingServer(t, "roms/ps3/Bibliotheque.iso")
	server.config.JWTSecret = facadeTestSecret
	disk := testDisk(t, "Gamebox", true, "ps3/Disque.iso")
	withDisks(server, disk)
	writeOnDisk := false
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/metadata/settings" || r.Header.Get("X-Internal-Secret") != "le-secret-interne" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]bool{"write_on_game_disk": writeOnDisk}})
	}))
	t.Cleanup(identity.Close)
	server.config.CredentialURL, server.config.InternalSecret = identity.URL, "le-secret-interne"
	forget := func() {
		metadataSettingsCache.Lock()
		metadataSettingsCache.fetchedAt = time.Time{}
		metadataSettingsCache.Unlock()
	}
	t.Cleanup(forget)
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	onDisk := "/ps3/roms/" + storage.RomID("ps3", "Disque.iso")
	sheets := func(base string) []string {
		found, _ := filepath.Glob(filepath.Join(base, "metadata", "ps3", "*", "manual", "source.json"))
		return found
	}
	put := func(title string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPut, onDisk+"/metadata/manual", strings.NewReader(`{"title":"`+title+`"}`))
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), title) {
			t.Fatalf("PUT %s = %d %s", title, recorder.Code, recorder.Body.String())
		}
	}

	// Coupé, le jeu déjà connu du serveur (son dossier, nommé par son contenu, y est) : ce dossier sert, tel quel.
	key := metadata.GameKey(filepath.Join(disk.RomsDir(), "ps3", "Disque.iso"), storage.RomID("ps3", "Disque.iso"))
	if err := os.MkdirAll(filepath.Join(root, "metadata", "ps3", key, "manual"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "metadata", "ps3", key, "manual", "source.json"), []byte(`{"source":"manual","title":"Déjà au serveur"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	forget()
	request := httptest.NewRequest(http.MethodGet, onDisk+"/metadata", nil)
	request.Header.Set("Authorization", "Bearer "+admin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), `"title":"Déjà au serveur"`) {
		t.Fatalf("coupé, le dossier du serveur n'a pas servi : %s", recorder.Body.String())
	}
	put("Au serveur")
	if got := sheets(root); len(got) != 1 || !strings.Contains(got[0], key) {
		t.Fatalf("coupé : le dossier du serveur a %v (attendu le seul %s)", got, key)
	}
	if got := sheets(disk.Dir); len(got) != 0 {
		t.Fatalf("coupé : le disque a été écrit : %v", got)
	}

	// Allumé, le disque n'a pas encore de dossier pour ce jeu : il s'y crée.

	writeOnDisk = true
	forget()
	put("Sur le disque")
	if got := sheets(disk.Dir); len(got) != 1 {
		t.Fatalf("allumé : le disque a %v", got)
	}
	if content, _ := os.ReadFile(sheets(root)[0]); !strings.Contains(string(content), "Au serveur") {
		t.Fatalf("allumé : la fiche du serveur a changé : %s", content)
	}
}

// UNE ÉCRITURE SUR UN DISQUE EN LECTURE SEULE se dit, avec sa cause, jamais un échec muet.
func TestAReadOnlyDiskSaysSo(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeMetadataError(recorder, &os.PathError{Op: "open", Path: "/mnt/ntfs/metadata/x", Err: syscall.EROFS})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"errorCode":"disk_read_only"`) {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
}
