package remote

// RELAYER UN JEU D'UN PAIR jusqu'au navigateur qui le télécharge (chantier 14),
// sa fiche et ses images jusqu'à la page qui le montre (chantier 57), le fond
// d'une console qui n'en a pas ici (`#740`), et dire pourquoi on ne le peut
// pas — le même geste dans les deux kernels.
//
// Le jeton du pair ne sort pas du kernel : c'est lui qui lit chez le pair, le
// navigateur ne voit que lui. Rien ne s'écrit sur le disque — le fichier ne
// fait que passer.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gameflex/storage"
	"gameflex/storage/listing"
)

// StatusError — un refus à rendre tel quel : son statut HTTP et son message.
// Chaque kernel l'écrit dans sa propre enveloppe.
type StatusError struct {
	Status  int
	Code    string
	Message string
}

func (e *StatusError) Error() string { return e.Message }

var (
	refusedAccess = &StatusError{Status: http.StatusForbidden, Code: "peer_denied", Message: "this federated server does not allow access to its consoles"}
	notAnswering  = &StatusError{Status: http.StatusBadGateway, Code: "peer_unreachable", Message: "this federated server did not answer"}
	unknownGame   = &StatusError{Status: http.StatusNotFound, Code: "game_not_found", Message: "rom not found"}
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
			return &StatusError{Status: http.StatusBadGateway, Code: "peer_unreachable", Message: err.Error()}
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

// notLent — le pair répond 404 ou 405 à la fiche d'un de ses jeux : il n'a plus ce jeu, ou sa version n'a pas ces
// routes (chantier 57 ; fetchMetadata). La page dit qu'il ne prête pas cette fiche ; le kernel ne se rabat pas sur son
// propre fournisseur.
var notLent = &StatusError{Status: http.StatusNotFound, Code: "peer_not_lending", Message: "this federated server does not lend this game's metadata"}

// relayedHeaders — ce qu'une fiche ou une image relayée garde de la réponse du pair : le navigateur garde l'image en
// cache par son `ETag`, et revalide (`Cache-Control: private, no-cache`, serveChangingImage chez lui).
var relayedHeaders = []string{"Content-Type", "Content-Length", "ETag", "Cache-Control", "Last-Modified"}

// RelayMetadata — LA FICHE D'UN JEU D'UN PAIR, relayée telle quelle (chantier 57) : son statut, son corps, ses
// en-têtes. Une `*StatusError` rendue AVANT le premier octet est à répondre par l'appelant : 403 quand il refuse notre
// jeton, 404 quand il ne prête pas cette fiche, 502 quand il ne répond pas. Rien ne se copie.
func RelayMetadata(w http.ResponseWriter, r *http.Request, peer Peer, consoleID, reference string) error {
	return relayMetadata(w, r, peer, func(ctx context.Context, client *Server) (*http.Response, error) {
		return client.StreamGameMetadata(ctx, consoleID, reference)
	})
}

// RelayImage — UNE IMAGE D'UN JEU D'UN PAIR, relayée telle quelle, à la largeur que la page demande (`?w=`) : son
// `If-None-Match` va jusqu'au pair, et un 304 revient tel quel. Mêmes refus que RelayMetadata ; un 404 dit alors
// « pas d'image », et la page dessine son affiche générée.
func RelayImage(w http.ResponseWriter, r *http.Request, peer Peer, consoleID, reference, kind string) error {
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))

	return relayMetadata(w, r, peer, func(ctx context.Context, client *Server) (*http.Response, error) {
		return client.StreamGameImage(ctx, consoleID, reference, kind, width, r.Header.Get("If-None-Match"))
	})
}

// RelayConsoleBackground — UNE IMAGE DU FOND D'UNE CONSOLE CHEZ UN PAIR, relayée telle quelle (`#740` : une console qui
// n'a aucune image ici montre celles du premier serveur fédéré qui en a), à la largeur que la page demande (`?w=`), son
// `If-None-Match` jusqu'au pair. Mêmes refus que RelayMetadata ; un 404 dit « pas d'image ». Rien ne se copie.
func RelayConsoleBackground(w http.ResponseWriter, r *http.Request, peer Peer, consoleID, name string) error {
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))

	return relayMetadata(w, r, peer, func(ctx context.Context, client *Server) (*http.Response, error) {
		return client.StreamConsoleBackground(ctx, consoleID, name, width, r.Header.Get("If-None-Match"))
	})
}

func relayMetadata(w http.ResponseWriter, r *http.Request, peer Peer, open func(context.Context, *Server) (*http.Response, error)) error {
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return notAnswering
	}
	ctx, cancel := context.WithTimeout(r.Context(), metadataTimeout)
	defer cancel()
	response, err := open(ctx, client)
	switch {
	case errors.Is(err, ErrDenied):
		return refusedAccess
	case errors.Is(err, ErrNotLent):
		return notLent
	case err != nil:
		return notAnswering
	}
	defer response.Body.Close()

	for _, header := range relayedHeaders {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if response.StatusCode == http.StatusNotModified || r.Method == http.MethodHead {
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
