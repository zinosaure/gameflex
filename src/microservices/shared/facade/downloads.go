package facade

// LES TÉLÉCHARGEMENTS EN TÂCHE DE FOND, LEUR PROGRESSION, ET LA FILE DES JEUX —
// partagés par les deux kernels.
//
// La salle des téléchargements est née au serveur (chantier 53, P5, #671 ; le
// porteur, 2026-11-13 : « Les téléchargements doivent aussi afficher la
// progression ») : chaque téléchargement était un POST qui répondait une seule
// fois, à la fin, et la page tournait sans rien savoir. Elle passe ici, dans le
// paquet du centre de messages, pour que le téléphone serve la même commande
// `download` par le même code (chantier 54, #680, décision 1 : « le même
// protocole et le même code, pas une copie »).
//
// LE PATRON EST CELUI DU LANCEMENT :
//
//   - la commande `download` du centre de messages, que le kernel accepte en
//     frappant un identifiant, rendu tout de suite (Start) ;
//   - le geste tourne ENSUITE, détaché de la connexion : quitter la page ne
//     l'arrête pas ;
//   - un salon par téléchargement, `download/<id>`, qui rejoue son dernier état
//     à qui s'abonne tard (Subscribe) ;
//   - une durée de vie bornée, un canal consultatif : publier ne coûte qu'un
//     verrou, un abonné lent perd des étapes plutôt que de ralentir la copie.
//
// LA FILE DES JEUX (le porteur, 2026-11-16 : « c'est possible de mettre les jeux
// à télécharger en queue ? ») : un geste confié avec `Queued` attend son tour —
// plusieurs jeux demandés à la suite se téléchargent l'un après l'autre, dans
// l'ordre où ils ont été demandés. Le KERNEL garde la file : elle survit à une
// page fermée, et la page qui revient la relit (List, le sujet `downloads`). Un
// jeu de la file dit son rang (`position`, 1 : le prochain) ; le retirer est la
// même commande que l'annuler (`download.cancel`). Le serveur y met ses jeux et
// garde le reste en parallèle (un émulateur, un BIOS, un correctif ne
// patientent pas derrière un jeu de plusieurs dizaines de Go) ; le téléphone y
// met tout, puisqu'il ne télécharge qu'une chose à la fois (chantier 54,
// décision 9). La file vit en mémoire : un kernel qui redémarre la vide, comme
// il coupe le téléchargement en cours.
//
// LES OCTETS SE COMPTENT À LA COPIE : chaque copie d'un téléchargement écrit à
// travers ProgressWriter, qui n'existe que si le contexte porte un
// téléchargement suivi — le même code sert un geste qui ne compte rien. Une
// copie faite ailleurs (l'application du téléphone) se relaie par
// ReportProgress. Le total vient de la réponse (Content-Length) ou, à défaut, de
// la commande (la taille que la liste annonçait). Un état part au plus quatre
// fois par seconde.
//
// UN SALON APPARTIENT À CELUI QUI L'A OUVERT : seul ce compte s'y abonne, le
// voit dans sa liste, et l'annule.
//
// ANNULER (le porteur, 2026-11-14, « 3. A ») : un geste en cours a son contexte
// annulé — ProgressWriter refuse alors d'écrire, la copie s'arrête, le geste
// retire son fichier partiel, et le salon finit en `failed`, « cancelled ». Un
// jeu de la file en sort sans avoir commencé, et finit de la même façon.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	downloadStepBuffer = 8
	// downloadTTL — un téléchargement plus long, compté depuis son départ, est
	// abandonné par le salon (pas par la copie, qui continue) : un jeu de
	// plusieurs dizaines de Go chez un pair lent tient dans ce délai. Un jeu
	// qui attend dans la file n'a pas encore commencé : il ne vieillit pas.
	downloadTTL = 6 * time.Hour
	// downloadGrace — combien de temps un téléchargement fini reste lisible.
	downloadGrace = 5 * time.Minute
	// downloadEvery — l'écart minimal entre deux états publiés.
	downloadEvery = 250 * time.Millisecond
)

// DownloadCancelled — la raison d'un téléchargement annulé, ou retiré de la file.
const DownloadCancelled = "cancelled"

// ErrDownloadPending — le même téléchargement (`Key`) attend déjà, ou tourne.
var ErrDownloadPending = errors.New("this download is already queued or running")

