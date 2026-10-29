// Server-side emulation via Libretro cores (.so), streamed to the browser
// over WebRTC — replaces the EmulatorJS (client-side WASM) approach. Ported
// from the standalone webrtcso prototype (see
// /home/gixi/Github/retroremoteplay-migration/webrtcso) into this server's
// existing retrogaming package, reusing its ROM-scanning conventions
// (ConsoleSummary/RomEntry from scanner.go, resolved by catalog.go's
// ResolveCatalogRom) instead of duplicating a second ROM-listing API.
//
// The CGo bridge (internal/retrogaming/libretro) holds the loaded core as
// process-global state (dlopen/retro_init are not per-session) — so each
// emulation session now runs in its own dedicated child OS process (see
// child.go) rather than sharing this package's process. The
// per-connection logic in this file (serveEmulationSession, the players, the
// encoders, save/load state files) is otherwise unchanged from when it ran
// directly in the shared server process — the sessions service relays the
// browser's WebSocket to it. The game itself runs on the shared libretro
// engine (gameflex/libretro/engine, chantier 13), the phone's too.
package retrogaming

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gameflex/consoles"
	"gameflex/gatewayclient"
	"gameflex/libretro"
	"gameflex/libretro/engine"
	"gameflex/retrogaming/internal/retrogaming/encoder"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// =========================================================================
// Console → Libretro core mapping
// =========================================================================

// gameFileExtPriority ranks which file to hand to the core when a ROM
// resolves to a directory (a multi-file game — see scanner.go's convention:
// the directory itself is the RomEntry, covering every file inside it, e.g.
// a PS1 .cue + several .bin tracks). Libretro cores want the "entry point"
// file (the .cue/.m3u referencing the rest), never a raw .bin/.img track.
var gameFileExtPriority = []string{".m3u", ".cue", ".iso", ".chd", ".pbp"}

// resolveGameFile turns a (consoleID, reference) pair — the same vocabulary
// the kernel's catalog hands out — into the single file path to hand to
// libretro.LoadGame, picking the right entry-point file out of a multi-file
// directory when needed.
func resolveGameFile(root, consoleID, reference string) (string, error) {
	path, isDir, err := ResolveCatalogRom(root, consoleID, reference)
	if err != nil {
		return "", err
	}
	if !isDir {
		return path, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || isHidden(e.Name()) {
			continue
		}
		files = append(files, e.Name())
	}
	if len(files) == 0 {
		return "", fmt.Errorf("empty game folder: %s", path)
	}

	for _, ext := range gameFileExtPriority {
		for _, f := range files {
			if strings.EqualFold(filepath.Ext(f), ext) {
				return filepath.Join(path, f), nil
			}
		}
	}
	// No preferred entry-point extension found — fall back to the first
	// file (os.ReadDir's sorted order).
	return filepath.Join(path, files[0]), nil
}

// resolveCoreFile trouve le fichier d'un core, rangé par console d'abord.
//
//	cores/<consoleId>/<nom>_libretro.so   le rangement par console
//	cores/<nom>_libretro.so               la pile à plat
//
// Le rangement par console est celui qu'on veut : deux cents fichiers
// déposés côte à côte ne disent plus à quoi ils servent, alors que
// `cores/snes/` se lit d'un coup d'œil — le même raisonnement que pour
// `bios/<console>/`. Un core sert parfois plusieurs consoles
// (genesis_plus_gx couvre la Master System ET la Mega Drive) : dans ce
// rangement, il est déposé sous chacune.
//
// La pile à plat reste lue derrière, parce qu'elle est ce qu'une
// bibliothèque existante contient, et qu'un rangement ne se fait pas au prix
// d'une partie qui refuse de démarrer. Même ordre de lecture que
// `retro_files.ListCoreFiles` côté kernel et que `Cores.java` côté
// téléphone : les trois doivent trouver le même fichier.
func resolveCoreFile(coresRoot, consoleID, coreName string) (string, error) {
	file := coreName + "_libretro.so"
	scoped := filepath.Join(coresRoot, strings.ToLower(consoleID), file)
	if info, err := os.Stat(scoped); err == nil && !info.IsDir() {
		return scoped, nil
	}

	flat := filepath.Join(coresRoot, file)
	if info, err := os.Stat(flat); err == nil && !info.IsDir() {
		return flat, nil
	}

	return "", fmt.Errorf("cherché dans %s puis %s", scoped, flat)
}

// resolveRomForCore transparently decompresses a .zip ROM before handing it
// to a core. Some cores (confirmed working: genesis_plus_gx, snes9x) have
// their own built-in zip support and load a raw .zip fine; others (confirmed
// failing: mgba/GBA — retro_load_game returns false, no crash, just a clean
// rejection) expect already-decompressed ROM data and don't unzip anything
// themselves. Rather than depending on each core's own inconsistent support,
// every core is always handed the same thing: a plain, already-decompressed
// ROM — both as an in-memory blob AND as a real extracted file on disk
// (mirrors LoadGame's own "always pass both path and data, some cores only
// use one or the other" approach — confirmed necessary: desmume/NDS still
// rejected an in-memory-only decompression, since it needs an actual file
// per GetSystemInfo().NeedFullpath). Picks the largest entry in the archive
// as the ROM itself if it contains more than one file (in practice, any
// incidental extras in a single-game zip — a .txt/.nfo — are far smaller).
// The returned path is a real temp file the caller must remove once the
// session using it ends (see emulationSession.tempRomPath).
func resolveRomForCore(romPath string, raw []byte) (loadPath string, data []byte, err error) {
	if !strings.EqualFold(filepath.Ext(romPath), ".zip") {
		return romPath, raw, nil
	}
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", nil, fmt.Errorf("invalid zip: %w", err)
	}
	var best *zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() || isHidden(f.Name) {
			continue
		}
		if best == nil || f.UncompressedSize64 > best.UncompressedSize64 {
			best = f
		}
	}
	if best == nil {
		return "", nil, fmt.Errorf("no files found inside zip")
	}
	rc, err := best.Open()
	if err != nil {
		return "", nil, fmt.Errorf("failed to open zip entry %q: %w", best.Name, err)
	}
	defer rc.Close()
	data, err = io.ReadAll(rc)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read zip entry %q: %w", best.Name, err)
	}

	// Scoped by this OS process's PID — necessary now that each emulation
	// session runs in its OWN container (see child.go):
	// two concurrent sessions extracting a same-named zip entry (the same
	// game played twice at once, or just a coincidental basename collision)
	// used to share one unscoped directory and could clobber/delete each
	// other's file. PID is unique per process and trivially available
	// without threading a sessionID through this whole call chain.
	tmpDir := filepath.Join(os.TempDir(), "retrogaming-extracted", fmt.Sprintf("%d", os.Getpid()))
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	tmpPath := filepath.Join(tmpDir, filepath.Base(best.Name))
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", nil, fmt.Errorf("failed to write extracted ROM: %w", err)
	}
	return tmpPath, data, nil
}

// =========================================================================
// WebSocket signaling messages
// =========================================================================

// pcsx2RendererExperiment forces pcsx2's "Video > Renderer" option — see
// the GET_VARIABLE case that reads it. "" (the value to keep) leaves the
// core on its own default. Kept as a named knob rather than deleted
// because it answered a real question on 2026-08-12 and will be the first
// thing to re-try on the next PS2 attempt: forcing "Software (SW)" changes
// nothing about the first-retro_run segfault, which rules the GL/headless
// context out as its cause. Other values: OpenGL, Vulkan, paraLLEl-GS,
// Software (HW).
const pcsx2RendererExperiment = ""

// Les cœurs qui veulent leurs ports après retro_load_game, et les consoles à
// qui le rendu matériel est accordé, vivent dans le moteur partagé
// (engine.LatePortRegistration, engine.HWRenderAllowed) : le téléphone en
// hérite (chantier 13).

type streamMessage struct {
	Type      string `json:"type"`
	ConsoleID string `json:"consoleId,omitempty"`
	Reference string `json:"reference,omitempty"` // matches RomEntry.Reference (a catalog uuid, see catalog.go)
	// Title — the game's human display name (RomEntry.Title), sent
	// alongside Reference on "start" purely so sessionRecord/
	// internal/sessions.Session have something besides a raw uuid to show
	// in presence widgets/session lists/dashboard.
	Title string `json:"title,omitempty"`
	// Core — explicit core choice from the frontend's per-console picker
	// (internal/consoles registry's Cores list — some consoles list more
	// than one working core). Optional: empty falls back to the registry's
	// own single default, so older/simpler callers that never send it
	// still work.
	Core string `json:"core,omitempty"`
	// Bios — explicit BIOS file choice from the frontend's per-console
	// picker, which the KERNEL serves (retro_files.ListBiosFiles, reading
	// bios/<consoleId>/ — the folder is the console id, see consoles.BiosDir).
	// Optional: empty keeps the flat BIOS root, except for a core that only
	// reads a mirror subfolder (see resolveBiosDir's own doc).
	Bios  string `json:"bios,omitempty"`
	Error string `json:"error,omitempty"`
	// Code — un refus que la page dit dans sa langue : « state-refused », le
	// cœur ne relit pas cette sauvegarde (#396), avec celui qui l'avait écrite
	// quand elle le dit (StateCore). `Core` sert alors DANS L'AUTRE SENS : le
	// cœur installé, qui refuse. Les mêmes champs qu'au téléphone
	// (relayMessage).
	Code      string `json:"code,omitempty"`
	StateCore string `json:"stateCore,omitempty"`
	// Step / Detail — LA PROGRESSION DU DÉMARRAGE (#191).
	//
	// Le chargement d'une partie traverse des jalons que ce processus est seul
	// à connaître — cœur résolu, cœur chargé, ROM lue ou passée par chemin,
	// BIOS retenu, jeu chargé, géométrie et cadence lues — et qui ne partaient
	// jusqu'ici que dans ses propres `log.Printf`, c'est-à-dire dans un fichier
	// que personne ne lit pendant qu'il attend. À l'écran, toute cette durée
	// tenait sous un unique « En attente de la vidéo… ».
	//
	// Step est un IDENTIFIANT stable, que la page traduit ; Detail le
	// renseignement qui n'a de sens que pour ce jalon-là (le nom du cœur, la
	// taille lue, « 320x240 @ 60.0 images/s »). Jamais de phrase traduite ici :
	// ce processus ne connaît pas la langue de celui qui regarde.
	Step   string `json:"step,omitempty"`
	Detail string `json:"detail,omitempty"`
	// SessionID — bidirectional. Client -> server on a "join" message
	// (2nd-player join, see child.go's serveJoinerSession) to say which
	// existing session to attach to. Server -> client on the owner's
	// "started" message (see sendGatewayStarted) to tell the frontend its own
	// newly-spawned session's id (minted by the sessions service when it
	// spawns the container on "start" — the client doesn't know it up
	// front, unlike "join" where it already does).
	SessionID string `json:"sessionId,omitempty"`
	// Username — sent by the frontend on "start" from the logged-in user's
	// own account (see python/src/app/users/models.py), trusted as-is (see
	// stream_session.go's top-of-file doc / GAMEFLEX_JOURNAL.md — the
	// user explicitly chose not to have Go cryptographically verify this).
	// Scopes SAVES_PATH per-user (see handleStart) — empty falls back to
	// "anonymous" so an unauthenticated/older caller still works, just
	// without real per-user save isolation.
	Username string `json:"username,omitempty"`
	// ViewerID — the browser's gateway viewer id (created via POST
	// /api/v1/gateway-webrtc/streams/{sessionId}/viewer-offer BEFORE the
	// start/join message), so this child can map gateway input/viewer events
	// back to a controller port.
	ViewerID string `json:"viewerId,omitempty"`
	// Takeover — the start message re-sent AFTER the player confirmed the
	// device handoff on the new device (see serveEmulationSession's guard).
	Takeover bool `json:"takeover,omitempty"`
	// Info — ack payload for non-input control messages ("savestate"/
	// "loadstate" echoed back with Info: "saved"/"loaded" on success; a
	// failure uses the existing Type: "error" path instead).
	Info string `json:"info,omitempty"`
	// Slot — bidirectional, "savestate"/"loadstate" only. Client -> server:
	// which of the 10 slots (0-9, see maxSaveSlots/autoSaveSlot) to act on.
	// Server -> client: echoed back on the ack so the frontend's slot
	// carousel knows which one just changed. No omitempty — slot 0 (the
	// auto-save slot) is a real, meaningful value here, not "field absent".
	Slot int `json:"slot"`
	// Disks — serveur -> client seulement, message "disks" : combien
	// d'images le jeu en cours contient. 0 veut dire « la question n'a pas
	// de sens ici » — core sans interface de disques, ou jeu en un seul
	// fichier — et c'est ce qui fait disparaître le sélecteur du menu sans
	// qu'aucun drapeau de capacité n'ait à être tenu à jour des deux côtés.
	Disks int `json:"disks"`
	// Disk — bidirectionnel. Client -> serveur sur "changedisk" : l'image à
	// monter. Serveur -> client sur "disks" : celle qui l'est réellement,
	// relue dans le core après coup — pas celle qu'on a demandée. -1 quand
	// le core ne sait pas le dire. Pas d'omitempty : le disque 0 est une
	// valeur, pas un champ absent.
	Disk int `json:"disk"`
	// Enabled — "fastforward"/"rewind" toggle state, both request (client ->
	// server) and ack (server -> client, echoes the state actually applied).
	Enabled bool `json:"enabled,omitempty"`
	// Port — which controller port a "playerleft"/"playerjoined" broadcast
	// (see emulationSession.broadcast) is about. 0 is always the owner;
	// 1..maxPlayers-1 are joiners (see joiners' doc).
	Port int `json:"port,omitempty"`
	// Visible — client -> server only, "visibility" message, one connection's
	// own document.visibilityState (see updateVisibility/recomputePauseState
	// and the frontend's PiP-on-hide hook). No omitempty: false is a real,
	// meaningful value here (tab backgrounded), not "field absent".
	Visible bool `json:"visible"`
	// Paused — client -> serveur, message "pause" : la pause DÉLIBÉRÉE,
	// demandée en quittant. Pas d'omitempty — `false` est la reprise, pas
	// un champ absent.
	Paused bool `json:"paused"`
	// SaveProgress — client -> server only, "exit" message, sent by the
	// owner right before intentionally closing the WS (see Streaming.jsx's
	// handleExit and its exit-confirm checkbox). No omitempty: false (the
	// checkbox left unchecked) is a real, meaningful "skip the auto-save"
	// value here, not "field absent" — see emulationSession.skipAutoSave.
	SaveProgress bool `json:"saveProgress"`
	// Cheats — bidirectional. Client -> server on "setcheats" (the full
	// desired list every time, see handleSetCheats); server -> client on
	// the "cheats" ack, echoing back what was actually applied.
	Cheats []Cheat `json:"cheats,omitempty"`
	// CheatsSupported — server -> client only, sent once on the owner's
	// "started" message (sendGatewayStarted) — whether THIS core exports
	// retro_cheat_set at all (see libretro.CheatSupported), so the frontend
	// can hide the whole Cheats tab instead of a form that visibly does
	// nothing.
	CheatsSupported bool `json:"cheatsSupported,omitempty"`
	// RewindSupported — server -> client only, sent on the same "started"
	// message as CheatsSupported: whether rewind is
	// actually available for THIS core (see the engine's rewind.go), so the
	// menu hides the switch instead of offering one that would cost a
	// stutter every second for nothing. No omitempty: false is the
	// meaningful "hide it" value here, not "field absent" — same reasoning
	// as SaveProgress above.
	RewindSupported bool `json:"rewindSupported"`
	// FastForwardSupported — server -> client only, same "started" message:
	// the GameFlex engine fast-forwards any core, so always true here. The
	// phone says false for a desktop guest (RPCS3, xemu…), and the menu hides
	// a switch that would do nothing (2026-10-28).
	FastForwardSupported bool `json:"fastForwardSupported"`
	// RumbleWeak/RumbleStrong — server -> client only, "rumble" message
	// (see onRumble). 0-65535, mirroring libretro's own
	// retro_set_rumble_state_t strength range as-is — the frontend maps
	// this down to whatever range navigator.vibrate()/the Gamepad Haptics
	// API actually wants, not this side's job.
	RumbleWeak   uint16 `json:"rumbleWeak,omitempty"`
	RumbleStrong uint16 `json:"rumbleStrong,omitempty"`
	// Port/DeviceID — client -> server only, "setcontrollertype": the
	// controller choice of the game menu (the PS1's DualShock, #309; see
	// handleSetControllerType) — Port (declared above, already used by
	// playerleft/playerjoined) is reused as-is.
	DeviceID uint32 `json:"deviceId,omitempty"`
}

