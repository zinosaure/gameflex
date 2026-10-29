package saves

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gameflex/storage/installs"
)

// LES CONSOLES RÉCENTES : L'UNITÉ EST LE JEU. « On remplace ou copie le jeu
// de la console seulement. Pas tous les jeux » (le porteur, 2026-10-29).
//
// Chaque émulateur range sous `streamex/<console>/` ses sauvegardes, mais aussi
// sa configuration, ses journaux et ses caches. Une sauvegarde de jeu y est un
// dossier nommé par l'identifiant du jeu, sous un dossier connu (relevé sur la
// dev le 2026-10-29) — c'est cette table. Le reste n'est jamais copié.

// XboxDisk — l'identifiant du « jeu » Xbox : la Xbox n'a qu'un disque pour
// tous ses jeux, et c'est lui qui se copie.
const XboxDisk = "disk"

// layout — où une console range les sauvegardes de ses jeux.
type layout struct {
	// parents — les dossiers dont chaque sous-dossier est la sauvegarde d'un
	// jeu, relatifs au dossier de la console ; `*` vaut un segment (le compte
	// de l'émulateur : `home/00000001` de la PS3, `home/1000` de la PS4).
	parents []string
	// idOf — le jeu d'un sous-dossier, vide quand ce n'en est pas un.
	idOf func(name string) string
	// title — ce qu'un dossier dit du jeu, vide quand rien ne se lit.
	title func(dir string) string
	// profile — le `*` du parent est le PROFIL de l'émulateur (la Switch) :
	// une sauvegarde restaurée passe sous le profil d'ici.
	profile bool
	// files — la Xbox : quelques fichiers, la console entière.
	files []string
}

var (
	ps3Product = regexp.MustCompile(`^[A-Z]{4}[0-9]{5}`)
	switchID   = regexp.MustCompile(`^[0-9A-Fa-f]{16}$`)
	wiiuTitle  = regexp.MustCompile(`<(?:shortname|longname)_en[^>]*>([^<]+)<`)
)

func plainID(name string) string {
	if strings.HasPrefix(name, ".") {
		return ""
	}

	return name
}

var layouts = map[string]layout{
	// UN JEU PS3 A PLUSIEURS DOSSIERS : un par sauvegarde (`MRTC00003-00`,
	// `MRTC00003-01`, `NPEB01140_AUTO`). Ils commencent tous par le code du
	// produit, qui est donc le jeu.
	"ps3": {
		parents: []string{"config/dev_hdd0/home/*/savedata"},
		idOf: func(name string) string {
			if code := ps3Product.FindString(name); code != "" {
				return code
			}

			return plainID(name)
		},
		title: func(dir string) string { return installs.SFOTitle(filepath.Join(dir, "PARAM.SFO")) },
	},
	"ps4": {
		parents: []string{"data/home/*/savedata"},
		idOf:    plainID,
		title: func(dir string) string {
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if title := installs.SFOTitle(filepath.Join(dir, entry.Name(), "sce_sys", "param.sfo")); title != "" {
					return title
				}
			}

			return ""
		},
	},
	"ps5": {parents: []string{"savedata"}, idOf: plainID},
	"switch": {
		parents: []string{"nand/user/save/0000000000000000/*"},
		idOf: func(name string) string {
			if switchID.MatchString(name) {
				return strings.ToUpper(name)
			}

			return ""
		},
		profile: true,
	},
	"wiiu": {
		parents: []string{"mlc01/usr/save/00050000"},
		idOf:    plainID,
		title: func(dir string) string {
			raw, err := os.ReadFile(filepath.Join(dir, "meta", "meta.xml"))
			if err != nil {
				return ""
			}
			if match := wiiuTitle.FindSubmatch(raw); match != nil {
				return strings.TrimSpace(string(match[1]))
			}

			return ""
		},
	},
	"xbox": {files: []string{"xemu/xbox_hdd.qcow2", "xemu/eeprom.bin"}},
}

