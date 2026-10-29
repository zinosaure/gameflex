// Package docker owns every interaction with the Docker daemon (docker.sock):
// port allocation, session-container create/start with the streamex env
// contract, readiness polling, status reads for the TTL watchdog, and
// re-adoption from labels after a handler restart.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameflex/logfile"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"

	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/sessionstatus"
	"gameflex/storage/installs"
)

// statusClient — UN SEUL client, BORNÉ, pour les trois `/status`.
//
// `http.DefaultClient` n'a aucun délai de garde. Les trois lectures d'état
// (`StatusExternal`, `StatusRetro`, `StatusRemoteplay`) s'en servaient, avec un
// `context.Background()` sans échéance : un conteneur qui ACCEPTE encore les
// connexions sans jamais répondre bloquait donc l'appel pour toujours. Et comme
// le chien de garde est une seule goroutine, il ne revoyait jamais un tic —
// plus aucun balayage, aucune plateforme, jusqu'au redémarrage du kernel.
// Constaté le 2026-09-19 : 18 heures de silence après qu'un enfant se soit tu
// (#197).
//
// Cinq secondes, comme `askChildIfSilent` l'a déjà choisi pour interroger un
// child — et un `/status` sain répond en MICROSECONDES (191 µs, 220 µs relevés
// dans les journaux). La marge est donc énorme ; ce délai n'existe que pour
// qu'un silence ait une fin.
//
// POURQUOI PAS UN DÉLAI SUR LE CLIENT DOCKER, qui serait plus large : parce que
// `FollowLogs` s'en sert pour suivre les journaux d'une session en FLUX CONTINU
// (`Follow: true`). Un délai global les couperait au bout de N secondes — or
// c'est ce flux qui fait les journaux permanents, seuls témoins d'une partie qui
// a mal fini. On borne les APPELS, jamais le client.
var statusClient = &http.Client{Timeout: 5 * time.Second}

// sessionPidsLimit — les processus ET les fils (le noyau les compte ensemble)
// d'un conteneur de partie (revue de sécurité du kernel, #601). Un garde-fou
// contre une bombe fork lancée depuis un émulateur, très au-dessus de ce qu'un
// émulateur tient : quelques centaines de fils pour RPCS3 ou shadPS4. Pas de
// quota de processeur, lui : une partie a besoin de toute la machine.
var sessionPidsLimit int64 = 16384

// Docker labels are the source of truth for live sessions and what lets the
// handler re-adopt containers after a restart (see docs/GAMEFLEX.md).
const (
	labelSessionID = "gameflex.session.id"
	labelConsole   = "gameflex.console"
	labelOwner     = "gameflex.owner"
	labelGame      = "gameflex.game"
	labelPort      = "gameflex.port"
	// Retro-only labels: kind separates retro session containers from the
	// external ones during adoption (the session.id label alone is shared),
	// and reference/core/bios rebuild a full record across restarts.
	labelKind      = "gameflex.kind"
	labelReference = "gameflex.reference"
	labelCore      = "gameflex.core"
	labelBios      = "gameflex.bios"
)

// isRetroContainer — the compose.service label ("retro-<console>") has been
// set on retro sessions since the first spawn, so it also identifies
// containers created before the explicit kind label existed.
func isRetroContainer(labels map[string]string) bool {
	return labels[labelKind] == "retro" ||
		strings.HasPrefix(labels["com.docker.compose.service"], "retro-")
}

const sessionContainerPort = "8080/tcp"

// Stream quality defaults — the launch request (Stream Settings form,
// validated in manager.Launch) overrides these per session; they only fill
// in when the client sends none.
const (
	streamDefaultResolution  = "1280x720"
	streamDefaultFPS         = "60"
	streamDefaultBitrate     = "5000"
	streamPS3DefaultBitrate  = "6000"
	streamXboxDefaultBitrate = "6000"
)

type PortAllocator struct {
	low       int
	high      int
	next      int
	blacklist map[int]bool
	mu        sync.Mutex
}

func newPortAllocator(portRange string) (*PortAllocator, error) {
	lowText, highText, found := strings.Cut(portRange, "-")
	if !found {
		return nil, fmt.Errorf("invalid GAMEFLEX_PORT_RANGE %q (expected lo-hi)", portRange)
	}

	low, lowErr := strconv.Atoi(lowText)
	high, highErr := strconv.Atoi(highText)
	if lowErr != nil || highErr != nil || low > high {
		return nil, fmt.Errorf("invalid GAMEFLEX_PORT_RANGE %q", portRange)
	}

	return &PortAllocator{low: low, high: high, next: low - 1, blacklist: map[int]bool{}}, nil
}

func (allocator *PortAllocator) size() int {
	return allocator.high - allocator.low + 1
}

// Allocate walks the range round-robin, skipping blacklisted ports and any
// port already bound inside THIS container's netns. Host-side conflicts can
// only be detected by the daemon at start time — the spawn loop blacklists
// them and retries (see spawnLoop.run, spawn.go).
func (allocator *PortAllocator) Allocate() (int, error) {
	allocator.mu.Lock()
	defer allocator.mu.Unlock()

	for attempt := 0; attempt < allocator.size(); attempt++ {
		allocator.next = allocator.low + (allocator.next-allocator.low+1)%allocator.size()
		port := allocator.next
		if allocator.blacklist[port] {
			continue
		}

		listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			continue
		}
		_ = listener.Close()

		return port, nil
	}

	return 0, fmt.Errorf("no free port in range %d-%d", allocator.low, allocator.high)
}

func (allocator *PortAllocator) blacklistPort(port int) {
	allocator.mu.Lock()
	defer allocator.mu.Unlock()
	allocator.blacklist[port] = true
}

// releaseBlock un-blacklists a whole block of ports, called when the session
// that held it is gone and the host ports are free again.
func (allocator *PortAllocator) releaseBlock(base, blockSize int) {
	allocator.mu.Lock()
	defer allocator.mu.Unlock()
	for offset := 0; offset < blockSize; offset++ {
		delete(allocator.blacklist, base+offset)
	}
}

// SpawnSpec is everything one session container needs, assembled by the
// manager. It mirrors the streamex env contract (streaming-external/docs/GAMEFLEX.md).
type SpawnSpec struct {
	SessionID   string
	GameRelPath string
	GameTitle   string
	Console     string
	// Core — l'émulateur choisi sur la fiche, au nom du registre ; vide,
	// le premier de la console (streamex_emulators.go, chantier 26).
	Core     string
	Username string
	// Seats — the places of the GAME (manager.Seats): the console's own count
	// from the registry. The container may carry more pads than that.
	Seats int

	// ContainerBase + ContainerIndex name the session container
	// "streamex-<console>-<nnn>" — the console and how many of it are
	// running are readable straight off `docker ps`. The OWNER stays in the
	// gameflex.owner label, never in the name: an owner change (or a
	// re-adoption) must not require a rename.
	ContainerBase  string
	ContainerIndex int

	// Per-launch stream quality overrides from the Stream Settings form
	// (see manager.LaunchSettings) — already normalized to concrete env
	// values by the manager; zero values keep the console's default.
	Resolution string // e.g. "1280x720", "" = console default
	FPS        int
	Bitrate    int
	// Language — la langue de la page qui lance, déjà ramenée à « fr » ou
	// « en » (chantier 21) ; vide, l'émulateur garde la sienne.
	Language string
	// GamesHostDir — le dossier de jeux de la console sur un disque interne
	// (chantier 23), monté sur /opt/game à la place de celui de la
	// bibliothèque principale ; vide, c'est celui-là.
	GamesHostDir string
	// Engine — le moteur du jeu (storage.GameEngine, chantier 36) : ses
	// réglages passent par-dessus les paramètres de l'émulateur. Vide : aucun.
	Engine string
}

// gamesDir — le dossier de jeux monté sur /opt/game : celui du disque interne
// où est le jeu (chantier 23), sinon celui de la bibliothèque principale.
func (spec SpawnSpec) gamesDir(cfg config.Config, consoleID string) string {
	if spec.GamesHostDir != "" {
		return spec.GamesHostDir
	}

	return gamesHostDir(cfg.StorageHostDir, consoleID)
}

// librarySections — des sections du dossier (`bios`, `patches`…), chacune à
// sa place sous /gameflex, en lecture seule (chantier 24). JAMAIS LE DOSSIER
// ENTIER : il porte aussi la base et les sauvegardes de tous les comptes,
// qu'un émulateur n'a pas à voir.
func librarySections(cfg config.Config, sections ...string) []mount.Mount {
	mounts := make([]mount.Mount, 0, len(sections))
	for _, section := range sections {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   cfg.StorageHostSection(section),
			Target:   filepath.Join(config.StorageDir, section),
			ReadOnly: true,
		})
	}

	return mounts
}

// seatsOr — a spec without seats (one built before they existed) keeps the
// container's ceiling rather than a queue of zero.
func seatsOr(seats, ceiling int) int {
	if seats > 0 {
		return seats
	}

	return ceiling
}

// padsFor — les manettes uinput d'une partie : le plafond du conteneur, et
// jamais moins que les places de la console (la Switch en a huit, #443).
func padsFor(ceiling, seats int) int {
	if seats > ceiling {
		return seats
	}

	return ceiling
}

// AdoptedSession is what label inspection yields at startup.
type AdoptedSession struct {
	SessionID string
	Container string
	Console   string
	Game      string
	Owner     string
	Port      int
	// StartedAt — la création du conteneur, c'est-à-dire le lancement de la
	// partie : une partie retrouvée après un redémarrage du kernel garde sa
	// durée (« Live now » l'affiche) au lieu de repartir de zéro.
	StartedAt time.Time
}

// ContainerStatus is the /status payload read by the TTL watchdog.
type ContainerStatus struct {
	Players        int   `json:"players"`
	Spectators     int   `json:"spectators"`
	LastActivityMS int64 `json:"last_activity_ms"`
	// State — « booting » tant que streamex lance le jeu (un .pkg de PS4 se
	// déballe d'abord, 2026-10-22) ; la partie n'est prête qu'ensuite.
	State string `json:"state"`
	// Unpacking — l'avancée d'un déballage en cours : octets écrits sur ce
	// que pèse l'archive.
	Unpacking *struct {
		Done  int64 `json:"done"`
		Total int64 `json:"total"`
	} `json:"unpacking,omitempty"`
}

