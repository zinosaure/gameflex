package metadata

// LE FOND DE L'ÉCRAN D'UNE CONSOLE (chantier 49, #659, pour le launcher #660)
// — une bande de jaquettes des grands jeux de la console, jamais tirée de la
// bibliothèque du joueur : une console sans jeu a le même fond que les
// autres.
//
// Le fond, c'est le dossier `metadata/<console>/background/` : les images
// qui s'y trouvent (PNG, JPEG, WebP), dans l'ordre de leur nom. Le joueur en
// ajoute, en retire ou en remplace à la main, et ce qui est là est ce qui se
// montre.
//
// Par défaut, le serveur le remplit une fois, au premier besoin, avec les
// jaquettes des jeux de consoleBackgroundGames (01.jpg, 02.jpg… dans l'ordre
// de la liste) : libretro-thumbnails (`Named_Boxarts`, sans compte), sinon
// IGDB puis SteamGridDB quand leurs comptes sont saisis. Le dépôt ne garde
// que les noms : les jaquettes ont des droits, aucune image de jeu n'entre
// dans git. Le remplissage fini, `background/.seeded` le note, et il ne se
// refait plus : un dossier vidé exprès reste vide. Un fichier déjà là n'est
// jamais écrasé.
//
// Les réductions vont dans `cache/metadata/console-backgrounds/<console>/`,
// jamais dans le dossier du joueur.
//
// UNE CONSOLE SANS AUCUNE IMAGE ICI MONTRE CELLES DU PREMIER SERVEUR FÉDÉRÉ
// (`#740` ; le porteur, 2026-11-16 : « il faut que par défaut les consoles
// prennent les images des serveurs fédérés si en local on n'a pas d'images
// background. On prend le premier fédéré pour ne pas se prendre la tête. ») :
// ShownBackgrounds. Quand le dossier n'a aucune image — rien de déposé, et le
// remplissage par défaut n'a rien trouvé (aucun compte, une console que
// libretro n'a pas) —, le premier serveur de l'annuaire, dans son ordre, qui
// prête des images les donne ; celui qui ne répond pas, refuse notre jeton,
// n'a pas la route (une version plus ancienne) ou n'en a pas laisse la place au
// suivant. La liste dit alors de quel serveur elles viennent, et ses images se
// RELAIENT de chez lui : rien ne s'écrit ici. Les gestes (remplacer,
// réinitialiser) restent sur les images de cette machine ; une image remplacée
// ici, et la console montre de nouveau les siennes. Un pair ne prête que les
// siennes (ConsoleBackgrounds), jamais celles qu'il tiendrait d'un autre : deux
// serveurs qui se fédèrent l'un l'autre ne se renvoient pas la question.
//
// HUIT EMPLACEMENTS (chantier 53, P9 ; le porteur, 2026-11-14, remarque e :
// « Dans console appearance, gardons maximum 8 images par console. On peut
// donc remplacer des images dans les 1 à 8 slots seulement. ») : l'emplacement
// N est l'image `0N.<ext>` du dossier, celle que le remplissage par défaut
// écrit pour le N-ième jeu de sa liste. La bande ne montre que les
// emplacements. Un autre fichier d'image du dossier est EN TROP : la liste le
// signale, et rien ne l'efface — ce sont les fichiers du joueur.
//
// L'administrateur les change par l'API (Réglages › Consoles › une console ›
// Apparence du launcher) : remplacer l'image d'un emplacement par une image
// choisie chez un fournisseur (image_search.go) — le porteur, 2026-11-14 : « on
// aura toujours 8 images. On ne peut que les remplacer » —, remettre un
// emplacement par défaut (l'image de fond d'une console Remote Play), ou les
// huit (la vraie remise à zéro : ResetConsoleBackgrounds). Le serveur remplit
// toujours les huit emplacements ; quand les fournisseurs n'ont pas l'image d'un
// jeu de la liste, l'emplacement reste vide — rien n'est fabriqué.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"gameflex/consoles"
)

