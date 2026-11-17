package facade

// LES PARTIES D'UN SERVEUR FÉDÉRÉ, POUSSÉES (`#747`) — le porteur, 2026-11-16 : « Si mon app a un serveur fédéré donc
// j'ai pas de compte et que quelqu'un d'autre joue sur ce serveur. Je dois pouvoir voir la vidéo mais je ne peux pas
// rejoindre car j'ai pas le token. »
//
// Les deux bouts vivent ici, parce que les deux kernels les tiennent :
//
//   - LE PAIR (ServePeerSessions) : `GET /api/v1/federation/sessions`, ouvert en WebSocket sous la portée catalogue,
//     pousse ses parties en cours à chaque changement — ce qu'il faut pour une carte et pour regarder (PeerSession),
//     rien d'autre : ni jeton, ni adresse, ni spectateurs ;
//   - L'AMI (FollowPeerSessions) : le sujet `servers/<id>/sessions` du centre de messages. Tant qu'une page y est
//     abonnée, le kernel tient le socket du pair avec le jeton de catalogue (qui ne sort jamais du kernel) et pousse
//     chaque liste à la page ; l'abonnement fini, le socket se ferme.
//
// Pas de sondage (« Pas de sondage : WebSocket ou WebRTC », CODE_CONVENTIONS.md) : le pair pousse, l'ami relaie. Un
// pair injoignable ne retient rien — la page reçoit une liste vide, et l'ami réessaie à un pas qui double, borné.
//
// LES CONSOLES D'UN SERVEUR FÉDÉRÉ, POUSSÉES DE MÊME (`#750`) — le porteur, 2026-11-16 : « Quand j'allume la PS5 d'un
// serveur fédéré, ça me dit allumage mais l'état ne change jamais en "jouer" j'étais obligé de refresh la page ».
// L'état d'une console prêtée n'était lu qu'au chargement. Le même mécanisme le porte maintenant : `GET
// /api/v1/federation/remoteplay/devices` en WebSocket sous la portée des consoles (ServePeerDevices), relayé au sujet
// `servers/<id>/devices` (FollowPeerDevices). Une liste par sujet, une seule boucle pour les deux (servePeerList,
// followPeer).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// PeerSession — une partie, telle qu'un serveur la montre à qui tient son lien. Le pair la remplit, l'ami la relit
// dans cette forme seule : un champ que le pair ajouterait par mégarde ne traverse pas.
type PeerSession struct {
	ID          string    `json:"id"`
	Platform    string    `json:"platform"`
	ConsoleID   string    `json:"consoleId"`
	Reference   string    `json:"reference,omitempty"`
	Title       string    `json:"title"`
	Owner       string    `json:"owner,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	PlayerCount int       `json:"playerCount"`
	OpenSlots   int       `json:"openSlots"`
}

// PeerDevice — une console à distance, telle qu'un serveur la montre au jeton qui ouvre ses consoles : de quoi la
// reconnaître et dire si elle est allumée, rien d'autre (ni ses clés d'appairage, ni le compte PSN). Les mêmes champs
// que la liste lue en HTTP (`federationDevice` du kernel).
type PeerDevice struct {
	ID               string `json:"id"`
	HostID           string `json:"hostId"`
	HostName         string `json:"hostName"`
	HostType         string `json:"hostType"`
	IPAddress        string `json:"ipAddress"`
	Status           string `json:"status"`
	HasActiveSession bool   `json:"hasActiveSession"`
}

// peerFrame — ce qui passe sur le socket du pair, une liste entière à chaque fois.
type peerFrame[T any] struct {
	Data []T `json:"data"`
}

// Les deux sockets d'un pair, par leur chemin chez lui.
const (
	peerSessionsPath = "/api/v1/federation/sessions"
	peerDevicesPath  = "/api/v1/federation/remoteplay/devices"
)

// Des variables, pour que les tests les raccourcissent.
var (
	// Le pair pinge, l'ami répond : un socket muet au-delà de peerReadWait est mort, des deux côtés.
	peerPingEvery = 25 * time.Second
	peerReadWait  = 60 * time.Second
	peerWriteWait = 10 * time.Second
	// L'ami réessaie un pair perdu à ce pas, qui double jusqu'au plafond ; une connexion qui a tenu remet le pas au
	// début.
	peerRetryFirst = 2 * time.Second
	peerRetryMax   = time.Minute
	peerHeldLong   = 30 * time.Second
)

// ServePeerSessions — le bout du PAIR : pousse `read()` sur un socket déjà accepté, tout de suite puis à chaque réveil
// s'il a changé, pingue, et se ferme quand l'ami part ou quand `admitted()` dit que son jeton ne vaut plus (vérifié à
// chaque ping). Bloque jusqu'à la fin.
func ServePeerSessions(socket *websocket.Conn, wake <-chan struct{}, read func() []PeerSession, admitted func() bool) {
	servePeerList(socket, wake, read, admitted)
}

// ServePeerDevices — la même chose pour ses consoles à distance : `wake` sonne quand leur état change.
func ServePeerDevices(socket *websocket.Conn, wake <-chan struct{}, read func() []PeerDevice, admitted func() bool) {
	servePeerList(socket, wake, read, admitted)
}

// servePeerList — la boucle du pair, pour une liste de n'importe quelle forme.
func servePeerList[T any](socket *websocket.Conn, wake <-chan struct{}, read func() []T, admitted func() bool) {
	defer socket.Close()
	gone := make(chan struct{})

	// La lecture ne sert qu'à voir partir l'ami et à entendre ses pongs.
	socket.SetReadLimit(4096)
	_ = socket.SetReadDeadline(time.Now().Add(peerReadWait))
	socket.SetPongHandler(func(string) error {
		return socket.SetReadDeadline(time.Now().Add(peerReadWait))
	})
	go func() {
		defer close(gone)

		for {
			if _, _, err := socket.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(peerPingEvery)
	defer ping.Stop()
	var last []byte

	for {
		list := read()

		if list == nil {
			list = []T{}
		}
		if encoded, err := json.Marshal(peerFrame[T]{Data: list}); err == nil && string(encoded) != string(last) {
			last = encoded
			_ = socket.SetWriteDeadline(time.Now().Add(peerWriteWait))

			if socket.WriteMessage(websocket.TextMessage, encoded) != nil {
				return
			}
		}

		select {
		case <-wake:
		case <-ping.C:
			if !admitted() {
				_ = socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "token revoked"), time.Now().Add(peerWriteWait))

				return
			}
			if socket.WriteControl(websocket.PingMessage, nil, time.Now().Add(peerWriteWait)) != nil {
				return
			}
		case <-gone:
			return
		}
	}
}

// errPeerRefused — le pair a refusé la poignée de main (jeton refusé, ou route inconnue d'un pair plus ancien) : réessayer
// ne changera rien tant que la page ne se réabonne pas. Le téléphone pousse ses consoles comme un serveur depuis `#757`.
var errPeerRefused = errors.New("peer refused the socket")

// FollowPeerSessions — le bout de L'AMI : le sujet `topic` (`servers/<id>/sessions`) d'une connexion du centre de
// messages, alimenté par le socket du pair `link`, ouvert avec son jeton de catalogue. Pousse chaque liste du pair telle
// qu'il l'envoie, et une liste vide quand il est perdu ; réessaie à un pas borné, s'arrête sur un refus, et ferme le
// socket du pair à la fin de l'abonnement (`ctx`). Bloque jusqu'à la fin.
func FollowPeerSessions(ctx context.Context, conn *Conn, topic, link, token string) {
	followPeer[PeerSession](ctx, conn, "sessions", topic, link, peerSessionsPath, token)
}

// FollowPeerDevices — le bout de L'AMI pour les consoles d'un pair : le sujet `servers/<id>/devices`, poussé sous le
// type `devices`. Le pair perdu, une liste vide : ses consoles ne se disent plus prêtes. Les identifiants restent les
// siens ; la page les préfixe, comme ceux de ses parties.
func FollowPeerDevices(ctx context.Context, conn *Conn, topic, link, token string) {
	followPeer[PeerDevice](ctx, conn, "devices", topic, link, peerDevicesPath, token)
}

// followPeer — la boucle de l'ami, pour une liste de n'importe quelle forme, poussée sous `kind`.
func followPeer[T any](ctx context.Context, conn *Conn, kind, topic, link, path, token string) {
	target, err := peerSocketURL(link, path)
	if err != nil {
		conn.Send(Push{Type: kind, Topic: topic, Data: []T{}})

		return
	}
	delay := peerRetryFirst
	shown := false

	for {
		opened := time.Now()
		err := followOnce(ctx, target, token, func(list []T) {
			shown = len(list) > 0
			conn.Send(Push{Type: kind, Topic: topic, Data: list})
		})

		if ctx.Err() != nil {
			return
		}
		// Le pair perdu, ses parties ne se voient plus : la page ne garde pas une liste que personne ne tient à jour.
		// Une seule fois : tant qu'il reste perdu, rien ne repart.
		if shown {
			conn.Send(Push{Type: kind, Topic: topic, Data: []T{}})
			shown = false
		}
		if errors.Is(err, errPeerRefused) {
			return
		}
		if time.Since(opened) >= peerHeldLong {
			delay = peerRetryFirst
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		delay = min(delay*2, peerRetryMax)
	}
}

// followOnce — une connexion au pair, jusqu'à sa fin ou celle de `ctx`. Un pair qui ne répond pas à la poignée de main
// ne retient pas plus que HandshakeTimeout.
func followOnce[T any](ctx context.Context, target, token string, onList func([]T)) error {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second, Proxy: http.ProxyFromEnvironment}
	socket, response, err := dialer.DialContext(ctx, target, http.Header{"Authorization": []string{"Bearer " + token}})

	if err != nil {
		if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound) {
			return errPeerRefused
		}

		return err
	}
	defer socket.Close()

	// La fin de l'abonnement ferme le socket du pair : la lecture en cours tombe avec lui.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			socket.Close()
		case <-done:
		}
	}()

	socket.SetReadLimit(1 << 20)
	_ = socket.SetReadDeadline(time.Now().Add(peerReadWait))
	socket.SetPingHandler(func(data string) error {
		_ = socket.SetReadDeadline(time.Now().Add(peerReadWait))

		return socket.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(peerWriteWait))
	})

	for {
		_, payload, err := socket.ReadMessage()
		if err != nil {
			var closed *websocket.CloseError
			if errors.As(err, &closed) && closed.Code == websocket.ClosePolicyViolation {
				return errPeerRefused
			}

			return err
		}
		_ = socket.SetReadDeadline(time.Now().Add(peerReadWait))

		var frame peerFrame[T]
		if json.Unmarshal(payload, &frame) != nil {
			continue
		}
		if frame.Data == nil {
			frame.Data = []T{}
		}
		onList(frame.Data)
	}
}

// peerSocketURL — l'adresse d'un socket du pair (`path`), au protocole de son lien (https → wss).
func peerSocketURL(link, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(link), "/"))
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid peer link")
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", errors.New("invalid peer link")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path

	return parsed.String(), nil
}
