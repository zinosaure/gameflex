package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/storage/listing"
)

// askListingUpdated — la question d'un ami : la date du listing.
func askListingUpdated(t *testing.T, server *Server) time.Time {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.handleFederationListingUpdated(recorder, httptest.NewRequest(http.MethodGet, "/listing/updated", nil))
	var answer struct {
		Data struct {
			UpdatedAt time.Time `json:"updatedAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("listing/updated = %d %s", recorder.Code, recorder.Body.String())
	}

	return answer.Data.UpdatedAt
}

// copyListing — ce qu'un ami recopie : le listing servi.
func copyListing(t *testing.T, server *Server) listing.Listing {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.handleFederationListing(recorder, httptest.NewRequest(http.MethodGet, "/listing", nil))
	var served struct {
		Data listing.Listing `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &served); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("listing = %d %s", recorder.Code, recorder.Body.String())
	}

	return served.Data
}

// LA QUESTION DE FRAÎCHEUR SE RÉPOND DE MÉMOIRE (journal 629) : une fois le
// listing et les disques lus, elle ne regarde plus ni les disques, ni le
// fichier du listing. Un disque endormi ne la retient plus au-delà des cinq
// secondes qu'un ami lui donne.
func TestListingUpdatedAnswersFromMemory(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	disk := testDisk(t, "gamebox", true, "nes/Zelda.nes")
	withDisks(server, disk)

	copied := copyListing(t, server)
	if len(copied.Roms["nes"]) != 2 {
		t.Fatalf("listing servi = %+v, attendu Mario et Zelda", copied.Roms["nes"])
	}
	first := askListingUpdated(t, server)
	if !first.Equal(copied.Header.UpdatedAt) {
		t.Fatalf("date = %v, attendu celle du listing servi %v", first, copied.Header.UpdatedAt)
	}

	// Le disque dort, le listing est hors d'atteinte : la question ne touche
	// ni à l'un, ni à l'autre.
	server.internalDisks = func(context.Context) []docker.InternalDisk {
		t.Error("la question a regardé les disques")

		return []docker.InternalDisk{disk}
	}
	if err := os.RemoveAll(listingCacheDir); err != nil {
		t.Fatal(err)
	}
	if again := askListingUpdated(t, server); !again.Equal(first) {
		t.Fatalf("date = %v, attendu %v", again, first)
	}
	if _, err := os.Stat(listingCacheDir); err == nil {
		t.Fatal("la question a refait le listing")
	}
}

// CE QUI CHANGE LE LISTING FAIT AVANCER LA DATE (journal 629) : une liste du
// disque qui voit un jeu déposé à la main, « Relire », une écriture du listing,
// un disque parti. Et le listing servi porte toujours la date que rend la
// question : un ami qui vient de le recopier ne le recopie pas une seconde fois.
func TestWhatChangesTheListingAdvancesItsDate(t *testing.T) {
	server, root := listingServer(t, "roms/nes/Mario.nes")
	disk := testDisk(t, "gamebox", true, "nes/Zelda.nes")
	withDisks(server, disk)
	copyListing(t, server)
	date := askListingUpdated(t, server)
	advanced := func(what string) {
		t.Helper()
		next := askListingUpdated(t, server)
		if !next.After(date) {
			t.Fatalf("%s : date %v, attendu après %v", what, next, date)
		}
		if served := copyListing(t, server); !served.Header.UpdatedAt.Equal(next) {
			t.Fatalf("%s : listing servi daté %v, la question dit %v", what, served.Header.UpdatedAt, next)
		}
		date = next
	}

	// Un jeu déposé à la main sur le disque, qu'une liste de ce disque voit.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(disk.Dir, "roms", "nes", "Metroid.nes"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/{consoleId}/roms", server.handleRetroRoms)
	router.Post("/disks/{name}/rescan", server.handleInternalDiskRescan)
	if listed := call(router, http.MethodGet, "/nes/roms?disk=gamebox", nil); listed.Code != http.StatusOK {
		t.Fatalf("liste du disque = %d", listed.Code)
	}
	advanced("un jeu vu sur le disque")

	// « Relire » le disque.
	time.Sleep(20 * time.Millisecond)
	if rescanned := call(router, http.MethodPost, "/disks/gamebox/rescan", nil); rescanned.Code != http.StatusOK {
		t.Fatalf("relire = %d %s", rescanned.Code, rescanned.Body.String())
	}
	advanced("relire le disque")

	// Une écriture du listing de ce serveur (un import).
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(root, "roms", "nes", "Kirby.nes"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	server.refreshSelf(listing.Roms)
	advanced("un jeu importé")

	// Le disque parti, vu par une liste.
	time.Sleep(20 * time.Millisecond)
	withDisks(server)
	server.presentDisks(context.Background())
	advanced("un disque parti")
	if names := copyListing(t, server).Roms["nes"]; len(names) != 2 {
		t.Fatalf("après le départ du disque : %+v, attendu Mario et Kirby", names)
	}
}
