package emuparams

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// binaryPatch — la langue d'une console dans un fichier binaire (chantier 21) :
// le fichier, relatif au XDG_DATA_HOME de l'émulateur, la fonction qui y écrit
// une valeur, et la valeur de chaque langue de l'application. Seule la langue
// change, jamais la région (le porteur, 2026-10-22).
type binaryPatch struct {
	file   string
	apply  func(content []byte, value uint32) ([]byte, error)
	values map[string]uint32
}

// patchLanguage — la langue de l'application posée dans le fichier binaire de
// cet émulateur. Le fichier absent — un premier lancement, que l'émulateur
// crée avec ses défauts —, la langue suivra au lancement suivant : le dire.
func (w writer) patchLanguage(report *Report, configHome, language string) {
	patch := w.languagePatch
	if patch == nil {
		return
	}
	value, known := patch.values[language]
	if !known {
		return
	}
	target := filepath.Join(filepath.Dir(configHome), ".local", "share", patch.file)
	content, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		report.Skipped = append(report.Skipped, patch.file+" (absent : la langue suivra au prochain lancement)")

		return
	}
	if err != nil {
		report.Error = err.Error()

		return
	}
	patched, err := patch.apply(content, value)
	if err != nil {
		report.Error = patch.file + " : " + err.Error()

		return
	}
	if string(patched) != string(content) {
		if err := os.WriteFile(target, patched, 0o644); err != nil {
			report.Error = err.Error()

			return
		}
	}
	report.Set = append(report.Set, patch.file+" > langue = "+strconv.FormatUint(uint64(value), 10)+" (langue de l'application)")
}

// xboxEEPROMLanguage — la langue de la Xbox dans son EEPROM (256 octets) : un
// mot à 0x90, petit-boutiste (1 anglais, 4 français…), dans la section des
// réglages de l'utilisateur, non chiffrée, que protège la somme à 0x60 sur
// les mots de 0x64 à 0xBF. La somme est celle de xemu
// (hw/xbox/eeprom_generation.c) : vérifiée sur les EEPROM de root et de gino,
// elle redonne celle qu'ils portent. Sans elle, le noyau de la Xbox refuse la
// section.
func xboxEEPROMLanguage(content []byte, value uint32) ([]byte, error) {
	if len(content) < 0xC0 {
		return nil, fmt.Errorf("EEPROM de %d octets, 256 attendus", len(content))
	}
	patched := append([]byte(nil), content...)
	binary.LittleEndian.PutUint32(patched[0x90:], value)
	binary.LittleEndian.PutUint32(patched[0x60:], xboxSectionChecksum(patched[0x64:0xC0]))

	return patched, nil
}

// xboxSectionChecksum — la somme d'une section de l'EEPROM : les mots de 32
// bits additionnés avec report, puis complémentés.
func xboxSectionChecksum(section []byte) uint32 {
	var high, low uint64
	for offset := 0; offset+4 <= len(section); offset += 4 {
		word := uint64(binary.LittleEndian.Uint32(section[offset:]))
		sum := (high << 32) | low
		sum += word
		high = sum >> 32
		low = (low + word) & 0xFFFFFFFF
	}

	return ^uint32(high + low)
}

// xeniaLanguage — la langue de la Xbox 360 dans le xconfig.settings de Xenia
// (6680 octets, la structure de ses réglages telle quelle, gros-boutiste) : le
// mot à 0x912 (1 anglais, 4 français…). Xenia ne vérifie aucune somme. Le
// décalage, calculé depuis la structure (src/xenia/kernel/xconfig.h), est
// vérifié sur un vrai fichier : il y porte 1, et le pays (103, les États-Unis)
// est bien à 0x926 — que l'on ne touche pas.
func xeniaLanguage(content []byte, value uint32) ([]byte, error) {
	if len(content) < 0x916 {
		return nil, fmt.Errorf("xconfig.settings de %d octets, 6680 attendus", len(content))
	}
	patched := append([]byte(nil), content...)
	binary.BigEndian.PutUint32(patched[0x912:], value)

	return patched, nil
}
