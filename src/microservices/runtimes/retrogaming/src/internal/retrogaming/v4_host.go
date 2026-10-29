package retrogaming

// chantier-4 — this child owns its session. Everything generic (the socket,
// the intent webhook, authentication, the sweep, the kernel mirror) lives in
// /shared/session; what is left here is what only retro-gaming knows: how a
// queue slot maps to a libretro controller port, and how to stop the emulator.
//
// Served BESIDE /session, /session/join and /session/spectate, which remain
// the transport for video and input. v4 decides WHO plays; those routes still
// carry it.

import (
	"log"
	"net/http"
	"os"
	"time"

	"gameflex/auth"
	"gameflex/consoles"
	"gameflex/session"
)

var v4 *session.HTTPHost

// v4Enabled reports whether this child was given what it takes to own its
// session. Missing pieces disable the route entirely rather than half-run it:
// a service without its secret must fail closed.
func v4Enabled() bool {
	return auth.PublicKeyFromEnv() != nil && os.Getenv("GAMEFLEX_SESSION_OWNER") != ""
}

func startV4Host(sessionID string) {
	consoleID := os.Getenv("GAMEFLEX_CONSOLE_ID")

	v4 = session.NewHTTPHost(session.Config{
		SessionID:      sessionID,
		PlatformID:     "retrogaming",
		Owner:          os.Getenv("GAMEFLEX_SESSION_OWNER"),
		MaxPlayers:     consoles.MaxPlayers(consoleID),
		JWTPublicKey:   auth.PublicKeyFromEnv(),
		KernelURL:      os.Getenv("GAMEFLEX_KERNEL_URL"),
		InternalSecret: os.Getenv("GAMEFLEX_INTERNAL_SECRET"),
		BeatTimeout:    90 * time.Second,
		// VINGT-NEUF, PAS QUINZE — et une minute de grâce par-dessus.
		//
		// Quinze minutes excluaient quelqu'un qui regardait son propre jeu :
		// une cinématique peut durer plus d'un quart d'heure, et ne rien
		// toucher pendant ce temps est exactement ce que le jeu demande.
		// Vingt-neuf plus une font la demi-heure, qui reste le seuil.
		//
		// Et on PRÉVIENT avant d'exclure : le joueur concerné — lui seul —
		// reçoit la question et une minute pour toucher n'importe quoi.
		AFKAfter:  29 * time.Minute,
		AFKGrace:  time.Minute,
		IdleAfter: 5 * time.Minute,
		// Une manette qui change de mains revient au neutre (releasePort).
		// La file compte les manettes depuis 1, libretro ses ports depuis 0.
		OnPadHandover: func(slot int) {
			activeChildSessionMu.Lock()
			sess := activeChildSession
			activeChildSessionMu.Unlock()

			if sess != nil {
				sess.releasePort(slot - 1)
			}
		},
		EndSession: func(reason string) {
			log.Printf("[retrogaming-child %s] chantier-4: ending session (%s)", sessionID, reason)

			activeChildSessionMu.Lock()
			sess := activeChildSession
			activeChildSessionMu.Unlock()

			if sess != nil {
				sess.shutdown()
			}

			os.Exit(0)
		},
	})

	log.Printf("[retrogaming-child %s] chantier-4 host ready (owner=%q console=%q maxPlayers=%d)",
		sessionID, os.Getenv("GAMEFLEX_SESSION_OWNER"), consoleID, consoles.MaxPlayers(consoleID))
}

// v4Borrower — à qui le propriétaire prête sa manette, "" sinon (#274).
func v4Borrower() string {
	if v4 == nil {
		return ""
	}

	return v4.Host.Borrower()
}

// v4Players — combien de personnes tiennent une place, selon la file ; -1
// sans file.
func v4Players() int {
	if v4 == nil {
		return -1
	}

	return len(v4.Host.Snapshot().Players)
}

func registerV4Routes(mux *http.ServeMux) {
	v4.Routes(mux, "/v4")
}

// v4PortForViewer maps the queue's answer onto a libretro controller port.
//
// libretro counts ports from 0 while the queue counts slots from 1, so the
// owner's slot 1 is port 0. A refusal returns -1 and NEVER 0: port 0 is the
// owner's, so a zero sentinel would hand every watcher the player-1 pad — the
// zero value of an int is the wrong sentinel exactly where it is most
// tempting.
func v4PortForViewer(viewerID string) (port int, managed bool) {
	if v4 == nil {
		return -1, false
	}

	slot, managed := v4.PortForViewer(viewerID)
	if !managed {
		return -1, false
	}
	if slot == 0 {
		return -1, true // known to v4, holding nothing: watching
	}

	return slot - 1, true
}
