package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// LE CANDIDAT RETENU : tous les mots qui comptent du titre cherché, les
// mêmes nombres. Les cas vus sur la dev le 2026-11-12, et leurs voisins.
func TestTitleAccepts(t *testing.T) {
	for _, test := range []struct {
		wanted, candidate string
		want              bool
	}{
		{"EA Sports FC 27", "EA Sports FC 24", false},
		{"EA Sports FC 27", "EA SPORTS FC 27", true},
		{"Final Fantasy Resonance Deluxe Edition", "FINAL FANTASY", false},
		{"Final Fantasy Resonance Deluxe Edition", "Final Fantasy Resonance: Digital Deluxe Edition", true},
		{"Final Fantasy 15", "Final Fantasy XV: Windows Edition", true},
		{"Final Fantasy 15", "Final Fantasy XIV", false},
		{"Final Fantasy XIII-2", "Final Fantasy XIII", false},
		{"Final Fantasy XIII - Lightning Returns", "Lightning Returns: Final Fantasy XIII", true},
		{"Kingdom Hearts - HD 1.5 ReMIX", "Kingdom Hearts HD 1.5 Remix", true},
		{"Kingdom Hearts - HD 1.5 ReMIX", "Kingdom Hearts HD 1.5 + 2.5 ReMIX", false},
		{"Marvels Spider Man 2 Deluxe Edition", "Marvel's Spider-Man 2", true},
		{"Marvels Spider Man 2", "Marvel's Spider-Man: Miles Morales", false},
		{"Neptunia ReVerse", "Neptunia re★Verse", true},
		{"Driveclub", "DRIVECLUB", true},
		{"Drive Club", "Driveclub", true},
		{"Octopath Traveler 0", "OCTOPATH TRAVELER 0", true},
		{"Octopath Traveler 0", "Octopath Traveler II", false},
		{"Final Fantasy VIII Remastered", "Final Fantasy VIII", false},
		{"Legend of Zelda, The - A Link to the Past", "The Legend of Zelda: A Link to the Past", true},
		{"Assassin's Creed - Ezio Trilogy", "Assassin's Creed: The Ezio Collection", false},
		{"Spider Man", "Batman", false},
	} {
		if got := titleAccepts(test.wanted, test.candidate); got != test.want {
			t.Errorf("%q / %q : %v, attendu %v", test.wanted, test.candidate, got, test.want)
		}
	}
}

// PARMI LES RETENUS : les mêmes mots d'abord, sinon le plus court ; aucun
// retenu, aucun choix.
func TestBestTitle(t *testing.T) {
	for _, test := range []struct {
		wanted string
		names  []string
		want   int
	}{
		{"Final Fantasy 15", []string{"Final Fantasy XV: Windows Edition", "Monster of the Deep: Final Fantasy XV", "Final Fantasy XV"}, 2},
		{"Final Fantasy 15", []string{"Final Fantasy XV: Windows Edition"}, 0},
		{"EA Sports FC 27", []string{"EA Sports FC 24", "EA Sports FC 25"}, -1},
		{"Final Fantasy Resonance Deluxe Edition", []string{"Final Fantasy", "Final Fantasy Resonance: Digital Deluxe Edition"}, 1},
		{"Astro Bot", []string{"Sackboy: A Big Adventure - Astro Bot Costume", "Astro Bot: Digital Deluxe Edition", "Astro Bot"}, 2},
		// Les mêmes mots l'emportent sur un candidat plus court.
		{"Tekken Tag Tournament 2", []string{"Tekken Tag Tournament 2: We Are Tekken Edition", "Tekken Tag Tournament 2"}, 1},
		// Mêmes mots tous les deux : le mieux classé, pas le plus court (le DLC).
		{"Kingdom Hearts III + Re Mind", []string{"Kingdom Hearts III + Re Mind", "Kingdom Hearts III: Re Mind"}, 0},
		{"Inconnu", nil, -1},
	} {
		if got := bestTitle(test.names, test.wanted); got != test.want {
			t.Errorf("%q : %d, attendu %d", test.wanted, got, test.want)
		}
	}
}

