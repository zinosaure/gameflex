package resources

import (
	"archive/zip"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"gameflex/storage"
)

// SERVIR UN ÉLÉMENT DEPUIS SES SOURCES (repris de gameflex-federation, api/disks.go). Un jeu, un BIOS, un cœur, un paquet
// ou un correctif qu'aucune bibliothèque d'ici n'a passe par les copies de l'élément, de la meilleure à la moins bonne
// (rank.go) : `?source=<id>` en tête si l'appareil en a choisi une. La première qui répond sert ; une source qui ne
// s'ouvre pas est marquée indisponible et la suivante prend le relais ; chaque transfert réussi donne son débit à la
// mesure de sa source. Un transfert déjà commencé ne peut plus changer de source : ses en-têtes sont partis.

// PreferredSource — la source qu'un appareil a choisie (`?source=`), 0 sinon.
func PreferredSource(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get("source"), 10, 64)

	return id
}

// LocatorPath — le fichier d'une copie : son emplacement dans la source.
func LocatorPath(copy Copy) string { return copy.Where.Path }

// countingWriter — compte ce qui part, pour mesurer le débit d'une source.
type countingWriter struct {
	io.Writer
	written int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.written += int64(n)

	return n, err
}

// Serve — sert l'élément depuis la première de ses copies qui répond ; faux s'il n'en a aucune (l'appelant répond
// « introuvable » à sa façon). `file` rend le chemin du fichier à servir dans la source ; `fail` écrit une erreur au
// format de l'appelant quand aucune copie n'a pu servir.
func (c *Catalog) Serve(w http.ResponseWriter, r *http.Request, copies []Copy, file func(Copy) string, fail func(status int, code, message string)) bool {
	if len(copies) == 0 {
		return false
	}
	var lastErr error
	for _, copy := range copies {
		target := file(copy)
		if target == "" {
			continue
		}
		if r.Method == http.MethodHead {
			writeHeaders(w, copy, target)

			return true
		}
		source, err := copy.Open()
		if err != nil {
			c.Failed(copy.Source, err)
			lastErr = err

			continue
		}
		opener, ok := source.(storage.Opener)
		if !ok {
			lastErr = errors.New("this resource cannot be read")

			continue
		}
		started := time.Now()
		if copy.Where.Dir && target == copy.Where.Path {
			if _, err := source.List(target); err != nil {
				c.Failed(copy.Source, err)
				lastErr = err

				continue
			}
			counter := &countingWriter{Writer: w}
			writeHeaders(w, copy, target)
			_ = zipFolder(counter, source, opener, target)
			c.Served(copy.Source, counter.written, time.Since(started))

			return true
		}
		in, err := opener.Open(target)
		if err != nil {
			c.Failed(copy.Source, err)
			lastErr = err

			continue
		}
		writeHeaders(w, copy, target)
		written, err := io.Copy(w, in)
		in.Close()
		if err == nil {
			c.Served(copy.Source, written, time.Since(started))
		}

		return true
	}
	message := "no source could serve it"
	if lastErr != nil {
		message += ": " + lastErr.Error()
	}
	fail(http.StatusBadGateway, "resource_unavailable", message)

	return true
}

func writeHeaders(w http.ResponseWriter, copy Copy, target string) {
	name := strings.ReplaceAll(path.Base(target), `"`, "")
	w.Header().Set("X-GameFlex-Source", strconv.FormatInt(copy.Source, 10))
	if copy.Where.Dir && target == copy.Where.Path {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)

		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if copy.Where.Size > 0 && copy.Where.Android == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(copy.Where.Size, 10))
	}
}

// zipFolder — le dossier d'un élément, zippé à la volée sans compression (des jeux déjà compressés pour la plupart).
// L'erreur d'un fichier qui ne se lit pas revient : une archive tronquée ne doit pas passer pour entière.
func zipFolder(w io.Writer, source storage.Source, opener storage.Opener, folder string) error {
	archive := zip.NewWriter(w)
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := source.List(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			child := path.Join(dir, e.Name)
			if e.Dir {
				if err := walk(child); err != nil {
					return err
				}

				continue
			}
			name := strings.TrimPrefix(strings.TrimPrefix(child, folder), "/")
			out, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
			if err != nil {
				return err
			}
			in, err := opener.Open(child)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, in)
			in.Close()
			if err != nil {
				return err
			}
		}

		return nil
	}
	if err := walk(folder); err != nil {
		slog.Warn("relais d'un dossier de jeu interrompu", "folder", folder, "error", err)
		_ = archive.Close()

		return err
	}

	return archive.Close()
}

// Open — un élément lu depuis UNE ressource précise (l'import d'un jeu dans la bibliothèque, Réglages › Consoles ›
// Jeux) : le fichier tel quel, ou le dossier zippé à la volée — l'appelant le déplie (importIntoLibrary). Faux si la
// ressource ne le porte pas.
func (c *Catalog) Open(section, consoleID, key string, source int64) (io.ReadCloser, Copy, error) {
	for _, copy := range c.Candidates(section, consoleID, key, source) {
		if copy.Source != source {
			continue
		}
		opened, err := copy.Open()
		if err != nil {
			c.Failed(copy.Source, err)

			return nil, copy, err
		}
		opener, ok := opened.(storage.Opener)
		if !ok {
			return nil, copy, errors.New("this resource cannot be read")
		}
		if !copy.Where.Dir {
			in, err := opener.Open(copy.Where.Path)
			if err != nil {
				c.Failed(copy.Source, err)
			}

			return in, copy, err
		}
		reader, writer := io.Pipe()
		go func() {
			writer.CloseWithError(zipFolder(writer, opened, opener, copy.Where.Path))
		}()

		return reader, copy, nil
	}

	return nil, Copy{}, os.ErrNotExist
}
