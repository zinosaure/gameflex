package storage

// L'IDENTIFIANT D'UN JEU SWITCH, LU DANS SON FICHIER (le porteur, 2026-11-15 :
// « Le lire dans le fichier »). « Crash Bandicoot.xci » ne dit rien de lui dans son
// nom, et ses sauvegardes se rangent pourtant sous son identifiant
// (`nand/user/save/…/0100D1B006744000/`), comme ses réglages
// (`patches/switch/<identifiant>/`) et ses métadonnées (`metadata/switch/<identifiant>/`) :
//
//   - un paquet `.nsp` (PFS0) porte souvent le ticket de son jeu, `<rightsid>.tik` :
//     les 16 premiers chiffres de son nom sont l'identifiant, sans aucune clé ;
//   - sinon, et toujours pour une cartouche `.xci` (HFS0, sa partition `secure`),
//     l'en-tête d'un NCA de programme ou de contrôle, chiffré par la `header_key`
//     des clés de la console (`bios/switch/prod.keys`, déjà là pour l'émulateur) :
//     son « program id » est à 0x210.
//
// L'IDENTIFIANT RENDU EST CELUI DU JEU DE BASE : une mise à jour (+0x800), un contenu
// additionnel (+0x1000 et son numéro) ou le programme d'une compilation (+1, +2… :
// *Super Mario 3D All-Stars*, *Kingdom Hearts 1.5 + 2.5*) ne diffèrent de lui que
// par les 13 bits du bas, que l'écosystème efface (Yuzu, Eden : GetBaseTitleID,
// `& 0xFFFFFFFFFFFFE000`). Les dossiers de sauvegarde se nomment par lui.
//
// SEULS LES EN-TÊTES SE LISENT — quelques Ko d'un fichier de 8 à 57 Go, de 0,1 à
// 250 ms sur le disque Gamebox (relevé le 2026-11-15 sur ses 37 paquets) —, et le
// résultat se garde par fichier, taille et date. Sans clés, ou sur un fichier qui
// n'est pas un paquet Switch, rien : le jeu garde ce qu'il avait (sa référence).
// Un résultat manqué faute de clés ne se garde pas : des clés déposées valent à la
// lecture suivante.

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"gameflex/consoles"
)

const (
	// switchKeysFile — les clés de la console, que le registre exige de la Switch.
	switchKeysFile = "prod.keys"
	// switchBaseMask — ce qui reste du jeu de base dans l'identifiant d'une mise à
	// jour, d'un contenu additionnel ou d'un programme d'une compilation.
	switchBaseMask = ^uint64(0x1FFF)
	// ncaSector — le secteur du chiffrement de l'en-tête d'un NCA. Le second
	// (0x200-0x3FF) porte la magie, la sorte du contenu et le « program id ».
	ncaSector = 0x200
	// Les sortes de contenu d'un NCA qui disent le jeu lui-même (0 programme,
	// 2 contrôle) — pas sa notice (3), ni ses métadonnées (1).
	ncaProgram = 0
	ncaControl = 2
	// gameCardKeyArea — la zone des clés d'une cartouche, qu'une copie complète
	// garde en tête du `.xci`.
	gameCardKeyArea = 0x1000
	// Au-delà, ce n'est pas l'en-tête d'une partition : rien ne s'alloue sur la
	// foi d'un fichier quelconque.
	partitionMaxFiles = 4096
	partitionMaxNames = 1 << 20
)

// switchTitlePattern — l'identifiant d'un titre Switch : 16 chiffres
// hexadécimaux, en majuscules comme les dossiers de sauvegarde.
var switchTitlePattern = regexp.MustCompile(`^[0-9A-F]{16}$`)

// switchPackages — les conteneurs d'un jeu Switch, par extension : PFS0 pour un
// paquet (`.nsp`, et `.nsz`, dont les NCZ gardent l'en-tête chiffré de leur NCA),
// HFS0 pour une cartouche (`.xci`, `.xcz`).
var switchPackages = map[string]func(io.ReaderAt) ([]packageFile, error){
	".nsp": nspFiles, ".nsz": nspFiles, ".xci": xciFiles, ".xcz": xciFiles,
}

var errNotSwitchPackage = errors.New("not a Switch package")

