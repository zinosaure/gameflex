package engine

import (
	"runtime"
	"testing"
	"time"
)

// LE FIL DU MOTEUR N'EST JAMAIS LE FIL PRINCIPAL DU RUNTIME, et il se termine
// avec la partie (2026-10-15). Go gare m0 au lieu de le terminer : une partie
// dont la goroutine y tombait gardait son fil, et son cœur en mémoire.
//
// Le fil de ce test, verrouillé, joue le rôle de m0.
func TestTheEngineThreadIsNeverTheRuntimeMainThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	saved := mainThread
	mainThread = gettid()
	defer func() { mainThread = saved }()

	ran := make(chan int)
	onOwnThread(func() { ran <- gettid() })
	tid := <-ran
	if tid == mainThread {
		t.Fatal("le corps tourne sur le fil principal : Go ne le terminera pas")
	}
	if gettid() != mainThread {
		t.Fatal("l'appelant a perdu son fil")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !threadGone(tid) && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !threadGone(tid) {
		t.Fatal("le fil du moteur vit encore après la fin de son corps")
	}
}

// AILLEURS QUE SUR M0, LE CORPS TOURNE SUR LE FIL DE L'APPELANT, verrouillé, et
// ce fil se termine avec la goroutine.
func TestTheEngineThreadIsTheCallersOwnElsewhere(t *testing.T) {
	tids := make(chan int)
	go onOwnThread(func() { tids <- gettid() })
	tid := <-tids
	if tid == mainThread {
		t.Skip("la goroutine est tombée sur m0 : couvert par le test précédent")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !threadGone(tid) && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !threadGone(tid) {
		t.Fatal("le fil verrouillé ne s'est pas terminé avec sa goroutine")
	}
}
