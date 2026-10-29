package retrogaming

import "testing"

// recomputePauseState/registerBroadcastTarget/unregisterBroadcastTarget/
// updateVisibility touch only plain Go fields (mu, broadcastTargets,
// paused) — no libretro/CGo core involved, so a bare &emulationSession{}
// exercises the real production logic directly, same as any other pure Go
// state machine.

func newTestSession() *emulationSession {
	return &emulationSession{}
}

func TestPauseStateWithNoTargets(t *testing.T) {
	s := newTestSession()
	s.recomputePauseState()
	if s.paused {
		t.Errorf("a session with zero connected targets must not be paused (the queue owns teardown, not pause)")
	}
}

func TestPauseStateSingleTargetHidden(t *testing.T) {
	s := newTestSession()
	s.registerBroadcastTarget("owner", "alice", func(streamMessage) {})

	s.updateVisibility("owner", false)
	if !s.paused {
		t.Errorf("paused = false, want true once the only connected target reports hidden")
	}

	s.updateVisibility("owner", true)
	if s.paused {
		t.Errorf("paused = true, want false once that target reports visible again")
	}
}

func TestPauseStateRequiresEveryoneHidden(t *testing.T) {
	s := newTestSession()
	s.registerBroadcastTarget("owner", "alice", func(streamMessage) {})
	s.registerBroadcastTarget("spectator-1", "bob", func(streamMessage) {})

	s.updateVisibility("owner", false)
	if s.paused {
		t.Errorf("paused = true, want false — the spectator is still visible")
	}

	s.updateVisibility("spectator-1", false)
	if !s.paused {
		t.Errorf("paused = false, want true — every connected target (owner AND spectator) is now hidden")
	}
}

func TestPauseStateResumesWhenHiddenTargetDisconnects(t *testing.T) {
	s := newTestSession()
	s.registerBroadcastTarget("owner", "alice", func(streamMessage) {})
	s.registerBroadcastTarget("spectator-1", "bob", func(streamMessage) {})
	s.updateVisibility("owner", false)
	s.updateVisibility("spectator-1", false)
	if !s.paused {
		t.Fatalf("setup failed: session should be paused with both targets hidden")
	}

	// A brand-new connection defaults to visible=true (see
	// registerBroadcastTarget's own doc) — joining/reconnecting must resume
	// immediately, without that new tab having to send its own explicit
	// "visibility: true" first.
	s.registerBroadcastTarget("joiner-1", "carol", func(streamMessage) {})
	if s.paused {
		t.Errorf("paused = true, want false — a newly-registered target defaults visible")
	}
}

func TestPauseStateUnregisterRecomputes(t *testing.T) {
	s := newTestSession()
	s.registerBroadcastTarget("owner", "alice", func(streamMessage) {})
	s.registerBroadcastTarget("spectator-1", "bob", func(streamMessage) {})
	s.updateVisibility("owner", false)
	s.updateVisibility("spectator-1", true)
	if s.paused {
		t.Fatalf("setup failed: session should not be paused while the spectator is visible")
	}

	// The last visible connection leaves — nobody visible remains.
	s.unregisterBroadcastTarget("spectator-1")
	if !s.paused {
		t.Errorf("paused = false, want true — the only still-visible target just disconnected")
	}
}
