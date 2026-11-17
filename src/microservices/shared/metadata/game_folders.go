package metadata

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// LES DOSSIERS DES JEUX D'UNE CONSOLE, sous `<racine>/metadata/<console>/` — pour trouver les métadonnées qui n'ont plus
// de jeu (le porteur, 2026-11-14 : « on peut se retrouver avec des metadata des jeux qu'on n'a plus accès. Ce qui fera
// des métadata orphelin. […] Cette page listera que les metadata qui non pas jeux et quand on clique dessus ça propose
// de le supprimer avec confirmation. »). Le kernel décide de ce qui est orphelin (il connaît les jeux accessibles) ; ce
// paquet ne fait que lister, décrire et retirer un dossier de jeu, jamais un autre.

// ErrNotAGameFolder — un nom qui ne désigne pas le dossier d'un jeu : vide, caché, un chemin, ou un dossier par console
// (le fond de sa bande, ses images à elle).
var ErrNotAGameFolder = errors.New("not a game metadata folder")

// consoleFolderNames — les dossiers PAR CONSOLE rangés à côté de ceux des jeux, pris là où ils se nomment
// (ConsoleBackgroundDir, consoleDir) et non de mémoire : jamais listés, jamais retirés.
func consoleFolderNames() map[string]bool {
	return map[string]bool{
		filepath.Base(ConsoleBackgroundDir("", "console")): true,
		filepath.Base(consoleDir("", "console")):           true,
	}
}