type Client struct {
	dockerClient *client.Client
	config       config.Config
	allocator    *PortAllocator
}

func New(dockerClient *client.Client, cfg config.Config) (*Client, error) {
	allocator, err := newPortAllocator(cfg.PortRange)
	if err != nil {
		return nil, err
	}

	return &Client{dockerClient: dockerClient, config: cfg, allocator: allocator}, nil
}

// CheckImages logs which of the configured session images are actually in
// the daemon. Called once at startup, and purely informative — the service
// stays up either way, since only the platform whose image is missing is
// affected and the others keep working.
//
// It exists because the failure it reports is otherwise invisible until a
// player clicks Play: the images are build artefacts nothing keeps alive, so
// a `docker image prune` silently removes any that no container currently
// references. Seeing it in the boot log beats debugging it from a raw daemon
// error surfaced in the UI.
func (dockerClient *Client) CheckImages(ctx context.Context) {
	images := map[string]string{
		"GAMEFLEX_RETRO_IMAGE":       dockerClient.config.RetroImage,
		"GAMEFLEX_REMOTEPLAY_IMAGE":  dockerClient.config.RemoteplayImage,
		"GAMEFLEX_SWITCH_IMAGE":      dockerClient.config.SwitchImage,
		"GAMEFLEX_SWITCH_EDEN_IMAGE": dockerClient.config.SwitchEdenImage,
		"GAMEFLEX_PS3_IMAGE":         dockerClient.config.PS3Image,
		"GAMEFLEX_XBOX_IMAGE":        dockerClient.config.XboxImage,
		"GAMEFLEX_PS4_IMAGE":         dockerClient.config.PS4Image,
		"GAMEFLEX_PS5_IMAGE":         dockerClient.config.PS5Image,
		"GAMEFLEX_WIIU_IMAGE":        dockerClient.config.WiiUImage,
		"GAMEFLEX_XBOX360_IMAGE":     dockerClient.config.Xbox360Image,
		"GAMEFLEX_KEYTEST_IMAGE":     dockerClient.config.KeyTestImage,
	}

	for variable, name := range images {
		if name == "" {
			continue
		}
		if _, _, err := dockerClient.dockerClient.ImageInspectWithRaw(ctx, name); err != nil {
			// EnsureImageBuilt dit lui-même quand l'image n'est pas installée
			// et que rien ne la reconstruira ici (#136).
			if dockerClient.EnsureImageBuilt(name) {
				slog.Warn("session image missing — building it now (see EnsureImageBuilt)",
					"variable", variable, "image", name)
			}
			continue
		}
		slog.Debug("session image present", "variable", variable, "image", name)
	}
}

// CheckGPU dit au démarrage les GPU de l'hôte et celui que reçoivent les
// parties — et, quand GAMEFLEX_GPU nomme une carte absente (en OCuLink,
// éteinte au démarrage), que les parties tournent sur une autre.
func (dockerClient *Client) CheckGPU() {
	wanted := dockerClient.config.GPU
	gpu, declared, ok := sessionstatus.SessionGPU("/sys", wanted)
	if !ok {
		slog.Warn("no GPU: games get none, streamex encodes on the CPU")
		return
	}
	for _, each := range sessionstatus.GPUs("/sys") {
		slog.Info("GPU", "pci", each.PCI, "vendor", each.Vendor, "card", each.Card, "render", each.Render,
			"vramBytes", each.VRAMBytes, "games", each.PCI == gpu.PCI)
	}
	if wanted != "" && !declared {
		slog.Warn("GAMEFLEX_GPU names no GPU of this host — games run on the first one",
			"GAMEFLEX_GPU", wanted, "pci", gpu.PCI)
	}
}

// sessionGPUDevices — les deux nœuds du GPU des parties (sa carte et son
// nœud de rendu), et rien d'autre de /dev/dri : l'émulateur, Vulkan,
// l'encodeur et le panneau de performances ne voient que lui (chantier 39).
// Lu à chaque partie : les numéros de « card » changent d'un démarrage à
// l'autre. Sans GPU, aucun — streamex encode alors sur le processeur.
//
// Aucun groupe à ajouter : les processus des parties tournent en root, que
// les droits du nœud n'arrêtent pas. Les groupes 44 et 990 écrits ici
// étaient ceux de cette machine-ci, d'un autre numéro ailleurs.
func sessionGPUDevices(wanted string) []container.DeviceMapping {
	gpu, _, ok := sessionstatus.SessionGPU("/sys", wanted)
	if !ok {
		return nil
	}
	var devices []container.DeviceMapping
	for _, node := range []string{gpu.Card, gpu.Render} {
		if node == "" {
			continue
		}
		devices = append(devices, container.DeviceMapping{
			PathOnHost:        "/dev/dri/" + node,
			PathInContainer:   "/dev/dri/" + node,
			CgroupPermissions: "rwm",
		})
	}

	return devices
}

// streamexSaveDir — le dossier de sauvegarde d'une console streamex.
//
// SOUS `streamex/`, par symétrie avec le `cores/` du côté rétro : ce sont
// deux fois « ce que l'émulateur écrit pour lui-même ». L'un est keyé par
// cœur (un cœur sert parfois deux consoles — Dolphin pour gc et wii), l'autre
// par console, parce qu'une console streamex EST un émulateur, dans sa propre
// image.
//
// Ils étaient au premier niveau de `saves/<user>/`, mêlés aux catégories
// (`states/`, `sram/`, `cores/`) : en ouvrant le dossier, rien ne disait si
// `ps5` était une catégorie ou une console, et la liste s'allongeait d'un nom
// de console à chaque ajout (2026-09-10). Plus rien au premier niveau ne
// nomme une console désormais.
func streamexSaveDir(savesRoot, username, console string, sub ...string) string {
	return filepath.Join(append([]string{savesRoot, username, "streamex", console}, sub...)...)
}

// installsDir — LE PREMIER LANCEMENT D'UN JEU, PARTAGÉ PAR TOUS LES COMPTES
// (chantier 22) : `cache/installs/<console>/…` du dossier (#448, rangé dans le
// cache depuis le 2026-10-24 : tout s'y refait, et Réglages › Stockage › Cache
// le montre). Le porteur, 2026-10-22 : « il faut que le déballage ne se fasse
// qu'une seule fois, la première fois, pour tous les comptes ». Ce qui est à
// quelqu'un — sauvegardes, trophées, réglages — reste dans `saves/<compte>/`.
func installsDir(root, console string, sub ...string) string {
	return installs.Dir(root, console, sub...)
}

// ps3Installs — ce que RPCS3 fait une fois par jeu, et qui ne dépend d'aucun
// compte : les données qu'un jeu installe lui-même et les .pkg installés
// (`dev_hdd0/game`), le firmware (`dev_flash`), la compilation des modules
// PPU et SPU (`cache`) et le cache de shaders de Mesa. Chacun est monté
// par-dessus le dossier du compte, à sa place.
var ps3Installs = []struct{ name, target string }{
	{"game", "/root/.config/rpcs3/dev_hdd0/game"},
	{"dev_flash", "/root/.config/rpcs3/dev_flash"},
	{"cache", "/root/.cache/rpcs3/cache"},
	{"mesa_shader_cache", "/root/.cache/rpcs3/mesa_shader_cache"},
}

