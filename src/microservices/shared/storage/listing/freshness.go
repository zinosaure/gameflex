package listing

// LA DATE QU'UN KERNEL REND À `GET /federation/listing/updated`, TENUE EN
// MÉMOIRE (journal 629 ; le porteur, 2026-11-15 : « Répondre de mémoire »).
//
// C'est la question qu'un ami pose avant d'afficher nos jeux, au plus une fois
// par minute, et il ne l'attend que cinq secondes (remote.listTimeout) :
// au-delà, nous « ne répondons pas » pendant une minute (remote.CheckEvery).
// Elle relisait à chaque fois les dossiers des disques internes, et un disque
// rotatif endormi devait d'abord se réveiller : 5,6 s à la dev et 28,5 s à la
// release le 2026-11-15, des séries à 20-35 s.
//
// Elle se répond maintenant sans ouvrir un fichier, et la date avance avec ce
// qui CHANGE le listing :
//   - une écriture du listing du kernel (un parcours, un import, une
//     suppression, un signalement) : son en-tête se relit après elle (Self) ;
//   - un disque relu en entier, au démarrage et par « Relire » : le dernier
//     changement de ses sections (Disk) ;
//   - un disque apparu, parti ou déplacé, dès que le kernel regarde ses
//     disques (Disks) ;
//   - le scan des jeux d'un disque, que chaque kernel garde en mémoire et donne
//     à la lecture (UpdatedAt).
//
// La date ne recule jamais : un ami ne recopie que quand elle a AVANCÉ
// (remote.Copies). Ce qu'on dépose à la main sur un disque attend donc qu'une
// liste de ce disque le voie, ou « Relire » — comme la bibliothèque attend
// son parcours.

import (
	"sync"
	"time"
)

// Freshness — la date du listing servi à un ami. Zéro-valeur utilisable.
type Freshness struct {
	mu sync.Mutex
	// self — la date de l'en-tête du listing ; known : il a été lu une fois.
	self  time.Time
	known bool
	// disks — les disques présents la dernière fois que le kernel les a
	// regardés, par nom ; nil avant la première fois.
	disks map[string]freshDisk
	// disksChangedAt — la dernière fois qu'un disque est apparu, parti ou a
	// changé de dossier.
	disksChangedAt time.Time
}

// freshDisk — un disque présent : son dossier, et le dernier changement de ses
// sections hors jeux, lu la dernière fois qu'on l'a relu.
type freshDisk struct {
	dir      string
	sections time.Time
}

// Self — l'en-tête du listing vient d'être écrit ou lu : sa date.
func (freshness *Freshness) Self(at time.Time) {
	freshness.mu.Lock()
	defer freshness.mu.Unlock()

	freshness.self = latest(freshness.self, at)
	freshness.known = true
}

// Disks — les disques présents (nom → dossier), chaque fois que le kernel les
// regarde. La première fois, rien n'avance : c'est le démarrage, et le scan qui
// le suit date déjà les jeux. Ensuite, un disque apparu, parti ou déplacé fait
// avancer la date à maintenant : l'ami recopie un listing qui l'a, ou ne l'a
// plus.
func (freshness *Freshness) Disks(present map[string]string) {
	freshness.mu.Lock()
	defer freshness.mu.Unlock()

	if freshness.disks != nil && sameDisks(freshness.disks, present) {
		return
	}
	if freshness.disks != nil {
		freshness.disksChangedAt = time.Now().UTC()
	}
	kept := make(map[string]freshDisk, len(present))
	for name, dir := range present {
		disk := freshDisk{dir: dir}
		if known, ok := freshness.disks[name]; ok && known.dir == dir {
			disk.sections = known.sections
		}
		kept[name] = disk
	}
	freshness.disks = kept
}

func sameDisks(known map[string]freshDisk, present map[string]string) bool {
	if len(known) != len(present) {
		return false
	}
	for name, dir := range present {
		if disk, ok := known[name]; !ok || disk.dir != dir {
			return false
		}
	}

	return true
}

// Disk — le dernier changement des sections d'un disque présent, lu sur lui.
func (freshness *Freshness) Disk(name string, at time.Time) {
	freshness.mu.Lock()
	defer freshness.mu.Unlock()

	if disk, ok := freshness.disks[name]; ok {
		disk.sections = latest(disk.sections, at)
		freshness.disks[name] = disk
	}
}

// UpdatedAt — la date à rendre, et vrai une fois l'en-tête connu. `scanned`
// donne, DE MÉMOIRE, la date du scan des jeux d'un disque (son nom et son
// dossier) : elle ne doit rien lire sur lui.
func (freshness *Freshness) UpdatedAt(scanned func(name, dir string) time.Time) (time.Time, bool) {
	freshness.mu.Lock()
	defer freshness.mu.Unlock()

	if !freshness.known {
		return time.Time{}, false
	}
	at := latest(freshness.self, freshness.disksChangedAt)
	for name, disk := range freshness.disks {
		at = latest(at, disk.sections)
		if scanned != nil {
			at = latest(at, scanned(name, disk.dir))
		}
	}

	return at.UTC(), true
}

// latest — la plus récente de deux dates.
func latest(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}

	return first
}
