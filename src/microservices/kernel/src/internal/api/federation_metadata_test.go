package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/docker"
	"gameflex/metadata"
	"gameflex/storage"
)

// sheetPeer — un pair qui prête les fiches de ses jeux (chantier 57) : « Jeu.iso » dans sa bibliothèque, avec un texte
// corrigé, une jaquette et une icône posées chez lui, et « Disque.iso » sur un de ses disques internes. Son jeton de
// catalogue est `catalogue`, celui qui n'ouvre que ses consoles `consoles`, celui qui partage aussi ses fournisseurs
// `fournisseurs`.
func sheetPeer(t *testing.T) (surface *httptest.Server, game, diskGame string) {
	t.Helper()
	peer, root := listingServer(t, "roms/ps3/Jeu.iso")
	diskDir := t.TempDir()
	writeUnder(t, diskDir, "roms/ps3/Disque.iso", "y")
	peer.internalDisks = func(context.Context) []docker.InternalDisk {
		return []docker.InternalDisk{{Name: "gamebox", Dir: diskDir, Present: true}}
	}
	identity := newFakeIdentity(t, map[string]string{"catalogue": "catalog", "consoles": "devices", "fournisseurs": "catalog,metadata-providers"}, nil)
	peer.config = config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal, JWTSecret: facadeTestSecret}

	game, diskGame = storage.RomID("ps3", "Jeu.iso"), storage.RomID("ps3", "Disque.iso")
	path := filepath.Join(root, "roms", "ps3", "Jeu.iso")
	if err := metadata.SetManualText(root, "ps3", path, game, map[string]json.RawMessage{"genres": json.RawMessage(`["Action"]`), "releaseDate": json.RawMessage(`"2006"`)}); err != nil {
		t.Fatal(err)
	}
	var cover bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 300, 400)))
	for _, kind := range []string{metadata.KindCover, metadata.KindIcon} {
		if err := metadata.SetManualImage(root, "ps3", path, game, kind, cover.Bytes()); err != nil {
			t.Fatal(err)
		}
	}

	surface = httptest.NewServer(http.StripPrefix("/api/v1/federation", peer.federationRouter()))
	t.Cleanup(surface.Close)

	return surface, game, diskGame
}

// sheetClient — le kernel qui affiche, avec le pair 3 dans son registre (son lien et notre jeton chez lui), et ce
// qui y entre par son routeur, pour un administrateur.
func sheetClient(t *testing.T, link, token string) (*Server, func(method, path, body string, header ...string) *httptest.ResponseRecorder) {
	t.Helper()
	client := &Server{storageRoot: t.TempDir(), config: config.Config{JWTSecret: facadeTestSecret}}
	client.serverRegistry.items = []registeredServer{{ID: 3, Link: link, Token: token, Hostname: "ami"}}
	client.serverRegistry.fetchedAt = time.Now()
	router := client.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)

	return client, func(method, path, body string, header ...string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+admin)
		for index := 0; index+1 < len(header); index += 2 {
			request.Header.Set(header[index], header[index+1])
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
}

