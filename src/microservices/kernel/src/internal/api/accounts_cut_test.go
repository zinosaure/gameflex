package api

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"gameflex/kernel/internal/accounts"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/registry"
)

// accountsTestStore — la copie des comptes, branchée comme au démarrage, avec
// une liste déjà reçue. La porte est globale au paquet : retirée en fin de test.
func accountsTestStore(t *testing.T, server *Server, list ...accounts.Account) *accounts.Store {
	t.Helper()
	store := accounts.New()
	store.OnChange = server.cutAccounts
	server.accounts = store
	auth.SetGate(store.Admit)
	t.Cleanup(func() { auth.SetGate(nil) })
	store.Replace(list)

	return store
}

// UN COMPTE RETIRÉ PERD SON CANAL (chantier 20) : un compte absent de la liste
// n'en ouvre pas, et la suppression d'un compte ferme celui qu'il tenait.
func TestARemovedAccountLosesItsChannel(t *testing.T) {
	server, httpServer := facadeTestServer(t)
	store := accountsTestStore(t, server, accounts.Account{Username: "marc", RoleLevel: accounts.RoleUser, Consoles: []string{"*"}})

	if _, response, err := facadeTestDial(t, httpServer, facadeTestToken(t, "fantome")); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("un compte absent de la liste : %v, attendu 401", err)
	}
	page, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "marc"))
	if err != nil {
		t.Fatalf("marc : %v", err)
	}
	defer page.Close()
	time.Sleep(100 * time.Millisecond)

	store.Replace(nil) // marc supprimé
	page.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, err := page.ReadMessage()
		if err == nil {
			continue
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			t.Fatal("le canal de marc est resté ouvert après sa suppression")
		}

		return // fermé par le serveur : c'est ce qu'on attendait
	}
}

// UN COMPTE NE LANCE QUE SES CONSOLES — et avant que la liste n'arrive, tout
// passe, comme avant (la promesse de internal/auth).
func TestALaunchNeedsTheConsole(t *testing.T) {
	server := &Server{}
	granted := func(username, consoleID string) bool {
		var answer bool
		handler := auth.Middleware(facadeTestSecret, func(w http.ResponseWriter, r *http.Request) {
			answer = server.consoleRefusal(r, consoleID) == ""
		})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/launch", nil)
		request.Header.Set("Authorization", "Bearer "+facadeTestToken(t, username))
		handler(httptest.NewRecorder(), request)

		return answer
	}

	store := accounts.New()
	server.accounts = store
	if !granted("marc", "ps3") {
		t.Fatal("avant la liste, tout lancement doit passer")
	}

	auth.SetGate(store.Admit)
	t.Cleanup(func() { auth.SetGate(nil) })
	store.Replace([]accounts.Account{{Username: "marc", RoleLevel: accounts.RoleUser, Consoles: []string{"ps3"}}})
	if !granted("marc", "ps3") || granted("marc", "nes") || granted("marc", "rp-ps5") {
		t.Fatal("marc n'a que la PS3")
	}
}

