package api

// The unified idle rule (docs/MODELE_SESSION_UNIFIE.md, étape 3): ONE
// watchdog for every platform, driven by the pool's IdleSince — the
// manager's own per-platform TTL/grace rules (streamex SessionTTLSeconds,
// retro retroPlayersAt, remoteplay remoteplayIdleStopAfter) were removed
// in favor of this. Spectators do NOT keep a session alive: the rule is
// "container without a live player for 5 min → stop", exactly as decided.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/registry"
)

const (
	// unifiedIdleTimeout matches the heartbeat timeout: a player who left
	// (gracefully or by the sweep) gets a 5-min rejoin window, then the
	// container dies — the same window retro's child grace and remoteplay's
	// idle stop used to implement separately.
	unifiedIdleTimeout = 5 * time.Minute
	idleWatchdogTick   = 1 * time.Minute
	// sweepBudget — ce qu'on accorde au balayage d'UNE session avant de passer
	// à la suivante.
	//
	// Dix secondes, soit le double du délai de `docker.statusClient` (5 s) : de
	// quoi laisser l'appel HTTP aller au bout de sa propre garde, plus une
	// marge pour l'aller-retour Docker qui le précède. Au-delà, ce n'est plus
	// une session lente, c'est une session qui ne répondra pas — et le tic doit
	// continuer sans elle. C'est précisément ce qui manquait le 2026-09-19 :
	// une session bloquée emportait les 60 s du tic, puis tous les suivants.
	sweepBudget = 10 * time.Second
)

// idleWatchdog is the single idle shutdown loop.
func (server *Server) idleWatchdog() {
	ticker := time.NewTicker(idleWatchdogTick)
	defer ticker.Stop()

	for range ticker.C {
		// A container can die on its own, and then nothing calls the stop
		// path above. Reconcile: a mirror whose session left the pool is
		// stale by definition.
		server.mirrors.RetainOnly(server.pool.List())

		for _, session := range server.pool.List() {
			// chantier-4: a child-managed session runs its OWN idle clock and
			// exits by itself. Running a second clock here over a COPY of its
			// state is precisely the shape that kept diverging — and the copy
			// would always be the one that was wrong.
			if _, mirrored := server.mirrors.Get(session.ID); mirrored {
				// ON NE TUE PAS UN CHILD, ON CONSTATE QU'IL N'EST PLUS LÀ.
				//
				// L'ordre d'arrêt vient du child — il décide de sa propre fin
				// et sortir EST le signal. Le seul cas où le kernel agit sans
				// ordre est celui où il n'y a plus personne pour en donner :
				// le conteneur est mort. Et ça, Docker le sait avec
				// certitude — inutile de le deviner en chronométrant un
				// silence, ce qu'une première version de cette garde faisait
				// (et qui revenait à tuer de sa propre initiative un child
				// bien vivant dont le rapport avait pris du retard).
				//
				// Ces balayages sont idempotents et n'agissent QUE si le
				// conteneur a disparu ; sinon ils se contentent de rafraîchir
				// ce que Docker dit — le domaine du kernel.
				// BORNÉ, parce que c'est ICI que tout s'est arrêté le
				// 2026-09-19. `sweepContainer` mène à `Status*`, et l'appel
				// n'avait alors aucune fin : la ligne suivante — la
				// VÉRIFICATION que le kernel doit faire quand un child se tait —
				// n'a jamais été atteinte, 18 heures durant. Le client HTTP est
				// désormais borné lui aussi (`docker.statusClient`) ; cette
				// échéance est la seconde garde, celle qui couvre un appel
				// Docker qui traînerait.
				sweepCtx, cancelSweep := context.WithTimeout(context.Background(), sweepBudget)
				server.sweepContainer(sweepCtx, session)
				cancelSweep()
				// Le conteneur vit toujours mais le child s'est tu ? On lui
				// DEMANDE, au lieu de deviner (voir askChild).
				server.askChildIfSilent(session)

				continue
			}

			idleFor := server.idleFor(session)
			if idleFor <= unifiedIdleTimeout {
				continue
			}

			slog.Warn("session idle, stopping (unified rule)", "session_id", session.ID, "platform", session.Platform, "idle_for", idleFor.String())
			server.stopSessionByPlatform(context.Background(), session.ID, session.Platform)
		}
	}
}

// idleFor computes how long the session has been without a live player:
// IdleSince when a player ever left, else the ORPHAN clock (StartedAt) — a
// session that no child mirrors and that nobody ever left: launched, then
// abandoned before its child reported. (It also consulted a table of player
// heartbeats; nothing has fed it since #195 and #267, so the orphan clock was
// already the only answer — the table is gone, chantier 18.)
func (server *Server) idleFor(session registry.Session) time.Duration {
	if session.IdleSince != nil {
		return time.Since(*session.IdleSince)
	}
	if len(session.Players) > 0 {
		return time.Since(session.StartedAt)
	}
	return 0
}

