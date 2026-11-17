package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gameflex/facade"
	"gameflex/storage/remote"
)

// UN JEU PRIS CHEZ UN PAIR COMPTE SES OCTETS À LA COPIE (chantier 53, P5) : importIntoLibrary écrit à travers
// l'écrivain de la salle partagée (gameflex/facade), et le salon dit ce qui est reçu, sur le total annoncé ; à la fin,
// la réponse du geste. La salle elle-même — la file, les rangs, l'abonné tardif, le rythme — a ses tests dans son
// paquet (shared/facade/downloads_test.go).
func TestAGameFromAPeerCountsItsBytes(t *testing.T) {
	var downloads facade.Downloads
	payload := bytes.Repeat([]byte("x"), 3<<20)
	target := filepath.Join(t.TempDir(), "roms", "snes", "Game.sfc")
	id, err := downloads.Start(facade.DownloadJob{Owner: "claude", Total: int64(len(payload)), Queued: true, Run: func(ctx context.Context) (int, []byte) {
		written, err := importIntoLibrary(ctx, func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		}, target, zipRefuse)
		if err != nil || written != int64(len(payload)) {
			return 502, facade.ErrorBody("server_failed", fmt.Sprintf("copie : %d %v", written, err))
		}

		return 200, facade.OKBody(map[string]string{"name": "Game.sfc"})
	}})
	if err != nil {
		t.Fatal(err)
	}
	last := finalStep(t, &downloads, id)
	if last.Type != "done" || last.Received != int64(len(payload)) || !strings.Contains(string(last.Result), "Game.sfc") {
		t.Fatalf("dernier état : %+v", last)
	}
}

// finalStep — l'état final d'un salon de `claude`, lu jusqu'à la fermeture de son canal.
func finalStep(t *testing.T, downloads *facade.Downloads, id string) facade.DownloadStep {
	t.Helper()
	steps, last, _, known := downloads.Subscribe(id, "claude")
	if !known {
		t.Fatalf("salon %s inconnu", id)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case step, open := <-steps:
			if !open {
				return last
			}
			last = step
		case <-deadline:
			t.Fatalf("salon %s pas fini : %+v", id, last)
		}
	}
}

// UN REFUS DU PAIR NE LAISSE RIEN DANS LA BIBLIOTHÈQUE : ni fichier, ni le dossier de sa console — il naissait avant la
// demande, et un jeu inconnu laissait un `roms/<console>/` vide (vu le 2026-11-16).
func TestARefusedGameLeavesNoFolderBehind(t *testing.T) {
	target := filepath.Join(t.TempDir(), "roms", "nes", "Inconnu.nes")
	_, err := importIntoLibrary(context.Background(), func() (io.ReadCloser, error) {
		return nil, errors.New("le serveur a répondu 404 Not Found")
	}, target, zipKeep)
	if err == nil {
		t.Fatal("un refus du pair est une erreur")
	}
	if _, statErr := os.Stat(filepath.Dir(target)); !os.IsNotExist(statErr) {
		t.Fatalf("le dossier de la console reste : %v", statErr)
	}
}

// LA SOURCE OFFICIELLE EST INTERROGÉE (chantier 53, #672) : un HEAD, 200 pour
// « disponible », sa réponse sinon ; et une demande incomplète est refusée.
func TestTheOfficialSourceIsAskedAndARequestMustNameItsFile(t *testing.T) {
	buildbot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("un %s chez le buildbot : un HEAD suffit", r.Method)
		}
		if strings.Contains(r.URL.Path, "absent") {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer buildbot.Close()
	if got := remote.ProbeOfficial(context.Background(), buildbot.URL+"/fceumm_libretro.so.zip"); !got.Available || got.Source != "official" {
		t.Fatalf("présent : %+v", got)
	}
	if got := remote.ProbeOfficial(context.Background(), buildbot.URL+"/absent_libretro.so.zip"); got.Available || got.Reason != "answered 404" {
		t.Fatalf("absent : %+v", got)
	}

	server, _ := listingServer(t)
	for _, query := range []string{"", "?console=nes&section=roms&name=x", "?console=nope&section=cores&name=x", "?console=nes&section=cores"} {
		recorder := httptest.NewRecorder()
		server.handleStorageAvailability(recorder, httptest.NewRequest(http.MethodGet, "/storage/availability"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%q : %d", query, recorder.Code)
		}
	}
}

// ANNULER UN TÉLÉCHARGEMENT (le porteur, 2026-11-14 : « 3. A ») : seul le compte qui l'a lancé l'arrête ; la copie
// suivante refuse d'écrire, le fichier partiel part avec l'erreur, et le salon dit « cancelled ».
func TestACancelledDownloadStopsItsCopyAndSaysSo(t *testing.T) {
	var downloads facade.Downloads
	target := filepath.Join(t.TempDir(), "roms", "snes", "Big.sfc")
	reader, writer := io.Pipe()
	copying := make(chan struct{})
	id, _ := downloads.Start(facade.DownloadJob{Owner: "claude", Queued: true, Run: func(ctx context.Context) (int, []byte) {
		close(copying)
		if _, err := importIntoLibrary(ctx, func() (io.ReadCloser, error) { return reader, nil }, target, zipRefuse); err != nil {
			return 502, facade.ErrorBody("server_failed", err.Error())
		}

		return 200, facade.OKBody(nil)
	}})
	<-copying
	if downloads.Cancel(id, "someone-else") {
		t.Fatal("un autre compte n'annule pas")
	}
	go func() {
		_, _ = writer.Write(bytes.Repeat([]byte("x"), 64<<10))
		if !downloads.Cancel(id, "claude") {
			t.Error("le propriétaire annule")
		}
		_, _ = writer.Write(bytes.Repeat([]byte("y"), 64<<10))
		writer.Close()
	}()
	last := finalStep(t, &downloads, id)
	if last.Type != "failed" || last.Error != facade.DownloadCancelled {
		t.Fatalf("état final : %+v", last)
	}
	if _, statErr := os.Stat(target + ".part"); !os.IsNotExist(statErr) {
		t.Fatalf("le fichier partiel reste : %v", statErr)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("le fichier est arrivé malgré l'annulation : %v", statErr)
	}
	// Le dossier de la console, créé par ce téléchargement et resté vide, repart avec lui.
	if _, statErr := os.Stat(filepath.Dir(target)); !os.IsNotExist(statErr) {
		t.Fatalf("le dossier de la console reste après l'annulation : %v", statErr)
	}
	if downloads.Cancel(id, "claude") {
		t.Fatal("un téléchargement fini ne s'annule plus")
	}
}
