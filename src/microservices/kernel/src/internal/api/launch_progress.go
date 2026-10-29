package api

// LA PROGRESSION D'UN LANCEMENT, AVANT QU'UNE SESSION EXISTE (#191).
//
// Le problème, en une phrase : entre le clic sur « jouer » et la réponse du
// POST, dix étapes se déroulent côté serveur — garde « déjà en train de
// jouer », plafond, mise en cache d'un jeu distant (qui peut durer des
// minutes), vérification du cœur, création du conteneur, attente de
// disponibilité jusqu'à quatre-vingt-dix secondes — et le navigateur attend
// une unique réponse HTTP, sans rien savoir. Le serveur CONNAÎT toutes ces
// étapes : il les écrit dans ses journaux, c'est-à-dire au seul endroit où
// celui qui attend ne peut pas regarder.
//
// UN IDENTIFIANT DE LANCEMENT, et pas la session : il n'y a pas encore de
// `sessionId`, c'est justement ce que le lancement va rendre.
//
// Le navigateur le FABRIQUAIT (2026-09-18) : il ouvrait un canal dessus, puis
// postait — le POST et le canal étaient deux connexions, et il fallait que le
// second existe avant que la première étape ne survienne. Depuis que le
// lancement est une commande du centre de messages (facade_ws.go), c'est le
// kernel qui le frappe : les étapes reviennent sur l'`id` de la commande, et
// l'identifiant ne sert plus qu'à la page de jeu qui s'ouvre ensuite et
// rejoue l'histoire en s'abonnant à `launch/<launchId>`.
//
// CE CANAL EST CONSULTATIF, ET C'EST UNE PROPRIÉTÉ, PAS UN PIS-ALLER. Un
// lancement ne dépend jamais de lui : personne ne l'écoute, `publish` ne coûte
// qu'un verrou ; un abonné trop lent perd des étapes plutôt que de retarder la
// création du conteneur. Un écran d'attente ne doit pas pouvoir ralentir ce
// qu'il décrit.

import (
	"encoding/json"
	"sync"
	"time"
)

// launchStepBuffer — ce qu'un abonné peut prendre de retard sans rien perdre.
// Les étapes sont rares (une dizaine pour un lancement), donc une marge de
// seize couvre largement un navigateur occupé à peindre.
const launchStepBuffer = 16

// launchTTL — au-delà, un lancement qu'on n'a jamais fini est abandonné.
// Généreux devant le pire cas réel (mise en cache d'un jeu distant, puis
// quatre-vingt-dix secondes d'attente) : ce délai protège d'une fuite, il
// n'arbitre rien.
const launchTTL = 10 * time.Minute

// launchGrace — combien de temps un lancement TERMINÉ reste consultable. Assez
// pour qu'une page qui navigue juste après la réponse du POST rejoue tout ce
// qui s'est passé, assez court pour qu'un identifiant ne traîne pas.
const launchGrace = 60 * time.Second

// launchWaitingRooms — les salons qu'un ABONNÉ a ouverts sur un identifiant que
// le kernel n'a pas frappé. Un tel salon ne vit que `launchGrace`, et il n'y en
// a jamais plus que ceci : chaque abonnement à un identifiant inventé en
// gardait un dix minutes, sans limite (revue de sécurité, #601). Les salons du
// kernel (`open`, ou une étape publiée) ne comptent pas et vivent `launchTTL` :
// un lancement Remote Play ne publie rien pendant qu'il réveille une console.
const launchWaitingRooms = 64

