package retrogaming

// BIOS selection — some cores (PS1's pcsx_rearmed in particular) fall back
// to an imperfect HLE (High-Level Emulation) BIOS replacement whenever they
// can't find/pick a real BIOS dump, and HLE is well known to be
// incompatible with a meaningful number of real games. The flat BIOS_PATH
// root (see main.go/child.go's biosRoot) passed to libretro.InitEnvPaths as
// systemDir is an ES-DE/Batocera-style tree of PER-CONSOLE subfolders (e.g.
// {BIOS_PATH}/ps1/scph5501.bin), not a flat pile of files at the root
// itself — before this file existed, EVERY console's systemDir was that
// flat root regardless, so a core scanning it directly (as pcsx_rearmed
// does) found nothing at all and silently fell back to HLE. Confirmed via
// direct inspection of a real deployment's BIOS_PATH (2026-08-01) — 24 real
// PS1 BIOS files sitting one level deeper than any core would ever look.
import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gameflex/consoles"
)

// biosMirrorSubdir maps our consoleId to an ADDITIONAL path, relative to
// the session's temp systemDir, where the chosen BIOS file is written on
// top of the systemDir root. Most cores scan that root directly (which is
// why the root copy stays unconditional, keeping every already-working
// console byte-for-byte unchanged), but a core is free to look somewhere
// else entirely and never fall back: LRPS2 (the pcsx2 core) only ever
// looks in <systemDir>/pcsx2/bios and says so plainly before refusing the
// game — "Could not find any valid PS2 BIOS File in
// /tmp/retrogaming-bios-*/pcsx2/bios" (2026-08-12), which is exactly why
// retro_load_game returned false on a machine holding 70 valid dumps.
// Keyed by console rather than by core because the alternative (play, the
// other PS2 core) reads the root fine and is unaffected by an extra copy
// it never opens — 4 MiB duplicated once per session, in a directory
// already discarded when the session ends.
var biosMirrorSubdir = map[string]string{
	"ps2": "pcsx2/bios",
	// flycast ne lit que <systemDir>/dc — même histoire que LRPS2 ci-dessus,
	// et même symptôme : « retro_load_game returned false » sur une machine
	// qui a pourtant dc_boot.bin et dc_flash.bin sous la main (2026-09-08).
	// Les DEUX fichiers comptent ici, pas un seul : sans choix explicite, la
	// mise en scène recopie tout le dossier de la console, ce qui est
	// exactement ce qu'il faut.
	"dreamcast": "dc",
}

// biosCompanionCache — où vivent les fichiers que l'émulateur écrit à côté de
// son BIOS, quand on ne peut pas les écrire à côté.
//
// Sous les SAUVEGARDES, et le choix est le point : roms, cores et bios
// résolvent sous la racine FROIDE, montée en lecture seule ; seules les
// sauvegardes vivent sous la racine CHAUDE, qui est inscriptible (voir le
// commentaire de sectionPath dans kernel/internal/docker/docker.go, qui pose
// cette frontière). Écrire un cache ailleurs qu'ici échouerait exactement
// comme ce qu'on cherche à contourner. Un dossier par console et par
// entrée, parce que deux dumps PS2 ne partagent ni leur région ni leur
// horloge. Chaîne vide quand on ne sait pas où écrire — l'appelant retombe
// alors sur l'ancien comportement, sans compagnon persistant.
func biosCompanionCache(savesRoot, consoleID, bios string) string {
	if strings.TrimSpace(savesRoot) == "" || consoleID == "" || bios == "" {
		return ""
	}

	return filepath.Join(savesRoot, ".bios-companions", consoleID, sanitizeEntryName(bios))
}

// sanitizeEntryName rend un nom d'entrée utilisable comme nom de dossier. Une
// entrée vient du disque, mais elle a traversé une requête : un nom de cache
// ne doit jamais pouvoir désigner un fichier ailleurs.
func sanitizeEntryName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)

	return strings.TrimLeft(cleaned, ".")
}

