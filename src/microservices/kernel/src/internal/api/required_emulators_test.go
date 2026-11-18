package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/storage/listing"
)

const kytyFile = "KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz"

// requiringKyty — un serveur dont l'image PS5 attend l'archive de Kyty, que
// l'index des paquets streamex `index` offre (vide : l'index du projet).
func requiringKyty(t *testing.T, index string) (*Server, string) {
	t.Helper()
	server, root := listingServer(t)
	server.streamexIndex = index
	server.emulatorsRequired = func(context.Context) []docker.RequiredEmulator {
		return []docker.RequiredEmulator{{
			Console: "ps5", Core: "KytyPS5 (streamex)", Pattern: "KytyPS5-*-Linux-x86_64.tar.gz",
			File: kytyFile, Tested: []string{kytyFile},
		}}
	}

	return server, root
}

// streamexRelease — un faux dépôt de paquets streamex : un paquet PS5 qui porte
// l'archive de Kyty et notre binaire, et son index. `sum` remplace l'empreinte
// annoncée (vide : la vraie).
func streamexRelease(t *testing.T, sum string) *httptest.Server {
	t.Helper()
	var packaged bytes.Buffer
	writer := zip.NewWriter(&packaged)
	for name, content := range map[string]string{kytyFile: "the publisher's archive", "kyty_emulator-gameflex-test": "our build"} {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0o755)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packaged.Bytes())
	if sum == "" {
		sum = hex.EncodeToString(digest[:])
	}
	var release *httptest.Server
	release = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/components.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"format": 1, "packages": []map[string]any{{
				"kind": "server", "console": "ps5", "image": "streamex-ps5", "version": "test",
				"files": []string{kytyFile, "kyty_emulator-gameflex-test"}, "file": "streamex-ps5-test.zip",
				"sizeBytes": packaged.Len(), "sha256": sum, "url": release.URL + "/streamex-ps5-test.zip",
			}}})
		case "/streamex-ps5-test.zip":
			_, _ = w.Write(packaged.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(release.Close)

	return release
}

// UNE CONSOLE RÉCENTE SANS SON ÉMULATEUR NE LANCE RIEN (chantier 29) : l'image
// le déballe au lancement depuis cores/<console>/, et le refus nomme le
// fichier qui manque, au lieu d'un conteneur qui s'arrête au démarrage.
func TestARecentConsoleWithoutItsEmulatorIsRefused(t *testing.T) {
	server, root := requiringKyty(t, "")
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"platform":"streamex","game":"Demo","console_id":"ps5"}`)
	server.handleLaunch(recorder, httptest.NewRequest(http.MethodPost, "/launch", body))
	if recorder.Code != http.StatusPreconditionFailed || !strings.Contains(recorder.Body.String(), kytyFile) {
		t.Fatalf("sans son émulateur : %d %s", recorder.Code, recorder.Body.String())
	}

	place(t, root, kytyFile, time.Now())
	if err := server.requireEmulator(context.Background(), "ps5", ""); err != nil {
		t.Fatalf("avec son émulateur, copié à la main : %v", err)
	}
	// Une image qui n'attend rien — keytest, ou une console dont l'image manque.
	if err := server.requireEmulator(context.Background(), "xbox", ""); err != nil {
		t.Fatalf("sans image qui l'attende : %v", err)
	}
}

// L'ÉCRAN ÉMULATEURS MONTRE LE FICHIER QU'ATTEND L'IMAGE, et l'installe depuis
// le paquet streamex qui le porte (chantier 63) : ses fichiers sous les noms que
// l'image attend, notre binaire avec, exécutable. Un second téléchargement est
// refusé, comme pour un cœur (#143).
func TestTheEmulatorAnImageWantsIsShownAndDownloaded(t *testing.T) {
	release := streamexRelease(t, "")
	server, root := requiringKyty(t, release.URL+"/components.json")

	router := chi.NewRouter()
	router.Get("/storage/cores", server.handleStorageCores)
	router.Post("/storage/cores", server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDownload))
	call := func(method, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/storage/cores", strings.NewReader(body)))

		return response
	}
	kyty := func() (storeItem, int) {
		var listed struct {
			Data struct {
				Consoles []struct {
					ConsoleID string      `json:"consoleId"`
					Items     []storeItem `json:"items"`
				} `json:"consoles"`
			} `json:"data"`
		}
		response := call(http.MethodGet, "")
		if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
			t.Fatalf("liste illisible : %s", response.Body.String())
		}
		found, count := storeItem{}, 0
		for _, console := range listed.Data.Consoles {
			for _, item := range console.Items {
				if console.ConsoleID == "ps5" && item.Name == kytyFile {
					found, count = item, count+1
				}
			}
		}

		return found, count
	}

	if item, count := kyty(); count != 1 || item.Installed || item.Package != "server" || item.Source == "" {
		t.Fatalf("avant : %d ligne(s), %+v — attendu une seule, manquante, avec son adresse", count, item)
	}

	response := call(http.MethodPost, `{"consoleId":"ps5","core":"`+kytyFile+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("téléchargement : %d %s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "cores", "ps5", kytyFile))
	if err != nil || string(content) != "the publisher's archive" {
		t.Fatalf("le fichier rangé : %q %v", content, err)
	}
	if info, err := os.Stat(filepath.Join(root, "cores", "ps5", "kyty_emulator-gameflex-test")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("notre binaire, du même paquet : %v %v", info, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "cores", "ps5", "*.part")); len(leftovers) > 0 {
		t.Fatalf("des fichiers partiels sont restés : %v", leftovers)
	}
	if item, count := kyty(); count != 1 || !item.Installed {
		t.Fatalf("après : %d ligne(s), %+v — attendu une seule, présente", count, item)
	}

	if again := call(http.MethodPost, `{"consoleId":"ps5","core":"`+kytyFile+`"}`); again.Code != http.StatusConflict {
		t.Fatalf("un second téléchargement : %d %s", again.Code, again.Body.String())
	}
}

