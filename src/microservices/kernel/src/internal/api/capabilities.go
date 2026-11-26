package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"gameflex/kernel/internal/config"
	"gameflex/logfile"
	"gameflex/sessionstatus"
)

// handleCapabilities — GET /api/v1/capabilities : ce que CETTE installation
// sait faire, pour que la page taise le reste.
//
// La règle de la page est « l'hôte DÉCLARE, l'interface obéit »
// (webapp, libs/capabilities.js). Le téléphone servait déjà sa déclaration ;
// le serveur ne déclarait rien, la page gardait ses défauts, et montrait
// l'interrupteur Chromecast alors qu'il n'est pas livré (#105 relevé le
// 2026-09-14, D5 ; route ajoutée le 2026-09-16).
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
	// LA FENÊTRE DU PC AUSSI (chantier 70, Rejoindre) : elle se nomme dans son agent (« GameFlexDesktop/ ») et
	// intercepte le même lien, qui la ramène à sa console. Elle a un plein écran : l'interrupteur lui reste.
	declared["appHomeLink"] = inAppWebView || strings.Contains(r.UserAgent(), " GameFlexDesktop/")
	// L'APPLICATION EST TOUJOURS EN PLEIN ÉCRAN (le porteur, 2026-10-29 : « ce
	// switch ne sert à rien pour l'App […] C'est utile quand c'est dans un
	// navigateur ») : son interrupteur ne se montre qu'à un navigateur.
	declared["fullscreen"] = !inAppWebView
	// LE CAST VERS LA TV, SUR TOUTES LES PLATEFORMES (le porteur, 2026-11-26 : « le cast to tv c'est toutes les
	// plateformes qui utilise ») : la page le propose quand l'administration a l'identifiant du récepteur enregistré
	// chez Google (`/api/v1/cast`). Il part du SDK Cast de Chrome. Les deux fenêtres de l'application, qui ne l'ont pas,
	// suivent le lien réservé `gameflex://cast` (`castLink`, #124) : la WebView du téléphone caste par l'application
	// (TvCast.java), la fenêtre du PC ouvre la page de cast dans le navigateur du système (window/main.js).
	declared["castToTv"] = true
	declared["castLink"] = inAppWebView || strings.Contains(r.UserAgent(), " GameFlexDesktop/")
	_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": declared})
}

// capabilities — la déclaration du serveur. Les clés sont celles de la page ;
// une clé absente garde son défaut.
func (server *Server) capabilities() map[string]any {
	return map[string]any{
		"platform":            "web",
		"accounts":            true,
		"invites":             "accounts",
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
		// LE NOM DE CE SERVEUR, celui que voient ses serveurs fédérés et la
		// découverte du réseau local (chantier 60) : la fenêtre d'un code
		// d'appairage le montre, pour qu'on vérifie le serveur qu'on vise.
		"serverName": server.federationHostname(),
		// LES IMAGES DES JEUX (chantier 49) : ce serveur va les chercher chez
		// IGDB et SteamGridDB, avec les comptes que l'administrateur saisit
		// dans Réglages.
		"metadataServices": true,
		// CE QUI S'ÉCRIT DANS SES JOURNAUX SE RÈGLE DE LA PAGE
		// (log_settings.go). Le téléphone le déclare aussi, pour le seul
		// journal de son kernel.
		"logSettings": true,
		// SA CONFIGURATION SE RÈGLE DE LA PAGE (Réglages › Configuration du serveur, server_config.go, chantier 81) : un
		// serveur seulement — le téléphone et le PC n'ont ni compose ni GPU des parties à choisir.
		"serverConfig": true,
		// LES SIGNALEMENTS S'ÉCRIVENT-ILS (Réglages › Journaux, log_settings.go) ?
		// Lu à chaque demande : éteints, l'écran d'erreur d'un lancement ne
		// propose plus « Signaler l'erreur ». Le téléphone ne le déclare pas :
		// les siens s'écrivent toujours, la page le prend vrai par défaut.
		"reports": logfile.Current().Reports,
		// LES NOTIFICATIONS PUSH, REMISES (le porteur, 2026-10-15 : « push.js
		// : il faut le remettre »). L'écran s'abonne pour de bon, et la clé
		// VAPID du service d'identité survit à ses redémarrages.
		"pushNotifications": true,
		// LES PAQUETS D'ÉMULATEURS QUE CET HÔTE FAIT TOURNER (chantier 81) : ceux d'un serveur ; la fiche d'une console
		// désactive l'autre liste (les paquets du téléphone) — une déclaration, jamais un test de plateforme dans la page.
		"emulatorPackage": "server",
		// CE QUE LA MACHINE OFFRE À SES ÉMULATEURS DE BUREAU (2026-10-23) :
		// le GPU que les sessions reçoivent, son processeur et sa mémoire. La
		// page le dit avant une partie lancée ici, et un serveur qui nous a
		// fédérés le lit ici pour le dire chez lui (remote.PeerResources).
		"resources": sessionstatus.ServerResources(config.CurrentLive().GPU),
	}
}