// LA RACINE DES BIOS, où sont les clés de la console (`<racine>/switch/prod.keys`) :
// le dossier du serveur, ou la bibliothèque du téléphone, qui change avec elle.
// Chaque kernel la donne au démarrage (UseBiosRoot) ; sans elle, un jeu Switch
// n'a d'identifiant que par son ticket.
var biosRoot struct {
	sync.Mutex
	find func() string
}

// UseBiosRoot — où chercher les clés d'une console : `find` rend la racine des
// BIOS, "" quand elle n'est pas encore connue.
func UseBiosRoot(find func() string) {
	biosRoot.Lock()
	defer biosRoot.Unlock()
	biosRoot.find = find
}

// switchTitle — ce qu'un paquet dit de son jeu : l'identifiant du jeu de base, et
// s'il l'a lu tel quel (`base`) plutôt que dans une mise à jour ou un contenu
// additionnel — un dossier qui range les deux préfère le jeu lui-même.
type switchTitle struct {
	id   uint64
	base bool
}

func (title switchTitle) String() string {
	return fmt.Sprintf("%016X", title.id)
}

// better — `title`, ou l'identifiant `raw` s'il dit mieux le jeu de base.
func (title switchTitle) better(raw uint64) switchTitle {
	candidate := switchTitle{id: raw & switchBaseMask, base: raw&^switchBaseMask == 0}
	if raw == 0 || (title.id != 0 && (title.base || !candidate.base)) {
		return title
	}

	return candidate
}

var switchTitles = struct {
	sync.Mutex
	byFile map[string]switchTitle
}{byFile: map[string]switchTitle{}}

// switchFileTitle — l'identifiant du jeu de base du paquet `path`. Faux quand il
// ne se lit pas.
func switchFileTitle(path string) (switchTitle, bool) {
	list := switchPackages[strings.ToLower(filepath.Ext(path))]
	info, err := os.Stat(path)
	if list == nil || err != nil || !info.Mode().IsRegular() {
		return switchTitle{}, false
	}
	stamp := fileStamp(path, info)
	switchTitles.Lock()
	title, found := switchTitles.byFile[stamp]
	switchTitles.Unlock()
	if found {
		return title, title.id != 0
	}

	title, final := readSwitchTitle(path, list)
	if final {
		switchTitles.Lock()
		switchTitles.byFile[stamp] = title
		switchTitles.Unlock()
	}

	return title, title.id != 0
}

// switchFolderTitle — l'identifiant d'un jeu-dossier Switch : celui de ses
// paquets, le jeu de base d'abord. *Mario Kart 8 Deluxe* range le sien avec son
// contenu additionnel : les deux disent le même jeu, aux bits du bas près.
func switchFolderTitle(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	found := switchTitle{}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		title, ok := switchFileTitle(filepath.Join(dir, entry.Name()))
		if !ok {
			continue
		}
		if title.base {
			return title.String()
		}
		if found.id == 0 {
			found = title
		}
	}
	if found.id == 0 {
		return ""
	}

	return found.String()
}

// readSwitchTitle — le ticket d'abord, sans clé ; puis les en-têtes des NCA de
// programme et de contrôle. `final` : le résultat vaut tant que le fichier ne
// change pas — faux quand il manquait les clés, ou que le fichier ne s'ouvrait
// pas.
func readSwitchTitle(path string, list func(io.ReaderAt) ([]packageFile, error)) (title switchTitle, final bool) {
	file, err := os.Open(path)
	if err != nil {
		return switchTitle{}, false
	}
	defer file.Close()
	files, err := list(file)
	if err != nil {
		return switchTitle{}, true
	}

	for _, entry := range files {
		name := strings.ToLower(entry.name)
		if len(name) != 32+len(".tik") || !strings.HasSuffix(name, ".tik") {
			continue
		}
		if raw, err := strconv.ParseUint(name[:16], 16, 64); err == nil {
			title = title.better(raw)
		}
	}
	if title.id != 0 {
		return title, true
	}

	key := switchHeaderKey()
	if key == nil {
		return switchTitle{}, false
	}
	header, err := newNCAHeaderCipher(key)
	if err != nil {
		return switchTitle{}, false
	}
	for _, entry := range files {
		name := strings.ToLower(entry.name)
		if !strings.HasSuffix(name, ".nca") && !strings.HasSuffix(name, ".ncz") {
			continue
		}
		raw, kind, ok := ncaProgramID(file, entry.offset, header)
		if !ok || (kind != ncaProgram && kind != ncaControl) {
			continue
		}
		if title = title.better(raw); title.base {
			break
		}
	}

	return title, true
}

