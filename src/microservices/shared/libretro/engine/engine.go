// Package engine — LE MOTEUR LIBRETRO DE GAMEFLEX, pour Linux et Android
// (chantier 13) : un cœur, une partie, sa cadence, ses images, son son.
//
// C'EST LE MOTEUR DU SERVEUR, SORTI DE SON RUNTIME. Sa boucle, ses requêtes
// servies entre deux images, son rembobinage, sa sauvegarde de cartouche, sa
// gestion du son et des manettes ont été écrits et corrigés en faisant tourner
// de vraies consoles (stream_session.go, 2026-07 à 2026-10) ; ils sont ici
// pour que le téléphone en hérite au lieu d'en réécrire un autre. Ce qui reste
// au serveur et au téléphone, c'est le TRANSPORT : l'encodeur, le réseau, les
// joueurs, et où ranger les fichiers.
//
// UN SEUL FIL SYSTÈME POUR TOUT. Le chargement du cœur, retro_init,
// retro_load_game, le contexte graphique, chaque retro_run, chaque requête
// (sauvegarde, triche, disque, manette) et le déchargement passent par la même
// goroutine, verrouillée sur son fil (runtime.LockOSThread, jamais
// déverrouillé) : un contexte EGL n'est courant que sur le fil qui l'a rendu
// courant, et un cœur à recompilateur (lightrec) peut casser la pile de
// signaux du sien — Go jette ce fil quand la goroutine se termine, au lieu de
// le prêter à une autre. Le serveur faisait l'init sur un fil et la boucle sur
// un autre ; ce moteur n'a plus cet écart.
//
// UN MOTEUR PAR PROCESSUS. Le pont libretro est global — un cœur chargé à la
// fois, des callbacks globaux — : Load refuse une seconde partie tant que la
// première n'est pas déchargée. Le serveur a un processus par partie ; le
// téléphone n'en joue qu'une à la fois.
package engine

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"gameflex/libretro"
	"gameflex/libretro/gl_context"
)

// LA CADENCE — une échéance absolue, pas un time.Ticker.
//
// Un time.Ticker a un tampon de UN : quand retro_run() déborde, qu'une
// sauvegarde passe, que le ramasse-miettes se réveille, le tick suivant est
// JETÉ et rien ne le rattrape. La boucle tournait alors à 58,9 images par
// seconde au lieu de 59,94, et le son, produit au rythme des images, creusait
// le tampon du navigateur jusqu'à la coupure (PS2, 2026-09-08). On garde donc
// une échéance (`next`) qu'on fait avancer du nombre d'images dues.
//
//   - catchUpMaxFrames borne ce qu'un réveil rattrape, pour que l'arrêt et les
//     requêtes restent servis entre deux rafales ; le reste demeure dans
//     `next`, le minuteur se rarme avec un délai négatif et repasse aussitôt ;
//   - au-delà de catchUpLagCeiling (processus gelé, machine endormie), on
//     resynchronise et on compte les images abandonnées, plutôt que de faire
//     défiler le jeu en accéléré pour un temps qui n'a jamais été joué.
const (
	catchUpMaxFrames  = 4
	catchUpLagCeiling = 250 * time.Millisecond
)

// fastForwardMultiplier — combien de retro_run() par réveil pendant l'avance
// rapide. Seule la DERNIÈRE image est rendue (le flux garde sa cadence), et le
// son se tait : l'envoyer pour de vrai demanderait de le transposer, ou
// recréerait l'arriéré que le rejet des lots de son évite.
const fastForwardMultiplier = 4

// sramFlushInterval — la sauvegarde de cartouche part vers l'appelant toutes
// les trente secondes, et à l'arrêt, si elle a changé. Comme RetroArch.
//
// N'écrire qu'à l'arrêt serait plus simple et perdrait tout sur un plantage —
// et un cœur qui plante, on en a vu deux dans la même journée (pcsx2 sur une
// cinématique, dolphin sur l'enregistrement des ports). Trente secondes est
// le compromis habituel : assez rare pour ne rien coûter, assez fréquent pour
// que ce qu'on perd tienne dans ce qu'on accepte de rejouer.
const sramFlushInterval = 30 * time.Second

// emulationStall — un cœur sans image depuis aussi longtemps est déclaré pendu
// (Config.OnStall). Constaté sur une PS2 qui réouvrait son moteur graphique et
// ne revenait jamais : une image figée des minutes durant, sans un mot.
const emulationStall = 15 * time.Second

// Les identifiants de manette que le moteur pose lui-même (libretro.h).
const (
	DeviceNone   = 0
	DeviceJoypad = 1
	// DevicePSXDualShock — RETRO_DEVICE_SUBCLASS(RETRO_DEVICE_ANALOG, 1) de
	// pcsx_rearmed : (2 << 8) | 5. Une manette ANALOG nue tombe dans son
	// `default: PSE_PAD_TYPE_NONE` — aucune manette du tout.
	DevicePSXDualShock = 517
)

// ErrBusy — une partie occupe déjà le pont de ce processus.
var ErrBusy = errors.New("engine: une partie tourne déjà dans ce processus")

// ErrStopped — la partie est finie : la requête n'a plus personne à qui parler.
var ErrStopped = errors.New("engine: la partie est arrêtée")

// LoadError dit à quelle étape le démarrage a échoué — "core", "prepare",
// "game" ou "graphics" —, pour que l'appelant le dise à sa façon.
type LoadError struct {
	Stage string
	Err   error
}

func (e *LoadError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *LoadError) Unwrap() error { return e.Err }

