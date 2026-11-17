package metadata

import (
	"context"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLibretro — un site de vignettes fabriqué : la page d'index d'un dossier
// et ses images. Compte les requêtes.
func fakeLibretro(t *testing.T, names map[string][]string) *atomic.Int32 {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		path := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(path, "/") {
			listed, found := names[strings.TrimSuffix(path, "/")]
			if !found {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, "<html><title>Index of /x</title><table>")
			fmt.Fprint(w, `<tr><td><a href="?C=N;O=D">Name</a></td><td><a href="/Parent/">Parent Directory</a></td></tr>`)
			for _, name := range listed {
				fmt.Fprintf(w, `<tr><td><a href="%s.png">%s.png</a></td></tr>`, html.EscapeString(url.PathEscape(name)), html.EscapeString(name))
			}
			return
		}
		folder, file := filepath.Split(path)
		name := strings.TrimSuffix(file, ".png")
		for _, listed := range names[strings.TrimSuffix(folder, "/")] {
			if listed == name {
				picture := image.NewNRGBA(image.Rect(0, 0, 600, 800))
				for index := range picture.Pix {
					picture.Pix[index] = 200
				}
				picture.Set(0, 0, color.NRGBA{1, 2, 3, 255})
				_ = png.Encode(w, picture)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	previous := libretroBase
	libretroBase = server.URL
	t.Cleanup(func() { libretroBase = previous })
	indexesMutex.Lock()
	indexes = map[string]*libretroNames{}
	indexesMutex.Unlock()

	return requests
}

func rom(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("rom "+name), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestACoverIsFoundByItsCanonicalName(t *testing.T) {
	fakeLibretro(t, map[string][]string{
		"Nintendo - Super Nintendo Entertainment System/Named_Boxarts": {"7th Saga, The (USA)", "Super Metroid (Japan, USA) (En,Ja)", "Tom & Jerry (USA)"},
	})
	root := t.TempDir()
	game := rom(t, "7th Saga, The (USA).sfc")

	path, err := Image(context.Background(), onRoot(root), nil, "snes", game, "ref", KindCover, 512)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join("libretro", "cover-512.jpg")) || !strings.Contains(path, filepath.Join("metadata", "snes", "crc-")) {
		t.Fatalf("path = %s", path)
	}
	state := readLibretroSource(filepath.Dir(path))
	if got := state.Images[KindCover]; got.Match != "gameid" || got.Name != "7th Saga, The (USA)" {
		t.Fatalf("source.json = %+v", got)
	}

	// « & » arrive échappé dans la page du site, et se remplace par « _ » dans le nom du fichier.
	path, err = Image(context.Background(), onRoot(root), nil, "snes", rom(t, "Tom & Jerry (USA).sfc"), "ref2", KindCover, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := readLibretroSource(filepath.Dir(path)).Images[KindCover]; got.Name != "Tom & Jerry (USA)" {
		t.Fatalf("ampersand: %+v", got)
	}
}

func TestALooseNameFindsTheBestEdition(t *testing.T) {
	fakeLibretro(t, map[string][]string{
		"Nintendo - Nintendo 64/Named_Boxarts": {"Banjo-Kazooie (Europe) (En,Fr,De)", "Banjo-Kazooie (USA)", "Banjo-Kazooie (USA) (Beta)"},
	})
	path, err := Image(context.Background(), onRoot(t.TempDir()), nil, "n64", rom(t, "Banjo Kazooie.z64"), "ref", KindCover, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := readLibretroSource(filepath.Dir(path)).Images[KindCover]; got.Name != "Banjo-Kazooie (USA)" || got.Match != "title+platform" {
		t.Fatalf("source.json = %+v", got)
	}

	// La région du fichier l'emporte.
	path, err = Image(context.Background(), onRoot(t.TempDir()), nil, "n64", rom(t, "Banjo-Kazooie (Europe).z64"), "ref", KindCover, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := readLibretroSource(filepath.Dir(path)).Images[KindCover]; got.Name != "Banjo-Kazooie (Europe) (En,Fr,De)" {
		t.Fatalf("region: %+v", got)
	}
}

func TestAnAbsentImageIsNotAskedAgainForAMonth(t *testing.T) {
	requests := fakeLibretro(t, map[string][]string{"Nintendo - Nintendo 64/Named_Boxarts": {"Doom 64 (USA)"}})
	root := t.TempDir()
	game := rom(t, "Inconnu.z64")

	for round := 0; round < 2; round++ {
		if _, err := Image(context.Background(), onRoot(root), nil, "n64", game, "ref", KindCover, 0); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("round %d: err = %v", round, err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("%d requests, want 1 (the index)", got)
	}

	// Trente jours plus tard, on redemande.
	timeNow = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	t.Cleanup(func() { timeNow = time.Now })
	indexesMutex.Lock()
	indexes = map[string]*libretroNames{}
	indexesMutex.Unlock()
	_, _ = Image(context.Background(), onRoot(root), nil, "n64", game, "ref", KindCover, 0)
	if got := requests.Load(); got != 2 {
		t.Fatalf("%d requests after a month, want 2", got)
	}
}

func TestAConsoleWithoutLibretroFolderHasNoCover(t *testing.T) {
	requests := fakeLibretro(t, nil)
	if _, err := Image(context.Background(), onRoot(t.TempDir()), nil, "switch", rom(t, "Zelda.nsp"), "ref", KindCover, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("a console without folder asked the site")
	}
}

func TestLooseTitles(t *testing.T) {
	for _, pair := range [][2]string{
		{"7th Saga, The (USA)", "the7thsaga"},
		{"Banjo-Kazooie (USA) [!]", "banjokazooie"},
		{"Pokémon - Version Rouge (France)", "pokémonversionrouge"},
	} {
		if got := looseTitle(pair[0]); got != pair[1] {
			t.Errorf("looseTitle(%q) = %q, want %q", pair[0], got, pair[1])
		}
	}
}
