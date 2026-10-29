package gamestatus

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var instant = time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)

func report(status string, emulators ...string) Report {
	return Report{Status: status, Emulators: emulators, By: "root", At: instant}
}

// SANS RIEN : jouable pour une console rétro, non testé pour une console
// récente ; et une console réglée d'un geste fait foi pour tous ses jeux.
func TestUnJeuSansSignalementSuitSaConsole(t *testing.T) {
	if got := Resolve("nes", "", nil).Status; got != Playable {
		t.Fatalf("console rétro sans statut : %q, attendu %q", got, Playable)
	}
	if got := Resolve("ps5", "", nil).Status; got != Untested {
		t.Fatalf("console récente sans statut : %q, attendu %q", got, Untested)
	}
	if got := Resolve("nes", Untested, nil).Status; got != Untested {
		t.Fatalf("console rétro réglée sur non testé : %q", got)
	}
	view := Resolve("nes", Playable, nil)
	if view.Status != Playable || view.Emulator != "" {
		t.Fatalf("console jouable : %+v", view)
	}
	if view.Emulators["fceumm"] != Playable || view.Emulators["nestopia"] != Playable {
		t.Fatalf("chaque émulateur suit la console : %+v", view.Emulators)
	}
}

// UN SIGNALEMENT POUR TOUS (aucun émulateur nommé) l'emporte sur la console :
// un jeu injouable partout ne se dit pas « jouable » parce que sa console l'est.
func TestUnSignalementPourTousLEmporteSurLaConsole(t *testing.T) {
	view := Resolve("nes", Playable, []Report{report(Unplayable)})
	if view.Status != Unplayable {
		t.Fatalf("statut %q, attendu %q", view.Status, Unplayable)
	}
}

// UN ÉMULATEUR NOMMÉ : les autres suivent la console, et le meilleur se dit
// avec le nom de l'émulateur qui le donne.
func TestUnEmulateurNommeLaisseLesAutresALaConsole(t *testing.T) {
	view := Resolve("switch", Playable, []Report{report(Unplayable, "Citron (streamex)")})
	if view.Status != Playable || view.Emulator != "Eden (streamex)" {
		t.Fatalf("attendu jouable par Eden : %+v", view)
	}
	if view.Emulators["Citron (streamex)"] != Unplayable || view.Emulators["Eden (streamex)"] != Playable {
		t.Fatalf("par émulateur : %+v", view.Emulators)
	}
}

// TOUS NOMMÉS : « les autres » ne comptent plus — sinon un jeu injouable sur
// ses deux émulateurs se dirait « non testé ».
func TestTousLesEmulateursNommesNeLaissentPasDeDefaut(t *testing.T) {
	view := Resolve("switch", "", []Report{
		report(Unplayable, "Citron (streamex)"),
		report(Unplayable, "Eden (streamex)"),
	})
	if view.Status != Unplayable || view.Emulator != "" {
		t.Fatalf("attendu injouable, sans nom : %+v", view)
	}
	view = Resolve("switch", "", []Report{
		report(Unplayable, "Citron (streamex)"),
		report(Crashes, "Eden (streamex)"),
	})
	if view.Status != Crashes || view.Emulator != "Eden (streamex)" {
		t.Fatalf("attendu plante en jeu par Eden : %+v", view)
	}
}

// LE PLUS RÉCENT GAGNE : un signalement pour tous efface ce qu'on avait dit
// d'un émulateur.
func TestLeSignalementLePlusRecentGagne(t *testing.T) {
	view := Resolve("switch", "", []Report{
		report(Unplayable, "Citron (streamex)"),
		report(Playable),
	})
	if view.Status != Playable || view.Emulator != "" || view.Emulators["Citron (streamex)"] != Playable {
		t.Fatalf("le signalement pour tous devait tout couvrir : %+v", view)
	}
}

// LES NOTES, de la plus récente à la plus ancienne ; les signalements sans
// note n'en donnent pas.
func TestLesNotesDuPlusRecentAuPlusAncien(t *testing.T) {
	first, second := report(Crashes), report(Playable)
	first.Note, second.Note = "plante au chapitre 2", "corrigé par la mise à jour"
	view := Resolve("ps1", "", []Report{first, report(Unplayable), second})
	if len(view.Notes) != 2 || view.Notes[0].Note != second.Note || view.Notes[1].Note != first.Note {
		t.Fatalf("notes : %+v", view.Notes)
	}
}

// LA VERSION NE VOYAGE PAS : elle est à l'administrateur.
func TestLaVersionNeVoyagePas(t *testing.T) {
	withVersion := report(Playable, "Eden (streamex)")
	withVersion.Versions = map[string]string{"Eden (streamex)": "de7e6e01d4"}
	public := Public([]Report{withVersion})
	if public[0].Versions != nil {
		t.Fatalf("la version est partie : %+v", public[0])
	}
	if withVersion.Versions == nil {
		t.Fatal("Public a modifié l'original")
	}
}

func TestLeFichierDUneConsole(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if console, err := store.Load("nes"); err != nil || console.Status != "" || len(console.Games) != 0 {
		t.Fatalf("fichier absent : %+v, %v", console, err)
	}
	long := report(Crashes)
	long.Note = strings.Repeat("é", MaxNote+20)
	if err := store.Add("nes", "jeu-1", long); err != nil {
		t.Fatal(err)
	}
	if err := store.SetConsole("nes", Playable, "root", instant); err != nil {
		t.Fatal(err)
	}
	console, err := store.Load("nes")
	if err != nil {
		t.Fatal(err)
	}
	if console.Status != Playable || console.By != "root" || console.At == nil {
		t.Fatalf("console : %+v", console)
	}
	if notes := console.Games["jeu-1"]; len(notes) != 1 || len([]rune(notes[0].Note)) != MaxNote {
		t.Fatalf("la note n'a pas été bornée : %+v", notes)
	}
	if statuses := store.Statuses(); statuses["nes"] != Playable || len(statuses) != 1 {
		t.Fatalf("statuts des consoles : %+v", statuses)
	}
}

func TestUnSignalementInvalideEstRefuse(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	for _, attempt := range []func() error{
		func() error { return store.Add("nes", "jeu", report("peut-être")) },
		func() error { return store.Add("nes", "", report(Playable)) },
		func() error { return store.Add("../etc", "jeu", report(Playable)) },
		func() error { return store.SetConsole("nes", "", "root", instant) },
	} {
		if err := attempt(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("attendu ErrInvalid, reçu %v", err)
		}
	}
}

// DEUX SIGNALEMENTS EN MÊME TEMPS ne s'écrasent pas.
func TestLesSignalementsSimultanesSontTousGardes(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := store.Add("snes", "jeu", report(Playable)); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	console, _ := store.Load("snes")
	if got := len(console.Games["jeu"]); got != 20 {
		t.Fatalf("%d signalements gardés sur 20", got)
	}
}