// Game — ce qu'on donne au cœur, et où il range ce qu'il écrit lui-même.
// Prérempli depuis Config ; Config.Prepare peut le modifier une fois le cœur
// chargé (le serveur y extrait une archive, refait une fiche .cue, choisit un
// BIOS).
type Game struct {
	Path      string
	Data      []byte // nil : le cœur lira Path lui-même (need_fullpath)
	SystemDir string // GET_SYSTEM_DIRECTORY : les BIOS
	SaveDir   string // GET_SAVE_DIRECTORY : ce que le cœur écrit de lui-même
	// Variables — des réponses imposées à GET_VARIABLE, par l'appelant (le
	// BIOS que pcsx2 doit prendre). Elles passent avant tout le reste.
	Variables map[string]string
}

// Config — ce qu'il faut pour charger une partie.
type Config struct {
	CorePath  string // le .so du cœur
	Core      string // son nom court (« pcsx_rearmed ») : voir cores.go
	ConsoleID string // l'identifiant GameFlex : rendu matériel, manettes, options
	Ports     int    // manettes branchées ; 1 si rien n'est dit
	Game      Game

	// Prepare est appelé une fois le cœur chargé — c'est seulement alors
	// qu'on sait s'il veut un chemin ou un tampon (need_fullpath). Sans lui,
	// le moteur lit Game.Path quand le cœur veut un tampon.
	Prepare func(system libretro.SystemInfo, game *Game) error

	// SaveRAM — la sauvegarde de cartouche à remettre en place après
	// retro_load_game, lue par l'appelant là où il la range.
	SaveRAM []byte
	// OnSaveRAM reçoit la sauvegarde de cartouche quand elle a changé — toutes
	// les trente secondes, et à l'arrêt. L'appelant l'écrit où il veut.
	OnSaveRAM func(data []byte, reason string)

	// StreamFPS — la cadence du flux choisie par le joueur, dans le panneau
	// des réglages de flux (2026-10-11). Plus basse que celle du cœur, le
	// moteur ne rend qu'une partie de ses images (voir OutputFPS) ; 0 les rend
	// toutes.
	StreamFPS float64

	// StreamHeight — la hauteur du flux choisie par le joueur (360, 540, 720,
	// 1080). Un cœur qui a une résolution interne dessine au plus près
	// (internalResolutions, #340) ; 0 lui laisse son défaut.
	StreamHeight int

	// Language — la langue de la page qui a lancé la partie, « fr » ou « en »
	// (chantier 21) : le moteur la répond à GET_LANGUAGE, et la pose dans
	// l'option de langue des cœurs qui ne lisent pas GET_LANGUAGE
	// (languageVariables). Vide : l'anglais, comme avant.
	Language string

	// Les rappels suivants tournent tous sur le fil du moteur : à garder
	// courts, la cadence attend leur retour.

	OnFrame         func(Frame)           // une image neuve, en BGRA
	OnAudio         func(samples []int16) // ~10 ms de son stéréo entrelacé
	OnRumble        func(port int, weak, strong uint16)
	OnRewindSupport func(supported bool)        // tranché à la première capture
	OnProgress      func(step, detail string)   // "game.loaded", "emulator.ready"
	OnStall         func(stalled time.Duration) // le cœur ne rend plus d'image (rendu matériel)
}

// Frame — une image de la partie : BGRA compact, rangées de haut en bas.
type Frame struct {
	Pixels        []byte
	Width, Height int
	// Aspect — le format d'affichage que le cœur annonce (largeur/hauteur),
	// 0 s'il n'en dit rien : FitBGRA y met l'image, bandes noires autour.
	Aspect float64
}

// Cheat — un code, actif ou non. L'index d'un code est sa place dans la liste.
type Cheat struct {
	Code    string
	Enabled bool
}

// DiskState — les disques d'un jeu à plusieurs images.
type DiskState struct {
	Count   int
	Current int
	OK      bool // le changement demandé a été accepté (vrai pour une simple lecture)
}

// Engine — une partie chargée.
type Engine struct {
	config Config
	inputs inputs

	// Fixés au chargement, lus seulement ensuite.
	system    libretro.SystemInfo
	info      libretro.SystemAVInfo
	fps       float64
	frameTime time.Duration

	// Le cœur est-il chargé ? Lu par unloadAfter, qui le décharge une fois le
	// fil du moteur terminé.
	coreLoaded bool

	// Le fil du moteur seul — les callbacks du cœur y tournent tous.
	pixelFormat int
	hw          *hwRequest
	glContext   *gl_context.Context
	// emitFrame — l'image de ce retro_run part-elle ? Faux pour les images
	// sautées de l'avance rapide, ET avant la boucle : un cœur dessine
	// parfois pendant son chargement (dolphin dès context_reset), quand
	// l'appelant n'a pas encore pu préparer son encodeur — ffmpeg reçut une
	// cadence de 0 et la GameCube n'eut jamais d'image (2026-10-11).
	emitFrame bool
	// frameCredit — le crédit d'images de la cadence du flux : chaque image du
	// cœur en apporte OutputFPS/fps, une image part dès qu'il atteint 1.
	frameCredit float64
	looping     bool // la boucle tourne : le son peut partir
	// signalDriftSeen — l'écart de la pile de signaux du fil, pendant la partie,
	// est dit une fois (voir reportSignalStack).
	signalDriftSeen bool
	// threadsAtStart — les noms des fils avant le cœur (voir bornSince).
	threadsAtStart map[string]int
	frameSeen      bool
	rejectSeen     bool
	readFailed     bool
	frames         int
	unknownSeen    map[uint32]bool
	defaults       map[string]string
	resolutions    map[string]string // les valeurs choisies par chooseResolution
	sramLast       []byte
	audioBuf       []int16
	audioBufMax    int
	comboFrames    int
	devicesShown   map[uint32]bool
	rewind         rewindState

	// Partagés.
	mu          sync.Mutex
	paused      bool
	fastForward bool
	rewinding   bool
	latestFPS   float64
	rumble      [MaxPorts][2]uint16 // [port][0] faible, [port][1] fort
	alive       atomic.Int64
	// aspect — Frame.Aspect, en bits de float64 : SET_GEOMETRY peut venir d'un
	// autre fil du cœur que celui des images.
	aspect atomic.Uint64
	// aspectRejected — un format absurde est dit une fois par partie.
	aspectRejected atomic.Bool

	commands chan func()
	start    chan struct{}
	stop     chan struct{}
	done     chan struct{}
	runOnce  sync.Once
	stopOnce sync.Once
}

