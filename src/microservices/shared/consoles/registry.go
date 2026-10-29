// Package consoles is the single source of truth for console metadata —
// replaces the old frontend-only nodejs/src/app/gameflex/consoles.js
// (a leftover from the client-side EmulatorJS/WASM era; the app is now
// fully server-side streaming over WebRTC, so this data belongs here, not
// duplicated in a JS file the Go server can't read). Deliberately its own
// package, not nested under internal/retrogaming: consoles are grouped by
// the SERVICE that streams them, not by display category.
package consoles

import "strings"

// Method groups a console by the service that serves it — the frontend's
// buildMenuItems (index.js) routes each group to its own section.
const (
	// MethodRetroGaming — libretro cores, streamed by the retrogaming
	// runtime (one container per session, internal/retrogaming's bridge).
	MethodRetroGaming = "retrogaming"
	// MethodStreamex — per-console emulator images (Eden/RPCS3/xemu),
	// one container per session spawned by kernel.
	MethodStreamex = "streamex"
	// MethodRemotePlay — remote-play device platform labels (PS4/PS5/…);
	// the actual device registry lives in the remoteplay service, this is
	// just the category list.
	MethodRemotePlay = "remoteplay"
)

// Les avertissements d'une console expérimentale (Console.Experimental).
const (
	ExperimentalGeneral  = "general"
	ExperimentalHeavy    = "heavy"
	ExperimentalFirmware = "firmware"
)

type Console struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Publisher  string   `json:"publisher"`
	Method     string   `json:"method"`
	Cores      []string `json:"cores"`
	MaxPlayers int      `json:"maxPlayers"`
	// Extensions — les extensions de fichier de cette console, en minuscules
	// et sans le point. Servent de SECONDE vérification au scanner : le
	// dossier dit à quelle console un jeu appartient, l'extension le
	// confirme. Un `.gba` dans un dossier `ps2` est une erreur de rangement,
	// pas un jeu PS2 — et deux signaux discordants valent mieux qu'un seul
	// qu'on croit sur parole.
	//
	// Vide = pas de confirmation possible, le dossier fait foi seul. C'est
	// le cas des plateformes dont les jeux sont des DOSSIERS ou des formats
	// trop variés (scummvm, dos), et ce n'est pas un oubli.
	Extensions []string `json:"extensions,omitempty"`
	// Analog — cette console a-t-elle un VRAI stick analogique ?
	//
	// La question n'est pas cosmétique : le stick à l'écran existe sur tous
	// les gabarits, y compris ceux des consoles qui n'ont qu'une croix, et le
	// client doit savoir quoi en faire. Sur une NES il le convertit en appuis
	// de croix (c'est plus confortable qu'une croix tactile) ; sur une PS2 il
	// doit envoyer la déflexion telle quelle, sinon on joue au tout-ou-rien.
	//
	// Le client tenait cette liste en dur — `psp`, `ps1`, et rien d'autre.
	// PS2, N64, GameCube, Wii et Dreamcast recevaient donc des appuis de
	// croix depuis leur stick, alors qu'elles en ont toutes un vrai
	// (2026-09-08). Un fait de console appartient au registre des consoles.
	Analog bool `json:"analog,omitempty"`
	// Experimental — la console prévient AVANT la partie (la page des jeux),
	// et dit pourquoi : ExperimentalGeneral (l'émulateur lui-même est
	// expérimental), ExperimentalHeavy (il lâche sur quelques jeux lourds),
	// ExperimentalFirmware (certains jeux demandent un autre firmware). Vide :
	// rien à dire. La page testait une liste d'identifiants écrite en dur —
	// une console ajoutée ne prévenait pas sans qu'on y touche (D9).
	Experimental string `json:"experimental,omitempty"`
	// Bios — les fichiers que le cœur de cette console RÉCLAME pour démarrer.
	//
	// Pas ceux qu'on a : ceux qu'il faut. L'écran de stockage compare les
	// deux, et c'est toute la différence entre « voici ton dossier » et
	// « voici ce qui te manque » — la seule question qu'un administrateur se
	// pose vraiment devant un jeu qui ne démarre pas.
	//
	// Vide veut dire « ce cœur n'en a pas besoin », pas « on ne sait pas » :
	// la majorité des consoles 8/16 bits démarrent sans rien. Les BIOS
	// FACULTATIFS (le gba_bios de mgba, qui améliore la compatibilité sans
	// être requis) n'y figurent pas non plus — les annoncer manquants ferait
	// chercher un fichier dont personne n'a besoin.
	//
	// Les noms viennent de ce que chaque cœur cherche réellement. Une entrée
	// fausse se corrige ici, à un seul endroit, et l'écran suit.
	//
	// CE QUE CETTE LISTE NE SAIT PAS DIRE, et qu'il faut savoir avant de s'y
	// fier :
	//
	//   - la PS2 exige un vrai dump, mais N'IMPORTE LEQUEL — leurs noms
	//     portent la région et la révision (ps2-0230a-20080220.bin…), et il y
	//     en a des dizaines. Nommer les uns exclurait les autres, donc elle
	//     reste vide ici et l'écran ne peut annoncer que ce qui est présent.
	//   - dolphin (GameCube/Wii) ne veut pas un fichier mais un DOSSIER, son
	//     `Sys`, dont l'absence ne se voit qu'au démarrage d'un titre Wii.
	//
	// Les deux méritent une forme à eux le jour où l'écran devra les dire —
	// « au moins un fichier dans ce dossier » et « ce dossier existe » ne se
	// modélisent pas avec une liste de noms.
	Bios []string `json:"bios,omitempty"`
	// BiosOptional — les fichiers qui AMÉLIORENT cette console sans être
	// exigés pour démarrer (#161, demandé le 2026-09-17).
	//
	// C'est la « forme à eux » que le commentaire de `Bios` réclamait sans
	// l'avoir : un BIOS facultatif était jusqu'ici simplement OMIS, donc
	// invisible — l'écran ne pouvait ni dire qu'il existe, ni proposer de le
	// récupérer, et son absence ne devait surtout pas être annoncée comme un
	// manque. Déclaré ici, il s'affiche sans jamais bloquer un lancement.
	//
	// DEUX LISTES, PAS UN DRAPEAU, et c'est l'utilisateur qui a raison sur ce
	// point : ses tables `REQUIRED_BIOS` / `OPTIONAL_BIOS` (webapp,
	// localLibrary.js) faisaient déjà le travail — elles étaient juste au
	// mauvais endroit. J'avais d'abord écrit un booléen « ces BIOS sont
	// facultatifs » ; il ne sait pas dire « ces deux fichiers, dont un seul
	// est requis », qui est précisément le cas de la Switch. Deux listes le
	// disent sans rien inventer.
	//
	// LA SÉMANTIQUE DE `Bios` EST DONC « AU MOINS UN », pas « tous » — celle
	// que la webapp applique déjà (`missingBiosFor` : `wanted.some(present)`,
	// « ce sont des révisions du même BIOS »). Le serveur, lui, énumérait
	// chaque nom avec son propre état sans jamais dire lesquels suffisaient :
	// les deux côtés disaient des choses différentes de la même liste, et la
	// Switch en était la preuve (registre : les deux ; webapp : `prod.keys`
	// requise, `title.keys` optionnelle).
	//
	// UNE CONSOLE QUI SE PASSE DE TOUT n'a rien à déclarer dans `Bios` : la
	// liste vide suffit déjà à l'exempter. Et une console dont les BIOS sont
	// entièrement contournables (le HLE de `pcsx_rearmed`) les déclare ICI,
	// pas au-dessus — ainsi l'écran les propose, et le jeu part sans.
	BiosOptional []string `json:"biosOptional,omitempty"`
	// Width/Height — la GÉOMÉTRIE NATIVE du cœur, en pixels.
	//
	// Un hôte qui rend hors écran doit choisir une taille de tampon AVANT
	// que le cœur ne tourne. Le serveur, lui, attend la première image et
	// prend les dimensions réelles ; le téléphone n'en a pas le loisir —
	// son `ImageReader` existe avant le premier appel vidéo.
	//
	// Il forçait donc 640×480 pour TOUT. Une NES (256×240) s'encodait en
	// cinq fois trop de pixels, et une DS (256×384, portrait) se retrouvait
	// en boîte dans du 4/3 (signalé le 2026-09-05 : « l'écran est plus
	// petit pour que le portrait s'adapte dans du paysage »).
	//
	// LA RÈGLE, quand une console n'a pas UNE seule géométrie :
	//
	//   géométrie fixe        → le natif. C'est gratuit et c'est exact.
	//   géométrie variable    → le HAUT de sa plage. Une PlayStation alterne
	//                           320×240 et 640×480 selon la scène ; figer le
	//                           bas perdrait la moitié des jeux, et le
	//                           surcoût de tenir le haut est acceptable —
	//                           ce sont justement les consoles dont on
	//                           attend de la définition (décidé le
	//                           2026-09-05).
	//
	// Ce qu'on ne fait PAS : forcer une taille ronde partout. C'était le cas
	// avant, et une NES y payait cinq fois trop de pixels pendant qu'une DS
	// y perdait son format. Le coût n'est pas le même problème que l'aspect,
	// et c'est l'aspect qui rendait la DS illisible.
	//
	// ZÉRO = on ne sait pas ; l'hôte garde alors son propre défaut.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// Touch — cette console a un ÉCRAN TACTILE, et son core lit donc
	// RETRO_DEVICE_POINTER.
	//
	// C'est une donnée du matériel, pas une capacité à découvrir : la DS a
	// un écran du bas, la NES n'en a pas, et aucun core n'annonce ça. La
	// page s'en sert pour décider si un clic sur l'image est un stylet ou
	// rien du tout — sans quoi il faudrait coder une liste de consoles dans
	// le frontend, à côté de celle-ci.
	Touch bool `json:"touch,omitempty"`
	// Year — l'année de la PREMIÈRE sortie de la machine, la plus ancienne :
	// souvent au Japon (la Famicom en 1983, pas la NES de 1985 ; la Mark III
	// en 1985, pas la Master System). Une entrée qui couvre deux machines
	// prend la première (Game Boy, 1989, pour « Game Boy / Game Boy Color »).
	//
	// Demandée par le porteur le 2026-10-11, pour le menu. consoles.js portait
	// une date (`release`) que personne ne lisait, et elle n'avait pas été
	// reprise ; elle revient avec un usage. 0 = pas d'année qui ait un sens
	// (ScummVM est un moteur de jeux, pas une machine).
	Year int `json:"year,omitempty"`
	// Enabled, not exported to JSON (see List) — a disabled entry's data is
	// kept here rather than deleted so it's ready to flip on rather than
	// re-author from scratch, same spirit as consoles.js's own commented-out
	// entries before this migration.
	Enabled bool `json:"-"`
}

