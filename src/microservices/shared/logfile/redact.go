// Masquer les secrets — à l'écriture, et à la relecture.
//
// Un jeton voyage dans l'URL sur deux chemins où il ne peut pas faire
// autrement : les WebSocket (un navigateur ne sait pas poser d'en-tête
// `Authorization` sur une poignée de main) et les liens de téléchargement (un
// `<a href>` non plus). C'est un compromis assumé côté réseau.
//
// Sur le DISQUE, ça n'en est pas un. Le journal d'accès HTTP écrivait l'URL
// entière, donc des JWT valides en clair, dans un fichier qui se lit, se
// télécharge et se garde. Signalé le 2026-09-09 en ouvrant l'écran des
// journaux, qui n'a fait que rendre visible ce qui s'écrivait depuis
// toujours.
//
// Deux gardes, pas une : `HTTPLogger` empêche les nouveaux journaux d'en
// contenir, `Redact` couvre tous ceux qui sont déjà écrits. Corriger
// seulement l'écriture laisserait des mois d'historique exploitable ;
// corriger seulement la lecture laisserait le secret sur le disque, à portée
// de n'importe quelle autre lecture.
package logfile

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// secretParams — les noms qui portent un secret dans une URL ou un JSON.
//
// Une liste explicite plutôt qu'une heuristique : masquer « ce qui ressemble
// à du base64 » masquerait aussi des identifiants de session, des empreintes
// et des noms de fichiers — c'est-à-dire précisément ce qu'on vient lire dans
// un journal.
var secretParams = []string{
	"token", "access_token", "refresh_token", "id_token",
	"password", "passphrase", "secret", "credential", "api_key", "apikey",
	"authorization", "auth", "signature", "sig",
}

// Deux écritures à couvrir, et la seconde manquait au premier jet : un
// paramètre d'URL colle son `=` au nom (`token=…`), mais une clé JSON a un
// guillemet fermant avant son `:` (`"password":"…"`). Le motif tolère donc
// ce guillemet — sans quoi tout ce qui fuit par un journal structuré passait
// au travers, ce qui est précisément la moitié la plus bavarde.
//
// La valeur s'arrête à ce qui termine un paramètre d'URL ou une chaîne JSON.
var secretPattern = regexp.MustCompile(
	`(?i)\b(` + strings.Join(secretParams, "|") + `)("?\s*[=:]\s*"?)([^\s&"',;}\\]+)`)

// masked — ce qui remplace la valeur. Un marqueur lisible plutôt qu'une
// suite d'étoiles de longueur égale : la longueur d'un secret est déjà une
// information, et « masqué » dit ce qui s'est passé.
const masked = "<masqué>"

// Redact remplace la valeur des paramètres sensibles par un marqueur.
//
// La CLÉ est conservée : savoir qu'une requête portait un jeton est ce qui
// permet de comprendre un refus d'authentification. C'est la valeur qui n'a
// rien à faire là.
func Redact(text string) string {
	if text == "" {
		return text
	}

	return secretPattern.ReplaceAllString(text, "${1}${2}"+masked)
}

// HTTPLogger — le journal d'accès, sans les secrets.
//
// Remplace `chimiddleware.Logger`, qui écrit l'URL brute. Même ligne, même
// information, à ceci près que la valeur des paramètres sensibles est
// masquée. Écrit en `net/http` pur pour qu'aucun service n'hérite d'une
// dépendance de plus en s'en servant.
func HTTPLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		slog.Info("http",
			"method", r.Method,
			"path", Redact(r.URL.RequestURI()),
			"from", r.RemoteAddr,
			"status", recorder.status,
			"bytes", recorder.written,
			"took", time.Since(started).String(),
		)
	})
}

// statusRecorder retient ce que le handler a répondu — un `ResponseWriter` ne
// le rend pas.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	written, err := recorder.ResponseWriter.Write(data)
	recorder.written += written

	return written, err
}

// Flush et Hijack passent au travers : sans eux, un flux SSE ne part plus et
// une poignée de main WebSocket échoue. Un enrobage de journalisation ne doit
// jamais retirer une capacité au writer qu'il enrobe — c'est le piège
// classique de ce motif.
func (recorder *statusRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (recorder *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ce writer ne sait pas être détourné")
	}

	return hijacker.Hijack()
}