// EnsureSaveDir creates the per-user, per-console save directories (this
// container's own view; the bind source is the same path on the host via
// SavesHostDir()), and the console's shared installs. RPCS3 splits
// config/data/cache into three directories; xemu keeps everything under one
// (xemu.toml, eeprom.bin, snapshots).
func (dockerClient *Client) EnsureSaveDir(username, console string) error {
	dirs := []string{streamexSaveDir(config.SavesDir, username, console)}
	if console == "ps3" {
		base := streamexSaveDir(config.SavesDir, username, "ps3")
		dirs = append(dirs,
			filepath.Join(base, "config"),
			filepath.Join(base, "data"),
			filepath.Join(base, "cache"),
		)
		for _, install := range ps3Installs {
			dirs = append(dirs, installsDir(config.StorageDir, "ps3", install.name))
		}
	}
	if console == "ps4" {
		dirs = append(dirs,
			filepath.Join(streamexSaveDir(config.SavesDir, username, "ps4"), "data"),
			installsDir(config.StorageDir, "ps4"),
		)
	}
	if console == "ps5" {
		// Kyty écrit dans son dossier de travail, pas sous ~/.local/share :
		// "savedata" reçoit `_SaveData` (les sauvegardes de jeu) et "shaders"
		// son `_PipelineCache`. Les JOURNAUX ne sont pas ici — ils vont dans
		// logs/<jour>/<session>/, où FollowLogs les écrit déjà pour tous les
		// conteneurs de session et où l'interface d'administration les lit.
		base := streamexSaveDir(config.SavesDir, username, "ps5")
		dirs = append(dirs,
			filepath.Join(base, "savedata"),
			filepath.Join(base, "shaders"),
		)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// sessionImage picks the per-console streamex image (switch default).
// consoleWantsPads — à qui le kernel ouvre /dev/uinput.
//
// C'ÉTAIT UNE LISTE DE QUATRE CONSOLES, écrite deux fois (le device et la
// règle cgroup), qu'il fallait penser à allonger. Personne n'y a pensé : la
// PS4 y a manqué (corrigé le 2026-09-09 après « aucune manette »), la PS5 et
// la Wii U y manquaient encore le 2026-09-10 — leurs sessions démarraient,
// diffusaient l'image, et basculaient en silence sur le repli clavier XTest
// que ni shadPS4 ni Kyty n'écoutent. Une manette morte, sans un mot.
//
// La question n'appartenait pas au kernel : c'est l'image qui sait si son
// émulateur veut un pad, et elle le dit dans son `adapter.json`
// (`gamepad.enabled`, lu par WantsGamepad dans le conteneur). Le kernel ne
// fait qu'ouvrir la porte — il l'ouvre donc pour toute session streamex, sauf
// keytest, qui n'a pas d'adaptateur du tout : ses « pads » sont des identités
// sans uinput derrière (voir GAMEFLEX_MAX_PLAYERS plus haut).
func consoleWantsPads(console string) bool {
	return console != "keytest"
}

// sessionImage — l'image d'une session : celle de l'émulateur choisi (le
// premier de la console sans choix) ; vide pour un émulateur qu'aucune image
// ne porte, ou une console sans image configurée.
func sessionImage(cfg config.Config, console, core string) string {
	if console == "switch" {
		emulator, ok := streamexEmulatorFor(console, core)
		if !ok {
			return ""
		}
		if emulator.image == "streamex-switch-eden" {
			return cfg.SwitchEdenImage
		}

		return cfg.SwitchImage
	}
	if console == "ps3" {
		return cfg.PS3Image
	}
	if console == "xbox" {
		return cfg.XboxImage
	}
	if console == "ps4" {
		return cfg.PS4Image
	}
	if console == "ps5" {
		return cfg.PS5Image
	}
	if console == "wiiu" {
		return cfg.WiiUImage
	}
	if console == "xbox360" {
		return cfg.Xbox360Image
	}
	if console == "keytest" {
		return cfg.KeyTestImage
	}
	return cfg.SwitchImage
}

// sessionService — le service compose de l'image d'une session, posé en
// étiquette du conteneur.
func sessionService(console, core string) string {
	if emulator, ok := streamexEmulatorFor(console, core); ok {
		return emulator.image
	}

	return "streamex-" + console
}

// switchDataTarget — où l'émulateur Switch choisi cherche ses données.
func switchDataTarget(core string) string {
	if emulator, ok := streamexEmulatorFor("switch", core); ok && emulator.data != "" {
		return emulator.data
	}

	return "/root/.local/share/citron"
}

// BlacklistPort removes one TCP port from the pool — the manager retries a
// spawn whose child exited before ready on a fresh port (host networking:
// the bind failure happens INSIDE the container, invisible to docker start).
func (dockerClient *Client) BlacklistPort(port int) {
	dockerClient.allocator.blacklistPort(port)
}

// reserveAdopted — ce qu'une session RÉ-ADOPTÉE tient déjà : son port TCP
// (#103).
//
// Au démarrage du kernel, les allocateurs repartent vides : le tourniquet des
// ports recommence à `low`, aucun bloc n'est en liste noire. Les sessions
// ré-adoptées (un rechargement d'`air`, un kernel relancé) tiennent pourtant
// les leurs, et la sonde de bind ne les voit pas — le kernel vit sur un réseau
// pont, les enfants sur celui de l'hôte. Une session neuve pouvait donc
// recevoir le port d'une ancienne encore vivante : `StatusRetro` interroge
// `<hôte>:<port>/status` SANS le nom du conteneur, et c'est l'ANCIEN enfant qui
// répondait 200 — la session neuve était déclarée prête avant même qu'on voie
// le nouvel enfant échouer à lier son port.
func (dockerClient *Client) reserveAdopted(port int) {
	if port > 0 && dockerClient.allocator != nil {
		dockerClient.allocator.blacklistPort(port)
	}
}

// ReleasePort rend au pool le port d'une session arrêtée. Un port attribué
// normalement n'est pas en liste noire :
// seul celui d'une session ré-adoptée l'est (reserveAdopted), et c'est lui
// qu'on rend ici ; pour les autres l'appel est sans effet. Sans lui, chaque
// session ré-adoptée qui s'arrête perdrait son port jusqu'au redémarrage.
func (dockerClient *Client) ReleasePort(port int) {
	if port > 0 && dockerClient.allocator != nil {
		dockerClient.allocator.releaseBlock(port, 1)
	}
}

// gamesHostDir is the host-side games folder for a streamex console —
// derived from the storage folder like the retro children's own layout: the
// console id IS the parent folder name (roms/<console-id>). No per-console
// env plumbing.
func gamesHostDir(storageHostDir, consoleID string) string {
	return filepath.Join(storageHostDir, "roms", consoleID)
}

// biosHostDir — same derivation for bios/<console-id> (Eden keys under
// bios/switch/..., RPCS3 firmware under bios/ps3, xemu BIOS under
// bios/xbox).
func biosHostDir(storageHostDir, consoleID string) string {
	return filepath.Join(storageHostDir, "bios", consoleID)
}

// CreateAndStart spawns one session container, retrying with a fresh port
// when the daemon reports a host-side conflict. It returns the published
// host port.
// sessionKeys — ce qu'une partie reçoit pour authentifier (chantier 34, P10) :
// la clé PUBLIQUE qui vérifie les jetons, et un secret interne qui n'ouvre que
// SA partie (gameflex/auth.SessionSecret). Jamais les secrets de
// l'installation : la revue de sécurité du 2026-10-28 (#598) a montré qu'une
// exécution de code dans un émulateur en aurait tiré un jeton root et les
// routes internes de tous les services. Sans secret d'installation, les
// valeurs restent vides et la partie refuse tout — fermée, pas ouverte.
func sessionKeys(cfg config.Config, sessionID string) []string {
	publicKey := ""
	if key, err := auth.PublicKey(cfg.JWTSecret); err == nil {
		publicKey = sharedauth.EncodePublicKey(key)
	}

	return []string{
		"GAMEFLEX_JWT_PUBLIC_KEY=" + publicKey,
		"GAMEFLEX_INTERNAL_SECRET=" + sharedauth.SessionSecret(cfg.InternalSecret, sessionID),
	}
}

func (dockerClient *Client) CreateAndStart(ctx context.Context, spec SpawnSpec) (port int, containerName string, err error) {
	cfg := dockerClient.config
	// Une console sans image configurée n'est pas installée ici — keytest,
	// dans une installation publique (#136).
	if sessionImage(cfg, spec.Console, spec.Core) == "" {
		return 0, "", fmt.Errorf("%w: %s", ErrImageNotInstalled, spec.Console)
	}
	// Places de manette DU CONTENEUR : le plafond général
	// (STREAMEX_MAX_PLAYERS). Les places de la PARTIE, elles, viennent du
	// registre partagé (`manager.Seats`, posé dans spec.Seats) et vont à la
	// file du child (GAMEFLEX_MAX_PLAYERS) — deux nombres distincts, et c'est
	// voulu : le conteneur peut porter plus de pads que la console n'en
	// propose. Les deux étaient le même jusqu'au 2026-10-09 : quatre places
	// pour toutes les consoles, PS3 et PS5 comprises.
	//
	// La Switch était forcée à UN pad ici, héritage de l'ère Eden. C'était
	// faux : les jeux familiaux se jouent à deux ou quatre, l'émulateur sert
	// jusqu'à quatre manettes cemuhook, et `docs/GAMEFLEX.md` annonçait déjà
	// « défaut 4 pour la Switch ». Forçage retiré le 2026-09-12, registre
	// passé à 4. Keytest garde la limite générale — mesurer la latence à
	// plusieurs demande plusieurs pads, et les siens sont de pures identités,
	// sans uinput derrière.
	//
	// ET JAMAIS MOINS DE PADS QUE DE PLACES (#443) : la Switch en a huit, le
	// plafond général quatre — les joueurs 5 à 8 auraient tenu une place sans
	// manette, et leurs touches seraient allées au clavier de l'émulateur.
	maxPlayers := padsFor(cfg.MaxPlayers, spec.Seats)

	// Stream quality: per-console defaults, overridable per launch by the
	// Stream Settings form (validated upstream in manager.Launch). Built
	// into plain variables FIRST so an override replaces the default
	// instead of appending a duplicate env key (duplicate keys in the
	// container env list are undefined behavior — glibc returns the first
	// match, so an appended override would silently lose).
	resolution, fps, bitrate := streamDefaultResolution, streamDefaultFPS, streamDefaultBitrate
	if spec.Console == "ps3" {
		bitrate = streamPS3DefaultBitrate
	} else if spec.Console == "xbox" || spec.Console == "xbox360" || spec.Console == "ps4" || spec.Console == "ps5" {
		// La Xbox 360, la PS4 et la PS5 émulées partagent le débit par défaut
		// de la Xbox : même famille de rendu 3D, même besoin.
		bitrate = streamXboxDefaultBitrate
	}
	if spec.Resolution != "" {
		resolution = spec.Resolution
	}
	if spec.FPS > 0 {
		fps = strconv.Itoa(spec.FPS)
	}
	if spec.Bitrate > 0 {
		bitrate = strconv.Itoa(spec.Bitrate)
	}

	environment := []string{
		"STREAMEX_SESSION_ID=" + spec.SessionID,
		"STREAMEX_GAME_PATH=/opt/game/" + spec.GameRelPath,
		"STREAMEX_MAX_PLAYERS=" + strconv.Itoa(maxPlayers),
		// chantier-4: what it takes for this container to own its session —
		// who it belongs to, how many pads, and the key that verifies a
		// token, because the party that DECIDES is the party that must
		// authenticate.
		"GAMEFLEX_SESSION_OWNER=" + spec.Username,
		// The QUEUE's places: the console's, not the container's ceiling —
		// it gave four to every console, PS3 and PS5 included (2026-10-09).
		"GAMEFLEX_MAX_PLAYERS=" + strconv.Itoa(seatsOr(spec.Seats, maxPlayers)),
		"STREAMEX_RESOLUTION=" + resolution,
		"STREAMEX_FPS=" + fps,
		"STREAMEX_BITRATE=" + bitrate,
	}
	environment = append(environment, sessionKeys(cfg, spec.SessionID)...)
	// Chantier 21 — la langue de la page : streamex la donne à l'écriture des
	// paramètres de l'émulateur (gameflex/emuparams), qui la pose à chaque
	// lancement.
	if spec.Language != "" {
		environment = append(environment, "GAMEFLEX_LANGUAGE="+spec.Language)
	}
	// LA CONSOLE SE NOMME ELLE-MÊME.
	//
	// C'était une chaîne de `if` par console dont chaque branche faisait la
	// MÊME chose avec une chaîne différente, et un `else` qui annonçait
	// « citron » pour tout le reste. La cible de capture, elle, vient de
	// l'`adapter.json` de l'image (`window_name`) — cette variable ne sert
	// qu'à se nommer : le journal, le champ `console` de `/status`, et le nom
	// du pad virtuel quand l'image n'en impose pas.
	//
	// Ce n'était donc pas seulement inexact — une session PS5 s'annonçait
	// « citron » dans le panneau de stats (2026-09-10) — c'était une
	// COLLISION : la Wii U tombait dans le `else`, et ses pads uinput
	// portaient le même nom que ceux d'une session Switch. Or le code qui les
	// crée met justement en garde : « device names MUST be unique — several
	// containers share the host's uinput and a same-name device would make
	// the node discovery match another container's gamepad ».
	//
	// Conséquence assumée : le pad d'une session Switch s'appelle désormais
	// « streamex pad switch » et non « streamex pad citron ». Si Citron avait
	// retenu une association de touches sous l'ancien nom, elle est à refaire
	// une fois.
	//
	// Reste ouvert (voir docs/roadmap) : ce nom devrait dériver de la SESSION,
	// pas de la console — deux sessions de la même console se retrouvent
	// encore avec des pads homonymes.
	environment = append(environment, "STREAMEX_CONSOLE="+spec.Console)
	// Les paramètres de l'émulateur, s'il en a (emulator_params.go, #347).
	if entry, ok := emulatorParamsEnv(config.StorageDir, spec.Console, spec.Core, spec.Engine); ok {
		environment = append(environment, entry)
	}

	// Essayé le 2026-09-09 et ÉCARTÉ pour la PS4 : STREAMEX_CAPTURE_MODE=desktop
	// ne change rien, parce que la capture lit DÉJÀ la racine dans les deux
	// modes (elle découpe seulement la région de la fenêtre). Les deux donnent
	// une luminance de zéro.
	//
	// Ni serveurs ICE ni ports UDP : streamex n'a plus de WebRTC à lui, tout
	// passe par gatewaywebrtc (2026-10-08).

	// LA BIBLIOTHÈQUE DE LA CONSOLE QU'ON LANCE, pas celle de la Switch.
	//
	// Ce dossier était écrit « switch » en dur, et les consoles ajoutées
	// ensuite s'en sortaient uniquement parce que chacune remplaçait TOUT le
	// tableau plus bas. Une console qui ne le remplaçait pas montait donc la
	// ludothèque Switch sur /opt/game et cherchait son jeu dedans : « game
	// not found » sur un chemin pourtant exact, vécu en branchant la Wii U
	// (2026-09-08). La Switch, elle, ne change pas de comportement — pour
	// elle, spec.Console vaut « switch ».
	gamesConsole := spec.Console
	if gamesConsole == "" {
		gamesConsole = "switch"
	}

	mounts := []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   spec.gamesDir(cfg, gamesConsole),
			Target:   "/opt/game",
			ReadOnly: true,
		},
		{
			Type:   mount.TypeBind,
			Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "switch"),
			// Citron et Eden rangent leurs données de la même façon (héritage
			// yuzu) : le MÊME dossier de l'hôte, monté là où l'émulateur de
			// l'image les cherche — une partie sauvée sous l'un se continue
			// sous l'autre (chantier 26).
			Target: switchDataTarget(spec.Core),
		},
	}
	// Les clés de la Switch (`/gameflex/bios/switch`) : l'entrypoint de
	// streamex les prend là.
	mounts = append(mounts, librarySections(cfg, "bios")...)
	if spec.Console == "ps3" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "ps3"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				Type:     mount.TypeBind,
				Source:   biosHostDir(cfg.StorageHostDir, "ps3"),
				Target:   "/opt/ps3/firmware",
				ReadOnly: true,
			},
			{
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps3", "config"),
				Target: "/root/.config/rpcs3",
			},
			{
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps3", "data"),
				Target: "/root/.local/share/rpcs3",
			},
			{
				// Les journaux de RPCS3 (RPCS3.log, TTY.log) vivent ici : ils
				// restent au compte, seul le cache de compilation se partage.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps3", "cache"),
				Target: "/root/.cache/rpcs3",
			},
		}
		for _, install := range ps3Installs {
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: installsDir(cfg.StorageHostDir, "ps3", install.name),
				Target: install.target,
			})
		}
	}
	if spec.Console == "wiiu" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "wiiu"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				// mlc01 (les sauvegardes Wii U elles-mêmes), le cache de
				// shaders et les packs graphiques vivent là — donc en
				// écriture, et PAR UTILISATEUR, même discipline que les
				// autres consoles streamex.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "wiiu"),
				Target: "/root/.local/share/Cemu",
			},
		}
		// Les clés (`bios/wiiu`) et les packs graphiques (`patches/wiiu`),
		// que l'entrypoint de Cemu prend sous /gameflex.
		mounts = append(mounts, librarySections(cfg, "bios", "patches")...)
	}
	if spec.Console == "ps4" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "ps4"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				// LE JEU N'ARRIVE PAS LANÇABLE.
				//
				// La bibliothèque PS4 tient des .pkg — des archives, que
				// shadPS4 ne sait pas démarrer. L'adaptateur les déballe une
				// fois vers /opt/games (voir consoles/ps4/adapter.json), donc
				// ce volume-là doit être ACCESSIBLE EN ÉCRITURE et survivre à
				// la partie : sans lui, chaque lancement repaierait une
				// extraction qui se compte en minutes.
				//
				// UNE FOIS POUR TOUS LES COMPTES (chantier 22) : FF7 Remake
				// pèse 145 Go déballé, et chaque compte le déballait chez lui.
				// Deux premières parties du même jeu se mettent l'une derrière
				// l'autre (le verrou de streamex, adapter/generic.go).
				Type:   mount.TypeBind,
				Source: installsDir(cfg.StorageHostDir, "ps4"),
				Target: "/opt/games",
			},
			{
				// shadPS4 0.18 tient TOUT ici — réglages, sauvegardes,
				// trophées, cache de shaders. Pas de ~/.config séparé,
				// contrairement à RPCS3 et ses trois dossiers.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps4", "data"),
				Target: "/root/.local/share/shadPS4",
			},
			{
				// LES MODULES SYSTÈME DE LA CONSOLE, s'ils sont fournis.
				//
				// shadPS4 cherche ici la vraie libSceLibcInternal.sprx au
				// démarrage, et les autres modules à la demande du jeu ; sans
				// eux il les imite. Ils s'extraient de la console qu'on
				// possède, comme tout BIOS : bios/ps4/, déclarés au registre.
				// Monté par-dessus le sys_modules du dossier persisté, en
				// lecture seule — une partie lit le firmware, elle ne l'écrit
				// pas.
				Type:     mount.TypeBind,
				Source:   biosHostDir(cfg.StorageHostDir, "ps4"),
				Target:   "/root/.local/share/shadPS4/sys_modules",
				ReadOnly: true,
			},
		}
	}
	if spec.Console == "ps5" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "ps5"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				// LES SAUVEGARDES DU JEU.
				//
				// Kyty écrit tout dans son DOSSIER DE TRAVAIL (`launch.cwd` =
				// /opt/kyty), qui est éphémère : `_SaveData` pour les
				// sauvegardes, `_PipelineCache` pour les shaders. Rien sous
				// ~/.local/share — vérifié, ce chemin reste vide après
				// plusieurs parties.
				//
				// Sans ce montage, une sauvegarde PS5 disparaît avec le
				// conteneur à la fin de la partie.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps5", "savedata"),
				Target: "/opt/kyty/_SaveData",
			},
			{
				// Le cache de pipelines Vulkan, même raison. Sans lui, les
				// shaders sont recompilés à CHAQUE partie — visible à l'œil
				// nu sur les premières secondes.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "ps5", "shaders"),
				Target: "/opt/kyty/_PipelineCache",
			},
		}
	}
	if spec.Console == "keytest" {
		// No games, saves or pad devices — the display app is baked into the
		// image and GAME_PATH only serves as the adapter's boot trigger.
		mounts = nil
	}
	if spec.Console == "xbox" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "xbox"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				// MCPX ROM, BIOS and the prebuilt xbox_hdd.qcow2 live on the
				// same volume as the PS3 firmware does (see the upstream
				// streamex compose's /opt/xbox/bios mount).
				Type:     mount.TypeBind,
				Source:   biosHostDir(cfg.StorageHostDir, "xbox"),
				Target:   "/opt/xbox/bios",
				ReadOnly: true,
			},
			{
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "xbox"),
				Target: "/root/.local/share/xemu",
			},
		}
	}
	if spec.Console == "xbox360" {
		mounts = []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   spec.gamesDir(cfg, "xbox360"),
				Target:   "/opt/game",
				ReadOnly: true,
			},
			{
				// La racine de stockage de Xenia ($XDG_DATA_HOME/Xenia) : sa
				// configuration, les profils et les sauvegardes (content/),
				// la langue de la console (xconfig.settings) et les caches de
				// shaders — en écriture, et PAR UTILISATEUR, même discipline
				// que les autres consoles streamex. Aucun firmware à monter :
				// Xenia n'en demande pas.
				Type:   mount.TypeBind,
				Source: streamexSaveDir(cfg.SavesHostDir(), spec.Username, "xbox360"),
				Target: "/root/.local/share/Xenia",
			},
		}
	}

	// L'ÉMULATEUR, que l'entrypoint déballe au lancement (chantier 29) : l'image
	// ne le porte plus, pour pouvoir se publier. `cores/` en lecture seule, à
	// sa place sous /gameflex comme les autres sections ; l'image y prend le
	// fichier que dit son étiquette (RequiredEmulatorFor).
	if spec.Console != "keytest" {
		mounts = append(mounts, librarySections(cfg, "cores")...)
	}

	if cfg.ProfilesHostDir != "" {
		// Console pad profile — streamex main serves a bare
		// {"console","keylab"} without STREAMEX_INPUT_PROFILE, and the
		// GameFlex on-screen pads would hide. Written once at startup (see
		// internal/docker/profiles), mounted read-only per console.
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   filepath.Join(cfg.ProfilesHostDir, spec.Console+".json"),
			Target:   "/opt/profile.json",
			ReadOnly: true,
		})
		environment = append(environment, "STREAMEX_INPUT_PROFILE=/opt/profile.json")
	}

	return dockerClient.spawn(ctx).run(spec.ContainerBase, spec.ContainerIndex, func(candidatePort int) (*container.Config, *container.HostConfig) {
		containerEnv := append(environment, "STREAMEX_PORT="+strconv.Itoa(candidatePort))
		// UN ÉCRAN X PAR SESSION, ET C'EST UNE QUESTION D'ISOLATION.
		//
		// Les images de console démarrent leur serveur X sur `:2` par défaut.
		// Or X écoute AUSSI sur une socket abstraite (`@/tmp/.X11-unix/X2`),
		// et les sockets abstraites appartiennent au NAMESPACE RÉSEAU — que
		// ces conteneurs partagent tous, puisqu'ils tournent en
		// `network_mode: host` (voir plus bas : c'est le contournement du
		// filtrage FORWARD qui rend ICE possible).
		//
		// Deux sessions simultanées se retrouvaient donc sur le même écran :
		// la capture de l'une filmait la fenêtre de l'autre. Constaté le
		// 2026-09-07 avec deux comptes et deux navigateurs distincts — une
		// partie Switch affichait l'écran de démarrage d'un jeu PS3, manette
		// et titre restant les siens, parce que rien de tout cela ne passe
		// par X. Vérifiable depuis n'importe quel conteneur de session :
		// `grep X11-unix /proc/net/unix` y montre les sockets des autres.
		//
		// Le port de la session est unique par construction (un port, une
		// session vivante) : il fait donc un numéro d'écran unique, et
		// lisible — l'écran dit à quelle session il appartient. Les deux
		// entrypoints lisent déjà `${DISPLAY:-:2}`, il n'y a rien à changer
		// dans les images.
		containerEnv = append(containerEnv, "DISPLAY=:"+strconv.Itoa(candidatePort))
		// The child attaches to gatewaywebrtc (host networking on both sides,
		// 127.0.0.1 IS the gateway), which holds every viewer's WebRTC —
		// streamex does not start without it.
		// GAMEFLEX_KERNEL_URL lets the child mirror players/pads/heartbeats
		// into the unified pool, and GAMEFLEX_PUBLIC_HOST is the address the
		// kernel (bridge network) must use to reach this host-networked
		// child's own heartbeat-disconnect callback.
		containerEnv = append(containerEnv,
			"GAMEFLEX_GATEWAY_URL="+cfg.GatewayURL,
			"GAMEFLEX_KERNEL_URL="+cfg.KernelURL,
			"GAMEFLEX_PUBLIC_HOST="+cfg.PublicHost,
		)

		containerConfig := &container.Config{
			Image: sessionImage(cfg, spec.Console, spec.Core),
			Env:   containerEnv,
			Labels: map[string]string{
				labelSessionID: spec.SessionID,
				labelStack:     cfg.StackName,
				labelConsole:   spec.Console,
				labelOwner:     spec.Username,
				labelGame:      spec.GameTitle,
				labelPort:      strconv.Itoa(candidatePort),
				labelCore:      spec.Core,

				// Explicit compose project/service labels: the streamex
				// images carry their OWN compose project's labels (they are
				// built in the streamex repo), and Docker merges
				// image labels into containers created from them — session
				// containers would show up under the "streamex" group in
				// UIs like the VS Code containers view. They belong to the
				// gameflex stack, so pin the labels here — THIS stack's name
				// (GAMEFLEX_STACK_NAME, the compose project): a second stack on
				// the same machine (the public install next to the dev) must
				// not see them as its own, nor remove them as orphans.
				"com.docker.compose.project": cfg.StackName,
				"com.docker.compose.service": sessionService(spec.Console, spec.Core),
			},
			ExposedPorts: nat.PortSet{sessionContainerPort: struct{}{}},
		}

		devices := sessionGPUDevices(cfg.GPU)
		if consoleWantsPads(spec.Console) {
			// uinput virtual gamepad — Eden and Cemu feed their cemuhook
			// (DSU) pads from streamex's own pad state, RPCS3/xemu/shadPS4/Kyty
			// read the device in evdev/SDL mode, real analog sticks (see
			// docs/streamex/GAMEPAD.md). The server falls back to keyboard
			// emulation if the device is missing.
			devices = append(devices, container.DeviceMapping{
				PathOnHost:        "/dev/uinput",
				PathInContainer:   "/dev/uinput",
				CgroupPermissions: "rwm",
			})
		}

		// Host networking — same reasoning as CreateAndStartRetro's own
		// doc: bridge + port-publishing routes every ICE UDP packet through
		// Docker's NAT into the bridge subnet, and this host's iptables
		// FORWARD policy (DROP, only a couple of stale per-IP ACCEPT rules)
		// silently drops it there. It also meant the webpack proxy had to
		// resolve the container's own DNS alias to reach it at all (see
		// streamex_router.go's own doc) — host networking removes that
		// indirection too, same PublicHost:port model the retro and
		// remoteplay kinds already use.
		runInit := true
		hostConfig := &container.HostConfig{
			NetworkMode: "host",
			// UN VRAI INIT EN PID 1 : les orphelins sont récupérés.
			//
			// L'entrypoint lance pulseaudio, weston, Xwayland et openbox en
			// arrière-plan, puis `exec streamex` : streamex devient PID 1 et
			// hérite de tout ce qui a été lancé. Or il n'attend que ses propres
			// enfants — l'émulateur de l'adapter. Tout processus orphelin qui
			// meurt dans le conteneur restait donc `<defunct>` jusqu'à la fin
			// de la session (constaté le 2026-09-11 sur un RPCS3 relancé à la
			// main). docker-init fait ce travail, et transmet les signaux
			// d'arrêt à streamex.
			Init: &runInit,
			Resources: container.Resources{
				Devices: devices,
				// 12 Go, PAS 6. C'est un PLAFOND (memory.max du cgroup), rien
				// n'est réservé tant que l'émulateur ne s'en sert pas.
				//
				// À 6 Go, charger un savestate de Final Fantasy XIII sous RPCS3
				// s'enlisait (2026-09-11) : le cgroup a touché son plafond 2 562
				// fois (`memory.events: max`) sans jamais tuer personne
				// (`oom_kill 0`) — le noyau pousse alors l'émulateur en swap, et
				// la partie reste figée sur l'écran de chargement. Rien dans le
				// journal de RPCS3 ne dit pourquoi : il faut lire le cgroup.
				//
				// Mesuré ensuite à 12 Go (`memory.stat`) : 9,6 Go facturés au
				// conteneur, dont 3,4 Go de cache de fichiers RÉCUPÉRABLE. La
				// mémoire vraiment tenue est ~6,4 Go (tas 6,0 + partagée 0,25 +
				// noyau) — elle dépassait donc à elle seule l'ancien plafond. 12
				// laisse ~5,6 Go de marge au-dessus, et deux sessions lourdes
				// tiennent encore dans l'hôte. Ne pas lire `memory.current` seul :
				// il compte le cache, et gonfle le besoin réel de moitié.
				Memory:    12 * 1024 * 1024 * 1024,
				PidsLimit: &sessionPidsLimit,
				// PAS DE MemorySwap : Docker autorise alors autant de swap que de
				// mémoire.
				//
				// Borné à mémoire + 2 Go le 2026-09-11 (« tomber en le disant
				// plutôt que figer en silence »), puis retiré le même soir sur une
				// FAUSSE PISTE : le rechargement de savestate que RPCS3 enchaîne
				// après une création échouait en affichant `errno=12 Cannot
				// allocate memory`. C'était une vérification interne du lecteur
				// zstd de RPCS3 (serialization_ext.cpp:942), dont l'errno n'est
				// qu'un reste : elle revient à l'identique sans borne, sans un seul
				// événement mémoire du cgroup (docs/streamex/TROUBLESHOOTING.md).
				// La borne reste retirée faute de pic mesuré : rien ne dit que 2 Go
				// de marge suffisent à une partie PS3.
			},
			Mounts:  mounts,
			ShmSize: 2 * 1024 * 1024 * 1024,
		}
		if consoleWantsPads(spec.Console) {
			// CHAQUE CONTENEUR NE VOIT QUE SES PROPRES MANETTES.
			//
			// Le /dev/input de l'hôte n'est plus monté : il montrait les pads
			// de TOUTES les sessions. Deux sessions de la même console se
			// disputaient alors leurs manettes — même nom pour RPCS3, qui
			// choisit son pad par le nom, mêmes VID/PID pour SDL, qui filtre
			// sur eux. streamex crée désormais lui-même, dans le conteneur, le
			// nœud de chacun de SES pads (voir gamepad.createNode).
			//
			// La règle reste nécessaire : le cgroup des périphériques (eBPF en
			// cgroup v2) refuse sinon d'ouvrir ces nœuds (r, w) comme de les
			// créer (m). RPCS3 avait pris un EPERM sur son propre pad sans elle
			// (« aucun bouton ne répond »).
			hostConfig.DeviceCgroupRules = []string{"c 13:* rmw"}
		}

		return containerConfig, hostConfig
	})
}

