// Package config loads the sessions service environment contract. Every
// variable is GAMEFLEX_*-prefixed and defined in docker-compose.yaml only — no
// .env files (see AGENTS.md, "Variables d'environnement et secrets").
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Container-side layout — FIXED contract targeted by every bind mount in
// docker-compose.yaml. Only the HOST-side bind sources are configurable
// (the Docker daemon resolves those on the host, not in this container).
const (
	HandlerPort = "8080"
	// StorageDir — LE DOSSIER DE CE SERVEUR (chantier 24) : tout ce qu'il
	// possède, rangé comme celui du téléphone (StorageFolders). Un seul
	// montage, déclaré par une seule variable, GAMEFLEX_STORAGE_PATH.
	StorageDir  = "/gameflex"
	SavesDir    = "/gameflex/saves"
	ProfilesDir = "/profiles"
	// InternalDisksDir — les RESSOURCES INTERNES (chantiers 23 et 24) :
	// chaque dossier monté dessous par le compose (`<chemin>:/mnt/<nom>`) est
	// un autre disque de cette machine, qui apporte des jeux, des BIOS, des
	// cœurs, des paramètres et des correctifs, rangés comme le dossier.
	InternalDisksDir = "/mnt"
)

// StorageFolders — le rangement du dossier, le même que sur le téléphone
// (mobileapp/kernel/library_tree.go), plus `cache/` (où vit aussi le premier
// lancement d'un jeu, partagé par tous les comptes : `cache/installs/`,
// chantier 22, #448). Le kernel les crée au démarrage : un montage dans une
// partie exige que sa source existe.
var StorageFolders = []string{"databases", "logs", "cache", "saves", "roms", "bios", "cores", "params", "patches"}

// DiskFolders — ce qu'un disque interne apporte (chantier 24). Le reste
// (saves, databases, logs, cache) y est ignoré : ces dossiers-là
// n'appartiennent qu'au dossier de ce serveur.
var DiskFolders = []string{"roms", "bios", "cores", "params", "patches"}

