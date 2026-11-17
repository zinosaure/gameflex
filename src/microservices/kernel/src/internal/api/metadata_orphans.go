package api

// LES MÉTADONNÉES SANS JEU (le porteur, 2026-11-14 : « on peut se retrouver avec des metadata des jeux qu'on n'a plus
// accès. Ce qui fera des métadata orphelin. Faudra peut être rajouter dans console […] le lien vers games without
// metadata. Cette page listera que les metadata qui non pas jeux et quand on clique dessus ça propose de le supprimer
// avec confirmation. ») — Réglages › Consoles › une console › « Métadonnées sans jeu » :
//
//   - GET    /api/v1/retrogaming/{consoleId}/metadata/orphans                 les dossiers
//     `metadata/<console>/<gameid>/` du dossier du serveur ET de chaque disque interne présent dont aucun jeu
//     accessible (la bibliothèque, les disques présents) ne porte ce gameid (GameKey) : `{orphans: [{gameId, disk,
//     title, cover}]}` — `disk` vide pour le dossier du serveur ;
//   - GET    …/metadata/orphans/{gameId}/cover?disk=                          sa jaquette, s'il en garde une ;
//   - DELETE …/metadata/orphans/{gameId}?disk=                                le retirer.
//
// GARDE-FOUS : jamais un dossier par console (le fond de sa bande, `_console/` : metadata.IsGameFolderName) ; un
// gameid qui n'est qu'un nom simple ; une racine d'une liste fermée (le dossier du serveur, ou un disque déclaré et
// présent : diskNamed) ; la suppression REVÉRIFIE au moment même que le dossier est orphelin (409 sinon, rien
// retiré) ; une liste des jeux pas encore lue refuse (503) plutôt que de faire passer pour orphelin un jeu
// momentanément absent ; un disque en lecture seule le dit (writeMetadataError). Un administrateur seulement.
//
// SANS ATTENDRE LA LECTURE DES JEUX (chantier 53, 2026-11-14). Le nom d'un dossier est l'empreinte du CONTENU d'un jeu
// (metadata.GameKey) ; le listing et le catalogue que lisent les écrans ne gardent que sa référence, tirée de son nom.
// Ces empreintes ne vivent qu'en mémoire, une à une à mesure qu'une jaquette les demande, et se perdent à chaque
// redémarrage du kernel : la première vérification relisait toutes les ROM de la console DANS la requête — 101 s pour
// les 10 785 ROM NES de Gamebox, une suppression sans réponse pendant ce temps — et, même toutes connues, relisait les
// dates des ~200 dossiers du disque pour chaque jeu (5 s). La vérification ne lit plus que ce qui est en mémoire
// (retro_files.CatalogPaths, metadata.KnownGameKey) ; ce qui manque se lit en fond (warmGameKeys) et la requête répond
// 503 `games_not_ready` aussitôt — et la page s'abonne au sujet `metadata-orphans/<console>`, qui lui pousse l'instant
// où les empreintes sont lues (#751 : elle relisait la liste toutes les quatre secondes).
//
// UN DOSSIER SOUS LA RÉFÉRENCE D'UN JEU PRÉSENT N'EST PAS ORPHELIN (2026-11-15) : son jeu a désormais un identifiant lu
// dans son contenu (un jeu Switch, dans son paquet), et ses métadonnées le rejoignent — la page le déplace sous ce
// gameid avant de juger (metadata.MoveReferenceFolder), sur le dossier du serveur comme sur chaque disque ; une
// suppression demandée avant ce déplacement le fait, au lieu de retirer le dossier (409). Seul celui dont le gameid a
// DÉJÀ son dossier reste, et paraît orphelin : plus aucun jeu ne le porte, celui du gameid sert ; l'administrateur voit
// sa jaquette et le retire.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
)

type metadataOrphan struct {
	GameID string `json:"gameId"`
	Disk   string `json:"disk,omitempty"`
	Title  string `json:"title,omitempty"`
	Cover  bool   `json:"cover"`
}

// metadataPlace — une racine où chercher des métadonnées : le dossier du serveur (`disk` vide), ou un disque présent.
type metadataPlace struct {
	disk, root string
}