// LA FICHE D'UN JEU FÉDÉRÉ SE LIT CHEZ SON SERVEUR (chantier 57, P2), par deux vrais routeurs : sa fiche et sa jaquette
// relayées telles quelles (son ETag, puis 304), celle d'un jeu de son disque, un jeu qu'il n'a pas (404) ; et RIEN ne
// s'écrit chez le kernel qui affiche.
func TestFederatedGameSheetIsReadAtItsServer(t *testing.T) {
	surface, game, diskGame := sheetPeer(t)
	client, call := sheetClient(t, surface.URL, "catalogue")
	federated := "/ps3/roms/srv3%3A" + game

	sheet := call(http.MethodGet, federated+"/metadata?server=3", "")
	var answer struct {
		Data metadata.GameInfo `json:"data"`
	}
	if sheet.Code != http.StatusOK || json.Unmarshal(sheet.Body.Bytes(), &answer) != nil {
		t.Fatalf("la fiche relayée = %d %s", sheet.Code, sheet.Body.String())
	}
	if answer.Data.ReleaseDate != "2006" || len(answer.Data.Genres) != 1 || answer.Data.Genres[0] != "Action" || answer.Data.GameID == "" {
		t.Fatalf("la fiche relayée n'est pas celle du pair : %s", sheet.Body.String())
	}

	cover := call(http.MethodGet, federated+"/images/cover?server=3&w=256", "")
	etag := cover.Header().Get("ETag")
	if cover.Code != http.StatusOK || !strings.HasPrefix(cover.Header().Get("Content-Type"), "image/") || etag == "" ||
		cover.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("la jaquette relayée = %d %v", cover.Code, cover.Header())
	}
	if again := call(http.MethodGet, federated+"/images/cover?server=3&w=256", "", "If-None-Match", etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("la même jaquette, revalidée = %d (%d octets), attendu 304", again.Code, again.Body.Len())
	}
	if none := call(http.MethodGet, federated+"/images/logo?server=3", ""); none.Code != http.StatusNotFound {
		t.Fatalf("une image qu'il n'a pas = %d, attendu 404", none.Code)
	}

	onDisk := call(http.MethodGet, "/ps3/roms/srv3%3A"+diskGame+"/metadata?server=3", "")
	if onDisk.Code != http.StatusOK || !strings.Contains(onDisk.Body.String(), `"gameId"`) {
		t.Fatalf("la fiche d'un jeu de son disque = %d %s", onDisk.Code, onDisk.Body.String())
	}
	if absent := call(http.MethodGet, "/ps3/roms/srv3%3A"+storage.RomID("ps3", "Absent.iso")+"/metadata?server=3", ""); absent.Code != http.StatusNotFound {
		t.Fatalf("un jeu qu'il n'a pas = %d, attendu 404", absent.Code)
	}
	if mismatch := call(http.MethodGet, federated+"/metadata?server=4", ""); mismatch.Code != http.StatusBadRequest {
		t.Fatalf("une référence et un ?server= qui se contredisent = %d, attendu 400", mismatch.Code)
	}
	if unknown := call(http.MethodGet, "/ps3/roms/srv9%3A"+game+"/metadata?server=9", ""); unknown.Code != http.StatusNotFound {
		t.Fatalf("un serveur inconnu = %d, attendu 404", unknown.Code)
	}
	if _, err := os.Stat(filepath.Join(client.storageRoot, "metadata")); !os.IsNotExist(err) {
		t.Fatalf("lire la fiche d'un jeu fédéré a écrit chez le kernel qui affiche : %v", err)
	}
}

// QUAND LE PAIR NE LA PRÊTE PAS : il refuse notre jeton (403, qu'il réponde 401 ou que le jeton n'ouvre que ses
// consoles), il ne répond pas (502), il est d'une version plus ancienne et n'a pas la route (404). Le kernel ne se
// rabat jamais sur son propre fournisseur.
func TestFederatedGameSheetRefusals(t *testing.T) {
	surface, game, _ := sheetPeer(t)
	federated := "/ps3/roms/srv3%3A" + game

	for _, test := range []struct {
		name, link, token string
		want              int
	}{
		{"un jeton qu'il ne connaît pas", surface.URL, "inconnu", http.StatusForbidden},
		{"un jeton qui n'ouvre que ses consoles", surface.URL, "consoles", http.StatusForbidden},
		{"un pair éteint", "http://127.0.0.1:1", "catalogue", http.StatusBadGateway},
	} {
		_, call := sheetClient(t, test.link, test.token)
		if got := call(http.MethodGet, federated+"/metadata?server=3", "").Code; got != test.want {
			t.Errorf("%s, la fiche = %d, attendu %d", test.name, got, test.want)
		}
		if got := call(http.MethodGet, federated+"/images/cover?server=3", "").Code; got != test.want {
			t.Errorf("%s, la jaquette = %d, attendu %d", test.name, got, test.want)
		}
	}

	// UN PAIR D'UNE VERSION PLUS ANCIENNE : sa surface fédérée n'a que son `OPTIONS /*` pour un chemin qu'elle ne connaît
	// pas, et chi y répond 405 avant même le jeton (la release 0.11.0, le 2026-11-15) ; un téléphone répondrait 404.
	older := chi.NewRouter()
	older.Options("/*", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	older.Get("/identity", func(w http.ResponseWriter, r *http.Request) {})
	for name, handler := range map[string]http.Handler{"405": http.StripPrefix("/api/v1/federation", older), "404": http.NotFoundHandler()} {
		peer := httptest.NewServer(handler)
		t.Cleanup(peer.Close)
		_, call := sheetClient(t, peer.URL, "catalogue")
		if got := call(http.MethodGet, federated+"/metadata?server=3", ""); got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "does not lend") {
			t.Errorf("un pair sans la route (%s), la fiche = %d %s, attendu 404", name, got.Code, got.Body.String())
		}
		if got := call(http.MethodGet, federated+"/images/cover?server=3", "").Code; got != http.StatusNotFound {
			t.Errorf("un pair sans la route (%s), la jaquette = %d, attendu 404 (l'affiche générée)", name, got)
		}
	}
}

