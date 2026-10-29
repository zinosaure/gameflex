// Package sessionstatus tient LA forme du panneau de performances — une
// seule, pour les trois plateformes et pour le téléphone.
//
// Elle n'existait pas, et ça se voyait : le rétro renvoyait `cpu_percent`,
// `rss_bytes`, `gpu.vram_used_bytes` ; streamex sa propre variante avec des
// `rates`/`latency`/`input` imbriqués ; remoteplay une troisième. Le panneau,
// lui, lisait `cpuPercent`, `rssBytes`, `vramUsedBytes` — c'est-à-dire aucune
// des trois. Toutes ces lignes affichaient donc un tiret alors que la mesure
// était bel et bien là, calculée, transmise, et jetée faute d'un nom commun
// (constaté le 2026-09-08 sur une vraie session : le serveur envoyait
// 13 393 920 octets de RSS, l'écran affichait « — »).
//
// Une plateforme ne remplit que ce qu'elle sait. Tous les champs mesurés sont
// donc `omitempty` : absent veut dire « pas mesurable ici », et le panneau
// affiche un tiret — ce qui est vrai, et différent de zéro.
package sessionstatus

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Status — ce qu'une session dit d'elle-même quand on ouvre le panneau.
//
// Les noms sont en camelCase, comme la façade (`playerCount`, `padSlot`) et
// comme ce que le panneau lisait déjà : c'est le CONSOMMATEUR qui fixe la
// forme, il n'y en a qu'un.
type Status struct {
	// StartedAt — l'heure de démarrage, jamais une durée déjà calculée.
	//
	// « En cours depuis 00:07:12 » se périme en une seconde : envoyer la
	// durée obligerait à repoller pour que le compteur avance, et il
	// avancerait par à-coups de deux secondes. L'instant de départ, lui, ne
	// se périme jamais — la page compte toute seule, à sa propre cadence.
	StartedAt time.Time `json:"startedAt,omitempty"`

	// Emulator — l'émulateur qui joue la partie et sa version (« Eden nightly
	// de7e6e01d4 », « shadPS4 0.19.0 (GameFlex) », un cœur libretro par son
	// `library_name` et son `library_version`). Vide pour Remote Play, qui
	// n'émule rien, et partout où on ne le sait pas.
	Emulator string `json:"emulator,omitempty"`

	// Host — la machine qui fait tourner la partie : son GPU, son processeur,
	// sa mémoire (Resources). Celle d'un serveur fédéré quand on joue chez
	// lui — c'est lui qui répond.
	Host *Resources `json:"host,omitempty"`

	// Qui est là. Players compte les manettes tenues, Spectators ceux qui
	// regardent.
	Players    int `json:"players"`
	Spectators int `json:"spectators"`

	// EmulationFPS — la cadence de l'ÉMULATION, pas celle du flux vidéo (que
	// le navigateur mesure lui-même de son côté). Les deux peuvent diverger,
	// et c'est justement quand elles divergent que ce panneau sert.
	EmulationFPS float64 `json:"emulationFps,omitempty"`

	// CPUPercent — fraction d'UN cœur, donc au-dessus de 100 sur plusieurs.
	//
	// UN POINTEUR, ET C'EST LE SUJET : `omitempty` sur un nombre confond
	// « zéro » et « pas mesuré ». Une charge GPU réellement à 0 % disparaissait
	// donc du message et s'affichait « — », c'est-à-dire « on ne sait pas »,
	// alors qu'on savait très bien (vu le 2026-09-08 : le serveur mesurait 2 %,
	// puis 0 % l'instant d'après, et l'écran passait de « 2 % » à « — »).
	// nil veut dire absent, 0 veut dire zéro, et les deux se disent
	// différemment.
	CPUPercent *float64 `json:"cpuPercent,omitempty"`

	// RSSBytes — la mémoire de l'ÉMULATEUR lui-même (son processus).
	// ContainerRSSBytes — celle du CONTENEUR entier, telle que Docker la
	// compte : l'émulateur plus l'encodeur, le serveur X, l'audio. Les deux,
	// parce que l'écart entre elles est précisément ce qu'on veut voir quand
	// une session gonfle sans que le jeu n'y soit pour rien.
	RSSBytes          int64 `json:"rssBytes,omitempty"`
	ContainerRSSBytes int64 `json:"containerRssBytes,omitempty"`
	// ContainerLimitBytes — le plafond mémoire imposé au conteneur, quand il
	// y en a un. Sans lui, « 900 Mo » ne dit pas s'il reste de la marge.
	ContainerLimitBytes int64 `json:"containerLimitBytes,omitempty"`

	// Le GPU est celui de l'HÔTE, partagé par toutes les sessions — pas une
	// mesure par partie. C'est voulu : ce qui sature, c'est la carte.
	GPUBusyPercent *float64 `json:"gpuBusyPercent,omitempty"`
	VRAMUsedBytes  int64    `json:"vramUsedBytes,omitempty"`
	VRAMTotalBytes int64    `json:"vramTotalBytes,omitempty"`

	// DiskReadBytesPerSec — ce que l'émulateur lit sur le DISQUE, par
	// seconde : `read_bytes` de /proc/<pid>/io (ReadBytes), les octets que le
	// noyau a vraiment demandés au stockage, pas ceux que le cache rendait
	// déjà. Le porteur, 2026-10-27 : « voir la vitesse à laquelle le jeu se
	// copie » — FF7 Remake démarrait en trois minutes sur le disque USB du
	// S25, et rien ne disait si le disque y était pour quelque chose. Un
	// pointeur, pour la raison de CPUPercent : un disque au repos lit 0.
	DiskReadBytesPerSec *float64 `json:"diskReadBytesPerSec,omitempty"`

	// La température (°C) et la batterie (%) de la machine qui joue — voir
	// power.go. 100 % pour un serveur sur secteur.
	TemperatureCelsius *float64 `json:"temperatureCelsius,omitempty"`
	BatteryPercent     *float64 `json:"batteryPercent,omitempty"`

	// Start — la première mesure de la partie (Baselines) : la page en tire
	// l'écart entre parenthèses.
	Start *Baseline `json:"start,omitempty"`
}

// ReadBytes — `read_bytes` de /proc/<pid>/io : ce que le processus a lu sur
// le disque depuis sa naissance. Faux quand le fichier ne se lit pas (un
// noyau sans comptabilité des entrées-sorties, un processus d'un autre
// utilisateur).
func ReadBytes(pid int) (int64, bool) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/io")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if value, found := strings.CutPrefix(line, "read_bytes: "); found {
			bytes, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)

			return bytes, err == nil
		}
	}

	return 0, false
}

// Percent emballe une mesure en pourcentage pour les champs à pointeur —
// voir CPUPercent : c'est ce qui distingue « zéro » de « pas mesuré ».
func Percent(value float64) *float64 { return &value }
