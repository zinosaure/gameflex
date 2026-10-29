package api

import "testing"

// RIEN DEMANDÉ VEUT DIRE « LE CATALOGUE ». C'était la seule portée jusqu'au
// chantier 9 : un appelant qui ne dit rien demande celle-là, et les jetons
// créés avant ce paramètre gardent exactement ce qu'ils ouvraient.
func TestNoScopeAskedMeansCatalog(t *testing.T) {
	granted, err := parseScopes(nil)
	if err != nil || granted != ScopeCatalog {
		t.Fatalf("parseScopes(nil) = %q / %v, attendu %q", granted, err, ScopeCatalog)
	}
}

// UNE PORTÉE INCONNUE EST REFUSÉE, jamais ignorée. Ignorer une faute de
// frappe créerait un jeton qui ouvre MOINS que ce que son créateur croit, et
// rien ne le lui dirait avant que l'autre ne se plaigne.
func TestUnknownScopeIsRefused(t *testing.T) {
	if _, err := parseScopes([]string{ScopeCatalog, "play"}); err == nil {
		t.Fatal("une portée inventée a été acceptée")
	}
}

// L'ordre est STABLE et les doublons disparaissent : la chaîne stockée est
// comparée segment par segment côté kernel, et deux jetons accordant la même
// chose doivent se lire pareil.
func TestScopesAreDeduplicatedAndOrdered(t *testing.T) {
	granted, err := parseScopes([]string{ScopeDevices, ScopeCatalog, ScopeDevices, "  "})
	if err != nil {
		t.Fatalf("parseScopes : %v", err)
	}
	if granted != ScopeCatalog+","+ScopeDevices {
		t.Fatalf("portées %q, attendu %q", granted, ScopeCatalog+","+ScopeDevices)
	}
}

// LES CONSOLES SE DEMANDENT SEULES. Prêter sa bibliothèque et montrer son
// salon sont deux décisions ; rien n'oblige à prendre la première pour avoir
// la seconde.
func TestDevicesCanBeGrantedAlone(t *testing.T) {
	granted, err := parseScopes([]string{ScopeDevices})
	if err != nil || granted != ScopeDevices {
		t.Fatalf("parseScopes([devices]) = %q / %v, attendu %q", granted, err, ScopeDevices)
	}
}