// LE REFUS SE DIT AVEC UN CODE que la page traduit.
func TestTheRefusalCarriesACode(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeConsoleRefused(recorder, "consoleNotGranted", "ps3")
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"errorCode":"consoleNotGranted"`) {
		t.Fatalf("refus : %d %s", recorder.Code, recorder.Body.String())
	}
}

// CHANTIER 66 — UNE CONSOLE COUPÉE POUR TOUT LE SERVEUR, PUIS PAR L'ACCÈS D'UNE SESSION FÉDÉRÉE, PUIS PAR LE COMPTE :
// trois refus, dans cet ordre, chacun son code (les textes du bouton Jouer).
func TestTheThreeRefusalsComeInOrder(t *testing.T) {
	server := &Server{}
	refusal := func(claims auth.Claims, consoleID string) string {
		var answer string
		handler := auth.Middleware(facadeTestSecret, func(w http.ResponseWriter, r *http.Request) {
			answer = server.consoleRefusal(r, consoleID)
		})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/launch", nil)
		request.Header.Set("Authorization", "Bearer "+testToken(t, claims))
		handler(httptest.NewRecorder(), request)

		return answer
	}

	store := accounts.New()
	server.accounts = store
	auth.SetGate(store.Admit)
	t.Cleanup(func() { auth.SetGate(nil) })
	store.SetRights([]accounts.Access{{ID: 7, Consoles: []string{"retro", "rp-ps5"}}}, []string{"ps5"})
	store.Replace([]accounts.Account{
		{Username: "marc", RoleLevel: accounts.RoleUser, Consoles: []string{"*"}},
		{Username: "lea", RoleLevel: accounts.RoleUser, Consoles: []string{"retro"}},
	})
	marc := auth.Claims{Username: "marc"}
	viaAccess := auth.Claims{Username: "marc", Acc: 7}
	viaRevoked := auth.Claims{Username: "marc", Acc: 8}

	for _, check := range []struct {
		claims  auth.Claims
		console string
		want    string
	}{
		{marc, "ps5", "consoleDisabled"},
		{viaAccess, "ps5", "consoleDisabled"},
		{marc, "ps4", ""},
		{viaAccess, "ps4", "federationDisabled"},
		{viaAccess, "nes", ""},
		{viaAccess, "rp-ps5", ""},
		{viaRevoked, "nes", "federationDisabled"},
		{auth.Claims{Username: "lea"}, "snes", ""},
		{auth.Claims{Username: "lea"}, "ps4", "consoleNotGranted"},
	} {
		if got := refusal(check.claims, check.console); got != check.want {
			t.Fatalf("%s (accès %d) sur %s : %q, attendu %q", check.claims.Username, check.claims.Acc, check.console, got, check.want)
		}
	}
}

// DEUX CLÉS, DEUX CONSOLES : refusé avant tout. La porte vérifiait `consoleId`
// pendant que le lancement partait sur `console_id` (revue de sécurité,
// 2026-10-28) — un compte qui n'avait que la NES lançait la PS1.
func TestALaunchCannotNameTwoConsoles(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/launch",
		strings.NewReader(`{"platform":"retrogaming","consoleId":"nes","console_id":"ps1"}`))
	server.handleUnifiedLaunch(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "two different consoles") {
		t.Fatalf("deux consoles dans un lancement : %d %s, attendu le refus de la porte", recorder.Code, recorder.Body.String())
	}
}

// closeCode — le code de la trame qui ferme cette page, 0 si elle reste ouverte.
func closeCode(t *testing.T, page *websocket.Conn) int {
	t.Helper()
	page.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, _, err := page.ReadMessage()
		if err == nil {
			continue
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return 0
		}
		var closed *websocket.CloseError
		if errors.As(err, &closed) {
			return closed.Code
		}

		return -1
	}
}

// DES SESSIONS COUPÉES (chantier 55) : deux canaux du même compte, l'ancien (sa
// génération d'avant) se ferme en 1008, le neuf reste ; un administrateur
// rétrogradé est fermé sans 1008, pour revenir avec son rang.
func TestCutSessionsCloseTheOlderChannelsIn1008(t *testing.T) {
	server, httpServer := facadeTestServer(t)
	store := accountsTestStore(t, server,
		accounts.Account{ID: 2, Username: "marc", RoleLevel: accounts.RoleUser, Consoles: []string{"*"}},
		accounts.Account{ID: 1, Username: "admin", RoleLevel: accounts.RoleAdmin, Consoles: []string{"*"}})
	dial := func(claims auth.Claims) *websocket.Conn {
		t.Helper()
		page, _, err := facadeTestDial(t, httpServer, testToken(t, claims))
		if err != nil {
			t.Fatalf("%s (génération %d) : %v", claims.Username, claims.Gen, err)
		}
		t.Cleanup(func() { page.Close() })

		return page
	}
	older := dial(auth.Claims{Username: "marc", Gen: 0, RegisteredClaims: jwt.RegisteredClaims{Subject: "2"}})
	newer := dial(auth.Claims{Username: "marc", Gen: 1, RegisteredClaims: jwt.RegisteredClaims{Subject: "2"}})
	administrator := dial(auth.Claims{Username: "admin", RoleLevel: 1, RegisteredClaims: jwt.RegisteredClaims{Subject: "1"}})
	time.Sleep(100 * time.Millisecond)

	store.Replace([]accounts.Account{
		{ID: 2, Username: "marc", RoleLevel: accounts.RoleUser, Consoles: []string{"*"}, SessionGeneration: 1},
		{ID: 1, Username: "admin", RoleLevel: accounts.RoleUser, Consoles: []string{"*"}},
	})
	if code := closeCode(t, older); code != websocket.ClosePolicyViolation {
		t.Fatalf("le canal d'avant : %d, attendu 1008", code)
	}
	if code := closeCode(t, newer); code != 0 {
		t.Fatalf("le canal du jeton neuf s'est fermé : %d", code)
	}
	if code := closeCode(t, administrator); code != websocket.CloseNormalClosure {
		t.Fatalf("le rétrogradé : %d, attendu une fermeture ordinaire (1000)", code)
	}
	if _, response, err := facadeTestDial(t, httpServer, testToken(t, auth.Claims{Username: "marc", Gen: 0, RegisteredClaims: jwt.RegisteredClaims{Subject: "2"}})); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("un jeton d'avant rouvre un canal : %v", err)
	}
}

// UN COMPTE SUPPRIMÉ PERD SES SAUVEGARDES, ET LUI SEUL (le porteur, 2026-11-14 :
// « Supprimer ») : son dossier saves/<compte>/ part en entier quand le service
// d'identité le nomme supprimé ; celui d'un compte qui manque sans être nommé
// (une base vide ou remplacée) reste, comme celui des autres comptes et les
// dossiers communs (`.bios-companions`).
func TestADeletedAccountLosesItsSavesAndOnlyIts(t *testing.T) {
	root := t.TempDir()
	server := &Server{storageRoot: root, pool: registry.NewPool(), mirrors: registry.NewMirrors()}
	saves := func(parts ...string) string { return filepath.Join(append([]string{root, "saves"}, parts...)...) }
	for _, file := range [][]string{
		{"lea", "sram", "nes", "jeu.srm"},
		{"lea", "streamex", "switch", "nand", "save.bin"},
		{"lea", "sources", "local", "states", "nes", "jeu_slot1.state"},
		{"marc", "sram", "nes", "jeu.srm"},
		{"gino", "states", "snes", "jeu_slot0.state"},
		{".bios-companions", "ps1", "scph.bin"},
	} {
		path := saves(file...)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := accountsTestStore(t, server,
		accounts.Account{ID: 1, Username: "gino", RoleLevel: accounts.RoleAdmin},
		accounts.Account{ID: 2, Username: "marc", RoleLevel: accounts.RoleUser},
		accounts.Account{ID: 3, Username: "lea", RoleLevel: accounts.RoleUser})

	// lea supprimée et nommée ; marc absent, sans être nommé.
	store.Replace([]accounts.Account{{ID: 1, Username: "gino", RoleLevel: accounts.RoleAdmin}}, "lea")
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(saves("lea")); errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("les sauvegardes de lea sont restées")
		}
	}
	time.Sleep(100 * time.Millisecond) // un effacement lancé à tort aurait fini
	for _, kept := range [][]string{{"marc", "sram", "nes", "jeu.srm"}, {"gino", "states", "snes", "jeu_slot0.state"}, {".bios-companions", "ps1", "scph.bin"}} {
		if _, err := os.Stat(saves(kept...)); err != nil {
			t.Fatalf("%s touché : %v", filepath.Join(kept...), err)
		}
	}
}