// LA CADENCE, L'AVANCE RAPIDE, LE REMBOBINAGE ET LA SAUVEGARDE DE CARTOUCHE
// vivent dans le moteur partagé (gameflex/libretro/engine), avec leurs
// garde-fous et leur histoire — chantier 13, phase 1. Cette session-ci garde le
// TRANSPORT : les connexions, les encodeurs, les joueurs, et où ranger les
// fichiers.

// =========================================================================
// emulationSession: one browser client = one emulation session
//
// Each session now runs in its OWN container (see child.go) — the CGo bridge's process-global core state (dlopen/retro_init)
// is therefore naturally isolated per session without needing a shared
// in-process gate: a child process only ever runs ONE emulationSession in
// its lifetime, so there's nothing else in that process to collide with.
// (Previously a package-level globalEmulationActive mutex/bool served this
// purpose when every session shared one process — removed now that it's
// structurally impossible for two sessions to share a core.)
// =========================================================================

// activeChildSession tracks the one emulationSession a child process is
// running, if any — set by serveEmulationSession, cleared by shutdown().
// Only meaningful inside a child process (see child.go's RunChild); the
// the sessions service's relay never creates an emulationSession, so
// this stays nil there. Read by RunChild's SIGTERM handler so a graceful
// `air` rebuild/restart can unload the core
// and stop ffmpeg cleanly before the child process exits, instead of
// leaking an orphaned ffmpeg subprocess.
var (
	activeChildSessionMu sync.Mutex
	activeChildSession   *emulationSession
)

