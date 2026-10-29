package x11grab

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Un écran qui ne change pas ne se convertit plus : le tampon garde l'image
// précédente, et l'empreinte la dit identique. Une fenêtre qui s'ouvre, elle, se
// convertit.
func TestGrabNV12ConvertsOnlyChangedFrames(t *testing.T) {
	socket := startXvfb(t, 64, 64)
	display, err := Open(socket)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	t.Cleanup(display.Close)

	first := make([]byte, display.NV12Size())
	if err := display.GrabNV12(first); err != nil {
		t.Fatalf("première capture : %v", err)
	}
	hash := display.FrameHash()

	// Un tampon marqué : converti, il serait réécrit.
	marked := bytes.Repeat([]byte{0xAB}, display.NV12Size())
	if err := display.GrabNV12(marked); err != nil {
		t.Fatalf("seconde capture : %v", err)
	}
	if !bytes.Equal(marked, bytes.Repeat([]byte{0xAB}, display.NV12Size())) {
		t.Fatal("l'écran n'a pas changé, et l'image a pourtant été reconvertie")
	}
	if display.FrameHash() != hash {
		t.Fatal("l'écran n'a pas changé, et son empreinte si")
	}

	whiteWindow(t, socket)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := display.GrabNV12(marked); err != nil {
			t.Fatalf("capture après la fenêtre : %v", err)
		}
		if display.FrameHash() != hash {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("une fenêtre blanche s'est ouverte, et l'image n'a pas été reconvertie")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if bytes.Equal(marked, first) {
		t.Fatal("l'empreinte a changé, mais pas l'image convertie")
	}
}

// Un pixel échantillonné qui change, sur n'importe laquelle de ses composantes,
// change l'empreinte ; un pixel sauté, non (la limite nommée dans GrabNV12).
func TestRawFrameHashSamplesWholePixels(t *testing.T) {
	frame := make([]byte, 64*64*4)
	base := rawFrameHash(frame)
	for channel := range 4 {
		changed := append([]byte(nil), frame...)
		changed[64+channel] = 1
		if rawFrameHash(changed) == base {
			t.Fatalf("composante %d du pixel 16 changée, empreinte identique", channel)
		}
	}
	skipped := append([]byte(nil), frame...)
	skipped[4] = 1
	if rawFrameHash(skipped) != base {
		t.Fatal("le pixel 1 n'est pas échantillonné, et l'empreinte a changé")
	}
}

// whiteWindow ouvre une fenêtre blanche de 32×32 sur l'écran de `socket`.
func whiteWindow(t *testing.T, socket string) {
	t.Helper()
	conn, err := xgb.NewConnDisplay(":" + strings.TrimPrefix(socket, "/tmp/.X11-unix/X"))
	if err != nil {
		t.Fatalf("connexion X : %v", err)
	}
	t.Cleanup(conn.Close)
	screen := xproto.Setup(conn).DefaultScreen(conn)
	window, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatalf("identifiant de fenêtre : %v", err)
	}
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, window, screen.Root, 0, 0, 32, 32, 0,
		xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel,
		[]uint32{screen.WhitePixel}).Check(); err != nil {
		t.Fatalf("fenêtre : %v", err)
	}
	if err := xproto.MapWindowChecked(conn, window).Check(); err != nil {
		t.Fatalf("affichage de la fenêtre : %v", err)
	}
}
