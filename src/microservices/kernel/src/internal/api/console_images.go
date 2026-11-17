package api

// L'IMAGE D'UNE CONSOLE (chantier 49, #659, pour le launcher #660) :
//
//   - GET    /api/v1/retrogaming/consoles/{consoleId}/image?w=<largeur>
//   - GET    /api/v1/retrogaming/consoles/{consoleId}/image/source
//   - PUT    /api/v1/retrogaming/consoles/{consoleId}/image   (administrateur)
//   - DELETE /api/v1/retrogaming/consoles/{consoleId}/image   (administrateur)
//
// L'image est celle de l'administrateur, sinon l'icône de la console dans la
// collection de KyleBing (CC BY 4.0), gardée au premier appel
// (gameflex/kernel/internal/metadata, console_images.go). 404 quand la
// console n'en a pas (toute console streamex ou de jeu à distance, pour
// l'instant), 400 pour une console inconnue. La source est une route à part, et non des en-têtes : le
// launcher charge l'image par `fetch` puis `createObjectURL`, et lit la
// ligne d'attribution une fois, sans retenir la réponse de l'image.
//
// Pas encore d'écran pour déposer une image : le PUT et le DELETE n'ont que
// l'API.
//
// LE FOND DE SON ÉCRAN, des jaquettes des grands jeux de la console, en HUIT
// EMPLACEMENTS (console_backgrounds.go ; le porteur, 2026-11-14, remarque e :
// « gardons maximum 8 images par console ») :
//
//   - GET /api/v1/retrogaming/consoles/{consoleId}/backgrounds
//     → {"slots": [{"slot": 1, "name": "01.jpg"}, …, {"slot": 8, "name": ""}],
//     "extra": ["09.jpg"]} : les huit emplacements dans l'ordre (`name` vide :
//     libre), puis les images du dossier en trop, que la bande ne montre pas
//     et que rien n'efface ;
//   - GET /api/v1/retrogaming/consoles/{consoleId}/backgrounds/{name}?w=<largeur>
//
// Le premier appel de la liste lance le remplissage par défaut et l'attend
// quelques secondes ; ce qui arrive après est là à la visite suivante.
//
// UNE CONSOLE SANS AUCUNE IMAGE ICI MONTRE CELLES DU PREMIER SERVEUR FÉDÉRÉ
// (`#740`, metadata.ShownBackgrounds ; le porteur, 2026-11-16 : « il faut que
// par défaut les consoles prennent les images des serveurs fédérés si en local
// on n'a pas d'images background. On prend le premier fédéré ») : la liste
// porte alors `server` et `serverName`, et la page demande chaque image avec
// `?server=<id>` — le kernel la RELAIE de chez lui (remote.RelayConsoleBackground),
// rien ne s'écrit ici. Le pair les prête sous la portée `catalog` :
//
//   - GET /api/v1/federation/metadata/{consoleId}/backgrounds
//   - GET /api/v1/federation/metadata/{consoleId}/backgrounds/{name}?w=
//
// les siennes seulement, jamais celles qu'il tiendrait d'un autre.
//
// Et, pour un administrateur (Réglages › Consoles › une console ›
// Apparence) — une image se CHOISIT chez un fournisseur, elle ne s'envoie plus
// (remarque d du porteur ; metadata/image_search.go) :
//
//   - GET    …/backgrounds/search?q=        les images que proposent IGDB
//     (artworks), SteamGridDB (bandeaux) et libretro (jaquettes) pour ce titre
//     sur la console → {query, kind, items, sources} ;
//   - PUT    …/backgrounds/slots/{slot}     {source, url} : l'image choisie,
//     récupérée chez l'hôte d'images de sa source seulement, dans
//     l'emplacement 1 à 8 → 200 {"slot": 3, "name": "03.jpg"} ;
//   - POST   …/backgrounds/reset            retire `.seeded` : les images par
//     défaut reviennent à la visite suivante dans les emplacements libres,
//     sans toucher à celles qui sont là.
//
// L'envoi d'une image (POST …/backgrounds, PUT …/backgrounds/{name}) et le
// retrait par nom sont partis avec les emplacements ; vider un emplacement
// (DELETE …/backgrounds/slots/{slot}) aussi, le 2026-11-14 — une image ne se
// supprime pas seule, elle se remplace (le porteur : « Il est préférable de
// ne pas laisser le choix de supprimer une image. On ne peut que la
// remplacer. »).

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/metadata"
	"gameflex/storage/remote"
)