// Cheat is one entry in a session's cheat list — the wire shape for
// "setcheats" (client -> server, the full desired list every time) AND its
// ack (server -> client). Code is in whatever raw format the CORE itself
// expects (e.g. Game Genie/Pro Action Replay strings) — there's no cheat
// database/lookup here (see libretro.CheatSet's own doc), the caller/user
// supplies it as-is. Persisted to disk per (user, game) — see
// saveCheatsToDisk/loadCheats — so a fresh session for the same user/game
// starts with whatever list was last pushed, rather than empty every time.
type Cheat struct {
	Code        string `json:"code"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// engineCheats — la liste telle que le moteur l'applique : le code et son
// état, à son index. La description ne regarde que la page et le disque.
func engineCheats(cheats []Cheat) []engine.Cheat {
	out := make([]engine.Cheat, len(cheats))
	for i, cheat := range cheats {
		out[i] = engine.Cheat{Code: cheat.Code, Enabled: cheat.Enabled}
	}

	return out
}

// joinerConn is another player's seat on a controller port — see
// serveJoinerSession (child.go). Its video, audio and input travel through
// gatewaywebrtc, like everyone's.
type joinerConn struct {
	// username — who's occupying this port, sent by the browser on "join"
	// (see child.go's serveJoinerSession) — used for the "playerleft"/
	// "playerjoined" broadcast, and to pick a new initiator (lowest
	// occupied port) when the owner disconnects — see ownerDisconnected.
	username string
	// ViewerID — the browser viewer this port is mapped to (see
	// emulationSession.viewerPorts).
	ViewerID string
}

// spectatorConn is a watch-only connection (see serveSpectatorSession,
// child.go) — never assigned a controller port, so never wired to
// handleInput: a spectator has no way to act on the game at all,
// structurally, not just by UI convention. Its viewer watches through
// gatewaywebrtc; this only counts it.
type spectatorConn struct{}

type emulationSession struct {
	mu sync.Mutex

	ws   *websocket.Conn
	wsMu sync.Mutex
	// gatewayClient — where the encoded frames go: childGatewayClient (the
	// shared transport, see child.go) once the owner has started, nil after
	// shutdown. Every browser connects to gatewaywebrtc directly.
	gatewayClient *gatewayclient.Client
	ownerViewerID string
	// viewerPorts maps a gateway viewerId to a controller port (0 = owner,
	// 1..maxPlayers-1 = joiners) for input routing.
	viewerPorts map[string]int

	videoEnc *encoder.VideoEncoder
	audioEnc *encoder.AudioEncoder

	running bool
	// stopping se ferme quand la session s'arrête (shutdown) : les deux
	// consommateurs d'encodeurs s'en vont. Le moteur, lui, a son propre arrêt.
	stopping   chan struct{}
	frameReady chan []byte
	// engine — la partie elle-même : le cœur, sa cadence, ses états, ses
	// manettes (gameflex/libretro/engine). Posé par handleStart une fois le
	// jeu chargé, gardé sous mu ; nil avant et après.
	engine *engine.Engine

	// tempRomPath is set by resolveRomForCore when the ROM was a .zip that
	// needed extracting to a real file on disk — removed once the session
	// ends (both the normal path in shutdown() and the early-failure defer
	// in handleStart), since it's a scratch copy, not the real ROM.
	tempRomPath string

	// consoleID/reference identify the running game for save-state file
	// naming (see saveStatePath) — set once in handleStart, read-only after.
	consoleID string
	reference string
	// username scopes SAVES_PATH per-user (both libretro.InitEnvPaths'
	// battery/SRAM saves and saveStatePath's .state files) — set once in
	// handleStart (defaulted to "anonymous" if the client didn't send one),
	// read-only after.
	username string

	// cheats — la dernière liste de codes appliquée (propriétaire seul, en
	// solo seulement, comme les sauvegardes d'état), gardée sous mu pour
	// l'accusé "cheats" et le message "started".
	cheats []Cheat

	// paused freezes the game without unloading the core — set by
	// recomputePauseState whenever every currently-connected browser tab
	// (owner/joiners/spectators) reports itself hidden via the "visibility"
	// WS message (see PiP-on-hide, gameflex/shared/usePictureInPictureOnHide.js).
	paused bool

	// heldPaused — LA PAUSE DÉLIBÉRÉE, celle qu'un joueur demande en
	// quittant.
	//
	// Séparée de `paused` juste au-dessus, qui est DÉRIVÉE de la visibilité
	// des onglets et recalculée à chaque changement : la ranger au même
	// endroit ferait défaire le choix du joueur au premier onglet qui
	// s'affiche ou se cache. Deux causes différentes, deux drapeaux ; le
	// moteur gèle si l'un des deux le demande (applyPause).
	heldPaused bool

	// fps/sampleRate — la cadence et le débit de son que le cœur annonce,
	// recopiés du moteur pour les encodeurs.
	fps        float64
	sampleRate float64

	// audioBatchReady carries fully-accumulated batches to a single
	// sequential consumer goroutine (see consumeAudioOutput) — same
	// non-blocking, drop-rather-than-queue policy as frameReady/emitFrame
	// for video. Without this, audio had no backpressure at all (a new
	// unsynchronized goroutine per batch, all of which eventually wrote to
	// ffmpeg regardless of delay) and could fall behind real-time under any
	// momentary slowdown without ever catching back up — see onAudio.
	audioBatchReady chan []int16

	// Ce qu'on a jeté faute de consommateur disponible, et depuis quand — voir
	// le `default:` d'onAudio. Un trou dans le son est audible ; il doit donc
	// être comptable.
	audioDrops      atomic.Int64
	audioDropsSince atomic.Value

	// maxPlayers is this session's console-specific controller-port count
	// (see consoles.MaxPlayers) — sizes inputStates/analogStates/joiners
	// below at handleStart. Port 0 is always the owner; ports 1..maxPlayers-1
	// are up for grabs by joiners.
	maxPlayers int

	// ownerConnected tracks whether port 0's connection (serveEmulationSession)
	// is still up — set true at construction, false the moment its read loop
	// errors out. It decides nothing about the game's life: the queue does
	// (P4, 2026-10-09) — this only says whether a reattach may claim the
	// owner's connection.
	ownerConnected bool

	// joiners holds the other players' own WebRTC connections, if any have
	// joined this session (see serveJoinerSession in child.go) — index i
	// corresponds to controller port i+1 (port 0 is always the owner), nil
	// entries are free slots. Sized to maxPlayers-1 in handleStart. Only
	// ever populated inside a child process (a session's owner process),
	// guarded by mu like the rest of this struct's mutable state.
	joiners []*joinerConn

	// spectators are watch-only connections — no controller port, no input
	// wiring, unlimited (see serveSpectatorSession, child.go) — same
	// already-encoded-bytes-to-N-independent-PeerConnections pattern as
	// joiners, mirroring remoteplay's own unlimited viewer support
	// (internal/api/viewer_handlers.go's StreamState.Viewers). Keyed by a
	// random ID (crypto/rand) rather than a slot index since there's no
	// fixed capacity to index into.
	spectators   map[string]*spectatorConn
	spectatorsMu sync.Mutex

	// biosRoot/savesRoot are handleStart's own params, stashed here for the
	// paths derived from them later (save states, screenshots). The saves
	// stay the OWNER's for the whole game: the "initiator handoff" that used
	// to re-point them at the first joiner is gone (P4, 2026-10-09).
	biosRoot  string
	savesRoot string
	// biosDir is the systemDir ACTUALLY passed to libretro.InitEnvPaths —
	// either biosRoot unchanged, or a session-scoped temp directory
	// containing just the chosen BIOS file (see resolveBiosDir, bios.go).
	// biosTempDir is only set (and only ever needs
	// cleanup, in handleStart's own defer and shutdown()) when biosDir is
	// that temp directory, not biosRoot itself.
	biosDir     string
	biosTempDir string
	// coreName est le cœur chargé (« desmume », « dolphin », « ppsspp »…).
	// Gardé pour la même raison que biosDir/savesRoot juste au-dessus : la
	// passation d'initiateur (ownerDisconnected) doit rappeler
	// libretro.InitEnvPaths, et le dossier de sauvegarde du cœur est keyé par
	// CŒUR — voir coreSaveDir.
	coreName string
	// biosFile is the chosen BIOS FILENAME (not a path), kept because some
	// cores don't pick a BIOS by scanning systemDir at all — they expose a
	// core option naming the file and read it back through GET_VARIABLE.
	// pcsx2's is `pcsx2_bios`: without an answer there it enumerates the
	// dumps at retro_init ("Bios Found: Europe v02.20"), then refuses the
	// game anyway at retro_load_game because nothing ever SELECTED one.
	// Empty whenever no explicit choice was made (see resolveBiosDir).
	biosFile string

	// broadcastTargets holds one entry per currently-connected browser
	// connection — owner, each joiner, AND each spectator (unlike
	// broadcast's own players-only messages, the take-a-slot request flow
	// needs to reach spectators too, see requestSlot/sendToUsername) — keyed
	// by "owner", "joiner-<port>", or "spectator-<id>". Each is served by
	// its own goroutine with its own *websocket.Conn (sess.ws for the
	// owner, a local `ws` in serveJoinerSession/serveSpectatorSession for
	// the others) that broadcast/sendToUsername have no other way to reach.
	broadcastTargets   map[string]*broadcastTarget
	broadcastTargetsMu sync.Mutex

	videoEncStarted bool
	encoderReady    chan struct{}
	encW, encH      int

	// lastFrame/lastFrameW/lastFrameH — the most recently emitted BGRA
	// frame, set in emitFrame — read by captureScreenshot (handleSaveState)
	// to save a thumbnail alongside the .state file. Guarded by mu like the
	// rest of this struct's mutable state.
	lastFrame              []byte
	lastFrameW, lastFrameH int
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mustMarshal is only used for the small fixed streamMessage shapes above —
// json.Marshal on a struct like this cannot fail.
func mustMarshal(msg streamMessage) []byte {
	data, _ := json.Marshal(msg)
	return data
}

// startHeartbeat pings ws every 25s (well under any reverse proxy's idle
// timeout — nginx defaults to 60s — so an otherwise-silent connection never
// looks abandoned to an intermediary) and arms a 60s read deadline that each
// received pong pushes back out. gorilla/websocket answers pings
// automatically browser-side, no client change needed. Ghost-session fix:
// without the deadline half, a client that vanishes without a clean close
// (wifi cut, laptop sleeps — no FIN/RST) leaves ReadMessage blocked
// indefinitely and the session running forever (confirmed in practice); if
// the pong never arrives, ReadMessage instead returns a timeout error and
// the caller's existing cleanup-on-error path runs exactly as it would for
// a normal disconnect. Callers must pass the *same* mutex they use to guard
// their own writes to ws — gorilla forbids concurrent WriteMessage calls on
// one *Conn, and the ping goroutine writes to it too.
func startHeartbeat(ws *websocket.Conn, writeMu *sync.Mutex) (stop func()) {
	const pongWait = 60 * time.Second
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				writeMu.Lock()
				err := ws.WriteMessage(websocket.PingMessage, nil)
				writeMu.Unlock()
				if err != nil {
					return
				}
			case <-stopCh:
				return
			}
		}
	}()
	return func() { close(stopCh) }
}

// serveEmulationSession is what the old in-process handler used to be — now
// only ever invoked inside a CHILD process (see child.go's RunChild), on
// its own local `/session` route, one connection per process lifetime.
func (h *Handler) serveEmulationSession(w http.ResponseWriter, r *http.Request) {
	// LE PROPRIÉTAIRE, C'EST LA FILE QUI LE DIT — et c'est ce child qui en
	// juge, pas le kernel (chantier 4). Cette porte croyait quiconque y
	// arrivait : le relais du kernel laissait tout compte connecté envoyer
	// `start` sur une session tenue par son child, et le conteneur écoute de
	// toute façon sur le réseau local. Or la connexion du propriétaire
	// sauvegarde, charge, met en pause et ARRÊTE la partie (relevé le
	// 2026-10-08). Le relais transmet maintenant le jeton du navigateur ;
	// ce child le vérifie lui-même, et seul celui que sa file désigne
	// comme propriétaire ouvre cette porte. Le propriétaire ne change
	// jamais : quand il prête sa manette, il garde sa place et cette
	// connexion, et l'emprunteur reste un spectateur dont seules les
	// touches vont au pad 1 (#274).
	if v4 != nil {
		username, ok := v4.Identify(r)
		if !ok {
			http.Error(w, "invalid or missing token", http.StatusUnauthorized)

			return
		}
		if owner := v4.Host.Snapshot().Owner; username != owner {
			log.Printf("[retrogaming-stream] owner socket refused to %q (owner is %q)", username, owner)
			http.Error(w, "not the owner of this session", http.StatusForbidden)

			return
		}
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[retrogaming-stream] child WebSocket upgrade error: %v", err)
		return
	}
	defer ws.Close()

	// Device handoff flag — forwarded by the relay from the frontend's start
	// Multiple connections are allowed: the latest owner attach always wins
	// (the reattach branch below tells the previous device to switch to
	// watching). The relay still forwards ?takeover=1 for an explicit
	// "bring it here" click, but the gate it used to unlock is gone
	// (2026-08-23: what is forbidden is SPAWNING a second session as owner,
	// the kernel's one-active-session-per-player rule).

	// Owner-attach guard: a child process is dedicated to exactly ONE
	// session AND one loaded libretro core for its entire lifetime — the
	// CGo bridge holds the core as process-global state (see this file's
	// top-of-file doc), so a second serveEmulationSession call creating a
	// second *emulationSession and calling handleStart again would try to
	// load a SECOND core into the same process-global slot from scratch,
	// while any existing joiners stay attached to the OLD (now orphaned)
	// session object — corrupting state rather than either connection
	// cleanly failing.
	//
	// Reconnect ("MySession"/Dashboard.jsx rejoin, and a plain page refresh):
	// the owner's tab closing leaves activeChildSession set with
	// ownerConnected already false — the game runs on as long as the queue
	// keeps it (somebody plays, or the idle delay has not run out), so this
	// can reattach to the SAME running session instead of either failing or
	// reloading the ROM from scratch (which would reset the game a
	// rejoining player did not ask to restart). Only a
	// genuinely still-connected owner (ownerConnected true — two tabs open
	// at once, or a reattach racing the old tab's own disconnect) gets
	// rejected below; everything else reattaches.
	activeChildSessionMu.Lock()
	existing := activeChildSession
	activeChildSessionMu.Unlock()

	var sess *emulationSession
	isReattach := false

	if existing != nil {
		// Check-and-claim under ONE lock, not check-then-act — two reattach
		// attempts racing each other (e.g. the same rejoin link opened in
		// two tabs) would otherwise both read ownerConnected==false and
		// both proceed, the second silently stealing the peer connection
		// the first just set up.
		existing.mu.Lock()
		stillOwned := existing.ownerConnected
		if !stillOwned {
			existing.ownerConnected = true
		}
		existing.mu.Unlock()

		if stillOwned {
			// Multiple connections are ALLOWED (watch-first pages, retries,
			// a second device): the latest owner attach wins and the
			// previous device is TOLD to switch to watching — never a
			// silent steal, never a rejection. The former "this session
			// already has an active connection" rejection was REMOVED
			// (2026-08-23): what is forbidden is SPAWNING a second session
			// as owner — the kernel's one-active-session-per-player rule —
			// not connecting to an existing one.
			log.Printf("[retrogaming-stream] owner reattach — switching the previous device to watch")
			existing.sendMessage(streamMessage{Type: "takeover"})
			existing.wsMu.Lock()
			if existing.ws != nil {
				existing.ws.Close()
			}
			existing.wsMu.Unlock()
		}

		if err := existing.reattachOwner(ws); err != nil {
			log.Printf("[retrogaming-stream] reattach failed: %v", err)
			ws.WriteMessage(websocket.TextMessage, mustMarshal(streamMessage{Type: "error", Error: "reconnect failed: " + err.Error()}))
			return
		}
		sess = existing
		isReattach = true
	} else {
		sess = &emulationSession{
			ws:              ws,
			stopping:        make(chan struct{}),
			frameReady:      make(chan []byte, 4),
			encoderReady:    make(chan struct{}),
			audioBatchReady: make(chan []int16, 4),
			// Initialized here, not in handleStart — activeChildSession below
			// makes this session reachable to /session/spectate (and /join)
			// immediately, before handleStart (triggered separately by the
			// "start" message, see the read loop below) has run far enough to
			// set this up itself. A spectator connecting in that window used to
			// panic on a nil map write (confirmed via testing) — same reasoning
			// applies to inputStates/joiners/maxPlayers below, though a nil
			// slice there just harmlessly range()s as empty rather than panics.
			spectators:     make(map[string]*spectatorConn),
			viewerPorts:    make(map[string]int),
			ownerConnected: true,
		}

		activeChildSessionMu.Lock()
		activeChildSession = sess
		activeChildSessionMu.Unlock()
	}

	// Username isn't known yet at this point (only handleStart's "start"
	// message carries it, below) — registered with a blank one for now,
	// filled in by updateBroadcastTargetUsername once handleStart sets
	// s.username, so sendToUsername can reach the owner too.
	sess.registerBroadcastTarget("owner", "", sess.sendMessage)
	defer sess.unregisterBroadcastTarget("owner")

	// This leg (child's own /session endpoint) is only ever dialed by the
	// sessions service's relay, not directly by a browser — a relay that dies
	// still closes this
	// socket via a normal TCP RST/FIN, so the ghost-connection risk that
	// motivates startHeartbeat elsewhere is much smaller here. Added anyway
	// for symmetry with serveJoinerSession/serveSpectatorSession, and as a
	// backstop against any other case (e.g. a hung parent) that wouldn't
	// otherwise close the socket cleanly.
	stopHeartbeat := startHeartbeat(ws, &sess.wsMu)
	defer stopHeartbeat()

	for {
		_, msgBytes, err := ws.ReadMessage()
		if err != nil {
			log.Printf("[retrogaming-stream] read error: %v", err)
			// The owner's connection closing ends nothing: the queue decides
			// the game's life (P4, 2026-10-09). This only closes port 0's own
			// connection (the deferred ws.Close() above).
			// Identity guard: a takeover-kicked connection must NOT run this
			// teardown (the new device already rebinded as owner).
			if sess.isCurrentOwnerWS(ws) {
				sess.ownerDisconnected()
			}
			return
		}

		var msg streamMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("[retrogaming-stream] invalid message: %v", err)
			continue
		}

		switch msg.Type {
		case "start":
			// The frontend always sends "start" on connect regardless of
			// whether this is a fresh session or a reattach (it can't tell
			// the difference itself) — reattachOwner already ran (and
			// already sent a fresh "started") before this loop started, so
			// running handleStart here too would reload the ROM into the
			// SAME live core a rejoining player very much wants left alone.
			//
			// The owner's gateway viewer id rides this message —
			// recorded BEFORE the isReattach check (a reattach brings a NEW
			// viewerId, and input routing must follow it, not the dead one).
			if msg.ViewerID != "" {
				sess.mu.Lock()
				sess.ownerViewerID = msg.ViewerID
				sess.viewerPorts[msg.ViewerID] = 0
				sess.mu.Unlock()
			}
			if isReattach {
				continue
			}
			go sess.handleStart(h.romsRoot(), h.coresRoot(), h.savesRoot(), h.biosRoot(), msg.ConsoleID, msg.Reference, msg.Core, msg.Bios, msg.Username)
		case "savestate":
			go sess.handleSaveState(h.savesRoot(), msg.Slot)
		case "loadstate":
			go sess.handleLoadState(h.savesRoot(), msg.Slot)
		case "setcheats":
			go sess.handleSetCheats(msg.Cheats)
		case "fastforward":
			go sess.handleFastForward(msg.Enabled)
		case "rewind":
			go sess.handleRewind(msg.Enabled)
		case "setcontrollertype":
			go sess.handleSetControllerType(msg.Port, msg.DeviceID)
		case "disks":
			go sess.handleDisks()
		case "changedisk":
			go sess.handleChangeDisk(msg.Disk)
		case "reset":
			go sess.handleReset()
		case "visibility":
			sess.updateVisibility("owner", msg.Visible)
		case "pause":
			sess.holdPause(msg.Paused)
		case "exit":
			// RIEN À FAIRE ICI. Sur un serveur, une partie s'arrête par la
			// file — `terminate`, ou le `session.stop` du centre de messages
			// que ce child juge. Ce message en décidait encore par une
			// horloge à lui, en parallèle de la file : retirée le 2026-10-09
			// (P4). Le téléphone, lui, s'en sert toujours — son relais
			// arrête la partie sur `exit` —, d'où la page qui l'envoie.
		default:
			log.Printf("[retrogaming-stream] unknown message type: %s", msg.Type)
		}
	}
}

// attachOwnerPeer (re)attaches the owner's side only — NOT the core/ROM
// state, which handleStart alone owns. Used two ways: once, at the tail of
// handleStart's normal startup, and again by reattachOwner (below) when the
// owner reconnects to a session that's still running. The gateway already
// hosts the browser's viewer: this points the frames at it and acks the
// start with the session info (sendGatewayStarted).
func (s *emulationSession) attachOwnerPeer() error {
	s.mu.Lock()
	s.gatewayClient = childGatewayClient
	s.mu.Unlock()
	s.sendGatewayStarted()

	return nil
}

// reattachOwner rebinds an already-running session to a NEW owner
// connection — the "MySession"/rejoin path (Dashboard.jsx): the owner left
// (tab closed) while the queue kept the game alive, so activeChildSession is
// still this same *emulationSession* with ownerConnected already false. Called
// from serveEmulationSession INSTEAD OF constructing a fresh
// *emulationSession + handleStart — the core is already loaded and the
// engine is already running, re-running either would reset the game
// a rejoining player very much did not ask to restart.
func (s *emulationSession) reattachOwner(ws *websocket.Conn) error {
	s.wsMu.Lock()
	s.ws = ws
	s.wsMu.Unlock()

	s.mu.Lock()
	s.ownerConnected = true
	// REVENIR JOUER, C'EST REPRENDRE. Une pause demandée en quittant n'a de
	// sens que jusqu'au retour de celui qui l'a demandée : la laisser tenir
	// obligerait à un second geste pour un choix qu'on vient de faire en
	// rouvrant la partie.
	s.heldPaused = false
	s.mu.Unlock()
	s.applyPause()

	s.registerBroadcastTarget("owner", "", s.sendMessage)

	if err := s.attachOwnerPeer(); err != nil {
		return err
	}

	log.Printf("[retrogaming-stream] owner reattached to running session")
	return nil
}

// =========================================================================
// Session: start emulation
// =========================================================================

func (s *emulationSession) handleStart(romsRoot, coresRoot, savesRoot, biosRoot, consoleID, reference, explicitCore, bios, username string) {
	if username == "" {
		username = "anonymous"
	}

	s.mu.Lock()
	s.running = true
	s.consoleID = consoleID
	s.reference = reference
	s.username = username
	s.biosRoot = biosRoot
	s.savesRoot = savesRoot
	s.mu.Unlock()
	s.updateBroadcastTargetUsername("owner", username)

	// Any early return below (missing core/ROM, a load failure, encoder/
	// WebRTC setup failure, ...) must undo what was started — otherwise a
	// single failed "start" leaves the session marked running for good. A
	// failed engine.Load has already unloaded everything; an engine that
	// loaded is stopped here, which unloads the core on its own thread.
	startedOK := false
	var eng *engine.Engine
	defer func() {
		if startedOK {
			return
		}
		if eng != nil {
			eng.Stop()
		}

		if s.tempRomPath != "" {
			os.Remove(s.tempRomPath)
			s.tempRomPath = ""
		}
		if s.biosTempDir != "" {
			persistBiosCompanions(s.biosRoot, s.savesRoot, s.consoleID, s.biosFile, s.biosTempDir)
			os.RemoveAll(s.biosTempDir)
			s.biosTempDir = ""
		}

		s.mu.Lock()
		s.engine = nil
		s.running = false
		s.mu.Unlock()
	}()

	// The frontend's per-console core picker (consoles registry's `cores`
	// list, GameDetailPanel's radio buttons) sends its selection explicitly
	// — prefer that over the single default whenever a console genuinely
	// has more than one working core (e.g. nes: fceumm/nestopia). Falls back
	// to the registry's own first entry for older/simpler callers that never
	// set it.
	coreName := explicitCore
	if coreName == "" {
		cores := consoles.CoresFor(strings.ToLower(consoleID))
		if len(cores) == 0 {
			s.sendError(fmt.Sprintf("No Libretro core mapped for console: %s", consoleID))
			return
		}
		coreName = cores[0]
	}

	s.mu.Lock()
	s.coreName = coreName
	s.mu.Unlock()

	corePath, err := resolveCoreFile(coresRoot, consoleID, coreName)
	if err != nil {
		s.sendError(fmt.Sprintf("Core not found: %s (%v)", coreName, err))
		return
	}
	s.sendProgress("core.resolved", coreName)

	reference, err = url.PathUnescape(reference)
	if err != nil {
		s.sendError(fmt.Sprintf("Invalid reference encoding: %v", err))
		return
	}
	romPath, err := resolveGameFile(romsRoot, consoleID, reference)
	if err != nil {
		s.sendError(fmt.Sprintf("ROM not found: %s/%s (%v)", consoleID, reference, err))
		return
	}

	// Every port up to this console's maxPlayers is registered (by the
	// engine, before retro_init) so a player can join at any point during
	// the session — see joinerConn/serveJoinerSession. NOTE: for consoles
	// configured above 2, the core itself may ALSO need a multitap option
	// enabled to actually read more than 2 ports — untested beyond 2.
	maxPlayers := consoles.MaxPlayers(consoleID)
	s.mu.Lock()
	s.maxPlayers = maxPlayers
	s.joiners = make([]*joinerConn, maxPlayers-1)
	s.mu.Unlock()

	// LE MOTEUR CHARGE LE CŒUR ET LE JEU, SUR SON PROPRE FIL (chantier 13). Ce
	// qui reste ici est ce que lui ne peut pas savoir : où sont les fichiers
	// de CE serveur (prepareGame), où ranger ce qu'il rend, et à qui l'envoyer.
	eng, err = engine.Load(engine.Config{
		CorePath:  corePath,
		Core:      coreName,
		ConsoleID: consoleID,
		Ports:     maxPlayers,
		Game:      engine.Game{Path: romPath},
		// La cadence choisie dans le panneau des réglages de flux (#332) : le
		// moteur ne rend alors qu'une partie des images du cœur.
		StreamFPS: float64(launchSetting("GAMEFLEX_FPS")),
		// La hauteur choisie au même endroit (#340) : un cœur qui a une
		// résolution interne dessine au plus près, au lieu de rendre sa
		// taille d'époque qu'on agrandit ensuite. Sans choix, son défaut.
		StreamHeight: chosenHeight(),
		// La langue de la page qui a lancé (chantier 21), passée par le
		// kernel : le moteur la répond aux cœurs.
		Language: consoles.Language(os.Getenv("GAMEFLEX_LANGUAGE")),
		Prepare: func(system libretro.SystemInfo, game *engine.Game) error {
			return s.prepareGame(system, game, romPath, savesRoot, biosRoot, consoleID, coreName, bios, username)
		},
		SaveRAM:         readSaveRAM(savesRoot, username, consoleID, reference),
		OnSaveRAM:       s.writeSaveRAM,
		OnFrame:         s.onFrame,
		OnAudio:         s.onAudio,
		OnRumble:        s.onRumble,
		OnRewindSupport: func(supported bool) { go s.sendRewindSupport(supported) },
		OnProgress:      s.sendProgress,
		OnStall:         s.onStall,
	})
	if err != nil {
		log.Printf("[retrogaming-stream] la partie n'a pas démarré (%s) : %v", romPath, err)
		s.sendError(loadErrorMessage(err))
		return
	}

	info := eng.Info()
	s.mu.Lock()
	s.engine = eng
	// LA CADENCE DES IMAGES RENDUES, pas celle du cœur : c'est elle que
	// l'encodeur et la durée de chaque image doivent connaître (#332).
	s.fps = eng.OutputFPS()
	s.sampleRate = info.Timing.SampleRate
	s.mu.Unlock()
	// Une pause déjà demandée — un onglet caché avant le démarrage — vaut
	// pour la partie qui commence.
	s.applyPause()

	// Video encoder is created lazily on the first frame, once the core's
	// ACTUAL frame dimensions are known — GetSystemAVInfo's
	// Geometry.MaxWidth/MaxHeight is unreliable on some cores (e.g.
	// pcsx_rearmed reports 1024x512 but renders 640x480, see webrtcso
	// STATUS.md bug #13).
	s.audioEnc = encoder.NewAudioEncoder(int(s.sampleRate), 2)
	if err := s.audioEnc.Start(); err != nil {
		s.sendError(fmt.Sprintf("Audio encoder error: %v", err))
		return
	}

	if err := s.attachOwnerPeer(); err != nil {
		s.sendError(err.Error())
		return
	}

	startedOK = true

	go s.consumeVideoOutput()
	go s.consumeAudioOutput()
	eng.Run()

	// Persisted cheats — loaded once consoleID/reference/username are known
	// (set above) and applied straight to the just-started core. Stored into
	// s.cheats regardless of core support — sendOfferToBrowser reads it
	// unconditionally, and a core the frontend already hides the Cheats tab
	// for (cheatsSupported=false) simply never lets the player add more.
	// Best-effort and silent beyond the log: a missing/corrupt file here
	// should not stop a session from starting.
	if loaded, err := loadCheats(savesRoot, username, consoleID, reference); err != nil {
		log.Printf("[retrogaming-stream] cheats: load failed: %v", err)
	} else if len(loaded) > 0 {
		s.mu.Lock()
		s.cheats = loaded
		s.mu.Unlock()
		if err := eng.SetCheats(engineCheats(loaded)); err != nil {
			log.Printf("[retrogaming-stream] cheats: startup apply failed: %v", err)
		}
	}
}

// prepareGame — entre le chargement du cœur et celui du jeu, ce que seul le
// serveur sait faire : lire le jeu (ou le laisser sur le disque), déplier une
// archive, refaire une fiche .cue, choisir un BIOS. Appelé par le moteur, sur
// son fil, une fois le cœur chargé : c'est LUI qui dit comment on lui donne le
// jeu (need_fullpath).
//
// Rend des erreurs déjà rédigées pour la page — voir loadErrorMessage.
func (s *emulationSession) prepareGame(system libretro.SystemInfo, game *engine.Game, romPath, savesRoot, biosRoot, consoleID, coreName, bios, username string) error {
	isArchive := strings.EqualFold(filepath.Ext(romPath), ".zip")

	// `need_fullpath` EST UNE RÉPONSE, PAS UN DÉTAIL.
	//
	// Le cœur déclare s'il veut un CHEMIN (il ouvrira le fichier lui-même, à sa
	// façon — un DVD se lit secteur par secteur, pas d'un bloc) ou un TAMPON.
	// On passait les deux, systématiquement, avec ce raisonnement écrit dans le
	// code : « certains cœurs n'utilisent que l'un ou l'autre ». Vrai, et sans
	// conséquence tant que les jeux tiennent en quelques mégaoctets.
	//
	// Une PS2 change l'échelle : 4,1 Go lus dans la mémoire du conteneur, à
	// chaque lancement, pour un cœur qui les ignore. La spec libretro dit
	// l'inverse de ce qu'on faisait — quand `need_fullpath` est vrai, `data`
	// doit être nul.
	//
	// Une archive fait exception dans les deux sens : il FAUT la lire pour
	// l'ouvrir, et un cœur qui veut un chemin a besoin d'un vrai fichier —
	// resolveRomForCore en écrit un.
	loadPath := romPath
	var romData []byte
	if system.NeedFullpath && !isArchive {
		log.Printf("[retrogaming-stream] %s veut un chemin (need_fullpath) : le jeu n'est pas lu en mémoire",
			coreName)
	} else {
		raw, readErr := os.ReadFile(romPath)
		if readErr != nil {
			return fmt.Errorf("Failed to read ROM: %s (%v)", romPath, readErr)
		}
		var err error
		loadPath, romData, err = resolveRomForCore(romPath, raw)
		if err != nil {
			return fmt.Errorf("Failed to extract zipped ROM: %s (%v)", romPath, err)
		}
	}
	// UNE FICHE .CUE DONT LES PISTES N'EXISTENT QU'À LA CASSE PRÈS — un dump
	// Windows sur un disque Linux. Le cœur la lit refaite dans le dossier
	// temporaire du conteneur, à côté de liens aux noms exacts ; voir
	// cueWithExactNames. Une erreur ici n'arrête rien : la fiche part telle
	// quelle, et le cœur dira ce qui manque.
	if strings.EqualFold(filepath.Ext(loadPath), ".cue") {
		staged, cueErr := cueWithExactNames(loadPath, os.TempDir())
		if cueErr != nil {
			log.Printf("[retrogaming-stream] fiche %s : %v — passée telle quelle", loadPath, cueErr)
		} else if staged != loadPath {
			log.Printf("[retrogaming-stream] fiche %s : pistes à la casse près, refaite dans %s", filepath.Base(loadPath), filepath.Dir(staged))
			loadPath = staged
		}
	}
	if loadPath != romPath {
		s.tempRomPath = loadPath
	}

	log.Printf("[retrogaming-stream] console=%s core=%s rom=%s (%d octets en mémoire, need_fullpath=%v)",
		consoleID, coreName, romPath, len(romData), system.NeedFullpath)

	// Le jeu est PRÊT à être donné au cœur — lu en mémoire, ou laissé sur le
	// disque parce que le cœur veut un chemin. La distinction compte pour qui
	// attend : une image DVD de plusieurs gigaoctets ne passe pas par la RAM,
	// et l'étape est alors instantanée là où elle durerait sinon.
	if romData == nil {
		s.sendProgress("rom.ready", filepath.Base(romPath))
	} else {
		s.sendProgress("rom.ready", fmt.Sprintf("%s (%d)", filepath.Base(romPath), len(romData)))
	}

	log.Printf("[retrogaming-stream] core loaded: %s v%s", system.LibraryName, system.LibraryVersion)
	s.sendProgress("core.loaded", fmt.Sprintf("%s %s", system.LibraryName, system.LibraryVersion))

	// bios ("" unless the frontend's BIOS picker sent an explicit choice,
	// see admin/GameDetailPanel's own doc) resolves to either biosRoot
	// unchanged or a session-scoped temp dir holding just that one file —
	// see resolveBiosDir's own doc (bios.go) for why forcing just one file
	// into view is what actually makes a core use it.
	biosDir, biosTempDir, biosChoice, err := resolveBiosDir(biosRoot, savesRoot, consoleID, bios)
	if err != nil {
		return fmt.Errorf("Failed to resolve BIOS: %v", err)
	}
	// LE BIOS RÉELLEMENT RETENU, pas celui qu'on a demandé — c'est la valeur
	// que resolveBiosDir a choisie quand personne n'a choisi. Une console qui
	// démarre noire à cause d'un firmware absent se diagnostique ici, et nulle
	// part ailleurs à l'écran.
	s.sendProgress("bios.resolved", biosChoice)
	s.mu.Lock()
	s.biosDir = biosDir
	s.biosTempDir = biosTempDir
	// LE BIOS RÉELLEMENT DÉSIGNÉ, pas seulement celui qu'on a demandé : quand
	// personne n'a choisi, resolveBiosDir en nomme un pour les cœurs qui ne
	// choisissent pas eux-mêmes (pcsx2). C'est cette valeur que GET_VARIABLE
	// doit rendre, sinon le cœur voit les dumps et refuse quand même.
	s.biosFile = biosChoice
	s.mu.Unlock()

	game.Path = loadPath
	game.Data = romData
	game.SystemDir = biosDir
	// Scoped per-user — GET_SAVE_DIRECTORY fires during retro_init/
	// retro_load_game, and the username is only known once "start" arrives.
	game.SaveDir = coreSaveDir(savesRoot, username, coreName)
	game.Variables = serverVariables(consoleID, biosChoice)

	return nil
}

// serverVariables — ce que ce serveur impose à GET_VARIABLE, par console.
//
// pcsx2 choisit son BIOS par une option, pas en balayant le dossier : sans
// réponse, il énumère les dumps à retro_init (« Bios Found: Europe v02.20 »)
// et refuse quand même le jeu à retro_load_game, faute d'en avoir désigné un
// (voir biosFile). Le rendu forcé reste un bouton d'expérience, vide par
// défaut — voir pcsx2RendererExperiment.
func serverVariables(consoleID, biosFile string) map[string]string {
	if consoleID != "ps2" {
		return nil
	}
	variables := map[string]string{}
	if biosFile != "" {
		variables["pcsx2_bios"] = biosFile
	}
	if pcsx2RendererExperiment != "" {
		variables["pcsx2_renderer"] = pcsx2RendererExperiment
	}

	return variables
}

// loadErrorMessage rédige pour la page ce qui a empêché la partie de
// démarrer, dans les mots que le serveur employait déjà.
func loadErrorMessage(err error) string {
	var loadErr *engine.LoadError
	if !errors.As(err, &loadErr) {
		return fmt.Sprintf("Failed to load core: %v", err)
	}
	switch loadErr.Stage {
	case "core":
		return fmt.Sprintf("Failed to load core: %v", loadErr.Err)
	case "prepare":
		return loadErr.Err.Error()
	case "game":
		return fmt.Sprintf("Failed to load game: %v", loadErr.Err)
	default:
		return fmt.Sprintf("Failed to start hardware rendering: %v", loadErr.Err)
	}
}

// currentEngine — la partie en cours, ou nil.
func (s *emulationSession) currentEngine() *engine.Engine {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.engine
}

// applyPause dit au moteur ce que les deux pauses demandent ensemble : celle
// des onglets cachés (paused) et celle du joueur (heldPaused).
func (s *emulationSession) applyPause() {
	s.mu.Lock()
	paused := s.paused || s.heldPaused
	eng := s.engine
	s.mu.Unlock()
	if eng != nil {
		eng.SetPaused(paused)
	}
}

// setControllerType change la manette d'un port, par le moteur — jamais
// directement dans le cœur depuis la goroutine d'une connexion, qui croiserait
// un retro_run en vol.
func (s *emulationSession) setControllerType(port int, device uint32) {
	eng := s.currentEngine()
	if eng == nil {
		return
	}
	if err := eng.SetControllerType(port, device); err != nil {
		log.Printf("[retrogaming-stream] manette du port %d : %v", port, err)
	}
}

// sendRewindSupport dit au navigateur ce que le moteur vient de trancher
// (Config.OnRewindSupport). Le message "started" porte le même champ pour une reconnexion —
// à ce moment-là la réponse est déjà connue.
func (s *emulationSession) sendRewindSupport(supported bool) {
	s.sendMessage(streamMessage{Type: "rewindsupport", RewindSupported: supported})
}

// =========================================================================
// Ce que le moteur rend : les images, le son, la vibration
// =========================================================================

// startVideoEncoderOnce lazily creates the video encoder at the size of the
// FIRST frame actually emitted (software OR hardware-rendered), not
// GetSystemAVInfo's (sometimes wrong) reported geometry — see onFrame.
// Returns false if encoder startup failed
// (already logged), in which case the caller must not emit this frame.
func (s *emulationSession) startVideoEncoderOnce(w, h int) bool {
	s.mu.Lock()
	if s.videoEncStarted {
		s.mu.Unlock()
		return true
	}
	s.videoEncStarted = true
	consoleID := s.consoleID
	s.mu.Unlock()

	// LA RÉSOLUTION D'ÉPOQUE PLUTÔT QUE LA PREMIÈRE IMAGE.
	//
	// Le canevas se figeait sur la première image reçue. Sur une PlayStation
	// dont la scène d'ouverture est en 320×240, il restait à 320×240 pour
	// TOUTE la session : chaque scène 640×480 suivante était réduite, sans
	// retour possible (signalé le 2026-09-05).
	//
	// Une console rétro a une résolution connue d'avance — c'est celle de son
	// époque, et un cœur libretro n'émettra jamais de la haute définition. Le
	// registre partagé la porte (`consoles.NativeSize`), et c'est la même
	// valeur que le téléphone emploie pour dimensionner son tampon.
	//
	// PLANCHER, pas plafond : on ne prend la valeur déclarée que si elle est
	// PLUS GRANDE. Un cœur configuré pour rendre en interne à 2× (PPSSPP sait
	// le faire) émet de plus grandes images, et les réduire perdrait ce que
	// l'utilisateur vient de demander.
	//
	// Et on compare le nombre de PIXELS, pas chaque dimension séparément :
	// mélanger la largeur d'une source et la hauteur d'une autre déforme
	// l'image. Une géométrie vient d'un seul endroit, entière.
	//
	// Ce qui NE change pas : streamex et remoteplay. Leurs sources sont de
	// vraies sorties HD, et cette question s'y pose autrement — c'est un
	// autre chantier.
	// UN CHOIX DU JOUEUR PRIME SUR TOUT LE RESTE (#150).
	//
	// La résolution demandée avant le lancement arrive déjà en pixels
	// (« 640x480 ») : le kernel l'a traduite une seule fois, avec la même
	// table que streamex et remote-play. Elle devient le canevas, et le
	// redimensionneur d'emitFrame s'en charge image par image.
	//
	// Elle gagne MÊME quand elle réduit. La mise en garde du plancher
	// ci-dessous vise un défaut IMPLICITE qui écraserait un cœur rendant à
	// 2× ; ici c'est le joueur qui demande, et lui obéir n'est pas lui
	// reprendre son réglage.
	//
	// Ce que ça coûte, et ce n'est pas gratuit : agrandir 256×240 vers 1080p
	// se fait au plus proche voisin (engine.FitBGRA, au format du jeu, bandes noires autour), une image entière allouée à
	// chaque trame, sans ajouter un pixel de détail. Le défaut reste donc la
	// géométrie d'époque, et c'est le bon défaut.
	chosen := false
	if wantW, wantH, ok := chosenSize(); ok {
		log.Printf("[retrogaming-stream] canevas: %dx%d demandé au lancement (première image %dx%d)",
			wantW, wantH, w, h)
		w, h = wantW, wantH
		chosen = true
	} else if raw := strings.TrimSpace(os.Getenv("GAMEFLEX_RESOLUTION")); raw != "" {
		log.Printf("[retrogaming-stream] canevas: GAMEFLEX_RESOLUTION=%q illisible, ignorée", raw)
	}

	if nativeW, nativeH := consoles.NativeSize(consoleID); !chosen && nativeW*nativeH > w*h {
		log.Printf("[retrogaming-stream] canevas: %dx%d déclaré pour %s (première image %dx%d)",
			nativeW, nativeH, consoleID, w, h)
		w, h = nativeW, nativeH
	}

	// 0 = AUTOMATIQUE : le débit se déduit de la taille et de la cadence
	// (voir encoder.AutoBitrate). La valeur fixe de 2000 ne pouvait être
	// juste que pour une seule taille d'image, et ce n'était pas celle des
	// consoles qui en demandent le plus. Un débit choisi dans le panneau des
	// réglages de flux passe avant (#332) : jouer en ligne dépend de la
	// connexion de chacun.
	bitrate := launchSetting("GAMEFLEX_BITRATE")
	if bitrate <= 0 {
		bitrate = encoder.AutoBitrate(w, h, s.fps)
	}
	log.Printf("[retrogaming-stream] creating encoder: %dx%d@%.2f bitrate=%dk", w, h, s.fps, bitrate)
	s.videoEnc = encoder.NewVideoEncoder(w, h, s.fps, bitrate)
	if err := s.videoEnc.Start(); err != nil {
		log.Printf("[retrogaming-stream] FATAL: video encoder start failed: %v", err)
		return false
	}
	s.encW, s.encH = w, h
	close(s.encoderReady)
	return true
}

// chosenSize — la résolution du flux choisie avant le lancement, que le kernel
// pose déjà en pixels (« 960x540 ») dans GAMEFLEX_RESOLUTION. `false` quand
// elle est absente ou illisible.
func chosenSize() (int, int, bool) {
	raw := strings.TrimSpace(os.Getenv("GAMEFLEX_RESOLUTION"))
	if raw == "" {
		return 0, 0, false
	}
	var width, height int
	if _, err := fmt.Sscanf(raw, "%dx%d", &width, &height); err != nil || width < 160 || height < 120 {
		return 0, 0, false
	}

	return width, height, true
}

// chosenHeight — la hauteur de chosenSize, 0 sans choix.
func chosenHeight() int {
	_, height, _ := chosenSize()

	return height
}

// launchSetting lit un réglage de flux posé par le kernel sur le conteneur
// (GAMEFLEX_FPS, GAMEFLEX_BITRATE), déjà validé là-bas : 0 quand il est absent
// ou illisible, c'est-à-dire « le défaut ».
func launchSetting(name string) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value < 0 {
		return 0
	}

	return value
}

// onFrame — une image neuve du moteur (Config.OnFrame), déjà en BGRA : l'encodeur
// naît à la taille de la première, puis chaque image part vers son canevas.
func (s *emulationSession) onFrame(frame engine.Frame) {
	if !s.startVideoEncoderOnce(frame.Width, frame.Height) {
		return
	}
	s.emitFrame(frame.Pixels, frame.Width, frame.Height, frame.Aspect)
}

// emitFrame pads/scales to the encoder's fixed canvas if the core's
// resolution changed mid-game (e.g. PS1 640x480 <-> 320x240 scene changes,
// see webrtcso STATUS.md bug #15), then hands off to frameReady —
// non-blocking: if the encoder is falling behind, the newest frame is
// dropped rather than queued, so real-time playback never grows a backlog.
func (s *emulationSession) emitFrame(frame []byte, w, h int, aspect float64) {
	s.mu.Lock()
	ew, eh := s.encW, s.encH
	s.mu.Unlock()

	// LES DIMENSIONS SUIVENT LE TAMPON, elles ne le précèdent pas.
	//
	// La frame est redimensionnée vers le canevas de l'encodeur quand le cœur
	// ne rend pas exactement à cette taille — et on rangeait ensuite le tampon
	// REDIMENSIONNÉ avec les ANCIENNES dimensions. La vignette de sauvegarde
	// relisait donc `ew*eh*4` octets en croyant lire une image `w x h` : une
	// image brouillée, décalée d'une ligne de plus à chaque rangée.
	//
	// Invisible tant que les deux tailles coïncident, ce qui est le cas de la
	// plupart des consoles rétro. Une PS2 rend en 640x448 dans un canevas de
	// 512x512 : elles ne coïncident jamais, et la vignette était fausse à tous
	// les coups (signalé le 2026-09-08).
	frameW, frameH := w, h
	if ew > 0 && eh > 0 && (w != ew || h != eh) {
		frame = engine.FitBGRA(frame, w, h, ew, eh, aspect)
		frameW, frameH = ew, eh
	}

	// Stashed for handleSaveState's screenshot (see captureScreenshot) —
	// the exact same BGRA bytes handed to the encoder, so the saved
	// thumbnail matches what viewers actually see, not a separately-decoded
	// copy. Cheap: a slice header + dimensions, not a copy — safe because
	// the engine allocates a FRESH frame per image (ToBGRA, ReadPixels)
	// rather than reusing/mutating a shared buffer.
	s.mu.Lock()
	s.lastFrame, s.lastFrameW, s.lastFrameH = frame, frameW, frameH
	s.mu.Unlock()

	select {
	case s.frameReady <- frame:
	default:
	}
}

// reportAudioDrops dit, au plus une fois toutes les cinq secondes, combien de
// lots de son ont été jetés — et rien du tout quand il n'y en a pas eu.
//
// La première valeur est posée ICI plutôt qu'à la construction de la session :
// un compteur qui dépend d'une initialisation faite ailleurs finit par tomber
// sur un zéro qu'il ne sait pas lire.
//
// LE PREMIER LOT JETÉ SE DIT AUSSITÔT. Il ne faisait que poser l'horloge : une
// rafale de pertes plus courte que cinq secondes, puis plus rien, ne se disait
// jamais — et l'arrêt de la partie emportait le compte (relevé le 2026-10-11,
// en cherchant ces pertes dans 215 journaux de session sans en trouver une
// seule ligne). Ce qui reste au compte se dit à l'arrêt (flushAudioDrops).
func (s *emulationSession) reportAudioDrops() {
	now := time.Now()
	previous, ok := s.audioDropsSince.Load().(time.Time)
	if !ok {
		s.audioDropsSince.Store(now)
		if dropped := s.audioDrops.Swap(0); dropped > 0 {
			log.Printf("[retrogaming-stream] son : premier lot jeté (%d) — l'encodeur audio ne suit pas", dropped)
		}

		return
	}
	if now.Sub(previous) < 5*time.Second {
		return
	}
	s.audioDropsSince.Store(now)

	if dropped := s.audioDrops.Swap(0); dropped > 0 {
		log.Printf("[retrogaming-stream] son : %d lots jetés en 5 s (~%d ms de silence) — l'encodeur audio ne suit pas",
			dropped, dropped*10)
	}
}

// flushAudioDrops — ce qui restait au compte quand la partie s'arrête.
func (s *emulationSession) flushAudioDrops() {
	if dropped := s.audioDrops.Swap(0); dropped > 0 {
		log.Printf("[retrogaming-stream] son : %d lots jetés depuis le dernier relevé (~%d ms de silence)",
			dropped, dropped*10)
	}
}

// onAudio — dix millisecondes de son du moteur (Config.OnAudio), vers le seul
// consommateur qui les écrit dans ffmpeg.
//
// Non-blocking, drop-rather-than-queue-a-backlog — same policy as emitFrame
// (video). consumeAudioOutput's inner goroutine is the single sequential
// consumer of this channel; if it's still busy writing a previous batch to
// ffmpeg, this batch is dropped (a small audio gap/click) instead of
// accumulating as yet another pending write behind it — the mechanism that
// used to make audio drift behind video over a session.
func (s *emulationSession) onAudio(chunk []int16) {
	select {
	case s.audioBatchReady <- chunk:
	default:
		// ON COMPTE CE QU'ON JETTE. Le rejet est délibéré et coûte un trou
		// dans le son ; tant qu'il reste rare, c'est le bon arbitrage face à
		// une dérive qui, elle, ne se rattrape jamais. Une ligne toutes les
		// cinq secondes, et seulement s'il y a eu des pertes.
		s.audioDrops.Add(1)
		s.reportAudioDrops()
	}
}

// onRumble — les deux moteurs d'une manette, déjà combinés par le moteur
// (Config.OnRumble), envoyés à la connexion qui tient ce port.
func (s *emulationSession) onRumble(port int, weak, strong uint16) {
	rumble := streamMessage{Type: "rumble", RumbleWeak: weak, RumbleStrong: strong}
	// PENDANT UN PRÊT, la vibration va aux mains qui tiennent la manette :
	// l'emprunteur, sur sa propre connexion de spectateur — pas le
	// propriétaire, qui regarde (#274).
	if port == 0 {
		if borrower := v4Borrower(); borrower != "" {
			sent := s.sendToUsername(borrower, rumble)
			log.Printf("[retrogaming-stream] rumble: port=0 lent to %q weak=%d strong=%d sent=%v", borrower, weak, strong, sent)

			return
		}
	}

	sent := s.sendToPort(uint32(port), rumble)
	log.Printf("[retrogaming-stream] rumble: port=%d weak=%d strong=%d sent=%v", port, weak, strong, sent)
}

// onStall — le cœur ne rend plus d'image depuis quinze secondes (moteur,
// Config.OnStall). La partie s'arrête, plutôt que de laisser une image figée
// sans explication : ce processus n'existe que pour elle.
func (s *emulationSession) onStall(stalled time.Duration) {
	log.Printf("[retrogaming-stream] LE CŒUR NE RÉPOND PLUS : aucune image depuis %s — "+
		"la partie s'arrête, plutôt que de laisser une image figée sans explication",
		stalled.Round(time.Second))

	os.Exit(1)
}

// handleInput applies input state pushed by the browser over the WebRTC
// DataChannel to controller port `player` (0 for the session owner,
// 1..maxPlayers-1 for a joined player — fixed once per connection at the call
// site, never carried in the JSON itself). The batch's shape and meaning are
// the engine's (inputs.apply) — the same on the phone.
func (s *emulationSession) handleInput(player int, data []byte) {
	eng := s.currentEngine()
	if eng == nil {
		return
	}
	if err := eng.Input(player, data); err != nil {
		log.Printf("[retrogaming-stream] input: %v", err)
	}
}

// =========================================================================
// Encoder output consumers (read from encoder, push to gatewaywebrtc)
// =========================================================================

func (s *emulationSession) consumeVideoOutput() {
	go func() {
		fc := 0
		for {
			select {
			case <-s.stopping:
				log.Printf("[retrogaming-stream] video-feed stopped after %d frames", fc)
				return
			case frame := <-s.frameReady:
				if s.videoEnc != nil {
					if err := s.videoEnc.EncodeFrame(frame); err != nil {
						log.Printf("[retrogaming-stream] video-feed error: %v", err)
					}
					fc++
				}
			}
		}
	}()

	select {
	case <-s.stopping:
		return
	case <-s.encoderReady:
	}

	frameDur := time.Duration(float64(time.Second) / s.fps)

	for {
		select {
		case <-s.stopping:
			return
		case nal := <-s.videoEnc.Output:
			dur := time.Duration(0)
			if encoder.IsVCLNal(nal) {
				dur = frameDur
			}
			// ONE push to the shared transport — gatewaywebrtc fans the same
			// NAL out to every viewer, owner, joiners and spectators alike.
			if s.gatewayClient != nil {
				_ = s.gatewayClient.PushVideo(nal, dur)
			}
		}
	}
}

// IsVCLNal moved to internal/retrogaming/encoder (encoder.IsVCLNal) — both
// the libretro session and the external runtime need it, and neither can
// import the other. See consumeVideoOutput's call site.

func (s *emulationSession) consumeAudioOutput() {
	// Single sequential consumer of audioBatchReady (see onAudio)
	// — same shape as consumeVideoOutput's own inner goroutine for
	// frameReady. Processing batches one at a time, in the order they were
	// produced, also fixes a latent ordering bug the old per-batch-goroutine
	// approach had: EncodeSamples was mutex-protected against concurrent
	// writes corrupting each other, but nothing guaranteed two goroutines
	// acquired that mutex in the same order their batches were produced.
	go func() {
		for {
			select {
			case <-s.stopping:
				return
			case buf := <-s.audioBatchReady:
				if s.audioEnc != nil {
					s.audioEnc.EncodeSamples(buf)
				}
			}
		}
	}()

	const packetDuration = 10 * time.Millisecond // must match audioBufMax / -frame_duration
	for {
		select {
		case <-s.stopping:
			return
		case packet := <-s.audioEnc.Output:
			// Same single push as the video branch above.
			if s.gatewayClient != nil {
				_ = s.gatewayClient.PushAudio(packet, packetDuration)
			}
		}
	}
}

// =========================================================================
// WebSocket messaging + signaling + shutdown
// =========================================================================

func (s *emulationSession) sendMessage(msg streamMessage) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	data, _ := json.Marshal(msg)
	s.ws.WriteMessage(websocket.TextMessage, data)
}

// isCurrentOwnerWS reports whether this connection is the one the session
// currently treats as its owner — a takeover-kicked old connection is not
// (see serveEmulationSession's takeover branch).
func (s *emulationSession) isCurrentOwnerWS(ws *websocket.Conn) bool {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return s.ws == ws
}

type broadcastTarget struct {
	username string
	send     func(streamMessage)
	// visible — this connection's own tab visibility, self-reported via the
	// "visibility" WS message (see updateVisibility/recomputePauseState
	// below). Defaults to true at registration: a connection that never
	// sends one (an older client, or one that simply hasn't backgrounded
	// yet) is assumed to be actively watched, matching the previous
	// (always-running) behavior.
	visible bool
}

// registerBroadcastTarget/unregisterBroadcastTarget/broadcast/sendToUsername
// let any connected browser connection (owner, joiners, AND spectators —
// each served by its own goroutine with its own *websocket.Conn) reach any
// other. Two uses: broadcast tells every PLAYER when someone else's
// connection comes or goes (a toast: "Player X left — the controller
// froze, ask someone to take over or keep playing without them" — rather
// than a character just silently stopping mid-screen with no explanation);
// sendToUsername is the point-to-point primitive the take-a-slot request
// flow (requestSlot/handleCedeSlot/handleSlotDenied) uses to reach one
// specific person, player or spectator, by their own self-reported
// username (same trust model as everywhere else in this file — see
// GAMEFLEX_JOURNAL.md).
func (s *emulationSession) registerBroadcastTarget(key, username string, send func(streamMessage)) {
	s.broadcastTargetsMu.Lock()
	if s.broadcastTargets == nil {
		s.broadcastTargets = make(map[string]*broadcastTarget)
	}
	s.broadcastTargets[key] = &broadcastTarget{username: username, send: send, visible: true}
	s.broadcastTargetsMu.Unlock()
	// A brand-new connection is assumed visible — if the session was paused
	// because everyone else was backgrounded, this one joining/reconnecting
	// resumes it immediately, same spirit as the controller-reconnect
	// best-effort signal elsewhere in this file.
	s.recomputePauseState()
}

func (s *emulationSession) unregisterBroadcastTarget(key string) {
	s.broadcastTargetsMu.Lock()
	delete(s.broadcastTargets, key)
	s.broadcastTargetsMu.Unlock()
	s.recomputePauseState()
}

// updateVisibility records one connection's own self-reported tab
// visibility (see the "visibility" WS message, handled in this file's and
// child.go's read loops) and recomputes whether the whole session should be
// paused.
func (s *emulationSession) updateVisibility(key string, visible bool) {
	s.broadcastTargetsMu.Lock()
	if t, ok := s.broadcastTargets[key]; ok {
		t.visible = visible
	}
	s.broadcastTargetsMu.Unlock()
	s.recomputePauseState()
}

// recomputePauseState pauses the engine (applyPause) once NONE of
// the currently-connected browser tabs — owner, joiners, AND spectators,
// deliberately all three: a session with only watchers left shouldn't keep
// burning CPU/encoding either — report themselves visible, and resumes it
// the moment any one of them does. A session with zero connections at all
// is left unpaused (meaningless either way — the queue ends a game nobody
// plays, on its own idle delay).
func (s *emulationSession) recomputePauseState() {
	s.broadcastTargetsMu.Lock()
	total := len(s.broadcastTargets)
	anyVisible := false
	for _, t := range s.broadcastTargets {
		if t.visible {
			anyVisible = true
			break
		}
	}
	s.broadcastTargetsMu.Unlock()

	paused := total > 0 && !anyVisible
	s.mu.Lock()
	changed := s.paused != paused
	s.paused = paused
	s.mu.Unlock()
	s.applyPause()
	if changed {
		log.Printf("[retrogaming-stream] session %s/%s pause state changed: paused=%v", s.consoleID, s.reference, paused)
	}
}

// updateBroadcastTargetUsername patches an already-registered target's
// username in place — needed for "owner", whose connection is registered
// (serveEmulationSession) before its username is actually known (only the
// "start" message, handled by handleStart, carries it).
func (s *emulationSession) updateBroadcastTargetUsername(key, username string) {
	s.broadcastTargetsMu.Lock()
	defer s.broadcastTargetsMu.Unlock()
	if t, ok := s.broadcastTargets[key]; ok {
		t.username = username
	}
}

// broadcast reaches PLAYERS only (owner + joiners) — spectators have no
// agency over the game and aren't who "playerleft"/"playerjoined" is
// actually useful to.
func (s *emulationSession) broadcast(msg streamMessage) {
	s.broadcastTargetsMu.Lock()
	defer s.broadcastTargetsMu.Unlock()
	for key, t := range s.broadcastTargets {
		if strings.HasPrefix(key, "spectator-") {
			continue
		}
		t.send(msg)
	}
}

// sendToUsername reaches ANYONE currently connected (player or spectator)
// by username — if more than one connection self-reports the same
// username (e.g. a spectator with two tabs open), an arbitrary one of them
// gets it, which is an acceptable rough edge for a best-effort notice.
func (s *emulationSession) sendToUsername(username string, msg streamMessage) bool {
	s.broadcastTargetsMu.Lock()
	defer s.broadcastTargetsMu.Unlock()
	for _, t := range s.broadcastTargets {
		if t.username == username {
			t.send(msg)
			return true
		}
	}
	return false
}

// sendToPort routes a message to whichever connection currently occupies
// controller port N (0 = owner, 1..maxPlayers-1 = a joiner, see joiners'
// own doc) — same broadcastTargets registry sendToUsername uses above, but
// keyed directly by the "owner"/"joiner-N" registration key instead of a
// self-reported username: the owner registers with an EMPTY username (see
// registerBroadcastTarget("owner", "", ...) in serveEmulationSession), so
// username-based lookup can't reliably reach it.
func (s *emulationSession) sendToPort(port uint32, msg streamMessage) bool {
	key := "owner"
	if port > 0 {
		key = fmt.Sprintf("joiner-%d", port)
	}
	s.broadcastTargetsMu.Lock()
	defer s.broadcastTargetsMu.Unlock()
	t, ok := s.broadcastTargets[key]
	if !ok {
		return false
	}
	t.send(msg)
	return true
}

func (s *emulationSession) sendError(errStr string) {
	log.Printf("[retrogaming-stream] error: %s", errStr)
	s.sendMessage(streamMessage{Type: "error", Error: errStr})
}

// sendProgress annonce un jalon du démarrage (#191).
//
// Le frère de sendError, et pour la même raison : ces étapes n'existaient que
// dans les journaux de ce conteneur — le seul endroit où celui qui attend ne
// peut pas regarder. Le relais du kernel les fait suivre sans rien savoir
// d'elles : sa pompe écrit TOUT ce que cet enfant envoie (retro_relay.go), et
// son rappel ne fait qu'inspecter deux types au passage.
//
// Un jalon n'est jamais un échec : il n'interrompt rien, et une page qui
// l'ignore se comporte exactement comme avant.
func (s *emulationSession) sendProgress(step, detail string) {
	s.sendMessage(streamMessage{Type: "progress", Step: step, Detail: detail})
}

// sendGatewayStarted acks the owner's start — the browser's own viewer-offer
// to gatewaywebrtc already negotiated the media. SessionID here (owner path
// only — see child.go's childSessionID doc) is how the frontend learns its
// own Go session id, to report as part of its presence and to build a
// join-invitation deep link (Phase 5).
//
// Cheats travel on THIS message rather than the "cheats" one handleSetCheats
// sends: that one only resolves an explicit in-flight setcheats request (see
// the frontend's pendingCheatsCallbackRef), so an unsolicited copy sent
// before the panel ever opens would arrive with nobody listening. "started"
// is the one message every owner connection already reads at startup, so
// CheatsPanel seeds its list from here.
func (s *emulationSession) sendGatewayStarted() {
	s.mu.Lock()
	cheats := s.cheats
	eng := s.engine
	s.mu.Unlock()
	rewindSupported := eng != nil && eng.RewindSupported()
	s.sendMessage(streamMessage{Type: "started", SessionID: childSessionID, CheatsSupported: libretro.CheatSupported(), Cheats: cheats, RewindSupported: rewindSupported, FastForwardSupported: true})
}

// handleGatewayInput routes one gateway DataChannel input message to the
// controller port its viewerId is mapped to (see viewerPorts).
func (s *emulationSession) handleGatewayInput(viewerID string, data []byte) {
	// chantier-4: the QUEUE decides — and it decides for everything,
	// including refusing. Falling through to the legacy map for a viewer v4
	// knows is watching would let a watcher play.
	//
	// AND FOR A VIEWER IT DOES NOT KNOW AT ALL. The legacy map is filled by
	// the relay's `start`, which the kernel let any signed-in account send on
	// a session its child owns: open a gateway viewer, send `start` with it,
	// and that unannounced viewer drove pad 1 through the fallback below
	// (found 2026-10-08). With a v4 host, a viewer the queue never heard of
	// is refused — the page always announces its viewer on its v4 socket.
	if port, managed := v4PortForViewer(viewerID); managed || v4 != nil {
		if port < 0 {
			return
		}
		s.handleInput(port, data)

		return
	}

	s.mu.Lock()
	port, ok := s.viewerPorts[viewerID]
	s.mu.Unlock()
	if !ok {
		return
	}
	s.handleInput(port, data)
}

// handleGatewayViewerLeft handles a browser viewer detaching from
// gatewaywebrtc. The owner's viewer is treated exactly like the owner's WS
// closing (same grace/handoff path); a joiner's viewer releases its port
// and broadcasts playerleft, mirroring the joiner-WS disconnect cleanup in
// child.go's serveJoinerSession.
func (s *emulationSession) handleGatewayViewerLeft(viewerID string) {
	s.mu.Lock()
	if viewerID == s.ownerViewerID {
		s.mu.Unlock()
		log.Printf("[retrogaming-stream] gateway: owner viewer left (%s)", viewerID)
		s.ownerDisconnected()
		return
	}
	port, _ := s.viewerPorts[viewerID]
	delete(s.viewerPorts, viewerID)
	for i, joiner := range s.joiners {
		if joiner != nil && joiner.ViewerID == viewerID {
			port = i + 1
		}
	}
	var leftUsername string
	if port > 0 && port <= len(s.joiners) {
		if joiner := s.joiners[port-1]; joiner != nil {
			leftUsername = joiner.username
			s.joiners[port-1] = nil
			s.clearPortLocked(port)
		}
	}
	s.mu.Unlock()

	if port <= 0 {
		return // unknown viewer (spectator) — nothing to release
	}
	s.setControllerType(port, engine.DeviceNone)
	s.broadcast(streamMessage{Type: "playerleft", Username: leftUsername, Port: port})
	log.Printf("[retrogaming-stream] gateway: joiner viewer left (port %d)", port)
}

// maxSaveSlots/autoSaveSlot — 10 slots per (user, console, game), numbered
// 0-9. Slot 0 is still CALLED autoSaveSlot, but nothing writes it
// automatically any more: the on-exit save was removed on 2026-08-24 (see the
// teardown path), and what writes slot 0 today is the player's own explicit
// gesture — "save on quit", an ordinary "savestate" message. The name stays
// because it still says what the slot is FOR: where I left off.
//
// Only the slot picker keeps slot 0 read-only on screen; the server accepts
// writes to it (see handleSaveState). This comment claimed the opposite until
// 2026-09-18, and cited an `autoSaveOnExit` that exists in no code at all.
const (
	maxSaveSlots = 10
	autoSaveSlot = 0
)

// stateFilePath returns where a save-state-related file lives —
// {savesRoot}/{username}/states/{consoleID}/{reference}_slot{N}{ext} —
// shared by saveStatePath/screenshotPath below (ext ".state"/".png") AND by
// the standalone HTTP routes (handlers.go's SaveStateScreenshot/
// ListSaveSlots), which have no live emulationSession to read
// consoleID/reference/username off of, just the request's query params.
// reference can contain characters unsafe for a single filename (ROM folder
// names, see resolveGameFile) — replaced rather than rejected, since this is
// derived from names the library scan already produced (see catalog.go).
func stateFilePath(savesRoot, username, consoleID, reference string, slot int, ext string) string {
	safeName := strings.NewReplacer("/", "_", "\\", "_").Replace(reference)
	return filepath.Join(savesRoot, username, "states", consoleID, fmt.Sprintf("%s_slot%d%s", safeName, slot, ext))
}

// cheatsDir is where one game's cheats live —
// {savesRoot}/{username}/cheats/{reference} — a directory of small .txt
// files, one per cheat, rather than a single JSON blob: a cheat can be
// dropped in or hand-edited outside the app too, same spirit as a RetroArch
// .cht folder. Keyed on reference alone (not consoleID+reference like save
// states) — reference is already the catalog's own unique id per game (see
// catalog.go's deriveRomID doc), and a flat "one folder per game" layout is
// what a player poking around the volume actually expects.
func cheatsDir(savesRoot, username, reference string) string {
	safeName := strings.NewReplacer("/", "_", "\\", "_").Replace(reference)
	return filepath.Join(savesRoot, username, "cheats", safeName)
}

// cheatFileName turns a cheat's description into a filesystem-safe .txt
// name — same character substitution as stateFilePath's reference handling.
// Falls back to a positional name for a cheat with no description (the
// panel's Name field is optional, see CheatsPanel), since every cheat needs
// SOME filename and two blank-named cheats can't share one file.
func cheatFileName(description string, index int) string {
	name := strings.TrimSpace(description)
	if name == "" {
		return fmt.Sprintf("cheat-%d.txt", index+1)
	}
	return strings.NewReplacer("/", "_", "\\", "_").Replace(name) + ".txt"
}

// loadCheats reads every .txt file in cheatsDir back into a []Cheat — the
// exact inverse of saveCheatsToDisk. A missing directory (no cheats ever
// saved for this game) is not an error, it's just an empty list, same
// contract as ListSaveSlots for a game with no save states yet.
func loadCheats(savesRoot, username, consoleID, reference string) ([]Cheat, error) {
	_ = consoleID // kept in the signature to mirror stateFilePath's callers; cheatsDir itself doesn't need it, see its own doc
	dir := cheatsDir(savesRoot, username, reference)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cheats []Cheat
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			log.Printf("[retrogaming-stream] cheats: skipping %s: %v", entry.Name(), err)
			continue
		}
		// First line is "enabled=true"/"enabled=false", the rest is the
		// code verbatim (multi-line codes are common, e.g. Game Genie).
		lines := strings.SplitN(string(raw), "\n", 2)
		enabled := strings.TrimSpace(lines[0]) == "enabled=true"
		code := ""
		if len(lines) > 1 {
			code = strings.TrimRight(lines[1], "\n")
		}
		if code == "" {
			continue
		}
		cheats = append(cheats, Cheat{
			Code:        code,
			Enabled:     enabled,
			Description: strings.TrimSuffix(entry.Name(), ".txt"),
		})
	}
	return cheats, nil
}

// saveCheatsToDisk replaces the ENTIRE on-disk cheat set for one game with
// the given list — matches handleSetCheats' own "whole list every time"
// contract (see its doc), so there is no separate add/remove path to keep
// in sync here. The directory is wiped first: a cheat removed from the
// panel must not survive as an orphan file the next time this game loads.
func saveCheatsToDisk(savesRoot, username, consoleID, reference string, cheats []Cheat) error {
	_ = consoleID
	dir := cheatsDir(savesRoot, username, reference)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(cheats) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, cheat := range cheats {
		content := fmt.Sprintf("enabled=%t\n%s", cheat.Enabled, cheat.Code)
		path := filepath.Join(dir, cheatFileName(cheat.Description, i))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *emulationSession) saveStatePath(savesRoot string, slot int) string {
	s.mu.Lock()
	consoleID, reference, username := s.consoleID, s.reference, s.username
	s.mu.Unlock()
	return stateFilePath(savesRoot, username, consoleID, reference, slot, ".state")
}

// screenshotPath is saveStatePath's sibling — see captureScreenshot.
func (s *emulationSession) screenshotPath(savesRoot string, slot int) string {
	s.mu.Lock()
	consoleID, reference, username := s.consoleID, s.reference, s.username
	s.mu.Unlock()
	return stateFilePath(savesRoot, username, consoleID, reference, slot, ".png")
}

// captureScreenshot encodes the most recently emitted frame (see emitFrame)
// as a PNG next to the .state file — best-effort: a failure here (no frame
// captured yet, e.g. a save attempted before the very first frame rendered)
// only logs, it never fails the save-state operation itself, which already
// succeeded by the time this runs.
func (s *emulationSession) captureScreenshot(savesRoot string, slot int) {
	s.mu.Lock()
	frame, w, h := s.lastFrame, s.lastFrameW, s.lastFrameH
	s.mu.Unlock()
	if frame == nil || w == 0 || h == 0 {
		log.Printf("[retrogaming-stream] screenshot skipped: no frame captured yet")
		return
	}

	// frame is BGRA (engine.Frame) — image.RGBA wants R,G,B,A
	// per pixel, so swap the B/R bytes on this copy rather than the shared
	// one still possibly in flight to the video encoder.
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, frame)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
	}

	path := s.screenshotPath(savesRoot, slot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[retrogaming-stream] screenshot skipped: %v", err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("[retrogaming-stream] screenshot skipped: %v", err)
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Printf("[retrogaming-stream] screenshot encode failed: %v", err)
		return
	}
	log.Printf("[retrogaming-stream] screenshot saved: %s", path)
}

// handleSaveState/handleLoadState are solo-only — see activePlayerCount's
// doc for why: a snapshot/restore pauses and rewinds time for the whole
// session, which isn't something one connected player should be able to do
// to another's still-in-progress game without consent.
//
// slot is 0-9 on BOTH sides. Slot 0 stopped being reserved when the automatic
// exit save was removed (see maxSaveSlots/autoSaveSlot) — and the inline
// comment four lines below said so already, while this one still announced
// « 1-9 for an explicit save ». Two comments of the same function disagreed;
// the one that matched the code won (2026-09-18).
func (s *emulationSession) handleSaveState(savesRoot string, slot int) {
	// Slot 0 included: it used to be reserved for the automatic exit save,
	// which no longer exists — the player's own "save on quit" writes there
	// now, so it keeps meaning "where I left off".
	if slot < 0 || slot >= maxSaveSlots {
		s.sendError(fmt.Sprintf("Invalid save slot: %d", slot))
		return
	}
	if n := s.playersInGame(); n > 1 {
		s.sendError("Save state is only available in solo play")
		return
	}
	eng := s.currentEngine()
	if eng == nil {
		s.sendError("Save state failed: no running emulation session")
		return
	}
	data, err := eng.SaveState()
	if err != nil {
		s.sendError(fmt.Sprintf("Save state failed: %v", err))
		return
	}

	path := s.saveStatePath(savesRoot, slot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.sendError(fmt.Sprintf("Save state failed: %v", err))
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.sendError(fmt.Sprintf("Save state failed: %v", err))
		return
	}
	log.Printf("[retrogaming-stream] state saved: %s (%d bytes)", path, len(data))
	s.captureScreenshot(savesRoot, slot)
	s.sendMessage(streamMessage{Type: "savestate", Info: "saved", Slot: slot})
}

func (s *emulationSession) handleLoadState(savesRoot string, slot int) {
	if slot < 0 || slot >= maxSaveSlots {
		s.sendError(fmt.Sprintf("Invalid save slot: %d", slot))
		return
	}
	if n := s.playersInGame(); n > 1 {
		s.sendError("Load state is only available in solo play")
		return
	}
	path := s.saveStatePath(savesRoot, slot)
	data, err := os.ReadFile(path)
	if err != nil {
		s.sendError(fmt.Sprintf("No save state found in slot %d: %v", slot, err))
		return
	}

	eng := s.currentEngine()
	if eng == nil {
		s.sendError("Load state failed: no running emulation session")
		return
	}
	if err := eng.LoadState(data); err != nil {
		var refused *engine.StateRefusedError
		if errors.As(err, &refused) {
			log.Printf("[retrogaming-stream] state refused: %s (%v)", path, refused)
			s.sendMessage(streamMessage{
				Type: "error", Error: refused.Error(),
				Code: "state-refused", Core: refused.Core, StateCore: refused.StateCore,
			})
			return
		}
		s.sendError(fmt.Sprintf("Load state failed: %v", err))
		return
	}
	log.Printf("[retrogaming-stream] state loaded: %s", path)
	s.sendMessage(streamMessage{Type: "loadstate", Info: "loaded", Slot: slot})
}

// handleSetCheats applies a full cheat list — same solo-only gate as save/
// load state (a cheat affects the whole session's game state, not just the
// requester's) and same owner-only reach (no "setcheats" case exists on the
// joiner read loop, child.go). Best-effort: a core with no retro_cheat_set
// (CheatSupported() false) just silently no-ops per-cheat inside the core
// itself — reported upfront via the "started" message's CheatsSupported flag
// (sendGatewayStarted) so the frontend can hide the whole tab instead of
// letting someone type codes that visibly do nothing.
func (s *emulationSession) handleSetCheats(cheats []Cheat) {
	if n := s.playersInGame(); n > 1 {
		s.sendError("Cheats are only available in solo play")
		return
	}

	eng := s.currentEngine()
	if eng == nil {
		s.sendError("Set cheats failed: no running emulation session")
		return
	}
	if err := eng.SetCheats(engineCheats(cheats)); err != nil {
		s.sendError(fmt.Sprintf("Set cheats failed: %v", err))
		return
	}

	s.mu.Lock()
	s.cheats = cheats
	consoleID, reference, username, savesRoot := s.consoleID, s.reference, s.username, s.savesRoot
	s.mu.Unlock()
	log.Printf("[retrogaming-stream] cheats applied: %d entries", len(cheats))
	s.sendMessage(streamMessage{Type: "cheats", Cheats: cheats})

	// Persisted the same "whole list every time" way it arrived — see
	// saveCheatsToDisk's own doc. Best-effort: a write failure here doesn't
	// undo the cheats already applied to the running core above, it only
	// means they won't survive to the next session.
	if err := saveCheatsToDisk(savesRoot, username, consoleID, reference, cheats); err != nil {
		log.Printf("[retrogaming-stream] cheats: save failed: %v", err)
	}
}

// handleFastForward toggles fast-forward (four frames per tick, only the last
// one shown, sound muted — see the engine) and echoes back the state applied.
// The engine reads the flag on its own next tick: no core call happens here.
func (s *emulationSession) handleFastForward(enabled bool) {
	eng := s.currentEngine()
	if eng == nil {
		s.sendError("No emulation session running")
		return
	}
	// Exclusive du rembobinage : le moteur arrête l'un quand l'autre part.
	eng.SetFastForward(enabled)
	s.sendMessage(streamMessage{Type: "fastforward", Enabled: enabled})
}

// handleReset — le bouton RESET de la console (« reset », chantier 13) : le
// jeu repart de son démarrage, comme sur la machine, ce qui n'a pas été
// sauvegardé est perdu. Par le moteur, entre deux images. Un échec se dit
// dans le journal, pas sur le canal d'erreur de la page : celui-ci est fatal,
// et un bouton qui n'a rien fait ne doit pas couper la partie.
func (s *emulationSession) handleReset() {
	eng := s.currentEngine()
	if eng == nil {
		log.Printf("[retrogaming-stream] remise à zéro demandée sans partie en cours")

		return
	}
	if err := eng.Reset(); err != nil {
		log.Printf("[retrogaming-stream] remise à zéro : %v", err)

		return
	}
	log.Printf("[retrogaming-stream] console remise à zéro")
}

// handleSetControllerType — lets the frontend switch a port's libretro device
// id live without restarting the session ("setcontrollertype"). Routed
// through the engine (setControllerType), which applies it between two
// frames — never concurrently with retro_run(), same as save/load state and
// cheats.
func (s *emulationSession) handleSetControllerType(port int, deviceID uint32) {
	if s.currentEngine() == nil {
		s.sendError("No emulation session running")
		return
	}
	s.setControllerType(port, deviceID)
}

// holdPause gèle — ou dégèle — la partie sur demande explicite.
//
// Le seul moment où une partie s'arrête sans que le core soit déchargé. Le
// core reste en mémoire, donc la reprise est instantanée : c'est toute la
// différence avec « arrêter la session », qui, elle, libère la machine.
func (s *emulationSession) holdPause(paused bool) {
	s.mu.Lock()
	changed := s.heldPaused != paused
	s.heldPaused = paused
	s.mu.Unlock()
	s.applyPause()
	if changed {
		log.Printf("[retrogaming-stream] pause demandée : %v", paused)
	}
	s.sendMessage(streamMessage{Type: "pause", Paused: paused})
}

// handleDisks répond « combien de disques, et lequel est monté ».
//
// Le menu pose la question à son ouverture, sans savoir si l'hôte sait y
// répondre : une session sans partie en cours, un core sans interface de
// disques et un jeu à un seul fichier rendent tous la même chose — zéro
// disque — et le sélecteur ne s'affiche pas. C'est volontairement une
// QUESTION et non une capacité déclarée : une capacité, il faudrait la tenir
// à jour des deux côtés, et elle finirait par mentir.
func (s *emulationSession) handleDisks() {
	s.sendMessage(s.diskMessage(-1))
}

// handleChangeDisk monte une autre image, puis répond l'état réel.
func (s *emulationSession) handleChangeDisk(index int) {
	if index < 0 {
		s.sendError("Invalid disk index")

		return
	}
	s.sendMessage(s.diskMessage(index))
}

// diskMessage fait l'aller-retour avec le moteur — c'est lui qui appelle le
// core, entre deux images (engine.Disks).
//
// La réponse est TOUJOURS un message "disks", que le changement ait réussi ou
// non : le menu se recale sur ce que le core dit avoir monté, et un choix
// refusé revient de lui-même au disque précédent au lieu de rester affiché à
// tort.
//
// Un index négatif ne change rien et se contente de lire.
func (s *emulationSession) diskMessage(index int) streamMessage {
	// Pas de partie en cours : zéro disque, ce qui est la même réponse que
	// « ce core n'en a pas ». Répondre au lieu de se taire — un menu qui
	// attend une réponse qui ne vient jamais garde son sélecteur vide.
	eng := s.currentEngine()
	if eng == nil {
		return streamMessage{Type: "disks", Disks: 0, Disk: -1}
	}
	state, err := eng.Disks(index)
	if err != nil {
		return streamMessage{Type: "disks", Disks: 0, Disk: -1}
	}
	message := streamMessage{Type: "disks", Disks: state.Count, Disk: state.Current}
	// Un refus se DIT. Sans ça, le sélecteur revenait au disque précédent
	// sans un mot — indistinguable d'un clic qui n'a pas pris.
	if !state.OK {
		message.Info = "denied"
	}

	return message
}

// handleRewind toggles rewind (see the engine's rewind.go) — like
// handleFastForward, this only sets a flag the engine reads on its own next
// tick; no core call happens here.
func (s *emulationSession) handleRewind(enabled bool) {
	eng := s.currentEngine()
	if eng == nil {
		s.sendError("No emulation session running")
		return
	}
	// Le moteur répond ce qui est appliqué : un rembobinage qui n'est pas
	// accordé à cette console (voir le moteur, rewind.go) reste éteint, et
	// on le dit sans erreur bruyante — un client plus ancien peut encore
	// envoyer l'interrupteur que le menu ne montre pas.
	applied := eng.SetRewind(enabled)
	s.sendMessage(streamMessage{Type: "rewind", Enabled: applied})
}

// releaseJoinerSlot libère le port d'un joiner — SI ET SEULEMENT SI c'est
// encore le sien.
//
// POURQUOI L'IDENTITÉ, ET PAS SEULEMENT « non nil » (2026-09-20). Ce créneau a
// deux propriétaires concurrents : la boucle de lecture du joiner
// (`serveJoinerSession`, child.go) et le chemin passerelle « ce viewer est
// parti » (`s.joiners[port-1] = nil`, plus bas dans ce fichier). Ils tournent
// sur des goroutines différentes, et le second a gagné la course le
// 2026-09-19 : neuf secondes après un `gateway: owner viewer left`, le ménage
// du joiner a déréférencé un créneau déjà vidé. Panic, `nil pointer
// dereference`.
//
// Une simple garde `!= nil` ne suffirait pas : entre-temps le port libéré peut
// avoir été REPRIS par un autre joiner (l'allocation prend le premier créneau
// libre), et le vider déconnecterait un tiers — un défaut silencieux, plus
// pénible que le plantage qu'il remplacerait. On compare donc le POINTEUR reçu
// à l'entrée.
//
// Et le verrou est pris en `defer` : c'est ce qui manquait le 2026-09-19. Le
// panic avait laissé `mu` tenu à jamais — `net/http` rattrape, le processus
// survit, le verrou non. L'enfant s'est alors bloqué dans son propre
// `shutdown()` (juste en dessous, qui commence par `mu.Lock()`), n'a jamais
// atteint son `os.Exit(0)`, et le conteneur a survécu 16 h, à l'écoute et muet.
//
// Rend `true` si ce créneau a bien été libéré ici.
func (s *emulationSession) releaseJoinerSlot(port int, mine *joinerConn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mine == nil || port < 1 || port > len(s.joiners) {
		return false
	}
	if s.joiners[port-1] != mine {
		return false
	}

	if mine.ViewerID != "" {
		delete(s.viewerPorts, mine.ViewerID)
	}
	s.joiners[port-1] = nil

	// Remettre à zéro l'état d'entrée de ce port, sinon une touche ou un stick
	// maintenu à l'instant de la déconnexion reste « enfoncé » pour toujours.
	// Sous la garde d'identité : on ne touche jamais l'état d'un port repris
	// par quelqu'un d'autre.
	s.clearPortLocked(port)

	return true
}

// releasePort remet un port au neutre. La file l'appelle chaque fois qu'une
// manette CHANGE DE MAINS — prêt, reprise, retour, place donnée, départ,
// exclusion (session.Host.OnPadHandover) : une touche que les mains d'avant
// tenaient ne doit pas rester enfoncée pour les suivantes, ni pour personne
// (2026-10-09). `port` compte depuis 0, comme libretro.
func (s *emulationSession) releasePort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clearPortLocked(port)
}

