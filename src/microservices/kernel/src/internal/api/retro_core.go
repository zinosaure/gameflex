package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
)

// requireRetroCore — le cœur d'une partie rétro doit être là AVANT de lancer.
//
// IL NE SE TÉLÉCHARGE PLUS AU LANCEMENT, et c'est une décision de
// l'utilisateur (2026-09-17, #143). Ça l'a fait un temps, par analogie avec le
// téléphone (#135) : un cœur absent partait chercher `latest/` chez le
// buildbot au moment où quelqu'un appuyait sur Jouer. Le problème n'est pas le
// téléchargement, c'est le MOMENT — le binaire le plus récent, jamais éprouvé,
// arrivait pendant une partie. Quand `latest` a dépassé la libstdc++ de
// l'image de session, deux consoles sont devenues injouables (GameCube et Wii)
// sans que rien ne dise pourquoi : `dlopen` échoue en silence.
//
// Un cœur manquant est donc désormais une erreur CLAIRE, qui envoie vers
// l'écran où l'on choisit — Réglages → Stockage → Émulateurs. Là, le
// téléchargement reste disponible, il refuse d'écraser un fichier existant, et
// il vérifie l'ABI de ce qu'il descend avant de l'installer
// (`verifyCoreABI`). C'est la même opération, à un endroit où elle se voit.
func (server *Server) requireRetroCore(consoleID, core string) error {
	return requireCore(server.retroCoresRoot(), consoleID, core)
}

// requireBios — le firmware qu'une console EXIGE doit être là avant de lancer :
// la règle de la page (`consoles.MissingRequiredBios`), lue dans le même
// listing qu'elle (`/bios`). Un BIOS facultatif n'arrête rien ; un requis
// absent, si. La page grise « Jouer » ; ce refus tient pour ce qui passerait
// outre — la façade, un lien (#393, 2026-10-16). Rétro comme bureau : la PS3
// veut son PS3UPDAT.PUP, la Switch ses prod.keys.
func (server *Server) requireBios(consoleID string) error {
	names := []string{}
	for _, item := range server.selfItems(listing.Bios, consoleID) {
		names = append(names, item.Name)
	}
	if missing := consoles.MissingRequiredBios(consoleID, names); len(missing) > 0 {
		return fmt.Errorf("le firmware de cette console est requis et n'est pas installé (%s) : "+
			"le télécharger depuis Réglages → Stockage → Firmwares", strings.Join(missing, ", "))
	}

	return nil
}

// requireCore — le travail de requireRetroCore, sans serveur : un dossier de
// cœurs, une console, et un cœur (vide = le premier que le registre déclare,
// le même choix que le runtime).
//
// Les deux emplacements que le runtime lit (resolveCoreFile) comptent : sous la
// console d'abord, puis à plat.
//
// UN CŒUR QUE LE REGISTRE NE DÉCLARE PAS POUR CETTE CONSOLE EST REFUSÉ. Il était
// laissé au runtime, « qui dira qu'il manque » — mais son nom vient de la
// requête, et le runtime le joint tel quel au dossier des cœurs avant de
// charger la bibliothèque : `../../saves/<compte>/…` lui faisait charger un
// `_libretro.so` pris ailleurs (revue de sécurité, 2026-10-28).
func requireCore(root, consoleID, core string) error {
	consoleID = strings.ToLower(strings.TrimSpace(consoleID))
	core = strings.TrimSpace(core)
	if core == "" {
		declared := consoles.CoresFor(consoleID)
		if len(declared) == 0 {
			return nil
		}
		core = declared[0]
	}
	if !coreBelongsTo(consoleID, core) {
		return fmt.Errorf("« %s » n'est pas un cœur de cette console", core)
	}

	// UN ÉMULATEUR EN CONTENEUR N'EST PAS UN FICHIER À EXIGER.
	//
	// Le registre déclare aussi des noms comme « RPCS3 (streamex) » ou
	// « xemu (streamex) » : ils nomment un émulateur qui tourne dans son propre
	// conteneur, pas une bibliothèque libretro posée ici. L'espace est le
	// signe, et `ListCoreFiles` les écarte déjà pour cette raison — sans la
	// même règle ici, on réclamerait un « RPCS3 (streamex)_libretro.so » qui
	// n'existera jamais, et la console refuserait de démarrer.
	//
	// Ces consoles passent aujourd'hui par le lancement streamex, donc ce
	// chemin ne les voit pas. La garde est là pour que ça reste vrai même si
	// quelqu'un route un jour une de ces consoles par ici.
	if strings.Contains(core, " ") {
		return nil
	}

	file := retro_files.CoreFileName(core)
	for _, candidate := range []string{filepath.Join(root, consoleID, file), filepath.Join(root, file)} {
		if _, err := os.Stat(candidate); err == nil {
			return nil
		}
	}

	return fmt.Errorf(
		"le cœur « %s » de cette console n'est pas installé : le télécharger depuis "+
			"Réglages → Stockage → Émulateurs, ou le déposer dans cores/%s/%s",
		core, consoleID, file)
}
