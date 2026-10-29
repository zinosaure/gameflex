package session

// The HTTP/WebSocket surface of a child-hosted session (chantier-4).
//
// It lives here rather than in each runtime because there are three of them,
// and this codebase already knows what happens when the same thing is written
// three times: the retro/streamex/remoteplay session handling drifted until
// no two behaved alike. What is genuinely platform-specific — which input
// port a slot maps to, how to stop the emulator — stays in the runtime, as a
// couple of hooks.
//
// streamex is a vendored project, so its integration must stay thin: with
// this, it is a dozen lines of glue rather than a fork of the whole thing.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gameflex/auth"

	"github.com/gorilla/websocket"
)

// Config is what a runtime must supply to host its own session.
type Config struct {
	SessionID  string
	PlatformID string
	// Owner, ConsoleID and JWTPublicKey come from the container's
	// environment: the party that decides is the party that authenticates,
	// so the child verifies tokens itself rather than trusting a
	// pre-verified identity. A PUBLIC key since 2026-10-28 (chantier 34,
	// P10): it verifies a token, it cannot sign one.
	Owner        string
	MaxPlayers   int
	JWTPublicKey ed25519.PublicKey

	// KernelURL and InternalSecret address the kernel's mirror. Reporting is
	// best effort: the mirror may lag, it may not lie about an action. The
	// secret is THIS session's (gameflex/auth.SessionSecret), never the
	// installation's.
	KernelURL      string
	InternalSecret string

	// BeatTimeout catches the FROZEN tab, whose socket stays open while its
	// JS no longer runs. AFKAfter catches the abandoned one, which beats
	// happily with nobody behind it. IdleAfter ends a session nobody plays.
	BeatTimeout time.Duration
	AFKAfter    time.Duration
	// AFKGrace — le temps laissé pour répondre à « toujours là ? ».
	//
	// On ne coupe pas sec : le joueur reçoit la question, et une minute pour
	// toucher n'importe quoi. AFKAfter vaut 29 minutes et non 15 pour une
	// raison qu'aucune horloge ne devine — une cinématique peut durer plus
	// d'un quart d'heure, et quelqu'un qui regarde son jeu n'est pas absent.
	// Vingt-neuf plus une font la demi-heure, qui reste le seuil.
	//
	// Zéro = on exclut sans demander (l'ancien comportement).
	AFKGrace  time.Duration
	IdleAfter time.Duration

	// EndSession tears the platform down and exits. Exiting IS the signal:
	// the container stops and the kernel drops the session and its mirror.
	EndSession func(reason string)

	// OnPadHandover puts a pad back to neutral when it changes hands (see
	// Host.OnPadHandover). Optional: a runtime without held state needs none.
	OnPadHandover func(slot int)
}

// HTTPHost serves one session over HTTP/WS and keeps the kernel's mirror fed.
type HTTPHost struct {
	Host   *Host
	config Config

	sockets sync.Map // socketID -> *hostSocket
	viewers sync.Map // gateway viewerID -> socketID

	reports chan struct{}

	// La santé de NOTRE rapport vers le kernel — voir noteMirrorResult.
	mirrorMu       sync.Mutex
	mirrorLastOK   time.Time
	mirrorFailures int
	mirrorDisabled bool

	nextID   uint64
	nextIDMu sync.Mutex

	// ending — EndSession runs once: a terminate ends the game at once, and
	// the sweep would otherwise end it a second time on its next tick.
	ending atomic.Bool
}

type hostSocket struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (socket *hostSocket) write(message map[string]any) error {
	socket.writeMu.Lock()
	defer socket.writeMu.Unlock()

	return socket.conn.WriteJSON(message)
}

// NewHTTPHost builds the session and starts its sweep and reporting loops.
func NewHTTPHost(config Config) *HTTPHost {
	if config.MaxPlayers < 1 {
		config.MaxPlayers = 1
	}

	httpHost := &HTTPHost{config: config, reports: make(chan struct{}, 1)}
	httpHost.Host = NewHost(
		New(config.SessionID, config.PlatformID, config.Owner, config.MaxPlayers, time.Now()),
		httpHost.sendTo,
	)
	httpHost.Host.OnChange = httpHost.reportSoon
	httpHost.Host.OnPadHandover = config.OnPadHandover

	go httpHost.reportLoop()
	go httpHost.reportHeartbeat()
	go httpHost.sweepLoop()

	// Declare at startup, before any tab exists. "This session is
	// child-managed" must be true from the moment the container is up: the
	// kernel decides whether to defer based on whether a mirror exists, and a
	// boundary that depends on a message having arrived is not a boundary.
	httpHost.reportSoon()

	return httpHost
}

