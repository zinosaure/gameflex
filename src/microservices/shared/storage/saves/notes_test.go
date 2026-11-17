package saves

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var wednesday = time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)

// UNE SAUVEGARDE DE SECOURS PORTE SA NOTE (le porteur, 2026-11-16) : un
// note.json à côté d'elle, dans les backups — ce qu'on en dit, quand, et sous
// quel compte —, qui se relit, se corrige, et part quand on la vide.
func TestABackupKeepsItsNote(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "gino")
	write(t, filepath.Join(principal, "states", "nes", "ici_slot1.state"), "ici", monday)
	if err := BackupSlot(root, "gino", "nes", "ici", 1); err != nil {
		t.Fatal(err)
	}
	slot := NoteItem{Kind: KindStates, Console: "nes", Reference: "ici", Slot: 1}

	if _, err := WriteNote(root, "gino", slot, "  avant le boss  ", tuesday); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(principal, "sources", "local", "notes", "states", "nes", "ici_slot1", "note.json")
	var stored map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &stored); err != nil {
		t.Fatal(err)
	}
	if stored["note"] != "avant le boss" || stored["account"] != "gino" || stored["date"] != tuesday.Format(time.RFC3339) {
		t.Fatalf("note.json : %v", stored)
	}

	local := filepath.Join(principal, "sources", "local")
	if note, found := ReadNote(local, slot); !found || note.Text != "avant le boss" || !note.Date.Equal(tuesday) {
		t.Fatalf("relue : %+v %v", note, found)
	}
	notes, err := Notes(root, "gino")
	if err != nil || len(notes) != 1 || notes[0].Kind != KindStates || notes[0].Console != "nes" || notes[0].Reference != "ici" ||
		notes[0].Slot != 1 || notes[0].Text != "avant le boss" || notes[0].Account != "gino" {
		t.Fatalf("listée : %+v %v", notes, err)
	}

	if kept, err := WriteNote(root, "gino", slot, "après le boss", wednesday); err != nil || kept == nil || kept.Text != "après le boss" || kept.Slot != 1 {
		t.Fatalf("corrigée, rendue : %+v %v", kept, err)
	}
	if note, _ := ReadNote(local, slot); note.Text != "après le boss" || !note.Date.Equal(wednesday) {
		t.Fatalf("corrigée : %+v", note)
	}

	if _, err := WriteNote(root, "gino", slot, strings.Repeat("é", MaxNoteRunes+1), wednesday); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("trop longue : %v", err)
	}
	if _, err := WriteNote(root, "gino", NoteItem{Kind: KindStates, Console: "nes", Reference: "ici", Slot: 2}, "rien", wednesday); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sans backup : %v", err)
	}
	write(t, filepath.Join(principal, "states", "nes", "ici_slot3.state"), "pas de backup", monday)
	if _, err := WriteNote(root, "gino", NoteItem{Kind: KindStates, Console: "nes", Reference: "ici", Slot: 3}, "rien", wednesday); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("une note ne va que dans les backups : %v", err)
	}

	if kept, err := WriteNote(root, "gino", slot, "   ", wednesday); err != nil || kept != nil {
		t.Fatalf("vidée : %+v %v", kept, err)
	}
	missing(t, path)
	if notes, _ := Notes(root, "gino"); len(notes) != 0 {
		t.Fatalf("vidée, elle reste : %+v", notes)
	}
}

// UNE NOTE DIT LA SAUVEGARDE QU'ELLE ACCOMPAGNE : un backup qui la remplace —
// le geste, ou celui que fait une restauration — l'emporte, et effacer la
// sauvegarde efface sa note.
func TestANoteGoesWithTheBackupItDescribes(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "gino")
	local := filepath.Join(principal, "sources", "local")
	write(t, filepath.Join(principal, "states", "nes", "ici_slot1.state"), "ici", monday)
	write(t, filepath.Join(principal, "sources", "prod", "states", "nes", "ici_slot1.state"), "prod", tuesday)
	slot := NoteItem{Kind: KindStates, Console: "nes", Reference: "ici", Slot: 1}
	noted := func() bool {
		_, found := ReadNote(local, slot)

		return found
	}

	if err := BackupSlot(root, "gino", "nes", "ici", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNote(root, "gino", slot, "première", monday); err != nil || !noted() {
		t.Fatalf("écrite : %v", err)
	}
	if err := BackupSlot(root, "gino", "nes", "ici", 1); err != nil || noted() {
		t.Fatalf("un nouveau backup garde l'ancienne note (%v)", err)
	}

	if _, err := WriteNote(root, "gino", slot, "seconde", monday); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSlot(root, "gino", "prod", "nes", "ici", "ici", 1); err != nil || noted() {
		t.Fatalf("le backup d'une restauration garde l'ancienne note (%v)", err)
	}

	if _, err := WriteNote(root, "gino", slot, "troisième", monday); err != nil {
		t.Fatal(err)
	}
	if err := ForgetNote(local, slot); err != nil || noted() {
		t.Fatalf("oubliée : %v", err)
	}

	fakeConsoles(t, principal)
	game := NoteItem{Kind: KindStreamex, Console: "ps4", ID: "CUSA07010"}
	if err := Backup(root, "gino", "ps4", "CUSA07010"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNote(root, "gino", game, "avant la mise à jour", monday); err != nil {
		t.Fatal(err)
	}
	if note, found := ReadNote(local, game); !found || note.Text != "avant la mise à jour" {
		t.Fatalf("la note d'un jeu : %+v", note)
	}
	missing(t, filepath.Join(local, "streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "note.json"))
	if notes, _ := Notes(root, "gino"); len(notes) != 1 || notes[0].Kind != KindStreamex || notes[0].ID != "CUSA07010" {
		t.Fatalf("listée : %+v", notes)
	}
	if err := Remove(local, "ps4", "CUSA07010"); err != nil {
		t.Fatal(err)
	}
	if _, found := ReadNote(local, game); found {
		t.Fatal("la sauvegarde effacée, sa note reste")
	}
}

// UNE NOTE NE DÉSIGNE QU'UNE SAUVEGARDE : ni une sorte inconnue, ni un chemin
// qui sortirait du dossier des notes.
func TestANoteNamesOnlyABackup(t *testing.T) {
	root := t.TempDir()
	for _, item := range []NoteItem{
		{Kind: "sram", Console: "nes", Reference: "ici", Slot: 1},
		{Kind: KindStates, Console: "../nes", Reference: "ici", Slot: 1},
		{Kind: KindStates, Console: "nes", Reference: "", Slot: 1},
		{Kind: KindStates, Console: "nes", Reference: "ici", Slot: -1},
		{Kind: KindStreamex, Console: "ps4", ID: ".."},
		{Kind: KindStreamex, Console: "atari", ID: "CUSA07010"},
	} {
		if _, err := WriteNote(root, "gino", item, "x", monday); !errors.Is(err, ErrNotASave) {
			t.Fatalf("%+v : %v", item, err)
		}
	}
	tree := filepath.Join(root, "gino")
	dir, err := NoteItem{Kind: KindStates, Console: "nes", Reference: "a/b", Slot: 0}.noteDir(tree)
	if err != nil || dir != filepath.Join(tree, "notes", "states", "nes", "a_b_slot0") {
		t.Fatalf("une référence se range comme son état : %q %v", dir, err)
	}
}
