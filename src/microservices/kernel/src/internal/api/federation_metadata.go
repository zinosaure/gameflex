package api

// LA FICHE D'UN JEU FÉDÉRÉ SE LIT CHEZ SON SERVEUR (chantier 57, P2 ; le porteur, 2026-11-15 : « les jeux qui viennent
// de la fédération, leurs metadata viennent du même serveur. Ce qui fait que si je partage mes jeux, je partage aussi
// mes métadonnées »). Deux bouts :
//
// LE PAIR PRÊTE LES FICHES DE SES JEUX, sous la portée `catalog` comme ses correctifs (catalogRoutes) :
//
//   - GET /api/v1/federation/metadata/{consoleId}/{ref}                   la fiche d'un de ses jeux (metadata.Info) ;
//   - GET /api/v1/federation/metadata/{consoleId}/{ref}/images/{kind}?w=  une de ses images, avec son ETag (304 sur
//     If-None-Match, serveChangingImage).
//
// Elles se lisent par la RÉFÉRENCE de son listing (l'UUID que le listing annonce), et non par le `gameId` : une fiche se
// calcule depuis le fichier du jeu, et le listing ne porte pas de `gameId`. Le jeu se trouve comme pour
// `/federation/roms/{c}/{ref}` — la bibliothèque, puis ses disques —, et ses racines suivent son interrupteur
// (gameMetadataRoots) : il montre exactement ce qu'il se montre à lui-même. En lecture seule : aucun geste ne passe
// par la fédération.
//
// LE KERNEL QUI AFFICHE RELAIE. Un jeu fédéré — sa référence `srv<id>:<ref>` (remoteReference), et `?server=<id>` que
// la page envoie avec — a sa fiche et ses images relayées telles quelles (remote.RelayMetadata, remote.RelayImage) :
// le statut du pair, son `Content-Type`, son `ETag`, son `Cache-Control`. Rien ne se copie, et le kernel ne se rabat
// JAMAIS sur son propre fournisseur (décision 7) : un pair qui refuse notre jeton, c'est 403 ; qui ne répond pas, 502 ;
// d'une version plus ancienne (sa route n'existe pas), 404 — « ce serveur ne prête pas cette fiche ». Les gestes sur
// la fiche d'un jeu fédéré (corriger, choisir une image, chercher, désigner, oublier) sont refusés, 409
// `federated_game`, avant toute recherche (metadataGame) : elle se corrige chez son serveur.
//
// UN SERVEUR FÉDÉRÉ EST AUSSI UNE SOURCE DE RECHERCHE POUR MES PROPRES JEUX (P3, `#715` ; le porteur : « je peux
// réutiliser les métadonnées venant d'un serveur fédéré pour mes propres jeux ») :
//
//   - GET /api/v1/federation/metadata/{consoleId}/search?gameId=&q=&kind=  le pair cherche dans ses fiches
//     (metadata.SearchLent) : ses jeux du même `gameId`, puis du titre ; ce qui manque de ses identifiants se lit en
//     fond (warmGameKeys), la réponse le dit (`ready: false`) ;
//   - GET /api/v1/federation/metadata/{consoleId}/providers/{service}/{externalId}  le jeu d'un de ses fournisseurs,
//     relu par son compte — seulement si le jeton partage ses fournisseurs (la portée `metadata-providers`, `#719`,
//     coupée par défaut) ; avec elle, sa recherche interroge aussi ses comptes pour un titre qu'il n'a pas. Ses clés
//     ne sortent jamais.
//
// Chez le kernel qui cherche, mes serveurs entrent dans « Choisir une image », « Ce n'est pas le bon jeu ? » et la
// copie d'une fiche (game_metadata.go) ; un jeu téléchargé d'un pair emporte sa fiche (copyPeerSheet). Le tout vit dans
// gameflex/metadata (peers.go), que le téléphone partagera.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
	"gameflex/storage/remote"
)

// lentGame — un de MES jeux, désigné par un pair : la console, la référence de mon listing, son fichier et les racines
// de ses métadonnées. Faux après avoir répondu (400 une console inconnue, 404 un jeu absent).
func (server *Server) lentGame(w http.ResponseWriter, r *http.Request) (consoleID, reference, gamePath string, roots metadata.Roots, ok bool) {
	consoleID, reference = chi.URLParam(r, "consoleId"), pathParam(r, "ref")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return "", "", "", roots, false
	}
	gamePath, _, disk, err := server.findRom(r.Context(), consoleID, reference)
	if errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "game_not_found", "rom not found")
		return "", "", "", roots, false
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid rom reference: "+err.Error())
		return "", "", "", roots, false
	}

	return consoleID, reference, gamePath, server.gameMetadataRoots(disk), true
}

