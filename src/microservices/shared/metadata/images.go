package metadata

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gameflex/consoles"
	"gameflex/storage"
)

// ErrUnknownKind — une image qui n'a pas de nom connu.
var ErrUnknownKind = errors.New("unknown image kind")

// Les sources dans l'ordre du § 3 du plan, par sorte d'image, après
// l'image déposée par l'administrateur (`manual`), toujours première, et la
// fiche copiée d'un serveur fédéré (`server`, chantier 57). Le rétro passe
// par libretro d'abord (ses noms No-Intro y sont exacts), les consoles
// récentes par ce que porte le jeu, puis IGDB et SteamGridDB.
const (
	sourceEmbedded    = "embedded"
	sourceLibretro    = "libretro"
	sourceIGDB        = "igdb"
	sourceSteamGridDB = "steamgriddb"
)

var (
	retroOrder = map[string][]string{
		KindIcon:       {sourceEmbedded},
		KindBackground: {sourceEmbedded, sourceIGDB, sourceSteamGridDB},
		KindCover:      {sourceLibretro, sourceIGDB, sourceSteamGridDB},
		KindScreenshot: {sourceLibretro, sourceIGDB},
		KindTitle:      {sourceLibretro},
		KindLogo:       {sourceLibretro, sourceSteamGridDB},
	}
	recentOrder = map[string][]string{
		KindIcon:       {sourceEmbedded},
		KindBackground: {sourceEmbedded, sourceIGDB, sourceSteamGridDB},
		KindCover:      {sourceIGDB, sourceSteamGridDB, sourceLibretro},
		KindScreenshot: {sourceIGDB, sourceLibretro},
		KindTitle:      {sourceLibretro},
		KindLogo:       {sourceSteamGridDB, sourceLibretro},
	}
)

// Roots — où vivent les données d'un jeu (chantier 49, décision du porteur
// du 2026-11-13, « avec les jeux du disque ») :
//
//   - Game : la racine qui porte `metadata/<console>/<gameid>/`, où les
//     données du jeu se LISENT ET S'ÉCRIVENT — une seule : le dossier du
//     serveur pour un jeu de la bibliothèque ; pour un jeu d'un disque
//     interne, la racine de ce disque quand l'interrupteur « Écrire les
//     métadonnées sur le disque du jeu » est allumé (c'est alors la seule
//     écriture de GameFlex sur un disque interne), le dossier du serveur
//     quand il est coupé (un disque en lecture seule, NTFS branché au
//     téléphone). Le dossier d'un jeu se nomme par son contenu (GameKey) :
//     s'il existe déjà sous cette racine — le même jeu déjà connu —, il sert
//     tel quel, sinon il se crée (le porteur, 2026-11-14 : « S'il arrive qu'un
//     jeu qui se trouve sur le disque externe existe déjà sur le serveur
//     principal alors on utilise le même metadata »). Le kernel la choisit en
//     UN point (api/game_metadata.go, gameMetadataRoots) ;
//   - Server : le dossier du serveur, qui garde ce qui n'est pas d'un jeu —
//     les caches (`cache/metadata/` : l'index de libretro-thumbnails, les
//     `.dat` de la libretro-database) et ce qui est par console
//     (`metadata/<console>/background/`, `_console/`).
type Roots struct {
	Server string
	Game   string
}

// Image — le fichier de l'image `kind` d'un jeu, à la largeur `width`, de la
// première source qui l'a. `roots` : où vivent ses métadonnées et les caches. `reference` : la référence du jeu, le `<gameid>`
// de repli d'un jeu sans identifiant ni empreinte. `accounts` : les comptes
// des services (vide : seulement ce que porte le jeu et libretro).
//
// Une source qui échoue (réseau, compte refusé) est notée au journal et la
// suivante est essayée : un service tombé ne prive pas des autres.
func Image(ctx context.Context, roots Roots, accounts Accounts, consoleID, gamePath, reference, kind string, width int) (string, error) {
	return eachImageSource(ctx, roots, accounts, consoleID, gamePath, reference, kind, width, func(source, file string, reduced bool) (string, error) {
		// Ce que porte le jeu est déjà réduit ; une image déposée ou copiée en WebP sert telle quelle.
		if reduced || ((source == sourceManual || source == sourceServer) && strings.EqualFold(filepath.Ext(file), ".webp")) {
			return file, nil
		}

		return reducedCopy(file, kind, width)
	})
}

// ImageSize — la taille de l'image `kind` d'un jeu, telle que sa première source la garde (ce que porte le jeu : à la
// plus grande largeur, 1920 px) : ce qu'un pair dit d'une de ses images à qui la cherche (SearchLent). os.ErrNotExist
// quand aucune source ne l'a ; une image WebP existe, de taille inconnue (0 × 0).
func ImageSize(ctx context.Context, roots Roots, accounts Accounts, consoleID, gamePath, reference, kind string) (width, height int, err error) {
	_, err = eachImageSource(ctx, roots, accounts, consoleID, gamePath, reference, kind, 0, func(source, file string, reduced bool) (string, error) {
		if strings.EqualFold(filepath.Ext(file), ".webp") {
			width, height = 0, 0

			return file, nil
		}
		opened, err := os.Open(file)
		if err != nil {
			return "", err
		}
		defer opened.Close()
		config, _, err := image.DecodeConfig(opened)
		if err != nil {
			return "", fmt.Errorf("%s: %w", file, err)
		}
		width, height = config.Width, config.Height

		return file, nil
	})

	return width, height, err
}

