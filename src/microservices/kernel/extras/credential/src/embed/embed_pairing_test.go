package embed

// LE CODE D'APPAIRAGE AU TÉLÉPHONE (chantier 60) : les deux modes y sont, par
// construction, comme au serveur (`#756`) ; un code `federation` y délivre un
// jeton de catalogue que VerifyCatalogToken reconnaît, jusqu'à sa révocation. Le
// kernel apprend chaque changement par OnPairingChanged, et la valeur d'un code
// n'y passe jamais.

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThePhoneHasBothModes(t *testing.T) {
	service, err := New(Options{
		DBPath: filepath.Join(t.TempDir(), "identity.db"), JWTSecret: "secret-de-test", RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan string, 16)
	service.OnPairingChanged(func(frame []byte) { frames <- string(frame) })
	root := login(t, service, "root", "mot-de-passe-du-porteur")

	if _, err := service.RedeemInvitation("123456", "federation", "Salon", "a"); !errors.Is(err, ErrPairingClosed) {
		t.Fatalf("un code federation, mode fermé : %v", err)
	}
	if answer := call(t, service, http.MethodPost, "/api/v1/token-invitations/pairing/server/open", root, nil); answer.Code != http.StatusOK {
		t.Fatalf("le mode serveur : %d %s", answer.Code, answer.Body.String())
	}
	fed := call(t, service, http.MethodPost, "/api/v1/token-invitations/pairing/codes", root, map[string]any{
		"kind": "federation", "label": "Salon", "scopes": []string{"devices"},
	})
	var fedEnvelope struct {
		Item struct {
			Code string `json:"code"`
		} `json:"item"`
	}
	_ = json.Unmarshal(fed.Body.Bytes(), &fedEnvelope)
	if fed.Code != http.StatusCreated || len(fedEnvelope.Item.Code) != 6 {
		t.Fatalf("le code federation : %d %s", fed.Code, fed.Body.String())
	}
	joined, err := service.RedeemInvitation(fedEnvelope.Item.Code, "federation", "Salon", "192.0.2.11")
	if err != nil || joined.Token == "" || joined.Scopes != "catalog,devices" {
		t.Fatalf("l'échange du code federation : %+v %v", joined, err)
	}
	if label, scopes, err := service.VerifyCatalogToken(joined.Token); err != nil || label != "Salon" || scopes != "catalog,devices" {
		t.Fatalf("le jeton délivré : %q %q %v", label, scopes, err)
	}
	if answer := call(t, service, http.MethodPatch, "/api/v1/users/1/token", root, map[string]any{"enabled": true, "duration": "1w"}); answer.Code != http.StatusOK {
		t.Fatalf("activation : %d", answer.Code)
	}
	if answer := call(t, service, http.MethodPost, "/api/v1/token-invitations/pairing/account/open", root, nil); answer.Code != http.StatusOK {
		t.Fatalf("le mode compte : %d %s", answer.Code, answer.Body.String())
	}
	created := call(t, service, http.MethodPost, "/api/v1/token-invitations/pairing/codes", root, map[string]any{"kind": "account"})
	var envelope struct {
		Item struct {
			Code string `json:"code"`
		} `json:"item"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &envelope)
	if created.Code != http.StatusCreated || len(envelope.Item.Code) != 6 {
		t.Fatalf("le code : %d %s", created.Code, created.Body.String())
	}

	redeemed, err := service.RedeemInvitation(envelope.Item.Code, "account", "Salon", "192.0.2.10")
	if err != nil || redeemed.Identity != "root" || redeemed.Token == "" {
		t.Fatalf("l'échange : %+v %v", redeemed, err)
	}
	for deadline := time.After(3 * time.Second); ; {
		select {
		case frame := <-frames:
			if strings.Contains(frame, envelope.Item.Code) {
				t.Fatal("la trame porte la valeur du code")
			}
			if strings.Contains(frame, `"reason":"used"`) && strings.Contains(frame, `"by":"Salon"`) {
				return
			}
		case <-deadline:
			t.Fatalf("le kernel n'a pas appris l'échange : %s", service.PairingFrame())
		}
	}
}
