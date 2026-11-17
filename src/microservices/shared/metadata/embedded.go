// Package metadata — les images et les informations d'un jeu (chantier 49,
// #659), rangées dans `metadata/<console>/<gameid>/` du dossier du serveur.
//
// Premier morceau : la source `embedded`, ce que le jeu porte lui-même. Les
// jeux PS3 rangés en `.iso` ont les leurs dans `PS3_GAME/` de l'image ; les
// jeux PS4 et PS5 rangés en dossier, dans `sce_sys/` : une
// icône carrée (`icon0.png`, 512 px), un ou deux fonds 16:9 (`pic0.png`,
// `pic1.png`, jusqu'à 3840 px de large) et parfois un calque transparent
// (`pic2.png`). Elles sont réduites à la demande, à quelques largeurs fixes,
// et gardées dans `metadata/<console>/<gameid>/embedded/` : un fond 4K pèse
// près de 10 Mo, une rangée de vignettes n'en veut que 256 px.
//
// Les autres sources sont à côté : libretro-thumbnails (libretro.go), IGDB et
// SteamGridDB (remote.go), et le texte de la fiche (info.go, la
// libretro-database). Pas encore : un `metadata.json` écrit, les `.pkg`,
// `.zar` et `.dds` seuls — un jeu qui n'a pas de PNG lisible n'a pas
// d'image, plutôt qu'une fausse.
package metadata

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gameflex/storage"
)

// Les noms d'images sont fixes (chantier 49, §2).
const (
	KindIcon       = "icon"
	KindBackground = "background"
	KindLogo       = "logo"
	KindCover      = "cover"
	KindScreenshot = "screenshot"
	KindTitle      = "title"
)

// embeddedFiles — d'où vient chaque image, dans l'ordre de préférence.
// `pic1` d'abord pour le fond : sur PS4 c'est souvent le seul ; sur PS5,
// `pic0` et `pic1` sont deux versions du même visuel (regardés le
// 2026-11-12 sur les 22 jeux PS4 et PS5 de la dev).
//
// PAS DE LOGO : `pic2` n'en est pas un. C'est un calque plein cadre que la
// PS5 pose sur le fond — vide sur Astro Bot, le logo ET les personnages sur
// Spider-Man 2. Le servir comme logo poserait des personnages à la place
// d'un titre ; le logo viendra des autres sources du chantier.
var embeddedFiles = map[string][]string{
	KindIcon:       {"icon0.png"},
	KindBackground: {"pic1.png", "pic0.png"},
}

// isoFiles — la même chose dans `PS3_GAME/` d'une image PS3 : `ICON0.PNG`
// (320×176, le format des icônes PS3) et `PIC1.PNG` (1920×1080). `PIC0` y est
// un calque transparent de 1000×560, pas un fond (8 des 9 jeux PS3 de la dev
// ont un PIC1, le 2026-11-12).
var isoFiles = map[string][]string{
	KindIcon:       {"PS3_GAME/ICON0.PNG"},
	KindBackground: {"PS3_GAME/PIC1.PNG"},
}

// Widths — les largeurs servies. Une largeur demandée prend la première qui
// la couvre ; jamais plus que l'image d'origine. Peu de largeurs, pour que le
// cache reste petit.
var Widths = []int{256, 512, 1280, 1920}

// Embedded — le fichier de l'image `kind` du jeu rangé en `gamePath`, à la
// largeur `width` (0 : la plus grande), réduite et gardée sous
// `metadataRoot/<console>/<gameid>/embedded/` au premier appel.
// os.ErrNotExist quand le jeu n'a pas d'identifiant lisible ou pas cette
// image.
func Embedded(metadataRoot, consoleID, gamePath, kind string, width int) (string, error) {
	if kind != KindIcon && kind != KindBackground {
		return "", os.ErrNotExist
	}
	gameID := storage.GameTitleID(gamePath)
	if gameID == "" {
		return "", os.ErrNotExist
	}
	sources, err := embeddedSources(gamePath, kind)
	if err != nil {
		return "", err
	}

	width = widthFor(width)
	extension := ".png"
	if kind == KindBackground {
		extension = ".jpg"
	}
	target := filepath.Join(metadataRoot, consoleID, gameID, "embedded", fmt.Sprintf("%s-%d%s", kind, width, extension))

	lock := lockFor(target)
	lock.Lock()
	defer lock.Unlock()

	for _, source := range sources {
		if targetInfo, err := os.Stat(target); err == nil && !targetInfo.ModTime().Before(source.modified) {
			return target, nil
		}
		content, err := source.read()
		if err != nil {
			continue
		}
		picture, err := decodeImage(content)
		if err != nil {
			continue
		}
		if err := writeReduced(target, picture, width, extension); err != nil {
			return "", err
		}

		return target, nil
	}

	return "", os.ErrNotExist
}

// embeddedSource — une image que porte le jeu : sa date, pour savoir si la
// réduction gardée est à jour, et de quoi la lire.
type embeddedSource struct {
	modified time.Time
	read     func() ([]byte, error)
}

