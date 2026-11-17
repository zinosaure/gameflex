package metadata

// LES DISQUES ET LA SÉRIE DE CHAQUE JEU D'UNE LISTE (chantier 49, pour le
// launcher du chantier 51) — le porteur veut une fiche par JEU, ses disques
// dedans, et les jeux d'une même série ensemble.
//
// La liste des jeux d'une console les pose, quelle que soit la source (la
// bibliothèque, un disque interne, un ami), au serveur comme au téléphone
// (chantier 57, `#716` : le téléphone montrait chaque disque d'un jeu à part,
// et pas de choix du disque). Chaque kernel a sa forme de ligne : il donne la
// référence et le nom de chaque jeu, et reçoit les groupes dans l'ordre de sa
// liste. Ici parce que la règle a besoin des noms (gameflex/storage,
// gamenames.go) ET de la série que la fiche a déjà donnée (CachedSeries) :
// storage ne peut pas importer ce module.
//
// Rien n'est lu sur le disque ni demandé au-dehors : une liste NES en a 10 783.
//
//   - `discGroup`, `disc`, `discCount`, `discLabel` : les disques d'un même
//     jeu, quand la liste en montre au moins deux ;
//   - `series`, `seriesSource` : la série qu'IGDB a donnée à la fiche du jeu,
//     sinon la franchise de la libretro-database si son index est en mémoire,
//     sinon celle que dit le nom, rattachée à une série plus courte de la même
//     liste (« Final Fantasy Anthology » → « Final Fantasy »).

import (
	"sort"
	"strings"
	"sync"

	"gameflex/storage"
)

// RomGroups — ce que la liste pose sur un jeu : ses disques et sa série,
// d'après les noms seulement. Absents de la réponse quand ils sont vides.
type RomGroups struct {
	// DiscGroup — la même valeur pour tous les disques d'un jeu ; absent
	// quand la liste n'en montre pas au moins deux.
	DiscGroup string `json:"discGroup,omitempty"`
	// Disc — sa place dans le groupe, à partir de 1 ; DiscCount, combien.
	Disc      int `json:"disc,omitempty"`
	DiscCount int `json:"discCount,omitempty"`
	// DiscLabel — « GDI », « A » : quand le disque est nommé plutôt que
	// numéroté.
	DiscLabel string `json:"discLabel,omitempty"`
	// Series — « Final Fantasy » ; SeriesSource : igdb, libretro-database
	// ou name.
	Series       string `json:"series,omitempty"`
	SeriesSource string `json:"seriesSource,omitempty"`
}

// nameFacts — ce qu'un nom dit, calculé une fois par nom : la liste NES
// repasse les mêmes 10 783 à chaque affichage.
type nameFacts struct {
	disc    storage.Disc
	hasDisc bool
	series  string
}

var (
	nameFactsMutex sync.Mutex
	nameFactsCache = map[string]nameFacts{}
)

func factsOf(name string) nameFacts {
	nameFactsMutex.Lock()
	facts, found := nameFactsCache[name]
	nameFactsMutex.Unlock()
	if found {
		return facts
	}

	facts.disc, facts.hasDisc = storage.DiscOf(name)
	facts.series = storage.SeriesFromName(name)
	nameFactsMutex.Lock()
	nameFactsCache[name] = facts
	nameFactsMutex.Unlock()

	return facts
}

// GroupRoms — les disques et la série de chacun des `count` jeux d'une liste
// de la console, dans l'ordre de la liste : `rom(index)` donne la référence
// et le nom (le titre que la liste montre) du jeu `index`.
func GroupRoms(consoleID string, count int, rom func(index int) (reference, name string)) []RomGroups {
	cached := CachedSeries(consoleID)
	groups := make([]RomGroups, count)
	names := make([]string, count)
	facts := make([]nameFacts, count)
	discs := map[string][]int{}
	known := map[string]string{}

	for index := range groups {
		reference, name := rom(index)
		names[index], facts[index] = name, factsOf(name)
		if facts[index].hasDisc {
			discs[facts[index].disc.Group] = append(discs[facts[index].disc.Group], index)
		}
		series, source := cached(reference, name)
		if series == "" && facts[index].series != "" {
			series, source = facts[index].series, "name"
		}
		groups[index].Series, groups[index].SeriesSource = series, source
		if series != "" {
			known[strings.ToLower(series)] = series
		}
	}

	for index := range groups {
		if groups[index].SeriesSource == "name" {
			groups[index].Series = storage.FoldSeries(groups[index].Series, known)
		}
	}

	for group, members := range discs {
		if len(members) < 2 {
			continue
		}
		sort.SliceStable(members, func(left, right int) bool {
			first, second := facts[members[left]].disc, facts[members[right]].disc
			if first.Number != second.Number {
				return first.Number < second.Number
			}
			if first.Label != second.Label {
				return first.Label < second.Label
			}

			return names[members[left]] < names[members[right]]
		})
		for order, index := range members {
			groups[index].DiscGroup, groups[index].Disc, groups[index].DiscCount = group, order+1, len(members)
			groups[index].DiscLabel = facts[index].disc.Label
		}
	}

	return groups
}
