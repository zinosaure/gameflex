package saves

// LES NOTES DES SAUVEGARDES DE SECOURS (le porteur, 2026-11-16 : « Dans backup
// de mes sauvegardes, avoir la possibilité de rajouter une note textuelle dans
// un fichier note.json. Cela permettra de se rappeler pourquoi et quand on a
// backup la sauvegarde. ») — une note par sauvegarde des backups (`local`) : ce
// qu'on en dit, quand on l'a écrit, et sous quel compte.
//
//	sources/local/notes/states/<console>/<jeu>_slot<N>/note.json   un emplacement du rétro
//	sources/local/notes/streamex/<console>/<jeu>/note.json         un jeu d'une console récente
//
// À CÔTÉ DE LA SAUVEGARDE, PAS DEDANS : celle d'une console récente est le
// dossier même que l'émulateur lit (une restauration le remet en place tel
// quel, un envoi le transporte), et celle du rétro, deux fichiers sans dossier
// à elle. L'arbre des notes suit celui de la copie (`states/…`, `streamex/…`),
// et un emplacement s'y nomme comme son état (storage.SaveStatePath).
//
// UNE NOTE DIT LA SAUVEGARDE QU'ELLE ACCOMPAGNE : un backup qui la remplace —
// un nouveau Backup, ou celui que fait une restauration — l'emporte avec
// l'ancienne (BackupSlot, Backup), et effacer la sauvegarde efface sa note
// (Remove, et la suppression d'un emplacement par chaque kernel : ForgetNote).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gameflex/storage"
)

const (
	notesDir = "notes"
	noteFile = "note.json"
	// MaxNoteRunes — la plus longue note, en caractères : celle du formulaire
	// de la page, revalidée ici.
	MaxNoteRunes = 500
)

// Les deux sortes de sauvegardes, nommées comme leurs dossiers.
const (
	KindStates   = "states"
	KindStreamex = "streamex"
)

var (
	// ErrNoteTooLong — une note de plus de MaxNoteRunes caractères.
	ErrNoteTooLong = fmt.Errorf("a note holds at most %d characters", MaxNoteRunes)
	// ErrNotASave — une note qui ne désigne aucune sauvegarde : une sorte
	// inconnue, un segment qui sortirait du dossier des notes.
	ErrNotASave = errors.New("this names no save")
)

var notesMu sync.Mutex

// NoteItem — la sauvegarde qu'une note accompagne : un emplacement du rétro
// (Reference, Slot), ou le jeu d'une console récente (ID).
type NoteItem struct {
	Kind      string `json:"kind"`
	Console   string `json:"console"`
	Reference string `json:"reference,omitempty"`
	Slot      int    `json:"slot"`
	ID        string `json:"id,omitempty"`
}

// Note — ce que dit note.json.
type Note struct {
	Text    string    `json:"note"`
	Date    time.Time `json:"date"`
	Account string    `json:"account"`
}

// NotedItem — une note, et la sauvegarde qu'elle accompagne.
type NotedItem struct {
	NoteItem
	Note
}

// noteDir — le dossier de la note de cette sauvegarde, dans un rangement. Un
// segment se juge comme le nom d'une source (ValidSource : un seul, visible) :
// aucun ne sort du dossier des notes.
func (item NoteItem) noteDir(tree string) (string, error) {
	if !ValidSource(item.Console) {
		return "", fmt.Errorf("%w: console %q", ErrNotASave, item.Console)
	}
	switch item.Kind {
	case KindStates:
		name := fmt.Sprintf("%s_slot%d", storage.SafeSaveName(item.Reference), item.Slot)
		if item.Reference == "" || item.Slot < 0 || !ValidSource(name) {
			return "", fmt.Errorf("%w: slot %q %d", ErrNotASave, item.Reference, item.Slot)
		}

		return filepath.Join(tree, notesDir, KindStates, item.Console, name), nil
	case KindStreamex:
		if !Supported(item.Console) || !ValidSource(item.ID) {
			return "", fmt.Errorf("%w: game %q of %q", ErrNotASave, item.ID, item.Console)
		}

		return filepath.Join(tree, notesDir, KindStreamex, item.Console, item.ID), nil
	}

	return "", fmt.Errorf("%w: kind %q", ErrNotASave, item.Kind)
}

