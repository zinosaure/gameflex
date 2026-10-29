package logfile

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// UN SERVICE A LE JOURNAL DE SA JOURNÉE. Il tourne en continu ; le découper
// par redémarrage éparpillerait une enquête sur plusieurs fichiers.
func TestServiceLandsUnderTheDay(t *testing.T) {
	file, err := Service(t.TempDir(), "kernel")
	if err != nil || file == nil {
		t.Fatalf("Service : %v / %v", file, err)
	}
	defer file.Close()

	attendu := regexp.MustCompile(`logs[/\\]\d{8}[/\\]kernel\.log$`)
	if !attendu.MatchString(file.Name()) {
		t.Fatalf("chemin %q, attendu …/logs/AAAAMMJJ/kernel.log", file.Name())
	}
}

// UNE SESSION A L'HEURE DE SON DÉMARRAGE, et c'est ce qui la sépare de la
// précédente : Docker RÉUTILISE les noms de conteneurs. Deux parties de NES
// dans la journée s'appellent toutes les deux `retro-nes-001`.
func TestSessionLandsUnderItsStartTime(t *testing.T) {
	file, err := Session(t.TempDir(), "retro-nes-001")
	if err != nil || file == nil {
		t.Fatalf("Session : %v / %v", file, err)
	}
	defer file.Close()

	attendu := regexp.MustCompile(`logs[/\\]\d{8}[/\\]\d{6}[/\\]retro-nes-001\.log$`)
	if !attendu.MatchString(file.Name()) {
		t.Fatalf("chemin %q, attendu …/logs/AAAAMMJJ/HHMMSS/retro-nes-001.log", file.Name())
	}
}

// Le journal d'un service reste LE MÊME d'un redémarrage à l'autre, et il
// s'y ajoute. Perdre ce qui précède un redémarrage serait perdre exactement
// ce qu'on cherche quand on enquête dessus.
func TestServiceReopensTheSameFileAndAppends(t *testing.T) {
	racine := t.TempDir()

	premier, _ := Service(racine, "kernel")
	premier.WriteString("avant\n")
	premier.Close()

	second, _ := Service(racine, "kernel")
	second.WriteString("après\n")
	second.Close()

	if premier.Name() != second.Name() {
		t.Fatalf("deux fichiers :\n  %s\n  %s", premier.Name(), second.Name())
	}

	contenu, err := os.ReadFile(second.Name())
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

	service, _ := Service(racine, "kernel")
	session, _ := Session(racine, "retro-nes-001")
	if service == nil || session == nil {
		t.Fatal("ouverture manquée")
	}
	defer service.Close()
	defer session.Close()

	jour := filepath.Dir(service.Name())
	if filepath.Dir(filepath.Dir(session.Name())) != jour {
		t.Fatalf("la session n'est pas sous le jour du service :\n  %s\n  %s",
			service.Name(), session.Name())
	}
}

// Éteint, on ne rend rien — et surtout pas une erreur. Un journal ne doit
// jamais empêcher un service de démarrer.
func TestDisabledOpensNothingAndFailsNothing(t *testing.T) {
	t.Setenv(EnvDisable, "1")

	for nom, ouvrir := range map[string]func(string, string) (*os.File, error){
		"Service": Service,
		"Session": Session,
	} {
		file, err := ouvrir(t.TempDir(), "kernel")
		if file != nil || err != nil {
			t.Fatalf("%s éteint : %v / %v", nom, file, err)
		}
	}
}

// Sans racine non plus — un service dont le stockage n'est pas monté continue.
func TestNoRootOpensNothing(t *testing.T) {
	file, err := Service("", "kernel")
	if file != nil || err != nil {
		t.Fatalf("sans racine : %v / %v", file, err)
	}
}
