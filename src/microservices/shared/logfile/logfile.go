// Package logfile — les journaux SUR LE DISQUE, à côté des données.
//
// `docker logs` meurt avec le conteneur. C'est vrai de tous les services, et
// c'est surtout vrai des conteneurs de SESSION : ils sont créés pour une
// partie, détruits à la fin, et c'est justement quand une partie s'est mal
// passée qu'on voudrait relire ce qu'ils ont dit. Le kernel le savait déjà à
// moitié — `containerLogTail` garde les dernières lignes d'un enfant mort,
// « the only trace of WHY it died once the container is gone ». Ce paquet
// garde le reste.
//
// ── DEUX RANGEMENTS, PARCE QUE DEUX DURÉES DE VIE ──────────────────────
//
//	<racine>/logs/20260905/kernel.log            un SERVICE
//	<racine>/logs/20260905/141932/retro-nes-001.log   une SESSION
//
// Un SERVICE tourne en continu : son journal est celui de la journée, et il
// s'y ajoute. Un redémarrage — un plantage, un `air` qui recompile — continue
// le même fichier, parce que ce qui précède un redémarrage est justement ce
// qu'on cherche quand on enquête dessus.
//
// Une SESSION est une instance : il y en a plusieurs par jour, à des heures
// différentes, et **elles portent le même nom**. Docker réutilise
// `retro-nes-001` d'une partie à l'autre — sans l'heure, deux parties de NES
// se mélangeraient dans un seul fichier, et on ne saurait plus où finit l'une
// et où commence l'autre. L'heure de DÉMARRAGE de la session les sépare.
//
// C'est la correction du 2026-09-05 : le premier jet mettait tout dans un
// dossier par lancement de la pile, ce qui découpait les services sans
// raison et n'aurait pas suffi à séparer deux sessions du même lancement.
package logfile

import (
	"os"
	"path/filepath"
	"time"
)

// EnvDisable — pour éteindre l'écriture sans reconstruire quoi que ce soit.
//
// Allumé par défaut, contrairement au téléphone : un serveur a un disque
// qu'on administre, et l'intérêt de ces fichiers est justement d'exister
// AVANT qu'on sache qu'on en aura besoin. Les allumer après coup, c'est
// arriver trop tard.
const EnvDisable = "GAMEFLEX_FILE_LOGS_OFF"

// Disabled dit si l'écriture est éteinte.
func Disabled() bool {
	return os.Getenv(EnvDisable) != ""
}

// Service ouvre le journal du jour d'un service — `logs/20260905/kernel.log`.
//
// En APPEND : le service peut redémarrer, il continue son fichier.
func Service(root, name string) (*os.File, error) {
	return open(root, filepath.Join(day(), name+".log"))
}

// ReportsFile — le nom du journal des signalements, cité par les deux kernels
// plutôt que réécrit dans chacun.
const ReportsFile = "reports.log"

// Reports ouvre le journal des SIGNALEMENTS — `logs/reports.log`.
//
// À LA RACINE, et non sous le jour, contrairement aux deux autres. Demande
// explicite de l'utilisateur (#191), et elle se tient : un signalement se lit
// en bloc, dans l'ordre où il est arrivé. Rangés par date, retrouver « ce qui a
// échoué au lancement cette semaine » demanderait d'ouvrir sept fichiers ; et
// un chemin unique, toujours le même, est ce qui rend ce journal citable.
//
// En APPEND, comme un journal de service : une ligne JSON par signalement,
// jamais réécrite.
func Reports(root string) (*os.File, error) {
	return open(root, ReportsFile)
}

// Session ouvre le journal d'UNE session, sous l'heure de son démarrage —
// `logs/20260905/141932/retro-nes-001.log`.
//
// L'heure est prise MAINTENANT, à l'ouverture : c'est le démarrage de cette
// session-là, et c'est ce qui la distingue de la précédente qui portait le
// même nom.
func Session(root, name string) (*os.File, error) {
	return open(root, filepath.Join(day(), time.Now().Format("150405"), name+".log"))
}

func day() string {
	return time.Now().Format("20060102")
}

// open crée l'arborescence et ouvre le fichier.
//
// Rend nil, nil quand l'écriture est éteinte ou qu'aucune racine n'est
// connue : l'appelant continue sans fichier, et rien ne casse pour un
// journal.
func open(root, relative string) (*os.File, error) {
	if root == "" || Disabled() {
		return nil, nil
	}

	full := filepath.Join(root, "logs", relative)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}

	return os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
