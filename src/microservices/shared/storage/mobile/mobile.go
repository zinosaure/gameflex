// Package mobile est la façade que l'application Android appelle.
//
// gomobile ne sait exposer qu'un jeu de types très étroit — entiers, chaînes,
// booléens, []byte — et surtout PAS une tranche de structures. Une API
// idiomatique en Go (`[]Game`) ne traverse donc pas le pont.
//
// D'où cette couche : du JSON entre, du JSON sort. C'est laid vu de Go et
// parfaitement naturel vu de Kotlin, et ça garde la vraie logique — la
// convention de rangement, la hiérarchie des descripteurs — dans un paquet
// pur, testable sans téléphone.
//
// La règle qui justifie tout ça : le scan est écrit UNE fois. Ses règles ont
// coûté quatre bugs trouvés sur une vraie bibliothèque de 23 000 jeux ; les
// réécrire en Java, c'est signer pour les retrouver un par un.
package mobile

import (
	"encoding/json"
	"fmt"

	"gameflex/storage"
)

// SourceSpec décrit une source à monter, telle que l'app la connaît.
type SourceSpec struct {
	// Name — le nom de montage, qui devient le premier segment du chemin.
	Name string `json:"name"`
	// Kind — "local" : la bibliothèque n'a plus d'autre source. Les partages (Samba, WebDAV, kDrive, FTP, SFTP) se
	// lisent par un gameflex-federation, que l'appareil rejoint comme un serveur fédéré (chantier 61, P6).
	Kind string `json:"kind"`
	// Root — le chemin racine, pour une source locale.
	Root string `json:"root"`
}

// Scan parcourt les sources LOCALES de la bibliothèque et rend les jeux
// trouvés, en JSON — chacune posée sur un dossier des jeux, rangé selon la
// convention `<consoleid>/<jeu>` (chantier 15).
//
// La bibliothèque n'a que des sources locales : un partage se lit par un gameflex-federation (chantier 61). Un autre
// type est une erreur qui le nomme.
//
// Une erreur revient en erreur, jamais en JSON vide : « aucun jeu » et « je
// n'ai pas pu lire » sont deux réponses différentes, et les confondre donne
// une bibliothèque vide sans raison affichée.
func Scan(sourcesJSON string) (string, error) {
	var specs []SourceSpec
	if err := json.Unmarshal([]byte(sourcesJSON), &specs); err != nil {
		return "", fmt.Errorf("sources illisibles : %w", err)
	}

	var games []storage.Game
	for _, spec := range specs {
		if spec.Kind != "" && spec.Kind != "local" {
			return "", fmt.Errorf("source %q : type %q inconnu pour une bibliothèque", spec.Name, spec.Kind)
		}
		source, err := storage.NewLocalSource(spec.Name, spec.Root)
		if err != nil {
			return "", fmt.Errorf("source %q : %w", spec.Name, err)
		}
		found, err := storage.ScanConvention(source, "")
		if err != nil {
			return "", err
		}
		games = append(games, found...)
	}
	// Une tranche nil se sérialise en `null`, pas en `[]` — et l'appelant se
	// prend un « Value null ... cannot be converted » au lieu d'une
	// bibliothèque vide. « Rien trouvé » est une réponse, elle doit avoir la
	// même forme que les autres.
	if games == nil {
		games = []storage.Game{}
	}

	out, err := json.Marshal(games)
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// Consoles rend les consoles connues, en JSON — de quoi grouper la
// bibliothèque et nommer chaque section sans dupliquer le registre.
func Consoles() (string, error) {
	out, err := json.Marshal(storage.KnownConsoles())
	if err != nil {
		return "", err
	}

	return string(out), nil
}