// consoleBackgroundGames — les grands jeux de chaque console, par leur nom
// exact dans libretro-thumbnails (vérifiés le 2026-11-12 contre l'index
// `Named_Boxarts` de chaque système, aucun deviné), sinon par le titre
// qu'IGDB connaît pour les consoles que libretro n'a pas (PS5, Switch,
// Switch 2, Xbox One).
//
// La PS4 n'a que 20 jaquettes chez libretro, presque toutes japonaises :
// les huit retenues en viennent, pour un fond sans compte. Les ports PC
// prennent les jaquettes N64 des jeux que la communauté a portés, Majora's
// Mask (Zelda 64: Recompiled) en tête.
var consoleBackgroundGames = map[string][]string{
	"nes": {
		"Super Mario Bros. (World)",
		"Legend of Zelda, The (USA)",
		"Super Mario Bros. 3 (USA)",
		"Metroid (USA)",
		"Mega Man 2 (USA)",
		"Castlevania (USA)",
		"Punch-Out!! (USA)",
		"Kirby's Adventure (USA)",
	},
	"snes": {
		"Super Mario World (USA)",
		"Legend of Zelda, The - A Link to the Past (USA)",
		"Super Metroid (Japan, USA) (En,Ja)",
		"Chrono Trigger (USA)",
		"Donkey Kong Country (USA)",
		"Super Mario Kart (USA)",
		"Final Fantasy III (USA)",
		"Street Fighter II Turbo (USA)",
	},
	"gbc": {
		"Pokemon - Gold Version (USA, Europe) (SGB Enhanced) (GB Compatible)",
		"Legend of Zelda, The - Link's Awakening DX (USA, Europe) (SGB Enhanced) (GB Compatible)",
		"Pokemon - Red Version (USA, Europe) (SGB Enhanced)",
		"Tetris (World) (Rev 1)",
		"Super Mario Land 2 - 6 Golden Coins (USA, Europe)",
		"Wario Land 3 (World) (En,Ja)",
		"Legend of Zelda, The - Oracle of Seasons (USA, Australia)",
		"Metroid II - Return of Samus (World)",
	},
	"gba": {
		"Pokemon - Emerald Version (USA, Europe)",
		"Legend of Zelda, The - The Minish Cap (USA)",
		"Metroid Fusion (USA)",
		"Castlevania - Aria of Sorrow (USA)",
		"Advance Wars (USA)",
		"Golden Sun (USA, Europe)",
		"Mario Kart - Super Circuit (USA)",
		"Final Fantasy Tactics Advance (USA)",
	},
	"vb": {
		"Mario's Tennis (Japan, USA)",
		"Galactic Pinball (Japan, USA)",
		"Red Alarm (USA)",
		"Teleroboxer (Japan, USA)",
		"Mario Clash (Japan, USA)",
		"Vertical Force (USA)",
		"Jack Bros. (USA)",
		"3-D Tetris (USA)",
	},
	"n64": {
		"Super Mario 64 (USA)",
		"Legend of Zelda, The - Ocarina of Time (USA)",
		"GoldenEye 007 (USA)",
		"Mario Kart 64 (USA)",
		"Super Smash Bros. (USA)",
		"Banjo-Kazooie (USA)",
		"Star Fox 64 (USA)",
		"Paper Mario (USA)",
	},
	"nds": {
		"New Super Mario Bros. (USA)",
		"Mario Kart DS (USA) (En,Fr,De,Es,It)",
		"Pokemon - HeartGold Version (USA)",
		"Professor Layton and the Curious Village (USA)",
		"Castlevania - Dawn of Sorrow (USA)",
		"Chrono Trigger (USA) (En,Fr)",
		"Phoenix Wright - Ace Attorney (USA)",
		"Pokemon - Platinum Version (USA)",
	},
	"gc": {
		"Super Smash Bros. Melee (USA)",
		"Legend of Zelda, The - The Wind Waker (USA)",
		"Metroid Prime (USA)",
		"Super Mario Sunshine (USA)",
		"Mario Kart - Double Dash!! (USA)",
		"Resident Evil 4 (USA)",
		"Paper Mario - The Thousand-Year Door (USA)",
		"F-Zero GX (USA)",
	},
	"wii": {
		"Super Mario Galaxy (USA)",
		"Mario Kart Wii (USA)",
		"Super Smash Bros. Brawl (USA)",
		"Legend of Zelda, The - Twilight Princess (USA)",
		"Xenoblade Chronicles (USA)",
		"Wii Sports (USA)",
		"Metroid Prime 3 - Corruption (USA)",
		"Donkey Kong Country Returns (USA)",
	},
	"wiiu": {
		"Mario Kart 8 (USA)",
		"Super Mario 3D World (USA)",
		"Legend of Zelda, The - Breath of the Wild (USA) (En,Fr,Es)",
		"Splatoon (USA) (En,Fr,Es)",
		"Super Smash Bros. for Wii U (USA) (En,Fr,Es)",
		"Bayonetta 2 (USA)",
		"Donkey Kong Country - Tropical Freeze (USA) (En,Fr,Es)",
		"Xenoblade Chronicles X (USA) (En,Fr,Es)",
	},
	"switch": {
		"The Legend of Zelda: Breath of the Wild",
		"Super Mario Odyssey",
		"Mario Kart 8 Deluxe",
		"Super Smash Bros. Ultimate",
		"Animal Crossing: New Horizons",
		"The Legend of Zelda: Tears of the Kingdom",
		"Metroid Dread",
		"Xenoblade Chronicles 3",
	},
	"switch2": {
		"Mario Kart World",
		"Donkey Kong Bananza",
		"Pokémon Legends: Z-A",
		"Kirby Air Riders",
		"Metroid Prime 4: Beyond",
		"Hyrule Warriors: Age of Imprisonment",
	},
	"sms": {
		"Alex Kidd in Miracle World (USA, Europe)",
		"Sonic The Hedgehog (USA, Europe, Brazil) (En)",
		"Phantasy Star (USA, Europe)",
		"Wonder Boy III - The Dragon's Trap (USA, Europe)",
		"Castle of Illusion Starring Mickey Mouse (USA, Europe, Brazil) (En)",
		"R-Type (World)",
		"Golden Axe Warrior (USA, Europe, Brazil) (En)",
		"Psycho Fox (USA, Europe, Brazil) (En)",
	},
	"megadrive": {
		"Sonic The Hedgehog (USA, Europe)",
		"Sonic The Hedgehog 2 (World)",
		"Streets of Rage 2 (USA)",
		"Gunstar Heroes (USA)",
		"Phantasy Star IV (USA)",
		"Shinobi III - Return of the Ninja Master (USA)",
		"Castlevania - Bloodlines (USA)",
		"Comix Zone (USA)",
	},
	"saturn": {
		"Nights Into Dreams... (USA)",
		"Panzer Dragoon Saga (USA)",
		"Virtua Fighter 2 (USA)",
		"Sega Rally Championship (USA)",
		"Guardian Heroes (USA)",
		"Radiant Silvergun (Japan)",
		"Burning Rangers (USA)",
		"Shining Force III (USA)",
	},
	"dreamcast": {
		"Sonic Adventure (USA)",
		"Shenmue (USA)",
		"Soulcalibur (USA)",
		"Jet Grind Radio (USA)",
		"Crazy Taxi (USA)",
		"Skies of Arcadia (USA)",
		"Marvel vs. Capcom 2 (USA)",
		"Resident Evil - Code - Veronica (USA)",
	},
	"atari2600": {
		"Pitfall! - Pitfall Harry's Jungle Adventure (USA)",
		"Adventure (USA)",
		"Space Invaders (USA)",
		"River Raid (USA)",
		"Asteroids (USA)",
		"Missile Command (USA)",
		"Yars' Revenge (USA)",
		"Combat (USA)",
	},
	"atari5200": {
		"Pac-Man (USA)",
		"Centipede (USA)",
		"Star Raiders (USA)",
		"Missile Command (USA)",
		"Defender (USA)",
		"Pitfall! (USA)",
		"Joust (USA)",
		"Galaxian (USA)",
	},
	"atari7800": {
		"Ms. Pac-Man (USA)",
		"Joust (USA)",
		"Centipede (USA)",
		"Food Fight (USA)",
		"Galaga (USA)",
		"Dig Dug (USA)",
		"Ninja Golf (USA)",
		"Robotron - 2084 (USA)",
	},
	"lynx": {
		"California Games (World)",
		"Chip's Challenge (World)",
		"Blue Lightning (World)",
		"Gates of Zendocon (World)",
		"Klax (World)",
		"Rampage (World)",
		"Todd's Adventures in Slime World (World)",
		"Gauntlet - The Third Encounter (World)",
	},
	"jaguar": {
		"Alien vs Predator (World)",
		"Tempest 2000 (World)",
		"Doom (World)",
		"Wolfenstein 3D (World)",
		"Rayman (World)",
		"Iron Soldier (World)",
		"Defender 2000 (World)",
		"Atari Karts (World)",
	},
	"pce": {
		"Bonk's Adventure (USA)",
		"Blazing Lazers (USA)",
		"R-Type (USA)",
		"Splatterhouse (USA)",
		"Bomberman '93 (USA)",
		"Military Madness (USA)",
		"Devil's Crush (USA)",
		"Neutopia (USA)",
	},
	"pcfx": {
		"Battle Heat (Japan)",
		"Langrisser FX, Der (Japan)",
		"Cutey Honey FX (Japan)",
		"Return to Zork (Japan)",
		"Power DoLLS FX (Japan)",
		"Kishin Douji Zenki FX - Vajra Fight (Japan)",
		"Team Innocent - The Point of No Return - G.C.P.O.SS (Japan)",
		"Super Power League FX (Japan)",
	},
	"neogeo": {
		"Metal Slug - Super Vehicle-001",
		"Metal Slug 2 - Super Vehicle-001_II (NGM-2410) (NGH-2410)",
		"Metal Slug 3 (NGM-2560)",
		"Garou - Mark of the Wolves (NGM-2530)",
		"Samurai Shodown II _ Shin Samurai Spirits - Haohmaru jigokuhen (NGM-063)(NGH-063)",
		"Fatal Fury Special _ Garou Densetsu Special (set 1)(NGM-058)(NGH-058)",
		"Blazing Star",
		"Windjammers _ Flying Power Disc",
	},
	"ngp": {
		"SNK vs. Capcom - The Match of the Millennium (World) (En,Ja)",
		"Metal Slug - 2nd Mission (World) (En,Ja)",
		"Sonic The Hedgehog - Pocket Adventure (World)",
		"King of Fighters R-2 - Pocket Fighting Series (World) (En,Ja)",
		"Last Blade, The - Beyond the Destiny (USA, Europe)",
		"Faselei! (Europe)",
		"Biomotor Unitron (USA, Europe)",
		"Dark Arms - Beast Buster 1999 (World) (En,Ja)",
	},
	"cps1": {
		"Street Fighter II_ The World Warrior (USA 910206)",
		"Street Fighter II'_ Champion Edition (World 920513)",
		"Final Fight (World, set 1)",
		"Ghouls'n Ghosts (World)",
		"Strider (USA, B-Board 89624B-2)",
		"Captain Commando (USA 910928)",
		"Knights of the Round (World 911127)",
		"Mercs (World 900302)",
	},
	"cps2": {
		"Super Street Fighter II Turbo (World 940223)",
		"Street Fighter Alpha 3 (USA 980904)",
		"Darkstalkers_ The Night Warriors (USA 940705)",
		"Vampire Savior_ The Lord of Vampire (USA 970519)",
		"Dungeons _ Dragons_ Shadow over Mystara (USA 960209)",
		"Alien vs. Predator (USA 940520)",
		"1944_ The Loop Master (USA 000620)",
		"Armored Warriors (USA 941024)",
	},
	"wswan": {
		"Final Fantasy (Japan)",
		"Final Fantasy IV (Japan)",
		"GunPey EX (Japan)",
		"Judgement Silversword - Rebirth Edition (Japan) (Rev 5C21)",
		"Makaimura for WonderSwan (Japan)",
		"Digimon Adventure 02 - D1 Tamers (Japan)",
		"Pocket Fighter (Japan)",
		"Rhyme Rider Kerorican (Japan)",
	},
	"coleco": {
		"Donkey Kong (USA)",
		"Zaxxon (USA, Europe)",
		"Venture (USA, Europe)",
		"Mr. Do! (USA, Europe)",
		"Lady Bug (USA, Europe)",
		"Smurf - Rescue in Gargamel's Castle (USA, Europe)",
		"BurgerTime (USA, Europe)",
		"Donkey Kong Junior (USA, Europe)",
	},
	"vectrex": {
		"Mine Storm (World)",
		"Scramble (USA, Europe)",
		"Berzerk (World)",
		"Star Trek - The Motion Picture (USA)",
		"Fortress of Narzod (USA, Europe)",
		"Armor..Attack (World)",
		"Solar Quest (World)",
		"Rip Off (World)",
	},
	"c64": {
		"Impossible Mission (USA, Europe)",
		"Boulder Dash (USA, Europe)",
		"Maniac Mansion (USA, Europe)",
		"Last Ninja, The (USA, Europe)",
		"Turrican (USA, Europe)",
		"Bubble Bobble (USA, Europe)",
		"Wizball (USA, Europe)",
		"Paradroid (USA, Europe)",
	},
	"amstrad": {
		"Gryzor (Ocean)",
		"Rick Dangerous (Firebird)",
		"Prince of Persia (Broderbund Software)",
		"Renegade (Imagine Software)",
		"Head over Heels (Ocean Software)",
		"Batman (Ocean Software)",
		"Sorcery (Virgin Games)",
		"Operation Wolf (Ocean)",
	},
	"zxspectrum": {
		"Jet Set Willy (Software Projects Ltd)",
		"Head over Heels (Ocean Software Ltd)",
		"Saboteur! (Durell Software Ltd)",
		"Great Escape, The (Ocean Software Ltd)",
		"Match Day (Ocean Software Ltd)",
		"Exolon (Hewson Consultants Ltd)",
		"Skool Daze (Microsphere)",
		"Rainbow Islands (Ocean Software Ltd)",
	},
	"doom": {
		"Doom",
		"Doom II - Hell on Earth",
		"Final Doom",
		"Heretic",
		"Hexen - Beyond Heretic",
		"Chex Quest",
	},
	"scummvm": {
		"Secret of Monkey Island, The",
		"Monkey Island 2_ LeChuck's Revenge",
		"Day of the Tentacle",
		"Full Throttle",
		"Sam _ Max Hit the Road",
		"Indiana Jones and the Fate of Atlantis",
		"Beneath a Steel Sky",
		"Grim Fandango",
	},
	"ps1": {
		"Final Fantasy VII (USA)",
		"Metal Gear Solid (USA)",
		"Castlevania - Symphony of the Night (USA)",
		"Gran Turismo 2 (USA)",
		"Crash Bandicoot (USA)",
		"Resident Evil 2 (USA)",
		"Tekken 3 (USA)",
		"Silent Hill (USA)",
	},
	"ps2": {
		"Grand Theft Auto - San Andreas (USA) (v1.03)",
		"Shadow of the Colossus (USA)",
		"Final Fantasy X (USA, Canada)",
		"Metal Gear Solid 3 - Snake Eater (USA)",
		"God of War (USA)",
		"Kingdom Hearts (USA)",
		"Ico (USA)",
		"Silent Hill 2 (USA) (En,Ja) (v1.20)",
	},
	"psp": {
		"God of War - Chains of Olympus (USA)",
		"Monster Hunter Freedom Unite (USA) (En,Fr,De,Es,It)",
		"Crisis Core - Final Fantasy VII (USA)",
		"Metal Gear Solid - Peace Walker (USA) (En,Fr,Es) (v1.01)",
		"Grand Theft Auto - Liberty City Stories (USA) (En,Fr,De,Es,It) (v1.05)",
		"Patapon (USA)",
		"Daxter (USA)",
		"Final Fantasy Tactics - The War of the Lions (USA)",
	},
	"ps3": {
		"Grand Theft Auto V (USA)",
		"Demon's Souls (USA)",
		"Heavy Rain (USA)",
		"Metal Gear Solid V - Ground Zeroes (USA)",
		"Final Fantasy XIII (USA)",
		"Kingdom Hearts HD 2.5 ReMIX (USA)",
		"Resident Evil 5 (USA)",
		"Valkyria Chronicles (USA)",
	},
	"ps4": {
		"Persona 5 (USA)",
		"Biohazard 7 - Resident Evil (Japan)",
		"Dark Souls III (Japan)",
		"Final Fantasy XV (Japan)",
		"NieR Automata (Japan)",
		"Nioh (Japan)",
		"Gravity Rush Remastered (Europe)",
		"Final Fantasy X & X-2 HD Remaster (Europe)",
	},
	"ps5": {
		"Astro Bot",
		"Marvel's Spider-Man 2",
		"God of War Ragnarök",
		"Demon's Souls",
		"Ratchet & Clank: Rift Apart",
		"Final Fantasy VII Rebirth",
		"Returnal",
		"Gran Turismo 7",
	},
	"xbox": {
		"Halo - Combat Evolved (USA)",
		"Halo 2 (USA)",
		"Fable (USA)",
		"Star Wars - Knights of the Old Republic (USA)",
		"Jet Set Radio Future (USA)",
		"Ninja Gaiden Black (USA)",
		"Project Gotham Racing 2 (USA)",
		"Panzer Dragoon Orta (USA)",
	},
	"xbox360": {
		"Bayonetta (USA)",
		"Portal 2 (USA)",
		"Final Fantasy XIII (USA)",
		"Dead Rising (USA)",
		"Catherine (USA)",
		"Super Street Fighter IV (Europe)",
		"Dead or Alive 5 (USA)",
		"Lollipop Chainsaw (Japan)",
	},
	"xboxone": {
		"Halo 5: Guardians",
		"Forza Horizon 4",
		"Gears 5",
		"Sea of Thieves",
		"Ori and the Will of the Wisps",
		"Halo: The Master Chief Collection",
		"Titanfall 2",
		"Cuphead",
	},
	"ports": {
		"Legend of Zelda, The - Majora's Mask (USA)",
		"Legend of Zelda, The - Ocarina of Time (USA)",
		"Super Mario 64 (USA)",
		"Star Fox 64 (USA)",
		"Mario Kart 64 (USA)",
		"Perfect Dark (USA)",
		"Banjo-Kazooie (USA)",
		"GoldenEye 007 (USA)",
	},
}

