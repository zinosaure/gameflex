package api

// LE TEXTE DE LA FICHE D'UN JEU (chantier 49, #659, §7) —
// GET /api/v1/retrogaming/{consoleId}/roms/{ref}/metadata.
//
// `{gameId, title?, releaseDate, publishers, developers, genres, series, sources}` : un champ
// inconnu est absent, et un jeu que personne ne connaît rend son seul
// `gameId`. `sources` dit, champ par champ, quel service l'a donné
// (`libretro-database`, `igdb`) ; un champ corrigé n'en cite aucun, et la fiche
// ne dit jamais qu'un texte ou une image a été mis « à la main » (le porteur,
// 2026-11-14 : un administrateur peut toujours changer le dossier metadata
// sans passer par l'interface — l'information serait fausse). `title` n'y est
// que corrigé. 404 seulement quand le jeu n'existe pas,
// 400 pour une console inconnue. La fusion est dans
// gameflex/kernel/internal/metadata (info.go).
//
// Route gardée, comme les images du jeu (game_images.go).
//
// LA FICHE D'UN JEU FÉDÉRÉ SE LIT CHEZ SON SERVEUR (chantier 57, P2) : sa
// référence `srv<id>:<ref>` et `?server=<id>` la font relayer telle quelle
// (federation_metadata.go) ; ses gestes ci-dessous sont refusés, 409
// `federated_game` — elle se corrige chez lui.
//
// LES GESTES DE L'ADMINISTRATEUR (§7, Réglages › Jeux du launcher), sur le
// même chemin et le même jeu (metadataGame : `?disk=` désigne le disque) :
//
//   - PUT    …/metadata/manual                 le texte corrigé, champ par champ
//   - GET    …/metadata/images/search?kind=&q= les images d'une sorte que
//     proposent IGDB, SteamGridDB et libretro, puis mes serveurs fédérés
//     (chantier 57, P3 : leurs jeux du même gameId, puis du titre) ; `q` vide :
//     le titre du jeu (chantier 53, P9 ; metadata/image_search.go)
//   - PUT    …/metadata/manual/images/{kind}   {source, url} : une image
//     choisie dans une recherche, récupérée chez l'hôte d'images de sa source
//     seulement — l'envoi d'un fichier est parti (remarque d du porteur) ; ou
//     {source: "server", server, reference} : celle d'un jeu d'un serveur
//     fédéré, récupérée chez lui par le lien et le jeton du registre, jamais
//     une adresse — une image seule ne se retire pas, elle se remplace (le
//     porteur, 2026-11-14 : « On ne peut que la remplacer. »)
//   - GET    …/metadata/candidates?source=igdb|steamgriddb|server:<id>&q=
//   - POST   …/metadata/fetch                  {source, externalId?} ; chez un
//     serveur fédéré (`server:<id>`), la COPIE de la fiche d'un de ses jeux
//     dans la source `server` (metadata.FetchPeerSource)
//   - DELETE …/metadata?source=igdb|steamgriddb|libretro|server  oublier une source
//   - DELETE …/metadata?source=manual          RÉINITIALISER la fiche : les
//     textes corrigés et les images remplacées partent d'un coup (le porteur,
//     2026-11-14 : « Le bouton réinitialiser permet de tout remettre comme
//     avant. ») ; ses images seules ne se remettent plus à part
//
// LES MÉTADONNÉES D'UN JEU VIVENT AVEC LUI : celles d'un jeu d'un disque
// interne sur ce disque (`<disque>/metadata/<console>/<gameid>/`), celles
// d'un jeu de la bibliothèque dans le dossier du serveur (le porteur,
// 2026-11-13) — sauf quand l'interrupteur « Écrire les métadonnées sur le
// disque du jeu » est coupé (2026-11-14) : celles d'un jeu d'un disque vivent
// alors au dossier du serveur (gameMetadataRoots, le seul point de choix).
//
// Le détail est dans gameflex/kernel/internal/metadata (game_admin.go).

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
)

// L'INTERRUPTEUR « ÉCRIRE LES MÉTADONNÉES SUR LE DISQUE DU JEU » (le porteur,
// 2026-11-14 : « Sur téléphone, quand on rajoute un disque NTFS, le disque
// devient read-only […] rajouter un switch qui demande si on veut écrire les
// metadata sur le même disque où le jeu se trouve. Par défaut c'est checked. »)
// — gardé par le service d'identité avec les comptes des services, lu par sa
// route interne, et gardé ici dix secondes : une grille de jaquettes ne le
// redemande pas à chaque image, et le geste de l'administrateur vaut presque
// aussitôt. Sans service d'identité, ou s'il ne répond pas : la dernière
// valeur lue, sinon allumé (le défaut).
var metadataSettingsCache struct {
	sync.Mutex
	source          string
	writeOnGameDisk bool
	fetchedAt       time.Time
}

const metadataSettingsAge = 10 * time.Second