// ncaProgramID — le « program id » et la sorte de contenu de l'en-tête du NCA qui
// commence à `offset`. Seul son second secteur se lit et se déchiffre.
func ncaProgramID(reader io.ReaderAt, offset int64, header *ncaHeaderCipher) (programID uint64, kind byte, ok bool) {
	sector := make([]byte, ncaSector)
	if _, err := reader.ReadAt(sector, offset+ncaSector); err != nil {
		return 0, 0, false
	}
	header.decryptSector(sector, 1)
	if magic := string(sector[:4]); magic != "NCA3" && magic != "NCA2" {
		return 0, 0, false
	}

	return binary.LittleEndian.Uint64(sector[0x10:0x18]), sector[0x05], true
}

// ncaHeaderCipher — l'AES-128-XTS de l'en-tête d'un NCA : la clé de 32 octets en
// deux (les données, puis le tweak), des secteurs de 0x200 octets, et le numéro
// du secteur écrit en GRAND-boutiste dans le tweak — l'inverse de la norme et de
// golang.org/x/crypto/xts, qui l'écrivent en petit-boutiste (le « tweak de
// Nintendo » de hactool). Le reste est l'XTS de la norme.
type ncaHeaderCipher struct {
	data, tweak cipher.Block
}

func newNCAHeaderCipher(key []byte) (*ncaHeaderCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("the NCA header key is 32 bytes")
	}
	data, err := aes.NewCipher(key[:16])
	if err != nil {
		return nil, err
	}
	tweak, err := aes.NewCipher(key[16:])
	if err != nil {
		return nil, err
	}

	return &ncaHeaderCipher{data: data, tweak: tweak}, nil
}

// decryptSector — déchiffre sur place le secteur numéro `number`.
func (header *ncaHeaderCipher) decryptSector(sector []byte, number uint64) {
	var tweak [16]byte
	binary.BigEndian.PutUint64(tweak[8:], number)
	header.tweak.Encrypt(tweak[:], tweak[:])
	for start := 0; start+16 <= len(sector); start += 16 {
		block := sector[start : start+16]
		subtle.XORBytes(block, block, tweak[:])
		header.data.Decrypt(block, block)
		subtle.XORBytes(block, block, tweak[:])
		nextTweak(&tweak)
	}
}

// nextTweak — le tweak du bloc suivant : multiplié par α dans GF(2¹²⁸), lu en
// petit-boutiste comme le veut la norme.
func nextTweak(tweak *[16]byte) {
	carry := tweak[15] >> 7
	for index := 15; index > 0; index-- {
		tweak[index] = tweak[index]<<1 | tweak[index-1]>>7
	}
	tweak[0] <<= 1
	if carry != 0 {
		tweak[0] ^= 0x87
	}
}

var switchKeys = struct {
	sync.Mutex
	byFile map[string][]byte
}{byFile: map[string][]byte{}}

// switchHeaderKey — la `header_key` des clés de la console : celles d'une console
// dont le registre exige un `prod.keys` (la Switch, la Switch 2), sous la racine
// des BIOS. nil quand aucune n'en a. ELLE NE S'ÉCRIT NULLE PART : ni au journal,
// ni dans une erreur.
func switchHeaderKey() []byte {
	biosRoot.Lock()
	find := biosRoot.find
	biosRoot.Unlock()
	if find == nil {
		return nil
	}
	root := find()
	if root == "" {
		return nil
	}
	for _, console := range consoles.All() {
		if !slices.Contains(console.Bios, switchKeysFile) {
			continue
		}
		if key := headerKeyIn(filepath.Join(root, consoles.BiosDir(console.ID), switchKeysFile)); key != nil {
			return key
		}
	}

	return nil
}