// stopSessionByPlatform stops a session's container and drops it from the
// pool. Shared by the idle watchdog and the v1 AFK kick (chantier-3: an
// owner kicked for inactivity ends the session, not just their socket).
// Only entries this process actually stopped leave the pool — a foreign
// registration, unknown to the manager, must not be silently unregistered.
func (server *Server) stopSessionByPlatform(ctx context.Context, sessionID, platform string) bool {
	stopped := false

	switch platform {
	case "streamex":
		stopped = server.manager.StopStreamex(ctx, sessionID)
	case "retrogaming":
		stopped = server.manager.StopRetro(ctx, sessionID)
	case "remoteplay":
		stopped = server.manager.StopRemoteplay(ctx, sessionID)
	default:
		return false
	}

	if stopped {
		server.pool.Unregister(sessionID)
	}
	// The mirror of a session that no longer exists is a phantom — exactly
	// the shape of remoteplay's "Someone is connected" on a console that was
	// switched off. Drop it with the session.
	server.mirrors.Drop(sessionID)

	return stopped
}

// sweepContainer demande à Docker si le conteneur d'une session pilotée existe
// encore, et laisse le balayage de la plateforme en tirer les conséquences.
// Aucune décision ici : on relaie une constatation.
func (server *Server) sweepContainer(ctx context.Context, session registry.Session) {
	switch session.Platform {
	case "retrogaming":
		server.manager.SweepRetro(ctx, session.ID)
	case "remoteplay":
		server.manager.SweepRemoteplay(ctx, session.ID)
	}
}

// childV4URL — l'adresse d'une route de l'enfant, sur le port de SON
// conteneur. Chaque runtime la monte sous `/v4` (`v4.Routes(mux, "/v4")`,
// shared/session/httphost.go). Ce contrôle les appelait sous
// `/api/v1/sessions/<id>/child/v4/…` — le préfixe que seul le kernel du
// TÉLÉPHONE sert, et que le relais public `v4_proxy.go` retire : un 404 à
// chaque fois, depuis son premier jour (2026-09-09). Le kernel journalisait
// « le child répond mal » et ne demandait jamais de conclure ; une session
// fantôme le restait.
func childV4URL(host string, port int, action string) string {
	return fmt.Sprintf("http://%s:%d/v4/%s", host, port, action)
}

// childReportSilence — au bout de combien de temps sans rapport on va poser la
// question. Trois rapports manqués (session.mirrorHeartbeat en émet un toutes
// les 30 s) : de quoi laisser passer un réseau qui hoquette.
const childReportSilence = 90 * time.Second

// windDownAttempts — combien de fois on demande avant de conclure que le child
// est planté. Trois : une sollicitation ratée peut être un hoquet, trois
// d'affilée sur un conteneur que Docker dit vivant ne le sont plus.
const windDownAttempts = 3

