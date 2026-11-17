package api

// LE CODE D'APPAIRAGE CÔTÉ KERNEL (chantier 60) : le sujet `pairing`, et les
// six chiffres sur les routes de celui qui rejoint.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
)

// pairingFrameForTest — un mode serveur ouvert par claude, le mode compte de
// marc, un code de chaque sorte.
const pairingFrameForTest = `{"type":"pairing",
	"server":{"open_until":"2026-11-17T14:05:00Z","opened_by":"claude"},
	"accounts":[{"username":"marc","open_until":"2026-11-17T14:04:00Z"}],
	"codes":[{"id":1,"kind":"federation","author":"claude","expires_at":"2026-11-17T14:02:00Z"},
	         {"id":2,"kind":"account","author":"marc","expires_at":"2026-11-17T14:02:00Z"}],
	"ended":[],"closed":[]}`

// LE SUJET `pairing` : l'administrateur voit le mode serveur et les codes
// federation ; un membre, son mode compte et ses codes. Une trame nouvelle
// repart sans rien redemander.
func TestThePairingTopicIsTheShareOfItsViewer(t *testing.T) {
	server, httpServer := facadeTestServer(t)
	server.followPairing([]byte(pairingFrameForTest), true)

	read := func(token string) (*facadeTestPush, func() facadeTestPush) {
		connection, _, err := facadeTestDial(t, httpServer, token)
		if err != nil {
			t.Fatalf("poignée de main : %v", err)
		}
		t.Cleanup(func() { connection.Close() })
		facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "pairing"}})
		next := func() facadeTestPush {
			return facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Topic == "pairing" })
		}
		first := next()

		return &first, next
	}
	type view struct {
		Server  *map[string]string `json:"server"`
		Account *map[string]string `json:"account"`
		Codes   []map[string]any   `json:"codes"`
		Ended   []map[string]any   `json:"ended"`
	}
	decode := func(push facadeTestPush) view {
		var decoded view
		if err := json.Unmarshal(push.Data, &decoded); err != nil {
			t.Fatal(err)
		}

		return decoded
	}

	admin, _ := read(testToken(t, auth.Claims{Username: "claude", RoleLevel: auth.RoleAdmin}))
	if seen := decode(*admin); seen.Server == nil || seen.Account != nil || len(seen.Codes) != 1 || seen.Codes[0]["kind"] != "federation" {
		t.Fatalf("la part de l'administrateur : %s", admin.Data)
	}
	member, next := read(testToken(t, auth.Claims{Username: "marc", RoleLevel: 2}))
	if seen := decode(*member); seen.Server != nil || seen.Account == nil || len(seen.Codes) != 1 || seen.Codes[0]["kind"] != "account" {
		t.Fatalf("la part d'un membre : %s", member.Data)
	}

	server.followPairing([]byte(`{"type":"pairing","server":null,"accounts":[],"codes":[],
		"ended":[{"id":2,"kind":"account","author":"marc","reason":"used","by":"Gamebox","from":"192.168.1.20","at":"x"}],"closed":[]}`), false)
	if seen := decode(next()); seen.Account != nil || len(seen.Ended) != 1 || seen.Ended[0]["by"] != "Gamebox" {
		t.Fatalf("la trame suivante : %+v", seen)
	}
}

// pairingInviter — le serveur qui invite : sa surface fédérée, et un service
// d'identité qui sert UNE fois le code 482913 (sorte federation), répond 423
// pour 000000, et note le nom de celui qui échange.
func pairingInviter(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu     sync.Mutex
		served bool
		froms  []string
	)
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var body struct{ Code, Kind, From, Token string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/internal/federation/redeem":
			froms = append(froms, body.From)
			switch {
			case body.Code == "000000":
				w.WriteHeader(http.StatusLocked)
			case body.Code != "482913" || served || body.Kind != "federation":
				w.WriteHeader(http.StatusUnauthorized)
			default:
				served = true
				_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{
					"kind": "federation", "token": "jeton-ne-du-code", "identity": "Gamebox", "scopes": "catalog",
				}})
			}
		case "/internal/federation/verify":
			_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"label": "Gamebox", "scopes": "catalog"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(identity.Close)

	server := &Server{config: config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal}}
	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", server.federationRouter()))
	t.Cleanup(surface.Close)

	return surface, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return append([]string{}, froms...)
	}
}

// L'ADRESSE ET LES SIX CHIFFRES DEVIENNENT LE JETON DU LIEN : notre nom part
// avec le code ; un serveur hors du mode découverte se dit `pairingClosed` ; un
// serveur déjà dans la liste est refusé AVANT l'échange ; un code mal formé,
// sans rien demander là-bas.
func TestAPairingCodeJoinsAServer(t *testing.T) {
	t.Setenv("GAMEFLEX_SERVER_NAME", "Salon")
	surface, froms := pairingInviter(t)
	typed := strings.TrimPrefix(surface.URL, "http://")
	if response, body := redeemAt(t, surface, `{"code":"000000","kind":"federation","from":"Salon"}`, ""); response.StatusCode != http.StatusLocked ||
		body["errorCode"] != "pairingClosed" {
		t.Fatalf("la route publique, hors du mode = %d %v", response.StatusCode, body)
	}

	already := guestServer(t, []registeredServer{{ID: 1, Link: surface.URL}})
	if status, body := already(`{"link":"` + typed + `","code":"482913"}`); status != http.StatusConflict || body["errorCode"] != "alreadyRegistered" {
		t.Fatalf("déjà dans la liste = %d %v", status, body)
	}
	if status, body := guestServer(t, nil)(`{"link":"` + typed + `","code":"48291"}`); status != http.StatusBadRequest || body["errorCode"] != "notPairingCode" {
		t.Fatalf("cinq chiffres = %d %v", status, body)
	}
	if len(froms()) != 1 {
		t.Fatalf("un échange est parti avant les refus : %v", froms())
	}

	redeem := guestServer(t, nil)
	if status, body := redeem(`{"link":"` + typed + `","code":"000000"}`); status != http.StatusBadRequest || body["errorCode"] != "pairingClosed" {
		t.Fatalf("hors du mode découverte = %d %v", status, body)
	}
	status, body := redeem(`{"link":"` + typed + `/","code":"482 913"}`)
	data, _ := body["data"].(map[string]any)
	if status != http.StatusOK || data["token"] != "jeton-ne-du-code" || data["link"] != surface.URL {
		t.Fatalf("l'échange = %d %v", status, body)
	}
	if got := froms(); len(got) != 3 || got[2] != "Salon" {
		t.Fatalf("notre nom n'est pas parti : %v", got)
	}
	if status, body := redeem(`{"link":"` + typed + `","code":"482913"}`); status != http.StatusBadRequest || body["errorCode"] != "invitationRefused" {
		t.Fatalf("le même code, encore = %d %v", status, body)
	}
}