// WaitReadyHostExternal polls the container's /status endpoint until it
// answers — host networking (see CreateAndStart's own doc), so this is the
// external/streamex kind's counterpart to WaitReadyHostRetro/WaitReadyHost:
// a host-mode container has no address on any bridge network, so the
// bridge-based readiness check cannot work here either.
//
// UN DÉBALLAGE QUI AVANCE REPOUSSE L'ÉCHÉANCE (2026-10-22). streamex répond
// « booting » pendant qu'il lance le jeu, et un .pkg de PS4 se déballe
// d'abord : deux minutes et demie pour les 90 Go de FF7 Remake, que les 90
// secondes coupaient en plein milieu. Tant que des octets s'écrivent,
// l'attente continue ; `timeout` sans progrès reste un échec. onUnpacking
// reçoit l'avancée (octets écrits, poids de l'archive).
func (dockerClient *Client) WaitReadyHostExternal(ctx context.Context, containerName string, port int, timeout time.Duration, onUnpacking func(done, total int64)) error {
	deadline := time.Now().Add(timeout)
	var unpacked int64

	for time.Now().Before(deadline) {
		if status, err := dockerClient.StatusExternal(ctx, containerName, port); err == nil {
			if status.State != "booting" {
				return nil
			}
			if status.Unpacking != nil && status.Unpacking.Done > unpacked {
				unpacked = status.Unpacking.Done
				deadline = time.Now().Add(timeout)
				if onUnpacking != nil {
					onUnpacking(status.Unpacking.Done, status.Unpacking.Total)
				}
			}
		}

		if exitCode, exited, inspectErr := dockerClient.containerExitInfo(ctx, containerName); inspectErr == nil && exited {
			return fmt.Errorf("%w (exit code %d): %s", ErrChildExited, exitCode, dockerClient.containerLogTail(ctx, containerName, 5))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return fmt.Errorf("session did not become ready in time")
}

// StatusExternal reads the live /status payload — the host's own LAN
// address on the port the allocator assigned, same as StatusRetro/
// StatusRemoteplay.
func (dockerClient *Client) StatusExternal(ctx context.Context, containerName string, port int) (*ContainerStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/status", dockerClient.config.PublicHost, port), nil)
	if err != nil {
		return nil, err
	}

	response, err := statusClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status endpoint returned %s", response.Status)
	}

	var status ContainerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

// IsRunning reports whether the container exists and is running.
func (dockerClient *Client) IsRunning(ctx context.Context, containerName string) (bool, error) {
	inspect, err := dockerClient.dockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return false, err
	}

	return inspect.State != nil && inspect.State.Running, nil
}

// Stop removes the container by force — a session container has nothing to
// preserve itself (saves live on the shared volume).
func (dockerClient *Client) Stop(ctx context.Context, containerName string) error {
	return dockerClient.dockerClient.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})
}

