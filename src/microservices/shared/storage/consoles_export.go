package storage

import "gameflex/consoles"

// KnownConsole — une console du registre telle que `/api/v1/consoles` la sert
// sur le serveur, plus ce que seul l'appareil a besoin de savoir.
//
// LA CONSOLE ENTIÈRE, EMBARQUÉE (2026-10-11). Ce type recopiait les champs un
// par un, « mêmes noms, même forme » — et en oubliait. Le menu du téléphone
// n'avait pas l'année ; la page n'y ouvrait pas l'écran tactile de la
// Nintendo DS (`touch` : aucun toucher n'arrivait au cœur) ; le stick à
// l'écran y devenait une croix directionnelle pour toutes les consoles
// (`analog`) ; et les BIOS facultatifs d'une console y paraissaient
// inexistants (`biosOptional`). Embarquer la structure du registre rend
// l'oubli impossible : un champ ajouté au registre arrive sur le téléphone
// sans qu'on y pense.
type KnownConsole struct {
	consoles.Console
	// Playable — la console a-t-elle un core libretro connu ? Sur un
	// téléphone, ça ne dit pas qu'on PEUT y jouer (encore faut-il l'avoir
	// téléchargé), mais que c'est possible en principe. Une console sans
	// core ne se joue que sur le serveur.
	Playable bool `json:"playable"`
	// Enabled — GameFlex sait-il servir cette console AUJOURD'HUI ? Le
	// registre le garde pour lui (`json:"-"`) ; ce pont le rend.
	//
	// Distinct de Playable, et la différence compte : la Xbox Series est
	// parfaitement jouable en principe, personne ne l'a implémentée. Le
	// serveur ne rend que les consoles activées (consoles.List) ; ce pont-ci
	// rendait TOUT, y compris ce qui n'existe pas encore — d'où quatre
	// consoles de jeu à distance dans le menu du téléphone pour une seule
	// implémentée (signalé le 2026-09-01).
	//
	// Le champ est rendu plutôt que filtré ici parce que l'autre appelant du
	// registre — le scanner — a besoin des désactivées : un dossier nommé
	// « scummvm » veut dire scummvm, que la plateforme soit servie ou non.
	Enabled bool `json:"enabled"`
}

// KnownConsoles expose le registre partagé sous une forme que le pont mobile
// peut sérialiser. Le registre reste la seule vérité ; ceci n'en est qu'une
// vue.
func KnownConsoles() []KnownConsole {
	all := consoles.All()
	out := make([]KnownConsole, 0, len(all))
	for _, console := range all {
		playable := len(console.Cores) > 0
		if console.Cores == nil {
			// Jamais nil : côté mobile, nil se sérialise en null et fait
			// planter la boucle d'en face.
			console.Cores = []string{}
		}
		out = append(out, KnownConsole{
			Console:  console,
			Playable: playable,
			Enabled:  console.Enabled,
		})
	}

	return out
}
