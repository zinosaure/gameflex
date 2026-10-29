// Command retrogaming runs ONE emulation session, as PID 1 of its own
// container. The sessions service spawns that container per session (see
// docker.CreateAndStartRetro) and passes the session id as argv; the process
// binds GAMEFLEX_CHILD_PORT, serves the session routes, and exits when the game
// ends — which is what stops the container.
//
// It used to be a long-lived HTTP service that re-exec'd itself once per
// session, handing each child a pre-bound socket on fd 3, with a supervisor
// tracking them and a REST/WS surface on port 10210. All of that is gone: the
// sessions service owns the retro API and the WS relay, the container IS the
// isolation, and this binary has exactly one job. The word "child" survives
// in the argv verb and in a few identifiers because it is what the sessions
// service passes — there is no parent left to be a child of.
package main

import (
	"log/slog"
	"os"

	"gameflex/retrogaming/internal/retrogaming"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 3 || os.Args[1] != "retrogaming-child" {
		slog.Error("usage: retrogaming retrogaming-child <sessionID>")
		os.Exit(2)
	}

	retrogaming.RunChild(os.Args[2])
}