// Consoles — les consoles dont les sauvegardes se copient, dans l'ordre de
// l'affichage. LA XBOX 360 N'Y EST PAS : la dev n'a aucune sauvegarde de Xenia
// pour relever son rangement (`content/<profil>/<titre>/`, d'après sa
// documentation). La copier sur une supposition rangerait peut-être des
// sauvegardes où Xenia ne les lit pas — elle attend une vraie sauvegarde.
func Consoles() []string {
	return []string{"ps3", "ps4", "ps5", "switch", "wiiu", "xbox"}
}

// Supported — cette console se copie-t-elle ?
func Supported(console string) bool {
	_, found := layouts[console]

	return found
}

// Game — la sauvegarde d'un jeu, dans un rangement.
type Game struct {
	Console   string    `json:"console"`
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	SizeBytes int64     `json:"sizeBytes"`
}

// consoleDir — `streamex/<console>` d'un rangement.
func consoleDir(tree, console string) string {
	return filepath.Join(tree, "streamex", console)
}

// roots — les dossiers (ou, pour la Xbox, les fichiers) de chaque jeu d'une
// console, relatifs à son dossier, par jeu.
func roots(dir, console string) map[string][]string {
	found := map[string][]string{}
	layout := layouts[console]
	for _, file := range layout.files {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file))); err == nil && info.Mode().IsRegular() {
			found[XboxDisk] = append(found[XboxDisk], file)
		}
	}
	for _, parent := range layout.parents {
		matches, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(parent)))
		for _, match := range matches {
			entries, err := os.ReadDir(match)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if id := layout.idOf(entry.Name()); id != "" {
					relative, _ := filepath.Rel(dir, filepath.Join(match, entry.Name()))
					found[id] = append(found[id], filepath.ToSlash(relative))
				}
			}
		}
	}

	return found
}

// Has — ce rangement a-t-il une sauvegarde de ce jeu ?
func Has(tree, console, id string) bool {
	return Supported(console) && len(roots(consoleDir(tree, console), console)[id]) > 0
}

// Games — les jeux qui ont une sauvegarde, console par console.
func Games(tree string) []Game {
	games := []Game{}
	for _, console := range Consoles() {
		dir := consoleDir(tree, console)
		for id, paths := range roots(dir, console) {
			game := Game{Console: console, ID: id}
			for _, relative := range paths {
				full := filepath.Join(dir, filepath.FromSlash(relative))
				size, latest := measure(full)
				game.SizeBytes += size
				if latest.After(game.UpdatedAt) {
					game.UpdatedAt = latest
				}
				if game.Title == "" && layouts[console].title != nil {
					game.Title = layouts[console].title(full)
				}
			}
			games = append(games, game)
		}
	}
	sort.Slice(games, func(i, j int) bool {
		if games[i].Console != games[j].Console {
			return games[i].Console < games[j].Console
		}

		return games[i].ID < games[j].ID
	})

	return games
}

// measure — la taille d'un dossier (ou d'un fichier) et la date de son
// fichier le plus récent. Les FICHIERS seulement : un dossier déplié par une
// archive porte la date du transfert, pas celle de la partie.
func measure(path string) (size int64, latest time.Time) {
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		size += info.Size()
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}

		return nil
	})

	return size, latest
}

// belongs — ce chemin d'archive est-il la sauvegarde de CE jeu ? Une archive
// ne dépose que là : ni la configuration de l'émulateur, ni un autre jeu, ni
// rien hors du dossier de la console.
func belongs(console, id, name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) || path.Clean(name) != name ||
		name == ".." || strings.HasPrefix(name, "../") {
		return false
	}
	layout := layouts[console]
	for _, file := range layout.files {
		if name == file && id == XboxDisk {
			return true
		}
	}
	parts := strings.Split(name, "/")
	for _, parent := range layout.parents {
		pattern := strings.Split(parent, "/")
		if len(parts) <= len(pattern) {
			continue
		}
		matched := true
		for index, segment := range pattern {
			if ok, _ := path.Match(segment, parts[index]); !ok || strings.HasPrefix(parts[index], ".") {
				matched = false

				break
			}
		}
		if matched && layout.idOf(parts[len(pattern)]) == id {
			return true
		}
	}

	return false
}

