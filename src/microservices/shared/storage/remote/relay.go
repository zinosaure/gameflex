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
	"encoding/json"
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
	// La source choisie par la page, quand le serveur en propose plusieurs (chantier 61, listing.Item.Sources).
	response, err := client.PreferSource(r.URL.Query().Get("source")).StreamRom(r.Context(), consoleID, game.ID)
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

// LE RELAIS DE LA FÉDÉRATION (chantier 83) — un serveur qu'un ami nous relaie se lit CHEZ L'AMI, sous
// `/api/v1/federation/relay/<id>` : un pair virtuel, dont l'adresse est celle du relais suivie de ce préfixe, et le
// jeton celui qu'on a chez le relais. Le reste du paquet le traite comme un pair direct.

// RelayLink — l'adresse d'un serveur relayé : celle du relais, puis son préfixe.
func RelayLink(relayLink string, id int64) string {
	return strings.TrimRight(relayLink, "/") + RelayPrefix + strconv.FormatInt(id, 10)
}

// RelayPrefix — le préfixe des routes relayées, chez le relais.
const RelayPrefix = "/api/v1/federation/relay/"

// RelayID — l'identifiant, dans notre registre, d'un serveur relayé : négatif, il ne heurte jamais celui d'un pair
// direct (la base les donne positifs) ; le relais et l'identifiant chez lui le composent.
func RelayID(viaID, id int64) int64 {
	return -(viaID*1_000_000 + id)
}

// Forward — une demande (GET ou HEAD) relayée telle quelle à ce serveur, son chemin et sa requête, avec notre jeton
// chez lui : le relais de la fédération la rend à l'appareil qui l'a faite. Pas d'échéance : un jeu se télécharge par
// là.
func (server *Server) Forward(ctx context.Context, method, pathAndQuery string) (*http.Response, error) {
	return server.send(ctx, method, pathAndQuery, server.relay)
}

// relayablePaths — les routes du catalogue qu'un relais repasse (sous `/api/v1/federation`) : le listing, les
// fichiers, les paramètres, les correctifs, les fiches et leurs images.
var relayablePaths = []string{"/listing", "/roms/", "/bios/", "/cores/", "/bundles/", "/params/", "/patches/", "/metadata/"}

// Relayable — ce chemin (sous `/api/v1/federation`) se relaie-t-il ? Voir et télécharger seulement (le porteur,
// réponse 2) : jamais une partie, une console Remote Play (réponse 3), l'identité, ni la recherche par les comptes des
// fournisseurs de l'origine — elle prêterait au porteur ce que l'origine n'a prêté qu'au relais.
func Relayable(path string) bool {
	if strings.Contains(path, "..") || strings.Contains(path, "/providers") {
		return false
	}
	for _, prefix := range relayablePaths {
		if path == prefix || strings.HasPrefix(path, prefix) || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}

	return false
}

// RelayedListing — le listing de l'origine (`/listing`), ou sa fraîcheur (`/listing/updated`), tels qu'un relais les
// rend à son porteur : voir et télécharger seulement (aucune console à lancer, la portée `catalog` seule), et un seul
// saut (ni ses serveurs relayés, ni la permission de le relayer encore). `body` est la réponse enveloppée du kernel
// (`{"success", "data"}`) ; le résultat est la donnée à renvoyer, enveloppée à son tour par l'appelant.
func RelayedListing(path string, body io.Reader) (map[string]json.RawMessage, error) {
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil || envelope.Data == nil {
		return nil, errors.New("the relayed listing cannot be read")
	}
	fields := envelope.Data
	if path == "/listing" {
		var header map[string]json.RawMessage
		if err := json.Unmarshal(fields["header"], &header); err != nil || header == nil {
			return nil, errors.New("the relayed listing cannot be read")
		}
		fields = header
	}
	set := func(name string, value any) { fields[name], _ = json.Marshal(value) }
	set("scopes", []string{"catalog"})
	set("consoles", []string{})
	set("relayAllowed", false)
	set("relayed", []listing.RelayedServer{})
	if path == "/listing" {
		envelope.Data["header"], _ = json.Marshal(fields)
	}

	return envelope.Data, nil
}

