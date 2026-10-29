package api

// LES DEUX VARIANTES D'UN CŒUR — chantier 13, phase 5.
//
// Le bouton « Télécharger » d'Émulateurs prend aussi la version Android
// (`<nom>_libretro_android.so`, arm64), rangée à côté de la version Linux ; les
// téléphones de ce serveur viennent la prendre par la route de fédération,
// `?platform=android`. Ces tests tiennent les règles : chaque variante se
// descend si elle manque, aucune ne s'écrase (#143), une variante Android
// d'une autre architecture est refusée, et la suppression emporte les deux.

import (
	"archive/zip"
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"gameflex/storage/listing"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
)

// elfHeader — l'en-tête d'une bibliothèque ELF 64 bits pour `machine`, sans
// section : assez pour que debug/elf la lise et dise son architecture.
func elfHeader(machine elf.Machine) []byte {
	header := make([]byte, 64)
	copy(header, []byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)})
	binary.LittleEndian.PutUint16(header[16:], uint16(elf.ET_DYN))
	binary.LittleEndian.PutUint16(header[18:], uint16(machine))
	binary.LittleEndian.PutUint32(header[20:], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint16(header[52:], 64)

	return header
}

func zipOf(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

// variantBuildbots — deux faux buildbots, Linux et Android, qui comptent leurs
// visites. `android` nil : la variante Android n'existe pas (404).
func variantBuildbots(t *testing.T, android []byte) (linuxHits, androidHits *atomic.Int32) {
	t.Helper()
	linuxHits, androidHits = &atomic.Int32{}, &atomic.Int32{}
	linux := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		linuxHits.Add(1)
		_, _ = w.Write(zipOf(t, "core_libretro.so", []byte("\x7fELF bibliothèque linux")))
	}))
	t.Cleanup(linux.Close)
	androidBot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		androidHits.Add(1)
		if android == nil {
			http.NotFound(w, r)

			return
		}
		_, _ = w.Write(zipOf(t, "core_libretro_android.so", android))
	}))
	t.Cleanup(androidBot.Close)

	previousLinux, previousAndroid := coreBuildbotBase, coreAndroidBuildbotBase
	coreBuildbotBase, coreAndroidBuildbotBase = linux.URL+"/", androidBot.URL+"/"
	t.Cleanup(func() { coreBuildbotBase, coreAndroidBuildbotBase = previousLinux, previousAndroid })

	return linuxHits, androidHits
}

func coresServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()

	return &Server{storageRoot: root}, filepath.Join(root, "cores")
}