// handleFederationGameMetadata — GET /api/v1/federation/metadata/{consoleId}/{ref}.
func (server *Server) handleFederationGameMetadata(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.lentGame(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, metadata.Info(r.Context(), roots, server.metadataAccounts(), consoleID, gamePath, reference))
}

// handleFederationGameImage — GET /api/v1/federation/metadata/{consoleId}/{ref}/images/{kind}?w=.
func (server *Server) handleFederationGameImage(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.lentGame(w, r)
	if !ok {
		return
	}

	server.serveGameImage(w, r, roots, consoleID, gamePath, reference)
}

// federatedGame — le jeu de l'adresse est-il un jeu fédéré, et de quel pair ? Sa référence porte le préfixe de son
// serveur (`srv<id>:<ref>`, comme la liste la donne) ; `?server=<id>`, s'il est là, doit dire le même. Faux pour un
// jeu d'ici, le cas normal. `invalid` : un `?server=` qui ne désigne pas la référence.
func federatedGame(r *http.Request) (serverID int64, peerReference string, federated, invalid bool) {
	serverID, peerReference, federated = parseRemoteReference(pathParam(r, "ref"))
	named := r.URL.Query().Get("server")
	if named == "" {
		return serverID, peerReference, federated, false
	}

	return serverID, peerReference, true, !federated || named != strconv.FormatInt(serverID, 10)
}

// federatedPeer — le pair d'un jeu fédéré, et sa référence chez lui ; faux quand le jeu est d'ici. `answered` : la
// réponse est écrite (une référence et un `?server=` qui se contredisent, une console ou un serveur inconnus).
func (server *Server) federatedPeer(w http.ResponseWriter, r *http.Request) (entry registeredServer, consoleID, peerReference string, federated, answered bool) {
	serverID, peerReference, federated, invalid := federatedGame(r)
	if !federated {
		return entry, "", "", false, false
	}
	if invalid {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a federated game is srv<id>:<reference>, with ?server=<id>")
		return entry, "", "", true, true
	}
	consoleID = chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return entry, "", "", true, true
	}
	entry, found := server.registeredByIDValue(w, strconv.FormatInt(serverID, 10))
	if !found {
		return entry, "", "", true, true
	}

	return entry, consoleID, peerReference, true, false
}

// relayFederatedSheet — la fiche d'un jeu fédéré, relayée de chez son serveur ; faux pour un jeu d'ici (rien n'est
// écrit).
func (server *Server) relayFederatedSheet(w http.ResponseWriter, r *http.Request) bool {
	entry, consoleID, peerReference, federated, answered := server.federatedPeer(w, r)
	if !federated || answered {
		return federated
	}
	server.answerRelay(w, entry, consoleID, remote.RelayMetadata(w, r, peerOf(entry), consoleID, peerReference))

	return true
}

// relayFederatedImage — une image d'un jeu fédéré, relayée de chez son serveur ; faux pour un jeu d'ici.
func (server *Server) relayFederatedImage(w http.ResponseWriter, r *http.Request) bool {
	entry, consoleID, peerReference, federated, answered := server.federatedPeer(w, r)
	if !federated || answered {
		return federated
	}
	server.answerRelay(w, entry, consoleID, remote.RelayImage(w, r, peerOf(entry), consoleID, peerReference, chi.URLParam(r, "kind")))

	return true
}

// answerRelay — un refus d'un relais, rendu AVANT son premier octet (remote.StatusError) ; une autre erreur arrive
// après : le navigateur a abandonné, ou le pair a coupé — il n'y a plus qu'à la noter.
func (server *Server) answerRelay(w http.ResponseWriter, entry registeredServer, consoleID string, err error) {
	var refused *remote.StatusError
	switch {
	case errors.As(err, &refused):
		writeErrorCode(w, refused.Status, codeOr(refused.Code, "metadata_failed"), refused.Message)
	case err != nil:
		slog.Info("federated game metadata interrupted", "server", entry.Hostname, "console", consoleID, "error", err)
	}
}

// ── P3 : un pair, source de recherche ───────────────────────────────────