// CheckNote — une note qui tient (MaxNoteRunes) : un kernel la juge avant le
// backup qu'elle accompagnera, pour ne pas faire le geste à moitié.
func CheckNote(text string) error {
	if utf8.RuneCountInString(strings.TrimSpace(text)) > MaxNoteRunes {
		return ErrNoteTooLong
	}

	return nil
}

// backedUp — les backups ont-ils cette sauvegarde ?
func (item NoteItem) backedUp(local string) bool {
	if item.Kind == KindStreamex {
		return Has(local, item.Console, item.ID)
	}

	return HasSlot(local, item.Console, item.Reference, item.Slot)
}

// WriteNote — la note d'une sauvegarde des backups, écrite par `username` à
// `now`, telle qu'elle est gardée ; vide (des espaces seulement), elle part, et
// rien n'est rendu. os.ErrNotExist quand les backups n'ont pas cette
// sauvegarde : une note ne va que là.
func WriteNote(savesRoot, username string, item NoteItem, text string, now time.Time) (*NotedItem, error) {
	local, err := Tree(savesRoot, username, Local)
	if err != nil {
		return nil, err
	}
	dir, err := item.noteDir(local)
	if err != nil {
		return nil, err
	}
	if err := CheckNote(text); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ForgetNote(local, item)
	}
	if !item.backedUp(local) {
		return nil, os.ErrNotExist
	}

	note := Note{Text: text, Date: now.UTC().Truncate(time.Second), Account: username}
	raw, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return nil, err
	}
	notesMu.Lock()
	defer notesMu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomically(filepath.Join(dir, noteFile), raw); err != nil {
		return nil, err
	}

	return &NotedItem{NoteItem: item, Note: note}, nil
}

// ReadNote — la note d'une sauvegarde de ce rangement ; faux sans elle.
func ReadNote(tree string, item NoteItem) (Note, bool) {
	dir, err := item.noteDir(tree)
	if err != nil {
		return Note{}, false
	}

	return readNote(filepath.Join(dir, noteFile))
}

func readNote(path string) (Note, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Note{}, false
	}
	var note Note
	if json.Unmarshal(raw, &note) != nil || note.Text == "" {
		return Note{}, false
	}

	return note, true
}

// ForgetNote — la note d'une sauvegarde, retirée de ce rangement ; rien sans
// elle. Une sauvegarde qu'aucune note ne peut désigner n'en a pas : rien à
// retirer non plus.
func ForgetNote(tree string, item NoteItem) error {
	dir, err := item.noteDir(tree)
	if err != nil {
		return nil
	}
	notesMu.Lock()
	defer notesMu.Unlock()
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// slotNoteName — `<jeu>_slot<N>`, le dossier de la note d'un emplacement.
var slotNoteName = regexp.MustCompile(`^(.+)_slot(\d+)$`)

// Notes — les notes des backups d'un compte, triées par sorte, console et
// sauvegarde. Une sauvegarde s'y désigne comme ses listes la nomment : la
// référence du rétro telle que le nom de son état la porte.
func Notes(savesRoot, username string) ([]NotedItem, error) {
	local, err := Tree(savesRoot, username, Local)
	if err != nil {
		return nil, err
	}
	notes := []NotedItem{}
	for _, kind := range []string{KindStates, KindStreamex} {
		consoles, _ := os.ReadDir(filepath.Join(local, notesDir, kind))
		for _, console := range consoles {
			entries, _ := os.ReadDir(filepath.Join(local, notesDir, kind, console.Name()))
			for _, entry := range entries {
				item := NoteItem{Kind: kind, Console: console.Name(), ID: entry.Name()}
				if kind == KindStates {
					match := slotNoteName.FindStringSubmatch(entry.Name())
					if match == nil {
						continue
					}
					slot, _ := strconv.Atoi(match[2])
					item = NoteItem{Kind: kind, Console: console.Name(), Reference: match[1], Slot: slot}
				}
				if note, found := readNote(filepath.Join(local, notesDir, kind, console.Name(), entry.Name(), noteFile)); found {
					notes = append(notes, NotedItem{NoteItem: item, Note: note})
				}
			}
		}
	}
	sort.SliceStable(notes, func(i, j int) bool {
		left, right := notes[i].NoteItem, notes[j].NoteItem
		if left.Kind != right.Kind || left.Console != right.Console {
			return left.Kind+"/"+left.Console < right.Kind+"/"+right.Console
		}
		if left.Reference != right.Reference || left.ID != right.ID {
			return left.Reference+left.ID < right.Reference+right.ID
		}

		return left.Slot < right.Slot
	})

	return notes, nil
}
