// Le kernel en tant que CLIENT d'autres serveurs — chantier 6.
//
// Deux gestes, séparés exprès :
//
//	sonder      — l'adresse répond-elle, le jeton est-il bon, et comment
//	              s'appelle ce serveur ? Aucune écriture.
//	agréger     — aller chercher les catalogues des serveurs enregistrés et
//	              les mêler à la bibliothèque locale.
//
// Le REGISTRE, lui, vit dans le service d'identité : c'est la même question
// que les jetons de catalogue — à qui fait-on confiance, et qui nous fait
// confiance. Le kernel le lit par le plan interne quand il agrège.
//
// Et un troisième depuis le chantier 20 — jouer : un membre d'ici qui a un
// compte chez un ami colle UNE fois le jeton de ce compte (handleSetServerAccount),
// gardé avec son compte d'ici ; le kernel l'échange contre une session à son nom
// quand il joue là-bas (handleServerSession). C'est l'ancien formulaire de
// connexion chez un ami, sans mot de passe : « un token et c'est bon ».
//
// Pourquoi sonder est une route à part plutôt qu'une validation cachée dans
// l'enregistrement : parce que l'interface a quelque chose à montrer entre
// les deux. « Serveur trouvé : nas-du-salon, vous y êtes connu comme Marc »
// avant d'écrire quoi que ce soit vaut mieux qu'un formulaire qui accepte en
// silence et qui rend une liste vide trois écrans plus loin.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gameflex/kernel/internal/auth"
	"gameflex/storage/remote"
)

// handleFederationCheck — POST /api/v1/servers/check {link, token}.
func (server *Server) handleFederationCheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Link  string `json:"link"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	distant, err := remote.New(body.Link, body.Token)
	if errors.Is(err, remote.ErrNoLink) {
		writeError(w, http.StatusBadRequest, "a link is required")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	identity, err := distant.Check()
	if errors.Is(err, remote.ErrDenied) {
		// 400 et pas 401 : ce n'est pas NOTRE authentification qui échoue,
		// c'est le jeton saisi qui est refusé là-bas. Un 401 ferait croire
		// au navigateur que sa propre session a expiré.
		writeError(w, http.StatusBadRequest, "this server refused the token")
		return
	}
	if err != nil {
		// Une erreur de transport de Go ("Get \"http://…\": dial tcp …:
		// connect: connection refused") est exacte et illisible. Ce que la
		// personne a besoin de savoir tient en une phrase, et c'est déjà
		// tout ce qu'elle peut corriger : l'adresse, ou la machine d'en face.
		var transport *url.Error
		if errors.As(err, &transport) {
			writeError(w, http.StatusBadGateway, "this address did not answer")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"link":     distant.Link(),
		"hostname": identity.Hostname,
		"identity": identity.Identity,
		"scopes":   identity.Scopes,
	})
}

// handleServerSession — GET /api/v1/servers/{id}/session (chantier 20) : une
// session chez ce serveur, au nom du compte de l'APPELANT là-bas — celui dont
// il a collé le jeton. Sans jeton, 404 `noFederatedAccount` : la page montre le
// formulaire du jeton, et les jeux de ce serveur restent à voir et à
// télécharger.
//
// Le jeton ne sort pas d'ici ; la session, si : la page en a besoin pour parler
// à l'hôte (son canal, le flux d'une partie), comme avec le compte qu'on y
// ouvrait à la main avant ce chantier.
func (server *Server) handleServerSession(w http.ResponseWriter, r *http.Request) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}
	member := auth.Username(r.Context())
	token, err := server.federatedAccountToken(member, entry.ID)
	if errors.Is(err, errNoFederatedAccount) {
		writeErrorCode(w, http.StatusNotFound, "noFederatedAccount", "no token of yours for this server")

		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot read your token: "+err.Error())

		return
	}

	session, err := server.peerSessions.At(entry.ID, entry.Link, member, token)
	if errors.Is(err, remote.ErrDenied) {
		writeErrorCode(w, http.StatusForbidden, "accountRefused", "this server refused your token")

		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "serverUnreachable", "this server did not answer")

		return
	}

	writeJSON(w, http.StatusOK, session)
}

// handleSetServerAccount — PUT /api/v1/servers/{id}/account {token} ou
// {invitation} (chantiers 20 et 33) : le jeton du compte de l'appelant chez ce serveur, collé UNE
// fois. Il s'essaie d'abord là-bas — un jeton refusé ne se garde pas, et le dit
// tout de suite —, puis se garde avec le compte d'ici (« avec mon compte », le
// porteur : sur tous ses appareils). La session que l'essai a ouverte sert à
// la première partie : chaque échange compte une connexion chez l'ami.
func (server *Server) handleSetServerAccount(w http.ResponseWriter, r *http.Request) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}
	// UNE SESSION OUVERTE PAR JETON JOUE, ET RIEN D'AUTRE : quelqu'un d'un autre
	// serveur qui joue ici ne confie pas ici ses jetons chez d'autres —
	// localOnly, sur la route (api.go).
	var body struct {
		Token      string `json:"token"`
		Invitation string `json:"invitation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		(strings.TrimSpace(body.Token) == "" && strings.TrimSpace(body.Invitation) == "") {
		writeError(w, http.StatusBadRequest, "a token or an invitation is required")

		return
	}
	token := strings.TrimSpace(body.Token)
	// OU L'INVITATION DE CE COMPTE (chantier 33) : échangée chez lui contre le
	// jeton, qui ne passe ainsi jamais par une messagerie.
	if invitation := strings.TrimSpace(body.Invitation); invitation != "" {
		redeemed, err := remote.RedeemAccount(entry.Link, invitation)
		if invitationRefusal(w, err) {
			return
		}
		if err != nil {
			writeErrorCode(w, http.StatusBadGateway, "serverUnreachable", "this server did not answer")

			return
		}
		token = redeemed
	}

	client, err := remote.New(entry.Link, token)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	session, err := client.OpenSession()
	if errors.Is(err, remote.ErrDenied) {
		writeErrorCode(w, http.StatusForbidden, "accountRefused", "this server refused this token")

		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "serverUnreachable", "this server did not answer")

		return
	}

	member := auth.Username(r.Context())
	if err := server.storeFederatedAccount(http.MethodPut, map[string]any{
		"username": member, "server": entry.ID, "token": token,
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot keep your token: "+err.Error())

		return
	}
	server.peerSessions.Hold(entry.ID, member, token, session)

	writeJSON(w, http.StatusOK, session)
}

