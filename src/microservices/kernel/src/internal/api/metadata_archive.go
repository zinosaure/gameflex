package api

// L'ARCHIVE DES MÉTADONNÉES D'UN JEU, PAR LE NAVIGATEUR (le porteur, 2026-11-16, le Catalogue des Réglages : « dans une
// section téléchargement on pour télécharger le jeu, télécharger le metadata […] Les téléchargement sont par navigateur
// et non directement dans la librairie ») — son dossier `metadata/<console>/<gameid>/` en .zip (metadata.WriteGameArchive),
// par le geste d'un jeu (retro_download_ticket.go) : un ticket émis sous un compte, consommé une fois, sans en-tête.
//
//   - POST     /api/v1/retrogaming/{consoleId}/roms/{ref}/metadata/ticket   (gardée) le ticket, une fois vérifié qu'il y
//     a une archive à donner : 404 `no_metadata` pour un jeu dont aucune fiche n'a encore été montrée ;
//   - GET|HEAD /api/v1/retrogaming/downloads/metadata/{ticket}              (sans en-tête) l'archive, une fois ;
//   - GET      /api/v1/federation/metadata/{consoleId}/{ref}/archive         (un pair, la portée du catalogue)
//     l'archive d'un de MES jeux, qu'il relaie à son navigateur.
//
// UN JEU D'ICI : celui que trouve findRom — la bibliothèque, puis les disques, la copie que la liste montre —, son dossier
// sous ses racines (gameMetadataRoots, l'interrupteur). UN JEU D'UN PAIR (`srv<id>:<ref>`) : son archive se lit chez lui
// et se relaie telle quelle (remote.RelayMetadataArchive) ; il doit la prêter à l'émission, ou le ticket ne s'émet pas
// (403 notre jeton refusé, 404 rien à prêter ou une version sans cette route, 502 muet). Rien ne se copie ici.
//
// UN TICKET À PART de ceux des jeux (metadataArchiveTickets) : le ticket d'un jeu n'ouvre pas ses métadonnées, ni
// l'inverse.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/metadata"
	"gameflex/storage/remote"
	"gameflex/storage/ticket"
)

// metadataArchiveTickets — les tickets des archives de métadonnées en cours.
var metadataArchiveTickets ticket.Store

// localMetadataArchive — le dossier des métadonnées d'un jeu d'ici, son gameid et le nom de son archive ; faux après avoir
// répondu (400 une console inconnue, 404 un jeu absent, 404 `no_metadata` un jeu qui n'a pas encore de dossier).
func (server *Server) localMetadataArchive(ctx context.Context, w http.ResponseWriter, consoleID, reference string) (folder, gameID, name string, ok bool) {
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
		return "", "", "", false
	}
	gamePath, _, disk, err := server.findRom(ctx, consoleID, reference)
	if errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "game_not_found", "rom not found")
		return "", "", "", false
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid rom reference: "+err.Error())
		return "", "", "", false
	}
	folder, gameID, err = metadata.GameArchiveFolder(server.gameMetadataRoots(disk).Game, consoleID, gamePath, reference)
	if errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "no_metadata", "this game has no metadata yet")
		return "", "", "", false
	}
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "metadata_failed", "cannot read the metadata folder: "+err.Error())
		return "", "", "", false
	}

	return folder, gameID, metadata.GameName(gamePath) + " - metadata.zip", true
}

// writeMetadataArchive — l'archive d'un jeu d'ici, son nom proposé au navigateur ; rien qu'en-têtes pour un HEAD.
func writeMetadataArchive(w http.ResponseWriter, r *http.Request, folder, consoleID, gameID, name string) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := metadata.WriteGameArchive(w, folder, consoleID, gameID); err != nil {
		slog.Info("metadata archive interrupted", "console", consoleID, "gameId", gameID, "error", err)
	}
}

// handleMintMetadataArchiveTicket — POST /{consoleId}/roms/{ref}/metadata/ticket.
func (server *Server) handleMintMetadataArchiveTicket(w http.ResponseWriter, r *http.Request) {
	consoleID, reference := chi.URLParam(r, "consoleId"), pathParam(r, "ref")

	if serverID, peerReference, federated := parseRemoteReference(reference); federated {
		if !consoles.Exists(consoleID) {
			writeErrorCode(w, http.StatusBadRequest, "console_unknown", "unknown console")
			return
		}
		entry, found := server.registeredByIDValue(w, strconv.FormatInt(serverID, 10))
		if !found {
			return
		}
		if refused := remote.CheckMetadataArchive(r.Context(), peerOf(entry), consoleID, peerReference); refused != nil {
			writeErrorCode(w, refused.Status, codeOr(refused.Code, "metadata_failed"), refused.Message)
			return
		}
	} else if _, _, _, ok := server.localMetadataArchive(r.Context(), w, consoleID, reference); !ok {
		return
	}

	nonce, err := metadataArchiveTickets.Mint(consoleID, reference)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "download_failed", "cannot mint a download ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": nonce, "expiresInSecond": int(ticket.DefaultTTL.Seconds())})
}

// handleMetadataArchiveByTicket — GET/HEAD /downloads/metadata/{ticket}, HORS DU MIDDLEWARE comme celle d'un jeu : le
// ticket est l'autorisation, et il ne vaut qu'une fois (un HEAD le consomme aussi).
func (server *Server) handleMetadataArchiveByTicket(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, ok := metadataArchiveTickets.Consume(chi.URLParam(r, "ticket"))
	if !ok {
		writeErrorCode(w, http.StatusForbidden, "download_link_expired", "this download link is no longer valid")
		return
	}

	if serverID, peerReference, federated := parseRemoteReference(reference); federated {
		entry, found := server.registeredByIDValue(w, strconv.FormatInt(serverID, 10))
		if !found {
			return
		}
		server.answerRelay(w, entry, consoleID, remote.RelayMetadataArchive(w, r, peerOf(entry), consoleID, peerReference))
		return
	}

	folder, gameID, name, ok := server.localMetadataArchive(r.Context(), w, consoleID, reference)
	if !ok {
		return
	}
	writeMetadataArchive(w, r, folder, consoleID, gameID, name)
}

// handleFederationGameMetadataArchive — GET /api/v1/federation/metadata/{consoleId}/{ref}/archive : l'archive d'un de
// MES jeux, demandée par un pair. Un jeu que je tiens moi-même d'un pair (`srv<id>:`) n'est pas à moi : findRom ne le
// trouve pas, 404.
func (server *Server) handleFederationGameMetadataArchive(w http.ResponseWriter, r *http.Request) {
	consoleID, reference := chi.URLParam(r, "consoleId"), pathParam(r, "ref")
	folder, gameID, name, ok := server.localMetadataArchive(r.Context(), w, consoleID, reference)
	if !ok {
		return
	}
	writeMetadataArchive(w, r, folder, consoleID, gameID, name)
}
