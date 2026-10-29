package retrogaming

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// cueFileLine — une ligne `FILE "nom" TYPE` d'une fiche .cue, le nom entre
// guillemets ou non.
var cueFileLine = regexp.MustCompile(`(?i)^\s*FILE\s+(?:"([^"]+)"|(\S+))\s+\S+\s*$`)

// cueWithExactNames — une fiche .cue dont chaque fichier se trouve SOUS LE NOM
// QU'ELLE DONNE.
//
// LA CASSE D'UN DUMP WINDOWS. Une fiche écrite sous Windows nomme souvent ses
// pistes autrement que le disque : `FINAL_FANTASY_5_(PAL)_[SCES-13840].ISO`
// pour un fichier `Final_Fantasy_5_….iso`. Windows ne voit pas la
// différence ; un serveur Linux, si. Le cœur ne trouve pas sa piste et rend
// « retro_load_game returned false » — mesuré le 2026-10-02 sur dix fiches de
// la bibliothèque PS1 (FF I, II, IV, V, VII ×3, Syphon Filter 2 ×2, Ace
// Combat 2). pcsx_rearmed en sauve une partie par un repli de son cru (un
// .bin du nom de la fiche) ; FF I, II, IV et V, dont la piste est un .iso,
// restaient injouables.
//
// On ne touche JAMAIS à la bibliothèque : si un nom ne correspond qu'à une
// casse près, on refait la fiche dans `scratchDir`, à côté de liens qui
// portent exactement les noms attendus et mènent aux vrais fichiers. Le cœur
// lit la fiche là, et y trouve ses pistes — quel que soit le cœur, puisque
// chacun résout les noms relativement à sa fiche. Une fiche juste est rendue
// telle quelle, et un nom introuvable même à la casse près aussi : le cœur
// dira lui-même ce qui manque.
//
// `scratchDir` vit dans le conteneur de la partie, qui disparaît avec elle :
// rien à nettoyer.
func cueWithExactNames(cuePath, scratchDir string) (string, error) {
	file, err := os.Open(cuePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	folder := filepath.Dir(cuePath)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return "", err
	}

	// referenced → le fichier réel, pour chaque nom qu'il faudra exposer.
	links := map[string]string{}
	needsRewrite := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := cueFileLine.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}

		referenced := match[1]
		if referenced == "" {
			referenced = match[2]
		}
		if filepath.Base(referenced) != referenced {
			// Un chemin, pas un nom : hors de ce qu'on sait réparer sans
			// deviner. On laisse le cœur en juger.
			continue
		}

		if _, err := os.Stat(filepath.Join(folder, referenced)); err == nil {
			links[referenced] = filepath.Join(folder, referenced)

			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(entry.Name(), referenced) {
				links[referenced] = filepath.Join(folder, entry.Name())
				needsRewrite = true

				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// LE .SBI SUIT LA FICHE. pcsx_rearmed le cherche à côté de la fiche qu'il
	// lit, sous son nom (`opensbifile` : la fiche, en .sbi) : il porte les
	// sous-canaux qu'une protection LibCrypt vérifie — Final Fantasy VIII PAL,
	// et une centaine d'autres (2026-10-13). Une fiche refaite le perdait ; un
	// .sbi rangé sous une autre casse n'était jamais trouvé non plus.
	sbiName := strings.TrimSuffix(filepath.Base(cuePath), filepath.Ext(cuePath)) + ".sbi"
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(entry.Name(), sbiName) {
			continue
		}
		links[sbiName] = filepath.Join(folder, entry.Name())
		if entry.Name() != sbiName {
			needsRewrite = true
		}

		break
	}

	if !needsRewrite {
		return cuePath, nil
	}

	staged, err := os.MkdirTemp(scratchDir, "cue-")
	if err != nil {
		return "", fmt.Errorf("dossier de la fiche : %w", err)
	}
	for name, real := range links {
		if err := os.Symlink(real, filepath.Join(staged, name)); err != nil {
			return "", fmt.Errorf("lien %s : %w", name, err)
		}
	}
	// La fiche est COPIÉE, pas liée : un cœur qui résoudrait le lien
	// chercherait ses pistes dans le dossier d'origine, avec la mauvaise casse.
	content, err := os.ReadFile(cuePath)
	if err != nil {
		return "", err
	}
	stagedCue := filepath.Join(staged, filepath.Base(cuePath))
	if err := os.WriteFile(stagedCue, content, 0o644); err != nil {
		return "", fmt.Errorf("copie de la fiche : %w", err)
	}

	return stagedCue, nil
}
