package session

import (
	"testing"
	"time"
)

// UNE VIBRATION S'ADRESSE, ELLE NE SE DIFFUSE PAS.
//
// `PortForViewer` répondait « quel siège tient ce viewer ? » ; il manquait la
// question inverse, celle du retour de force : « quel viewer tient ce siège ? »
// Sans elle, le child ne savait pas à qui pousser la vibration, et la manette
// ne vibrait pas sur le téléphone (2026-09-10).
func TestViewerForSlotIsTheInverseOfPortForViewer(t *testing.T) {
	host := NewHTTPHost(Config{SessionID: "test", Owner: "gino", MaxPlayers: 1})
	host.Host.Claim("gino", 1, time.Now())

	// PERSONNE NE TIENT LE SIÈGE : il n'y a rien à adresser, et rendre un
	// viewer au hasard ferait vibrer un spectateur.
	if viewer := host.ViewerForSlot(1); viewer != "" {
		t.Fatalf("ViewerForSlot sur un siège libre = %q, attendu vide", viewer)
	}

	// Un viewer connu, mais qui ne joue pas : toujours rien.
	host.viewers.Store("viewer_regarde", "socket_regarde")
	if viewer := host.ViewerForSlot(1); viewer != "" {
		t.Fatalf("ViewerForSlot avec un simple spectateur = %q, attendu vide", viewer)
	}

	// Et le détenteur, lui, se retrouve — c'est toute l'utilité.
	// Un siège ne se prend que depuis un onglet ouvert (invariant 6).
	host.Host.Session().OpenTab("socket_joue", "gino")
	if _, err := host.Host.Session().Join("gino", "socket_joue", time.Now()); err != nil {
		t.Fatalf("Join: %v", err)
	}
	host.viewers.Store("viewer_joue", "socket_joue")
	if viewer := host.ViewerForSlot(1); viewer != "viewer_joue" {
		t.Fatalf("ViewerForSlot = %q, attendu \"viewer_joue\"", viewer)
	}
}
