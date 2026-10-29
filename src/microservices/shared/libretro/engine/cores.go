package engine

// CE QU'ON SAIT DES CŒURS ET DES CONSOLES, appris en les faisant tourner sur
// le serveur, et qui vaut pour toute plateforme : sorti du runtime du serveur
// (stream_session.go) pour que le téléphone en hérite (chantier 13).

import "runtime"

// hwRenderConsoles — les consoles pour lesquelles le rendu matériel a été
// construit ET essayé. Accepter la demande de n'importe quel cœur a déjà
// pendu une session PSP : PPSSPP demande le rendu matériel, annonce une
// taille maximale de 0x0, et la boucle n'a pas survécu à son FBO vide. Les
// consoles hors liste rendent en logiciel, comme elles l'ont toujours fait.
var hwRenderConsoles = map[string]bool{
	"n64": true,
	"ps2": true,
	"gc":  true, // dolphin — sans lui, injouable (2026-09-08)
	"wii": true, // le même cœur
	// flycast rend en matériel comme les autres 3D de cette liste.
	"dreamcast": true,
}

// hwRenderAndroidConsoles — accordées EN PLUS sur Android, où le contexte est
// GLES (acceptHWRender n'y prend que lui).
var hwRenderAndroidConsoles = map[string]bool{
	// PPSSPP. Sa version Android n'a pas de rendu logiciel : sans contexte,
	// elle tournait à 60 images par seconde… d'images vides
	// (`données=false`), là où LibretroDroid lui donne GLES (mesuré le
	// 2026-10-11). Sur le serveur, il reste en logiciel, qui y marche : en
	// OpenGL de bureau par EGL, il échoue à `glewInit()` puis tombe en
	// SIGSEGV en créant ses shaders — la pendaison racontée plus haut, sous
	// une autre forme, et toujours vraie le même jour.
	"psp": true,
}

// HWRenderAllowed — le rendu matériel est-il accordé à cette console ?
func HWRenderAllowed(consoleID string) bool {
	return hwRenderConsoles[consoleID] || (runtime.GOOS == "android" && hwRenderAndroidConsoles[consoleID])
}

// latePortRegistrationCores — les cœurs qui SEGFAULT si on leur enregistre
// les ports de manette avant retro_init, et à qui il faut donc parler après
// retro_load_game. L'ordre inverse reste le défaut pour tous les autres.
var latePortRegistrationCores = map[string]bool{
	"pcsx2":   true, // LRPS2 v2.0.0 — trouvé le 2026-08-12
	"dolphin": true, // dolphin-emu v2606 — trouvé le 2026-09-08, même trace
}

// LatePortRegistration — ce cœur veut-il ses ports après retro_load_game ?
// `core` est le nom court du cœur (« pcsx2 », pas le fichier).
func LatePortRegistration(core string) bool {
	return latePortRegistrationCores[core]
}

// internalResolutions — l'option de résolution interne des cœurs qui en ont
// une (#340). Le joueur choisit la résolution du flux, et l'émulateur dessine
// au plus près, au lieu de rendre sa taille d'époque qu'on agrandit ensuite —
// décidé par le porteur le 2026-10-12, pour le rétro comme pour les
// émulateurs de bureau, sur le serveur comme sur le téléphone.
//
// Seule la CLÉ est apprise ici. Les valeurs, le cœur les déclare lui-même
// (SET_VARIABLES, voir renderResolution) : une version qui en ajoute ou en
// retire ne casse rien. `native` dit ce que vaut « ×1 » quand ses valeurs
// sont des multiplicateurs ; `screens`, combien d'écrans empilés une valeur
// décrit un par un. Relevé le 2026-10-12 en chargeant chaque cœur du serveur.
var internalResolutions = map[string]resolutionOption{
	"ppsspp_internal_resolution":  {},           // 480x272|960x544|1440x816|…
	"parallel-n64-screensize":     {},           // 640x480|320x240|960x720|…
	"reicast_internal_resolution": {},           // flycast : 640x480|320x240|800x600|…
	"desmume_internal_resolution": {screens: 2}, // 256x192|512x384|… : PAR écran, et la DS en empile deux
	// Déclarée au chargement du jeu seulement, pas avant.
	"dolphin_efb_scale":        {native: 528}, // 1|2|…|6 : le tampon d'image de dolphin, 640x528
	"pcsx2_upscale_multiplier": {native: 448}, // 1x Native (PS2)|2x Native (~720p)|…
	"kronos_resolution_mode":   {native: 240}, // original|2X|4X|8X
	// Le rendu « amélioré » de pcsx_rearmed double la résolution : ×1 ou ×2.
	"pcsx_rearmed_neon_enhancement_enable": {native: 240}, // disabled|enabled
}

type resolutionOption struct {
	native  int // la hauteur de « ×1 », pour des valeurs en multiplicateurs
	screens int // les écrans empilés qu'une valeur décrit un par un (la DS : 2)
}