// consoleBackgroundAliases — une console qui prend la liste et les sources
// d'une autre : le jeu à distance d'une PS4 a les jeux de la PS4.
var consoleBackgroundAliases = map[string]string{
	"rp-ps4":     "ps4",
	"rp-ps5":     "ps5",
	"rp-switch2": "switch2",
}

// consoleBackgroundSources — la console dont les sources servent : celle
// des ports lit les ROM de la N64, et libretro range ses jaquettes là.
var consoleBackgroundSources = map[string]string{
	consoles.PortsID: "n64",
}

// backgroundMarker — la trace du remplissage par défaut, dans le dossier du
// joueur ; un point en tête, pour qu'elle ne compte pas comme une image.
const backgroundMarker = ".seeded"

// backgroundName — un nom de fichier du dossier : pas de chemin, pas de
// point en tête, une extension d'image.
var backgroundName = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9 _.,()'+-]{0,127}\.(png|jpe?g|webp)$`)

// backgroundSeed — ce que `.seeded` garde : chaque jeu de la liste, son
// fichier et d'où il vient (ou pourquoi il n'est pas là).
type backgroundSeed struct {
	SeededAt time.Time            `json:"seededAt"`
	Games    []backgroundSeedGame `json:"games"`
}

type backgroundSeedGame struct {
	Name   string `json:"name"`
	File   string `json:"file,omitempty"`
	Source string `json:"source,omitempty"` // libretro, igdb, steamgriddb, kept (déjà là)
	Result string `json:"result"`           // found, kept, notFound, error, skipped
}

