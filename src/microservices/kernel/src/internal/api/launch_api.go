package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// maxLaunchBodyBytes — un corps de lancement tient en quelques centaines
// d'octets ; la limite protège la relecture ci-dessous d'un corps qui n'en
// finirait pas.
const maxLaunchBodyBytes = 64 << 10

// handleUnifiedLaunch — POST /api/v1/launch.
//
// UNE SEULE PORTE, ET LE KERNEL AIGUILLE. Demande de l'utilisateur, après avoir
// remarqué que le lancement était namespacé « alors qu'il y a des choses en
// commun entre rétro/streamex et rétro/streamex/remote ». C'est exact, et le
// dépôt l'avouait déjà au-dessus de la route streamex : son ancien chemin sans
// préfixe *« looking like a generic fallback it never was »*.
//
// CE N'EST PAS UNE RÉÉCRITURE, ET C'EST LE POINT.
//
// Le corps est relu tel quel par le handler de la plateforme, qui garde ses
// gardes (`IsAlreadyPlaying`, `reserveSpawn`), ses publications d'étapes et
// sa taxonomie d'erreurs d'image. Recopier leur contenu ici aurait créé un
// SECOND endroit où corriger chaque défaut — et c'est exactement le désordre
// que ce chantier vient réparer, pas un désordre à déplacer.
//
// `r.Body` ne se lit qu'une fois : on le remet donc en place avant de passer la
// main, sinon le handler délégué décoderait un corps vide et refuserait tout.
func (server *Server) handleUnifiedLaunch(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxLaunchBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	var envelope struct {
		Platform string `json:"platform"`
		// La console — `consoleId` au rétro, `console_id` à streamex. Celle
		// d'une console Remote Play n'est connue qu'une fois l'appareil résolu
		// (launch_remoteplay.go).
		ConsoleID      string `json:"consoleId"`
		ConsoleIDSnake string `json:"console_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))

	// LES CONSOLES D'UN COMPTE (chantier 20) : un compte ne lance que celles
	// qu'on lui a données — aucune, et il ne joue pas ici (accounts_cut.go).
	//
	// DEUX CLÉS QUI DISENT DEUX CONSOLES : refusé. La porte lisait `consoleId`
	// d'abord, le rétro lance sur `console_id` d'abord et streamex ne lit que
	// lui : `{"consoleId":"nes","console_id":"psx"}` faisait vérifier nes et
	// lancer psx (revue de sécurité, 2026-10-28).
	if envelope.ConsoleID != "" && envelope.ConsoleIDSnake != "" && envelope.ConsoleID != envelope.ConsoleIDSnake {
		writeError(w, http.StatusBadRequest, "consoleId and console_id name two different consoles")

		return
	}
	consoleID := envelope.ConsoleID
	if consoleID == "" {
		consoleID = envelope.ConsoleIDSnake
	}
	if envelope.Platform != "remoteplay" && !server.consoleGranted(r, consoleID) {
		writeNotGranted(w, consoleID)

		return
	}

	switch envelope.Platform {
	case "retrogaming":
		server.handleRetroLaunch(w, r)
	case "streamex":
		server.handleLaunch(w, r)
	case "remoteplay":
		// LE SENS EST INVERSÉ : le kernel lance, puis DEMANDE au runtime la
		// poignée de main Takion sur le port obtenu — pour que les clés
		// d'appairage de la console ne traversent jamais le kernel. Voir
		// `launch_remoteplay.go`, qui explique pourquoi il faut deux appels.
		server.handleRemoteplayThroughDoor(w, r, raw)
	default:
		writeError(w, http.StatusBadRequest, "unknown platform: "+envelope.Platform)
	}
}
