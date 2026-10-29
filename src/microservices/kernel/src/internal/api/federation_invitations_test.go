package api

// LES INVITATIONS DU CHANTIER 33, des deux côtés : chez celui qui invite, la
// route publique relaie l'échange au service d'identité ; chez l'invité, le
// lien collé devient le jeton du lien, ou celui d'un compte.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
)

// lienInvitation — l'invitation de fédération que le serveur qui invite connaît.
const lienInvitation = "invitation-du-lien-001"

// invitingServer — le serveur qui invite : sa surface fédérée, et un service
// d'identité qui sert UNE fois l'invitation du lien (portées catalogue et
// consoles), puis accepte le jeton né de l'échange.
func invitingServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var (
		mu        sync.Mutex
		served    bool
		addresses []string
	)
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != federationTestInternal {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		mu.Lock()
		defer mu.Unlock()
		var body struct{ Code, Kind, Address, Token string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/internal/federation/redeem":
			addresses = append(addresses, body.Address)
			switch {
			case body.Code == "trop-d-essais":
				w.Header().Set("Retry-After", "61")
				w.WriteHeader(http.StatusTooManyRequests)
			case body.Code != lienInvitation || served:
				w.WriteHeader(http.StatusUnauthorized)
			case body.Kind != "federation":
				w.WriteHeader(http.StatusConflict)
			default:
				served = true
				_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{
					"kind": "federation", "token": "jeton-ne-de-l-echange", "identity": "Marc", "scopes": "catalog,devices",
				}})
			}
		case "/internal/federation/verify":
			if body.Token != "jeton-ne-de-l-echange" {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"label": "Marc", "scopes": "catalog,devices"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(identity.Close)

	server := &Server{config: config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal}}
	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", server.federationRouter()))
	t.Cleanup(surface.Close)

	return surface, &addresses
}

func redeemAt(t *testing.T, surface *httptest.Server, body, forwarded string) (*http.Response, map[string]any) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, surface.URL+"/api/v1/federation/redeem", bytes.NewReader([]byte(body)))
	if forwarded != "" {
		request.Header.Set("X-Forwarded-For", forwarded)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)

	return response, decoded
}

// LA ROUTE PUBLIQUE RELAIE, SANS JETON : le code en tient lieu. Le jeton et
// ses portées reviennent avec notre nom ; l'adresse annoncée part au service
// d'identité (le frein, le journal) ; ses refus passent tels quels.
func TestLaRoutePubliqueRelaieLEchange(t *testing.T) {
	surface, addresses := invitingServer(t)

	response, body := redeemAt(t, surface, `{"code":"`+lienInvitation+`","kind":"account"}`, "")
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("l'autre sorte = %d %v, attendu 409", response.StatusCode, body)
	}
	response, body = redeemAt(t, surface, `{"code":"`+lienInvitation+`","kind":"federation"}`, "203.0.113.7, 10.0.0.1")
	data, _ := body["data"].(map[string]any)
	if response.StatusCode != http.StatusOK || data["token"] != "jeton-ne-de-l-echange" || data["identity"] != "Marc" ||
		len(data["scopes"].([]any)) != 2 || data["hostname"] == "" {
		t.Fatalf("l'échange = %d %v", response.StatusCode, body)
	}
	if got := (*addresses)[1]; got != "203.0.113.7" {
		t.Fatalf("adresse relayée %q, attendu celle du proxy", got)
	}
	if response, _ := redeemAt(t, surface, `{"code":"`+lienInvitation+`","kind":"federation"}`, ""); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("le second échange = %d, attendu 401", response.StatusCode)
	}
	response, _ = redeemAt(t, surface, `{"code":"trop-d-essais","kind":"federation"}`, "")
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") != "61" {
		t.Fatalf("freiné = %d, Retry-After %q", response.StatusCode, response.Header.Get("Retry-After"))
	}
	if response, _ := redeemAt(t, surface, `{}`, ""); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("sans code = %d", response.StatusCode)
	}
}

// guestServer — le serveur de l'invité : `registered` est sa liste.
func guestServer(t *testing.T, registered []registeredServer) func(body string) (int, map[string]any) {
	t.Helper()
	server := &Server{config: config.Config{JWTSecret: facadeTestSecret}}
	server.serverRegistry.items = registered
	server.serverRegistry.fetchedAt = time.Now()
	handler := auth.Middleware(facadeTestSecret, server.handleRedeemServerInvitation)
	caller := facadeTestToken(t, "admin")

	return func(body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
		request.Header.Set("Authorization", "Bearer "+caller)
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		var decoded map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)

		return recorder.Code, decoded
	}
}