// handleForgetServerAccount — DELETE /api/v1/servers/{id}/account : oublier le
// jeton de l'appelant chez ce serveur. Ses jeux restent à voir et à
// télécharger.
func (server *Server) handleForgetServerAccount(w http.ResponseWriter, r *http.Request) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}
	member := auth.Username(r.Context())
	query := url.Values{"username": {member}, "server": {strconv.FormatInt(entry.ID, 10)}}
	err := server.storeFederatedAccount(http.MethodDelete, query)
	if errors.Is(err, errNoFederatedAccount) {
		writeErrorCode(w, http.StatusNotFound, "noFederatedAccount", "no token of yours for this server")

		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot forget your token: "+err.Error())

		return
	}
	server.peerSessions.Forget(entry.ID, member)

	w.WriteHeader(http.StatusNoContent)
}

// errNoFederatedAccount — ce membre n'a pas de jeton pour ce serveur.
var errNoFederatedAccount = errors.New("no token for this server")

// federatedAccountToken — le jeton que ce membre a collé pour ce serveur, lu
// au service d'identité par le plan interne.
func (server *Server) federatedAccountToken(member string, serverID int64) (string, error) {
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return "", errNoFederatedAccount
	}
	query := url.Values{"username": {member}, "server": {strconv.FormatInt(serverID, 10)}}
	request, err := http.NewRequest(http.MethodGet, server.config.CredentialURL+"/internal/federation/accounts?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", errNoFederatedAccount
	default:
		return "", errors.New("identity service answered " + response.Status)
	}
	var envelope struct {
		Item struct {
			Token string `json:"token"`
		} `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return "", err
	}
	if envelope.Item.Token == "" {
		return "", errNoFederatedAccount
	}

	return envelope.Item.Token, nil
}

// storeFederatedAccount — garder (PUT, un corps JSON) ou oublier (DELETE, une
// requête `?username=…&server=…`) le jeton d'un membre pour un serveur.
func (server *Server) storeFederatedAccount(method string, payload any) error {
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return errors.New("federation is not configured")
	}
	target := server.config.CredentialURL + "/internal/federation/accounts"
	var body *bytes.Reader
	if query, isQuery := payload.(url.Values); isQuery {
		target += "?" + query.Encode()
		body = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(payload)
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, target, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errNoFederatedAccount
	default:
		return errors.New("identity service answered " + response.Status)
	}
}

// registeredServer — une ligne du registre, telle que le service d'identité
// la rend sur le plan interne (jeton compris).
type registeredServer struct {
	ID       int64  `json:"id"`
	Link     string `json:"link"`
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	Identity string `json:"identity"`
}

// registeredServers lit le registre. Rend une liste vide plutôt qu'une
// erreur quand la fédération n'est pas configurée : un serveur sans amis
// n'est pas un serveur en panne.
// Mémorisée dix secondes (#190) : cette liste était redemandée à chaque
// affichage de jeux et à chaque frappe de recherche, AVANT toute lecture du
// cache des catalogues — donc payée même quand tout était chaud.
func (server *Server) registeredServers() ([]registeredServer, error) {
	return server.serverRegistry.get(server.fetchRegisteredServers)
}

func (server *Server) fetchRegisteredServers() ([]registeredServer, error) {
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return nil, nil
	}

	request, err := http.NewRequest(http.MethodGet, server.config.CredentialURL+"/internal/federation/servers", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.New("cannot read the server registry: " + response.Status)
	}

	var envelope struct {
		Items []registeredServer `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	// UN AMI RETIRÉ EMPORTE LA COPIE DE SON LISTING (chantier 15) : le
	// registre relu est celui qui fait foi.
	peers := make([]remote.Peer, 0, len(envelope.Items))
	for _, entry := range envelope.Items {
		peers = append(peers, peerOf(entry))
	}
	server.copies().Prune(peers)

	return envelope.Items, nil
}
