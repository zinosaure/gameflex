package metadata

// CE QUE LES GESTIONNAIRES ONT DE COMMUN (chantier 57, P1) : la table des
// refus, le corps d'une image choisie, le jugement des orphelins. Le serveur
// les lisait dans son paquet api ; le téléphone, qui aura les mêmes routes
// (P4), les lit ici au lieu de les recopier. Chaque kernel écrit un refus dans
// sa propre enveloppe, comme remote.StatusError.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"syscall"
)

// Refused — un refus à répondre : son statut HTTP, un code quand la page doit
// le reconnaître (`service_not_configured`, `disk_read_only`), son message.
// Unknown : l'erreur n'est pas un refus connu — une source qui a échoué, 502,
// que le kernel note au journal.
type Refused struct {
	Status  int
	Code    string
	Message string
	Unknown bool
}

// Refusal — les refus communs aux gestes sur la fiche d'un jeu : le texte
// corrigé, une image choisie, les candidats, un jeu désigné, une source
// oubliée, les orphelins.
func Refusal(err error) Refused {
	switch {
	case errors.Is(err, ErrUnknownSource):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: "unknown source"}
	case errors.Is(err, ErrUnknownKind):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: "unknown image kind"}
	case errors.Is(err, ErrNotAGameFolder):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: "not a game metadata folder"}
	case errors.Is(err, ErrInvalidManual), errors.Is(err, ErrInvalidExternalID):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: err.Error()}
	case errors.Is(err, ErrUnreadableImage):
		return Refused{Status: http.StatusUnsupportedMediaType, Code: "image_format", Message: err.Error()}
	case errors.Is(err, ErrServiceNotConfigured):
		return Refused{Status: http.StatusConflict, Code: "service_not_configured", Message: "this service has no account: enter it in Settings"}
	case errors.Is(err, ErrPeerDenied):
		return Refused{Status: http.StatusForbidden, Code: "peer_denied", Message: err.Error()}
	case errors.Is(err, ErrPeerNotLending):
		return Refused{Status: http.StatusNotFound, Code: "peer_not_lending", Message: err.Error()}
	case errors.Is(err, syscall.EROFS):
		// Un disque en lecture seule (NTFS branché au téléphone) : dit tel quel, jamais un échec muet — l'interrupteur
		// « Garder les métadonnées sur le disque des jeux », coupé, les écrit dans le dossier du serveur.
		return Refused{Status: http.StatusConflict, Code: "disk_read_only", Message: "the game's disk is read-only: turn off « Keep metadata on the games' disk » in Settings › Metadata provider"}
	case errors.Is(err, os.ErrNotExist):
		return Refused{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
	}

	return Refused{Status: http.StatusBadGateway, Code: "metadata_failed", Message: err.Error(), Unknown: true}
}

// ChosenImageRefusal — les refus d'une image choisie : son corps, puis sa
// récupération chez l'hôte d'images de sa source, ou chez le serveur fédéré qui
// la prête (403 il refuse notre jeton, 404 il ne la prête pas).
func ChosenImageRefusal(err error) Refused {
	switch {
	case errors.Is(err, ErrInvalidChoice), errors.Is(err, ErrImageNotAllowed):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: err.Error()}
	case errors.Is(err, ErrPeerDenied):
		return Refused{Status: http.StatusForbidden, Code: "peer_denied", Message: err.Error()}
	case errors.Is(err, ErrPeerNotLending):
		return Refused{Status: http.StatusNotFound, Code: "peer_not_lending", Message: err.Error()}
	case errors.Is(err, ErrImageTooLarge):
		return Refused{Status: http.StatusRequestEntityTooLarge, Code: "image_too_large", Message: err.Error()}
	case errors.Is(err, ErrUnreadableImage):
		return Refused{Status: http.StatusUnsupportedMediaType, Code: "image_format", Message: err.Error()}
	}

	return Refused{Status: http.StatusBadGateway, Code: "metadata_failed", Message: err.Error(), Unknown: true}
}

// BackgroundRefusal — les refus communs aux gestes sur le fond d'une console,
// puis ceux d'une image choisie.
func BackgroundRefusal(err error) Refused {
	switch {
	case errors.Is(err, ErrUnknownConsole):
		return Refused{Status: http.StatusBadRequest, Code: "console_unknown", Message: "unknown console"}
	case errors.Is(err, ErrBackgroundSlot):
		return Refused{Status: http.StatusBadRequest, Code: "bad_request", Message: "a background slot is 1 to 8"}
	}

	return ChosenImageRefusal(err)
}

