package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gameflex/storage/listing"
)

// LE RELAIS D'UN JEU : le fichier du pair passe tel quel, sous son nom — tiré de
// ce que son listing annonce quand le pair, un TÉLÉPHONE, ne le pose pas ; et
// un refus se rend avant le premier octet, avec son statut.
func TestRelayPasseLeFichierEtDitSesRefus(t *testing.T) {
	denied := false
	peerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if denied {
			w.WriteHeader(http.StatusForbidden)

			return
		}
		if r.URL.Path == "/api/v1/federation/roms/nes/4f9523a0" {
			_, _ = w.Write([]byte("le hack!"))

			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer peerServer.Close()

	peer := Peer{ID: 7, Link: peerServer.URL, Token: "jeton"}
	metroid := listing.Item{ID: "4f9523a0", Name: "Metroid", Files: []string{"Metroid.nes"}}

	response := httptest.NewRecorder()
	if err := Relay(response, httptest.NewRequest(http.MethodGet, "/", nil), peer, "nes", metroid); err != nil {
		t.Fatal(err)
	}
	if response.Body.String() != "le hack!" || response.Header().Get("Content-Disposition") != `attachment; filename="Metroid.nes"` {
		t.Fatalf("relais = %q, %q", response.Body.String(), response.Header().Get("Content-Disposition"))
	}

	var refused *StatusError
	gone := listing.Item{ID: "zelda", Name: "Zelda", Files: []string{"Zelda.nes"}}
	if err := Relay(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), peer, "nes", gone); !errors.As(err, &refused) || refused.Status != http.StatusBadGateway {
		t.Errorf("un fichier que le pair ne rend plus : %v, attendu 502", err)
	}
	denied = true
	if err := Relay(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), peer, "nes", metroid); !errors.As(err, &refused) || refused.Status != http.StatusForbidden {
		t.Errorf("un pair qui refuse : %v, attendu 403", err)
	}
	if got := Refusal(errors.New("injoignable")); got.Status != http.StatusBadGateway {
		t.Errorf("un pair éteint : %d, attendu 502", got.Status)
	}
	if got := Refusal(unknownGame); got.Status != http.StatusNotFound {
		t.Errorf("un refus déjà nommé se rend tel quel : %d, attendu 404", got.Status)
	}
}

// LE NOM D'UN JEU VENU D'UN PAIR — signalé le 2026-10-10 : un jeu N64 d'un pair
// finissait en « ROM not found », parce que la référence d'un serveur est un
// UUID et que le titre seul servait de nom.
func TestFileNamePorteLExtension(t *testing.T) {
	cases := []struct {
		nom     string
		game    listing.Item
		attendu string
	}{
		{"un fichier prend l'extension annoncée",
			listing.Item{Name: "Super Mario 64", Files: []string{"Super Mario 64.z64"}}, "Super Mario 64.z64"},
		// 26 dossiers PSP sur 26 n'ont qu'un `.iso` à leur racine : un
		// fichier, mais un dossier — il garde son titre.
		{"un dossier garde son titre, même avec un seul fichier",
			listing.Item{Name: "[PSP] Crisis Core", Files: []string{"[PSP] Crisis Core.iso"}, Dir: true}, "[PSP] Crisis Core"},
		{"rien d'annoncé : le titre, faute de mieux",
			listing.Item{Name: "Mystère"}, "Mystère"},
	}
	for _, test := range cases {
		if got, err := FileName(test.game); err != nil || got != test.attendu {
			t.Errorf("%s : %q (%v), attendu %q", test.nom, got, err, test.attendu)
		}
	}
	if _, err := FileName(listing.Item{Name: "../../databases"}); err == nil {
		t.Error("un titre « ../../databases » a été accepté comme nom de fichier")
	}
}