// WriteArchive — la sauvegarde d'un jeu, en zip, ses chemins relatifs au
// dossier de la console. os.ErrNotExist quand ce rangement ne l'a pas.
func WriteArchive(out io.Writer, tree, console, id string) error {
	if !Supported(console) {
		return fmt.Errorf("unsupported console %q", console)
	}
	dir := consoleDir(tree, console)
	paths := roots(dir, console)[id]
	if len(paths) == 0 {
		return os.ErrNotExist
	}

	archive := zip.NewWriter(out)
	for _, relative := range paths {
		base := filepath.Join(dir, filepath.FromSlash(relative))
		err := filepath.Walk(base, func(full string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() {
				return err
			}
			name, _ := filepath.Rel(dir, full)
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(name)
			header.Method = zip.Deflate
			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			file, err := os.Open(full)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)

			return err
		})
		if err != nil {
			return err
		}
	}

	return archive.Close()
}

// ErrForeignEntry — l'archive dépose ailleurs que dans la sauvegarde du jeu.
var ErrForeignEntry = errors.New("the archive writes outside this game's save")

// Receive — une archive (WriteArchive) mise dans un rangement, à la place de
// ce qu'il avait pour ce jeu. `fits` juge la taille dépliée qu'elle annonce
// (la place du disque), nil pour ne rien juger.
//
// DÉPLIÉE À CÔTÉ, PUIS MISE EN PLACE : une archive refusée ou coupée ne laisse
// rien, et l'ancienne sauvegarde ne disparaît qu'une fois la nouvelle entière.
func Receive(archivePath, tree, console, id string, fits func(uint64) error) error {
	if !Supported(console) {
		return fmt.Errorf("unsupported console %q", console)
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("unreadable archive: %w", err)
	}
	defer archive.Close()

	var total uint64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() || !belongs(console, id, entry.Name) {
			return fmt.Errorf("%w: %s", ErrForeignEntry, entry.Name)
		}
		total += entry.UncompressedSize64
	}
	if fits != nil {
		if err := fits(total); err != nil {
			return err
		}
	}

	dir := consoleDir(tree, console)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(dir, ".incoming-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if err := extract(entry, filepath.Join(staging, filepath.FromSlash(entry.Name))); err != nil {
			return err
		}
	}

	return replaceGame(dir, staging, console, id)
}

func extract(entry *zip.File, target string) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, source)
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}

	return os.Chtimes(target, time.Now(), entry.Modified)
}

// replaceGame — la sauvegarde d'un jeu préparée dans `staging` (rangée comme
// le dossier de la console) prend la place de celle de `dir`.
func replaceGame(dir, staging, console, id string) error {
	for _, relative := range roots(dir, console)[id] {
		if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(relative))); err != nil {
			return err
		}
	}
	for _, relative := range roots(staging, console)[id] {
		target := filepath.Join(dir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(staging, filepath.FromSlash(relative)), target); err != nil {
			return err
		}
	}

	return nil
}

// stage — la sauvegarde d'un jeu de `from`, copiée dans un dossier de
// préparation sous `to` (le même disque : la mise en place est un renommage).
// `profile` non vide range une sauvegarde Switch sous ce profil ; quand deux
// profils de la source ont le même jeu, le plus récent l'emporte.
func stage(from, to, console, id, profile string) (string, error) {
	paths := roots(from, console)[id]
	if len(paths) == 0 {
		return "", os.ErrNotExist
	}
	sort.Slice(paths, func(i, j int) bool {
		_, a := measure(filepath.Join(from, filepath.FromSlash(paths[i])))
		_, b := measure(filepath.Join(from, filepath.FromSlash(paths[j])))

		return a.Before(b)
	})
	if err := os.MkdirAll(to, 0o755); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(to, ".incoming-")
	if err != nil {
		return "", err
	}
	for _, relative := range paths {
		target := relative
		if profile != "" && layouts[console].profile {
			parts := strings.Split(relative, "/")
			parts[len(strings.Split(layouts[console].parents[0], "/"))-1] = profile
			target = strings.Join(parts, "/")
		}
		destination := filepath.Join(staging, filepath.FromSlash(target))
		if err := os.RemoveAll(destination); err != nil {
			os.RemoveAll(staging)

			return "", err
		}
		source := filepath.Join(from, filepath.FromSlash(relative))
		info, err := os.Stat(source)
		if err == nil && info.IsDir() {
			err = copyTree(source, destination)
		} else if err == nil {
			err = copyFile(source, destination)
		}
		if err != nil {
			os.RemoveAll(staging)

			return "", err
		}
	}

	return staging, nil
}

