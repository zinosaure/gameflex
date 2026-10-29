package api

// LA FÉDÉRATION DU CHANTIER 20, des deux côtés (le porteur, 2026-10-21 : « le
// lien entre 2 serveurs reste comme avant […] pour y jouer, au lieu d'avoir un
// formulaire de login, je passe mon token user ») :
//
//   - le LIEN est un jeton de catalogue : il fait lister et télécharger, selon
//     ses portées, et n'ouvre aucune session ;
//   - JOUER passe par le jeton d'un COMPTE de chez l'ami, qu'un membre d'ici
//     colle une fois : son serveur l'échange contre une session à son nom.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"gameflex/kernel/internal/accounts"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
)

const federationTestInternal = "secret-interne"

// identityAccount — un compte de chez nous, tel que le service d'identité le
// rend avec la session qu'il ouvre.
type identityAccount struct {
	Username, DisplayName string
	Consoles              []string
}

// fakeIdentityService — le service d'identité, réduit à ce que le kernel lui
// demande : ce qu'ouvre un jeton de catalogue, une session pour le jeton d'un
// compte, et les jetons que nos membres ont collés pour leurs amis. `down` le
// fait répondre 500, comme en panne.
type fakeIdentityService struct {
	*httptest.Server

	mu       sync.Mutex
	catalog  map[string]string
	accounts map[string]identityAccount
	members  map[string]string
	down     bool
	sessions int
}

func newFakeIdentity(t *testing.T, catalog map[string]string, known map[string]identityAccount) *fakeIdentityService {
	t.Helper()
	identity := &fakeIdentityService{catalog: catalog, accounts: known, members: map[string]string{}}
	identity.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != federationTestInternal {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		identity.mu.Lock()
		defer identity.mu.Unlock()
		if identity.down {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
		if r.URL.Path == "/internal/federation/accounts" {
			identity.serveMembers(w, r)

			return
		}
		var body struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/internal/federation/verify":
			scopes, found := identity.catalog[body.Token]
			if !found {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"label": "Téléphone de Marc", "scopes": scopes}})
		case "/internal/federation/session":
			account, found := identity.accounts[body.Token]
			if !found {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
			identity.sessions++
			signed := testToken(t, auth.Claims{
				Username: account.Username, RoleLevel: accounts.RoleUser, Fed: body.Token[:8],
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour))},
			})
			_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"access_token": signed, "user": map[string]any{
				"username": account.Username, "display_name": account.DisplayName, "consoles": account.Consoles,
			}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(identity.Close)

	return identity
}

// serveMembers — `/internal/federation/accounts` : les jetons de nos membres
// pour leurs amis, par « membre|serveur ».
func (identity *fakeIdentityService) serveMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Username string `json:"username"`
			Server   int64  `json:"server"`
			Token    string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		identity.members[body.Username+"|"+jsonNumber(body.Server)] = body.Token
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet, http.MethodDelete:
		key := r.URL.Query().Get("username") + "|" + r.URL.Query().Get("server")
		token, found := identity.members[key]
		if !found {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		if r.Method == http.MethodDelete {
			delete(identity.members, key)
			w.WriteHeader(http.StatusNoContent)

			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"token": token}})
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)

	return string(encoded)
}

func (identity *fakeIdentityService) sessionCount() int {
	identity.mu.Lock()
	defer identity.mu.Unlock()

	return identity.sessions
}

func (identity *fakeIdentityService) member(key string) string {
	identity.mu.Lock()
	defer identity.mu.Unlock()

	return identity.members[key]
}

func (identity *fakeIdentityService) memberCount() int {
	identity.mu.Lock()
	defer identity.mu.Unlock()

	return len(identity.members)
}

const (
	catalogOnly  = "catalogue-seul"
	catalogPlus  = "catalogue-et-consoles"
	marcAccount  = "marc0001-jeton-de-compte"
	leaAccount   = "lea00002-jeton-de-compte"
	friendsToken = "jeton-de-marc-chez-l-ami"
	// marcInvitation — l'invitation du compte de Marc chez l'ami : 22 signes,
	// comme 16 octets en base64.
	marcInvitation = "invitation-de-marc-001"
)

