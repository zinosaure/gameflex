package engine

import (
	"log"
	"time"

	"gameflex/libretro"
)

// LE REMBOBINAGE — un anneau borné d'états COMPLETS (libretro.Serialize),
// pris une fois par seconde d'horloge pendant le jeu, rejoués un par seconde
// tant qu'on le tient. Le vrai retour image par image (celui de RetroArch)
// demande des différences de mémoire par image : un autre chantier. Ici, un
// pas visible par seconde, sur une vingtaine de secondes d'historique.
const (
	rewindCaptureInterval = 1 * time.Second
	rewindStepInterval    = 1 * time.Second
	rewindBufferMax       = 20
)

// LE REMBOBINAGE N'EST PAS GRATUIT, ET IL NE VA PAS À TOUTES LES CONSOLES.
//
// Chaque capture sérialise l'état complet, sur le fil du moteur, entre deux
// images. Un état NES fait 14 Ko, un état PS1 4,3 Mo, un état PS2 ou PSP en
// fait CINQUANTE : à une capture par seconde, la boucle prendrait du retard
// une fois par seconde, et vingt instantanés pèseraient un gigaoctet. D'où :
//
//   - rewindStateSizeMax et rewindCaptureBudget, mesurés sur la PREMIÈRE
//     capture réelle — pas avant : dolphin déréférence un pointeur nul si on
//     lui demande la taille de son état avant qu'il ait tourné (2026-09-08) ;
//   - rewindBufferBytesMax borne la mémoire totale : le nombre d'instantanés
//     gardés devient le plus petit des deux.
const (
	rewindStateSizeMax   = 32 << 20  // 32 Mio
	rewindBufferBytesMax = 192 << 20 // 192 Mio d'historique
	rewindCaptureBudget  = 5 * time.Millisecond
)

// rewindState — sur le fil du moteur, sauf `supported`, lu sous e.mu.
type rewindState struct {
	buffer       [][]byte
	lastCapture  time.Time
	lastStep     time.Time
	evaluated    bool
	decided      bool // comme supported : lu sous e.mu
	supported    bool
	snapshotsMax int
}

// captureRewind prend un instantané au plus une fois par seconde, depuis la
// boucle — jamais pendant un retro_run().
func (e *Engine) captureRewind() {
	e.mu.Lock()
	if e.rewind.evaluated && !e.rewind.supported {
		e.mu.Unlock()

		return
	}
	e.mu.Unlock()
	if time.Since(e.rewind.lastCapture) < rewindCaptureInterval {
		return
	}
	e.rewind.lastCapture = time.Now()

	// On CHRONOMÈTRE : un état compact mais lent à sérialiser coûterait le
	// même hoquet par seconde sans que sa taille ne l'ait annoncé.
	started := time.Now()
	data, err := libretro.Serialize()
	if err != nil {
		return
	}
	elapsed := time.Since(started)

	if !e.rewind.evaluated {
		e.rewind.evaluated = true
		e.decideRewind(len(data), elapsed)
	}

	e.mu.Lock()
	supported := e.rewind.supported
	if supported && elapsed > rewindCaptureBudget {
		// Une capture qui se met à coûter trop cher en cours de partie coupe
		// le rembobinage, plutôt que de laisser la partie en payer le prix.
		e.rewind.supported = false
		e.rewinding = false
		supported = false
		log.Printf("[engine] rembobinage désactivé : la capture a pris %s (budget %s)",
			elapsed.Round(100*time.Microsecond), rewindCaptureBudget)
		e.mu.Unlock()
		e.rewind.buffer = nil
		e.announceRewind(false)

		return
	}
	e.mu.Unlock()
	if !supported {
		e.rewind.buffer = nil

		return
	}

	e.rewind.buffer = append(e.rewind.buffer, data)
	if len(e.rewind.buffer) > e.rewind.snapshotsMax {
		e.rewind.buffer = e.rewind.buffer[1:]
	}
}

// decideRewind tranche, une fois par partie, sur la capture qu'on vient de
// faire : sa taille et sa durée.
func (e *Engine) decideRewind(size int, elapsed time.Duration) {
	supported := size > 0 && size <= rewindStateSizeMax && elapsed <= rewindCaptureBudget
	snapshots := rewindBufferMax
	if supported {
		if budget := rewindBufferBytesMax / size; budget < snapshots {
			snapshots = budget
		}
		if snapshots < 1 {
			snapshots = 1
		}
	}

	e.mu.Lock()
	e.rewind.supported = supported
	e.rewind.decided = true
	e.mu.Unlock()
	e.rewind.snapshotsMax = snapshots

	mib := float64(size) / (1 << 20)
	took := elapsed.Round(100 * time.Microsecond)
	switch {
	case size > rewindStateSizeMax:
		log.Printf("[engine] rembobinage indisponible (%s) : état de %.1f Mio, au-dessus du plafond de %d Mio",
			e.config.ConsoleID, mib, rewindStateSizeMax>>20)
	case elapsed > rewindCaptureBudget:
		log.Printf("[engine] rembobinage indisponible (%s) : la capture a pris %s (budget %s)",
			e.config.ConsoleID, took, rewindCaptureBudget)
	default:
		log.Printf("[engine] rembobinage disponible (%s) : état de %.2f Mio capturé en %s, %d instantanés gardés",
			e.config.ConsoleID, mib, took, snapshots)
	}

	e.announceRewind(supported)
}

func (e *Engine) announceRewind(supported bool) {
	if e.config.OnRewindSupport != nil {
		e.config.OnRewindSupport(supported)
	}
}

// stepRewind recharge l'instantané le plus récent, au plus une fois par
// seconde, et rejoue une image depuis ce point. L'historique épuisé, l'image
// reste figée jusqu'à ce qu'on lâche.
func (e *Engine) stepRewind() {
	if time.Since(e.rewind.lastStep) < rewindStepInterval || len(e.rewind.buffer) == 0 {
		return
	}
	last := len(e.rewind.buffer) - 1
	data := e.rewind.buffer[last]
	e.rewind.buffer = e.rewind.buffer[:last]
	e.rewind.lastStep = time.Now()

	if err := libretro.Unserialize(data); err != nil {
		log.Printf("[engine] rembobinage : rechargement impossible : %v", err)

		return
	}
	e.emitFrame = true
	libretro.Run()
}
