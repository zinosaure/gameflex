package metadata

// LE TEXTE DE LA FICHE D'UN JEU (chantier 49, §3 et §7) — sa date de sortie,
// ses éditeurs, ses développeurs et ses genres, champ par champ, chacun de la
// première source qui l'a, avec le nom de cette source : la fiche cite ses
// sources (CC BY-SA l'exige pour la libretro-database).
//
// L'ordre du § 3 : pour le rétro, la libretro-database, puis IGDB ; pour les
// consoles récentes, IGDB (titledb pour la Switch viendra après). ScreenScraper,
// prévu en tête du rétro, est abandonné (le porteur, 2026-11-13 : payant). `manual`, le texte corrigé par
// l'administrateur (game_admin.go), passe devant tout, champ par champ ; puis
// `server`, la fiche copiée d'un serveur fédéré (chantier 57, peers.go) — un jeu
// désigné chez lui est un geste voulu, qui passe devant les services, et chacun
// de ses champs cite la source d'où le pair le tenait.
//
// LA SÉRIE fait exception : la correction, puis la fiche copiée d'un pair, puis
// IGDB, ses collections étant faites pour ça, même quand ses autres champs n'ont
// pas servi, s'il a déjà été demandé ; puis la franchise de la libretro-database. La liste des jeux tire la même
// série de ce qui est déjà en mémoire (CachedSeries), et du nom à défaut.
//
// LA FUSION SE REFAIT À CHAQUE DEMANDE, sans `metadata.json` : elle ne lit
// que ce qui est déjà gardé (l'index des `.dat` en mémoire, le `source.json`
// d'IGDB), et un fichier de fusion devrait être refait chaque fois qu'une
// source se rafraîchit. Un ordre changé vaut ainsi aussitôt, sans rien
// redemander aux services.

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gameflex/consoles"
)

const sourceLibretroDatabase = "libretro-database"

var (
	retroInfoOrder  = []string{sourceLibretroDatabase, sourceIGDB}
	recentInfoOrder = []string{sourceIGDB}
)

