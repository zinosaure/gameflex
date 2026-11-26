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

// LA FICHE ET UNE IMAGE D'UN JEU D'UN PAIR (chantier 57) passent telles quelles — leur statut, leur type, leur ETag, et
// un 304 quand l'ETag est encore le bon — ; un refus se rend avant le premier octet : 403 pour un jeton refusé, 404
// pour un pair qui ne prête pas cette fiche (une version plus ancienne n'a pas la route), 502 pour un pair muet.
func TestRelayMetadataPasseLaFicheEtDitSesRefus(t *testing.T) {
	answer := http.StatusOK
	peerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answer != http.StatusOK {
			w.WriteHeader(answer)

			return
		}
		switch r.URL.RequestURI() {
		case "/api/v1/federation/metadata/ps3/jeu":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"gameId":"BLES01269"}}`))
		case "/api/v1/federation/metadata/ps3/jeu/images/cover?w=256":
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Cache-Control", "private, no-cache")
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)

				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer peerServer.Close()
	peer := Peer{ID: 3, Link: peerServer.URL, Token: "jeton"}

	sheet := httptest.NewRecorder()
	if err := RelayMetadata(sheet, httptest.NewRequest(http.MethodGet, "/", nil), peer, "ps3", "jeu"); err != nil {
		t.Fatal(err)
	}
	if sheet.Code != http.StatusOK || sheet.Body.String() != `{"success":true,"data":{"gameId":"BLES01269"}}` || sheet.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("la fiche relayée = %d %q %v", sheet.Code, sheet.Body.String(), sheet.Header())
	}
	cover := httptest.NewRecorder()
	if err := RelayImage(cover, httptest.NewRequest(http.MethodGet, "/?w=256", nil), peer, "ps3", "jeu", "cover"); err != nil {
		t.Fatal(err)
	}
	if cover.Code != http.StatusOK || cover.Body.String() != "jpeg" || cover.Header().Get("ETag") != `"v1"` || cover.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("l'image relayée = %d %q %v", cover.Code, cover.Body.String(), cover.Header())
	}
	revalidating := httptest.NewRequest(http.MethodGet, "/?w=256", nil)
	revalidating.Header.Set("If-None-Match", `"v1"`)
	unchanged := httptest.NewRecorder()
	if err := RelayImage(unchanged, revalidating, peer, "ps3", "jeu", "cover"); err != nil || unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 {
		t.Fatalf("revalidée = %d %q (%v), attendu 304", unchanged.Code, unchanged.Body.String(), err)
	}

	var refused *StatusError
	for status, want := range map[int]int{http.StatusUnauthorized: http.StatusForbidden, http.StatusForbidden: http.StatusForbidden,
		http.StatusNotFound: http.StatusNotFound, http.StatusMethodNotAllowed: http.StatusNotFound, http.StatusInternalServerError: http.StatusBadGateway} {
		answer = status
		if err := RelayMetadata(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), peer, "ps3", "jeu"); !errors.As(err, &refused) || refused.Status != want {
			t.Errorf("le pair répond %d : %v, attendu %d", status, err, want)
		}
	}
	silent := Peer{ID: 3, Link: "http://127.0.0.1:1", Token: "jeton"}
	if err := RelayImage(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), silent, "ps3", "jeu", "cover"); !errors.As(err, &refused) || refused.Status != http.StatusBadGateway {
		t.Errorf("un pair éteint : %v, attendu 502", err)
	}
}

// LE RELAIS DE LA FÉDÉRATION (chantier 83) : l'adresse d'un serveur relayé est sous celle du relais, et son
// identifiant, négatif, ne heurte jamais celui d'un pair direct.
func TestARelayedServerLivesUnderItsRelay(t *testing.T) {
	if got := RelayLink("https://ami.example/", 3); got != "https://ami.example/api/v1/federation/relay/3" {
		t.Fatalf("RelayLink = %q", got)
	}
	if got := RelayID(2, 3); got != -2_000_003 {
		t.Fatalf("RelayID = %d", got)
	}
	if RelayID(2, 3) == RelayID(3, 2) {
		t.Fatal("deux serveurs relayés différents ont le même identifiant")
	}
}