// launchStep — une étape, telle qu'elle part sur le fil.
//
// Step est un IDENTIFIANT stable que la page traduit, jamais une phrase : ce
// service ne connaît pas la langue de celui qui regarde. Detail porte le
// renseignement qui n'a de sens que pour cette étape (le nom d'un cœur, une
// géométrie), et Error la remplace quand l'étape a échoué.
type launchStep struct {
	Type      string `json:"type"`
	Step      string `json:"step"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	At        int64  `json:"at"`
}

type launchRoom struct {
	createdAt time.Time
	// announced — le kernel a frappé cet identifiant (`open`, ou une étape) ;
	// faux pour un salon qu'un abonné a ouvert seul.
	announced bool
	// finishedAt — quand le lancement s'est terminé, réussi ou non. Le salon
	// SURVIT à sa fin pendant `launchGrace`, et c'est ce qui rend le dessin
	// possible : la page des jeux attend la réponse du POST (pour garder son
	// traitement du conflit « tu joues déjà ailleurs », dont le bouton de
	// reprise appartient à ce composant), puis navigue. Elle arrive donc APRÈS
	// la dernière étape — sans ce délai, elle trouverait un salon effacé et
	// tout le lancement serait perdu pour l'écran qui doit le montrer.
	finishedAt  time.Time
	subscribers []chan launchStep
	// history rejoue ce qui a déjà eu lieu. Le navigateur ouvre le canal avant
	// de poster, mais rien ne garantit l'ordre d'arrivée de deux requêtes
	// distinctes : sans ce rappel, une étape publiée pendant la poignée de
	// main serait perdue, et l'écran afficherait un trou.
	history []launchStep
}

// launchProgress — les lancements en cours et ce qu'ils ont annoncé.
//
// Zéro-valeur utilisable, comme `listing.Rescans` : rien à initialiser dans le
// constructeur du serveur, donc rien à oublier d'y ajouter.
type launchProgress struct {
	mu    sync.Mutex
	rooms map[string]*launchRoom
}

// sweep retire les lancements abandonnés. Appelée depuis les deux chemins qui
// prennent déjà le verrou — pas de goroutine dédiée pour une carte qui compte
// au plus quelques entrées.
func (progress *launchProgress) sweep() {
	for id, room := range progress.rooms {
		expired := time.Since(room.createdAt) > launchTTL
		consumed := !room.finishedAt.IsZero() && time.Since(room.finishedAt) > launchGrace
		unannounced := !room.announced && time.Since(room.createdAt) > launchGrace

		if expired || consumed || unannounced {
			for _, subscriber := range room.subscribers {
				close(subscriber)
			}
			delete(progress.rooms, id)
		}
	}
}

// waiting — les salons que seul un abonné a ouverts. Le verrou est tenu par
// l'appelant.
func (progress *launchProgress) waiting() int {
	progress.sweep()
	count := 0
	for _, room := range progress.rooms {
		if !room.announced {
			count++
		}
	}

	return count
}

func (progress *launchProgress) room(id string) *launchRoom {
	if progress.rooms == nil {
		progress.rooms = map[string]*launchRoom{}
	}
	progress.sweep()

	if existing, known := progress.rooms[id]; known {
		return existing
	}

	created := &launchRoom{createdAt: time.Now()}
	progress.rooms[id] = created

	return created
}

// subscribe rend le canal d'un abonné ET ce qui a déjà eu lieu.
func (progress *launchProgress) subscribe(id string) (<-chan launchStep, []launchStep, func()) {
	progress.mu.Lock()
	defer progress.mu.Unlock()

	if progress.rooms[id] == nil && progress.waiting() >= launchWaitingRooms {
		// Trop de salons d'attente : l'abonné voit un flux vide et fermé, comme
		// celui d'un lancement fini sans histoire.
		closed := make(chan launchStep)
		close(closed)

		return closed, nil, func() {}
	}
	room := progress.room(id)
	channel := make(chan launchStep, launchStepBuffer)
	replay := append([]launchStep(nil), room.history...)

	// DÉJÀ TERMINÉ : on rejoue tout, et on ferme aussitôt. L'abonné voit le
	// lancement entier puis la fin du flux — il n'attend pas une suite qui ne
	// viendra jamais.
	if !room.finishedAt.IsZero() {
		close(channel)

		return channel, replay, func() {}
	}

	room.subscribers = append(room.subscribers, channel)

	unsubscribe := func() {
		progress.mu.Lock()
		defer progress.mu.Unlock()

		current, known := progress.rooms[id]
		if !known {
			return
		}

		for position, subscriber := range current.subscribers {
			if subscriber == channel {
				current.subscribers = append(current.subscribers[:position], current.subscribers[position+1:]...)
				close(channel)

				break
			}
		}
	}

	return channel, replay, unsubscribe
}

// send diffuse une étape. Le verrou est déjà tenu par l'appelant.
func (progress *launchProgress) send(id string, step launchStep) {
	if id == "" {
		return
	}
	step.At = time.Now().UnixMilli()

	room := progress.room(id)
	room.announced = true
	room.history = append(room.history, step)

	for _, subscriber := range room.subscribers {
		// NON BLOQUANT : un abonné lent perd une étape, il ne retarde pas la
		// création du conteneur. C'est tout l'esprit de ce canal.
		select {
		case subscriber <- step:
		default:
		}
	}
}

// open — le salon d'un lancement que le kernel vient de frapper, avant toute
// étape : l'abonnement qui suit le trouve à lui.
func (progress *launchProgress) open(id string) {
	if id == "" {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.room(id).announced = true
}

// publish annonce une étape franchie.
func (progress *launchProgress) publish(id, step, detail string) {
	if id == "" {
		return
	}

	progress.mu.Lock()
	defer progress.mu.Unlock()

	progress.send(id, launchStep{Type: "progress", Step: step, Detail: detail})
}

// fail annonce l'étape qui a échoué, et termine le lancement.
func (progress *launchProgress) fail(id, step, reason string) {
	if id == "" {
		return
	}

	progress.mu.Lock()
	defer progress.mu.Unlock()

	progress.send(id, launchStep{Type: "failed", Step: step, Error: reason})
	progress.closeRoom(id)
}

// done annonce la session obtenue, et termine le lancement.
func (progress *launchProgress) done(id, sessionID string) {
	if id == "" {
		return
	}

	progress.mu.Lock()
	defer progress.mu.Unlock()

	progress.send(id, launchStep{Type: "done", Step: "session.ready", SessionID: sessionID})
	progress.closeRoom(id)
}

// closeRoom ferme les canaux mais GARDE l'histoire le temps de la grâce. Le
// verrou est déjà tenu.
//
// L'inverse — oublier aussitôt — était le premier jet, et il rendait le dessin
// impossible : la page arrive après la réponse du POST, donc après la dernière
// étape. Elle aurait trouvé un salon vide et n'aurait eu à montrer que ce que
// le conteneur dit ENSUITE, en laissant croire que rien ne s'était passé avant.
func (progress *launchProgress) closeRoom(id string) {
	room, known := progress.rooms[id]
	if !known {
		return
	}

	for _, subscriber := range room.subscribers {
		close(subscriber)
	}
	room.subscribers = nil
	room.finishedAt = time.Now()
}

// settle termine un lancement que son gestionnaire a laissé ouvert.
//
// Deux chemins rendaient leur réponse sans rien dire au salon — une image de
// console manquante, et tout lancement remote-play, qui ne publie aucune
// étape. Celui qui écoutait attendait alors jusqu'à l'expiration (dix
// minutes) une fin qui ne venait pas. Le centre de messages, lui, connaît la
// réponse : il la reporte ici, et un lancement déjà terminé reste tel quel.
//
// L'échec est placé sur la dernière étape franchie — c'est là que le
// lancement s'est arrêté — ou sur la demande elle-même quand rien n'avait
// commencé.
func (progress *launchProgress) settle(id string, status int, answer []byte) {
	if id == "" {
		return
	}

	progress.mu.Lock()
	defer progress.mu.Unlock()

	room := progress.room(id)
	if !room.finishedAt.IsZero() {
		return
	}

	var body struct {
		ErrorMessage string `json:"errorMessage"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(answer, &body)

	if status >= 200 && status < 300 {
		progress.send(id, launchStep{Type: "done", Step: "session.ready", SessionID: answeredSessionID(answer)})
		progress.closeRoom(id)

		return
	}

	step := "session.requested"
	if last := len(room.history); last > 0 {
		step = room.history[last-1].Step
	}
	reason := body.ErrorMessage
	if reason == "" {
		reason = body.Error
	}

	progress.send(id, launchStep{Type: "failed", Step: step, Error: reason})
	progress.closeRoom(id)
}

// answeredSessionID — la partie qu'un lancement a créée, lue dans sa réponse :
// `sessionId` au rétro et au Remote Play, `session_id` à streamex.
func answeredSessionID(answer []byte) string {
	var body struct {
		Data struct {
			SessionID       string `json:"sessionId"`
			StreamSessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(answer, &body)
	if body.Data.SessionID != "" {
		return body.Data.SessionID
	}

	return body.Data.StreamSessionID
}