// IsGameFolderName — `name` peut être le dossier d'un jeu : un nom simple (ni `..`, ni séparateur, ni caché), qui
// n'est pas un dossier par console.
func IsGameFolderName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\`) &&
		filepath.Base(name) == name && !consoleFolderNames()[name]
}

// GameFolder — le dossier des métadonnées d'un jeu : son nom (le `<gameid>`), le titre qu'il garde (la correction,
// sinon le nom trouvé chez un service ; vide s'il n'en a pas) et le fichier de sa jaquette s'il en a une.
type GameFolder struct {
	GameID string
	Title  string
	Cover  string
}

// GameFolders — les dossiers de jeux de la console sous `root`, triés ; aucun s'il n'y a pas de dossier.
func GameFolders(root, consoleID string) ([]GameFolder, error) {
	entries, err := os.ReadDir(filepath.Join(root, "metadata", consoleID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	folders := []GameFolder{}
	for _, entry := range entries {
		if !entry.IsDir() || !IsGameFolderName(entry.Name()) {
			continue
		}
		folders = append(folders, describeGameFolder(filepath.Join(root, "metadata", consoleID, entry.Name()), entry.Name()))
	}
	sort.Slice(folders, func(left, right int) bool { return folders[left].GameID < folders[right].GameID })

	return folders, nil
}

// GameFolderAt — le dossier `gameID` de la console sous `root`. os.ErrNotExist s'il n'y est pas.
func GameFolderAt(root, consoleID, gameID string) (GameFolder, error) {
	if !IsGameFolderName(gameID) {
		return GameFolder{}, ErrNotAGameFolder
	}
	dir := filepath.Join(root, "metadata", consoleID, gameID)
	if info, err := os.Stat(dir); err != nil {
		return GameFolder{}, err
	} else if !info.IsDir() {
		return GameFolder{}, os.ErrNotExist
	}

	return describeGameFolder(dir, gameID), nil
}

// RemoveGameFolder — retire le dossier `gameID` de la console sous `root`, tout ce qu'il garde. Le kernel a vérifié,
// juste avant, qu'aucun jeu accessible ne le porte.
func RemoveGameFolder(root, consoleID, gameID string) error {
	if _, err := GameFolderAt(root, consoleID, gameID); err != nil {
		return err
	}
	dir := filepath.Join(root, "metadata", consoleID, gameID)
	lock := lockFor(dir)
	lock.Lock()
	defer lock.Unlock()

	return os.RemoveAll(dir)
}

// ErrGameFolderTaken — le dossier qu'un jeu devrait rejoindre existe déjà : rien ne s'écrase.
var ErrGameFolderTaken = errors.New("a metadata folder already carries this game id")

// MoveReferenceFolder — LE DOSSIER QU'UN JEU AVAIT SOUS SA RÉFÉRENCE le suit sous son gameid (2026-11-15). Un jeu dont
// l'identifiant ne se lisait pas avait son dossier sous sa référence (GameKey, son repli) ; il se lit désormais — un
// jeu Switch dans son paquet (storage.GameTitleID : « Crash Bandicoot.xci » est 0100D1B006744000) — et ses métadonnées
// le suivent, au lieu de se refaire à côté : `metadata/<console>/<référence>/` devient
// `metadata/<console>/<gameid>/` sous `root`, d'un renommage (le même disque). Vrai quand il a été déplacé ; faux sans
// rien faire quand il n'y a rien à déplacer. ErrGameFolderTaken quand un dossier porte déjà ce gameid : les deux
// restent tels quels, et celui de la référence, que plus aucun jeu ne porte, paraît dans « Métadonnées sans jeu »
// (api/metadata_orphans.go), où l'administrateur voit sa jaquette et le retire.
func MoveReferenceFolder(root, consoleID, reference, gameID string) (bool, error) {
	if reference == gameID {
		return false, nil
	}
	if !IsGameFolderName(reference) || !IsGameFolderName(gameID) {
		return false, ErrNotAGameFolder
	}
	from := filepath.Join(root, "metadata", consoleID, reference)
	to := filepath.Join(root, "metadata", consoleID, gameID)
	// Toujours la référence d'abord : aucun geste ne prend ces deux verrous dans l'autre ordre.
	fromLock, toLock := lockFor(from), lockFor(to)
	fromLock.Lock()
	defer fromLock.Unlock()
	toLock.Lock()
	defer toLock.Unlock()

	if info, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir()) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := os.Lstat(to); err == nil {
		return false, ErrGameFolderTaken
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(from, to); err != nil {
		return false, err
	}

	return true, nil
}

// LES JEUX DÉJÀ SUIVIS, par racine, console, référence et gameid (un *sync.Once chacun) : un jeu ne se vérifie qu'une
// fois par vie du kernel, à son premier accès — une grille de jaquettes demande son dossier à chaque image.
var followed sync.Map

// followReference — MoveReferenceFolder au premier accès d'un jeu à ses métadonnées (gameDirectory), une seule fois.
// LES DEMANDES DU MÊME JEU QUI CROISENT ATTENDENT LE DÉPLACEMENT : sa jaquette, son fond et son logo partent ensemble,
// et la première qui créerait le dossier du gameid avant lui le ferait refuser (ErrGameFolderTaken). Ce qui a été fait
// se lit au journal ; un refus ne se retente pas à chaque image (un disque en lecture seule le referait sans fin) : il
// attend le prochain démarrage, ou la page des métadonnées sans jeu.
func followReference(root, consoleID, reference, gameID string) {
	if reference == gameID {
		return
	}
	once, _ := followed.LoadOrStore(root+"\x00"+consoleID+"\x00"+reference+"\x00"+gameID, &sync.Once{})
	once.(*sync.Once).Do(func() {
		moved, err := MoveReferenceFolder(root, consoleID, reference, gameID)
		switch {
		case moved:
			slog.Info("game metadata moved to the game id", "root", root, "console", consoleID, "from", reference, "to", gameID)
		case errors.Is(err, ErrGameFolderTaken):
			slog.Warn("game metadata left under the reference: the game id already has its folder", "root", root,
				"console", consoleID, "reference", reference, "gameId", gameID)
		case err != nil:
			slog.Warn("game metadata not moved to the game id", "root", root, "console", consoleID, "from", reference,
				"to", gameID, "error", err)
		}
	})
}

// describeGameFolder — le titre et la jaquette qu'un dossier garde : la correction d'abord, puis la fiche copiée d'un
// serveur fédéré, IGDB, SteamGridDB et libretro, dans l'ordre de la fiche.
func describeGameFolder(dir, gameID string) GameFolder {
	folder := GameFolder{GameID: gameID, Title: readManualState(dir).Title, Cover: manualImage(dir, KindCover)}
	copied := readServerCopy(dir)
	folder.Title = firstOf(folder.Title, copied.Title, copied.Name)
	if folder.Cover == "" {
		folder.Cover = serverImage(dir, KindCover)
	}
	note := func(title, cover string) {
		if folder.Title == "" {
			folder.Title = title
		}
		if folder.Cover == "" && cover != "" {
			if _, err := os.Stat(cover); err == nil {
				folder.Cover = cover
			}
		}
	}
	for _, service := range []remoteService{igdbService{}, steamGridDBService{}} {
		sourceDir := filepath.Join(dir, service.name())
		state := readRemoteState(sourceDir, service.name())
		cover := ""
		if file := state.Images[KindCover]; file != "" {
			cover = filepath.Join(sourceDir, file)
		}
		note(state.Name, cover)
	}
	if image, found := readLibretroSource(filepath.Join(dir, sourceLibretro)).Images[KindCover]; found && image.File != "" {
		note(image.Name, filepath.Join(dir, sourceLibretro, image.File))
	}

	return folder
}