// consoleImageLimit — la taille d'une image déposée : une image de
// console tient largement en 16 Mo.
const consoleImageLimit = 16 << 20

// serveChangingImage — une image dont le contenu change SOUS LA MÊME ADRESSE :
// l'image d'une console, un emplacement de son fond, une image d'un jeu, qu'un
// administrateur remplace (chantier 53, P12 ; le porteur : « Plusieurs fois
// j'essaie de changer l'image de la PS5 en remote mais ça ne fonctionne pas. Ça
// retourne toujours à l'image Astro Bot »). Elles répondaient `max-age=86400` :
// le navigateur gardait l'ancienne un jour, et le numéro que la page ajoute
// après un remplacement (libs/images.js) ne survivait pas à un rechargement.
// Désormais `no-cache` et un ETag (la date de modification et la taille du
// fichier servi — une réduction refaite en change) : le navigateur redemande à
// chaque fois, et reçoit 304 tant que rien n'a changé, la nouvelle image sinon.
// http.ServeContent répond aux If-None-Match.
func serveChangingImage(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	}

	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func (server *Server) handleConsoleImage(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))

	path, err := metadata.ConsoleImage(r.Context(), server.storageRoot, consoleID, width)
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	case err != nil:
		slog.Warn("console image", "console", consoleID, "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}

	serveChangingImage(w, r, path)
}

func (server *Server) handleConsoleImageSource(w http.ResponseWriter, r *http.Request) {
	source, err := metadata.ConsoleImageAttribution(server.storageRoot, chi.URLParam(r, "consoleId"))
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// handleConsoleImageUpload — le corps de la requête EST l'image (PNG, JPEG
// ou WebP), reconnue à son contenu.
func (server *Server) handleConsoleImageUpload(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	content, ok := readConsoleImageBody(w, r)
	if !ok {
		return
	}
	extension := metadata.ImageExtension(content)
	if extension == "" {
		writeErrorCode(w, http.StatusUnsupportedMediaType, "image_format", "PNG, JPEG or WebP only")
		return
	}

	err := metadata.SetManualConsoleImage(server.storageRoot, consoleID, extension, content)
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	case errors.Is(err, metadata.ErrUnreadableImage):
		writeErrorCode(w, http.StatusUnsupportedMediaType, "images_failed", err.Error())
		return
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}
	server.handleConsoleImageSource(w, r)
}