// federationSurface — CE serveur, vu d'un ami : sa surface fédérée, son service
// d'identité (deux jetons de catalogue ; Marc joue à la PS3, Léa n'a aucune
// console), et deux consoles à distance, une PS4 et une PS5.
func federationSurface(t *testing.T) (*httptest.Server, *fakeIdentityService) {
	t.Helper()
	server, _ := listingServer(t, "roms/nes/Metroid.nes")
	identity := newFakeIdentity(t,
		map[string]string{catalogOnly: "catalog", catalogPlus: "catalog,devices"},
		map[string]identityAccount{
			marcAccount: {Username: "marc", DisplayName: "Marc", Consoles: []string{"ps3"}},
			leaAccount:  {Username: "lea", DisplayName: "Léa", Consoles: []string{}},
		})
	remoteplay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "1", "hostName": "PS4 du salon", "hostType": "PS4", "rp_key": "secret"},
			{"id": "2", "hostName": "PS5 du salon", "hostType": "PS5", "rp_key": "secret"},
		}})
	}))
	t.Cleanup(remoteplay.Close)
	server.config = config.Config{
		CredentialURL: identity.URL, InternalSecret: federationTestInternal, JWTSecret: facadeTestSecret,
		RemoteplayControlURL: remoteplay.URL,
	}
	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", server.federationRouter()))
	t.Cleanup(surface.Close)

	return surface, identity
}

func federationCall(t *testing.T, surface *httptest.Server, method, path, token string) (int, map[string]any) {
	t.Helper()
	request, _ := http.NewRequest(method, surface.URL+"/api/v1/federation"+path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)

	return response.StatusCode, decoded
}

// LE LIEN FAIT LISTER, UN COMPTE FAIT JOUER : le jeton de catalogue ouvre le
// catalogue et, avec la portée « devices », la liste des consoles à distance
// (sans clé) ; il n'ouvre aucune session. Le jeton d'un compte ouvre une
// session à son nom — même sans console : la page dira qu'il n'y lance rien —
// et n'ouvre pas le catalogue. Un jeton inconnu, ou absent, n'ouvre rien.
func TestLeLienFaitListerEtUnCompteFaitJouer(t *testing.T) {
	surface, identity := federationSurface(t)

	status, body := federationCall(t, surface, http.MethodGet, "/identity", catalogOnly)
	data, _ := body["data"].(map[string]any)
	if status != http.StatusOK || data["identity"] != "Téléphone de Marc" || len(data["scopes"].([]any)) != 1 {
		t.Fatalf("identité du lien = %d %v", status, body)
	}
	if status, _ := federationCall(t, surface, http.MethodGet, "/listing/updated", catalogOnly); status != http.StatusOK {
		t.Fatalf("le catalogue, avec le jeton du lien = %d, attendu 200", status)
	}
	if status, _ := federationCall(t, surface, http.MethodGet, "/remoteplay/devices", catalogOnly); status != http.StatusForbidden {
		t.Fatalf("les consoles à distance sans la portée = %d, attendu 403", status)
	}
	_, body = federationCall(t, surface, http.MethodGet, "/remoteplay/devices", catalogPlus)
	lent, _ := body["data"].([]any)
	if len(lent) != 2 || lent[0].(map[string]any)["rp_key"] != nil {
		t.Fatalf("les consoles à distance, avec la portée = %v, attendu les deux, sans clé", body)
	}
	if status, _ := federationCall(t, surface, http.MethodPost, "/session", catalogPlus); status != http.StatusUnauthorized || identity.sessionCount() != 0 {
		t.Fatalf("une session pour le jeton du lien = %d, attendu 401", status)
	}

	if status, _ := federationCall(t, surface, http.MethodGet, "/listing/updated", marcAccount); status != http.StatusUnauthorized {
		t.Fatalf("le catalogue, avec le jeton d'un compte = %d, attendu 401", status)
	}
	status, body = federationCall(t, surface, http.MethodPost, "/session", marcAccount)
	session, _ := body["data"].(map[string]any)
	if status != http.StatusOK || session["username"] != "marc" || identity.sessionCount() != 1 {
		t.Fatalf("la session de Marc = %d %v", status, body)
	}
	claims, err := auth.ParseToken(facadeTestSecret, session["token"].(string))
	if err != nil || claims.Fed != marcAccount[:8] || claims.RoleLevel != accounts.RoleUser {
		t.Fatalf("la session rendue = %+v (%v)", claims, err)
	}
	status, body = federationCall(t, surface, http.MethodPost, "/session", leaAccount)
	if session, _ := body["data"].(map[string]any); status != http.StatusOK || session["username"] != "lea" ||
		len(session["consoles"].([]any)) != 0 {
		t.Fatalf("la session de Léa, sans console = %d %v", status, body)
	}

	for _, token := range []string{"inconnu-de-ce-serveur", ""} {
		if status, _ := federationCall(t, surface, http.MethodGet, "/listing/updated", token); status != http.StatusUnauthorized {
			t.Errorf("jeton %q = %d, attendu 401", token, status)
		}
		if status, _ := federationCall(t, surface, http.MethodPost, "/session", token); status != http.StatusUnauthorized {
			t.Errorf("session pour %q = %d, attendu 401", token, status)
		}
	}
}