// resolveBiosDir picks the systemDir to actually pass to
// libretro.InitEnvPaths for this session — biosRoot UNCHANGED if no
// explicit choice was made (bios == "") or this console has nothing to
// choose from (not a registry console): the exact same behavior as before
// BIOS selection existed, so a caller that never sends `bios` at all sees
// zero change. Otherwise, copies ONLY the chosen file into a fresh
// session-scoped temp directory and returns THAT as systemDir — forcing
// just that one file into an otherwise-empty directory is what actually
// makes the core use it: most cores auto-detect a BIOS by scanning every
// file in systemDir against known filenames/checksums, so leaving 20+
// regional variants all visible at once hands that choice to the core's
// own heuristic (not necessarily the game-correct region) instead of the
// user's. tempDir is "" when no cleanup is needed (the biosRoot-unchanged
// path); the caller (handleStart) is responsible for os.RemoveAll-ing a
// non-empty one once the session ends (see emulationSession.biosTempDir).
func resolveBiosDir(biosRoot, savesRoot, consoleID, bios string) (dir string, tempDir string, chosen string, err error) {
	// LE DOSSIER EST L'IDENTIFIANT DE LA CONSOLE, et c'est le registre qui le
	// dit (consoles.BiosDir, décidé le 2026-09-01). Ce fichier tenait sa propre
	// table, restée à « ps1 → psx » alors que le disque et le sélecteur du
	// kernel étaient passés à `ps1/` : un BIOS choisi dans la fiche était
	// cherché dans un dossier qui n'existait plus. Et la table ne connaissait
	// que deux consoles — la Dreamcast, pourtant dotée d'un miroir, rendait la
	// racine avant de l'atteindre, et un choix fait pour toute autre console
	// était jeté sans un mot (corrigé le 2026-09-14).
	//
	// Seule une console HORS registre n'a rien à choisir : son identifiant ne
	// désigne aucun dossier, et le joindre à la racine mettrait en scène
	// n'importe quoi.
	if !consoles.Exists(consoleID) {
		return biosRoot, "", bios, nil
	}
	subdir := consoles.BiosDir(consoleID)

	// SANS CHOIX, ON MET QUAND MÊME EN SCÈNE — quand le cœur ne lit QUE le
	// miroir.
	//
	// Le raccourci « pas de choix, pas de mise en scène » était juste pour tout
	// cœur qui balaie la racine de son systemDir. LRPS2 ne la balaie pas : il ne
	// regarde que `<systemDir>/pcsx2/bios`, et ce dossier n'existe QUE si on l'a
	// fabriqué. Sans choix explicite, il cherchait donc dans
	// `<racine des BIOS>/pcsx2/bios` — un dossier que personne ne crée — et
	// refusait le jeu : « Could not find any valid PS2 BIOS File in
	// /gameflex/bios/pcsx2/bios » (relevé sur une vraie session le 2026-09-08).
	// La PS2 était donc injouable tant qu'on n'avait pas choisi soi-même, sans
	// que rien ne le dise.
	//
	// On met alors en scène TOUT ce que la console a : c'est exactement le
	// comportement attendu quand personne n'a choisi — le cœur balaie et décide,
	// comme il l'aurait fait à la racine s'il la lisait.
	// ET LE RACCOURCI ÉTAIT FAUX POUR TOUTE CONSOLE QUI DÉCLARE DES BIOS (#144).
	//
	// Il reposait sur « un cœur qui balaie la racine de son systemDir s'en
	// sortira tout seul ». Mesuré le 2026-09-17 : la racine des BIOS ne contient
	// QUE des dossiers de console (`3do/`, `dreamcast/`, `ps1/`…) et pas un seul
	// fichier. Un cœur qui la balaie n'y trouve donc jamais rien — `pcsx_rearmed`
	// écrivait `No BIOS files found.` à chaque session et se rabattait sur son
	// BIOS émulé EN SILENCE, alors que `bios/ps1/` portait vingt-quatre vrais
	// dumps. Les jeux tournaient quand même, mais le HLE casse une partie des
	// titres et rien ne le disait.
	//
	// On met donc en scène dès que la console déclare des BIOS : le cœur balaie
	// alors un dossier qui en contient, et décide — ce que le raccourci
	// prétendait déjà lui laisser faire. Rien ne change pour une console qui
	// n'en déclare aucun (elle garde le chemin économique, sans dossier
	// temporaire), ni pour la PS2 et la Dreamcast, qui passaient déjà par ici
	// via leur miroir.
	//
	// Le coût est celui que ce fichier accepte déjà pour la PS2 : quelques
	// mégaoctets recopiés une fois par session, dans un dossier jeté à la fin
	// (douze pour les vingt-quatre dumps d'une PS1 bien garnie).
	_, needsMirror := biosMirrorSubdir[consoleID]
	declaresBios := len(consoles.BiosFor(consoleID)) > 0
	if bios == "" && !needsMirror && !declaresBios {
		return biosRoot, "", bios, nil
	}

	srcDir, err := safeJoin(biosRoot, subdir)
	if err != nil {
		return "", "", "", err
	}

	var staged map[string]string
	if bios == "" {
		staged, err = biosDirFiles(srcDir)
	} else {
		var srcPath string
		srcPath, err = safeJoin(srcDir, bios)
		if err != nil {
			return "", "", "", err
		}
		staged, err = biosEntryFiles(srcPath, bios)
	}
	if err != nil {
		return "", "", "", err
	}
	if len(staged) == 0 {
		// Rien à mettre en scène : on rend la racine, et le cœur dira lui-même
		// ce qui lui manque. Fabriquer un dossier vide ne ferait que déplacer
		// le message d'erreur.
		return biosRoot, "", bios, nil
	}

	tmp, err := os.MkdirTemp("", "retrogaming-bios-*")
	if err != nil {
		return "", "", "", err
	}
	// Every file of the entry lands FLAT in the temp root (and flat again in
	// the mirror), whichever shape it came from — a core scans a directory,
	// it doesn't care that we grouped a dump's files in a folder on disk.
	mirror := ""
	if sub, ok := biosMirrorSubdir[consoleID]; ok {
		mirror = filepath.Join(tmp, filepath.FromSlash(sub))
		if err := os.MkdirAll(mirror, 0o755); err != nil {
			os.RemoveAll(tmp)
			return "", "", "", err
		}
	}
	for name, path := range staged {
		data, err := os.ReadFile(path)
		if err != nil {
			os.RemoveAll(tmp)
			return "", "", "", fmt.Errorf("bios file not readable: %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o644); err != nil {
			os.RemoveAll(tmp)
			return "", "", "", err
		}
		if mirror != "" {
			if err := os.WriteFile(filepath.Join(mirror, name), data, 0o644); err != nil {
				os.RemoveAll(tmp)
				return "", "", "", err
			}
		}
	}

	// LES COMPAGNONS D'ABORD MIS PAR L'ÉMULATEUR, RELUS ICI.
	//
	// Ils vivent dans un cache inscriptible parce que le volume des BIOS ne
	// l'est pas (voir biosCompanionCache). Sans ce dépôt, une PS2 repartirait à
	// chaque partie avec sa langue, son horloge et sa région d'usine — ce qui
	// se voit tout de suite, et ce que le regroupement en dossiers cherchait
	// justement à éviter.
	//
	// Ils ne remplacent JAMAIS un fichier du dump : c'est le BIOS qui fait foi,
	// le cache ne fait que compléter.
	overlayCachedCompanions(biosCompanionCache(savesRoot, consoleID, bios), tmp, mirror, staged)

	// ET ON EN DÉSIGNE UN, parce que certains cœurs ne choisissent pas.
	//
	// pcsx2 énumère les dumps qu'il voit (« Bios Found: Japan v02.20 ») puis
	// refuse le jeu quand même : il attend qu'une option de cœur — `pcsx2_bios`
	// — nomme celui à prendre. Sans choix explicite de l'utilisateur, personne
	// ne la remplissait, et « aucun BIOS valable » s'affichait alors que trois
	// venaient d'être trouvés (relevé sur une vraie session le 2026-09-08).
	//
	// On en désigne donc un : le plus RÉCENT (voir defaultStagedBios), dans un
	// ordre stable — un ordre de parcours de dossier changerait d'une machine à
	// l'autre pour la même bibliothèque.
	if chosen = bios; chosen == "" {
		chosen = defaultStagedBios(staged)
	}

	return tmp, tmp, chosen, nil
}

// defaultStagedBios désigne le dump à nommer quand l'utilisateur n'a rien
// choisi. Les `.bin` d'abord — c'est la forme d'un BIOS — et le DERNIER dans
// l'ordre alphabétique.
//
// Le dernier, pas le premier, et ça compte. Les dumps se nomment par version
// (`ps2-0100j`, `ps2-0120a`, `ps2-0220e`…) : l'ordre alphabétique est donc
// l'ordre des versions, et prendre le premier revenait à désigner le plus
// VIEUX. Vu en vrai le 2026-09-08 : `scph10000.bin`, le tout premier BIOS
// japonais de janvier 2000, dump incomplet par-dessus le marché
// (« BIOS rom1 module not found, skipping ») — il démarre, n'atteint jamais
// le disque, et le jeu reste sur un écran noir sans que rien n'échoue.
//
// Un BIOS récent joue les jeux anciens ; l'inverse est faux. À défaut de
// savoir lequel l'utilisateur veut, le plus récent est le moins mauvais pari
// — et il peut toujours choisir lui-même dans la fiche du jeu.
func defaultStagedBios(staged map[string]string) string {
	names := make([]string, 0, len(staged))
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)

	for i := len(names) - 1; i >= 0; i-- {
		if strings.EqualFold(filepath.Ext(names[i]), ".bin") {
			return names[i]
		}
	}
	if len(names) > 0 {
		return names[len(names)-1]
	}

	return ""
}

