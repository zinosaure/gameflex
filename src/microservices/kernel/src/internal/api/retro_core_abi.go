package api

import (
	"debug/elf"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// errCoreIncompatible — le cœur est arrivé entier, mais il ne se chargerait pas.
//
// Une sentinelle plutôt qu'un simple texte, pour que l'appelant puisse répondre
// « condition préalable non remplie » et non « la passerelle a échoué » : rien
// n'a raté côté réseau. Et surtout, pour que LE MESSAGE S'AFFICHE tel quel —
// demandé par l'utilisateur le 2026-09-17 : « si la dernière version n'est pas
// compatible on l'affichera comme message ».
var errCoreIncompatible = errors.New("ce cœur ne se chargerait pas sur cette machine")

// « LE DERNIER » N'EST PAS « CELUI QUI MARCHE » (#143).
//
// Le buildbot libretro ne publie que `nightly/…/latest/` — pas de répertoire
// daté, et son index est un navigateur JavaScript sans listage (mesuré le
// 2026-09-17 : `2026-07-30/`, `2026-07-01/`, `2026-09-01/` répondent 404).
// Épingler l'adresse, ce qu'on a pu faire pour xemu (#137), n'est donc pas
// possible ici.
//
// Ce qui reste possible, c'est de LIRE le fichier avant de l'installer. Le
// mode de panne est précis : un cœur compilé contre une libstdc++ plus récente
// que celle de l'image de session échoue au `dlopen`, sans message utile — la
// console se contente de ne pas démarrer. La version exigée est pourtant
// écrite dans le binaire lui-même, en clair, et `debug/elf` la lit sans aucune
// dépendance externe.
//
// LE PLAFOND EST MESURÉ, PAS SUPPOSÉ. Le 2026-09-17, sur les DEUX images de
// session — `gameflex-retrogaming:latest` (dev) et
// `gameflex-release/gameflex-retrogaming:test` (release) — qui portent toutes
// deux `libstdc++.so.6 -> libstdc++.so.6.0.30`, soit GLIBCXX_3.4.30 :
//
//	strings …/libstdc++.so.6 | grep -oE 'GLIBCXX_3\.4\.[0-9]+' | sort -uV | tail -1
//
// ATTENTION AU `sort` : sans `-V`, il trie en ALPHABÉTIQUE et répond
// « 3.4.9 » comme plus grand que « 3.4.30 ». C'est le même piège que
// `isOlderVersion` existe pour éviter plus bas — et il m'a fait lire un faux
// plafond une fois. Le lien `libstdc++.so.6.0.NN` donne la même réponse sans
// aucun tri : NN est le dernier nombre de GLIBCXX_3.4.NN.
//
// À refaire après toute reconstruction de l'image de session qui changerait sa
// libstdc++ : c'est ELLE qui charge le cœur, pas le noyau. Relever le plafond
// sans avoir reconstruit l'image rouvrirait exactement le trou de #143.
const maxCoreGlibcxx = "3.4.30"

// verifyCoreABI refuse un cœur que l'image de session ne saura pas charger.
//
// UN FICHIER ILLISIBLE N'EST PAS UN FICHIER REFUSÉ. Si `debug/elf` n'arrive
// pas à l'ouvrir, on ne sait rien — et prétendre le contraire transformerait
// « je n'ai pas pu juger » en « c'est mauvais ». On le signale dans le journal
// et on laisse passer : le refus doit reposer sur une mesure, pas sur une
// absence de mesure.
func verifyCoreABI(path string) error {
	file, err := elf.Open(path)
	if err != nil {
		slog.Warn("cœur installé sans contrôle d'ABI : illisible comme ELF",
			"path", path, "error", err)

		return nil
	}
	defer file.Close()

	symbols, err := file.ImportedSymbols()
	if err != nil {
		slog.Warn("cœur installé sans contrôle d'ABI : symboles illisibles",
			"path", path, "error", err)

		return nil
	}

	// La plus EXIGEANTE des versions demandées : une seule suffit à faire
	// échouer le chargement, donc c'est le maximum qui décide.
	required := ""
	for _, symbol := range symbols {
		version := strings.TrimPrefix(symbol.Version, "GLIBCXX_")
		if version == symbol.Version {
			continue
		}
		if required == "" || isOlderVersion(required, version) {
			required = version
		}
	}

	if required == "" || !isOlderVersion(maxCoreGlibcxx, required) {
		return nil
	}

	return fmt.Errorf(
		"%w — il exige GLIBCXX_%s, l'image de session ne fournit que GLIBCXX_%s. "+
			"Le « latest » du buildbot a dépassé cette image : garder le cœur "+
			"déjà en place, ou reconstruire l'image de session",
		errCoreIncompatible, required, maxCoreGlibcxx)
}

// isOlderVersion — `left` est-il strictement antérieur à `right` ?
//
// Comparaison NOMBRE PAR NOMBRE, pas alphabétique : « 3.4.9 » est antérieur à
// « 3.4.30 », alors que les chaînes disent l'inverse.
func isOlderVersion(left, right string) bool {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")

	for index := 0; index < len(leftParts) || index < len(rightParts); index++ {
		leftValue := versionPart(leftParts, index)
		rightValue := versionPart(rightParts, index)
		if leftValue != rightValue {
			return leftValue < rightValue
		}
	}

	return false
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(parts[index]))
	if err != nil {
		return 0
	}

	return value
}

// verifyAndroidCoreABI — la variante Android d'un cœur doit être une
// bibliothèque arm64 : c'est l'architecture des téléphones qui viendront la
// prendre (chantier 13, phase 5 ; décision 4 : un seul fichier Android par
// cœur, celui des téléphones). Même règle que verifyCoreABI : on ne refuse que
// sur une mesure.
func verifyAndroidCoreABI(path string) error {
	file, err := elf.Open(path)
	if err != nil {
		slog.Warn("cœur Android installé sans contrôle : illisible comme ELF",
			"path", path, "error", err)

		return nil
	}
	defer file.Close()

	if file.Machine != elf.EM_AARCH64 {
		return fmt.Errorf("%w — la variante Android est une bibliothèque %s, les téléphones attendent arm64",
			errCoreIncompatible, file.Machine)
	}

	return nil
}
