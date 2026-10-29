package session

import (
	"testing"
	"time"
)

// ZÉRO VEUT DIRE « JAMAIS », PAS « TOUT DE SUITE ».
//
// Un hôte qui tient l'horloge d'inactivité lui-même pose `IdleAfter: 0`. Sans
// garde, la comparaison `now.Sub(idleSince) > 0` est vraie dès la première
// seconde : toute session était déclarée inactive, et la boucle de balayage
// rendait la main quinze secondes après le démarrage (2026-09-05).
func TestZeroIdleAfterMeansNever(t *testing.T) {
	host := NewHTTPHost(Config{
		SessionID: "device-session", PlatformID: "retrogaming",
		Owner: "gino", MaxPlayers: 2, IdleAfter: 0,
	})

	// Une heure sans personne dans la file : l'horloge est à zéro, donc muette.
	if reason := host.endReason(time.Now().Add(time.Hour)); reason != "" {
		t.Fatalf("IdleAfter=0 a rendu %q, attendu le silence", reason)
	}
}

// Et une vraie durée garde son sens.
func TestPositiveIdleAfterStillEndsTheSession(t *testing.T) {
	host := NewHTTPHost(Config{
		SessionID: "s", PlatformID: "retrogaming",
		Owner: "gino", MaxPlayers: 2, IdleAfter: 5 * time.Minute,
	})

	if reason := host.endReason(time.Now().Add(6 * time.Minute)); reason == "" {
		t.Fatal("IdleAfter=5m n'a rien dit après six minutes")
	}
	if reason := host.endReason(time.Now()); reason != "" {
		t.Fatalf("a parlé trop tôt : %q", reason)
	}
}

// LA FIN D'UNE SESSION N'EST PAS LA FIN DE LA SURVEILLANCE.
//
// La boucle rendait la main après avoir signalé la fin — juste pour un
// conteneur, qui meurt derrière. Sur un hôte qui SURVIT (le téléphone), la
// partie suivante se retrouvait sans personne pour balayer ses sockets morts.
// La première session allait toujours bien, les suivantes non.
func TestTerminatedSessionStillReportsWithoutADemolisher(t *testing.T) {
	host := NewHTTPHost(Config{
		SessionID: "device-session", PlatformID: "retrogaming",
		Owner: "gino", MaxPlayers: 2, IdleAfter: 0,
		// EndSession nil : personne ne démolit derrière.
	})

	host.Host.Submit(Command{Kind: CmdTerminate, Username: "gino"}, time.Now())

	if reason := host.endReason(time.Now()); reason == "" {
		t.Fatal("une session terminée doit se dire terminée")
	}
	// Et la reprendre la remet en service, sans avoir eu à redémarrer le
	// processus — c'est tout l'objet du correctif.
	host.Host.Claim("gino", 2, time.Now())
	if reason := host.endReason(time.Now()); reason != "" {
		t.Fatalf("après reprise : %q, attendu le silence", reason)
	}
}
