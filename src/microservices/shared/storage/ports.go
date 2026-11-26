package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gameflex/consoles"
)

// QUEL PORT JOUE CE JEU ? (chantiers 40, 72)
//
// Les jeux de la console `ports` sont rangés chez leur console d'origine
// (`roms/ps5`) : un port les reconnaît à leur identifiant et à leur version
// (consoles.Port.Titles). Un fichier n'est jamais un jeu de `ports` : la
// reconnaissance d'une ROM par sa taille et son SHA-1 servait les ports rétro,
// retirés le 2026-11-23 (le porteur : pas de jeux rétro).

// knownPorts — la table du registre ; un test y met la sienne.
var knownPorts = consoles.Ports

// PortOf — le port qui joue le jeu-dossier `path`, rangé chez la console
// `from`. Faux pour un fichier, ou un jeu qu'aucun port n'accepte.
func PortOf(from, path string) (consoles.Port, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return consoles.Port{}, false
	}

	return portOfTitle(from, path)
}

// portOfTitle — le port qui joue le jeu-dossier `dir` (une copie PS5) : son identifiant et sa version, lus dans
// `sce_sys/param.json` (titleId, contentVersion).
func portOfTitle(from, dir string) (consoles.Port, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "sce_sys", "param.json"))
	if err != nil {
		return consoles.Port{}, false
	}
	var param struct {
		TitleID        string `json:"titleId"`
		ContentVersion string `json:"contentVersion"`
	}
	if json.Unmarshal(raw, &param) != nil {
		return consoles.Port{}, false
	}
	for _, port := range knownPorts() {
		if port.From != from {
			continue
		}
		for _, title := range port.Titles {
			if title.TitleID == param.TitleID && title.Version == param.ContentVersion {
				return port, true
			}
		}
	}

	return consoles.Port{}, false
}
