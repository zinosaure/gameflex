# Préparation Raspberry Pi et Android TV

Cette extension prépare dix accueils GameFlex, une API de préférences et d’images,
un client Android TV et un essai NES libre. Le site est maintenant déployé sur le Pi à `http://192.168.1.63:2000`,
avec une bibliothèque de démonstration uniquement. Aucun fork ou push.
Le client TV est désormais installé et testé sur la Philips ; voir le
[bilan Android TV](android-tv.md). Les restrictions d’accès TV ci-dessous
décrivent l’audit initial, avant l’autorisation explicite d’installation.
Aucun paquet hôte, module noyau, pare-feu ou routeur n’a été modifié.
Le dépôt conserve son remote `upstream`.

## Audit et plan

L’audit du dépôt et de TvRework a précédé les modifications. La distribution est
annoncée x86-64 dans AGENTS.md. Le serveur est en Go, le moteur utilise libretro/CGo,
EGL/GLES et ffmpeg ; l’interface React est construite dans l’image edge. Le lancement
existant vérifie les fichiers et les droits de console : les nouveaux accueils ouvrent
ces mêmes fiches, sans ajouter de route de lancement permissive.

La machine visible est un Pi 5 ARM64 (`/proc/cpuinfo`). `/dev/dri` et `/dev/uinput`
ne sont pas visibles dans le bac à sable ; cela ne prouve pas leur absence sur l’hôte.
Les manifestes publiés ont été vérifiés : gateway, remoteplay et retrogaming
sont distribués uniquement en amd64. Kernel et retrogaming ont été reconstruits en
ARM64 ; la passerelle publiée tourne via QEMU explicite, sans enregistrement binfmt. Ne pas lancer
`launch.sh` en supposant une installation Pi officiellement prise en charge.

TvRework a été étudié en lecture seule dans
`/home/config/TvRework/output/LoutreTV-1.4.3-sources.zip` :
`docs/INSTALLATION.md`, `docs/API.md`, `docs/MOONLIGHT-PREPARATION.md` et le manifeste Android.
Son historique identifie une Philips 50PUS7906/12, Android 11/API 30, applications ARM32.
Il décrit ADB sur TCP avec une identité déjà appairée et des contrôles avant installation.
Aucune vérification actuelle, connexion, découverte réseau, lecture de clé ADB ou
commande de télécommande n’a été faite. L’adresse et les clés ne sont pas recopiées.
Cette archive ne prouve pas l’état actuel de la TV.

Ordre d’implémentation :

1. Conserver l’authentification et les chemins de jeu, créer dix compositions.
2. Demander un choix après connexion et permettre de le changer dans les paramètres.
3. Ajouter une API locale authentifiée pour synchroniser les préférences et publier la compatibilité.
4. Préparer l’import et le cache d’images avec les crédits.
5. Générer une cartouche NES originale et tenter un core ARM64 isolé.
6. Préparer le client TV et le déploiement, puis vérifier ce que permet l’environnement.
7. Après une prochaine demande : installer et mesurer sur le matériel, sans présumer les performances.

## Les dix interfaces

Les composants se trouvent dans `src/webapp/src/app/gameflex/experience/`.
Le nouveau choix s’affiche après authentification, sur les routes privées hors streaming.
Une entrée des paramètres le rouvre. La préférence est séparée par compte et origine
dans le navigateur ; l’API complémentaire permet sa synchronisation serveur.
Les modes, favoris et collections se changent sans interrompre une session.

| Mode | Structure et composants |
| --- | --- |
| Minimal | Liste horizontale compacte, vignettes étroites, titres lisibles, fond sobre |
| Jaquettes | Bibliothèque claire, grille de couvertures verticales et navigation bleue |
| Consoles | Grandes cartes de machines pastel ; accès à leur bibliothèque |
| Favoris | Grille sélectionnée, cœurs persistants, barre de jeu en bas |
| Reprise rapide | Grand panneau du dernier jeu ouvert, rail des précédents |
| Collections | Étagères personnelles horizontales ; création et ajout/retrait depuis Fiche de jeu |
| Fiche de jeu | Couverture, informations et illustration en colonnes, bande de sélection |
| Archives | Filtres console/état, tableau de fichiers, panneau d’inspection |
| Salon multijoueur | Bannière de salon, sélection des consoles à plusieurs joueurs et sessions réelles |
| Tableau de bord | Quatre raccourcis, derniers jeux ouverts, favoris et bibliothèque |

