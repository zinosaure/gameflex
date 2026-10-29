// Package edge aiguille les requêtes du frontal : /api/v1/<domaine>/… vers le
// service qui possède le domaine, WebSocket compris, et tout le reste à la page
// construite.
package edge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"gameflex/edge/internal/domains"
)

// New construit l'aiguillage. env rend la valeur d'une variable
// d'environnement (os.Getenv en production). Une variable absente ou une
// adresse illisible arrête le frontal au démarrage, en la nommant : un
// service injoignable découvert à la première requête coûte bien plus cher.
func New(services []domains.Service, env func(string) string, page http.Handler) (http.Handler, error) {
	if err := domains.Validate(services); err != nil {
		return nil, err
	}

	routes := map[string]http.Handler{}
	for _, service := range services {
		raw := env(service.Target)
		if raw == "" {
			return nil, fmt.Errorf("%s manque — l'adresse du service %s", service.Target, service.Service)
		}

		target, err := url.Parse(raw)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			return nil, fmt.Errorf("%s=%q n'est pas une adresse http(s) — le service %s", service.Target, raw, service.Service)
		}

		handler := proxyTo(service, target)
		for _, domain := range service.Domains {
			routes[domain] = handler
		}
	}

	return &router{routes: routes, page: page}, nil
}

type router struct {
	routes map[string]http.Handler
	page   http.Handler
}

func (router *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// UN CHEMIN QUI REMONTE est renvoyé vers sa forme propre AVANT d'être
	// aiguillé, comme le ferait http.ServeMux : sinon /api/v1/health/../users
	// partirait au kernel avec un chemin qui désigne un domaine de credential.
	if cleaned := path.Clean(r.URL.Path); cleaned != r.URL.Path && cleaned+"/" != r.URL.Path {
		target := *r.URL
		target.Path = cleaned
		target.RawPath = ""
		http.Redirect(w, r, target.RequestURI(), http.StatusMovedPermanently)

		return
	}

	if rest, found := strings.CutPrefix(r.URL.Path, "/api/v1/"); found {
		domain, _, _ := strings.Cut(rest, "/")
		if handler, known := router.routes[domain]; known {
			handler.ServeHTTP(w, r)

			return
		}
	}

	// Tout le reste est à la page, y compris un domaine inconnu : la page y
	// répond un 404 JSON qui le nomme (gameflex/webpage).
	router.page.ServeHTTP(w, r)
}

// proxyTo relaie vers un service. L'hôte de la requête devient celui du
// service, comme le `changeOrigin` du serveur de développement.
func proxyTo(service domains.Service, target *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)

			if service.ForwardClientAddress {
				// La chaîne reçue d'un proxy placé devant (HTTPS) est
				// conservée, et l'adresse du client ajoutée au bout.
				request.Out.Header["X-Forwarded-For"] = request.In.Header["X-Forwarded-For"]
				request.SetXForwarded()
			}
		},
		// Chaque écriture part aussitôt : une réponse longue ne doit pas
		// attendre dans un tampon du frontal.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("edge: service unreachable", "service", service.Service, "path", r.URL.Path, "error", err)
			writeError(w, http.StatusBadGateway, service.Service+" is unreachable")
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Même règle que le serveur de développement : un service qui ne
		// déclare pas de WebSocket n'en reçoit pas.
		if !service.WebSocket && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			writeError(w, http.StatusNotFound, "no websocket under "+r.URL.Path)

			return
		}

		proxy.ServeHTTP(w, r)
	})
}

// writeError — une erreur sous la forme de toutes les réponses d'API.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":      false,
		"errorMessage": message,
	})
}
