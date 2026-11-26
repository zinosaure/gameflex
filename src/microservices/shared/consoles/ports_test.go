package consoles

import "testing"

// CHAQUE PORT EST UN CŒUR DE LA CONSOLE `ports`, et lit une console qui a un
// dossier : sinon la fiche ne le propose pas, ou ses jeux ne se trouvent
// nulle part.
func TestEachPortIsACoreOfThePortsConsole(t *testing.T) {
	if !Exists(PortsID) {
		t.Fatalf("la console %q n'est pas au registre", PortsID)
	}
	cores := map[string]bool{}
	for _, core := range CoresFor(PortsID) {
		cores[core] = true
	}
	stored := map[string]bool{}
	for _, console := range Stored() {
		stored[console.ID] = true
	}
	for _, port := range Ports() {
		if !cores[port.Core] {
			t.Errorf("le port %q n'est pas un cœur de %q (%v)", port.Core, PortsID, CoresFor(PortsID))
		}
		if !stored[port.From] {
			t.Errorf("le port %q lit %q, qui n'a pas de dossier", port.Core, port.From)
		}
		if len(port.Titles) == 0 {
			t.Errorf("le port %q n'accepte aucun jeu", port.Core)
		}
	}
	if stored[PortsID] {
		t.Errorf("%q a un dossier à elle : ses jeux doivent vivre chez les consoles qu'elle lit", PortsID)
	}
}

// LA MANETTE D'UN PORT (chantier 72) : celle qu'il déclare — Astro Bot se joue à la DualSense —, sinon celle de la
// console ; une autre console garde la sienne.
func TestAPortPlaysWithThePadItDeclares(t *testing.T) {
	cases := []struct{ console, core, want string }{
		{PortsID, "AstroHybrid (streamex)", "ps5"},
		{PortsID, "", PortsID},
		{"switch", "Eden (streamex)", "switch"},
	}
	for _, c := range cases {
		if got := PadOf(c.console, c.core); got != c.want {
			t.Errorf("PadOf(%q, %q) = %q, want %q", c.console, c.core, got, c.want)
		}
	}
}