// LES GESTES SUR LA FICHE D'UN JEU FÉDÉRÉ SONT REFUSÉS (409 `federated_game`), avant toute recherche : elle se
// corrige chez son serveur. Le pair n'est même pas interrogé.
func TestFederatedGameSheetGesturesAreRefused(t *testing.T) {
	asked := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(peer.Close)
	client, call := sheetClient(t, peer.URL, "catalogue")
	federated := "/ps3/roms/srv3%3A" + storage.RomID("ps3", "Jeu.iso")

	for _, gesture := range []struct{ method, path, body string }{
		{http.MethodPut, "/metadata/manual", `{"title":"x"}`},
		{http.MethodPut, "/metadata/manual/images/cover", `{"source":"igdb","url":"https://images.igdb.com/igdb/image/upload/t_cover_big_2x/co1.jpg"}`},
		{http.MethodGet, "/metadata/images/search?kind=cover", ""},
		{http.MethodGet, "/metadata/candidates?source=igdb", ""},
		{http.MethodPost, "/metadata/fetch", `{"source":"igdb"}`},
		{http.MethodDelete, "/metadata?source=manual", ""},
	} {
		got := call(gesture.method, federated+gesture.path+map[bool]string{true: "&", false: "?"}[strings.Contains(gesture.path, "?")]+"server=3", gesture.body)
		if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "federated_game") {
			t.Errorf("%s %s = %d %s, attendu 409 federated_game", gesture.method, gesture.path, got.Code, got.Body.String())
		}
	}
	if asked != 0 {
		t.Fatalf("un geste refusé a interrogé le pair %d fois", asked)
	}
	if _, err := os.Stat(filepath.Join(client.storageRoot, "metadata")); !os.IsNotExist(err) {
		t.Fatalf("un geste refusé a écrit : %v", err)
	}
}