// SwitchProfile — le profil de l'émulateur Switch d'un rangement : le premier
// de `profiles.dat`, tel que la NAND nomme son dossier — l'UUID lu comme deux
// entiers de 64 bits, le haut d'abord (« Eden »,
// `CB4433B032A5AC306FD0DCCA511C514B` sur la dev). Vide quand l'émulateur n'en
// a pas encore créé : une sauvegarde garde alors le profil d'où elle vient.
func SwitchProfile(tree string) string {
	raw, err := os.ReadFile(filepath.Join(consoleDir(tree, "switch"),
		"nand", "system", "save", "8000000000000010", "su", "avators", "profiles.dat"))
	if err != nil {
		return ""
	}
	const header, entry = 0x10, 0xC8
	for offset := header; offset+16 <= len(raw) && offset < header+8*entry; offset += entry {
		low := binary.LittleEndian.Uint64(raw[offset : offset+8])
		high := binary.LittleEndian.Uint64(raw[offset+8 : offset+16])
		if low != 0 || high != 0 {
			return fmt.Sprintf("%016X%016X", high, low)
		}
	}

	return ""
}

// Backup — la sauvegarde d'un jeu du principal, copiée dans `local` (à la
// place de la précédente du même jeu).
func Backup(savesRoot, username, console, id string) error {
	if !Supported(console) {
		return fmt.Errorf("unsupported console %q", console)
	}
	principal, err := Tree(savesRoot, username, "")
	if err != nil {
		return err
	}
	local, err := Tree(savesRoot, username, Local)
	if err != nil {
		return err
	}
	to := consoleDir(local, console)
	staging, err := stage(consoleDir(principal, console), to, console, id, "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	return replaceGame(to, staging, console, id)
}

// Restore — la sauvegarde d'un jeu d'une copie remise dans le principal, à la
// place de la sienne, qui part d'abord dans `local`. La Switch la range sous
// le profil d'ici.
//
// LA COPIE EST PRÉPARÉE AVANT LE BACKUP : restaurer depuis `local` lui-même
// échange les deux versions au lieu de perdre celle qu'on restaure.
func Restore(savesRoot, username, source, console, id string) error {
	if !Supported(console) {
		return fmt.Errorf("unsupported console %q", console)
	}
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
	to := consoleDir(principal, console)
	profile := ""
	if layouts[console].profile {
		profile = SwitchProfile(principal)
	}
	staging, err := stage(consoleDir(from, console), to, console, id, profile)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if len(roots(to, console)[id]) > 0 {
		if err := Backup(savesRoot, username, console, id); err != nil {
			return err
		}
	}

	return replaceGame(to, staging, console, id)
}

// Remove — la sauvegarde d'un jeu, effacée d'un rangement.
func Remove(tree, console, id string) error {
	if !Supported(console) {
		return fmt.Errorf("unsupported console %q", console)
	}
	dir := consoleDir(tree, console)
	for _, relative := range roots(dir, console)[id] {
		if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(relative))); err != nil {
			return err
		}
	}

	return nil
}

// Size — ce que pèse la sauvegarde d'un jeu dans un rangement (la place qu'il
// faut pour la copier).
func Size(tree, console, id string) int64 {
	dir := consoleDir(tree, console)
	var total int64
	for _, relative := range roots(dir, console)[id] {
		size, _ := measure(filepath.Join(dir, filepath.FromSlash(relative)))
		total += size
	}

	return total
}
