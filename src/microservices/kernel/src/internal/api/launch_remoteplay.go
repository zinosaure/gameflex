package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gameflex/kernel/internal/auth"
)

// remoteplayControlClient — le plan interne vers le service remote-play.
//
// Trente-cinq secondes : la poignée de main Takion en prend jusqu'à trente
// côté runtime (`startChildSession` s'y donne ce délai), et un client plus
// pressé que le geste qu'il déclenche abandonne un lancement qui aboutissait.
var remoteplayControlClient = &http.Client{Timeout: 35 * time.Second}

// askRemoteplayControl — un POST interne, signé du secret partagé.
//
// Même patron que les huit autres appels internes du kernel
// (`federation_api.go`, `idle.go`, `mounts_client.go`…) : `X-Internal-Secret`
// en en-tête, jamais dans l'URL. `out` peut être nil quand la réponse n'a rien
// à dire au-delà de son code.
func (server *Server) askRemoteplayControl(path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	request, err := http.NewRequest(http.MethodPost, server.config.RemoteplayControlURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s: %w", path, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	response, err := remoteplayControlClient.Do(request)
	if err != nil {
		return fmt.Errorf("remote play control plane unreachable: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode >= 300 {
		var failure struct {
			ErrorMessage string `json:"errorMessage"`
		}
		_ = json.NewDecoder(response.Body).Decode(&failure)
		if failure.ErrorMessage != "" {
			return fmt.Errorf("%s", failure.ErrorMessage)
		}

		return fmt.Errorf("%s answered %d", path, response.StatusCode)
	}

	if out == nil {
		return nil
	}

	// LE RUNTIME ENVELOPPE SES SUCCÈS, PAS SES ERREURS.
	//
	// `writeJSON` (remoteplay, internal/api/handlers.go) rend toujours
	// `{"success":…, "data":…}`, tandis que `writeError` met `errorMessage` au
	// premier niveau. D'où l'asymétrie de cette fonction : le bloc d'erreur
	// ci-dessus décode à plat, celui-ci déballe.
	//
	// CE QUI ARRIVE QUAND ON L'OUBLIE, et c'est arrivé (2026-09-19) : décoder
	// un succès à plat ne produit AUCUNE erreur — Go laisse simplement la
	// structure vide quand les champs manquent. La porte résolvait donc
	// l'appareil, recevait un `hostType` vide, et `launchRemoteplay` refusait
	// avec « hostId and hostType are required », par un `writeError` sans
	// ligne de journal : aucune PS5 ne se lançait et rien ne disait pourquoi.
	// `remoteplayDevices` (federation_devices.go) déballait déjà correctement ;
	// c'est ce voisin-là qu'il fallait relire avant d'écrire celui-ci.
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("%s answered without data", path)
	}

	return json.Unmarshal(envelope.Data, out)
}

// handleRemoteplayThroughDoor — la branche `remoteplay` de la porte unique.
//
// LE SENS EST INVERSÉ, ET C'EST TOUT L'OBJET (#195). Avant, le service
// remote-play menait : il cherchait l'appareil, demandait au kernel de créer le
// conteneur, puis faisait la poignée de main. Ici le kernel mène — mais il ne
// prend pas pour autant ce qu'il n'a pas à connaître.
//
// LES CLÉS D'APPAIRAGE NE TRAVERSENT JAMAIS LE KERNEL. La résolution ne rend
// que des métadonnées (`rp_key`, `regist_key` et l'identifiant PSN sont marqués
// `json:"-"` dans le modèle du runtime, donc ils ne peuvent pas sortir même par
// mégarde), et c'est le runtime qui exécute la poignée de main, en lisant ces
// clés dans sa propre base. Le kernel ne voit qu'un port.
//
// POURQUOI DEUX APPELS. `hostType` est structurant côté kernel : il décide du
// plafond de sessions par appareil et du nom du conteneur
// (`manager.LaunchRemoteplay`). Il faut donc le connaître AVANT de lancer,
// tandis que la poignée de main ne peut venir qu'APRÈS, puisqu'elle a besoin du
// port. Deux moments, deux appels.
//
// C'EST LE SEUL CHEMIN, DEPUIS LE 2026-09-19. La page poste sur la porte, et
// l'ancienne route publique — avec son handler, la route interne qu'elle
// appelait et la direction qu'elle servait — est supprimée de bout en bout,
// une fois la bascule éprouvée sur une vraie PS5.
func (server *Server) handleRemoteplayThroughDoor(w http.ResponseWriter, r *http.Request, raw []byte) {
	var body struct {
		HostID   string `json:"hostId"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.HostID == "" {
		writeError(w, http.StatusBadRequest, "hostId is required")

		return
	}

	// Le JWT fait foi, comme partout ailleurs : le nom du corps n'est qu'un
	// repli pour les appelants qui l'envoient encore.
	username := auth.Username(r.Context())
	if username == "" {
		username = body.Username
	}

	var device struct {
		DeviceID string `json:"deviceId"`
		HostName string `json:"hostName"`
		HostType string `json:"hostType"`
	}
	if err := server.askRemoteplayControl("/internal/remoteplay/devices/resolve",
		map[string]string{"hostId": body.HostID}, &device); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	// LES CONSOLES D'UN COMPTE (chantier 20) : la console Remote Play n'est
	// connue qu'ici, une fois l'appareil résolu (PS4, PS5 → rp-ps4, rp-ps5) —
	// et AVANT la reprise d'une partie en cours, qui se rendait à quiconque la
	// demandait.
	if consoleID := "rp-" + strings.ToLower(device.HostType); !server.consoleGranted(r, consoleID) {
		writeNotGranted(w, consoleID)

		return
	}

	result, ok := server.launchRemoteplay(w, r, remoteplayLaunchBody{
		SessionID: device.DeviceID,
		HostID:    body.HostID,
		HostName:  device.HostName,
		HostType:  device.HostType,
		Username:  username,
	})
	if !ok {
		return
	}

	// REPRISE : la session tournait déjà. Refaire la poignée de main
	// relancerait Takion sous une partie en cours — le contrat que l'ancien
	// chemin respectait déjà en sautant son `startChildSession` quand le kernel
	// répondait « existing ».
	if result.Existing {
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": result.SessionID, "existing": true})

		return
	}

	if err := server.askRemoteplayControl("/internal/remoteplay/handshake", map[string]any{
		"deviceId": device.DeviceID,
		"port":     result.Port,
		"username": username,
	}, nil); err != nil {
		// LE CONTENEUR MEURT AVEC LA POIGNÉE DE MAIN RATÉE.
		//
		// On vient de le créer, et la console n'a pas répondu : le laisser
		// tourner serait une fuite — il occupe un port, un bloc ICE et une
		// place au plafond de la plateforme, pour une partie qui n'a jamais
		// commencé. L'ancien chemin mené par le runtime le faisait déjà
		// (`StopRemoteplaySession` juste avant de rendre son erreur) ; la
		// porte l'avait perdu, et comme elle devient le SEUL chemin, plus
		// personne ne nettoierait.
		//
		// Le code de retour est ignoré à dessein : l'échec qu'on rapporte est
		// celui de la console, pas celui du ménage, et masquer le premier par
		// le second ferait perdre la seule information utile au joueur.
		server.manager.StopRemoteplay(r.Context(), result.SessionID)
		// Le conteneur tourne mais la console n'a pas répondu : on le dit, et
		// on ne prétend pas que la partie a commencé.
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessionId": result.SessionID})
}
