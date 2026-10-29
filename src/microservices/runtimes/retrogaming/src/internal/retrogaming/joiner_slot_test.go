package retrogaming

import "testing"

// releaseJoinerSlot ne touche que des champs Go ordinaires (mu, joiners,
// viewerPorts) — aucun cœur libretro dans l'affaire, donc un
// `&emulationSession{}` nu exerce la vraie logique de production, comme
// pause_state_test.go juste à côté. Relâcher les touches du port libéré est le
// travail du moteur (engine.ClearInput, testé dans engine/input_test.go) : sans
// moteur, une session de test n'en a aucune à relâcher.
//
// Ce que ces cas protègent est un défaut VÉCU (2026-09-19) : le chemin
// passerelle « ce viewer est parti » vide le créneau, et neuf secondes plus
// tard la boucle de lecture du joiner a déréférencé ce créneau déjà nul. Le
// panic a laissé `mu` tenu à jamais, l'enfant s'est bloqué dans son propre
// shutdown(), et le conteneur a survécu 16 h à l'écoute sans répondre.
func newSlotTestSession(ports int) *emulationSession {
	return &emulationSession{
		joiners:     make([]*joinerConn, ports),
		viewerPorts: map[string]int{},
	}
}

func TestReleaseJoinerSlotFreesItsOwn(t *testing.T) {
	s := newSlotTestSession(3)
	mine := &joinerConn{username: "alice", ViewerID: "viewer-a"}
	s.joiners[0] = mine
	s.viewerPorts["viewer-a"] = 1

	if !s.releaseJoinerSlot(1, mine) {
		t.Fatalf("releaseJoinerSlot = false, want true — le créneau nous appartient")
	}
	if s.joiners[0] != nil {
		t.Errorf("joiners[0] = %v, want nil", s.joiners[0])
	}
	if _, still := s.viewerPorts["viewer-a"]; still {
		t.Errorf("viewerPorts garde encore « viewer-a » — l'entrée doit partir avec le créneau")
	}
}

// LE CAS QUI A PANIQUÉ : le créneau a déjà été vidé par le chemin passerelle.
func TestReleaseJoinerSlotAlreadyCleared(t *testing.T) {
	s := newSlotTestSession(3)
	mine := &joinerConn{username: "alice"}
	// s.joiners[0] reste nil : « le viewer est parti » est passé avant nous.

	if s.releaseJoinerSlot(1, mine) {
		t.Errorf("releaseJoinerSlot = true, want false — il n'y a rien à libérer")
	}
}

// LE CAS QUE « != nil » N'AURAIT PAS ATTRAPÉ : le port a été repris.
func TestReleaseJoinerSlotTakenBySomeoneElse(t *testing.T) {
	s := newSlotTestSession(3)
	mine := &joinerConn{username: "alice", ViewerID: "viewer-a"}
	theirs := &joinerConn{username: "bob", ViewerID: "viewer-b"}
	s.joiners[0] = theirs
	s.viewerPorts["viewer-b"] = 1

	if s.releaseJoinerSlot(1, mine) {
		t.Errorf("releaseJoinerSlot = true, want false — ce créneau est à quelqu'un d'autre")
	}
	if s.joiners[0] != theirs {
		t.Errorf("joiners[0] = %v, want le joiner de bob intact", s.joiners[0])
	}
	if _, still := s.viewerPorts["viewer-b"]; !still {
		t.Errorf("viewerPorts a perdu « viewer-b » — on a déconnecté un tiers")
	}
}

// Idempotence : le ménage passe par les chemins d'échec ET par la sortie de
// boucle. Un second appel ne doit rien casser.
func TestReleaseJoinerSlotTwiceIsHarmless(t *testing.T) {
	s := newSlotTestSession(3)
	mine := &joinerConn{username: "alice"}
	s.joiners[1] = mine

	if !s.releaseJoinerSlot(2, mine) {
		t.Fatalf("premier appel = false, want true")
	}
	if s.releaseJoinerSlot(2, mine) {
		t.Errorf("second appel = true, want false — le créneau est déjà libre")
	}
}

// Bornes : un port hors plage ou un pointeur nil ne doit pas paniquer — c'est
// précisément ce genre de déréférencement qui a coûté la session.
func TestReleaseJoinerSlotOutOfRange(t *testing.T) {
	s := newSlotTestSession(2)
	mine := &joinerConn{username: "alice"}

	for _, port := range []int{-1, 0, 3, 99} {
		if s.releaseJoinerSlot(port, mine) {
			t.Errorf("releaseJoinerSlot(%d) = true, want false", port)
		}
	}
	if s.releaseJoinerSlot(1, nil) {
		t.Errorf("releaseJoinerSlot(1, nil) = true, want false")
	}
}