// eachImageSource — l'image `kind` d'un jeu, source après source : celle que l'administrateur a choisie (`manual`),
// toujours première ; la fiche copiée d'un serveur fédéré (`server`, chantier 57 : un jeu désigné chez lui, ou
// téléchargé de chez lui), qui passe devant les services ; puis les sources de l'ordre du § 3. `use` reçoit le fichier
// de la première qui l'a (`reduced` : déjà à la largeur `width`, ce que porte le jeu) et rend ce que rend la fonction ;
// son erreur est notée au journal, et la source suivante est essayée.
func eachImageSource(ctx context.Context, roots Roots, accounts Accounts, consoleID, gamePath, reference, kind string, width int,
	use func(source, file string, reduced bool) (string, error)) (string, error) {
	order := recentOrder
	if consoles.MethodOf(consoleID) == consoles.MethodRetroGaming {
		order = retroOrder
	}
	sources, known := order[kind]
	if !known {
		return "", ErrUnknownKind
	}

	metadataRoot := filepath.Join(roots.Game, "metadata")
	gameDir := gameDirectory(roots.Game, consoleID, gamePath, reference)
	name := GameName(gamePath)
	try := func(source, file string, reduced bool) (string, bool) {
		path, err := use(source, file, reduced)
		if err != nil {
			slog.Warn("game image", "source", source, "console", consoleID, "game", name, "error", err)

			return "", false
		}

		return path, true
	}

	if original := manualImage(gameDir, kind); original != "" {
		if path, ok := try(sourceManual, original, false); ok {
			return path, nil
		}
	}
	if copied := serverImage(gameDir, kind); copied != "" {
		if path, ok := try(sourceServer, copied, false); ok {
			return path, nil
		}
	}
	for _, source := range sources {
		var original string
		var err error
		reduced := false
		switch source {
		case sourceEmbedded:
			original, err = Embedded(metadataRoot, consoleID, gamePath, kind, width)
			reduced = true
		case sourceLibretro:
			original, err = Libretro(ctx, filepath.Join(roots.Server, "cache", "metadata", "sources"), gameDir, consoleID, name, kind)
		case sourceIGDB:
			original, err = Remote(ctx, igdbService{}, accounts, gameDir, consoleID, searchTitle(name), kind)
		case sourceSteamGridDB:
			original, err = Remote(ctx, steamGridDBService{}, accounts, gameDir, consoleID, searchTitle(name), kind)
		}
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn("game image", "source", source, "console", consoleID, "game", name, "error", err)
			}
			continue
		}
		if path, ok := try(source, original, reduced); ok {
			return path, nil
		}
	}

	return "", os.ErrNotExist
}

// IsImageKind — `kind` est une sorte d'image d'un jeu (cover, background, logo, title, screenshot, icon).
func IsImageKind(kind string) bool {
	return validImageKind(kind)
}

// reducedCopy — l'image d'origine ramenée à la largeur demandée, gardée à côté
// d'elle (`cover-512.jpg`) et refaite seulement si l'origine change.
func reducedCopy(original, kind string, width int) (string, error) {
	extension := ".jpg"
	if kind == KindLogo {
		extension = ".png"
	}
	width = widthFor(width)
	target := filepath.Join(filepath.Dir(original), fmt.Sprintf("%s-%d%s", kind, width, extension))

	lock := lockFor(target)
	lock.Lock()
	defer lock.Unlock()
	originalInfo, err := os.Stat(original)
	if err != nil {
		return "", err
	}
	if targetInfo, err := os.Stat(target); err == nil && !targetInfo.ModTime().Before(originalInfo.ModTime()) {
		return target, nil
	}
	content, err := os.ReadFile(original)
	if err != nil {
		return "", err
	}
	picture, err := decodeImage(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", original, err)
	}
	if err := writeReduced(target, picture, width, extension); err != nil {
		return "", err
	}

	return target, nil
}

// GameName — le nom d'un jeu tel que les bases le connaissent : celui de son
// image `.iso` quand il est rangé dans un dossier (« Final Fantasy XIII
// (Europe) (En,Fr,De,Es,It) », virgules comprises), sinon son fichier ou son
// dossier, sans extension.
func GameName(gamePath string) string {
	if image := storage.GameISO(gamePath); image != "" {
		return strings.TrimSuffix(filepath.Base(image), filepath.Ext(image))
	}
	base := filepath.Base(gamePath)
	if info, err := os.Stat(gamePath); err == nil && info.IsDir() {
		return base
	}

	return strings.TrimSuffix(base, filepath.Ext(base))
}
