package embed

// LE JETON D'UN COMPTE SUR LE TÉLÉPHONE (chantier 20) : les mêmes règles que le
// serveur, sans HTTP. Un jeton désactivé ne vaut rien ; activé, il ouvre une
// session à son compte qui n'est jamais administrateur ; un compte supprimé n'a
// plus de rôle ; et le kernel est prévenu de chaque changement. Et le jeton
// qu'un membre du téléphone colle pour un serveur fédéré se garde avec son
// compte, et part avec lui ou avec le serveur.

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestTheDeviceJudgesAccountTokensLikeAServer(t *testing.T) {
	service, err := New(Options{
		DBPath:       filepath.Join(t.TempDir(), "identity.db"),
		JWTSecret:    "secret-de-test",
		RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan struct{}, 8)
	service.OnAccountsChanged(func() { changed <- struct{}{} })
	waitChange := func(what string) {
		t.Helper()
		select {
		case <-changed:
		case <-time.After(3 * time.Second):
			t.Fatalf("le kernel n'a pas été prévenu : %s", what)
		}
	}

	root := login(t, service, "root", "mot-de-passe-du-porteur")
	created := call(t, service, http.MethodPost, "/api/v1/users", root, map[string]any{
		"username": "marc", "password": "mot-de-passe-de-marc", "display_name": "Marc", "role_level": 1,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("création : %d %s", created.Code, created.Body.String())
	}
	waitChange("création")
	var token string
	if err := service.DB.QueryRow(`SELECT t.token FROM account_tokens t JOIN users u ON u.id = t.user_id WHERE u.username = 'marc'`).Scan(&token); err != nil {
		t.Fatal(err)
	}

	if _, _, err := service.OpenFederatedSession(token); !errors.Is(err, ErrTokenRefused) {
		t.Fatalf("un jeton désactivé : %v, attendu ErrTokenRefused", err)
	}
	enabled := call(t, service, http.MethodPatch, "/api/v1/users/2/token", root, map[string]any{"enabled": true, "duration": "1w"})
	if enabled.Code != http.StatusOK {
		t.Fatalf("activation : %d %s", enabled.Code, enabled.Body.String())
	}
	waitChange("activation")
	session, account, err := service.OpenFederatedSession(token)
	if err != nil || account.Username != "marc" || len(account.Consoles) != 1 || account.Consoles[0] != "*" {
		t.Fatalf("le compte du jeton : %+v (%v)", account, err)
	}
	if role, err := service.RoleOf(session); err != nil || role != 2 {
		t.Fatalf("la session d'un administrateur, ouverte par son jeton : rang %d (%v), attendu 2", role, err)
	}

	if deleted := call(t, service, http.MethodDelete, "/api/v1/users/2", root, nil); deleted.Code != http.StatusOK && deleted.Code != http.StatusNoContent {
		t.Fatalf("suppression : %d %s", deleted.Code, deleted.Body.String())
	}
	waitChange("suppression")
	if _, err := service.RoleOf(session); err == nil {
		t.Fatal("la session d'un compte supprimé a encore un rang")
	}
	if _, _, err := service.OpenFederatedSession(token); !errors.Is(err, ErrTokenRefused) {
		t.Fatalf("le jeton d'un compte supprimé : %v", err)
	}
}

// MON COMPTE CHEZ UN AMI (chantier 20) : le jeton qu'un membre du téléphone
// colle pour un serveur fédéré se garde avec son compte, se remplace, s'oublie
// — et part avec le serveur retiré, ou avec le membre supprimé.
func TestTheDeviceKeepsAMembersTokenForAFriend(t *testing.T) {
	service, err := New(Options{
		DBPath:       filepath.Join(t.TempDir(), "identity.db"),
		JWTSecret:    "secret-de-test",
		RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := login(t, service, "root", "mot-de-passe-du-porteur")
	added := call(t, service, http.MethodPost, "/api/v1/server-registry", root, map[string]any{
		"link": "https://ami.example", "token": "jeton-du-lien", "hostname": "Chez l'ami",
	})
	if added.Code != http.StatusCreated {
		t.Fatalf("ajout du serveur : %d %s", added.Code, added.Body.String())
	}

	if _, err := service.FederatedAccountToken("root", 1); !errors.Is(err, ErrNoFederatedAccount) {
		t.Fatalf("avant d'avoir collé quoi que ce soit : %v", err)
	}
	if err := service.SetFederatedAccountToken("root", 99, "jeton"); !errors.Is(err, ErrNoFederatedAccount) {
		t.Fatalf("un serveur inconnu : %v", err)
	}
	for _, pasted := range []string{"premier-jeton", "second-jeton"} {
		if err := service.SetFederatedAccountToken("root", 1, pasted); err != nil {
			t.Fatal(err)
		}
		if kept, err := service.FederatedAccountToken("root", 1); err != nil || kept != pasted {
			t.Fatalf("gardé : %q (%v), attendu %q", kept, err, pasted)
		}
	}
	if err := service.ForgetFederatedAccountToken("root", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FederatedAccountToken("root", 1); !errors.Is(err, ErrNoFederatedAccount) {
		t.Fatalf("oublié : %v", err)
	}

	// Le serveur retiré emporte les jetons qu'on avait pour lui.
	if err := service.SetFederatedAccountToken("root", 1, "jeton"); err != nil {
		t.Fatal(err)
	}
	if removed := call(t, service, http.MethodDelete, "/api/v1/server-registry/1", root, nil); removed.Code != http.StatusNoContent {
		t.Fatalf("retrait du serveur : %d %s", removed.Code, removed.Body.String())
	}
	var left int
	if err := service.DB.QueryRow(`SELECT COUNT(*) FROM federated_accounts`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("après le retrait du serveur, %d jeton(s) restent (%v)", left, err)
	}

	// Le membre supprimé emporte les siens.
	created := call(t, service, http.MethodPost, "/api/v1/users", root, map[string]any{
		"username": "marc", "password": "mot-de-passe-de-marc", "display_name": "Marc", "role_level": 2,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("création : %d %s", created.Code, created.Body.String())
	}
	if again := call(t, service, http.MethodPost, "/api/v1/server-registry", root, map[string]any{
		"link": "https://ami.example", "token": "jeton-du-lien", "hostname": "Chez l'ami",
	}); again.Code != http.StatusCreated {
		t.Fatalf("nouvel ajout du serveur : %d %s", again.Code, again.Body.String())
	}
	if err := service.SetFederatedAccountToken("marc", 2, "jeton-de-marc"); err != nil {
		t.Fatal(err)
	}
	if deleted := call(t, service, http.MethodDelete, "/api/v1/users/2", root, nil); deleted.Code != http.StatusOK && deleted.Code != http.StatusNoContent {
		t.Fatalf("suppression : %d %s", deleted.Code, deleted.Body.String())
	}
	if err := service.DB.QueryRow(`SELECT COUNT(*) FROM federated_accounts`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("après la suppression du membre, %d jeton(s) restent (%v)", left, err)
	}
}
