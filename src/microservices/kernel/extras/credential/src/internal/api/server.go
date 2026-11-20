// Package api serves the auth service's HTTP surface: the exact routes and
// response envelope the frontend already calls on the Python service, so
// the webpack proxy can be re-pointed with zero frontend changes.
package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/presence"
	"gameflex/credential/internal/web_push"
)

type Server struct {
	config   config.Config
	db       *sql.DB
	presence *presence.Registry
	web_push *web_push.Client
	mux      *http.ServeMux
	// accounts — les kernels abonnés à l'état des comptes (accounts_ws.go).
	accounts *accountsHub
	// deleted — les comptes supprimés depuis peu, que la liste nomme au kernel
	// pour qu'il efface leurs sauvegardes (accounts_ws.go).
	deleted deletedAccounts
	// logins — les essais de mot de passe ratés, qui font attendre (#454).
	logins *loginLimiter
	// pairing — les modes découverte et les codes d'appairage (chantier 60,
	// pairing.go), en mémoire.
	pairing *pairing
}

func NewServer(config config.Config, db *sql.DB, registry *presence.Registry, pushClient *web_push.Client) *Server {
	server := &Server{config: config, db: db, presence: registry, web_push: pushClient, mux: http.NewServeMux(), accounts: newAccountsHub(), logins: newLoginLimiter(), pairing: newPairing(systemClock{})}
	server.routes()
	return server
}

