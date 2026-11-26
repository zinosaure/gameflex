// Package saves — LES SAUVEGARDES PAR SOURCE (chantier 38, le porteur,
// 2026-10-28 et 29) : un principal, que les émulateurs jouent, et des copies.
//
//	saves/<compte>/                    le principal — inchangé
//	saves/<compte>/sources/local/      les backups
//	saves/<compte>/sources/<serveur>/  ce qu'un serveur fédéré nous a envoyé,
//	                                   ou ce qu'on y a téléchargé
//
// Une copie se range comme le principal : `states/<console>/…` pour le rétro,
// `streamex/<console>/…` pour les consoles récentes.
//
// AUCUN TRANSFERT N'ÉCRIT DANS UN PRINCIPAL. Envoyer et Télécharger écrivent
// dans une source ; seule la restauration — un geste de l'utilisateur, sur sa
// machine — remet une copie dans le principal, et ce que le principal avait
// part d'abord dans `local`. « Chaque upload qu'on fera sur des serveurs
// fédérés, ça ne va pas remplacer la sauvegarde principale » (le porteur).
//
// Le serveur et le téléphone rangent de la même façon : la règle est ici,
// écrite une fois, comme celle de `save_paths.go`.
package saves

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"gameflex/storage"
)

// Local — la source des backups.
const Local = "local"

// sourcesDir — le dossier des copies, dans celui du compte.
const sourcesDir = "sources"

// maxSourceName — un nom de dossier raisonnable, en octets.
const maxSourceName = 100

// Tree — la racine d'un rangement : le principal du compte pour une source
// vide, sinon sa copie. Le compte et la source sont des segments vérifiés :
// « ../bob » ne sort pas du dossier du compte.
func Tree(savesRoot, username, source string) (string, error) {
	root, owner, err := TreeParts(savesRoot, username, source)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, owner), nil
}

// TreeParts — la même racine en deux morceaux, (dossier, nom) : la forme
// `(savesRoot, username)` que prennent les chemins du rétro
// (storage.SaveStatePath, et `retro_files` au serveur). Une copie s'y passe
// comme un compte : `(saves/<compte>/sources, <source>)`.
func TreeParts(savesRoot, username, source string) (root, owner string, err error) {
	userDir, err := storage.UserSavesDir(savesRoot, username)
	if err != nil {
		return "", "", err
	}
	if source == "" {
		return savesRoot, username, nil
	}
	if !ValidSource(source) {
		return "", "", fmt.Errorf("invalid save source %q", source)
	}

	return filepath.Join(userDir, sourcesDir), source, nil
}