// clearPortLocked — l'état d'entrée d'un port, remis à zéro. Sous s.mu.
func (s *emulationSession) clearPortLocked(port int) {
	if port < 0 || s.engine == nil {
		return
	}
	s.engine.ClearInput(port)
}

func (s *emulationSession) shutdown() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	log.Println("[retrogaming-stream] shutting down session...")
	close(s.stopping)
	s.running = false
	eng := s.engine
	s.mu.Unlock()

	// LE MOTEUR S'ARRÊTE D'ABORD : la dernière sauvegarde de cartouche part,
	// puis le contexte graphique et le cœur sont déchargés — sur SON fil, celui
	// de toutes ses images. Stop rend la main une fois tout ça fini : plus aucun
	// retro_run() ne peut tourner pendant qu'on ferme le reste (un dlclose sous
	// un retro_run en vol, c'était un SIGSEGV dans videoCallback).
	if eng != nil {
		eng.Stop()
	}
	s.flushAudioDrops()

	// No automatic save on teardown (2026-08-24). It existed as a safety net
	// back when a session died on any error and progress was lost by
	// accident; sessions now survive on their grace, so the net costs more
	// than it protects — it wrote a full state for every game merely opened
	// for ten seconds, and the save storage grew accordingly.
	//
	// Saving on the way out is now the player's explicit choice, sent as an
	// ordinary "savestate" on slot 0 BEFORE leaving (see the frontend's
	// saveOnQuit). Nothing is written unless they ask for it.

	// Detach from gatewaywebrtc — the shared client is process-wide, but
	// this child process exits right after shutdown anyway.
	if s.gatewayClient != nil {
		s.gatewayClient.Close()
		s.gatewayClient = nil
	}
	if s.videoEnc != nil {
		s.videoEnc.Stop()
	}
	if s.audioEnc != nil {
		s.audioEnc.Stop()
	}

	if s.tempRomPath != "" {
		os.Remove(s.tempRomPath)
		s.tempRomPath = ""
	}
	if s.biosTempDir != "" {
		// Before the temp dir goes away — this is where a core's freshly
		// created .NVM/.MEC still lives (see persistBiosCompanions).
		persistBiosCompanions(s.biosRoot, s.savesRoot, s.consoleID, s.biosFile, s.biosTempDir)
		os.RemoveAll(s.biosTempDir)
		s.biosTempDir = ""
	}

	activeChildSessionMu.Lock()
	if activeChildSession == s {
		activeChildSession = nil
	}
	activeChildSessionMu.Unlock()

	log.Println("[retrogaming-stream] session shut down complete")
}

