package api

// UN JEU D'UN PARTAGE MONTÉ, tel qu'on va le copier dans la bibliothèque.
//
// Un partage (WebDAV, kDrive, Samba) n'est plus une source qu'on joue : depuis
// le chantier 14 (2026-10-12), ses jeux ne s'affichent qu'à l'import, dans
// Réglages › Jeux, et c'est la copie dans la bibliothèque qui les rend
// jouables (`handleMountImport`). Ce fichier tenait aussi la mise en CACHE d'un
// jeu distant le temps d'y jouer — le mode « hybride » —, retirée avec lui :
// « mettre en cache dans gameflex/cache/roms c'est exactement comme mettre dans
// gameflex/roms » (le porteur).

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gameflex/storage"
	"gameflex/storage/listing"
)

// mountedGame — un jeu d'un partage tel qu'on va le copier : son nom
// d'arrivée, et ses fichiers.
type mountedGame struct {
	// name est le nom d'arrivée : celui du dossier quand le jeu en est un.
	name string
	// folder dit si ces fichiers vont dans un dossier de ce nom, ou à plat.
	folder bool
	files  []mountedFile
}

type mountedFile struct {
	// relative est le chemin DANS le jeu — il porte les sous-dossiers.
	relative string
	// size est ce que le partage annonce : de quoi comparer une fois la
	// copie faite, plutôt que de croire qu'elle a abouti.
	size int64
	open func() (io.ReadCloser, error)
}

// mountGame — le jeu tel qu'il vit sur un partage, avec TOUS ses fichiers.
func mountGame(mount mountedShare, consoleID, path string) (*mountedGame, error) {
	source, err := cachedSourceFor(mount)
	if err != nil {
		return nil, err
	}
	opener, canOpen := source.(storage.Opener)
	if !canOpen {
		return nil, errors.New("this share cannot serve files")
	}

	// LE CATALOGUE D'ABORD, le partage ensuite.
	//
	// Le parcours a déjà noté, pour chaque jeu, s'il occupe un dossier et ce
	// qu'il pèse. Le redemander coûtait un listing du dossier parent — celui
	// de la NES en compte 10 783, paginées — avant même le premier octet.
	//
	// Dans les deux cas la réponse vient de NOUS, jamais de la requête : un
	// « dossier » choisi par le demandeur désignerait n'importe quel coin du
	// partage (voir storage.GameFolderOf).
	item, known := itemFromListing(mount, consoleID, path)
	if known && item.Folder == "" {
		name := filepath.Base(path)
		_, inside, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")

		return &mountedGame{
			name: name,
			files: []mountedFile{{
				relative: name,
				size:     item.Size,
				open:     func() (io.ReadCloser, error) { return opener.Open(inside) },
			}},
		}, nil
	}

	folder := item.Folder
	if !known {
		got, err := storage.GameFolderOf(source, consoleID, path)
		if err != nil {
			return nil, err
		}
		folder = got
	}
	files, err := storage.GameFiles(source, path, folder)
	if err != nil {
		return nil, err
	}

	name := filepath.Base(path)
	if folder != "" {
		name = folder[strings.LastIndex(folder, "/")+1:]
	}

	game := &mountedGame{name: name, folder: folder != ""}
	for _, file := range files {
		// Le chemin porte le nom du montage en tête (le scanner l'y met) ;
		// la source, elle, part de sa propre racine.
		_, inside, _ := strings.Cut(strings.TrimPrefix(file.Path, "/"), "/")
		game.files = append(game.files, mountedFile{
			relative: file.Name,
			size:     file.Size,
			open:     func() (io.ReadCloser, error) { return opener.Open(inside) },
		})
	}

	return game, nil
}

// importGameFile écrit un fichier de jeu dans la BIBLIOTHÈQUE (#157), et le
// dit au journal.
//
// L'écriture elle-même (`writeGameFile`) adopte le propriétaire du dossier :
// sans elle, ce chemin ré-ouvrirait #146 en posant des fichiers `root:root`
// dans la bibliothèque.
func importGameFile(open func() (io.ReadCloser, error), target string, expected int64) error {
	started := time.Now()
	written, err := writeGameFile(open, target, expected)
	if err != nil {
		return err
	}

	slog.Info("game imported into the library",
		"path", target, "bytes", written, "took", time.Since(started).String())

	return nil
}

// writeGameFile — l'écriture atomique d'un fichier de jeu.
//
// Fichier temporaire puis renommage : une coupure laisserait sinon un jeu
// tronqué que le lancement suivant croirait valide, et qui échouerait sans dire
// pourquoi — la leçon du téléchargeur de cores.
//
// Et ce qu'on crée appartient au propriétaire du dossier parent (#146) : le
// noyau tourne en root, donc sans ça la personne ne pourrait plus gérer ce
// qu'on vient de déposer chez elle.
func writeGameFile(
	open func() (io.ReadCloser, error), target string, expected int64,
) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	adoptParentOwner(filepath.Dir(target))

	body, err := open()
	if err != nil {
		return 0, err
	}
	defer body.Close()

	partial := target + ".part"
	file, err := os.Create(partial)
	if err != nil {
		return 0, err
	}

	written, err := io.Copy(file, body)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(partial)

		return 0, fmt.Errorf("copying %s: %w", filepath.Base(target), err)
	}

	// CE QU'ON A REÇU EST-IL CE QU'ON ATTENDAIT ? Le fichier n'est rangé que si
	// oui : un jeu tronqué qui porte le nom du vrai est pire qu'un jeu absent —
	// il échoue plus tard, ailleurs, sans rien dire.
	if expected > 0 && written != expected {
		_ = os.Remove(partial)

		return 0, fmt.Errorf("%s : %d octets reçus sur %d attendus",
			filepath.Base(target), written, expected)
	}

	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)

		return 0, err
	}
	adoptParentOwner(target)

	return written, nil
}

// itemFromListing — ce que le dernier parcours avait noté de ce jeu.
//
// Faux en second retour quand le cache ne le connaît pas (ressource jamais
// parcourue, jeu apparu depuis, ou cache d'une version antérieure —
// listing.LoadSection refuse alors de le lire). L'appelant redemande au
// partage lui-même.
func itemFromListing(mount mountedShare, consoleID, reference string) (listing.Item, bool) {
	items, err := listing.LoadSection(listingCacheDir, resourceKey(mount), consoleID, "roms")
	if err != nil {
		return listing.Item{}, false
	}
	for _, item := range items {
		if item.Path == reference {
			return item, true
		}
	}

	return listing.Item{}, false
}
