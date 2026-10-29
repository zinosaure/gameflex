// Package facade — LE CENTRE DE MESSAGES : un seul WebSocket par onglet et par
// kernel, le même sur le serveur et sur le téléphone.
//
// Une page de jeu ouvrait jusqu'à cinq canaux vers son kernel — la liste des
// sessions, le mode « session » de la façade, le suivi d'un lancement, l'état
// de la partie — et lançait ou arrêtait par autant de requêtes HTTP. Le dessin
// voulu par l'utilisateur (2026-10-08) : un canal, et le TYPE du message dit à
// qui il s'adresse, « un peu à la manière d'un message-center qui va
// dispatcher les messages type reçu et envoyé ».
//
// LE FIL, dans les deux sens, est une enveloppe :
//
//	page → kernel   {type, id?, data?}
//	kernel → page   {type, id?, topic?, status?, launchId?, data?}
//
// Trois familles de messages :
//
//   - `subscribe` / `unsubscribe` {topic} — un ABONNEMENT. Le kernel pousse
//     tout de suite ce qu'il sait, puis chaque changement, sous le même
//     `topic`. Une page qui se reconnecte se réabonne et repart d'un
//     instantané : rien ne se rattrape, tout se renvoie.
//   - les COMMANDES — une réponse `reply` porte le même `id`, avec un statut
//     HTTP et le corps qu'aurait rendus la route remplacée.
//   - les POUSSÉES du kernel, typées par ce qu'elles portent.
//
// CE PAQUET NE CONNAÎT AUCUN SUJET NI AUCUNE COMMANDE. Il porte la mécanique —
// l'enveloppe, l'écrivain unique, la détection d'un onglet mort, les
// abonnements — et chaque kernel y branche les siens (`Router`). La page étant
// la même partout, les deux kernels parlent ainsi le même protocole par
// construction, pas par discipline.
//
// CE QUI N'Y PASSE PAS : ce qui se décide DANS une session (rejoindre,
// demander ou céder la manette) appartient au child qui la tient, et la page
// le lui dit sur SON canal. Le kernel surveille les childs ; il ne s'interpose
// pas entre eux et leurs joueurs.
package facade

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// PingInterval — le kernel sonde la page. Le navigateur répond seul aux
	// pings : une page vivante n'a rien à dire pour rester là.
	PingInterval = 25 * time.Second
	// ReadDeadline — deux pings sans réponse, et l'onglet est tenu pour mort.
	ReadDeadline = 60 * time.Second
	// writeWait — le temps qu'on laisse à une trame pour partir.
	writeWait = 10 * time.Second
	// outboxSize — ce qu'un onglet peut avoir de retard avant qu'on le coupe.
	// Large devant le débit réel : seul un onglet qui ne LIT plus le remplit.
	outboxSize = 256
	// ReadLimit — un message de la page tient en quelques centaines
	// d'octets ; un lancement est le plus gros, et reste sous 64 Kio.
	ReadLimit = 64 << 10
)

// Le rythme d'un sujet périodique (l'état d'une partie), en bornes : ni un
// client trop pressé, ni une valeur absurde ne font relever un child à leur
// guise.
const (
	PaceDefault = 2 * time.Second
	PaceMin     = time.Second
	PaceMax     = 10 * time.Second
)

// Request — un message de la page.
type Request struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Push — un message du kernel.
type Push struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Status   int    `json:"status,omitempty"`
	LaunchID string `json:"launchId,omitempty"`
	Data     any    `json:"data,omitempty"`
}

// Subscription — ce qu'un `subscribe` demande.
type Subscription struct {
	Topic string `json:"topic"`
	// Every — le rythme d'un sujet périodique, en millisecondes (voir Pace).
	Every int `json:"every"`
}

