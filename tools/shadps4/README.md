# shadPS4 construit par GameFlex

Le shadPS4 de l'éditeur, plus nos patchs, appliqués dans l'ordre de leur numéro (les deux
premiers ci-dessous ; les suivants dans le tableau, et en tête de `build.sh`).

**La source est un commit de `main`**, écrit dans `COMMIT`, depuis le 2026-10-27 (le porteur : le
rendu de la 0.20 arrêtait *FIFA 22*, et l'amont en a corrigé une partie depuis). Sans `COMMIT`,
`build.sh` revient à l'étiquette du serveur (`SHADPS4_VERSION`). Le binaire garde le nom de
cette version (`shadps4-gameflex-0.20.0`) : il remplace celui de son AppImage. 0008, repris de
l'amont pour la 0.20, est dans `main` et n'existe plus ici.

Les deux premiers :

1. **Un réglage** : `user_vaddr_max_in_gbytes` dans son `config.json` (section `General`) — la plus
   haute adresse du jeu, en Gio. `0` (le défaut) laisse la mise en page de l'éditeur ; le
   téléphone la ramène à `320` Gio, pour qu'elle tienne dans les 39 bits de son noyau (chantier 17,
   `#367`), sous Box64 chargé à 340 Gio. C'était `256` jusqu'au 2026-10-24 : *Kingdom Hearts III*
   place 128 Mio à `0x4000000000`, juste au-dessus, et s'arrêtait (« outside the memory map »).
   Le serveur le laisse à `0` : rien ne change pour lui.
2. **Un défaut de l'amont** (`#424`, encore là dans la 0.20.0 et sur `main` le 2026-10-22). Souris
   en mode pavé tactile — streamex l'y met au lancement (la touche Suppr, `adapter.json`), pour le
   pavé tactile de la manette à l'écran —, `ApplyMouseInputBlockers` bloque toute entrée dont le
   numéro vaut celui d'un clic gauche ou droit (1 ou 3), sans regarder son type. Il avalait donc
   l'axe 1 (stick gauche vertical), l'axe 3 (stick droit vertical), le bouton 1 (rond) et le bouton
   3 (triangle). Le patch ne bloque plus que les boutons de la souris.

| Fichier | Rôle |
|---|---|
| `0001-user-vaddr-max-setting.patch` | le réglage (25 lignes sur `src/core/emulator_settings.h` et `src/core/address_space.cpp`) |
| `0002-mouse-blockers-only-mouse-buttons.patch` | le mode pavé tactile ne bloque que les boutons de la souris (`src/input/input_mouse.cpp`) |
| `0003-static-patching-only-when-a-cpu-patch-applies.patch` | la passe statique des patchs CPU n'a lieu que si l'un d'eux peut s'appliquer (`#503`) |
| `0004-guest-signals-only-at-safe-points.patch` | un signal du jeu n'est délivré qu'à un point sûr — les jeux Unity ne se figent plus (`#510`) |
| `0005-avplayer-no-lock-across-game-events.patch` | le lecteur vidéo relâche son verrou avant de prévenir le jeu (`#511`) |
| `0006-phone-pads-from-dsu.patch` | `GAMEFLEX_DSU_SERVER=<ip>:<port>` : les quatre manettes d'un serveur DSU (cemuhook) en manettes virtuelles de SDL, joueurs 1 à 4 — celles du téléphone (`#509`) ; sans la variable, rien ne change |
| `0007-kernel-service-requests-not-lost.patch` | le fil de service du noyau ne perd plus une minuterie programmée au moment où `io_context.run()` finissait — FIFA 22 et FC 27 attendaient sans fin au démarrage (`#512`) ; défaut de l'amont |
| `0009-ajm-job-on-destroyed-instance.patch` | une tâche audio pour une instance détruite échoue au lieu d'arrêter l'émulateur (`ajm_context.cpp`, FIFA 22, `#513`) ; défaut de l'amont |
| `COMMIT` | le commit de l'amont construit (`main`) ; absent, l'étiquette du serveur |
| `build.sh` | construit shadPS4 depuis `COMMIT` (ou la version du serveur), avec les patchs, dans un conteneur trixie gardé |
| `bench.sh`, `bench/` | l'essai sur x86_64 dans l'image PS4 du serveur : plage du téléphone (320 Gio) contre témoin, ou les plages de `RUNS` ; le jeu déballé, ou son `.pkg` |

## Mettre à jour quand l'éditeur publie une version

shadPS4 change souvent ; la version se change à **un seul endroit**, et le reste suit.

1. **La version** : le commit dans `COMMIT` (ou, sans lui, `ARG SHADPS4_VERSION=…` dans
   `src/microservices/runtimes/streamex/consoles/ps4/Dockerfile`, que le script lit là).
2. **Construire** : `tools/shadps4/build.sh <sortie>`. Si le patch ne s'applique plus tel quel, le
   script tente une fusion à trois voies et le dit ; s'il s'arrête, rebaser le patch fautif. Pour
   0001 : appliquer ses deux idées à la main sur la nouvelle source (le `Setting<int>` à côté de
   `extra_fmem_in_mbytes`, et la borne de `user_size` dans le constructeur POSIX de
   `AddressSpace::Impl`), puis `git diff > 0001-user-vaddr-max-setting.patch`.
3. **Essayer** : `tools/shadps4/bench.sh <sortie>/shadps4 "<bibliothèque>/roms/ps4/Sonic Mania Plus"`
   — même écran et même journal à 256 qu'au témoin.
4. **Livrer au serveur** : copier `<sortie>/shadps4` dans `<bibliothèque>/cores/ps4/` sous le nom
   `shadps4-gameflex-<version>`, puis relancer une partie : l'image le pose au lancement par-dessus
   celui de l'éditeur s'il est là (`consoles/unpack-emulator.sh`), et le journal de la partie le
   dit. Aucune reconstruction (chantier 29).
5. **Sa source** : une ligne dans `tools/gpl-sources/manifest.tsv` (le commit amont, celui de ce
   dépôt qui l'a construit), puis `tools/gpl-sources/assemble.py <bibliothèque>` une fois livré dans
   la release, que le miroir sert (chantier 34). Sans sa ligne, le script refuse.

**Le passage à la 0.20.0** (2026-10-26, `#487`) : 0001 est rebasé — `shad_net_enabled` a quitté
`GeneralSettings` pour une section réseau, ce qui déplaçait trois contextes de `emulator_settings.h` ;
le réglage et sa borne dans `address_space.cpp` sont inchangés. 0002 s'applique tel quel. Sur la
dev, *Kingdom Hearts III* jusqu'à son premier écran ; au banc, à 320 Gio (la plage du téléphone),
le même écran.

Le conteneur de build (`gameflex-shadps4-build`) garde dépendances, sources et objets : une
nouvelle version se reconstruit en quelques minutes. `docker rm -f gameflex-shadps4-build` pour
repartir de zéro.