// UNE PANNE DU SERVICE D'IDENTITÉ N'EST PAS UN REFUS : 503, et pas 401 — l'ami
// lisait sinon un refus de son jeton, et effaçait sa copie de notre catalogue.
func TestUnePanneDuServiceDIdentiteNEstPasUnRefus(t *testing.T) {
	surface, identity := federationSurface(t)
	identity.mu.Lock()
	identity.down = true
	identity.mu.Unlock()

	if status, _ := federationCall(t, surface, http.MethodGet, "/listing/updated", catalogOnly); status != http.StatusServiceUnavailable {
		t.Fatalf("service d'identité en panne = %d, attendu 503", status)
	}
	if status, _ := federationCall(t, surface, http.MethodPost, "/session", marcAccount); status != http.StatusServiceUnavailable {
		t.Fatalf("une session, service d'identité en panne = %d, attendu 503", status)
	}
}

// fakeLender — l'ami chez qui on joue : il échange le jeton du compte de Marc
// chez lui contre une session, et juge ensuite les sessions qu'il a données
// (`/api/v1/auth/me`). Le jeton du lien, lui, n'en ouvre pas.
type fakeLender struct {
	*httptest.Server

	mu        sync.Mutex
	exchanges int
	alive     string
	refusing  bool
	// invited — l'invitation du compte de Marc (chantier 33), servie une fois.
	invited bool
	// consoles — ce que le compte de Marc lance chez lui, À L'INSTANT : ce que
	// `/api/v1/auth/me` en dit.
	consoles []string
}

func newFakeLender(t *testing.T) *fakeLender {
	t.Helper()
	lender := &fakeLender{consoles: []string{"ps3"}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/federation/redeem", func(w http.ResponseWriter, r *http.Request) {
		lender.mu.Lock()
		defer lender.mu.Unlock()
		var body struct{ Code, Kind string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Code != marcInvitation || lender.invited {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		if body.Kind != "account" {
			w.WriteHeader(http.StatusConflict)

			return
		}
		lender.invited = true
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"kind": "account", "token": friendsToken, "identity": "marc", "hostname": "ami",
		}})
	})
	mux.HandleFunc("POST /api/v1/federation/session", func(w http.ResponseWriter, r *http.Request) {
		lender.mu.Lock()
		defer lender.mu.Unlock()
		if lender.refusing || r.Header.Get("Authorization") != "Bearer "+friendsToken {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		lender.exchanges++
		lender.alive = "session-" + strings.Repeat("x", lender.exchanges)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"token": lender.alive, "username": "marc", "consoles": []string{"ps3"}, "expiresAt": time.Now().Add(7 * 24 * time.Hour),
		}})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		lender.mu.Lock()
		defer lender.mu.Unlock()
		if lender.alive == "" || r.Header.Get("Authorization") != "Bearer "+lender.alive {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"item": map[string]any{"username": "marc", "consoles": lender.consoles}})
	})
	lender.Server = httptest.NewServer(mux)
	t.Cleanup(lender.Close)

	return lender
}

