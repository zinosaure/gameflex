// L'ENTRÉE PAR XTEST — l'extension que tout serveur X porte pour injecter
// clavier et pointeur comme s'ils venaient d'un périphérique. C'est le dernier
// mètre de l'entrée d'un invité X : le navigateur envoie ses gestes par le
// DataChannel, le routeur Java les traduit, et ils entrent ici dans l'écran, où
// SDL2 (xemu) ou Qt (RPCS3) les lisent comme n'importe quelle souris.
//
// Les touches sont désignées par KEYSYM (le sens : Return, Escape, une flèche),
// jamais par keycode (la position sur un clavier donné) : la table du serveur,
// lue une fois, fait la conversion, et le keymap d'Xvfb (pc105, us) n'a pas à
// être connu de l'appelant.
package x11grab

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// initInput active XTEST et lit la table keysym → keycode. Une fois.
func (d *Display) initInput() error {
	if d.keycodes != nil {
		return nil
	}
	if err := xtest.Init(d.conn); err != nil {
		return fmt.Errorf("extension XTEST : %w", err)
	}
	setup := xproto.Setup(d.conn)
	first := setup.MinKeycode
	count := byte(setup.MaxKeycode-setup.MinKeycode) + 1
	reply, err := xproto.GetKeyboardMapping(d.conn, first, count).Reply()
	if err != nil {
		return fmt.Errorf("table des touches : %w", err)
	}
	per := int(reply.KeysymsPerKeycode)
	keycodes := make(map[uint32]byte)
	for i := 0; i < int(count); i++ {
		for level := 0; level < per; level++ {
			at := i*per + level
			if at >= len(reply.Keysyms) {
				break
			}
			sym := uint32(reply.Keysyms[at])
			if sym == 0 {
				continue
			}
			// Le premier keycode qui porte ce keysym : le niveau 0 (sans
			// modificateur) gagne, puisqu'on parcourt les niveaux dans l'ordre.
			if _, seen := keycodes[sym]; !seen {
				keycodes[sym] = byte(int(first) + i)
			}
		}
	}
	d.keycodes = keycodes

	return nil
}

// Pointer déplace le pointeur en (x, y) — coordonnées de l'écran — puis, si
// `button` > 0, presse (press) ou relâche ce bouton (1 = gauche, 2 = milieu,
// 3 = droit). Un clic complet est deux appels : press puis release.
func (d *Display) Pointer(x, y int, button int, press bool) error {
	if err := d.initInput(); err != nil {
		return err
	}
	root := xproto.Window(d.root)
	if err := xtest.FakeInputChecked(d.conn, byte(xproto.MotionNotify), 0, 0, root,
		int16(x), int16(y), 0).Check(); err != nil {
		return fmt.Errorf("déplacement en (%d,%d) : %w", x, y, err)
	}
	if button <= 0 {
		return nil
	}
	kind := byte(xproto.ButtonPress)
	if !press {
		kind = byte(xproto.ButtonRelease)
	}
	if err := xtest.FakeInputChecked(d.conn, kind, byte(button), 0, root, 0, 0, 0).Check(); err != nil {
		return fmt.Errorf("bouton %d : %w", button, err)
	}

	return nil
}

// Key presse ou relâche la touche qui porte `keysym` dans le keymap du serveur.
func (d *Display) Key(keysym uint32, press bool) error {
	if err := d.initInput(); err != nil {
		return err
	}
	code, ok := d.keycodes[keysym]
	if !ok {
		// LA TAILLE DU KEYMAP DANS LE MESSAGE, et ce n'est pas un détail.
		//
		// Un Xvfb qui a démarré sans xkbcomp n'a qu'une table dégénérée, et
		// « keysym 0xff0d sans touche » ne dit pas si c'est CE keysym qui
		// manque ou TOUTE la table. Le lien vers xkbcomp ne survit pas à une
		// mise à jour de l'application (voir `Xserver.ensure`), donc le cas est
		// réel, et il doit se lire d'un coup d'œil à distance. Un keymap sain
		// en porte des dizaines — mesuré à 248 lignes sur un Xvfb amont, avec
		// les seize keysyms de la manette tous présents (2026-09-13).
		return fmt.Errorf("keysym %#x absent d'un keymap de %d touches",
			keysym, len(d.keycodes))
	}
	kind := byte(xproto.KeyPress)
	if !press {
		kind = byte(xproto.KeyRelease)
	}
	if err := xtest.FakeInputChecked(d.conn, kind, code, 0, xproto.Window(d.root), 0, 0, 0).Check(); err != nil {
		return fmt.Errorf("touche %#x : %w", keysym, err)
	}

	return nil
}
