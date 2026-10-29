package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
	"gameflex/credential/internal/secretbox"
	"gameflex/storage/smb"
)

// CHAQUE REFUS SMB DIT OÙ ALLER CORRIGER.
//
// Vus du formulaire, ils se ressemblent tous — « ça ne marche pas ». Mais
// l'un se règle dans le champ du mot de passe, l'autre dans l'interrupteur
// d'accès anonyme, un troisième dans le nom du partage, le dernier sur le
// serveur lui-même. Un message unique ferait tout réessayer au hasard.
func TestSMBProbeFailuresSayWhereToLook(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"compte", fmt.Errorf("%w (réponse)", smb.ErrUnauthorized), "username or password"},
		{"invité", fmt.Errorf("%w (réponse)", smb.ErrGuestRefused), "Turn anonymous access off"},
		{"partage", fmt.Errorf("%w (réponse)", smb.ErrShareNotFound), "no share by that name"},
		{"droits", fmt.Errorf("%w (réponse)", smb.ErrForbidden), "permission set on the server"},
		{"dossier", &os.PathError{Op: "open", Path: `roms\nes`, Err: os.ErrNotExist}, `no folder "roms"`},
		{"injoignable", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, "Nothing answers"},
	}

	for _, tc := range cases {
		if got := probeFailure("smb", "roms", tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("%s : %q ne contient pas %q", tc.name, got, tc.want)
		}
	}
}

// Une adresse où rien ne répond : la vraie erreur de la source SMB, pas une
// doublure, arrive jusqu'au bon message.
func TestAnSMBProbeWhereNothingAnswers(t *testing.T) {
	_, err := probeShare("smb", "probe", "smb://127.0.0.1:1/jeux", "", "", "")
	if err == nil {
		t.Fatal("rien n'écoute sur le port 1 : la sonde ne peut pas réussir")
	}
	if got := probeFailure("smb", "", err); !strings.Contains(got, "Nothing answers") {
		t.Fatalf("message = %q", got)
	}
}

// UN TYPE INCONNU N'EST PLUS SONDÉ COMME DU WEBDAV. Tout ce qui n'était pas
// kDrive partait vers WebDAV : avec un troisième protocole, ce repli aurait
// sondé un partage SMB en HTTP et rapporté une erreur sans rapport.
func TestAnUnknownKindIsNotProbedAsWebDAV(t *testing.T) {
	_, err := probeShare("ftp", "probe", "ftp://nas/jeux", "", "marc", "secret")
	if err == nil || !strings.Contains(err.Error(), "unsupported mount kind") {
		t.Fatalf("attendu un refus explicite, obtenu %v", err)
	}
}

// LES MOTS DE PASSE DES PARTAGES NE SORTENT QUE VERS LE KERNEL (2026-10-21).
//
// `/internal/storage/mounts` rend les montages distants AVEC leur mot de passe
// — c'est pour monter les partages. Elle ne demandait pas le secret interne :
// quiconque joignait le port publié du service (10203, 11203) les lisait. Sans
// secret ou avec un faux, rien ; avec le bon, la liste.
func TestTheMountPasswordsGoOnlyToTheKernel(t *testing.T) {
	config.DBPath = filepath.Join(t.TempDir(), "credential.db")
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Rangé scellé avec le secret de GameFlex (#459), servi en clair au kernel seul.
	sealed, err := secretbox.Seal("le-secret-des-jetons", "secret-du-partage")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO remote_mounts (name, kind, base_url, folder, username, password) VALUES ('nas', 'webdav', 'https://nas/dav', 'jeux', 'marc', ?)`, sealed); err != nil {
		t.Fatal(err)
	}

	server := &Server{db: db, config: config.Config{InternalSecret: "le-secret-interne", JWTSecret: "le-secret-des-jetons"}}
	read := func(secret string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/internal/storage/mounts", nil)
		if secret != "" {
			request.Header.Set("X-Internal-Secret", secret)
		}
		server.handleInternalRemoteMounts(recorder, request)

		return recorder
	}

	for _, secret := range []string{"", "pas-le-bon"} {
		if answer := read(secret); answer.Code != http.StatusUnauthorized || strings.Contains(answer.Body.String(), "secret-du-partage") {
			t.Fatalf("secret %q : %d %s, attendu 401 sans mot de passe", secret, answer.Code, answer.Body.String())
		}
	}
	if answer := read("le-secret-interne"); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), "secret-du-partage") {
		t.Fatalf("le kernel : %d %s, attendu 200 et la liste", answer.Code, answer.Body.String())
	}
}