// AdoptExisting re-lists running session containers from their labels — the
// source of truth after a handler restart (no orphaned sessions).
func (dockerClient *Client) AdoptExisting(ctx context.Context) ([]AdoptedSession, error) {
	list, err := dockerClient.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: dockerClient.sessionFilters(),
	})
	if err != nil {
		return nil, fmt.Errorf("list session containers: %w", err)
	}

	adopted := make([]AdoptedSession, 0, len(list))
	for _, listed := range list {
		labels := listed.Labels
		// Retro containers share the session.id label but belong to the
		// RETRO records — adopting them as streamex sessions here made the
		// streamex TTL watchdog stop live retro containers (seen in real:
		// "session idle, stopping" on a retro session id).
		if isRetroContainer(labels) {
			continue
		}
		// Remoteplay children share the session.id label too but belong to
		// the REMOTEPLAY records — adopting them as streamex sessions here
		// made the pool serve platform "streamex" for a PS5 session id
		// (seen in real: the dashboard preview opened /api/v1/streamex/{id}/ws
		// → 404, and the idle watchdog reported the PS5 session with
		// platform "streamex").
		if isRemoteplayContainer(labels) {
			continue
		}

		sessionID := labels[labelSessionID]
		if sessionID == "" {
			continue
		}

		port := 0
		if portText := labels[labelPort]; portText != "" {
			port, _ = strconv.Atoi(portText)
		}

		dockerClient.reserveAdopted(port)
		adopted = append(adopted, AdoptedSession{
			SessionID: sessionID,
			Container: strings.TrimPrefix(listed.Names[0], "/"),
			Console:   labels[labelConsole],
			Game:      labels[labelGame],
			Owner:     labels[labelOwner],
			Port:      port,
			StartedAt: time.Unix(listed.Created, 0),
		})
	}

	return adopted, nil
}