var (
	seedsMutex   sync.Mutex
	seedsRunning = map[string]chan struct{}{}
	seedsFailed  = map[string]time.Time{}
)

// ConsoleBackgroundDir — le dossier du fond d'une console.
func ConsoleBackgroundDir(storageRoot, consoleID string) string {
	return filepath.Join(storageRoot, "metadata", consoleID, "background")
}

// BackgroundSlotCount — les emplacements du fond d'une console.
const BackgroundSlotCount = 8

// BackgroundSlot — un emplacement et son image ; `name` vide quand il est
// libre.
type BackgroundSlot struct {
	Slot int    `json:"slot"`
	Name string `json:"name"`
}

// BackgroundSlots — le fond d'une console : ses huit emplacements, dans
// l'ordre, et les images du dossier en trop (aucun emplacement), dans l'ordre
// de leur nom. `server` et `serverName` : le serveur fédéré qui les prête
// (ShownBackgrounds), absents pour celles d'ici.
type BackgroundSlots struct {
	Slots      []BackgroundSlot `json:"slots"`
	Extra      []string         `json:"extra"`
	Server     int64            `json:"server,omitempty"`
	ServerName string           `json:"serverName,omitempty"`
}

// hasImage — le fond a au moins une image : un emplacement pris, ou une image
// du dossier en trop.
func (slots BackgroundSlots) hasImage() bool {
	for _, slot := range slots.Slots {
		if slot.Name != "" {
			return true
		}
	}

	return len(slots.Extra) > 0
}