// Router — ce qu'un kernel branche sur le centre de messages.
type Router struct {
	// Topic rend de quoi alimenter un sujet jusqu'à la fin de son contexte,
	// ou le statut et la raison d'un refus.
	Topic func(conn *Conn, subscription Subscription) (run func(context.Context), status int, reason string)
	// Commands — une par type. Chacune tourne à part : un lancement dure des
	// minutes, il ne doit pas retenir la lecture des messages suivants.
	Commands map[string]func(conn *Conn, request Request)
}

// Conn — une connexion, et tout ce qui vit avec elle.
type Conn struct {
	router   Router
	socket   *websocket.Conn
	username string
	token    string
	// ctx se termine avec la connexion : chaque abonnement en dérive.
	ctx    context.Context
	cancel context.CancelFunc
	// outbox — l'écrivain est UNIQUE (gorilla n'en tolère qu'un) : tout ce
	// qui part passe par ici.
	outbox chan []byte

	mu     sync.Mutex
	topics map[string]*topic
}

type topic struct {
	cancel context.CancelFunc
}

// Upgrader — toutes origines : la page d'un téléphone vit sur 127.0.0.1, un
// invité arrive d'une autre machine, et c'est le JETON qui ouvre le canal, pas
// l'origine.
var Upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// LES CONNEXIONS OUVERTES (chantier 20) — « déconnecter le joueur depuis le
// serveur s'il n'est plus » (le porteur). Une connexion n'était revérifiée
// qu'à sa poignée de main : un compte supprimé gardait son canal tant qu'il
// le tenait ouvert. Le serveur ferme désormais celles d'un compte retiré
// (Close) ; la page, qui ne peut plus se reconnecter, le déconnecte d'elle-même.
var (
	openMu sync.Mutex
	open   = map[*Conn]struct{}{}
)

// Close ferme les connexions que `match` désigne, par leur nom et le jeton de
// leur poignée de main. Rend leur nombre.
func Close(match func(username, token string) bool) int {
	openMu.Lock()
	victims := []*Conn{}
	for conn := range open {
		if match(conn.username, conn.token) {
			victims = append(victims, conn)
		}
	}
	openMu.Unlock()

	for _, conn := range victims {
		conn.cancel()
		conn.socket.Close()
	}

	return len(victims)
}

// Serve fait vivre une connexion déjà authentifiée jusqu'à sa fin, abonnements
// compris. Bloquant.
func Serve(socket *websocket.Conn, username, token string, router Router) {
	ctx, cancel := context.WithCancel(context.Background())

	conn := &Conn{
		router:   router,
		socket:   socket,
		username: username,
		token:    token,
		ctx:      ctx,
		cancel:   cancel,
		outbox:   make(chan []byte, outboxSize),
		topics:   map[string]*topic{},
	}

	openMu.Lock()
	open[conn] = struct{}{}
	openMu.Unlock()
	defer func() {
		openMu.Lock()
		delete(open, conn)
		openMu.Unlock()
	}()

	go conn.writeLoop()

	conn.readLoop()
	cancel()
}

// Username — le nom vérifié à la poignée de main.
func (conn *Conn) Username() string { return conn.username }

// Token — le jeton de la poignée de main, que les commandes présentent au
// gestionnaire qu'elles rejouent.
func (conn *Conn) Token() string { return conn.token }

// Context se termine avec la connexion.
func (conn *Conn) Context() context.Context { return conn.ctx }

// readLoop lit la page jusqu'à ce qu'elle se taise.
//
// LE PONG REPOUSSE L'ÉCHÉANCE, et c'est ce qui manquait à l'ancienne façade du
// serveur : elle ne la repoussait qu'avant chaque message de DONNÉES. Or la
// liste des sessions n'envoie jamais rien — son socket était donc coupé toutes
// les soixante secondes et rouvert trois secondes plus tard, ce que les
// journaux montraient ligne après ligne (`"took":"1m0.000s"`, 2026-10-08).
func (conn *Conn) readLoop() {
	conn.socket.SetReadLimit(ReadLimit)
	_ = conn.socket.SetReadDeadline(time.Now().Add(ReadDeadline))
	conn.socket.SetPongHandler(func(string) error {
		return conn.socket.SetReadDeadline(time.Now().Add(ReadDeadline))
	})

	for {
		_, raw, err := conn.socket.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.socket.SetReadDeadline(time.Now().Add(ReadDeadline))

		var request Request
		if json.Unmarshal(raw, &request) != nil {
			continue
		}

		conn.dispatch(request)
	}
}