func downloadVariants(t *testing.T, server *Server, consoleID, core string) (int, map[string]any) {
	t.Helper()
	body := strings.NewReader(`{"consoleId":"` + consoleID + `","core":"` + core + `"}`)
	recorder := httptest.NewRecorder()
	server.handleStorageCoreDownload(recorder, httptest.NewRequest(http.MethodPost, "/storage/cores", body))
	var envelope struct {
		Data         map[string]any `json:"data"`
		ErrorMessage string         `json:"errorMessage"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if envelope.Data == nil {
		envelope.Data = map[string]any{"errorMessage": envelope.ErrorMessage}
	}

	return recorder.Code, envelope.Data
}

func TestCoreDownloadTakesBothVariants(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	linuxHits, androidHits := variantBuildbots(t, elfHeader(elf.EM_AARCH64))
	server, cores := coresServer(t)

	status, answer := downloadVariants(t, server, "nes", core)
	if status != http.StatusOK || answer["androidSizeBytes"] == nil {
		t.Fatalf("réponse %d %v, attendu 200 avec les deux variantes", status, answer)
	}
	for _, name := range []string{retro_files.CoreFileName(core), retro_files.AndroidCoreFileName(core)} {
		if _, err := os.Stat(filepath.Join(cores, "nes", name)); err != nil {
			t.Errorf("%s absent après le téléchargement", name)
		}
	}

	// LES DEUX PRÉSENTES : rien ne s'écrase (#143).
	if status, _ := downloadVariants(t, server, "nes", core); status != http.StatusConflict {
		t.Errorf("les deux variantes en place : %d, attendu 409", status)
	}
	if linuxHits.Load() != 1 || androidHits.Load() != 1 {
		t.Errorf("visites linux=%d android=%d, attendu 1 et 1", linuxHits.Load(), androidHits.Load())
	}
}

// UN SERVEUR QUI A DÉJÀ LA VERSION LINUX ne descend que la variante Android, et
// la ligne de l'écran garde la taille du cœur en place.
func TestCoreDownloadFetchesOnlyTheMissingVariant(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	linuxHits, androidHits := variantBuildbots(t, elfHeader(elf.EM_AARCH64))
	server, cores := coresServer(t)
	if err := os.MkdirAll(filepath.Join(cores, "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cores, "nes", retro_files.CoreFileName(core)), []byte("cœur en place"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, answer := downloadVariants(t, server, "nes", core)
	if status != http.StatusOK || answer["sizeBytes"] != float64(len("cœur en place")) {
		t.Fatalf("réponse %d %v", status, answer)
	}
	if linuxHits.Load() != 0 || androidHits.Load() != 1 {
		t.Errorf("visites linux=%d android=%d, attendu 0 et 1", linuxHits.Load(), androidHits.Load())
	}
}

// PAS DE VERSION ANDROID CHEZ LIBRETRO : la version Linux s'installe quand
// même, et la réponse le dit.
func TestCoreDownloadSurvivesAMissingAndroidBuild(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	variantBuildbots(t, nil)
	server, cores := coresServer(t)

	status, answer := downloadVariants(t, server, "nes", core)
	if status != http.StatusOK || !strings.Contains(asString(answer["androidError"]), "android/arm64-v8a") {
		t.Fatalf("réponse %d %v, attendu 200 avec androidError", status, answer)
	}
	if _, err := os.Stat(filepath.Join(cores, "nes", retro_files.CoreFileName(core))); err != nil {
		t.Error("la version Linux devait être installée")
	}
}

// UNE VARIANTE ANDROID QUI N'EST PAS ARM64 EST REFUSÉE — mesurée, pas devinée.
func TestCoreDownloadRefusesAnAndroidVariantOfAnotherArchitecture(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	variantBuildbots(t, elfHeader(elf.EM_X86_64))
	server, cores := coresServer(t)
	if err := os.MkdirAll(filepath.Join(cores, "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cores, "nes", retro_files.CoreFileName(core)), []byte("linux"), 0o644); err != nil {
		t.Fatal(err)
	}

	if status, answer := downloadVariants(t, server, "nes", core); status != http.StatusPreconditionFailed {
		t.Fatalf("réponse %d %v, attendu 412", status, answer)
	}
	if _, err := os.Stat(filepath.Join(cores, "nes", retro_files.AndroidCoreFileName(core))); err == nil {
		t.Error("une variante x86_64 a été installée")
	}
}

// LA SUPPRESSION EMPORTE LES DEUX — sinon les téléphones garderaient
// l'ancienne variante après une mise à jour du cœur.
func TestCoreDeleteRemovesBothVariants(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	server, cores := coresServer(t)
	if err := os.MkdirAll(filepath.Join(cores, "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{retro_files.CoreFileName(core), retro_files.AndroidCoreFileName(core)} {
		if err := os.WriteFile(filepath.Join(cores, "nes", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	remove := func() int {
		recorder := httptest.NewRecorder()
		server.handleStorageCoreDelete(recorder, httptest.NewRequest(http.MethodDelete, "/storage/cores?consoleId=nes&core="+core, nil))

		return recorder.Code
	}
	if code := remove(); code != http.StatusOK {
		t.Fatalf("suppression : %d", code)
	}
	entries, _ := os.ReadDir(filepath.Join(cores, "nes"))
	if len(entries) != 0 {
		t.Errorf("il reste %d fichier(s) après la suppression", len(entries))
	}
	if code := remove(); code != http.StatusNotFound {
		t.Errorf("seconde suppression : %d, attendu 404", code)
	}
}

// LA ROUTE DE FÉDÉRATION SERT LA VARIANTE DEMANDÉE, et la liste dit laquelle
// existe.
func TestFederationServesTheRequestedVariant(t *testing.T) {
	core := consoles.CoresFor("nes")[0]
	server, cores := coresServer(t)
	router := chi.NewRouter()
	router.Get("/cores/{consoleId}/{name}", server.handleFederationCore)
	serve := func(query string) (int, string) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/cores/nes/"+core+query, nil))

		return recorder.Code, recorder.Body.String()
	}

	if code, _ := serve("?platform=android"); code != http.StatusNotFound {
		t.Errorf("variante absente : %d, attendu 404", code)
	}
	if err := os.MkdirAll(filepath.Join(cores, "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		retro_files.CoreFileName(core):        "linux",
		retro_files.AndroidCoreFileName(core): "android",
	} {
		if err := os.WriteFile(filepath.Join(cores, "nes", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code, body := serve("?platform=android"); code != http.StatusOK || body != "android" {
		t.Errorf("?platform=android : %d %q", code, body)
	}
	if code, body := serve(""); code != http.StatusOK || body != "linux" {
		t.Errorf("sans plateforme : %d %q", code, body)
	}

	// Déposés À LA MAIN : le listing ne les voit qu'après un parcours — le
	// bouton de l'écran (chantier 15).
	freshListing(t)
	server.refreshSelf(listing.Cores)
	found := false
	for _, section := range server.coresSections(context.Background()) {
		for _, item := range section.Items {
			if section.ConsoleID == "nes" && item.Name == core {
				found = item.AndroidInstalled != nil && *item.AndroidInstalled &&
					item.AndroidSizeBytes == int64(len("android"))
			}
		}
	}
	if !found {
		t.Error("la liste ne dit pas que la variante Android est là")
	}
}

func asString(value any) string {
	text, _ := value.(string)

	return text
}
