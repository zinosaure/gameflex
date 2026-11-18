package api

// LA DISPONIBILITÉ PAR SOURCE, DEMANDÉE À LA SOURCE (chantier 53, P5, #672).
// L'écran d'un émulateur ou d'un BIOS absent propose « Le récupérer » : la
// source officielle (un cœur libretro, chez le buildbot ; un émulateur de
// console récente, chez son éditeur), puis chaque serveur fédéré, grisés quand
// ils ne l'ont pas. Jusqu'ici, la disponibilité se lisait dans la COPIE du
// listing d'un pair — périmée, et muette : un pair injoignable donnait une
// liste vide sans dire pourquoi —, et la source officielle n'était jamais
// interrogée : un cœur absent du buildbot ne se voyait qu'à l'échec.
//
// Ici, chaque source est interrogée au moment où l'écran s'ouvre : un HEAD chez
// le buildbot ou l'éditeur ; chez un pair, le fichier lui-même, demandé avec
// notre jeton, sa réponse refermée dès qu'elle commence (ce que le pair refuse
// ou ne connaît pas se dit alors avec sa raison). Les sources répondent en
// parallèle, chacune dans un délai borné. La manière de demander est partagée
// avec le téléphone (remote.ProbeOfficial, remote.ProbePeer) ; ce fichier dit
// où la source officielle publie, et quel fichier ce serveur prendrait.
//
//	GET /api/v1/retrogaming/storage/availability?console=<id>&section=cores|bios&name=<nom>

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/remote"
)

func (server *Server) handleStorageAvailability(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	consoleID := strings.ToLower(strings.TrimSpace(query.Get("console")))
	section := query.Get("section")
	name := strings.TrimSpace(query.Get("name"))
	if !consoles.Exists(consoleID) || name == "" || (section != "cores" && section != "bios") {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "console, section (cores or bios) and name are required")

		return
	}

	sources := []remote.Availability{}
	if official := server.officialURL(r.Context(), consoleID, section, name); official != "" {
		sources = append(sources, remote.ProbeOfficial(r.Context(), official))
	}

	// Chez un pair, le fichier que ce serveur prendrait (le geste partagé, remote.ProbePeer) : son BIOS, son paquet
	// d'émulateur, ou son cœur Linux.
	open := func(client *remote.Server) (io.ReadCloser, error) {
		switch {
		case section == "bios":
			return client.OpenBios(consoleID, name)
		case retro_files.IsEmulatorFile(consoleID, name):
			return client.OpenEmulatorFile(consoleID, name)
		}

		return client.OpenCore(consoleID, name)
	}
	peers, _ := server.registeredServers()
	answers := make([]remote.Availability, len(peers))
	var wait sync.WaitGroup
	for index, peer := range peers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			answers[index] = remote.ProbePeer(peerOf(peer), peer.Hostname, open)
		}()
	}
	wait.Wait()
	sources = append(sources, answers...)

	writeJSON(w, http.StatusOK, map[string]any{"consoleId": consoleID, "section": section, "name": name, "sources": sources})
}

// officialURL — où la source officielle le publie : un cœur du registre chez
// le buildbot, le fichier qu'une image attend dans l'index des paquets streamex
// (chantier 63 : plus chez son éditeur). Vide pour un BIOS (personne ne les
// distribue) et pour tout autre fichier.
func (server *Server) officialURL(ctx context.Context, consoleID, section, name string) string {
	if section != "cores" {
		return ""
	}
	if retro_files.IsEmulatorFile(consoleID, name) {
		for _, need := range server.requiredEmulators(ctx) {
			if need.Console == consoleID && need.File == name {
				return server.streamexIndexURL()
			}
		}

		return ""
	}
	if coreBelongsTo(consoleID, name) && !strings.Contains(name, " ") {
		return coreBuildbotURL(name)
	}

	return ""
}