// registry — ported field-for-field from the old consoles.js. release/
// generation were NOT ported: confirmed via a full codebase search that
// neither was ever read anywhere, frontend or Go.
//
// Les extensions, elles, sont revenues le 2026-08-26 : le commentaire
// d'origine disait « à remettre si un vrai consommateur en a besoin », et le
// scanner de l'application mobile en est un (shared/storage). Le scanner du
// serveur, lui, continue d'accepter tout fichier non caché — il ne s'en sert
// pas encore, et ce champ ne change rien pour lui.
var registry = []Console{
	// ── Nintendo ──
	{ID: "nes", Name: "Nintendo Ent. System (NES)", Publisher: "Nintendo", Year: 1983, Method: MethodRetroGaming, Cores: []string{"fceumm", "nestopia"}, MaxPlayers: 2, Extensions: []string{"nes", "fds", "unf", "unif"}, Width: 256, Height: 240, Enabled: true},
	{ID: "snes", Name: "Super Nintendo (SNES)", Publisher: "Nintendo", Year: 1990, Method: MethodRetroGaming, Cores: []string{"snes9x"}, MaxPlayers: 4, Extensions: []string{"smc", "sfc", "swc", "fig", "bs"}, Width: 256, Height: 224, Enabled: true}, // official Super Multitap
	{ID: "gbc", Name: "Game Boy / Game Boy Color", Publisher: "Nintendo", Year: 1989, Method: MethodRetroGaming, Cores: []string{"gambatte"}, MaxPlayers: 2, Extensions: []string{"gb", "gbc", "sgb"}, Width: 160, Height: 144, Enabled: true},
	{ID: "gba", Name: "Game Boy Advance", Publisher: "Nintendo", Year: 2001, Method: MethodRetroGaming, Cores: []string{"mgba"}, MaxPlayers: 2, Extensions: []string{"gba", "agb"}, Width: 240, Height: 160, Enabled: true},
	{ID: "vb", Name: "Virtual Boy", Publisher: "Nintendo", Year: 1995, Method: MethodRetroGaming, Cores: []string{"mednafen_vb"}, MaxPlayers: 2, Extensions: []string{"vb", "vboy"}, Width: 384, Height: 224, Enabled: true},
	// n64 — parallel_n64 confirmed working (2026-08-01, real gameplay
	// screenshot verified) running fully in software (its default renderer
	// config never even calls RETRO_ENVIRONMENT_SET_HW_RENDER) — no GPU
	// context needed at all. The OLD note here about mupen64plus_next
	// SIGSEGV-ing "the whole shared Go server" no longer applies to ANY
	// core: that was true before the multi-session refactor, when every
	// session ran in one shared process; today each session is its own OS
	// process (supervisor.go's re-exec), so even a real crash only takes
	// down that one session (confirmed in practice via PS2/Play!'s own
	// crash, see GAMEFLEX_JOURNAL.md's "PS2 (Play!) et N64" section —
	// contained cleanly to its child process).
	//
	// N64 : parallel_n64, ET LUI SEUL POUR L'INSTANT.
	//
	// mupen64plus_next a été essayé le 2026-09-03 parce que parallel_n64 est
	// lourd — son rendu RDP logiciel demande bien plus qu'un émulateur N64
	// ordinaire, au point que le son hache et que les entrées semblent
	// mortes. Il FAIT TOMBER le processus enfant :
	//
	//     [core-log] mupen64plus: Starting R4300 emulator: Pure Interpreter
	//     fatal error: morestack on g0
	//
	// Le core charge, s'initialise, puis tue le processus. C'est un défaut
	// de cgo, pas du core : mupen64plus_next crée ses PROPRES fils (il a une
	// option « Threaded Renderer ») et rappelle les callbacks libretro depuis
	// eux ; le runtime Go n'a pas de pile utilisable sur ces fils-là.
	// parallel_n64 rappelle depuis le fil d'émulation, et passe.
	//
	// Pour le rouvrir un jour il faudra pouvoir passer des VARIABLES DE CORE
	// au chargement — au minimum mupen64plus-ThreadedRenderer=False. On n'en
	// passe aucune aujourd'hui, pour aucun core (voir le point d'étape du
	// 2026-09-03). Tant que c'est le cas, le proposer revient à proposer un
	// plantage.
	//
	// "dolphin" a été retiré de la liste au passage : c'est le core
	// GameCube/Wii (voir `gc`), il ne charge aucune ROM N64. Le commentaire
	// d'origine le disait déjà — « left over from editing » — mais cette
	// liste est PRÉSENTÉE dans le sélecteur d'émulateur, et y laisser une
	// entrée qui ne peut que rendre 404 promet quelque chose de faux.
	{ID: "n64", Name: "Nintendo 64", Publisher: "Nintendo", Year: 1996, Method: MethodRetroGaming, Cores: []string{"parallel_n64"}, MaxPlayers: 4, Extensions: []string{"n64", "z64", "v64", "u64", "ndd"}, Width: 640, Height: 480, Analog: true, Enabled: true}, // le HAUT de sa plage : le mode haute résolution est courant
	// LES BIOS DE LA DS SONT FACULTATIFS (décidé par le porteur le
	// 2026-10-11). Ils étaient déclarés requis, et le bouton Jouer d'une DS
	// restait grisé sans eux — sur le téléphone dès qu'il a connu le registre
	// entier (#311). Or desmume démarre sans : il ne lit même ses BIOS
	// externes que si `desmume_use_external_bios` est activée, ce que nous ne
	// faisons nulle part. Professeur Layton tourne sans, vérifié sur le
	// téléphone.
	{ID: "nds", Name: "Nintendo DS", Publisher: "Nintendo", Year: 2004, Method: MethodRetroGaming, Cores: []string{"desmume"}, MaxPlayers: 2, Extensions: []string{"nds", "dsi"}, Touch: true, Width: 256, Height: 384, BiosOptional: []string{"bios7.bin", "bios9.bin", "firmware.bin"}, Enabled: true},
	// Les extensions ne sont pas devinées : elles sortent de ce que le cœur
	// DÉCLARE lui-même — `elf|dol|gcm|iso|tgc|wbfs|ciso|gcz|wad|wia|rvz|m3u`,
	// lu dans dolphin_libretro.so. On garde les formats de disque de chaque
	// console, plus les conteneurs communs ; le .wad reste côté Wii, c'est un
	// titre WiiWare et la GameCube n'en a pas.
	//
	// gc/wii — dolphin, et le MÊME cœur pour les deux : dolphin émule la
	// GameCube et la Wii sans qu'on ait à choisir, seul le jeu chargé décide.
	// Elles restent DEUX consoles ici parce que c'est le rangement du disque
	// qui pilote le scan (roms/gc, roms/wii) et que le joueur, lui, sait très
	// bien de laquelle il parle.
	// Le commentaire d'avant disait « aucun vrai cœur libretro disponible »
	// et laissait `Cores` vide. C'était faux depuis un moment :
	// dolphin_libretro.so est bien dans le dossier des cœurs. Une liste vide
	// n'est pas neutre — handleStart s'arrête sur « No Libretro core mapped
	// for console: gc » — donc la console s'affichait, se scannait, listait
	// ses jeux, et refusait de les lancer (signalé le 2026-09-08).
	{ID: "gc", Name: "Nintendo GameCube", Publisher: "Nintendo", Year: 2001, Method: MethodRetroGaming, Cores: []string{"dolphin"}, MaxPlayers: 4, Extensions: []string{"iso", "gcm", "rvz", "gcz", "ciso", "tgc", "wia", "dol", "elf"}, Analog: true, Enabled: true},
	// La Wii était rangée en streamex avec zéro cœur — donc une console
	// allumée que rien ne pouvait lancer, ni par un conteneur (aucune image
	// Wii n'existe) ni par un cœur (aucun n'était déclaré). Elle passe en
	// rétro sous dolphin, comme la GameCube plus haut : c'est le même cœur.
	{ID: "wii", Name: "Nintendo Wii", Publisher: "Nintendo", Year: 2006, Method: MethodRetroGaming, Cores: []string{"dolphin"}, MaxPlayers: 4, Extensions: []string{"iso", "rvz", "wbfs", "wad", "gcz", "ciso", "wia", "dol", "elf"}, Analog: true, Enabled: true},

	// ── Sega ──
	{ID: "sms", Name: "Sega Master System", Publisher: "Sega", Year: 1985, Method: MethodRetroGaming, Cores: []string{"genesis_plus_gx"}, MaxPlayers: 2, Extensions: []string{"sms", "gg", "sg"}, Width: 256, Height: 192, Enabled: true},
	{ID: "megadrive", Name: "Sega Mega Drive / Genesis", Publisher: "Sega", Year: 1988, Method: MethodRetroGaming, Cores: []string{"genesis_plus_gx", "picodrive"}, MaxPlayers: 4, Extensions: []string{"md", "gen", "smd", "bin", "32x"}, Width: 320, Height: 240, Enabled: true}, // official 4-Way Play / Team Player adaptor
	// saturn/dreamcast — same GPU-crash-risk reasoning as n64 (both need real
	// 3D hardware acceleration in most configurations).
	//
	// La Dreamcast pointait sur `yabasanshiro`, qui est un cœur SATURN — il
	// n'a jamais su lire un GD-ROM. Personne ne l'avait vu parce que la
	// console était désactivée : une valeur fausse dans une ligne éteinte ne
	// se remarque qu'au moment où on l'allume. Le cœur Dreamcast est flycast,
	// et il est sur le disque (2026-09-08).
	// `saturn_bios.bin` AJOUTÉ le 2026-09-18 (#162), et il est sur le disque :
	// la webapp l'exigeait dans sa propre table (`REQUIRED_BIOS`) là où le
	// registre ne le connaissait pas. Unifier les deux sans le reprendre
	// aurait donc cessé de l'accepter — une régression silencieuse, puisque
	// la règle est « n'importe lequel suffit ».
	{ID: "saturn", Name: "Sega Saturn", Publisher: "Sega", Year: 1994, Method: MethodRetroGaming, Cores: []string{"yabause", "kronos", "mednafen_saturn"}, MaxPlayers: 4, Extensions: []string{"cue", "chd", "iso", "ccd", "mds"}, Bios: []string{"sega_101.bin", "mpr-17933.bin", "saturn_bios.bin"}, Enabled: true},
	{ID: "dreamcast", Name: "Sega Dreamcast", Publisher: "Sega", Year: 1998, Method: MethodRetroGaming, Cores: []string{"flycast"}, MaxPlayers: 4, Extensions: []string{"cdi", "gdi", "chd", "cue"}, Analog: true, Bios: []string{"dc_boot.bin", "dc_flash.bin"}, Enabled: true},

	// ── Atari ──
	{ID: "atari2600", Name: "Atari 2600", Publisher: "Atari", Year: 1977, Method: MethodRetroGaming, Cores: []string{"stella2014"}, MaxPlayers: 2, Extensions: []string{"a26", "bin"}, Enabled: true},
	// atari5200 — no matching core available (would need a5200_libretro.so).
	{ID: "atari5200", Name: "Atari 5200", Publisher: "Atari", Year: 1982, Method: MethodRetroGaming, Cores: []string{"a5200"}, MaxPlayers: 2, Extensions: []string{"a52", "bin"}, Enabled: false},
	{ID: "atari7800", Name: "Atari 7800", Publisher: "Atari", Year: 1986, Method: MethodRetroGaming, Cores: []string{"prosystem"}, MaxPlayers: 2, Extensions: []string{"a78", "bin"}, Enabled: true},
	{ID: "lynx", Name: "Atari Lynx", Publisher: "Atari", Year: 1989, Method: MethodRetroGaming, Cores: []string{"handy"}, MaxPlayers: 2, Extensions: []string{"lnx", "o"}, Enabled: true},
	{ID: "jaguar", Name: "Atari Jaguar", Publisher: "Atari", Year: 1993, Method: MethodRetroGaming, Cores: []string{"virtualjaguar"}, MaxPlayers: 2, Extensions: []string{"j64", "jag", "rom", "abs", "cof"}, Enabled: true},

	// ── NEC ──
	{ID: "pce", Name: "PC Engine / TurboGrafx-16", Publisher: "NEC", Year: 1987, Method: MethodRetroGaming, Cores: []string{"mednafen_pce_fast"}, MaxPlayers: 2, Extensions: []string{"pce", "cue", "chd", "sgx"}, Enabled: true},
	{ID: "pcfx", Name: "PC-FX", Publisher: "NEC", Year: 1994, Method: MethodRetroGaming, Cores: []string{"mednafen_pcfx"}, MaxPlayers: 2, Extensions: []string{"cue", "chd", "ccd"}, Bios: []string{"pcfx.rom"}, Enabled: true},

	// ── SNK ──
	{ID: "neogeo", Name: "Neo Geo / Arcade", Publisher: "SNK", Year: 1990, Method: MethodRetroGaming, Cores: []string{"fbalpha2012_neogeo"}, MaxPlayers: 2, Extensions: []string{"zip", "chd"}, Bios: []string{"neogeo.zip"}, Enabled: true},
	{ID: "ngp", Name: "Neo Geo Pocket", Publisher: "SNK", Year: 1998, Method: MethodRetroGaming, Cores: []string{"mednafen_ngp"}, MaxPlayers: 2, Extensions: []string{"ngp", "ngc"}, Width: 160, Height: 152, Enabled: true},

	// ── Capcom ──
	{ID: "cps1", Name: "Capcom CPS-1", Publisher: "Capcom", Year: 1988, Method: MethodRetroGaming, Cores: []string{"fbalpha2012_cps1"}, MaxPlayers: 2, Extensions: []string{"zip"}, Enabled: true},
	{ID: "cps2", Name: "Capcom CPS-2", Publisher: "Capcom", Year: 1993, Method: MethodRetroGaming, Cores: []string{"fbalpha2012_cps2"}, MaxPlayers: 2, Extensions: []string{"zip"}, Enabled: true},

	// ── Bandai ──
	{ID: "wswan", Name: "WonderSwan", Publisher: "Bandai", Year: 1999, Method: MethodRetroGaming, Cores: []string{"mednafen_wswan"}, MaxPlayers: 2, Extensions: []string{"ws", "wsc"}, Enabled: true},

	// ── Autres ──
	{ID: "coleco", Name: "ColecoVision", Publisher: "Coleco", Year: 1982, Method: MethodRetroGaming, Cores: []string{"gearcoleco"}, MaxPlayers: 2, Extensions: []string{"col", "cv", "rom", "bin"}, Bios: []string{"colecovision.rom"}, Enabled: true},
	{ID: "vectrex", Name: "Vectrex", Publisher: "GCE", Year: 1982, Method: MethodRetroGaming, Cores: []string{"vecx"}, MaxPlayers: 2, Extensions: []string{"vec", "bin"}, Enabled: true},
	{ID: "c64", Name: "Commodore 64", Publisher: "Commodore", Year: 1982, Method: MethodRetroGaming, Cores: []string{"vice_x64sc"}, MaxPlayers: 2, Extensions: []string{"d64", "t64", "prg", "crt", "tap", "g64"}, Enabled: true},
	{ID: "amstrad", Name: "Amstrad CPC", Publisher: "Amstrad", Year: 1984, Method: MethodRetroGaming, Cores: []string{"cap32"}, MaxPlayers: 2, Extensions: []string{"dsk", "sna", "cdt", "kcr"}, Enabled: true},
	{ID: "zxspectrum", Name: "Sinclair ZX Spectrum", Publisher: "Sinclair", Year: 1982, Method: MethodRetroGaming, Cores: []string{"fuse"}, MaxPlayers: 2, Extensions: []string{"tzx", "tap", "z80", "sna", "szx"}, Enabled: true},
	{ID: "doom", Name: "Doom", Publisher: "id Software", Year: 1993, Method: MethodRetroGaming, Cores: []string{"prboom"}, MaxPlayers: 2, Extensions: []string{"wad", "iwad", "pwad"}, Enabled: true},
	// scummvm — scummvm_libretro.so is 133MB, over GitHub's 100MB hard
	// per-file push limit (no Git LFS set up here) — can't be committed at
	// all as a plain blob, regardless of runtime safety.
	{ID: "scummvm", Name: "ScummVM", Publisher: "ScummVM Team", Method: MethodRetroGaming, Cores: []string{"scummvm"}, MaxPlayers: 2, Enabled: false},
	// ── Sony ──
	// LA PS1 : SES DUMPS SONT FACULTATIFS, et c'est mesuré (#161).
	//
	// `pcsx_rearmed` se rabat sur son BIOS émulé (HLE) quand aucun dump n'est
	// là — toute l'histoire de #144 : elle jouait ainsi, en silence, alors que
	// les vrais dumps étaient sur le disque. Le HLE casse une partie des
	// titres, donc ils restent DÉCLARÉS (l'écran les propose, et dit s'ils
	// manquent) mais du côté FACULTATIF : leur absence ne doit pas empêcher de
	// jouer.
	//
	// Six noms côté webapp contre trois ici, et le sien est plus complet
	// (scph1001, scph7001, psxonpsp660 en plus) — je reprends sa liste, qui
	// vient de ce que les gens ont réellement sur leurs étagères.
	{ID: "ps1", Name: "PlayStation 1", Publisher: "Sony", Year: 1994, Method: MethodRetroGaming, Cores: []string{"pcsx_rearmed"}, MaxPlayers: 4, Extensions: []string{"cue", "bin", "chd", "pbp", "img", "ecm", "m3u", "iso"}, Width: 640, Height: 480, Analog: true, BiosOptional: []string{"scph5500.bin", "scph5501.bin", "scph5502.bin", "scph1001.bin", "scph7001.bin", "psxonpsp660.bin"}, Enabled: true}, // official Multitap — 640×480 : le HAUT de sa plage, voir Width/Height

	// ── Générations avancées — streamex (images par console) et remote play ──
	// ps2 — pcsx2 first (likely-better default), play kept as a fallback:
	// confirmed to crash on its first retro_run() in this headless-GL setup
	// (SIGABRT, contained to its own child process). pcsx2 is untested here
	// (may default to a rejected Vulkan context instead of OpenGL — no
	// renderer-selection UI to steer it either way).
	// « play » RETIRÉ le 2026-09-08. Il charge le jeu, obtient son contexte de
	// rendu, démarre sa boucle — puis meurt au premier `retro_run` sur
	// « free(): invalid pointer », un abandon de tas dans le cœur lui-même
	// (relevé sur une vraie session). Proposer un cœur qui abat sa session à
	// coup sûr ne rend service à personne : il revient le jour où il tient
	// debout.
	{ID: "ps2", Name: "PlayStation 2", Publisher: "Sony", Year: 2000, Method: MethodRetroGaming, Cores: []string{"pcsx2"}, MaxPlayers: 4, Extensions: []string{"iso", "chd", "cso", "bin", "cue"}, Analog: true, Enabled: true},
	{ID: "psp", Name: "PlayStation Portable (PSP)", Publisher: "Sony", Year: 2004, Method: MethodRetroGaming, Cores: []string{"ppsspp"}, MaxPlayers: 1, Extensions: []string{"iso", "cso", "pbp", "chd"}, Width: 480, Height: 272, Analog: true, Enabled: true},
	// LE FIRMWARE EST DÉCLARÉ, parce que l'interface doit pouvoir dire qu'il
	// manque AVANT de lancer (#161, demandé le 2026-09-17).
	//
	// Il ne l'était pas, et ce n'était pas un oubli : c'est le moteur Streamex
	// qui vérifiait, seul. Le partage des rôles tenait — sauf qu'un écran ne
	// peut pas prévenir avec une vérification qui n'a lieu qu'au démarrage du
	// conteneur. Le registre déclare donc, l'entrypoint garde son contrôle :
	// lui est le dernier rempart, pas le premier avertissement.
	//
	// LA LISTE VIENT DU MOTEUR, pas du disque. L'entrypoint de RPCS3 refuse
	// sans `PS3UPDAT.PUP` — « RPCS3 cannot run games without the official
	// firmware installed in dev_flash » — et n'en réclame aucun autre.
	// QUATRE MANETTES, comme le RPCS3 de l'image les branche : Player 1 à 4
	// sur les quatre pads virtuels de streamex (consoles/ps3/rpcs3/
	// input_configs/global/Default.yml). Le 1 qui était là datait du premier
	// registre, quand streamex ne savait servir qu'un joueur (« MaxPlayers
	// stays 1 until joiners are ported to that runtime ») ; streamex l'ignorait
	// alors et en donnait quatre. Il l'a lu le 2026-10-09 (#278), et la PS3
	// s'est retrouvée à une place — relevé le jour même par le porteur.
	{ID: "ps3", Name: "PlayStation 3", Publisher: "Sony", Year: 2006, Method: MethodStreamex, Cores: []string{"RPCS3 (streamex)"}, MaxPlayers: 4, Width: 1280, Height: 720, Bios: []string{"PS3UPDAT.PUP"}, Enabled: true},
	// LES MODULES SYSTÈME DE LA CONSOLE, FACULTATIFS — la liste « Firmware
	// files » du README de shadPS4 0.19.0, dans son ordre. Sans eux, shadPS4
	// imite chaque bibliothèque (HLE) ; avec eux, il charge la vraie, tirée de
	// la console qu'on possède (/system/common/lib/). Le premier compte plus
	// que les autres : libSceLibcInternal.sprx est chargé au démarrage
	// (linker.cpp), et c'est lui qui déroule les exceptions C++ d'un jeu.
	// FF XV Pocket Edition tourne en boucle sur ce déroulement sans lui
	// (2026-10-08, bios/ps4 vide). Le kernel monte bios/ps4 sur le
	// sys_modules de shadPS4 ; le téléphone y recopie les fichiers.
	//
	// QUATRE JOUEURS (le porteur, 2026-10-23, #441) : shadPS4 a son
	// multijoueur local depuis la 0.16.0 — la n-ième manette SDL va au n-ième
	// utilisateur (1000 à 1003, créés d'office) — et streamex en crée quatre.
	// Le téléphone reste à un (une seule table de touches, OnDesktopPlayers).
	//
	// `pkg`, LA SEULE EXTENSION (2026-10-24, #446) — la règle de la PS5, pour
	// la même raison. Un jeu PS4 se range aussi DÉBALLÉ : un dossier, eboot.bin
	// à sa racine (tools/ps4-unpack.sh). Sans extension déclarée, le scanner
	// prenait eboot.bin pour le fichier à lancer, et le téléphone le refusait
	// (« shadps4 ne lance pas un fichier .bin », S25) ; il listait aussi pour
	// un jeu n'importe quel fichier posé à côté (un `.lock` copié avec le
	// déballé). Avec `pkg`, un dossier sans .pkg se lance par lui-même, et
	// l'invité y trouve eboot.bin (GlibcGuest.resolveGame, `scan_dir_for` du
	// serveur).
	{ID: "ps4", Name: "PlayStation 4", Publisher: "Sony", Year: 2013, Method: MethodStreamex, Cores: []string{"shadPS4 (streamex)"}, MaxPlayers: 4, Width: 1280, Height: 720, BiosOptional: []string{
		"libSceAt9Enc.sprx", "libSceAudiodec.sprx", "libSceAudiodecCpu.sprx", "libSceAudiodecCpuDdp.sprx",
		"libSceAudiodecCpuDtsHdLbr.sprx", "libSceAudiodecCpuHevag.sprx", "libSceAudiodecCpuM4aac.sprx", "libSceAvPlayer.sprx",
		"libSceAvPlayerStreaming.sprx", "libSceBeisobmf.sprx", "libSceBemp2sys.sprx", "libSceCesCs.sprx",
		"libSceFont.sprx", "libSceFontFt.sprx", "libSceFreeTypeOl.sprx", "libSceFreeTypeOptOl.sprx",
		"libSceFreeTypeOt.sprx", "libSceJpegDec.sprx", "libSceJpegEnc.sprx", "libSceJson.sprx",
		"libSceJson2.sprx", "libSceLibcInternal.sprx", "libSceNgs2.sprx", "libScePngEnc.sprx",
		"libScePsmKitSystem.sprx", "libSceRtc.sprx", "libSceRudp.sprx", "libSceSystemGesture.sprx",
		"libSceUlt.sprx", "libSceWkFontConfig.sprx", "libSceXml.sprx", "libSceDepth.sprx",
		"libScePadTracker.sprx", "libSceMoveTracker.sprx",
	}, Experimental: ExperimentalGeneral, Enabled: true, Extensions: []string{"pkg"}},
	// ── PS5 ÉMULÉE : l'identifiant SIMPLE, parce que c'est elle qui reste ──
	//
	// Elle porte `ps5`, la variante à distance `rp-ps5`. Le menu les distingue
	// déjà par sa section (STREAMEX / REMOTE PLAY), donc les deux portent le
	// même nom d'affichage — c'est bien la même console, jouée de deux façons.
	//
	// EXPÉRIMENTALE, et son propre auteur le dit : KytyPS5 annonce « expect
	// crashes, graphical glitches, low compatibility, and poor performance ».
	// Elle est là pour que le setup existe et qu'un jeu puisse être essayé —
	// pas pour promettre une partie.
	// ps5 — UN JEU PS5 EST UN DOSSIER (eboot.bin à sa racine, parmi des .prx,
	// .json, .index…). Sans extension déclarée, le scanner accepte tout et prend
	// pour chemin le premier fichier de la racine — `ampr_emu.index` sur le
	// téléphone, que Kyty refuse (S25, 2026-10-14). Avec `elf` (la règle de
	// l'adaptateur du serveur), rien n'est accepté à la racine et le jeu se
	// lance par son dossier, où l'invité trouve eboot.bin (GlibcGuest.resolveGame,
	// `scan_dir_for` du serveur).
	// `zar` (#431, 2026-10-22) : un jeu-dossier compressé en ZArchive, que
	// Kyty lit tel quel (`--game <fichier.zar>`, sans extraction) depuis sa
	// version du 2026-10-22. Un dossier qui garde son `.pkg` à côté se lance
	// par son `.zar` : le paquet, Kyty ne le lit pas.
	// QUATRE JOUEURS (#441) : Kyty donne chaque manette SDL au premier joueur
	// libre depuis le 2026-10-17 (amont, « Support multiple controllers for
	// local multiplayer »), compris dans la version du serveur. Les joueurs 2
	// à 4 y sont des invités, sans profil ni trophées. Le téléphone reste à un.
	{ID: "ps5", Name: "PlayStation 5", Publisher: "Sony", Year: 2020, Method: MethodStreamex, Cores: []string{"KytyPS5 (streamex)"}, MaxPlayers: 4, Width: 1280, Height: 720, Experimental: ExperimentalGeneral, Enabled: true, Extensions: []string{"elf", "zar"}},
	// wiiu — streamex + Cemu. Comme switch/ps3/xbox, le champ Cores est une
	// ÉTIQUETTE d'affichage pour le sélecteur (il ne doit pas être vide, voir
	// le commentaire de switch juste en dessous) : le cœur n'est pas un .so
	// libretro ici mais un émulateur entier dans son conteneur.
	{ID: "wiiu", Name: "Nintendo Wii U", Publisher: "Nintendo", Year: 2012, Method: MethodStreamex, Cores: []string{"Cemu (streamex)"}, MaxPlayers: 2, Width: 1280, Height: 720, Extensions: []string{"wux", "wud", "wua", "rpx"}, Bios: []string{"keys.txt"}, Experimental: ExperimentalFirmware, Enabled: true},
	// switch — streamex + Citron, et Eden en second. The Cores field is a display label for the
	// picker (it must not be empty: Games.jsx's GameDetailPanel fills its
	// dropdown from this list); for MethodRetroGaming consoles the same
	// field is a bare core filename instead. MaxPlayers stays 1 until
	// joiners are ported to the streamex runtime.
	// 1280x720 : la sortie portable de la Switch, et c'est une taille FIXE —
	// donc elle a sa place ici. Sur un serveur elle ne sert à rien (le
	// conteneur lit la géométrie de sa première image) ; sur le téléphone le
	// tampon hors écran doit exister AVANT que l'émulateur ne tourne, donc il
	// faut choisir d'avance. Sans elle, le kernel retombait sur 640x480 et
	// demandait à une Switch de rendre dans un format qui n'est pas le sien —
	// le repli 1280x720 du lanceur ne s'appliquait jamais, puisqu'une
	// taille lui était déjà fournie (constaté le 2026-09-08 dans le constat de
	// panne d'un vrai appareil : « image : 640x480 »).
	// QUATRE joueurs, pas un : les jeux familiaux (Mario Kart, Smash…) se
	// jouent à deux ou quatre, et l'émulateur sert jusqu'à quatre manettes
	// cemuhook. Valait 1 jusqu'au 2026-09-12 — héritage de l'ère Eden, que
	// `docs/GAMEFLEX.md` contredisait déjà (« défaut 4 pour la Switch »).
	// LA SWITCH SÉPARE SES DEUX CLÉS, et c'est le désaccord que la webapp
	// arbitrait toute seule (#161) : le registre les déclarait toutes les deux
	// au même rang, `localLibrary.js` ne tenait `prod.keys` pour requise et
	// `title.keys` pour optionnelle. Les deux sources disaient donc des choses
	// différentes du même jeu de fichiers ; celle qui avait raison est la
	// webapp — `prod.keys` déchiffre le titre, `title.keys` ne sert qu'aux
	// jeux installés, donc son absence n'empêche pas de jouer une cartouche.
	// EDEN PREMIER, CITRON EN SECOND (2026-10-23, #438), partout. Eden lit
	// seul les jeux du firmware 22 (modules ZBIC : EA Sports FC 27, #433) ;
	// sur le téléphone, notre Eden (tools/eden, son HeapTracker) exécute
	// nativement sur tous les cœurs sans buter sur les 65 530 zones mémoire
	// d'Android, où Citron gèle les gros jeux (#434). Citron reste : « sur des
	// téléphones moins puissants et des petits jeux, Citron est meilleur »
	// (le porteur). L'ordre est le défaut ; la fiche d'un jeu propose l'autre.
	// HUIT JOUEURS, comme la console (le porteur, 2026-10-23, #443) : deux
	// serveurs cemuhook de quatre manettes au serveur — Eden en lit huit, et
	// l'index d'une manette y est global (le second serveur sert 4 à 7). Le
	// téléphone en garde quatre (OnDesktopPlayers). Toutes sont branchées :
	// un jeu qui en accepte moins et ouvre l'applet des manettes peut montrer
	// le dialogue d'Eden sur le flux.
	// TACTILE, comme la vraie console (2026-10-23, #435). Sur le téléphone,
	// un doigt posé sur l'image devient un clic dans la fenêtre de
	// l'émulateur (GatewayInputRouter, XTest) : c'est ce qui rend utilisable
	// le clavier de saisie de l'émulateur — le nom d'Hogwarts Legacy —, dont
	// les touches de la manette ne font que taper des lettres. Au serveur
	// (streamex), le même doigt est un clic absolu dans la fenêtre d'Eden
	// (`ma`) : sa manette, en cemuhook, atteint AUSSI le jeu derrière le
	// clavier — B y efface une lettre et fait revenir le jeu en arrière.
	{ID: "switch", Name: "Nintendo Switch", Publisher: "Nintendo", Year: 2017, Method: MethodStreamex, Cores: []string{"Eden (streamex)", "Citron (streamex)"}, MaxPlayers: 8, Width: 1280, Height: 720, Bios: []string{"prod.keys"}, BiosOptional: []string{"title.keys"}, Touch: true, Experimental: ExperimentalHeavy, Enabled: true},

	// xbox — streamex + xemu, same per-console pattern as switch above.
	//
	// LES DEUX MÊMES OUBLIS QUE LA SWITCH AVANT LE 2026-09-12, signalés par
	// l'utilisateur le 2026-09-13 en lisant ce fichier.
	//
	// 640x480 : la sortie de la console, et une taille FIXE — donc elle a sa
	// place ici. Sans elle, `NativeSize` rend zéro et le kernel du téléphone
	// retombe sur un 640×480 écrit en dur ailleurs. Le chiffre est le même par
	// chance, mais il vient alors d'un repli générique au lieu de la fiche de
	// la console : une PlayStation ou une DS y perdraient leur format, et c'est
	// exactement le défaut que la Switch a connu (« image : 640x480 » dans le
	// constat de panne d'un vrai appareil).
	//
	// QUATRE joueurs, pas un : la Xbox a quatre ports de manette, et xemu les
	// émule — la ligne QEMU qu'il assemble porte `-device usb-hub,port=1,
	// ports=4` (relevé le 2026-09-13 sur le binaire 0.8.136). Le 1 était un
	// héritage du commentaire ci-dessus, « MaxPlayers stays 1 until joiners are
	// ported », qui ne vaut plus depuis que la Switch en déclare quatre.
	// TROIS FICHIERS, ET C'EST `xemu.toml` QUI LE DIT (#161).
	//
	// L'entrypoint ne nomme que l'image de disque ; les deux autres chemins
	// vivent dans la configuration que l'image sème au premier démarrage :
	//
	//     bootrom_path  = "/opt/xbox/bios/mcpx_1.0.bin"
	//     flashrom_path = "/opt/xbox/bios/Complex_4627.bin"
	//
	// J'avais d'abord cherché ces noms dans le shell et conclu qu'ils
	// n'existaient pas — mon grep excluait les `.toml`. D'où la règle : la
	// liste se dérive de ce que le MOTEUR lit, pas de ce que le disque porte.
	//
	// `xbox_hdd.qcow2` n'est pas un firmware mais une image de disque, et elle
	// est déclarée quand même : elle vit dans `bios/xbox/`, l'entrypoint la
	// recopie au premier démarrage, et sans elle la console ne part pas. Ce que
	// cette liste dit, c'est « ce qu'il faut avoir ici » — pas une taxonomie.
	//
	// `cromwell_1024.bin`, présent sur certains disques, n'est référencé par
	// aucun code : ce n'est pas une exigence, donc il n'est pas déclaré.
	{ID: "xbox", Name: "Xbox", Publisher: "Microsoft", Year: 2001, Method: MethodStreamex, Cores: []string{"xemu (streamex)"}, MaxPlayers: 4, Width: 640, Height: 480, Bios: []string{"mcpx_1.0.bin", "Complex_4627.bin", "xbox_hdd.qcow2"}, Enabled: true},
	// xbox360 — streamex + Xenia Canary, sa version Linux (#414, branchée le
	// 2026-10-22 avec le premier jeu de la bibliothèque, Halo 3). Le même
	// gabarit que la Xbox : pas d'extension déclarée — un jeu est un dossier,
	// et c'est l'adaptateur de l'image qui y trouve l'ISO —, quatre joueurs
	// (quatre ports de manette, que Xenia ouvre par SDL), et la sortie de la
	// console, 1280×720, qui est celle de presque tous ses jeux. Aucun
	// firmware : Xenia n'en demande pas.
	//
	// Le nom de l'émulateur ne doit contenir aucun nom d'invité du téléphone
	// (xemu, rpcs3…) : le téléphone range un cœur chez l'invité dont le nom y
	// figure (GlibcGuest.guestForCore).
	{ID: "xbox360", Name: "Xbox 360", Publisher: "Microsoft", Year: 2005, Method: MethodStreamex, Cores: []string{"Xenia Canary (streamex)"}, MaxPlayers: 4, Width: 1280, Height: 720, Experimental: ExperimentalGeneral, Enabled: true},
	{ID: "xboxone", Name: "Xbox One", Publisher: "Microsoft", Year: 2013, Method: MethodStreamex, Cores: []string{}, MaxPlayers: 2, Enabled: false},

	// ── Consoles jouées À DISTANCE : identifiants préfixés `rp-` ──
	//
	// LE TEMPORAIRE PORTE LE QUALIFICATIF, PAS LE DURABLE (décidé le
	// 2026-09-10).
	//
	// Jouer une vraie console à distance est une étape : celles d'aujourd'hui
	// seront émulées demain, comme la PS3 et la PS4 l'ont été. C'est donc la
	// variante remote play qui est provisoire, et c'est elle qui doit porter la
	// marque — pas celle qui restera.
	//
	// Et le coût n'est pas symétrique : un identifiant streamex ou rétro traîne
	// derrière lui `roms/<id>/`, `bios/<id>/` et `cores/<id>/` sur le disque de
	// quelqu'un, plus les URL qu'on a partagées. Un identifiant remote play ne
	// traîne rien : le renommer ne coûte que du code, et le faire MAINTENANT ne
	// coûte presque rien.
	// LA PS4 À DISTANCE : LE MÊME CODE, paramétré par `hostType`.
	//
	// Elle manquait alors que le panneau d'appairage l'acceptait déjà — on
	// pouvait donc appairer une PS4 sans avoir de ligne de menu pour y jouer.
	// Rien de spécifique à écrire : le protocole partagé porte ses tables PS4
	// (`crypto/reg_key_1_ps4.go`, `session_key_0_ps4.go`), sa dérivation de
	// clés branche sur `hostType == "PS4"`, et le réveil par le réseau change
	// de port pour elle (`discovery/wake.go`). Ajoutée le 2026-09-10 en même
	// temps que le préfixe, pour prendre de l'avance.
	//
	// PAS ESSAYÉE SUR MATÉRIEL : aucune PS4 ici. La console apparaît, tout le
	// chemin est celui de la PS5 — mais personne ne l'a encore vue marcher.
	{ID: "rp-ps4", Name: "PlayStation 4", Publisher: "Sony", Year: 2013, Method: MethodRemotePlay, Cores: []string{}, MaxPlayers: 1, Enabled: true},
	{ID: "rp-ps5", Name: "PlayStation 5", Publisher: "Sony", Year: 2020, Method: MethodRemotePlay, Cores: []string{}, MaxPlayers: 1, Enabled: true},
	{ID: "rp-xboxseries", Name: "Xbox Series X|S", Publisher: "Microsoft", Year: 2020, Method: MethodRemotePlay, Cores: []string{}, MaxPlayers: 1, Enabled: false},
	{ID: "rp-steam", Name: "Steam (PC)", Publisher: "Valve", Year: 2003, Method: MethodRemotePlay, Cores: []string{}, MaxPlayers: 1, Enabled: false},
	{ID: "rp-switch2", Name: "Nintendo Switch 2", Publisher: "Nintendo", Year: 2025, Method: MethodRemotePlay, Cores: []string{}, MaxPlayers: 1, Enabled: false},
}

