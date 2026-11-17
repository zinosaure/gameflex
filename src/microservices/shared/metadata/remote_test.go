package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeServices — IGDB (jeton et recherche), SteamGridDB et leurs images,
// fabriqués. Compte les requêtes et garde la dernière recherche IGDB.
func fakeServices(t *testing.T) (*atomic.Int32, *string) {
	t.Helper()
	requests := &atomic.Int32{}
	lastQuery := new(string)
	png1x1 := func(w http.ResponseWriter) {
		picture := image.NewNRGBA(image.Rect(0, 0, 40, 20))
		picture.Set(0, 0, color.NRGBA{9, 9, 9, 255})
		_ = png.Encode(w, picture)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"access_token":"jeton","expires_in":5000000}`)
		case r.URL.Path == "/igdb/games":
			if r.Header.Get("Authorization") != "Bearer jeton" || r.Header.Get("Client-ID") != "client" {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			*lastQuery = string(body)
			if strings.Contains(*lastQuery, "Inconnu") {
				fmt.Fprint(w, `[]`)
				return
			}
			// Le jeu cherché : un autre titre ne serait pas retenu (titles.go).
			name, series := "Final Fantasy VII Rebirth", "Final Fantasy"
			if strings.Contains(*lastQuery, "7th Saga") {
				name, series = "The 7th Saga", "The 7th Saga"
			}
			fmt.Fprint(w, `[{"id":7,"name":"`+name+`","cover":{"image_id":"co1"},"artworks":[{"image_id":"ar1"}],`+
				`"first_release_date":1709164800,"genres":[{"id":12,"name":"Role-playing (RPG)"},{"id":31,"name":"Adventure"}],`+
				`"involved_companies":[{"company":{"name":"Square Enix Creative Business Unit I"},"developer":true,"publisher":false},`+
				`{"company":{"name":"Square Enix"},"developer":false,"publisher":true}],`+
				`"collections":[{"name":"`+series+`"}],"franchises":[{"name":"Final Fantasy VII"}]}]`)
		case r.URL.Path == "/sgdb/search/autocomplete/Final Fantasy VII Rebirth":
			fmt.Fprint(w, `{"data":[{"id":42,"name":"Final Fantasy VII Rebirth"}]}`)
		case strings.HasPrefix(r.URL.Path, "/sgdb/logos/game/42"):
			fmt.Fprintf(w, `{"data":[{"url":"%s/img/logo.png"}]}`, server.URL)
		case strings.HasPrefix(r.URL.Path, "/sgdb/"):
			fmt.Fprint(w, `{"data":[]}`)
		case strings.HasPrefix(r.URL.Path, "/img/"):
			png1x1(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	previous := []string{igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI}
	igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI = server.URL+"/token", server.URL+"/igdb", server.URL+"/img", server.URL+"/sgdb"
	t.Cleanup(func() {
		igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI = previous[0], previous[1], previous[2], previous[3]
	})
	igdbTokenMutex.Lock()
	igdbTokens = map[string]igdbToken{}
	igdbTokenMutex.Unlock()

	return requests, lastQuery
}

func ps5Game(t *testing.T, folder string) string {
	t.Helper()
	game := filepath.Join(t.TempDir(), folder)
	writeTestFile(t, filepath.Join(game, "sce_sys", "param.json"), `{"titleId":"PPSA01234"}`)

	return game
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onRoot — un jeu de la bibliothèque : ses métadonnées et les caches sous
// le même dossier.
func onRoot(root string) Roots {
	return Roots{Server: root, Game: root}
}

var testAccounts = Accounts{"igdb": {"client_id": "client", "client_secret": "secret"}, "steamgriddb": {"api_key": "cle"}}

func TestAGameWithoutBackgroundGetsTheIGDBArtwork(t *testing.T) {
	_, lastQuery := fakeServices(t)
	root := t.TempDir()

	path, err := Image(context.Background(), onRoot(root), testAccounts, "ps5", ps5Game(t, "Final Fantasy VII Rebirth"), "ref", KindBackground, 1280)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "metadata", "ps5", "PPSA01234", "igdb", "background-1280.jpg"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	if !strings.Contains(*lastQuery, `search "Final Fantasy VII Rebirth"`) || !strings.Contains(*lastQuery, "where platforms = (167)") {
		t.Fatalf("query = %q", *lastQuery)
	}
	state := readRemoteState(filepath.Dir(path), "igdb")
	if state.Result != "found" || state.ExternalID != "7" || state.Images[KindBackground] != "background.jpg" {
		t.Fatalf("source.json = %+v", state)
	}
}

func TestTheLogoComesFromSteamGridDB(t *testing.T) {
	fakeServices(t)
	path, err := Image(context.Background(), onRoot(t.TempDir()), testAccounts, "ps5", ps5Game(t, "Final Fantasy VII Rebirth"), "ref", KindLogo, 512)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join("steamgriddb", "logo-512.png")) {
		t.Fatalf("path = %s", path)
	}
}

func TestWithoutAccountsNoServiceIsAsked(t *testing.T) {
	requests, _ := fakeServices(t)
	if _, err := Image(context.Background(), onRoot(t.TempDir()), nil, "ps5", ps5Game(t, "Final Fantasy VII Rebirth"), "ref", KindBackground, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("%d requests without accounts", requests.Load())
	}
}

func TestAnUnknownGameIsAskedOnce(t *testing.T) {
	requests, _ := fakeServices(t)
	root := t.TempDir()
	game := ps5Game(t, "Inconnu")
	accounts := Accounts{"igdb": testAccounts["igdb"]}
	for round := 0; round < 3; round++ {
		if _, err := Image(context.Background(), onRoot(root), accounts, "ps5", game, "ref", KindCover, 0); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("round %d: err = %v", round, err)
		}
	}
	// Le jeton, puis une seule recherche.
	if got := requests.Load(); got != 2 {
		t.Fatalf("%d requests, want 2", got)
	}
}

// LE TEXTE D'IGDB vient de la recherche qui sert déjà les images : gardé dans
// son source.json, rendu par Info, et une image demandée ensuite ne refait
// pas la recherche.
func TestIGDBTextIsStoredAndReturned(t *testing.T) {
	requests, lastQuery := fakeServices(t)
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	accounts := Accounts{"igdb": testAccounts["igdb"]}

	info := Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref")
	if info.GameID != "PPSA01234" || info.ReleaseDate != "2024-02-29" ||
		strings.Join(info.Publishers, "|") != "Square Enix" ||
		strings.Join(info.Developers, "|") != "Square Enix Creative Business Unit I" ||
		strings.Join(info.Genres, "|") != "Role-playing (RPG)|Adventure" {
		t.Fatalf("info = %+v", info)
	}
	for _, field := range []string{"releaseDate", "publishers", "developers", "genres"} {
		if info.Sources[field] != "igdb" {
			t.Fatalf("sources = %v", info.Sources)
		}
	}
	if info.Series != "Final Fantasy" || info.Sources["series"] != "igdb" {
		t.Fatalf("series: %+v", info)
	}
	for _, field := range []string{"first_release_date", "genres.name", "involved_companies.company.name", "involved_companies.publisher", "involved_companies.developer", "cover.image_id", "collections.name", "franchises.name"} {
		if !strings.Contains(*lastQuery, field) {
			t.Fatalf("query %q lacks %s", *lastQuery, field)
		}
	}

	state := readRemoteState(filepath.Join(root, "metadata", "ps5", "PPSA01234", "igdb"), "igdb")
	if state.Format != remoteFormat || state.ReleaseDate != "2024-02-29" || len(state.Genres) != 2 || state.Publishers[0] != "Square Enix" || strings.Join(state.Collections, "|") != "Final Fantasy" || state.text().series != "Final Fantasy" {
		t.Fatalf("source.json = %+v", state)
	}

	// Le jeton et la recherche ; l'image ne coûte que son téléchargement.
	if got := requests.Load(); got != 2 {
		t.Fatalf("%d requests, want 2", got)
	}
	if _, err := Image(context.Background(), onRoot(root), accounts, "ps5", game, "ref", KindCover, 0); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("%d requests after the cover, want 3 (no second search)", got)
	}
}

func TestWithoutAccountsIGDBIsNotAskedForText(t *testing.T) {
	requests, _ := fakeServices(t)
	info := Info(context.Background(), onRoot(t.TempDir()), nil, "ps5", ps5Game(t, "Final Fantasy VII Rebirth"), "ref")
	content, _ := json.Marshal(info)
	if string(content) != `{"gameId":"PPSA01234"}` {
		t.Fatalf("info = %s", content)
	}
	if requests.Load() != 0 {
		t.Fatalf("%d requests without accounts", requests.Load())
	}
}

// Un source.json d'avant le texte est refait, pas lu à moitié.
func TestAnOlderSourceFileIsFetchedAgain(t *testing.T) {
	requests, _ := fakeServices(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "metadata", "ps5", "PPSA01234", "igdb", "source.json"),
		`{"source":"igdb","result":"found","fetchedAt":"2026-11-11T10:00:00Z","query":"Final Fantasy VII Rebirth","urls":{"cover":"x"}}`)

	info := Info(context.Background(), onRoot(root), Accounts{"igdb": testAccounts["igdb"]}, "ps5", ps5Game(t, "Final Fantasy VII Rebirth"), "ref")
	if info.ReleaseDate != "2024-02-29" || requests.Load() != 2 {
		t.Fatalf("info = %+v after %d requests", info, requests.Load())
	}
}
