// Routes internes remote-play — et le sens est l'INVERSE de ce qu'il était.
//
// Jusqu'au 2026-09-19, le plan de contrôle menait : il résolvait l'appareil
// puis demandait ICI la création du conteneur (`POST /internal/remoteplay/
// sessions`). Cette route est supprimée avec l'ancienne porte publique qui
// l'alimentait (#195) : désormais c'est le kernel qui mène, depuis la porte
// unique, et qui appelle le runtime pour ce qu'il ne sait pas faire —
// résoudre un `hostId` et exécuter la poignée de main Takion
// (`launch_remoteplay.go`).
//
// Ce qui reste ici sert deux besoins bien vivants : `launchRemoteplay`, le
// corps que la porte emprunte, et les routes d'ARRÊT et de CONSULTATION, que
// le runtime appelle encore — la consultation porte même l'auto-réparation du
// port de l'enfant. Gardées par le secret interne, comme tout `/internal/*`.
package api

import (
	"errors"
	"log/slog"
	"net/http"

	"gameflex/kernel/internal/manager"
	"gameflex/kernel/internal/registry"
)

type remoteplayLaunchBody struct {
	SessionID string `json:"sessionId"`
	HostID    string `json:"hostId"`
	HostName  string `json:"hostName"`
	HostType  string `json:"hostType"`
	Username  string `json:"username"`
}

// remoteplayLaunchResult — ce qu'un lancement rend à SES DEUX appelants.
//
// `Port` n'est pas décoratif : c'est lui que la porte unique doit connaître
// pour demander ensuite la poignée de main Takion au runtime (#195). C'est
// précisément parce qu'il fallait cette valeur que le corps ci-dessous a été
// extrait plutôt que délégué — un handler qui écrit sa réponse ne rend rien à
// son appelant.
type remoteplayLaunchResult struct {
	SessionID string
	Port      int
	Existing  bool
}

// launchRemoteplay — LE LANCEMENT LUI-MÊME, sans sa mise en forme HTTP.
//
// Extrait du handler de l'ancienne route interne de lancement — supprimée
// depuis, avec la direction qu'elle servait (#195) — pour que la porte unique
// l'emprunte au lieu d'en recopier les gardes : recopier aurait créé un
// second endroit où corriger « une seule session jouée à la fois », le plafond
// par plateforme et l'inscription au pool — exactement le désordre que ce
// chantier répare.
//
// Rend `ok = false` APRÈS avoir écrit le refus : `writeAlreadyPlaying` et
// `reserveSpawn` prennent déjà `w` et savent le rendre mieux que ne le
// ferait une enveloppe d'erreur réinventée ici.
func (server *Server) launchRemoteplay(w http.ResponseWriter, r *http.Request, body remoteplayLaunchBody) (remoteplayLaunchResult, bool) {
	if body.HostID == "" || body.HostType == "" {
		writeError(w, http.StatusBadRequest, "hostId and hostType are required")

		return remoteplayLaunchResult{}, false
	}
	// No "anonymous" fallback: that placeholder became the child's session
	// owner, and the queue held a slot for a user nobody can ever be — fatal
	// on a one-slot console. An empty owner is honest and the child treats it
	// as "nothing reserved" (shared/session, freeSlot/capacityFor).

	// Resume path: a session already running for this device IS this
	// session (device id = session id) — return it instead of rejecting a
	// second launch. The control plane skips its child handshake when
	// "existing" comes back true, so Takion is never re-launched.
	if existing, ok := server.manager.LookupRemoteplayByHost(body.HostID); ok {
		return remoteplayLaunchResult{SessionID: existing.SessionID, Port: existing.Port, Existing: true}, true
	}

	// One active player-session per user, across platforms — same guard as
	// the retro launch (the pool is the single source of truth).
	if otherID, already := server.alreadyPlaying(body.Username); already {
		server.writeAlreadyPlaying(w, otherID)

		return remoteplayLaunchResult{}, false
	}
	if owned, players, goesOn := server.ownedGameGoesOn(body.Username); goesOn {
		server.writeOwnedGameGoesOn(w, owned, players)

		return remoteplayLaunchResult{}, false
	}

	// Temporary per-platform cap (étape 4).
	release, reserved := server.reserveSpawn(w, "remoteplay")
	if !reserved {
		return remoteplayLaunchResult{}, false
	}
	defer release()

	record, err := server.manager.LaunchRemoteplay(r.Context(), body.SessionID, body.HostID, body.HostName, body.HostType, body.Username)
	if err != nil {
		switch {
		case errors.Is(err, manager.ErrInvalidUsername):
			writeError(w, http.StatusBadRequest, "invalid username")
		case errors.Is(err, manager.ErrDeviceLimitReached):
			writeError(w, http.StatusConflict, err.Error())
		default:
			slog.Error("remoteplay launch failed", "error", err, "username", body.Username, "host_id", body.HostID)
			writeError(w, http.StatusInternalServerError, "session could not be started")
		}

		return remoteplayLaunchResult{}, false
	}

	// Register the pool entry NOW — IsAlreadyPlaying must be airtight for a
	// second launch before the bridge's next sync (the bridge adopts the
	// entry on its next notification, same contract as the retro kind).
	// PadUsers: the owner holds pad 1 from the start — the kernel is the
	// control authority (B2), so the initial holder is known here, not only
	// in the child.
	server.pool.Register(registry.Session{
		ID:         record.SessionID,
		Platform:   "remoteplay",
		ConsoleID:  record.HostType,
		Reference:  record.HostID,
		Title:      record.HostName,
		Initiator:  record.Owner,
		StartedAt:  record.StartedAt,
		MaxPlayers: 1,
		Players:    []string{record.Owner},
		PadUsers:   map[int]string{1: record.Owner},
	})

	return remoteplayLaunchResult{SessionID: record.SessionID, Port: record.Port}, true
}

// handleInternalRemoteplayLookup — GET /internal/remoteplay/sessions/{id}.
func (server *Server) handleInternalRemoteplayLookup(w http.ResponseWriter, r *http.Request) {
	record, ok := server.manager.LookupRemoteplay(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, record)
}