// isNameConflict matches the daemon's "container name already taken"
// error — the spawn numbering retries with the next number on it.
func isNameConflict(err error) bool {
	return strings.Contains(err.Error(), "is already in use by container")
}

// ErrImageMissing — the session image is not in the daemon. Its own error
// value rather than a string check at the call site: this is the ONE spawn
// failure a user can act on, and it deserves a message that says what to do.
//
// It happens for real: gameflex-retrogaming:latest is a build artefact that
// nothing keeps alive — no long-running container references it, so a
// `docker image prune` collects it as dangling. The whole retro feature then
// fails at the first launch with a raw daemon error surfaced to the player
// (constaté le 2026-08-17).
var ErrImageMissing = errors.New("session image not built")

// ErrImageNotInstalled — l'image de partie manque, et cette installation ne
// sait pas la construire : pas de contexte de build, ou pas d'image
// configurée pour la console. C'est le cas d'une installation publique, où
// l'utilisateur tire ou construit ses images. À la différence
// d'ErrImageMissing, réessayer plus tard n'y changera rien (#136).
var ErrImageNotInstalled = errors.New("session image not installed")

// isImageMissing matches the daemon's "No such image" on container create.
func isImageMissing(err error) bool {
	return strings.Contains(err.Error(), "No such image")
}

func isPortConflict(err error) bool {
	message := err.Error()
	return strings.Contains(message, "port is already allocated") ||
		strings.Contains(message, "address already in use")
}

// RetroSpawnSpec is the spawn recipe for one retrogaming session container:
// the retrogaming image runs the SAME binary in standalone child mode
// (GAMEFLEX_CHILD_PORT, see retrogaming/internal/retrogaming/child.go), with the
// whole storage root mounted at /gameflex — the sections' sub-paths
// (resolved by the manager from the storage service) become the child's
// absolute env paths.
type RetroSpawnSpec struct {
	SessionID string
	ConsoleID string
	Reference string
	Core      string
	Bios      string
	Username  string
	GameTitle string
	// Resolution — le canevas demandé, déjà en pixels (« 640x480 »), ou vide
	// pour garder la géométrie d'époque de la console (#150).
	Resolution string
	// FPS et Bitrate — la cadence et le débit (kbps) du panneau des réglages
	// de flux, déjà bornés ; 0 garde le défaut de la console (#332).
	FPS     int
	Bitrate int
	// Language — la langue de la page qui lance (chantier 21), « fr » ou
	// « en » ; vide, les cœurs gardent la leur.
	Language string
	// ContainerBase names the container "retro-<console>-<nnn>".
	ContainerBase  string
	ContainerIndex int
	// Disk — le disque interne où est le jeu (chantier 23) ; nil, il est dans
	// la bibliothèque principale. Ses jeux sont montés à part, et le moteur
	// rétro les lit là ; la console (cœurs, BIOS) reste celle de la
	// bibliothèque principale.
	Disk *InternalDisk
}

// CreateAndStartRetro spawns one retrogaming session container with the
// same discipline as CreateAndStart: port-allocator retry loop, and a network
// alias ("retro-<sessionId>") so the service (and later the dev proxy) can
// reach the container by DNS name.
func (dockerClient *Client) CreateAndStartRetro(ctx context.Context, spec RetroSpawnSpec) (port int, containerName string, err error) {
	cfg := dockerClient.config
	// These env paths ARE the child's whole world (storageclient.AdoptEnv):
	// the sections of the storage folder, at their place under /gameflex.
	section := func(name string) string {
		return filepath.Join(config.StorageDir, name)
	}
	romsPath := section("roms")
	if spec.Disk != nil {
		romsPath = spec.Disk.RomsDir()
	}
	environment := []string{
		"SESSION_ID=" + spec.SessionID,
		"ROMS_PATH=" + romsPath,
		"CORES_PATH=" + section("cores"),
		"BIOS_PATH=" + section("bios"),
		"SAVES_PATH=" + section("saves"),
	}

	// roms/cores/bios in read-only — they never change during a session.
	// The kernel's own mount is writable (downloaded cores, games imported
	// from a friend); this one deliberately is not: a running emulator reads
	// the library, it never adds to it, and the narrower right is the one
	// worth keeping. saves/ is the only writable part.
	mounts := append(librarySections(cfg, "roms", "cores", "bios"), mount.Mount{
		Type:   mount.TypeBind,
		Source: cfg.SavesHostDir(),
		Target: config.SavesDir,
	})
	if spec.Disk != nil {
		// LE JEU SUR UN DISQUE INTERNE (chantier 23) : ses jeux, en lecture
		// seule comme ceux de la bibliothèque, au même chemin que dans le
		// kernel. Le moteur les lit là (ROMS_PATH) ; cœurs et BIOS restent
		// ceux de /gameflex.
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   spec.Disk.RomsHostDir(),
			Target:   spec.Disk.RomsDir(),
			ReadOnly: true,
		})
	}

	return dockerClient.spawn(ctx).run(spec.ContainerBase, spec.ContainerIndex, func(candidatePort int) (*container.Config, *container.HostConfig) {
		containerEnv := append(environment, "GAMEFLEX_CHILD_PORT="+strconv.Itoa(candidatePort))
		// The child attaches to gatewaywebrtc over the shared internal
		// transport (host networking on both sides, 127.0.0.1 is the gateway
		// itself) — its only media path, it does not start without it. Its
		// session secret (sessionKeys) authenticates the child's internal WS
		// to the gateway.
		containerEnv = append(containerEnv,
			"GAMEFLEX_GATEWAY_URL="+cfg.GatewayURL,
			"GAMEFLEX_KERNEL_URL="+cfg.KernelURL,
			// chantier-4: the child owns its own session, so it needs what it
			// takes to build one at boot — who it belongs to, which console
			// (hence how many pads), and the key that verifies a token,
			// because the party that DECIDES is the party that must
			// authenticate. Handing the identity down pre-verified would put
			// us back where retro_relay.go was on 2026-08-23: trusting a claim.
			"GAMEFLEX_SESSION_OWNER="+spec.Username,
			"GAMEFLEX_CONSOLE_ID="+spec.ConsoleID,
		)
		containerEnv = append(containerEnv, sessionKeys(cfg, spec.SessionID)...)
		// #150 — la résolution choisie devient le canevas de l'encodeur.
		// Absente, le child garde la géométrie d'époque de la console
		// (`consoles.NativeSize`) : c'est le défaut, et il reste le bon pour
		// la grande majorité des parties.
		if spec.Resolution != "" {
			containerEnv = append(containerEnv, "GAMEFLEX_RESOLUTION="+spec.Resolution)
		}
		// Chantier 21 — la langue de la page : le moteur la répond aux cœurs.
		if spec.Language != "" {
			containerEnv = append(containerEnv, "GAMEFLEX_LANGUAGE="+spec.Language)
		}
		// #332 — la cadence et le débit du même panneau : le moteur ne rend
		// que les images de la cadence choisie, l'encodeur prend ce débit.
		if spec.FPS > 0 {
			containerEnv = append(containerEnv, "GAMEFLEX_FPS="+strconv.Itoa(spec.FPS))
		}
		if spec.Bitrate > 0 {
			containerEnv = append(containerEnv, "GAMEFLEX_BITRATE="+strconv.Itoa(spec.Bitrate))
		}

		containerConfig := &container.Config{
			Image: cfg.RetroImage,
			Env:   containerEnv,
			// The image's own CMD is the air dev supervisor — a spawned
			// session overrides it and runs the pre-built binary directly:
			// one process, one game, no watcher.
			Cmd: []string{"/app/retrogaming", "retrogaming-child", spec.SessionID},
			Labels: map[string]string{
				labelSessionID: spec.SessionID,
				labelStack:     cfg.StackName,
				labelConsole:   spec.ConsoleID,
				labelOwner:     spec.Username,
				labelGame:      spec.GameTitle,
				labelPort:      strconv.Itoa(candidatePort),

				labelKind:      "retro",
				labelReference: spec.Reference,
				labelCore:      spec.Core,
				labelBios:      spec.Bios,

				"com.docker.compose.project": cfg.StackName,
				"com.docker.compose.service": "retro-" + spec.ConsoleID,
			},
			ExposedPorts: nat.PortSet{sessionContainerPort: struct{}{}},
		}

		// Host networking — same reasoning as CreateAndStartRemoteplay's
		// own doc: bridge + port-publishing means every ICE UDP packet is
		// forwarded through Docker's NAT into the bridge subnet, and this
		// host's iptables FORWARD policy (DROP, only a couple of stale
		// per-IP ACCEPT rules) silently drops it there — confirmed live
		// (2026-08-18): ICE exchanges valid host+srflx candidates on both
		// sides, then times out to "failed" after the standard ~30s, on
		// EVERY subnet Docker has assigned this project so far, custom or
		// default. network_mode: host sidesteps Docker's NAT/forwarding
		// entirely, exactly like remoteplay/coturn already do.
		hostConfig := &container.HostConfig{
			NetworkMode: "host",
			Resources: container.Resources{
				Devices:   sessionGPUDevices(cfg.GPU),
				Memory:    6 * 1024 * 1024 * 1024,
				PidsLimit: &sessionPidsLimit,
			},
			Mounts:  mounts,
			ShmSize: 1 * 1024 * 1024 * 1024,
		}

		return containerConfig, hostConfig
	})
}

