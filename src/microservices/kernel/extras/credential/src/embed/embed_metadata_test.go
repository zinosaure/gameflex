package embed

// LE FOURNISSEUR DE MÉTADONNÉES DU TÉLÉPHONE (chantier 57, P4) : les comptes saisis par la page d'administration
// (`/api/v1/metadata-services`, le Handler embarqué) se lisent en clair par le kernel du téléphone, dans le même
// processus ; l'API ne rend jamais leurs valeurs. L'interrupteur « Écrire les métadonnées sur le disque du jeu » aussi,
// allumé par défaut.

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheDeviceReadsItsMetadataAccountsAndSwitch(t *testing.T) {
	service, err := New(Options{
		DBPath:       filepath.Join(t.TempDir(), "identity.db"),
		JWTSecret:    "secret-de-test",
		RootPassword: "mot-de-passe-du-porteur",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.DB.Close() })
	root := login(t, service, "root", "mot-de-passe-du-porteur")

	if accounts, err := service.MetadataAccounts(); err != nil || len(accounts) != 0 {
		t.Fatalf("comptes d'un appareil neuf = %v, %v", accounts, err)
	}
	if on, err := service.MetadataWriteOnGameDisk(); err != nil || !on {
		t.Fatalf("interrupteur par défaut = %v, %v ; attendu allumé", on, err)
	}

	saved := call(t, service, http.MethodPut, "/api/v1/metadata-services/steamgriddb", root, map[string]any{"fields": map[string]string{"api_key": "cle-factice"}})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body.String(), "cle-factice") {
		t.Fatalf("enregistrer SteamGridDB = %d %s (la valeur ne doit jamais ressortir)", saved.Code, saved.Body.String())
	}
	if accounts, err := service.MetadataAccounts(); err != nil || accounts["steamgriddb"]["api_key"] != "cle-factice" {
		t.Fatalf("comptes lus par le kernel = %v, %v", accounts, err)
	}

	switched := call(t, service, http.MethodPut, "/api/v1/metadata-services/settings", root, map[string]any{"write_on_game_disk": false})
	if switched.Code != http.StatusOK {
		t.Fatalf("couper l'interrupteur = %d %s", switched.Code, switched.Body.String())
	}
	if on, err := service.MetadataWriteOnGameDisk(); err != nil || on {
		t.Fatalf("interrupteur coupé = %v, %v", on, err)
	}

	if removed := call(t, service, http.MethodDelete, "/api/v1/metadata-services/steamgriddb", root, nil); removed.Code != http.StatusOK {
		t.Fatalf("retirer SteamGridDB = %d %s", removed.Code, removed.Body.String())
	}
	if accounts, err := service.MetadataAccounts(); err != nil || len(accounts) != 0 {
		t.Fatalf("comptes après retrait = %v, %v", accounts, err)
	}
}
