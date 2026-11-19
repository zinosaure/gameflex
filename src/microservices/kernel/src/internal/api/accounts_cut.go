package api

// LA COUPURE D'UN COMPTE RETIRÉ (chantier 20) — « ce qu'il faut c'est de
// déconnecter le joueur depuis le serveur s'il n'est plus » (le porteur).
//
// La copie des comptes (internal/accounts) suit le service d'identité ; quand
// une liste retire quelque chose, le kernel coupe :
//   - un compte SUPPRIMÉ : ses canaux de commandes se ferment (sa page, qui ne
//     peut plus se reconnecter, le déconnecte d'elle-même), et ses parties
//     finissent — proprement par leur conteneur et à son nom, sauvegarde de
//     sortie comprise, sinon par le kernel. Puis ses SAUVEGARDES partent (le
//     porteur, 2026-11-14 : « Supprimer »), quand le service d'identité le
//     nomme supprimé — jamais parce que son nom manque (eraseSavesOf) ;
//   - un administrateur RÉTROGRADÉ : ses canaux se ferment, et il se reconnecte
//     avec son nouveau rang (auth.Admit le lit dans la copie) ;
//   - un JETON désactivé, régénéré ou raccourci : les canaux ouverts avec lui
//     depuis un autre serveur se ferment ;
//   - des SESSIONS COUPÉES (chantier 55 : la génération des sessions du compte a
//     avancé) : les canaux d'une génération plus ancienne se ferment, celui d'un
//     jeton neuf reste. Quand TOUTES ses sessions tombent — RÉINITIALISÉ, ou
//     « Déconnecter tous les appareils » : la génération de ses parties avance
//     avec —, il perd aussi ses parties, comme un compte supprimé (« Oui, toutes
//     les sessions » ; « Oui », le porteur, 2026-11-14). Changer son mot de passe
//     ou couper ses autres appareils ne finit aucune partie : rien ne dit de quel
//     appareil elle vient, et celle de cet appareil doit continuer.
// Retirer une console à un compte ne coupe rien : la partie en cours continue,
// seuls les nouveaux lancements sont refusés (le porteur, décision 3).
//
// LE CODE DE LA FERMETURE DIT POURQUOI (facade.Close) : 1008 pour une session
// finie — la page vérifie son jeton (/auth/me) et revient à la connexion s'il
// est refusé ; une fermeture ordinaire pour un administrateur rétrogradé, qui se
// reconnecte avec son nouveau rang.

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gameflex/consoles"
	"gameflex/facade"
	"gameflex/kernel/internal/accounts"
	"gameflex/kernel/internal/auth"
)

// followAccounts — la copie des comptes, sa porte, et la coupure. Sans service
// d'identité configuré, rien ne change : tout jeton valide passe.
func (server *Server) followAccounts() {
	store := accounts.New()
	store.OnChange = server.cutAccounts
	store.OnPairing = server.followPairing
	server.accounts = store
	auth.SetGate(store.Admit)

	if server.config.CredentialURL == "" {
		return
	}
	url := "ws" + strings.TrimPrefix(server.config.CredentialURL, "http") + "/internal/accounts/ws"
	go store.Follow(context.Background(), url, server.config.InternalSecret)
}

func (server *Server) cutAccounts(change accounts.Change) {
	for _, name := range change.Deleted {
		closed := facade.Close(facade.SessionEnded, func(username, _ string) bool { return username == name })
		ended := server.endSessionsOf(name)
		slog.Info("compte supprimé : coupé", "username", name, "canaux", closed, "parties", ended)
	}
	for _, name := range change.Demoted {
		closed := facade.Close(facade.Reconnect, func(username, _ string) bool { return username == name })
		slog.Info("administrateur rétrogradé : canaux fermés", "username", name, "canaux", closed)
	}
	for _, name := range change.TokenRetired {
		closed := facade.Close(facade.SessionEnded, func(username, token string) bool { return username == name && server.federatedToken(token) })
		slog.Info("jeton retiré : sessions fédérées fermées", "username", name, "canaux", closed)
	}
	for name, generation := range change.Revoked {
		closed := facade.Close(facade.SessionEnded, func(username, token string) bool {
			return username == name && server.tokenGeneration(token) < generation
		})
		slog.Info("sessions coupées : canaux fermés", "username", name, "canaux", closed)
	}
	for _, name := range change.GamesEnded {
		ended := server.endSessionsOf(name)
		slog.Info("toutes les sessions coupées : parties finies", "username", name, "parties", ended)
	}
	for _, name := range change.Erased {
		server.eraseSavesOf(name)
	}
}

