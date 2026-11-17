package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/metadata"
)

// LA PORTE DES IMAGES DE CONSOLE, par le vrai routeur du domaine
// retrogaming : jeton exigé ; lire est ouvert à tous ; déposer et retirer
// sont réservés aux administrateurs ; une console inconnue est un 400, une
// console sans image un 404. Rien ne part sur le réseau : la Switch 2 n'a
// pas d'icône, et l'image déposée l'emporte.
func TestConsoleImageRoutesAndTheAdminGuard(t *testing.T) {
	server := &Server{config: config.Config{JWTSecret: facadeTestSecret}, storageRoot: t.TempDir()}
	router := server.retroRouter()
	admin := testToken(t, auth.Claims{Username: "gino", RoleLevel: auth.RoleAdmin})
	user := testToken(t, auth.Claims{Username: "ami", RoleLevel: auth.RoleAdmin + 1})
	var photo bytes.Buffer
	_ = png.Encode(&photo, image.NewNRGBA(image.Rect(0, 0, 600, 300)))

	call := func(token, method, path string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}

	for _, step := range []struct {
		token, method, path string
		body                []byte
		want                int
	}{
		{"", http.MethodGet, "/consoles/switch2/image", nil, http.StatusUnauthorized},
		{user, http.MethodGet, "/consoles/playstation9/image", nil, http.StatusBadRequest},
		{user, http.MethodGet, "/consoles/playstation9/image/source", nil, http.StatusBadRequest},
		{user, http.MethodGet, "/consoles/switch2/image", nil, http.StatusNotFound},
		{user, http.MethodGet, "/consoles/switch2/image/source", nil, http.StatusNotFound},
		{user, http.MethodPut, "/consoles/switch2/image", photo.Bytes(), http.StatusForbidden},
		{user, http.MethodDelete, "/consoles/switch2/image", nil, http.StatusForbidden},
		{admin, http.MethodPut, "/consoles/switch2/image", []byte("<svg/>"), http.StatusUnsupportedMediaType},
		{admin, http.MethodPut, "/consoles/playstation9/image", photo.Bytes(), http.StatusBadRequest},
		{admin, http.MethodPut, "/consoles/switch2/image", photo.Bytes(), http.StatusOK},
		{user, http.MethodGet, "/consoles/switch2/image?w=256", nil, http.StatusOK},
		{user, http.MethodGet, "/consoles/switch2/image/source", nil, http.StatusOK},
		{admin, http.MethodDelete, "/consoles/switch2/image", nil, http.StatusOK},
		{user, http.MethodGet, "/consoles/switch2/image", nil, http.StatusNotFound},
	} {
		answer := call(step.token, step.method, step.path, step.body)
		if answer.Code != step.want {
			t.Fatalf("%s %s = %d %s, attendu %d", step.method, step.path, answer.Code, answer.Body.String(), step.want)
		}
		if step.path == "/consoles/switch2/image?w=256" {
			if answer.Header().Get("Content-Type") != "image/png" || answer.Header().Get("Cache-Control") != "private, no-cache" || answer.Header().Get("ETag") == "" {
				t.Fatalf("en-têtes : %v", answer.Header())
			}
			picture, err := png.Decode(answer.Body)
			if err != nil || picture.Bounds().Dx() != 256 {
				t.Fatalf("image : %v, %v", err, picture)
			}
		}
		if step.path == "/consoles/switch2/image/source" && step.want == http.StatusOK {
			var envelope struct {
				Data struct {
					Source string `json:"source"`
				} `json:"data"`
			}
			if err := json.Unmarshal(answer.Body.Bytes(), &envelope); err != nil || envelope.Data.Source != "manual" {
				t.Fatalf("source : %s", answer.Body.String())
			}
		}
	}
}