// lentGames — mes jeux de la console qu'un ami peut chercher : la bibliothèque, puis les disques présents (une
// référence vue une fois), chacun avec les racines de ses métadonnées ; et les racines de jeux lues.
func (server *Server) lentGames(ctx context.Context, consoleID string) ([]metadata.LentGame, []string) {
	type place struct {
		root string
		disk *docker.InternalDisk
	}
	places := []place{{root: server.retroRomsRoot()}}
	for _, disk := range server.presentDisks(ctx) {
		places = append(places, place{root: disk.RomsDir(), disk: &disk})
	}
	games, roots, seen := []metadata.LentGame{}, []string{}, map[string]bool{}
	for _, place := range places {
		if _, err := os.Stat(place.root); err != nil {
			continue
		}
		entries, err := retro_files.ListCatalog(place.root, consoleID, "")
		if err != nil {
			continue
		}
		paths, _ := retro_files.CatalogPaths(place.root, consoleID)
		roots = append(roots, place.root)
		metadataRoots := server.gameMetadataRoots(place.disk)
		for _, entry := range entries {
			path := paths[entry.Reference]
			if path == "" || seen[entry.Reference] {
				continue
			}
			seen[entry.Reference] = true
			games = append(games, metadata.LentGame{Reference: entry.Reference, Title: entry.Title, Path: path, Roots: metadataRoots})
		}
	}

	return games, roots
}

// limited — une valeur de la requête, ramenée à `size` caractères.
func limited(value string, size int) string {
	if runes := []rune(value); len(runes) > size {
		return string(runes[:size])
	}

	return value
}

// handleFederationMetadataSearch — GET /api/v1/federation/metadata/{consoleId}/search?gameId=&q=&kind=.
func (server *Server) handleFederationMetadataSearch(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	query := r.URL.Query()
	kind := query.Get("kind")
	if kind != "" && !metadata.IsImageKind(kind) {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "unknown image kind")
		return
	}
	games, roots := server.lentGames(r.Context(), consoleID)
	answer := metadata.SearchLent(r.Context(), metadata.LentSearch{
		ConsoleID: consoleID, GameID: limited(query.Get("gameId"), 64), Query: limited(query.Get("q"), 200), Kind: kind,
		Games: games, Accounts: server.metadataAccounts(), LendProviders: grants(r, scopeMetadataProviders),
	})
	if !answer.Ready {
		for _, root := range roots {
			warmGameKeys(root, consoleID)
		}
	}

	writeJSON(w, http.StatusOK, answer)
}

// handleFederationProviderGame — GET /api/v1/federation/metadata/{consoleId}/providers/{service}/{externalId}.
func (server *Server) handleFederationProviderGame(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	sheet, err := metadata.LendProviderGame(r.Context(), server.metadataAccounts(), consoleID, chi.URLParam(r, "service"), chi.URLParam(r, "externalId"))
	if err != nil {
		writeMetadataError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sheet)
}

// metadataPeerOf — un serveur de mon registre, source de mes fiches : son client, avec le lien et le jeton du registre.
func metadataPeerOf(entry registeredServer) (metadata.Peer, error) {
	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		return metadata.Peer{}, err
	}

	return metadata.Peer{ID: entry.ID, Name: firstNonEmpty(entry.Hostname, entry.Link), Link: client.Link(), Client: client}, nil
}

// metadataPeers — mes serveurs fédérés, dans l'ordre du registre, pour la recherche d'images ; aucun quand le registre
// ne se lit pas (la recherche chez les fournisseurs ne l'attend pas).
func (server *Server) metadataPeers() []metadata.Peer {
	entries, err := server.registeredServers()
	if err != nil {
		slog.Warn("image search: the server registry cannot be read", "error", err)
		return nil
	}
	peers := []metadata.Peer{}
	for _, entry := range entries {
		if peer, err := metadataPeerOf(entry); err == nil {
			peers = append(peers, peer)
		}
	}

	return peers
}

// metadataPeerNamed — le serveur `raw` de mon registre (son identifiant), source d'un geste ; faux après avoir répondu.
func (server *Server) metadataPeerNamed(w http.ResponseWriter, raw string) (metadata.Peer, registeredServer, bool) {
	entry, found := server.registeredByIDValue(w, raw)
	if !found {
		return metadata.Peer{}, entry, false
	}
	peer, err := metadataPeerOf(entry)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_unreachable", "this federated server has no usable link")
		return metadata.Peer{}, entry, false
	}

	return peer, entry, true
}

