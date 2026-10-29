package api

// LA COUPURE D'UN COMPTE RETIRÉ (chantier 20) — « ce qu'il faut c'est de
// déconnecter le joueur depuis le serveur s'il n'est plus » (le porteur).
//
// La copie des comptes (internal/accounts) suit le service d'identité ; quand
// une liste retire quelque chose, le kernel coupe :
//   - un compte SUPPRIMÉ : ses canaux de commandes se ferment (sa page, qui ne
//     peut plus se reconnecter, le déconnecte d'elle-même), et ses parties
//     finissent — proprement par leur conteneur et à son nom, sauvegarde de
//     sortie comprise, sinon par le kernel ;
//   - un administrateur RÉTROGRADÉ : ses canaux se ferment, et il se reconnecte
//     avec son nouveau rang (auth.Admit le lit dans la copie) ;
//   - un JETON désactivé, régénéré ou raccourci : les canaux ouverts avec lui
//     depuis un autre serveur se ferment.
// Retirer une console à un compte ne coupe rien : la partie en cours continue,
// seuls les nouveaux lancements sont refusés (le porteur, décision 3).

import (
	"context"
	"log/slog"
	"net/http"
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
		closed := facade.Close(func(username, _ string) bool { return username == name })
		ended := server.endSessionsOf(name)
		slog.Info("compte supprimé : coupé", "username", name, "canaux", closed, "parties", ended)
	}
	for _, name := range change.Demoted {
		closed := facade.Close(func(username, _ string) bool { return username == name })
		slog.Info("administrateur rétrogradé : canaux fermés", "username", name, "canaux", closed)
	}
	for _, name := range change.TokenRetired {
		closed := facade.Close(func(username, token string) bool { return username == name && server.federatedToken(token) })
		slog.Info("jeton retiré : sessions fédérées fermées", "username", name, "canaux", closed)
	}
}

// federatedToken — cette session a-t-elle été ouverte par un jeton de compte,
// depuis un autre serveur ?
func (server *Server) federatedToken(token string) bool {
	claims, err := auth.ParseToken(server.config.JWTSecret, token)

	return err == nil && claims.Fed != ""
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

// consoleGranted — ce compte peut-il lancer cette console ? Avant que la liste
// des comptes n'arrive, oui : la promesse de internal/auth.
func (server *Server) consoleGranted(r *http.Request, consoleID string) bool {
	if server.accounts == nil || consoleID == "" || consoleID == "keytest" {
		return true
	}
	grants, known := server.accounts.Grants(auth.Username(r.Context()))
	if !known {
		return true
	}

	return consoles.Allows(grants, consoleID)
}

// writeNotGranted — le refus, avec un code que la page traduit.
func writeNotGranted(w http.ResponseWriter, consoleID string) {
	writeErrorCode(w, http.StatusForbidden, "consoleNotGranted", "this account cannot launch "+consoleID)
}