// ErrBackgroundSlot — un emplacement hors de 1 à 8.
var ErrBackgroundSlot = errors.New("a background slot is 1 to 8")

// slotBase — le nom sans extension de l'image d'un emplacement : « 03 ».
func slotBase(slot int) string {
	return fmt.Sprintf("%02d", slot)
}

func validSlot(slot int) bool {
	return slot >= 1 && slot <= BackgroundSlotCount
}

// backgroundSlotsOf — les images du dossier rangées : la première (par son
// nom) de base « 0N » occupe l'emplacement N, toute autre est en trop.
func backgroundSlotsOf(names []string) BackgroundSlots {
	slots := BackgroundSlots{Slots: make([]BackgroundSlot, BackgroundSlotCount), Extra: []string{}}
	for index := range slots.Slots {
		slots.Slots[index].Slot = index + 1
	}
	for _, name := range names {
		placed := false
		for index := range slots.Slots {
			if slots.Slots[index].Name == "" && strings.TrimSuffix(name, filepath.Ext(name)) == slotBase(index+1) {
				slots.Slots[index].Name, placed = name, true
				break
			}
		}
		if !placed {
			slots.Extra = append(slots.Extra, name)
		}
	}

	return slots
}

// ConsoleBackgrounds — le fond de la console, en emplacements. Le premier
// appel lance le remplissage par défaut et l'attend au plus `wait` : ce qui
// n'est pas arrivé à temps sera là à la visite suivante.
func ConsoleBackgrounds(storageRoot string, accounts Accounts, consoleID string, wait time.Duration) (BackgroundSlots, error) {
	if !consoles.Exists(consoleID) {
		return BackgroundSlots{}, ErrUnknownConsole
	}
	if done := startBackgroundSeed(storageRoot, accounts, consoleID); done != nil {
		select {
		case <-done:
		case <-time.After(wait):
		}
	}

	return backgroundSlotsOf(listBackgrounds(ConsoleBackgroundDir(storageRoot, consoleID))), nil
}

// ShownBackgrounds — LE FOND QUE MONTRE UNE CONSOLE (voir l'en-tête, `#740`) :
// le sien (ConsoleBackgrounds, son remplissage par défaut compris) ; quand son
// dossier n'a aucune image, celui du premier de `peers` (l'annuaire, dans son
// ordre, lu seulement s'il le faut) qui en prête au moins une, avec son nom.
// Aucun ne le fait : le fond d'ici, vide.
func ShownBackgrounds(ctx context.Context, storageRoot string, accounts Accounts, consoleID string, wait time.Duration, peers func() []Peer) (BackgroundSlots, error) {
	local, err := ConsoleBackgrounds(storageRoot, accounts, consoleID, wait)
	if err != nil || local.hasImage() || peers == nil {
		return local, err
	}

	for _, peer := range peers() {
		if lent, ok := peerBackgrounds(ctx, peer, consoleID); ok {
			return lent, nil
		}
	}

	return local, nil
}

// peerBackgrounds — les emplacements que prête un serveur fédéré, en dix
// secondes au plus (peerTimeout) : seulement ceux d'un emplacement de 1 à 8
// sous son nom (« 03.jpg » au 3), jamais un chemin — la page redemande chaque
// nom au kernel, qui le relaie. Ses images en trop ne viennent pas : la bande
// ne les montre pas, et ce sont ses fichiers. Faux quand il n'en prête aucune.
func peerBackgrounds(ctx context.Context, peer Peer, consoleID string) (BackgroundSlots, bool) {
	ctx, cancel := context.WithTimeout(ctx, peerTimeout)
	defer cancel()
	var answer BackgroundSlots
	if err := peer.Client.ConsoleBackgrounds(ctx, consoleID, &answer); err != nil {
		slog.Debug("console backgrounds: a federated server lends none", "server", peer.Name, "console", consoleID, "reason", peerReason(peerError(peer, err)))
		return BackgroundSlots{}, false
	}
	names := []string{}
	for _, slot := range answer.Slots {
		if validSlot(slot.Slot) && validBackgroundName(slot.Name) && strings.TrimSuffix(slot.Name, filepath.Ext(slot.Name)) == slotBase(slot.Slot) {
			names = append(names, slot.Name)
		}
	}
	if len(names) == 0 {
		return BackgroundSlots{}, false
	}
	lent := backgroundSlotsOf(names)
	lent.Extra, lent.Server, lent.ServerName = []string{}, peer.ID, peer.Name

	return lent, true
}