func (server *Server) metadataPlaces(ctx context.Context) []metadataPlace {
	places := []metadataPlace{{root: server.storageRoot}}
	for _, disk := range server.presentDisks(ctx) {
		places = append(places, metadataPlace{disk: disk.Name, root: disk.Dir})
	}

	return places
}

// accessibleGameKeys — les gameid (GameKey) des jeux de la console qu'on peut lancer d'ici : la bibliothèque et les
// disques présents, tels que la mémoire les connaît — sans rien lire au-delà d'un `stat` par jeu. Faux quand une de
// leurs listes n'est pas lue, ou pas à jour, ou qu'un de leurs jeux n'a pas encore son empreinte pour son contenu
// d'aujourd'hui : rien ne se juge orphelin alors, et ce qui manque se lit en fond (warmGameKeys).
//
// Un jeu-DOSSIER (un vidage Switch, PS3, PS4, PS5) ne fait pas exception : CatalogPaths donne son dossier, warmGameKeys
// en lit l'empreinte (l'identifiant de titre de `sce_sys/` ou de l'ISO PS3 qu'il range, sinon sa référence) et
// KnownGameKey la retrouve sous la date du dossier — un fichier ajouté dedans, ou le dossier renommé, et elle se relit.
// Tant qu'il en manque une : 503, jamais une liste vide (TestOrphansOfFolderGamesWaitForTheirKeys).
//
// `references` : le gameid de chaque jeu, par sa référence (metadata.IsOrphan).
func (server *Server) accessibleGameKeys(ctx context.Context, consoleID string) (keys map[string]bool, references map[string]string, ready bool) {
	keys, references = map[string]bool{}, map[string]string{}
	roots := []string{server.retroRomsRoot()}
	for _, disk := range server.presentDisks(ctx) {
		roots = append(roots, disk.RomsDir())
	}
	ready = true
	for _, root := range roots {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if paths, current := retro_files.CatalogPaths(root, consoleID); current && metadata.KnownGameKeys(paths, keys, references) {
			continue
		}
		// Les autres racines se vérifient quand même : leur lecture part en même temps.
		ready = false
		warmGameKeys(root, consoleID)
	}
	if !ready {
		return nil, nil, false
	}

	return keys, references, true
}

// LES EMPREINTES EN COURS DE LECTURE, par racine et console : une seule lecture à la fois de chacune, quelle que soit
// la page qui la demande. gameKeysRead réveille, à la fin de chacune, les pages abonnées à
// `metadata-orphans/<console>` (facade_ws.go).
var (
	gameKeysMu      sync.Mutex
	gameKeysWarming = map[string]bool{}
	gameKeysRead    = map[chan struct{}]bool{}
)

// watchGameKeys rend un signal réveillé à la fin de chaque lecture d'empreintes, et de quoi s'en défaire.
func watchGameKeys() (chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	gameKeysMu.Lock()
	gameKeysRead[wake] = true
	gameKeysMu.Unlock()

	return wake, func() {
		gameKeysMu.Lock()
		delete(gameKeysRead, wake)
		gameKeysMu.Unlock()
	}
}

// warmGameKeys — en fond : le catalogue de `root` construit (ou refait), puis l'empreinte de chaque jeu de la console,
// gardée en mémoire (metadata.GameKey). La vérification suivante la trouve.
func warmGameKeys(root, consoleID string) {
	id := root + "\x00" + consoleID
	gameKeysMu.Lock()
	if gameKeysWarming[id] {
		gameKeysMu.Unlock()
		return
	}
	gameKeysWarming[id] = true
	gameKeysMu.Unlock()

	go func() {
		defer func() {
			gameKeysMu.Lock()
			delete(gameKeysWarming, id)
			for wake := range gameKeysRead {
				select {
				case wake <- struct{}{}:
				default:
				}
			}
			gameKeysMu.Unlock()
		}()
		started := time.Now()
		if _, err := retro_files.ListCatalog(root, consoleID, ""); err != nil {
			slog.Warn("game keys not read", "root", root, "console", consoleID, "error", err)
			return
		}
		paths, _ := retro_files.CatalogPaths(root, consoleID)
		for reference, path := range paths {
			metadata.GameKey(path, reference)
		}
		slog.Info("game keys read", "root", root, "console", consoleID, "games", len(paths), "duration", time.Since(started))
	}()
}