func (server *Server) metadataOnGameDisk() bool {
	metadataSettingsCache.Lock()
	defer metadataSettingsCache.Unlock()
	source := server.config.CredentialURL
	known := metadataSettingsCache.source == source && !metadataSettingsCache.fetchedAt.IsZero()
	if known && time.Since(metadataSettingsCache.fetchedAt) < metadataSettingsAge {
		return metadataSettingsCache.writeOnGameDisk
	}
	fallback := !known || metadataSettingsCache.writeOnGameDisk
	if source == "" || server.config.InternalSecret == "" {
		return true
	}
	request, err := http.NewRequest(http.MethodGet, source+"/internal/metadata/settings", nil)
	if err != nil {
		return fallback
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		slog.Warn("metadata settings: identity service unreachable", "error", err)
		return fallback
	}
	defer response.Body.Close()
	var envelope struct {
		Item struct {
			WriteOnGameDisk *bool `json:"write_on_game_disk"`
		} `json:"item"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&envelope) != nil || envelope.Item.WriteOnGameDisk == nil {
		slog.Warn("metadata settings: cannot read them", "status", response.Status)
		return fallback
	}
	metadataSettingsCache.source, metadataSettingsCache.writeOnGameDisk, metadataSettingsCache.fetchedAt = source, *envelope.Item.WriteOnGameDisk, time.Now()

	return metadataSettingsCache.writeOnGameDisk
}

// gameMetadataRoots — LE SEUL POINT où se choisit la racine des métadonnées
// d'un jeu (metadata.Roots.Game), où elles se lisent ET s'écrivent : la fiche,
// les images, embedded/, les corrections, la réinitialisation et l'oubli d'une
// source passent tous par metadataGame, qui l'appelle. Un jeu de la
// bibliothèque (`disk` nil) : le dossier du serveur. Un jeu d'un disque
// interne : ce disque, interrupteur allumé ; le dossier du serveur, coupé (le
// porteur, 2026-11-14 : « si le Switch est off alors les metadata s'écrit dans
// le dossier principal gameflex/metadata »). Le dossier du jeu s'y nomme par
// son contenu (GameKey) : déjà là, il sert ; absent, il se crée.
func (server *Server) gameMetadataRoots(disk *docker.InternalDisk) metadata.Roots {
	roots := metadata.Roots{Server: server.storageRoot, Game: server.storageRoot}
	if disk != nil && server.metadataOnGameDisk() {
		roots.Game = disk.Dir
	}

	return roots
}

// metadataGame — la console, la référence et le fichier du jeu de l'adresse,
// et les racines de ses métadonnées (gameMetadataRoots : la bibliothèque, ou
// le disque interne qui le porte, selon l'interrupteur).
// `?disk=<nom>` désigne le disque ; sans lui, le jeu est cherché dans la
// bibliothèque, puis les disques, comme pour le lancer. Faux après avoir
// répondu (400 console inconnue, 404 jeu ou disque absent, 409 un jeu
// fédéré : sa fiche se corrige chez son serveur — la lire passe par le relais,
// avant d'arriver ici).
func (server *Server) metadataGame(w http.ResponseWriter, r *http.Request) (consoleID, reference, gamePath string, roots metadata.Roots, ok bool) {
	if _, _, federated, _ := federatedGame(r); federated {
		writeErrorCode(w, http.StatusConflict, "federated_game", "the metadata of a federated game are corrected on its server")
		return "", "", "", roots, false
	}
	consoleID, reference = chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return "", "", "", roots, false
	}

	var err error
	if name := r.URL.Query().Get("disk"); name != "" {
		disk, found := server.diskNamed(w, r, name)
		if !found {
			return "", "", "", roots, false
		}
		gamePath, _, err = retro_files.ResolveCatalogRom(disk.RomsDir(), consoleID, reference)
		roots = server.gameMetadataRoots(&disk)
	} else {
		var disk *docker.InternalDisk
		gamePath, _, disk, err = server.findRom(r.Context(), consoleID, reference)
		roots = server.gameMetadataRoots(disk)
	}
	if errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "game_not_found", "rom not found")
		return "", "", "", roots, false
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid rom reference: "+err.Error())
		return "", "", "", roots, false
	}

	return consoleID, reference, gamePath, roots, true
}

func (server *Server) handleGameMetadata(w http.ResponseWriter, r *http.Request) {
	// Un jeu fédéré : sa fiche se lit chez son serveur (federation_metadata.go).
	if server.relayFederatedSheet(w, r) {
		return
	}
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, metadata.Info(r.Context(), roots, server.metadataAccounts(), consoleID, gamePath, reference))
}

// writeMetadataError — les refus communs aux gestes sur la fiche (metadata.Refusal).
func writeMetadataError(w http.ResponseWriter, err error) {
	writeRefusal(w, metadata.Refusal(err), "game metadata", err)
}

// writeRefusal — un refus du module des métadonnées, dans l'enveloppe du kernel ; une erreur qui n'est pas un refus
// connu se note au journal sous `what`.
func writeRefusal(w http.ResponseWriter, refused metadata.Refused, what string, err error) {
	if refused.Unknown {
		slog.Warn(what, "error", err)
	}
	if refused.Code != "" {
		writeErrorCode(w, refused.Status, refused.Code, refused.Message)
		return
	}
	writeErrorCode(w, refused.Status, codeOr(refused.Code, "metadata_failed"), refused.Message)
}

// handleGameMetadataManual — PUT …/metadata/manual : un objet JSON dont
// chaque champ présent est appliqué (null ou vide l'efface). Rend la fiche.
func (server *Server) handleGameMetadataManual(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	var changes map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&changes); err != nil || changes == nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a JSON object is expected")
		return
	}
	if err := metadata.SetManualText(roots.Game, consoleID, gamePath, reference, changes); err != nil {
		if errors.Is(err, metadata.ErrInvalidManual) {
			writeMetadataError(w, err)
			return
		}
		writeErrorCode(w, http.StatusInternalServerError, "metadata_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, metadata.Info(r.Context(), roots, server.metadataAccounts(), consoleID, gamePath, reference))
}

// handleGameMetadataManualImage — PUT …/metadata/manual/images/{kind}
// {source, url} ou {source: "server", server, reference} : l'image choisie
// remplace celle de cette sorte.
func (server *Server) handleGameMetadataManualImage(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	content, read := server.readGameChosenImage(w, r, consoleID)
	if !read {
		return
	}
	if err := metadata.SetManualImage(roots.Game, consoleID, gamePath, reference, chi.URLParam(r, "kind"), content); err != nil {
		writeMetadataError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGameImageSearch — GET …/metadata/images/search?kind=&q= : `q` vide,
// le titre du jeu (sans ses mentions), comme les candidats. Mes serveurs
// fédérés répondent après les fournisseurs, par le gameid du jeu puis le titre.
func (server *Server) handleGameImageSearch(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	title := strings.TrimSpace(query.Get("q"))
	if title == "" {
		title = metadata.SearchTitle(gamePath)
	}

	found, err := metadata.SearchGameImages(r.Context(), filepath.Join(roots.Server, "cache", "metadata", "sources"), server.metadataAccounts(), consoleID,
		query.Get("kind"), title, metadata.GameKey(gamePath, reference), server.metadataPeers())
	switch {
	case errors.Is(err, metadata.ErrUnknownKind):
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "unknown image kind")
	case errors.Is(err, metadata.ErrNotAGameFolder):
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "not a game metadata folder")
	case errors.Is(err, metadata.ErrEmptyQuery):
		writeErrorCode(w, http.StatusBadRequest, "metadata_failed", err.Error())
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "metadata_failed", err.Error())
	default:
		writeJSON(w, http.StatusOK, found)
	}
}

// handleGameMetadataCandidates — GET …/metadata/candidates?source=&q= :
// `q` vide, le titre du jeu. `source=server:<id>` : les jeux d'un de mes
// serveurs fédérés (et ceux de ses fournisseurs, s'il les partage).
func (server *Server) handleGameMetadataCandidates(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, _, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()

	var candidates []metadata.Candidate
	var err error
	if serverID, fromServer := strings.CutPrefix(query.Get("source"), "server:"); fromServer {
		peer, _, found := server.metadataPeerNamed(w, serverID)
		if !found {
			return
		}
		candidates, err = metadata.PeerCandidates(r.Context(), peer, consoleID, gamePath, reference, query.Get("q"))
	} else {
		candidates, err = metadata.Candidates(r.Context(), server.metadataAccounts(), consoleID, gamePath, query.Get("source"), query.Get("q"))
	}
	if err != nil {
		writeMetadataError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": candidates})
}

// handleGameMetadataFetch — POST …/metadata/fetch {source, externalId?}.
func (server *Server) handleGameMetadataFetch(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	var body struct {
		Source     string `json:"source"`
		ExternalID string `json:"externalId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a JSON object {source, externalId?} is expected")
		return
	}

	var state metadata.SourceState
	var err error
	if serverID, fromServer := strings.CutPrefix(strings.TrimSpace(body.Source), "server:"); fromServer {
		peer, entry, found := server.metadataPeerNamed(w, serverID)
		if !found {
			return
		}
		// Le titre du jeu chez lui, que la copie de son listing annonce (un jeu de ses fournisseurs n'y est pas).
		name := ""
		if game, gameErr := server.copies().Game(peerOf(entry), consoleID, strings.TrimSpace(body.ExternalID)); gameErr == nil {
			name = game.Name
		}
		state, err = metadata.FetchPeerSource(r.Context(), roots.Game, peer, consoleID, gamePath, reference, body.ExternalID, name)
	} else {
		state, err = metadata.FetchSource(r.Context(), roots.Game, server.metadataAccounts(), consoleID, gamePath, reference, strings.TrimSpace(body.Source), body.ExternalID)
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "unknown_game_id", "the service has no game with this id")
	case err != nil && state.Source == "":
		writeMetadataError(w, err)
	default:
		// Une recherche qui échoue est gardée comme telle : l'état le dit.
		writeJSON(w, http.StatusOK, state)
	}
}

// handleGameMetadataDrop — DELETE …/metadata?source=.
func (server *Server) handleGameMetadataDrop(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	if err := metadata.DropSource(roots.Game, consoleID, gamePath, reference, r.URL.Query().Get("source")); err != nil {
		writeMetadataError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
