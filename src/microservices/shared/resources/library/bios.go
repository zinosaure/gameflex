// Repris de kernel/src/internal/retro_files/bios.go (chantier 61 ; le porteur, 2026-11-18 : « Ce sont du code que tu peux
// copier depuis Gameflex actuel ») : les cœurs, les émulateurs et les BIOS d'un disque, listés et résolus comme chez GameFlex.

package library

// BIOS selection — some cores (PS1's pcsx_rearmed in particular) fall back
// to an imperfect HLE (High-Level Emulation) BIOS replacement whenever they
// can't find/pick a real BIOS dump, and HLE is well known to be
// incompatible with a meaningful number of real games. The flat BIOS_PATH
// root (see main.go/child.go's biosRoot) passed to libretro.InitEnvPaths as
// systemDir is an ES-DE/Batocera-style tree of PER-CONSOLE subfolders (e.g.
// {BIOS_PATH}/psx/scph5501.bin), not a flat pile of files at the root
// itself — before this file existed, EVERY console's systemDir was that
// flat root regardless, so a core scanning it directly (as pcsx_rearmed
// does) found nothing at all and silently fell back to HLE. Confirmed via
// direct inspection of a real deployment's BIOS_PATH (2026-08-01) — 24 real
// PS1 BIOS files sitting one level deeper than any core would ever look.
import (
	"gameflex/consoles"
)

// consoles.BiosDir maps our own consoleId to the actual on-disk BIOS
// subfolder name under BIOS_PATH — deliberately a small, explicit table
// rather than assuming consoleId always matches the folder name (it
// usually doesn't: "ps1" here is "psx" on disk). Only consoles genuinely
// gated by picking a specific BIOS file are listed — everything else keeps
// passing BIOS_PATH's flat root straight through to InitEnvPaths,
// completely unchanged from before this file existed (zero regression risk
// for any console not listed here). PPSSPP in particular deliberately stays
// OUT of this table: it expects its own asset folder ({BIOS_PATH}/PPSSPP/,
// see the project README's BIOS note) directly at that flat root, and
// scoping its systemDir down to a subfolder would break that.
// La table elle-même vit dans shared/consoles depuis le 2026-09-01 : le
// téléphone en a besoin AUSSI (il range désormais ses BIOS par console, et
// un BIOS récupéré par fédération doit atterrir là où le core le cherche).
// Deux tables auraient fini par ne plus dire la même chose.
func biosDirFor(consoleID string) (string, bool) {
	dir := consoles.BiosDir(consoleID)

	return dir, dir != ""
}