// ownerDisconnected — la connexion du propriétaire vient de se fermer.
//
// Elle ne décide plus RIEN de la partie (P4, 2026-10-09). Elle décidait de
// deux choses, et les deux contredisaient la file :
//
//   - LA SAUVEGARDE CHANGEAIT DE MAINS. Le premier invité devenait
//     « l'initiateur » : la SRAM et les sauvegardes d'état partaient dans SON
//     dossier, et n'en revenaient jamais, même au retour du propriétaire.
//     Or le propriétaire ne change jamais : la partie écrit chez lui, quel que
//     soit le pad qui joue.
//   - LA FIN DE LA PARTIE, par une horloge à elle : cinq minutes sans
//     connexion de joueur de l'ancien modèle, puis os.Exit — même si la file
//     voyait quelqu'un jouer. C'est elle qui a tué une partie prêtée dans le
//     commit d'étape 171dc280. La file décide seule désormais : quelqu'un joue,
//     la partie continue ; personne, elle passe en idle et s'éteint (IdleAfter).
func (s *emulationSession) ownerDisconnected() {
	s.mu.Lock()
	s.ownerConnected = false
	username := s.username
	s.mu.Unlock()

	s.broadcast(streamMessage{Type: "playerleft", Username: username, Port: 0})
}

// activePlayerCount counts currently-connected players (owner + joiners,
// NOT spectators — see spectatorConn's doc, they never occupy a controller
// port or count toward anything player-related). The old model's count: see
// playersInGame for the one the save/load-state guard reads. It used to have a sibling that
// skipped "guest" joiners — extra local controllers of the retired
// console-mode hub, a flag the client declared; the kernel stopped relaying
// it on 2026-09-15, so every joiner is a person again.
func (s *emulationSession) activePlayerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	if s.ownerConnected {
		count++
	}
	for _, j := range s.joiners {
		if j != nil {
			count++
		}
	}
	return count
}