// embeddedSources — les images `kind` présentes, dans l'ordre de préférence :
// dans l'image `.iso` d'un jeu PS3, sinon dans le `sce_sys/` du dossier.
func embeddedSources(gamePath, kind string) ([]embeddedSource, error) {
	if image := storage.GameISO(gamePath); image != "" {
		info, err := os.Stat(image)
		if err != nil {
			return nil, err
		}
		sources := []embeddedSource{}
		for _, name := range isoFiles[kind] {
			sources = append(sources, embeddedSource{modified: info.ModTime(), read: func() ([]byte, error) {
				return storage.ISOFile(image, name)
			}})
		}

		return sources, nil
	}

	gameDir := gamePath
	if info, err := os.Stat(gamePath); err != nil {
		return nil, err
	} else if !info.IsDir() {
		gameDir = filepath.Dir(gamePath)
	}
	sources := []embeddedSource{}
	for _, name := range embeddedFiles[kind] {
		path := filepath.Join(gameDir, "sce_sys", name)
		if info, err := os.Stat(path); err == nil {
			sources = append(sources, embeddedSource{modified: info.ModTime(), read: func() ([]byte, error) {
				return os.ReadFile(path)
			}})
		}
	}

	return sources, nil
}

func widthFor(requested int) int {
	for _, width := range Widths {
		if requested > 0 && requested <= width {
			return width
		}
	}

	return Widths[len(Widths)-1]
}

var (
	locksMutex sync.Mutex
	locks      = map[string]*sync.Mutex{}
)

// lockFor — un verrou par fichier produit : deux vignettes demandées en même
// temps ne réduisent pas deux fois le même fond 4K.
func lockFor(path string) *sync.Mutex {
	locksMutex.Lock()
	defer locksMutex.Unlock()
	lock, found := locks[path]
	if !found {
		lock = &sync.Mutex{}
		locks[path] = lock
	}

	return lock
}

// decodeImage — un PNG ou un JPEG, ramené en RGBA.
func decodeImage(content []byte) (*image.RGBA, error) {
	decoded, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	picture := image.NewRGBA(decoded.Bounds())
	draw.Draw(picture, picture.Bounds(), decoded, decoded.Bounds().Min, draw.Src)

	return picture, nil
}

// writeReduced — l'image ramenée à `width` (jamais agrandie), écrite à côté
// puis renommée : un lecteur ne voit jamais un fichier à moitié écrit.
func writeReduced(target string, picture *image.RGBA, width int, extension string) error {
	if err := makeOwnedDir(filepath.Dir(target)); err != nil {
		return err
	}
	reduced := reduce(picture, width)
	temporary, err := os.CreateTemp(filepath.Dir(target), ".embedded-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if extension == ".jpg" {
		err = jpeg.Encode(temporary, reduced, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(temporary, reduced)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	return renameOwned(temporary.Name(), target)
}

// makeOwnedDir — le dossier, créé s'il manque ; chaque dossier créé prend le
// propriétaire de son parent (storage.AdoptParentOwner) : sur un disque
// interne, celui du disque, et pas root (les métadonnées d'un jeu de disque
// vivent sur le disque, chantier 49).
func makeOwnedDir(directory string) error {
	missing := []string{}
	for current := directory; ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil || current == filepath.Dir(current) {
			break
		}
		missing = append(missing, current)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		storage.AdoptParentOwner(missing[index])
	}

	return nil
}

// renameOwned — le fichier écrit à côté prend sa place, lisible par tous
// (un fichier temporaire naît en 0600) et au propriétaire de son dossier.
func renameOwned(temporary, target string) error {
	if err := os.Chmod(temporary, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	storage.AdoptParentOwner(target)

	return nil
}

// reduce — une moyenne par zone : chaque pixel d'arrivée est la moyenne des
// pixels de départ qu'il recouvre. Les couleurs sont prémultipliées
// (image.RGBA) : un bord transparent ne noircit pas.
func reduce(picture *image.RGBA, width int) *image.RGBA {
	bounds := picture.Bounds()
	sourceWidth, sourceHeight := bounds.Dx(), bounds.Dy()
	if width >= sourceWidth {
		return picture
	}
	height := max(1, sourceHeight*width/sourceWidth)
	reduced := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		top, bottom := y*sourceHeight/height, max((y+1)*sourceHeight/height, y*sourceHeight/height+1)
		for x := 0; x < width; x++ {
			left, right := x*sourceWidth/width, max((x+1)*sourceWidth/width, x*sourceWidth/width+1)
			var sum [4]int
			for sourceY := top; sourceY < bottom; sourceY++ {
				offset := (sourceY)*picture.Stride + left*4
				for sourceX := left; sourceX < right; sourceX++ {
					sum[0] += int(picture.Pix[offset])
					sum[1] += int(picture.Pix[offset+1])
					sum[2] += int(picture.Pix[offset+2])
					sum[3] += int(picture.Pix[offset+3])
					offset += 4
				}
			}
			count := (bottom - top) * (right - left)
			index := y*reduced.Stride + x*4
			for channel := 0; channel < 4; channel++ {
				reduced.Pix[index+channel] = uint8(sum[channel] / count)
			}
		}
	}

	return reduced
}
