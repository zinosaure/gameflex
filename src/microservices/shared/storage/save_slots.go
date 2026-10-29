// Le nombre d'emplacements de sauvegarde, là où les DEUX hôtes le lisent.
//
// POURQUOI ICI ET PAS CHEZ L'UN DES DEUX. « Dix emplacements par (utilisateur,
// console, jeu) » est une règle du produit, pas d'un hôte : le serveur les
// liste, le téléphone les liste, et la page affiche « #N » d'après le même
// compte. Le nombre vivait dans `retro_files` (serveur), paquet INTERNE au
// module du kernel — donc inatteignable depuis `gameflex/mobilekernel`. Écrire
// « 10 » une troisième fois chez le téléphone garantissait la dérive : le jour
// où l'un passe à douze, l'autre supprime neuf emplacements sur douze sans rien
// signaler.
//
// LA COPIE JAVA RESTE, ET C'EST DIT EXPRÈS. `StreamingHost.stateSlots` énumère
// `slot < 10` de son côté ; une constante Go ne l'atteint pas. Le prochain qui
// change ce nombre doit savoir qu'il y a un second exemplaire, et où.
package storage

const (
	// MaxSaveSlots — dix emplacements, numérotés 0 à 9.
	MaxSaveSlots = 10

	// AutoSaveSlot — le 0, celui que « sauvegarder en quittant » écrit.
	//
	// Il n'a plus rien d'automatique depuis le 2026-08-24 : la sauvegarde de
	// démontage a été retirée (elle écrivait un état complet pour tout jeu
	// simplement ouvert dix secondes). Le nom dit à quoi ce créneau SERT —
	// « là où je me suis arrêté » — pas qui l'écrit.
	AutoSaveSlot = 0
)