// headerKeyIn — la `header_key` d'un `prod.keys` (des lignes `nom = hexadécimal`),
// gardée par fichier, taille et date.
func headerKeyIn(path string) []byte {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	stamp := fileStamp(path, info)
	switchKeys.Lock()
	key, found := switchKeys.byFile[stamp]
	switchKeys.Unlock()
	if found {
		return key
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "header_key") {
			continue
		}
		if decoded, err := hex.DecodeString(strings.TrimSpace(value)); err == nil && len(decoded) == 32 {
			key = decoded
		}
		break
	}
	switchKeys.Lock()
	switchKeys.byFile[stamp] = key
	switchKeys.Unlock()

	return key
}

// fileStamp — sous quoi se garde ce qu'un fichier dit : son chemin, sa taille, sa
// date. Réécrit, il se relit.
func fileStamp(path string, info os.FileInfo) string {
	return fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
}

// packageFile — un fichier d'une partition : son nom, où il commence dans le
// paquet, sa taille.
type packageFile struct {
	name         string
	offset, size int64
}

// nspFiles — les fichiers d'un paquet : une partition PFS0 dès son début.
func nspFiles(reader io.ReaderAt) ([]packageFile, error) {
	return partitionFiles(reader, 0, "PFS0", 0x18)
}

// xciFiles — les fichiers de la partition `secure` d'une cartouche, là où sont ses
// NCA : son en-tête (« HEAD » à 0x100) donne à 0x130 où commence la partition
// racine, dont `secure` est une entrée (avec `update`, `normal`, `logo`).
func xciFiles(reader io.ReaderAt) ([]packageFile, error) {
	for _, start := range []int64{0, gameCardKeyArea} {
		header := make([]byte, 0x140)
		if _, err := reader.ReadAt(header, start); err != nil {
			return nil, err
		}
		if string(header[0x100:0x104]) != "HEAD" {
			continue
		}
		root, err := partitionFiles(reader, start+int64(binary.LittleEndian.Uint64(header[0x130:0x138])), "HFS0", 0x40)
		if err != nil {
			return nil, err
		}
		for _, partition := range root {
			if partition.name == "secure" {
				return partitionFiles(reader, partition.offset, "HFS0", 0x40)
			}
		}

		return nil, errNotSwitchPackage
	}

	return nil, errNotSwitchPackage
}

// partitionFiles — les fichiers de la partition PFS0 ou HFS0 qui commence à
// `base` : un en-tête de 0x10 octets (la magie, le nombre de fichiers, la taille
// de la table des noms), une entrée par fichier (0x18 octets en PFS0, 0x40 en
// HFS0 : où il commence après la table des noms, sa taille, où est son nom), la
// table des noms, puis les données.
func partitionFiles(reader io.ReaderAt, base int64, magic string, entrySize int64) ([]packageFile, error) {
	header := make([]byte, 0x10)
	if base < 0 {
		return nil, errNotSwitchPackage
	}
	if _, err := reader.ReadAt(header, base); err != nil {
		return nil, err
	}
	if string(header[:4]) != magic {
		return nil, errNotSwitchPackage
	}
	count := int64(binary.LittleEndian.Uint32(header[4:8]))
	namesSize := int64(binary.LittleEndian.Uint32(header[8:12]))
	if count > partitionMaxFiles || namesSize > partitionMaxNames {
		return nil, errNotSwitchPackage
	}
	table := make([]byte, count*entrySize+namesSize)
	if _, err := reader.ReadAt(table, base+0x10); err != nil {
		return nil, err
	}
	names := table[count*entrySize:]
	dataStart := base + 0x10 + int64(len(table))
	files := make([]packageFile, 0, count)
	for index := int64(0); index < count; index++ {
		entry := table[index*entrySize : (index+1)*entrySize]
		nameAt := int64(binary.LittleEndian.Uint32(entry[16:20]))
		if nameAt >= namesSize {
			return nil, errNotSwitchPackage
		}
		name := names[nameAt:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		files = append(files, packageFile{
			name:   string(name),
			offset: dataStart + int64(binary.LittleEndian.Uint64(entry[0:8])),
			size:   int64(binary.LittleEndian.Uint64(entry[8:16])),
		})
	}

	return files, nil
}