// LE LIEN COLLÉ DEVIENT LE JETON DU LIEN, UNE FOIS : sondé chez lui avec le
// jeton qui vient de naître ; un serveur déjà dans la liste est refusé AVANT
// l'échange — l'invitation n'est pas perdue ; un texte qui n'est pas une
// invitation non plus.
func TestLeLienColleDevientLeJetonDuLien(t *testing.T) {
	surface, _ := invitingServer(t)
	link := surface.URL + "/invite/" + lienInvitation

	if status, body := guestServer(t, nil)(`{"invitation":"` + surface.URL + `/invite/court"}`); status != http.StatusBadRequest ||
		body["errorCode"] != "notInvitation" {
		t.Fatalf("pas une invitation = %d %v", status, body)
	}
	already := guestServer(t, []registeredServer{{ID: 1, Link: surface.URL + "/"}})
	if status, body := already(`{"invitation":"` + link + `"}`); status != http.StatusConflict || body["errorCode"] != "alreadyRegistered" {
		t.Fatalf("déjà dans la liste = %d %v", status, body)
	}

	redeem := guestServer(t, nil)
	status, body := redeem(`{"invitation":"  ` + link + `  "}`)
	data, _ := body["data"].(map[string]any)
	if status != http.StatusOK || data["token"] != "jeton-ne-de-l-echange" || data["link"] != surface.URL ||
		data["identity"] != "Marc" || len(data["scopes"].([]any)) != 2 {
		t.Fatalf("l'échange, l'invitation intacte = %d %v", status, body)
	}
	if status, body := redeem(`{"invitation":"` + link + `"}`); status != http.StatusBadRequest || body["errorCode"] != "invitationRefused" {
		t.Fatalf("la même invitation, encore = %d %v", status, body)
	}
}

// L'INVITATION D'UN COMPTE DEVIENT SON JETON, gardé comme un jeton collé : la
// première partie suit sans nouvel échange de session ; une seconde fois, elle
// est refusée ; d'une autre sorte, rien n'est gardé.
func TestLInvitationDUnCompteDevientSonJeton(t *testing.T) {
	lender := newFakeLender(t)
	identity, ask := borrowing(t, lender)
	marc := facadeTestToken(t, "marc")
	invitation := lender.URL + "/invite/" + marcInvitation

	if status, body := ask(http.MethodPut, "/servers/3/account", marc, `{"invitation":"pas un lien"}`); status != http.StatusBadRequest ||
		body["errorCode"] != "notInvitation" {
		t.Fatalf("pas une invitation = %d %v", status, body)
	}
	status, body := ask(http.MethodPut, "/servers/3/account", marc, `{"invitation":"`+invitation+`"}`)
	if session, _ := body["data"].(map[string]any); status != http.StatusOK || session["username"] != "marc" ||
		identity.member("marc|3") != friendsToken || lender.count() != 1 {
		t.Fatalf("l'invitation de son compte = %d %v (gardé %q)", status, body, identity.member("marc|3"))
	}
	if status, body := ask(http.MethodPut, "/servers/3/account", marc, `{"invitation":"`+invitation+`"}`); status != http.StatusBadRequest ||
		body["errorCode"] != "invitationRefused" {
		t.Fatalf("la même invitation, encore = %d %v", status, body)
	}
}

// LE QR D'UNE INVITATION, DESSINÉ PUIS RELU PAR LES DEUX ROUTES : un PNG qui
// rend son lien ; autre chose qu'une invitation n'est pas dessiné ; une image
// sans QR se dit `noQR`.
func TestLeQRDUneInvitationSeDessineEtSeRelit(t *testing.T) {
	server := &Server{config: config.Config{JWTSecret: facadeTestSecret}}
	caller := facadeTestToken(t, "marc")
	call := func(handler http.HandlerFunc, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+caller)
		recorder := httptest.NewRecorder()
		auth.Middleware(facadeTestSecret, handler)(recorder, request)

		return recorder
	}
	link := "https://gameflex.example.com/invite/" + lienInvitation

	drawn := call(server.handleInvitationQR, []byte(`{"text":"`+link+`","size":300}`))
	if drawn.Code != http.StatusOK || drawn.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("dessin = %d %s", drawn.Code, drawn.Header().Get("Content-Type"))
	}
	read := call(server.handleReadInvitationQR, drawn.Body.Bytes())
	var decoded map[string]map[string]string
	_ = json.Unmarshal(read.Body.Bytes(), &decoded)
	if read.Code != http.StatusOK || decoded["data"]["text"] != link {
		t.Fatalf("relu = %d %s", read.Code, read.Body.String())
	}

	if refused := call(server.handleInvitationQR, []byte(`{"text":"https://example.com/autre-chose"}`)); refused.Code != http.StatusBadRequest {
		t.Fatalf("autre chose qu'une invitation = %d", refused.Code)
	}
	if blank := call(server.handleReadInvitationQR, []byte("pas une image")); !bytes.Contains(blank.Body.Bytes(), []byte(`"noQR"`)) {
		t.Fatalf("sans QR = %d %s", blank.Code, blank.Body.String())
	}
}
