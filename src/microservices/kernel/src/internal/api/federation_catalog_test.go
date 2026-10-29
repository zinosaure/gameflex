package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
)

// fakePeer — un serveur fédéré : son listing, sa date, et ses jeux. `answer`
// dit ce qu'il répond : 200, ou 403 quand il refuse notre jeton.
type fakePeer struct {
	*httptest.Server

	mu       sync.Mutex
	answer   int
	served   listing.Listing
	listings int
	dates    int
}

func newFakePeer(t *testing.T, hostname string, roms map[string][]listing.Item, files map[string]string) *fakePeer {
	t.Helper()
	served := listing.New(listing.SelfSource, hostname)
	served.Header.Scopes = []string{"catalog"}
	served.SetSection(listing.Roms, roms, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
	served.SetSection(listing.Bios, map[string][]listing.Item{"ps1": {{Name: "scph5501.bin", Size: 524288}}}, served.Header.UpdatedAt)
	peer := &fakePeer{answer: http.StatusOK, served: served}

	peer.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer.mu.Lock()
		answer := peer.answer
		peer.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer jeton" || answer != http.StatusOK {
			w.WriteHeader(firstStatus(answer, http.StatusUnauthorized))

			return
		}
		switch r.URL.Path {
		case "/api/v1/federation/listing":
			peer.mu.Lock()
			peer.listings++
			peer.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": peer.served})

			return
		case "/api/v1/federation/listing/updated":
			peer.mu.Lock()
			peer.dates++
			peer.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"updatedAt": peer.served.Header.UpdatedAt, "scopes": peer.served.Header.Scopes,
			}})

			return
		}
		// LE GESTE D'UN TÉLÉPHONE : `http.ServeFile`, sans nom de fichier.
		for path, content := range files {
			if r.URL.Path == "/api/v1/federation/roms/"+path {
				_, _ = io.WriteString(w, content)

				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(peer.Close)

	return peer
}

func firstStatus(answer, fallback int) int {
	if answer != http.StatusOK {
		return answer
	}

	return fallback
}

func (peer *fakePeer) refuse(status int) {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	peer.answer = status
}

// serverWithPeer — un serveur qui a CE pair pour ami (l'identifiant 3), et un
// cache de listings à lui.
func serverWithPeer(t *testing.T, peer *fakePeer) *Server {
	t.Helper()
	freshListing(t)
	server := &Server{}
	server.serverRegistry.items = []registeredServer{
		{ID: 3, Link: peer.URL, Token: "jeton", Hostname: "ami", Identity: "moi"},
	}
	server.serverRegistry.fetchedAt = time.Now()

	return server
}

// LA LISTE D'UN PAIR, POUR UNE CONSOLE (chantier 14), lue dans la copie de son
// listing (chantier 15) : ses jeux seulement, préfixés à son nom, triés par
// titre, et la recherche y mord comme ici. Trois listes ne le dérangent
// qu'une fois.
func TestLesJeuxDUnPairSeListentConsoleParConsole(t *testing.T) {
	peer := newFakePeer(t, "ami", map[string][]listing.Item{
		"nes":  {{ID: "uuid-z", Name: "Zelda", Files: []string{"Zelda.nes"}}, {ID: "uuid-m", Name: "Metroid"}},
		"snes": {{ID: "uuid-w", Name: "Mario World"}},
	}, nil)
	server := serverWithPeer(t, peer)

	list := func(serverID, query string) (int, []retro_files.RomEntry) {
		response := httptest.NewRecorder()
		server.servePeerRoms(response, serverID, "nes", query)
		var payload struct {
			Data []retro_files.RomEntry `json:"data"`
		}
		_ = json.NewDecoder(response.Body).Decode(&payload)

		return response.Code, payload.Data
	}

	code, roms := list("3", "")
	if code != http.StatusOK || len(roms) != 2 {
		t.Fatalf("liste = %d, %d jeu(x) : %+v", code, len(roms), roms)
	}
	if roms[0].Title != "Metroid" || roms[1].Title != "Zelda" {
		t.Errorf("ordre = %q, %q : attendu par titre", roms[0].Title, roms[1].Title)
	}
	if roms[1].Reference != "srv3:uuid-z" || roms[1].Kind != "server" || roms[1].ServerID != 3 || roms[1].Source != "ami" ||
		roms[1].Identity != "moi" || roms[1].Files[0] != "Zelda.nes" {
		t.Errorf("entrée = %+v : la référence doit être préfixée et l'origine dite", roms[1])
	}

	if _, found := list("3", "zel"); len(found) != 1 || found[0].Title != "Zelda" {
		t.Errorf("recherche « zel » = %+v", found)
	}
	if code, _ := list("9", ""); code != http.StatusNotFound {
		t.Errorf("un serveur inconnu = %d, attendu 404", code)
	}
	if peer.listings != 1 || peer.dates != 1 {
		t.Errorf("%d listings et %d dates pour trois listes, attendu 1 et 1 : la copie est gardée", peer.listings, peer.dates)
	}
}

// UN PAIR QUI REFUSE N'EST PAS UN PAIR ÉTEINT. La page dit « ce serveur
// n'autorise pas l'accès » dans un cas, « il ne répond pas » dans l'autre : une
// liste vide ne dirait ni l'un ni l'autre. L'icône (re-télécharger) voit
// aussitôt un accès accordé entre-temps.
func TestUnRefusSeDitAutrementQuUnPairEteint(t *testing.T) {
	peer := newFakePeer(t, "ami", map[string][]listing.Item{"nes": {{ID: "uuid-z", Name: "Zelda"}}}, nil)
	peer.refuse(http.StatusForbidden)
	server := serverWithPeer(t, peer)
	server.peerCopies.Every = time.Nanosecond

	response := httptest.NewRecorder()
	server.servePeerRoms(response, "3", "nes", "")
	if response.Code != http.StatusForbidden {
		t.Errorf("un pair qui refuse = %d, attendu 403", response.Code)
	}

	peer.refuse(http.StatusOK)
	router := chi.NewRouter()
	router.Post("/servers/{id}/rescan", server.handleRemoteRescan)
	rescan := httptest.NewRecorder()
	router.ServeHTTP(rescan, httptest.NewRequest(http.MethodPost, "/servers/3/rescan", nil))
	if rescan.Code != http.StatusOK {
		t.Fatalf("l'icône, une fois l'accès accordé = %d %s", rescan.Code, rescan.Body.String())
	}
	response = httptest.NewRecorder()
	server.servePeerRoms(response, "3", "nes", "")
	if response.Code != http.StatusOK {
		t.Errorf("le même pair, une fois l'accès accordé = %d, attendu 200", response.Code)
	}

	peer.Close()
	response = httptest.NewRecorder()
	server.servePeerRoms(response, "3", "nes", "")
	if response.Code != http.StatusBadGateway {
		t.Errorf("un pair éteint = %d, attendu 502", response.Code)
	}
}

// LA FICHE D'UN JEU DEMANDE À L'AMI CE QU'IL NOUS OUVRE, À L'INSTANT : une
// portée « Consoles » retirée chez lui se voit aussitôt — la liste des serveurs
// ne demandait rien, et Play restait (relevé par le porteur le 2026-10-13).
// Éteint, il le dit : la fiche refuse alors « ne répond pas ».
func TestLaFicheDemandeCeQueLAmiOuvreALInstant(t *testing.T) {
	peer := newFakePeer(t, "ami", map[string][]listing.Item{"nes": {{ID: "uuid-z", Name: "Zelda"}}}, nil)
	peer.mu.Lock()
	peer.served.Header.Scopes = []string{"catalog", "devices"}
	peer.mu.Unlock()
	server := serverWithPeer(t, peer)
	router := chi.NewRouter()
	router.Get("/servers/{id}", server.handleKnownServer)
	known := func() knownServer {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/servers/3", nil))
		var answer struct {
			Data knownServer `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK {
			t.Fatalf("serveur = %d %s", response.Code, response.Body.String())
		}

		return answer.Data
	}

	if first := known(); first.Listing == nil || len(first.Listing.Scopes) != 2 || first.Online == nil || !*first.Online {
		t.Fatalf("avant : %+v", first)
	}
	peer.mu.Lock()
	peer.served.Header.Scopes = []string{"catalog"}
	peer.mu.Unlock()
	if after := known(); after.Listing == nil || len(after.Listing.Scopes) != 1 {
		t.Fatalf("« Consoles » retirée, la fiche ne le voit pas : %+v", after.Listing)
	}

	// IL A RÉVOQUÉ NOTRE LIEN (chantier 20) : il répond, et le dit.
	peer.refuse(http.StatusUnauthorized)
	if refused := known(); refused.Online == nil || !*refused.Online || !refused.Refused {
		t.Fatalf("un ami qui refuse notre jeton : %+v", refused)
	}
	peer.refuse(http.StatusOK)

	peer.Close()
	if gone := known(); gone.Online == nil || *gone.Online || gone.Refused {
		t.Fatalf("un ami éteint se dit en ligne : %+v", gone)
	}
}

// LES ÉCRANS DE STOCKAGE D'UN AMI LISENT SA COPIE, aux formes de ceux d'ici ;
// et la liste des serveurs dit ce que sa copie dit de lui.
func TestLesEcransDUnAmiLisentSaCopie(t *testing.T) {
	peer := newFakePeer(t, "ami", map[string][]listing.Item{
		"ps1": {{ID: "uuid-ace", Name: "Ace Combat 2", Files: []string{"Ace Combat 2.bin", "Ace Combat 2.cue"}, Dir: true, Size: 9}},
	}, nil)
	server := serverWithPeer(t, peer)
	router := chi.NewRouter()
	router.Get("/servers/{id}/storage/roms", server.handleRemoteStorageRoms)
	router.Get("/servers/{id}/storage/bios", server.handleRemoteStorageBios)
	router.Get("/servers", server.handleKnownServers)
	read := func(path string, into any) {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if err := json.Unmarshal(response.Body.Bytes(), into); err != nil || response.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", path, response.Code, response.Body.String())
		}
	}

	var roms struct {
		Data struct {
			Consoles []struct {
				ConsoleID string `json:"consoleId"`
				Roms      []struct {
					Reference string `json:"reference"`
					Folder    bool   `json:"folder"`
				} `json:"roms"`
			} `json:"consoles"`
		} `json:"data"`
	}
	read("/servers/3/storage/roms", &roms)
	if len(roms.Data.Consoles) != 1 || roms.Data.Consoles[0].ConsoleID != "ps1" ||
		roms.Data.Consoles[0].Roms[0].Reference != "uuid-ace" || !roms.Data.Consoles[0].Roms[0].Folder {
		t.Fatalf("jeux de l'ami = %+v", roms.Data)
	}

	var bios struct {
		Data struct {
			Consoles []consoleSection `json:"consoles"`
		} `json:"data"`
	}
	read("/servers/3/storage/bios", &bios)
	installed := false
	for _, section := range bios.Data.Consoles {
		for _, item := range section.Items {
			if section.ConsoleID == "ps1" && item.Name == "scph5501.bin" {
				installed = item.Installed && item.SizeBytes == 524288
			}
		}
	}
	if !installed {
		t.Fatalf("BIOS de l'ami = %+v", bios.Data.Consoles)
	}

	var known struct {
		Data []knownServer `json:"data"`
	}
	read("/servers", &known)
	if len(known.Data) != 1 || known.Data[0].Listing == nil || len(known.Data[0].Listing.Scopes) != 1 ||
		known.Data[0].Listing.UpdatedAt.IsZero() {
		t.Fatalf("serveurs connus = %+v", known.Data)
	}
}

// UN JEU FÉDÉRÉ SE TÉLÉCHARGE PAR LE KERNEL (chantier 14). Le ticket s'émet
// pour un jeu que la copie du listing du pair annonce, et seulement pour lui ;
// le navigateur reçoit le fichier relayé, sous son nom. Le pair est un
// TÉLÉPHONE : deux jeux y portent le même titre dans deux dossiers (#214), et
// il ne nomme pas ce qu'il sert.
func TestUnJeuFedereSeTelechargeParLeKernel(t *testing.T) {
	peer := newFakePeer(t, "A35", map[string][]listing.Item{
		"nes": {
			{ID: "uuid-metroid", Name: "Metroid", Files: []string{"Metroid.nes"}, Size: 9},
			{ID: "uuid-hack", Name: "Metroid", Files: []string{"Metroid.nes"}, Size: 7},
		},
	}, map[string]string{
		"nes/uuid-hack": "le hack!",
	})
	server := serverWithPeer(t, peer)

	router := chi.NewRouter()
	router.Post("/{consoleId}/roms/{ref}/ticket", server.handleRetroMintDownloadTicket)
	router.Get("/downloads/{ticket}", server.handleRetroDownloadByTicket)
	mint := func(escapedRef string) (int, string) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/nes/roms/"+escapedRef+"/ticket", nil))
		var payload struct {
			Data struct {
				Ticket string `json:"ticket"`
			} `json:"data"`
		}
		_ = json.NewDecoder(response.Body).Decode(&payload)

		return response.Code, payload.Data.Ticket
	}

	// La référence telle que la page l'envoie : `encodeURIComponent`.
	code, ticket := mint("srv3%3Auuid-hack")
	if code != http.StatusOK || ticket == "" {
		t.Fatalf("émission = %d, ticket %q", code, ticket)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/downloads/"+ticket, nil))
	if response.Code != http.StatusOK || response.Body.String() != "le hack!" {
		t.Fatalf("téléchargement = %d %q : attendu le fichier du dossier hacks", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="Metroid.nes"` {
		t.Errorf("Content-Disposition = %q : le nom vient du listing quand le pair n'en pose pas", got)
	}

	if code, _ := mint("srv3%3Auuid-zelda"); code != http.StatusNotFound {
		t.Errorf("un jeu que le pair n'annonce pas = %d, attendu 404", code)
	}
	peer.refuse(http.StatusForbidden)
	server.peerCopies.Every = time.Nanosecond
	if code, _ := mint("srv3%3Auuid-metroid"); code != http.StatusForbidden {
		t.Errorf("un pair qui refuse = %d, attendu 403", code)
	}
}

// LE LISTING DE CE SERVEUR, SERVI À UN AMI (chantier 15) : en un document, avec
// ce que SON jeton lui ouvre ici ; et sa date, seule.
func TestLeListingSeSertAvecCeQueLeJetonOuvre(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Metroid.nes")
	withScopes := func(path string) *http.Request {
		request := httptest.NewRequest(http.MethodGet, path, nil)

		return request.WithContext(context.WithValue(request.Context(), tokenScopesKey{}, "catalog, devices"))
	}

	response := httptest.NewRecorder()
	server.handleFederationListing(response, withScopes("/listing"))
	var served struct {
		Data listing.Listing `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &served); err != nil || response.Code != http.StatusOK {
		t.Fatalf("listing = %d %s", response.Code, response.Body.String())
	}
	if len(served.Data.Header.Scopes) != 2 || served.Data.Header.Scopes[1] != "devices" ||
		len(served.Data.Roms["nes"]) != 1 || served.Data.Roms["nes"][0].Name != "Metroid" {
		t.Fatalf("listing servi = %+v", served.Data)
	}

	response = httptest.NewRecorder()
	server.handleFederationListingUpdated(response, withScopes("/listing/updated"))
	var updated struct {
		Data struct {
			UpdatedAt time.Time `json:"updatedAt"`
			Scopes    []string  `json:"scopes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil ||
		!updated.Data.UpdatedAt.Equal(served.Data.Header.UpdatedAt) || len(updated.Data.Scopes) != 2 {
		t.Fatalf("date = %d %s", response.Code, response.Body.String())
	}
}
