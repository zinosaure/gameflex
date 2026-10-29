package api

// LE TÉLÉCHARGEMENT PAR TICKET — pour que le NAVIGATEUR télécharge lui-même.
//
// Un lien ne peut pas porter d'en-tête Authorization : on émet donc un ticket
// (route gardée, il faut un compte), et le navigateur s'en sert sans en-tête
// (route enregistrée HORS du middleware, comme /stream/ws pour la même raison).
// Le ticket est aléatoire, ne vaut que pour UN jeu, une minute, et se consomme
// au premier usage : voir `gameflex/storage/ticket`, partagé avec le téléphone,
// qui dit pourquoi c'est un ticket tenu par l'hôte et non une signature.
//
// UN JEU D'UN SERVEUR FÉDÉRÉ aussi (chantier 14) : le kernel le relaie depuis
// le pair, avec le jeton que l'administrateur a enregistré.

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"

	"gameflex/storage/listing"
	"gameflex/storage/remote"
	"gameflex/storage/ticket"
)

// downloadTickets — les tickets en cours (`gameflex/storage/ticket`, partagé
// avec le téléphone depuis le chantier 14).
var downloadTickets ticket.Store

// handleRetroMintDownloadTicket — POST /{consoleId}/roms/{ref}/ticket.
//
// DANS LE GROUPE GARDÉ : c'est ici qu'on exige un compte. Le jeu est résolu
// AVANT d'émettre, pour deux raisons — ne pas distribuer un ticket qui ne mène
// nulle part, et garantir qu'un ticket ne peut nommer qu'un jeu qui existait à
// l'émission.
func (server *Server) handleRetroMintDownloadTicket(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	ref := pathParam(r, "ref")

	// UN JEU D'UN SERVEUR FÉDÉRÉ SE TÉLÉCHARGE AUSSI (chantier 14) : il ne
	// passe plus par un cache pour se jouer ici, mais le bouton « télécharger »
	// de sa fiche doit mener quelque part. Le pair doit le connaître AVANT
	// l'émission, pour la même raison qu'un jeu d'ici.
	if serverID, peerRef, ok := parseRemoteReference(ref); ok {
		if _, _, found := server.resolvePeerGame(w, serverID, consoleID, peerRef); !found {
			return
		}
	} else if _, _, _, err := server.findRom(r.Context(), consoleID, ref); err != nil {
		// La bibliothèque, puis les disques internes (chantier 23) : un jeu
		// qui n'est que sur un disque se télécharge aussi.
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "rom not found")

			return
		}
		writeError(w, http.StatusBadRequest, "invalid rom reference: "+err.Error())

		return
	}

	nonce, err := downloadTickets.Mint(consoleID, ref)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot mint a download ticket")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":          nonce,
		"expiresInSecond": int(ticket.DefaultTTL.Seconds()),
	})
}

// handleRetroDownloadByTicket — GET/HEAD /downloads/{ticket}.
//
// HORS DU MIDDLEWARE, et c'est tout l'intérêt : le navigateur y navigue sans
// en-tête, avec sa propre barre et sa propre reprise. Le ticket EST
// l'autorisation, et il ne vaut qu'une fois.
//
// UN HEAD CONSOMME AUSSI. Il n'y a pas de demi-usage : laisser un HEAD passer
// sans consommer donnerait un moyen de vérifier l'existence d'un jeu en boucle
// avec le même ticket.
func (server *Server) handleRetroDownloadByTicket(w http.ResponseWriter, r *http.Request) {
	consoleID, ref, ok := downloadTickets.Consume(chi.URLParam(r, "ticket"))
	if !ok {
		// Le même refus pour un ticket inconnu, expiré ou déjà servi : dire
		// lequel des trois apprendrait quelque chose à qui essaie.
		writeError(w, http.StatusForbidden, "this download link is no longer valid")

		return
	}

	if serverID, peerRef, ok := parseRemoteReference(ref); ok {
		server.relayPeerRom(w, r, serverID, consoleID, peerRef)

		return
	}

	server.serveRomFile(w, r, consoleID, ref)
}

// pathParam — un paramètre du chemin, DÉCODÉ. chi le capture dans le chemin
// tel qu'il est arrivé, échappements compris : la référence d'un jeu fédéré
// (`srv3:<référence>`, et un chemin entier pour un téléphone) arrive en
// `srv3%3A…`, que le préfixe ne reconnaîtrait pas.
func pathParam(r *http.Request, name string) string {
	raw := chi.URLParam(r, name)
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}

	return raw
}

// resolvePeerGame — le pair, et ce que la copie de son listing annonce pour ce
// jeu ; ou faux, la réponse d'échec déjà écrite : un refus de son jeton (403),
// un pair qui ne répond pas (502), un jeu qu'il n'a pas (404).
func (server *Server) resolvePeerGame(
	w http.ResponseWriter, serverID int64, consoleID, reference string,
) (registeredServer, listing.Item, bool) {
	entry, ok := server.registeredByIDValue(w, strconv.FormatInt(serverID, 10))
	if !ok {
		return registeredServer{}, listing.Item{}, false
	}

	game, err := server.copies().Game(peerOf(entry), consoleID, reference)
	var refused *remote.StatusError
	if errors.As(err, &refused) {
		writeError(w, refused.Status, refused.Message)

		return registeredServer{}, listing.Item{}, false
	}

	return entry, game, true
}

// relayPeerRom — un jeu d'un pair, relayé tel quel au navigateur (chantier 14,
// `remote.Relay`, le même geste que le téléphone).
func (server *Server) relayPeerRom(w http.ResponseWriter, r *http.Request, serverID int64, consoleID, reference string) {
	entry, game, ok := server.resolvePeerGame(w, serverID, consoleID, reference)
	if !ok {
		return
	}

	err := remote.Relay(w, r, peerOf(entry), consoleID, game)
	var refused *remote.StatusError
	switch {
	case errors.As(err, &refused):
		writeError(w, refused.Status, refused.Message)
	case err != nil:
		// Le navigateur a abandonné, ou le pair a coupé : la réponse est
		// partie, il n'y a plus qu'à le noter.
		slog.Info("federated download interrupted", "server", entry.Hostname, "console", consoleID, "error", err)
	}
}