// PortForViewer answers the only question an input path has to ask: may THIS
// gateway viewer's keystrokes reach the emulator, and on which slot?
//
// Returns the 1-based SLOT; the runtime maps it to whatever its own input
// layer counts from. A refusal is (0, true): known to us, holding nothing.
// Never (0, false) — the zero value must not read as "slot 0".
//
// A LOAN changes the answer and nothing else: the borrower's keys reach the
// owner's pad, the owner's own reach nothing until they take it back (see
// Session.ControlledSlot). Read under the host's lock — this runs for every
// keystroke, concurrently with whatever the queue is doing.
func (httpHost *HTTPHost) PortForViewer(viewerID string) (slot int, managed bool) {
	value, ok := httpHost.viewers.Load(viewerID)
	if !ok {
		return 0, false
	}

	// Known to us: managed, whatever the answer. Holding nothing means
	// watching — refused, never handed to a legacy mapping that would let a
	// watcher play.
	return httpHost.Host.ControlledSlot(value.(string)), true
}

// ViewerForSlot est la question INVERSE de PortForViewer : quel viewer tient
// ce siège ?
//
// Elle sert au retour de force. Une vibration ne se diffuse pas : elle
// s'adresse à celui qui tient la manette, et lui seul — les spectateurs n'ont
// rien à sentir. Sans cette lecture, la seule direction disponible était
// viewer → siège, et le child ne savait pas à qui parler (2026-09-10).
//
// Rend "" quand personne ne tient ce siège, ou quand son détenteur n'est pas
// arrivé par un viewer de la passerelle.
//
// Pendant un prêt, c'est l'emprunteur : la vibration va aux mains qui tiennent
// la manette, pas à la place.
func (httpHost *HTTPHost) ViewerForSlot(slot int) string {
	socketID := httpHost.Host.ControllerOf(slot)
	if socketID == "" {
		return ""
	}

	viewerID := ""
	httpHost.viewers.Range(func(key, value any) bool {
		if value.(string) != socketID {
			return true
		}
		viewerID, _ = key.(string)

		return false
	})

	return viewerID
}

// Routes registers the two endpoints on a mux, under the given prefix.
func (httpHost *HTTPHost) Routes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc(prefix+"/ws", httpHost.serveSocket)
	mux.HandleFunc(prefix+"/terminate", httpHost.serveTerminate)
	mux.HandleFunc(prefix+"/report", httpHost.serveReport)
	mux.HandleFunc(prefix+"/wind-down", httpHost.serveWindDown)
}

// serveWindDown — « tu peux m'envoyer une demande d'arrêt ? »
//
// LE KERNEL SOLLICITE, IL N'ORDONNE PAS. La différence n'est pas une nuance
// de vocabulaire : ce qui suit est la décision du child, prise avec SA propre
// règle (endReason), sur SON état — pas sur la copie qu'en garde le kernel.
// S'il n'est pas d'accord, il le dit et il continue.
//
// D'où ça vient : un child peut se retrouver inoccupé depuis longtemps sans
// avoir émis son ordre — sa boucle est coincée, son rapport ne part plus, et
// le conteneur, lui, vit très bien. C'est la vraie session fantôme. Le kernel
// la voit en interrogeant (voir serveReport), mais la voir ne l'autorise pas
// à trancher : il demande au child de conclure lui-même.
//
// La réponse part AVANT l'arrêt : sortir tue ce processus, et le kernel doit
// savoir que sa demande a été acceptée plutôt que de la voir échouer en même
// temps qu'elle réussit.
func (httpHost *HTTPHost) serveWindDown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)

		return
	}
	if !auth.SameSecret(r.Header.Get("X-Internal-Secret"), httpHost.config.InternalSecret) {
		auth.Refuse(w, http.StatusForbidden, "forbidden")

		return
	}

	reason := httpHost.endReason(time.Now())
	ending := reason != "" && httpHost.config.EndSession != nil
	if !ending && reason == "" {
		reason = "la partie a encore une raison de vivre"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ending": ending, "reason": reason})

	if ending {
		// Après la réponse, et dans son propre fil : `EndSession` démolit la
		// plateforme et sort.
		go httpHost.config.EndSession(reason)
	}
}