// RetroContainerStatus is the retro child's own /status payload (see
// retrogaming/internal/retrogaming/child.go's serveSelfStatus) — CPU/RAM
// are self-sampled from /proc by the child process.
type RetroContainerStatus struct {
	CPUPercent float64         `json:"cpu_percent"`
	RSSBytes   int64           `json:"rss_bytes"`
	Players    int             `json:"players"`
	Spectators int             `json:"spectators"`
	FPS        float64         `json:"fps"`
	GPU        *RetroGPUStatus `json:"gpu"`
	// Emulator — le cœur libretro de la partie et sa version.
	Emulator string `json:"emulator"`
}

// RetroGPUStatus is the child's sysfs GPU sampling (see readGPUStatus in
// retrogaming/internal/retrogaming/child.go) — nil when the child omits the
// key (no readable sysfs on the host), so the frontend shows "—" instead of
// fabricated zeros.
type RetroGPUStatus struct {
	BusyPercent    int    `json:"busy_percent"`
	VRAMTotalBytes uint64 `json:"vram_total_bytes"`
	VRAMUsedBytes  uint64 `json:"vram_used_bytes"`
}

// StatusRetro reads the session container's own /status — the watchdog's
// player-count source and the performance panel's CPU/RAM source. The
// child runs in host networking (see CreateAndStartRetro's own doc), so the
// endpoint is the host's own LAN address on the port the allocator
// assigned, same as StatusRemoteplay.
func (dockerClient *Client) StatusRetro(ctx context.Context, containerName string, port int) (*RetroContainerStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/status", dockerClient.config.PublicHost, port), nil)
	if err != nil {
		return nil, err
	}

	response, err := statusClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status endpoint returned %s", response.Status)
	}

	var status RetroContainerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// AdoptedRetroSession — a retro session container rebuilt from its labels
// at adoption time (same lifecycle discipline as AdoptExisting).
type AdoptedRetroSession struct {
	SessionID string
	Container string
	ConsoleID string
	Reference string
	Title     string
	Core      string
	Bios      string
	Owner     string
	Port      int
	// StartedAt — la création du conteneur, c'est-à-dire le lancement de la
	// partie : une partie retrouvée après un redémarrage du kernel garde sa
	// durée (« Live now » l'affiche) au lieu de repartir de zéro.
	StartedAt time.Time
}

// AdoptExistingRetro lists the running retro session containers (kind
// label) so the manager can rebuild its records after a restart — without
// this, live sessions would keep emulating with nobody tracking them.
func (dockerClient *Client) AdoptExistingRetro(ctx context.Context) ([]AdoptedRetroSession, error) {
	list, err := dockerClient.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: dockerClient.sessionFilters(),
	})
	if err != nil {
		return nil, fmt.Errorf("list session containers: %w", err)
	}

	adopted := make([]AdoptedRetroSession, 0, len(list))
	for _, listed := range list {
		labels := listed.Labels
		if !isRetroContainer(labels) {
			continue
		}

		sessionID := labels[labelSessionID]
		if sessionID == "" {
			continue
		}

		port := 0
		if portText := labels[labelPort]; portText != "" {
			port, _ = strconv.Atoi(portText)
		}

		dockerClient.reserveAdopted(port)
		adopted = append(adopted, AdoptedRetroSession{
			SessionID: sessionID,
			Container: strings.TrimPrefix(listed.Names[0], "/"),
			ConsoleID: labels[labelConsole],
			Reference: labels[labelReference],
			Title:     labels[labelGame],
			Core:      labels[labelCore],
			Bios:      labels[labelBios],
			Owner:     labels[labelOwner],
			Port:      port,
			StartedAt: time.Unix(listed.Created, 0),
		})
	}
	return adopted, nil
}

// ListExitedSessionContainers returns the names of session containers (our
// label) that are stopped — leftovers nobody removed because they exited
// while this service was down or mid-restart (a container is only adopted
// while running). The watchdog prunes them so the port names and the
// docker ps output stay clean.
func (dockerClient *Client) ListExitedSessionContainers(ctx context.Context) ([]string, error) {
	list, err := dockerClient.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: dockerClient.sessionFilters(filters.Arg("status", "exited")),
	})
	if err != nil {
		return nil, fmt.Errorf("list exited session containers: %w", err)
	}

	names := make([]string, 0, len(list))
	for _, listed := range list {
		if len(listed.Names) > 0 {
			names = append(names, strings.TrimPrefix(listed.Names[0], "/"))
		}
	}
	return names, nil
}

// ──────────────────────────────────────────────────────────────
// Remoteplay session containers — same one-container-per-session model as
// the retro kind, but the child carries NO game files: the device payload
// (hostId/rpKey/registKey) arrives over POST /session/start (see the
// remoteplay child mode), the WebRTC stream runs in the container and the
// control plane stays in the remoteplay service.

// labelHostID is the physical device a remoteplay session streams from —
// the capacity rule ("how many streams does one device accept") keys on it.
const labelHostID = "gameflex.host_id"

func isRemoteplayContainer(labels map[string]string) bool {
	return labels[labelKind] == "remoteplay"
}

type RemoteplaySpawnSpec struct {
	SessionID string
	HostID    string
	HostName  string
	HostType  string
	Username  string
	// ContainerBase names the container "remoteplay-<hostType>-<nnn>".
	ContainerBase  string
	ContainerIndex int
}

// CreateAndStartRemoteplay spawns one remoteplay session container: child
// mode, one published TCP port for the API surface, and the network alias
// "remoteplay-<sessionId>".
func (dockerClient *Client) CreateAndStartRemoteplay(ctx context.Context, spec RemoteplaySpawnSpec) (port int, containerName string, err error) {
	cfg := dockerClient.config
	environment := []string{
		"SESSION_ID=" + spec.SessionID,
		// chantier-4 — what the child needs to own its own session: who it
		// belongs to, and the key to verify a claim itself. remoteplay is
		// one pad, so maxPlayers stays 1: nobody can JOIN it (a non-owner's
		// smallest slot is 2), and the pad only ever moves by handover.
		"GAMEFLEX_SESSION_OWNER=" + spec.Username,
		"GAMEFLEX_MAX_PLAYERS=1",
	}
	environment = append(environment, sessionKeys(cfg, spec.SessionID)...)

	return dockerClient.spawn(ctx).run(spec.ContainerBase, spec.ContainerIndex, func(candidatePort int) (*container.Config, *container.HostConfig) {
		containerEnv := append(environment, "GAMEFLEX_CHILD_PORT="+strconv.Itoa(candidatePort))
		// Same gateway hand-off as retro/streamex — the child pushes Takion
		// frames over the shared transport, its only media path.
		containerEnv = append(containerEnv,
			"GAMEFLEX_GATEWAY_URL="+cfg.GatewayURL,
			"GAMEFLEX_KERNEL_URL="+cfg.KernelURL,
		)

		containerConfig := &container.Config{
			Image: cfg.RemoteplayImage,
			Env:   containerEnv,
			// The image's own CMD is the air dev supervisor — a spawned
			// session overrides it and runs the pre-built binary in child
			// mode: one process, one session, no watcher.
			Cmd: []string{"/app/remoteplay", "remoteplay-child"},
			Labels: map[string]string{
				labelSessionID: spec.SessionID,
				labelStack:     cfg.StackName,
				labelConsole:   spec.HostType,
				labelHostID:    spec.HostID,
				labelOwner:     spec.Username,
				labelGame:      spec.HostName,
				labelPort:      strconv.Itoa(candidatePort),

				labelKind: "remoteplay",

				"com.docker.compose.project": cfg.StackName,
				"com.docker.compose.service": "remoteplay-" + spec.HostType,
			},
			ExposedPorts: nat.PortSet{sessionContainerPort: struct{}{}},
		}

		// Host networking — the child must reach the console on the LAN and
		// its ICE candidates are the host's own addresses, exactly like the
		// old remoteplay service. The bridge network is a 192.168.0.0/16
		// subnet that swallows the LAN route ("no route to host" towards
		// 192.168.1.12 seen live), so bridge + port publishing cannot work
		// here. The allocator still assigns the UNIQUE host port the child
		// binds (GAMEFLEX_CHILD_PORT above); no /dev/dri, no GPU groups, no
		// storage mounts — the container only relays the console's own
		// encoded stream.
		hostConfig := &container.HostConfig{
			NetworkMode: "host",
			Resources: container.Resources{
				Memory:    2 * 1024 * 1024 * 1024,
				PidsLimit: &sessionPidsLimit,
			},
		}

		return containerConfig, hostConfig
	})
}