// UN SERVEUR FÉDÉRÉ, SOURCE DE RECHERCHE POUR MES PROPRES JEUX (chantier 57, P3), par deux vrais routeurs : « Mon
// jeu.iso » d'ici a le même contenu que « Jeu.iso » du pair, donc le même gameid. « Choisir une image » propose son
// icône (sa vignette par le relais), la choisir l'écrit ici sans adresse ; une adresse inventée est refusée ; « Ce
// n'est pas le bon jeu ? » le propose ; le désigner copie sa fiche, qui s'oublie ; un jeu téléchargé chez lui emporte
// la sienne.
func TestAFederatedServerIsASearchSource(t *testing.T) {
	surface, peerGame, _ := sheetPeer(t)
	client, call := sheetClient(t, surface.URL, "catalogue")
	writeUnder(t, client.storageRoot, "roms/ps3/Mon jeu.iso", "x")
	mine := "/ps3/roms/" + storage.RomID("ps3", "Mon jeu.iso")

	search := call(http.MethodGet, mine+"/metadata/images/search?kind=icon&q=Mon", "")
	var found struct {
		Data metadata.ImageSearch `json:"data"`
	}
	if search.Code != http.StatusOK || json.Unmarshal(search.Body.Bytes(), &found) != nil {
		t.Fatalf("la recherche d'images = %d %s", search.Code, search.Body.String())
	}
	var lent *metadata.ImageResult
	for index, item := range found.Data.Items {
		if item.Source == "server" {
			lent = &found.Data.Items[index]
		}
	}
	if lent == nil || lent.Server != 3 || lent.ServerName != "ami" || lent.Reference != peerGame || lent.Width != 300 || lent.URL != "" {
		t.Fatalf("l'icône du jeu du pair n'est pas proposée : %s", search.Body.String())
	}
	last := found.Data.Sources[len(found.Data.Sources)-1]
	if last.Source != "server" || last.Server != 3 || last.State != "ok" {
		t.Fatalf("l'état du pair, après les fournisseurs : %+v", found.Data.Sources)
	}
	if thumb := call(http.MethodGet, strings.TrimPrefix(lent.ThumbPath, "/api/v1/retrogaming"), ""); thumb.Code != http.StatusOK || !strings.HasPrefix(thumb.Header().Get("Content-Type"), "image/") {
		t.Fatalf("sa vignette par le relais = %d", thumb.Code)
	}

	chosen := fmt.Sprintf(`{"source":"server","server":3,"reference":%q}`, peerGame)
	if put := call(http.MethodPut, mine+"/metadata/manual/images/icon", chosen); put.Code != http.StatusOK {
		t.Fatalf("choisir son icône = %d %s", put.Code, put.Body.String())
	}
	written, _ := filepath.Glob(filepath.Join(client.storageRoot, "metadata", "ps3", "*", "manual", "icon.*"))
	if len(written) != 1 {
		t.Fatalf("l'icône choisie chez le pair n'est pas écrite ici : %v", written)
	}
	for body, want := range map[string]int{
		`{"source":"server","url":"https://ailleurs.invalid/x.png"}`:           http.StatusBadRequest,
		`{"source":"igdb","url":"https://ailleurs.invalid/x.png"}`:             http.StatusBadRequest,
		fmt.Sprintf(`{"source":"server","server":3,"reference":%q}`, "absent"): http.StatusNotFound,
	} {
		if got := call(http.MethodPut, mine+"/metadata/manual/images/cover", body); got.Code != want {
			t.Errorf("%s = %d %s, attendu %d", body, got.Code, got.Body.String(), want)
		}
	}

	candidates := call(http.MethodGet, mine+"/metadata/candidates?source=server:3", "")
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), `"externalId":"`+peerGame+`"`) || !strings.Contains(candidates.Body.String(), `"coverPath":"/api/v1/retrogaming/ps3/roms/srv3:`) {
		t.Fatalf("ses candidats = %d %s", candidates.Code, candidates.Body.String())
	}
	fetched := call(http.MethodPost, mine+"/metadata/fetch", fmt.Sprintf(`{"source":"server:3","externalId":%q}`, peerGame))
	if fetched.Code != http.StatusOK || !strings.Contains(fetched.Body.String(), `"source":"server"`) || !strings.Contains(fetched.Body.String(), `"match":"manual-pin"`) {
		t.Fatalf("le désigner = %d %s", fetched.Code, fetched.Body.String())
	}
	if sheet := call(http.MethodGet, mine+"/metadata", ""); !strings.Contains(sheet.Body.String(), `"genres":["Action"]`) || !strings.Contains(sheet.Body.String(), `"releaseDate":"2006"`) {
		t.Fatalf("la fiche copiée = %s", sheet.Body.String())
	}
	if forget := call(http.MethodDelete, mine+"/metadata?source=server", ""); forget.Code != http.StatusOK {
		t.Fatalf("oublier la copie = %d", forget.Code)
	}
	if sheet := call(http.MethodGet, mine+"/metadata", ""); strings.Contains(sheet.Body.String(), "Action") {
		t.Fatalf("la copie oubliée reste : %s", sheet.Body.String())
	}

	importing := chi.NewRouter()
	importing.Post("/servers/{id}/storage/{section}", client.handleRemoteStorageImport)
	recorder := httptest.NewRecorder()
	importing.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/servers/3/storage/roms",
		strings.NewReader(fmt.Sprintf(`{"consoleId":"ps3","reference":%q,"title":"Jeu","files":["Jeu.iso"]}`, peerGame))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("le télécharger = %d %s", recorder.Code, recorder.Body.String())
	}
	arrived := filepath.Join(client.storageRoot, "roms", "ps3", "Jeu.iso")
	copied, err := os.ReadFile(filepath.Join(client.storageRoot, "metadata", "ps3", metadata.GameKey(arrived, storage.RomID("ps3", "Jeu.iso")), "server", "source.json"))
	if err != nil || !strings.Contains(string(copied), `"Action"`) || !strings.Contains(string(copied), `"reference": "`+peerGame+`"`) {
		t.Fatalf("le jeu téléchargé n'a pas emporté sa fiche : %s (%v)", copied, err)
	}

	// En dernier : un serveur inconnu relit le registre, que ce kernel d'essai n'a pas (il le vide).
	if got := call(http.MethodPut, mine+"/metadata/manual/images/cover", `{"source":"server","server":9,"reference":"x"}`); got.Code != http.StatusNotFound {
		t.Errorf("un serveur inconnu = %d %s, attendu 404", got.Code, got.Body.String())
	}
}

