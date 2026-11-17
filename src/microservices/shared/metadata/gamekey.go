package metadata

import (
	"archive/zip"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gameflex/storage"
)

// crcSizeLimit — au-delà, on ne lit pas tout un fichier pour son empreinte :
// une ROM rétro tient en quelques Mo, une image de disque PS2 en plusieurs Go.
const crcSizeLimit = 64 << 20

// GameKey — le `<gameid>` d'un jeu, son dossier sous `metadata/<console>/`,
// lu dans son contenu et non dans son nom (chantier 49, §1) :
//   - l'identifiant de titre d'un jeu PlayStation (CUSA07010, BLES01269) ;
//   - sinon l'empreinte CRC32 de la ROM, en « crc-1A2B3C4D » — celle que
//     note le répertoire central d'un `.zip` (rien à décompresser), ou celle
//     du fichier s'il est petit ;
//   - sinon `fallback` (la référence du jeu, tirée de son nom) : un jeu trop
//     gros pour son empreinte et sans identifiant garde quand même un dossier.
//
// Gardée en mémoire par fichier, taille et date : une grille de jaquettes la
// demande à chaque image.
func GameKey(gamePath, fallback string) string {
	info, err := os.Stat(gamePath)
	if err != nil {
		return fallback
	}
	cacheKey := gameKeyCacheKey(gamePath, info)
	keysMutex.Lock()
	key, found := keys[cacheKey]
	keysMutex.Unlock()
	if found {
		return key
	}

	key = storage.GameTitleID(gamePath)
	if key == "" && !info.IsDir() {
		if sum, ok := romCRC(gamePath, info.Size()); ok {
			key = fmt.Sprintf("crc-%08X", sum)
		}
	}
	if key == "" {
		key = fallback
	}
	keysMutex.Lock()
	keys[cacheKey] = key
	keysMutex.Unlock()

	return key
}

// KnownGameKey — le `<gameid>` que GameKey rendrait, s'il est DÉJÀ en mémoire
// pour ce fichier tel qu'il est (même taille, même date) : faux quand il
// faudrait lire le jeu pour son empreinte. Rien n'est lu au-delà d'un `stat` —
// toute une bibliothèque se lit en minutes (les 10 785 ROM NES de Gamebox :
// 101 s, chantier 53). Un jeu qui ne se lit plus a son repli, comme chez
// GameKey.
func KnownGameKey(gamePath, fallback string) (string, bool) {
	info, err := os.Stat(gamePath)
	if err != nil {
		return fallback, true
	}
	keysMutex.Lock()
	defer keysMutex.Unlock()
	key, found := keys[gameKeyCacheKey(gamePath, info)]

	return key, found
}

// gameKeyCacheKey — sous quoi un `<gameid>` se garde : le fichier, sa taille,
// sa date. Un jeu réécrit change d'entrée, et son empreinte se relit.
func gameKeyCacheKey(gamePath string, info os.FileInfo) string {
	return fmt.Sprintf("%s|%d|%d", gamePath, info.Size(), info.ModTime().UnixNano())
}

var (
	keysMutex sync.Mutex
	keys      = map[string]string{}
)

// romCRC — l'empreinte d'une ROM : celle de la plus grosse entrée d'un `.zip`
// (lue dans son répertoire central), sinon celle du fichier.
func romCRC(path string, size int64) (uint32, bool) {
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		archive, err := zip.OpenReader(path)
		if err != nil {
			return 0, false
		}
		defer archive.Close()
		var best *zip.File
		for _, entry := range archive.File {
			if entry.FileInfo().IsDir() {
				continue
			}
			if best == nil || entry.UncompressedSize64 > best.UncompressedSize64 {
				best = entry
			}
		}
		if best == nil {
			return 0, false
		}

		return best.CRC32, true
	}
	if size > crcSizeLimit {
		return 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	hash := crc32.NewIEEE()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, false
	}

	return hash.Sum32(), true
}

// timeNow — remplaçable dans un test.
var timeNow = time.Now
