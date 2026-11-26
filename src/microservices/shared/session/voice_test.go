package session

import (
	"errors"
	"testing"
)

// voiceOfTab — ce que la page de cet onglet lit de sa voix (you.voice).
func voiceOfTab(t *testing.T, rec *recorder, socketID string) map[string]any {
	t.Helper()

	return you(t, rec, socketID)["voice"].(map[string]any)
}

// submitVoice — un geste de voix, venu de cet onglet, et ce qu'il est devenu.
func submitVoice(host *Host, kind CommandKind, username, socketID string, on bool) Outcome {
	target := "off"
	if on {
		target = "on"
	}

	return host.Do(Command{Kind: kind, Username: username, SocketID: socketID, Target: target}, now)
}

// SEUL, RIEN NE S'OUVRE (le porteur, 2026-11-23) : le micro refusé, la page le sait (« alone »), et le propriétaire ne
// peut pas ouvrir la conversation aux spectateurs. Entendre est vrai d'emblée, le micro jamais.
func TestVoiceAloneOpensNothing(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Do(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	voice := voiceOfTab(t, rec, "sock-G")
	if voice["usable"] != false || voice["reason"] != "alone" || voice["mic"] != false || voice["hear"] != true || voice["canOpen"] != false {
		t.Fatalf("seul : %v", voice)
	}
	if outcome := submitVoice(host, CmdVoiceMic, "gino", "sock-G", true); !errors.Is(outcome.Err, ErrVoiceAlone) {
		t.Fatalf("micro seul : %v", outcome.Err)
	}
	// Un second onglet du même compte n'est pas quelqu'un d'autre.
	host.Attach("sock-G2", "gino")
	if outcome := submitVoice(host, CmdVoiceMic, "gino", "sock-G", true); !errors.Is(outcome.Err, ErrVoiceAlone) {
		t.Fatalf("micro avec son propre onglet : %v", outcome.Err)
	}
}

// UN JOUEUR REJOINT : chacun peut ouvrir son micro ; la file donne à la passerelle de quoi relayer la voix de l'un vers
// l'autre, jamais vers soi, et rien à qui ne veut pas entendre.
func TestVoiceTwoPlayersTalk(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Do(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Do(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)

	if outcome := submitVoice(host, CmdVoiceMic, "gino", "sock-G", true); outcome.Err != nil {
		t.Fatalf("micro : %v", outcome.Err)
	}
	rights := host.VoiceRights()
	if rights["sock-G"] != (VoiceRights{Talk: true, Hear: true}) || rights["sock-D"] != (VoiceRights{Talk: false, Hear: true}) {
		t.Fatalf("droits : %v", rights)
	}
	if voice := voiceOfTab(t, rec, "sock-D"); voice["usable"] != true || voice["mic"] != false {
		t.Fatalf("dave : %v", voice)
	}
	state := rec.last("sock-D", "state")
	if talkers := state["voice"].(map[string]any)["talkers"].([]string); len(talkers) != 1 || talkers[0] != "gino" {
		t.Fatalf("qui parle : %v", talkers)
	}

	// Dave n'entend plus : son micro à lui n'y change rien, celui de Gino non plus.
	submitVoice(host, CmdVoiceHear, "dave", "sock-D", false)
	if rights := host.VoiceRights(); rights["sock-D"].Hear || !rights["sock-G"].Talk {
		t.Fatalf("dave n'entend plus : %v", rights)
	}

	// Dave part : Gino n'a plus personne à qui parler, son micro se ferme — et ne se rouvrira pas seul.
	host.Detach("sock-D", now)
	if voice := voiceOfTab(t, rec, "sock-G"); voice["mic"] != false || voice["reason"] != "alone" {
		t.Fatalf("après le départ : %v", voice)
	}
	host.Attach("sock-D", "dave")
	if rights := host.VoiceRights(); rights["sock-G"].Talk {
		t.Fatalf("micro rouvert sans le vouloir : %v", rights)
	}
}

// LES SPECTATEURS, C'EST LE PROPRIÉTAIRE QUI DÉCIDE : fermée, la conversation leur refuse micro et son (et la
// passerelle ne leur relaie rien) ; ouverte, ils parlent et entendent ; refermée, leur micro se coupe aussitôt. Seul le
// propriétaire l'ouvre ; un onglet sans compte n'en est jamais.
func TestVoiceSpectatorsFollowTheOwner(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-S", "sam")
	host.Attach("sock-A", "")
	host.Do(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	if voice := voiceOfTab(t, rec, "sock-G"); voice["canOpen"] != true || voice["reason"] != "alone" {
		t.Fatalf("propriétaire avec un spectateur : %v", voice)
	}
	if voice := voiceOfTab(t, rec, "sock-S"); voice["usable"] != false || voice["reason"] != "closed" {
		t.Fatalf("spectateur, fermée : %v", voice)
	}
	if outcome := submitVoice(host, CmdVoiceMic, "sam", "sock-S", true); !errors.Is(outcome.Err, ErrVoiceClosed) {
		t.Fatalf("micro du spectateur, fermée : %v", outcome.Err)
	}
	if rights := host.VoiceRights(); rights["sock-S"] != (VoiceRights{}) {
		t.Fatalf("droits du spectateur, fermée : %v", rights)
	}
	if outcome := submitVoice(host, CmdVoiceSpectators, "sam", "sock-S", true); !errors.Is(outcome.Err, ErrNotOwner) {
		t.Fatalf("un spectateur ouvre : %v", outcome.Err)
	}

	submitVoice(host, CmdVoiceSpectators, "gino", "sock-G", true)
	if outcome := submitVoice(host, CmdVoiceMic, "sam", "sock-S", true); outcome.Err != nil {
		t.Fatalf("micro du spectateur, ouverte : %v", outcome.Err)
	}
	if rights := host.VoiceRights(); rights["sock-S"] != (VoiceRights{Talk: true, Hear: true}) || rights["sock-G"] != (VoiceRights{Talk: false, Hear: true}) || rights["sock-A"] != (VoiceRights{}) {
		t.Fatalf("ouverte : %v", rights)
	}

	submitVoice(host, CmdVoiceSpectators, "gino", "sock-G", false)
	if voice := voiceOfTab(t, rec, "sock-S"); voice["mic"] != false || voice["reason"] != "closed" {
		t.Fatalf("refermée : %v", voice)
	}
	if rights := host.VoiceRights(); rights["sock-S"] != (VoiceRights{}) {
		t.Fatalf("droits, refermée : %v", rights)
	}
}

// UN JOUEUR QUI REND SA MANETTE DEVIENT SPECTATEUR : sa voix suit la règle des spectateurs. Le propriétaire, lui, reste
// de la conversation même sans manette.
func TestVoiceFollowsTheRole(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Do(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)

	if outcome := submitVoice(host, CmdVoiceMic, "gino", "sock-G", true); outcome.Err != nil {
		t.Fatalf("le propriétaire sans manette : %v", outcome.Err)
	}
	submitVoice(host, CmdVoiceMic, "dave", "sock-D", true)
	host.Do(Command{Kind: CmdLeave, Username: "dave", SocketID: "sock-D"}, now)
	if rights := host.VoiceRights(); rights["sock-D"] != (VoiceRights{}) || rights["sock-G"].Talk {
		t.Fatalf("dave spectateur : %v", rights)
	}
}

// HUIT MICROS AU PLUS (VoiceSlots) : le neuvième est refusé ; en refermer un en libère la place.
func TestVoiceSlotsAreBounded(t *testing.T) {
	host, _ := newHost(t, 2)
	host.Attach("sock-G", "gino")
	submitVoice(host, CmdVoiceSpectators, "gino", "sock-G", true)
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	for _, name := range names {
		host.Attach("sock-"+name, name)
	}
	for index, name := range names {
		outcome := submitVoice(host, CmdVoiceMic, name, "sock-"+name, true)
		if index < VoiceSlots && outcome.Err != nil {
			t.Fatalf("micro %d : %v", index+1, outcome.Err)
		}
		if index == VoiceSlots && !errors.Is(outcome.Err, ErrVoiceFull) {
			t.Fatalf("micro %d : %v", index+1, outcome.Err)
		}
	}
	submitVoice(host, CmdVoiceMic, "a", "sock-a", false)
	if outcome := submitVoice(host, CmdVoiceMic, "i", "sock-i", true); outcome.Err != nil {
		t.Fatalf("une place libérée : %v", outcome.Err)
	}
}

// UNE NOUVELLE PARTIE (le téléphone, Claim) : la conversation se referme aux spectateurs et les micros se coupent.
func TestVoiceResetsWithANewGame(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Do(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)
	submitVoice(host, CmdVoiceSpectators, "gino", "sock-G", true)
	submitVoice(host, CmdVoiceMic, "gino", "sock-G", true)

	host.Claim("gino", 4, now)
	if host.Session().SpectatorsTalk || host.VoiceRights()["sock-G"].Talk {
		t.Fatal("la voix survit à la partie")
	}
}
