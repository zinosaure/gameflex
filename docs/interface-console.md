# Interface unique GameFlex

La seule interface active est `ps-home`. Les 22 compositions précédentes sont archivées, avec leurs identifiants historiques, leur code, leurs styles et leurs références de crédits dans [archives/interfaces-2026-11-10](../archives/interfaces-2026-11-10/README.md). Le manifeste couvre les 68 fichiers de la copie originale. Le code historique n’est plus sélectionnable dans le produit.

## Direction visuelle

Références : [accueil et hubs de jeux PS5](https://www.playstation.com/en-us/ps5/ps5-features/) et [Sony Design, interface PS5](https://www.sony.com/en/SonyInfo/design/stories/PS5/).

Rail de petites tuiles carrées avec agrandissement de la sélection ; titre court et action principale à gauche ; ambiance issue de la jaquette sélectionnée ; commandes système en haut à droite ; centre de contrôle superposé ; bibliothèque en grille. Fond graphite, textes blancs, accent bleu discret. Les photos et jaquettes restent intactes : aucun détourage des blancs ou recoloration du premier plan. L’interface garde la marque GameFlex et n’utilise pas de logo PlayStation.

La composition s’adapte à 390 × 844, 1440 × 1000 et 1920 × 1080. Les flèches naviguent entre les tuiles ; Entrée ouvre la fiche ; Échap ferme le centre de contrôle et rend le focus. Les animations respectent `prefers-reduced-motion`. Recherche, filtres, collections, favoris, archives et paramètres restent des destinations distinctes de la présentation.

## Préférences et données

`migratePreference` remplace uniquement l’interface active par `ps-home` et conserve l’identifiant précédent dans `archivedInterface`. Favoris, historique et collections sont préservés. La migration est idempotente ; les six anciens modes déplacés conservent leur destination. Les anciens liens fonctionnent. Aucun fichier de jeu ou sauvegarde n’est déplacé.

Le serveur accepte `ps-home` et les anciens identifiants. Le correctif de jaquettes rétablit `covers.search`, appelé par `POST /v1/covers/search`, en conservant l’attribution. Il produit des URLs de jaquettes uniquement et ne télécharge aucun jeu, BIOS ou firmware.

## Vérifications reproductibles

Avec les dépendances npm installées dans `src/webapp/src` :

```sh
node tests/pi-tv/run-playstation-tests.cjs
PYTHONPATH=tools/pi-tv python3 -m unittest discover -s tests/pi-tv -p 'test_interface*.py'
PYTHONPATH=tools/pi-tv python3 -m unittest discover -s tests/pi-tv -p 'test_cover*.py'
node tests/pi-tv/build-interface-preview.cjs
python3 tests/pi-tv/serve-playstation-preview.py --bind 127.0.0.1 --port 18228
```

Puis, dans un environnement contenant Playwright et Chromium :

```sh
INTERFACE_PREVIEW=http://127.0.0.1:18228/gameflex/ RESULT_DIR=.local/results python3 tests/pi-tv/playstation-browser.py
```

Les parcours utilisent un transport entièrement simulé : aucune écriture de préférence ni lancement en production. Ils couvrent les trois formats, le retour de fiche avec recherche/filtre/focus, les favoris, les rubriques communes, la création de collection, les réglages avancés, le rechargement et le contrat de lancement. Les états chargement, vide et erreur sont testés séparément.

Les captures avec de vraies jaquettes utilisent uniquement une sélection du cache local existant, copiée dans `.local/interface-cleanup/media/`, avec crédits. Ces images protégées ne sont pas incluses dans le dépôt. `PREVIEW_HAS_CACHED_ART=1` active ces vérifications visuelles supplémentaires quand ce cache est présent ; les tests fonctionnels restent exécutables sans lui.

Build de production, sans modifier la machine : `GITHUB_COMMIT_SHA=<sha> node tools/pi-tv/build-console-production.mjs`. Artefacts dans `.local/console-release/web/`. L’image web conserve les anciens chunks hachés pour les onglets déjà ouverts. Les anciens conteneurs web/API peuvent être conservés arrêtés pour le retour arrière ; les volumes de bibliothèque restent en place.
