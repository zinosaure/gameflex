// Package logfile — les journaux SUR LE DISQUE, à côté des données.
//
// `docker logs` meurt avec le conteneur. C'est vrai de tous les services, et
// c'est surtout vrai des conteneurs de SESSION : ils sont créés pour une
// partie, détruits à la fin, et c'est justement quand une partie s'est mal
// passée qu'on voudrait relire ce qu'ils ont dit. Le kernel le savait déjà à
// moitié — `containerLogTail` garde les dernières lignes d'un enfant mort,
// « the only trace of WHY it died once the container is gone ». Ce paquet
// garde le reste.
//
// ── DEUX RANGEMENTS, PARCE QUE DEUX DURÉES DE VIE ──────────────────────
//
//	<racine>/logs/20260905/kernel.log            un SERVICE
//	<racine>/logs/20260905/141932/retro-nes-001.log   une SESSION
//
// Un SERVICE tourne en continu : son journal est celui de la journée, et il
// s'y ajoute. Un redémarrage — un plantage, un `air` qui recompile — continue
// le même fichier, parce que ce qui précède un redémarrage est justement ce
// qu'on cherche quand on enquête dessus.
//
// Une SESSION est une instance : il y en a plusieurs par jour, à des heures
// différentes, et **elles portent le même nom**. Docker réutilise
// `retro-nes-001` d'une partie à l'autre — sans l'heure, deux parties de NES
// se mélangeraient dans un seul fichier, et on ne saurait plus où finit l'une
// et où commence l'autre. L'heure de DÉMARRAGE de la session les sépare.
//
// C'est la correction du 2026-09-05 : le premier jet mettait tout dans un
// dossier par lancement de la pile, ce qui découpait les services sans
// raison et n'aurait pas suffi à séparer deux sessions du même lancement.
package logfile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ServiceHandler — le journal d'un service : `console` reçoit tout ce qu'il
// recevait (le JSON de slog, au niveau Info), et le fichier du jour —
// `logs/20260905/kernel.log` — ce que le réglage laisse passer (Settings) :
// rien kernel éteint, ses niveaux cochés, les requêtes à part.
//
// Le fichier en APPEND : le service peut redémarrer, il continue celui de sa
// journée.
func ServiceHandler(console io.Writer, root, name string) slog.Handler {
	file := &switchedFile{
		root:     root,
		relative: func() string { return filepath.Join(day(), name+".log") },
		on:       func(settings *Settings) bool { return settings.Kernel },
	}

	return serviceHandler{
		console: slog.NewJSONHandler(console, &slog.HandlerOptions{Level: slog.LevelInfo}),
		file:    slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
}

// serviceHandler — deux sorties, chacune son filtre. Le niveau Debug, coché,
// va au fichier sans changer la sortie standard.
type serviceHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (handler serviceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.console.Enabled(ctx, level) || current.Load().mayWrite(level)
}

func (handler serviceHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error
	if handler.console.Enabled(ctx, record.Level) {
		err = handler.console.Handle(ctx, record)
	}

	if current.Load().writes(record.Level, record.Message) {
		_ = handler.file.Handle(ctx, record)
	}

	return err
}

func (handler serviceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return serviceHandler{console: handler.console.WithAttrs(attrs), file: handler.file.WithAttrs(attrs)}
}

func (handler serviceHandler) WithGroup(name string) slog.Handler {
	return serviceHandler{console: handler.console.WithGroup(name), file: handler.file.WithGroup(name)}
}

// ReportsFile — le nom du journal des signalements, cité par les deux kernels
// plutôt que réécrit dans chacun.
const ReportsFile = "reports.log"

// Reports ouvre le journal des SIGNALEMENTS — `logs/reports.log`.
//
// À LA RACINE, et non sous le jour, contrairement aux deux autres. Demande
// explicite de l'utilisateur (#191), et elle se tient : un signalement se lit
// en bloc, dans l'ordre où il est arrivé. Rangés par date, retrouver « ce qui a
// échoué au lancement cette semaine » demanderait d'ouvrir sept fichiers ; et
// un chemin unique, toujours le même, est ce qui rend ce journal citable.
//
// En APPEND, comme un journal de service : une ligne JSON par signalement,
// jamais réécrite. Son interrupteur (Settings.Reports) éteint : ErrReportsOff,
// et rien ne s'ouvre — l'appelant le dit à celui qui signale, au lieu de lui
// répondre « envoyé ».
func Reports(root string) (*os.File, error) {
	if !current.Load().Reports {
		return nil, ErrReportsOff
	}

	return open(root, ReportsFile)
}

// ErrReportsOff — les signalements sont éteints (Settings.Reports).
var ErrReportsOff = errors.New("reports are turned off on this server")

// Session — le journal d'UNE session, sous l'heure de son démarrage —
// `logs/20260905/141932/retro-nes-001.log`.
//
// L'heure est prise MAINTENANT : c'est le démarrage de cette session-là, et
// c'est ce qui la distingue de la précédente qui portait le même nom.
//
// Il suit le réglage (Settings.Sessions) à chaque écriture : éteint, rien ne
// s'écrit et le fichier se ferme ; rallumé, il s'ouvre — créé à ce moment-là
// si la partie a commencé éteinte — et la suite s'y ajoute.
func Session(root, name string) io.WriteCloser {
	relative := filepath.Join(day(), time.Now().Format("150405"), name+".log")

	return &switchedFile{
		root:     root,
		relative: func() string { return relative },
		on:       func(settings *Settings) bool { return settings.Sessions },
	}
}

// switchedFile — un journal qui n'existe sur le disque que quand son
// interrupteur est allumé.
//
// Ouvert à sa première ligne, jamais avant : éteint au démarrage, aucun
// fichier vide n'apparaît. Rouvert quand son chemin change — le jour d'un
// service, à minuit : il écrivait jusqu'ici dans le fichier de la veille
// jusqu'au redémarrage suivant — et quand `generation` avance (un réglage, une
// suppression). Une ligne qui ne s'écrit pas n'est jamais une erreur : un
// journal ne doit pas faire échouer ce qu'il raconte.
type switchedFile struct {
	mutex    sync.Mutex
	root     string
	relative func() string
	on       func(*Settings) bool
	file     *os.File
	path     string
	opened   uint64
}

func (writer *switchedFile) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	if !writer.on(current.Load()) {
		writer.closeLocked()

		return len(data), nil
	}
	relative, seen := writer.relative(), generation.Load()

	if writer.file == nil || relative != writer.path || seen != writer.opened {
		writer.closeLocked()
		file, err := open(writer.root, relative)

		if err != nil || file == nil {
			return len(data), nil
		}
		writer.file, writer.path, writer.opened = file, relative, seen
	}
	_, _ = writer.file.Write(data)

	return len(data), nil
}

func (writer *switchedFile) Close() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	writer.closeLocked()

	return nil
}

func (writer *switchedFile) closeLocked() {
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
}

func day() string {
	return time.Now().Format("20060102")
}

// open crée l'arborescence et ouvre le fichier.
//
// Rend nil, nil quand aucune racine n'est connue : l'appelant continue sans
// fichier, et rien ne casse pour un journal.
func open(root, relative string) (*os.File, error) {
	if root == "" {
		return nil, nil
	}

	full := filepath.Join(root, "logs", relative)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}

	return os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
