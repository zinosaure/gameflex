package x11grab

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// startXvfb lance un vrai Xvfb sur un écran libre, ou saute le test s'il n'y en
// a pas (le conteneur claude-sandbox en a un). Rend le chemin de sa socket.
func startXvfb(t *testing.T, width, height int) string {
	t.Helper()
	binary, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb absent : ce test a besoin d'un vrai serveur X")
	}
	for display := 91; display < 120; display++ {
		socket := fmt.Sprintf("/tmp/.X11-unix/X%d", display)
		if _, err := os.Stat(socket); err == nil {
			continue
		}
		command := exec.Command(binary, fmt.Sprintf(":%d", display), "-screen", "0",
			fmt.Sprintf("%dx%dx24", width, height), "-nolisten", "tcp")
		if err := command.Start(); err != nil {
			t.Fatalf("Xvfb : %v", err)
		}
		t.Cleanup(func() {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		})
		for i := 0; i < 50; i++ {
			if _, err := os.Stat(socket); err == nil {
				return socket
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("la socket de Xvfb n'est jamais apparue (%s)", socket)
	}
	t.Skip("aucun écran X libre")

	return ""
}

// clientWindow ouvre une fenêtre de premier niveau comme le ferait un
// émulateur : à sa taille à lui, avec son titre.
func clientWindow(t *testing.T, socket string, x, y, width, height int, title string) (*xgb.Conn, xproto.Window) {
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
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, window, screen.Root,
		int16(x), int16(y), uint16(width), uint16(height), 0,
		xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
		t.Fatalf("fenêtre : %v", err)
	}
	netName, _ := xproto.InternAtom(conn, false, uint16(len("_NET_WM_NAME")), "_NET_WM_NAME").Reply()
	utf8, _ := xproto.InternAtom(conn, false, uint16(len("UTF8_STRING")), "UTF8_STRING").Reply()
	xproto.ChangeProperty(conn, xproto.PropModeReplace, window, netName.Atom, utf8.Atom, 8,
		uint32(len(title)), []byte(title))
	if err := xproto.MapWindowChecked(conn, window).Check(); err != nil {
		t.Fatalf("affichage de la fenêtre : %v", err)
	}

	return conn, window
}

// La fenêtre de jeu de RPCS3 garde les 1280×720 de la sortie PS3 sur un écran
// de 640×360 ; la garde la met à la taille de l'écran, une fois, et le titre
// où RPCS3 écrit sa cadence se lit.
func TestFillScreenAndWindowTitles(t *testing.T) {
	socket := startXvfb(t, 640, 360)
	title := "FPS: 43.21 | Vulkan | 0.0.43 | Tekken 6 [BLES00635]"
	client, window := clientWindow(t, socket, 0, 0, 1280, 720, title)

	display, err := Open(socket)
	if err != nil {
		t.Fatalf("Open : %v", err)
	}
	defer display.Close()

	titles, err := display.WindowTitles()
	if err != nil || len(titles) != 1 || titles[0] != title {
		t.Fatalf("WindowTitles = %q, %v ; attendu [%q]", titles, err, title)
	}

	fixed, err := display.FillScreen()
	if err != nil || fixed != 1 {
		t.Fatalf("FillScreen = %d, %v ; attendu 1", fixed, err)
	}
	geometry, err := xproto.GetGeometry(client, xproto.Drawable(window)).Reply()
	if err != nil || geometry.X != 0 || geometry.Y != 0 || geometry.Width != 640 || geometry.Height != 360 {
		t.Fatalf("après la garde : %+v, %v ; attendu 0,0 640x360", geometry, err)
	}
	if again, _ := display.FillScreen(); again != 0 {
		t.Fatalf("une fenêtre déjà à la taille de l'écran a été retouchée (%d)", again)
	}
}
