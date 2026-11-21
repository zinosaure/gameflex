# Interfaces et rubriques communes

Branche : `cleanup/interface-destinations`. L’instance publique et ses données ne sont pas déployées ou modifiées par cette intervention. Le dépôt contenait déjà un chantier non commité ; les commits `b6ed1af` et `2887ba5` capturent les fichiers existants concernés avant nettoyage. Leur copie locale complète et le statut initial sont dans `.local/interface-cleanup/before/` et `.local/interface-cleanup/initial-status.txt`. L’auteur des commits est le compte d’automatisation CodexPi, sans identité personnelle déduite.

## Registre et préférences

Le registre historique `src/webapp/src/app/gameflex/experience/interfaces.js` conserve les 15 identifiants. `selectableInterfaces` sélectionne les neuf apparences ; `movedInterfaces` est la correspondance des six anciens modes. Aucun identifiant conservé n’est renommé.

| Identifiant conservé | Interface |
|---|---|
| station | Horizon |
| crt | Signal CRT |
| shelf | La collection |
| orbit | Orbitale |
| arcade | Borne Arcade |
| minimal | Minimal |
| covers | Jaquettes |
| consoles | Consoles |
| dashboard | Tableau de bord |

| Ancien identifiant | Destination |
|---|---|
| favorites | `/?section=favorites` : Favoris |
| resume | `/?section=resume` : Récents ; raccourci Reprendre du Tableau de bord |
| collections | `/?section=collections` : Collections |
| detail | Fiche commune du premier jeu encore disponible dans l’historique ; sinon Mes jeux |
| archives | `/?section=management&view=archives` : Gestion / Archives |
| multiplayer | `/?section=multiplayer` : Multijoueur |

Les clés locales restent `gameflex:interface:v1:<origine API encodée>:<compte encodé>` et leur suffixe `:library`. `preferenceKey` conserve donc l’isolation des comptes et origines. Les champs `favorites`, `history`, `collections` et leurs identifiants sont inchangés. L’API authentifiée `/experience-api/v1/preferences` conserve aussi `previousInterface` et `interfaceDestination`, ce dernier pouvant devenir `null` après restauration. Les anciens clients peuvent encore enregistrer les 15 identifiants.

La migration pure conserve les tableaux, remplace seulement un mode déplacé par `station`, mémorise son identifiant et prépare une destination. Une seconde exécution sur l’objet migré ne change rien. Le fournisseur de préférences est resynchronisé lors d’un changement de compte. Une destination explicite ou une console dans l’URL passe avant la restauration d’une préférence ; la destination en attente sera consommée à l’accueil. Une fiche n’est restaurée que si son jeu existe dans le contexte chargé. Les paramètres historiques `interface=<ancien mode>`, `mode=<ancien mode>`, `section=detail` et `section=archives` restent utilisables. La restauration ne crée ni favoris ni collections.

## Routage et chargement

`appBase.js` détermine `/gameflex`, `index.js` applique le basename du routeur ; la navigation interne utilise les chemins relatifs à ce basename. Les API et médias suivent `serverBase.js`. Aucun proxy public, service worker ou port de production n’est modifié. Le test navigateur utilise un véritable BrowserRouter sous `/gameflex/`, le fournisseur et les composants réels, avec une identité et un transport de fixtures distincts. Il n’accède à aucun compte réel.

Les cinq interfaces de `RetroLibrary` exposent désormais les rubriques communes ; ces rubriques passent par `ExperienceView` sans changer le mode sauvegardé. Les neuf interfaces ouvrent `CatalogGameDialog`, puis `LaunchDialog` pour jouer. La fiche propose aussi les favoris et l’ajout/retrait dans une collection. La restauration du focus utilise l’élément d’origine ou son identifiant stable lorsqu’un rendu l’a remplacé ; la chaîne fiche → lancement → fermeture retrouve la carte initiale.

Les compteurs et messages vides sont masqués tant que la bibliothèque charge. Le singulier est corrigé dans les vues rétro. Les listes Favoris et Récents vides ne présentent plus la bibliothèque comme si elle était sélectionnée ; la recherche filtre aussi les récents.

## Audit des compteurs

Sur le listing local GBA existant, sans rescan ni téléchargement : **430 références, 430 fichiers, 428 cartes regroupées**. `ListCatalog` / le listing publient une référence avec une liste `files`. `LibraryExperience` charge ces références ; `RetroLibrary` les compte directement. `mergeGames` regroupe les titres et conserve les variantes dans `variants`, d’où 428 cartes. L’égalité entre références et fichiers dans cet échantillon n’est pas une règle : un jeu peut avoir plusieurs fichiers.

Le calcul de regroupement reste inchangé. La liste rétro et le Tableau de bord indiquent des « références » ; la console regroupée compte des « jeux ». Archives indique séparément fichiers et références. Les données de préférence utilisent toujours les références, pas les titres regroupés. Les crédits existants restent sur les visuels et dans les paramètres ; aucune nouvelle image externe n’a été ajoutée.

## Vérifications et reproduction

Depuis la racine, avec les dépendances frontend déjà installées :

```sh
node tests/pi-tv/run-interface-tests.cjs
PYTHONPATH=tools/pi-tv python3 -m unittest discover -s tests/pi-tv -p test_interface_migration.py
node tests/pi-tv/build-interface-preview.cjs
python3 tests/pi-tv/serve-interface-preview.py
```

Ouvrir `http://127.0.0.1:18196/gameflex/`. `?saved=favorites` teste une ancienne préférence avec un stockage de prévisualisation neuf. `?interface=archives` teste un ancien lien. `?pack=1` affiche 22 entrées de test explicitement fictives. Le serveur ne sert que les bundles de fixtures : aucun relais vers la production. Pour un navigateur dans un autre conteneur, choisir une adresse de liaison accessible sur le réseau de test.

`tests/pi-tv/interface-cleanup-browser.py` utilise Playwright/Chromium déjà présents dans l’image de test ; `INTERFACE_PREVIEW` définit son adresse. Il vérifie les six migrations et leur relecture, la priorité des liens, recherche, favoris, collections, filtres des archives, fiche, focus, contrat de lancement, mobile et recherche dans 22 entrées. Le transport de lancement y est simulé ; le test complémentaire de cœur lance réellement un programme GBA original CC0 déjà disponible, sans BIOS propriétaire : 90 images, 240 × 160, image non noire. Rapports dans `output/interface-cleanup-*.json`.

La suite API générale a deux échecs reproduits sur le serveur initial : annonce des capacités FreeROMs (`downloads` faux), et anciennes routes de vérification manuelle (410 au lieu de 401). Ils sont indépendants de la migration ; les tests dédiés de validation des préférences passent.

## Préparation du pack de 22

`InterfacePicker` accepte une liste `options`, des identifiants stables comme clés, une recherche et un nombre dynamique de résultats. Les tests couvrent 22 entrées sur mobile et le filtrage clavier. `interfacePackCapacity` documente la cible, sans limiter artificiellement le nombre de cartes. Les 13 entrées supplémentaires du test ne sont jamais enregistrées dans le registre public. Pour intégrer un pack réel, fournir son manifeste approuvé (identifiants, agencements, crédits), l’ajouter au registre et à la validation serveur, puis vérifier chaque interface avant de la rendre sélectionnable. Aucun nom ni visuel de pack n’a été inventé.
