package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gameflex/facade"
	"gameflex/storage"
)

// LES PAQUETS STREAMEX, SOURCE OFFICIELLE DES ÉMULATEURS DES CONSOLES RÉCENTES (chantier 63, P4).
//
// Le porteur, 2026-11-18 : « Nous on ne fournit pas d'émulateurs, on fournit un moteur qui peut utiliser les
// émulateurs » ; « On ne publie pas Eden mais streamex-switch. À l'intérieur du streamex-switch on aura Eden. » Les
// paquets vivent dans la Release `streamex` du dépôt zinosaure/gameflex-streamex, avec leur index (`components.json`,
// tools/streamex-packages.sh) : chaque paquet, sa console, ses fichiers, son adresse, sa taille et son sha256.
//
// Ils remplacent l'adresse de l'éditeur que portait l'image (l'étiquette `gameflex.emulator.url`, supprimée) : un
// éditeur peut retirer sa version, déménager ou disparaître (Citron, parti de GitHub en 2026) ; le paquet, lui, reste
// là où on l'a validé. L'index se lit au moment du téléchargement, jamais en tâche de fond (pas de sondage).
//
// GAMEFLEX_STREAMEX_INDEX désigne un autre index (un miroir, un essai).
const defaultStreamexIndex = "https://github.com/zinosaure/gameflex-streamex/releases/download/streamex/components.json"

// streamexIndexTimeout — l'index est petit ; un dépôt muet ne doit pas tenir la page.
const streamexIndexTimeout = 30 * time.Second

// streamexPackageTimeout — les plus gros paquets font quelques centaines de mégaoctets.
const streamexPackageTimeout = 20 * time.Minute

type streamexPackage struct {
	Kind      string              `json:"kind"`
	Items     []streamexPatchItem `json:"items,omitempty"`
	Console   string              `json:"console"`
	Image     string              `json:"image"`
	Version   string              `json:"version"`
	Files     []string            `json:"files"`
	File      string              `json:"file"`
	SizeBytes int64               `json:"sizeBytes"`
	SHA256    string              `json:"sha256"`
	URL       string              `json:"url"`
}

// streamexPatchItem — un correctif du paquet `streamex-patches` (P6) : `<console>/<name>/` dans le zip.
type streamexPatchItem struct {
	Console string   `json:"console"`
	Name    string   `json:"name"`
	Files   []string `json:"files"`
}

type streamexIndex struct {
	Format   int               `json:"format"`
	Packages []streamexPackage `json:"packages"`
}

// streamexIndexURL — l'index que ce serveur suit : celui d'un essai (le champ), celui que dit l'environnement, ou celui
// du projet.
func (server *Server) streamexIndexURL() string {
	if server.streamexIndex != "" {
		return server.streamexIndex
	}
	if value := strings.TrimSpace(os.Getenv("GAMEFLEX_STREAMEX_INDEX")); value != "" {
		return value
	}

	return defaultStreamexIndex
}

func fetchStreamexIndex(ctx context.Context, url string) (streamexIndex, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return streamexIndex{}, err
	}
	response, err := (&http.Client{Timeout: streamexIndexTimeout}).Do(request)
	if err != nil {
		return streamexIndex{}, fmt.Errorf("the streamex packages index did not answer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return streamexIndex{}, fmt.Errorf("the streamex packages index answered %d", response.StatusCode)
	}
	var index streamexIndex
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&index); err != nil {
		return streamexIndex{}, fmt.Errorf("the streamex packages index cannot be read: %w", err)
	}

	return index, nil
}

// packageFor — le paquet du serveur qui porte ce fichier pour cette console.
func (index streamexIndex) packageFor(console, file string) (streamexPackage, bool) {
	for _, candidate := range index.Packages {
		if candidate.Kind != "" && candidate.Kind != "server" {
			continue
		}
		if candidate.Console != console {
			continue
		}
		for _, name := range candidate.Files {
			if name == file {
				return candidate, true
			}
		}
	}

	return streamexPackage{}, false
}