type hwRequest struct {
	contextType  int
	major, minor uint32
}

var running atomic.Bool

// Load charge le cœur et le jeu sur le fil du moteur, et rend la main la
// partie prête, l'émulation pas encore lancée : l'appelant prépare ce qui doit
// exister avant la première image (un encodeur à la cadence du cœur), puis
// appelle Run. Sur un échec, tout est déjà déchargé.
func Load(config Config) (*Engine, error) {
	if !running.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	if config.Ports < 1 {
		config.Ports = 1
	}
	if config.Ports > MaxPorts {
		config.Ports = MaxPorts
	}

	e := &Engine{
		config:   config,
		commands: make(chan func()),
		start:    make(chan struct{}),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	booted := make(chan error, 1)
	go e.run(booted)
	if err := <-booted; err != nil {
		<-e.done

		return nil, err
	}

	return e, nil
}

// Run lance la boucle d'émulation. Sans effet la seconde fois.
func (e *Engine) Run() {
	e.runOnce.Do(func() { close(e.start) })
}

// Stop arrête la partie — la sauvegarde de cartouche part une dernière fois —
// et rend la main le cœur déchargé. Valable aussi avant Run.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() { close(e.stop) })
	<-e.done
}

// Done se ferme quand la partie est finie, cœur déchargé.
func (e *Engine) Done() <-chan struct{} {
	return e.done
}

// Info — la géométrie et la cadence que le cœur a annoncées au chargement.
func (e *Engine) Info() libretro.SystemAVInfo {
	return e.info
}

// System — le nom et la version du cœur.
func (e *Engine) System() libretro.SystemInfo {
	return e.system
}

// CoreName — le cœur comme il se déclare (`retro_get_system_info`) : son nom,
// puis sa version s'il en donne une. Pour le panneau Performance.
func (e *Engine) CoreName() string {
	system := e.System()
	if system.LibraryVersion == "" {
		return system.LibraryName
	}

	return system.LibraryName + " " + system.LibraryVersion
}

// streamFPSSlack — l'écart en deçà duquel une cadence choisie ne change rien :
// « 60 » pour une NES qui en rend 60,1, ou pour une console PAL à 50, laisse
// passer toutes ses images au lieu d'en jeter une sur six cents.
const streamFPSSlack = 2

// OutputFPS — la cadence des images que le moteur rend : celle du cœur, ou la
// cadence du flux choisie (Config.StreamFPS) quand elle est franchement plus
// basse. C'est elle que l'appelant donne à son encodeur. Fixée au chargement.
func (e *Engine) OutputFPS() float64 {
	if want := e.config.StreamFPS; want > 0 && want < e.fps-streamFPSSlack {
		return want
	}

	return e.fps
}

// paceFrame — cette image part-elle ? `ratio` est la part des images du cœur
// qui partent (OutputFPS/fps, 1 pour toutes). Un crédit plutôt qu'un modulo :
// 50 images sur 60 en donne cinq sur six, régulièrement espacées.
func paceFrame(credit *float64, ratio float64) bool {
	if ratio >= 1 {
		return true
	}
	*credit += ratio
	if *credit < 1 {
		return false
	}
	*credit--

	return true
}

// FPS — la cadence mesurée sur les cinq dernières secondes, 0 avant.
func (e *Engine) FPS() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.latestFPS
}

// CheatsSupported — le cœur sait-il appliquer des codes ?
func (e *Engine) CheatsSupported() bool {
	return libretro.CheatSupported()
}

// Input applique un lot du canal de données de la page à la manette `port`.
func (e *Engine) Input(port int, batch []byte) error {
	return e.inputs.apply(port, batch)
}

// ClearInput relâche tout sur une manette — un joueur qui part ne doit pas
// laisser une touche enfoncée derrière lui.
func (e *Engine) ClearInput(port int) {
	e.inputs.clear(port)
}

// SetPaused gèle la partie, ou la reprend. Le cœur reste chargé : la reprise
// est instantanée, et le temps passé en pause n'est pas rattrapé.
func (e *Engine) SetPaused(paused bool) {
	e.mu.Lock()
	e.paused = paused
	e.mu.Unlock()
}

// SetFastForward lance ou arrête l'avance rapide, qui arrête le rembobinage.
func (e *Engine) SetFastForward(enabled bool) {
	e.mu.Lock()
	e.fastForward = enabled
	if enabled {
		e.rewinding = false
	}
	e.mu.Unlock()
}

// SetRewind lance ou arrête le rembobinage, et rend ce qui est appliqué : une
// partie à qui il n'est pas accordé (voir rewind.go) ne rembobine pas.
func (e *Engine) SetRewind(enabled bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if enabled && !e.rewind.supported {
		return false
	}
	e.rewinding = enabled
	if enabled {
		e.fastForward = false
	}

	return enabled
}

// RewindSupported — le rembobinage est-il accordé à cette partie ? Faux tant
// que la première capture n'a pas tranché.
func (e *Engine) RewindSupported() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.rewind.supported
}

// RewindDecided — la première capture a-t-elle tranché ? Avant, RewindSupported
// vaut faux par prudence, pas par verdict : un appelant qui doit le dire une
// seule fois (le message "started" du téléphone) peut attendre ce moment.
func (e *Engine) RewindDecided() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.rewind.decided
}