// RemoteplayContainerStatus is the remoteplay child's own /status payload
// (see remoteplay/internal/api/child.go's ChildStatus) — CPU/RAM are
// self-sampled from /proc by the child process.
type RemoteplayContainerStatus struct {
	CPUPercent float64 `json:"cpu_percent"`
	RSSBytes   int64   `json:"rss_bytes"`
	Players    int     `json:"players"`
	Spectators int     `json:"spectators"`
}

// StatusRemoteplay reads the session container's own /status. The child
// runs in host networking, so the endpoint is the host's own LAN address on
// the port the allocator assigned.
func (dockerClient *Client) StatusRemoteplay(ctx context.Context, containerName string, port int) (*RemoteplayContainerStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/status", dockerClient.config.PublicHost, port), nil)
	if err != nil {
		return nil, err
	}

	response, err := statusClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status endpoint returned %s", response.Status)
	}

	var status RemoteplayContainerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// WaitReadyHost polls the child's /status through the host's LAN address
// (host networking — a host-mode container has no bridge address, so the
// bridge-based WaitReady cannot work).
// ErrChildExited — a session container exited before it became ready. Host
// networking means a port held by a lingering process kills the child at
// bind time while docker start itself succeeds, so only the readiness poll
// (and this error) makes the failure visible.
var ErrChildExited = errors.New("child exited before becoming ready")

// WaitReadyHost polls the child's /status until it answers, the deadline
// elapses, the caller cancels, or the container itself exits — a dead
// child is reported immediately with its exit code and last log lines
// instead of being polled silently for the whole timeout.
func (dockerClient *Client) WaitReadyHost(ctx context.Context, containerName string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := dockerClient.StatusRemoteplay(ctx, containerName, port); err == nil {
			return nil
		}

		if exitCode, exited, inspectErr := dockerClient.containerExitInfo(ctx, containerName); inspectErr == nil && exited {
			return fmt.Errorf("%w (exit code %d): %s", ErrChildExited, exitCode, dockerClient.containerLogTail(ctx, containerName, 5))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return fmt.Errorf("session did not become ready in time")
}

// WaitReadyHostRetro is WaitReadyHost's retro counterpart (StatusRetro
// instead of StatusRemoteplay) — same host-networking reasoning as
// CreateAndStartRetro's own doc: a host-mode container has no bridge
// address, so the bridge-based WaitReady cannot work here either.
func (dockerClient *Client) WaitReadyHostRetro(ctx context.Context, containerName string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := dockerClient.StatusRetro(ctx, containerName, port); err == nil {
			return nil
		}

		if exitCode, exited, inspectErr := dockerClient.containerExitInfo(ctx, containerName); inspectErr == nil && exited {
			return fmt.Errorf("%w (exit code %d): %s", ErrChildExited, exitCode, dockerClient.containerLogTail(ctx, containerName, 5))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return fmt.Errorf("session did not become ready in time")
}

// containerExitInfo reports the container's exit code and whether it has
// left the running state. A failed inspect (container removed mid-poll)
// counts as exited: in a readiness poll a container that cannot be
// inspected is certainly not ready either.
func (dockerClient *Client) containerExitInfo(ctx context.Context, containerName string) (int, bool, error) {
	inspected, err := dockerClient.dockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return -1, true, nil
	}
	return inspected.State.ExitCode, !inspected.State.Running, nil
}

// FollowLogs recopie la sortie d'un conteneur dans un fichier, tant qu'il
// vit.
//
// C'est le vrai besoin : `docker logs` meurt avec le conteneur, et un
// conteneur de session est créé pour une partie puis détruit. Quand une
// partie s'est mal passée, il ne reste RIEN — `containerLogTail` juste en
// dessous en garde les dernières lignes au moment de la mort, ce qui aide à
// dire pourquoi il est mort mais pas à comprendre ce qu'il faisait avant.
//
// Le fichier vit dans `logs/` du dossier, sous L'HEURE DE DÉMARRAGE de cette
// session — et c'est ce qui la distingue de la précédente, car Docker
// RÉUTILISE les noms : deux parties de NES dans la journée s'appellent toutes
// les deux `retro-nes-001`. Sans l'heure, elles se mélangeraient dans un seul
// fichier et on ne saurait plus où finit l'une.
//
// Le tout sous le dossier du JOUR, celui où le kernel écrit aussi : un
// incident se raconte à plusieurs voix, et on descend d'un cran pour les lire
// côte à côte.
//
// Détaché, et silencieux en cas d'échec : un journal ne doit jamais retarder
// ni empêcher le démarrage d'une partie. `Follow` rend la main tout seul
// quand le conteneur s'arrête.
func (dockerClient *Client) FollowLogs(containerName string) {
	if logfile.Disabled() {
		return
	}

	go func() {
		file, err := logfile.Session(storageDir, containerName)
		if err != nil || file == nil {
			return
		}
		defer file.Close()

		reader, err := dockerClient.dockerClient.ContainerLogs(context.Background(), containerName,
			container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
		if err != nil {
			return
		}
		defer reader.Close()

		// DÉMULTIPLEXER, sans quoi le fichier est illisible.
		//
		// Un conteneur sans TTY rend un flux MULTIPLEXÉ : chaque ligne est
		// précédée de huit octets d'en-tête qui disent « ceci vient de la
		// sortie standard / de l'erreur, et fait n octets ». Copiés tels
		// quels, ils se retrouvent dans le fichier — qui s'ouvre alors sur
		// des caractères de contrôle avant chaque ligne, et qu'aucun outil de
		// journal ne sait relire.
		//
		// Les deux flux vont dans le MÊME fichier, comme `docker logs` les
		// montre : ce qu'on cherche, c'est l'ordre des événements, et
		// séparer la sortie de l'erreur le détruirait.
		_, _ = stdcopy.StdCopy(file, file, reader)
	}()
}

// storageDir — le dossier de ce serveur, tel qu'il est monté dans ce
// conteneur : les journaux des parties vont dans son `logs/`.
const storageDir = config.StorageDir

// containerLogTail returns the container's last log lines, trimmed — the
// child's fatal message is the only trace of WHY it died once the
// container is gone.
func (dockerClient *Client) containerLogTail(ctx context.Context, containerName string, lines int) string {
	reader, err := dockerClient.dockerClient.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(lines),
	})
	if err != nil {
		return "<logs unavailable>"
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return "<logs unavailable>"
	}
	return strings.TrimSpace(string(content))
}

// AdoptedRemoteplaySession — a remoteplay session container rebuilt from
// its labels at adoption time.
type AdoptedRemoteplaySession struct {
	SessionID string
	Container string
	HostID    string
	HostName  string
	HostType  string
	Owner     string
	Port      int
	// StartedAt — la création du conteneur, c'est-à-dire le lancement de la
	// partie : une partie retrouvée après un redémarrage du kernel garde sa
	// durée (« Live now » l'affiche) au lieu de repartir de zéro.
	StartedAt time.Time
}

// AdoptExistingRemoteplay lists the running remoteplay session containers
// so the manager can rebuild its records after a restart.
func (dockerClient *Client) AdoptExistingRemoteplay(ctx context.Context) ([]AdoptedRemoteplaySession, error) {
	list, err := dockerClient.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: dockerClient.sessionFilters(),
	})
	if err != nil {
		return nil, fmt.Errorf("list session containers: %w", err)
	}

	adopted := make([]AdoptedRemoteplaySession, 0, len(list))
	for _, listed := range list {
		labels := listed.Labels
		if !isRemoteplayContainer(labels) {
			continue
		}

		sessionID := labels[labelSessionID]
		if sessionID == "" {
			continue
		}

		port := 0
		if portText := labels[labelPort]; portText != "" {
			port, _ = strconv.Atoi(portText)
		}

		dockerClient.reserveAdopted(port)
		adopted = append(adopted, AdoptedRemoteplaySession{
			SessionID: sessionID,
			Container: strings.TrimPrefix(listed.Names[0], "/"),
			HostID:    labels[labelHostID],
			HostName:  labels[labelGame],
			HostType:  labels[labelConsole],
			Owner:     labels[labelOwner],
			Port:      port,
			StartedAt: time.Unix(listed.Created, 0),
		})
	}
	return adopted, nil
}

// ContainerMemory — ce que DOCKER compte pour ce conteneur : l'émulateur plus
// tout ce qui l'entoure (encodeur, serveur X, audio). À ne pas confondre avec
// le RSS que l'enfant rapporte de son PROPRE processus : l'écart entre les
// deux est exactement ce qu'on veut voir quand une session gonfle sans que le
// jeu n'y soit pour rien.
//
// Un seul échantillon (`stream=false`) : on ne veut pas d'un flux, juste une
// valeur, et le panneau la redemande à sa cadence. Best-effort — un conteneur
// qui vient de mourir renvoie zéro plutôt qu'une erreur qui ferait échouer
// tout le statut pour une ligne d'affichage.
func (dockerClient *Client) ContainerMemory(ctx context.Context, containerName string) (used int64, limit int64) {
	stats, err := dockerClient.dockerClient.ContainerStatsOneShot(ctx, containerName)
	if err != nil {
		return 0, 0
	}
	defer stats.Body.Close()

	var payload struct {
		MemoryStats struct {
			Usage int64 `json:"usage"`
			Limit int64 `json:"limit"`
			Stats struct {
				// Le cache de fichiers est compté dans `usage` par le noyau,
				// et il n'est PAS de la mémoire que la session retient : le
				// soustraire donne le chiffre que `docker stats` affiche.
				InactiveFile int64 `json:"inactive_file"`
			} `json:"stats"`
		} `json:"memory_stats"`
	}
	if err := json.NewDecoder(stats.Body).Decode(&payload); err != nil {
		return 0, 0
	}

	used = payload.MemoryStats.Usage - payload.MemoryStats.Stats.InactiveFile
	if used < 0 {
		used = 0
	}
	return used, payload.MemoryStats.Limit
}
