package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/registry"
	"gameflex/storage/listing"
)

const facadeTestSecret = "secret-de-test"

func facadeTestToken(t *testing.T, username string) string {
	t.Helper()

	return testToken(t, auth.Claims{Username: username})
}

// testToken — un jeton comme le service d'identité en signe : EdDSA, la clé
// tirée du secret de l'installation, une heure si les claims n'en disent rien.
func testToken(t *testing.T, claims auth.Claims) string {
	t.Helper()
	private, _, err := sharedauth.KeyPair(facadeTestSecret)
	if err != nil {
		t.Fatalf("clé : %v", err)
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(private)
	if err != nil {
		t.Fatalf("jeton : %v", err)
	}

	return signed
}

// facadeTestServer — un kernel réduit à ce que le centre de messages lit sans
// Docker : le pool, les miroirs, les battements, les lancements.
func facadeTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	server := &Server{
		config:  config.Config{JWTSecret: facadeTestSecret},
		pool:    registry.NewPool(),
		mirrors: registry.NewMirrors(),
	}
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleFacadeWebSocket))
	t.Cleanup(httpServer.Close)

	return server, httpServer
}

func facadeTestDial(t *testing.T, httpServer *httptest.Server, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	target := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/"
	if token != "" {
		target += "?token=" + token
	}

	return websocket.DefaultDialer.Dial(target, nil)
}

func facadeTestSend(t *testing.T, connection *websocket.Conn, message map[string]any) {
	t.Helper()
	if err := connection.WriteJSON(message); err != nil {
		t.Fatalf("envoi : %v", err)
	}
}

// facadeTestPush — une trame telle que la page la lit.
type facadeTestPush struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Topic    string          `json:"topic"`
	Status   int             `json:"status"`
	LaunchID string          `json:"launchId"`
	Data     json.RawMessage `json:"data"`
}

// facadeTestNext lit jusqu'à la première trame qui convient, en ignorant les
// autres — les abonnements poussent en parallèle des réponses.
func facadeTestNext(t *testing.T, connection *websocket.Conn, wanted func(facadeTestPush) bool) facadeTestPush {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_ = connection.SetReadDeadline(deadline)
		var push facadeTestPush
		if err := connection.ReadJSON(&push); err != nil {
			t.Fatalf("aucune trame attendue n'est arrivée : %v", err)
		}
		if wanted(push) {
			return push
		}
	}
}

// Le canal dit qui joue où : sans compte, pas de canal.
func TestTheMessageCentreRefusesAnAnonymousPage(t *testing.T) {
	_, httpServer := facadeTestServer(t)

	_, response, err := facadeTestDial(t, httpServer, "")
	if err == nil {
		t.Fatal("une poignée de main sans jeton a été acceptée")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("réponse %v, attendu 401", response)
	}
}

// La liste des sessions part à l'abonnement, puis à chaque changement — et
// une liste identique ne repart pas.
func TestSessionsArePushedOnSubscribeThenOnChangeOnly(t *testing.T) {
	server, httpServer := facadeTestServer(t)

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "sessions"}})

	first := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "sessions" })
	if first.Topic != "sessions" || string(first.Data) != "[]" {
		t.Fatalf("premier instantané : %+v", first)
	}

	server.pool.Register(registry.Session{ID: "abc", Platform: "retrogaming", Initiator: "root", MaxPlayers: 2})
	second := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "sessions" })
	var sessions []map[string]any
	if err := json.Unmarshal(second.Data, &sessions); err != nil || len(sessions) != 1 || sessions[0]["id"] != "abc" {
		t.Fatalf("la session lancée n'est pas arrivée : %s", second.Data)
	}

	// Un réveil du pool qui ne change rien ne renvoie rien.
	server.pool.Notify()
	_ = connection.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var extra facadeTestPush
	if err := connection.ReadJSON(&extra); err == nil && extra.Type == "sessions" {
		t.Fatalf("une liste identique est repartie : %s", extra.Data)
	}
}

// Une page qui ouvre la partie APRÈS la fin du lancement en voit toute
// l'histoire — le kernel la garde une minute.
func TestALaunchIsReplayedToTheTopicSubscriber(t *testing.T) {
	server, httpServer := facadeTestServer(t)
	server.launches.publish("L1", "session.requested", "nes")
	server.launches.done("L1", "abc")

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "launch/L1"}})

	var steps []launchStep
	for len(steps) < 2 {
		push := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "launch" })
		if push.Topic != "launch/L1" {
			t.Fatalf("sujet %q", push.Topic)
		}
		var step launchStep
		_ = json.Unmarshal(push.Data, &step)
		steps = append(steps, step)
	}
	if steps[0].Step != "session.requested" || steps[1].Type != "done" || steps[1].SessionID != "abc" {
		t.Fatalf("histoire rejouée : %+v", steps)
	}
}

