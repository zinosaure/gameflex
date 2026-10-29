// Remoteplay session records — the one-container-per-session model applied
// to PlayStation remoteplay (see GAMEFLEX_JOURNAL.md).
// Same discipline as the retro records: sessions spawns the container, the
// watchdog keeps records and live counts in sync, and the capacity rule is
// DATA per device type (PS4/PS5 stream one session today, Steam/Xbox may
// allow more when they land), never a hardcoded one-per-device in the code.
package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gameflex/kernel/internal/docker"
	"gameflex/sessionstatus"
)

// ErrDeviceLimitReached — the target device already runs its maximum
// number of concurrent streams (see maxSessionsPerDevice).
var ErrDeviceLimitReached = errors.New("device already at its session limit")

// maxSessionsPerDevice is the per-platform capacity data. Conservative
// default of 1 until a platform declares otherwise — the mechanism is
// generic, the values will be extended when Steam/Xbox land.
func maxSessionsPerDevice(hostType string) int {
	switch strings.ToUpper(hostType) {
	case "PS4", "PS5":
		return 1
	}
	return 1
}

// RemoteplaySession is one spawned remoteplay session container. The child
// receives the device payload over POST /session/start (the relay's job in
// the next migration step) and streams the console's own encoded video —
// no mounts, no GPU, no game files.
type RemoteplaySession struct {
	BaseSession
	HostID   string `json:"host_id"`
	HostName string `json:"host_name"`
	HostType string `json:"host_type"`
}

func (s *RemoteplaySession) base() *BaseSession { return &s.BaseSession }

// LaunchRemoteplay spawns one remoteplay session container. The
// cross-platform "one playing session per user" rule is enforced by the
// API layer through the pool (IsAlreadyPlaying); this function enforces the
// per-device capacity and the username shape.
func (manager *Manager) LaunchRemoteplay(ctx context.Context, sessionID, hostID, hostName, hostType, username string) (*RemoteplaySession, error) {
	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if hostID == "" {
		return nil, fmt.Errorf("host id is required")
	}
	if sessionID == "" {
		sessionID = newSessionID()
	}

	// Capacity is per DEVICE (one console accepts one Takion stream), not
	// per host type — two different PS5s must never block each other.
	if manager.runningRemoteplayCount(hostID) >= maxSessionsPerDevice(hostType) {
		return nil, ErrDeviceLimitReached
	}

	spec := docker.RemoteplaySpawnSpec{
		SessionID:      sessionID,
		HostID:         hostID,
		HostName:       hostName,
		HostType:       hostType,
		Username:       username,
		ContainerBase:  "remoteplay-" + strings.ToLower(hostType),
		ContainerIndex: manager.runningRemoteplayCount(hostID) + 1,
	}

	// Host networking means a port held by a lingering process kills
	// the child at bind time while docker start succeeds — only the
	// readiness poll sees it (WaitReadyHost's exit detection). On that
	// failure blacklist the port and retry on a fresh one, the same
	// discipline as the daemon-reported port conflict at start time.
	const spawnAttempts = 3
	for attempt := 0; attempt < spawnAttempts; attempt++ {
		port, containerName, err := manager.dockerHandler.CreateAndStartRemoteplay(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("spawn container: %w", err)
		}

		if err := manager.dockerHandler.WaitReadyHost(ctx, containerName, port, 60*time.Second); err != nil {
			_ = manager.dockerHandler.Stop(context.Background(), containerName)
			if !errors.Is(err, docker.ErrChildExited) {
				return nil, fmt.Errorf("spawn container: %w", err)
			}
			manager.dockerHandler.BlacklistPort(port)
			slog.Warn("remoteplay child exited before ready, retrying on a fresh port", "session_id", sessionID, "container", containerName, "port", port, "error", err)
			continue
		}

		record := &RemoteplaySession{
			BaseSession: BaseSession{
				SessionID: spec.SessionID,
				Container: containerName,
				Owner:     username,
				Port:      port,
				StartedAt: time.Now(),
				State:     "running",
			},
			HostID:   hostID,
			HostName: hostName,
			HostType: hostType,
		}

		manager.remoteplay.register(record)
		manager.notify()

		slog.Info("remoteplay session launched", "session_id", record.SessionID, "container", record.Container, "host_type", hostType, "host_id", hostID, "owner", username, "port", port)
		return record, nil
	}

	return nil, fmt.Errorf("spawn container: child kept exiting before ready")
}

// StopRemoteplay removes the session container and the record. Reports
// whether the session existed.
func (manager *Manager) StopRemoteplay(ctx context.Context, sessionID string) bool {
	ok := stopSession(manager.remoteplay, manager.dockerHandler, ctx, sessionID, "remoteplay")
	if ok {
		manager.baselines.Forget(sessionID)
		manager.notify()
	}
	return ok
}