func (server *Server) routes() {
	// Root routes stay outside the /api envelope, same as Python.
	server.mux.HandleFunc("GET /", server.handleHome)
	server.mux.HandleFunc("GET /health", server.handleHealth)

	// Auth.
	//
	// Le pré-vol de la connexion, et lui seul (chantier 9, C9-P3). Une page
	// servie par UN AUTRE GameFlex doit pouvoir présenter un formulaire de
	// connexion vers celui-ci : c'est ce qui permet de jouer sur une console
	// d'ici avec un compte d'ici, depuis là-bas.
	//
	// Sans cet OPTIONS, le navigateur refuse d'envoyer la requête — et ce
	// qu'il rapporte alors ne parle jamais de CORS : il dit « échec réseau »,
	// et on cherche du côté du mot de passe. Mesuré le 2026-09-07 : la route
	// répondait 405, sans un seul en-tête Access-Control.
	server.mux.HandleFunc("OPTIONS /api/v1/auth/login", handleAuthPreflight)
	server.mux.HandleFunc("POST /api/v1/auth/login", server.handleLogin)
	server.mux.HandleFunc("OPTIONS /api/v1/auth/me", handleAuthPreflight)

	// Le pré-vol de TOUT ce que la page de streaming demande à un hôte.
	//
	// Sans lui, le navigateur n'envoie même pas la requête, et l'écran qui en
	// dépend reste muet — une manette sans disposition, un mapping physique
	// qui ne se charge pas. Enregistrés en boucle depuis la liste ci-dessous
	// pour qu'il n'y ait qu'UN endroit à tenir à jour : les ajouter un par un
	// a coûté trois allers-retours dans la même journée (2026-09-07).
	for _, path := range crossOriginStreamingPaths {
		server.mux.HandleFunc("OPTIONS "+path, handleAuthPreflight)
	}
	server.mux.HandleFunc("OPTIONS /api/v1/controller-mapping/{console_id}/{kind}", handleAuthPreflight)
	server.mux.HandleFunc("GET /api/v1/auth/me", server.withUser(server.handleMe))
	server.mux.HandleFunc("PATCH /api/v1/auth/me", server.withUser(server.handleUpdateMe))
	server.mux.HandleFunc("POST /api/v1/auth/logout", server.withUser(server.handleLogout))
	// LA GÉNÉRATION DES SESSIONS (chantier 55, sessions.go) : couper ses autres
	// appareils (celui-ci reçoit un jeton neuf), ou tous.
	server.mux.HandleFunc("POST /api/v1/auth/logout/others", server.withUser(server.handleLogoutOthers))
	server.mux.HandleFunc("POST /api/v1/auth/logout/all", server.withUser(server.handleLogoutAll))
	// LE LIEN D'UN COMPTE (chantier 55, member_links.go), SANS JETON : à qui il
	// est, puis le mot de passe choisi — le code dans le corps, jamais dans
	// l'adresse.
	server.mux.HandleFunc("POST /api/v1/auth/join/check", server.handleCheckJoin)
	server.mux.HandleFunc("POST /api/v1/auth/join", server.handleJoin)
	server.mux.HandleFunc("POST /api/v1/users/resolve", server.withUser(server.handleResolveUsers))

	// Member list for the invite panel — any member can see it
	// (sorted by most recent connection, see handleListMembers).
	server.mux.HandleFunc("GET /api/v1/users/members", server.withUser(server.handleListMembers))

	// Admin member management (admin-gated like the Python service).
	server.mux.HandleFunc("GET /api/v1/users", server.withUser(server.withAdmin(server.handleListUsers)))
	server.mux.HandleFunc("GET /api/v1/users/{id}", server.withUser(server.withAdmin(server.handleFetchUser)))
	server.mux.HandleFunc("POST /api/v1/users", server.withUser(server.withAdmin(server.handleCreateUser)))
	server.mux.HandleFunc("PATCH /api/v1/users/{id}", server.withUser(server.withAdmin(server.handleUpdateUser)))
	server.mux.HandleFunc("DELETE /api/v1/users/{id}", server.withUser(server.withAdmin(server.handleDeleteUser)))
	// Un nouveau lien pour un membre qui attend ; la réinitialisation d'un membre
	// qui a rejoint (chantier 55, member_links.go).
	server.mux.HandleFunc("POST /api/v1/users/{id}/invitation", server.withUser(server.withAdmin(server.handleNewMemberLink)))
	server.mux.HandleFunc("POST /api/v1/users/{id}/reset", server.withUser(server.withAdmin(server.handleResetMember)))

	// Profile data (history, mappings).
	server.mux.HandleFunc("GET /api/v1/history", server.withUser(server.handleListHistory))
	server.mux.HandleFunc("POST /api/v1/history/start", server.withUser(server.handleStartHistory))
	server.mux.HandleFunc("PATCH /api/v1/history/{id}", server.withUser(server.handleUpdateHistory))
	server.mux.HandleFunc("GET /api/v1/controller-mapping/{console_id}/{kind}", server.withUser(server.handleGetControllerMapping))
	server.mux.HandleFunc("PUT /api/v1/controller-mapping/{console_id}/{kind}", server.withUser(server.handleSaveControllerMapping))
	server.mux.HandleFunc("GET /api/v1/key-mapping", server.withUser(server.handleListKeyMappings))
	server.mux.HandleFunc("POST /api/v1/key-mapping", server.withUser(server.handleSaveKeyMapping))
	server.mux.HandleFunc("DELETE /api/v1/key-mapping", server.withUser(server.handleDeleteKeyMapping))
	server.mux.HandleFunc("GET /api/v1/key-positioning", server.withUser(server.handleListKeyPositionings))
	server.mux.HandleFunc("POST /api/v1/key-positioning", server.withUser(server.handleSaveKeyPositioning))
	server.mux.HandleFunc("DELETE /api/v1/key-positioning", server.withUser(server.handleDeleteKeyPositioning))
	server.mux.HandleFunc("GET /api/v1/notification-preferences", server.withUser(server.handleListNotificationPreferences))
	server.mux.HandleFunc("PUT /api/v1/notification-preferences", server.withUser(server.handleSaveNotificationPreference))

	// Push + invites + presence.
	server.mux.HandleFunc("GET /api/v1/push/vapid-public-key", server.handleVapidPublicKey)
	server.mux.HandleFunc("POST /api/v1/push/subscribe", server.withUser(server.handleSubscribe))
	server.mux.HandleFunc("DELETE /api/v1/push/subscribe", server.withUser(server.handleUnsubscribe))
	server.mux.HandleFunc("POST /api/v1/invites", server.withUser(server.handleInvite))

	// Jetons de catalogue (chantier 6) — LE LIEN : un autre serveur lit le
	// nôtre. Administration seulement : donner accès à SES jeux est une
	// décision du propriétaire du serveur, pas d'un membre. La vérification,
	// elle, vit sur le plan interne : c'est le kernel qui la demande, jamais
	// un navigateur.
	//
	// Sous /api/v1/catalog-tokens, PAS sous /api/v1/federation/ : ce dernier préfixe est
	// la surface qu'un INCONNU appelle sur le kernel (catalogue, fichiers).
	// Deux services derrière un même préfixe, c'est un routage qui dépend de
	// l'ordre des entrées d'un proxy — c'est-à-dire une panne qui attend son
	// tour. Ici l'adresse dit à qui elle s'adresse.
	server.mux.HandleFunc("GET /api/v1/catalog-tokens", server.withUser(server.withAdmin(server.handleListFederationTokens)))
	server.mux.HandleFunc("POST /api/v1/catalog-tokens", server.withUser(server.withAdmin(server.handleCreateFederationToken)))
	server.mux.HandleFunc("PATCH /api/v1/catalog-tokens/{id}", server.withUser(server.withAdmin(server.handleUpdateFederationToken)))
	server.mux.HandleFunc("DELETE /api/v1/catalog-tokens/{id}", server.withUser(server.withAdmin(server.handleRevokeFederationToken)))
	server.mux.HandleFunc("POST /internal/federation/verify", server.handleVerifyFederationToken)
	// LES CONSOLES COUPÉES POUR TOUT LE SERVEUR (chantier 66) : la page les lit pour griser Jouer, l'administrateur les change.
	server.mux.HandleFunc("GET /api/v1/console-switches", server.withUser(server.handleListConsoleSwitches))
	server.mux.HandleFunc("PUT /api/v1/console-switches/{id}", server.withUser(server.withAdmin(server.handleSetConsoleSwitch)))

	// LE JETON D'UN COMPTE (chantier 20) — pour JOUER ici depuis un autre
	// serveur, à la place d'un identifiant et d'un mot de passe. L'administrateur
	// l'active pour une durée, le désactive, le régénère ; son propriétaire le
	// lit dans son profil (pas dans /auth/me, ouverte aux autres origines). Le
	// kernel l'échange contre une session, par le plan interne : jamais un
	// navigateur.
	server.mux.HandleFunc("PATCH /api/v1/users/{id}/token", server.withUser(server.withAdmin(server.handleUpdateAccountToken)))
	server.mux.HandleFunc("POST /api/v1/users/{id}/token/regenerate", server.withUser(server.withAdmin(server.handleRegenerateAccountToken)))
	server.mux.HandleFunc("GET /api/v1/auth/me/token", server.withUser(server.handleMyAccountToken))
	// MES FAVORIS (favorites.go ; le porteur, 2026-11-16) : ceux du compte connecté, sur tous ses appareils.
	server.mux.HandleFunc("GET /api/v1/auth/me/favorites", server.withUser(server.handleListFavorites))
	server.mux.HandleFunc("POST /api/v1/auth/me/favorites", server.withUser(server.handleAddFavorite))
	server.mux.HandleFunc("DELETE /api/v1/auth/me/favorites", server.withUser(server.handleRemoveFavorite))
	// LES SERVEURS FÉDÉRÉS DONT CE COMPTE NE VOIT PAS LES JEUX (hidden_servers.go ; le porteur, 2026-11-16).
	server.mux.HandleFunc("GET /api/v1/auth/me/hidden-servers", server.withUser(server.handleListHiddenServers))
	server.mux.HandleFunc("PUT /api/v1/auth/me/hidden-servers/{id}", server.withUser(server.handleHideServer))
	server.mux.HandleFunc("DELETE /api/v1/auth/me/hidden-servers/{id}", server.withUser(server.handleShowServer))
	server.mux.HandleFunc("POST /internal/federation/session", server.handleFederationSession)

	// LES INVITATIONS (chantier 33) : un code à usage unique, valable 12
	// heures, que le serveur d'en face échange contre le jeton — un jeton de
	// catalogue, ou le jeton d'un compte. Créer celle d'un compte est à son
	// titulaire ; le reste à l'administrateur (invitations.go).
	server.mux.HandleFunc("POST /api/v1/token-invitations", server.withUser(server.handleCreateInvitation))
	server.mux.HandleFunc("GET /api/v1/token-invitations", server.withUser(server.withAdmin(server.handleListInvitations)))
	server.mux.HandleFunc("DELETE /api/v1/token-invitations/{id}", server.withUser(server.withAdmin(server.handleRevokeInvitation)))
	// LE CODE D'APPAIRAGE (chantier 60, pairing.go) : deux modes découverte de
	// cinq minutes — le serveur, à l'administrateur ; le compte, à chacun pour le
	// sien —, et un code de six chiffres, valable deux minutes, une fois.
	server.mux.HandleFunc("POST /api/v1/token-invitations/pairing/server/open", server.withUser(server.withAdmin(server.handleOpenServerPairing)))
	server.mux.HandleFunc("POST /api/v1/token-invitations/pairing/server/close", server.withUser(server.withAdmin(server.handleCloseServerPairing)))
	server.mux.HandleFunc("POST /api/v1/token-invitations/pairing/account/open", server.withUser(server.handleOpenAccountPairing))
	server.mux.HandleFunc("POST /api/v1/token-invitations/pairing/account/close", server.withUser(server.handleCloseAccountPairing))
	server.mux.HandleFunc("POST /api/v1/token-invitations/pairing/codes", server.withUser(server.handleCreatePairingCode))
	server.mux.HandleFunc("DELETE /api/v1/token-invitations/pairing/codes/{id}", server.withUser(server.handleCancelPairingCode))
	server.mux.HandleFunc("POST /internal/federation/redeem", server.handleRedeemInvitation)
	// L'état des comptes, poussé au kernel (accounts_ws.go) : il coupe un compte
	// retiré sans attendre l'expiration de ses jetons.
	server.mux.HandleFunc("GET /internal/accounts/ws", server.handleAccountsWS)

	// L'autre sens — les serveurs auxquels NOUS avons accès. Même service
	// parce que c'est la même question : à qui fait-on confiance, et qui
	// nous fait confiance.
	server.mux.HandleFunc("GET /api/v1/server-registry", server.withUser(server.withAdmin(server.handleListFederationServers)))
	server.mux.HandleFunc("POST /api/v1/server-registry", server.withUser(server.withAdmin(server.handleAddFederationServer)))
	server.mux.HandleFunc("DELETE /api/v1/server-registry/{id}", server.withUser(server.withAdmin(server.handleRemoveFederationServer)))
	// MON COMPTE CHEZ UN SERVEUR FÉDÉRÉ (chantier 20) : le jeton qu'un membre
	// d'ici a collé pour y jouer. Le kernel le vérifie chez l'ami avant de le
	// confier ici, puis le relit pour échanger une session — par le plan
	// interne : il ne repasse jamais par un navigateur.
	server.mux.HandleFunc("GET /internal/federation/accounts", server.handleInternalFederatedAccount)
	server.mux.HandleFunc("PUT /internal/federation/accounts", server.handleInternalSetFederatedAccount)
	server.mux.HandleFunc("DELETE /internal/federation/accounts", server.handleInternalForgetFederatedAccount)

	// Les montages distants (WebDAV, kDrive) — chantier « ressources
	// externes ». La liste d'administration ne rend JAMAIS les mots de
	// passe ; le kernel a sa propre route interne pour ça.
	server.mux.HandleFunc("GET /internal/federation/servers", server.handleInternalFederationServers)
	// Les montages distants, mots de passe compris : le kernel monte les
	// partages, il ne peut pas le faire avec un mot de passe qu'on lui cache.
	// Les comptes des services de métadonnées (chantier 49) : même règle —
	// la liste d'administration ne rend jamais les valeurs.
	server.mux.HandleFunc("GET /api/v1/metadata-services", server.withUser(server.withAdmin(server.handleListMetadataServices)))
	server.mux.HandleFunc("GET /api/v1/metadata-services/settings", server.withUser(server.handleGetMetadataSettings))
	server.mux.HandleFunc("PUT /api/v1/metadata-services/settings", server.withUser(server.withAdmin(server.handlePutMetadataSettings)))
	server.mux.HandleFunc("PUT /api/v1/metadata-services/{service}", server.withUser(server.withAdmin(server.handlePutMetadataService)))
	server.mux.HandleFunc("DELETE /api/v1/metadata-services/{service}", server.withUser(server.withAdmin(server.handleDeleteMetadataService)))
	server.mux.HandleFunc("GET /internal/metadata/services", server.handleInternalMetadataServices)
	server.mux.HandleFunc("GET /internal/metadata/settings", server.handleInternalMetadataSettings)
	// Le statut des consoles et des jeux (chantier 30, en base depuis le 2026-11-17 : game_status.go).
	server.mux.HandleFunc("GET /internal/game-status", server.handleInternalGameStatuses)
	server.mux.HandleFunc("GET /internal/game-status/{console}", server.handleInternalGameStatus)
	server.mux.HandleFunc("PUT /internal/game-status/{console}", server.handleInternalGameStatusConsole)
	server.mux.HandleFunc("POST /internal/game-status/{console}/games/{game}", server.handleInternalGameStatusReport)
	server.mux.HandleFunc("DELETE /internal/game-status/{console}/games/{game}", server.handleInternalGameStatusRemove)
	server.mux.HandleFunc("GET /api/v1/presence/ws", server.handlePresenceWS)
}