// SaveState sérialise la partie, entre deux images.
func (e *Engine) SaveState() ([]byte, error) {
	var data []byte
	var err error
	if callErr := e.do(func() { data, err = libretro.Serialize() }); callErr != nil {
		return nil, callErr
	}

	return data, err
}

// LoadState recharge une partie sérialisée, entre deux images. Un refus du
// cœur revient en *StateRefusedError : qui refuse, et qui avait écrit (#396).
func (e *Engine) LoadState(data []byte) error {
	var err error
	if callErr := e.do(func() { err = libretro.Unserialize(data) }); callErr != nil {
		return callErr
	}
	if errors.Is(err, libretro.ErrUnserializeFailed) {
		return stateRefused(coreIdentity{name: e.system.LibraryName, version: e.system.LibraryVersion}, data)
	}

	return err
}

// SetCheats remplace la liste des codes : remise à zéro, puis TOUTE la liste,
// chacun à son index — y compris les inactifs, plutôt que de supposer que
// chaque cœur traite un index absent comme un code éteint.
func (e *Engine) SetCheats(cheats []Cheat) error {
	return e.do(func() {
		libretro.CheatReset()
		for index, cheat := range cheats {
			libretro.CheatSet(uint(index), cheat.Enabled, cheat.Code)
		}
	})
}

// SetControllerType change la manette d'un port en cours de partie.
func (e *Engine) SetControllerType(port int, device uint32) error {
	return e.do(func() {
		libretro.SetControllerPortDevice(uint32(port), device)
		log.Printf("[engine] manette du port %d : périphérique %d", port, device)
		// LA DUALSHOCK EN MODE ANALOGIQUE : pcsx_rearmed la monte en
		// numérique, et seule sa combinaison L1+R1+SELECT, tenue une
		// demi-seconde, la bascule (voir inputs.forceCombo). Sans elle, les
		// sticks de la DualShock choisie au menu ne répondraient pas.
		if e.config.ConsoleID == "ps1" && port == 0 && device == DevicePSXDualShock {
			e.comboFrames = 30
		} else if port == 0 && e.comboFrames > 1 {
			// Repartie avant la fin de la combinaison : elle se relâche à
			// l'image suivante (c'est la dernière qui libère les boutons),
			// plutôt que de rester tenue sur la manette numérique.
			e.comboFrames = 1
		}
	})
}

// Disks lit les disques du jeu — et en monte un autre si index ≥ 0. Le
// compte et le disque courant sont TOUJOURS relus après coup, y compris quand
// le changement a échoué : c'est l'état réel du cœur qui remonte au menu.
func (e *Engine) Disks(index int) (DiskState, error) {
	state := DiskState{Current: -1, OK: index < 0}
	err := e.do(func() {
		if index >= 0 {
			state.OK = libretro.DiskSelect(index)
			log.Printf("[engine] disque %d demandé : %v", index, state.OK)
		}
		state.Count = libretro.DiskCount()
		state.Current = libretro.DiskIndex()
	})

	return state, err
}

// Reset remet la console à zéro, comme son bouton.
func (e *Engine) Reset() error {
	return e.do(func() {
		libretro.Reset()
		e.reportSignalStack("remise à zéro")
	})
}

// do confie un appel au fil du moteur et attend qu'il soit fait. Les délais
// sont ceux du serveur : deux secondes pour qu'une boucle le prenne, cinq pour
// qu'elle le finisse — un appelant bloqué pour toujours est pire qu'une
// erreur claire.
func (e *Engine) do(call func()) error {
	finished := make(chan struct{})
	select {
	case e.commands <- func() { call(); close(finished) }:
	case <-e.done:
		return ErrStopped
	case <-time.After(2 * time.Second):
		return errors.New("engine: la boucle d'émulation ne prend pas la requête")
	}
	select {
	case <-finished:
		return nil
	case <-e.done:
		return ErrStopped
	case <-time.After(5 * time.Second):
		return errors.New("engine: la boucle d'émulation n'a pas fini la requête")
	}
}

func (e *Engine) run(booted chan<- error) {
	// Un fil à lui, verrouillé, qui se terminera avec la partie — jamais le
	// fil principal du runtime, que Go ne termine pas (voir onOwnThread).
	onOwnThread(func() { e.runLocked(booted) })
}

func (e *Engine) runLocked(booted chan<- error) {
	// La pile de signaux de CE fil, avant que le cœur ne la touche : ses
	// arrêts la remettent s'il l'a changée (#356, voir le pont). Et les fils
	// du processus avant lui, pour dire ceux qu'il laisserait derrière lui.
	libretro.NoteSignalStack()
	e.threadsAtStart = threadCensus()
	thread := gettid()
	// Le cœur ne se décharge pas ici : voir unloadAfter.
	defer func() { go e.unloadAfter(thread) }()
	// Le dernier geste du fil, après son dernier appel au cœur : que ses
	// destructeurs tournent signaux synchrones ouverts quand Go le terminera
	// (#356, voir le pont).
	defer func() {
		if !libretro.UnblockFaultsAtThreadExit() {
			log.Printf("[engine] la fin du fil du moteur ne rouvrira pas ses signaux : la libc a refusé")
		}
	}()

	if err := e.boot(); err != nil {
		booted <- err

		return
	}
	booted <- nil

	select {
	case <-e.start:
		e.loop()
	case <-e.stop:
	}
	e.shutdown()
}