type Config struct {
	// StorageHostDir — le dossier de ce serveur, côté HÔTE : la source que
	// le démon Docker résout pour monter ses morceaux dans une partie
	// (jamais lue dans ce conteneur, qui le voit en StorageDir).
	StorageHostDir string

	// ProfilesHostDir — HOST-side folder (daemon-resolved) holding the
	// console pad profiles (profile.json, see internal/docker/profiles).
	// This service writes them through its own bind mount at ProfilesDir,
	// and each streamex session container bind-mounts its own from here
	// into /opt/profile.json (STREAMEX_INPUT_PROFILE). Empty = no profile
	// mounted (streamex serves its generic {"console","keylab"} fallback
	// and the on-screen pad hides).
	ProfilesHostDir string

	SwitchImage string
	// SwitchEdenImage — Eden, le second émulateur de la Switch (chantier 26) ;
	// SwitchImage porte Citron, le premier.
	SwitchEdenImage string
	PS3Image        string
	XboxImage       string
	PS4Image        string
	PS5Image        string
	WiiUImage       string
	Xbox360Image    string
	KeyTestImage    string
	// Retro sessions — the retrogaming image runs its own binary in
	// standalone child mode (GAMEFLEX_CHILD_PORT), one container per session,
	// with the sections it reads bind-mounted under StorageDir.
	RetroImage string
	// RemoteplayImage is the per-session container image for
	// PlayStation remoteplay (one-container-per-session model — the
	// control plane stays in the remoteplay service).
	RemoteplayImage string
	// RemoteplayControlURL — where the control plane lives for calls
	// that carry NO session id (the device-status input WebSocket).
	// Sessions only fronts data-plane traffic for live sessions.
	RemoteplayControlURL string
	// CredentialURL is the identity service — the kernel calls it to
	// verify the account token another server presents (chantiers 6 et 20),
	// and follows its accounts (internal/accounts). Il possède les fichiers,
	// l'autre possède l'identité : personne ne garde deux copies.
	CredentialURL string

	PortRange string

	MaxPlayers int

	// ConsoleLimits caps concurrent running containers PER CONSOLE — each
	// emulator's footprint differs (Eden is light, RPCS3 heavy), so the
	// budget is a per-console map parsed from GAMEFLEX_CONSOLE_LIMITS
	// ("switch:2,ps3:3,xbox:5").
	ConsoleLimits map[string]int

	// PublicHost is what play/watch URLs are built against — the host the
	// browser can reach, not a container-internal address.
	PublicHost string

	JWTSecret string

	// InternalSecret guards the service-to-service routes (/internal/*) —
	// shared with the platform owners (retro-gaming, remote-play) that
	// mirror their sessions into this service. Generated, never committed.
	InternalSecret string

	// GatewayURL — injected into every spawned session container as
	// GAMEFLEX_GATEWAY_URL: gatewaywebrtc carries every session's media, and
	// no child starts without it (their in-process WebRTC left with #257).
	GatewayURL string

	// GatewayControlURL — où le kernel joint LUI-MÊME la passerelle : pour
	// ouvrir le flux d'un lancement avant que la partie n'existe, puis l'y
	// rattacher (chantier 35, l'intro). La passerelle est en `network_mode:
	// host` et le kernel sur un bridge : c'est l'adresse LAN de l'hôte, comme
	// RemoteplayControlURL — pas GatewayURL, qui est en 127.0.0.1 pour les
	// enfants. Vide : aucun lancement n'est annoncé, la page attend son image
	// sur un fond noir comme avant.
	GatewayControlURL string

	// KernelURL — injected into spawned session containers as
	// GAMEFLEX_KERNEL_URL. Children run with host networking, so it is a
	// 127.0.0.1 URL on the kernel's published host port — never the bridge
	// container name this service itself uses.
	KernelURL string

	// Staging — CETTE INSTALLATION EST UN BANC D'ESSAI (GAMEFLEX_STAGING, le
	// porteur, 2026-10-26 : « qu'on ait une différence dans l'interface entre
	// les deux », la release et la dev). Déclaré par /api/v1/capabilities : la
	// page écrit « staging » à côté du nom, et son lien de téléchargement de
	// l'application mène aux APK que cette installation construit
	// (public/downloads) au lieu de la Release GitHub. Ouvert à toute
	// installation, pas seulement à la nôtre. Remplace GAMEFLEX_APP_DOWNLOAD.
	Staging bool

	// MirrorURL — LE MIROIR (chantier 29, le porteur, 2026-10-24) : un lien
	// EXTERNE, sans rapport avec le projet, où l'administrateur télécharge
	// lui-même des émulateurs, des firmwares, des correctifs et des
	// paramètres, rangés comme son dossier (`cores/`, `bios/`, `patches/`,
	// `params/`). Déclaré à la page par /api/v1/capabilities, qui le montre
	// dans ces quatre écrans ; le serveur ne le lit jamais — ni liste, ni
	// téléchargement. Vide : rien ne le propose.
	MirrorURL string

	// StackName — le nom que portent les conteneurs de partie de CETTE pile
	// (étiquette gameflex.stack). Deux piles sur un même démon Docker — la
	// pile de dev et une pile de release à côté, ou deux instances — n'adoptent,
	// ne balaient et ne listent que les leurs. Voir internal/docker/stack.go.
	StackName string

	// GPU — l'adresse PCI du GPU que reçoivent les parties (GAMEFLEX_GPU,
	// « 0000:c4:00.0 ») quand le serveur en a plusieurs ; vide, le premier
	// dans l'ordre PCI. Voir sessionstatus.SessionGPU (chantier 39).
	GPU string
}