// handleMetadataOrphans — GET …/{consoleId}/metadata/orphans.
func (server *Server) handleMetadataOrphans(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	type found struct {
		place  metadataPlace
		folder metadata.GameFolder
	}
	candidates := []found{}
	for _, place := range server.metadataPlaces(r.Context()) {
		folders, err := metadata.GameFolders(place.root, consoleID)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "metadata_failed", err.Error())
			return
		}
		for _, folder := range folders {
			candidates = append(candidates, found{place: place, folder: folder})
		}
	}
	orphans := []metadataOrphan{}
	// Sans dossier, rien à comparer : la liste des jeux (des milliers d'empreintes) ne se lit pas pour rien.
	if len(candidates) > 0 {
		keys, references, ready := server.accessibleGameKeys(r.Context(), consoleID)
		if !ready {
			writeErrorCode(w, http.StatusServiceUnavailable, "games_not_ready", "the game list is still being read: try again in a moment")
			return
		}
		for _, candidate := range candidates {
			if !metadata.IsOrphan(candidate.place.root, consoleID, candidate.folder.GameID, keys, references) {
				continue
			}
			orphans = append(orphans, metadataOrphan{GameID: candidate.folder.GameID, Disk: candidate.place.disk, Title: candidate.folder.Title, Cover: candidate.folder.Cover != ""})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"orphans": orphans})
}

// orphanPlace — la console, le gameid et la place de l'adresse, validés : un gameid simple, une racine de la liste
// fermée. Faux après avoir répondu.
func (server *Server) orphanPlace(w http.ResponseWriter, r *http.Request) (consoleID, gameID string, place metadataPlace, ok bool) {
	consoleID, gameID = chi.URLParam(r, "consoleId"), chi.URLParam(r, "gameId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return "", "", place, false
	}
	if !metadata.IsGameFolderName(gameID) {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "not a game metadata folder")
		return "", "", place, false
	}
	place = metadataPlace{root: server.storageRoot}
	if name := r.URL.Query().Get("disk"); name != "" {
		disk, found := server.diskNamed(w, r, name)
		if !found {
			return "", "", place, false
		}
		place = metadataPlace{disk: disk.Name, root: disk.Dir}
	}

	return consoleID, gameID, place, true
}

// handleMetadataOrphanCover — GET …/metadata/orphans/{gameId}/cover?disk= : la jaquette que garde le dossier.
func (server *Server) handleMetadataOrphanCover(w http.ResponseWriter, r *http.Request) {
	consoleID, gameID, place, ok := server.orphanPlace(w, r)
	if !ok {
		return
	}
	folder, err := metadata.GameFolderAt(place.root, consoleID, gameID)
	if err != nil || folder.Cover == "" {
		writeErrorCode(w, http.StatusNotFound, "no_image", "no cover")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, folder.Cover)
}

// handleMetadataOrphanDelete — DELETE …/metadata/orphans/{gameId}?disk= : revérifié au moment même.
func (server *Server) handleMetadataOrphanDelete(w http.ResponseWriter, r *http.Request) {
	consoleID, gameID, place, ok := server.orphanPlace(w, r)
	if !ok {
		return
	}
	if _, err := metadata.GameFolderAt(place.root, consoleID, gameID); err != nil {
		writeMetadataError(w, err)
		return
	}
	keys, references, ready := server.accessibleGameKeys(r.Context(), consoleID)
	if !ready {
		writeErrorCode(w, http.StatusServiceUnavailable, "games_not_ready", "the game list is still being read: try again in a moment")
		return
	}
	if !metadata.IsOrphan(place.root, consoleID, gameID, keys, references) {
		writeErrorCode(w, http.StatusConflict, "game_present", "a game of this console carries these metadata again: they are kept")
		return
	}
	if err := metadata.RemoveGameFolder(place.root, consoleID, gameID); err != nil {
		writeMetadataError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