// ValidBackgroundName — `name` peut être une image du fond d'une console : pas
// de chemin, pas de point en tête, une extension d'image. Le kernel le vérifie
// avant de relayer une image d'un serveur fédéré.
func ValidBackgroundName(name string) bool {
	return validBackgroundName(name)
}

// ConsoleBackground — le fichier de l'image `name` du fond, réduit à la
// largeur `width` dans le cache. os.ErrNotExist pour un nom invalide ou
// absent : un nom ne sort jamais du dossier.
func ConsoleBackground(storageRoot, consoleID, name string, width int) (string, error) {
	if !consoles.Exists(consoleID) {
		return "", ErrUnknownConsole
	}
	if !validBackgroundName(name) {
		return "", os.ErrNotExist
	}
	original := filepath.Join(ConsoleBackgroundDir(storageRoot, consoleID), name)
	if info, err := os.Lstat(original); err != nil || !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}

	return reducedBackground(original, backgroundCacheDir(storageRoot, consoleID), name, width)
}

func listBackgrounds(directory string) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && backgroundName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	return names
}

// reducedBackground — comme reducedConsoleImage, mais la réduction est
// rangée dans le cache, au nom de l'image : `01.jpg-512.jpg`.
func reducedBackground(original, cacheDir, name string, width int) (string, error) {
	if strings.EqualFold(filepath.Ext(original), ".webp") {
		return original, nil
	}
	width = widthFor(width)
	originalInfo, err := os.Stat(original)
	if err != nil {
		return "", err
	}
	for _, extension := range []string{".jpg", ".png"} {
		target := filepath.Join(cacheDir, fmt.Sprintf("%s-%d%s", name, width, extension))
		if targetInfo, err := os.Stat(target); err == nil && !targetInfo.ModTime().Before(originalInfo.ModTime()) {
			return target, nil
		}
	}

	lock := lockFor(filepath.Join(cacheDir, fmt.Sprintf("%s-%d", name, width)))
	lock.Lock()
	defer lock.Unlock()
	content, err := os.ReadFile(original)
	if err != nil {
		return "", err
	}
	picture, err := decodeImage(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", original, err)
	}
	extension := ".jpg"
	if !picture.Opaque() {
		extension = ".png"
	}
	// Une réduction d'un autre format, plus ancienne, partirait sinon devant.
	for _, stale := range []string{".jpg", ".png"} {
		_ = os.Remove(filepath.Join(cacheDir, fmt.Sprintf("%s-%d%s", name, width, stale)))
	}
	target := filepath.Join(cacheDir, fmt.Sprintf("%s-%d%s", name, width, extension))
	if err := writeReduced(target, picture, width, extension); err != nil {
		return "", err
	}

	return target, nil
}

// startBackgroundSeed — lance le remplissage par défaut s'il y a lieu, et
// rend de quoi l'attendre ; nil quand il n'y a rien à faire (déjà rempli,
// pas de liste, ou un échec du réseau il y a moins d'une heure).
func startBackgroundSeed(storageRoot string, accounts Accounts, consoleID string) chan struct{} {
	directory := ConsoleBackgroundDir(storageRoot, consoleID)
	if _, err := os.Stat(filepath.Join(directory, backgroundMarker)); err == nil {
		return nil
	}
	if len(backgroundGames(consoleID)) == 0 {
		return nil
	}

	seedsMutex.Lock()
	defer seedsMutex.Unlock()
	if done, running := seedsRunning[directory]; running {
		return done
	}
	if failedAt, failed := seedsFailed[directory]; failed && timeNow().Sub(failedAt) < libretroErrorAge {
		return nil
	}
	done := make(chan struct{})
	seedsRunning[directory] = done
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		failed := seedBackgrounds(ctx, storageRoot, accounts, consoleID)

		seedsMutex.Lock()
		delete(seedsRunning, directory)
		if failed {
			seedsFailed[directory] = timeNow()
		} else {
			delete(seedsFailed, directory)
		}
		seedsMutex.Unlock()
		close(done)
	}()

	return done
}

// backgroundGames — les jeux du remplissage par défaut, un par emplacement :
// huit au plus.
func backgroundGames(consoleID string) []string {
	if alias, found := consoleBackgroundAliases[consoleID]; found {
		consoleID = alias
	}
	games := consoleBackgroundGames[consoleID]
	if len(games) > BackgroundSlotCount {
		games = games[:BackgroundSlotCount]
	}

	return games
}

func backgroundSourceConsole(consoleID string) string {
	if alias, found := consoleBackgroundAliases[consoleID]; found {
		return alias
	}
	if source, found := consoleBackgroundSources[consoleID]; found {
		return source
	}

	return consoleID
}