var byID = func() map[string]Console {
	m := make(map[string]Console, len(registry))
	for _, c := range registry {
		m[c.ID] = c
	}
	return m
}()

// List returns every enabled console — what GET /api/v1/consoles serves.
func List() []Console {
	out := make([]Console, 0, len(registry))
	for _, c := range registry {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out
}

// All returns EVERY console of the registry, désactivées comprises.
//
// La différence avec List() est une différence de question. `Enabled` dit
// « GameFlex sait servir cette console aujourd'hui » — une propriété du
// serveur. Reconnaître un dossier nommé « scummvm » n'est pas la même
// question : le dossier veut dire ce qu'il veut dire, que la plateforme soit
// branchée ou non. Et sur un téléphone, la jouabilité ne dépend même pas de
// ce drapeau : elle dépend du core que l'utilisateur a téléchargé.
//
// Un scanner qui filtrerait sur Enabled rendrait donc une bibliothèque
// amputée sans jamais dire pourquoi.
func All() []Console {
	out := make([]Console, len(registry))
	copy(out, registry)

	return out
}

// Stored returns the consoles that OWN FOLDERS on someone's disk.
//
// Troisième question, distincte des deux précédentes, et c'est la règle posée
// par l'utilisateur : « un identifiant streamex ou rétro traîne derrière lui
// roms/<id>/, bios/<id>/, cores/<id>/ sur le disque de quelqu'un ; un
// identifiant remote play ne traîne rien ». Une PS5 se joue à distance — elle
// n'a ni ROM ni BIOS ici, par construction.
//
// D'où `Enabled && Method != MethodRemotePlay` :
//
//   - `All()` créerait des dossiers pour des consoles désactivées. C'est ce
//     que faisait le téléphone, et il en avait neuf qui ne servaient à rien —
//     atari5200, scummvm, xbox360, xboxone, rp-steam… (constaté le 2026-09-27).
//   - `List()` seul laisserait passer les plateformes de jeu à distance.
//   - Et surtout PAS « la console déclare des BIOS » (`len(Bios) > 0`), le
//     prédicat des écrans de stockage : il ferait disparaître bios/ps2, dont
//     le registre ne déclare rien EXPRÈS — n'importe quel dump convient.
//
// Lister n'est pas créer : cette fonction dit quels dossiers ont un sens, pas
// lesquels existent. Rien ici ne supprime quoi que ce soit.
func Stored() []Console {
	out := make([]Console, 0, len(registry))
	for _, console := range registry {
		if console.Enabled && console.Method != MethodRemotePlay {
			out = append(out, console)
		}
	}

	return out
}

// ExtensionsFor returns a console's file extensions, lowercase and without
// the dot. Empty when the console has none worth checking (a scummvm game is
// a directory, not a file) — an empty list means "the folder decides alone",
// never "reject everything".
func ExtensionsFor(id string) []string {
	return byID[id].Extensions
}

// archives — des CONTENEURS, pas des formats de console.
//
// C'est ainsi que les sets de ROMs circulent, et les cores libretro les lisent
// tels quels. Découvert en passant le scanner sur une vraie bibliothèque le
// 2026-08-26 : 11 640 jeux sur 23 000 étaient dans des .zip, et la règle
// d'extension les rejetait tous — gba, snes, nds, sms et megadrive avaient
// entièrement disparu.
//
// Une archive est donc acceptée pour toute console : on ne peut pas confirmer
// ce qu'elle contient sans l'ouvrir, et ouvrir 11 000 archives à travers un
// partage réseau pour dresser une liste serait ruineux. Le dossier décide
// seul, comme pour une console sans extensions déclarées.
//
// Ce n'est pas qu'un repli : pour neogeo et les CPS, le .zip EST le format du
// jeu (un set MAME), et il est déclaré comme tel dans le registre.
var archives = map[string]bool{"zip": true, "7z": true}

// Accepts reports whether a filename's extension is plausible for a console.
// TRUE when the console declares no extensions: the absence of a second
// signal must not turn into a refusal.
func Accepts(id, filename string) bool {
	known := byID[id].Extensions

	dot := strings.LastIndexByte(filename, '.')
	if dot < 0 || dot == len(filename)-1 {
		return len(known) == 0
	}
	ext := strings.ToLower(filename[dot+1:])

	if archives[ext] {
		return true
	}
	if len(known) == 0 {
		return true
	}

	for _, candidate := range known {
		if candidate == ext {
			return true
		}
	}

	return false
}

// CoresFor returns a console's known libretro core names, most-preferred
// first (index 0 is the default used when a session starts without an
// explicit core choice — see internal/retrogaming/stream_session.go's
// handleStart). Empty for a console with no working core yet.
func CoresFor(id string) []string {
	return byID[id].Cores
}

// MethodOf — le moteur d'une console (MethodRetroGaming, MethodStreamex…),
// vide pour un identifiant inconnu.
func MethodOf(id string) string {
	return byID[id].Method
}

// NativeSize — la géométrie du cœur, ou (0, 0) quand elle varie ou qu'on ne
// la connaît pas. L'appelant garde alors son propre défaut.
func NativeSize(id string) (int, int) {
	if c, ok := byID[id]; ok {
		return c.Width, c.Height
	}

	return 0, 0
}

func MaxPlayers(id string) int {
	if c, ok := byID[id]; ok {
		return c.MaxPlayers
	}
	return 2
}

// SeatsOr — les places d'une PARTIE de cette console : celles du registre
// (Wii U 2 ; PS3, PS4, PS5 et Xbox 4 ; Switch 8…), ou `fallback` pour ce qu'il
// ne connaît pas. Le banc KeyTest n'y est pas, et c'est voulu : mesurer la
// latence à plusieurs demande autant de manettes que le conteneur en porte.
//
// Streamex donnait 4 places à toutes ses consoles — sa file prenait le
// plafond général du conteneur, jamais ce nombre-ci (relevé le 2026-10-09).
func SeatsOr(id string, fallback int) int {
	if c, ok := byID[id]; ok && c.MaxPlayers > 0 {
		return c.MaxPlayers
	}

	return fallback
}

// ── Les dossiers du stockage, par console ───────────────────────────────
//
// UN SEUL VOCABULAIRE : le dossier de BIOS d'une console porte le nom de la
// console.
//
// La table qui vivait ici traduisait vers les noms d'ES-DE (« psx » pour la
// PlayStation, « genesis » pour la Mega Drive). Elle a coûté un vrai bug :
// un BIOS déposé au bon endroit restait invisible, et la fiche du jeu
// réclamait un fichier déjà installé (2026-09-01). Une traduction entre deux
// vocabulaires est une chose de plus à tenir juste, et celle-ci ne rendait
// aucun service — rien d'extérieur ne lit ces dossiers.
//
// Décidé le 2026-09-01, sans reprise de l'ancien nommage. Les dossiers
// hérités ont été renommés une fois, à la main, à ce moment-là : il ne reste
// aucune migration au démarrage — le commentaire qui renvoyait ici à
// `Library.migrateBiosDirs` citait une fonction disparue (corrigé le
// 2026-09-12, seule occurrence du nom dans tout le dépôt).

// BiosDir — le sous-dossier de BIOS d'une console. C'est son identifiant.
func BiosDir(consoleID string) string {
	return consoleID
}

// ConsolesForCore — les consoles qu'un core sert. Un core en sert souvent
// PLUSIEURS (genesis_plus_gx couvre sms et megadrive, fbalpha2012_cps1 ne
// couvre que cps1) : c'est ce qui empêche de déduire la console d'un core
// depuis son seul nom de fichier.
func ConsolesForCore(core string) []string {
	var found []string
	for _, console := range registry {
		if !console.Enabled {
			continue
		}
		for _, candidate := range console.Cores {
			if candidate == core {
				found = append(found, console.ID)

				break
			}
		}
	}

	return found
}

// Exists — cet identifiant est-il celui d'une console du registre ?
func Exists(consoleID string) bool {
	for _, console := range registry {
		if console.ID == consoleID {
			return true
		}
	}

	return false
}

// BiosFor rend TOUS les fichiers BIOS que cette console DÉCLARE — les requis
// puis les facultatifs.
//
// « Déclaré », pas « requis », et c'est bien ce que ses quatre appelants
// demandent : le staging avant lancement (runtimes/retrogaming/bios.go),
// l'appartenance d'un fichier à une console lors d'un import ou d'une
// suppression (federation_devices.go), et les deux écrans de stockage (kernel
// et mobileapp). Aucun des quatre ne pose la question « dois-je refuser de
// lancer ? » ; tous posent « ce fichier-là, la console le connaît-elle ? ».
//
// MESURÉ, et c'est pour ça que la distinction compte (#161) : si cette
// fonction ne rendait que les requis, la PS1 — dont les six dumps sont
// facultatifs depuis que le HLE est reconnu — rendrait une liste VIDE, et ses
// vrais BIOS cesseraient d'être stagés (la régression même que #144 venait de
// corriger), d'être rapatriables depuis un pair, et d'être listés à l'écran.
//
// Rendu vide plutôt que nil pour un identifiant inconnu : l'appelant boucle
// dessus, il ne teste pas son existence.
func BiosFor(id string) []string {
	return append(BiosRequiredFor(id), BiosOptionalFor(id)...)
}

// BiosRequiredFor rend les BIOS dont AU MOINS UN est exigé pour lancer cette
// console. Liste vide = aucun n'est nécessaire.
//
// « Au moins un », jamais « tous » : ce sont des révisions du même BIOS (les
// deux dumps Saturn, les trois de la DS), et c'est déjà la règle que la webapp
// applique de son côté (`missingBiosFor` : `wanted.some(present)`). C'est
// cette fonction, et non BiosFor, que doit interroger tout code qui décide
// d'autoriser ou de refuser un lancement.
func BiosRequiredFor(id string) []string {
	for _, console := range registry {
		if console.ID == id {
			out := make([]string, len(console.Bios))
			copy(out, console.Bios)

			return out
		}
	}

	return []string{}
}

// MissingRequiredBios — ce qui manque pour lancer cette console : rien quand
// l'un de ses BIOS requis est présent (« au moins un », des révisions du même
// BIOS), sinon la liste des requis. Un BIOS facultatif n'arrête rien.
//
// LA RÈGLE DE LA PAGE (`missingRequiredBios`, launch-mode.js), noms comparés
// sans la casse, pour que les deux kernels refusent ce qu'elle grise — un
// lancement par la façade ou par un lien passait sans firmware (#393,
// 2026-10-16 : « si l'émulateur n'a pas besoin du bios on doit pouvoir lancer
// le jeu sans problème. Mais c'est manquant alors le Play devient disabled »).
func MissingRequiredBios(id string, present []string) []string {
	wanted := BiosRequiredFor(id)
	if len(wanted) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, name := range present {
		have[strings.ToLower(name)] = true
	}
	for _, name := range wanted {
		if have[strings.ToLower(name)] {
			return nil
		}
	}

	return wanted
}

// BiosOptionalFor rend les BIOS qui améliorent cette console sans être exigés
// — voir le champ BiosOptional. Leur absence s'affiche, elle ne bloque pas.
func BiosOptionalFor(id string) []string {
	for _, console := range registry {
		if console.ID == id {
			out := make([]string, len(console.BiosOptional))
			copy(out, console.BiosOptional)

			return out
		}
	}

	return []string{}
}