// UN LANCEMENT SE TERMINE TOUJOURS : refusé avant même d'avoir publié une
// étape, il dit quand même sa fin — sur l'id de la commande, puis dans sa
// réponse, qui porte le statut et le corps de l'ancienne route.
func TestALaunchCommandAlwaysEnds(t *testing.T) {
	_, httpServer := facadeTestServer(t)

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "launch", "id": "7", "data": map[string]any{"platform": "bogus"}})

	reply := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "reply" && push.ID == "7" })
	if reply.Status != http.StatusBadRequest || reply.LaunchID == "" {
		t.Fatalf("réponse : %+v", reply)
	}
	var body map[string]any
	_ = json.Unmarshal(reply.Data, &body)
	if body["success"] != false || !strings.Contains(body["errorMessage"].(string), "unknown platform") {
		t.Fatalf("corps : %s", reply.Data)
	}

	// Et l'histoire du lancement se termine : une page qui s'y abonne voit
	// l'échec au lieu d'attendre dix minutes.
	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "launch/" + reply.LaunchID}})
	push := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "launch" })
	var step launchStep
	_ = json.Unmarshal(push.Data, &step)
	if step.Type != "failed" || step.Step != "session.requested" || !strings.Contains(step.Error, "unknown platform") {
		t.Fatalf("fin du lancement : %+v", step)
	}
}

// Ce qui n'existe pas se dit — une page ne doit pas attendre une réponse qui
// ne viendra jamais.
func TestTheMessageCentreAnswersWhatItCannotDo(t *testing.T) {
	_, httpServer := facadeTestServer(t)

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	cases := []struct {
		message map[string]any
		status  int
	}{
		{map[string]any{"type": "bogus", "id": "a"}, http.StatusBadRequest},
		{map[string]any{"type": "subscribe", "id": "b", "data": map[string]any{"topic": "bogus"}}, http.StatusBadRequest},
		{map[string]any{"type": "subscribe", "id": "c", "data": map[string]any{"topic": "status/bogus/abc"}}, http.StatusNotFound},
		{map[string]any{"type": "session.stop", "id": "d", "data": map[string]any{"platform": "bogus", "sessionId": "abc"}}, http.StatusBadRequest},
		{map[string]any{"type": "session.stop", "id": "e", "data": map[string]any{"platform": "retrogaming"}}, http.StatusBadRequest},
		// Une intention n'est pas l'affaire du kernel : elle va au child.
		{map[string]any{"type": "session.intent", "id": "f", "data": map[string]any{"action": "join"}}, http.StatusBadRequest},
	}

	for _, entry := range cases {
		facadeTestSend(t, connection, entry.message)
		id := entry.message["id"].(string)
		reply := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "reply" && push.ID == id })
		if reply.Status != entry.status {
			t.Errorf("%v : statut %d, attendu %d (%s)", entry.message, reply.Status, entry.status, reply.Data)
		}
	}
}

// Le résumé d'une ressource distante part à l'abonnement, puis au début et à
// la fin d'un parcours — plus jamais relu par la page toutes les 4 s.
func TestAMountSummaryIsPushedWhenAScanStartsAndEnds(t *testing.T) {
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":7,"name":"Drive","kind":"webdav","base_url":"http://drive.invalid","folder":"games"}]}`))
	}))
	t.Cleanup(registryServer.Close)

	previous := listingCacheDir
	listingCacheDir = t.TempDir()
	t.Cleanup(func() { listingCacheDir = previous })

	server, httpServer := facadeTestServer(t)
	server.config.CredentialURL = registryServer.URL
	server.config.InternalSecret = "interne"

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	summary := func() map[string]any {
		push := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "mount" })
		if push.Topic != "mount/7" {
			t.Fatalf("sujet %q, attendu mount/7", push.Topic)
		}
		var value map[string]any
		if err := json.Unmarshal(push.Data, &value); err != nil {
			t.Fatalf("résumé illisible : %s", push.Data)
		}

		return value
	}

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "mount/7"}})
	if first := summary(); first["scanned"] != false || first["scanning"] != false {
		t.Fatalf("premier instantané : %v", first)
	}

	key := resourceKey(mountedShare{ID: 7, Kind: "webdav", BaseURL: "http://drive.invalid", Folder: "games"})
	server.rescans.Start(key)
	if started := summary(); started["scanning"] != true {
		t.Fatalf("début de parcours : %v", started)
	}

	built := listing.New("webdav", "drive")
	built.SetSection(listing.Roms, map[string][]listing.Item{"nes": {{Name: "Zelda"}}}, time.Now())
	if err := listing.Save(listingCacheDir, key, built); err != nil {
		t.Fatalf("catalogue : %v", err)
	}
	server.rescans.Finish(key, nil)
	finished := summary()
	consoles, _ := finished["consoles"].([]any)
	if finished["scanned"] != true || finished["scanning"] != false || len(consoles) != 1 {
		t.Fatalf("fin de parcours : %v", finished)
	}

	// Une ressource inconnue est refusée, pas suivie dans le vide.
	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "2", "data": map[string]any{"topic": "mount/99"}})
	refused := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "2" })
	if refused.Status != http.StatusNotFound {
		t.Fatalf("ressource inconnue : %+v", refused)
	}
}