func (e *Engine) boot() error {
	if err := libretro.LoadCore(e.config.CorePath); err != nil {
		return &LoadError{Stage: "core", Err: err}
	}
	e.coreLoaded = true
	e.system = libretro.GetSystemInfo()

	game := e.config.Game
	if e.config.Prepare != nil {
		if err := e.config.Prepare(e.system, &game); err != nil {
			return &LoadError{Stage: "prepare", Err: err}
		}
	} else if !e.system.NeedFullpath && game.Data == nil {
		data, err := os.ReadFile(game.Path)
		if err != nil {
			return &LoadError{Stage: "prepare", Err: err}
		}
		game.Data = data
	}
	e.config.Game = game
	log.Printf("[engine] cœur %s %s, console %s, jeu %s (%d octets en mémoire, need_fullpath=%v)",
		e.system.LibraryName, e.system.LibraryVersion, e.config.ConsoleID,
		filepath.Base(game.Path), len(game.Data), e.system.NeedFullpath)

	// Les chemins AVANT les callbacks : un cœur peut interroger son
	// environnement dès retro_set_environment.
	libretro.InitEnvPaths(game.SystemDir, game.SaveDir)
	libretro.SetEnvironmentCallback(e.environment)
	libretro.SetVideoRefreshCallback(e.video)
	libretro.SetAudioSampleCallback(e.audioSample)
	libretro.SetAudioSampleBatchCallback(e.audioBatch)
	libretro.SetInputPollCallback(func() {})
	libretro.SetInputStateCallback(e.inputState)
	libretro.SetRumbleStateCallback(e.rumbleState)
	libretro.RegisterCallbacks()

	// LES MANETTES AVANT retro_init — appelé après, input_state n'est jamais
	// interrogé par certains cœurs (bug #3 du serveur). Sauf pour ceux qui en
	// meurent (LatePortRegistration) : à eux, après retro_load_game.
	late := LatePortRegistration(e.config.Core)
	if !late {
		e.registerPorts(DeviceJoypad)
	}
	libretro.Init()

	// Le jeu ne se décharge QUE s'il s'est chargé : pcsx2 s'abat dans son
	// retro_unload_game quand retro_load_game a échoué.
	if err := libretro.LoadGame(libretro.GameInfo{Path: game.Path, Data: game.Data}); err != nil {
		libretro.Deinit()
		e.reportSignalStack("arrêt du cœur (jeu refusé)")

		return &LoadError{Stage: "game", Err: err}
	}
	e.progress("game.loaded", filepath.Base(game.Path))

	// La sauvegarde de cartouche, remise en place ICI : son tampon n'existe
	// pas avant retro_load_game, et il doit être plein avant la première
	// image, sinon le jeu lit une pile vierge et se croit neuf.
	e.restoreSaveRAM()

	// DOLPHIN A BESOIN DE SES PROPRES FICHIERS, ET IL LE DIT TROP BAS : une
	// Wii sans le dossier `Sys` tourne à la bonne cadence et n'envoie que des
	// images dupliquées. On le dit à notre hauteur.
	if e.config.Core == "dolphin" {
		sysDir := filepath.Join(game.SystemDir, "dolphin-emu", "Sys")
		if _, err := os.Stat(filepath.Join(sysDir, "Wii")); err != nil {
			log.Printf("[engine] dolphin : %s absent — une GameCube démarrera quand même, une Wii n'affichera rien (déposez le dossier Sys de Dolphin dans bios/dolphin-emu/)",
				filepath.Join(sysDir, "Wii"))
		} else {
			log.Printf("[engine] dolphin : dossier Sys trouvé (%s)", sysDir)
		}
	}

	if late {
		e.registerPorts(DeviceJoypad)
		log.Printf("[engine] %d port(s) enregistré(s) après le chargement (%s ne supporte pas l'appel avant retro_init)",
			e.config.Ports, e.config.Core)
	}

	// LA PS1 DÉMARRE AVEC LA MANETTE D'ORIGINE, numérique (décidé par le
	// porteur le 2026-10-11) : avec la DualShock, X ne passait pas l'écran
	// titre de Crash Team Racing. Elle démarrait en DualShock depuis
	// août 2026, pour les jeux qui veulent un stick (Ace Combat 3) ; c'est
	// maintenant le menu qui la donne (SetControllerType), « au risque que
	// certains jeux ne fonctionnent pas » d'emblée.

	e.info = libretro.GetSystemAVInfo()
	e.setAspect(e.info.Geometry, "géométrie de départ")
	e.fps = e.info.Timing.FPS
	if e.fps <= 0 || e.fps > 240 {
		e.fps = 60
	}
	if e.info.Timing.SampleRate <= 0 {
		e.info.Timing.SampleRate = 44100
	}
	e.info.Timing.FPS = e.fps
	e.frameTime = time.Duration(float64(time.Second) / e.fps)
	// La première image part tout de suite : le crédit commence là où il la
	// laisse passer.
	e.frameCredit = 1 - e.OutputFPS()/e.fps
	if e.OutputFPS() < e.fps {
		log.Printf("[engine] cadence du flux : %.0f images par seconde sur les %.2f du cœur", e.OutputFPS(), e.fps)
	}
	// Dix millisecondes de son stéréo par lot — la durée d'une trame Opus du
	// serveur : attendre un lot plus gros, c'est autant de latence en plus.
	e.audioBufMax = int(e.info.Timing.SampleRate*0.010) * 2

	log.Printf("[engine] %dx%d (max %dx%d) à %.2f images par seconde, son %.0f Hz, rendu %s",
		e.info.Geometry.BaseWidth, e.info.Geometry.BaseHeight,
		e.info.Geometry.MaxWidth, e.info.Geometry.MaxHeight,
		e.fps, e.info.Timing.SampleRate, map[bool]string{true: "matériel", false: "logiciel"}[e.hw != nil])
	e.progress("emulator.ready", fmt.Sprintf("%dx%d @ %.1f",
		e.info.Geometry.BaseWidth, e.info.Geometry.BaseHeight, e.fps))

	if e.hw != nil {
		if err := e.openGL(); err != nil {
			libretro.UnloadGame()
			libretro.Deinit()
			e.reportSignalStack("arrêt du cœur (contexte graphique refusé)")

			return &LoadError{Stage: "graphics", Err: err}
		}
	}

	return nil
}

