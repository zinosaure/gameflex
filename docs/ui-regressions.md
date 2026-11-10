# Correctif des parcours des 22 interfaces

Travail sur `cleanup/interface-destinations`. L’état du client existant a été capturé dans le commit `c584203` et dans `.local/ui-regressions/before/`, avant les corrections. Le statut initial des travaux non commités est conservé dans `.local/ui-regressions/initial-status.txt`. Les changements préexistants du moteur, de la diffusion et des téléchargements restent séparés du correctif.

## Reproduction et causes

La reproduction utilise le fournisseur de préférences, le routeur, les 22 compositions et les fiches réels avec un transport de fixtures indépendant. Aucun mot de passe ni compte public n’est utilisé. Le rapport initial est `output/ui-regressions-before.json`.

| Symptôme | Reproduction initiale et cause |
|---|---|
| Mes jeux interrompu après changement d’interface | Horizon, La collection et Signal CRT affichent les jeux. Minimal échoue avec un statut du kernel `{status, emulators}` : son filtre et sa cellule de statut tentaient de rendre l’objet directement. |
| Archives interrompu depuis Accueil ou Favoris | Le filtre d’état d’Archives rendait lui aussi les objets `g.status` dans ses options. Ce second plantage a été reproduit séparément avant de confirmer la cause commune avec Minimal. L’ancien navigateur de fichiers pouvait également afficher l’objet. |
| Thème perdu sur les rubriques | Le registre choisissait correctement l’identifiant ; les rubriques communes utilisaient les couleurs par défaut de `gx-app`. Le décor et les données du mode restaient couplés au choix de composition de l’accueil. |
| Rechargement du lien `/gameflex?section=management` | La redirection Nginx de `/gameflex` vers `/gameflex/` supprimait la requête. Le contrôle HTTPS initial a confirmé un 308 sans `section`. Les modèles du dépôt conservent désormais `$is_args$args` ; seule la prévisualisation les applique pendant ces essais. |
| Plus de Mosaïque Joy ouvre Collections | Le raccourci `more` était associé directement à la destination `collections`, sans menu. |
| Faux vide dans les trois accueils | Les composants d’accueil recevaient un tableau vide avant la fin du registre des consoles et de la lecture des jeux. Certains initialisaient leur sélection une seule fois sur ce tableau vide. |
| Préférence ancienne qui revient | Le choix était sauvegardé après la synchronisation seulement ; une réponse GET tardive pouvait remplacer le mode local. Le sélecteur remplaçait aussi entièrement la page, perdant son état local. |

`game-presentation.js` dérive maintenant un code et un libellé sans modifier le rapport du kernel ni ses données par émulateur. Les deux écrans sont testés séparément, avec l’objet complet et avec les anciennes chaînes de caractères.

## Comportement corrigé

- La version du cache PWA passe de 47 à 48 pour que le prochain déploiement remplace aussi le bundle conservé par le service worker. Les caches des autres applications restent isolés.
- La rubrique, le filtre de recherche et le thème sont des états indépendants. Les 22 compositions d’accueil et les agencements de bibliothèque existants sont conservés. Le thème partagé vient de la palette de l’interface, notamment le crème de La collection et les couleurs d’Horizon et Signal CRT ; les fiches, réglages et panneaux de compte utilisent cette même palette.
- Les erreurs de rendu sont contenues à l’intérieur de la rubrique. Le menu reste disponible et « Réessayer sur place » recharge ses données sans navigation forcée. Un changement de rubrique ou de filtre permet aussi de sortir d’un état d’erreur.
- Une minuterie de focus de Bureau du collectionneur pouvait reprendre le focus au menu pendant un rafraîchissement, notamment dans le parcours TV. Elle est annulée au démontage et ne déplace plus le focus si une modale ou une commande utilisateur est active. La sélection est retrouvée par référence stable.
- Le sélecteur est une superposition ; sa fermeture conserve la page, les données et le filtre et rend le focus à la commande d’origine. Un choix explicite conserve la destination compatible, y compris la console ouverte. La recherche reste disponible lors du passage d’un agencement rétro à Minimal.
- Mosaïque Joy possède un vrai menu Plus. Les accueils possèdent aussi un menu principal commun avec Paramètres, Changer d’interface et les rubriques. Il gère Échap, Tab et les flèches. Les commandes restent disponibles pendant le chargement et en cas d’erreur.
- Le chargement du registre des consoles est pris en compte avant de conclure à une bibliothèque vide. Un chargement, une bibliothèque réellement vide, une rubrique vide, une recherche sans résultat et un échec de lecture ont des messages différents. Des données déjà disponibles peuvent rester visibles pendant un rechargement.
- Le choix explicite est enregistré immédiatement dans le stockage du compte et de l’origine API. Une révision locale bloque les réponses de lecture commencées avant un nouveau choix. Une préférence locale datée reste prioritaire sur une réponse non datée ou plus ancienne, y compris après un rechargement hors ligne. L’API conserve le champ optionnel `interfaceUpdatedAt` et continue d’accepter les préférences anciennes.
- Le nombre de références installées n’est pas recalculé depuis les résultats de recherche. Le parcours de référence utilise 432 références **simulées**, dont deux correspondent à Advance Wars. Le regroupement des éditions et les identifiants utilisés pour lancer les jeux ne changent pas.
- La fiche présente un titre court et des champs distincts pour l’édition, la région et les langues connues. Une seule édition donne un libellé simple, sans sélecteur inutile. Les éditions installées distinctes restent sélectionnables.
- Reprendre demande une compatibilité de sauvegarde explicite ; un jeu simplement récent indique Relancer. Une sauvegarde dont l’existence seule est connue n’est pas présentée comme compatible. Un jeu qui n’est pas récent indique Jouer. Le titre de Collections ne se répète plus dans la rubrique.
- `/gameflex/settings?advanced=1` est conservé. Les libellés du compte et de la bibliothèque sont en français dans cette expérience, sans modifier la préférence globale de langue ; le lien vers cette même page y est retiré.

