// Parcourir les journaux déjà écrits — lister, lire la fin, télécharger,
// supprimer.
//
// Écrire un journal ne sert que si on peut le relire, et jusqu'ici la seule
// façon de le faire était d'ouvrir un terminal sur la machine. Sur un
// téléphone, il n'y a pas de terminal ; sur un serveur, l'administrateur qui
// veut comprendre une session finie n'a pas toujours d'accès SSH. Ces
// quatre gestes vivent donc ici, avec l'écriture, et les deux plateformes
// servent exactement les mêmes.
package logfile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry — un journal sur le disque, décrit par ce que son CHEMIN raconte.
//
// Le rangement porte déjà l'information (voir la doc du paquet) : le jour est
// le premier dossier, et un deuxième niveau signifie une session lancée à
// cette heure-là. On ne relit donc pas le contenu pour savoir de quoi il
// s'agit.
type Entry struct {
	// Path — relatif à la racine des journaux. C'est ce que les autres
	// fonctions attendent, et ce qui voyage jusqu'à la page.
	Path string `json:"path"`
	Name string `json:"name"`
	Day  string `json:"day"`
	// Session — l'heure de lancement (HHMMSS) quand ce journal appartient à
	// une partie, vide pour un journal de service.
	Session    string    `json:"session,omitempty"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// ErrOutsideRoot — un chemin qui sort du dossier des journaux.
//
// Le chemin vient d'une requête HTTP : sans cette vérification, un
// « ../../databases/gameflex.db » ferait de cette API un lecteur de fichiers
// arbitraires, et son bouton de suppression une arme.
var ErrOutsideRoot = errors.New("chemin hors du dossier des journaux")

// LogsRoot rend le dossier des journaux d'une racine de stockage.
func LogsRoot(root string) string { return filepath.Join(root, "logs") }

// resolve vérifie qu'un chemin relatif reste sous la racine, et rend le
// chemin absolu correspondant.
func resolve(root, relative string) (string, error) {
	logs := LogsRoot(root)
	full := filepath.Join(logs, filepath.FromSlash(relative))
	// filepath.Join nettoie déjà les « .. », mais il peut aussi remonter :
	// c'est le résultat qu'on vérifie, jamais l'entrée.
	if full != logs && !strings.HasPrefix(full, logs+string(os.PathSeparator)) {
		return "", ErrOutsideRoot
	}

	return full, nil
}

// List rend tous les journaux, le plus récent d'abord.
//
// Le plus récent d'abord parce que c'est toujours celui qu'on cherche : on
// ouvre cet écran après qu'une chose vient de mal se passer.
func List(root string) ([]Entry, error) {
	logs := LogsRoot(root)
	entries := []Entry{}

	err := filepath.Walk(logs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			// Un dossier illisible ne doit pas emporter tout le listing —
			// on montre ce qu'on peut lire.
			return nil //nolint:nilerr
		}
		if !strings.HasSuffix(info.Name(), ".log") {
			return nil
		}
		relative, err := filepath.Rel(logs, path)
		if err != nil {
			return nil //nolint:nilerr
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		entry := Entry{
			Path:       filepath.ToSlash(relative),
			Name:       info.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC(),
		}
		// UN JOURNAL À LA RACINE N'A PAS DE JOUR. `reports.log` (#191) vit hors
		// des dossiers datés : lui donner son propre nom pour « jour »
		// afficherait « reports.log » là où l'écran attend une date, et
		// surtout le ferait écarter par n'importe quel filtre de dates, qui
		// compare des chaînes AAAAMMJJ — il disparaîtrait sans un mot. Vide
		// veut dire « pas daté », et c'est la vérité.
		if len(parts) > 1 {
			entry.Day = parts[0]
		}
		if len(parts) == 3 {
			entry.Session = parts[1]
		}
		entries = append(entries, entry)

		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ModifiedAt.After(entries[j].ModifiedAt)
	})

	return entries, nil
}

// Tail rend les `lines` dernières lignes d'un journal.
//
// La FIN, et pas le début : ce qu'on cherche dans un journal, c'est ce qui
// vient de casser. Un journal de session PS2 pèse plusieurs mégaoctets, et
// tout rendre ferait ramer la page d'un téléphone pour du texte que personne
// ne lit.
//
// Lu par la fin, par blocs, plutôt qu'en chargeant tout pour n'en garder que
// la queue.
func Tail(root, relative string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	full, err := resolve(root, relative)
	if err != nil {
		return "", err
	}
	file, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	const block = 64 * 1024
	var (
		collected []byte
		offset    = info.Size()
	)
	for offset > 0 && strings.Count(string(collected), "\n") <= lines {
		size := int64(block)
		if offset < size {
			size = offset
		}
		offset -= size
		buffer := make([]byte, size)
		if _, err := file.ReadAt(buffer, offset); err != nil && err != io.EOF {
			return "", err
		}
		collected = append(buffer, collected...)
	}

	all := strings.Split(strings.TrimRight(string(collected), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}

	// MASQUÉ AVANT DE SORTIR D'ICI. Les journaux déjà écrits contiennent des
	// jetons en clair (voir redact.go) ; les corriger à l'écriture ne fait
	// rien pour les mois d'historique qui sont sur le disque.
	return Redact(strings.Join(all, "\n")), nil
}

// WriteRedacted déverse un journal entier, secrets masqués.
//
// Ligne par ligne plutôt qu'en une fois : un journal de session pèse
// plusieurs mégaoctets, et le masquage ne doit pas obliger à le charger en
// mémoire pour le rendre. Le téléchargement passe par ici et NON par
// `http.ServeContent` — servir le fichier tel qu'il est sur le disque
// rendrait les jetons que la lecture vient de masquer.
func WriteRedacted(w io.Writer, root, relative string) error {
	full, err := resolve(root, relative)
	if err != nil {
		return err
	}
	file, err := os.Open(full)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Une ligne de journal peut être longue (une trace, un SDP entier) : la
	// limite par défaut de 64 Ko la couperait au milieu, et une ligne coupée
	// est une ligne dont le masquage peut rater.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if _, err := io.WriteString(w, Redact(scanner.Text())+"\n"); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// Remove supprime un journal, et le dossier de sa session s'il devient vide.
//
// Le dossier vide compte : sans ça, une liste par jour se remplirait de
// dossiers d'heures qui ne contiennent plus rien, et l'écran montrerait des
// sessions dont on vient d'effacer la trace.
func Remove(root, relative string) error {
	full, err := resolve(root, relative)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(full, ".log") {
		return fmt.Errorf("%w : seuls les journaux se suppriment ici", ErrOutsideRoot)
	}
	if err := os.Remove(full); err != nil {
		return err
	}
	// Best-effort : os.Remove refuse un dossier non vide, ce qui est
	// exactement la garde qu'on veut.
	parent := filepath.Dir(full)
	if parent != LogsRoot(root) {
		_ = os.Remove(parent)
	}

	return nil
}

// Line — une ligne de journal, lue plutôt que rendue telle quelle.
//
// Un journal est du JSON structuré (`{"time","level","msg",…}`), et le
// montrer brut redonne à l'écran ce qu'un terminal donne déjà : des
// accolades qu'on relit à la main pour retrouver l'heure et le message. Sur
// un téléphone, où chaque ligne se replie sur quatre, c'est illisible.
//
// Découpé, ça devient une liste : le message en titre, l'heure et le niveau
// dessous — donc quelque chose qu'on peut filtrer par texte, par heure et
// par niveau.
type Line struct {
	// Time — l'horodatage tel qu'il est écrit, vide s'il n'y en a pas.
	Time string `json:"time,omitempty"`
	// Clock — HH:MM:SS, la seule partie qu'on lit quand on cherche « ce qui
	// s'est passé à ce moment-là ». La date est déjà celle du dossier.
	Clock   string `json:"clock,omitempty"`
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	// Details — les champs restants, dans LEUR ordre d'écriture. Cet ordre
	// porte du sens (`sessionId` avant `button` avant `action` raconte une
	// action) et une map le perdrait.
	Details string `json:"details,omitempty"`
	// Raw — la ligne d'origine, gardée pour ce qui n'est pas du JSON : les
	// journaux d'accès HTTP, les sorties d'émulateur. Ne rien montrer d'une
	// ligne qu'on n'a pas su lire serait cacher précisément celle qui sort
	// de l'ordinaire.
	Raw string `json:"raw,omitempty"`
}

// ParseTail découpe un bloc de journal en lignes lisibles.
//
// Tolérant par construction : une ligne qui n'est pas du JSON n'est pas une
// erreur, c'est un autre format qui cohabite dans le même fichier (le
// serveur HTTP y écrit ses accès en texte). Elle ressort en `Raw`.
func ParseTail(content string) []Line {
	if strings.TrimSpace(content) == "" {
		return []Line{}
	}

	rows := strings.Split(content, "\n")
	lines := make([]Line, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		lines = append(lines, parseLine(row))
	}

	return lines
}

func parseLine(row string) Line {
	trimmed := strings.TrimSpace(row)
	if !strings.HasPrefix(trimmed, "{") {
		return Line{Raw: trimmed}
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed))
	if _, err := decoder.Token(); err != nil { // l'accolade ouvrante
		return Line{Raw: trimmed}
	}

	line := Line{}
	details := make([]string, 0, 8)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return Line{Raw: trimmed}
		}
		key, _ := keyToken.(string)

		var value any
		if err := decoder.Decode(&value); err != nil {
			return Line{Raw: trimmed}
		}

		switch key {
		case "time":
			line.Time, _ = value.(string)
		case "level":
			line.Level, _ = value.(string)
		case "msg", "message":
			line.Message, _ = value.(string)
		default:
			details = append(details, key+"="+scalar(value))
		}
	}

	// Ni message ni heure : ce n'était pas un journal, juste du JSON. On rend
	// la ligne entière plutôt qu'une entrée vide.
	if line.Message == "" && line.Time == "" {
		return Line{Raw: trimmed}
	}

	line.Details = strings.Join(details, " ")
	line.Clock = clockOf(line.Time)

	return line
}

// clockOf extrait HH:MM:SS d'un horodatage RFC 3339, sans passer par
// time.Parse : un fuseau ne doit pas entrer ici. L'heure écrite dans le
// fichier est celle qu'on veut lire, telle quelle.
func clockOf(stamp string) string {
	start := strings.IndexByte(stamp, 'T')
	if start < 0 || len(stamp) < start+9 {
		return ""
	}

	return stamp[start+1 : start+9]
}

func scalar(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		// Les journaux comptent des images et des octets : des entiers, que
		// %v rendrait en notation scientifique passé le million.
		if typed == math.Trunc(typed) && math.Abs(typed) < 1e15 {
			return strconv.FormatInt(int64(typed), 10)
		}

		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}

		return string(encoded)
	}
}
