package metadata

// LE BON JEU DANS UNE RECHERCHE PAR TITRE (chantier 49, #659) — IGDB et
// SteamGridDB rendent des candidats classés par pertinence, et le premier
// n'est pas toujours le jeu : « EA Sports FC 27 » ramenait le logo de
// « EA Sports FC 24 », « Final Fantasy Resonance Deluxe Edition » le logo
// générique « FINAL FANTASY » (vus sur la dev le 2026-11-12).
//
// Un candidat n'est retenu que s'il porte chaque mot qui compte du titre
// cherché, et exactement les mêmes nombres (chiffres et romains confondus :
// « Final Fantasy 15 » est « Final Fantasy XV »). Les mots d'édition du
// titre cherché ne comptent pas (« Deluxe Edition ») ; « Remastered » si,
// c'est parfois un autre jeu. Parmi les candidats retenus : celui qui a
// exactement les mêmes mots, sinon le plus court (le jeu plutôt que son
// édition « Windows » ou son DLC). Aucun : le service n'a pas ce jeu.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// titleLinkWords — les articles et liaisons, et les régions écrites sans
// parenthèses : ils ne distinguent jamais un jeu d'un autre.
var titleLinkWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "of": true, "to": true,
	"le": true, "la": true, "les": true, "l": true, "de": true, "du": true, "des": true,
	"usa": true, "eur": true, "europe": true, "pal": true, "ntsc": true, "jpn": true, "japan": true,
}

// titleEditionWords — les mots d'édition : ils ne comptent pas dans le titre
// cherché (« Deluxe Edition » d'un jeu acheté ainsi), mais un candidat qui
// les porte n'a pas « les mêmes mots » que le jeu (« Astro Bot: Digital
// Deluxe Edition » passe après « Astro Bot »).
var titleEditionWords = map[string]bool{
	"deluxe": true, "edition": true, "digital": true, "standard": true, "limited": true,
	"collector": true, "collectors": true, "premium": true, "goty": true,
	"platinum": true, "essentials": true, "greatest": true, "hits": true,
}

// romanNumeral — un nombre romain de 1 à 39 : assez pour les numéros de
// jeux, et rien qui ressemble à un mot courant (« mix », « civ » ne le sont
// pas ici).
var romanNumeral = regexp.MustCompile(`^(x{0,3})(ix|iv|v?i{0,3})$`)

var romanValues = map[rune]int{'i': 1, 'v': 5, 'x': 10}