func (e *Engine) registerPorts(device uint32) {
	for port := 0; port < e.config.Ports; port++ {
		libretro.SetControllerPortDevice(uint32(port), device)
	}
}

// openGL ouvre le contexte que le cœur a demandé pendant retro_load_game, à
// la taille MAXIMALE qu'il annonce — une PS2 change de résolution en cours de
// partie, le FBO ne doit jamais avoir à être refait —, puis lui dit qu'il
// peut s'en servir (context_reset). Et lance le chien de garde : un cœur en
// rendu matériel peut se verrouiller dans son pilote.
func (e *Engine) openGL() error {
	width, height := int(e.info.Geometry.MaxWidth), int(e.info.Geometry.MaxHeight)
	if width == 0 || height == 0 {
		// PPSSPP annonce 0x0 comme maximum : le serveur s'y est pendu.
		width, height = int(e.info.Geometry.BaseWidth), int(e.info.Geometry.BaseHeight)
	}
	if width == 0 || height == 0 {
		return errors.New("rendu matériel demandé sans taille d'image")
	}

	ctx, err := gl_context.New(e.hw.contextType, int(e.hw.major), int(e.hw.minor))
	if err != nil {
		return err
	}
	if err := ctx.Resize(width, height); err != nil {
		ctx.Destroy()

		return err
	}
	e.glContext = ctx
	libretro.SetActiveGLContext(ctx)
	libretro.NotifyHWContextReset()
	log.Printf("[engine] contexte graphique ouvert : type %d %d.%d, %dx%d",
		e.hw.contextType, e.hw.major, e.hw.minor, width, height)

	if e.config.OnStall != nil {
		go e.watchAlive()
	}

	return nil
}

func (e *Engine) loop() {
	log.Printf("[engine] boucle d'émulation lancée, %.2f images par seconde visées", e.fps)
	e.looping = true

	timer := time.NewTimer(e.frameTime)
	defer timer.Stop()
	next := time.Now().Add(e.frameTime)

	frameCount := 0
	abandoned := 0
	lastReport := time.Now()
	lastSRAMFlush := time.Now()
	// UNE REQUÊTE N'EST PAS DU RETARD. Sérialiser 44 Mio prend des centaines
	// de millisecondes pendant lesquelles l'émulation était à l'arrêt exprès :
	// les rattraper ferait défiler le jeu à quadruple vitesse juste après
	// chaque sauvegarde. On resynchronise au tour suivant.
	stalled := false

	for {
		select {
		case <-e.stop:
			log.Printf("[engine] boucle d'émulation arrêtée")

			return
		case command := <-e.commands:
			stalled = true
			command()
		case <-timer.C:
			now := time.Now()
			e.mu.Lock()
			paused := e.paused
			rewinding := e.rewinding
			fastForward := e.fastForward
			e.mu.Unlock()

			if stalled {
				stalled = false
				next = now
			}
			owed := 1
			lag := now.Sub(next)
			if lag > 0 {
				owed += int(lag / e.frameTime)
			}
			if lag > catchUpLagCeiling {
				abandoned += owed - 1
				owed = 1
				next = now
			}
			// Pour la pause, le rembobinage et l'avance rapide, le temps
			// écoulé n'est PAS du retard : plus aucune dette.
			resync := func() {
				next = now.Add(e.frameTime)
			}

			switch {
			case paused:
				resync()
			case rewinding:
				e.stepRewind()
				resync()
			default:
				runs := owed
				// Pendant un rattrapage, CHAQUE image est rendue — sauter
				// des images ferait décrocher l'image du son. Seule l'avance
				// rapide saute les siennes.
				emitAll := true
				if fastForward {
					runs = fastForwardMultiplier
					emitAll = false
					resync()
				} else {
					if runs > catchUpMaxFrames {
						runs = catchUpMaxFrames
					}
					next = next.Add(time.Duration(runs) * e.frameTime)
				}
				for i := 0; i < runs; i++ {
					e.emitFrame = emitAll || i == runs-1
					if e.comboFrames > 0 {
						e.comboFrames--
						e.inputs.forceCombo(e.comboFrames > 0)
					}
					libretro.Run()
				}
				frameCount += runs
				e.alive.Store(time.Now().UnixNano())
				e.captureRewind()
			}

			if elapsed := time.Since(lastReport); elapsed >= 5*time.Second {
				fps := float64(frameCount) / elapsed.Seconds()
				if abandoned > 0 {
					log.Printf("[engine] émulation : %d images, ~%.1f par seconde (%d abandonnée(s) — retard > %s)",
						frameCount, fps, abandoned, catchUpLagCeiling)
					abandoned = 0
				} else {
					log.Printf("[engine] émulation : %d images, ~%.1f par seconde, %d rendues", frameCount, fps, e.frames)
				}
				e.mu.Lock()
				e.latestFPS = fps
				e.mu.Unlock()
				e.watchSignalStack()
				lastReport = time.Now()
				frameCount = 0
				e.frames = 0
			}

			if time.Since(lastSRAMFlush) >= sramFlushInterval {
				lastSRAMFlush = time.Now()
				e.flushSaveRAM("périodique")
			}

			// Rarmer POUR L'ÉCHÉANCE : un délai négatif (dette restante)
			// redéclenche aussitôt, c'est voulu.
			timer.Reset(time.Until(next))
		}
	}
}

