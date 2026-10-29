package x11grab

import (
	"github.com/jezek/xgb/xproto"
)

// FillScreen agrandit à la taille de l'écran chaque fenêtre de premier niveau
// visible qui ne le couvre pas, et rend le nombre de fenêtres retouchées.
//
// LA GARDE PLEIN ÉCRAN DU SERVEUR, SANS GESTIONNAIRE DE FENÊTRES (2026-10-11).
//
// Sur le serveur, la PS3 en a besoin depuis longtemps
// (runtimes/streamex/consoles/ps3/rpcs3/entrypoint.sh, « Fullscreen guard ») :
// sous `--no-gui`, la fenêtre de jeu de RPCS3 garde la taille de sortie de la
// PS3, 1280×720, quelle que soit celle de l'écran — son code ne ramène que la
// POSITION dans l'écran, pas la taille (`create_centered_window_geometry`). Le
// serveur demande donc le plein écran à openbox, toutes les trois secondes.
//
// Le téléphone n'a pas de gestionnaire de fenêtres, et n'en avait pas besoin
// tant que son écran X faisait 1280×720 : la fenêtre tombait juste. Depuis
// qu'il suit la résolution choisie dans le panneau, une PS3 en 360p n'aurait
// montré que le quart haut-gauche de son jeu. Sans gestionnaire, rien
// n'intercepte une demande de configuration : le serveur X l'applique telle
// quelle, et la fenêtre reçoit sa nouvelle taille comme si on l'avait tirée
// à la souris — ce que tous ces émulateurs savent suivre.
//
// Les fenêtres « override-redirect » (menus, bulles) et les fenêtres de 2×2 ou
// moins (les ancres invisibles de Qt) restent comme elles sont, comme sur le
// serveur.
func (d *Display) FillScreen() (int, error) {
	tree, err := xproto.QueryTree(d.conn, xproto.Window(d.root)).Reply()
	if err != nil {
		return 0, err
	}

	fixed := 0
	for _, window := range tree.Children {
		attributes, err := xproto.GetWindowAttributes(d.conn, window).Reply()
		if err != nil || attributes.MapState != xproto.MapStateViewable || attributes.OverrideRedirect {
			continue
		}
		geometry, err := xproto.GetGeometry(d.conn, xproto.Drawable(window)).Reply()
		if err != nil || (geometry.Width <= 2 && geometry.Height <= 2) {
			continue
		}
		if geometry.X == 0 && geometry.Y == 0 && int(geometry.Width) == d.Width && int(geometry.Height) == d.Height {
			continue
		}

		mask := uint16(xproto.ConfigWindowX | xproto.ConfigWindowY | xproto.ConfigWindowWidth | xproto.ConfigWindowHeight)
		if err := xproto.ConfigureWindowChecked(d.conn, window, mask,
			[]uint32{0, 0, uint32(d.Width), uint32(d.Height)}).Check(); err != nil {
			continue
		}
		fixed++
	}

	return fixed, nil
}

// WindowTitles rend le titre de chaque fenêtre de premier niveau visible —
// `_NET_WM_NAME` (UTF-8), ou `WM_NAME` à défaut.
//
// CE QUE L'ÉMULATEUR DIT DE LUI-MÊME (2026-10-11). La capture ne voit que les
// images NEUVES qu'elle prélève, donc jamais plus que sa propre cadence : en
// 360p à 30 images par seconde, « Emulation FPS » affichait 30 pendant que
// RPCS3 en rendait davantage, et le porteur a cru l'émulateur ralenti. RPCS3
// écrit sa vraie cadence dans le titre de sa fenêtre de jeu (« FPS: 43.21 |
// Vulkan | … », son « Window Title Format » par défaut) : il suffit de le lire.
func (d *Display) WindowTitles() ([]string, error) {
	tree, err := xproto.QueryTree(d.conn, xproto.Window(d.root)).Reply()
	if err != nil {
		return nil, err
	}

	netName := d.atom("_NET_WM_NAME")
	var titles []string
	for _, window := range tree.Children {
		attributes, err := xproto.GetWindowAttributes(d.conn, window).Reply()
		if err != nil || attributes.MapState != xproto.MapStateViewable || attributes.OverrideRedirect {
			continue
		}
		title := d.textProperty(window, netName)
		if title == "" {
			title = d.textProperty(window, xproto.AtomWmName)
		}
		if title != "" {
			titles = append(titles, title)
		}
	}

	return titles, nil
}

// textProperty lit une propriété texte d'une fenêtre, quel que soit son type
// (STRING ou UTF8_STRING) ; "" si elle manque.
func (d *Display) textProperty(window xproto.Window, property xproto.Atom) string {
	if property == xproto.AtomNone {
		return ""
	}
	reply, err := xproto.GetProperty(d.conn, false, window, property, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil || reply == nil || reply.Format != 8 {
		return ""
	}

	return string(reply.Value)
}

// atom rend l'atome nommé, ou AtomNone s'il n'existe pas encore sur ce
// serveur (alors aucune fenêtre ne le porte).
func (d *Display) atom(name string) xproto.Atom {
	reply, err := xproto.InternAtom(d.conn, true, uint16(len(name)), name).Reply()
	if err != nil || reply == nil {
		return xproto.AtomNone
	}

	return reply.Atom
}