// titleWords — les mots d'un titre : sans ce qui est entre parenthèses ou
// crochets, en minuscules, sans apostrophe (« Marvel's » = « Marvels »),
// coupés sur tout le reste ; un nombre romain devient son nombre.
func titleWords(name string) []string {
	name = strings.ToLower(libretroMentions.ReplaceAllString(name, " "))
	name = strings.NewReplacer("'", "", "’", "").Replace(name)
	words := strings.FieldsFunc(name, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
	for index, word := range words {
		if word != "" && romanNumeral.MatchString(word) {
			words[index] = strconv.Itoa(romanValue(word))
		}
	}

	return words
}

func romanValue(numeral string) int {
	total := 0
	for index, character := range numeral {
		value := romanValues[character]
		if index+1 < len(numeral) && romanValues[rune(numeral[index+1])] > value {
			total -= value
		} else {
			total += value
		}
	}

	return total
}

func isNumber(word string) bool {
	_, err := strconv.Atoi(word)

	return err == nil
}

// titleParts — les mots qui comptent et les nombres d'un titre, les
// nombres normalisés (« 007 » est 7) et triés.
func titleParts(name string) (words []string, numbers []string) {
	return titlePartsKeeping(name, false)
}

// titlePartsKeeping — titleParts, en gardant ou non les mots d'édition.
func titlePartsKeeping(name string, keepEdition bool) (words []string, numbers []string) {
	for _, word := range titleWords(name) {
		switch {
		case isNumber(word):
			value, _ := strconv.Atoi(word)
			numbers = append(numbers, strconv.Itoa(value))
		case titleLinkWords[word], titleEditionWords[word] && !keepEdition:
		default:
			words = append(words, word)
		}
	}
	sort.Strings(numbers)

	return words, numbers
}

// containsWord — le mot est-il dans le candidat : un de ses mots, ou, pour
// un mot de plus de trois lettres, un morceau de ses mots collés
// (« Driveclub » et « Drive Club », « ReVerse » et « re★Verse »).
func containsWord(candidateWords []string, joined, word string) bool {
	for _, candidate := range candidateWords {
		if candidate == word {
			return true
		}
	}

	return len([]rune(word)) > 3 && strings.Contains(joined, word)
}

// titleAccepts — le candidat est-il le jeu cherché : tous les mots qui
// comptent du titre, et les mêmes nombres.
func titleAccepts(wanted, candidate string) bool {
	wantedWords, wantedNumbers := titleParts(wanted)
	candidateWords, candidateNumbers := titleParts(candidate)
	if strings.Join(wantedNumbers, " ") != strings.Join(candidateNumbers, " ") {
		return false
	}
	joined := strings.Join(candidateWords, "")
	for _, word := range wantedWords {
		if !containsWord(candidateWords, joined, word) {
			return false
		}
	}

	return len(wantedWords) > 0 || len(wantedNumbers) > 0
}

// titleKey — les mots et les nombres, triés : deux titres de même clé sont
// le même jeu, dans n'importe quel ordre (« Lightning Returns: Final
// Fantasy XIII »). `keepEdition` : un candidat garde ses mots d'édition, le
// titre cherché non.
func titleKey(name string, keepEdition bool) string {
	words, numbers := titlePartsKeeping(name, keepEdition)
	all := append(append([]string{}, words...), numbers...)
	sort.Strings(all)

	return strings.Join(all, " ")
}

// bestTitle — l'indice du candidat retenu pour `wanted` parmi `names`, -1
// s'il n'y en a pas : le mieux classé par le service parmi ceux qui ont les
// mêmes mots (« Kingdom Hearts III + Re Mind » avant son DLC « Kingdom
// Hearts III: Re Mind »), sinon le plus court des retenus.
func bestTitle(names []string, wanted string) int {
	wantedKey := titleKey(wanted, false)
	best, bestExact := -1, false
	for index, name := range names {
		if !titleAccepts(wanted, name) {
			continue
		}
		exact := titleKey(name, true) == wantedKey
		switch {
		case best < 0,
			exact && !bestExact,
			!exact && !bestExact && len([]rune(name)) < len([]rune(names[best])):
			best, bestExact = index, exact
		}
	}

	return best
}

// editionWords — ce qu'une recherche n'envoie pas : « Marvels Spider Man 2
// Deluxe Edition » trouvait chez IGDB l'édition, rangée dans aucune
// collection, et pas le jeu.
var editionWords = regexp.MustCompile(`(?i)\b(deluxe|edition|digital|standard|limited|collector'?s|premium|goty|platinum|essentials)\b`)

// searchQuery — le titre envoyé à un service : sans ses mots d'édition.
// Le choix du candidat, lui, se fait toujours sur le titre entier.
func searchQuery(title string) string {
	query := strings.Join(strings.Fields(editionWords.ReplaceAllString(title, " ")), " ")
	query = strings.TrimRight(query, " -:")
	if query == "" {
		return title
	}

	return query
}

// seriesName — la série d'un jeu parmi ce qu'IGDB en dit, dans son ordre
// (collections puis franchises) : la première dont les mots se lisent dans
// le nom du jeu. IGDB range « Marvel's Wolverine » dans la collection
// « Marvel's Spider-Man », « Kingdom Hearts HD 1.5 ReMIX » d'abord dans la
// franchise « Disney », « Final Fantasy VII Remake » d'abord dans
// « Compilation of Final Fantasy VII » : une série qui ne se lit pas dans le
// nom n'est pas prise.
//
// La marque possessive en tête du nom (« Marvel » de « Marvel's
// Wolverine », « Tom Clancy » de « Tom Clancy's Splinter Cell ») ne passe
// qu'en dernier : c'est un éditeur ou une licence, pas la série (le porteur,
// 2026-11-13 : Wolverine sous « Wolverine »). « Marvel's Spider-Man » n'est
// pas la marque seule, et reste la série de Spider-Man 2.
//
// « La plus longue » a été essayée et écartée : sur les 31 jeux PS de la
// dev, elle rangeait Remake et Rebirth sous « Final Fantasy VII » (IGDB les
// met aussi dans cette collection), Lightning Returns et XIII-2 sous
// « Final Fantasy XIII », et gardait « Marvel » pour Wolverine.
//
// Aucune : pas de série d'IGDB, et la suivante (libretro-database, puis le
// nom) décide.
func seriesName(gameName string, groups ...[]string) string {
	gameWords, gameNumbers := titleParts(gameName)
	joined := strings.Join(gameWords, "")
	brand := brandWords(gameName)
	fallback := ""
	for _, group := range groups {
		for _, series := range group {
			name := strings.TrimSpace(series)
			seriesWords, seriesNumbers := titleParts(name)
			if name == "" || len(seriesWords) == 0 {
				continue
			}
			readable := true
			for _, word := range seriesWords {
				if !containsWord(gameWords, joined, word) {
					readable = false
					break
				}
			}
			for _, number := range seriesNumbers {
				if !containsWord(gameNumbers, "", number) {
					readable = false
					break
				}
			}
			switch {
			case !readable:
			case len(seriesNumbers) == 0 && wordsWithin(seriesWords, brand):
				if fallback == "" {
					fallback = name
				}
			default:
				return name
			}
		}
	}

	return fallback
}

// possessiveBrand — jusqu'à trois mots en tête du nom, sans ponctuation,
// le dernier au possessif anglais (« Marvel's », « Tom Clancy's ») ; pas
// « Uncharted: Drake's Fortune », dont le possessif est dans le sous-titre.
var possessiveBrand = regexp.MustCompile(`^\s*((?:[^\s'’:\-]+\s+){0,2}[^\s'’:\-]+)['’]s\s`)

// brandWords — les mots de la marque possessive en tête du nom, sans le
// possessif ; aucun s'il n'y en a pas.
func brandWords(gameName string) []string {
	found := possessiveBrand.FindStringSubmatch(gameName)
	if found == nil {
		return nil
	}

	return titleWords(found[1])
}

// wordsWithin — tous les mots de `words` sont-ils dans `set` ?
func wordsWithin(words, set []string) bool {
	if len(set) == 0 {
		return false
	}
	for _, word := range words {
		found := false
		for _, candidate := range set {
			if candidate == word {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
