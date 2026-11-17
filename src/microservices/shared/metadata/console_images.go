package metadata

// L'IMAGE D'UNE CONSOLE (chantier 49, #659, pour le launcher #660) — son
// icône, montrée sur son écran du launcher.
//
// Deux sources, dans cet ordre :
//
//   - `manual` : l'image que l'administrateur a déposée dans
//     `metadata/<console>/_console/manual/` (un PNG, un JPEG ou un WebP) ;
//   - `kylebing` : l'icône de la console dans « retro-game-console-icons » de
//     KyleBing (github.com/KyleBing/retro-game-console-icons, CC BY 4.0),
//     choisie ici à la main (consoleIcons), téléchargée une fois depuis
//     raw.githubusercontent.com dans `metadata/<console>/_console/kylebing/`
//     avec son `source.json` (le fichier, le dépôt, la licence, l'adresse,
//     la date). Une fois gardée, elle ne se redemande plus : le serveur la
//     sert hors ligne.
//
// Une seule série pour toutes : `series_trimui/600w@2x`, des PNG de 600×600
// sur fond transparent, dessinés pour un fond sombre. Les consoles récentes
// (streamex, jeu à distance) n'ont pas d'icône tant qu'une source cohérente
// avec celle-ci n'est pas trouvée : 404, et le launcher dessine sans.
//
// CC BY 4.0 demande de citer la collection et de dire ce qui a changé : la
// route de la source rend l'auteur, la licence, le lien et `changes`
// (l'icône est réduite).
//
// `_console` ne peut pas être un `<gameid>` : un identifiant de jeu n'a
// jamais de « _ » en tête (gamekey.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gameflex/consoles"
)

// ErrUnknownConsole — un identifiant qui n'est pas au registre.
var ErrUnknownConsole = errors.New("unknown console")

// ErrUnreadableImage — une image déposée qui ne se décode pas.
var ErrUnreadableImage = errors.New("unreadable image")

// consoleIconsBase — les fichiers bruts du dépôt des icônes ; remplaçable
// dans un test.
var consoleIconsBase = "https://raw.githubusercontent.com/KyleBing/retro-game-console-icons/main"

// consoleIconsSeries — la série retenue, la même pour toutes les consoles.
const consoleIconsSeries = "series_trimui/600w@2x"

// L'attribution que demande la licence du dépôt (son README, 2026-11-12) :
// « Retro game console icons by KyleBing », le lien, « CC BY 4.0 », et les
// changements.
const (
	consoleIconsAuthor     = "KyleBing"
	consoleIconsTitle      = "Retro game console icons"
	consoleIconsRepository = "https://github.com/KyleBing/retro-game-console-icons"
	consoleIconsLicense    = "CC BY 4.0"
	consoleIconsLicenseURL = "https://creativecommons.org/licenses/by/4.0/"
	consoleIconsChanges    = "resized"
)

// fetchUserAgent — un nom de client générique, rien qui désigne
// l'installation ni son propriétaire.
const fetchUserAgent = "GameFlex/1.0 (self-hosted game server)"

var consoleIconsClient = &http.Client{Timeout: 60 * time.Second}

// consoleIcons — l'icône de chaque console rétro, dans `consoleIconsSeries`
// (chaque fichier vérifié dans l'arbre du dépôt le 2026-11-12, et regardé).
// Les noms sont ceux du dépôt : FC (la manette Famicom) pour la NES, SFC
// pour la Super Nintendo, NGP pour la Neo Geo Pocket (et Color), WSC pour la
// WonderSwan (et Color).
//
// SANS ICÔNE dans la série : gc (NGC y est la Neo Geo Pocket Color, pas la
// GameCube), wii, jaguar, ps2. Et toutes les consoles streamex et de jeu à
// distance, par décision du porteur (2026-11-12).
var consoleIcons = map[string]string{
	"nes":            "FC.png",
	"snes":           "SFC.png",
	"gbc":            "GBC.png",
	"gba":            "GBA.png",
	"vb":             "VB.png",
	"n64":            "N64.png",
	"nds":            "NDS.png",
	"sms":            "MS.png",
	"megadrive":      "MD.png",
	"saturn":         "SATURN.png",
	"dreamcast":      "DC.png",
	"atari2600":      "ATARI2600.png",
	"atari5200":      "ATARI5200.png",
	"atari7800":      "ATARI7800.png",
	"lynx":           "LYNX.png",
	"pce":            "PCE.png",
	"pcfx":           "PCFX.png",
	"neogeo":         "NEOGEO.png",
	"ngp":            "NGP.png",
	"cps1":           "CPS1.png",
	"cps2":           "CPS2.png",
	"wswan":          "WSC.png",
	"coleco":         "COLECO.png",
	"vectrex":        "VECTREX.png",
	"c64":            "C64.png",
	"amstrad":        "CPC.png",
	"zxspectrum":     "ZXS.png",
	"doom":           "DOOM.png",
	"scummvm":        "SCUMMVM.png",
	"ps1":            "PS.png",
	"psp":            "PSP.png",
	consoles.PortsID: "PORTS.png",
}

