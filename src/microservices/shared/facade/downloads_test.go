package facade

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// blockingJob — un geste qui tient jusqu'à ce qu'on le relâche (`release`), et qui dit quand il part (`started`).
func blockingJob(owner, key string, queued bool) (DownloadJob, chan struct{}, chan struct{}) {
	started, release := make(chan struct{}), make(chan struct{})
	job := DownloadJob{Owner: owner, Key: key, Label: key, Queued: queued, Run: func(ctx context.Context) (int, []byte) {
		close(started)
		select {
		case <-release:
			return 200, OKBody(map[string]string{"name": key})
		case <-ctx.Done():
			return 502, ErrorBody("server_failed", ctx.Err().Error())
		}
	}}

	return job, started, release
}

func waitFor(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s : rien au bout de 3 s", what)
	}
}

func notYet(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatalf("%s : parti trop tôt", what)
	case <-time.After(150 * time.Millisecond):
	}
}

// lastStep — l'état final d'un salon, lu jusqu'à la fermeture de son canal.
func lastStep(t *testing.T, downloads *Downloads, id, owner string) DownloadStep {
	t.Helper()
	steps, last, _, known := downloads.Subscribe(id, owner)
	if !known {
		t.Fatalf("salon %s inconnu", id)
	}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case step, open := <-steps:
			if !open {
				return last
			}
			last = step
		case <-deadline:
			t.Fatalf("salon %s pas fini : %+v", id, last)
		}
	}
}

// LA FILE DES JEUX (le porteur, 2026-11-16, 2.4 : « c'est possible de mettre les jeux à télécharger en queue ? ») :
// trois jeux demandés à la suite partent l'un après l'autre, dans l'ordre ; chacun dit son rang tant qu'il attend, et la
// liste de son propriétaire les montre — celui qui tourne, puis les autres dans l'ordre de la file.
func TestQueuedGamesDownloadOneAfterTheOther(t *testing.T) {
	var downloads Downloads
	first, firstStarted, firstRelease := blockingJob("claude", "roms/nes/a", true)
	second, secondStarted, secondRelease := blockingJob("claude", "roms/nes/b", true)
	third, thirdStarted, thirdRelease := blockingJob("claude", "roms/nes/c", true)
	firstID, _ := downloads.Start(first)
	secondID, _ := downloads.Start(second)
	thirdID, _ := downloads.Start(third)

	waitFor(t, firstStarted, "le premier")
	notYet(t, secondStarted, "le deuxième, pendant le premier")
	notYet(t, thirdStarted, "le troisième, pendant le premier")
	listed := downloads.List("claude")
	if len(listed) != 3 || listed[0].DownloadID != firstID || listed[0].State != "running" ||
		listed[1].DownloadID != secondID || listed[1].State != "queued" || listed[1].Position != 1 ||
		listed[2].DownloadID != thirdID || listed[2].Position != 2 {
		t.Fatalf("la liste : %+v", listed)
	}
	if _, waiting, _, _ := downloads.Subscribe(thirdID, "claude"); waiting.Type != "queued" || waiting.Position != 2 {
		t.Fatalf("le rang du troisième : %+v", waiting)
	}
	if len(downloads.List("someone-else")) != 0 {
		t.Fatal("la liste d'un autre compte ne montre pas les miens")
	}

	close(firstRelease)
	waitFor(t, secondStarted, "le deuxième, après le premier")
	notYet(t, thirdStarted, "le troisième, pendant le deuxième")
	if _, waiting, _, _ := downloads.Subscribe(thirdID, "claude"); waiting.Type != "queued" || waiting.Position != 1 {
		t.Fatalf("le troisième passe premier de la file : %+v", waiting)
	}
	close(secondRelease)
	waitFor(t, thirdStarted, "le troisième, après le deuxième")
	close(thirdRelease)
	if step := lastStep(t, &downloads, thirdID, "claude"); step.Type != "done" || !strings.Contains(string(step.Result), "roms/nes/c") {
		t.Fatalf("la fin du troisième : %+v", step)
	}
}

// RETIRER UN JEU DE LA FILE, ANNULER CELUI EN COURS : seul son propriétaire le fait ; retiré, il finit « cancelled »
// sans avoir commencé, et ceux d'après avancent d'un rang ; annulé en cours, le suivant part.
func TestAQueuedGameLeavesTheQueueAndTheRunningOneIsCancelled(t *testing.T) {
	var downloads Downloads
	first, firstStarted, _ := blockingJob("claude", "roms/nes/a", true)
	second, secondStarted, _ := blockingJob("claude", "roms/nes/b", true)
	third, thirdStarted, thirdRelease := blockingJob("claude", "roms/nes/c", true)
	firstID, _ := downloads.Start(first)
	secondID, _ := downloads.Start(second)
	thirdID, _ := downloads.Start(third)
	waitFor(t, firstStarted, "le premier")

	if downloads.Cancel(secondID, "someone-else") {
		t.Fatal("un autre compte ne retire pas mon jeu de la file")
	}
	if !downloads.Cancel(secondID, "claude") {
		t.Fatal("le propriétaire retire son jeu de la file")
	}
	if step := lastStep(t, &downloads, secondID, "claude"); step.Type != "failed" || step.Error != DownloadCancelled {
		t.Fatalf("le jeu retiré : %+v", step)
	}
	if _, waiting, _, _ := downloads.Subscribe(thirdID, "claude"); waiting.Position != 1 {
		t.Fatalf("le troisième avance d'un rang : %+v", waiting)
	}

	if !downloads.Cancel(firstID, "claude") {
		t.Fatal("le propriétaire annule celui en cours")
	}
	if step := lastStep(t, &downloads, firstID, "claude"); step.Type != "failed" || step.Error != DownloadCancelled {
		t.Fatalf("le jeu annulé : %+v", step)
	}
	waitFor(t, thirdStarted, "le troisième, après l'annulation du premier")
	notYet(t, secondStarted, "le jeu retiré de la file")
	close(thirdRelease)
	lastStep(t, &downloads, thirdID, "claude")
	if downloads.Cancel(thirdID, "claude") {
		t.Fatal("un téléchargement fini ne s'annule plus")
	}
}

