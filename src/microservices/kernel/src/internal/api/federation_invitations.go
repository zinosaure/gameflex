// LES INVITATIONS — chantier 33 (le porteur, 2026-10-26 : « Ce matin j'ai dû
// partager les tokens par WhatsApp à mon ami »).
//
// Une invitation est un lien `<serveur>/invite/<code>` : un code à usage
// unique, valable 12 heures, créé et gardé par le service d'identité
// (invitations.go). Les deux bouts sont ici :
//
//	chez celui qui invite — POST /api/v1/federation/redeem : le serveur d'en
//	                        face échange le code contre le jeton, de serveur à
//	                        serveur ; ce kernel le relaie au service d'identité,
//	                        qui le consomme ;
//	chez l'invité         — POST /api/v1/servers/redeem : le lien collé (ou lu
//	                        dans un QR) devient un jeton du lien, que la page
//	                        range ensuite comme avant ; PUT
//	                        /api/v1/servers/{id}/account {invitation} fait de
//	                        même pour le jeton d'un compte.
//
// Le jeton durable ne passe jamais par une messagerie : seul y voyage un code
// qui ne vaut plus rien une fois servi ou échu.
package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"gameflex/storage/remote"
)

// claimedAddress — d'où la requête PRÉTEND venir : l'en-tête du proxy, sinon
// la connexion. Pour freiner et pour le journal, jamais pour autoriser.
//
// Le frein des invitations ne freinait rien (revue de sécurité, #601) : edge ne
// transmettait pas l'adresse du client au kernel, et la connexion donnait
// l'adresse ET LE PORT d'edge, différents à chaque requête. Edge la transmet
// désormais (`forwardClientAddress`, api-domains.json), et le repli perd son
// port.
func claimedAddress(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")

		return strings.TrimSpace(first)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}

// handleFederationRedeem — POST /api/v1/federation/redeem {code, kind}.
//
// Un autre serveur échange une de nos invitations. Pas de jeton : le code en
// tient lieu, une seule fois. Les refus du service d'identité passent tels
// quels — inconnue, servie ou échue (401), d'une autre sorte (409, rien de
// consommé), le jeton du compte éteint (403), trop d'échecs (429).
func (server *Server) handleFederationRedeem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil || body.Code == "" {
		writeError(w, http.StatusBadRequest, "an invitation code is required")

		return
	}
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "federation is not configured")

		return
	}
	response, err := server.identityPost("/internal/federation/redeem", map[string]string{
		"code": body.Code, "kind": body.Kind, "address": claimedAddress(r),
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot redeem: "+err.Error())

		return
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusConflict, http.StatusForbidden:
		writeError(w, response.StatusCode, "invitation refused: "+http.StatusText(response.StatusCode))

		return
	case http.StatusTooManyRequests:
		w.Header().Set("Retry-After", response.Header.Get("Retry-After"))
		writeError(w, http.StatusTooManyRequests, "too many failed invitations")

		return
	default:
		writeError(w, http.StatusServiceUnavailable, "cannot redeem: identity service answered "+response.Status)

		return
	}

	var decoded struct {
		Item struct {
			Kind        string `json:"kind"`
			Token       string `json:"token"`
			Identity    string `json:"identity"`
			Scopes      string `json:"scopes"`
			DisplayName string `json:"display_name"`
		} `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot redeem: "+err.Error())

		return
	}
	scopes := []string{}
	for _, scope := range strings.Split(decoded.Item.Scopes, ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopes = append(scopes, scope)
		}
	}

	writeJSON(w, http.StatusOK, remote.Redeemed{
		Kind: decoded.Item.Kind, Token: decoded.Item.Token, Identity: decoded.Item.Identity,
		DisplayName: decoded.Item.DisplayName, Scopes: scopes, Hostname: server.federationHostname(),
	})
}

// invitationRefusal — un échec d'échange dit à la page, par un code qu'elle
// traduit (remote.RefusalCode). Faux quand ce n'en est pas un.
func invitationRefusal(w http.ResponseWriter, err error) bool {
	status, code, ok := remote.RefusalCode(err)
	if ok {
		writeErrorCode(w, status, code, err.Error())
	}

	return ok
}

// handleRedeemServerInvitation — POST /api/v1/servers/redeem {invitation}.
//
// Le lien d'une invitation de fédération devient le jeton du lien : échangé
// chez lui, puis sondé comme /servers/check. La réponse est celle de la sonde
// plus le jeton, que la page range comme un jeton collé
// (POST /api/v1/server-registry). Un serveur déjà dans la liste est refusé
// AVANT l'échange : l'invitation n'est pas perdue.
func (server *Server) handleRedeemServerInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Invitation string `json:"invitation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")

		return
	}
	link, code, err := remote.ParseInvitation(body.Invitation)
	if invitationRefusal(w, err) {
		return
	}
	known, err := server.registeredServers()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot read the server list: "+err.Error())

		return
	}
	for _, entry := range known {
		if strings.TrimRight(entry.Link, "/") == link {
			writeErrorCode(w, http.StatusConflict, "alreadyRegistered", "this server is already in the list")

			return
		}
	}

	distant, err := remote.New(link, "")
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "notInvitation", err.Error())

		return
	}
	redeemed, err := distant.Redeem(code, remote.InvitationFederation)
	if invitationRefusal(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	// La sonde, avec le jeton qui vient de naître. L'échange a eu lieu : une
	// sonde qui échoue ne doit pas perdre le jeton, la page le range quand
	// même, sous le nom que l'échange a rendu.
	identity := remote.Identity{Hostname: redeemed.Hostname, Identity: redeemed.Identity, Scopes: redeemed.Scopes}
	if probe, err := remote.New(link, redeemed.Token); err == nil {
		if seen, err := probe.Check(); err == nil {
			identity = seen
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"link":     link,
		"token":    redeemed.Token,
		"hostname": identity.Hostname,
		"identity": identity.Identity,
		"scopes":   identity.Scopes,
	})
}

// qrSize — le côté du QR demandé, borné : assez pour un écran, pas plus.
func qrSize(asked int) int {
	if asked <= 0 {
		return 512
	}

	return min(max(asked, 160), 1024)
}

// handleInvitationQR — POST /api/v1/qr {text, size} : le QR d'un lien
// d'invitation, en PNG (remote.InvitationPNG). En POST et non en GET : le lien
// porte un code qui vaut quelque chose pendant 12 heures, et une adresse se
// retrouve dans les journaux du proxy.
func (server *Server) handleInvitationQR(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
		Size int    `json:"size"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")

		return
	}
	image, err := remote.InvitationPNG(strings.TrimSpace(body.Text), qrSize(body.Size))
	if invitationRefusal(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(image)
}

// handleReadInvitationQR — POST /api/v1/qr/read, l'image en corps : le lien
// d'invitation que porte son QR — une capture reçue par une messagerie, une
// photo. `noQR` sans QR lisible, `notInvitation` si ce n'est pas le nôtre.
func (server *Server) handleReadInvitationQR(w http.ResponseWriter, r *http.Request) {
	text, err := remote.ReadInvitation(r.Body)
	if invitationRefusal(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}
