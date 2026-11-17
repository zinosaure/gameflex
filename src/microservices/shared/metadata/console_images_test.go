package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gameflex/consoles"
)

// fakeIcons — les fichiers bruts du dépôt des icônes, fabriqués : `files`
// dit lesquels existent dans la série, chacun servi comme un PNG
// transparent de 600×600 (un disque opaque au milieu, des coins vides).
// Compte les requêtes ; jamais le vrai réseau.
func fakeIcons(t *testing.T, files ...string) (*atomic.Int32, *httptest.Server) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("User-Agent") != fetchUserAgent {
			http.Error(w, "no user agent", http.StatusForbidden)
			return
		}
		for _, file := range files {
			if r.URL.Path == "/"+consoleIconsSeries+"/"+file {
				_ = png.Encode(w, transparentIcon(600, 600))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	previous := consoleIconsBase
	consoleIconsBase = server.URL
	t.Cleanup(func() { consoleIconsBase = previous })

	return requests, server
}

func transparentIcon(width, height int) *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx, dy := x-width/2, y-height/2
			if dx*dx+dy*dy < (height/3)*(height/3) {
				picture.Set(x, y, color.NRGBA{30, 30, 40, 255})
			}
		}
	}

	return picture
}

func decodedPNG(t *testing.T, path string) image.Image {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("%s n'est pas un PNG : %v", path, err)
	}

	return picture
}

// L'ICÔNE SE TÉLÉCHARGE UNE FOIS : la première image la va chercher, et la
// note dans `source.json` avec ce que la licence demande de citer ; une
// autre largeur ensuite ne demande plus rien.
func TestAConsoleIconIsDownloadedOnceAndKept(t *testing.T) {
	file := consoleIcons["ps1"]
	requests, _ := fakeIcons(t, file)
	root := t.TempDir()

	path, err := ConsoleImage(context.Background(), root, "ps1", 256)
	if err != nil {
		t.Fatal(err)
	}
	if picture := decodedPNG(t, path); picture.Bounds().Dx() != 256 {
		t.Fatalf("largeur %d, attendu 256", picture.Bounds().Dx())
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("%d requêtes, attendu 1", got)
	}

	var state ConsoleImageSource
	content, _ := os.ReadFile(filepath.Join(root, "metadata", "ps1", "_console", "kylebing", "source.json"))
	if err := json.Unmarshal(content, &state); err != nil {
		t.Fatal(err)
	}
	if state.Result != "found" || state.File != file || state.Author != "KyleBing" || state.License != "CC BY 4.0" ||
		state.Repository != "https://github.com/KyleBing/retro-game-console-icons" || state.Changes != "resized" ||
		state.URL != "https://github.com/KyleBing/retro-game-console-icons/blob/main/series_trimui/600w@2x/"+file || state.FetchedAt == nil {
		t.Fatalf("source.json : %s", content)
	}

	if _, err := ConsoleImage(context.Background(), root, "ps1", 512); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("%d requêtes après la seconde largeur, attendu toujours 1", got)
	}
	attribution, err := ConsoleImageAttribution(root, "ps1")
	if err != nil || attribution.Source != "kylebing" || attribution.License != "CC BY 4.0" || attribution.Original != "" {
		t.Fatalf("attribution : %+v, %v", attribution, err)
	}
}

// HORS LIGNE : une icône gardée se sert, réductions comprises, quand le
// dépôt ne répond plus. Jamais agrandie : 1280 demandés, 600 servis.
func TestAKeptIconServesOffline(t *testing.T) {
	_, server := fakeIcons(t, consoleIcons["snes"])
	root := t.TempDir()
	if _, err := ConsoleImage(context.Background(), root, "snes", 256); err != nil {
		t.Fatal(err)
	}

	server.Close()
	if err := os.RemoveAll(filepath.Join(root, "metadata", "snes", "_console", "kylebing", "reduced")); err != nil {
		t.Fatal(err)
	}
	path, err := ConsoleImage(context.Background(), root, "snes", 1280)
	if err != nil {
		t.Fatalf("hors ligne : %v", err)
	}
	if picture := decodedPNG(t, path); picture.Bounds().Dx() != 600 {
		t.Fatalf("largeur %d, attendu 600", picture.Bounds().Dx())
	}
}

// L'IMAGE DE L'ADMINISTRATEUR L'EMPORTE, quel que soit son nom, sans rien
// demander au dépôt ; la retirer rend l'icône.
func TestTheManualImageWins(t *testing.T) {
	requests, _ := fakeIcons(t, consoleIcons["n64"])
	root := t.TempDir()
	manual := filepath.Join(root, "metadata", "n64", "_console", "manual")
	if err := os.MkdirAll(manual, 0o755); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, transparentIcon(300, 200))
	if err := os.WriteFile(filepath.Join(manual, "ma console.png"), encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := ConsoleImage(context.Background(), root, "n64", 1280)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, manual) || requests.Load() != 0 {
		t.Fatalf("servi %s après %d requêtes, attendu l'image de manual/ sans requête", path, requests.Load())
	}
	if picture := decodedPNG(t, path); picture.Bounds().Dx() != 300 {
		t.Fatalf("largeur %d, attendu 300", picture.Bounds().Dx())
	}
	if attribution, _ := ConsoleImageAttribution(root, "n64"); attribution.Source != "manual" {
		t.Fatalf("attribution : %+v", attribution)
	}

	if err := RemoveManualConsoleImage(root, "n64"); err != nil {
		t.Fatal(err)
	}
	if path, err := ConsoleImage(context.Background(), root, "n64", 256); err != nil || !strings.Contains(path, "kylebing") {
		t.Fatalf("après le retrait : %s, %v", path, err)
	}
}