// GameInfo — ce que GET …/roms/{ref}/metadata rend. Un champ inconnu est
// absent ; `sources` dit, par champ, quel service l'a donné — un champ
// corrigé n'en cite aucun : la fiche ne dit jamais « à la main » (le porteur,
// 2026-11-14 : « un administrateur peut toujours aller dans le dossier
// metadata et changer les images sans passer par l'interface. Ce serait
// impossible pour la base de données de savoir que l'image ou le texte a été
// changé. Donc, ça ne sert à rien de garder cette information. »).
type GameInfo struct {
	GameID      string            `json:"gameId"`
	Title       string            `json:"title,omitempty"`       // seulement corrigé ; sinon le nom du jeu
	ReleaseDate string            `json:"releaseDate,omitempty"` // « 2024-09-06 », ou l'année seule
	Publishers  []string          `json:"publishers,omitempty"`
	Developers  []string          `json:"developers,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	Series      string            `json:"series,omitempty"`  // « Final Fantasy »
	Players     int               `json:"players,omitempty"` // le nombre de joueurs, quand une source le sait
	Sources     map[string]string `json:"sources,omitempty"`
}

// gameText — les champs qu'une source donne d'un jeu.
type gameText struct {
	releaseDate                    string
	publishers, developers, genres []string
	series                         string
	players                        int
}

// Info — le texte de la fiche du jeu rangé en `gamePath`. Mêmes arguments
// que Image. Une source qui échoue est notée au journal et la suivante est
// essayée ; une source n'est pas interrogée quand les précédentes ont déjà
// tout donné.
func Info(ctx context.Context, roots Roots, accounts Accounts, consoleID, gamePath, reference string) GameInfo {
	order := recentInfoOrder
	if consoles.MethodOf(consoleID) == consoles.MethodRetroGaming {
		order = retroInfoOrder
	}
	gameDir := gameDirectory(roots.Game, consoleID, gamePath, reference)
	gameID := filepath.Base(gameDir)
	name := GameName(gamePath)
	info := GameInfo{GameID: gameID, Sources: map[string]string{}}
	texts := map[string]gameText{}

	// La correction passe devant les services, sans citer de source : `take` la note sous un nom vide, retiré à la fin.
	manual := readManualState(gameDir)
	info.Title = manual.Title
	info.take("", manual.text())
	// La fiche copiée d'un pair, ensuite : chaque champ garde la source d'où il la tenait (vide : corrigé chez lui).
	copied := readServerCopy(gameDir)
	info.Title = firstOf(info.Title, copied.Title)
	info.takeFrom(copied.text(), func(field string) string { return copied.Sources[field] })

	for _, source := range order {
		if info.complete() {
			break
		}
		var text gameText
		var err error
		switch source {
		case sourceLibretroDatabase:
			text, err = libretroDatabaseText(ctx, filepath.Join(roots.Server, "cache", "metadata", "sources"), consoleID, gameID, []string{name, filepath.Base(gamePath)})
		case sourceIGDB:
			text, err = remoteText(ctx, igdbService{}, accounts, gameDir, consoleID, searchTitle(name))
		}
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn("game info", "source", source, "console", consoleID, "game", name, "error", err)
			}
			continue
		}
		texts[source] = text
		info.take(source, text)
	}

	igdb, asked := texts[sourceIGDB]
	if !asked {
		igdb = cachedRemoteText(igdbService{}, gameDir)
	}
	fromServer := false
	switch {
	case manual.Series != "":
		info.Series, info.Sources["series"] = manual.Series, ""
	case copied.Series != "":
		info.Series, info.Sources["series"], fromServer = copied.Series, copied.Sources["series"], true
	case igdb.series != "":
		info.Series, info.Sources["series"] = igdb.series, sourceIGDB
	case texts[sourceLibretroDatabase].series != "":
		info.Series, info.Sources["series"] = texts[sourceLibretroDatabase].series, sourceLibretroDatabase
	}
	// La liste des jeux garde la série corrigée, celle de la fiche d'un pair ou celle d'IGDB ; corrigée, elle ne cite
	// pas de source (`seriesSource`).
	if source, known := info.Sources["series"]; known && (source == "" || source == sourceIGDB || fromServer) {
		rememberSeries(consoleID, reference, info.Series, source)
	} else {
		forgetSeries(consoleID, reference)
	}
	for field, source := range info.Sources {
		if source == "" {
			delete(info.Sources, field)
		}
	}

	return info
}

// LA SÉRIE QU'IGDB OU L'ADMINISTRATEUR A DONNÉE, par console et par jeu,
// pour la liste des jeux : la lire dans les `source.json` demanderait
// l'identifiant de chaque jeu, donc son empreinte. Tenue en mémoire à chaque
// fiche ; vide après un redémarrage, jusqu'à ce que la fiche du jeu soit
// rouverte.
type knownSeries struct {
	series, source string
}

var (
	igdbSeriesMutex sync.Mutex
	igdbSeries      = map[string]map[string]knownSeries{} // console → référence → série
)

func rememberSeries(consoleID, reference, series, source string) {
	igdbSeriesMutex.Lock()
	defer igdbSeriesMutex.Unlock()
	if series == "" {
		delete(igdbSeries[consoleID], reference)
		return
	}
	if igdbSeries[consoleID] == nil {
		igdbSeries[consoleID] = map[string]knownSeries{}
	}
	igdbSeries[consoleID][reference] = knownSeries{series: series, source: source}
}

// forgetSeries — la série gardée en mémoire ne vaut plus (une source a
// changé) : la liste prend la suite jusqu'à la prochaine fiche.
func forgetSeries(consoleID, reference string) {
	rememberSeries(consoleID, reference, "", "")
}

// CachedSeries — la série d'un jeu de la console, pour la liste : celle
// que l'administrateur ou IGDB a donnée à sa fiche, sinon la franchise de la libretro-database si
// son index est déjà en mémoire. Ne lit rien, ne télécharge rien : vide quand
// rien n'est déjà su, et la liste prend alors le nom.
func CachedSeries(consoleID string) func(reference, gameName string) (series, source string) {
	igdbSeriesMutex.Lock()
	known := make(map[string]knownSeries, len(igdbSeries[consoleID]))
	for reference, series := range igdbSeries[consoleID] {
		known[reference] = series
	}
	igdbSeriesMutex.Unlock()
	indexes := loadedDatabases(consoleID)

	return func(reference, gameName string) (string, string) {
		if series := known[reference]; series.series != "" {
			return series.series, series.source
		}
		for _, index := range indexes {
			if franchise := index.cachedFranchise(gameName); franchise != "" {
				return franchise, sourceLibretroDatabase
			}
		}

		return "", ""
	}
}

func (info *GameInfo) complete() bool {
	return info.ReleaseDate != "" && len(info.Publishers) > 0 && len(info.Developers) > 0 && len(info.Genres) > 0
}

// take — les champs que les sources précédentes n'ont pas donnés, cités sous `source`.
func (info *GameInfo) take(source string, text gameText) {
	info.takeFrom(text, func(string) string { return source })
}

// takeFrom — la même chose, la source de chaque champ dite par `sourceOf` : la fiche copiée d'un pair garde celle d'où
// il tenait chaque champ.
func (info *GameInfo) takeFrom(text gameText, sourceOf func(field string) string) {
	if info.ReleaseDate == "" && text.releaseDate != "" {
		info.ReleaseDate, info.Sources["releaseDate"] = text.releaseDate, sourceOf("releaseDate")
	}
	if len(info.Publishers) == 0 && len(text.publishers) > 0 {
		info.Publishers, info.Sources["publishers"] = text.publishers, sourceOf("publishers")
	}
	if len(info.Developers) == 0 && len(text.developers) > 0 {
		info.Developers, info.Sources["developers"] = text.developers, sourceOf("developers")
	}
	if len(info.Genres) == 0 && len(text.genres) > 0 {
		info.Genres, info.Sources["genres"] = text.genres, sourceOf("genres")
	}
	if info.Players == 0 && text.players > 0 {
		info.Players, info.Sources["players"] = text.players, sourceOf("players")
	}
}

// appendNew — `value` au bout de la liste, s'il n'y est pas déjà.
func appendNew(list []string, value string) []string {
	if value == "" {
		return list
	}
	for _, present := range list {
		if present == value {
			return list
		}
	}

	return append(list, value)
}