// seedBackgrounds — télécharge la jaquette de chaque jeu de la liste qui
// n'a pas encore son fichier, puis pose `.seeded` quand chaque jeu a eu sa
// réponse. Rend vrai après une erreur du réseau : le marqueur n'est pas
// posé, et le remplissage reprendra (sans refaire ce qui est arrivé).
func seedBackgrounds(ctx context.Context, storageRoot string, accounts Accounts, consoleID string) bool {
	directory := ConsoleBackgroundDir(storageRoot, consoleID)
	sourceConsole := backgroundSourceConsole(consoleID)
	cacheDir := filepath.Join(storageRoot, "cache", "metadata", "console-backgrounds", consoleID)
	indexRoot := filepath.Join(storageRoot, "cache", "metadata", "sources")
	present := map[string]string{}
	for _, name := range listBackgrounds(directory) {
		present[strings.TrimSuffix(name, filepath.Ext(name))] = name
	}

	seed := backgroundSeed{Games: []backgroundSeedGame{}}
	failed, complete := false, true
	for index, game := range backgroundGames(consoleID) {
		number := slotBase(index + 1)
		record := backgroundSeedGame{Name: game}
		if existing, found := present[number]; found {
			record.File, record.Source, record.Result = existing, "kept", "kept"
			seed.Games = append(seed.Games, record)
			continue
		}

		source, original, err := fetchBackgroundCover(ctx, indexRoot, filepath.Join(cacheDir, "download"), accounts, sourceConsole, game, number)
		// Déposée par l'administrateur pendant le téléchargement : elle reste.
		if existing := backgroundWithBase(directory, number); existing != "" {
			record.File, record.Source, record.Result = existing, "kept", "kept"
			seed.Games = append(seed.Games, record)
			continue
		}
		switch {
		case err == nil:
			target := filepath.Join(directory, number+".jpg")
			if err := saveBackgroundCover(original, target); err != nil {
				slog.Warn("console background", "console", consoleID, "game", game, "error", err)
				record.Result, failed = "error", true
				break
			}
			giveToStorageOwner(storageRoot, filepath.Dir(directory), directory, target)
			record.File, record.Source, record.Result = number+".jpg", source, "found"
		case errors.Is(err, errBackgroundSkipped):
			record.Result, complete = "skipped", false
		case errors.Is(err, os.ErrNotExist):
			record.Result = "notFound"
		default:
			slog.Warn("console background", "console", consoleID, "game", game, "error", err)
			record.Result, failed = "error", true
		}
		seed.Games = append(seed.Games, record)
	}
	_ = os.RemoveAll(filepath.Join(cacheDir, "download"))
	if failed || !complete {
		return failed
	}

	seed.SeededAt = timeNow().UTC()
	content, err := json.MarshalIndent(seed, "", "  ")
	if err == nil {
		err = writeAtomically(filepath.Join(directory, backgroundMarker), content)
	}
	if err == nil {
		err = os.Chmod(filepath.Join(directory, backgroundMarker), 0o644)
		giveToStorageOwner(storageRoot, filepath.Dir(directory), directory, filepath.Join(directory, backgroundMarker))
	}
	if err != nil {
		slog.Warn("console background", "console", consoleID, "error", err)
		return true
	}

	return false
}

// errBackgroundSkipped — aucune source n'a pu être interrogée : la console
// n'a pas de dossier chez libretro, et aucun service à compte n'est saisi.
// Le remplissage se refera quand un compte le sera.
var errBackgroundSkipped = errors.New("no source for this console")

// fetchBackgroundCover — la jaquette du jeu : libretro par son nom exact,
// sinon IGDB puis SteamGridDB par son titre. Rend la source et le fichier
// téléchargé.
func fetchBackgroundCover(ctx context.Context, indexRoot, downloadDir string, accounts Accounts, consoleID, game, number string) (string, string, error) {
	systems := libretroSystems[consoleID]
	var networkErr error
	for _, system := range systems {
		index, err := libretroIndex(ctx, indexRoot, system, libretroFolders[KindCover])
		if err != nil {
			networkErr = err
			continue
		}
		if _, found := index.exact[game]; !found {
			continue
		}
		target := filepath.Join(downloadDir, number+".png")
		if err := libretroDownload(ctx, system, libretroFolders[KindCover], game, target); err != nil {
			return "", "", err
		}

		return sourceLibretro, target, nil
	}

	attempted := len(systems) > 0
	for _, service := range []remoteService{igdbService{}, steamGridDBService{}} {
		if !service.configured(accounts) {
			continue
		}
		attempted = true
		path, err := Remote(ctx, service, accounts, filepath.Join(downloadDir, number), consoleID, searchTitle(game), KindCover)
		if err == nil {
			return service.name(), path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			networkErr = err
		}
	}
	switch {
	case networkErr != nil:
		return "", "", networkErr
	case !attempted:
		return "", "", errBackgroundSkipped
	}

	return "", "", os.ErrNotExist
}

// saveBackgroundCover — la jaquette, en JPEG, dans le dossier du joueur ;
// ramenée à la plus grande largeur servie.
func saveBackgroundCover(original, target string) error {
	content, err := os.ReadFile(original)
	if err != nil {
		return err
	}
	picture, err := decodeImage(content)
	if err != nil {
		return err
	}

	if err := writeReduced(target, picture, Widths[len(Widths)-1], ".jpg"); err != nil {
		return err
	}

	// Lisible par le joueur, qui ouvre ce dossier à la main (un fichier
	// temporaire naît en 0600).
	return os.Chmod(target, 0o644)
}

// giveToStorageOwner — le dossier du fond et ce que le serveur y écrit
// appartiennent au propriétaire du dossier du serveur, pas à root : le
// joueur doit pouvoir y déposer ses images à la main (le même geste que
// adoptParentOwner du paquet api, #427). Un échec est ignoré : le kernel
// du téléphone n'est pas root, et ses fichiers sont déjà à lui.
func giveToStorageOwner(storageRoot string, paths ...string) {
	root, err := os.Stat(storageRoot)
	if err != nil {
		return
	}
	owner, ok := root.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	for _, path := range paths {
		_ = os.Lchown(path, int(owner.Uid), int(owner.Gid))
	}
}

// ── Les gestes de l'administrateur ──────────────────────────────────────

// backgroundImageExtensions — les extensions d'un même nom de base, pour
// retrouver « 03.jpg » quand « 03.png » le remplace.
var backgroundImageExtensions = []string{".jpg", ".jpeg", ".png", ".webp"}