// crossOriginAuthPaths — les SEULES routes qu'un autre GameFlex peut appeler.
//
// Deux, et pas le service entier. L'administration des membres, les favoris,
// la présence n'ont aucune raison d'être joints d'ailleurs ; les ouvrir « tant
// qu'on y est » est très exactement la façon dont /api/v1/remoteplay s'est
// retrouvé sans authentification pendant des mois.
//
// Origine libre et AUCUN cookie, même raisonnement que federationCORS côté
// kernel : l'autorisation tient entièrement dans ce que la requête porte
// elle-même. Un site tiers qui forgerait l'appel n'obtiendrait rien qu'il
// n'ait déjà — il lui faudrait le mot de passe, et s'il l'a, il n'a pas
// besoin de nous.
var crossOriginAuthPaths = map[string]bool{
	"/api/v1/auth/login": true,
	"/api/v1/auth/me":    true,
}

// crossOriginStreamingPaths — CE QUE LA PAGE DE STREAMING DEMANDE À SON HÔTE.
//
// Une page qui joue chez un serveur fédéré vise CET hôte pour tout (règle
// posée en C9-P4c) : elle y joue sous le compte qu'on y a ouvert, elle y lit
// donc les préférences de ce compte-là. Ce sont toutes des préférences de
// joueur — la forme d'une manette à l'écran, le mapping d'une manette
// physique, l'historique, le nom d'affichage des autres joueurs.
//
// ÉNUMÉRÉES, ET PAS LE SERVICE ENTIER. L'administration des membres, les
// jetons de fédération, les notifications et les invitations n'ont aucune
// raison d'être joints d'ailleurs ; les ouvrir « tant qu'on y est » est très
// exactement la façon dont /api/v1/remoteplay s'est retrouvé sans
// authentification pendant des mois. Chacune de celles-ci exige déjà un jeton
// de compte : l'ouvrir n'ajoute aucun pouvoir à qui n'en a pas.
var crossOriginStreamingPaths = []string{
	"/api/v1/key-positioning",
	"/api/v1/key-mapping",
	"/api/v1/history",
	"/api/v1/users/resolve",
}