// UN PAQUET QUI NE CORRESPOND PAS À SON INDEX N'INSTALLE RIEN (chantier 63) :
// l'empreinte se vérifie avant de déballer.
func TestAStreamexPackageThatDoesNotMatchItsIndexInstallsNothing(t *testing.T) {
	release := streamexRelease(t, strings.Repeat("0", 64))
	server, root := requiringKyty(t, release.URL+"/components.json")
	router := chi.NewRouter()
	router.Post("/storage/cores", server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDownload))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/storage/cores", strings.NewReader(`{"consoleId":"ps5","core":"`+kytyFile+`"}`)))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "sha256") {
		t.Fatalf("empreinte fausse : %d %s", response.Code, response.Body.String())
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "cores", "ps5")); len(entries) > 0 {
		t.Fatalf("rien ne devait être déposé : %v", entries)
	}
}

// LA FICHE D'UN JEU GRISE L'ÉMULATEUR dont aucune version n'est là, et nomme
// la version testée.
func TestAGameSheetGreysAnEmulatorWithoutItsFile(t *testing.T) {
	server, root := requiringKyty(t, "")
	// Un paquet du téléphone, rangé au même endroit, n'en est pas une.
	place(t, root, "kyty-2026-10-23+box64-d653d0ebe-forky-r1-x86_64.zip", time.Now())
	entries := server.streamexCoreEntries(context.Background(), "ps5")
	if len(entries) != 1 || entries[0].Installed || entries[0].File != kytyFile {
		t.Fatalf("sans le fichier : %+v", entries)
	}

	place(t, root, kytyFile, time.Now())
	if entries := server.streamexCoreEntries(context.Background(), "ps5"); len(entries) != 1 || !entries[0].Installed {
		t.Fatalf("avec le fichier : %+v", entries)
	}
}

// LE DERNIER DÉPOSÉ L'EMPORTE (le porteur, 2026-10-24), qu'il soit plus récent
// ou plus ancien que la version testée : c'est la date du fichier qui
// départage, comme dans l'image (consoles/unpack-emulator.sh).
func TestTheLastEmulatorAddedIsTheOneUsed(t *testing.T) {
	server, root := requiringKyty(t, "")
	now := time.Now()
	place(t, root, kytyFile, now.Add(-time.Hour))
	place(t, root, "KytyPS5-2026-11-02-0abc123-Linux-x86_64.tar.gz", now)
	if entries := server.streamexCoreEntries(context.Background(), "ps5"); len(entries) != 1 ||
		entries[0].File != "KytyPS5-2026-11-02-0abc123-Linux-x86_64.tar.gz" {
		t.Fatalf("la plus récente déposée : %+v", entries)
	}

	place(t, root, kytyFile, now.Add(time.Hour))
	if entries := server.streamexCoreEntries(context.Background(), "ps5"); entries[0].File != kytyFile {
		t.Fatalf("la testée, redéposée : %+v", entries)
	}
}

// place — un fichier de cores/ps5/, daté de `when`.
func place(t *testing.T, root, name string, when time.Time) {
	t.Helper()
	target := filepath.Join(root, "cores", "ps5", name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, when, when); err != nil {
		t.Fatal(err)
	}
}

// L'ÉCRAN DIT QUELLE VERSION SERT, ET LESQUELLES SONT ÉPROUVÉES : une nightly
// plus récente déposée par-dessus la version testée sert aux parties, marquée
// « non testée » ; la testée reste, marquée comme telle. Un paquet du
// téléphone ne reçoit aucune des deux marques.
func TestTheEmulatorsScreenSaysWhichVersionIsUsedAndWhichAreTested(t *testing.T) {
	const nightly = "KytyPS5-2026-11-02-0abc123-Linux-x86_64.tar.gz"
	const bundle = "kyty-2026-10-23+box64-d653d0ebe-forky-r1-x86_64.zip"
	server, root := listingServer(t, "cores/ps5/"+kytyFile, "cores/ps5/"+nightly, "cores/ps5/"+bundle)
	now := time.Now()
	for name, when := range map[string]time.Time{kytyFile: now.Add(-time.Hour), nightly: now, bundle: now} {
		if err := os.Chtimes(filepath.Join(root, "cores", "ps5", name), when, when); err != nil {
			t.Fatal(err)
		}
	}
	base, _ := requiringKyty(t, "")
	server.emulatorsRequired = base.emulatorsRequired

	rows := map[string]storeItem{}
	for _, section := range server.coresSections(context.Background()) {
		for _, item := range section.Items {
			if section.ConsoleID == "ps5" {
				rows[item.Name] = item
			}
		}
	}
	if row := rows[nightly]; !row.InUse || row.Tested == nil || *row.Tested {
		t.Errorf("la nightly déposée en dernier : %+v — attendu utilisée, non testée", row)
	}
	if row := rows[kytyFile]; row.InUse || row.Tested == nil || !*row.Tested {
		t.Errorf("la version testée : %+v — attendu testée, pas utilisée", row)
	}
	if row := rows[bundle]; row.InUse || row.Tested != nil {
		t.Errorf("le paquet du téléphone : %+v — attendu sans marque", row)
	}
}
