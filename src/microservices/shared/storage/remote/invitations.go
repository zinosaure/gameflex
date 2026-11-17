package remote

// LES INVITATIONS — chantier 33. Ce qui voyage par une messagerie n'est plus un
// jeton mais un lien `<serveur>/invite/<code>` : un code à usage unique, valable
// 12 heures, que NOTRE serveur échange auprès du sien contre le vrai jeton —
// celui du lien entre deux serveurs, ou celui d'un compte de chez lui. Le jeton
// durable ne passe donc jamais par la messagerie.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Les deux sortes d'invitation, comme le service d'identité les nomme.
const (
	InvitationFederation = "federation"
	InvitationAccount    = "account"
)

var (
	// ErrNotInvitation — ce texte n'est pas le lien d'une invitation.
	ErrNotInvitation = errors.New("ce n'est pas le lien d'une invitation GameFlex")
	// ErrInvitationRefused — déjà servie, échue, révoquée ou inconnue là-bas.
	ErrInvitationRefused = errors.New("invitation refusée : déjà utilisée, expirée ou inconnue")
	// ErrInvitationKind — l'autre sorte : rien n'a été consommé, elle se colle
	// ailleurs.
	ErrInvitationKind = errors.New("invitation d'une autre sorte")
	// ErrInvitationTokenOff — le jeton de ce compte n'est pas actif là-bas.
	ErrInvitationTokenOff = errors.New("le jeton de ce compte n'est pas actif")
	// ErrInvitationThrottled — trop d'échecs depuis chez nous : attendre.
	ErrInvitationThrottled = errors.New("trop d'invitations refusées, réessayer plus tard")
	// ErrPairingClosed — un code d'appairage (chantier 60) présenté à un
	// serveur, ou un compte, qui n'est pas en mode découverte.
	ErrPairingClosed = errors.New("ce serveur n'est pas en mode découverte")
	// ErrNotPairingCode — ce texte n'est pas un code de six chiffres.
	ErrNotPairingCode = errors.New("un code d'appairage a six chiffres")
)

// pairingCode — six chiffres, zéros de tête compris (chantier 60).
var pairingCode = regexp.MustCompile(`^[0-9]{6}$`)

// PairingCode — le code tapé, sans espace (« 482 913 » s'affiche en deux
// groupes) ; ErrNotPairingCode s'il n'a pas six chiffres.
func PairingCode(text string) (string, error) {
	code := strings.Join(strings.Fields(text), "")
	if !pairingCode.MatchString(code) {
		return "", ErrNotPairingCode
	}

	return code, nil
}

// NormalizeLink — l'adresse d'un serveur tapée à la main : sans schéma, elle
// reçoit http:// si elle est privée, locale ou en .local (un serveur du réseau
// local), https:// sinon. La barre finale part.
func NormalizeLink(text string) (string, error) {
	link := strings.TrimRight(strings.TrimSpace(text), "/")
	if link == "" {
		return "", ErrNoLink
	}
	if !strings.Contains(link, "://") {
		host, _, _ := strings.Cut(link, "/")
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		address := net.ParseIP(host)
		local := address != nil && (address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast())
		scheme := "https://"
		if local || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
			scheme = "http://"
		}
		link = scheme + link
	}
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrNoLink
	}

	return link, nil
}

// invitationCode — 16 octets d'aléa en base64 sans remplissage.
var invitationCode = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// ParseInvitation — l'adresse du serveur et le code, depuis le lien collé, lu
// dans un QR ou dans l'image déposée. Ce qui précède `/invite/` est l'adresse
// (un serveur peut vivre sous un préfixe).
func ParseInvitation(text string) (link, code string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(text))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", ErrNotInvitation
	}
	at := strings.LastIndex(parsed.Path, "/invite/")
	if at < 0 {
		return "", "", ErrNotInvitation
	}
	code = strings.Trim(parsed.Path[at+len("/invite/"):], "/")
	if !invitationCode.MatchString(code) {
		return "", "", ErrNotInvitation
	}

	return parsed.Scheme + "://" + parsed.Host + parsed.Path[:at], code, nil
}

// IsMemberLink — ce texte est-il le lien d'un compte (chantier 55) :
// `<launcher>/join/<code>`, qu'une personne ouvre dans son navigateur pour
// choisir son mot de passe ? Il ne s'échange pas de serveur à serveur :
// ParseInvitation le refuse, et le coller à la place d'une invitation ne
// consomme rien. Seul le QR le dessine (InvitationPNG).
func IsMemberLink(text string) bool {
	parsed, err := url.Parse(strings.TrimSpace(text))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	at := strings.LastIndex(parsed.Path, "/join/")

	return at >= 0 && invitationCode.MatchString(strings.Trim(parsed.Path[at+len("/join/"):], "/"))
}

