package storage

// LE CORPUS D'ESSAI — les formes qui cassent.
//
// Le banc d'essai ne demande pas beaucoup de jeux, il demande les bonnes
// FORMES : une dizaine de fichiers bien choisis attrapent plus que mille
// cartouches identiques. Chaque entrée ci-dessous correspond à un bug réel :
//
//   - roms/nes/USA/                      un dossier sous la console : UN jeu
//                                        (voir scanner.go — décision 2026-09-02),
//                                        qui doit partir AVEC ses deux cartouches ;
//   - roms/ps1/[PS1] Ace Combat 2/       le jeu-dossier : un .cue de 75 o et
//                                        un .bin de 554 Mio — la panne la plus
//                                        chère (on ne copiait que le .cue) ;
//   - roms/ps1/[PS1] Final Fantasy IX/   un multi-disques : le .m3u doit
//                                        gagner sur les .cue ;
//   - roms/psp/[PSP] Tekken 6/           nom du dossier ≠ nom du fichier ;
//   - roms/ps2/[PS2] Metal Slug 4/       un jeu-dossier AVEC un sous-dossier ;
//   - roms/nds/Pokemon Platinum.zip      une ROM qui EST un zip : il ne faut
//                                        jamais l'ouvrir (les ROMs NDS le sont) ;
//   - roms/doom/                         VIDE : une console sans jeu, qui ne
//                                        doit pas hériter des 16 .WAD rangés
//                                        dans bios/doom/ ;
//   - roms/snes/                         accents et caractères d'URL, fichiers
//                                        cachés, dossiers de service ;
//   - roms/ps3/[PS3] Gros Jeu/           2,5 Go — la troncature du navigateur.
//
// Les fichiers sont REMPLIS DE ZÉROS : le scanner juge sur les noms, pas sur
// le contenu. Les gros sont des fichiers clairsemés (Truncate sans écrire) —
// le corpus entier tient dans quelques mégaoctets de disque.

import (
	"fmt"
	"os"
	"path/filepath"
)

// corpusFile — une entrée du corpus : un chemin relatif à la racine du
// partage, et la taille à lui donner. Une taille nulle pour un dossier.
type corpusFile struct {
	path string
	size int64
}

// corpusFiles — LE corpus, dans l'ordre de création.
var corpusFiles = []corpusFile{
	{"roms/nes/Mario.nes", 40960},
	{"roms/nes/Zelda.nes", 40960},
	{"roms/nes/USA/Metroid.nes", 40960},
	{"roms/nes/USA/Contra.nes", 40960},

	{"roms/ps1/[PS1] Ace Combat 2 [F] [SCES-00901]/Ace Combat 2.cue", 75},
	{"roms/ps1/[PS1] Ace Combat 2 [F] [SCES-00901]/Ace Combat 2.bin", 581734272},
	{"roms/ps1/[PS1] Final Fantasy IX [F]/FF9.m3u", 64},
	{"roms/ps1/[PS1] Final Fantasy IX [F]/FF9 (Disc 1).cue", 75},
	{"roms/ps1/[PS1] Final Fantasy IX [F]/FF9 (Disc 1).bin", 419430400},
	{"roms/ps1/[PS1] Final Fantasy IX [F]/FF9 (Disc 2).cue", 75},
	{"roms/ps1/[PS1] Final Fantasy IX [F]/FF9 (Disc 2).bin", 314572800},

	{"roms/psp/[PSP] Tekken 6 [E-F-G-I-S-R] [ULES-01512]/Tekken 6.iso", 838860800},

	{"roms/ps2/[PS2] Metal Slug 4/Metal Slug 4.cue", 75},
	{"roms/ps2/[PS2] Metal Slug 4/Metal Slug 4.bin", 209715200},
	{"roms/ps2/[PS2] Metal Slug 4/media/cover.png", 32000},

	{"roms/nds/Pokemon Platinum (Europe).zip", 67108864},
	{"roms/gba/Fire Emblem.gba", 16777216},
	{"roms/doom", 0},

	{"roms/snes/Chrono Trigger (é, à, ç).sfc", 40960},
	{"roms/snes/Super Mario World #1 & 2 [!].smc", 40960},
	{"roms/snes/.DS_Store", 6148},
	{"roms/snes/System Volume Information", 0},

	{"roms/ps3/[PS3] Gros Jeu/gros.iso", 2684354560},

	{"bios/psx/scph5501.bin", 524288},
	{"bios/scph1001.bin", 524288},
	{"bios/doom/CHEX.WAD", 4194304},
	{"bios/doom/DOOM.WAD", 4194304},
	{"bios/doom/DOOM1.WAD", 4194304},
	{"bios/doom/DOOM2.WAD", 4194304},
	{"bios/doom/DOOMU.WAD", 4194304},
	{"bios/doom/PLUTONIA.WAD", 4194304},
	{"bios/doom/TNT.WAD", 4194304},
	{"bios/doom/HEXEN.WAD", 4194304},
	{"bios/doom/HEXDD.WAD", 4194304},
	{"bios/doom/HERETIC.WAD", 4194304},
	{"bios/doom/STRIFE0.WAD", 4194304},
	{"bios/doom/STRIFE1.WAD", 4194304},
	{"bios/doom/HACX.WAD", 4194304},
	{"bios/doom/FREEDOOM1.WAD", 4194304},
	{"bios/doom/FREEDOOM2.WAD", 4194304},
	{"bios/doom/REKKRSA.WAD", 4194304},

	{"cores/fceumm_libretro.so", 2097152},
}

// WriteCorpus pose le corpus sous root — la racine du PARTAGE (celle qui
// contient roms/, bios/ et cores/), créée si besoin. Idempotent : relancer
// écrase les fichiers existants, sans toucher au reste.
func WriteCorpus(root string) error {
	for _, wanted := range corpusFiles {
		target := filepath.Join(root, filepath.FromSlash(wanted.path))
		if wanted.size == 0 {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("storage: dossier %q : %w", wanted.path, err)
			}

			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("storage: parent de %q : %w", wanted.path, err)
		}

		file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("storage: %q : %w", wanted.path, err)
		}

		// Truncate crée un TROU de lecture : le fichier annonce sa taille
		// sans occuper de disque. C'est ce qui permet un corpus avec un
		// .bin de 554 Mio et un .iso de 2,5 Go.
		if err := file.Truncate(wanted.size); err != nil {
			_ = file.Close()

			return fmt.Errorf("storage: taille de %q : %w", wanted.path, err)
		}

		if err := file.Close(); err != nil {
			return fmt.Errorf("storage: fermeture de %q : %w", wanted.path, err)
		}
	}

	return nil
}