// shutdown — sur le fil du moteur : la dernière sauvegarde de cartouche tant
// que le cœur est là, puis l'ordre de RetroArch et de LibretroDroid —
// context_destroy (le cœur y libère ses ressources), la partie, le cœur — et
// SEULEMENT ENSUITE notre contexte graphique. Le DÉCHARGEMENT du cœur
// (dlclose) n'est plus ici : il attend que ce fil soit fini (unloadAfter).
//
// Le contexte était détruit juste après context_destroy, avant
// retro_unload_game et retro_deinit : un cœur qui touche encore au GL en
// sortant le faisait sans contexte courant. Ce n'est pas l'ordre que les cœurs
// connaissent — LibretroDroid (destroy : context_destroy, unload_game, deinit,
// puis dlclose, le contexte toujours vivant) et RetroArch gardent le contexte
// jusqu'au bout. Aligné le 2026-10-11, après la mort de l'application du S25
// en quittant une partie PSP — qui n'a pas suffi : voir unloadAfter.
func (e *Engine) shutdown() {
	e.flushSaveRAM("fin de partie")
	if e.glContext != nil {
		libretro.NotifyHWContextDestroy()
	}
	libretro.UnloadGame()
	libretro.Deinit()
	e.reportSignalStack("arrêt du cœur")
	if e.glContext != nil {
		e.glContext.Destroy()
		e.glContext = nil
	}
	log.Printf("[engine] partie arrêtée")
}

// setAspect — le format d'affichage d'une géométrie, selon la règle de
// libretro : aspect_ratio, ou à défaut base_width / base_height. Une géométrie
// muette ou absurde ne remplace pas le format connu : Dolphin annonce un format
// qui n'est pas un nombre tant que son rendu n'a pas de taille, et l'image de la
// Wii sortait entièrement noire (2026-10-15). Chaque format retenu est dit au
// journal quand il change.
func (e *Engine) setAspect(geometry libretro.GameGeometry, source string) {
	aspect := float64(geometry.AspectRatio)
	if !PlausibleAspect(aspect) && geometry.BaseWidth > 0 && geometry.BaseHeight > 0 {
		aspect = float64(geometry.BaseWidth) / float64(geometry.BaseHeight)
	}
	if !PlausibleAspect(aspect) {
		if e.aspectRejected.CompareAndSwap(false, true) {
			log.Printf("[engine] format d'affichage ignoré, une fois dit (%s : %dx%d, aspect_ratio %g)",
				source, geometry.BaseWidth, geometry.BaseHeight, geometry.AspectRatio)
		}

		return
	}
	previous := math.Float64frombits(e.aspect.Swap(math.Float64bits(aspect)))
	if math.Abs(previous-aspect) > 0.001 {
		log.Printf("[engine] format d'affichage %.3f (%s : %dx%d, aspect_ratio %g)",
			aspect, source, geometry.BaseWidth, geometry.BaseHeight, geometry.AspectRatio)
	}
}

// reportSignalStack — un arrêt du cœur a-t-il dû rendre au fil du moteur sa
// pile de signaux ? Dit au journal : c'était la mort du téléphone en quittant
// une partie PSP (#356) — PPSSPP laissait au fil la pile d'un fil terminé, et
// aucun constat d'Android n'en gardait la trace.
func (e *Engine) reportSignalStack(when string) {
	if replaced := libretro.SignalStackReplaced(); replaced != "" {
		log.Printf("[engine] %s : le cœur laissait au fil du moteur la pile de signaux %s", when, replaced)
	}
}

// watchSignalStack — pendant la partie, le cœur a-t-il changé la pile de
// signaux du fil ? Mesuré seulement (un cœur peut l'avoir posée pour son propre
// gestionnaire) et dit une fois : l'arrêt la remettra.
func (e *Engine) watchSignalStack() {
	if e.signalDriftSeen {
		return
	}
	if drift := libretro.SignalStackDrift(); drift != "" {
		e.signalDriftSeen = true
		log.Printf("[engine] pendant la partie, le cœur a changé la pile de signaux du fil du moteur : %s (l'arrêt remettra la sienne)", drift)
	}
}

func (e *Engine) progress(step, detail string) {
	if e.config.OnProgress != nil {
		e.config.OnProgress(step, detail)
	}
}

// restoreSaveRAM remet en place la sauvegarde de cartouche que l'appelant a
// lue — après retro_load_game, avant la première image.
func (e *Engine) restoreSaveRAM() {
	size := libretro.SaveRAMSize()
	if size == 0 {
		return
	}
	if len(e.config.SaveRAM) == 0 {
		log.Printf("[engine] sram : %s annoncés par le cœur, aucune sauvegarde encore", humanBytes(size))

		return
	}
	if err := libretro.RestoreSaveRAM(e.config.SaveRAM); err != nil {
		log.Printf("[engine] sram : restauration impossible : %v", err)

		return
	}
	e.sramLast = e.config.SaveRAM
	log.Printf("[engine] sram : %s remis en place", humanBytes(len(e.config.SaveRAM)))
}

// flushSaveRAM passe la sauvegarde de cartouche à l'appelant si elle a changé
// depuis la dernière fois — et seulement depuis le fil du moteur : c'est de la
// mémoire que le cœur fait vivre.
func (e *Engine) flushSaveRAM(reason string) {
	if e.config.OnSaveRAM == nil {
		return
	}
	current := libretro.SaveRAM()
	if len(current) == 0 || bytes.Equal(current, e.sramLast) {
		return
	}
	e.sramLast = current
	e.config.OnSaveRAM(current, reason)
}

