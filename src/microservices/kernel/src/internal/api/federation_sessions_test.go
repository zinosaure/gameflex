package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/registry"
)

// sessionsPeer — un pair qui a une partie en cours (une d'ami, vue par un spectateur, avec une manette libre) ; son
// jeton de catalogue est `catalogue`, celui qui n'ouvre que ses consoles `consoles`.
func sessionsPeer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	identity := newFakeIdentity(t, map[string]string{"catalogue": "catalog", "consoles": "devices"}, nil)
	peer := &Server{
		config:  config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal, JWTSecret: facadeTestSecret},
		pool:    registry.NewPool(),
		mirrors: registry.NewMirrors(),
	}
	peer.pool.Register(registry.Session{ID: "partie-1", Platform: "retrogaming", ConsoleID: "snes", Reference: "ref-aladdin", Title: "Aladdin", Initiator: "ami", MaxPlayers: 2, Players: []string{"ami"}})
	peer.pool.AddSpectator("partie-1", "voisin")
	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", peer.federationRouter()))
	t.Cleanup(surface.Close)

	return peer, surface
}

func peerGet(t *testing.T, surface *httptest.Server, token string) *http.Response {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, surface.URL+"/api/v1/federation/sessions", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })

	return response
}

// LES PARTIES D'UN PAIR SE DONNENT AU JETON DE CATALOGUE, ET RIEN DE PLUS (`#747`) : 401 sans jeton ou avec un faux, 403
// avec un jeton qui n'ouvre pas le catalogue ; au bon, ses parties, avec ces seuls champs — ni
// jeton, ni adresse, ni les noms de ceux qui regardent.
func TestPeerSessionsAreGivenToTheCatalogueTokenOnly(t *testing.T) {
	_, surface := sessionsPeer(t)

	for token, want := range map[string]int{"": http.StatusUnauthorized, "faux": http.StatusUnauthorized, "consoles": http.StatusForbidden} {
		if got := peerGet(t, surface, token).StatusCode; got != want {
			t.Fatalf("jeton %q : %d, attendu %d", token, got, want)
		}
	}

	response := peerGet(t, surface, "catalogue")
	var answer struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&answer) != nil {
		t.Fatalf("au jeton de catalogue : %d", response.StatusCode)
	}
	if len(answer.Data) != 1 {
		t.Fatalf("%d parties, attendu 1 : %+v", len(answer.Data), answer.Data)
	}
	fields := []string{}
	for field := range answer.Data[0] {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	if strings.Join(fields, ",") != "consoleId,id,openSlots,owner,platform,playerCount,reference,startedAt,title" {
		t.Fatalf("champs servis : %v", fields)
	}
	if string(answer.Data[0]["id"]) != `"partie-1"` || string(answer.Data[0]["owner"]) != `"ami"` || string(answer.Data[0]["openSlots"]) != "1" {
		t.Fatalf("la partie servie : %+v", answer.Data[0])
	}
}

// DE SERVEUR À SERVEUR, PAR DEUX VRAIS ROUTEURS : le sujet `servers/3/sessions` de notre canal suit le socket du pair,
// avec notre jeton de catalogue chez lui — sa partie à l'abonnement, puis la suivante dès qu'elle démarre ; un serveur
// qui n'est pas au registre est refusé.
func TestPeerSessionsReachOurPageThroughOurChannel(t *testing.T) {
	peer, surface := sessionsPeer(t)
	client, httpServer := facadeTestServer(t)
	client.serverRegistry.items = []registeredServer{{ID: 3, Link: surface.URL, Token: "catalogue", Hostname: "ami"}}
	client.serverRegistry.fetchedAt = time.Now()

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "gino"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "x", "data": map[string]any{"topic": "servers/9/sessions"}})
	if refused := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "x" }); refused.Status != http.StatusNotFound {
		t.Fatalf("un serveur absent du registre : %+v", refused)
	}

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/sessions"}})
	read := func() []map[string]any {
		push := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "sessions" && push.Topic == "servers/3/sessions" })
		var list []map[string]any
		if err := json.Unmarshal(push.Data, &list); err != nil {
			t.Fatalf("liste illisible : %s", push.Data)
		}

		return list
	}
	first := read()
	if len(first) != 1 || first[0]["id"] != "partie-1" || first[0]["title"] != "Aladdin" {
		t.Fatalf("premier instantané : %+v", first)
	}

	peer.pool.Register(registry.Session{ID: "partie-2", Platform: "streamex", ConsoleID: "switch", Title: "Mario", Initiator: "autre", MaxPlayers: 1})
	if second := read(); len(second) != 2 {
		t.Fatalf("la partie démarrée chez le pair n'est pas arrivée : %+v", second)
	}
}
