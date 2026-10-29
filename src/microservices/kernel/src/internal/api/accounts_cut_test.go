package api

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gameflex/kernel/internal/accounts"
	"gameflex/kernel/internal/auth"
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
			answer = server.consoleGranted(r, consoleID)
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
	if !granted("marc", "keytest") {
		t.Fatal("la page de test des manettes n'est pas une console")
	}
}

// LE REFUS SE DIT AVEC UN CODE que la page traduit.
func TestTheRefusalCarriesACode(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeNotGranted(recorder, "ps3")
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"errorCode":"consoleNotGranted"`) {
		t.Fatalf("refus : %d %s", recorder.Code, recorder.Body.String())
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