func validBackgroundName(name string) bool {
	return backgroundName.MatchString(name) && !strings.Contains(name, "..")
}

func backgroundCacheDir(storageRoot, consoleID string) string {
	return filepath.Join(storageRoot, "cache", "metadata", "console-backgrounds", consoleID)
}

// backgroundWithBase — le fichier du dossier dont le nom sans extension est
// `base` (« 03 » → « 03.jpg »), vide s'il n'y en a pas.
func backgroundWithBase(directory, base string) string {
	for _, name := range listBackgrounds(directory) {
		if strings.TrimSuffix(name, filepath.Ext(name)) == base {
			return name
		}
	}

	return ""
}

// removeBackgroundFile — l'image et toutes ses réductions du cache.
func removeBackgroundFile(directory, cacheDir, name string) error {
	if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	reductions, _ := filepath.Glob(filepath.Join(cacheDir, name+"-*"))
	for _, reduction := range reductions {
		_ = os.Remove(reduction)
	}

	return nil
}

// SetConsoleBackground — l'image `content` dans l'emplacement `slot` (1 à 8)
// du fond de la console : il se remplit, ou son image est remplacée.
// L'extension suit le contenu : « 03.jpg » remplacé par un PNG devient
// « 03.png ». Rend le nom écrit.
func SetConsoleBackground(storageRoot, consoleID string, slot int, content []byte) (string, error) {
	if !consoles.Exists(consoleID) {
		return "", ErrUnknownConsole
	}
	if !validSlot(slot) {
		return "", ErrBackgroundSlot
	}
	extension := ImageExtension(content)
	if extension == "" {
		return "", fmt.Errorf("%w: not a PNG, JPEG or WebP", ErrUnreadableImage)
	}
	if extension != ".webp" {
		if _, err := decodeImage(content); err != nil {
			return "", fmt.Errorf("%w: %v", ErrUnreadableImage, err)
		}
	}
	directory := ConsoleBackgroundDir(storageRoot, consoleID)

	lock := lockFor(directory)
	lock.Lock()
	defer lock.Unlock()
	base := slotBase(slot)
	// Le même emplacement sous une autre extension part, réductions comprises.
	cacheDir := backgroundCacheDir(storageRoot, consoleID)
	for _, other := range backgroundImageExtensions {
		if err := removeBackgroundFile(directory, cacheDir, base+other); err != nil {
			return "", err
		}
	}
	written := base + extension
	target := filepath.Join(directory, written)
	if err := writeAtomically(target, content); err != nil {
		return "", err
	}
	if err := os.Chmod(target, 0o644); err != nil {
		return "", err
	}
	giveToStorageOwner(storageRoot, filepath.Dir(directory), directory, target)

	return written, nil
}

// ResetConsoleBackgroundSlot — l'emplacement `slot` (1 à 8) reprend son image
// par défaut : la sienne part, réductions comprises, puis `.seeded` (comme
// ResetConsoleBackgrounds) — le remplissage par défaut la refait à la
// prochaine visite, dans cet emplacement redevenu libre, sans toucher aux
// autres. C'est la « Danger » de l'image de fond d'une console Remote Play,
// qui n'a qu'un emplacement (chantier 53, P12 : « on choisit une seule
// grande image ») : une image ne se supprime pas seule, elle se remplace ou
// redevient celle par défaut.
func ResetConsoleBackgroundSlot(storageRoot, consoleID string, slot int) error {
	if !consoles.Exists(consoleID) {
		return ErrUnknownConsole
	}
	if !validSlot(slot) {
		return ErrBackgroundSlot
	}
	directory := ConsoleBackgroundDir(storageRoot, consoleID)
	cacheDir := backgroundCacheDir(storageRoot, consoleID)

	lock := lockFor(directory)
	lock.Lock()
	for _, extension := range backgroundImageExtensions {
		if err := removeBackgroundFile(directory, cacheDir, slotBase(slot)+extension); err != nil {
			lock.Unlock()
			return err
		}
	}
	lock.Unlock()

	return forgetSeed(storageRoot, consoleID)
}

// ResetConsoleBackgrounds — LA VRAIE REMISE À ZÉRO DES HUIT EMPLACEMENTS
// (chantier 53, P9 ; le porteur : « La vraie remise à zéro : le serveur vide
// les 8 emplacements, puis les remplit des images par défaut ») : l'image de
// chaque emplacement part, réductions comprises, puis `.seeded` — le
// remplissage par défaut les refait tous (ConsoleBackgrounds le relance
// aussitôt, api/console_images.go). Une image du dossier hors des huit
// emplacements n'est pas touchée : ce sont les fichiers du joueur. Avant, seul
// `.seeded` partait et les images présentes restaient : le geste ne changeait
// rien de visible.
func ResetConsoleBackgrounds(storageRoot, consoleID string) error {
	if !consoles.Exists(consoleID) {
		return ErrUnknownConsole
	}
	directory := ConsoleBackgroundDir(storageRoot, consoleID)
	cacheDir := backgroundCacheDir(storageRoot, consoleID)

	lock := lockFor(directory)
	lock.Lock()
	for slot := 1; slot <= BackgroundSlotCount; slot++ {
		for _, extension := range backgroundImageExtensions {
			if err := removeBackgroundFile(directory, cacheDir, slotBase(slot)+extension); err != nil {
				lock.Unlock()
				return err
			}
		}
	}
	lock.Unlock()

	return forgetSeed(storageRoot, consoleID)
}

// forgetSeed — retire `.seeded` et l'échec retenu : le remplissage par défaut
// se refait, dans les emplacements libres, à la prochaine lecture.
func forgetSeed(storageRoot, consoleID string) error {
	directory := ConsoleBackgroundDir(storageRoot, consoleID)
	if err := os.Remove(filepath.Join(directory, backgroundMarker)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seedsMutex.Lock()
	delete(seedsFailed, directory)
	seedsMutex.Unlock()

	return nil
}