// ConsoleImageSource — d'où vient l'image d'une console : ce que montre la
// ligne d'attribution du launcher, et ce que `source.json` garde.
type ConsoleImageSource struct {
	Source     string     `json:"source"` // manual, kylebing
	Result     string     `json:"result,omitempty"`
	File       string     `json:"file,omitempty"`
	Title      string     `json:"title,omitempty"`
	Author     string     `json:"author,omitempty"`
	Repository string     `json:"repository,omitempty"`
	License    string     `json:"license,omitempty"`
	LicenseURL string     `json:"licenseUrl,omitempty"`
	Changes    string     `json:"changes,omitempty"`
	URL        string     `json:"url,omitempty"`
	FetchedAt  *time.Time `json:"fetchedAt,omitempty"` // absente : pas encore téléchargée
	Original   string     `json:"original,omitempty"`  // le fichier gardé, dans le dossier de la source
}

const (
	sourceManual   = "manual"
	sourceKyleBing = "kylebing"
)

// consoleImageTypes — les images qu'un administrateur peut déposer, par
// extension. Le WebP se garde et se sert tel quel : la bibliothèque
// standard ne sait pas le décoder, donc pas le réduire.
var consoleImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

func consoleDir(storageRoot, consoleID string) string {
	return filepath.Join(storageRoot, "metadata", consoleID, "_console")
}

// ConsoleImage — le fichier de l'image de la console, à la largeur `width`
// (0 : la plus grande) : celle de l'administrateur, sinon l'icône de KyleBing,
// téléchargée au premier appel. ErrUnknownConsole hors du registre,
// os.ErrNotExist quand la console n'a pas d'image.
func ConsoleImage(ctx context.Context, storageRoot, consoleID string, width int) (string, error) {
	if !consoles.Exists(consoleID) {
		return "", ErrUnknownConsole
	}
	directory := consoleDir(storageRoot, consoleID)

	if original := manualConsoleImage(directory); original != "" {
		return reducedConsoleImage(original, filepath.Join(directory, sourceManual), width)
	}

	original, err := kyleBingConsoleImage(ctx, directory, consoleID)
	if err != nil {
		return "", err
	}
	// `_console/` au propriétaire du dossier : l'administrateur y crée
	// `manual/` à la main.
	giveToStorageOwner(storageRoot, filepath.Dir(directory), directory)

	return reducedConsoleImage(original, filepath.Join(directory, sourceKyleBing), width)
}

// ConsoleImageAttribution — la source de l'image que ConsoleImage sert ou
// servira : `manual`, ou l'icône de KyleBing avec ce que sa licence demande
// de citer (la collection, l'auteur, la licence, le lien du fichier, les
// changements). Ne télécharge rien.
func ConsoleImageAttribution(storageRoot, consoleID string) (ConsoleImageSource, error) {
	if !consoles.Exists(consoleID) {
		return ConsoleImageSource{}, ErrUnknownConsole
	}
	directory := consoleDir(storageRoot, consoleID)

	if original := manualConsoleImage(directory); original != "" {
		return ConsoleImageSource{Source: sourceManual, File: filepath.Base(original)}, nil
	}

	file, mapped := consoleIcons[consoleID]
	if !mapped {
		return ConsoleImageSource{}, os.ErrNotExist
	}
	if state, found := readConsoleSource(filepath.Join(directory, sourceKyleBing)); found && state.Result == "found" && state.File == file {
		state.Original = ""

		return state, nil
	}

	return kyleBingSource(file), nil
}

