package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"gameflex/sessionstatus"
)

// handleCapabilities — GET /api/v1/capabilities : ce que CETTE installation
// sait faire, pour que la page taise le reste.
//
// La règle de la page est « l'hôte DÉCLARE, l'interface obéit »
// (webapp, libs/capabilities.js). Le téléphone servait déjà sa déclaration ;
// le serveur ne déclarait rien, la page gardait ses défauts, et montrait
// l'interrupteur Chromecast et l'écran TURN alors qu'aucun des deux n'est
// livré (#105 relevé le 2026-09-14, D5 ; route ajoutée le 2026-09-16).
//
// Sans l'enveloppe {success, data} de writeJSON : la page lit `capabilities`
// à la racine, comme chez le téléphone. Sans jeton : la page la lit avant la
// connexion, et elle ne dit rien de personne.
func (server *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	declared := server.capabilities()
	// UNE PAGE OUVERTE DANS L'APPLICATION D'UN INVITÉ (chantier 32) : le
	// téléphone qui a rejoint ce serveur la montre dans sa WebView, dont
	// l'agent porte « ; wv) ». Elle revient à l'accueil de l'application par
	// le lien réservé `gameflex://home` que la WebView intercepte — ce
	// serveur n'a pas de console derrière elle. Décidé par requête.
	inAppWebView := strings.Contains(r.UserAgent(), "; wv)")
	declared["appHomeLink"] = inAppWebView
	// L'APPLICATION EST TOUJOURS EN PLEIN ÉCRAN (le porteur, 2026-10-29 : « ce
	// switch ne sert à rien pour l'App […] C'est utile quand c'est dans un
	// navigateur ») : son interrupteur ne se montre qu'à un navigateur.
	declared["fullscreen"] = !inAppWebView
	_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": declared})
}

// capabilities — la déclaration du serveur. Les clés sont celles de la page ;
// une clé absente garde son défaut.
func (server *Server) capabilities() map[string]any {
	return map[string]any{
		"platform":            "web",
		"accounts":            true,
		"invites":             "accounts",
		"localLibrary":        false,
		"serverAdmin":         true,
		"spawnsSessions":      true,
		"pairsConsoles":       true,
		"issuesCatalogTokens": true,
		"fullscreen":          true,
		"pageAudio":           true,
		"rewind":              true,
		"nativeShare":         false,
		"screenOrientation":   false,
		// LA PAGE PROPOSE L'APPLICATION, partout sauf dans l'application :
		// celle que cette installation construit sur un banc d'essai, la
		// Release GitHub ailleurs (2026-10-26).
		"appDownload": true,
		// UN BANC D'ESSAI (GAMEFLEX_STAGING) : la page l'écrit à côté du nom.
		"staging": server.config.Staging,
		// LE STATUT DES JEUX SE RÈGLE ICI (chantier 30), comme sur le
		// téléphone depuis #471.
		"gameStatus": true,
		// LE MIROIR (GAMEFLEX_MIRROR_URL, chantier 29) : un lien externe, que
		// les écrans des émulateurs, des firmwares, des correctifs et des
		// paramètres montrent. Vide : ils ne proposent rien.
		"mirrorUrl": server.config.MirrorURL,
		// PAS LIVRÉS dans la 0.3.0 : le Chromecast (#124), TURN (#117) et le
		// bouton « Test » des manettes (#138).
		"castToTv":    false,
		"turnConfig":  false,
		"gamepadTest": false,
		// LES NOTIFICATIONS PUSH, REMISES (le porteur, 2026-10-15 : « push.js
		// : il faut le remettre »). L'écran s'abonne pour de bon, et la clé
		// VAPID du service d'identité survit à ses redémarrages.
		"pushNotifications": true,
		// CE QUE LA MACHINE OFFRE À SES ÉMULATEURS DE BUREAU (2026-10-23) :
		// le GPU que les sessions reçoivent, son processeur et sa mémoire. La
		// page le dit avant une partie lancée ici, et un serveur qui nous a
		// fédérés le lit ici pour le dire chez lui (remote.PeerResources).
		"resources": sessionstatus.ServerResources(server.config.GPU),
	}
}
