package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// LA TABLE DES REFUS (chantier 57, P1) : la même pour chaque kernel, qui
// l'écrit dans son enveloppe — les statuts et les codes que la page lit ;
// chaque refus a son code (chantier 59, P5).
func TestTheRefusalsOfTheGestures(t *testing.T) {
	readOnly := &os.PathError{Op: "open", Path: "/mnt/ntfs/metadata/x", Err: syscall.EROFS}
	cases := []struct {
		name    string
		refused Refused
		status  int
		code    string
		unknown bool
	}{
		{"source inconnue", Refusal(ErrUnknownSource), http.StatusBadRequest, "bad_request", false},
		{"sorte inconnue", Refusal(fmt.Errorf("x: %w", ErrUnknownKind)), http.StatusBadRequest, "bad_request", false},
		{"pas un dossier de jeu", Refusal(ErrNotAGameFolder), http.StatusBadRequest, "bad_request", false},
		{"correction invalide", Refusal(ErrInvalidManual), http.StatusBadRequest, "bad_request", false},
		{"identifiant invalide", Refusal(ErrInvalidExternalID), http.StatusBadRequest, "bad_request", false},
		{"image illisible", Refusal(ErrUnreadableImage), http.StatusUnsupportedMediaType, "image_format", false},
		{"compte manquant", Refusal(ErrServiceNotConfigured), http.StatusConflict, "service_not_configured", false},
		{"disque en lecture seule", Refusal(readOnly), http.StatusConflict, "disk_read_only", false},
		{"introuvable", Refusal(os.ErrNotExist), http.StatusNotFound, "not_found", false},
		{"source en panne", Refusal(errors.New("igdb: 500")), http.StatusBadGateway, "metadata_failed", true},
		{"choix illisible", ChosenImageRefusal(ErrInvalidChoice), http.StatusBadRequest, "bad_request", false},
		{"hôte refusé", ChosenImageRefusal(ErrImageNotAllowed), http.StatusBadRequest, "bad_request", false},
		{"trop grande", ChosenImageRefusal(ErrImageTooLarge), http.StatusRequestEntityTooLarge, "image_too_large", false},
		{"choisie illisible", ChosenImageRefusal(ErrUnreadableImage), http.StatusUnsupportedMediaType, "image_format", false},
		{"hôte en panne", ChosenImageRefusal(errors.New("dial tcp")), http.StatusBadGateway, "metadata_failed", true},
		{"console inconnue", BackgroundRefusal(ErrUnknownConsole), http.StatusBadRequest, "console_unknown", false},
		{"emplacement", BackgroundRefusal(ErrBackgroundSlot), http.StatusBadRequest, "bad_request", false},
		{"fond trop grand", BackgroundRefusal(ErrImageTooLarge), http.StatusRequestEntityTooLarge, "image_too_large", false},
	}
	for _, each := range cases {
		if each.refused.Status != each.status || each.refused.Code != each.code || each.refused.Unknown != each.unknown || each.refused.Message == "" {
			t.Errorf("%s : %+v, attendu %d %q inconnu=%v", each.name, each.refused, each.status, each.code, each.unknown)
		}
	}
}

// LE CORPS D'UNE IMAGE CHOISIE : `{source, url}`, sinon rien n'est demandé.
func TestAChosenImageIsReadFromItsBody(t *testing.T) {
	asked := ""
	fetch := func(_ context.Context, source, address string) ([]byte, error) {
		asked = source + " " + address

		return []byte("image"), nil
	}
	content, err := ReadChosenImage(context.Background(), strings.NewReader(`{"source":"igdb","url":"https://images.igdb.com/x.jpg"}`), fetch)
	if err != nil || string(content) != "image" || asked != "igdb https://images.igdb.com/x.jpg" {
		t.Fatalf("lu %q %v, demandé %q", content, err, asked)
	}
	asked = ""
	for _, body := range []string{``, `{}`, `{"source":"igdb"}`, `{"url":"https://x"}`, `[1]`} {
		if _, err := ReadChosenImage(context.Background(), strings.NewReader(body), fetch); !errors.Is(err, ErrInvalidChoice) || asked != "" {
			t.Errorf("corps %q : %v, demandé %q", body, err, asked)
		}
	}
}

// LE JUGEMENT D'UN ORPHELIN : un dossier qu'un jeu porte n'en est pas un ; un
// dossier rangé sous la référence d'un jeu présent rejoint son gameid et n'en
// est plus un ; les autres le sont.
func TestAnOrphanIsAFolderNoGameCarries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"porte", "reference-du-jeu", "parti"} {
		if err := os.MkdirAll(filepath.Join(root, "metadata", "ps5", name, "manual"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keys := map[string]bool{"porte": true, "PPSA01234": true}
	references := map[string]string{"reference-du-jeu": "PPSA01234"}

	if IsOrphan(root, "ps5", "porte", keys, references) {
		t.Fatal("le dossier d'un jeu présent jugé orphelin")
	}
	if IsOrphan(root, "ps5", "reference-du-jeu", keys, references) {
		t.Fatal("le dossier sous la référence d'un jeu présent jugé orphelin")
	}
	if _, err := os.Stat(filepath.Join(root, "metadata", "ps5", "PPSA01234")); err != nil {
		t.Fatalf("le dossier n'a pas rejoint son gameid : %v", err)
	}
	if !IsOrphan(root, "ps5", "parti", keys, references) {
		t.Fatal("un dossier qu'aucun jeu ne porte jugé présent")
	}
}
