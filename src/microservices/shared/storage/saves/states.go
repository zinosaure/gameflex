package saves

import (
	"errors"
	"os"
	"path/filepath"

	"gameflex/storage"
)

// LE RÉTRO : L'UNITÉ EST L'EMPLACEMENT — l'état et sa vignette
// (`states/<console>/<jeu>_slot<N>.state` et `.png`, storage.SaveStatePath).

// statePaths — l'état et la vignette d'un emplacement, dans un rangement.
func statePaths(tree, consoleID, reference string, slot int) (state, shot string, err error) {
	parent, owner := filepath.Dir(tree), filepath.Base(tree)
	if state, err = storage.SaveStatePath(parent, owner, consoleID, reference, slot, ".state"); err != nil {
		return "", "", err
	}
	if shot, err = storage.SaveStatePath(parent, owner, consoleID, reference, slot, ".png"); err != nil {
		return "", "", err
	}

	return state, shot, nil
}

// HasSlot — cet emplacement a-t-il un état, dans ce rangement ?
func HasSlot(tree, consoleID, reference string, slot int) bool {
	state, _, err := statePaths(tree, consoleID, reference, slot)
	if err != nil {
		return false
	}
	_, err = os.Stat(state)

	return err == nil
}

// CopySlot — un emplacement d'un rangement à l'autre, sous la référence
// `toReference` (celle du même jeu ici, quand une copie venue d'ailleurs le
// range sous la sienne). Remplace ce que la destination avait à cet
// emplacement ; une vignette absente de la source n'en laisse pas d'ancienne.
func CopySlot(fromTree, toTree, consoleID, reference, toReference string, slot int) error {
	fromState, fromShot, err := statePaths(fromTree, consoleID, reference, slot)
	if err != nil {
		return err
	}
	toState, toShot, err := statePaths(toTree, consoleID, toReference, slot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(fromState); err != nil {
		return err
	}

	if err := copyFile(fromState, toState); err != nil {
		return err
	}
	if _, err := os.Stat(fromShot); err == nil {
		return copyFile(fromShot, toShot)
	}
	if err := os.Remove(toShot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// BackupSlot — un emplacement du principal, copié dans `local`.
func BackupSlot(savesRoot, username, consoleID, reference string, slot int) error {
	principal, err := Tree(savesRoot, username, "")
	if err != nil {
		return err
	}
	local, err := Tree(savesRoot, username, Local)
	if err != nil {
		return err
	}

	return CopySlot(principal, local, consoleID, reference, reference, slot)
}

// RestoreSlot — un emplacement d'une copie remis dans le principal, sous
// `toReference`. Ce que le principal avait à cet emplacement part d'abord
// dans `local`.
//
// LA COPIE EST LUE AVANT LE BACKUP : restaurer depuis `local` lui-même
// échange les deux versions au lieu de perdre celle qu'on restaure.
func RestoreSlot(savesRoot, username, source, consoleID, reference, toReference string, slot int) error {
	if source == "" {
		return errors.New("a restore comes from a copy, not from the principal")
	}
	from, err := Tree(savesRoot, username, source)
	if err != nil {
		return err
	}
	principal, err := Tree(savesRoot, username, "")
	if err != nil {
		return err
	}
	if !HasSlot(from, consoleID, reference, slot) {
		return os.ErrNotExist
	}

	staging, err := os.MkdirTemp(principal, ".restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := CopySlot(from, staging, consoleID, reference, toReference, slot); err != nil {
		return err
	}
	if HasSlot(principal, consoleID, toReference, slot) {
		if err := BackupSlot(savesRoot, username, consoleID, toReference, slot); err != nil {
			return err
		}
	}

	return CopySlot(staging, principal, consoleID, toReference, toReference, slot)
}