// serveReport — la question que le kernel a le droit de poser.
//
// Le rapport part d'ici toutes les 30 s (voir mirrorHeartbeat). S'il n'arrive
// pas — un réseau qui hoquette, un child occupé — le kernel se retrouvait à
// deviner, et une première version de sa garde tuait sur un simple silence.
// Un silence n'est pas une information ; une réponse en est une.
//
// Alors il DEMANDE, et le child répond. L'autorité ne bouge pas d'un pouce :
// c'est toujours le child qui dit qui est là et depuis quand. On remplace
// seulement une déduction par un fait.
//
// C'est ce qui fait disparaître les sessions fantômes : un conteneur que
// Docker dit vivant et qui ne répond plus à ça n'est plus une partie, c'est un
// reste — et le kernel peut enfin le dire au lieu de l'afficher.
//
// Réservé au kernel (X-Internal-Secret) : la composition d'une partie ne
// regarde pas le premier venu sur le réseau local.
func (httpHost *HTTPHost) serveReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)

		return
	}
	if !auth.SameSecret(r.Header.Get("X-Internal-Secret"), httpHost.config.InternalSecret) {
		auth.Refuse(w, http.StatusForbidden, "forbidden")

		return
	}

	// LE KERNEL A DÛ DEMANDER : C'EST UN RAPPEL À L'ORDRE.
	//
	// Cette requête ne devrait jamais arriver. Le rapport part tout seul
	// toutes les 30 s ; si le kernel en est réduit à le réclamer, c'est que
	// le nôtre n'atterrit plus — et jusqu'ici personne, de ce côté-ci, ne
	// s'en apercevait. On le dit, avec de quoi comprendre : depuis quand la
	// dernière poussée a réussi, et combien ont échoué depuis.
	//
	// Puis on se remet à jour tout de suite, sans attendre le prochain tour
	// d'horloge : la réponse ci-dessous renseigne le kernel, la poussée
	// remet le canal habituel en marche. Si c'est le canal qui est cassé, la
	// tentative le redira — cette fois avec un compteur qui monte, ce qu'un
	// journal ne montrait pas.
	lastOK, failures := httpHost.mirrorHealth()
	httpHost.mirrorMu.Lock()
	disabled := httpHost.mirrorDisabled
	httpHost.mirrorMu.Unlock()

	switch {
	case disabled:
		log.Printf("[session %s] le kernel demande un rapport, mais ce child n'en envoie AUCUN "+
			"(ni adresse ni secret au démarrage) — c'est une erreur de configuration, pas un réseau",
			httpHost.config.SessionID)
	case lastOK.IsZero():
		log.Printf("[session %s] le kernel demande un rapport : aucune poussée n'a jamais réussi (%d échecs)",
			httpHost.config.SessionID, failures)
		httpHost.reportSoon()
	default:
		log.Printf("[session %s] le kernel demande un rapport : ma dernière poussée réussie date de %s (%d échecs depuis)",
			httpHost.config.SessionID, time.Since(lastOK).Round(time.Second), failures)
		httpHost.reportSoon()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(httpHost.Host.Snapshot()); err != nil {
		log.Printf("[session %s] rapport refusé: %v", httpHost.config.SessionID, err)
	}
}

func (httpHost *HTTPHost) sendTo(socketID string, message map[string]any) error {
	value, ok := httpHost.sockets.Load(socketID)
	if !ok {
		// Unknown socket: report failure so the host drops it. The normal end
		// of a tab, not worth a log line.
		return http.ErrServerClosed
	}

	return value.(*hostSocket).write(message)
}

// Identify verifies the JWT a request carries (`?token=` or a Bearer header)
// with this session's secret, and names its user. For the child's OTHER
// doors — the retro owner and joiner sockets — which must know who is
// knocking just as this host does: the container listens on the LAN, and a
// door that trusts a name in its query string is open to anyone who can
// reach the port.
func (httpHost *HTTPHost) Identify(r *http.Request) (string, bool) {
	return httpHost.identify(r)
}

func (httpHost *HTTPHost) identify(r *http.Request) (string, bool) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}

	claims, err := auth.Verify(httpHost.config.JWTPublicKey, token, time.Now())
	if err != nil {
		return "", false
	}

	return claims.Username, true
}

