package api

// LES PARTIES EN COURS, D'UN SERVEUR À SES AMIS (`#747`) — le porteur, 2026-11-16 : « Si mon app a un serveur fédéré
// donc j'ai pas de compte et que quelqu'un d'autre joue sur ce serveur. Je dois pouvoir voir la vidéo mais je ne peux
// pas rejoindre car j'ai pas le token. »
//
//	GET /api/v1/federation/sessions   (portée catalogue) ce serveur, comme PAIR : ses parties en cours — en WebSocket,
//	                                  poussées à chaque changement ; en GET simple, l'instantané
//	sujet `servers/<id>/sessions`     ce serveur, comme AMI : celles du pair <id>, poussées à la page sur NOTRE canal
//	                                  (facade.FollowPeerSessions), avec le jeton de catalogue qui ne sort pas d'ici
//
// Ce qu'une partie montre : de quoi dessiner sa carte et la regarder (facade.PeerSession) — sa passerelle et
// `GET /api/v1/sessions/{id}` sont déjà publiques. Ni jeton, ni adresse, ni les noms de ceux qui regardent.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"gameflex/facade"
	"gameflex/kernel/internal/auth"
)

// handleFederationSessions — GET /api/v1/federation/sessions, derrière federationAuth et la portée catalogue.
func (server *Server) handleFederationSessions(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		writeJSON(w, http.StatusOK, server.sessionsForFriends())

		return
	}
	token := auth.BearerToken(r)

	socket, err := facade.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	signals := server.pool.Subscribe()
	defer server.pool.Unsubscribe(signals)

	// UNE RÉVOCATION MORD AUSSI UN SOCKET OUVERT : le jeton se revérifie à chaque battement ; seul un refus ferme
	// (une panne du service d'identité ne coupe pas l'ami, comme pour les autres routes du catalogue).
	facade.ServePeerSessions(socket, signals, server.sessionsForFriends, func() bool {
		verified, err := server.verifyFederationToken(token)
		if errors.Is(err, errFederationDenied) {
			return false
		}

		return err != nil || hasScope(verified.Scopes, scopeCatalog)
	})
}

// sessionsForFriends — les parties de ce serveur, dans la forme qu'un ami en reçoit. Le banc d'essai interne (keytest) n'en
// est pas : ce n'est pas une partie.
func (server *Server) sessionsForFriends() []facade.PeerSession {
	list := []facade.PeerSession{}

	for _, session := range server.enrichSessions(server.pool.List(), facadeViewer{forceSpectate: true}) {
		if session.ConsoleID == "keytest" {
			continue
		}
		list = append(list, facade.PeerSession{
			ID:          session.ID,
			Platform:    session.Platform,
			ConsoleID:   session.ConsoleID,
			Reference:   session.Reference,
			Title:       session.Title,
			Owner:       session.Owner,
			StartedAt:   session.StartedAt,
			PlayerCount: session.PlayerCount,
			OpenSlots:   session.OpenSlots,
		})
	}

	return list
}

// hasScope — la portée `scope` est-elle parmi celles d'un jeton (des segments entiers, comme requireScope) ?
func hasScope(granted, scope string) bool {
	for _, one := range strings.Split(granted, ",") {
		if strings.TrimSpace(one) == scope {
			return true
		}
	}

	return false
}

// peerFromTopic — le pair que nomme le sujet `servers/<id>/sessions` ou `servers/<id>/devices` (`#750`) (facadeTopic),
// et ce qu'on y suit, ou faux : un identifiant qui n'est pas au registre ne s'écoute pas.
func (server *Server) peerFromTopic(topic string) (registeredServer, string, bool) {
	raw, found := strings.CutPrefix(topic, "servers/")
	if !found {
		return registeredServer{}, "", false
	}
	raw, kind, found := strings.Cut(raw, "/")
	if !found || (kind != "sessions" && kind != "devices") {
		return registeredServer{}, "", false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return registeredServer{}, "", false
	}
	registered, err := server.registeredServers()
	if err != nil {
		return registeredServer{}, "", false
	}
	for _, entry := range registered {
		if entry.ID == id {
			return entry, kind, true
		}
	}

	return registeredServer{}, "", false
}