// SetManualConsoleImage — l'image déposée par l'administrateur, à la place
// de la précédente. `extension` : celle de son type (consoleImageTypes).
func SetManualConsoleImage(storageRoot, consoleID, extension string, content []byte) error {
	if !consoles.Exists(consoleID) {
		return ErrUnknownConsole
	}
	if _, known := consoleImageTypes[extension]; !known {
		return fmt.Errorf("%w: type %q", ErrUnreadableImage, extension)
	}
	if extension != ".webp" {
		if _, err := decodeImage(content); err != nil {
			return fmt.Errorf("%w: %v", ErrUnreadableImage, err)
		}
	}
	directory := filepath.Join(consoleDir(storageRoot, consoleID), sourceManual)

	lock := lockFor(directory)
	lock.Lock()
	defer lock.Unlock()
	// Une seule image : celle d'avant part, et ses réductions avec elle.
	if err := os.RemoveAll(directory); err != nil {
		return err
	}

	target := filepath.Join(directory, "console"+extension)
	if err := writeAtomically(target, content); err != nil {
		return err
	}
	giveToStorageOwner(storageRoot, filepath.Dir(filepath.Dir(directory)), filepath.Dir(directory), directory, target)

	return os.Chmod(target, 0o644)
}

// RemoveManualConsoleImage — retour à l'icône de KyleBing.
func RemoveManualConsoleImage(storageRoot, consoleID string) error {
	if !consoles.Exists(consoleID) {
		return ErrUnknownConsole
	}
	directory := filepath.Join(consoleDir(storageRoot, consoleID), sourceManual)

	lock := lockFor(directory)
	lock.Lock()
	defer lock.Unlock()

	return os.RemoveAll(directory)
}

// ImageExtension — l'extension d'une image déposée (image de console, fond),
// d'après son contenu (jamais d'après ce que l'envoi prétend). Vide : pas une
// image acceptée.
func ImageExtension(content []byte) string {
	switch http.DetectContentType(content) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}

	return ""
}

// manualConsoleImage — l'image de l'administrateur : le premier fichier
// d'image de `manual/`, quel que soit son nom. Les réductions vivent dans
// `manual/reduced/`, un dossier, que la liste saute.
func manualConsoleImage(directory string) string {
	entries, err := os.ReadDir(filepath.Join(directory, sourceManual))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if _, known := consoleImageTypes[strings.ToLower(filepath.Ext(name))]; entry.Type().IsRegular() && known && !strings.HasPrefix(name, ".") {
			return filepath.Join(directory, sourceManual, name)
		}
	}

	return ""
}

// reducedConsoleImage — l'image ramenée à la largeur demandée, gardée dans
// `<source>/reduced/` et refaite seulement si l'origine change. En PNG dès
// qu'un pixel n'est pas opaque (le détourage reste), en JPEG sinon. Un WebP
// est servi tel quel.
func reducedConsoleImage(original, sourceDir string, width int) (string, error) {
	if strings.EqualFold(filepath.Ext(original), ".webp") {
		return original, nil
	}
	width = widthFor(width)
	originalInfo, err := os.Stat(original)
	if err != nil {
		return "", err
	}
	for _, extension := range []string{".png", ".jpg"} {
		target := filepath.Join(sourceDir, "reduced", fmt.Sprintf("console-%d%s", width, extension))
		if targetInfo, err := os.Stat(target); err == nil && !targetInfo.ModTime().Before(originalInfo.ModTime()) {
			return target, nil
		}
	}

	lock := lockFor(filepath.Join(sourceDir, "reduced", fmt.Sprint(width)))
	lock.Lock()
	defer lock.Unlock()
	content, err := os.ReadFile(original)
	if err != nil {
		return "", err
	}
	picture, err := decodeImage(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", original, err)
	}
	extension := ".jpg"
	if !picture.Opaque() {
		extension = ".png"
	}
	target := filepath.Join(sourceDir, "reduced", fmt.Sprintf("console-%d%s", width, extension))
	if err := writeReduced(target, picture, width, extension); err != nil {
		return "", err
	}

	return target, nil
}

