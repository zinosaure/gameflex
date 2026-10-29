package remote

// RELAYER UN JEU D'UN PAIR jusqu'au navigateur qui le télécharge (chantier 14),
// et dire pourquoi on ne le peut pas — le même geste dans les deux kernels.
//
// Le jeton du pair ne sort pas du kernel : c'est lui qui lit chez le pair, le
// navigateur ne voit que lui. Rien ne s'écrit sur le disque — le fichier ne
// fait que passer.

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"gameflex/storage"
	"gameflex/storage/listing"
)

// StatusError — un refus à rendre tel quel : son statut HTTP et son message.
// Chaque kernel l'écrit dans sa propre enveloppe.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string { return e.Message }

var (
	refusedAccess = &StatusError{Status: http.StatusForbidden, Message: "this federated server does not allow access to its consoles"}
	notAnswering  = &StatusError{Status: http.StatusBadGateway, Message: "this federated server did not answer"}
	unknownGame   = &StatusError{Status: http.StatusNotFound, Message: "rom not found"}
)

// Refusal — l'erreur d'une lecture chez un pair, telle que la page la lit :
// 403 quand il refuse notre jeton, 502 quand il ne répond pas. La page dit
// alors pourquoi il n'y a rien, au lieu d'une liste vide qui ne dit rien.
func Refusal(err error) *StatusError {
	var already *StatusError
	if errors.As(err, &already) {
		return already
	}
	if errors.Is(err, ErrDenied) {
		return refusedAccess
	}

	return notAnswering
}

// Relay — le jeu, relayé tel quel ; `game` est ce que la copie de son listing
// annonce (Copies.Game). Une `*StatusError` rendue AVANT le premier octet est
// à répondre par l'appelant ; une autre erreur arrive après : le navigateur a
// abandonné, ou le pair a coupé — il n'y a plus qu'à la noter.
//
// `ctx` de la requête borne le transfert (StreamRom) : un navigateur qui
// abandonne coupe aussi la lecture chez le pair.
func Relay(w http.ResponseWriter, r *http.Request, peer Peer, consoleID string, game listing.Item) error {
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return notAnswering
	}
	response, err := client.StreamRom(r.Context(), consoleID, game.ID)
	if err != nil {
		return Refusal(err)
	}
	defer response.Body.Close()

	for _, header := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Last-Modified"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	// UN TÉLÉPHONE NE NOMME PAS CE QU'IL SERT (`http.ServeFile`) : sans nom, le
	// navigateur retomberait sur le ticket. Le nom vient alors de ce que le
	// listing du pair annonce, par la règle du rapatriement.
	if w.Header().Get("Content-Disposition") == "" {
		name, err := FileName(game)
		if err != nil {
			return &StatusError{Status: http.StatusBadGateway, Message: err.Error()}
		}
		if game.Dir {
			name += ".zip"
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}

	_, err = io.Copy(w, response.Body)

	return err
}

// FileName — le nom sous lequel un jeu d'un pair arrive ici : téléchargé par le
// navigateur, ou rangé dans la bibliothèque.
//
// LA RÈGLE DU RAPATRIEMENT (`storage.RomFileName`), et pas une seconde : un
// DOSSIER garde son titre, sans extension (il arrive zippé) ; un FICHIER prend
// l'extension du fichier que son listing annonce — la référence d'un jeu est
// un UUID, et le titre seul donnait « Super Mario 64 », sans rien qui dise un
// jeu N64 (2026-10-10).
func FileName(game listing.Item) (string, error) {
	if game.Dir {
		return storage.RomFileName(game.Name, nil)
	}

	return storage.RomFileName(game.Name, game.Files)
}
