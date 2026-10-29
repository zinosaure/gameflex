package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gameflex/consoles"
)

// LE BUG DU 2026-09-10 : la liste des consoles à qui écrire un profil était
// écrite en dur dans ce fichier. La PS5, ajoutée au registre, partait sans
// profil, et sa première partie mourait sur « bind source path does not
// exist: …/profiles/ps5.json ».
func TestEveryStreamexConsoleOfTheRegistryGetsAProfile(t *testing.T) {
	written := Consoles()
	for _, console := range consoles.All() {
		if console.Method != consoles.MethodStreamex {
			continue
		}
		if !slices.Contains(written, console.ID) {
			t.Errorf("console streamex %q du registre sans profil", console.ID)
		}
	}
	// keytest n'est pas une console du registre, mais son profil existe.
	if !slices.Contains(written, "keytest") {
		t.Error("keytest a perdu son profil")
	}
}

func TestWriteAllWritesOneFilePerConsole(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAll(dir); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	for _, console := range Consoles() {
		path := filepath.Join(dir, console+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s absent: %v", console, err)
		}
		var profile struct {
			Console string `json:"console"`
			Buttons []struct {
				ID string `json:"id"`
			} `json:"buttons"`
		}
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("%s: JSON invalide: %v", console, err)
		}
		// Le fichier doit dire de QUELLE console il est : ps4.json annonçait
		// « ps3 », et wiiu.json devait être corrigé à la main.
		if profile.Console != console {
			t.Errorf("%s.json annonce console=%q", console, profile.Console)
		}
		if len(profile.Buttons) == 0 {
			t.Errorf("%s.json n'a aucun bouton — la manette à l'écran se cacherait", console)
		}
	}
}

func TestPS5ReusesTheDualShockButtons(t *testing.T) {
	buttonsOf := func(t *testing.T, console string) []string {
		t.Helper()
		var profile struct {
			Buttons []struct {
				ID string `json:"id"`
			} `json:"buttons"`
		}
		if err := json.Unmarshal([]byte(ForConsole(console)), &profile); err != nil {
			t.Fatalf("%s: %v", console, err)
		}
		ids := make([]string, 0, len(profile.Buttons))
		for _, b := range profile.Buttons {
			ids = append(ids, b.ID)
		}
		return ids
	}

	if got, want := buttonsOf(t, "ps5"), buttonsOf(t, "ps3"); !slices.Equal(got, want) {
		t.Errorf("la PS5 devrait reprendre les boutons de la PS3\n  ps5: %v\n  ps3: %v", got, want)
	}
	if got, want := buttonsOf(t, "wiiu"), buttonsOf(t, "switch"); !slices.Equal(got, want) {
		t.Errorf("la Wii U devrait reprendre les boutons de la Switch\n  wiiu: %v\n  switch: %v", got, want)
	}
	if got, want := buttonsOf(t, "xboxone"), buttonsOf(t, "xbox"); !slices.Equal(got, want) {
		t.Errorf("la Xbox One devrait reprendre les boutons de la Xbox\n  xboxone: %v\n  xbox: %v", got, want)
	}
}

// Un identifiant biscornu ne doit pas produire un JSON illisible : mieux
// vaut un profil non estampillé qu'un fichier que personne ne peut relire.
func TestAnIdWithAQuoteLeavesTheProfileIntact(t *testing.T) {
	got := ForConsole(`ps"5`)
	var probe map[string]any
	if err := json.Unmarshal([]byte(got), &probe); err != nil {
		t.Fatalf("profil illisible produit pour un identifiant à guillemet: %v", err)
	}
}
