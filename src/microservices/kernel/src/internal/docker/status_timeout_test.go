package docker

import (
	"context"
	"net"
	"testing"
	"time"

	"gameflex/kernel/internal/config"
)

// LE SILENCE D'UN CONTENEUR DOIT AVOIR UNE FIN.
//
// Le 2026-09-19, un enfant rétro a cessé de répondre tout en ACCEPTANT encore
// les connexions. `StatusRetro` utilisait `http.DefaultClient` — sans délai de
// garde — avec un contexte sans échéance : l'appel n'est jamais revenu. Le chien
// de garde étant une seule goroutine, il n'a plus jamais vu un tic : 18 heures
// sans le moindre balayage, toutes plateformes confondues, et une session figée
// dans la liste (#197).
//
// Ce test rejoue exactement ça : un écouteur qui ACCEPTE et ne répond JAMAIS.
// Sans `statusClient`, il ne rendrait pas la main et le test expirerait.
func acceptAndStayMute(t *testing.T) (port int, stop func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("écouteur : %v", err)
	}

	done := make(chan struct{})
	conns := make(chan net.Conn, 8)
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// On garde la connexion OUVERTE sans jamais écrire : c'est la
			// forme exacte du défaut. La fermer rendrait une erreur tout de
			// suite, et le test ne prouverait plus rien.
			conns <- conn
		}
	}()

	return listener.Addr().(*net.TCPAddr).Port, func() {
		listener.Close()
		<-done
		close(conns)
		for conn := range conns {
			conn.Close()
		}
	}
}

func TestStatusRetroGivesUpOnAMuteContainer(t *testing.T) {
	port, stop := acceptAndStayMute(t)
	defer stop()

	client := &Client{config: config.Config{PublicHost: "127.0.0.1"}}

	// La borne du test est LARGEMENT au-dessus du délai du client (5 s) et
	// largement en dessous de « pour toujours ». Si le délai disparaît, ce test
	// ne ralentit pas : il échoue.
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := client.StatusRetro(context.Background(), "conteneur-muet", port)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("StatusRetro a réussi sur un conteneur qui n'a jamais répondu")
		}
		if elapsed := time.Since(started); elapsed > 15*time.Second {
			t.Errorf("StatusRetro a mis %s à renoncer — le délai de garde ne joue pas", elapsed)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("StatusRetro n'est JAMAIS revenu : c'est le défaut du 2026-09-19, revenu")
	}
}

// Les trois lectures d'état partagent le même client : ce qui vaut pour l'une
// vaut pour les deux autres. On le vérifie plutôt que de le supposer — elles
// étaient déjà triplées à l'identique une fois.
func TestEveryStatusReadIsBounded(t *testing.T) {
	port, stop := acceptAndStayMute(t)
	defer stop()

	client := &Client{config: config.Config{PublicHost: "127.0.0.1"}}

	reads := map[string]func() error{
		"StatusExternal": func() error {
			_, err := client.StatusExternal(context.Background(), "muet", port)

			return err
		},
		"StatusRemoteplay": func() error {
			_, err := client.StatusRemoteplay(context.Background(), "muet", port)

			return err
		},
	}

	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- read() }()

			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("%s a réussi sur un conteneur muet", name)
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("%s n'est jamais revenu — appel non borné", name)
			}
		})
	}
}

// Le délai lui-même, épinglé : le changer est une décision, pas un accident.
func TestStatusClientHasATimeout(t *testing.T) {
	if statusClient.Timeout == 0 {
		t.Fatalf("statusClient n'a aucun délai de garde — c'est exactement le défaut de #197")
	}
	if statusClient.Timeout > 30*time.Second {
		t.Errorf("délai de %s : trop long pour un /status qui répond en microsecondes", statusClient.Timeout)
	}
}