func (lender *fakeLender) count() int {
	lender.mu.Lock()
	defer lender.mu.Unlock()

	return lender.exchanges
}

// borrowing — NOTRE serveur, qui a l'ami au registre (n° 3, avec le jeton du
// lien) et un service d'identité qui garde les jetons de nos membres.
func borrowing(t *testing.T, lender *fakeLender) (*fakeIdentityService, func(method, path, caller, body string) (int, map[string]any)) {
	t.Helper()
	identity := newFakeIdentity(t, nil, nil)
	server := &Server{config: config.Config{
		JWTSecret: facadeTestSecret, CredentialURL: identity.URL, InternalSecret: federationTestInternal,
	}}
	server.serverRegistry.items = []registeredServer{{ID: 3, Link: lender.URL, Token: "jeton-du-lien", Hostname: "ami"}}
	server.serverRegistry.fetchedAt = time.Now()
	mux := http.NewServeMux()
	// Les mêmes gardes qu'api.go : localOnly ferme ces routes à une session
	// ouverte depuis un autre serveur.
	mux.HandleFunc("GET /servers/{id}/session", auth.Middleware(facadeTestSecret, localOnly(server.handleServerSession)))
	mux.HandleFunc("PUT /servers/{id}/account", auth.Middleware(facadeTestSecret, localOnly(server.handleSetServerAccount)))
	mux.HandleFunc("DELETE /servers/{id}/account", auth.Middleware(facadeTestSecret, localOnly(server.handleForgetServerAccount)))

	return identity, func(method, path, caller, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		request.Header.Set("Authorization", "Bearer "+caller)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		var decoded map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)

		return response.Code, decoded
	}
}

// UN MEMBRE COLLE SON JETON, UNE FOIS, ET JOUE CHEZ L'AMI SOUS SON COMPTE : sans
// jeton, rien que le catalogue (`noFederatedAccount`) ; un jeton que l'ami
// refuse ne se garde pas ; le bon se garde, et sa première session sert sans
// nouvel échange ; il n'est qu'à lui ; l'ami le désactive, la page le lit ; il
// l'oublie, retour au catalogue. Une session ouverte par jeton, venue d'ailleurs,
// ne confie rien ici.
func TestUnMembreColleSonJetonEtJoueChezLAmiSousSonCompte(t *testing.T) {
	lender := newFakeLender(t)
	identity, ask := borrowing(t, lender)
	marc, lea := facadeTestToken(t, "marc"), facadeTestToken(t, "lea")

	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusNotFound || body["errorCode"] != "noFederatedAccount" {
		t.Fatalf("avant d'avoir collé un jeton = %d %v", status, body)
	}
	if status, body := ask(http.MethodPut, "/servers/3/account", marc, `{"token":"un-jeton-faux"}`); status != http.StatusForbidden ||
		body["errorCode"] != "accountRefused" || identity.memberCount() != 0 {
		t.Fatalf("un jeton que l'ami refuse = %d %v (%d gardé)", status, body, identity.memberCount())
	}

	status, body := ask(http.MethodPut, "/servers/3/account", marc, `{"token":"  `+friendsToken+`  "}`)
	if session, _ := body["data"].(map[string]any); status != http.StatusOK || session["username"] != "marc" ||
		identity.member("marc|3") != friendsToken || lender.count() != 1 {
		t.Fatalf("le bon jeton = %d %v (gardé %q, %d échange)", status, body, identity.member("marc|3"), lender.count())
	}
	status, body = ask(http.MethodGet, "/servers/3/session", marc, "")
	if session, _ := body["data"].(map[string]any); status != http.StatusOK || session["token"] != "session-x" || lender.count() != 1 {
		t.Fatalf("la première partie = %d %v (%d échange(s))", status, body, lender.count())
	}
	if status, body := ask(http.MethodGet, "/servers/3/session", lea, ""); status != http.StatusNotFound || body["errorCode"] != "noFederatedAccount" {
		t.Fatalf("Léa, sans jeton à elle = %d %v", status, body)
	}

	lender.mu.Lock()
	lender.alive, lender.refusing = "", true
	lender.mu.Unlock()
	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusForbidden || body["errorCode"] != "accountRefused" {
		t.Fatalf("son jeton désactivé chez l'ami = %d %v, attendu 403 accountRefused", status, body)
	}

	if status, _ := ask(http.MethodDelete, "/servers/3/account", marc, ""); status != http.StatusNoContent {
		t.Fatalf("oublier son jeton = %d", status)
	}
	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusNotFound || body["errorCode"] != "noFederatedAccount" {
		t.Fatalf("après l'oubli = %d %v", status, body)
	}

	visitor := testToken(t, auth.Claims{Username: "marc", Fed: "abcd1234"})
	lender.mu.Lock()
	lender.refusing = false
	lender.mu.Unlock()
	if status, _ := ask(http.MethodPut, "/servers/3/account", visitor, `{"token":"`+friendsToken+`"}`); status != http.StatusForbidden || identity.memberCount() != 0 {
		t.Fatalf("une session ouverte par jeton confie un jeton = %d (%d gardé)", status, identity.memberCount())
	}
	// Ni ne demande une session chez un troisième avec les jetons rangés ici
	// (revue de sécurité, 2026-10-28) : elle joue ici, et rien d'autre.
	if status, _ := ask(http.MethodGet, "/servers/3/session", visitor, ""); status != http.StatusForbidden {
		t.Fatalf("une session ouverte par jeton demande une session chez un tiers = %d, attendu 403", status)
	}
}

