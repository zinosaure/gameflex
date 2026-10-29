package remote_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

const (
	marioID = "4f9523a0-83b6-5141-a5a1-63a2d9b490e9"
	aceID   = "0f7c6d3e-1111-5222-8333-444455556666"
	// accountToken — le jeton du compte de Marc chez le faux GameFlex : lui
	// seul ouvre une session (chantier 20).
	accountToken = "jeton-du-compte-de-marc"
)

// fakePeer — un faux GameFlex qui répond comme le vrai : même enveloppe
// {"data":...}, même exigence de jeton, son listing en un document et sa date
// à part. Pas de réseau réel : ces tests doivent tourner sur une machine de
// compilation sans rien à joindre.
type fakePeer struct {
	*httptest.Server

	mu      sync.Mutex
	listing listing.Listing
	denied  bool
	// served, asked — les listings servis, les dates demandées.
	served, asked int
	// sessions — les sessions échangées ; `alive`, celle qu'il accepte encore.
	sessions int
	alive    string
}

func sampleListing(at time.Time) listing.Listing {
	served := listing.New(listing.SelfSource, "nas-du-salon")
	served.Header.Scopes = []string{"catalog", "devices"}
	served.SetSection(listing.Roms, map[string][]listing.Item{
		"nes": {{ID: marioID, Name: "Super Mario Bros", Files: []string{"Super Mario Bros.nes"}, Size: 40976}},
		"ps1": {{ID: aceID, Name: "Ace Combat 2", Files: []string{"Ace Combat 2.bin", "Ace Combat 2.cue"}, Dir: true, Size: 612345678}},
	}, at)
	served.SetSection(listing.Bios, map[string][]listing.Item{"ps1": {{Name: "scph5501.bin", Size: 524288}}}, at)

	return served
}

func newFakePeer(t *testing.T, token string) *fakePeer {
	t.Helper()
	peer := &fakePeer{listing: sampleListing(t0)}

	mux := http.NewServeMux()
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			peer.mu.Lock()
			denied := peer.denied
			peer.mu.Unlock()
			if denied || r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/api/v1/federation/identity", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"hostname": "nas-du-salon", "identity": "Téléphone de Marc", "scopes": []string{"catalog", "devices"},
		}})
	}))
	mux.HandleFunc("/api/v1/federation/listing", guard(func(w http.ResponseWriter, r *http.Request) {
		peer.mu.Lock()
		peer.served++
		served := peer.listing
		peer.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": served})
	}))
	mux.HandleFunc("/api/v1/federation/listing/updated", guard(func(w http.ResponseWriter, r *http.Request) {
		peer.mu.Lock()
		peer.asked++
		at, scopes := peer.listing.Header.UpdatedAt, peer.listing.Header.Scopes
		peer.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updatedAt": at, "scopes": scopes}})
	}))
	mux.HandleFunc("/api/v1/federation/params/ps3/rpcs3/presets/hd-perso", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"name": "HD perso", "system": "PS3", "values": map[string]any{"resolutionScale": "150"},
		}})
	}))
	mux.HandleFunc("/api/v1/federation/patches/wiiu/NewSuperMarioBrosU", guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PK\x03\x04 le pack"))
	}))
	mux.HandleFunc("/api/v1/federation/roms/nes/"+marioID, guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("NES\x1a le contenu"))
	}))
	// LA SESSION S'OUVRE AVEC LE JETON D'UN COMPTE (chantier 20), jamais avec
	// celui du lien : le catalogue et le jeu sont deux portes.
	mux.HandleFunc("POST /api/v1/federation/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+accountToken {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		peer.mu.Lock()
		peer.sessions++
		peer.alive = fmt.Sprintf("session-%d", peer.sessions)
		session := peer.alive
		peer.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"token": session, "username": "marc", "consoles": []string{"ps3"}, "expiresAt": t0.Add(7 * 24 * time.Hour),
		}})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		peer.mu.Lock()
		alive := peer.alive
		peer.mu.Unlock()
		if alive == "" || r.Header.Get("Authorization") != "Bearer "+alive {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}
		_, _ = w.Write([]byte(`{"item":{"username":"marc","consoles":["ps3","rp-ps5"]}}`))
	})

	peer.Server = httptest.NewServer(mux)
	t.Cleanup(peer.Close)

	return peer
}

