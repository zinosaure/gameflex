package retrogaming

import "testing"

// sendToPort and onRumble touch only plain Go fields (mu, broadcastTargets) —
// no libretro/CGo core involved, same "bare &emulationSession{} exercises the
// real production logic directly" pattern pause_state_test.go already uses.
// Le masque des boutons et la combinaison des deux moteurs de vibration sont
// au moteur, et testés là (engine/input_test.go, engine/engine_test.go).

func TestSendToPortOwner(t *testing.T) {
	s := newTestSession()
	var got streamMessage
	s.registerBroadcastTarget("owner", "", func(m streamMessage) { got = m })

	if ok := s.sendToPort(0, streamMessage{Type: "rumble", RumbleWeak: 100}); !ok {
		t.Fatal("sendToPort(0, ...) = false, want true — port 0 is the owner, registered with an empty username on purpose (see sendToPort's own doc)")
	}
	if got.Type != "rumble" || got.RumbleWeak != 100 {
		t.Errorf("owner target received %+v, want a rumble message with RumbleWeak=100", got)
	}
}

func TestSendToPortJoiner(t *testing.T) {
	s := newTestSession()
	var got streamMessage
	s.registerBroadcastTarget("joiner-2", "bob", func(m streamMessage) { got = m })

	if ok := s.sendToPort(2, streamMessage{Type: "rumble", RumbleStrong: 200}); !ok {
		t.Fatal("sendToPort(2, ...) = false, want true — matches the joiner-2 registration key")
	}
	if got.RumbleStrong != 200 {
		t.Errorf("joiner-2 target received %+v, want RumbleStrong=200", got)
	}
}

func TestSendToPortNoConnection(t *testing.T) {
	s := newTestSession()
	if ok := s.sendToPort(3, streamMessage{Type: "rumble"}); ok {
		t.Error("sendToPort for a port with no registered connection = true, want false")
	}
}

// onRumble — le moteur a déjà combiné les deux moteurs (engine.rumbleState,
// testé dans le moteur) ; ici, le message part bien au propriétaire, les deux
// intensités ensemble.
func TestOnRumbleSendsBothMotorsToTheOwner(t *testing.T) {
	s := newTestSession()
	var got streamMessage
	s.registerBroadcastTarget("owner", "", func(m streamMessage) { got = m })

	s.onRumble(0, 3000, 5000)
	if got.Type != "rumble" || got.RumbleWeak != 3000 || got.RumbleStrong != 5000 {
		t.Errorf("owner target received %+v, want rumble weak=3000 strong=5000", got)
	}
}
