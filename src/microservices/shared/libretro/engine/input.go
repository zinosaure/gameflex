package engine

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Les périphériques et identifiants que le moteur sait répondre (libretro.h).
const (
	deviceJoypad  = 1 // RETRO_DEVICE_JOYPAD
	deviceAnalog  = 5 // RETRO_DEVICE_ANALOG
	devicePointer = 6 // RETRO_DEVICE_POINTER

	joypadIDMask = 256 // RETRO_DEVICE_ID_JOYPAD_MASK

	pointerIDX     = 0
	pointerIDY     = 1
	pointerIDPress = 2
	pointerIDCount = 3
)

// MaxPorts — le plus de manettes qu'une partie tient (la Dreamcast et la
// Saturn en veulent quatre, la Wii U deux ; huit laisse de la marge).
const MaxPorts = 8

type touch struct {
	x, y    int16
	pressed bool
}

// inputs — l'état des manettes : écrit par le réseau (le canal de données de
// la page), lu par le cœur sur le fil du moteur. D'où le verrou, le seul du
// moteur.
//
// Mêmes formes et même sens que l'état du serveur (stream_session.go,
// handleInput et inputStateCallback) : la page envoie le même lot aux deux.
type inputs struct {
	mu      sync.Mutex
	buttons [MaxPorts][32]int16
	analog  [MaxPorts][2][2]int16
	touches [MaxPorts]touch
}

// apply lit un lot du canal de données — `[{button,value}]`, `[{axis,x,y}]`,
// `[{pointer,x,y}]`, mélangés — pour la manette `port` (0 pour le joueur qui
// a lancé la partie).
func (in *inputs) apply(port int, batch []byte) error {
	var items []struct {
		ButtonID *uint32 `json:"button,omitempty"`
		Value    int16   `json:"value,omitempty"`
		Axis     *uint32 `json:"axis,omitempty"`
		X        int16   `json:"x,omitempty"`
		Y        int16   `json:"y,omitempty"`
		// 1 = le doigt touche, 0 = il est parti.
		Pointer *int16 `json:"pointer,omitempty"`
	}
	if err := json.Unmarshal(batch, &items); err != nil {
		return fmt.Errorf("engine: lot de manette illisible : %w", err)
	}
	if port < 0 || port >= MaxPorts {
		return fmt.Errorf("engine: manette %d hors des %d ports", port, MaxPorts)
	}

	in.mu.Lock()
	defer in.mu.Unlock()
	for _, item := range items {
		switch {
		case item.Pointer != nil:
			// La POSITION reste au relâchement : un cœur lit souvent X/Y une
			// image après PRESSED, et la remettre à zéro ferait sauter le
			// stylet au centre de l'écran juste avant qu'on le lève.
			in.touches[port] = touch{x: item.X, y: item.Y, pressed: *item.Pointer != 0}
		case item.Axis != nil:
			if *item.Axis < 2 {
				in.analog[port][*item.Axis][0] = item.X
				in.analog[port][*item.Axis][1] = item.Y
			}
		case item.ButtonID != nil:
			if int(*item.ButtonID) < len(in.buttons[port]) {
				in.buttons[port][*item.ButtonID] = item.Value
			}
		}
	}

	return nil
}

// state répond à input_state : ce que vaut, maintenant, le bouton `id` de la
// manette `port`.
func (in *inputs) state(port, device, index, id uint32) int16 {
	in.mu.Lock()
	defer in.mu.Unlock()
	if port >= MaxPorts {
		return 0
	}

	switch device {
	case deviceAnalog:
		if index < 2 && id < 2 {
			return in.analog[port][index][id]
		}
		return 0
	case devicePointer:
		// UN SEUL DOIGT : la page envoie le pointeur qu'elle suit, pas une
		// liste. Répondre 0 pour index=1 est la réponse juste.
		if index != 0 {
			return 0
		}
		finger := in.touches[port]
		switch id {
		case pointerIDX:
			return finger.x
		case pointerIDY:
			return finger.y
		case pointerIDPress, pointerIDCount:
			if finger.pressed {
				return 1
			}
		}
		return 0
	}

	if id == joypadIDMask {
		// Les seize vrais boutons (RETRO_DEVICE_ID_JOYPAD_*) en un masque.
		var mask int16
		for button := 0; button < 16; button++ {
			if in.buttons[port][button] != 0 {
				mask |= 1 << button
			}
		}
		return mask
	}
	if int(id) < len(in.buttons[port]) {
		return in.buttons[port][id]
	}
	return 0
}

// clear relâche tout sur une manette.
func (in *inputs) clear(port int) {
	if port < 0 || port >= MaxPorts {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	in.buttons[port] = [32]int16{}
	in.analog[port] = [2][2]int16{}
	in.touches[port] = touch{}
}

// forceCombo tient L1+R1+SELECT sur la manette 1 — la bascule en mode
// analogique de pcsx_rearmed, une demi-seconde au démarrage d'une PS1 —, ou
// relâche tout quand `on` est faux.
//
// TOUT LE PORT EST REMIS À ZÉRO D'ABORD : la détection de pcsx_rearmed est
// une égalité EXACTE de masque (`ret == in_dualshock_analog_combo`), et le
// moindre autre bouton tenu la fait échouer — c'est ce qui l'empêchait de
// prendre, même tenue à la main plusieurs secondes.
func (in *inputs) forceCombo(on bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.buttons[0] = [32]int16{}
	if on {
		in.buttons[0][2] = 1  // SELECT
		in.buttons[0][10] = 1 // L1
		in.buttons[0][11] = 1 // R1
	}
}