// overlayCachedCompanions dépose dans le dossier de session ce que l'émulateur
// avait écrit lors d'une partie précédente. Silencieux de bout en bout : un
// cache absent est le cas ORDINAIRE (première partie), pas une erreur.
func overlayCachedCompanions(cacheDir, tmp, mirror string, staged map[string]string) {
	if cacheDir == "" {
		return
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || isHidden(name) {
			continue
		}
		if _, fromDump := staged[name]; fromDump {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, name))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(tmp, name), data, 0o644)
		if mirror != "" {
			_ = os.WriteFile(filepath.Join(mirror, name), data, 0o644)
		}
	}
}

// biosEntryFiles resolves one chosen BIOS entry to the set of files to
// stage, keyed by the name each must keep in the temp systemDir. A plain
// file is itself; a directory is every non-hidden file directly inside it
// (not recursive — a BIOS dump is a flat handful of files, and recursing
// would only invent nesting no core looks for).
func biosEntryFiles(srcPath, bios string) (map[string]string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("bios file not found: %s: %w", bios, err)
	}
	if !info.IsDir() {
		return map[string]string{bios: srcPath}, nil
	}
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || isHidden(e.Name()) {
			continue
		}
		files[e.Name()] = filepath.Join(srcPath, e.Name())
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("bios folder holds no files: %s", bios)
	}
	return files, nil
}