func Load() (Config, error) {
	maxPlayers, err := strconv.Atoi(getenv("GAMEFLEX_MAX_PLAYERS", "4"))
	if err != nil {
		return Config{}, fmt.Errorf("GAMEFLEX_MAX_PLAYERS: %w", err)
	}
	mirror, err := mirrorURL(getenv("GAMEFLEX_MIRROR_URL", ""))
	if err != nil {
		return Config{}, err
	}
	// L'ADRESSE DE CETTE MACHINE, SANS VALEUR PAR DÉFAUT (chantier 29, C1).
	// Le défaut était l'adresse de la machine de développement : une
	// installation qui oubliait la variable joignait ses parties chez nous, ou
	// nulle part, sans rien dire. Tous les compose la passent.
	publicHost := getenv("GAMEFLEX_PUBLIC_HOST", "")
	if publicHost == "" {
		return Config{}, fmt.Errorf("GAMEFLEX_PUBLIC_HOST manquante : l'adresse de cette machine sur le réseau local")
	}

	return Config{
		StorageHostDir:  getenv("GAMEFLEX_STORAGE_PATH", ""),
		ProfilesHostDir: getenv("GAMEFLEX_PROFILES_HOST_DIR", ""),
		SwitchImage:     getenv("GAMEFLEX_SWITCH_IMAGE", "streamex-switch:latest"),
		SwitchEdenImage: getenv("GAMEFLEX_SWITCH_EDEN_IMAGE", "streamex-switch-eden:latest"),
		PS3Image:        getenv("GAMEFLEX_PS3_IMAGE", "streamex-ps3:latest"),
		XboxImage:       getenv("GAMEFLEX_XBOX_IMAGE", "streamex-xbox:latest"),
		PS4Image:        getenv("GAMEFLEX_PS4_IMAGE", "streamex-ps4:latest"),
		PS5Image:        getenv("GAMEFLEX_PS5_IMAGE", "streamex-ps5:latest"),
		WiiUImage:       getenv("GAMEFLEX_WIIU_IMAGE", "streamex-wiiu:latest"),
		Xbox360Image:    getenv("GAMEFLEX_XBOX360_IMAGE", "streamex-xbox360:latest"),
		// Vide par défaut : keytest ne part pas dans une installation publique,
		// et une console sans image configurée n'est pas installée (#136).
		KeyTestImage:         getenv("GAMEFLEX_KEYTEST_IMAGE", ""),
		StackName:            getenv("GAMEFLEX_STACK_NAME", "gameflex"),
		GPU:                  getenv("GAMEFLEX_GPU", ""),
		RetroImage:           getenv("GAMEFLEX_RETRO_IMAGE", "gameflex-retrogaming:latest"),
		RemoteplayImage:      getenv("GAMEFLEX_REMOTEPLAY_IMAGE", "gameflex-remoteplay:latest"),
		RemoteplayControlURL: getenv("GAMEFLEX_REMOTEPLAY_CONTROL_URL", "http://localhost:10205"),
		CredentialURL:        getenv("GAMEFLEX_CREDENTIAL_URL", ""),
		PortRange:            getenv("GAMEFLEX_PORT_RANGE", "9000-9100"),
		MaxPlayers:           maxPlayers,
		ConsoleLimits:        parseConsoleLimits(getenv("GAMEFLEX_CONSOLE_LIMITS", "switch:2,ps3:3,xbox:5,keytest:3")),
		PublicHost:           publicHost,
		JWTSecret:            getenv("GAMEFLEX_JWT_SECRET", ""),
		InternalSecret:       os.Getenv("GAMEFLEX_INTERNAL_SECRET"),
		GatewayURL:           os.Getenv("GAMEFLEX_GATEWAY_URL"),
		GatewayControlURL:    os.Getenv("GAMEFLEX_GATEWAY_CONTROL_URL"),
		KernelURL:            getenv("GAMEFLEX_KERNEL_URL", "http://127.0.0.1:10201"),
		Staging:              staging(os.Getenv("GAMEFLEX_STAGING")),
		MirrorURL:            mirror,
	}, nil
}

// staging — « 1 » ou « true » (sans casse) : un banc d'essai. Tout le reste,
// vide compris : une installation ordinaire.
func staging(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true":
		return true
	}

	return false
}

// mirrorURL — le miroir est une adresse http(s), ou rien : la page en fait un
// lien, et une autre forme (`javascript:`, un chemin) n'a rien à y faire. Une
// valeur fausse arrête le démarrage plutôt que de disparaître sans le dire.
func mirrorURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("GAMEFLEX_MIRROR_URL : une adresse http(s) attendue, pas %q", raw)
	}

	return raw, nil
}

// SavesHostDir — `saves/` du dossier, côté hôte.
func (config Config) SavesHostDir() string {
	return filepath.Join(config.StorageHostDir, "saves")
}

// StorageHostSection — une section du dossier (`bios`, `patches`…), côté hôte.
func (config Config) StorageHostSection(section string) string {
	return filepath.Join(config.StorageHostDir, section)
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// parseConsoleLimits reads "console:max,console:max" — invalid entries are
// skipped, an unset console simply stays unlimited.
func parseConsoleLimits(raw string) map[string]int {
	limits := make(map[string]int)
	for _, entry := range strings.Split(raw, ",") {
		console, maxText, found := strings.Cut(strings.TrimSpace(entry), ":")
		if !found {
			continue
		}
		maximum, err := strconv.Atoi(strings.TrimSpace(maxText))
		if err != nil || maximum < 0 {
			continue
		}
		limits[strings.TrimSpace(console)] = maximum
	}
	return limits
}
