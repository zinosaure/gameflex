package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gameflex/libretro"
)

// LE CŒUR SE DÉCHARGE QUAND SON FIL N'EST PLUS (2026-10-14).
//
// Le téléphone mourait d'un SIGSEGV en quittant une partie PSP, une
// milliseconde après « partie arrêtée, cœur déchargé » : le 2026-10-11 (#317,
// #328), puis le 2026-10-14 sur le S25 avec Crisis Core. Android n'en garde que
// le signal (« tuée par un signal », statut 11), sans pile.
//
// Le fil du moteur est verrouillé et jamais rendu : Go le TERMINE quand la
// partie finit (voir l'en-tête de ce paquet). Un fil qui se termine exécute les
// destructeurs que le code a inscrits pour lui — les variables par fil du C++
// (`thread_local`), les clés de pthread —, et un cœur comme PPSSPP en inscrit
// sur le fil qui l'exécute. Or le cœur était déchargé (dlclose) sur ce même fil,
// juste avant sa fin : ces destructeurs sautaient dans un code qui n'était plus
// en mémoire. Le serveur ne le voyait pas, ou sans conséquence : une partie y
// est un processus, qui s'arrête avec elle.
//
// L'ordre devient : le fil du moteur arrête le cœur (retro_unload_game,
// retro_deinit) et rend son contexte graphique, puis se termine — ses
// destructeurs tournent, le code du cœur encore là ; un autre fil attend sa fin
// et seulement ALORS décharge le cœur. S'il ne finit pas, le cœur reste en
// mémoire, oublié : rien ne le déchargera plus, rien ne sautera dans le vide.
//
// ET LES SIGNAUX, À CETTE FIN (2026-10-15, suite). Ça n'a pas suffi : le S25
// mourait encore entre « partie arrêtée » et la fin du fil — avant le dlclose,
// donc —, sans rapport natif ni tombstone. Go bloque TOUS les signaux d'un fil
// avant de le rendre à la libc (mexit) ; ces destructeurs tournent donc
// SIGSEGV fermé, et une faute dans l'un d'eux, même une que son auteur
// rattrape, tue sans appeler personne. Le fil inscrit en dernier un destructeur
// à lui, qui passe avant tous et rouvre les signaux synchrones (voir
// UnblockFaultsAtThreadExit dans le pont) ; ce qu'il a trouvé est dit à la
// ligne « le fil du moteur a fini ».

// gettid — le numéro système du fil appelant.
func gettid() int {
	return syscall.Gettid()
}

// LE FIL PRINCIPAL DU RUNTIME NE SE TERMINE JAMAIS (2026-10-15).
//
// Go termine le fil d'une goroutine qui finit verrouillée — SAUF le fil
// principal de son runtime (m0) : celui-là, il le gare pour toujours. Une
// goroutine du moteur tombée par hasard sur m0 laissait donc son fil vivant :
// unloadAfter l'attendait cinq secondes, puis oubliait le cœur en mémoire
// (« le fil du moteur vit encore après 5s »). Sur le serveur, un arrêt de
// partie rallongé de cinq secondes, au hasard (GameCube, GBA, 2026-10-14) ; sur
// le téléphone, un cœur jamais déchargé, au plus une fois par vie de
// l'application — m0 garé ne sert plus. Vérifié par une expérience : une
// goroutine verrouillée sur m0 qui finit, son fil reste.
//
// mainThread est noté à l'initialisation du paquet : elle se fait sur m0, sa
// goroutine verrouillée dessus (runtime.main), pour un programme comme pour une
// bibliothèque (gomobile).
var mainThread = gettid()