// playersInGame — combien de personnes tiennent une manette. La file le sait
// (v4) ; sans elle, les connexions de l'ancien modèle. Garde des sauvegardes
// d'état « en solo seulement » : rembobiner une partie à plusieurs la change
// sous les autres.
func (s *emulationSession) playersInGame() int {
	if n := v4Players(); n >= 0 {
		return n
	}

	return s.activePlayerCount()
}

// =========================================================================
// SRAM — la sauvegarde que le JEU écrit
// =========================================================================

// humanBytes — « 512 o » plutôt que « 0 Ko ».
//
// La première mesure réelle l'a montré tout de suite : une sauvegarde GBA en
// EEPROM fait 512 octets, et le journal annonçait fièrement « 0 Ko écrits »
// juste après avoir écrit le fichier. Un chiffre arrondi à zéro se lit comme
// un échec.
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

// sramFilePath — {savesRoot}/{username}/sram/{consoleID}/{reference}.srm.
//
// Même rangement que les créneaux de sauvegarde juste à côté (stateFilePath),
// et surtout PAR UTILISATEUR : deux personnes qui jouent au même jeu sur le
// même serveur ont chacune leur partie, ce qui est la moitié de la demande
// « memory card par utilisateur ». L'autre moitié — que ça survive tout court
// — c'est ce fichier qui l'apporte.
//
// L'extension .srm est celle de RetroArch : un joueur qui arrive avec ses
// sauvegardes, ou qui repart avec, n'a rien à convertir.
func sramFilePath(savesRoot, username, consoleID, reference string) string {
	safeName := strings.NewReplacer("/", "_", "\\", "_").Replace(reference)

	return filepath.Join(savesRoot, username, "sram", consoleID, safeName+".srm")
}