// writeLoop est le seul à écrire sur le socket, pings compris.
func (conn *Conn) writeLoop() {
	ping := time.NewTicker(PingInterval)
	defer ping.Stop()
	// Fermer ici débloque la lecture : la connexion entière s'arrête avec son
	// écrivain.
	defer conn.socket.Close()

	for {
		select {
		case frame := <-conn.outbox:
			_ = conn.socket.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.socket.WriteMessage(websocket.TextMessage, frame); err != nil {
				conn.cancel()

				return
			}
		case <-ping.C:
			if err := conn.socket.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				conn.cancel()

				return
			}
		case <-conn.ctx.Done():
			return
		}
	}
}

// Send met une trame dans la file de l'écrivain.
//
// JAMAIS BLOQUANT : un onglet qui ne lit plus ne doit retenir ni une commande,
// ni la liste des sessions des autres. Une file pleine veut dire qu'il ne suit
// plus — on le coupe, il se reconnectera et repartira d'un instantané frais,
// ce qui vaut mieux qu'un retard qui grandit.
func (conn *Conn) Send(push Push) {
	frame, err := json.Marshal(push)
	if err != nil {
		return
	}

	select {
	case conn.outbox <- frame:
	case <-conn.ctx.Done():
	default:
		slog.Warn("facade: onglet trop lent, connexion coupée", "username", conn.username)
		conn.cancel()
	}
}

// Reply répond à une commande. Sans `id`, personne n'attend : rien ne part.
func (conn *Conn) Reply(id string, status int, body []byte) {
	if id == "" {
		return
	}

	conn.Send(Push{Type: "reply", ID: id, Status: status, Data: json.RawMessage(body)})
}

// dispatch — le type dit à qui le message s'adresse.
func (conn *Conn) dispatch(request Request) {
	switch request.Type {
	case "subscribe":
		conn.subscribe(request)
	case "unsubscribe":
		conn.unsubscribe(request)
	default:
		command, known := conn.router.Commands[request.Type]
		if !known {
			conn.Reply(request.ID, http.StatusBadRequest, ErrorBody("unknown message type: "+request.Type))

			return
		}

		go command(conn, request)
	}
}

func (conn *Conn) subscribe(request Request) {
	var wanted Subscription
	if json.Unmarshal(request.Data, &wanted) != nil || wanted.Topic == "" {
		conn.Reply(request.ID, http.StatusBadRequest, ErrorBody("missing topic"))

		return
	}

	run, status, reason := conn.router.Topic(conn, wanted)
	if run == nil {
		conn.Reply(request.ID, status, ErrorBody(reason))

		return
	}

	ctx, release := conn.Track(wanted.Topic)
	go func() {
		defer release()
		run(ctx)
	}()

	conn.Reply(request.ID, http.StatusOK, OKBody(nil))
}

func (conn *Conn) unsubscribe(request Request) {
	var wanted Subscription
	_ = json.Unmarshal(request.Data, &wanted)

	conn.mu.Lock()
	if current, known := conn.topics[wanted.Topic]; known {
		current.cancel()
		delete(conn.topics, wanted.Topic)
	}
	conn.mu.Unlock()

	conn.Reply(request.ID, http.StatusOK, OKBody(nil))
}

// Track inscrit un abonnement. S'abonner deux fois au même sujet REMPLACE le
// premier : une page qui se réabonne après une reconnexion ne doit pas
// recevoir chaque trame en double.
func (conn *Conn) Track(name string) (context.Context, func()) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if previous, known := conn.topics[name]; known {
		previous.cancel()
	}

	ctx, cancel := context.WithCancel(conn.ctx)
	entry := &topic{cancel: cancel}
	conn.topics[name] = entry

	return ctx, func() {
		cancel()

		conn.mu.Lock()
		defer conn.mu.Unlock()
		if conn.topics[name] == entry {
			delete(conn.topics, name)
		}
	}
}