// LE FOND D'UNE CONSOLE par le vrai routeur : la liste dans l'ordre des
// noms (le marqueur et ce qui n'est pas une image n'y sont pas), l'image
// réduite, un nom qui sort du dossier en 404, une console inconnue en 400.
// Le marqueur est posé d'avance : rien ne part sur le réseau.
func TestConsoleBackgroundRoutes(t *testing.T) {
	root := t.TempDir()
	server := &Server{config: config.Config{JWTSecret: facadeTestSecret}, storageRoot: root}
	router := server.retroRouter()
	user := testToken(t, auth.Claims{Username: "ami", RoleLevel: auth.RoleAdmin + 1})
	directory := filepath.Join(root, "metadata", "snes", "background")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	var cover bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 900, 1200)))
	for _, name := range []string{"02.png", "01.png", "ff9.png"} {
		_ = os.WriteFile(filepath.Join(directory, name), cover.Bytes(), 0o644)
	}
	_ = os.WriteFile(filepath.Join(directory, ".seeded"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(directory, "lisez-moi.txt"), []byte("x"), 0o644)

	call := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+user)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}

	// Huit emplacements, puis l'image en trop (remarque e du porteur) : servie, jamais effacée.
	answer := call("/consoles/snes/backgrounds")
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"slots":[{"slot":1,"name":"01.png"},{"slot":2,"name":"02.png"},{"slot":3,"name":""},`) ||
		!strings.Contains(answer.Body.String(), `{"slot":8,"name":""}],"extra":["ff9.png"]`) {
		t.Fatalf("liste = %d %s", answer.Code, answer.Body.String())
	}
	if answer := call("/consoles/snes/backgrounds/ff9.png?w=256"); answer.Code != http.StatusOK {
		t.Fatalf("image en trop = %d", answer.Code)
	}
	answer = call("/consoles/snes/backgrounds/02.png?w=300")
	if answer.Code != http.StatusOK || answer.Header().Get("Cache-Control") != "private, no-cache" || answer.Header().Get("ETag") == "" {
		t.Fatalf("image = %d %v", answer.Code, answer.Header())
	}
	if picture, err := jpeg.Decode(answer.Body); err != nil || picture.Bounds().Dx() != 512 {
		t.Fatalf("image réduite : %v", err)
	}

	// LA MÊME ADRESSE, UN AUTRE CONTENU (chantier 53, P12 : l'image remplacée revenait à l'ancienne) : le navigateur
	// revalide — 304 tant que rien n'a changé, puis la nouvelle image et un nouvel ETag une fois l'emplacement remplacé.
	revalidate := func(tag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/consoles/snes/backgrounds/02.png?w=300", nil)
		request.Header.Set("Authorization", "Bearer "+user)
		request.Header.Set("If-None-Match", tag)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	first := answer.Header().Get("ETag")
	if again := revalidate(first); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("même ETag = %d (%d octets), attendu 304", again.Code, again.Body.Len())
	}
	var wider bytes.Buffer
	_ = png.Encode(&wider, image.NewGray(image.Rect(0, 0, 1600, 900)))
	if _, err := metadata.SetConsoleBackground(root, "snes", 2, wider.Bytes()); err != nil {
		t.Fatal(err)
	}
	replaced := revalidate(first)
	if replaced.Code != http.StatusOK || replaced.Header().Get("ETag") == "" || replaced.Header().Get("ETag") == first {
		t.Fatalf("après le remplacement = %d, ETag %q (avant %q)", replaced.Code, replaced.Header().Get("ETag"), first)
	}
	if picture, err := jpeg.Decode(replaced.Body); err != nil || picture.Bounds().Dy() >= picture.Bounds().Dx() {
		t.Fatalf("la nouvelle image n'est pas servie : %v %v", err, picture.Bounds())
	}
	for path, want := range map[string]int{
		"/consoles/snes/backgrounds/..%2F..%2F..%2Fetc%2Fpasswd": http.StatusNotFound,
		"/consoles/snes/backgrounds/.seeded":                     http.StatusNotFound,
		"/consoles/snes/backgrounds/lisez-moi.txt":               http.StatusNotFound,
		"/consoles/snes/backgrounds/09.png":                      http.StatusNotFound,
		"/consoles/playstation9/backgrounds":                     http.StatusBadRequest,
		"/consoles/playstation9/backgrounds/01.png":              http.StatusBadRequest,
	} {
		if answer := call(path); answer.Code != want {
			t.Errorf("%s = %d %s, attendu %d", path, answer.Code, answer.Body.String(), want)
		}
	}
}

// fakeChosenImages — la récupération d'une image choisie, simulée pour les
// adresses données (une image, ou une erreur) ; toute autre passe par la
// vraie, qui refuse avant le réseau une adresse hors de la liste des hôtes.
func fakeChosenImages(t *testing.T, answers map[string]any) {
	t.Helper()
	previous := fetchChosenImage
	fetchChosenImage = func(ctx context.Context, source, address string) ([]byte, error) {
		switch answer := answers[source+" "+address].(type) {
		case []byte:
			return answer, nil
		case error:
			return nil, answer
		}

		return previous(ctx, source, address)
	}
	t.Cleanup(func() { fetchChosenImage = previous })
}

// chosen — le corps d'un choix.
func chosen(source, address string) []byte {
	body, _ := json.Marshal(map[string]string{"source": source, "url": address})

	return body
}

// LES GESTES SUR LE FOND, PAR EMPLACEMENT (chantier 53, P9), par le vrai
// routeur : réservés aux administrateurs ; une image choisie remplit ou
// remplace un emplacement de 1 à 8, rien au-delà ; une adresse hors de la
// liste des hôtes, un corps qui n'est pas un choix (l'envoi d'un fichier est
// parti), un contenu qui n'est pas une image, 16 Mo dépassés, tout refusé ;
// vider, remettre le remplissage. `.seeded` est posé d'avance, et la
// récupération est simulée : rien ne part sur le réseau.
func TestConsoleBackgroundAdminRoutes(t *testing.T) {
	root := t.TempDir()
	server := &Server{config: config.Config{JWTSecret: facadeTestSecret}, storageRoot: root}
	router := server.retroRouter()
	admin := testToken(t, auth.Claims{Username: "gino", RoleLevel: auth.RoleAdmin})
	user := testToken(t, auth.Claims{Username: "ami", RoleLevel: auth.RoleAdmin + 1})
	directory := filepath.Join(root, "metadata", "snes", "background")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, ".seeded")
	_ = os.WriteFile(marker, []byte("{}"), 0o644)
	var cover, photo bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 300, 400)))
	_ = jpeg.Encode(&photo, image.NewGray(image.Rect(0, 0, 300, 400)), nil)
	const art = "https://images.igdb.com/igdb/image/upload/t_1080p/ar1.jpg"
	const hero = "https://cdn2.steamgriddb.com/hero/abc.jpg"
	const drawing = "https://images.igdb.com/igdb/image/upload/t_1080p/svg.jpg"
	const huge = "https://images.igdb.com/igdb/image/upload/t_1080p/huge.jpg"
	fakeChosenImages(t, map[string]any{"igdb " + art: cover.Bytes(), "steamgriddb " + hero: photo.Bytes(), "igdb " + drawing: []byte("<svg/>"), "igdb " + huge: metadata.ErrImageTooLarge})

	call := func(token, method, path string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}

	for _, step := range []struct {
		token, method, path string
		body                []byte
		want                int
		contains            string
	}{
		{"", http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", art), http.StatusUnauthorized, ""},
		{user, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", art), http.StatusForbidden, ""},
		{user, http.MethodPost, "/consoles/snes/backgrounds/reset", nil, http.StatusForbidden, ""},
		{user, http.MethodGet, "/consoles/snes/backgrounds/search?q=Chrono", nil, http.StatusForbidden, ""},
		{admin, http.MethodGet, "/consoles/snes/backgrounds/search?q=", nil, http.StatusBadRequest, "title"},
		{admin, http.MethodGet, "/consoles/playstation9/backgrounds/search?q=x", nil, http.StatusBadRequest, "unknown console"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/0", chosen("igdb", art), http.StatusBadRequest, "1 to 8"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/9", chosen("igdb", art), http.StatusBadRequest, "1 to 8"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/deux", chosen("igdb", art), http.StatusBadRequest, "1 to 8"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", cover.Bytes(), http.StatusBadRequest, "{source, url}"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", []byte(`{"source":"igdb"}`), http.StatusBadRequest, "{source, url}"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", "http://127.0.0.1:9/x.png"), http.StatusBadRequest, "not an image of its source"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("steamgriddb", art), http.StatusBadRequest, "not an image of its source"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("manual", art), http.StatusBadRequest, "not an image of its source"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", drawing), http.StatusUnsupportedMediaType, ""},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", huge), http.StatusRequestEntityTooLarge, ""},
		{admin, http.MethodPut, "/consoles/playstation9/backgrounds/slots/1", chosen("igdb", art), http.StatusBadRequest, "unknown console"},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/1", chosen("igdb", art), http.StatusOK, `"name":"01.png","slot":1`},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/5", chosen("steamgriddb", hero), http.StatusOK, `"name":"05.jpg","slot":5`},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/5", chosen("igdb", art), http.StatusOK, `"name":"05.png","slot":5`},
		{user, http.MethodGet, "/consoles/snes/backgrounds", nil, http.StatusOK, `{"slot":1,"name":"01.png"},{"slot":2,"name":""},{"slot":3,"name":""},{"slot":4,"name":""},{"slot":5,"name":"05.png"}`},
		// Vider un emplacement est parti : une image se remplace (le porteur, 2026-11-14).
		{admin, http.MethodDelete, "/consoles/snes/backgrounds/slots/5", nil, http.StatusMethodNotAllowed, ""},
		// L'envoi d'un fichier et le retrait par nom sont partis.
		{admin, http.MethodPost, "/consoles/snes/backgrounds", cover.Bytes(), http.StatusMethodNotAllowed, ""},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/01.png", cover.Bytes(), http.StatusMethodNotAllowed, ""},
		{admin, http.MethodDelete, "/consoles/snes/backgrounds/01.png", nil, http.StatusMethodNotAllowed, ""},
		{user, http.MethodGet, "/consoles/snes/backgrounds", nil, http.StatusOK, `"slots":[{"slot":1,"name":"01.png"},{"slot":2,"name":""}`},
		// UN emplacement qui reprend son image par défaut (l'image de fond d'une console Remote Play, chantier 53, P12).
		{user, http.MethodPost, "/consoles/snes/backgrounds/slots/5/reset", nil, http.StatusForbidden, ""},
		{admin, http.MethodPost, "/consoles/snes/backgrounds/slots/9/reset", nil, http.StatusBadRequest, "1 to 8"},
		{admin, http.MethodPost, "/consoles/playstation9/backgrounds/slots/1/reset", nil, http.StatusBadRequest, "unknown console"},
		{admin, http.MethodPost, "/consoles/snes/backgrounds/slots/5/reset", nil, http.StatusOK, ""},
		{admin, http.MethodPut, "/consoles/snes/backgrounds/slots/5", chosen("igdb", art), http.StatusOK, `"name":"05.png","slot":5`},
		{admin, http.MethodPost, "/consoles/snes/backgrounds/reset", nil, http.StatusOK, ""},
	} {
		answer := call(step.token, step.method, step.path, step.body)
		if answer.Code != step.want || !strings.Contains(answer.Body.String(), step.contains) {
			t.Fatalf("%s %s = %d %s, attendu %d %s", step.method, step.path, answer.Code, answer.Body.String(), step.want, step.contains)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf(".seeded après reset : %v", err)
	}
	// La vraie remise à zéro : les emplacements vidés (la liste suivante les remplira par défaut).
	if names, _ := filepath.Glob(filepath.Join(directory, "*")); len(names) != 0 {
		t.Fatalf("dossier du fond après la remise à zéro : %v", names)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 {
		t.Fatalf("écrit hors de metadata/ : %v", entries)
	}
}