// watchAlive — LE CŒUR PEUT SE VERROUILLER, ET IL FAUT LE DIRE. Constaté sur
// une PS2 qui réouvrait son moteur graphique et ne revenait jamais : tous les
// fils en attente, l'image figée des minutes durant. On ne peut pas empêcher
// ce verrou ; on peut refuser de faire semblant (Config.OnStall).
func (e *Engine) watchAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-e.stop:
			return
		case <-ticker.C:
			last := e.alive.Load()
			if last == 0 {
				continue
			}
			if stalled := time.Since(time.Unix(0, last)); stalled > emulationStall {
				log.Printf("[engine] LE CŒUR NE RÉPOND PLUS : aucune image depuis %s", stalled.Round(time.Second))
				e.config.OnStall(stalled)

				return
			}
		}
	}
}

func (e *Engine) video(data unsafe.Pointer, width, height, pitch uint32) {
	// LA PREMIÈRE IMAGE SE DIT, ET LES REFUS AUSSI : « pas d'image »
	// ressemble à tout — un cœur bloqué, un format refusé, un encodeur jamais
	// démarré —, et ces trois-là se distinguent d'une ligne.
	if !e.frameSeen {
		e.frameSeen = true
		log.Printf("[engine] première image du cœur : %dx%d pitch=%d données=%v matériel=%v",
			width, height, pitch, data != nil, libretro.IsHWFrameBufferValid(data))
	}
	if data == nil || width == 0 || height == 0 {
		// Une image répétée (GET_CAN_DUPE) : rien de neuf à montrer.
		if !e.rejectSeen && data != nil {
			e.rejectSeen = true
			log.Printf("[engine] image refusée : %dx%d — le cœur appelle mais ne rend rien d'utilisable", width, height)
		}

		return
	}
	if !e.emitFrame {
		// Image sautée de l'avance rapide — avant la conversion, qui est le
		// coût d'une image.
		return
	}
	// LA CADENCE DU FLUX CHOISIE PAR LE JOUEUR (2026-10-11), au même endroit et
	// pour la même raison : 30 images par seconde pour une console qui en rend
	// 60, et l'image qui ne part pas ne coûte pas sa conversion. Le cœur, lui,
	// tourne toujours à sa cadence — le jeu ne ralentit pas, et la manette est
	// lue à chaque image.
	if !paceFrame(&e.frameCredit, e.OutputFPS()/e.fps) {
		return
	}

	w, h := int(width), int(height)
	var pixels []byte
	// Le rendu matériel n'a pas de tampon : `data` est la sentinelle
	// RETRO_HW_FRAME_BUFFER_VALID, l'image est dans le FBO. Sans contexte
	// ouvert, il n'y a rien à lire — et surtout pas la sentinelle comme un
	// tampon de pixels.
	if libretro.IsHWFrameBufferValid(data) {
		if e.glContext == nil {
			return
		}
		frame, err := e.glContext.ReadPixels(w, h)
		if err != nil {
			if !e.readFailed {
				e.readFailed = true
				log.Printf("[engine] relecture de l'image matérielle impossible : %v", err)
			}

			return
		}
		pixels = frame
	} else {
		pixels = ToBGRA(data, w, h, int(pitch), e.pixelFormat)
	}

	e.frames++
	if e.config.OnFrame != nil {
		e.config.OnFrame(Frame{Pixels: pixels, Width: w, Height: h, Aspect: math.Float64frombits(e.aspect.Load())})
	}
}

// audioBatch — le son se tait pendant l'avance rapide et le rembobinage, tout
// en disant au cœur qu'il est pris : un tampon qu'il croirait plein le
// bloquerait.
func (e *Engine) audioBatch(data unsafe.Pointer, frames uint32) uint32 {
	if data == nil || frames == 0 {
		return 0
	}
	if e.muted() {
		return frames
	}
	e.appendAudio(unsafe.Slice((*int16)(data), int(frames)*2))

	return frames
}

func (e *Engine) audioSample(left, right int16) {
	if e.muted() {
		return
	}
	e.appendAudio([]int16{left, right})
}

// muted — pas de son avant la boucle (l'appelant n'est pas prêt à le
// recevoir), pendant l'avance rapide ni pendant le rembobinage.
func (e *Engine) muted() bool {
	if !e.looping {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.fastForward || e.rewinding
}

func (e *Engine) appendAudio(samples []int16) {
	e.audioBuf = append(e.audioBuf, samples...)
	if len(e.audioBuf) < e.audioBufMax {
		return
	}
	chunk := make([]int16, len(e.audioBuf))
	copy(chunk, e.audioBuf)
	e.audioBuf = e.audioBuf[:0]
	if e.config.OnAudio != nil {
		e.config.OnAudio(chunk)
	}
}

func (e *Engine) inputState(port, device, index, id uint32) int16 {
	if !e.devicesShown[device] {
		if e.devicesShown == nil {
			e.devicesShown = map[uint32]bool{}
		}
		e.devicesShown[device] = true
		log.Printf("[engine] le cœur interroge le périphérique %d (console %s)", device, e.config.ConsoleID)
	}

	return e.inputs.state(port, device, index, id)
}

// rumbleState — le cœur envoie les deux moteurs séparément ; on garde le
// dernier état de chacun, par port, et on rend les deux ensemble.
func (e *Engine) rumbleState(port uint32, effect int32, strength uint16) bool {
	if int(port) >= MaxPorts || effect < 0 || effect > 1 {
		return false
	}
	e.mu.Lock()
	// effect 0 = RETRO_RUMBLE_STRONG, 1 = RETRO_RUMBLE_WEAK.
	if effect == 0 {
		e.rumble[port][1] = strength
	} else {
		e.rumble[port][0] = strength
	}
	weak, strong := e.rumble[port][0], e.rumble[port][1]
	e.mu.Unlock()

	if e.config.OnRumble != nil {
		e.config.OnRumble(int(port), weak, strong)
	}

	return true
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f Mo", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d Ko", n/(1<<10))
	default:
		return fmt.Sprintf("%d o", n)
	}
}
