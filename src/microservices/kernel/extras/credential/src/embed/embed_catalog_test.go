package embed

// LES ACCÈS DÉLIVRÉS PAR LE TÉLÉPHONE (`#756`) : `/api/v1/catalog-tokens` y est celui du serveur, et VerifyCatalogToken
// dit le nom et les portées d'un jeton émis ici, jusqu'à sa révocation ; un jeton inconnu ou vide est refusé.

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
)

func TestThePhoneIssuesAndRevokesCatalogTokens(t *testing.T) {
	service, err := New(Options{
		DBPath: filepath.Join(t.TempDir(), "identity.db"), JWTSecret: "secret-de-test", RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := login(t, service, "root", "mot-de-passe-du-porteur")

	created := call(t, service, http.MethodPost, "/api/v1/catalog-tokens", root, map[string]any{
		"label": "Ami", "scopes": []string{"metadata-providers"},
	})
	var envelope struct {
		Item struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		} `json:"item"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &envelope)
	if created.Code != http.StatusCreated || envelope.Item.Token == "" {
		t.Fatalf("création : %d %s", created.Code, created.Body.String())
	}

	if label, scopes, err := service.VerifyCatalogToken(envelope.Item.Token); err != nil || label != "Ami" || scopes != "catalog,metadata-providers" {
		t.Fatalf("vérification : %q %q %v", label, scopes, err)
	}
	for _, refused := range []string{"", "pas-un-jeton"} {
		if _, _, err := service.VerifyCatalogToken(refused); !errors.Is(err, ErrCatalogTokenRefused) {
			t.Fatalf("le jeton %q : %v, attendu ErrCatalogTokenRefused", refused, err)
		}
	}

	revoked := call(t, service, http.MethodDelete, "/api/v1/catalog-tokens/"+strconv.FormatInt(envelope.Item.ID, 10), root, nil)
	if revoked.Code != http.StatusOK && revoked.Code != http.StatusNoContent {
		t.Fatalf("révocation : %d %s", revoked.Code, revoked.Body.String())
	}
	if _, _, err := service.VerifyCatalogToken(envelope.Item.Token); !errors.Is(err, ErrCatalogTokenRefused) {
		t.Fatalf("un jeton révoqué : %v", err)
	}
}
