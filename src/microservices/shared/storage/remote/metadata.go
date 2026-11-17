package remote

// LES FICHES DES JEUX D'UN PAIR (chantier 57) — le client des routes `/api/v1/federation/metadata/…` d'un autre
// GameFlex : la fiche d'un de ses jeux et ses images, telles qu'il les montre lui-même. Le même geste dans les deux
// kernels.
//
// « Les jeux qui viennent de la fédération, leurs metadata viennent du même serveur » (le porteur, 2026-11-15) : le
// kernel qui affiche un jeu fédéré relaie sa fiche et ses images au navigateur (RelayMetadata, RelayImage, relay.go),
// sans rien copier, et le jeton du pair ne sort pas du kernel.
//
// Et un pair est une source de recherche pour mes propres jeux (P3) : sa recherche dans ses fiches, la fiche et les
// images d'un de ses jeux, le jeu d'un de ses fournisseurs s'il les partage — décodés dans la forme que l'appelant
// donne (`into`) : ce paquet parle le protocole, gameflex/metadata la forme (metadata.PeerClient).
//
// Et le fond d'une console (`#740` ; le porteur, 2026-11-16 : « il faut que par défaut les consoles prennent les
// images des serveurs fédérés si en local on n'a pas d'images background. On prend le premier fédéré ») : ses huit
// emplacements chez le pair, et l'image de l'un d'eux, relayée (RelayConsoleBackground).
//
// UN REFUS N'EST PAS UNE PANNE, ET UNE ROUTE ABSENTE NON PLUS : un pair qui refuse notre jeton répond 401 ou 403
// (ErrDenied) ; un jeu qu'il n'a plus, 404 ; un pair d'une version plus ancienne n'a pas ces routes et répond 405 —
// sa surface fédérée n'a, pour un chemin qu'elle ne connaît pas, que son `OPTIONS /*` (catalogRoutes) : chi y
// répond « méthode non permise », AVANT de vérifier le jeton (mesuré sur la release 0.11.0, le 2026-11-15) — ou 404
// (le téléphone, `http.ServeMux`). Les trois se lisent ErrNotLent ; le reste est un pair qui ne répond pas.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// metadataTimeout — une fiche ou une image d'un pair arrive vite ou n'arrivera pas : le pair la lit dans son dossier,
// et ne va chez ses fournisseurs que pour un jeu qu'il n'a encore jamais montré. Au-delà, la page dit qu'il ne répond
// pas, au lieu d'attendre.
const metadataTimeout = 20 * time.Second

// metadataPath — la fiche du jeu `reference` de la console chez le pair. La référence s'échappe : celle d'un téléphone
// est un chemin.
func metadataPath(consoleID, reference string) string {
	return "/api/v1/federation/metadata/" + url.PathEscape(consoleID) + "/" + url.PathEscape(reference)
}

// imagePath — son image `kind`, à la largeur `width` (0 : celle que le pair choisit).
func imagePath(consoleID, reference, kind string, width int) string {
	path := metadataPath(consoleID, reference) + "/images/" + url.PathEscape(kind)
	if width > 0 {
		path += "?w=" + strconv.Itoa(width)
	}

	return path
}

// fetchMetadata — une lecture des fiches : 200, ou 304 quand `etag` est encore le bon. Un refus de notre jeton rend
// ErrDenied (et fs.ErrPermission), un 404 ou un 405 ErrNotLent (et fs.ErrNotExist) : gameflex/metadata les reconnaît
// sans connaître ce paquet. `ctx` borne tout, corps compris.
func (server *Server) fetchMetadata(ctx context.Context, path, etag string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.link+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+server.token)
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}

	response, err := server.relay.Do(request)
	if err != nil {
		return nil, err
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusNotModified:
		return response, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		response.Body.Close()

		return nil, errors.Join(ErrDenied, fs.ErrPermission)
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		response.Body.Close()

		return nil, errors.Join(ErrNotLent, fs.ErrNotExist)
	}
	response.Body.Close()

	return nil, &answeredError{code: response.StatusCode, status: response.Status}
}