// kyleBingConsoleImage — l'icône de la console, téléchargée au premier
// appel. Une icône gardée sert pour toujours (hors ligne compris) ; un
// fichier absent du dépôt ne se redemande qu'après 30 jours, une erreur du
// réseau après une heure — comme libretro.
func kyleBingConsoleImage(ctx context.Context, directory, consoleID string) (string, error) {
	file, mapped := consoleIcons[consoleID]
	if !mapped {
		return "", os.ErrNotExist
	}
	sourceDir := filepath.Join(directory, sourceKyleBing)

	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()

	// Un autre fichier que celui du choix : le choix a changé, on refait.
	if state, found := readConsoleSource(sourceDir); found && state.File == file {
		switch {
		case state.Result == "found" && state.Original != "":
			path := filepath.Join(sourceDir, state.Original)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		case state.Result == "notFound" && state.FetchedAt != nil && timeNow().Sub(*state.FetchedAt) < libretroAbsentAge:
			return "", os.ErrNotExist
		case state.Result == "error" && state.FetchedAt != nil && timeNow().Sub(*state.FetchedAt) < libretroErrorAge:
			return "", os.ErrNotExist
		}
	}

	state, fetchErr := fetchConsoleIcon(ctx, sourceDir, file)
	if err := writeConsoleSource(sourceDir, state); err != nil {
		return "", err
	}
	if fetchErr != nil {
		return "", fmt.Errorf("kylebing: %w", fetchErr)
	}
	if state.Result != "found" {
		return "", os.ErrNotExist
	}

	return filepath.Join(sourceDir, state.Original), nil
}

// kyleBingSource — l'attribution d'une icône, telle que la licence la
// demande.
func kyleBingSource(file string) ConsoleImageSource {
	return ConsoleImageSource{
		Source:     sourceKyleBing,
		File:       file,
		Title:      consoleIconsTitle,
		Author:     consoleIconsAuthor,
		Repository: consoleIconsRepository,
		License:    consoleIconsLicense,
		LicenseURL: consoleIconsLicenseURL,
		Changes:    consoleIconsChanges,
		URL:        consoleIconsRepository + "/blob/main/" + consoleIconsSeries + "/" + file,
	}
}

// fetchConsoleIcon — télécharge l'icône et la garde telle quelle (un PNG
// de 600 px : rien à réduire avant).
func fetchConsoleIcon(ctx context.Context, sourceDir, file string) (ConsoleImageSource, error) {
	state := kyleBingSource(file)
	fetchedAt := timeNow().UTC()
	state.Result, state.FetchedAt = "error", &fetchedAt

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, consoleIconsBase+"/"+consoleIconsSeries+"/"+url.PathEscape(file), nil)
	if err != nil {
		return state, err
	}
	request.Header.Set("User-Agent", fetchUserAgent)
	response, err := consoleIconsClient.Do(request)
	if err != nil {
		return state, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		state.Result = "notFound"

		return state, nil
	}
	if response.StatusCode != http.StatusOK {
		return state, fmt.Errorf("%s: %s", file, response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return state, err
	}
	if _, err := decodeImage(content); err != nil {
		return state, fmt.Errorf("%s: %w", file, err)
	}
	state.Original = "original.png"
	if err := writeAtomically(filepath.Join(sourceDir, state.Original), content); err != nil {
		return state, err
	}
	state.Result = "found"

	return state, nil
}

func readConsoleSource(sourceDir string) (ConsoleImageSource, bool) {
	var state ConsoleImageSource
	content, err := os.ReadFile(filepath.Join(sourceDir, "source.json"))
	if err != nil || json.Unmarshal(content, &state) != nil {
		return ConsoleImageSource{}, false
	}

	return state, true
}

func writeConsoleSource(sourceDir string, state ConsoleImageSource) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return writeAtomically(filepath.Join(sourceDir, "source.json"), content)
}
