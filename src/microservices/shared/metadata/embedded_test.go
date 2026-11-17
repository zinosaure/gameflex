package metadata

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// fakeGame — un dossier de jeu PS5 fabriqué : aucune donnée de jeu dans le
// dépôt.
func fakeGame(t *testing.T, pictures map[string]image.Image) string {
	t.Helper()
	game := filepath.Join(t.TempDir(), "Un jeu")
	if err := os.MkdirAll(filepath.Join(game, "sce_sys"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "sce_sys", "param.json"), []byte(`{"titleId":"PPSA00001"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, picture := range pictures {
		file, err := os.Create(filepath.Join(game, "sce_sys", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, picture); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}

	return game
}

func filled(width, height int, fill color.Color) image.Image {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, fill)
		}
	}

	return picture
}

func TestABackgroundIsReducedToTheRequestedWidthAndKept(t *testing.T) {
	game := fakeGame(t, map[string]image.Image{"pic0.png": filled(3000, 1500, color.NRGBA{200, 10, 10, 255})})
	root := t.TempDir()

	path, err := Embedded(root, "ps5", game, KindBackground, 300)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "ps5", "PPSA00001", "embedded", "background-512.jpg"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	file, _ := os.Open(path)
	defer file.Close()
	decoded, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if size := decoded.Bounds().Size(); size.X != 512 || size.Y != 256 {
		t.Fatalf("size = %v, want 512×256", size)
	}
	red, _, _, _ := decoded.At(10, 10).RGBA()
	if red>>8 < 180 {
		t.Fatalf("the colour is lost: red = %d", red>>8)
	}

	// Gardée : un second appel ne la refait pas.
	before, _ := os.Stat(path)
	if _, err := Embedded(root, "ps5", game, KindBackground, 512); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("the reduced image was written twice")
	}
}

func TestAnImageIsNeverEnlarged(t *testing.T) {
	game := fakeGame(t, map[string]image.Image{"icon0.png": filled(200, 200, color.White)})

	path, err := Embedded(t.TempDir(), "ps5", game, KindIcon, 1920)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := os.Open(path)
	defer file.Close()
	decoded, _ := png.Decode(file)
	if decoded.Bounds().Dx() != 200 {
		t.Fatalf("width = %d, want 200", decoded.Bounds().Dx())
	}
}

func TestTheTransparentLayerIsNotServedAsALogo(t *testing.T) {
	game := fakeGame(t, map[string]image.Image{"pic2.png": filled(64, 32, color.NRGBA{255, 255, 255, 255})})

	if _, err := Embedded(t.TempDir(), "ps5", game, KindLogo, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestAGameWithoutIdentifierOrImageHasNone(t *testing.T) {
	game := fakeGame(t, nil)
	if _, err := Embedded(t.TempDir(), "ps5", game, KindIcon, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no icon: err = %v", err)
	}

	bare := t.TempDir()
	if _, err := Embedded(t.TempDir(), "nes", bare, KindIcon, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no identifier: err = %v", err)
	}

	if _, err := Image(context.Background(), onRoot(t.TempDir()), nil, "ps5", game, "", "poster", 0); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown kind: err = %v", err)
	}
}
