package storage

import (
	"sort"
	"strings"
)

// memorySource — une source en mémoire, décrite par la liste de ses chemins.
// Elle existe pour que les règles du scanner se testent sans disque, sans
// réseau et sans Android : c'est tout l'intérêt d'avoir réduit une source à
// « lister un dossier ».
type memorySource struct {
	name  string
	files map[string]int64 // chemin relatif → taille
}

func newMemorySource(name string, files ...string) *memorySource {
	source := &memorySource{name: name, files: map[string]int64{}}
	for _, f := range files {
		source.files[trimPath(f)] = 1024
	}

	return source
}

func (s *memorySource) Name() string { return s.name }

func (s *memorySource) List(dir string) ([]Entry, error) {
	prefix := trimPath(dir)
	if prefix != "" {
		prefix += "/"
	}

	seen := map[string]Entry{}
	for file, size := range s.files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		rest := file[len(prefix):]
		if rest == "" {
			continue
		}
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			name := rest[:slash]
			seen[name] = Entry{Name: name, Dir: true}

			continue
		}
		seen[rest] = Entry{Name: rest, Size: size}
	}

	out := make([]Entry, 0, len(seen))
	for _, entry := range seen {
		out = append(out, entry)
	}
	// Ordre stable : un test qui dépend de l'ordre d'une map échoue un jour
	// sur deux et pour rien.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}
