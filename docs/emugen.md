> Historique : EmuGen est désormais retiré du parcours actif. Voir [Imports et sources](imports-et-sources.md).

# EmuGen sur le Raspberry Pi

EmuGen est le fournisseur prioritaire des métadonnées. Le déploiement local
existant repose sur `tools/pi-tv/deploy-local.py`, avec le service Python
`gameflex-experience` et le frontend `gameflex-web` sur le port 2000. La distribution
Compose générale reste séparée ; son complément `deploy/pi-tv/compose.yml` utilise
aussi la nouvelle image du service. Aucun accès à la TV n’est nécessaire.

## Parcours et priorité

La recherche d’une console demande d’abord EmuGen, puis conserve l’ordre précédent
Planetemu, FreeROMs, ROMsPure. La recherche publique WordPress d’EmuGen fournit les
résultats, la taxonomie de console et les miniatures sans Chromium. La fiche HTML
complète fournit les champs supplémentaires. Une console différente est refusée.
Un titre non exact, plusieurs correspondances, ou une région/version non confirmée
nécessitent un choix explicite. Les recherches générales restent des listes de
candidats : elles ne constituent pas une identification automatique.

Chaque champ suit l’ordre : correction manuelle, valeur EmuGen valide, valeur déjà
connue, fournisseur secondaire compatible. Les champs couverts sont titre,
description, console, jaquettes avant/arrière, autres images, captures,
bandes-annonces, date de sortie, genres, langues, version et région. Le champ de
date français vient de `jeux_sortie_france` ; cela ne détermine pas la région d’une
ROM. Les visuels restent soumis aux droits de leurs ayants droit ; le cache ne leur
attribue pas de licence libre. Les crédits restent affichés et accessibles dans
les paramètres.

La même règle s’applique aux vignettes, à la fiche et aux jeux en bibliothèque.
Les corrections `manualMetadata`, `overrides`, `manualFields`, les champs marqués
`fieldSources: manual` et les images explicitement associées dans le registre local
sont prioritaires. Les valeurs vides d’une correction manuelle sont intentionnelles.
Les corrections enregistrées pendant une recherche réseau sont relues avant
l’écriture et conservées.

Le repli réutilise les champs des variantes déjà disponibles. Les adaptateurs
secondaires ne sont interrogés que pour les champs manquants qu’ils savent fournir.
FreeROMs et ROMsPure exposent actuellement surtout les jaquettes ; ils ne sont pas
considérés comme des sources de captures de gameplay lorsqu’ils ne les distinguent
pas. Une capture manquante reste absente si aucun fournisseur ne la fournit. Une
jaquette n’est jamais inventée comme capture pour remplir ce champ.

Dans **Mes jeux** ou une console, **Métadonnées** actualise les jeux affichés en
arrière-plan. **Recharger** reste la recherche de fichiers. Une fiche ambiguë est
signalée pour sélection dans la fiche du jeu. Au chargement normal de la bibliothèque,
seules les associations et métadonnées locales sont lues : aucune recherche réseau
ni réindexation complète n’est lancée au démarrage.

## Sources et Magic Cookie

Le bouton **Sources de téléchargement** déclenche explicitement Playwright.
Le navigateur lit la fiche, utilise le clic public sur `#emugen-cookie-btn`, puis
attend dans `#emugen-download-section` un résultat final : liens visibles ou message
explicite d’absence de téléchargement. Aucun cookie, jeton Turnstile ou CAPTCHA
n’est fabriqué. Les téléchargements sont désactivés dans Chromium ; les liens ne
sont pas suivis. L’URL fournie par EmuGen est affichée telle quelle, même pour un
hébergeur ou une page intermédiaire, avec sa provenance. **Ouvrir la fiche EmuGen**
reste disponible. Les autres fournisseurs viennent ensuite dans leur ordre habituel.

Les états distingués sont `links`, `empty`, `not_found`, `timeout`, `error`,
`challenge_required`, `blocked_navigation`, `busy`. Une vérification Cloudflare
nécessitant une personne renvoie `challenge_required`, avec repli vers les autres
sources. Ouvrir la fiche sur un autre appareil ne transfère pas son cookie au
service : les sessions sont distinctes.

Les destinations de navigation et redirections du navigateur sont limitées à
`www.emugen.net` et `emugen.net`, en HTTPS, sans identifiants ni port non standard.
Les sous-ressources sont limitées à ces domaines et au service public Turnstile.
Les adresses DNS validées sont fixées au lancement de Chromium ; les images,
médias, publicités, trackers, téléchargements et autres domaines sont bloqués.
Les requêtes HTML/images utilisent aussi HTTPS avec validation des certificats,
validation des destinations et connexion TCP sur l’adresse publique DNS validée.

## ARM64 et ressources

`deploy/pi-tv/Experience.Dockerfile` installe **Chromium natif Debian ARM64** et ses
bibliothèques dans l’image, avec Playwright Python 1.64.1. Aucun paquet n’est installé
sur DietPi. Le navigateur est lancé à la première ouverture des sources, réutilisé
entre les requêtes et limité à un seul worker ; chaque page est fermée dans un
`finally`. Le contexte anonyme conserve uniquement la session publique du site en
mémoire. Il ne reçoit aucun identifiant de l’utilisateur GameFlex.