// DownloadStep — un état : `queued` (son rang dans la file), `progress` (reçu,
// total), puis `done` (avec la réponse du geste) ou `failed` (avec sa raison).
type DownloadStep struct {
	Type     string `json:"type"`
	Position int    `json:"position,omitempty"`
	Received int64  `json:"received"`
	Total    int64  `json:"total,omitempty"`
	// Phase — l'étape de la copie (ReportPhase) : `download`, puis `unpack` pour un jeu reçu en archive.
	Phase string `json:"phase,omitempty"`
	Error string `json:"error,omitempty"`
	// ErrorCode — le code du refus (chantier 59, P5), que la page traduit ; Error n'est que son message.
	ErrorCode string          `json:"errorCode,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	At        int64           `json:"at"`
}

// DownloadJob — ce qu'un kernel confie : à qui il est (`Owner`), ce que la page
// en montre (`Label`, `Key` : ce qu'on télécharge, `Source` : d'où), le total
// annoncé, s'il prend la file (`Queued`), et le geste lui-même (`Run`), qui
// rend le statut et le corps qu'aurait rendus sa route.
type DownloadJob struct {
	Owner  string
	Label  string
	Key    string
	Source string
	Total  int64
	Queued bool
	// Kind — ce que le geste fait : "" un téléchargement, `remove` la suppression d'un jeu (RemoveTree), qui se suit
	// de même et que la page montre dans ses notifications (le porteur, 2026-11-17 : « La suppression des gros jeux
	// c'est long. Il faudra le mettre dans notifications avec la progression »).
	Kind string
	Run  func(ctx context.Context) (status int, answer []byte)
}

// DownloadEntry — un téléchargement qui attend ou tourne, tel que la liste de
// son propriétaire le montre (le sujet `downloads`).
type DownloadEntry struct {
	DownloadID string `json:"downloadId"`
	Key        string `json:"key,omitempty"`
	Label      string `json:"label,omitempty"`
	Source     string `json:"source,omitempty"`
	// State — `queued` (il attend son tour) ou `running`.
	State string `json:"state"`
	// Position — son rang dans la file, 1 pour le prochain ; 0 quand il tourne.
	Position int   `json:"position,omitempty"`
	Received int64 `json:"received"`
	Total    int64 `json:"total,omitempty"`
	// Phase — l'étape (DownloadStep.Phase), "" tant que personne ne l'a dite.
	Phase string `json:"phase,omitempty"`
	// Kind — DownloadJob.Kind : "" un téléchargement, `remove` une suppression.
	Kind   string `json:"kind,omitempty"`
	Queued bool   `json:"queued"`
	At     int64  `json:"at"`
}

type downloadRoom struct {
	id          string
	job         DownloadJob
	cancel      context.CancelFunc
	cancelled   bool
	waiting     bool
	createdAt   time.Time
	startedAt   time.Time
	finishedAt  time.Time
	received    int64
	total       int64
	phase       string
	sentAt      time.Time
	last        DownloadStep
	subscribers []chan DownloadStep
}

// Downloads — les téléchargements d'un kernel, et sa file. La valeur zéro est
// prête à servir.
type Downloads struct {
	mu    sync.Mutex
	rooms map[string]*downloadRoom
	// queue — les jeux qui attendent, dans l'ordre de leur demande.
	queue []string
	// running — le jeu de la file qui tourne, "" sinon : un seul à la fois.
	running  string
	watchers map[chan struct{}]struct{}
}

// newDownloadID — un identifiant de téléchargement. Il ADRESSE un salon, il ne
// protège rien (le canal exige un jeton) ; aléatoire tout de même, pour qu'un
// compte ne lise pas ceux des autres en devinant.
func newDownloadID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return hex.EncodeToString(raw)
}

func (downloads *Downloads) ready() {
	if downloads.rooms == nil {
		downloads.rooms = map[string]*downloadRoom{}
	}
	if downloads.watchers == nil {
		downloads.watchers = map[chan struct{}]struct{}{}
	}
}

func (downloads *Downloads) sweep() {
	for id, room := range downloads.rooms {
		expired := !room.startedAt.IsZero() && room.finishedAt.IsZero() && time.Since(room.startedAt) > downloadTTL
		consumed := !room.finishedAt.IsZero() && time.Since(room.finishedAt) > downloadGrace
		if expired || consumed {
			for _, subscriber := range room.subscribers {
				close(subscriber)
			}
			delete(downloads.rooms, id)
		}
	}
}

// signal — réveiller les listes (le sujet `downloads`) : un signal en attente
// suffit, un abonné relit tout.
func (downloads *Downloads) signal() {
	for watcher := range downloads.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

// Start — confier un geste. Il part tout de suite, ou prend la file (`Queued`) :
// il partira quand ceux d'avant auront fini. Rend son identifiant, ou
// ErrDownloadPending quand la même chose (`Key`) attend déjà ou tourne.
func (downloads *Downloads) Start(job DownloadJob) (string, error) {
	downloads.mu.Lock()
	downloads.ready()
	downloads.sweep()
	if job.Key != "" {
		for _, room := range downloads.rooms {
			if room.job.Key == job.Key && room.finishedAt.IsZero() {
				downloads.mu.Unlock()

				return "", ErrDownloadPending
			}
		}
	}
	room := &downloadRoom{id: newDownloadID(), job: job, createdAt: time.Now(), total: max(job.Total, 0)}
	downloads.rooms[room.id] = room
	var launch *downloadRoom
	if job.Queued {
		room.waiting = true
		downloads.queue = append(downloads.queue, room.id)
		room.last = DownloadStep{Type: "queued", Position: len(downloads.queue), Total: room.total, At: time.Now().UnixMilli()}
		launch = downloads.nextLocked()
	} else {
		launch = room
	}
	if launch != nil {
		downloads.launchLocked(launch)
	}
	downloads.signal()
	downloads.mu.Unlock()

	return room.id, nil
}

// nextLocked — le prochain jeu de la file, quand aucun ne tourne ; nil sinon.
func (downloads *Downloads) nextLocked() *downloadRoom {
	if downloads.running != "" || len(downloads.queue) == 0 {
		return nil
	}
	id := downloads.queue[0]
	downloads.queue = downloads.queue[1:]
	room := downloads.rooms[id]
	room.waiting = false
	downloads.running = id
	downloads.renumberLocked()

	return room
}

// renumberLocked — le rang de chaque jeu qui attend encore, redit à ses abonnés.
func (downloads *Downloads) renumberLocked() {
	for index, id := range downloads.queue {
		if room := downloads.rooms[id]; room != nil && room.last.Position != index+1 {
			downloads.sendLocked(room, DownloadStep{Type: "queued", Position: index + 1, Total: room.total})
		}
	}
}

// launchLocked — faire partir le geste d'un salon, détaché de toute connexion.
func (downloads *Downloads) launchLocked(room *downloadRoom) {
	ctx, cancel := context.WithCancel(context.Background())
	room.cancel = cancel
	room.startedAt = time.Now()
	downloads.sendLocked(room, DownloadStep{Type: "progress", Received: room.received, Total: room.total})
	run := room.job.Run
	go func() {
		defer cancel()
		status, answer := 500, ErrorBody("server_failed", "nothing to run")
		if run != nil {
			status, answer = run(WithDownload(ctx, downloads, room.id))
		}
		downloads.finish(room.id, status, answer)
	}()
}

func (downloads *Downloads) sendLocked(room *downloadRoom, step DownloadStep) {
	step.At = time.Now().UnixMilli()
	room.last = step
	room.sentAt = time.Now()
	for _, subscriber := range room.subscribers {
		select {
		case subscriber <- step:
		default:
		}
	}
	downloads.signal()
}

// add — des octets de plus ; `contentLength`, ce que la réponse annonce, sert
// de total quand la commande n'en donnait pas (un geste d'un seul fichier : un
// dossier de plusieurs fichiers, lui, vient avec la taille que la liste
// annonçait).
func (downloads *Downloads) add(id string, written, contentLength int64) {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	room := downloads.rooms[id]
	if room == nil || !room.finishedAt.IsZero() {
		return
	}
	room.received += written
	if room.total == 0 && contentLength > 0 {
		room.total = contentLength
	}
	if time.Since(room.sentAt) >= downloadEvery {
		downloads.sendLocked(room, DownloadStep{Type: "progress", Received: room.received, Total: room.total, Phase: room.phase})
	}
}

// report — où en est une copie faite ailleurs, en absolu (ReportProgress) ; `phase`, son étape quand elle la dit. Une
// étape nouvelle part tout de suite : la page la montre dès qu'elle change.
func (downloads *Downloads) report(id string, received, total int64, phase string) {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	room := downloads.rooms[id]
	if room == nil || !room.finishedAt.IsZero() {
		return
	}
	changed := phase != "" && phase != room.phase
	if phase != "" {
		room.phase = phase
	}
	// Un reçu négatif ne dit que l'étape (ReportPhase(ctx, "sheet", -1, 0)) : les octets restent ceux d'avant.
	if received >= 0 {
		room.received = received
	}
	if total > 0 {
		room.total = total
	}
	if received < 0 && !changed {
		return
	}
	if changed || time.Since(room.sentAt) >= downloadEvery {
		downloads.sendLocked(room, DownloadStep{Type: "progress", Received: room.received, Total: room.total, Phase: room.phase})
	}
}

// finish — la réponse du geste, comme sa route l'aurait rendue ; puis le jeu
// suivant de la file, s'il y en a un.
func (downloads *Downloads) finish(id string, status int, answer []byte) {
	downloads.mu.Lock()
	var launch *downloadRoom
	if downloads.running == id {
		downloads.running = ""
		launch = downloads.nextLocked()
	}
	if room := downloads.rooms[id]; room != nil && room.finishedAt.IsZero() {
		downloads.finishLocked(room, status, answer)
	}
	if launch != nil {
		downloads.launchLocked(launch)
	}
	downloads.signal()
	downloads.mu.Unlock()
}

func (downloads *Downloads) finishLocked(room *downloadRoom, status int, answer []byte) {
	step := DownloadStep{Received: room.received, Total: room.total}
	if status >= 200 && status < 300 {
		step.Type = "done"
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(answer, &envelope) == nil && len(envelope.Data) > 0 {
			step.Result = envelope.Data
		}
	} else {
		var body struct {
			ErrorCode    string `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
			Error        string `json:"error"`
		}
		_ = json.Unmarshal(answer, &body)
		step.Type, step.Error, step.ErrorCode = "failed", body.ErrorMessage, body.ErrorCode
		if step.Error == "" {
			step.Error = body.Error
		}
		if room.cancelled {
			step.Error = DownloadCancelled
		}
	}
	downloads.sendLocked(room, step)
	for _, subscriber := range room.subscribers {
		close(subscriber)
	}
	room.subscribers = nil
	room.finishedAt = time.Now()
}

