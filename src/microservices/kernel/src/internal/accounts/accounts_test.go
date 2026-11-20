package accounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func list(t *testing.T, payload string) []Account {
	t.Helper()
	accounts, err := decodeList([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}

	return accounts
}

const twoAccounts = `{"type":"accounts","accounts":[
	{"id":1,"username":"admin","role_level":1,"consoles":["*"],"token":{"generation":"aaaa1111","enabled":false}},
	{"id":2,"username":"marc","role_level":2,"consoles":["ps3"],"token":{"generation":"bbbb2222","enabled":true,"expires_at":"2099-01-01T00:00:00Z"}}]}`

// AVANT TOUTE LISTE, RIEN NE BLOQUE : la promesse de internal/auth — une panne
// du service d'identité n'empêche pas de jouer.
func TestBeforeAnyListEveryValidTokenPasses(t *testing.T) {
	store := New()
	if role, ok := store.Admit("inconnu", "", "", 0, 0, 1); !ok || role != 1 {
		t.Fatalf("avant la liste : %d / %v, attendu 1 / true", role, ok)
	}
	if _, known := store.Grants("inconnu"); known {
		t.Fatal("avant la liste, les consoles ne sont pas connues")
	}
}

// APRÈS LA LISTE : un compte inconnu est refusé, le rang est celui de la liste
// (pas celui du jeton), les consoles aussi.
func TestAfterTheListTheAccountDecides(t *testing.T) {
	store := New()
	store.Replace(list(t, twoAccounts))
	if _, ok := store.Admit("supprime", "", "", 0, 0, 1); ok {
		t.Fatal("un compte absent de la liste est entré")
	}
	if role, ok := store.Admit("marc", "", "", 0, 0, 1); !ok || role != RoleUser {
		t.Fatalf("marc, jeton qui le dit administrateur : %d / %v, attendu 2 / true", role, ok)
	}
	if grants, known := store.Grants("marc"); !known || len(grants) != 1 || grants[0] != "ps3" {
		t.Fatalf("consoles de marc : %v / %v", grants, known)
	}
}

// UNE SESSION OUVERTE PAR JETON SUIT SON JETON : la bonne génération, actif,
// pas échu ; et jamais administrateur.
func TestATokenSessionFollowsItsToken(t *testing.T) {
	store := New()
	store.Replace(list(t, twoAccounts))
	if role, ok := store.Admit("marc", "", "bbbb2222", 0, 0, 1); !ok || role != RoleUser {
		t.Fatalf("bonne génération : %d / %v", role, ok)
	}
	if _, ok := store.Admit("marc", "", "cccc3333", 0, 0, 2); ok {
		t.Fatal("une génération régénérée depuis est entrée")
	}
	if _, ok := store.Admit("admin", "", "aaaa1111", 0, 0, 1); ok {
		t.Fatal("un jeton désactivé a ouvert une session")
	}
	store.now = func() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }
	if _, ok := store.Admit("marc", "", "bbbb2222", 0, 0, 2); ok {
		t.Fatal("un jeton échu a ouvert une session")
	}
}

// CE QU'UNE NOUVELLE LISTE RETIRE : un compte supprimé, un administrateur
// rétrogradé, un jeton régénéré, désactivé ou raccourci. La première liste ne
// retire rien.
func TestANewListSaysWhatItRetires(t *testing.T) {
	store := New()
	var seen Change
	store.OnChange = func(change Change) { seen = change }
	if change := store.Replace(list(t, twoAccounts)); !change.Empty() {
		t.Fatalf("la première liste retire : %+v", change)
	}

	change := store.Replace(list(t, `{"accounts":[
		{"id":1,"username":"admin","role_level":2,"consoles":["*"],"token":{"generation":"aaaa1111","enabled":false}}]}`))
	if len(change.Deleted) != 1 || change.Deleted[0] != "marc" || len(change.Demoted) != 1 || change.Demoted[0] != "admin" {
		t.Fatalf("suppression et rétrogradation : %+v", change)
	}
	if len(seen.Deleted) != 1 {
		t.Fatal("OnChange n'a pas été prévenu")
	}

	store.Replace(list(t, twoAccounts))
	for _, retire := range []string{
		`{"generation":"dddd4444","enabled":true,"expires_at":"2099-01-01T00:00:00Z"}`,
		`{"generation":"bbbb2222","enabled":false}`,
		`{"generation":"bbbb2222","enabled":true,"expires_at":"2098-01-01T00:00:00Z"}`,
	} {
		store.Replace(list(t, twoAccounts))
		change := store.Replace(list(t, `{"accounts":[
			{"id":1,"username":"admin","role_level":1,"consoles":["*"],"token":{"generation":"aaaa1111","enabled":false}},
			{"id":2,"username":"marc","role_level":2,"consoles":["ps3"],"token":`+retire+`}]}`))
		if len(change.TokenRetired) != 1 || change.TokenRetired[0] != "marc" {
			t.Fatalf("jeton %s : %+v", retire, change)
		}
	}
}