Les palettes du pack ont été recalées sur les styles d’accueil effectivement calculés, plutôt que sur les anciennes valeurs des fiches. `ui-palettes-browser.py` désactive momentanément la seule couche partagée pour mesurer ces styles, puis vérifie la palette des rubriques et des fiches ainsi que le maintien des dégradés. Les 66 contrôles de palette complètent les 66 parcours fonctionnels.

La [matrice des 22 interfaces](ui-regressions-matrix.md) détaille chaque résultat. Le code et les tests sont dans `0411255`.

## Prévisualisation et reproduction des tests

La prévisualisation séparée est servie sur **http://192.168.1.63:18215/gameflex/** par le conteneur d’aperçu existant. Ses requêtes, préférences et lancements sont simulés. La version publique n’est pas déployée par cette intervention. Aucun fichier de sauvegarde n’est écrit ou supprimé.

```sh
node tests/pi-tv/run-interface-tests.cjs
PYTHONPATH=tools/pi-tv python3 -m unittest discover -s tests/pi-tv -p 'test_interface*.py'
node tests/pi-tv/build-interface-preview.cjs
python3 tests/pi-tv/serve-interface-preview.py --port 18197
```

Avec l’image de test Chromium déjà présente, et une adresse de serveur accessible depuis son réseau :

```sh
INTERFACE_PREVIEW=http://127.0.0.1:18197/gameflex/ \
INTERFACE_FORMAT=pc python3 tests/pi-tv/ui-regressions-browser.py
# Répéter avec INTERFACE_FORMAT=mobile puis tv.
INTERFACE_PREVIEW=http://127.0.0.1:18197/gameflex/ \
INTERFACE_FORMAT=pc python3 tests/pi-tv/ui-palettes-browser.py
# Répéter aussi l’audit de palette en mobile puis tv.
INTERFACE_PREVIEW=http://127.0.0.1:18197/gameflex/ \
INTERFACE_SCREENSHOTS=output/ui-regressions \
python3 tests/pi-tv/ui-regressions-scenarios.py
INTERFACE_PREVIEW=http://127.0.0.1:18197/gameflex/ \
python3 tests/pi-tv/ui-regressions-focus-launch.py
```

La matrice couvre, pour chaque mode et chaque format : Paramètres → activation au clavier ou au toucher → accueil → Mes jeux → recherche → fiche → retour avec filtre et focus → Archives → Paramètres → rechargement → Compte et bibliothèque. Le sélecteur est aussi fermé par Échap sans choix. Une empreinte du squelette DOM de chaque accueil, sans les textes ni les styles, vérifie que les compositions restent distinctes.

Les scénarios complémentaires rejouent l’ordre Horizon → La collection → Signal CRT → Minimal, puis Favoris → Archives et le rechargement du lien profond. Ils couvrent les 432 références de test, le changement explicite de thème sur GBA, les lectures lentes des consoles et jeux, une vraie bibliothèque vide, une erreur et sa reprise, une erreur de cellule confinée, les réponses de préférences anciennes, la synchronisation indisponible et le menu Plus au toucher et aux flèches.

Les rapports par format, les scénarios et les captures WebP sont dans `output/ui-regressions/`. Le test « TV » désigne Chromium en 1920 × 1080 avec navigation clavier/directionnelle. **Aucun essai matériel Philips n’a été réalisé** : aucun accès ADB n’est disponible dans cet environnement. Le lancement est couvert par le contrat de transport de prévisualisation et par une exécution du programme GBA original CC0 en mémoire (90 images non noires, 240 × 160, sans BIOS propriétaire) ; aucun jeu public n’a été lancé. Les crédits et les chemins des visuels existants restent conservés ; les captures de tests utilisent des contenus de fixtures et des images de repli.

## Limite de la preuve de reprise

L’API existante `savestate/slots` publie l’existence, la date et la taille des états, sans preuve de compatibilité avec le cœur courant. Ces seules informations ne déclenchent jamais « Reprendre ». Le rendu positif est testé avec une compatibilité explicitement déclarée ; sans cette preuve, un jeu récent reste « Relancer ». Aucune sauvegarde ancienne n’est chargée pour tenter d’en déduire la compatibilité.

La production reste disponible et inchangée. Le déploiement ultérieur devra appliquer le frontend, l’API des préférences et les deux modèles de proxy corrigés ; aucun fichier Nginx partagé hors dépôt n’a été modifié par cette intervention.
