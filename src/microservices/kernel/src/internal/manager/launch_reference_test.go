package manager

import (
	"testing"

	"gameflex/consoles"
	"gameflex/kernel/internal/catalog"
	"gameflex/kernel/internal/docker"
)

// UNE PARTIE STREAMEX PORTE LA RÉFÉRENCE DE BIBLIOTHÈQUE qu'on a lancée (le
// porteur, 2026-11-14 : « seul le jeu en cours le bouton Play devient
// "Reprendre" ») — un UUID de jeu, jamais un titre —, et la retrouve après un
// redémarrage du kernel, par le label de son conteneur.
func TestAStreamexSessionKeepsItsLibraryReference(t *testing.T) {
	// La référence de la fiche de Crash Bandicoot (Switch, disque Gamebox), un jeu-dossier.
	const reference = "8083d777-3d13-5230-83a8-ae89718d9a46"
	crash := catalog.Game{ID: "313cdbcd", Title: "Crash Bandicoot", Folder: "Crash Bandicoot", RelPath: "Crash Bandicoot/Crash Bandicoot.xci"}
	if got := launchReference("switch", " "+reference+" ", catalog.Game{}); got != reference {
		t.Fatalf("lancé par sa référence : %q", got)
	}
	// La page lance une console récente par le titre : la référence se dérive du jeu trouvé.
	if got := launchReference("switch", "Crash Bandicoot", crash); got != reference {
		t.Fatalf("lancé par son titre : %q, attendu %q", got, reference)
	}
	if got := launchReference(consoles.PortsID, "Majora's Mask", catalog.Game{RelPath: "n64/Majora's Mask.z64"}); got != "" {
		t.Fatalf("un port natif : %q", got)
	}

	adopted := (&Manager{}).sessionFromAdopted(docker.AdoptedSession{SessionID: "s1", Console: "switch", Game: "Crash Bandicoot", Reference: reference})
	if adopted.Reference != reference {
		t.Fatalf("après un redémarrage : %q", adopted.Reference)
	}
}