// UNE SESSION CHEZ UN AMI SE GARDE TANT QU'IL L'ACCEPTE : une seule ouverture
// pour deux demandes (chacune compte « une connexion » chez lui) ; une console
// qu'il retire au compte se voit à la demande suivante, sans nouvel échange ;
// il la refuse (secret changé, jeton désactivé puis réactivé), elle se
// ré-échange ; éteint, il ne répond pas.
func TestUneSessionChezUnAmiSeGardeTantQuIlLAccepte(t *testing.T) {
	lender := newFakeLender(t)
	identity, ask := borrowing(t, lender)
	identity.mu.Lock()
	identity.members["marc|3"] = friendsToken
	identity.mu.Unlock()
	marc := facadeTestToken(t, "marc")

	first, body := ask(http.MethodGet, "/servers/3/session", marc, "")
	again, _ := ask(http.MethodGet, "/servers/3/session", marc, "")
	if first != http.StatusOK || again != http.StatusOK || lender.count() != 1 ||
		body["data"].(map[string]any)["token"] != "session-x" {
		t.Fatalf("deux demandes = %d, %d, %d échange(s) : %v", first, again, lender.count(), body)
	}

	lender.mu.Lock()
	lender.consoles = []string{}
	lender.mu.Unlock()
	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusOK ||
		len(body["data"].(map[string]any)["consoles"].([]any)) != 0 || lender.count() != 1 {
		t.Fatalf("la PS3 retirée au compte chez lui : %d %v (%d échange(s))", status, body, lender.count())
	}

	lender.mu.Lock()
	lender.alive = ""
	lender.mu.Unlock()
	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusOK || body["data"].(map[string]any)["token"] != "session-xx" {
		t.Fatalf("une session refusée chez lui ne se ré-échange pas : %d %v", status, body)
	}

	lender.Close()
	if status, body := ask(http.MethodGet, "/servers/3/session", marc, ""); status != http.StatusBadGateway || body["errorCode"] != "serverUnreachable" {
		t.Fatalf("un ami éteint = %d %v, attendu 502 serverUnreachable", status, body)
	}
}
