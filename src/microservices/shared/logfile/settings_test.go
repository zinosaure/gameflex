package logfile

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// appliquer pose un réglage le temps d'un test, et rend le défaut après : le
// réglage est celui du processus, les autres tests ne doivent pas en hériter.
func appliquer(t *testing.T, settings Settings) {
	t.Helper()
	Apply(settings)
	t.Cleanup(func() { Apply(DefaultSettings()) })
}

func kernelLog(t *testing.T, root string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "logs", day(), "kernel.log"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	return string(content)
}

// LES 18 MO DU 2026-11-14 : le journal d'accès, 99,6 % du fichier. Par défaut,
// les requêtes ne vont plus dans le fichier — et la sortie standard les montre
// toujours, comme avant.
func TestHTTPLinesStayOutOfTheFileButNotOutOfTheConsole(t *testing.T) {
	appliquer(t, DefaultSettings())
	root := t.TempDir()
	var console bytes.Buffer
	logger := slog.New(ServiceHandler(&console, root, "kernel"))

	logger.Info(HTTPMessage, "path", "/api/v1/health", "status", 200)
	logger.Info("sessions service listening", "port", "8080")

	written := kernelLog(t, root)
	if strings.Contains(written, "/api/v1/health") || !strings.Contains(written, "sessions service listening") {
		t.Fatalf("le fichier, requêtes éteintes :\n%s", written)
	}
	if !strings.Contains(console.String(), "/api/v1/health") || !strings.Contains(console.String(), "sessions service listening") {
		t.Fatalf("la sortie standard a perdu des lignes :\n%s", console.String())
	}

	// Allumées, elles s'écrivent même sans Info coché : un interrupteur à part.
	Apply(Settings{Kernel: true, Levels: []string{"error"}, HTTP: true})
	logger.Info(HTTPMessage, "path", "/api/v1/sessions", "status", 200)
	logger.Info("un événement", "niveau", "info")
	written = kernelLog(t, root)
	if !strings.Contains(written, "/api/v1/sessions") || strings.Contains(written, "un événement") {
		t.Fatalf("requêtes allumées, Info décoché :\n%s", written)
	}
}

// SEULS LES NIVEAUX COCHÉS vont au fichier. Debug coché s'y écrit sans changer
// la sortie standard, qui reste au niveau Info.
func TestOnlyTheCheckedLevelsReachTheFile(t *testing.T) {
	appliquer(t, Settings{Kernel: true, Levels: []string{"debug", "error"}})
	root := t.TempDir()
	var console bytes.Buffer
	logger := slog.New(ServiceHandler(&console, root, "kernel"))

	logger.Debug("détail")
	logger.Info("information")
	logger.Warn("avertissement")
	logger.Error("échec")

	written := kernelLog(t, root)
	for _, line := range []string{"détail", "échec"} {
		if !strings.Contains(written, line) {
			t.Errorf("le fichier n'a pas %q :\n%s", line, written)
		}
	}
	for _, line := range []string{"information", "avertissement"} {
		if strings.Contains(written, line) {
			t.Errorf("le fichier a %q, décoché :\n%s", line, written)
		}
	}
	if strings.Contains(console.String(), "détail") || !strings.Contains(console.String(), "information") {
		t.Fatalf("la sortie standard a changé :\n%s", console.String())
	}
}

// LE RÉGLAGE VAUT À LA LIGNE SUIVANTE, sans redémarrer : le kernel éteint
// n'ajoute plus rien ; rallumé, il reprend dans le même fichier.
func TestTheSettingAppliesFromTheNextLine(t *testing.T) {
	appliquer(t, DefaultSettings())
	root := t.TempDir()
	logger := slog.New(ServiceHandler(io.Discard, root, "kernel"))

	logger.Info("avant")
	Apply(Settings{Kernel: false, Levels: []string{"info"}})
	logger.Info("éteint")
	Apply(DefaultSettings())
	logger.Info("rallumé")

	written := kernelLog(t, root)
	if !strings.Contains(written, "avant") || strings.Contains(written, "éteint") || !strings.Contains(written, "rallumé") {
		t.Fatalf("le fichier :\n%s", written)
	}
}