Les compositions changent réellement de disposition. Le choix s’applique aussi à
la navigation des bibliothèques de consoles. Les fiches de lancement et les écrans
de streaming et d’administration existants sont conservés pour leurs contrôles métier. Les flèches déplacent le focus
selon la géométrie des éléments ; Entrée active les boutons. La grille s’adapte au
petit écran et la navigation devient une barre inférieure défilante.
Le salon filtre la capacité de la console : il ne prétend pas détecter le nombre de
joueurs de chaque jeu ou les manettes présentes. « Reprise » présente les jeux ouverts ;
la sauvegarde réelle est choisie dans la fiche existante. Aucune progression fictive.

Les cartes des 33 plateformes affichent désormais des photographies du matériel,
stockées dans le cache local ; voir [Photos des consoles](console-photos.md). Les
jaquettes de jeux manquantes restent signalées par un visuel abstrait. L’aperçu utilise des titres fictifs,
clairement signalés. Il est indépendant de la connexion et ne lance aucun jeu :
`output/experience-preview.html`, à construire avec `tools/pi-tv/build-preview.sh`
et un esbuild/React local. Le vrai frontend a été construit avec `npm ci && npm run build` dans
`src/webapp/src`, dans un conteneur Node temporaire. Les dépendances ne sont pas
installées sur le système hôte.

## API complémentaire

Implémentation Python standard, sans dépendance supplémentaire : `tools/pi-tv/server.py`.
Son contrat OpenAPI 3.1 est `deploy/pi-tv/openapi.json`.
Elle écoute **127.0.0.1:10211** par défaut. Le frontal Nginx optionnel sert les routes
sous `/experience-api/` et relaie le reste vers le edge GameFlex reconstruit avec cette interface.
L’API ne contrôle jamais ADB, Docker, les émulateurs ou le système.

| Route interne | Méthode | Contrôle |
| --- | --- | --- |
| `/health` | GET | Disponibilité du processus seulement |
| `/v1/interfaces` | GET | Liste publique des dix identifiants |
| `/v1/compatibility` | GET | Matrice publique des 33 systèmes |
| `/v1/preferences` | GET / PUT | Bearer vérifié auprès du vrai `/api/v1/auth/me` à chaque appel |
| `/v1/media` | GET | Métadonnées publiques des images et crédits |
| `/v1/media/<sha256>` | GET | Images approuvables du cache, adresse à empreinte vérifiée |

Format de préférences :

```json
{
  "interface": "consoles",
  "favorites": ["nes:reference-locale"],
  "history": ["nes:reference-locale"],
  "collections": [{"id": "homebrew", "name": "Homebrew", "games": ["nes:reference-locale"]}]
}
```

Les identifiants sont ceux du registre et des ROMs déjà scannées, pas des chemins à
exécuter. Le serveur ne fait confiance ni au nom envoyé par le client ni à un JWT
simplement décodé. Les fichiers de préférences sont nommés par une empreinte du compte,
mode 600, avec remplacement atomique et limites de taille. Pas de journal des requêtes,
URL, jetons ou contenus. Ne jamais exposer ce service directement à Internet.
En cas de panne de synchronisation, l’interface conserve sa copie locale et le dit.
La dernière écriture serveur gagne ; il n’y a pas de fusion simultanée entre appareils.

Exécution locale ultérieure, depuis le dépôt :

```sh
python3 tools/pi-tv/server.py
```

Pour le conteneur, seul le service credential/edge local déclaré est accepté comme
autorité ; pas d’URL d’authentification arbitraire. Les liens vers les images du cache
sont publics sur le frontal local : ne pas y importer d’image personnelle ou sensible.

## Compatibilité des 33 systèmes

La matrice complète, versionnée et exposée par l’API, est
[`deploy/pi-tv/compatibility.json`](../deploy/pi-tv/compatibility.json).
Chaque ligne précise le moteur, sa licence, la voie Pi, la voie TV ARM32 et les besoins
éventuels en firmware. Ce sont des candidats techniques ; la colonne de vérification
reste « non testé » tant qu’un essai réel n’a pas réussi.