// Cancel — arrêter le téléchargement `id`, ou le retirer de la file : seulement
// le compte qui l'a lancé, et seulement s'il n'a pas fini. Faux sinon.
func (downloads *Downloads) Cancel(id, owner string) bool {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	room := downloads.rooms[id]
	if room == nil || room.job.Owner != owner || !room.finishedAt.IsZero() {
		return false
	}
	room.cancelled = true
	if room.waiting {
		for index, waiting := range downloads.queue {
			if waiting == id {
				downloads.queue = append(downloads.queue[:index], downloads.queue[index+1:]...)

				break
			}
		}
		room.waiting = false
		downloads.finishLocked(room, 499, ErrorBody(DownloadCancelled, DownloadCancelled))
		downloads.renumberLocked()
		downloads.signal()

		return true
	}
	if room.cancel != nil {
		room.cancel()
	}

	return true
}

// Subscribe — le dernier état, puis la suite ; faux si le salon n'existe pas ou
// n'est pas à ce compte.
func (downloads *Downloads) Subscribe(id, owner string) (<-chan DownloadStep, DownloadStep, func(), bool) {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	downloads.ready()
	downloads.sweep()
	room := downloads.rooms[id]
	if room == nil || room.job.Owner != owner {
		return nil, DownloadStep{}, func() {}, false
	}
	channel := make(chan DownloadStep, downloadStepBuffer)
	if !room.finishedAt.IsZero() {
		close(channel)

		return channel, room.last, func() {}, true
	}
	room.subscribers = append(room.subscribers, channel)
	unsubscribe := func() {
		downloads.mu.Lock()
		defer downloads.mu.Unlock()
		current := downloads.rooms[id]
		if current == nil {
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

	return channel, room.last, unsubscribe, true
}

// List — les téléchargements de `owner` qui tournent, puis ceux qui attendent,
// dans l'ordre de la file.
func (downloads *Downloads) List(owner string) []DownloadEntry {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	downloads.ready()
	positions := map[string]int{}
	for index, id := range downloads.queue {
		positions[id] = index + 1
	}
	entries := []DownloadEntry{}
	for _, room := range downloads.rooms {
		if room.job.Owner != owner || !room.finishedAt.IsZero() {
			continue
		}
		entry := DownloadEntry{DownloadID: room.id, Key: room.job.Key, Label: room.job.Label, Source: room.job.Source, State: "running",
			Received: room.received, Total: room.total, Phase: room.phase, Kind: room.job.Kind, Queued: room.job.Queued, At: room.createdAt.UnixMilli()}
		if room.waiting {
			entry.State, entry.Position = "queued", positions[room.id]
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if (entries[left].State == "running") != (entries[right].State == "running") {
			return entries[left].State == "running"
		}
		if entries[left].Position != entries[right].Position {
			return entries[left].Position < entries[right].Position
		}

		return entries[left].At < entries[right].At
	})

	return entries
}

// Watch — un réveil à chaque changement d'un téléchargement (un départ, une
// étape, une fin, un rang), pour la liste (PushOnChange) ; et de quoi se
// désabonner.
func (downloads *Downloads) Watch() (<-chan struct{}, func()) {
	downloads.mu.Lock()
	defer downloads.mu.Unlock()
	downloads.ready()
	watcher := make(chan struct{}, 1)
	downloads.watchers[watcher] = struct{}{}

	return watcher, func() {
		downloads.mu.Lock()
		defer downloads.mu.Unlock()
		delete(downloads.watchers, watcher)
	}
}

// LE TÉLÉCHARGEMENT SUIVI D'UN CONTEXTE, s'il y en a un.
type downloadKey struct{}

type downloadTracker struct {
	downloads *Downloads
	id        string
}

// WithDownload — un contexte qui porte le téléchargement `id` : ses copies
// comptent leurs octets (ProgressWriter), une copie faite ailleurs se relaie
// (ReportProgress).
func WithDownload(ctx context.Context, downloads *Downloads, id string) context.Context {
	return context.WithValue(ctx, downloadKey{}, downloadTracker{downloads: downloads, id: id})
}

// ProgressWriter — `out`, qui compte ce qu'il reçoit quand le contexte porte un
// téléchargement suivi ; `out` tel quel sinon.
func ProgressWriter(ctx context.Context, out io.Writer, contentLength int64) io.Writer {
	tracker, ok := ctx.Value(downloadKey{}).(downloadTracker)
	if !ok || tracker.downloads == nil {
		return out
	}

	return &countingWriter{ctx: ctx, out: out, tracker: tracker, contentLength: contentLength}
}

// ReportProgress — où en est une copie que le kernel ne fait pas lui-même
// (l'application du téléphone, qui publie sa progression sur son bus) : reçu et
// total, en absolu. Sans téléchargement suivi, rien.
func ReportProgress(ctx context.Context, received, total int64) {
	ReportPhase(ctx, "", received, total)
}

// ReportPhase — la même, avec l'étape de la copie (`download`, `unpack`, `sheet` : la fiche du jeu recopiée de son
// serveur, `remove`) : chaque étape dit qu'elle travaille, au lieu d'une barre arrêtée (le porteur, 2026-11-17). ""
// garde l'étape d'avant ; un `received` négatif ne change que l'étape.
func ReportPhase(ctx context.Context, phase string, received, total int64) {
	if tracker, ok := ctx.Value(downloadKey{}).(downloadTracker); ok && tracker.downloads != nil {
		tracker.downloads.report(tracker.id, received, total, phase)
	}
}

type countingWriter struct {
	ctx           context.Context
	out           io.Writer
	tracker       downloadTracker
	contentLength int64
}

// Write — refuse d'écrire un téléchargement annulé : la copie s'arrête sur
// cette erreur, d'où qu'elle lise (un pair, un disque, le buildbot).
func (writer *countingWriter) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	written, err := writer.out.Write(data)
	if written > 0 {
		writer.tracker.downloads.add(writer.tracker.id, int64(written), writer.contentLength)
	}

	return written, err
}

// RemoveTree — efface `path` (un fichier, ou un dossier et tout ce qu'il porte) en disant sa progression : le total
// d'abord, puis chaque fichier effacé (ReportPhase, `remove`). Un gros jeu PS4 met des minutes à partir, et la page
// ne voyait rien. L'annulation n'est pas lue : un jeu à moitié effacé ne servirait plus.
func RemoveTree(ctx context.Context, path string) error {
	var total int64
	var files []string
	var dirs []string
	walkErr := filepath.WalkDir(path, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, file)

			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		files = append(files, file)

		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return walkErr
	}
	ReportPhase(ctx, "remove", 0, total)
	var removed int64
	for _, file := range files {
		info, _ := os.Lstat(file)
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if info != nil {
			removed += info.Size()
		}
		ReportPhase(ctx, "remove", removed, total)
	}
	// Les dossiers, du plus profond au plus haut ; ce qui resterait (un fichier apparu entre-temps) part avec RemoveAll.
	for index := len(dirs) - 1; index >= 0; index-- {
		_ = os.Remove(dirs[index])
	}

	return os.RemoveAll(path)
}
