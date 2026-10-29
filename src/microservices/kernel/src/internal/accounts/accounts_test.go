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
	if role, ok := store.Admit("inconnu", "", "", 1); !ok || role != 1 {
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
	if _, ok := store.Admit("supprime", "", "", 1); ok {
		t.Fatal("un compte absent de la liste est entré")
	}
	if role, ok := store.Admit("marc", "", "", 1); !ok || role != RoleUser {
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
	if role, ok := store.Admit("marc", "", "bbbb2222", 1); !ok || role != RoleUser {
		t.Fatalf("bonne génération : %d / %v", role, ok)
	}
	if _, ok := store.Admit("marc", "", "cccc3333", 2); ok {
		t.Fatal("une génération régénérée depuis est entrée")
	}
	if _, ok := store.Admit("admin", "", "aaaa1111", 1); ok {
		t.Fatal("un jeton désactivé a ouvert une session")
	}
	store.now = func() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }
	if _, ok := store.Admit("marc", "", "bbbb2222", 2); ok {
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

// LA COPIE SE TIENT AU CANAL : une liste reçue, la copie est prête.
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
		conn.ReadMessage()
	}))
	defer server.Close()

	store := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Follow(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "secret")

	for deadline := time.Now().Add(5 * time.Second); !store.Ready(); {
		if time.Now().After(deadline) {
			t.Fatal("aucune liste reçue")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := store.Admit("marc", "", "", 2); !ok {
		t.Fatal("marc, dans la liste reçue, est refusé")
	}
}

// UN COMPTE RECRÉÉ SOUS LE MÊME NOM N'HÉRITE PAS DES JETONS DE L'ANCIEN (#601) :
// le jeton nomme l'identifiant du compte (`sub`), et celui du nouveau diffère.
func TestARecreatedAccountRefusesTheTokensOfTheOldOne(t *testing.T) {
	store := New()
	store.Replace(list(t, twoAccounts))
	if _, ok := store.Admit("marc", "2", "", 2); !ok {
		t.Fatal("le jeton de marc (id 2) a été refusé")
	}
	store.Replace(list(t, `{"type":"accounts","accounts":[
	{"id":3,"username":"marc","role_level":2,"consoles":["ps3"]}]}`))
	if _, ok := store.Admit("marc", "2", "", 2); ok {
		t.Fatal("le jeton de l'ancien marc (id 2) a ouvert une session au nouveau (id 3)")
	}
	if _, ok := store.Admit("marc", "3", "", 2); !ok {
		t.Fatal("le jeton du nouveau marc a été refusé")
	}
}