// UNE CONSOLE INCONNUE est une erreur ; une console streamex n'a pas
// d'icône, sans rien demander ; un fichier disparu du dépôt est noté et pas
// redemandé avant 30 jours.
func TestUnknownStreamexAndMissingIcons(t *testing.T) {
	requests, _ := fakeIcons(t)
	root := t.TempDir()

	if _, err := ConsoleImage(context.Background(), root, "playstation9", 256); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("console inconnue : %v", err)
	}
	if _, err := ConsoleImageAttribution(root, "playstation9"); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("source d'une console inconnue : %v", err)
	}
	for _, consoleID := range []string{"ps4", "switch", "rp-ps5"} {
		if _, err := ConsoleImage(context.Background(), root, consoleID, 256); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s : %v", consoleID, err)
		}
		if _, err := ConsoleImageAttribution(root, consoleID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source de %s : %v", consoleID, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("%d requêtes pour des consoles sans icône", requests.Load())
	}

	// Le fichier de la PS1 n'existe pas sur ce dépôt-là.
	if _, err := ConsoleImage(context.Background(), root, "ps1", 256); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fichier absent : %v", err)
	}
	if _, err := ConsoleImage(context.Background(), root, "ps1", 256); !errors.Is(err, os.ErrNotExist) || requests.Load() != 1 {
		t.Fatalf("redemandé : %v après %d requêtes", err, requests.Load())
	}
	previous := timeNow
	timeNow = func() time.Time { return previous().Add(31 * 24 * time.Hour) }
	t.Cleanup(func() { timeNow = previous })
	_, _ = ConsoleImage(context.Background(), root, "ps1", 256)
	if requests.Load() != 2 {
		t.Fatalf("après 31 jours : %d requêtes, attendu 2", requests.Load())
	}
}

// LA RÉDUCTION GARDE LA TRANSPARENCE : un PNG détouré reste un PNG, les
// coins vides restent vides, l'icône reste opaque.
func TestReductionKeepsAlpha(t *testing.T) {
	fakeIcons(t, consoleIcons["dreamcast"])
	path, err := ConsoleImage(context.Background(), t.TempDir(), "dreamcast", 200)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(path) != ".png" {
		t.Fatalf("%s : attendu un PNG", path)
	}
	picture := decodedPNG(t, path)
	bounds := picture.Bounds()
	if bounds.Dx() != 256 || bounds.Dy() != 256 {
		t.Fatalf("taille %v, attendu 256×256", bounds.Size())
	}
	if _, _, _, alpha := picture.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("coin d'opacité %d, attendu transparent", alpha)
	}
	if _, _, _, alpha := picture.At(bounds.Dx()/2, bounds.Dy()/2).RGBA(); alpha != 0xffff {
		t.Fatalf("centre d'opacité %d, attendu opaque", alpha)
	}
}

// UN DÉPÔT DE L'ADMINISTRATEUR remplace le précédent et refuse ce qui n'est
// pas une image.
func TestSetManualConsoleImage(t *testing.T) {
	root := t.TempDir()
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, transparentIcon(100, 50))
	if extension := ImageExtension(encoded.Bytes()); extension != ".png" {
		t.Fatalf("extension %q", extension)
	}
	if ImageExtension([]byte("<svg></svg>")) != "" {
		t.Fatal("un SVG n'est pas accepté")
	}
	if err := SetManualConsoleImage(root, "ps2", ".png", []byte("\x89PNG\r\n\x1a\nfaux")); !errors.Is(err, ErrUnreadableImage) {
		t.Fatalf("PNG illisible : %v", err)
	}
	if err := SetManualConsoleImage(root, "ps2", ".jpg", nil); !errors.Is(err, ErrUnreadableImage) {
		t.Fatalf("vide : %v", err)
	}
	if err := SetManualConsoleImage(root, "nope", ".png", encoded.Bytes()); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("console inconnue : %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "metadata", "ps2", "_console", "manual"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "metadata", "ps2", "_console", "manual", "old.webp"), []byte("RIFF"), 0o644)
	if err := SetManualConsoleImage(root, "ps2", ".png", encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "metadata", "ps2", "_console", "manual"))
	if len(entries) != 1 || entries[0].Name() != "console.png" {
		t.Fatalf("manual/ : %v", entries)
	}
}

// LE CHOIX NE NOMME QUE DES CONSOLES RÉTRO DU REGISTRE (et les ports) : une
// faute de frappe laisserait une console sans image, et une console
// streamex n'a pas d'icône tant qu'une source cohérente n'est pas trouvée.
func TestConsoleIconsNameRetroConsoles(t *testing.T) {
	for consoleID, file := range consoleIcons {
		if !consoles.Exists(consoleID) {
			t.Errorf("%s n'est pas au registre", consoleID)
		}
		if consoles.MethodOf(consoleID) != consoles.MethodRetroGaming && consoleID != consoles.PortsID {
			t.Errorf("%s n'est pas une console rétro", consoleID)
		}
		if !strings.HasSuffix(file, ".png") {
			t.Errorf("%s : %q", consoleID, file)
		}
	}
}