// installStreamexPackage — le paquet téléchargé à côté, son empreinte vérifiée, puis ses fichiers déposés dans `dir`
// (cores/<console>/) sous leur nom. Un fichier déjà là n'est pas remplacé (le kernel ne remplace jamais un émulateur
// présent) ; un nom qui sortirait du dossier est refusé. Rend les octets déposés.
func installStreamexPackage(ctx context.Context, pkg streamexPackage, dir string) (int64, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("folder cannot be created: %w", err)
	}
	storage.AdoptParentOwner(dir)
	for _, name := range pkg.Files {
		if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
			return 0, fmt.Errorf("the package names an unsafe file %q", name)
		}
	}

	archivePath, err := downloadVerified(ctx, pkg, dir)
	if err != nil {
		return 0, err
	}
	defer os.Remove(archivePath)

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("the streamex package cannot be read: %w", err)
	}
	defer reader.Close()
	entries := map[string]*zip.File{}
	for _, entry := range reader.File {
		entries[entry.Name] = entry
	}

	var written int64
	for _, name := range pkg.Files {
		entry, ok := entries[name]
		if !ok {
			return written, fmt.Errorf("the streamex package lacks %s", name)
		}
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		n, err := extractZipEntry(entry, target)
		if err != nil {
			return written, err
		}
		written += n
	}

	return written, nil
}

// extractZipEntry — une entrée du paquet, écrite à côté puis renommée : une coupure ne laisse pas un fichier tronqué
// qui passerait pour présent. Le mode exécutable du paquet est gardé (nos binaires PS4 et PS5).
func extractZipEntry(entry *zip.File, target string) (int64, error) {
	in, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer in.Close()
	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	partial := target + ".part"
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return 0, fmt.Errorf("cannot write: %w", err)
	}
	written, err := io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(partial)

		return 0, fmt.Errorf("cannot unpack %s: %w", entry.Name, err)
	}
	if err := os.Rename(partial, target); err != nil {
		os.Remove(partial)

		return 0, err
	}
	storage.AdoptParentOwner(target)

	return written, nil
}

// patchesFor — le paquet des correctifs qui porte ce correctif, et sa taille annoncée (la somme de ses fichiers n'est
// pas dans l'index : celle du paquet entier, petit).
func (index streamexIndex) patchesFor(console, name string) (streamexPackage, bool) {
	for _, candidate := range index.Packages {
		if candidate.Kind != "patches" {
			continue
		}
		for _, item := range candidate.Items {
			if item.Console == console && item.Name == name {
				return candidate, true
			}
		}
	}

	return streamexPackage{}, false
}

// streamexIndexKept — l'index lu par une page, gardé dix minutes : la page d'un jeu le consulte (la source officielle de
// son correctif), et la relire à chaque page ferait attendre le dépôt. Ce n'est pas un sondage : il ne se relit que
// lorsqu'on le demande, passé ce délai.
const streamexIndexKept = 10 * time.Minute

func (server *Server) cachedStreamexIndex(ctx context.Context) (streamexIndex, error) {
	server.streamexIndexMu.Lock()
	defer server.streamexIndexMu.Unlock()
	if !server.streamexIndexAt.IsZero() && time.Since(server.streamexIndexAt) < streamexIndexKept {
		return server.streamexIndexValue, nil
	}
	index, err := fetchStreamexIndex(ctx, server.streamexIndexURL())
	if err != nil {
		return streamexIndex{}, err
	}
	server.streamexIndexValue, server.streamexIndexAt = index, time.Now()

	return index, nil
}