// LES FOURNISSEURS DU PAIR NE S'OUVRENT QU'AU JETON QUI LES PARTAGE (`#719`) : sans l'option, 403 ; avec, sa route
// répond (ici 409 : le pair n'a pas de compte IGDB). Et sa recherche reste ouverte au catalogue.
func TestPeerProvidersOpenOnlyToATokenThatSharesThem(t *testing.T) {
	surface, _, _ := sheetPeer(t)
	for token, want := range map[string]int{"catalogue": http.StatusForbidden, "consoles": http.StatusForbidden, "fournisseurs": http.StatusConflict} {
		if status, body := federationCall(t, surface, http.MethodGet, "/metadata/ps3/providers/igdb/1", token); status != want {
			t.Errorf("le jeu d'un fournisseur, jeton %s = %d %v, attendu %d", token, status, body, want)
		}
	}
	if status, body := federationCall(t, surface, http.MethodGet, "/metadata/ps3/search?q=jeu", "catalogue"); status != http.StatusOK || !strings.Contains(fmt.Sprint(body), "Jeu") {
		t.Errorf("sa recherche, au catalogue = %d %v", status, body)
	}
	if status, _ := federationCall(t, surface, http.MethodGet, "/metadata/ps3/search?q=jeu&kind=poster", "catalogue"); status != http.StatusBadRequest {
		t.Errorf("une sorte d'image inconnue = %d, attendu 400", status)
	}
}

