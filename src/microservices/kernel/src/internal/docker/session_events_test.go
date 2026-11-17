package docker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func unpacking(done, total int64) BootReport {
	report := BootReport{State: "booting"}
	report.Unpacking = &struct {
		Done  int64 `json:"done"`
		Total int64 `json:"total"`
	}{done, total}

	return report
}

// Le child dit « prêt » : l'attente finit sur SON message, sans rien relire (#751).
func TestAwaitBootEndsOnThePushedReady(t *testing.T) {
	reports := make(chan BootReport, 1)
	reports <- BootReport{State: "ready"}

	if err := awaitBoot(context.Background(), reports, nil, time.Second, nil); err != nil {
		t.Fatalf("prêt poussé, attente en échec : %v", err)
	}
}

// Un déballage qui avance repousse l'échéance et se montre ; une avancée répétée ne se remontre pas.
func TestAwaitBootUnpackingMovesTheDeadline(t *testing.T) {
	reports := make(chan BootReport)
	shown := []int64{}
	done := make(chan error, 1)
	go func() {
		done <- awaitBoot(context.Background(), reports, nil, 150*time.Millisecond, func(written, _ int64) {
			shown = append(shown, written)
		})
	}()

	// Des relevés espacés de 100 ms : sans prolongation, l'échéance de 150 ms tomberait au second. Le relevé répété
	// (20, deux fois) part juste après l'autre : il ne prolonge rien et ne se montre pas.
	for index, written := range []int64{10, 20, 20, 30} {
		if index != 2 {
			time.Sleep(100 * time.Millisecond)
		}
		reports <- unpacking(written, 100)
	}
	reports <- BootReport{State: "ready"}

	if err := <-done; err != nil {
		t.Fatalf("un déballage qui avance a fini en échec : %v", err)
	}
	if len(shown) != 3 || shown[0] != 10 || shown[1] != 20 || shown[2] != 30 {
		t.Errorf("avancées montrées %v, attendu [10 20 30] (une par octet de plus, jamais deux fois la même)", shown)
	}
}

func TestAwaitBootGivesUpWithoutNews(t *testing.T) {
	err := awaitBoot(context.Background(), make(chan BootReport), nil, 50*time.Millisecond, nil)
	if err == nil {
		t.Fatal("aucun message et aucune avancée : l'attente doit échouer à son échéance")
	}
}

// Docker dit que le conteneur est sorti : l'attente le rend aussitôt.
func TestAwaitBootSeesTheExitDockerReports(t *testing.T) {
	exited := make(chan struct{})
	close(exited)

	err := awaitBoot(context.Background(), make(chan BootReport), exited, time.Minute, nil)
	if !errors.Is(err, ErrChildExited) {
		t.Fatalf("sortie annoncée, rendu %v", err)
	}
}

func TestAwaitBootFollowsTheCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := awaitBoot(ctx, make(chan BootReport), nil, time.Minute, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("lancement annulé, rendu %v", err)
	}
}