// SUPPRIMÉ PENDANT QU'IL S'ÉCRIT (éteint, supprimé pour faire de la place,
// rallumé ; ou supprimé de la page) : le journal renaît à sa ligne suivante,
// au lieu d'écrire dans un fichier que plus rien ne montre.
func TestADeletedLogIsRecreatedByItsNextLine(t *testing.T) {
	appliquer(t, DefaultSettings())
	root := t.TempDir()
	logger := slog.New(ServiceHandler(io.Discard, root, "kernel"))

	logger.Info("première")
	if err := Remove(root, filepath.ToSlash(filepath.Join(day(), "kernel.log"))); err != nil {
		t.Fatal(err)
	}
	logger.Info("seconde")

	written := kernelLog(t, root)
	if strings.Contains(written, "première") || !strings.Contains(written, "seconde") {
		t.Fatalf("après suppression :\n%q", written)
	}
}

// UNE PARTIE suit le réglage à chaque écriture : commencée éteinte, son
// fichier naît quand on rallume ; éteinte en route, il s'arrête là.
func TestASessionFollowsTheSettingWhileItRuns(t *testing.T) {
	appliquer(t, Settings{Kernel: true, Levels: []string{"info"}, Sessions: false})
	root := t.TempDir()
	session := Session(root, "retro-nes-001")
	defer session.Close()

	session.Write([]byte("éteint\n"))
	if found, _ := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*.log")); len(found) != 0 {
		t.Fatalf("un fichier, parties éteintes : %q", found)
	}

	Apply(Settings{Kernel: true, Levels: []string{"info"}, Sessions: true})
	session.Write([]byte("allumé\n"))
	Apply(Settings{Kernel: true, Levels: []string{"info"}, Sessions: false})
	session.Write([]byte("éteint encore\n"))

	found, _ := filepath.Glob(filepath.Join(root, "logs", "*", "*", "retro-nes-001.log"))
	if len(found) != 1 {
		t.Fatalf("fichiers : %q", found)
	}
	content, _ := os.ReadFile(found[0])
	if string(content) != "allumé\n" {
		t.Fatalf("le journal de la partie : %q", content)
	}
}

// Les niveaux, chacun une fois, dans l'ordre ; un niveau inconnu refusé.
func TestNormalizedOrdersTheLevelsAndRefusesAnUnknownOne(t *testing.T) {
	normalized, err := Settings{Kernel: true, Levels: []string{"error", "debug", "error"}}.Normalized()
	if err != nil || !slices.Equal(normalized.Levels, []string{"debug", "error"}) {
		t.Fatalf("normalisé : %v / %v", normalized.Levels, err)
	}
	if _, err := (Settings{Levels: []string{"verbose"}}).Normalized(); err == nil {
		t.Fatal("un niveau inconnu passe")
	}
	if normalized, _ := (Settings{}).Normalized(); normalized.Levels == nil {
		t.Fatal("aucun niveau : une liste vide, pas nil (la page lit un tableau)")
	}
}

// LES SIGNALEMENTS ONT LEUR INTERRUPTEUR (le porteur, 2026-11-14 : « Oui pour
// le reports.log ») : éteints, Reports refuse (ErrReportsOff) et aucun fichier
// n'apparaît ; rallumés, il s'ouvre.
func TestReportsFollowTheirSwitch(t *testing.T) {
	off := DefaultSettings()
	off.Reports = false
	appliquer(t, off)
	root := t.TempDir()

	file, err := Reports(root)
	if !errors.Is(err, ErrReportsOff) || file != nil {
		t.Fatalf("éteints : %v, %v", file, err)
	}
	if _, err := os.Stat(filepath.Join(root, "logs", ReportsFile)); !os.IsNotExist(err) {
		t.Fatalf("un fichier, signalements éteints : %v", err)
	}

	Apply(DefaultSettings())
	file, err = Reports(root)
	if err != nil || file == nil {
		t.Fatalf("allumés : %v, %v", file, err)
	}
	file.Close()
}