// handleFederatedSheetCopy — POST /api/v1/retrogaming/{consoleId}/roms/{ref}/metadata/copy?server=<id> : COPIER LA
// FICHE D'UN JEU FÉDÉRÉ DANS MES MÉTADONNÉES, SANS LE JEU (le porteur, 2026-11-16, 2.4.1 : « dans consoles / jeux
// (depuis serveur fédéré) avoir dans section more : télécharger les métadonnées dans la librairie »). La même copie
// qu'un jeu téléchargé emporte (copyPeerSheet, metadata.FetchPeerSource) : son texte, la source de chaque champ, ses
// images, dans la source `server` de `metadata/<console>/<gameid>/` — au dossier du serveur, où vivent celles d'un jeu
// de la bibliothèque. Le `<gameid>` est celui que le pair lit dans le contenu du jeu (le même jeu porte le même sur
// toutes les machines), sinon sa référence : le jeu téléchargé plus tard retrouve sa fiche ; d'ici là, elle est une
// des « Métadonnées sans jeu » de la console. Un jeu déjà ici est refusé (409 `already_here`) : sa fiche se règle sur
// sa propre page (« Ce n'est pas le bon jeu ? »), dans les racines qui sont les siennes.
func (server *Server) handleFederatedSheetCopy(w http.ResponseWriter, r *http.Request) {
	entry, consoleID, peerReference, federated, answered := server.federatedPeer(w, r)
	if answered {
		return
	}
	if !federated {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "only a federated game has a sheet to copy: srv<id>:<reference>, with ?server=<id>")
		return
	}
	if server.gamesHere(r.Context(), consoleID)[peerReference] {
		writeErrorCode(w, http.StatusConflict, "already_here", "this game is already here: its sheet is set on its own page")
		return
	}
	peer, err := metadataPeerOf(entry)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_unreachable", "this federated server has no usable link")
		return
	}
	var info metadata.GameInfo
	if err := peer.Client.GameMetadata(r.Context(), consoleID, peerReference, &info); err != nil {
		switch {
		case errors.Is(err, fs.ErrPermission):
			err = fmt.Errorf("%s: %w", peer.Name, metadata.ErrPeerDenied)
		case errors.Is(err, fs.ErrNotExist):
			err = fmt.Errorf("%s: %w", peer.Name, metadata.ErrPeerNotLending)
		}
		writeMetadataError(w, err)
		return
	}
	gameID := firstNonEmpty(info.GameID, peerReference)
	// Le titre du jeu chez lui, que la copie de son listing annonce.
	name := ""
	if game, gameErr := server.copies().Game(peerOf(entry), consoleID, peerReference); gameErr == nil {
		name = game.Name
	}
	// UN CHEMIN QUI N'EXISTE PAS : le jeu n'est pas ici, et son dossier se nomme alors par la clé donnée (GameKey).
	state, err := metadata.FetchPeerSource(r.Context(), server.gameMetadataRoots(nil).Game, peer, consoleID, "", gameID, peerReference, name)
	if err != nil {
		writeMetadataError(w, err)
		return
	}
	slog.Info("federated game: its sheet was copied without the game", "server", entry.Hostname, "console", consoleID, "gameId", gameID)

	writeJSON(w, http.StatusOK, map[string]any{"gameId": gameID, "source": state})
}

// copyPeerSheet — UN JEU TÉLÉCHARGÉ D'UN PAIR EMPORTE SA FICHE (décision 10 : « un jeu TÉLÉCHARGÉ d'un pair emporte sa
// fiche, copiée comme si on l'avait désignée ») : à la fin de son téléchargement dans la bibliothèque, sa fiche se
// copie dans la source `server` du jeu arrivé, avec ses images. Une copie qui échoue ne fait pas échouer le
// téléchargement : le journal le dit. Indépendante de la requête, qui peut s'en aller : une minute au plus.
func (server *Server) copyPeerSheet(ctx context.Context, entry registeredServer, consoleID, gamePath, reference, peerReference, title string) {
	peer, err := metadataPeerOf(entry)
	if err == nil {
		copying, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		_, err = metadata.FetchPeerSource(copying, server.gameMetadataRoots(nil).Game, peer, consoleID, gamePath, reference, peerReference, title)
	}
	if err != nil {
		slog.Warn("downloaded game: its sheet was not copied from its server", "server", entry.Hostname, "console", consoleID, "game", title, "error", err)

		return
	}
	slog.Info("downloaded game: its sheet was copied from its server", "server", entry.Hostname, "console", consoleID, "game", title)
}
