package engine

import (
	"encoding/binary"
	"testing"
)

// ppssppHeader — le début d'une sauvegarde de PPSSPP : la section SaveStart,
// sa version, le compteur de sauvegardes et la version d'origine, comme dans
// la sauvegarde #2 de Crisis Core refusée sur le S25 (2026-10-17).
func ppssppHeader(sectionVersion uint32, origin string) []byte {
	data := make([]byte, 0, 64)
	marker := make([]byte, 16)
	copy(marker, "SaveStart")
	data = append(data, marker...)
	data = binary.LittleEndian.AppendUint32(data, sectionVersion)
	data = binary.LittleEndian.AppendUint32(data, 2)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(origin)+1))
	data = append(data, origin...)
	data = append(data, 0)

	return append(data, "Memory\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"...)
}

// Le refus dit qui refuse et, quand la sauvegarde le dit, qui l'avait écrite.
func TestARefusedStateSaysWhoWroteIt(t *testing.T) {
	installed := coreIdentity{name: "PPSSPP", version: "f6aea4c"}

	refused := stateRefused(installed, ppssppHeader(3, "93035c6"))
	if refused.Core != "PPSSPP f6aea4c" || refused.StateCore != "PPSSPP 93035c6" {
		t.Fatalf("refus = %+v", refused)
	}
	if got, want := refused.Error(), "le cœur PPSSPP f6aea4c ne sait pas relire cette sauvegarde, faite par PPSSPP 93035c6"; got != want {
		t.Fatalf("message %q, attendu %q", got, want)
	}

	// Une sauvegarde qui ne dit rien de son origine : seul le cœur installé.
	for name, data := range map[string][]byte{
		"un autre cœur":           []byte("NES\x1a garbage that is not a PPSSPP state at all"),
		"trop courte":             []byte("SaveStart"),
		"SaveStart d'avant la v2": ppssppHeader(1, "x"),
		"longueur absurde": func() []byte {
			data := ppssppHeader(3, "93035c6")
			binary.LittleEndian.PutUint32(data[24:28], 4096)

			return data
		}(),
	} {
		got := stateRefused(coreIdentity{name: "FCEUmm", version: "(SVN)"}, data)
		if got.StateCore != "" || got.Core != "FCEUmm (SVN)" {
			t.Fatalf("%s : refus = %+v", name, got)
		}
		if got.Error() != "le cœur FCEUmm (SVN) a refusé cette sauvegarde" {
			t.Fatalf("%s : message %q", name, got.Error())
		}
	}
}
