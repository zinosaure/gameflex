package storage

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// LA DATE D'UNE SAUVEGARDE COPIÉE, celle de son ORIGINAL (#395, 2026-10-16).
//
// La synchronisation des sauvegardes remplit d'abord les emplacements vides de
// l'autre côté — et « qu'une seule fois » (le porteur) : une sauvegarde déjà
// copiée ne doit pas repartir dans un autre emplacement à la synchronisation
// suivante. Elle se reconnaît à sa taille et à sa date, à condition que la
// copie GARDE la date de l'original : l'envoi la passe (`timestamp`, en
// millisecondes), et l'hôte qui écrit la pose sur le fichier. Elle dit aussi
// la vraie date de la partie, et non celle du transfert.
//
// Ici, pour les deux hôtes : la même règle, écrite une fois.

// ParseSaveTimestamp lit `timestamp` d'un envoi : zéro sans lui, une erreur
// s'il n'est pas un nombre de millisecondes positif.
func ParseSaveTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || millis <= 0 {
		return time.Time{}, errors.New("invalid timestamp")
	}

	return time.UnixMilli(millis), nil
}

// KeepSaveTimestamp pose cette date sur l'état écrit — rien pour une date
// nulle. L'appelant ne défait pas l'envoi sur une erreur : la sauvegarde est
// là, elle sera seulement moins bien reconnue.
func KeepSaveTimestamp(path string, saved time.Time) error {
	if saved.IsZero() {
		return nil
	}

	return os.Chtimes(path, saved, saved)
}
