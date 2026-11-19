package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/database"
)

// CHANTIER 66 — couper des consoles pour tout le serveur, ou pour un accès délivré ; et la session fédérée qui dit
// par quel accès elle est venue.

// snapshotOf — ce que les kernels reçoivent avec les comptes : les accès et les consoles coupées.
func snapshotOf(t *testing.T, server *Server) (accesses []accessEntry, disabled []string) {
	t.Helper()
	payload, err := server.accountsSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Accesses         []accessEntry `json:"accesses"`
		DisabledConsoles []string      `json:"disabledConsoles"`
	}
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}

	return message.Accesses, message.DisabledConsoles
}

func TestAConsoleIsSwitchedOffAndOnForTheWholeServer(t *testing.T) {
	server, _ := tokenServer(t)

	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "ps5", `{"enabled":false}`); answer.Code != http.StatusOK {
		t.Fatalf("couper la PS5 : %d %s", answer.Code, answer.Body.String())
	}
	if _, disabled := snapshotOf(t, server); len(disabled) != 1 || disabled[0] != "ps5" {
		t.Fatalf("consoles coupées envoyées aux kernels : %v, attendu [ps5]", disabled)
	}
	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "nintendo-game-boy-inventee", `{"enabled":false}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("une console inconnue : %d, attendu 400", answer.Code)
	}
	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "ps5", `{"enabled":true}`); answer.Code != http.StatusOK {
		t.Fatalf("rallumer la PS5 : %d %s", answer.Code, answer.Body.String())
	}
	if _, disabled := snapshotOf(t, server); len(disabled) != 0 {
		t.Fatalf("consoles coupées après rallumage : %v, attendu aucune", disabled)
	}
}

func TestAnAccessIsRestrictedLikeAMember(t *testing.T) {
	server, db := tokenServer(t)
	var id string
	if err := db.QueryRow(`SELECT id FROM federation_tokens WHERE token_hash = ?`, hashToken(testAccess)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	accesses, _ := snapshotOf(t, server)
	if len(accesses) != 1 || len(accesses[0].Consoles) != 1 || accesses[0].Consoles[0] != "*" {
		t.Fatalf("un accès neuf : %+v, attendu toutes les consoles", accesses)
	}
	answer := adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, id, `{"consoles":["retro","rp-ps5"]}`)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"consoles":["retro","rp-ps5"]`) {
		t.Fatalf("restreindre l'accès : %d %s", answer.Code, answer.Body.String())
	}
	if accesses, _ := snapshotOf(t, server); strings.Join(accesses[0].Consoles, ",") != "retro,rp-ps5" {
		t.Fatalf("l'accès envoyé aux kernels : %+v", accesses)
	}
	// Changer ses consoles ne touche pas ses portées.
	var scopes string
	_ = db.QueryRow(`SELECT scopes FROM federation_tokens WHERE id = ?`, id).Scan(&scopes)
	if scopes != "catalog" {
		t.Fatalf("portées après les consoles : %q, attendu catalog", scopes)
	}
	if answer := adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, id, `{"consoles":["ps9"]}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("une console inconnue : %d, attendu 400", answer.Code)
	}
}

func TestTheFederatedSessionCarriesItsAccess(t *testing.T) {
	server, db := tokenServer(t)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1m"}`)
	var id int64
	if err := db.QueryRow(`SELECT id FROM federation_tokens WHERE token_hash = ?`, hashToken(testAccess)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	answer := sessionCall(server, tokenOf(t, db, 2))
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil {
		t.Fatalf("session : %d %s", answer.Code, answer.Body.String())
	}
	claims, err := auth.ParseToken(server.config.JWTSecret, body.Item.AccessToken)
	if err != nil || claims.Acc != id {
		t.Fatalf("l'accès de la session : %+v (%v), attendu %d", claims, err, id)
	}
}

// LA PORTÉE `devices` QUITTE LES JETONS EXISTANTS au démarrage (database.Open) : ils gardent le reste.
func TestTheDevicesScopeLeavesStoredTokens(t *testing.T) {
	_, db := tokenServer(t)
	if _, err := db.Exec(`UPDATE federation_tokens SET scopes = 'catalog,devices,metadata-providers'`); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var scopes string
	_ = reopened.QueryRow(`SELECT scopes FROM federation_tokens`).Scan(&scopes)
	if scopes != "catalog,metadata-providers" {
		t.Fatalf("portées : %q, attendu catalog,metadata-providers", scopes)
	}
}
