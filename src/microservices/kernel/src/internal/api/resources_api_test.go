package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gameflex/kernel/internal/retro_files"
	"gameflex/resources"
	"gameflex/storage"
	"gameflex/storage/listing"

	"github.com/go-chi/chi/v5"
)

// shareInMemory — un partage en mémoire, pour les ressources externes d'un test.
type shareInMemory struct {
	name  string
	files map[string]string
}

func (m *shareInMemory) Name() string { return m.name }

func (m *shareInMemory) List(dir string) ([]storage.Entry, error) {
	dir = strings.Trim(dir, "/")
	seen := map[string]storage.Entry{}
	for file, content := range m.files {
		rel := file
		if dir != "" {
			if !strings.HasPrefix(file, dir+"/") {
				continue
			}
			rel = strings.TrimPrefix(file, dir+"/")
		}
		first, _, nested := strings.Cut(rel, "/")
		if nested {
			seen[first] = storage.Entry{Name: first, Dir: true}
		} else {
			seen[first] = storage.Entry{Name: first, Size: int64(len(content))}
		}
	}
	if len(seen) == 0 && dir != "" {
		return nil, os.ErrNotExist
	}
	entries := make([]storage.Entry, 0, len(seen))
	for _, e := range seen {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries, nil
}

func (m *shareInMemory) Open(p string) (io.ReadCloser, error) {
	content, ok := m.files[strings.Trim(p, "/")]
	if !ok {
		return nil, os.ErrNotExist
	}

	return io.NopCloser(strings.NewReader(content)), nil
}

// withShare — une ressource externe en mémoire sur ce serveur, parcourue.
func withShare(t *testing.T, server *Server, share *shareInMemory) {
	t.Helper()
	store, catalog, err := server.externalResources()
	if err != nil {
		t.Fatal(err)
	}
	catalog.OpenSource = func(r resources.Resource) (storage.Source, error) {
		if r.Name != share.name {
			return nil, errors.New("partage injoignable")
		}

		return share, nil
	}
	id, err := store.AddResource(resources.Resource{Name: share.name, Kind: "smb", Address: "smb://nas/jeux"})
	if err != nil {
		t.Fatal(err)
	}
	added := resources.Resource{ID: id, Name: share.name, Kind: "smb", Address: "smb://nas/jeux"}
	catalog.Rescan(added)
	deadline := time.Now().Add(5 * time.Second)
	for state := catalog.States()[id]; state.Scanning || state.ScannedAt.IsZero(); state = catalog.States()[id] {
		if time.Now().After(deadline) {
			t.Fatal("parcours sans fin")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LES RESSOURCES EXTERNES DERRIÈRE LA BIBLIOTHÈQUE (`#808`) : un ami voit ce qu'elles seules portent, avec leurs
// sources ; un jeu que la bibliothèque a déjà n'est pas repris.
func TestAFriendSeesTheResourcesBehindTheLibrary(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server)
	withShare(t, server, &shareInMemory{name: "NAS", files: map[string]string{
		"roms/nes/Mario.nes": "MARIO",
		"roms/nes/Zelda.nes": "ZELDA",
	}})

	served := listing.New(listing.SelfSource, "ici")
	served.SetSection(listing.Roms, map[string][]listing.Item{
		"nes": {{ID: storage.RomID("nes", "Mario.nes"), Name: "Mario"}},
	}, served.Header.UpdatedAt)
	server.withResources(&served)

	byName := map[string]listing.Item{}
	for _, item := range served.Roms["nes"] {
		if _, twice := byName[item.Name]; twice {
			t.Fatalf("%s listé deux fois", item.Name)
		}
		byName[item.Name] = item
	}
	if len(byName) != 2 {
		t.Fatalf("nes : %+v, attendu Mario et Zelda", served.Roms["nes"])
	}
	if len(byName["Mario"].Sources) != 0 {
		t.Fatalf("Mario vient de la bibliothèque, pas d'une ressource : %+v", byName["Mario"].Sources)
	}
	if sources := byName["Zelda"].Sources; len(sources) != 1 || sources[0].Name != "NAS" {
		t.Fatalf("Zelda : %+v", sources)
	}
	if served.Header.Sections[listing.Roms].Consoles["nes"] != 2 {
		t.Fatalf("compteur : %+v", served.Header.Sections[listing.Roms].Consoles)
	}
	// Zelda est prêté sans être lancé ici : compté à part, pour que la bibliothèque de l'ami le retire.
	if lent := served.Header.Sections[listing.Roms].Lent; lent["nes"] != 1 {
		t.Fatalf("prêtés : %+v, attendu nes 1", lent)
	}
	server.ensureSelf()
	server.rememberSelfListing()
	if at, known := server.listingUpdatedAt(); !known || server.resourcesUpdatedAt().IsZero() || at.Before(server.resourcesUpdatedAt()) {
		t.Fatalf("la date du listing (%v) doit suivre celle des ressources (%v)", at, server.resourcesUpdatedAt())
	}
}

// UN AMI TÉLÉCHARGE UN JEU QU'UNE RESSOURCE SEULE PORTE, par la route habituelle ; un jeu que personne n'a reste un 404.
func TestAFriendDownloadsAGameOnlyAResourceHas(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server)
	withShare(t, server, &shareInMemory{name: "NAS", files: map[string]string{"roms/nes/Zelda.nes": "ZELDA"}})

	router := chi.NewRouter()
	router.Get("/roms/{consoleId}/{ref}", server.handleFederationRom)
	get := func(ref string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/roms/nes/"+ref, nil))

		return recorder
	}

	if got := get(storage.RomID("nes", "Zelda.nes")); got.Code != http.StatusOK || got.Body.String() != "ZELDA" {
		t.Fatalf("Zelda : %d %q", got.Code, got.Body.String())
	}
	if got := get(storage.RomID("nes", "Metroid.nes")); got.Code != http.StatusNotFound {
		t.Fatalf("Metroid : %d, attendu 404", got.Code)
	}
}

// UN JEU D'UNE RESSOURCE IMPORTÉ DANS LA BIBLIOTHÈQUE (Réglages › Consoles › Jeux, la ressource comme source) : un
// fichier copié tel quel, un dossier zippé par la ressource et déplié ici ; la liste de la ressource le dit ensuite
// « déjà ici », et un second import est refusé.
func TestAResourceGameIsImportedIntoTheLibrary(t *testing.T) {
	server, root := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server)
	withShare(t, server, &shareInMemory{name: "NAS", files: map[string]string{
		"roms/nes/Zelda.nes":     "ZELDA",
		"roms/psp/Patapon/a.iso": "ISO",
		"roms/psp/Patapon/b.bin": "BIN",
	}})
	_, catalog, _ := server.externalResources()
	views, _ := catalog.Views()
	id := strconv.FormatInt(views[0].ID, 10)

	list := func(consoleID string) []retro_files.RomEntry {
		recorder := httptest.NewRecorder()
		server.serveResourceRoms(recorder, httptest.NewRequest(http.MethodGet, "/", nil), id, consoleID, "")
		var answer struct {
			Data []retro_files.RomEntry `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatalf("%s : %v (%s)", consoleID, err, recorder.Body.String())
		}

		return answer.Data
	}
	router := chi.NewRouter()
	router.Post("/resources/{id}/storage/{section}", server.handleResourceStorageImport)
	importGame := func(rom retro_files.RomEntry, consoleID string) int {
		body, _ := json.Marshal(map[string]any{"consoleId": consoleID, "reference": rom.Reference, "title": rom.Title, "files": rom.Files, "folder": rom.Folder})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/resources/"+id+"/storage/roms", strings.NewReader(string(body))))
		if recorder.Code != http.StatusOK {
			t.Logf("import de %s : %d %s", rom.Title, recorder.Code, recorder.Body.String())
		}

		return recorder.Code
	}

	nes := list("nes")
	if len(nes) != 1 || nes[0].Title != "Zelda" || nes[0].Here {
		t.Fatalf("nes : %+v", nes)
	}
	if code := importGame(nes[0], "nes"); code != http.StatusOK {
		t.Fatalf("Zelda : %d", code)
	}
	if content, err := os.ReadFile(filepath.Join(root, "roms", "nes", "Zelda.nes")); err != nil || string(content) != "ZELDA" {
		t.Fatalf("Zelda dans la bibliothèque : %q, %v", content, err)
	}

	psp := list("psp")
	if len(psp) != 1 || !psp[0].Folder {
		t.Fatalf("psp : %+v", psp)
	}
	if code := importGame(psp[0], "psp"); code != http.StatusOK {
		t.Fatalf("Patapon : %d", code)
	}
	for file, want := range map[string]string{"a.iso": "ISO", "b.bin": "BIN"} {
		if content, err := os.ReadFile(filepath.Join(root, "roms", "psp", "Patapon", file)); err != nil || string(content) != want {
			t.Fatalf("Patapon/%s : %q, %v", file, content, err)
		}
	}

	_ = retro_files.RescanCatalog(server.retroRomsRoot())
	if again := list("nes"); len(again) != 1 || !again[0].Here {
		t.Fatalf("Zelda devrait être « déjà ici » : %+v", again)
	}
	if code := importGame(nes[0], "nes"); code != http.StatusConflict {
		t.Fatalf("second import de Zelda : %d, attendu 409", code)
	}
}

// LE CATALOGUE TÉLÉCHARGE UN JEU D'UNE RESSOURCE par sa référence `res<id>:` (le ticket du navigateur passe par
// serveRomFile) ; une ressource qui ne l'a pas répond 404, comme une référence d'une ressource inconnue.
func TestTheCatalogDownloadsAResourceGameByItsReference(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server)
	withShare(t, server, &shareInMemory{name: "NAS", files: map[string]string{"roms/nes/Zelda.nes": "ZELDA"}})
	_, catalog, _ := server.externalResources()
	views, _ := catalog.Views()
	id := views[0].ID
	get := func(reference string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.serveRomFile(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "nes", reference)

		return recorder
	}
	if got := get(resources.Reference(id, storage.RomID("nes", "Zelda.nes"))); got.Code != http.StatusOK || got.Body.String() != "ZELDA" {
		t.Fatalf("Zelda : %d %q", got.Code, got.Body.String())
	}
	for _, reference := range []string{resources.Reference(id, storage.RomID("nes", "Metroid.nes")), resources.Reference(id+7, storage.RomID("nes", "Zelda.nes"))} {
		if got := get(reference); got.Code != http.StatusNotFound {
			t.Fatalf("%s : %d, attendu 404", reference, got.Code)
		}
	}
	lent, _ := catalog.LentList()
	if len(lent) != 1 || lent[0].Consoles["nes"] != 1 {
		t.Fatalf("liste du Catalogue : %+v", lent)
	}
}