// maxBiosCompanionBytes caps what persistBiosCompanions is willing to copy
// back. A real companion is tiny (PS2's .NVM is 1 KiB, its .MEC is 4
// bytes); the cap exists so that anything unexpectedly large a core decides
// to drop in its system directory stays in the temp dir and gets discarded
// with it, rather than silently growing the BIOS tree.
const maxBiosCompanionBytes = 1 << 20

// persistBiosCompanions copies files a core CREATED inside the session's
// temp systemDir back to where the chosen BIOS actually lives, so console
// state survives the session instead of being regenerated from scratch
// every launch. PS2 is the case that motivated it: pcsx2 writes an .NVM
// (NVRAM — console language, timezone, clock) and a .MEC next to whichever
// BIOS it loaded, and logs "Failed to open or read NVRAM ... creating
// default" on every single start without them.
//
// ONLY files that don't already exist at the destination are written: a
// companion is created once and then belongs to the user, so a later
// session can never overwrite settings they've changed, and the BIOS dump
// itself (always already there) is never rewritten. Best-effort throughout
// — this runs on the teardown path, where failing a session over an
// unwritable optional file would be absurd.
//
// Note this is the only place the backend writes outside saves/. Two PS2
// sessions on the SAME BIOS could race here, but both would be creating a
// default-valued file that neither has customized yet, so the loser of that
// race loses nothing.
func persistBiosCompanions(biosRoot, savesRoot, consoleID, bios, tempDir string) {
	if !consoles.Exists(consoleID) || bios == "" || tempDir == "" {
		return
	}
	subdir := consoles.BiosDir(consoleID) // voir resolveBiosDir
	srcDir, err := safeJoin(biosRoot, subdir)
	if err != nil {
		return
	}
	srcPath, err := safeJoin(srcDir, bios)
	if err != nil {
		return
	}
	// A folder-shaped entry keeps its companions inside the folder; a flat
	// .bin keeps them beside it, matching whichever layout is already in use.
	destDir := srcDir
	if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
		destDir = srcPath
	}

	// ET SI LE VOLUME EST EN LECTURE SEULE — ce qui est le cas courant en
	// déploiement — on écrit dans le cache inscriptible plutôt que de renoncer.
	// On le vérifie avant d'écrire quoi que ce soit : découvrir l'échec fichier
	// par fichier ne dirait rien de plus et écrirait la moitié d'un jeu de
	// compagnons dans un endroit et l'autre moitié ailleurs.
	cacheDir := biosCompanionCache(savesRoot, consoleID, bios)
	if !writable(destDir) {
		if cacheDir == "" {
			log.Printf("[retrogaming-bios] %s est en lecture seule et aucun cache de compagnons n est configuré : ceux de %s seront reperdus à chaque partie", destDir, bios)

			return
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			log.Printf("[retrogaming-bios] cache de compagnons inutilisable (%v)", err)

			return
		}
		destDir = cacheDir
	}

	// The mirror is where a core that ignores the systemDir root (pcsx2,
	// see biosMirrorSubdir) actually writes, so it's scanned first and wins
	// over a same-named file at the root.
	scan := []string{}
	if sub, ok := biosMirrorSubdir[consoleID]; ok {
		scan = append(scan, filepath.Join(tempDir, filepath.FromSlash(sub)))
	}
	scan = append(scan, tempDir)

	seen := map[string]bool{}
	for _, dir := range scan {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || isHidden(name) || seen[name] {
				continue
			}
			seen[name] = true
			info, err := e.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxBiosCompanionBytes {
				continue
			}
			dest, err := safeJoin(destDir, name)
			if err != nil {
				continue
			}
			if _, err := os.Stat(dest); err == nil {
				continue // already on disk — never overwrite
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				log.Printf("[retrogaming-bios] could not persist companion %s: %v", name, err)
				continue
			}
			log.Printf("[retrogaming-bios] created companion %s next to %s", name, bios)
		}
	}
}