func (peer *fakePeer) counts() (served, asked int) {
	peer.mu.Lock()
	defer peer.mu.Unlock()

	return peer.served, peer.asked
}

func TestCheckRapporteLeNomDuServeur(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")

	server, err := remote.New(fake.URL, "bon-jeton")
	if err != nil {
		t.Fatalf("New : %v", err)
	}

	identity, err := server.Check()
	if err != nil {
		t.Fatalf("Check : %v", err)
	}
	if identity.Hostname != "nas-du-salon" {
		t.Errorf("hostname = %q, attendu nas-du-salon", identity.Hostname)
	}
	// Le nom du jeton doit remonter : c'est lui qui s'affiche sous les jeux,
	// et c'est ce que l'utilisateur reconnaîtra pour savoir quel accès sert.
	if identity.Identity != "Téléphone de Marc" {
		t.Errorf("identity = %q, attendu « Téléphone de Marc »", identity.Identity)
	}
	// Et ce qu'il nous ouvre : son catalogue, et la liste de ses consoles.
	if len(identity.Scopes) != 2 || identity.Scopes[1] != "devices" {
		t.Errorf("portées = %v", identity.Scopes)
	}
}

// Un jeton révoqué et un serveur en panne ne se traitent pas pareil : l'un
// se retente, l'autre demande d'aller en chercher un nouveau. D'où une
// erreur nommée plutôt qu'un code HTTP à relire chez l'appelant.
func TestJetonRefuseEstUneErreurNommee(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")

	server, _ := remote.New(fake.URL, "mauvais-jeton")
	if _, err := server.Check(); !errors.Is(err, remote.ErrDenied) {
		t.Fatalf("erreur = %v, attendu ErrDenied", err)
	}
	if _, err := server.Listing(); !errors.Is(err, remote.ErrDenied) {
		t.Fatalf("listing = %v, attendu ErrDenied", err)
	}
}

// Une page qui répond 200 mais qui n'est pas GameFlex (un portail captif,
// la page d'accueil de quelqu'un) doit être refusée à l'ajout, et son
// « listing » aussi.
func TestUnServeurQuiNEnEstPasUnEstRefuse(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer fake.Close()

	server, _ := remote.New(fake.URL, "peu importe")
	if _, err := server.Check(); err == nil {
		t.Fatal("un serveur sans nom a été accepté")
	}
	if _, err := server.Listing(); err == nil {
		t.Fatal("une réponse sans en-tête a été prise pour un listing")
	}
}

// LE LISTING D'UN AMI ARRIVE EN UN DOCUMENT (chantier 15) : ses jeux avec leur
// UUID, leurs fichiers et leur forme, ses BIOS, et ce que notre jeton y ouvre.
func TestLeListingArriveEnUnDocument(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	server, _ := remote.New(fake.URL, "bon-jeton")

	got, err := server.Listing()
	if err != nil {
		t.Fatalf("Listing : %v", err)
	}
	if got.Header.Name != "nas-du-salon" || len(got.Header.Scopes) != 2 || !got.Header.UpdatedAt.Equal(t0) {
		t.Errorf("en-tête = %+v", got.Header)
	}
	mario := got.Roms["nes"]
	if len(mario) != 1 || mario[0].ID != marioID || mario[0].Files[0] != "Super Mario Bros.nes" || mario[0].Dir {
		t.Errorf("un fichier libre : %+v", mario)
	}
	if ace := got.Roms["ps1"]; len(ace) != 1 || !ace[0].Dir || len(ace[0].Files) != 2 {
		t.Errorf("un dossier : %+v", ace)
	}
	if bios := got.Bios["ps1"]; len(bios) != 1 || bios[0].Name != "scph5501.bin" {
		t.Errorf("BIOS = %+v", bios)
	}

	fresh, err := server.ListingUpdated()
	if err != nil || !fresh.UpdatedAt.Equal(t0) || len(fresh.Scopes) != 2 {
		t.Errorf("date = %+v (%v), attendu %v et deux portées", fresh, err, t0)
	}
}

