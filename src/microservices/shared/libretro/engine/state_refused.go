package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// UNE SAUVEGARDE QUE LE CŒUR REFUSE, ET POURQUOI (#396, 2026-10-19).
//
// Le porteur a chargé sur le S25 une sauvegarde PSP et lu « le cœur a refusé
// cette sauvegarde : unserialize failed » — sans rien pour comprendre. Elle
// avait été écrite par une préversion de PPSSPP que le buildbot de libretro a
// distribuée quelques jours (93035c6) ; le téléphone avait depuis la version
// officielle (f6aea4c), qui n'en relit pas trois sections. Le même refus
// attend une sauvegarde envoyée du téléphone au serveur tant que celui-ci a un
// PPSSPP plus ancien : chaque version relit les anciennes, pas les suivantes.
//
// Le moteur dit donc QUI refuse — le cœur installé et sa version — et, quand
// la sauvegarde le dit, QUI l'a écrite. Les deux hôtes en font un message
// codé (`state-refused`), que la page rend dans sa langue.
type StateRefusedError struct {
	// Core — le cœur installé : « PPSSPP f6aea4c ».
	Core string
	// StateCore — le cœur qui a écrit la sauvegarde, quand elle le dit
	// (PPSSPP l'inscrit dans son en-tête), vide sinon.
	StateCore string
}

func (refused *StateRefusedError) Error() string {
	if refused.StateCore != "" && refused.StateCore != refused.Core {
		return fmt.Sprintf("le cœur %s ne sait pas relire cette sauvegarde, faite par %s", refused.Core, refused.StateCore)
	}

	return fmt.Sprintf("le cœur %s a refusé cette sauvegarde", refused.Core)
}

// stateRefused — le refus du cœur chargé, pour ces octets.
func stateRefused(system coreIdentity, data []byte) *StateRefusedError {
	refused := &StateRefusedError{Core: strings.TrimSpace(system.name + " " + system.version)}
	if origin := ppssppStateOrigin(data); origin != "" {
		refused.StateCore = "PPSSPP " + origin
	}

	return refused
}

// coreIdentity — le nom et la version que le cœur donne de lui-même
// (retro_get_system_info).
type coreIdentity struct {
	name    string
	version string
}

// ppssppStateOrigin — la version de PPSSPP qui a commencé la chaîne de cette
// sauvegarde, lue dans son en-tête ; "" si ce n'est pas une sauvegarde de
// PPSSPP.
//
// Le format est celui de PPSSPP (PointerWrap, Common/Serialize) : la section
// `SaveStart` ouvre l'état — un marqueur de 16 octets complété de zéros, sa
// version en entier de 32 bits, puis à partir de la version 2 un compteur de
// sauvegardes et la version de PPSSPP qui a écrit la PREMIÈRE de la chaîne
// (`saveStateInitialGitVersion`) : sa longueur sur 32 bits, zéro final
// compris, puis ses octets. Tout petit-boutien.
//
// La première de la chaîne, pas forcément la dernière : une sauvegarde
// rechargée puis réécrite par une autre version garde l'origine de la
// première. C'est assez pour nommer le cas qui compte — une sauvegarde venue
// d'une autre version que celle installée.
func ppssppStateOrigin(data []byte) string {
	const header = 28
	if len(data) < header || string(bytes.TrimRight(data[:16], "\x00")) != "SaveStart" {
		return ""
	}
	if binary.LittleEndian.Uint32(data[16:20]) < 2 {
		return ""
	}
	length := int(binary.LittleEndian.Uint32(data[24:28]))
	if length <= 0 || length > 64 || header+length > len(data) {
		return ""
	}

	return strings.TrimRight(string(data[header:header+length]), "\x00")
}
