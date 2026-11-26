// Repris de kernel/src/internal/retro_files/cores.go (chantier 61 ; le porteur, 2026-11-18 : « Ce sont du code que tu peux
// copier depuis Gameflex actuel ») : les cœurs, les émulateurs et les BIOS d'un disque, listés et résolus comme chez GameFlex.

package library

// Les cores, vus PAR CONSOLE.
//
// Un core est un fichier `<nom>_libretro.so`, et le registre partagé dit
// déjà quelle console utilise lequel. C'est cette table qui fait autorité,
// pas le rangement sur le disque : un même core sert souvent plusieurs
// consoles (genesis_plus_gx couvre la Master System ET la Mega Drive), donc
// son emplacement ne peut pas, à lui seul, dire à qui il appartient.
//
// DEUX dispositions sont lues, dans cet ordre :
//
//	cores/<consoleId>/<nom>_libretro.so   le rangement par console
//	cores/<nom>_libretro.so               la pile à plat
//
// La seconde est celle des bibliothèques existantes — plusieurs centaines de
// fichiers déposés côte à côte — et la lire évite d'imposer une migration
// pour afficher un écran. Un core rangé par console gagne : c'est le plus
// précis des deux.

import (
	"regexp"
	"strings"

	"gameflex/consoles"
)

// CoreFileName — le nom de fichier d'un core libretro.
func CoreFileName(core string) string {
	return core + "_libretro.so"
}

// AndroidCoreFileName — le nom de la variante Android d'un core : celui du
// buildbot, et celui que le téléphone range dans SA bibliothèque. Le nom suffit
// à distinguer les deux plateformes (décision 4 du chantier 13) : pas de
// sous-dossier par plateforme.
func AndroidCoreFileName(core string) string {
	return core + "_libretro_android.so"
}

// LES PAQUETS D'ÉMULATEURS DU TÉLÉPHONE (chantier 28, #447) : `<version>-<abi>.zip`,
// rangés sous la console qu'ils jouent (`cores/ps5/kyty-…-forky-r1-aarch64.zip`).
// Ce sont les émulateurs de bureau tels qu'un téléphone les installe — l'émulateur,
// ses bibliothèques, Box64 en aarch64 —, nommés par la version que son APK attend
// (la table `GUESTS` de `GlibcGuest`). Un serveur les garde pour ses amis : il ne
// les lance pas lui-même.
var phoneBundleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*-(x86_64|aarch64)\.zip$`)

// IsPhoneBundle — ce nom de fichier est-il celui d'un paquet du téléphone ? Il
// devient un chemin sur le disque : rien qui puisse en sortir.
func IsPhoneBundle(name string) bool {
	return phoneBundleName.MatchString(name)
}

// LES FICHIERS D'UN ÉMULATEUR DE BUREAU (2026-10-24, chantier 29) : ce qu'une
// console qui tourne en conteneur range dans `cores/<console>/` — l'AppImage ou
// l'archive de son éditeur, nos constructions modifiées
// (`shadps4-gameflex-<version>`), et les paquets du téléphone ci-dessus. Ils se
// listent et se servent comme les cœurs : c'est ainsi qu'un ami prend la
// dernière version chez nous, et qu'un miroir se remplit. Le porteur : « les
// dernières versions doivent être disponibles par fédération […] l'émulateur
// doit être dispo dans la liste des cores ».
var emulatorFileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// HasDesktopEmulator — cette console tourne-t-elle dans un émulateur de bureau ?
// Le registre le dit par un nom de cœur à espace (« KytyPS5 (streamex) »).
func HasDesktopEmulator(consoleID string) bool {
	for _, core := range consoles.CoresFor(consoleID) {
		if strings.Contains(core, " ") {
			return true
		}
	}

	return false
}

// IsEmulatorFile — ce nom est-il celui d'un fichier d'émulateur de cette
// console ? Il devient un chemin : rien qui puisse sortir du dossier, ni un
// fichier caché (une suppression en cours), ni un transfert inachevé.
func IsEmulatorFile(consoleID, name string) bool {
	return HasDesktopEmulator(consoleID) && emulatorFileName.MatchString(name) &&
		!strings.HasSuffix(name, ".part") && !strings.Contains(name, "_libretro")
}
