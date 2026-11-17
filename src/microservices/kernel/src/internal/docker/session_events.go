package docker

// LE KERNEL NE SONDE PLUS SES CONTENEURS DE PARTIE (#751, qui clôt `#262`).
//
// Il leur redemandait `/status` toutes les secondes pendant un lancement, jusqu'à quatre-vingt-dix secondes, pour
// savoir qu'ils étaient prêts ; puis, toutes les vingt secondes, demandait à Docker s'ils vivaient encore et recopiait
// leur nombre de joueurs ; et, à la fin d'une partie, regardait tous les quarts de seconde si le conteneur était sorti.
// Trois sondages, contre la règle du porteur (« Tous les sondages se font par websocket. Pas d'exception »,
// 2026-11-16). Chacun a son émetteur :
//
//   - PRÊT : le child le dit lui-même quand son port écoute, et streamex y joint l'avancée d'un déballage
//     (`POST /internal/sessions/<id>/boot`, gameflex/session.ReportBoot) — awaitBoot les attend ;
//   - SORTI : Docker le dit, par son flux d'événements (`die`) et par `ContainerWait` — WatchSessionExits,
//     WaitExited ;
//   - LES JOUEURS : le child les pousse déjà dans son miroir (gameflex/session, chantier 4) ; la copie tirée de
//     `/status` est partie.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// BootReport — ce qu'un child pousse de son démarrage (la forme de gameflex/session.Boot) : « booting » avec
// l'avancée d'un déballage, ou « ready ».
type BootReport struct {
	State     string `json:"state"`
	Unpacking *struct {
		Done  int64 `json:"done"`
		Total int64 `json:"total"`
	} `json:"unpacking,omitempty"`
}

// WaitBoot attend que le child d'un conteneur se dise prêt (reports), au plus `timeout` sans nouvelle d'un déballage
// qui avance. Un conteneur qui sort avant rend ErrChildExited avec son code et ses dernières lignes.
func (dockerClient *Client) WaitBoot(ctx context.Context, containerName string, reports <-chan BootReport, timeout time.Duration, onUnpacking func(done, total int64)) error {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	exited := dockerClient.WaitExited(waitCtx, containerName)

	err := awaitBoot(ctx, reports, exited, timeout, onUnpacking)
	if errors.Is(err, ErrChildExited) {
		code := -1
		if inspected, inspectErr := dockerClient.dockerClient.ContainerInspect(ctx, containerName); inspectErr == nil {
			code = inspected.State.ExitCode
		}

		return fmt.Errorf("%w (exit code %d): %s", ErrChildExited, code, dockerClient.containerLogTail(ctx, containerName, 5))
	}

	return err
}

// awaitBoot — le cœur de WaitBoot, sans Docker : prêt, sorti, annulé, ou rien depuis `timeout`. UN DÉBALLAGE QUI
// AVANCE REPOUSSE L'ÉCHÉANCE (2026-10-22) : un .pkg de PS4 se déballe d'abord (deux minutes et demie pour les 90 Go
// de FF7 Remake) ; tant que des octets s'écrivent, l'attente continue.
func awaitBoot(ctx context.Context, reports <-chan BootReport, exited <-chan struct{}, timeout time.Duration, onUnpacking func(done, total int64)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var unpacked int64 = -1

	for {
		select {
		case report := <-reports:
			if report.State == "ready" {
				return nil
			}
			if report.Unpacking != nil && report.Unpacking.Done > unpacked {
				unpacked = report.Unpacking.Done
				deadline.Reset(timeout)
				if onUnpacking != nil {
					onUnpacking(report.Unpacking.Done, report.Unpacking.Total)
				}
			}
		case <-exited:
			return ErrChildExited
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("session did not become ready in time")
		}
	}
}

// WaitExited rend un signal fermé quand le conteneur n'est plus en marche — Docker le dit (`ContainerWait`), rien
// n'est relu. Un conteneur introuvable compte pour sorti. Le signal ne se ferme pas si ctx se termine avant.
func (dockerClient *Client) WaitExited(ctx context.Context, containerName string) <-chan struct{} {
	exited := make(chan struct{})
	results, errs := dockerClient.dockerClient.ContainerWait(ctx, containerName, container.WaitConditionNotRunning)
	go func() {
		select {
		case <-results:
			close(exited)
		case err := <-errs:
			if ctx.Err() == nil {
				slog.Debug("container wait ended", "container", containerName, "error", err)
				close(exited)
			}
		case <-ctx.Done():
		}
	}()

	return exited
}

// sessionExitsRetry — le pas de reconnexion au flux d'événements de Docker, s'il tombe (le démon redémarre). Un délai
// de reconnexion borné, pas un sondage : rien n'est relu entre deux essais.
const (
	sessionExitsRetryFirst = time.Second
	sessionExitsRetryMax   = 30 * time.Second
)

// WatchSessionExits suit la mort des conteneurs de partie de cette pile : onExit reçoit le nom de chacun à sa sortie.
// À chaque (re)connexion au flux, onResync rattrape ce qui a pu sortir sans qu'on l'entende (avant le démarrage du
// kernel, ou pendant une coupure du flux). Rend la main quand ctx se termine.
func (dockerClient *Client) WatchSessionExits(ctx context.Context, onExit func(containerName string), onResync func()) {
	wait := sessionExitsRetryFirst
	for ctx.Err() == nil {
		opened := time.Now()
		err := dockerClient.followSessionExits(ctx, onExit, onResync)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("docker events: session exits channel lost", "error", err)
		if time.Since(opened) > sessionExitsRetryMax {
			wait = sessionExitsRetryFirst
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, sessionExitsRetryMax)
	}
}

func (dockerClient *Client) followSessionExits(ctx context.Context, onExit func(string), onResync func()) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := dockerClient.sessionFilters(
		filters.Arg("type", string(events.ContainerEventType)),
		filters.Arg("event", string(events.ActionDie)),
	)
	messages, errs := dockerClient.dockerClient.Events(streamCtx, events.ListOptions{Filters: args})
	// Abonné D'ABORD, rattrapé ENSUITE : une sortie entre les deux arrive par le flux.
	onResync()

	for {
		select {
		case message := <-messages:
			if name := strings.TrimPrefix(message.Actor.Attributes["name"], "/"); name != "" {
				onExit(name)
			}
		case err := <-errs:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