func (server *Server) handleConsoleImageDelete(w http.ResponseWriter, r *http.Request) {
	err := metadata.RemoveManualConsoleImage(server.storageRoot, chi.URLParam(r, "consoleId"))
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// backgroundSeedWait — ce que la liste attend du premier remplissage : huit
// jaquettes de libretro arrivent en deux à quatre secondes.
const backgroundSeedWait = 8 * time.Second

// handleConsoleBackgrounds — GET …/backgrounds : les siennes, sinon celles du premier serveur fédéré qui en prête.
func (server *Server) handleConsoleBackgrounds(w http.ResponseWriter, r *http.Request) {
	slots, err := metadata.ShownBackgrounds(r.Context(), server.storageRoot, server.metadataAccounts(), chi.URLParam(r, "consoleId"), backgroundSeedWait, server.metadataPeers)
	if errors.Is(err, metadata.ErrUnknownConsole) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

// handleConsoleBackground — GET …/backgrounds/{name}?w= : l'image d'ici ; avec `?server=<id>`, celle du serveur
// fédéré qui prête le fond (la liste le nomme), relayée telle quelle.
func (server *Server) handleConsoleBackground(w http.ResponseWriter, r *http.Request) {
	if raw := r.URL.Query().Get("server"); raw != "" {
		server.relayConsoleBackground(w, r, raw, chi.URLParam(r, "consoleId"), chi.URLParam(r, "name"))
		return
	}

	server.serveConsoleBackground(w, r)
}

// serveConsoleBackground — l'image `{name}` du fond d'ici, à la largeur `?w=` ; c'est aussi celle que ce serveur prête
// (GET /api/v1/federation/metadata/{consoleId}/backgrounds/{name}), sans `?server=` : un ami ne fait pas lire ce
// serveur chez SES pairs.
func (server *Server) serveConsoleBackground(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))

	path, err := metadata.ConsoleBackground(server.storageRoot, consoleID, chi.URLParam(r, "name"), width)
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	case err != nil:
		slog.Warn("console background", "console", consoleID, "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}

	serveChangingImage(w, r, path)
}

// relayConsoleBackground — l'image `name` du fond d'une console chez le serveur fédéré `raw` de mon registre : 400 une
// console inconnue, 404 un nom qui n'est pas celui d'une image du fond (rien ne part chez lui), puis les refus du relais
// (403, 404, 502).
func (server *Server) relayConsoleBackground(w http.ResponseWriter, r *http.Request, raw, consoleID, name string) {
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	if !metadata.ValidBackgroundName(name) {
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	}
	entry, found := server.registeredByIDValue(w, raw)
	if !found {
		return
	}

	server.answerRelay(w, entry, consoleID, remote.RelayConsoleBackground(w, r, peerOf(entry), consoleID, name))
}

// handleFederationConsoleBackgrounds — GET /api/v1/federation/metadata/{consoleId}/backgrounds : LE FOND QUE CE
// SERVEUR PRÊTE (`#740`), le sien seulement — son remplissage par défaut compris, jamais celui d'un autre serveur.
func (server *Server) handleFederationConsoleBackgrounds(w http.ResponseWriter, r *http.Request) {
	slots, err := metadata.ConsoleBackgrounds(server.storageRoot, server.metadataAccounts(), chi.URLParam(r, "consoleId"), backgroundSeedWait)
	if errors.Is(err, metadata.ErrUnknownConsole) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

// readConsoleImageBody — le corps d'un dépôt : 16 Mo au plus. Faux après
// avoir répondu 413.
func readConsoleImageBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, consoleImageLimit))
	if err != nil {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "image_too_large", "image larger than 16 MiB")
		return nil, false
	}

	return content, true
}

// writeBackgroundError — les refus communs aux gestes sur le fond (metadata.BackgroundRefusal).
func writeBackgroundError(w http.ResponseWriter, err error) {
	writeRefusal(w, metadata.BackgroundRefusal(err), "chosen image", err)
}

// backgroundSlot — l'emplacement de l'adresse, 1 à 8. Faux après avoir
// répondu 400.
func backgroundSlot(w http.ResponseWriter, r *http.Request) (int, bool) {
	slot, err := strconv.Atoi(chi.URLParam(r, "slot"))
	if err != nil || slot < 1 || slot > metadata.BackgroundSlotCount {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a background slot is 1 to 8")
		return 0, false
	}

	return slot, true
}

// handleConsoleBackgroundSearch — GET …/backgrounds/search?q= : `q` est
// demandé (la page cherche ce qu'on tape).
func (server *Server) handleConsoleBackgroundSearch(w http.ResponseWriter, r *http.Request) {
	found, err := metadata.SearchBandImages(r.Context(), filepath.Join(server.storageRoot, "cache", "metadata", "sources"), server.metadataAccounts(),
		chi.URLParam(r, "consoleId"), r.URL.Query().Get("q"))
	switch {
	case errors.Is(err, metadata.ErrUnknownConsole):
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
	case errors.Is(err, metadata.ErrEmptyQuery):
		writeErrorCode(w, http.StatusBadRequest, "images_failed", err.Error())
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
	default:
		writeJSON(w, http.StatusOK, found)
	}
}

