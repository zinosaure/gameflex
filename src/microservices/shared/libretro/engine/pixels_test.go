package engine

import (
	"bytes"
	"math"
	"testing"
	"unsafe"
)

// Un pixel de chaque couleur primaire, dans chacun des trois formats, et une
// rangée plus large que l'image (pitch) pour vérifier qu'on ne lit pas la
// marge. Les valeurs attendues sont le BGRA que l'encodeur reçoit.
func TestToBGRA(t *testing.T) {
	// rouge, vert, bleu, en BGRA opaque.
	want := []byte{
		0, 0, 255, 255,
		0, 255, 0, 255,
		255, 0, 0, 255,
	}

	cases := []struct {
		name   string
		format int
		pitch  int
		pixels []byte
	}{
		{
			// 0RRRRRGG GGGBBBBB, petit-boutiste : octet bas d'abord.
			name: "0RGB1555", format: PixelFormat0RGB1555, pitch: 8,
			pixels: []byte{
				0x00, 0x7C, // rouge  0x7C00
				0xE0, 0x03, // vert   0x03E0
				0x1F, 0x00, // bleu   0x001F
				0xAA, 0xAA, // marge
			},
		},
		{
			// RRRRRGGG GGGBBBBB.
			name: "RGB565", format: PixelFormatRGB565, pitch: 8,
			pixels: []byte{
				0x00, 0xF8, // rouge 0xF800
				0xE0, 0x07, // vert  0x07E0
				0x1F, 0x00, // bleu  0x001F
				0xAA, 0xAA,
			},
		},
		{
			// 0xXXRRGGBB en mémoire petit-boutiste : B, G, R, X.
			name: "XRGB8888", format: PixelFormatXRGB8888, pitch: 16,
			pixels: []byte{
				0, 0, 255, 0,
				0, 255, 0, 0,
				255, 0, 0, 0,
				0xAA, 0xAA, 0xAA, 0xAA,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ToBGRA(unsafe.Pointer(&c.pixels[0]), 3, 1, c.pitch, c.format)
			if !bytes.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

// Deux rangées : la marge de la première ne doit pas glisser dans la seconde.
func TestToBGRAPitchSkipsPadding(t *testing.T) {
	pixels := []byte{
		0x00, 0xF8, 0xAA, 0xAA, // rouge, marge
		0x1F, 0x00, 0xAA, 0xAA, // bleu, marge
	}
	got := ToBGRA(unsafe.Pointer(&pixels[0]), 1, 2, 4, PixelFormatRGB565)
	want := []byte{0, 0, 255, 255, 255, 0, 0, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// AU FORMAT DU JEU, BANDES NOIRES AUTOUR (#377, décision du porteur le
// 2026-10-15). Elle étirait : une image carrée remplissait un canevas 2:1.
func TestFitBGRA(t *testing.T) {
	frame := []byte{
		1, 1, 1, 255, 2, 2, 2, 255,
		3, 3, 3, 255, 4, 4, 4, 255,
	}
	if got := FitBGRA(frame, 2, 2, 2, 2, 1); &got[0] != &frame[0] {
		t.Fatal("même taille : l'image doit être rendue telle quelle")
	}

	black := []byte{0, 0, 0, 255}
	pixel := func(v byte) []byte { return []byte{v, v, v, 255} }
	row := func(cells ...[]byte) []byte { return bytes.Join(cells, nil) }

	// Un jeu carré dans un canevas deux fois plus large : au milieu, des
	// bandes noires de chaque côté.
	got := FitBGRA(frame, 2, 2, 4, 2, 1)
	want := row(black, pixel(1), pixel(2), black, black, pixel(3), pixel(4), black)
	if !bytes.Equal(got, want) {
		t.Fatalf("pilier : got %v, want %v", got, want)
	}

	// Le format annoncé par le cœur prime sur celui de l'image : une image 2×2
	// qui s'affiche en 2:1 remplit le canevas 2:1.
	got = FitBGRA(frame, 2, 2, 4, 2, 2)
	want = row(pixel(1), pixel(1), pixel(2), pixel(2), pixel(3), pixel(3), pixel(4), pixel(4))
	if !bytes.Equal(got, want) {
		t.Fatalf("format annoncé : got %v, want %v", got, want)
	}

	// Sans format annoncé, celui de l'image.
	if got = FitBGRA(frame, 2, 2, 4, 2, 0); !bytes.Equal(got, row(black, pixel(1), pixel(2), black, black, pixel(3), pixel(4), black)) {
		t.Fatalf("sans format : got %v", got)
	}

	// Un jeu plus large que le canevas : des bandes en haut et en bas.
	wide := row(pixel(5), pixel(6), pixel(7), pixel(8))
	got = FitBGRA(wide, 4, 1, 4, 4, 4)
	want = row(black, black, black, black, pixel(5), pixel(6), pixel(7), pixel(8), black, black, black, black, black, black, black, black)
	if !bytes.Equal(got, want) {
		t.Fatalf("bandeau : got %v, want %v", got, want)
	}

	// UN FORMAT ABSURDE N'EFFACE PAS L'IMAGE : celui de l'image le remplace.
	// Dolphin annonçait un format qui n'est pas un nombre, et la Wii sortait
	// entièrement noire (2026-10-15).
	for _, absurd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e30, 1e-30, -2} {
		if got = FitBGRA(frame, 2, 2, 4, 2, absurd); !bytes.Equal(got, row(black, pixel(1), pixel(2), black, black, pixel(3), pixel(4), black)) {
			t.Fatalf("format %v : got %v", absurd, got)
		}
	}

	// Moins de 1 % d'écart (la PSP, 480×272, contre 16:9) : tout le canevas,
	// pas de filet noir.
	psp := bytes.Repeat(pixel(9), 48*27)
	got = FitBGRA(psp, 48, 27, 160, 90, 480.0/272.0)
	if !bytes.Equal(got[:4], pixel(9)) || !bytes.Equal(got[len(got)-4:], pixel(9)) {
		t.Fatalf("format voisin : les coins devraient être l'image, pas du noir (%v … %v)", got[:4], got[len(got)-4:])
	}
}

// BenchmarkFitBGRA — ce que coûte une image NES (256x240) mise au format d'un
// flux 540p : le tampon de 2 Mo alloué à chaque appel.
func BenchmarkFitBGRA(b *testing.B) {
	frame := make([]byte, 256*240*4)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FitBGRA(frame, 256, 240, 960, 540, 4.0/3)
	}
}

// BenchmarkFitBGRAInto — la même image, dans un tampon réutilisé : ce que le
// téléphone paie désormais par image.
func BenchmarkFitBGRAInto(b *testing.B) {
	frame := make([]byte, 256*240*4)
	var fitted []byte
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fitted = FitBGRAInto(fitted, frame, 256, 240, 960, 540, 4.0/3)
	}
}

// LE TAMPON RÉUTILISÉ DONNE LA MÊME IMAGE qu'un tampon neuf — y compris quand
// il porte encore l'image d'avant (les bandes noires sont repeintes).
func TestFitBGRAIntoReusesWithoutLeftovers(t *testing.T) {
	frame := make([]byte, 4*3*4)
	for i := range frame {
		frame[i] = byte(i*7 + 1)
	}
	want := FitBGRA(frame, 4, 3, 16, 9, 4.0/3)
	dirty := make([]byte, 16*9*4)
	for i := range dirty {
		dirty[i] = 0xAB
	}
	got := FitBGRAInto(dirty, frame, 4, 3, 16, 9, 4.0/3)
	if &got[0] != &dirty[0] {
		t.Fatal("le tampon fourni n'a pas servi")
	}
	if string(got) != string(want) {
		t.Fatal("l'image dans un tampon réutilisé diffère de celle d'un tampon neuf")
	}
	if small := FitBGRAInto(make([]byte, 8), frame, 4, 3, 16, 9, 4.0/3); string(small) != string(want) {
		t.Fatal("un tampon trop petit doit céder la place à un tampon neuf")
	}
}