// 2.4.1 — LA FICHE D'UN JEU FÉDÉRÉ SE COPIE DANS MES MÉTADONNÉES, SANS LE JEU (le porteur, 2026-11-16) : son texte et
// ses images, dans la source `server` de `metadata/<console>/<gameid>/` du dossier du serveur — le `gameId` que le pair
// lit dans son jeu —, et aucun jeu n'arrive. Un jeu d'ici n'a rien à copier (400), un pair qui refuse notre jeton le
// dit (403).
func TestAFederatedSheetIsCopiedWithoutTheGame(t *testing.T) {
	surface, game, _ := sheetPeer(t)
	client, call := sheetClient(t, surface.URL, "catalogue")
	federated := "/ps3/roms/srv3%3A" + game

	copied := call(http.MethodPost, federated+"/metadata/copy?server=3", "")
	var answer struct {
		Data struct {
			GameID string `json:"gameId"`
		} `json:"data"`
	}
	if copied.Code != http.StatusOK || json.Unmarshal(copied.Body.Bytes(), &answer) != nil || answer.Data.GameID == "" {
		t.Fatalf("la copie = %d %s", copied.Code, copied.Body.String())
	}
	sheetDir := filepath.Join(client.storageRoot, "metadata", "ps3", answer.Data.GameID, "server")
	source, err := os.ReadFile(filepath.Join(sheetDir, "source.json"))
	if err != nil || !strings.Contains(string(source), "Action") || !strings.Contains(string(source), `"releaseDate": "2006"`) {
		t.Fatalf("la fiche copiée : %v %s", err, source)
	}
	if covers, _ := filepath.Glob(filepath.Join(sheetDir, "cover.*")); len(covers) != 1 {
		t.Fatalf("la jaquette copiée : %v", covers)
	}
	if _, err := os.Stat(filepath.Join(client.storageRoot, "roms")); !os.IsNotExist(err) {
		t.Fatalf("le jeu est arrivé avec sa fiche : %v", err)
	}

	if mine := call(http.MethodPost, "/ps3/roms/"+game+"/metadata/copy", ""); mine.Code != http.StatusBadRequest {
		t.Fatalf("un jeu d'ici = %d, attendu 400", mine.Code)
	}
	_, refusedCall := sheetClient(t, surface.URL, "inconnu")
	if refused := refusedCall(http.MethodPost, federated+"/metadata/copy?server=3", ""); refused.Code != http.StatusForbidden {
		t.Fatalf("notre jeton refusé = %d %s, attendu 403", refused.Code, refused.Body.String())
	}
}

// backgroundsPeer — un pair qui prête le fond de sa PS5 (`#740`) : « 01.jpg » et « 03.png », son remplissage par défaut
// fait. Il a lui-même un serveur fédéré : un `?server=` sur sa surface ne doit jamais l'y faire lire.
func backgroundsPeer(t *testing.T) *httptest.Server {
	t.Helper()
	peer, root := listingServer(t)
	identity := newFakeIdentity(t, map[string]string{"catalogue": "catalog", "consoles": "devices"}, nil)
	peer.config = config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal, JWTSecret: facadeTestSecret}
	peer.serverRegistry.items = []registeredServer{{ID: 3, Link: "http://127.0.0.1:1", Token: "x", Hostname: "son ami"}}
	peer.serverRegistry.fetchedAt = time.Now()
	directory := metadata.ConsoleBackgroundDir(root, "ps5")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewGray(image.Rect(0, 0, 1600, 900)))
	for _, name := range []string{"01.jpg", "03.png"} {
		if err := os.WriteFile(filepath.Join(directory, name), picture.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(directory, ".seeded"), []byte("{}"), 0o644)

	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", peer.federationRouter()))
	t.Cleanup(surface.Close)

	return surface
}