// PushOnChange pousse `read()` tout de suite, puis à chaque réveil — mais
// seulement s'il a changé. Le réveil peut être un signal (le pool du serveur)
// ou un battement (le téléphone, qui n'a pas de notification de changement) :
// dans les deux cas, une liste identique à la précédente ne repart pas.
func PushOnChange(ctx context.Context, conn *Conn, frame Push, wake <-chan struct{}, read func() any) {
	var last []byte

	for {
		if encoded, err := json.Marshal(read()); err == nil && !bytes.Equal(encoded, last) {
			last = encoded
			frame.Data = json.RawMessage(encoded)
			conn.Send(frame)
		}

		select {
		case <-wake:
		case <-ctx.Done():
			return
		}
	}
}

// PushEvery pousse `read()` tout de suite, puis toutes les `every`. Un relevé
// qui échoue (le child ne répond pas à cet instant) est SAUTÉ : la page garde
// l'état précédent, comme elle le faisait d'une requête manquée.
func PushEvery(ctx context.Context, conn *Conn, frame Push, every time.Duration, read func() (any, bool)) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		if value, ok := read(); ok {
			frame.Data = value
			conn.Send(frame)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// Pace — le rythme demandé en millisecondes, borné.
func Pace(milliseconds int) time.Duration {
	if milliseconds <= 0 {
		return PaceDefault
	}

	every := time.Duration(milliseconds) * time.Millisecond
	if every < PaceMin {
		return PaceMin
	}
	if every > PaceMax {
		return PaceMax
	}

	return every
}

// Call rejoue une commande comme la requête HTTP qu'elle remplace, sur le
// gestionnaire qui la traitait déjà — mêmes gardes, même jeton, mêmes codes,
// mêmes corps d'erreur. Le centre de messages ne réimplémente ni un lancement
// ni un arrêt : il les transporte.
//
// UNE COMMANDE ACCEPTÉE VA AU BOUT. Le contexte est détaché de la connexion :
// un onglet fermé pendant un lancement ne doit pas laisser une partie à moitié
// créée — elle naît quand même, apparaît dans « Mes sessions », et s'éteint
// seule si personne ne la reprend.
func Call(conn *Conn, handler http.Handler, method, path string, body []byte, values map[string]string) (int, []byte) {
	request, err := http.NewRequestWithContext(context.WithoutCancel(conn.ctx), method, path, bytes.NewReader(body))
	if err != nil {
		return http.StatusInternalServerError, ErrorBody("invalid command")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+conn.token)
	for key, value := range values {
		request.SetPathValue(key, value)
	}

	recorder := &recorder{header: http.Header{}, status: http.StatusOK}
	handler.ServeHTTP(recorder, request)

	return recorder.status, recorder.body.Bytes()
}

// recorder — ce qu'un gestionnaire aurait écrit sur le réseau.
type recorder struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (recorder *recorder) Header() http.Header { return recorder.header }

func (recorder *recorder) WriteHeader(status int) {
	if recorder.wroteHeader {
		return
	}
	recorder.wroteHeader = true
	recorder.status = status
}

func (recorder *recorder) Write(data []byte) (int, error) {
	recorder.wroteHeader = true

	return recorder.body.Write(data)
}

// ErrorBody — l'enveloppe d'erreur des deux kernels.
func ErrorBody(message string) []byte {
	encoded, _ := json.Marshal(map[string]any{"success": false, "errorMessage": message})

	return encoded
}

// OKBody — l'enveloppe de succès des deux kernels.
func OKBody(data any) []byte {
	encoded, _ := json.Marshal(map[string]any{"success": true, "data": data})

	return encoded
}