// LA SÉRIE se lit dans le nom du jeu, dans l'ordre d'IGDB, la marque
// possessive en dernier : ce qu'IGDB rendait pour les 31 jeux PS de la dev
// (relevé le 2026-11-13), et ses voisins.
func TestSeriesName(t *testing.T) {
	list := func(names ...string) []string { return names }
	disney := list("Disney", "Kingdom Hearts", "Pixar", "Mickey Mouse", "Final Fantasy", "Disney's The Lion King")
	for _, test := range []struct {
		game                    string
		collections, franchises []string
		want                    string
	}{
		// Signalés par le porteur.
		{"Marvel's Wolverine", list("Marvel's Spider-Man"), list("Marvel", "X-Men", "Wolverine"), "Wolverine"},
		{"Marvel's Spider-Man 2", list("Marvel's Spider-Man", "Spider-Man"), list("Marvel", "Spider-Man", "Venom"), "Marvel's Spider-Man"},
		{"Kingdom Hearts HD 1.5 Remix", nil, list("Disney", "Final Fantasy", "Kingdom Hearts"), "Kingdom Hearts"},
		{"Final Fantasy VII Remake", list("Compilation of Final Fantasy VII", "Final Fantasy", "Final Fantasy VII Remake", "Final Fantasy VII"), list("Final Fantasy"), "Final Fantasy"},
		{"Final Fantasy VII Rebirth", list("Final Fantasy", "Compilation of Final Fantasy VII", "Final Fantasy VII Remake", "Final Fantasy VII"), list("Final Fantasy"), "Final Fantasy"},
		// Ceux que « la plus longue » aurait changés.
		{"Lightning Returns: Final Fantasy XIII", list("Final Fantasy", "Final Fantasy XIII"), list("Final Fantasy"), "Final Fantasy"},
		{"Final Fantasy XIII-2", list("Final Fantasy", "Final Fantasy XIII"), list("Final Fantasy"), "Final Fantasy"},
		{"Final Fantasy XV: Pocket Edition HD", list("Final Fantasy", "Final Fantasy XV"), list("Final Fantasy"), "Final Fantasy"},
		// Les autres.
		{"Kingdom Hearts III", list("Kingdom Hearts"), disney, "Kingdom Hearts"},
		{"Kingdom Hearts III + Re Mind", nil, disney, "Kingdom Hearts"},
		{"Assassin's Creed: Ezio Trilogy", nil, list("Assassin's Creed"), "Assassin's Creed"},
		{"Tekken Tag Tournament 2", list("Tekken Tag Tournament", "Tekken"), list("Tekken"), "Tekken Tag Tournament"},
		{"Final Fantasy Resonance", nil, list("Final Fantasy"), "Final Fantasy"},
		{"Astro Bot", list("Astro"), nil, "Astro"},
		{"Driveclub", list("DriveClub"), nil, "DriveClub"},
		{"Octopath Traveler 0", list("Octopath Traveler"), nil, "Octopath Traveler"},
		{"Clair Obscur: Expedition 33", nil, nil, ""},
		// La marque seule ne passe qu'en dernier, et reste mieux que rien.
		{"Tom Clancy's Splinter Cell", nil, list("Tom Clancy", "Splinter Cell"), "Splinter Cell"},
		{"Marvel's Avengers", nil, list("Marvel"), "Marvel"},
		// Rien ne se lit dans le nom : pas de série d'IGDB.
		{"Bloodborne", list("Soulsborne"), list("FromSoftware"), ""},
	} {
		if got := seriesName(test.game, test.collections, test.franchises); got != test.want {
			t.Errorf("%q : %q, attendu %q", test.game, got, test.want)
		}
	}
}

// LA MARQUE POSSESSIVE est en tête du nom, et seulement là.
func TestBrandWords(t *testing.T) {
	for name, want := range map[string]string{
		"Marvel's Wolverine":          "marvel",
		"Tom Clancy's Splinter Cell":  "tom clancy",
		"Sid Meier’s Civilization VI": "sid meier",
		"Uncharted: Drake's Fortune":  "",
		"Marvels Wolverine":           "",
		"Assassin's Creed":            "assassin",
	} {
		if got := strings.Join(brandWords(name), " "); got != want {
			t.Errorf("%q : %q, attendu %q", name, got, want)
		}
	}
}