func TestOpenRomRendLeContenu(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	server, _ := remote.New(fake.URL, "bon-jeton")

	body, err := server.OpenRom("nes", marioID)
	if err != nil {
		t.Fatalf("OpenRom : %v", err)
	}
	defer body.Close()

	content, _ := io.ReadAll(body)
	if string(content) != "NES\x1a le contenu" {
		t.Errorf("contenu = %q", content)
	}
}

// CE QU'UN AMI PRÊTE (chantier 15) : un préréglage de ses paramètres, un
// correctif ; ce qu'il ne prête pas se dit — ni refus, ni panne.
func TestParamsEtCorrectifsDUnAmi(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	server, _ := remote.New(fake.URL, "bon-jeton")

	lent, err := server.Params("ps3", "rpcs3", "hd-perso")
	if err != nil || lent.Name != "HD perso" || lent.Values["resolutionScale"] != "150" {
		t.Fatalf("préréglage prêté = %+v (%v)", lent, err)
	}
	if _, err := server.Params("ps3", "rpcs3", ""); !errors.Is(err, remote.ErrNotLent) {
		t.Fatalf("ce qu'il ne prête pas = %v, attendu ErrNotLent", err)
	}
	body, err := server.OpenPatch("wiiu", "NewSuperMarioBrosU")
	if err != nil {
		t.Fatalf("correctif : %v", err)
	}
	content, _ := io.ReadAll(body)
	body.Close()
	if !strings.HasPrefix(string(content), "PK") {
		t.Fatalf("correctif = %q", content)
	}
	if _, err := server.OpenPatch("wiiu", "Absent"); !errors.Is(err, remote.ErrNotLent) {
		t.Fatalf("un correctif absent = %v, attendu ErrNotLent", err)
	}
}

// Une adresse avec une barre oblique finale est la faute de frappe la plus
// banale du monde ; elle ne doit pas produire une URL à double barre.
func TestAdresseNormalisee(t *testing.T) {
	server, err := remote.New("  http://exemple.test/  ", "x")
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if server.Link() != "http://exemple.test" {
		t.Errorf("Link = %q", server.Link())
	}
}

func TestAdresseVideRefusee(t *testing.T) {
	if _, err := remote.New("   ", "x"); !errors.Is(err, remote.ErrNoLink) {
		t.Errorf("erreur = %v, attendu ErrNoLink", err)
	}
}

// LE JETON D'UN COMPTE S'ÉCHANGE CONTRE UNE SESSION (chantier 20), que l'hôte
// juge lui-même ensuite : refusée, elle ne vaut plus, même avant son échéance.
// Le jeton du LIEN n'en obtient pas : il n'ouvre que le catalogue.
func TestUnJetonDeCompteSEchangeContreUneSession(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	server, _ := remote.New(fake.URL, accountToken)

	session, err := server.OpenSession()
	if err != nil || session.Token != "session-1" || session.Username != "marc" || len(session.Consoles) != 1 {
		t.Fatalf("session = %+v (%v)", session, err)
	}
	// Vivante, et ce que son compte lance À L'INSTANT : relu chez lui.
	if now, alive, err := server.SessionNow(session); err != nil || !alive || len(now.Consoles) != 2 {
		t.Fatalf("la session qu'il vient de donner : %v, consoles %v (%v)", alive, now.Consoles, err)
	}
	if _, err := server.OpenSession(); err != nil {
		t.Fatal(err)
	}
	if _, alive, err := server.SessionNow(session); err != nil || alive {
		t.Fatalf("une session qu'il refuse se dit vivante : %v (%v)", alive, err)
	}

	link, _ := remote.New(fake.URL, "bon-jeton")
	if _, err := link.OpenSession(); !errors.Is(err, remote.ErrDenied) {
		t.Fatalf("le jeton du lien = %v, attendu ErrDenied", err)
	}

	fake.Close()
	if _, _, err := server.SessionNow(session); err == nil {
		t.Fatal("un hôte éteint juge la session")
	}
}
