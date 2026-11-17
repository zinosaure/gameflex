package embed

// LE LIEN D'UN COMPTE SUR LE TÉLÉPHONE (chantier 55) : le même code que le
// serveur, par le Handler embarqué — un membre créé sans mot de passe, rejoint
// par son lien, réinitialisé : son jeton ne vaut plus (RoleOf, que le kernel du
// téléphone appelle pour chaque geste), et le kernel est prévenu. L'inscription
// libre est partie : c'est ainsi qu'un compte naît désormais au téléphone.

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
)

func TestTheDeviceInvitesAndResetsLikeAServer(t *testing.T) {
	service, err := New(Options{
		DBPath:       filepath.Join(t.TempDir(), "identity.db"),
		JWTSecret:    "secret-de-test",
		RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.DB.Close() })
	changed := make(chan struct{}, 8)
	service.OnAccountsChanged(func() { changed <- struct{}{} })
	root := login(t, service, "root", "mot-de-passe-du-porteur")

	created := call(t, service, http.MethodPost, "/api/v1/users", root, map[string]any{"username": "lea", "display_name": "Léa"})
	var member struct {
		Item struct {
			ID         int64 `json:"id"`
			Invitation struct {
				Code string `json:"code"`
			} `json:"invitation"`
		} `json:"item"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &member); err != nil || created.Code != http.StatusCreated || member.Item.Invitation.Code == "" {
		t.Fatalf("créer lea = %d %s", created.Code, created.Body.String())
	}
	joinWith := func(code string) string {
		t.Helper()
		answer := call(t, service, http.MethodPost, "/api/v1/auth/join", "", map[string]string{"code": code, "password": "Gameflex#2026"})
		var session struct {
			Item struct {
				AccessToken string `json:"access_token"`
			} `json:"item"`
		}
		if err := json.Unmarshal(answer.Body.Bytes(), &session); err != nil || session.Item.AccessToken == "" {
			t.Fatalf("rejoindre = %d %s", answer.Code, answer.Body.String())
		}

		return session.Item.AccessToken
	}
	lea := joinWith(member.Item.Invitation.Code)
	if role, err := service.RoleOf(lea); err != nil || role != auth.RoleUser {
		t.Fatalf("le jeton du lien : %d, %v", role, err)
	}
	if login(t, service, "lea", "Gameflex#2026") == "" {
		t.Fatal("lea ne se connecte pas avec le mot de passe choisi")
	}

	for len(changed) > 0 {
		<-changed
	}
	reset := call(t, service, http.MethodPost, "/api/v1/users/"+jsonNumber(member.Item.ID)+"/reset", root, nil)
	var link struct {
		Item struct {
			Invitation struct {
				Code string `json:"code"`
			} `json:"invitation"`
		} `json:"item"`
	}
	if err := json.Unmarshal(reset.Body.Bytes(), &link); err != nil || reset.Code != http.StatusOK || link.Item.Invitation.Code == "" {
		t.Fatalf("réinitialiser lea = %d %s", reset.Code, reset.Body.String())
	}
	if _, err := service.RoleOf(lea); err == nil {
		t.Fatal("le jeton de lea vaut encore après sa réinitialisation")
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("le kernel du téléphone n'est pas prévenu de la réinitialisation")
	}
	if again := joinWith(link.Item.Invitation.Code); again == "" {
		t.Fatal("le lien de réinitialisation ne fait pas rejoindre")
	}
}

func jsonNumber(id int64) string {
	encoded, _ := json.Marshal(id)

	return string(encoded)
}