// UN JOUEUR, UNE SESSION — L'EMPRUNTEUR COMPRIS (#274). Celui à qui le
// propriétaire prête sa manette n'a pas de place à lui, mais il tient une
// manette : le kernel le compte comme jouant, et le child d'une autre partie
// refusera de l'asseoir.
func TestABorrowerCountsAsPlaying(t *testing.T) {
	server, _ := facadeTestServer(t)
	server.pool.Register(registry.Session{ID: "s1", Platform: "retrogaming", Initiator: "gino", MaxPlayers: 2})
	server.mirrors.Put("s1", registry.MirrorSnapshot{
		Owner:   "gino",
		Players: []registry.MirrorPlayer{{SlotID: 1, Username: "gino", LentTo: "gixi"}},
	})

	for _, username := range []string{"gino", "gixi"} {
		if session, playing := server.alreadyPlaying(username); !playing || session != "s1" {
			t.Fatalf("alreadyPlaying(%s) = (%q, %v), want (s1, true)", username, session, playing)
		}
	}
	if _, playing := server.alreadyPlaying("dave"); playing {
		t.Fatal("a viewer without a pad does not play")
	}
}

// TA PARTIE CONTINUE SANS TOI (#276). Relancer la coupait pour ceux qui y
// jouent : elle se signale maintenant, pour que la page propose d'y retourner
// ou de la terminer pour tout le monde. Une partie sans personne, elle, ne
// coupe personne : elle n'arrête pas la relance.
func TestAGameThatGoesOnWithoutItsOwnerIsSaidBeforeARelaunch(t *testing.T) {
	server, _ := facadeTestServer(t)
	server.pool.Register(registry.Session{ID: "s1", Platform: "retrogaming", Initiator: "gino", Title: "Mario Kart", MaxPlayers: 4})
	server.mirrors.Put("s1", registry.MirrorSnapshot{
		Owner:   "gino",
		Players: []registry.MirrorPlayer{{SlotID: 2, Username: "alice"}, {SlotID: 3, Username: "bob"}},
	})

	session, players, goesOn := server.ownedGameGoesOn("gino")
	if !goesOn || session.ID != "s1" || strings.Join(players, ",") != "alice,bob" {
		t.Fatalf("ownedGameGoesOn(gino) = (%s, %v, %v), want (s1, [alice bob], true)", session.ID, players, goesOn)
	}
	if _, _, goesOn := server.ownedGameGoesOn("alice"); goesOn {
		t.Fatal("alice owns nothing here")
	}

	server.mirrors.Put("s1", registry.MirrorSnapshot{Owner: "gino"})
	if _, _, goesOn := server.ownedGameGoesOn("gino"); goesOn {
		t.Fatal("a game nobody plays must not stop a relaunch")
	}
}

// LE REFUS SE LIT À LA RACINE, comme « tu joues déjà ailleurs » : la page lit
// `activeSessionId` et `ownedGameGoesOn` sur le corps lui-même. Emballé dans
// `{success, data}`, il lui arrivait sans rien (vu le 2026-10-09).
func TestTheGoesOnRefusalIsReadAtTheRoot(t *testing.T) {
	server, _ := facadeTestServer(t)
	recorder := httptest.NewRecorder()
	server.writeOwnedGameGoesOn(recorder, registry.Session{ID: "s1", Platform: "retrogaming", Title: "Mario Kart"}, []string{"alice"})

	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if recorder.Code != http.StatusConflict || body["ownedGameGoesOn"] != true || body["activeSessionId"] != "s1" || body["success"] != false {
		t.Fatalf("status %d, body %+v — want a bare 409 with the fields at the root", recorder.Code, body)
	}
}

// UN LANCEMENT À LA FOIS PAR COMPTE (2026-10-09). Trois lancements arrivés
// ensemble ouvraient trois parties : la règle « une partie par joueur » lit le
// pool, où rien n'était encore inscrit. Le second est refusé tant que le
// premier court — et passe une fois le premier terminé.
func TestASecondLaunchIsRefusedWhileTheFirstRuns(t *testing.T) {
	server, httpServer := facadeTestServer(t)
	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "gino"))
	if err != nil {
		t.Fatalf("connexion : %v", err)
	}
	defer connection.Close()

	server.launching.Store("gino", struct{}{})
	facadeTestSend(t, connection, map[string]any{"type": "launch", "id": "2", "data": map[string]any{"platform": "retrogaming"}})
	refused := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "reply" && push.ID == "2" })
	if refused.Status != http.StatusConflict {
		t.Fatalf("second lancement pendant le premier : %d, attendu 409", refused.Status)
	}

	// Le premier terminé, le suivant n'est plus arrêté par cette règle-ci (il
	// l'est ici par l'absence de gestionnaire, mais plus par un 409).
	server.launching.Delete("gino")
	facadeTestSend(t, connection, map[string]any{"type": "launch", "id": "3", "data": map[string]any{"platform": "nope"}})
	next := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "reply" && push.ID == "3" })
	if next.Status == http.StatusConflict {
		t.Fatalf("lancement après le premier : 409, la règle ne s'est pas relâchée")
	}
	if _, still := server.launching.Load("gino"); still {
		t.Fatal("le lancement fini a laissé le compte marqué « en cours »")
	}
}
