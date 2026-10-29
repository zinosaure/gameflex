package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/storage/listing"
)

// diskWith — un disque interne présent, rangé comme le dossier, avec ces
// fichiers (chemins depuis sa racine : `cores/nes/…`, `bios/ps1/…`).
func diskWith(t *testing.T, name string, files map[string]string) docker.InternalDisk {
	t.Helper()
	dir := t.TempDir()
	for file, content := range files {
		target := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return docker.InternalDisk{Name: name, Dir: dir, HostDir: "/hote/" + name, Present: true}
}

func diskStorageRouter(server *Server) *chi.Mux {
	router := chi.NewRouter()
	router.Get("/disks/{name}/storage/{section}", server.handleDiskStorage)
	router.Post("/disks/{name}/storage/{section}", server.handleDiskStorageImport)

	return router
}

func call(router *chi.Mux, method, path string, body any) *httptest.ResponseRecorder {
	var payload bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, &payload))

	return recorder
}

// installedIn — l'élément `name` de la console `consoleID` est-il présent dans
// cette liste (`{consoles: [{consoleId, items: [{name, installed}]}]}`) ?
func installedIn(t *testing.T, recorder *httptest.ResponseRecorder, consoleID, name string) bool {
	t.Helper()
	var answer struct {
		Data struct {
			Consoles []struct {
				ConsoleID string `json:"consoleId"`
				Items     []struct {
					Name      string `json:"name"`
					Installed bool   `json:"installed"`
				} `json:"items"`
			} `json:"consoles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("réponse illisible : %v — %s", err, recorder.Body.String())
	}
	for _, section := range answer.Data.Consoles {
		if section.ConsoleID != consoleID {
			continue
		}
		for _, item := range section.Items {
			if item.Name == name {
				return item.Installed
			}
		}
	}

	return false
}

// UN DISQUE EST UNE SOURCE pour ses cœurs et ses BIOS (chantier 24) : la même
// liste que pour ce serveur ou un ami, chaque élément dit s'il est sur le
// disque. Une section qui n'en est pas une répond 400, un disque inconnu 404.
func TestADiskListsItsCoresAndItsBios(t *testing.T) {
	server, _ := listingServer(t)
	disk := diskWith(t, "ssd", map[string]string{
		"cores/nes/fceumm_libretro.so": "cœur",
		"bios/ps1/scph5501.bin":        "bios",
	})
	withDisks(server, disk)
	router := diskStorageRouter(server)

	cores := call(router, http.MethodGet, "/disks/ssd/storage/cores", nil)
	if cores.Code != http.StatusOK || !installedIn(t, cores, "nes", "fceumm") {
		t.Fatalf("fceumm absent de la liste du disque : %d %s", cores.Code, cores.Body.String())
	}
	if installedIn(t, cores, "nes", "nestopia") {
		t.Fatal("un cœur que le disque n'a pas est annoncé présent")
	}
	bios := call(router, http.MethodGet, "/disks/ssd/storage/bios", nil)
	if bios.Code != http.StatusOK || !installedIn(t, bios, "ps1", "scph5501.bin") {
		t.Fatalf("scph5501.bin absent de la liste du disque : %d %s", bios.Code, bios.Body.String())
	}
	if code := call(router, http.MethodGet, "/disks/ssd/storage/roms", nil).Code; code != http.StatusBadRequest {
		t.Fatalf("une section inconnue répond %d, attendu 400", code)
	}
	if code := call(router, http.MethodGet, "/disks/inconnu/storage/cores", nil).Code; code != http.StatusNotFound {
		t.Fatalf("un disque inconnu répond %d, attendu 404", code)
	}
}

// IMPORTER D'UN DISQUE, C'EST COPIER DANS LE DOSSIER, et jamais par-dessus ce
// qui existe (#142/#143) : le cœur, avec sa variante Android ; un correctif,
// dossier et sous-dossiers compris, sans brouillon `.part` qui traîne. Ce que
// le disque n'a pas répond 404.
func TestImportingFromADiskCopiesIntoTheFolderAndNeverOverwrites(t *testing.T) {
	server, root := listingServer(t)
	disk := diskWith(t, "ssd", map[string]string{
		"cores/nes/fceumm_libretro.so":                "linux",
		"cores/nes/fceumm_libretro_android.so":        "android",
		"patches/wiiu/NewSuperMarioBrosU/rules.txt":   "règles",
		"patches/wiiu/NewSuperMarioBrosU/fix/pack.gp": "pack",
	})
	withDisks(server, disk)
	router := diskStorageRouter(server)

	imported := call(router, http.MethodPost, "/disks/ssd/storage/cores", map[string]string{"consoleId": "nes", "name": "fceumm"})
	if imported.Code != http.StatusOK {
		t.Fatalf("import du cœur : %d %s", imported.Code, imported.Body.String())
	}
	for file, want := range map[string]string{"fceumm_libretro.so": "linux", "fceumm_libretro_android.so": "android"} {
		got, err := os.ReadFile(filepath.Join(root, "cores", "nes", file))
		if err != nil || string(got) != want {
			t.Fatalf("%s copié : %q, %v — attendu %q", file, got, err, want)
		}
	}
	if code := call(router, http.MethodPost, "/disks/ssd/storage/cores", map[string]string{"consoleId": "nes", "name": "fceumm"}).Code; code != http.StatusConflict {
		t.Fatalf("un cœur déjà là répond %d, attendu 409", code)
	}
	if code := call(router, http.MethodPost, "/disks/ssd/storage/bios", map[string]string{"consoleId": "ps1", "name": "scph5501.bin"}).Code; code != http.StatusNotFound {
		t.Fatalf("un BIOS absent du disque répond %d, attendu 404", code)
	}
	if code := call(router, http.MethodPost, "/disks/ssd/storage/cores", map[string]string{"consoleId": "nes", "name": "../../etc"}).Code; code != http.StatusBadRequest {
		t.Fatalf("un nom hors du registre répond %d, attendu 400", code)
	}

	patch := call(router, http.MethodPost, "/disks/ssd/storage/patches", map[string]string{"consoleId": "wiiu", "name": "NewSuperMarioBrosU"})
	if patch.Code != http.StatusOK {
		t.Fatalf("import du correctif : %d %s", patch.Code, patch.Body.String())
	}
	pack := filepath.Join(paramsRoot, "patches", "wiiu", "NewSuperMarioBrosU")
	if got, err := os.ReadFile(filepath.Join(pack, "fix", "pack.gp")); err != nil || string(got) != "pack" {
		t.Fatalf("sous-dossier du correctif : %q, %v", got, err)
	}
	if _, err := os.Stat(pack + ".part"); !os.IsNotExist(err) {
		t.Fatalf("un brouillon .part est resté : %v", err)
	}
}

// POUR UN AMI, LE DOSSIER ET LES DISQUES NE FONT QU'UN, dans toutes les
// sections (chantier 24) : un cœur et un BIOS qu'un disque est seul à avoir
// apparaissent dans le listing servi, et se téléchargent.
func TestAFriendSeesAndDownloadsWhatOnlyADiskHas(t *testing.T) {
	server, _ := listingServer(t)
	disk := diskWith(t, "ssd", map[string]string{
		"cores/snes/snes9x_libretro.so": "snes9x",
		"bios/ps1/scph5501.bin":         "bios",
	})
	withDisks(server, disk)

	served := listing.New(listing.SelfSource, "test")
	server.withInternalDisks(context.Background(), &served)
	if len(served.Cores["snes"]) != 1 || served.Cores["snes"][0].Name != "snes9x" {
		t.Fatalf("cœurs servis : %+v", served.Cores)
	}
	if len(served.Bios["ps1"]) != 1 || served.Bios["ps1"][0].Name != "scph5501.bin" {
		t.Fatalf("BIOS servis : %+v", served.Bios)
	}
	if served.Header.Sections[listing.Cores].Consoles["snes"] != 1 {
		t.Fatalf("le compte des cœurs ne suit pas : %+v", served.Header.Sections[listing.Cores])
	}

	router := chi.NewRouter()
	router.Get("/cores/{consoleId}/{name}", server.handleFederationCore)
	router.Get("/bios/{consoleId}/{name}", server.handleFederationBios)
	for path, want := range map[string]string{"/cores/snes/snes9x": "snes9x", "/bios/ps1/scph5501.bin": "bios"} {
		recorder := call(router, http.MethodGet, path, nil)
		if recorder.Code != http.StatusOK || recorder.Body.String() != want {
			t.Fatalf("%s : %d %q, attendu %q", path, recorder.Code, recorder.Body.String(), want)
		}
	}
}