// StreamGameMetadata — la fiche d'un jeu de ce serveur, la réponse entière, pour la RELAYER (RelayMetadata).
// L'appelant ferme le corps.
func (server *Server) StreamGameMetadata(ctx context.Context, consoleID, reference string) (*http.Response, error) {
	return server.fetchMetadata(ctx, metadataPath(consoleID, reference), "")
}

// StreamGameImage — l'image `kind` d'un jeu de ce serveur, la réponse entière, pour la relayer (RelayImage) : 304
// quand `etag` est encore le bon. L'appelant ferme le corps.
func (server *Server) StreamGameImage(ctx context.Context, consoleID, reference, kind string, width int, etag string) (*http.Response, error) {
	return server.fetchMetadata(ctx, imagePath(consoleID, reference, kind, width), etag)
}

// decodeMetadata — la réponse `{data}` d'une lecture des fiches, décodée dans `into`.
func (server *Server) decodeMetadata(ctx context.Context, path string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	response, err := server.fetchMetadata(ctx, path, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return fmt.Errorf("réponse illisible : %v", err)
	}

	return json.Unmarshal(envelope.Data, into)
}

// GameMetadata — la fiche d'un jeu de ce serveur, décodée dans `into` (la copie d'une fiche, chantier 57, P3).
func (server *Server) GameMetadata(ctx context.Context, consoleID, reference string, into any) error {
	return server.decodeMetadata(ctx, metadataPath(consoleID, reference), into)
}

// SearchMetadata — sa recherche dans ses fiches (`gameId`, `q`, `kind`), décodée dans `into`.
func (server *Server) SearchMetadata(ctx context.Context, consoleID string, query url.Values, into any) error {
	return server.decodeMetadata(ctx, "/api/v1/federation/metadata/"+url.PathEscape(consoleID)+"/search?"+query.Encode(), into)
}

// ProviderGame — le jeu `externalID` d'un de ses fournisseurs, qu'il relit par son compte pour nous : seulement si
// notre jeton partage ses fournisseurs (la portée `metadata-providers`, `#719`), sinon il refuse (fs.ErrPermission).
func (server *Server) ProviderGame(ctx context.Context, consoleID, provider, externalID string, into any) error {
	return server.decodeMetadata(ctx, "/api/v1/federation/metadata/"+url.PathEscape(consoleID)+"/providers/"+
		url.PathEscape(provider)+"/"+url.PathEscape(externalID), into)
}

// consoleBackgroundsPath — LE FOND D'UNE CONSOLE CHEZ LE PAIR (`#740`) : ses huit emplacements, que montre une console
// qui n'a aucune image ici. Le segment `backgrounds` l'emporte sur `{ref}` chez chi, comme `search`.
func consoleBackgroundsPath(consoleID string) string {
	return "/api/v1/federation/metadata/" + url.PathEscape(consoleID) + "/backgrounds"
}

// ConsoleBackgrounds — les emplacements du fond d'une console chez ce serveur (`{slots, extra}`), décodés dans `into`.
func (server *Server) ConsoleBackgrounds(ctx context.Context, consoleID string, into any) error {
	return server.decodeMetadata(ctx, consoleBackgroundsPath(consoleID), into)
}

// StreamConsoleBackground — l'image `name` du fond d'une console chez ce serveur, à la largeur `width`, la réponse
// entière, pour la relayer (RelayConsoleBackground) : 304 quand `etag` est encore le bon. L'appelant ferme le corps.
func (server *Server) StreamConsoleBackground(ctx context.Context, consoleID, name string, width int, etag string) (*http.Response, error) {
	path := consoleBackgroundsPath(consoleID) + "/" + url.PathEscape(name)
	if width > 0 {
		path += "?w=" + strconv.Itoa(width)
	}

	return server.fetchMetadata(ctx, path, etag)
}

// GameImage — l'image `kind` d'un jeu de ce serveur, à la largeur `width`, `limit` octets au plus : une image choisie
// chez lui, ou la copie de sa fiche.
func (server *Server) GameImage(ctx context.Context, consoleID, reference, kind string, width int, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	response, err := server.fetchMetadata(ctx, imagePath(consoleID, reference, kind, width), "")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	content, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("image de plus de %d octets", limit)
	}

	return content, nil
}