// onOwnThread exécute body sur un fil système verrouillé qui n'est PAS m0, et
// que Go terminera quand body aura fini. Appelée sur m0, la goroutine garde m0
// verrouillé le temps qu'une autre verrouille son propre fil — elle ne peut donc
// pas tomber sur m0 —, puis le rend et revient tout de suite : body continue
// là-bas.
func onOwnThread(body func()) {
	runtime.LockOSThread()
	if gettid() != mainThread {
		body()

		return
	}
	locked := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		close(locked)
		body()
	}()
	<-locked
	runtime.UnlockOSThread()
}

// LES FILS QUI SURVIVENT À LA PARTIE (2026-10-15, #356). Un cœur qui laisse
// un de ses fils vivant après retro_deinit le fait exécuter du code retiré dès
// le dlclose : le téléphone mourait au déchargement de PPSSPP. Le nom de chaque
// fil (/proc/self/task/*/comm), compté, est noté avant le cœur ; au
// déchargement, ce qui est né depuis et vit encore est dit au journal.

// threadCensus — combien de fils portent chaque nom, maintenant.
func threadCensus() map[string]int {
	census := map[string]int{}
	entries, err := os.ReadDir("/proc/self/task")
	if err != nil {
		return census
	}
	for _, entry := range entries {
		name, err := os.ReadFile("/proc/self/task/" + entry.Name() + "/comm")
		if err == nil {
			census[strings.TrimSpace(string(name))]++
		}
	}

	return census
}

// bornSince — les noms dont le compte a grandi depuis `before`, « nom ×n »,
// triés ; "" si aucun.
func bornSince(before map[string]int) string {
	var born []string
	for name, count := range threadCensus() {
		if extra := count - before[name]; extra > 0 {
			born = append(born, fmt.Sprintf("%s ×%d", name, extra))
		}
	}
	sort.Strings(born)

	return strings.Join(born, ", ")
}

// exitSignals — ce que la fin du dernier fil du moteur a trouvé, pour le
// journal.
func exitSignals() string {
	switch libretro.ThreadExitFoundFaultsBlocked() {
	case "oui":
		return "Go avait fermé SIGSEGV pour sa fin : rouvert avant ses destructeurs"
	case "non":
		return "SIGSEGV était ouvert à sa fin"
	}

	return "sa fin n'a pas rouvert les signaux : destructeur non vu"
}

// threadGone — ce fil du processus a-t-il fini ? Faux là où /proc ne répond
// pas comme attendu : dans le doute, il vit.
func threadGone(tid int) bool {
	_, err := os.Stat("/proc/self/task/" + strconv.Itoa(tid))

	return errors.Is(err, fs.ErrNotExist)
}

// unloadAfter — sur un autre fil que celui du moteur : attend la fin de
// celui-ci (`tid`), décharge le cœur, puis seulement libère le moteur — Stop et
// Load, qui attendent `done`, rendent donc la main le cœur déchargé.
func (e *Engine) unloadAfter(tid int) {
	start := time.Now()
	for !threadGone(tid) && time.Since(start) < 5*time.Second {
		time.Sleep(2 * time.Millisecond)
	}
	waited := time.Since(start).Round(time.Millisecond)
	if e.coreLoaded {
		if threadGone(tid) {
			// AVANT le déchargement aussi : une mort entre « partie arrêtée » et
			// « cœur déchargé » se situe alors de part et d'autre de cette ligne
			// — la fin du fil, ou le dlclose (#356, 2026-10-15).
			libretro.ClearThreadExitTrace()
			log.Printf("[engine] le fil du moteur a fini en %v — déchargement du cœur (%s)", waited, exitSignals())
			if alive := bornSince(e.threadsAtStart); alive != "" {
				log.Printf("[engine] fils nés pendant la partie, encore vivants au déchargement : %s", alive)
			}
			libretro.UnloadCore()
			log.Printf("[engine] cœur déchargé — son fil a fini en %v", waited)
		} else {
			libretro.ForgetCore()
			log.Printf("[engine] le fil du moteur vit encore après %v : le cœur reste en mémoire, oublié", waited)
		}
	}
	running.Store(false)
	close(e.done)
}
