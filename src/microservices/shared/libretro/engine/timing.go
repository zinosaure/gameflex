package engine

import (
	"log"
	"math"
	"time"

	"gameflex/libretro"
)

// LA CADENCE QUE LE CŒUR ANNONCE EN COURS DE PARTIE (SET_SYSTEM_AV_INFO).
//
// Un cœur ne sait pas toujours au chargement à quelle cadence son jeu tourne.
// LRPS2 se charge à 59,94 (le mode du BIOS) et n'annonce 50 qu'une fois qu'un
// jeu PAL a posé son mode vidéo. Chaque retro_run émulant UNE image de la
// console, c'est la boucle du moteur qui fixe la vitesse du jeu : restée à
// 59,94, elle faisait tourner les jeux PAL 20 % trop vite — Tekken 5 en
// accéléré — et donnait 20 % de son de trop à l'encodeur, réglé sur
// l'horloge : Chrome en jetait un paquet sur six et gardait 0,8 s de retard
// (Kingdom Hearts II, le porteur, 2026-11-14). La commande tombait dans les
// « sans réponse », dite une seule fois : la bascule vers 50 ne se voyait pas.
//
// L'annonce est notée par le rappel (environment) et appliquée par la boucle,
// sur le fil du moteur, entre deux images : la cadence de la boucle, la part
// des images rendues (OutputFPS) et le son la suivent. Le son, lui, sort
// toujours à la fréquence du chargement — celle sur laquelle l'appelant a
// ouvert son encodeur : une autre fréquence du cœur y est ramenée (soundRate).
//
// Ce qui ne suit pas, NYI : une taille maximale d'image plus grande que celle
// du chargement. Le contexte graphique garde celle-là (openGL).

// noteTiming — appelé par SET_SYSTEM_AV_INFO, d'où que le cœur l'appelle : la
// boucle l'appliquera (applyTiming).
func (e *Engine) noteTiming(timing libretro.SystemTiming) {
	e.mu.Lock()
	e.announcedTiming = &timing
	e.mu.Unlock()
}

// applyTiming — sur le fil du moteur, entre deux images : la dernière cadence
// annoncée devient celle de la boucle. Une cadence absurde est refusée (la
// même borne qu'au chargement), une fréquence nulle laisse la précédente.
func (e *Engine) applyTiming() {
	e.mu.Lock()
	timing := e.announcedTiming
	e.announcedTiming = nil
	if timing == nil {
		e.mu.Unlock()

		return
	}
	fps, rate := timing.FPS, timing.SampleRate
	if fps <= 0 || fps > 240 {
		e.mu.Unlock()
		log.Printf("[engine] cadence annoncée par le cœur refusée : %g images par seconde", timing.FPS)

		return
	}
	if rate <= 0 {
		rate = e.coreSampleRate
	}
	previousFPS, previousRate := e.fps, e.coreSampleRate
	if math.Abs(fps-previousFPS) < 0.001 && rate == previousRate {
		e.mu.Unlock()

		return
	}
	e.fps = fps
	e.mu.Unlock()

	e.frameTime = time.Duration(float64(time.Second) / fps)
	e.coreSampleRate = rate
	e.sound.convert(rate, e.info.Timing.SampleRate)
	log.Printf("[engine] le cœur change de cadence : %.2f → %.2f images par seconde, son %.0f → %.0f Hz (rendu à %.0f Hz)",
		previousFPS, fps, previousRate, rate, e.info.Timing.SampleRate)
}

// soundRate — le son du cœur ramené à la fréquence de l'encodeur, quand le
// cœur en a changé : une interpolation linéaire entre ses échantillons. Sans
// changement (le cas de LRPS2, 48000 Hz en PAL comme en NTSC), le son passe
// tel quel.
type soundRate struct {
	// step — échantillons du cœur par échantillon rendu ; 0 : rien à convertir.
	step float64
	// position — où tombe le prochain échantillon rendu, en échantillons du
	// cœur comptés depuis `previous`.
	position float64
	// previous — le dernier échantillon stéréo du lot d'avant : le premier
	// échantillon rendu d'un lot s'interpole avec lui.
	previous [2]int16
}

// convert règle la conversion d'une fréquence du cœur vers celle de sortie.
func (sound *soundRate) convert(core, output float64) {
	sound.step = 0
	if core > 0 && output > 0 && core != output {
		sound.step = core / output
	}
}

// resample rend un lot stéréo entrelacé à la fréquence de sortie.
func (sound *soundRate) resample(samples []int16) []int16 {
	frames := len(samples) / 2
	if frames == 0 {
		return samples
	}
	previous := sound.previous
	sound.previous = [2]int16{samples[2*frames-2], samples[2*frames-1]}
	if sound.step == 0 {
		return samples
	}

	// L'échantillon `index` de la suite : 0 est celui du lot d'avant.
	sample := func(index, channel int) float64 {
		if index == 0 {
			return float64(previous[channel])
		}

		return float64(samples[2*(index-1)+channel])
	}
	resampled := make([]int16, 0, 2*(int(float64(frames)/sound.step)+1))
	for ; sound.position < float64(frames); sound.position += sound.step {
		index := int(sound.position)
		fraction := sound.position - float64(index)
		for channel := 0; channel < 2; channel++ {
			resampled = append(resampled, int16(sample(index, channel)*(1-fraction)+sample(index+1, channel)*fraction))
		}
	}
	sound.position -= float64(frames)

	return resampled
}