// coreSaveDir — le dossier que le CŒUR écrit lui-même (GET_SAVE_DIRECTORY).
//
// C'était `saves/<user>` tout court, donc la racine partagée par tout le
// monde : DeSmuME y déposait ses `.dsv` en vrac, Dolphin y créait un `User/`
// de 30 Mo, PPSSPP un `PSP/`, et rien ne disait d'où venait quoi (signalé le
// 2026-09-10 en ouvrant le dossier).
//
// KEYÉ PAR CŒUR, PAS PAR CONSOLE, contrairement à `states/<console>` et
// `sram/<console>` juste au-dessus — et c'est une différence de nature, pas
// une incohérence. Ces deux-là sont des fichiers que GameFlex écrit lui-même,
// un par ROM, donc la console est la bonne clé. Ici c'est l'émulateur qui
// range comme il l'entend, et un même émulateur sert parfois DEUX consoles :
// Dolphin est le cœur de `gc` ET de `wii`, avec un seul `User/` contenant
// `GC/` et `Wii/`. Le keyer par console dupliquerait ce dossier et
// découperait une disposition que Dolphin attend entière.
//
// Créé au passage : un cœur qui écrit dans un dossier absent échoue en
// silence (DeSmuME n'écrit son `.dsv` nulle part et ne dit rien).
func coreSaveDir(savesRoot, username, coreName string) string {
	dir := filepath.Join(savesRoot, username, "cores", coreName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[retrogaming-stream] dossier de sauvegarde du cœur %q inaccessible: %v", coreName, err)
	}

	return dir
}