// fakeSearch — IGDB et SteamGridDB qui rendent, pour toute recherche, les
// candidats donnés, dans cet ordre ; et une image pour chaque adresse.
func fakeSearch(t *testing.T, names ...string) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"access_token":"jeton","expires_in":5000000}`)
		case r.URL.Path == "/igdb/games":
			body, _ := io.ReadAll(r.Body)
			games := []map[string]any{}
			for index, name := range names {
				// `where id = N;` : la relecture d'un jeu désigné.
				if strings.Contains(string(body), "where id =") && !strings.Contains(string(body), fmt.Sprintf("where id = %d;", index+1)) {
					continue
				}
				games = append(games, map[string]any{"id": index + 1, "name": name, "cover": map[string]string{"image_id": fmt.Sprint("co", index+1)},
					"first_release_date": 1700000000, "platforms": []map[string]string{{"abbreviation": "PS5"}}})
			}
			_ = json.NewEncoder(w).Encode(games)
		case strings.HasPrefix(r.URL.Path, "/sgdb/games/id/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/sgdb/games/id/"))
			if id < 1 || id > len(names) {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "name": names[id-1]}})
		case strings.HasPrefix(r.URL.Path, "/sgdb/search/autocomplete/"):
			data := []map[string]any{}
			for index, name := range names {
				data = append(data, map[string]any{"id": index + 1, "name": name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		case strings.HasPrefix(r.URL.Path, "/sgdb/logos/game/"):
			fmt.Fprintf(w, `{"data":[{"url":"%s/img/logo-%s.png"}]}`, server.URL, strings.TrimPrefix(strings.SplitN(r.URL.Path, "?", 2)[0], "/sgdb/logos/game/"))
		case strings.HasPrefix(r.URL.Path, "/sgdb/"):
			fmt.Fprint(w, `{"data":[]}`)
		case strings.HasPrefix(r.URL.Path, "/img/"):
			_, _ = w.Write(solidPNG(t))
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
}

func solidPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, transparentIcon(20, 20)); err != nil {
		t.Fatal(err)
	}

	return encoded.Bytes()
}

// UN AUTRE ÉPISODE N'EST PAS LE JEU : SteamGridDB et IGDB qui ne
// connaissent que « EA Sports FC 24 » n'ont pas « EA Sports FC 27 » — pas
// de logo, pas de jaquette, plutôt que ceux d'un autre jeu.
func TestAnotherEpisodeIsNotTaken(t *testing.T) {
	fakeSearch(t, "EA Sports FC 24")
	gameDir := t.TempDir()
	for _, service := range []remoteService{steamGridDBService{}, igdbService{}} {
		kind := KindLogo
		if service.name() == "igdb" {
			kind = KindCover
		}
		if _, err := Remote(context.Background(), service, testAccounts, gameDir, "ps4", "EA Sports FC 27", kind); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s : %v, attendu os.ErrNotExist", service.name(), err)
		}
		state := readRemoteState(filepath.Join(gameDir, service.name()), service.name())
		if state.Result != "notFound" {
			t.Fatalf("%s : %+v", service.name(), state)
		}
	}
}

// LE BON CANDIDAT, PAS LE PREMIER : « FINAL FANTASY » vient en tête pour
// « Final Fantasy Resonance Deluxe Edition » ; c'est le second qui est pris.
func TestTheRightCandidateIsTaken(t *testing.T) {
	fakeSearch(t, "Final Fantasy", "Final Fantasy Resonance: Digital Deluxe Edition")
	gameDir := t.TempDir()
	path, err := Remote(context.Background(), steamGridDBService{}, testAccounts, gameDir, "ps5", "Final Fantasy Resonance Deluxe Edition", KindLogo)
	if err != nil {
		t.Fatal(err)
	}
	state := readRemoteState(filepath.Join(gameDir, "steamgriddb"), "steamgriddb")
	if state.ExternalID != "2" || !strings.HasSuffix(state.URLs[KindLogo], "/logo-2.png") || path == "" {
		t.Fatalf("retenu : %+v", state)
	}
}

// UNE RECHERCHE D'UNE FORME ANCIENNE SE REFAIT, ET SES IMAGES PARTENT : le
// logo d'un autre jeu gardé par la forme 3 ne reste pas à côté d'un
// « introuvable ».
func TestARedoneSearchDropsItsOldImages(t *testing.T) {
	fakeSearch(t, "EA Sports FC 24")
	sourceDir := filepath.Join(t.TempDir(), "steamgriddb")
	writeTestFile(t, filepath.Join(sourceDir, "logo.png"), "un autre jeu")
	writeTestFile(t, filepath.Join(sourceDir, "logo-512.png"), "un autre jeu")
	old, _ := json.Marshal(remoteState{Source: "steamgriddb", Format: 3, Result: "found", Query: "EA Sports FC 27", FetchedAt: timeNow(),
		Name: "EA Sports FC 24", URLs: map[string]string{KindLogo: "x"}, Images: map[string]string{KindLogo: "logo.png"}})
	writeTestFile(t, filepath.Join(sourceDir, "source.json"), string(old))

	if _, err := Remote(context.Background(), steamGridDBService{}, testAccounts, filepath.Dir(sourceDir), "ps4", "EA Sports FC 27", KindLogo); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logo : %v", err)
	}
	entries, _ := os.ReadDir(sourceDir)
	if len(entries) != 1 || entries[0].Name() != "source.json" {
		t.Fatalf("reste : %v", entries)
	}
	if state := readRemoteState(sourceDir, "steamgriddb"); state.Format != remoteFormat || state.Result != "notFound" {
		t.Fatalf("état : %+v", state)
	}
}

// LA RECHERCHE N'ENVOIE PAS LES MOTS D'ÉDITION, le choix les ignore.
func TestSearchQuery(t *testing.T) {
	for wanted, want := range map[string]string{
		"Marvels Spider Man 2 Deluxe Edition":    "Marvels Spider Man 2",
		"Final Fantasy Resonance Deluxe Edition": "Final Fantasy Resonance",
		"Final Fantasy VIII Remastered":          "Final Fantasy VIII Remastered",
		"Edition":                                "Edition",
	} {
		if got := searchQuery(wanted); got != want {
			t.Errorf("%q : %q, attendu %q", wanted, got, want)
		}
	}
}