// UNE CONSOLE SANS AUCUNE IMAGE ICI MONTRE CELLES DU PREMIER SERVEUR FÉDÉRÉ (`#740` ; le porteur, 2026-11-16 : « On
// prend le premier fédéré pour ne pas se prendre la tête »), par deux vrais routeurs : le premier de l'annuaire est
// d'une version plus ancienne (405), le suivant prête son fond ; la liste le nomme, ses images se relaient (ETag, puis
// 304), un nom qui n'en est pas un ne part pas chez lui, et RIEN ne s'écrit ici. Une console qui a une image à elle
// montre les siennes.
func TestAConsoleWithoutImagesShowsTheFirstFederatedServer(t *testing.T) {
	lending := backgroundsPeer(t)
	older := chi.NewRouter()
	older.Options("/*", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	olderSurface := httptest.NewServer(http.StripPrefix("/api/v1/federation", older))
	t.Cleanup(olderSurface.Close)
	client, call := sheetClient(t, olderSurface.URL, "catalogue")
	client.serverRegistry.items = []registeredServer{
		{ID: 2, Link: olderSurface.URL, Token: "catalogue", Hostname: "ancien"},
		{ID: 3, Link: lending.URL, Token: "catalogue", Hostname: "ami"},
	}

	listed := call(http.MethodGet, "/consoles/ps5/backgrounds", "")
	var answer struct {
		Data metadata.BackgroundSlots `json:"data"`
	}
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &answer) != nil {
		t.Fatalf("la liste = %d %s", listed.Code, listed.Body.String())
	}
	if answer.Data.Server != 3 || answer.Data.ServerName != "ami" || answer.Data.Slots[0].Name != "01.jpg" || answer.Data.Slots[1].Name != "" ||
		answer.Data.Slots[2].Name != "03.png" || len(answer.Data.Extra) != 0 {
		t.Fatalf("la liste n'est pas celle du serveur « ami » : %s", listed.Body.String())
	}

	relayed := call(http.MethodGet, "/consoles/ps5/backgrounds/01.jpg?w=512&server=3", "")
	etag := relayed.Header().Get("ETag")
	if relayed.Code != http.StatusOK || !strings.HasPrefix(relayed.Header().Get("Content-Type"), "image/") || etag == "" ||
		relayed.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("l'image relayée = %d %v", relayed.Code, relayed.Header())
	}
	if again := call(http.MethodGet, "/consoles/ps5/backgrounds/01.jpg?w=512&server=3", "", "If-None-Match", etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("la même image, revalidée = %d (%d octets), attendu 304", again.Code, again.Body.Len())
	}
	for path, want := range map[string]int{
		"/consoles/ps5/backgrounds/02.jpg?server=3":          http.StatusNotFound,
		"/consoles/ps5/backgrounds/.seeded?server=3":         http.StatusNotFound,
		"/consoles/ps5/backgrounds/01.jpg?server=9":          http.StatusNotFound,
		"/consoles/ps5/backgrounds/01.jpg?server=x":          http.StatusBadRequest,
		"/consoles/playstation9/backgrounds/01.jpg?server=3": http.StatusBadRequest,
		"/consoles/ps5/backgrounds/01.jpg":                   http.StatusNotFound,
	} {
		if got := call(http.MethodGet, path, ""); got.Code != want {
			t.Errorf("%s = %d %s, attendu %d", path, got.Code, got.Body.String(), want)
		}
	}
	if _, err := os.Stat(filepath.Join(client.storageRoot, "metadata")); !os.IsNotExist(err) {
		t.Fatalf("montrer le fond d'un serveur fédéré a écrit ici : %v", err)
	}

	// Le pair ne prête que les siennes : un `?server=` sur sa surface sert son image, sans lire chez son propre ami.
	request, _ := http.NewRequest(http.MethodGet, lending.URL+"/api/v1/federation/metadata/ps5/backgrounds/03.png?server=3&w=256", nil)
	request.Header.Set("Authorization", "Bearer catalogue")
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "image/") {
		t.Fatalf("son image, avec un ?server= = %v %v", response, err)
	}
	if status, _ := federationCall(t, lending, http.MethodGet, "/metadata/ps5/backgrounds", "consoles"); status != http.StatusForbidden {
		t.Fatalf("un jeton qui n'ouvre que ses consoles = %d, attendu 403", status)
	}

	var own bytes.Buffer
	_ = png.Encode(&own, image.NewGray(image.Rect(0, 0, 640, 360)))
	if _, err := metadata.SetConsoleBackground(client.storageRoot, "ps5", 2, own.Bytes()); err != nil {
		t.Fatal(err)
	}
	mine := call(http.MethodGet, "/consoles/ps5/backgrounds", "")
	if mine.Code != http.StatusOK || strings.Contains(mine.Body.String(), `"server"`) || !strings.Contains(mine.Body.String(), `{"slot":2,"name":"02.png"}`) {
		t.Fatalf("une console qui a une image à elle = %d %s, attendu la sienne", mine.Code, mine.Body.String())
	}
}
