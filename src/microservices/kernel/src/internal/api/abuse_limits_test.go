package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gameflex/kernel/internal/registry"
)

// Les bornes posées après la revue de sécurité du kernel (#601) : ce qu'un
// compte connecté ne peut plus faire en boucle.

// LES SIGNALEMENTS : soixante par heure et par compte, puis non — sans gêner
// les autres comptes, et l'heure suivante repart.
func TestReportsArePacedPerAccount(t *testing.T) {
	var budget reportBudget
	start := time.Now()
	for sent := 0; sent < reportsPerHour; sent++ {
		if !budget.allow("marc", start) {
			t.Fatalf("signalement %d refusé", sent+1)
		}
	}
	if budget.allow("marc", start) {
		t.Fatal("un signalement de plus que le plafond est passé")
	}
	if !budget.allow("lea", start) {
		t.Fatal("le plafond de marc a refusé lea")
	}
	if !budget.allow("marc", start.Add(time.Hour+time.Second)) {
		t.Fatal("l'heure suivante, marc reste refusé")
	}
}

// LES SALONS D'ATTENTE : un abonné ouvre au plus launchWaitingRooms salons sur
// des identifiants que le kernel n'a pas annoncés ; au-delà, un flux fermé. Un
// lancement du kernel ouvre toujours le sien.
func TestUnannouncedLaunchRoomsAreBounded(t *testing.T) {
	var progress launchProgress
	for room := 0; room < launchWaitingRooms; room++ {
		if _, _, unsubscribe := progress.subscribe(fmt.Sprintf("invente-%d", room)); unsubscribe == nil {
			t.Fatal("abonnement sans désabonnement")
		}
	}
	channel, replay, _ := progress.subscribe("un-de-trop")
	if _, open := <-channel; open || len(replay) != 0 {
		t.Fatal("un salon d'attente de plus que le plafond a été ouvert")
	}

	progress.publish("vrai-lancement", "session.requested", "nes")
	channel, replay, _ = progress.subscribe("vrai-lancement")
	if len(replay) != 1 || replay[0].Step != "session.requested" {
		t.Fatalf("le lancement du kernel n'a pas son salon : %v", replay)
	}
	select {
	case _, open := <-channel:
		if !open {
			t.Fatal("le salon d'un lancement en cours est fermé")
		}
	default:
	}
}

// LA PLACE SE RÉSERVE : deux lancements simultanés ne passent pas tous les
// deux un plafond d'une place, et la place revient à la fin du premier.
func TestASpawnReservationHoldsThePlace(t *testing.T) {
	server := &Server{pool: registry.NewPool()}
	release, ok := server.reserveSpawn(httptest.NewRecorder(), "remoteplay")
	if !ok {
		t.Fatal("la première réservation est refusée")
	}
	refused := httptest.NewRecorder()
	if _, ok := server.reserveSpawn(refused, "remoteplay"); ok || refused.Code != http.StatusConflict {
		t.Fatalf("un second lancement simultané est passé (%d)", refused.Code)
	}
	release()
	if _, ok := server.reserveSpawn(httptest.NewRecorder(), "remoteplay"); !ok {
		t.Fatal("la place n'est pas revenue après le premier lancement")
	}
}

// UNE ARCHIVE QUI NE TIENT PAS : refusée avant d'écrire un octet.
func TestAnArchiveThatDoesNotFitIsRefused(t *testing.T) {
	directory := t.TempDir()
	if err := fitsOnDisk(directory, 1<<20); err != nil {
		t.Fatalf("un mégaoctet ne tiendrait pas : %v", err)
	}
	if err := fitsOnDisk(directory, 1<<62); err == nil {
		t.Fatal("une archive de quatre exaoctets a été acceptée")
	}
}