Le conteneur reste non privilégié, utilisateur 1000, système de fichiers en lecture
seule, capacités supprimées, `no-new-privileges`, init pour récolter les processus,
1 Gio de RAM maximum, 1,5 CPU, 256 processus, 128 Mio de mémoire partagée et 192 Mio
pour `/tmp`. Chromium utilise le mode sans sandbox de Playwright dans ce conteneur
isolé ; aucun assouplissement du noyau DietPi n’est appliqué.

Le cache réutilise les fonctions et le répertoire de données existants : réponses
publiques 24 h, fiches/images 7 jours, résultats finaux de sources 5 min. Les erreurs
réseau, délais et vérifications ne sont jamais mémorisés comme absence de résultat.
Les associations/corrections sont persistées séparément dans `game-metadata/`.

## API

Toutes les routes de métadonnées et sources exigent une identité GameFlex vérifiée.
Les images publiques optimisées sont servies comme les autres médias du projet.

| Route | Fonction |
|---|---|
| `GET /experience-api/v1/emugen/<console>?q=<titre>&letter=A` | Recherche publique prioritaire, console vérifiée |
| `GET /experience-api/v1/emugen/games/<slug>?console=<id>` | Métadonnées publiques, sans navigateur |
| `POST /experience-api/v1/metadata/enrich` | Fiche et repli par champ, sans transfert de ROM |
| `POST /experience-api/v1/emugen/sources` avec `{slug}` | Résolution Playwright à la demande |
| `GET /experience-api/v1/metadata?console=<id>` | Associations locales, sans recherche réseau |
| `POST /experience-api/v1/metadata/bind` | Choix explicite d’une fiche ; administrateur |
| `PUT /experience-api/v1/metadata/corrections` | Corrections manuelles ; administrateur |
| `POST /experience-api/v1/metadata/refresh` | Actualisation explicite en arrière-plan ; administrateur |
| `GET /experience-api/v1/metadata/jobs/<id>` | Progression de l’actualisation de l’utilisateur |

## Déployer sur ce Pi

Depuis `/home/config/GameFlex`, Docker étant déjà installé :

```bash
DOCKER_CONFIG="$PWD/.local/docker" docker build \
  -f deploy/pi-tv/Experience.Dockerfile -t gameflex-experience:pi-local .
docker run --rm --cpus 2 --memory 1536m --user 1000:1000 \
  --mount type=bind,src="$PWD/src/webapp/src",dst=/app -w /app \
  node:20-bookworm-slim npm run build
cp src/webapp/src/public/service-worker.js src/webapp/src/dist/service-worker.js
DOCKER_CONFIG="$PWD/.local/docker" docker build \
  -f deploy/pi-tv/LiveFrontend.Dockerfile -t gameflex-web:pi-local .
python3 tools/pi-tv/deploy-local.py --bind 192.168.1.63 --port 2000 \
  --demo --interface-only --recreate-interface
```

La dernière commande refuse de remplacer un conteneur non géré ou d’interrompre un
transfert/rafraîchissement actif. Elle ne touche ni aux données, ni au moteur,
ni à la TV. Le mot `--demo` est le nom historique du profil local, pas un mode de
navigation ou une autorisation de télécharger des jeux commerciaux.

## Vérification

Les fixtures locales couvrent la priorité, le repli d’un champ, les corrections,
les ambiguïtés, la région/version, les liens, l’absence de liens, le délai, la fiche
introuvable, le cache d’erreurs, l’authentification et la réutilisation d’un seul
navigateur avec fermeture des pages. Elles s’exécutent en ARM64 dans la même image,
sans réseau et sans ROM commerciale :

```bash
docker run --rm --init --user 1000:1000 --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --memory 1024m \
  --pids-limit 256 --shm-size 128m --tmpfs /tmp:rw,size=192m,mode=1777 \
  --mount type=bind,src="$PWD",dst=/app -w /app \
  gameflex-experience:pi-local \
  python3 -m unittest discover -s tests/pi-tv -p 'test_*.py'
```

Essai réel sur le Raspberry Pi : Chromium 156 natif `aarch64`, jaquette publique et
description de la [fiche Wii fournie](https://www.emugen.net/jeux/prince-of-persia-les-sables-oublies-wii/)
récupérées sans navigateur. Le navigateur headless est arrêté par la vérification
Cloudflare et renvoie `challenge_required` : **le message d’absence de téléchargement
observé dans une autre session n’a pas pu être confirmé dans cette session**.
Il n’a pas été enregistré comme résultat vide. Aucun binaire de jeu n’est téléchargé
pendant cet essai. Les résultats détaillés restent dans `.local/deployment/`.

Bilan de livraison : 92 tests Python passent sur le Pi ARM64, dont les fixtures
Chromium, ainsi que les vérifications React de priorité, fusion, recherche,
interfaces et suivi des transferts. La compilation web réussit. Le lint ciblé ne
contient aucune erreur (quatre avertissements existants dans `ExperienceView.jsx`).
Le lint global reste en échec sur 47 erreurs hors des fichiers modifiés pour cette
intégration, principalement des règles de hooks dans le streaming et les paramètres.


Le parcours interactif et son isolation sont documentés dans [Vérification manuelle EmuGen](emugen-manual.md).
