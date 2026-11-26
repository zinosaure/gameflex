package session

// LA VOIX D'UNE PARTIE (chantier 71 ; le porteur, 2026-11-23 : « Le micro et le son des joueurs, c'est eux qui
// décident. Le micro et le son des spectateurs, c'est le owner qui décide s'ils peuvent interagir »).
//
// Trois choix, et la file les tient comme le reste de la partie, pour les quatre moteurs (rétro, streamex, Remote Play,
// le téléphone) :
//   - le micro de chaque onglet (Participant.Mic) et s'il entend les autres (Participant.Hear) : à lui ;
//   - la conversation ouverte aux spectateurs (Session.SpectatorsTalk) : au propriétaire.
//
// LA RÈGLE SE FAIT RESPECTER AILLEURS QUE DANS LA PAGE : la file calcule ce que chaque onglet a le droit d'envoyer et de
// recevoir (VoiceRights), le moteur le transmet à la passerelle, et la passerelle ne relaie rien d'autre. Une page qui
// ouvre son micro sans droit n'est entendue par personne.
//
// Seul dans sa partie, rien ne s'ouvre : un micro n'a de sens que si quelqu'un d'autre l'entend.

// VoiceSlots — les micros ouverts en même temps, au plus : autant de pistes de voix la passerelle prévoit dans chaque
// page (gatewaystreams), pour ne jamais renégocier quand un micro s'ouvre.
const VoiceSlots = 8

// VoiceRights — ce qu'un onglet a le droit d'envoyer (Talk) et de recevoir (Hear), que la passerelle applique.
type VoiceRights struct {
	Talk bool `json:"talk"`
	Hear bool `json:"hear"`
}

// participantAt — l'onglet de ce socket, nil s'il n'est pas ouvert.
func (session *Session) participantAt(socketID string) *Participant {
	for index := range session.Participants {
		if session.Participants[index].SocketID == socketID {
			return &session.Participants[index]
		}
	}

	return nil
}

// voiceMember — cet onglet peut prendre part à la conversation : un compte, et un joueur (le propriétaire compris,
// même quand il regarde sans manette ; l'emprunteur de sa manette, qui joue), ou un spectateur quand le propriétaire
// a ouvert la conversation. Un onglet sans compte, jamais : la file ne sait pas qui il est.
func (session *Session) voiceMember(tab Participant) bool {
	if tab.Username == "" {
		return false
	}
	if session.IsOwner(tab.Username) || session.PlayerOf(tab.Username) != nil {
		return true
	}
	if loan := session.loan(); loan != nil && loan.LentToSocket == tab.SocketID {
		return true
	}

	return session.SpectatorsTalk
}

// voicePartner — quelqu'un d'AUTRE (un autre compte) peut prendre part à la conversation avec cet onglet. Les autres
// onglets du même compte ne comptent pas : on ne se parle pas à soi-même.
func (session *Session) voicePartner(tab Participant) bool {
	for _, other := range session.Participants {
		if other.Username != "" && other.Username != tab.Username && session.voiceMember(other) {
			return true
		}
	}

	return false
}

// someoneElse — un autre compte est dans la partie, joueur ou spectateur : le propriétaire peut alors ouvrir la
// conversation aux spectateurs. Seul, les trois choix sont désactivés.
func (session *Session) someoneElse(tab Participant) bool {
	for _, other := range session.Participants {
		if other.Username != "" && other.Username != tab.Username {
			return true
		}
	}

	return false
}

// openMics — les micros ouverts, tous onglets confondus.
func (session *Session) openMics() int {
	count := 0
	for _, tab := range session.Participants {
		if tab.Mic {
			count++
		}
	}

	return count
}

// setMic — ouvrir ou fermer le micro de cet onglet. Fermer est toujours permis ; ouvrir demande d'avoir la parole,
// quelqu'un à qui parler, et une piste de voix libre.
func (session *Session) setMic(socketID string, on bool) error {
	tab := session.participantAt(socketID)
	if tab == nil {
		return ErrSocketClosed
	}
	if !on {
		tab.Mic = false

		return nil
	}
	switch {
	case !session.voiceMember(*tab):
		return ErrVoiceClosed
	case !session.voicePartner(*tab):
		return ErrVoiceAlone
	case !tab.Mic && session.openMics() >= VoiceSlots:
		return ErrVoiceFull
	}
	tab.Mic = true

	return nil
}

// setHear — entendre les autres, ou non. Un choix de l'onglet, gardé même quand il n'a pas la parole : il vaut dès
// qu'il l'a.
func (session *Session) setHear(socketID string, on bool) error {
	tab := session.participantAt(socketID)
	if tab == nil {
		return ErrSocketClosed
	}
	tab.Hear = on

	return nil
}

// settleVoice — un micro ouvert se ferme dès que son onglet perd la parole (la conversation refermée aux spectateurs,
// une manette rendue) ou qu'il n'a plus personne à qui parler. Rouvert plus tard, il ne doit pas l'être sans que son
// propriétaire l'ait choisi de nouveau. Appelé avant chaque état diffusé (Host.stateMessages) : tous les chemins qui
// changent la partie passent par là, et aucun n'a à s'en souvenir.
func (session *Session) settleVoice() {
	for index := range session.Participants {
		tab := session.Participants[index]
		if tab.Mic && (!session.voiceMember(tab) || !session.voicePartner(tab)) {
			session.Participants[index].Mic = false
		}
	}
}

// VoiceRightsBySocket — ce que chaque onglet peut envoyer et recevoir, après settleVoice.
func (session *Session) VoiceRightsBySocket() map[string]VoiceRights {
	rights := make(map[string]VoiceRights, len(session.Participants))
	for _, tab := range session.Participants {
		member := session.voiceMember(tab)
		rights[tab.SocketID] = VoiceRights{
			Talk: member && tab.Mic,
			Hear: member && tab.Hear && session.voicePartner(tab),
		}
	}

	return rights
}

// voiceOf — ce que la page de cet onglet montre de la voix : ses deux choix, s'il peut s'en servir et sinon pourquoi
// (« closed » : la conversation n'est pas ouverte aux spectateurs ; « alone » : personne d'autre ne peut y prendre
// part), et — au propriétaire — s'il peut l'ouvrir aux spectateurs.
func (session *Session) voiceOf(tab Participant) map[string]any {
	reason := ""
	switch {
	case !session.voiceMember(tab):
		reason = "closed"
	case !session.voicePartner(tab):
		reason = "alone"
	}
	voice := map[string]any{
		"mic":    tab.Mic,
		"hear":   tab.Hear,
		"usable": reason == "",
		"reason": reason,
	}
	if session.IsOwner(tab.Username) {
		voice["canOpen"] = session.someoneElse(tab)
	}

	return voice
}

// voiceTalkers — qui a le micro ouvert, une fois par compte : la liste des joueurs du menu le montre.
func (session *Session) voiceTalkers() []string {
	seen := map[string]bool{}
	talkers := []string{}
	for _, tab := range session.Participants {
		if tab.Mic && tab.Username != "" && !seen[tab.Username] {
			seen[tab.Username] = true
			talkers = append(talkers, tab.Username)
		}
	}

	return talkers
}