// LA COPIE SE TIENT AU CANAL : une liste reçue, la copie est prête ; la suivante
// coupe, et son `deleted` dit les comptes dont les sauvegardes partent.
func TestTheStoreFollowsTheIdentityChannel(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteMessage(websocket.TextMessage, []byte(twoAccounts))
		// marc supprimé, et nommé (`deleted`) : ses sauvegardes partent.
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"accounts","accounts":[
			{"id":1,"username":"admin","role_level":1,"consoles":["*"]}],"deleted":["marc"]}`))
		conn.ReadMessage()
	}))
	defer server.Close()

	store := New()
	changes := make(chan Change, 1)
	store.OnChange = func(change Change) { changes <- change }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Follow(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "secret")

	for deadline := time.Now().Add(5 * time.Second); !store.Ready(); {
		if time.Now().After(deadline) {
			t.Fatal("aucune liste reçue")
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case change := <-changes:
		if strings.Join(change.Deleted, ",") != "marc" || strings.Join(change.Erased, ",") != "marc" {
			t.Fatalf("la seconde liste : %+v, attendu marc supprimé et effacé", change)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la seconde liste n'a rien coupé")
	}
	if _, ok := store.Admit("marc", "", "", 0, 0, 2); ok {
		t.Fatal("marc, supprimé, est admis")
	}
}

// UN COMPTE RECRÉÉ SOUS LE MÊME NOM N'HÉRITE PAS DES JETONS DE L'ANCIEN (#601) :
// le jeton nomme l'identifiant du compte (`sub`), et celui du nouveau diffère.
func TestARecreatedAccountRefusesTheTokensOfTheOldOne(t *testing.T) {
	store := New()
	store.Replace(list(t, twoAccounts))
	if _, ok := store.Admit("marc", "2", "", 0, 0, 2); !ok {
		t.Fatal("le jeton de marc (id 2) a été refusé")
	}
	store.Replace(list(t, `{"type":"accounts","accounts":[
	{"id":3,"username":"marc","role_level":2,"consoles":["ps3"]}]}`))
	if _, ok := store.Admit("marc", "2", "", 0, 0, 2); ok {
		t.Fatal("le jeton de l'ancien marc (id 2) a ouvert une session au nouveau (id 3)")
	}
	if _, ok := store.Admit("marc", "3", "", 0, 0, 2); !ok {
		t.Fatal("le jeton du nouveau marc a été refusé")
	}
}

// LA GÉNÉRATION DES SESSIONS (chantier 55) : avant toute liste, tout passe ;
// ensuite, un jeton d'une génération plus ancienne que celle du compte est
// refusé, celui de sa génération passe — et celui d'une génération plus récente
// aussi, le jeton neuf d'un appareil pouvant arriver avant la liste qui le dit.
func TestAnOlderSessionGenerationIsRefused(t *testing.T) {
	store := New()
	if _, ok := store.Admit("marc", "2", "", 0, 0, 2); !ok {
		t.Fatal("avant la liste, un jeton valide est refusé")
	}
	store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"consoles":["*"],"session_generation":3}]}`))
	for generation, want := range map[int]bool{0: false, 2: false, 3: true, 4: true} {
		if _, ok := store.Admit("marc", "2", "", 0, generation, 2); ok != want {
			t.Fatalf("génération %d : %v, attendu %v", generation, ok, want)
		}
	}
	// Une session ouverte par jeton suit aussi la génération du compte.
	store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"consoles":["*"],"session_generation":3,
		"token":{"generation":"bbbb2222","enabled":true,"expires_at":"2099-01-01T00:00:00Z"}}]}`))
	if _, ok := store.Admit("marc", "2", "bbbb2222", 0, 2, 2); ok {
		t.Fatal("une session fédérée d'une génération coupée est entrée")
	}
}

// CE QUE DIT UNE GÉNÉRATION QUI AVANCE : `Revoked` porte la nouvelle ;
// `GamesEnded` seulement quand la génération des parties avance avec (une
// réinitialisation, « Déconnecter tous les appareils ») — changer son mot de
// passe ou couper ses autres appareils ne finit aucune partie.
func TestAnAdvancedGenerationIsRevokedAndAllDevicesEndTheGames(t *testing.T) {
	store := New()
	store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"session_generation":0},{"id":3,"username":"lea","role_level":2,"session_generation":0}]}`))

	change := store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"session_generation":1},{"id":3,"username":"lea","role_level":2,"session_generation":0}]}`))
	if change.Revoked["marc"] != 1 || len(change.Revoked) != 1 || len(change.GamesEnded) != 0 {
		t.Fatalf("mot de passe changé : %+v", change)
	}
	change = store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"session_generation":2,"games_generation":1},{"id":3,"username":"lea","role_level":2,"session_generation":0}]}`))
	if change.Revoked["marc"] != 2 || len(change.GamesEnded) != 1 || change.GamesEnded[0] != "marc" {
		t.Fatalf("tous les appareils, ou réinitialisé : %+v", change)
	}
	// Le lien accepté, une nouvelle connexion : rien à couper.
	if change := store.Replace(list(t, `{"accounts":[{"id":2,"username":"marc","role_level":2,"session_generation":2,"games_generation":1},{"id":3,"username":"lea","role_level":2,"session_generation":0}]}`)); !change.Empty() {
		t.Fatalf("rien n'a avancé, et la liste coupe : %+v", change)
	}
}

