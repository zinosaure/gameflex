package storage

// CE QUE LE NOM D'UN JEU DIT DE LUI (chantier 49, pour le launcher du
// chantier 51) — le disque d'un jeu qui en a plusieurs, et la série à
// laquelle il appartient. Le porteur veut une fiche par JEU, ses disques
// dedans, et les jeux d'une même série ensemble (« tous les Final Fantasy »).
//
// Une seule règle pour le serveur et le téléphone. Elle ne lit que des noms,
// rien sur le disque : une liste de 10 783 NES la passe à chaque affichage.
//
// Les noms rencontrés (relevé du 2026-11-12) :
//   - les dossiers PS1 de la dev : « [PS1] Final Fantasy IX [F] [Disc1of4][SLES-02966] »,
//     « [PS1] Command & Conquer [F] [GDI Disc][SLES-00531] » ;
//   - les jeux complets de No-Intro et Redump : « (Disc 1) », « (Disc 1 of 2) »,
//     « (Disk A) », « (Side B) », et le Famicom Disk System « (Disk 1 Side A) ».

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Disc — le disque qu'un nom désigne.
type Disc struct {
	// Group — le nom sans la mention du disque ni les numéros de série, qui
	// changent d'un disque à l'autre : la clé qui réunit les disques d'un jeu.
	Group string
	// Number — son numéro, ou la place de sa lettre (A = 1) ; 0 pour un
	// disque nommé (« GDI Disc »).
	Number int
	// Label — « GDI », « A », « 1B » : quand le disque est nommé plutôt que
	// numéroté. Vide pour « Disc 2 ».
	Label string
}

var (
	// mentionPattern — un groupe entre crochets ou parenthèses.
	mentionPattern = regexp.MustCompile(`\s*[\(\[]([^\)\]]*)[\)\]]`)
	discNumbered   = regexp.MustCompile(`^(?i:disc|disk|cd)\s*(\d+)(?:\s*(?i:of)\s*\d+)?$`)
	discSided      = regexp.MustCompile(`^(?i:disc|disk)\s*(\d+)\s*(?i:side)\s*([A-Da-d])$`)
	discLettered   = regexp.MustCompile(`^(?i:disc|disk|side)\s*([A-Da-d])$`)
	discNamed      = regexp.MustCompile(`^(\S+)\s+(?i:disc|disk)$`)
	// serialPattern — un numéro de série PlayStation : quatre lettres, cinq
	// chiffres, un tiret ou non (« SCES-02066 », « SLUS_012.72 »).
	serialPattern = regexp.MustCompile(`\b([A-Z]{4})[-_]?(\d{3})\.?(\d{2})\b`)
)

// DiscOf — le disque que désigne le nom d'un jeu ; faux quand le nom n'en
// mentionne aucun.
func DiscOf(name string) (Disc, bool) {
	lower := strings.ToLower(name)
	if !strings.Contains(lower, "dis") && !strings.Contains(lower, "side") && !strings.Contains(lower, "cd") {
		return Disc{}, false
	}

	disc, found := Disc{}, false
	group := mentionPattern.ReplaceAllStringFunc(name, func(mention string) string {
		content := strings.TrimSpace(mentionPattern.FindStringSubmatch(mention)[1])
		if !found {
			if parsed, ok := discMention(content); ok {
				disc, found = parsed, true
				return ""
			}
		}
		if len(SerialsIn(content)) > 0 {
			return ""
		}

		return mention
	})
	if !found {
		return Disc{}, false
	}
	disc.Group = strings.Join(strings.Fields(group), " ")

	return disc, true
}

func discMention(content string) (Disc, bool) {
	if found := discNumbered.FindStringSubmatch(content); found != nil {
		number, _ := strconv.Atoi(found[1])
		return Disc{Number: number}, true
	}
	if found := discSided.FindStringSubmatch(content); found != nil {
		number, _ := strconv.Atoi(found[1])
		side := strings.ToUpper(found[2])
		return Disc{Number: number*4 + int(side[0]-'A'), Label: found[1] + side}, true
	}
	if found := discLettered.FindStringSubmatch(content); found != nil {
		letter := strings.ToUpper(found[1])
		return Disc{Number: int(letter[0]-'A') + 1, Label: letter}, true
	}
	if found := discNamed.FindStringSubmatch(content); found != nil {
		return Disc{Label: found[1]}, true
	}

	return Disc{}, false
}

// SerialsIn — les numéros de série PlayStation d'un nom, sans leur
// ponctuation (« [SCES-02066] » → « SCES02066 »).
func SerialsIn(name string) []string {
	serials := []string{}
	for _, found := range serialPattern.FindAllStringSubmatch(name, -1) {
		serials = append(serials, found[1]+found[2]+found[3])
	}

	return serials
}

// SeriesFromName — la série que le nom d'un jeu laisse deviner : sans ses
// mentions entre crochets ou parenthèses (« [PS1] », « (USA) »), la partie
// avant « - » ou « : », sans numéro final, en chiffres ou en romain, et
// l'article rejeté remis devant (« Legend of Zelda, The »).
//
//	« [PS1] Final Fantasy IX [F] [Disc1of4][SLES-02966] » → « Final Fantasy »
//	« [PS1] Gran Turismo 2 - GT Mode [E-F-G-I-S] »          → « Gran Turismo »
//	« Final Fantasy Anthology - Final Fantasy IV »          → « Final Fantasy Anthology » (FoldSeries le rattache)
func SeriesFromName(name string) string {
	title := strings.Join(strings.Fields(mentionPattern.ReplaceAllString(name, " ")), " ")
	for _, separator := range []string{" - ", ": "} {
		if before, _, found := strings.Cut(title, separator); found && strings.TrimSpace(before) != "" {
			title = before
		}
	}
	title = strings.TrimSpace(title)
	lower := strings.ToLower(title)
	for _, article := range []string{", the", ", a", ", an", ", le", ", la", ", les"} {
		if strings.HasSuffix(lower, article) {
			title = title[len(title)-len(article)+2:] + " " + title[:len(title)-len(article)]
			break
		}
	}

	words := strings.Fields(title)
	if len(words) > 1 && isSequenceNumber(words[len(words)-1]) {
		words = words[:len(words)-1]
	}

	return strings.Join(words, " ")
}

// isSequenceNumber — un numéro d'épisode : « 2 », « 2000 », « IX ».
func isSequenceNumber(word string) bool {
	if word == "" {
		return false
	}
	digits, roman := true, true
	for _, character := range word {
		digits = digits && unicode.IsDigit(character)
		roman = roman && strings.ContainsRune("IVXLC", character)
	}

	return digits || roman
}

// FoldSeries — la série rattachée à la plus courte des séries `known` dont
// elle prolonge les mots : « Final Fantasy Anthology » et « Final Fantasy
// Origins » vont avec « Final Fantasy ». `known` : les séries de la liste,
// par leur nom en minuscules. « Crash Team Racing » reste seul tant qu'aucune
// série « Crash » n'existe.
func FoldSeries(series string, known map[string]string) string {
	words := strings.Fields(series)
	for length := 1; length < len(words); length++ {
		if shorter, found := known[strings.ToLower(strings.Join(words[:length], " "))]; found {
			return shorter
		}
	}

	return series
}
