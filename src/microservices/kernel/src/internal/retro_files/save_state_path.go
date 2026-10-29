// Save-state slot constants and the on-disk path formula — extracted from
// the old retrogaming package's stream_session.go (the child still owns the
// save/load runtime; the API only needs the formula to list/serve/delete).
package retro_files

import (
	"fmt"
	"strings"

	"gameflex/consoles"
	"gameflex/storage"
)

// LE COMPTE VIENT DU PAQUET PARTAGÉ, pas d'ici : le kernel du téléphone doit
// lire le même, et il n'a pas accès à ce paquet interne. Voir
// shared/storage/save_slots.go, qui porte aussi la note sur la copie Java.
const (
	maxSaveSlots = storage.MaxSaveSlots
	autoSaveSlot = storage.AutoSaveSlot
)

// MaxSaveSlots/AutoSaveSlot — exported accessors for the API handlers.
func MaxSaveSlots() int { return maxSaveSlots }
func AutoSaveSlot() int { return autoSaveSlot }

// StateFilePath — exported accessor for the API handlers.
func StateFilePath(savesRoot, username, consoleID, reference string, slot int, ext string) (string, error) {
	return stateFilePath(savesRoot, username, consoleID, reference, slot, ext)
}

// RemoveSaveFiles — exported accessor for the API handlers.
func RemoveSaveFiles(savesRoot, username, consoleID, reference string, slot int) error {
	return removeSaveFiles(savesRoot, username, consoleID, reference, slot)
}

// stateFilePath — reference can contain characters unsafe for a single
// filename (ROM folder names) — replaced rather than rejected, since this
// is derived from data the scanner already trusted enough to serve.
//
// username and consoleID now go through safeJoin like every other
// request-derived segment in this package (AGENTS.md: never a bare
// filepath.Join on a path built from a request). They used to be joined raw,
// which made `username=../saves/root` climb out of savesRoot and back in —
// confirmed live, and reachable without a token at the time. The username now
// comes from the JWT rather than the query string, so this is defence in
// depth; it belongs here because a formula this small gets reused, and the
// next caller may not be as careful about where its username came from.
//
// SOUS LE DOSSIER DU COMPTE, PAS SEULEMENT SOUS saves/ (revue de sécurité
// avant l'ouverture du kernel, 2026-10-28). safeJoin à partir de savesRoot
// laissait `consoleId=../../bob/states/gba` — la console vient de la query —
// redescendre chez un autre compte : un joueur lisait, écrasait et effaçait
// les sauvegardes de Bob. Le compte passe par storage.UserSavesDir, la console
// doit être celle du registre, et le reste se joint sous le dossier du compte.
func stateFilePath(savesRoot, username, consoleID, reference string, slot int, ext string) (string, error) {
	userDir, err := storage.UserSavesDir(savesRoot, username)
	if err != nil {
		return "", err
	}
	if !consoles.Exists(consoleID) {
		return "", fmt.Errorf("unknown console %q", consoleID)
	}
	safeName := strings.NewReplacer("/", "_", "\\", "_").Replace(reference)
	return safeJoin(
		userDir,
		"states",
		consoleID,
		fmt.Sprintf("%s_slot%d%s", safeName, slot, ext),
	)
}
