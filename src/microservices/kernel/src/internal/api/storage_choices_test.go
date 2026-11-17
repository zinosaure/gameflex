package api

import (
	"testing"

	"gameflex/storage/listing"
)

// CE QU'UNE LIGNE RÈGLE (chantier 54, P2, #682) : un cœur rétro règle `core`,
// une version d'émulateur de bureau que l'image reconnaît règle
// `emulatorFile` (même celle que l'image ne reconnaît pas : `tested` absent, la
// page grise le geste), un BIOS rétro `bios` ; un paquet du téléphone et un BIOS
// d'une console Streamex ne règlent rien. Le
// téléphone écrit le même champ, et la page n'a plus à le deviner.
func TestEachLineSaysWhatItChooses(t *testing.T) {
	tested := true
	cores := []consoleSection{
		{ConsoleID: "nes", Items: []storeItem{{Name: "fceumm"}}},
		{ConsoleID: "switch", Items: []storeItem{
			{Name: "eden-0.0.4.zip", Package: "server", Tested: &tested},
			{Name: "notes.zip", Package: "server"},
			{Name: "eden-v0.0.4-arm64.zip", Package: "phone"},
		}},
	}
	markChoices(cores, listing.Cores)
	want := map[string]string{"fceumm": "core", "eden-0.0.4.zip": "emulatorFile", "notes.zip": "emulatorFile", "eden-v0.0.4-arm64.zip": ""}
	for _, section := range cores {
		for _, item := range section.Items {
			if item.Choose != want[item.Name] {
				t.Errorf("%s/%s : choose %q, attendu %q", section.ConsoleID, item.Name, item.Choose, want[item.Name])
			}
		}
	}

	bios := []consoleSection{
		{ConsoleID: "ps1", Items: []storeItem{{Name: "scph5501.bin"}}},
		{ConsoleID: "switch", Items: []storeItem{{Name: "prod.keys"}}},
	}
	markChoices(bios, listing.Bios)
	if bios[0].Items[0].Choose != "bios" || bios[1].Items[0].Choose != "" {
		t.Errorf("BIOS : ps1 %q, switch %q, attendu « bios » et rien", bios[0].Items[0].Choose, bios[1].Items[0].Choose)
	}
}