// downloadVerified — un paquet téléchargé dans un fichier temporaire de `dir`, son empreinte vérifiée. Rend le chemin du
// fichier, à supprimer par l'appelant.
func downloadVerified(ctx context.Context, pkg streamexPackage, dir string) (string, error) {
	archive, err := os.CreateTemp(dir, ".streamex-package-*.zip.part")
	if err != nil {
		return "", fmt.Errorf("cannot write: %w", err)
	}
	defer archive.Close()
	fail := func(err error) (string, error) {
		os.Remove(archive.Name())

		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return fail(err)
	}
	response, err := (&http.Client{Timeout: streamexPackageTimeout}).Do(request)
	if err != nil {
		return fail(fmt.Errorf("the streamex package did not download: %w", err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("the streamex package answered %d for %s", response.StatusCode, pkg.URL))
	}
	digest := sha256.New()
	size := response.ContentLength
	if size <= 0 {
		size = pkg.SizeBytes
	}
	if _, err := io.Copy(facade.ProgressWriter(ctx, io.MultiWriter(archive, digest), size), response.Body); err != nil {
		return fail(fmt.Errorf("download interrupted: %w", err))
	}
	if got := hex.EncodeToString(digest.Sum(nil)); !strings.EqualFold(got, pkg.SHA256) {
		return fail(errors.New("the streamex package does not match its index (sha256): nothing installed"))
	}

	return archive.Name(), nil
}

// handleOfficialPatchImport — POST /api/v1/retrogaming/storage/official/patches {consoleId, name} : le correctif d'un
// jeu depuis la source officielle (chantier 63, P6), le paquet `streamex-patches` de l'index. Seul son dossier
// `<console>/<name>/` se déplie, dans `patches/<console>/<name>/` ; un correctif déjà là n'est pas remplacé.
func (server *Server) handleOfficialPatchImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsoleID string `json:"consoleId"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "corps illisible")

		return
	}
	target, ok := patchPath(body.ConsoleID, body.Name)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid patch")

		return
	}
	if _, err := os.Stat(target); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_installed", "ce correctif existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}
	index, err := fetchStreamexIndex(r.Context(), server.streamexIndexURL())
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "streamex_index_unreachable", err.Error())

		return
	}
	pkg, ok := index.patchesFor(body.ConsoleID, body.Name)
	if !ok {
		writeErrorCode(w, http.StatusNotFound, "not_offered", "the official source does not offer this patch")

		return
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())

		return
	}
	storage.AdoptParentOwner(parent)
	archivePath, err := downloadVerified(r.Context(), pkg, parent)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "download_failed", err.Error())

		return
	}
	defer os.Remove(archivePath)
	written, err := expandPatchFromPackage(archivePath, body.ConsoleID+"/"+body.Name+"/", target)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "storage_failed", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"section": "patches", "consoleId": body.ConsoleID, "name": body.Name, "sizeBytes": written, "source": "official",
	})
}

// expandPatchFromPackage — les entrées du paquet sous `prefix`, dépliées dans un dossier voisin puis renommé en
// `target` : un correctif n'apparaît qu'entier. Un chemin qui sortirait du dossier est refusé.
func expandPatchFromPackage(archivePath, prefix, target string) (int64, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("the streamex package cannot be read: %w", err)
	}
	defer reader.Close()
	staging, err := os.MkdirTemp(filepath.Dir(target), ".streamex-patch-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(staging)
	var written int64
	for _, entry := range reader.File {
		if !strings.HasPrefix(entry.Name, prefix) || strings.HasSuffix(entry.Name, "/") {
			continue
		}
		relative := filepath.Clean(strings.TrimPrefix(entry.Name, prefix))
		if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
			return 0, fmt.Errorf("the package names an unsafe path %q", entry.Name)
		}
		destination := filepath.Join(staging, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return 0, err
		}
		n, err := extractZipEntry(entry, destination)
		if err != nil {
			return 0, err
		}
		written += n
	}
	if written == 0 {
		return 0, errors.New("the streamex package holds nothing for this patch")
	}
	if err := os.Rename(staging, target); err != nil {
		return 0, err
	}
	storage.AdoptParentOwner(target)

	return written, nil
}
