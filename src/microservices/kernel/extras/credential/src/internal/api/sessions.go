package api

// LA GÉNÉRATION DES SESSIONS (chantier 55, `#696`) — ce qui manquait entièrement :
// rendre caduques les sessions d'un compte qui existe encore. Un jeton vivait sept
// jours sur un serveur et un an au téléphone, et rien ne disait « les jetons émis
// avant ceci ne valent plus ».
//
// UN SEUL MÉCANISME, pour quatre gestes (le porteur, 2026-11-14 : « c'est possible
// de rajouter "déconnexion des autres appareils seulement" et "déconnexion de tous
// les appareils" ») : `users.session_generation`, que chaque jeton porte (claim
// `gen`). La faire avancer coupe toutes les sessions d'avant :
//
//   - un administrateur réinitialise le mot de passe (member_links.go) ;
//   - le titulaire change le sien (« Mon mot de passe ») : les autres appareils
//     tombent, celui-ci reçoit un jeton neuf ;
//   - « Déconnecter les autres appareils » : pareil, sans changer de mot de passe ;
//   - « Déconnecter tous les appareils » : celui-ci compris.
//
// Ce qui juge la génération : LoadToken ici (et donc le téléphone, qui passe par
// lui), la présence, et le kernel, qui la reçoit dans la liste des comptes
// (accounts_ws.go) et ferme en 1008 les canaux d'une génération plus ancienne.
//
// LES PARTIES, quand TOUTES les sessions tombent (la réinitialisation, « tous les
// appareils » ; le porteur, 2026-11-14 : « Oui ») : la génération de ses parties
// (`users.games_generation`) avance avec, et le kernel finit ses parties en
// cours, sauvegarde de sortie comprise. Quand cet appareil reste connecté (« Mon
// mot de passe », « les autres appareils »), elles continuent : rien ne dit de
// quel appareil une partie vient, et celle de cet appareil doit continuer.

import (
	"log/slog"
	"net/http"

	"gameflex/credential/internal/auth"
)

// SessionToken — un jeton neuf pour ce compte, à la génération de ses sessions.
// Exporté pour le téléphone (embed), qui frappe ainsi ceux de son propriétaire et
// de ses invités.
func (server *Server) SessionToken(userID int64) (string, error) {
	var (
		username   string
		roleLevel  int
		generation int
	)
	if err := server.db.QueryRow(`SELECT username, role_level, session_generation FROM users WHERE id = ?`, userID).
		Scan(&username, &roleLevel, &generation); err != nil {
		return "", err
	}

	return auth.CreateToken(server.config.JWTSecret, userID, username, roleLevel, generation)
}

// AdvanceSessions — la génération des sessions de ce compte avance : tout jeton
// signé avant ne vaut plus. Puis sessionsEnded. Exporté pour le téléphone
// (embed), dont la console remet le mot de passe de root.
func (server *Server) AdvanceSessions(userID int64) error {
	if _, err := server.db.Exec(`UPDATE users SET session_generation = session_generation + 1 WHERE id = ?`, userID); err != nil {
		return err
	}
	server.sessionsEnded(userID)

	return nil
}

// sessionsEnded — la génération de ce compte vient d'avancer : la liste à jour
// part aux kernels (ils ferment ses canaux d'avant), et ses connexions de
// présence se ferment en 1008. La page d'un appareil qui a gardé sa session (le
// jeton neuf de « Mon mot de passe ») vérifie /auth/me et se reconnecte ; les
// autres reviennent à la connexion.
func (server *Server) sessionsEnded(userID int64) {
	server.accountsChanged()

	if server.presence != nil {
		server.presence.Disconnect(userID)
	}
}

// handleLogoutOthers — POST /api/v1/auth/logout/others : « Déconnecter les autres
// appareils ». La génération avance ; cet appareil reçoit un jeton neuf
// (`access_token`) et reste connecté.
func (server *Server) handleLogoutOthers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	if err := server.AdvanceSessions(principal.ID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	token, err := server.SessionToken(principal.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	slog.Info("other devices signed out", "username", principal.Username)
	respondItem(w, http.StatusOK, map[string]any{"access_token": token, "token_type": "bearer"})
}

// endAllSessions — toutes les sessions de ce compte tombent, et ses parties en
// cours finissent : les deux générations avancent ensemble. Puis sessionsEnded.
func (server *Server) endAllSessions(userID int64) error {
	if _, err := server.db.Exec(
		`UPDATE users SET session_generation = session_generation + 1, games_generation = games_generation + 1 WHERE id = ?`, userID,
	); err != nil {
		return err
	}
	server.sessionsEnded(userID)

	return nil
}

// handleLogoutAll — POST /api/v1/auth/logout/all : « Déconnecter tous les
// appareils », celui-ci compris, et ses parties en cours finissent, comme après
// une réinitialisation. Rien n'est rendu.
func (server *Server) handleLogoutAll(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	if err := server.endAllSessions(principal.ID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	slog.Info("all devices signed out", "username", principal.Username)
	respondItem(w, http.StatusOK, map[string]any{"logged_out": true})
}