// SweepRemoteplay re-runs the watchdog's checks for ONE session — the
// relay will call it after the owner disconnects (same flow as
// SweepRetro). Idempotent: an unknown session is a no-op.
func (manager *Manager) SweepRemoteplay(ctx context.Context, sessionID string) {
	record, ok := manager.remoteplay.lookup(sessionID)
	if !ok {
		return
	}

	running, err := manager.dockerHandler.IsRunning(ctx, record.Container)
	if err != nil || !running {
		manager.StopRemoteplay(ctx, sessionID)
		return
	}

	status, err := manager.dockerHandler.StatusRemoteplay(ctx, record.Container, record.Port)
	if err != nil {
		return
	}
	if manager.remoteplay.patchPlayerCount(sessionID, status.Players) {
		manager.notify()
	}
}

// LookupRemoteplay returns a copy of one remoteplay session record.
func (manager *Manager) LookupRemoteplay(sessionID string) (RemoteplaySession, bool) {
	return manager.remoteplay.lookup(sessionID)
}

// LookupRemoteplayByHost returns the running session for one device —
// remoteplay's idempotent-relaunch key: the device id IS the session id,
// so an already-running session for this host is the session itself (the
// resume path returns it instead of a 409). Not a generic store method
// (the other two platforms have no equivalent "look up by a non-ID field"
// need) — stays a plain scan here.
func (manager *Manager) LookupRemoteplayByHost(hostID string) (RemoteplaySession, bool) {
	for _, record := range manager.remoteplay.list() {
		if record.HostID == hostID && record.State == "running" {
			return record, true
		}
	}
	return RemoteplaySession{}, false
}

// ListRemoteplay returns remoteplay sessions sorted by start time (oldest
// first) — same deterministic order as List/ListRetro.
func (manager *Manager) ListRemoteplay() []RemoteplaySession {
	return manager.remoteplay.list()
}

func (manager *Manager) runningRemoteplayCount(hostID string) int {
	return manager.remoteplay.runningCount(func(r RemoteplaySession) bool { return r.HostID == hostID })
}

// watchdogRemoteplay keeps remoteplay records in sync with reality on the
// same tick cadence as watchdog(): exited containers drop their records
// and the live player count is mirrored. The idle shutdown lives in the
// kernel's unified pool watchdog (api/idle.go, étape 3) — this loop used
// to run its own remoteplayIdleStopAfter rule.
//
// Kept as its OWN ticker rather than joining watchdog()'s loop — pre-existing
// asymmetry, not something this refactor changes (see the plan's note on
// asymmetries to preserve rather than silently "fix").
func (manager *Manager) watchdogRemoteplay() {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()

		for _, session := range manager.ListRemoteplay() {
			running, err := manager.dockerHandler.IsRunning(ctx, session.Container)
			if err != nil || !running {
				manager.StopRemoteplay(ctx, session.SessionID)
				continue
			}

			status, err := manager.dockerHandler.StatusRemoteplay(ctx, session.Container, session.Port)
			if err != nil {
				continue
			}

			if manager.remoteplay.patchPlayerCount(session.SessionID, status.Players) {
				manager.notify()
			}
		}
	}
}

// RemoteplayStatus — le pendant de RetroStatus/StreamexStatus pour la PS4/PS5,
// dans la MÊME forme (sessionstatus.Status). Voir session_status côté partagé
// pour pourquoi une seule forme, et RetroStatus pour pourquoi la traduction
// vit ici : l'heure de démarrage et la mémoire vue par Docker ne sont connues
// que du kernel.
//
// Le GPU n'y figure pas, et ce n'est pas un oubli : une session remote-play
// n'émule rien: elle relaie une console qui, elle, est dans le salon. La carte
// graphique de CE serveur ne dit donc rien sur ce que fait le jeu.
func (manager *Manager) RemoteplayStatus(sessionID string) (sessionstatus.Status, bool) {
	record, ok := manager.remoteplay.lookup(sessionID)
	if !ok {
		return sessionstatus.Status{}, false
	}

	status, err := manager.dockerHandler.StatusRemoteplay(context.Background(), record.Container, record.Port)
	if err != nil {
		return sessionstatus.Status{}, false
	}

	out := sessionstatus.Status{
		StartedAt:  record.StartedAt,
		Players:    status.Players,
		Spectators: status.Spectators,
		CPUPercent: sessionstatus.Percent(status.CPUPercent),
		RSSBytes:   status.RSSBytes,
	}
	out.ContainerRSSBytes, out.ContainerLimitBytes = manager.dockerHandler.ContainerMemory(
		context.Background(), record.Container,
	)
	// La machine qui relaie : sa chaleur et sa batterie, pas celles de la
	// console du salon, qui ne les dit pas.
	sessionstatus.ServerPower(&out)
	manager.baselines.Stamp(sessionID, &out)

	return out, true
}