// allowsCrossOrigin — les deux listes, plus la famille à paramètres.
//
// `controller-mapping` porte la console et le type dans son chemin : il n'y a
// pas de clé fixe à mettre dans une map, d'où le préfixe. C'est la seule
// exception, et elle est bornée à ce préfixe-là.
func allowsCrossOrigin(path string) bool {
	if crossOriginAuthPaths[path] {
		return true
	}

	for _, allowed := range crossOriginStreamingPaths {
		if path == allowed {
			return true
		}
	}

	if strings.HasPrefix(path, "/api/v1/controller-mapping/") {
		return true
	}
	for _, family := range crossOriginAdminFamilies {
		if path == family || strings.HasPrefix(path, family+"/") {
			return true
		}
	}

	return false
}

// crossOriginAdminFamilies — ce que les réglages d'un serveur fédéré administré d'ici appellent (chantier 73) : les
// membres, les accès délivrés et leurs invitations, les serveurs rejoints, les consoles coupées, les fournisseurs de
// métadonnées. Chaque route y garde sa garde : une session venue d'ailleurs n'y est administrateur que si son
// propriétaire l'a autorisé.
var crossOriginAdminFamilies = []string{
	"/api/v1/users",
	"/api/v1/catalog-tokens",
	"/api/v1/token-invitations",
	"/api/v1/server-registry",
	"/api/v1/console-switches",
	"/api/v1/metadata-services",
}