// CE QUI NE PREND PAS LA FILE PART TOUT DE SUITE (au serveur : un émulateur, un BIOS, un correctif), même pendant
// un jeu ; et la même chose (`Key`) ne se demande pas deux fois tant qu'elle attend ou tourne.
func TestAnUnqueuedDownloadDoesNotWaitAndTheSameOneIsNotAskedTwice(t *testing.T) {
	var downloads Downloads
	game, gameStarted, gameRelease := blockingJob("claude", "roms/psp/a", true)
	core, coreStarted, coreRelease := blockingJob("claude", "cores/nes/fceumm", false)
	gameID, _ := downloads.Start(game)
	waitFor(t, gameStarted, "le jeu")
	coreID, _ := downloads.Start(core)
	waitFor(t, coreStarted, "l'émulateur, pendant le jeu")

	again, _, _ := blockingJob("claude", "roms/psp/a", true)
	if _, err := downloads.Start(again); err != ErrDownloadPending {
		t.Fatalf("le même jeu, demandé deux fois : %v", err)
	}
	close(gameRelease)
	close(coreRelease)
	lastStep(t, &downloads, gameID, "claude")
	lastStep(t, &downloads, coreID, "claude")
	if _, err := downloads.Start(again); err != nil {
		t.Fatalf("fini, il se redemande : %v", err)
	}
}

// UN TÉLÉCHARGEMENT COMPTE SES OCTETS À LA COPIE, et un abonné tardif reçoit le dernier état ; seul son compte s'y
// abonne.
func TestADownloadCountsItsBytesAndReplaysItsLastState(t *testing.T) {
	var downloads Downloads
	payload := bytes.Repeat([]byte("x"), 3<<20)
	var copied bytes.Buffer
	id, _ := downloads.Start(DownloadJob{Owner: "claude", Total: int64(len(payload)), Run: func(ctx context.Context) (int, []byte) {
		if _, err := io.Copy(ProgressWriter(ctx, &copied, 0), bytes.NewReader(payload)); err != nil {
			return 502, ErrorBody("server_failed", err.Error())
		}

		return 200, OKBody(map[string]string{"name": "Game.sfc"})
	}})
	if _, _, _, known := downloads.Subscribe(id, "someone-else"); known {
		t.Fatal("un autre compte ne s'abonne pas au salon")
	}
	last := lastStep(t, &downloads, id, "claude")
	if last.Type != "done" || last.Received != int64(len(payload)) || !strings.Contains(string(last.Result), "Game.sfc") {
		t.Fatalf("dernier état : %+v", last)
	}
	late, replay, _, known := downloads.Subscribe(id, "claude")
	if !known || replay.Type != "done" {
		t.Fatalf("abonné tardif : %v %+v", known, replay)
	}
	if _, open := <-late; open {
		t.Fatal("le canal d'un salon fini est fermé")
	}
}

// UN ÉCHEC DIT SA RAISON ; une copie relayée (le téléphone) dit son avance ; une copie hors d'un téléchargement suivi
// ne compte rien.
func TestAFailedDownloadSaysWhyAndAnUntrackedCopyCountsNothing(t *testing.T) {
	var downloads Downloads
	id, _ := downloads.Start(DownloadJob{Owner: "claude", Run: func(ctx context.Context) (int, []byte) {
		ReportProgress(ctx, 10, 100)

		return 502, ErrorBody("server_failed", "ce serveur ne l'a pas donné")
	}})
	last := lastStep(t, &downloads, id, "claude")
	if last.Type != "failed" || last.Error != "ce serveur ne l'a pas donné" || last.Total != 100 || last.Received != 10 {
		t.Fatalf("échec : %+v", last)
	}

	var buffer bytes.Buffer
	if writer := ProgressWriter(context.Background(), &buffer, 10); writer != &buffer {
		t.Fatal("sans téléchargement suivi, l'écrivain reste tel quel")
	}
}

// UN ÉTAT AU PLUS TOUS LES QUARTS DE SECONDE : une copie de mille morceaux ne publie pas mille états.
func TestProgressIsThrottled(t *testing.T) {
	var downloads Downloads
	job, started, release := blockingJob("claude", "", false)
	id, _ := downloads.Start(job)
	waitFor(t, started, "le geste")
	steps, _, unsubscribe, _ := downloads.Subscribe(id, "claude")
	defer unsubscribe()
	time.Sleep(downloadEvery)
	for range 1000 {
		downloads.add(id, 1, 0)
	}
	time.Sleep(10 * time.Millisecond)
	if count := len(steps); count > 2 {
		t.Fatalf("%d états pour mille morceaux en quelques millisecondes", count)
	}
	close(release)
}

// LA LISTE SE RÉVEILLE à chaque changement : un départ, une fin.
func TestTheListIsWokenOnEachChange(t *testing.T) {
	var downloads Downloads
	wake, stop := downloads.Watch()
	defer stop()
	job, started, release := blockingJob("claude", "roms/nes/a", true)
	id, _ := downloads.Start(job)
	waitFor(t, wake, "le départ")
	waitFor(t, started, "le geste")
	close(release)
	lastStep(t, &downloads, id, "claude")
	waitFor(t, wake, "la fin")
	if len(downloads.List("claude")) != 0 {
		t.Fatal("un téléchargement fini quitte la liste")
	}
}
