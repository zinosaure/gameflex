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
	granted, err := parseScopes([]string{ScopeMetadataProviders, ScopeCatalog, ScopeMetadataProviders, "  "})
	if err != nil {
		t.Fatalf("parseScopes : %v", err)
	}
	if granted != ScopeCatalog+","+ScopeMetadataProviders {
		t.Fatalf("portées %q, attendu %q", granted, ScopeCatalog+","+ScopeMetadataProviders)
	}
}

// LE CATALOGUE EST TOUJOURS ACCORDÉ (le porteur, 2026-11-16 : « par défaut
// les jeux sont toujours autorisé ») : demander les consoles, ou les
// fournisseurs, seuls donne aussi les jeux. Remplace « les consoles se
// demandent seules ».
func TestTheCatalogIsAlwaysGranted(t *testing.T) {
	for asked, want := range map[string]string{
		ScopeMetadataProviders: ScopeCatalog + "," + ScopeMetadataProviders,
	} {
		if granted, err := parseScopes([]string{asked}); err != nil || granted != want {
			t.Fatalf("parseScopes([%s]) = %q / %v, attendu %q", asked, granted, err, want)
		}
	}
}

// PARTAGER SES FOURNISSEURS DE MÉTADONNÉES EST UNE PORTÉE DE PLUS (chantier 57,
// `#719`) : elle s'accorde avec le catalogue, se range dans l'ordre stable, et
// rien ne la donne sans qu'on la demande.
func TestMetadataProvidersIsGrantedOnlyWhenAsked(t *testing.T) {
	granted, err := parseScopes([]string{ScopeMetadataProviders, ScopeCatalog})
	if err != nil || granted != ScopeCatalog+","+ScopeMetadataProviders {
		t.Fatalf("parseScopes = %q / %v, attendu %q", granted, err, ScopeCatalog+","+ScopeMetadataProviders)
	}
	if granted, _ := parseScopes([]string{ScopeCatalog}); granted != ScopeCatalog {
		t.Fatalf("sans la demander, la portée apparaît : %q", granted)
	}
}

// LA PORTÉE `devices` N'EXISTE PLUS (chantier 66) : les consoles d'un accès la remplacent ; la demander est refusé,
// comme toute portée inconnue.
func TestTheDevicesScopeIsGone(t *testing.T) {
	if _, err := parseScopes([]string{ScopeCatalog, "devices"}); err == nil {
		t.Fatal("la portée devices a été acceptée")
	}
}

// LE RELAIS (chantier 83) : un accès dit s'il peut nous relayer, et les serveurs qu'on relaie à son porteur ; la forme
// rangée garde les identifiants valides, sans doublon.
func TestRelayedServersRoundTrip(t *testing.T) {
	if got := joinRelayed([]int64{3, 0, 7, 3, -1}); got != "3,7" {
		t.Fatalf("joinRelayed = %q, attendu 3,7", got)
	}
	if got := parseRelayed("3, 7,x,,0"); len(got) != 2 || got[0] != 3 || got[1] != 7 {
		t.Fatalf("parseRelayed = %v, attendu [3 7]", got)
	}
	if got := parseRelayed(""); got == nil || len(got) != 0 {
		t.Fatalf("parseRelayed(\"\") = %v, attendu une liste vide", got)
	}
}
