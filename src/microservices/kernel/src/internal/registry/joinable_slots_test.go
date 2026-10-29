package registry

import "testing"

// Un siège vide n'est pas un siège libre quand il est à quelqu'un : la
// manette 1 est celle du propriétaire, et une partie à une seule manette ne
// se rejoint jamais, elle se regarde.
func TestPadOneIsNeverJoinable(t *testing.T) {
	pool := NewPool()
	pool.Register(Session{ID: "solo", Platform: "remoteplay", Initiator: "gino", MaxPlayers: 1})

	if slots := JoinableSlots(mustLookup(t, pool, "solo")); slots != 0 {
		t.Fatalf("places à rejoindre sur une partie à une manette = %d, attendu 0", slots)
	}
}

// Sur une partie à plusieurs manettes, seules les manettes 2 à n comptent.
func TestJoinableSlotsCountPadsTwoAndUp(t *testing.T) {
	pool := NewPool()
	pool.Register(Session{ID: "party", Platform: "retrogaming", Initiator: "gino", MaxPlayers: 4, Players: []string{"gino"}})

	if slots := JoinableSlots(mustLookup(t, pool, "party")); slots != 3 {
		t.Fatalf("places à rejoindre = %d, attendu 3 (manettes 2 à 4)", slots)
	}
}

func mustLookup(t *testing.T, pool *Pool, id string) Session {
	t.Helper()
	session, ok := pool.Lookup(id)
	if !ok {
		t.Fatalf("session %q introuvable", id)
	}

	return session
}
