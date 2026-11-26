package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// LE RELAIS NE REPASSE QUE LE CATALOGUE (chantier 83) : voir et télécharger ; jamais une partie, une console Remote
// Play, l'identité, ni la recherche par les comptes des fournisseurs de l'origine.
func TestOnlyTheCatalogIsRelayed(t *testing.T) {
	for path, want := range map[string]bool{
		"/listing": true, "/listing/updated": true, "/roms/nes/abc": true, "/bios/ps1/scph1001.bin": true,
		"/metadata/nes/abc/images/cover": true, "/patches/ps4/CUSA00001": true,
		"/sessions": false, "/remoteplay/devices": false, "/identity": false, "/relay/3/listing": false,
		"/metadata/nes/providers/igdb": false, "/roms/../../secret": false,
	} {
		if got := remote.Relayable(path); got != want {
			t.Errorf("relayable(%q) = %v, attendu %v", path, got, want)
		}
	}
}

// LE LISTING RELAYÉ : rien à lancer, la portée du catalogue seule, et un seul saut.
func TestARelayedListingOnlyShowsAndDownloads(t *testing.T) {
	origin := listing.New(listing.SelfSource, "Origine")
	origin.Header.Scopes = []string{scopeCatalog, scopeMetadataProviders}
	origin.Header.Consoles = []string{"*"}
	origin.Header.RelayAllowed = true
	origin.Header.Relayed = []listing.RelayedServer{{ID: 9, Name: "Plus loin"}}
	body, _ := json.Marshal(map[string]any{"success": true, "data": origin})

	recorder := httptest.NewRecorder()
	(&Server{}).writeRelayedListing(recorder, "/listing", strings.NewReader(string(body)))
	var envelope struct {
		Data listing.Listing `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	served := envelope.Data
	if served.Header.Name != "Origine" || served.Header.Version == 0 {
		t.Fatalf("le reste de l'en-tête doit passer tel quel : %+v", served.Header)
	}
	if len(served.Header.Consoles) != 0 || served.Header.RelayAllowed || len(served.Header.Relayed) != 0 {
		t.Fatalf("en-tête relayé : %+v", served.Header)
	}
	if len(served.Header.Scopes) != 1 || served.Header.Scopes[0] != scopeCatalog {
		t.Fatalf("portées relayées : %v", served.Header.Scopes)
	}
}
