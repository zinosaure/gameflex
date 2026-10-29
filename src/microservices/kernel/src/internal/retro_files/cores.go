package retro_files

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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gameflex/consoles"
)

// CoreEntry — un core tel que l'écran le montre.
type CoreEntry struct {
	Name string `json:"name"`
	// File est le nom de fichier réel (`fceumm_libretro.so`), ce qu'il faut
	// pour le supprimer. Vide quand le core est déclaré par le registre mais
	// absent du disque — l'écran le montre alors comme non installé plutôt
	// que de le taire.
	File      string `json:"file"`
	SizeBytes int64  `json:"sizeBytes"`
	Installed bool   `json:"installed"`
	// Scoped dit si le fichier trouvé était rangé sous sa console. Ce que
	// l'interface en fait la regarde ; ce que le lanceur en fait, non — il
	// accepte les deux.
	Scoped bool `json:"scoped"`
	// LA VARIANTE ANDROID (chantier 13, phase 5) : `<nom>_libretro_android.so`,
	// rangée à côté de la version Linux, sous sa console seulement. C'est
	// elle que les téléphones de ce serveur viennent prendre.
	AndroidInstalled bool  `json:"androidInstalled,omitempty"`
	AndroidSizeBytes int64 `json:"androidSizeBytes,omitempty"`
}

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

// ListCoreFiles rend les cores de cette console, installés ou non.
//
// La liste vient du REGISTRE, pas du dossier : une console a les cores
// qu'elle a, et un dossier vide doit se lire « rien d'installé » plutôt que
// « cette console n'existe pas ». Les entrées qui ne sont pas des cores
// libretro (« RPCS3 (streamex) ») sont écartées : elles nomment un
// émulateur qui tourne dans son propre conteneur, pas un fichier d'ici.
func ListCoreFiles(coresRoot, consoleID string) ([]CoreEntry, error) {
	var wanted []string
	for _, console := range consoles.All() {
		if console.ID == consoleID {
			wanted = console.Cores

			break
		}
	}

	entries := make([]CoreEntry, 0, len(wanted))
	for _, core := range wanted {
		if strings.Contains(core, " ") {
			continue // un émulateur en conteneur, pas un core libretro
		}

		entry := CoreEntry{Name: core, File: CoreFileName(core)}
		scoped := filepath.Join(coresRoot, consoleID, entry.File)
		flat := filepath.Join(coresRoot, entry.File)

		if info, err := os.Stat(scoped); err == nil && !info.IsDir() {
			entry.Installed, entry.Scoped, entry.SizeBytes = true, true, info.Size()
		} else if info, err := os.Stat(flat); err == nil && !info.IsDir() {
			entry.Installed, entry.SizeBytes = true, info.Size()
		}
		if info, err := os.Stat(filepath.Join(coresRoot, consoleID, AndroidCoreFileName(core))); err == nil && !info.IsDir() {
			entry.AndroidInstalled, entry.AndroidSizeBytes = true, info.Size()
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries, nil
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

// EmulatorFile — un fichier d'émulateur, tel que le listing l'annonce. Phone :
// c'est un paquet du téléphone (variante « bundle »), sinon celui du serveur
// (variante « emulator »).
type EmulatorFile struct {
	File      string
	SizeBytes int64
	Phone     bool
}

// ListEmulatorFiles — les fichiers d'émulateur rangés sous cette console, ceux
// du serveur comme ceux du téléphone.
func ListEmulatorFiles(coresRoot, consoleID string) []EmulatorFile {
	if !HasDesktopEmulator(consoleID) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(coresRoot, consoleID))
	if err != nil {
		return nil
	}
	files := []EmulatorFile{}
	for _, entry := range entries {
		if entry.IsDir() || !IsEmulatorFile(consoleID, entry.Name()) {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, EmulatorFile{
				File: entry.Name(), SizeBytes: info.Size(), Phone: IsPhoneBundle(entry.Name()),
			})
		}
	}

	return files
}