// readSaveRAM lit la sauvegarde de cartouche de ce joueur pour ce jeu, que le
// moteur remet en place après retro_load_game (Config.SaveRAM). nil quand il
// n'y en a pas encore — le moteur le dit lui-même, s'il en attend une.
func readSaveRAM(savesRoot, username, consoleID, reference string) []byte {
	path := sramFilePath(savesRoot, username, consoleID, reference)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[retrogaming-stream] sram : lecture impossible (%s) : %v", path, err)
		}

		return nil
	}
	log.Printf("[retrogaming-stream] sram : %s relus depuis %s", humanBytes(len(data)), filepath.Base(path))

	return data
}

// writeSaveRAM range la sauvegarde de cartouche que le moteur vient de rendre
// (Config.OnSaveRAM) — toutes les trente secondes et à l'arrêt, seulement si
// elle a changé. Écrite à côté puis renommée : une coupure au milieu laisse
// l'ancienne entière plutôt qu'une nouvelle à moitié.
func (s *emulationSession) writeSaveRAM(current []byte, reason string) {
	s.mu.Lock()
	savesRoot, username, consoleID, reference := s.savesRoot, s.username, s.consoleID, s.reference
	s.mu.Unlock()

	if savesRoot == "" || username == "" || reference == "" {
		return
	}

	path := sramFilePath(savesRoot, username, consoleID, reference)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[retrogaming-stream] sram : dossier impossible (%s) : %v", filepath.Dir(path), err)

		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, current, 0o644); err != nil {
		log.Printf("[retrogaming-stream] sram : écriture impossible (%s) : %v", tmp, err)

		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[retrogaming-stream] sram : renommage impossible : %v", err)
		os.Remove(tmp)

		return
	}
	log.Printf("[retrogaming-stream] sram : %s écrits (%s)", humanBytes(len(current)), reason)
}