// LES SAUVEGARDES D'UN COMPTE NE PARTENT QUE S'IL EST DIT SUPPRIMÉ (le porteur,
// 2026-11-14 : « Supprimer ») : un nom qui manque seul — une base vide ou
// remplacée — coupe ses sessions, et n'efface rien ; un nom que le service
// d'identité dit supprimé, et que la liste retire, s'efface ; à la première
// liste (le kernel redémarrait), tous ceux qu'il nomme et qui n'existent plus ;
// jamais un compte qui existe.
func TestOnlyAnAccountNamedDeletedLosesItsSaves(t *testing.T) {
	store := New()
	store.Replace(list(t, `{"accounts":[{"id":1,"username":"admin","role_level":1},{"id":2,"username":"marc","role_level":2},{"id":3,"username":"lea","role_level":2}]}`))

	change := store.Replace(list(t, `{"accounts":[{"id":1,"username":"admin","role_level":1}]}`), "lea")
	if len(change.Deleted) != 2 || strings.Join(change.Erased, ",") != "lea" {
		t.Fatalf("lea supprimée, marc seulement absent : %+v", change)
	}
	// Plus tard, la même liste : lea est déjà partie, rien ne s'efface deux fois.
	if change := store.Replace(list(t, `{"accounts":[{"id":1,"username":"admin","role_level":1}]}`), "lea"); !change.Empty() {
		t.Fatalf("une liste de plus efface encore : %+v", change)
	}

	restarted := New()
	change = restarted.Replace(list(t, `{"accounts":[{"id":1,"username":"admin","role_level":1},{"id":4,"username":"paul","role_level":2}]}`), "lea", "paul")
	if strings.Join(change.Erased, ",") != "lea" || len(change.Deleted) != 0 {
		t.Fatalf("la première liste d'un kernel qui redémarrait : %+v, attendu lea seule (paul existe de nouveau)", change)
	}
}

// ADMINISTRER DEPUIS UN SERVEUR FÉDÉRÉ (chantier 73) : la session ouverte par le jeton d'un administrateur n'est que
// joueur, sauf si l'accès délivré par lequel elle est venue laisse administrer — accès par accès (« pas ma smart TV »).
func TestARemoteAdminSessionKeepsTheRankOnlyThroughAnAllowedAccess(t *testing.T) {
	store := New()
	store.Replace(list(t, `{"type":"accounts","accounts":[
		{"id":1,"username":"admin","role_level":1,"consoles":["*"],"token":{"generation":"aaaa1111","enabled":true,"expires_at":"2099-01-01T00:00:00Z"}}]}`))
	store.SetRights([]Access{{ID: 7, Consoles: []string{"*"}, RemoteAdmin: true}, {ID: 8, Consoles: []string{"*"}}}, nil)
	if role, ok := store.Admit("admin", "", "aaaa1111", 8, 0, 1); !ok || role != RoleUser {
		t.Fatalf("accès sans l'administration : %d / %v, attendu joueur", role, ok)
	}
	if role, ok := store.Admit("admin", "", "aaaa1111", 7, 0, 1); !ok || role != 1 {
		t.Fatalf("accès qui laisse administrer : %d / %v, attendu administrateur", role, ok)
	}
	if role, ok := store.Admit("admin", "", "", 0, 0, 1); !ok || role != 1 {
		t.Fatalf("session d'ici : %d / %v, attendu administrateur", role, ok)
	}
}