NES/QuickNES, Atari 2600/Stella, GB/Gambatte et GBA/mGBA sont les premiers candidats.
Le registre NES accepte désormais `quicknes`, sans changer le core par défaut.
Pour les autres systèmes légers : Gearsystem, VICE, Fuse, Beetle/Mednafen, ProSystem
et Caprice32 sont étudiés. Une licence libre de moteur ne garantit pas les droits
sur ses fichiers système. Aucun BIOS ou firmware n’est fourni ou recherché.

Pour SNES, le candidat strictement libre est bsnes. Le registre historique propose
Snes9x, dont la licence impose des restrictions non commerciales. Même distinction
pour FB Alpha, Genesis Plus GX et PicoDrive : **ils ne sont pas ajoutés ni téléchargés
par cette préparation**. MAME actuel est une piste libre pour CPS, Neo Geo et Mega Drive,
avec coût CPU et construction ARM à mesurer. Ne pas substituer une ancienne variante
MAME à licence non commerciale. Voir les [licences Libretro](https://docs.libretro.com/development/licenses/).

N64, DS, PSP et Dreamcast demandent un essai par jeu, GPU et résolution. PPSSPP
prévoit [Android avec des performances variables](https://dev.ppsspp.org/docs/getting-started/system-requirements/).
PS1 peut utiliser le HLE de PCSX-ReARMed avec des limites de compatibilité.
Play! est une piste PS2 libre, très expérimentale ; PCSX2 du registre actuel ne devient
pas compatible Pi par le simple changement d’interface.

[Dolphin exige Android 64 bits](https://dolphin-emu.org/docs/faq/) : il ne peut donc
pas être installé nativement sur l’Android ARM32 décrit dans TvRework.
Pour GameCube/Wii, la voie prévue est Dolphin sur le Pi 5 puis streaming WebRTC
GameFlex vers le client TV. Sur Pi, certains titres resteront trop lourds : cette voie
n’est pas une garantie de jouabilité. Même réserve pour Jaguar, Saturn, PC-FX et PS2.
Si le Pi est insuffisant, un serveur plus puissant sera nécessaire ; la TV ne résout
pas cette limite de calcul.

Le client TV livré ici reçoit un flux ; il n’embarque pas 33 émulateurs Android.
Un port natif libre reste une autre voie à construire et mesurer. Pour Doom, le moteur
Chocolate Doom peut utiliser un WAD libre, sans partager les données commerciales.
Moonlight, déjà étudié par TvRework, reste une alternative facultative nécessitant un
hôte de streaming compatible préparé séparément ; rien n’est appairé ou installé ici.

## Essai NES libre

`tools/pi-tv/nes_demo.py` génère **Palette libre**, une cartouche NROM originale de
24 592 octets : code 6502, motifs et palette animée. Le contenu est CC0 ; il n’utilise
aucun jeu existant, BIOS, firmware ou ressource propriétaire. Le générateur est testé.

```sh
# Génération seule, sans réseau ni installation
python3 tools/pi-tv/nes_demo.py
# Essai d’un core déjà présent, sans installation système
python3 tools/pi-tv/nes_demo.py --core /chemin/autorise/quicknes_libretro.so
# Import explicite du seul core libre officiel ARM64 puis essai local
python3 tools/pi-tv/nes_demo.py --fetch-core
```

Le téléchargement cible uniquement le buildbot Libretro, pas des sites de jeux.
Le core reste dans `.local/pi-tv/cores/`, ignoré par Git. Il n’est pas redistribué.
Le wrapper libretro QuickNES est sous [GPL-2.0](https://github.com/libretro/QuickNES_Core/blob/master/LICENSE),
avec des composants QuickNES LGPL : préserver les notices et fournir les sources
correspondantes avant toute redistribution du binaire. Le helper relève une empreinte
locale et l’URL HTTPS, sans prétendre vérifier une signature éditeur.

Le test exécute 300 frames via l’ABI libretro, contrôle qu’au moins deux images diffèrent,
mesure la durée et écrit une image PPM et un rapport. Il vérifie une émulation vidéo
headless, pas les manettes, le son, l’encodage GPU ni la latence TV.

**Résultat vérifié sur le Pi ARM64 : 300 frames exécutées avec QuickNES,
29 images distinctes, en environ 0,08 seconde hors synchronisation temps réel.**
La démo affiche des motifs animés et des lettres originales. La cartouche et le
rapport sont dans `output/nes-demo/`. Le lancement et l’arrêt d’un conteneur de
session GameFlex sont également vérifiés. Le flux vidéo sur téléphone reste à mesurer.
Après essai réussi et choix du dossier de bibliothèque par son propriétaire, la
cartouche pourra aller dans `roms/nes/` et le core dans `cores/nes/`, puis **Rescan all**.
Aucune bibliothèque personnelle n’a été choisie ou modifiée automatiquement.

## Photos de consoles et jaquettes

Recherche des conditions effectuée le 8 novembre 2026. Le service distant ne reçoit
que le nom explicitement demandé à l’import ; aucun scan de ROM ni chemin personnel.

| Source | Gratuité et accès | Limites et droits | Attribution et cache |
| --- | --- | --- | --- |
| Wikimedia Commons, Action API | Gratuit, pas de clé pour les lectures publiques | Limites de plateforme, User-Agent identifiable, `maxlag=5`, respecter 429/Retry-After ; licence à lire pour chaque fichier | Titre, auteur, source et licence conservés ; BY/BY-SA/CC0/domaine public seulement dans l’importeur ; photos de machines, pas une promesse de couvertures commerciales |
| IGDB | API gratuite pour usages commercial et non commercial ; compte Twitch, 2FA, application confidentielle et OAuth serveur | 4 requêtes/s, jusqu’à 8 requêtes ouvertes ; conditions Twitch ; les couvertures gardent leurs droits d’auteur | Crédit IGDB visible ; cache encouragé par la documentation ; aucune clé dans le frontend ou les arguments de commande |
| ScreenScraper | API communautaire avec comptes/identifiants développeur | Threads, accès et quotas quotidiens variables selon compte/contribution ; lire les compteurs du compte au lieu d’inventer un quota universel | Conditions des médias et d’intégration à clarifier ; alternative documentée, non intégrée automatiquement |

Sources primaires : [Wikimedia API usage](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_API_Usage_Guidelines),
[étiquette MediaWiki](https://www.mediawiki.org/wiki/API:Etiquette),
[Imageinfo et métadonnées](https://www.mediawiki.org/wiki/API:Imageinfo),
[IGDB, accès, limites et cache](https://api-docs.igdb.com/),
[conditions Twitch](https://www.twitch.tv/p/en/legal/developer-agreement/),
[ScreenScraper API et compteurs](https://www.screenscraper.fr/webapi2.php).
L’accès à une API d’illustrations n’accorde aucun droit de télécharger les jeux.

`tools/pi-tv/artwork.py` implémente Commons pour les photos et IGDB pour les jaquettes.
L’import IGDB demande un drapeau explicite après lecture des conditions ; pour un
service commercial, obtenir un accord adapté avant activation. Les identifiants
`IGDB_CLIENT_ID` et `IGDB_CLIENT_SECRET` restent dans l’environnement du processus,
jamais dans Git, une URL ou un journal.

```sh
# Recherche d’images de matériel seulement, pas de ROM ou firmware
python3 tools/pi-tv/artwork.py commons 'Nintendo Entertainment System console'
# Après contrôle des candidats, approuver l’image choisie pour le registre NES
python3 tools/pi-tv/artwork.py commons 'Nintendo Entertainment System console' --target nes --approve-index 0
# Avec les identifiants fournis localement par le propriétaire du compte
python3 tools/pi-tv/artwork.py igdb 'Nom du jeu' --accept-igdb-terms
```

Les images sont enregistrées par SHA-256 avec un manifeste JSON : fournisseur, titre,
auteur, source, licence, attribution, date et échéance de révision à 30 jours.
Seule une image approuvée et associée à une cible apparaît dans l’interface.
Une cible de jeu s’écrit `consoleId:reference`, pas uniquement son nom ambigu.
Les crédits sont visibles dans les paramètres et sur l’image ; l’interface ne fait
aucune requête à un fournisseur quand elle affiche une carte.

Le cache est local (`.local/pi-tv/media`), plafonné à 256 Mio et 5 Mio par image.
Aucun SVG distant, URL arbitraire ou redirection authentifiée n’est accepté.
Formats JPEG/PNG/WebP vérifiés ; réponse API bornée. L’importeur sérialise les accès
à environ une requête par seconde et s’arrête en cas d’erreur/quota, sans boucle de
réessai. La date à 30 jours signale une révision manuelle, pas une suppression ni un
nouveau téléchargement automatique. L’affichage reste possible hors ligne.
Pour retirer une image, supprimer son `.img` et son `.json` du cache du projet ;
les crédits et l’image disparaissent au prochain chargement. 33 photos de matériel sont importées ; aucune jaquette de jeu n’est encore
importée dans la bibliothèque actuelle de démonstration. La photo CPS-2, trouvée via
Openverse et vérifiée sur Flickr, est sous CC BY-NC-SA 2.0 : usage non commercial
uniquement. Les 32 autres sont sous domaine public ou CC BY-SA. Voir les crédits
individuels dans [Photos des consoles](console-photos.md).

## Préparation du déploiement

La pile principale ne doit être lancée qu’après vérification des images ARM64 et des
dépendances EGL/GLES/ffmpeg sur Pi. Le frontend modifié exige une reconstruction du
edge depuis ces sources ; les images publiées actuelles ne contiennent pas ces changements.
Le fichier `docker-compose.build.yml` fournit déjà les recettes.
L’overlay expérimental `deploy/pi-tv/arm64.build.yml` prépare les builds ARM64 de
edge, kernel, credential et retrogaming avec des tags locaux. Il exige des références
ARM64 vérifiées pour gateway et remoteplay : leurs sources sont absentes du dépôt
public, qui ne contient que leurs Dockerfiles. Ces binaires sont soumis à
`LICENSE-BINARIES.md` et ne peuvent pas être reconstruits ou modifiés librement ici.
Il faut confirmer la disponibilité de leurs images ARM64 auprès de la distribution,
ou concevoir séparément une alternative libre de streaming. La présence de recettes
Docker ne suffit donc pas à garantir une pile complète sur le Pi.
L’overlay reste un fichier de préparation, pas un déploiement déjà fonctionnel. Les générations
récentes restent hors du périmètre ARM de cette préparation.

Le Compose **complémentaire** est `deploy/pi-tv/compose.yml` : API Python et Nginx,
aucun socket Docker, conteneur privilégié ou accès TV. Il rejoint le réseau d’une
pile GameFlex déjà validée. Son entrée est liée à **127.0.0.1:2000** par défaut (paramètres `GAMEFLEX_WEB_BIND` et `GAMEFLEX_WEB_PORT`).
Elle n’est donc pas encore accessible depuis la TV. Le futur passage à une adresse
LAN précise et le démarrage Docker doivent être approuvés comme changements de la
machine. Aucun port 10201 public, aucune ouverture de routeur ou de pare-feu.

Après autorisation et validation de la pile ARM, préparer `.local/pi-tv` avec un
propriétaire correspondant à l’UID 1000, préciser `GAMEFLEX_NETWORK` si nécessaire,
puis construire/démarrer uniquement cette extension. Le fichier ne démarre pas
les services GameFlex manquants. Arrêt possible avec son propre projet Compose,
sans supprimer le volume de préférences.

Le client TV dédié est maintenant compilé et signé en version 0.2.0. Voir
[GameFlex TV](android-tv.md) pour son architecture, la connexion par code,
la construction, les vérifications et le retour arrière. Les observations de
l’audit initial ci-dessus restent historiques ; le bilan actuel distingue
explicitement les tests locaux des essais sur la Philips.

Avant une vraie installation, le propriétaire devra choisir son dossier de jeux,
les consoles à activer et le mode de création du mot de passe administrateur.
Aucun secret ne doit être fourni à une commande visible ou apparaître dans un rapport.

## Déploiement actif sur le Pi

Adresse LAN vérifiée depuis le réseau de l’hôte : **http://192.168.1.63:2000**.
L’entrée est publiée uniquement sur cette adresse privée. Le téléphone doit être
sur le même réseau, sans isolation Wi-Fi entre clients. Aucun routeur ou pare-feu
n’est modifié. Le service de comptes existant et son volume `gameflex-accounts`
sont réutilisés.

Le compte `unknow` existait déjà avec le rôle principal 0. Son authentification a
été vérifiée ; son rôle et son mot de passe n’ont pas été réinitialisés. Aucun secret
n’est reproduit ici ni inscrit dans les scripts. Le script de création d’un nouveau
compte standard reste disponible, mais il n’est pas utilisé pour écraser ce compte.

Le dossier personnel de jeux n’ayant pas été fourni, le serveur pointe uniquement
sur `.local/demo-library`, généré dans ce projet avec la démo CC0 et le core libre.
Ce dossier n’est pas présenté comme la bibliothèque personnelle du propriétaire.
Le bandeau « Démo » a été retiré à la demande de la personne ; cela ne remplit pas sa bibliothèque personnelle. La préférence d’interface est laissée vierge
jusqu’au premier choix de la personne.

Conteneurs du déploiement :

- `gameflex-web` : frontend React construit, Nginx, port LAN 2000.
- `gameflex-kernel` : noyau natif ARM64 ; port 10201 lié à 127.0.0.1 seulement.
- `gameflex-experience` : préférences et cache, sans port hôte publié.
- `gameflex-gateway` : adaptateur WebRTC natif ARM64 local (`tools/pi-tv/gateway`), limité au runtime retrogaming libre, HTTP sur 127.0.0.1:10202 et ICE UDP 9201–9300 sur eth0.
- `gameflex-host-service` et `gameflex-host-relay` : liaison de contrôle privée.
- `gameflex-credential` : service préexistant conservé avec ses comptes et son volume.

La passerelle et les sessions utilisent le réseau hôte comme le prévoit GameFlex,
mais ce Pi bloque les connexions du bridge Docker vers ces ports. Le relais ne
change aucune règle réseau : son côté hôte écoute un socket Unix dans
`.local/host-relay`, et son côté bridge transporte uniquement TCP 10202 et
9001–9010 vers 127.0.0.1. Il ne publie aucun port hôte et ne journalise aucun octet,
URL, jeton ou mot de passe. Les autres destinations sont refusées.

`GAMEFLEX_INTERNAL_HOST=gameflex-host-relay` sépare ces adresses de contrôle de
`GAMEFLEX_PUBLIC_HOST`, qui reste l’adresse LAN réelle. Sans cette option, le noyau
conserve exactement son routage précédent. Le frontal utilise aussi ce relais pour
la signalisation de la passerelle. La vidéo WebRTC n’est pas transportée dans le
relais Unix : son chemin UDP doit encore être mesuré sur le téléphone.

Les essais QEMU 7.2 et 10.0 sont conservés dans `.local/`, avec leurs licences, sans installation sur l’hôte. Ils ne servent plus au déploiement : le Go x86 émulé reçoit des paquets netlink de 16 Ko dans un tampon de 4 Ko et échoue à énumérer les interfaces du Pi. Le relais natif utilise Pion et le protocole documenté de `shared/gatewayclient`. Il ne remplace pas les fonctions fermées de Remote Play, les flux scellés ou les intros de la passerelle officielle. Remote Play et les consoles récentes ne sont pas déployés. Aucune action
n’est envoyée à une TV, une console physique ou un autre service de la machine.

Les nouveaux services ont une politique de redémarrage `unless-stopped` et des
limites de CPU/mémoire. Les conteneurs de relais, API et frontend ont leur système
de fichiers en lecture seule, sans capacités supplémentaires. Les données sont
persistées dans le projet ; les comptes restent dans le volume préexistant.
Aucun autre conteneur de la machine n’a été arrêté ou remplacé.

Docker Compose est absent du client de cette session. Le déploiement utilise donc
le CLI Docker existant, sans installation supplémentaire :

```sh
# Après les builds locaux, dont NativeGateway.Dockerfile
python3 tools/pi-tv/start-native-gateway.py
python3 tools/pi-tv/deploy-local.py --bind 192.168.1.63 --port 2000 --demo
python3 tools/pi-tv/verify-deployment.py
# Test temporaire de lancement et arrêt de la seule démo générée
python3 tools/pi-tv/verify-deployment.py --launch
```

Les scripts réutilisent seulement leurs conteneurs portant le label du projet et
refusent d’écraser un nom appartenant à un autre déploiement. Les secrets du service
de comptes sont lus en mémoire et transmis comme environnement ou stdin privés,
sans valeurs dans les arguments des commandes. Ne pas afficher `docker inspect`
complet, les environnements ou des corps de réponses de connexion.

## Vérifications réalisées

- Build Webpack complet réussi ; trois avertissements de performance, aucune erreur.
- Kernel et moteur retrogaming construits pour ARM64.
- Dix tests Python réussis, dont refus de port et transfert réel via le relais Unix.
- Tests Go du paquet `internal/config` réussis, avec conservation de l’adresse publique.
- Dix compositions React et dix états vides rendus lors de la préparation.
- HTTP 200 sur la page LAN ; authentification `unknow`, catalogue et préférences vérifiés.
- Démo NES présente ; lancement et arrêt de session via WebSocket façade : HTTP 200.
- Démo QuickNES headless : 300 frames, 29 images différentes, sans contenu propriétaire.
- Capture réelle de la page de connexion obtenue avec `codexpi-browser`.
- Le choix d’interface n’a pas été effectué automatiquement au nom de la personne.
- Lors de l’audit initial, APK Android TV, décodage et commandes n’étaient pas testés.
  Le [bilan Android TV](android-tv.md) décrit les essais réels réalisés depuis.
- Latence et lecture WebRTC sur téléphone non confirmées depuis l’appareil réel.

Rapports : `output/phone-deployment-status.json`, `output/validation.json` et
`output/nes-demo/result.json`. Captures optimisées : `output/gameflex-pi-login.webp`
et `output/nes-demo/nes-demo.webp` (cette dernière montre la sortie du core).
Les anciens blocages du bac à sable ne s’appliquent plus à cette reprise.

## Navigation et catalogue externe

Les entrées de navigation (Accueil, Mes jeux, Consoles, Favoris, Récents, Collections, Archives, Multijoueur) modifient la section, jamais l’interface sauvegardée. L’apparence se choisit seulement au premier accès ou dans Paramètres. Une console montre deux rayons cartouches : jeux locaux et métadonnées Planetemu par lettre. Voir [le contrat et les limites](planetemu-catalogue.md). Le choix d’un jeu local ouvre un dialogue dans la même interface ; le bouton Jouer sélectionne un core réellement installé et affiche un échec explicite si aucun core n’est disponible. Une barrière d’erreur garde un retour vers l’accueil en cas d’échec du rendu.

### Validation du relais natif

Le 8 novembre 2026 : cinq tests Go du relais réussis (origines, secret interne, source absente, enregistrement, secret dérivé par session). L’enregistrement d’un runtime exige le HMAC de **sa** session, selon `shared/auth.SessionSecret` ; la clé d’installation et la clé d’une autre session sont refusées. La bibliothèque v4 garde l’autorisation des manettes ; le relais n’accorde aucun pad sur la simple présence d’un nom.

Sonde Pion native dans le réseau hôte : négociation ICE connectée, 60 unités vidéo H264 réellement reçues depuis QuickNES ; 49 images 256 × 240 décodées par FFmpeg après acquisition du premier IDR. Le transport conserve bien les SPS/PPS de durée nulle envoyés par le runtime. Le navigateur intégré, dans un bridge Docker, ne peut pas joindre les ports UDP hôte : un écho UDP 9300 passe depuis l’hôte et expire depuis ce bridge. Cela limite la validation du navigateur intégré et ne permet pas de conclure sur le trajet Wi-Fi de l’iPhone. Aucune règle de pare-feu n’a été modifiée ; le test iPhone reste à confirmer.

Le client affiche maintenant un échec explicite après une minute sans première image pour les sessions retrogaming, au lieu d’attendre indéfiniment. Les démarrages déclarés lents conservent leur attente. L’administration classique reste accessible par « Compte et bibliothèque » dans Paramètres.

Le relais se limite au LAN IPv4, H264/Opus, huit spectateurs par session, quatre runtimes et huit offres simultanées. Il utilise Pion WebRTC v4.2.26 et Gorilla WebSocket v1.5.3, verrouillés par `go.sum` ; les licences de dépendances sont conservées dans l’image sous `/licenses`. Pas de TURN, pas de Remote Play, pas de flux scellé ni d’intro propriétaire. Son HTTP écoute seulement 127.0.0.1:10202, derrière le relais Unix et Nginx existants.

### Jaquettes et filtre alphabétique

Le fournisseur Libretro Thumbnails est maintenant intégré pour la consultation personnelle : téléchargement à la demande des seuls PNG Named_Boxarts, correspondance par système et titre, cache local et attribution. Les variantes peuvent partager une jaquette publiée du même jeu ; les prototypes sans boîte ne reçoivent pas de fausse image. Les détails, quotas et droits restent documentés dans `planetemu-catalogue.md`. Les anciens caches non filtrés sont remplacés par la version v2, et le client ignore les réponses annulées lorsque la lettre change.