// newSocketID names a tab. UNGUESSABLE, on purpose: an intent names the tab
// it speaks for, and the id travels to the page and back. It used to be the
// clock plus a counter ("sock-HHMMSS-N") — anybody could enumerate the tabs
// of a session. The command layer now also checks the tab belongs to the
// caller (ErrNotYourTab); this keeps the id from being a map of who is
// connected. The counter stays as a prefix for the logs, which follow a tab
// by it.
func (httpHost *HTTPHost) newSocketID() string {
	httpHost.nextIDMu.Lock()
	httpHost.nextID++
	sequence := httpHost.nextID
	httpHost.nextIDMu.Unlock()

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "sock-" + itoa(sequence) + "-" + time.Now().Format("150405.000000000")
	}

	return "sock-" + itoa(sequence) + "-" + hex.EncodeToString(random)
}

func itoa(value uint64) string {
	if value == 0 {
		return "0"
	}

	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}

	return digits
}

// serveSocket is the ONE socket a tab holds: presence, beats, the pairing of
// its gateway viewer — and its intents (P3).
//
// An intent is still DECLARED, never inferred from a transport event: it is
// its own message, `{type:"intent", action, target}`. What changed is where it
// travels. It used to be POSTed beside this socket, naming the socket it spoke
// for — and that name travelled in the clear, which let one account seat
// another's tab until ErrNotYourTab closed it (2026-10-08). On the socket
// itself there is nothing to name: the tab that sends is the tab it speaks
// for, by construction.
func (httpHost *HTTPHost) serveSocket(w http.ResponseWriter, r *http.Request) {
	username, ok := httpHost.identify(r)
	if !ok {
		auth.Refuse(w, http.StatusUnauthorized, "invalid or missing token")
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	socketID := httpHost.newSocketID()
	httpHost.sockets.Store(socketID, &hostSocket{conn: conn})

	defer func() {
		httpHost.sockets.Delete(socketID)
		httpHost.viewers.Range(func(key, value any) bool {
			if value.(string) == socketID {
				httpHost.viewers.Delete(key)
			}

			return true
		})
		httpHost.Host.Detach(socketID, time.Now())
		_ = conn.Close()
	}()

	httpHost.Host.Attach(socketID, username)

	for {
		var message struct {
			Type     string `json:"type"`
			HadInput bool   `json:"hadInput"`
			ViewerID string `json:"viewerId"`
			Action   string `json:"action"`
			Target   string `json:"target"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return
		}

		switch message.Type {
		case "beat":
			httpHost.Host.Beat(socketID, message.HadInput, time.Now())
		case "viewer":
			// A tab holds two identities — a gateway viewer and this socket —
			// and only it knows both. Claiming somebody else's viewer id
			// would route input to THIS tab's slot, or drop it: nothing to
			// gain by lying.
			if message.ViewerID != "" {
				httpHost.viewers.Store(message.ViewerID, socketID)
			}
		case "intent":
			// In its own goroutine: the kernel may be asked first (P1), and a
			// slow answer must not stop this tab's beats from being read.
			// The OUTCOME rides this socket back, like every state change.
			go httpHost.submitIntent(Command{
				Kind: CommandKind(message.Action),
				// The VERIFIED identity of the socket, never a claim.
				Username: username,
				SocketID: socketID,
				Target:   message.Target,
			})
		}
	}
}

// submitIntent asks the kernel's one question when a command would seat
// somebody, then pushes it.
func (httpHost *HTTPHost) submitIntent(command Command) {
	if recipient := httpHost.recipientOf(command); recipient != "" && httpHost.playsElsewhere(recipient) {
		httpHost.Host.Refuse(command, ErrPlaysElsewhere)

		return
	}

	httpHost.Host.Submit(command, time.Now())
	httpHost.endIfTerminated()
}

// recipientOf — who would end up holding a pad if this command went through.
// Empty when nobody would: leaving, refusing, ending, asking — and taking a
// lent pad back or returning it, since the owner never left their place here.
func (httpHost *HTTPHost) recipientOf(command Command) string {
	switch command.Kind {
	case CmdJoin:
		return command.Username
	case CmdGrantHandover:
		return httpHost.Host.PendingFrom(command.Username, time.Now())
	case CmdGiveHandover:
		return command.Target
	}

	return ""
}

// playsElsewhere — L'ACCORD DU KERNEL (P1), sur sa seule question : cette
// personne conduit-elle déjà une autre partie ?
//
// Le child décide de sa partie ; ce qu'il ne voit pas, ce sont les AUTRES. La
// règle « une personne, une partie » n'était vérifiée qu'au lancement : on
// pouvait tenir une manette ici et une autre là, par une passation ou un
// simple « rejoindre ». Le kernel les voit toutes — il lit ce que chaque child
// lui rapporte.
//
// SANS RÉPONSE, LE CHILD DÉCIDE SEUL. Une panne du kernel ne doit pas empêcher
// de jouer ; elle peut, au pire, laisser passer une seconde manette.
func (httpHost *HTTPHost) playsElsewhere(username string) bool {
	if username == "" || httpHost.config.KernelURL == "" || httpHost.config.InternalSecret == "" {
		return false
	}

	request, err := http.NewRequest(http.MethodGet,
		strings.TrimSuffix(httpHost.config.KernelURL, "/")+"/internal/sessions/"+url.PathEscape(httpHost.config.SessionID)+
			"/elsewhere/"+url.PathEscape(username), nil)
	if err != nil {
		return false
	}
	request.Header.Set("X-Internal-Secret", httpHost.config.InternalSecret)

	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		log.Printf("[session %s] le kernel ne répond pas sur %s — le child décide seul : %v",
			httpHost.config.SessionID, username, err)

		return false
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	if json.NewDecoder(response.Body).Decode(&body) != nil {
		return false
	}

	return body.SessionID != "" && body.SessionID != httpHost.config.SessionID
}

// endIfTerminated ends the game NOW once its owner has asked — not on the
// sweep's next tick, up to fifteen seconds later, with everybody still
// looking at a game that is over.
func (httpHost *HTTPHost) endIfTerminated() {
	if httpHost.config.EndSession == nil || !httpHost.Host.Terminated() {
		return
	}
	if !httpHost.ending.CompareAndSwap(false, true) {
		return
	}

	go httpHost.config.EndSession(terminationReason(httpHost.Host.Session()))
}

// EndBecauseGameExited — le jeu s'est arrêté de lui-même (Host.GameExited) :
// chaque onglet l'apprend, puis la partie se termine maintenant.
func (httpHost *HTTPHost) EndBecauseGameExited() {
	httpHost.Host.GameExited()
	httpHost.endIfTerminated()
}

// terminationReason — qui a arrêté la partie, pour son journal.
func terminationReason(session *Session) string {
	if session.GameExited {
		return "the game stopped by itself (its emulator exited)"
	}
	if session.TerminatedBy != "" {
		return "an administrator (" + session.TerminatedBy + ") ended the session"
	}

	return "owner ended the session"
}

// serveTerminate — ARRÊTER PAR LE CHILD (P2). Le tableau de bord n'a pas de
// socket vers ce child : il demande au kernel, qui transmet ici avec le nom
// qu'il a vérifié — et c'est le child qui juge, sur SA file, comme pour un
// `terminate` venu d'un onglet.
//
// Le kernel tuait le conteneur après avoir vérifié le propriétaire sur sa
// copie : il intervenait sur une partie sans l'accord de son child. Il ne le
// fait plus que pour un conteneur qui ne répond pas.
//
// Réservé au kernel (X-Internal-Secret) : le nom voyage dans le corps, et
// seul celui qui l'a vérifié peut l'affirmer.
func (httpHost *HTTPHost) serveTerminate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)

		return
	}
	if !auth.SameSecret(r.Header.Get("X-Internal-Secret"), httpHost.config.InternalSecret) {
		auth.Refuse(w, http.StatusForbidden, "forbidden")

		return
	}

	var body struct {
		Username string `json:"username"`
		// Admin — le kernel a vérifié que le demandeur est administrateur.
		Admin bool `json:"admin"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Username == "" {
		http.Error(w, "username required", http.StatusBadRequest)

		return
	}

	outcome := httpHost.Host.Do(Command{Kind: CmdTerminate, Username: body.Username, ByAdmin: body.Admin}, time.Now())

	w.Header().Set("Content-Type", "application/json")
	if outcome.Err != nil {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorMessage": outcome.Err.Error()})

		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "ending": true})

	// Après la réponse : sortir tue ce processus, et le kernel doit savoir
	// que sa demande a été acceptée.
	httpHost.endIfTerminated()
}

// LE RAPPORT SAIT-IL SEULEMENT S'IL ARRIVE ?
//
// Il ne le savait pas. Chaque échec de poussée était journalisé puis oublié,
// si bien qu'un child pouvait rester muet pendant que sa partie allait très
// bien — et personne, chez lui, ne s'en apercevait. C'est le kernel qui devait
// finir par demander.
//
// On garde donc deux choses : la dernière poussée RÉUSSIE, et le nombre
// d'échecs d'affilée. Elles ne servent qu'à une seule question, posée dans
// serveReport : « le kernel a dû me demander — depuis quand est-ce que je ne
// lui parle plus ? »
func (httpHost *HTTPHost) noteMirrorResult(err error) {
	httpHost.mirrorMu.Lock()
	defer httpHost.mirrorMu.Unlock()

	if err == nil {
		httpHost.mirrorLastOK = time.Now()
		httpHost.mirrorFailures = 0

		return
	}
	httpHost.mirrorFailures++
}

func (httpHost *HTTPHost) mirrorHealth() (time.Time, int) {
	httpHost.mirrorMu.Lock()
	defer httpHost.mirrorMu.Unlock()

	return httpHost.mirrorLastOK, httpHost.mirrorFailures
}

func (httpHost *HTTPHost) reportSoon() {
	select {
	case httpHost.reports <- struct{}{}:
	default:
		// One already queued: reports coalesce. A burst of commands owes the
		// kernel one snapshot, not one per command — only the latest matters.
	}
}

// mirrorHeartbeat — à quelle cadence le child redit au kernel qui est vivant.
//
// Le miroir ne partait qu'AU CHANGEMENT, ce qui suffit à l'affichage mais pas
// à la vie : une partie tranquille ne change rien pendant dix minutes, le
// kernel n'entendait plus personne, et son balayage expulsait un joueur qui
// n'avait jamais cessé de jouer (constaté le 2026-09-09 : toute session mourait
// vers la 5ᵉ minute, sur les trois plateformes).
//
// Trente secondes : le même rythme que le battement de la page vers ce child.
// On ne réinvente pas un signal — on répète celui qu'on a déjà.
const mirrorHeartbeat = 30 * time.Second

// reportHeartbeat redit périodiquement ce que le child sait.
//
// LE CHILD EST L'AUTORITÉ, et le kernel l'a écrit lui-même à propos du miroir
// (« the child is the authority, this copy is the only thing the kernel
// knows ») — mais il gardait une autorité RIVALE pour la vie et la mort, un
// registre de battements que le miroir ne touchait jamais. D'où deux avis sur
// « qui est là », et c'est celui qui n'a pas l'information qui décidait.
//
// Un joueur présent dans ce rapport est vivant AU SENS DU CHILD : celui-ci a
// déjà évincé l'onglet gelé (BeatTimeout) et l'absent (AFKAfter). Il n'y a
// donc rien à recalculer plus haut.
func (httpHost *HTTPHost) reportHeartbeat() {
	ticker := time.NewTicker(mirrorHeartbeat)
	defer ticker.Stop()

	for range ticker.C {
		httpHost.reportSoon()
	}
}

func (httpHost *HTTPHost) reportLoop() {
	if httpHost.config.KernelURL == "" || httpHost.config.InternalSecret == "" {
		// RETENU, et pas seulement journalisé au démarrage : sans adresse ni
		// secret, ce child ne rapportera JAMAIS. Une ligne au boot se perd ;
		// la question se reposera quand le kernel viendra demander pourquoi
		// il n'entend rien, et il faut pouvoir y répondre à ce moment-là.
		httpHost.mirrorMu.Lock()
		httpHost.mirrorDisabled = true
		httpHost.mirrorMu.Unlock()
		log.Printf("[session %s] chantier-4: no kernel mirror (missing url/secret)", httpHost.config.SessionID)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := strings.TrimSuffix(httpHost.config.KernelURL, "/") +
		"/internal/sessions/" + httpHost.config.SessionID + "/mirror"

	for range httpHost.reports {
		body, err := json.Marshal(httpHost.Host.Snapshot())
		if err != nil {
			continue
		}

		request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
		if err != nil {
			continue
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Internal-Secret", httpHost.config.InternalSecret)

		response, err := client.Do(request)
		httpHost.noteMirrorResult(err)
		if err != nil {
			// Best effort by design. Retrying would only ever deliver a state
			// the next change is about to replace.
			log.Printf("[session %s] chantier-4: mirror push failed: %v", httpHost.config.SessionID, err)
			continue
		}
		_ = response.Body.Close()
	}
}

func (httpHost *HTTPHost) sweepLoop() {
	for range time.Tick(15 * time.Second) {
		now := time.Now()

		closing, warnings := httpHost.Host.Sweep(now,
			httpHost.config.BeatTimeout, httpHost.config.AFKAfter, httpHost.config.AFKGrace)

		// LA QUESTION EST INDIVIDUELLE. Elle part sur LA socket du joueur
		// concerné, pas en diffusion : les autres jouent peut-être très bien,
		// et une tablée entière avertie parce qu'un seul s'est éloigné est
		// exactement le genre de message qu'on apprend à ignorer.
		for _, warning := range warnings {
			log.Printf("[session %s] toujours là ? — %s n'a rien touché depuis %s",
				httpHost.config.SessionID, warning.Username, httpHost.config.AFKAfter)
			_ = httpHost.sendTo(warning.SocketID, map[string]any{
				"type":     "afk",
				"deadline": warning.Deadline.UTC().Format(time.RFC3339),
				"graceMs":  httpHost.config.AFKGrace.Milliseconds(),
			})
		}

		// FERMER EN DISANT POURQUOI. « socket fermée » n'apprend rien sur un
		// défaut qui met une demi-heure à se produire ; « parti sans répondre
		// à toujours là ? » se lit tout seul six semaines plus tard.
		for _, closed := range httpHost.Host.LastClosed() {
			log.Printf("[session %s] %s exclu : %s%s",
				httpHost.config.SessionID, closed.Username, closed.Why,
				map[bool]string{true: " — c'était le propriétaire : sa manette lui reste réservée, la partie continue tant que quelqu'un joue", false: ""}[closed.Owner])
		}

		for _, socketID := range closing {
			if value, ok := httpHost.sockets.Load(socketID); ok {
				_ = value.(*hostSocket).conn.Close()
			}
		}

		reason := httpHost.endReason(now)
		if reason == "" {
			continue
		}

		// ON NE S'ARRÊTE QUE SI QUELQU'UN DÉMOLIT DERRIÈRE.
		//
		// Cette boucle rendait la main après avoir signalé la fin. C'est juste
		// pour un CONTENEUR : il meurt dans la foulée, et il n'y aura pas de
		// session suivante à surveiller.
		//
		// Sur un téléphone il n'y a pas de conteneur. Le processus survit, la
		// partie suivante démarre dans le même — et plus personne ne balayait
		// ses sockets morts ni ses joueurs absents. La PREMIÈRE session allait
		// toujours bien ; les suivantes vivaient sans surveillance, ce qui a
		// coûté plusieurs soirées de diagnostic parce qu'installer une
		// nouvelle version remettait tout à zéro et « réglait » le problème
		// (signalé le 2026-09-05).
		//
		// Sans démolisseur, on continue : il y aura une partie d'après.
		if httpHost.config.EndSession == nil {
			continue
		}
		if !httpHost.ending.CompareAndSwap(false, true) {
			return // un `terminate` l'a déjà arrêtée
		}

		log.Printf("[session %s] la partie s'arrête d'elle-même : %s — le kernel n'a rien décidé",
			httpHost.config.SessionID, reason)
		httpHost.config.EndSession(reason)

		return
	}
}

// endReason reports why this session should stop, "" while it should not. The
// child decides its own end: the kernel used to run this clock for everybody,
// which meant deciding from a copy — and the copy is what kept diverging.
func (httpHost *HTTPHost) endReason(now time.Time) string {
	current := httpHost.Host.Session()

	if current.Terminated {
		return terminationReason(current)
	}
	// ZÉRO VEUT DIRE « JAMAIS », PAS « TOUT DE SUITE ».
	//
	// Sans cette garde, `IdleAfter: 0` rendait la comparaison vraie dès la
	// première seconde — toute session non nulle était déclarée inactive. Le
	// téléphone, qui pose délibérément zéro pour tenir cette horloge lui-même,
	// voyait donc sa boucle rendre la main quinze secondes après le démarrage,
	// à chaque fois (2026-09-05).
	if httpHost.config.IdleAfter > 0 &&
		current.IdleSince != nil && now.Sub(*current.IdleSince) > httpHost.config.IdleAfter {
		return "nobody has played for " + httpHost.config.IdleAfter.String()
	}

	return ""
}