// ValidSource — un nom de source tel qu'il est sur le disque : un seul
// segment, pas caché, pas de caractère de contrôle.
func ValidSource(source string) bool {
	if source == "" || len(source) > maxSourceName || strings.HasPrefix(source, ".") ||
		strings.ContainsAny(source, `/\`) || strings.TrimSpace(source) != source || !utf8.ValidString(source) {
		return false
	}
	for _, r := range source {
		if unicode.IsControl(r) {
			return false
		}
	}

	return true
}

// SourceName — le dossier où ranger ce qu'envoie, ou ce qu'on télécharge
// depuis, un serveur fédéré : LE NOM QUE LA FÉDÉRATION LUI DONNE (le porteur,
// 2026-10-29). Chez nous, celui de notre liste de fédération ; chez lui, celui
// que notre serveur annonce — c'est aussi le nom que sa propre liste nous
// aurait donné en nous ajoutant.
//
// Rendu sûr pour un système de fichiers (storage.SafeSaveName : le téléphone
// suit les règles de FAT), jamais caché, et jamais `local` : un serveur ainsi
// nommé n'écrit pas dans nos backups. Vide quand il ne reste rien.
func SourceName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, storage.SafeSaveName(strings.TrimSpace(name)))
	name = strings.TrimSpace(strings.TrimLeft(name, "."))
	for len(name) > maxSourceName {
		_, size := utf8.DecodeLastRuneInString(name)
		name = strings.TrimSpace(name[:len(name)-size])
	}
	if strings.EqualFold(name, Local) {
		name += "_"
	}

	return name
}

// Sources — les copies d'un compte : `local` d'abord, puis les serveurs, par
// nom. Vide quand il n'y en a aucune.
func Sources(savesRoot, username string) ([]string, error) {
	userDir, err := storage.UserSavesDir(savesRoot, username)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(userDir, sourcesDir))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && ValidSource(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		if (names[i] == Local) != (names[j] == Local) {
			return names[i] == Local
		}

		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	return names, nil
}

// LE JEU D'UNE SAUVEGARDE, GARDÉ AVEC ELLE (chantier 76). Une sauvegarde se
// nomme par un identifiant : la référence du jeu pour le rétro (un UUID), son
// identifiant de titre pour une console récente (PPSA14325, CUSA07010). C'est
// la bibliothèque d'ici qui en donne le titre ; quand le jeu la quittait, ou
// qu'une copie venait d'un serveur qui ne l'a pas, il se perdait. Chaque
// rangement garde donc `titles.json` à sa racine, hors des dossiers que lisent
// les émulateurs et que parcourt le listage : pour chaque sauvegarde, le
// titre de son jeu, sa référence et son chemin (le porteur, 2026-11-23 : « Le
// titre, l'identifiant et si possible le pathname »).
const titlesFile = "titles.json"

var titlesMu sync.Mutex

// KnownGame — ce que `titles.json` garde du jeu d'une sauvegarde : son titre,
// sa référence dans la bibliothèque, et son chemin depuis le dossier des jeux
// de la bibliothèque ou de son disque (« ps5/Astro Bot »).
type KnownGame struct {
	Title string `json:"title"`
	ID    string `json:"id,omitempty"`
	Path  string `json:"path,omitempty"`
}

// Named — une sauvegarde (sa console, son identifiant) et son jeu.
type Named struct {
	Console string
	Save    string
	Game    KnownGame
}

// Remember — ces sauvegardes et leur jeu, dans `titles.json` du rangement :
// une seule écriture, rien sans titre, rien quand tout y est déjà.
func Remember(tree string, named ...Named) error {
	titlesMu.Lock()
	defer titlesMu.Unlock()

	games, changed := readTitles(tree)
	for _, entry := range named {
		entry.Game.Title = strings.TrimSpace(entry.Game.Title)
		key := titleKey(entry.Console, entry.Save)
		if entry.Game.Title == "" || games[key] == entry.Game {
			continue
		}
		games[key], changed = entry.Game, true
	}
	if !changed {
		return nil
	}

	return writeTitles(tree, games)
}

// Known — le jeu gardé pour chaque sauvegarde de ce rangement, le fichier lu
// une fois pour toute une liste.
func Known(tree string) func(console, save string) (KnownGame, bool) {
	titlesMu.Lock()
	games, legacy := readTitles(tree)
	if legacy {
		_ = writeTitles(tree, games)
	}
	titlesMu.Unlock()

	return func(console, save string) (KnownGame, bool) {
		game, found := games[titleKey(console, save)]

		return game, found
	}
}

// Announced — le jeu qu'un envoi annonce avec une sauvegarde (`title`,
// `gameId`, `path` : ce que la liste de l'expéditeur en disait), pour la copie
// qui la reçoit. `reference` : l'identifiant que porte déjà la sauvegarde (le
// rétro), sinon `gameId`.
func Announced(query url.Values, reference string) KnownGame {
	if reference == "" {
		reference = query.Get("gameId")
	}

	return KnownGame{Title: query.Get("title"), ID: reference, Path: query.Get("path")}
}

// carryKnown — le jeu d'une sauvegarde suit sa copie d'un rangement à l'autre
// (un backup, une restauration), sous l'identifiant qu'elle prend là-bas.
func carryKnown(fromTree, toTree, console, save, toSave string) error {
	game, found := Known(fromTree)(console, save)
	if !found {
		return nil
	}

	return Remember(toTree, Named{Console: console, Save: toSave, Game: game})
}

// titleKey — l'identifiant tel que le nom du fichier le porte
// (storage.SafeSaveName) : une liste relit l'identifiant dans le nom, pas
// celui que l'envoi a reçu.
func titleKey(console, save string) string {
	return console + "/" + storage.SafeSaveName(save)
}

// readTitles — `titles.json` d'un rangement. LES ENTRÉES D'AVANT LE CHANTIER
// 76, un titre seul, se relisent en jeu sans référence ni chemin, et `legacy`
// demande de réécrire le fichier : une seule fois, puis il est au format du
// jeu (le porteur, 2026-11-23, sans rétro compatibilité au-delà).
func readTitles(tree string) (games map[string]KnownGame, legacy bool) {
	games = map[string]KnownGame{}
	raw, err := os.ReadFile(filepath.Join(tree, titlesFile))
	if err != nil {
		return games, false
	}
	entries := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &entries) != nil {
		return games, false
	}
	for key, value := range entries {
		var game KnownGame
		if json.Unmarshal(value, &game) == nil {
			games[key] = game

			continue
		}
		var title string
		if json.Unmarshal(value, &title) == nil && strings.TrimSpace(title) != "" {
			games[key], legacy = KnownGame{Title: strings.TrimSpace(title)}, true
		}
	}

	return games, legacy
}

func writeTitles(tree string, games map[string]KnownGame) error {
	raw, err := json.MarshalIndent(games, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tree, 0o755); err != nil {
		return err
	}

	return writeAtomically(filepath.Join(tree, titlesFile), raw)
}

func writeAtomically(path string, raw []byte) error {
	partial := path + ".part"
	if err := os.WriteFile(partial, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partial, path); err != nil {
		_ = os.Remove(partial)

		return err
	}

	return nil
}

// copyFile — un fichier, écrit à côté puis renommé (une copie coupée ne laisse
// jamais un demi-fichier sous un nom valide), avec la date de l'original : la
// date d'une sauvegarde est celle de la partie, pas celle de la copie.
func copyFile(from, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	partial := to + ".part"
	out, err := os.Create(partial)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, source)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partial)
		if copyErr != nil {
			return copyErr
		}

		return closeErr
	}
	if err := os.Rename(partial, to); err != nil {
		_ = os.Remove(partial)

		return err
	}

	return os.Chtimes(to, time.Now(), info.ModTime())
}

// copyTree — un dossier entier, fichier par fichier (copyFile), les dates des
// dossiers gardées aussi. Les liens symboliques ne se suivent pas : une
// sauvegarde n'en contient pas, et en suivre un sortirait du rangement.
func copyTree(from, to string) error {
	err := filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return nil
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		default:
			return copyFile(path, target)
		}
	})
	if err != nil {
		return err
	}

	return keepDirTimes(from, to)
}

// keepDirTimes — la date de chaque dossier copié, posée APRÈS les fichiers
// (écrire dedans la changerait).
func keepDirTimes(from, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(from, path)

		return os.Chtimes(filepath.Join(to, relative), time.Now(), info.ModTime())
	})
}
