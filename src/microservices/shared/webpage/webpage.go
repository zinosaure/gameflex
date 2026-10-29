// Package webpage sert la page construite — le dossier `dist/` de webpack :
// ses fichiers avec leur consigne de cache, et `index.html` pour toute adresse
// de navigation.
//
// UN SEUL EXEMPLAIRE pour les hôtes qui servent la page sans serveur de
// développement : le kernel du téléphone, et le frontal d'une installation
// publique (`microservices/edge`). Le code vivait dans le kernel du téléphone ;
// il en est sorti le 2026-09-15, quand le frontal en a eu besoin à son tour.
package webpage

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CE QUE LE NAVIGATEUR A LE DROIT DE GARDER (item 121).
//
// Après une mise à jour de l'APK, la page a tourné seize secondes sur
// l'ancienne interface avant de se recharger (relevé le 2026-09-15 dans le
// journal du téléphone). Le service worker n'y est pas pour grand-chose : il
// va déjà chercher la page et `bundle.js` sur le réseau d'abord. Mais ce
// « réseau » passe par le cache HTTP du navigateur, et le serveur ne disait
// RIEN de la durée de vie de ses fichiers — ni `Cache-Control`, ni date
// d'expiration. Un navigateur a le droit, dans ce cas, d'estimer lui-même
// qu'un fichier est encore frais à partir de sa date de modification, et de
// le resservir sans rien demander.
//
// Or `index.html` et `bundle.js` changent de contenu SANS changer de nom. Ils
// sont donc servis en `no-cache` : le navigateur peut les garder, mais doit
// demander avant chaque usage — un 304 quand rien n'a bougé, le fichier neuf
// sinon. Le serveur de dev fait la même chose, pour la même raison
// (`webpack.config.js`, `no-store`).
//
// Les morceaux dont le nom porte l'empreinte de leur contenu
// (`122.45943f554fd7242a675e.bundle.js`) ne changent jamais sous le même nom :
// ceux-là se gardent sans redemander.

// hashedAsset reconnaît un nom qui porte l'empreinte de son contenu, telle que
// webpack l'écrit (`output.chunkFilename`).
var hashedAsset = regexp.MustCompile(`\.[0-9a-f]{16,}\.`)

// CacheControl — la consigne de cache d'un fichier de la page.
func CacheControl(path string) string {
	if hashedAsset.MatchString(filepath.Base(path)) {
		return "public, max-age=31536000, immutable"
	}

	return "no-cache"
}

// Handler sert webRoot : un fichier qui existe, sinon `index.html` pour une
// adresse de navigation.
//
// LE REPLI VERS LA PAGE NE VAUT QUE POUR LA NAVIGATION : `/streaming`,
// `/retrogaming/nes`. Appliqué à `/api/…`, il transforme une route manquante
// en réponse 200 qui contient du HTML — et la page, qui attendait du JSON,
// annonce « Unexpected token '<' » sans dire de quelle requête il s'agit.
//
// C'est arrivé DEUX fois sur le téléphone. Le 2026-09-04, `/api/v1/users/12`
// tombait ici parce que le préfixe était monté sans barre finale : modifier un
// membre répondait « réponse serveur invalide ». Le 2026-09-05,
// `/api/v1/facade/sessions` et `/api/v1/sessions/{id}` — les deux replis REST
// de la page — n'étaient pas servis du tout.
//
// Deux causes différentes, un seul symptôme illisible. La garde ci-dessous
// rend le diagnostic immédiat : un 404 JSON nomme la route qui manque. Même
// garde pour `/internal/`, le service à service, qu'aucune page ne demande.
func Handler(webRoot string) http.Handler {
	fileServer := http.FileServer(http.Dir(webRoot))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + r.URL.Path)

		if strings.HasPrefix(clean, "/api/") || strings.HasPrefix(clean, "/internal/") {
			notFound(w, clean)

			return
		}

		full := filepath.Join(webRoot, clean)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", CacheControl(clean))
			fileServer.ServeHTTP(w, r)

			return
		}

		// UN FICHIER ABSENT NON PLUS NE DOIT PAS RENDRE LA PAGE — même
		// raison que pour `/api/`, un cran plus loin.
		//
		// `/downloads/gameflex.apk` répondait 200 avec `index.html` sur le
		// téléphone : l'APK n'y est délibérément PAS embarquée (elle ne
		// s'embarque pas elle-même), donc le chemin n'existe pas. Un
		// téléchargeur enregistrait alors 668 octets de HTML sous un nom en
		// `.apk`, et l'installation échouait sans que rien ne dise pourquoi
		// (2026-09-10).
		//
		// Le repli reste entier pour les adresses de NAVIGATION, qui n'ont
		// pas d'extension.
		if extension := filepath.Ext(clean); extension != "" && extension != ".html" {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Cache-Control", CacheControl("index.html"))
		http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
	})
}

// notFound — le 404 JSON d'une route qui n'existe pas, sous la forme de toutes
// les réponses d'API.
func notFound(w http.ResponseWriter, path string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":      false,
		"errorMessage": "no such route: " + path,
	})
}