// ErrInvalidChoice — le corps d'un choix d'image n'est ni `{source, url}`, ni,
// pour l'image d'un serveur fédéré, `{source: "server", server, reference}`.
var ErrInvalidChoice = errors.New("a JSON object {source, url} is expected: an image chosen from a search")

// ChosenImage — le corps d'un choix d'image : une image d'un fournisseur
// (`source`, `url`), ou celle d'un jeu d'un serveur fédéré (`source: "server"`,
// son identifiant dans le registre, la référence de son jeu chez lui — jamais
// une adresse : le lien et le jeton viennent du registre).
type ChosenImage struct {
	Source    string `json:"source"`
	URL       string `json:"url"`
	Server    int64  `json:"server"`
	Reference string `json:"reference"`
}

// FromServer — l'image vient d'un serveur fédéré.
func (chosen ChosenImage) FromServer() bool {
	return chosen.Source == sourceServer
}

// DecodeChosenImage — le corps d'un choix (8 Ko au plus). ErrInvalidChoice : ni
// une image d'un fournisseur, ni celle d'un serveur fédéré.
func DecodeChosenImage(body io.Reader) (ChosenImage, error) {
	var chosen ChosenImage
	if err := json.NewDecoder(io.LimitReader(body, 8<<10)).Decode(&chosen); err != nil || chosen.Source == "" {
		return ChosenImage{}, ErrInvalidChoice
	}
	if chosen.FromServer() {
		if chosen.Server <= 0 || chosen.Reference == "" || len(chosen.Reference) > 1024 {
			return ChosenImage{}, ErrInvalidChoice
		}

		return chosen, nil
	}
	if chosen.URL == "" {
		return ChosenImage{}, ErrInvalidChoice
	}

	return chosen, nil
}

// ReadChosenImage — le corps `{source, url}` d'un choix, puis l'image récupérée
// par `fetch` chez l'hôte d'images de sa source (FetchChosenImage, que le test
// d'un kernel remplace) : le fond d'une console, qui ne prend pas les images
// des pairs. Ses refus : ChosenImageRefusal.
func ReadChosenImage(ctx context.Context, body io.Reader, fetch func(ctx context.Context, source, address string) ([]byte, error)) ([]byte, error) {
	chosen, err := DecodeChosenImage(body)
	if err != nil || chosen.FromServer() {
		return nil, ErrInvalidChoice
	}

	return fetch(ctx, chosen.Source, chosen.URL)
}

// KnownGameKeys — ajoute à `keys` le gameid de chaque jeu de `paths` (référence
// → chemin), tel que la mémoire le connaît (KnownGameKey), et à `references`
// celui de sa référence ; faux dès qu'il en manque un. Ce qui manque se lit en
// fond (GameKey) : rien ne se juge orphelin entre-temps.
func KnownGameKeys(paths map[string]string, keys map[string]bool, references map[string]string) bool {
	for reference, path := range paths {
		key, found := KnownGameKey(path, reference)
		if !found {
			return false
		}
		keys[key] = true
		references[reference] = key
	}

	return true
}

// IsOrphan — le dossier `gameID` de `root` est-il orphelin, d'après les gameid
// des jeux accessibles (`keys`) et celui de chaque référence (`references`,
// KnownGameKeys) ? Un dossier rangé sous la référence d'un jeu présent rejoint
// d'abord son gameid (MoveReferenceFolder) : il n'est plus orphelin. Un gameid
// qui a déjà son dossier le laisse tel quel (ErrGameFolderTaken) : il reste
// orphelin.
func IsOrphan(root, consoleID, gameID string, keys map[string]bool, references map[string]string) bool {
	if keys[gameID] {
		return false
	}
	key, isReference := references[gameID]
	if !isReference || key == gameID {
		return true
	}
	moved, err := MoveReferenceFolder(root, consoleID, gameID, key)
	switch {
	case moved:
		slog.Info("game metadata moved to the game id", "root", root, "console", consoleID, "from", gameID, "to", key)
	case err != nil && !errors.Is(err, ErrGameFolderTaken):
		slog.Warn("game metadata not moved to the game id", "root", root, "console", consoleID, "from", gameID,
			"to", key, "error", err)
	}

	return !moved
}