// eraseSavesOf — LES SAUVEGARDES D'UN COMPTE SUPPRIMÉ PARTENT AVEC LUI (le
// porteur, 2026-11-14 : « Supprimer ») : tout son dossier `saves/<compte>/`.
// C'est le seul endroit où ce serveur range quelque chose par compte : les
// états et la SRAM du rétro, ce que les cœurs écrivent, ses codes, les consoles
// récentes (`streamex/`), et ses copies (`sources/` : ses backups, ce que les
// serveurs fédérés lui ont envoyé). Les installations des jeux
// (`cache/installs`) sont à tous, et restent.
//
// Ses parties finissent d'abord — elles écrivent leur sauvegarde de sortie dans
// ce dossier. À la suppression, c'est déjà fait (Deleted) ; à la première liste
// d'un kernel qui redémarrait, une partie peut tourner encore. Puis l'effacement
// part en fond, comme celui de Réglages (savesDeletionTracker) : des minutes
// pour un gros dossier.
func (server *Server) eraseSavesOf(username string) {
	ended := server.endSessionsOf(username)
	dir, err := server.saveUserDir(username)
	if err != nil {
		slog.Warn("compte supprimé : sauvegardes laissées", "username", username, "error", err)

		return
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return
	}
	server.deleteSavesInBackground(username, dir)
	slog.Info("compte supprimé : ses sauvegardes partent", "username", username, "dossier", dir, "parties", ended)
}

// federatedToken — cette session a-t-elle été ouverte par un jeton de compte,
// depuis un autre serveur ?
func (server *Server) federatedToken(token string) bool {
	claims, err := auth.ParseToken(server.config.JWTSecret, token)

	return err == nil && claims.Fed != ""
}

// tokenGeneration — la génération des sessions que porte ce jeton ; un jeton
// illisible compte pour le plus ancien.
func (server *Server) tokenGeneration(token string) int {
	claims, err := auth.ParseToken(server.config.JWTSecret, token)
	if err != nil {
		return -1
	}

	return claims.Gen
}

// endSessionsOf — les parties que ce compte possède finissent. Rend leur
// nombre.
func (server *Server) endSessionsOf(username string) int {
	ended := 0
	for _, session := range server.pool.List() {
		container, port, spawnOwner, found := server.sessionRecord(session.ID, session.Platform)
		if !found || !server.mayEndSession(session.ID, spawnOwner, username, false) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		// Le conteneur d'abord, au nom de son propriétaire : il sauvegarde et
		// sort de lui-même. Le kernel range ensuite — ou arrête, s'il n'a pas
		// répondu.
		server.endThroughChild(ctx, session.ID, container, port, username, false)
		if server.stopSessionByPlatform(ctx, session.ID, session.Platform) {
			ended++
		}
		cancel()
	}

	return ended
}

// sessionRecord — le conteneur, le port et le propriétaire de lancement d'une
// session que ce kernel a lancée.
func (server *Server) sessionRecord(sessionID, platform string) (container string, port int, owner string, found bool) {
	switch platform {
	case "retrogaming":
		if record, ok := server.manager.LookupRetro(sessionID); ok {
			return record.Container, record.Port, record.Owner, true
		}
	case "streamex":
		if record, ok := server.manager.LookupStreamex(sessionID); ok {
			return record.Container, record.Port, record.Owner, true
		}
	case "remoteplay":
		if record, ok := server.manager.LookupRemoteplay(sessionID); ok {
			return record.Container, record.Port, record.Owner, true
		}
	}

	return "", 0, "", false
}

// consoleRefusal — pourquoi cette console ne se lance pas pour cet appelant, ou "" : elle se lance. Avant que la liste
// des comptes n'arrive, elle se lance : la promesse de internal/auth.
//
// TROIS REFUS, DANS CET ORDRE (chantier 66 ; le porteur, 2026-11-19, les textes du bouton Jouer) :
//   - `consoleDisabled` — coupée pour tout le serveur, administrateur compris (« console désactivée ») ;
//   - `federationDisabled` — la session vient d'un serveur ami dont l'accès délivré ne l'ouvre pas, ou plus
//     (« fédération désactivée ») ;
//   - `consoleNotGranted` — le compte n'a pas cette console (« accès restreint »).
func (server *Server) consoleRefusal(r *http.Request, consoleID string) string {
	if server.accounts == nil || consoleID == "" || consoleID == "keytest" {
		return ""
	}
	if server.accounts.ConsoleDisabled(consoleID) {
		return "consoleDisabled"
	}
	if access := auth.Access(r.Context()); access != 0 {
		if grants, known := server.accounts.AccessGrants(access); known && !consoles.Allows(grants, consoleID) {
			return "federationDisabled"
		}
	}
	grants, known := server.accounts.Grants(auth.Username(r.Context()))
	if known && !consoles.Allows(grants, consoleID) {
		return "consoleNotGranted"
	}

	return ""
}

// writeConsoleRefused — le refus, avec le code que la page traduit.
func writeConsoleRefused(w http.ResponseWriter, code, consoleID string) {
	writeErrorCode(w, http.StatusForbidden, code, consoleID+" cannot be launched here: "+code)
}
