package emuparams

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// endAroundCarry — la somme que le noyau de la Xbox vérifie : les mots de la
// section, somme comprise, additionnés avec report circulaire ; 0xFFFFFFFF
// pour une section valide.
func endAroundCarry(section []byte) uint32 {
	var sum uint64
	for offset := 0; offset+4 <= len(section); offset += 4 {
		sum += uint64(binary.LittleEndian.Uint32(section[offset:]))
		sum = (sum & 0xFFFFFFFF) + (sum >> 32)
	}

	return uint32(sum)
}

// L'EEPROM DE LA XBOX EN FRANÇAIS (chantier 21) : la langue à 0x90, et une
// somme que le noyau accepte — sur une EEPROM synthétique, une vraie portant
// l'identité de la console émulée.
func TestXboxEEPROMLanguageKeepsTheSectionValid(t *testing.T) {
	eeprom := make([]byte, 256)
	for offset := 0x64; offset < 0xC0; offset += 4 {
		binary.LittleEndian.PutUint32(eeprom[offset:], uint32(offset)*0x9E3779B1)
	}
	binary.LittleEndian.PutUint32(eeprom[0x90:], 1)
	binary.LittleEndian.PutUint32(eeprom[0x60:], xboxSectionChecksum(eeprom[0x64:0xC0]))
	if endAroundCarry(eeprom[0x60:0xC0]) != 0xFFFFFFFF {
		t.Fatal("la somme de départ n'est déjà pas valide : le test lui-même est faux")
	}

	patched, err := xboxEEPROMLanguage(eeprom, 4)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(patched[0x90:]) != 4 || endAroundCarry(patched[0x60:0xC0]) != 0xFFFFFFFF {
		t.Fatalf("langue %d, section %#x", binary.LittleEndian.Uint32(patched[0x90:]), endAroundCarry(patched[0x60:0xC0]))
	}
	for offset := range eeprom {
		if (offset < 0x60 || offset >= 0xC0) && patched[offset] != eeprom[offset] {
			t.Fatalf("octet %#x touché hors de la section des réglages", offset)
		}
	}
	if _, err := xboxEEPROMLanguage(make([]byte, 16), 4); err == nil {
		t.Fatal("une EEPROM tronquée est acceptée")
	}
}

// Apply pose la langue dans l'EEPROM de xemu et dans le xconfig.settings de
// Xenia, sans toucher au pays ; un fichier absent est dit, pas créé.
func TestApplyPatchesTheBinaryLanguages(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(filepath.Dir(home), ".local", "share")
	if err := os.MkdirAll(filepath.Join(data, "Xenia"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := make([]byte, 6680)
	binary.BigEndian.PutUint32(settings[0x912:], 1)
	settings[0x926] = 103
	if err := os.WriteFile(filepath.Join(data, "Xenia", "xconfig.settings"), settings, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Apply("xenia", Settings{State: "absent", Entries: []Entry{}}, home, 720, "fr")
	written, _ := os.ReadFile(filepath.Join(data, "Xenia", "xconfig.settings"))
	if binary.BigEndian.Uint32(written[0x912:]) != 4 || written[0x926] != 103 || len(written) != 6680 {
		t.Fatalf("xconfig.settings : langue %d, pays %d (%v)", binary.BigEndian.Uint32(written[0x912:]), written[0x926], report)
	}

	// L'EEPROM de xemu absente (un premier lancement) : dit, pas créée.
	report = Apply("xemu", Settings{State: "absent", Entries: []Entry{}}, home, 540, "fr")
	if _, err := os.Stat(filepath.Join(data, "xemu", "xemu", "eeprom.bin")); !os.IsNotExist(err) || len(report.Skipped) == 0 {
		t.Fatalf("EEPROM absente : %v, %v", err, report.Skipped)
	}
}