// writable — peut-on écrire dans ce dossier ? Une question qu'on POSE au
// système de fichiers plutôt que de la déduire des permissions affichées : un
// montage en lecture seule rend des permissions parfaitement engageantes et
// refuse quand même l'écriture.
func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".gameflex-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)

	return true
}

// biosDirFiles rassemble tout ce qu'une console a comme BIOS, à plat — le cas
// « personne n'a choisi ». Les dossiers d'entrée sont descendus d'un cran, pour
// que leurs fichiers arrivent au même niveau que les .bin nus : un cœur balaie
// un dossier, il ne va pas y chercher une arborescence.
func biosDirFiles(srcDir string) (map[string]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}

		return nil, err
	}

	files := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if isHidden(name) {
			continue
		}
		path := filepath.Join(srcDir, name)
		if !entry.IsDir() {
			files[name] = path

			continue
		}
		inner, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		for _, child := range inner {
			if child.IsDir() || isHidden(child.Name()) {
				continue
			}
			// Le premier gagne : deux dumps qui portent le même nom de
			// fichier sont le cas d'un dossier mal rangé, pas une raison de
			// refuser de démarrer.
			if _, seen := files[child.Name()]; !seen {
				files[child.Name()] = filepath.Join(path, child.Name())
			}
		}
	}

	return files, nil
}