// askChildIfSilent interroge un child qui ne s'est plus manifesté.
//
// UN SILENCE N'EST PAS UNE INFORMATION, UNE RÉPONSE EN EST UNE. Le kernel ne
// déduit rien de l'absence de rapport — il pose la question, et c'est toujours
// le child qui répond qui est là. L'autorité ne bouge pas ; on remplace une
// devinette par un fait.
//
// C'est ce qui fait disparaître les sessions fantômes : un conteneur que
// Docker dit vivant et dont le child ne répond plus n'est plus une partie,
// c'est un reste. On le DIT — fort — et on ne le tue pas : arrêter reste une
// décision du child, et un conteneur qu'on abat sur une requête ratée est
// exactement l'initiative qu'on vient de retirer au kernel.
func (server *Server) askChildIfSilent(session registry.Session) {
	snapshot, ok := server.mirrors.Get(session.ID)
	if !ok {
		return
	}
	silence := time.Since(snapshot.ReceivedAt)
	if silence < childReportSilence {
		return
	}

	// `childPort` existait déjà (v4_proxy.go) : c'est le manager qui a
	// lancé le conteneur, donc lui qui sait où il écoute.
	port := server.childPort(session.ID)
	if port == 0 {
		// UN ABANDON SILENCIEUX EST UN ABANDON QU'ON NE CORRIGE JAMAIS.
		//
		// `childPort` rend 0 dès que l'enregistrement du manager a disparu — et
		// cette fonction renonçait alors sans un mot, exactement là où elle
		// devait constater. Une session dont le miroir se tait depuis plus de
		// 90 s ET dont on ne sait plus où joindre l'enfant est précisément le
		// fantôme qu'on cherche : on le DIT.
		slog.Warn("session fantôme : le child s'est tu et on ne sait plus où le joindre",
			"session_id", session.ID, "platform", session.Platform,
			"silence", silence.String())

		return
	}

	url := childV4URL(server.config.PublicHost, port, "report")
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	request.Header.Set("X-Internal-Secret", sharedauth.SessionSecret(server.config.InternalSecret, session.ID))

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		slog.Warn("session fantôme : le conteneur vit, le child ne répond pas",
			"session_id", session.ID, "platform", session.Platform,
			"silence", silence.String(), "error", err)

		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		slog.Warn("session fantôme : le child répond mal",
			"session_id", session.ID, "status", response.StatusCode, "silence", silence.String())

		return
	}

	var fresh registry.MirrorSnapshot
	if err := json.NewDecoder(response.Body).Decode(&fresh); err != nil {
		slog.Warn("session fantôme : rapport illisible", "session_id", session.ID, "error", err)

		return
	}

	slog.Info("child interrogé après un silence, il a répondu",
		"session_id", session.ID, "silence", silence.String(), "players", len(fresh.Players))
	server.mirrors.Put(session.ID, fresh)
	server.pool.Notify()

	// Il a répondu, et ce qu'il dit ressemble à un fantôme : plus personne, et
	// inoccupé depuis longtemps. Il aurait dû conclure lui-même — sa boucle
	// est sans doute coincée. On le lui DEMANDE.
	if len(fresh.Players) == 0 && fresh.IdleSince != nil &&
		time.Since(*fresh.IdleSince) > unifiedIdleTimeout {
		server.askChildToWindDown(session, port)
	}
}

// askChildToWindDown demande au child de conclure — il reste seul juge.
//
// LA SÉQUENCE, ET POURQUOI ELLE A CETTE FORME :
//
//	— « pas de nouvelles depuis 1 min 30, tu es là ? »
//	— « oui, mais plus personne et inoccupé depuis plus de 5 min »
//	— « tu peux m'envoyer une demande d'arrêt ? »
//	— et c'est LE CHILD qui l'envoie.
//
// À aucun moment le kernel ne conclut à sa place. Il constate, il sollicite,
// il attend. La décision reste chez celui qui a l'information.
//
// L'EXCEPTION EST BORNÉE, ET C'EST ce qui la rend acceptable : après
// windDownAttempts sollicitations sans que rien ne bouge, le child est
// considéré planté — son conteneur, lui, vit toujours, sinon Docker l'aurait
// dit. Toutes les voies de résolution ont échoué ; le kernel dispose alors du
// conteneur, ce qui est son domaine. On compte les tentatives précisément pour
// que ce ne soit jamais un premier réflexe.
func (server *Server) askChildToWindDown(session registry.Session, port int) {
	url := childV4URL(server.config.PublicHost, port, "wind-down")
	request, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return
	}
	request.Header.Set("X-Internal-Secret", sharedauth.SessionSecret(server.config.InternalSecret, session.ID))

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		defer response.Body.Close()
		var answer struct {
			Ending bool   `json:"ending"`
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(response.Body).Decode(&answer)
		if answer.Ending {
			// Il conclut lui-même : sortir EST le signal, il n'y a rien à
			// faire de plus ici.
			slog.Info("le child a accepté de conclure", "session_id", session.ID, "reason", answer.Reason)
			server.windDownAsks.Delete(session.ID)

			return
		}
		slog.Info("le child refuse de conclure", "session_id", session.ID, "reason", answer.Reason)
		server.windDownAsks.Delete(session.ID)

		return
	}

	// Pas de réponse à la sollicitation, alors qu'il venait de répondre au
	// rapport : on compte, et on ne fait rien d'autre tant que le compte n'est
	// pas atteint.
	asked, _ := server.windDownAsks.LoadOrStore(session.ID, 0)
	count := asked.(int) + 1
	server.windDownAsks.Store(session.ID, count)

	if count < windDownAttempts {
		slog.Warn("le child ne répond pas à la demande d'arrêt", "session_id", session.ID,
			"tentative", count, "sur", windDownAttempts, "error", err)

		return
	}

	slog.Error("child planté, conteneur toujours vivant : le kernel en dispose",
		"session_id", session.ID, "platform", session.Platform, "tentatives", count)
	server.windDownAsks.Delete(session.ID)
	server.stopSessionByPlatform(context.Background(), session.ID, session.Platform)
}
