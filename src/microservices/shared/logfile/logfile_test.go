package logfile

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// UN SERVICE A LE JOURNAL DE SA JOURNÉE. Il tourne en continu ; le découper
// par redémarrage éparpillerait une enquête sur plusieurs fichiers.
func TestServiceLandsUnderTheDay(t *testing.T) {
	racine := t.TempDir()
	slog.New(ServiceHandler(io.Discard, racine, "kernel")).Info("démarrage")

	trouvés, _ := filepath.Glob(filepath.Join(racine, "logs", "*", "kernel.log"))
	attendu := regexp.MustCompile(`logs[/\\]\d{8}[/\\]kernel\.log$`)
	if len(trouvés) != 1 || !attendu.MatchString(trouvés[0]) {
		t.Fatalf("fichiers %q, attendu …/logs/AAAAMMJJ/kernel.log", trouvés)
	}
}

// UNE SESSION A L'HEURE DE SON DÉMARRAGE, et c'est ce qui la sépare de la
// précédente : Docker RÉUTILISE les noms de conteneurs. Deux parties de NES
// dans la journée s'appellent toutes les deux `retro-nes-001`.
func TestSessionLandsUnderItsStartTime(t *testing.T) {
	racine := t.TempDir()
	session := Session(racine, "retro-nes-001")
	defer session.Close()
	session.Write([]byte("sortie de l'émulateur\n"))

	trouvés, _ := filepath.Glob(filepath.Join(racine, "logs", "*", "*", "retro-nes-001.log"))
	attendu := regexp.MustCompile(`logs[/\\]\d{8}[/\\]\d{6}[/\\]retro-nes-001\.log$`)
	if len(trouvés) != 1 || !attendu.MatchString(trouvés[0]) {
		t.Fatalf("fichiers %q, attendu …/logs/AAAAMMJJ/HHMMSS/retro-nes-001.log", trouvés)
	}
}

// Le journal d'un service reste LE MÊME d'un redémarrage à l'autre, et il
// s'y ajoute. Perdre ce qui précède un redémarrage serait perdre exactement
// ce qu'on cherche quand on enquête dessus.
func TestServiceReopensTheSameFileAndAppends(t *testing.T) {
	racine := t.TempDir()

	slog.New(ServiceHandler(io.Discard, racine, "kernel")).Info("avant")
	slog.New(ServiceHandler(io.Discard, racine, "kernel")).Info("après")

	trouvés, _ := filepath.Glob(filepath.Join(racine, "logs", "*", "kernel.log"))
	if len(trouvés) != 1 {
		t.Fatalf("fichiers : %q", trouvés)
	}
	contenu, err := os.ReadFile(trouvés[0])
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if !strings.Contains(string(contenu), "avant") || !strings.Contains(string(contenu), "après") {
		t.Fatalf("le fichier a été écrasé : %q", contenu)
	}
}

// Services et sessions du même jour se retrouvent SOUS LE MÊME JOUR — on
// lit l'incident en descendant d'un cran, pas en cherchant ailleurs.
func TestServiceAndSessionShareTheDay(t *testing.T) {
	racine := t.TempDir()

	slog.New(ServiceHandler(io.Discard, racine, "kernel")).Info("démarrage")
	session := Session(racine, "retro-nes-001")
	defer session.Close()
	session.Write([]byte("sortie\n"))

	service, _ := filepath.Glob(filepath.Join(racine, "logs", "*", "kernel.log"))
	partie, _ := filepath.Glob(filepath.Join(racine, "logs", "*", "*", "retro-nes-001.log"))
	if len(service) != 1 || len(partie) != 1 {
		t.Fatalf("ouverture manquée : %q %q", service, partie)
	}
	if filepath.Dir(filepath.Dir(partie[0])) != filepath.Dir(service[0]) {
		t.Fatalf("la session n'est pas sous le jour du service :\n  %s\n  %s", service[0], partie[0])
	}
}

// Éteints, ni fichier ni erreur — et la sortie standard reçoit toujours tout.
// Un journal ne doit jamais empêcher un service de démarrer.
func TestSwitchedOffOpensNothingAndFailsNothing(t *testing.T) {
	appliquer(t, Settings{})
	racine := t.TempDir()
	var console bytes.Buffer

	slog.New(ServiceHandler(&console, racine, "kernel")).Info("démarrage")
	session := Session(racine, "retro-nes-001")
	if written, err := session.Write([]byte("sortie\n")); written != 7 || err != nil {
		t.Fatalf("écriture éteinte : %d / %v", written, err)
	}
	session.Close()

	if _, err := os.Stat(filepath.Join(racine, "logs")); !os.IsNotExist(err) {
		t.Fatalf("un dossier de journaux a été créé : %v", err)
	}
	if !strings.Contains(console.String(), "démarrage") {
		t.Fatalf("la sortie standard a perdu sa ligne : %q", console.String())
	}
}

// Sans racine non plus — un service dont le stockage n'est pas monté continue.
func TestNoRootOpensNothing(t *testing.T) {
	var console bytes.Buffer
	slog.New(ServiceHandler(&console, "", "kernel")).Info("démarrage")
	if !strings.Contains(console.String(), "démarrage") {
		t.Fatalf("sans racine, la sortie standard : %q", console.String())
	}
}
