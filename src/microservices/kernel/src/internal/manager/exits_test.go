package manager

import (
	"context"
	"testing"

	"gameflex/kernel/internal/docker"
)

func bootManager() *Manager {
	return &Manager{boots: map[string]chan docker.BootReport{}}
}

// Ce que le child pousse arrive au lancement qui l'attend, et à lui seul (#751).
func TestReportBootReachesTheWaitingLaunch(t *testing.T) {
	manager := bootManager()
	reports := manager.expectBoot("abc")

	if !manager.ReportBoot("abc", docker.BootReport{State: "ready"}) {
		t.Fatal("un lancement attend : le rapport doit lui être remis")
	}
	if got := <-reports; got.State != "ready" {
		t.Fatalf("reçu %+v", got)
	}
	if manager.ReportBoot("autre", docker.BootReport{State: "ready"}) {
		t.Error("aucun lancement n'attend cette partie : le rapport ne doit aller nulle part")
	}

	manager.forgetBoot("abc")
	if manager.ReportBoot("abc", docker.BootReport{State: "ready"}) {
		t.Error("un lancement fini n'attend plus rien")
	}
}

// Une boîte pleine d'avancées ne perd jamais le « ready » : il remplace la plus ancienne, sans bloquer.
func TestReportBootNeverDropsReady(t *testing.T) {
	manager := bootManager()
	reports := manager.expectBoot("abc")
	for range cap(reports) + 3 {
		manager.ReportBoot("abc", docker.BootReport{State: "booting"})
	}
	manager.ReportBoot("abc", docker.BootReport{State: "ready"})

	sawReady := false
	for len(reports) > 0 {
		if (<-reports).State == "ready" {
			sawReady = true
		}
	}
	if !sawReady {
		t.Fatal("le « ready » s'est perdu derrière des avancées")
	}
}

// Sans Docker, la sortie est déjà dite : l'arrêt n'attend rien.
func TestWaitExitedWithoutDockerIsImmediate(t *testing.T) {
	var manager *Manager
	select {
	case <-manager.WaitExited(context.Background(), "retro-nes-001"):
	default:
		t.Fatal("sans Docker, WaitExited doit rendre un signal déjà donné")
	}
}

// Un arrêt demandé et la sortie que Docker annonce peuvent tomber ensemble : un seul des deux prend la partie.
func TestTakeHandsARecordToOneCallerOnly(t *testing.T) {
	store := newSessionStore[RetroSession, *RetroSession]()
	store.register(&RetroSession{BaseSession: BaseSession{SessionID: "abc", Container: "retro-nes-001"}})

	if _, ok := store.take("abc"); !ok {
		t.Fatal("la première prise doit trouver la partie")
	}
	if _, ok := store.take("abc"); ok {
		t.Fatal("la seconde prise ne doit plus rien trouver")
	}
	if _, ok := store.lookup("abc"); ok {
		t.Fatal("la partie prise reste dans la liste")
	}
}
