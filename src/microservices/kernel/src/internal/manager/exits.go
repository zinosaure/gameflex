package manager

// CE QUE LE KERNEL APPREND DE SES CONTENEURS, SANS LES SONDER (#751, `#262`).
//
//   - un child se dit prêt, ou dit où en est son déballage : ReportBoot, appelé par la route
//     `POST /internal/sessions/<id>/boot`, réveille le lancement qui l'attend (docker.WaitBoot) ;
//   - un conteneur sort : Docker le dit (docker.WatchSessionExits) et containerExited retire sa partie, sur-le-champ,
//     au lieu du balayage de vingt secondes qui le demandait à Docker ;
//   - ce qui est sorti sans qu'on l'entende (avant le démarrage du kernel, ou pendant une coupure du flux de Docker) :
//     resync, une fois à chaque (re)connexion, et c'est tout.
//
// Les joueurs ne se recopient plus de `/status` : le child les pousse dans son miroir (chantier 4), qui fait foi
// partout où le kernel les montre (api.applyMirror).

import (
	"context"
	"log/slog"

	"gameflex/kernel/internal/docker"
)

// expectBoot ouvre la boîte où le child d'un lancement poussera son démarrage.
func (manager *Manager) expectBoot(sessionID string) chan docker.BootReport {
	manager.bootMu.Lock()
	defer manager.bootMu.Unlock()

	reports := make(chan docker.BootReport, 8)
	manager.boots[sessionID] = reports

	return reports
}

// forgetBoot ferme la boîte d'un lancement fini, réussi ou non.
func (manager *Manager) forgetBoot(sessionID string) {
	manager.bootMu.Lock()
	defer manager.bootMu.Unlock()

	delete(manager.boots, sessionID)
}

// ReportBoot remet au lancement qui l'attend ce qu'un child dit de son démarrage. Faux si aucun lancement n'attend
// cette partie. Jamais bloquant : une avancée de déballage en trop est jetée, « ready » prend la place de la plus
// ancienne.
func (manager *Manager) ReportBoot(sessionID string, report docker.BootReport) bool {
	manager.bootMu.Lock()
	defer manager.bootMu.Unlock()

	reports, waiting := manager.boots[sessionID]
	if !waiting {
		return false
	}
	select {
	case reports <- report:
	default:
		if report.State != "ready" {
			return true
		}
		select {
		case <-reports:
		default:
		}
		reports <- report
	}

	return true
}

// watchExits suit la sortie des conteneurs de partie tant que le kernel vit.
func (manager *Manager) watchExits() {
	manager.dockerHandler.WatchSessionExits(context.Background(), manager.containerExited, manager.resync)
}

// containerExited — Docker dit qu'un conteneur de partie est sorti : sa partie s'arrête (l'enregistrement, son port,
// la façade). Un conteneur qu'aucune partie ne porte (un lancement en cours, qui voit la sortie lui-même ; une partie
// déjà arrêtée par nous) ne demande rien.
func (manager *Manager) containerExited(containerName string) {
	ctx := context.Background()
	for _, session := range manager.sessions.list() {
		if session.Container == containerName {
			slog.Info("session container exited", "session_id", session.SessionID, "container", containerName)
			manager.stop(ctx, session.SessionID, "container-gone")
			return
		}
	}
	for _, record := range manager.retro.list() {
		if record.Container == containerName {
			slog.Info("retro session container exited", "session_id", record.SessionID, "container", containerName)
			manager.StopRetro(ctx, record.SessionID)
			return
		}
	}
	for _, record := range manager.remoteplay.list() {
		if record.Container == containerName {
			slog.Info("remoteplay session container exited", "session_id", record.SessionID, "container", containerName)
			manager.StopRemoteplay(ctx, record.SessionID)
			return
		}
	}
}

// resync rattrape ce qui est sorti sans qu'on l'entende : les parties dont le conteneur ne tourne plus, et les
// conteneurs arrêtés qu'aucune partie ne porte (sortis pendant que le kernel était arrêté — l'adoption ne voit que ceux
// qui tournent).
func (manager *Manager) resync() {
	ctx := context.Background()
	gone := func(container string) bool {
		running, err := manager.dockerHandler.IsRunning(ctx, container)
		return err != nil || !running
	}
	for _, session := range manager.sessions.list() {
		if gone(session.Container) {
			manager.containerExited(session.Container)
		}
	}
	for _, record := range manager.retro.list() {
		if gone(record.Container) {
			manager.containerExited(record.Container)
		}
	}
	for _, record := range manager.remoteplay.list() {
		if gone(record.Container) {
			manager.containerExited(record.Container)
		}
	}
	manager.pruneExitedContainers()
}

// WaitExited — le signal de la sortie d'un conteneur, que Docker donne (docker.WaitExited). Pour l'arrêt du kernel
// (api/child_end.go), qui laisse un child d'accord pour finir sortir de lui-même avant de ranger derrière lui. Sans
// Docker (les tests), le signal est déjà donné : rien à attendre.
func (manager *Manager) WaitExited(ctx context.Context, container string) <-chan struct{} {
	if manager == nil || manager.dockerHandler == nil {
		exited := make(chan struct{})
		close(exited)

		return exited
	}

	return manager.dockerHandler.WaitExited(ctx, container)
}