func handleAuthPreflight(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if allowsCrossOrigin(r.URL.Path) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		// Le pré-vol d'une route ouverte aux autres origines : sans lui, le navigateur n'envoie jamais la requête.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	server.mux.ServeHTTP(w, r)
}

// withUser loads the bearer principal and maps the three distinct failures
// to the exact 401 detail messages the Python service returned.
func (server *Server) withUser(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := auth.LoadUser(server.db, server.config.JWTSecret, r)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrMissingToken):
				respondErrorCode(w, http.StatusUnauthorized, "auth_required", "Missing bearer token.")
			case errors.Is(err, auth.ErrUserGone):
				respondErrorCode(w, http.StatusUnauthorized, "account_gone", "User no longer exists.")
			default:
				respondErrorCode(w, http.StatusUnauthorized, "auth_required", "Invalid or expired token.")
			}
			return
		}
		next(w, r, principal)
	}
}

func (server *Server) withAdmin(next func(http.ResponseWriter, *http.Request, auth.Principal)) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if !principal.IsAdmin() {
			respondErrorCode(w, http.StatusForbidden, "admin_only", "Admin access required.")
			return
		}
		// L'ADMINISTRATION À DISTANCE LAISSE SA TRACE (chantier 73 ; le porteur : « oui, ce serait mieux ») : qui, depuis
		// quel accès délivré (donc quel serveur ami), quoi.
		if principal.Federated && r.Method != http.MethodGet {
			var from string
			_ = server.db.QueryRow(`SELECT label FROM federation_tokens WHERE id = ?`, principal.Access).Scan(&from)
			slog.Info("remote administration", "username", principal.Username, "access", principal.Access, "from", from,
				"method", r.Method, "path", r.URL.Path)
		}
		next(w, r, principal)
	}
}

func (server *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "up", "timestamp": nowStamp()})
}

func (server *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"timestamp": nowStamp(),
		"version":   server.config.CommitSHA,
	})
}
