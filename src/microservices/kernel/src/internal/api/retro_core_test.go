package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
)

// coreArchive — ce que sert le buildbot : un .zip qui contient la bibliothèque.
func coreArchive(t *testing.T, name string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("\x7fELF bibliothèque")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

// UN LANCEMENT NE TÉLÉCHARGE RIEN (#143, décidé le 2026-09-17).
//
// Ce test affirmait l'inverse — qu'un cœur absent se descendait au lancement.
// C'est ce comportement qui a rendu GameCube et Wii injouables : `latest` du
// buildbot avait dépassé la libstdc++ de l'image de session, et le binaire
// arrivait pendant une partie. Le cœur manquant doit maintenant produire une
// erreur qui dit où l'installer, et le buildbot ne doit **jamais** être
// contacté par ce chemin — c'est la seule chose que ce test garde de l'ancien :
// un faux buildbot qui compte ses visites, et qui doit en compter zéro.
func TestRequireCoreNeverDownloadsAndReportsWhatIsMissing(t *testing.T) {
	var mutex sync.Mutex
	var requested []string
	buildbot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		requested = append(requested, path.Base(r.URL.Path))
		mutex.Unlock()
		_, _ = w.Write(coreArchive(t, path.Base(r.URL.Path)))
	}))
	defer buildbot.Close()
	requests := func() []string {
		mutex.Lock()
		defer mutex.Unlock()

		return append([]string(nil), requested...)
	}

	previous := coreBuildbotBase
	coreBuildbotBase = buildbot.URL + "/"
	defer func() { coreBuildbotBase = previous }()

	root := t.TempDir()
	nesCore := consoles.CoresFor("nes")[0]
	nesFile := retro_files.CoreFileName(nesCore)

	// Absent : une erreur, et rien n'est écrit.
	err := requireCore(root, "NES", "")
	if err == nil {
		t.Fatal("un cœur absent doit être signalé, pas silencieusement téléchargé")
	}
	if _, statErr := os.Stat(filepath.Join(root, "nes", nesFile)); statErr == nil {
		t.Error("rien ne doit être installé par un lancement")
	}

	// Présent sous sa console : rien à dire.
	if writeErr := os.WriteFile(filepath.Join(root, "nes", nesFile), []byte("cœur"), 0o644); writeErr != nil {
		if mkErr := os.MkdirAll(filepath.Join(root, "nes"), 0o755); mkErr != nil {
			t.Fatal(mkErr)
		}
		if writeErr = os.WriteFile(filepath.Join(root, "nes", nesFile), []byte("cœur"), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := requireCore(root, "nes", nesCore); err != nil {
		t.Errorf("un cœur rangé sous sa console doit passer : %v", err)
	}

	// Posé à plat pour une autre console : le runtime le trouve, donc rien non plus.
	var otherConsole, otherCore string
	for _, console := range consoles.All() {
		if cores := consoles.CoresFor(console.ID); console.ID != "nes" && len(cores) > 0 {
			otherConsole, otherCore = console.ID, cores[0]
			break
		}
	}
	if otherConsole == "" {
		t.Fatal("aucune autre console avec un cœur dans le registre")
	}
	if err := os.WriteFile(filepath.Join(root, retro_files.CoreFileName(otherCore)), []byte("à plat"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireCore(root, otherConsole, ""); err != nil {
		t.Errorf("un cœur posé à plat doit passer : %v", err)
	}

	// Pas déclaré pour cette console : refusé, son nom viendrait de la requête
	// jusqu'au chemin que le runtime charge (revue de sécurité, 2026-10-28).
	for _, core := range []string{"pas-un-coeur-du-registre", "../../saves/marc/x", "/etc/x"} {
		if err := requireCore(root, "nes", core); err == nil {
			t.Errorf("le cœur %q, hors registre, est passé", core)
		}
	}

	if got := requests(); len(got) != 0 {
		t.Errorf("le buildbot a été contacté par un lancement : %v", got)
	}
}

// UN ÉMULATEUR EN CONTENEUR NE SE RÉCLAME PAS COMME UN FICHIER.
//
// Le registre déclare « RPCS3 (streamex) », « xemu (streamex) »… pour six
// consoles. Ce sont des émulateurs qui tournent dans leur propre conteneur, pas
// des bibliothèques libretro : en exiger le `.so` ferait refuser le lancement
// d'une console parfaitement installée. `ListCoreFiles` les écarte déjà sur le
// même signe — l'espace dans le nom — et ce test tient les deux côtés
// d'accord.
func TestRequireCoreIgnoresContainerisedEmulators(t *testing.T) {
	var consoleID, core string
	for _, console := range consoles.All() {
		if cores := consoles.CoresFor(console.ID); len(cores) > 0 && strings.Contains(cores[0], " ") {
			consoleID, core = console.ID, cores[0]
			break
		}
	}
	if consoleID == "" {
		t.Skip("le registre ne déclare plus d'émulateur en conteneur")
	}

	// Un dossier de cœurs VIDE : si la garde manquait, l'absence du fichier
	// ferait échouer le lancement.
	if err := requireCore(t.TempDir(), consoleID, core); err != nil {
		t.Errorf("%s (%s) ne doit pas réclamer de fichier : %v", consoleID, core, err)
	}
}

// Les versions se comparent NOMBRE PAR NOMBRE : « 3.4.9 » est antérieur à
// « 3.4.30 », alors que l'ordre alphabétique dit l'inverse — et c'est
// exactement l'écart qui décide si un cœur est refusé.
func TestIsOlderVersionComparesNumbers(t *testing.T) {
	cases := []struct {
		left, right string
		older       bool
	}{
		{"3.4.9", "3.4.30", true},
		{"3.4.30", "3.4.9", false},
		{"3.4.30", "3.4.30", false},
		{"3.4.30", "3.4.32", true},
		{"3.4.32", "3.4.30", false},
		{"3.4", "3.4.1", true},
	}

	for _, test := range cases {
		if got := isOlderVersion(test.left, test.right); got != test.older {
			t.Errorf("isOlderVersion(%q, %q) = %v, attendu %v",
				test.left, test.right, got, test.older)
		}
	}
}

// Un fichier qui n'est pas un ELF lisible ne peut pas être JUGÉ : le
// vérificateur le laisse passer en le signalant, au lieu de transformer
// « je n'ai pas pu mesurer » en « c'est mauvais ».
func TestVerifyCoreABIAcceptsWhatItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "faux_libretro.so")
	if err := os.WriteFile(path, []byte("\x7fELF pas vraiment"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifyCoreABI(path); err != nil {
		t.Errorf("un ELF illisible doit passer sans être refusé : %v", err)
	}
}

// LE CAS QUI COMPTE VRAIMENT : un VRAI cœur trop récent doit être refusé.
//
// Ignoré par défaut, parce qu'il demande un binaire que le dépôt n'a pas à
// porter (un .so de 16 Mo). Pour le jouer, désigner un cœur du buildbot, et si
// l'on veut le témoin, un cœur qui marche :
//
//	curl -sO https://buildbot.libretro.com/nightly/linux/x86_64/latest/dolphin_libretro.so.zip
//	GF_TEST_CORE_TOO_RECENT=…/dolphin_libretro.so \
//	GF_TEST_CORE_COMPATIBLE=…/cores/wii/dolphin_libretro.so go test ./internal/api/
//
// Mesuré le 2026-09-17 sur les deux vrais binaires : celui du buildbot exige
// GLIBCXX_3.4.32, celui de juillet 3.4.30, et les deux images de session
// fournissent 3.4.30. C'est ce couple qui a rendu GameCube et Wii injouables.
func TestVerifyCoreABIRefusesACoreTooRecent(t *testing.T) {
	path := os.Getenv("GF_TEST_CORE_TOO_RECENT")
	if path == "" {
		t.Skip("GF_TEST_CORE_TOO_RECENT non défini : aucun cœur trop récent sous la main")
	}

	err := verifyCoreABI(path)
	if err == nil {
		t.Fatal("un cœur exigeant une libstdc++ plus récente doit être refusé")
	}
	// La sentinelle, pas seulement un texte : c'est elle qui fait répondre 412
	// plutôt que « la passerelle a échoué », et qui fait afficher le message.
	if !errors.Is(err, errCoreIncompatible) {
		t.Errorf("le refus doit porter errCoreIncompatible : %v", err)
	}
	if !strings.Contains(err.Error(), maxCoreGlibcxx) {
		t.Errorf("le message doit nommer ce que la machine fournit : %v", err)
	}

	// Le témoin : sans lui, un vérificateur qui refuse TOUT passerait ce test.
	if witness := os.Getenv("GF_TEST_CORE_COMPATIBLE"); witness != "" {
		if err := verifyCoreABI(witness); err != nil {
			t.Errorf("le cœur compatible doit passer : %v", err)
		}
	}
}

// LE FIRMWARE EXIGÉ, ET LUI SEUL (#393) : la règle de la page, lue dans le
// listing de ce serveur — un des requis suffit (sans la casse), un requis
// absent est refusé avec ce qui manque, un facultatif n'arrête rien.
func TestRequireBiosFollowsTheRegistry(t *testing.T) {
	previous := listingCacheDir
	listingCacheDir = t.TempDir()
	t.Cleanup(func() { listingCacheDir = previous })

	self := listing.New("server", "ici")
	self.Bios["saturn"] = []listing.Item{{Name: "MPR-17933.BIN"}}
	if err := listing.Save(listingCacheDir, listing.SelfKey, self); err != nil {
		t.Fatal(err)
	}
	server := &Server{}

	if err := server.requireBios("saturn"); err != nil {
		t.Fatalf("Saturn, un de ses requis présent : %v", err)
	}
	if err := server.requireBios("dreamcast"); err == nil || !strings.Contains(err.Error(), "dc_boot.bin") {
		t.Fatalf("Dreamcast sans firmware : %v, attendu un refus qui nomme dc_boot.bin", err)
	}
	if err := server.requireBios("ps1"); err != nil {
		t.Fatalf("PS1, des BIOS facultatifs seulement : %v", err)
	}
}