// Redeemed — ce que l'échange rend : le jeton, et sous quel nom on y est connu.
type Redeemed struct {
	Kind        string   `json:"kind"`
	Token       string   `json:"token"`
	Identity    string   `json:"identity"`
	DisplayName string   `json:"displayName,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	Hostname    string   `json:"hostname"`
}

// Redeem — échanger chez ce serveur une invitation de la sorte attendue contre
// son jeton. Une seule fois : un second appel est refusé. `from` est notre nom,
// que la fenêtre d'un code d'appairage montre là-bas (« Code utilisé par … »).
func (server *Server) Redeem(code, kind, from string) (Redeemed, error) {
	payload, _ := json.Marshal(map[string]string{"code": code, "kind": kind, "from": from})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.link+"/api/v1/federation/redeem", bytes.NewReader(payload))
	if err != nil {
		return Redeemed{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.lister.Do(request)
	if err != nil {
		return Redeemed{}, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return Redeemed{}, ErrInvitationRefused
	case http.StatusConflict:
		return Redeemed{}, ErrInvitationKind
	case http.StatusForbidden:
		return Redeemed{}, ErrInvitationTokenOff
	case http.StatusTooManyRequests:
		return Redeemed{}, ErrInvitationThrottled
	case http.StatusLocked:
		return Redeemed{}, ErrPairingClosed
	default:
		return Redeemed{}, &answeredError{code: response.StatusCode, status: response.Status}
	}

	var envelope struct {
		Data Redeemed `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return Redeemed{}, fmt.Errorf("réponse illisible : %w", err)
	}
	if envelope.Data.Token == "" || envelope.Data.Kind != kind {
		return Redeemed{}, errors.New("ce serveur n'a pas rendu de jeton")
	}

	return envelope.Data, nil
}

// RedeemAccount — le jeton d'un compte chez ce serveur `link`, échangé contre
// l'invitation collée. L'adresse de l'invitation ne compte pas : le compte est
// chez le serveur de la fiche, et une invitation d'ailleurs y est simplement
// inconnue — refusée sans être consommée là-bas.
func RedeemAccount(link, invitation, from string) (string, error) {
	_, code, err := ParseInvitation(invitation)
	if err != nil {
		return "", err
	}

	return RedeemAccountCode(link, code, from)
}

// RedeemAccountCode — la même chose avec un code déjà lu : celui d'une
// invitation, ou les six chiffres d'un code d'appairage (chantier 60), tapés
// pour le serveur de la fiche.
func RedeemAccountCode(link, code, from string) (string, error) {
	distant, err := New(link, "")
	if err != nil {
		return "", err
	}
	redeemed, err := distant.Redeem(code, InvitationAccount, from)
	if err != nil {
		return "", err
	}

	return redeemed.Token, nil
}

// RefusalCode — un échec d'invitation tel que la page le traduit
// (`common.errorCodes.<code>`), avec son statut HTTP. Faux pour une autre
// erreur, à dire autrement. Le serveur et le téléphone répondent ainsi la même
// chose.
func RefusalCode(err error) (status int, code string, ok bool) {
	var transport *url.Error
	switch {
	case errors.Is(err, ErrNotInvitation):
		return http.StatusBadRequest, "notInvitation", true
	case errors.Is(err, ErrNoQR):
		return http.StatusBadRequest, "noQR", true
	case errors.Is(err, ErrInvitationRefused):
		return http.StatusBadRequest, "invitationRefused", true
	case errors.Is(err, ErrInvitationKind):
		return http.StatusBadRequest, "invitationWrongKind", true
	case errors.Is(err, ErrInvitationTokenOff):
		return http.StatusBadRequest, "invitationTokenOff", true
	case errors.Is(err, ErrInvitationThrottled):
		return http.StatusTooManyRequests, "invitationThrottled", true
	case errors.Is(err, ErrPairingClosed):
		return http.StatusBadRequest, "pairingClosed", true
	case errors.Is(err, ErrNotPairingCode):
		return http.StatusBadRequest, "notPairingCode", true
	case errors.Is(err, ErrNoLink):
		return http.StatusBadRequest, "invalidLink", true
	case errors.As(err, &transport):
		return http.StatusBadGateway, "serverUnreachable", true
	}

	return 0, "", false
}