// handleConsoleBackgroundSet — PUT …/backgrounds/slots/{slot} {source, url} :
// l'image choisie dans l'emplacement, qu'il soit libre ou pris.
func (server *Server) handleConsoleBackgroundSet(w http.ResponseWriter, r *http.Request) {
	slot, ok := backgroundSlot(w, r)
	if !ok {
		return
	}
	content, ok := readChosenImage(w, r)
	if !ok {
		return
	}

	written, err := metadata.SetConsoleBackground(server.storageRoot, chi.URLParam(r, "consoleId"), slot, content)
	if err != nil {
		writeBackgroundError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slot": slot, "name": written})
}

// handleConsoleBackgroundSlotReset — POST …/backgrounds/slots/{slot}/reset :
// l'emplacement reprend son image par défaut à la prochaine visite.
func (server *Server) handleConsoleBackgroundSlotReset(w http.ResponseWriter, r *http.Request) {
	slot, ok := backgroundSlot(w, r)
	if !ok {
		return
	}
	if err := metadata.ResetConsoleBackgroundSlot(server.storageRoot, chi.URLParam(r, "consoleId"), slot); err != nil {
		writeBackgroundError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleConsoleBackgroundsReset — POST …/backgrounds/reset : les huit
// emplacements vidés ; la lecture suivante de la liste (la page la relit
// aussitôt) relance le remplissage par défaut et l'attend
// (backgroundSeedWait).
func (server *Server) handleConsoleBackgroundsReset(w http.ResponseWriter, r *http.Request) {
	if err := metadata.ResetConsoleBackgrounds(server.storageRoot, chi.URLParam(r, "consoleId")); err != nil {
		writeBackgroundError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ── L'image choisie (metadata/image_search.go) ──────────────────────────

// fetchChosenImage — la récupération d'une image choisie (sa liste des hôtes
// permis comprise) ; remplaçable dans un test des routes, la récupération
// elle-même s'éprouve dans le paquet metadata.
var fetchChosenImage = metadata.FetchChosenImage

// readChosenImage — le corps `{source, url}` d'un choix, puis l'image
// récupérée chez l'hôte d'images de sa source (metadata.ReadChosenImage). Faux
// après avoir répondu : 400 un corps illisible ou une adresse hors de la liste,
// 413 trop grande, 502 la source qui ne la rend pas.
func readChosenImage(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	content, err := metadata.ReadChosenImage(r.Context(), r.Body, fetchChosenImage)
	if err != nil {
		writeChosenImageError(w, err)
		return nil, false
	}

	return content, true
}

// readGameChosenImage — le corps d'un choix pour l'image d'un jeu : une image d'un fournisseur (readChosenImage, chez
// l'hôte d'images de sa source), ou celle d'un jeu d'un serveur fédéré (chantier 57), récupérée chez lui à la plus
// grande largeur par le lien et le jeton du registre. Faux après avoir répondu.
func (server *Server) readGameChosenImage(w http.ResponseWriter, r *http.Request, consoleID string) ([]byte, bool) {
	chosen, err := metadata.DecodeChosenImage(r.Body)
	if err != nil {
		writeChosenImageError(w, err)
		return nil, false
	}
	var content []byte
	if chosen.FromServer() {
		if !metadata.IsImageKind(chi.URLParam(r, "kind")) {
			writeErrorCode(w, http.StatusBadRequest, "bad_request", "unknown image kind")
			return nil, false
		}
		peer, _, found := server.metadataPeerNamed(w, strconv.FormatInt(chosen.Server, 10))
		if !found {
			return nil, false
		}
		content, err = metadata.FetchPeerImage(r.Context(), peer, consoleID, chosen.Reference, chi.URLParam(r, "kind"))
	} else {
		content, err = fetchChosenImage(r.Context(), chosen.Source, chosen.URL)
	}
	if err != nil {
		writeChosenImageError(w, err)
		return nil, false
	}

	return content, true
}

// writeChosenImageError — les refus d'une image choisie (metadata.ChosenImageRefusal).
func writeChosenImageError(w http.ResponseWriter, err error) {
	writeRefusal(w, metadata.ChosenImageRefusal(err), "chosen image", err)
}
