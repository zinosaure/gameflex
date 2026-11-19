# shadPS4 construit par GameFlex

**Le dépôt officiel plus nos patchs** : [`shadps4-emu/shadPS4`](https://github.com/shadps4-emu/shadPS4) à un
commit de `main` (`BASE` dans `build.sh` : `7c0b846d`, du 2026-11-17), plus les patchs de `patches/`, un par
correctif (`00NN-gameflex-00NN-….patch`). `tools/shadps4/build.sh` clone le dépôt officiel à ce commit et applique
la série. Chaque entrée ci-dessous décrit un patch, par son numéro.

**Plus de fourche depuis le 2026-11-18** (chantier 63 ; le porteur : « on travaille que par des patchs […] Ce que
je ne veux pas c'est d'avoir des repos avec le code source des émulateurs »). La série a été tirée de l'ancienne
fourche `zinosaure/shadPS4`, supprimée de GitHub le 2026-11-18 (branche `gameflex`, `e441ba5d`) : appliquée à `7c0b846d`, elle redonne son arbre, fichier
pour fichier. Le code se change dans un clone de travail local du dépôt officiel (`~/Github/shadPS4`, branche
`gameflex`, jamais poussée), puis la série se réexporte (`tools/emulator-patches.sh export shadps4`). Pas de pull
request vers l'éditeur. Les commits de 2026-11-11 (« plus de fichiers de patch, du vrai changement de code ») restent
le fond : ce sont toujours de vrais changements, seule leur forme de livraison change.

**Une seule branche** : le serveur et le téléphone prennent le même binaire x86_64 (le téléphone sous Box64,
`tools/box64/bundle.sh`). Ce qui ne vaut que pour le téléphone se règle à l'exécution : la plage d'adresses
(0001) et ses manettes (0006).

**La source est un commit de `main`** depuis le 2026-10-27 (le porteur : le rendu de la 0.20 arrêtait
*FIFA 22*, et l'amont en a corrigé une partie depuis). Le binaire garde le nom de la version du serveur
(`SHADPS4_VERSION` dans `consoles/ps4/Dockerfile`, `shadps4-gameflex-0.20.0`) : il remplace celui de son
AppImage. Le journal d'une partie dit la révision construite (`Revision`) : `build.sh` applique la série en
commits d'une identité et à des dates fixes, si bien que la même série donne toujours la même révision. Le titre de
la fenêtre commence par `shadPS4` (streamex la trouve par là).

**Le passage aux commits ne change pas le code** (vérifié le 2026-11-11, à `3caec58`) : l'arbre de la branche
est celui de `ce7643c` plus les treize patchs ; sur 2 293 objets construits, 2 284 sont identiques à ceux de
l'ancienne méthode, les neuf autres ne diffèrent que par la révision git (`scm_rev.cpp`) et la date de
construction (`__DATE__` de fdk-aac) ; relié avec ces neuf objets-là de l'ancienne construction, le binaire de
la fourche est celui de l'ancienne méthode, octet pour octet.

## Les commits

**0001 — un réglage** : `user_vaddr_max_in_gbytes` dans son `config.json` (section `General`) — la plus
haute adresse du jeu, en Gio (`src/core/emulator_settings.h`, `src/core/address_space.cpp`). `0` (le défaut)
laisse la mise en page de l'éditeur ; le téléphone la ramène à `320` Gio, pour qu'elle tienne dans les 39 bits
de son noyau (chantier 17, `#367`), sous Box64 chargé à 340 Gio. C'était `256` jusqu'au 2026-10-24 : *Kingdom
Hearts III* place 128 Mio à `0x4000000000`, juste au-dessus, et s'arrêtait (« outside the memory map »). Le
serveur le laisse à `0` : rien ne change pour lui.

**0002 — la souris en mode pavé tactile ne bloque que ses boutons** (`#424`, défaut de l'amont, encore là dans
la 0.20.0 et sur `main` le 2026-10-22 ; `src/input/input_mouse.cpp`). streamex met la souris dans ce mode au
lancement (la touche Suppr, `adapter.json`), pour le pavé tactile de la manette à l'écran. `ApplyMouseInputBlockers`
y bloquait toute entrée dont le numéro vaut celui d'un clic gauche ou droit (1 ou 3), sans regarder son type :
l'axe 1 (stick gauche vertical), l'axe 3 (stick droit vertical), le bouton 1 (rond) et le bouton 3 (triangle).

**0003 — la passe statique des patchs CPU n'a lieu que si l'un d'eux peut s'appliquer** (`#503`). Sur un AMD
avec SSE4a (Box64 en `BOX64_CPUTYPE=1`, comme la console), elle parcourait tout le code du jeu pour rien —
108 s pour *Final Fantasy VII Remake* sur le S25 (`CpuPatchesCanApply`, `src/core/cpu_patches.cpp`).

**0004 — un signal du jeu n'est délivré qu'à un point sûr** (`#510`) : dans son code, ou dans une attente
émulée qui ne tient aucun verrou de l'hôte (`ParkedWait`) ; ailleurs il reste en attente et il est relancé
jusque-là. Les jeux Unity (IL2CPP) se figeaient au premier arrêt de leurs fils par le ramasse-miettes,
suspendus au milieu du journal ou de `malloc`.

**0005 — le lecteur vidéo relâche son verrou avant de prévenir le jeu** (`#511`, `avplayer_state.cpp`). Le
rappel du jeu, sur le même fil, le reprenait (`EDEADLK`), et shadPS4 s'arrêtait à la première vidéo de
*Kingdom Hearts Melody of Memory*.

**0006 — les manettes d'un serveur DSU** (`#509`, `src/input/gameflex_dsu.*`) : `GAMEFLEX_DSU_SERVER=<ip>:<port>`,
les quatre manettes d'un serveur DSU (cemuhook) deviennent quatre manettes virtuelles de SDL, données aux
joueurs 1 à 4 — celles du téléphone, dont le clavier X n'avait de touches que pour un joueur. Sans la variable,
rien ne change (le serveur).

**0007 — le fil de service du noyau ne perd plus une demande** (`#512`, défaut de l'amont, encore sur `main` le
2026-10-27 ; `kernel.cpp`). Il prend ses demandes AVANT de faire tourner `io_context`, au lieu de les effacer
après : une minuterie programmée au moment où `run()` finissait était perdue, et le jeu l'attendait sans fin —
*FIFA 22* et *FC 27* au démarrage, une fois sur deux.

**0008** (repris de l'amont, `b3d46e9`) n'existe plus : `main` l'a.

**0009 — une tâche audio pour une instance détruite échoue** (`#513`, défaut de l'amont, encore sur `main` le
2026-10-27 ; `ajm_context.cpp`) : paramètre invalide, au lieu d'arrêter l'émulateur. Un lot lancé avant la
destruction arrivait après — *FIFA 22*, quand un menu coupe ses sons.

**0010 — le tampon d'une copie couleur/profondeur a la taille de l'image** (`#613`, défaut de l'amont, encore
sur `main` le 2026-11-04 ; `vk_runtime.cpp`), et non 128 Mo fixes : *FF7 Remake* y copie 256 Mio après son
écran-titre, le GPU écrivait au-delà de la fin et se bloquait.

**0011 — `readv`, `writev`, `preadv` et `pwritev` acceptent une liste vide** (`#622`, `file_system.cpp`), comme
le noyau FreeBSD dont descend celui de la console : l'amont la refuse depuis le 2026-10-28 (`EINVAL`), et
*FC 27* ne pouvait plus sauvegarder ses réglages.

**0012 — les tables de sauts** (`#512`), reprises de la [demande de fusion #5580](https://github.com/shadps4-emu/shadPS4/pull/5580)
de l'amont, encore ouverte le 2026-11-05, sans ses tests : un saut indirect par une table bornée par `s_min_u32`,
un `switch` compilé, devient une suite de comparaisons, au lieu de l'assertion qui arrêtait *FC 27*. À retirer
quand `main` l'aura.

**0013 — `GAMEFLEX_UE_RENDER_HEIGHT`** (720, 540, 360 ; `#617`, `emulator.cpp`) : la hauteur de rendu d'un jeu
Unreal. Sa ligne de commande (`ue4commandline.txt`) n'a aucun effet sur PS4 ; il lit en revanche
`<projet>/saved/config/ps4/engine.ini` s'il existe. Ce dossier est en lecture seule : un `engine.ini` est écrit
dans le dossier de shadPS4 et monté sur `<projet>/saved` (le plus long préfixe l'emporte). *FF7 Remake*, même
scène à 30 images/s : GPU 69 → 37 % à 540p.

**0014 — les tables de caractères de la libc** (`#770`, `libc_internal_str.cpp`). `_Getpctype`, `_Getptolower` et
`_Getptoupper` n'étaient que des bouchons qui rendaient zéro. Depuis que l'amont émule `sceLibcMspaceCreate` (`#5425`,
venu avec le passage à `main` du 2026-11-17), la bibliothèque réseau de *Final Fantasy VII Remake* et de *Kingdom
Hearts III* ne renonce plus à son démarrage : elle lance son fil, classe un caractère par `_Getpctype` et lisait
l'adresse 2 × c (`0xc4` pour « b »), et shadPS4 s'arrêtait avant l'écran-titre. Chaque fonction rend désormais l'entrée
0 d'une table Dinkumware de 257 `short`, dont l'entrée -1 est EOF. Au banc, les deux jeux retrouvent leur écran-titre.

**0015 — une image à la même adresse, déclarée plus grande, dont les ressources sont déjà là** (`#770`,
`texture_cache.cpp`, `ResolveOverlap`). *FIFA 22* s'arrêtait en jeu, à un moment variable, sur « Unresolvable image
overlap with equal memory address » puis `UNREACHABLE` : il dessine dans une cible de rendu 1920×1080 à un seul niveau,
déclarée sur 16 Mio, à l'adresse d'une texture du même format, des mêmes dimensions, du même pas et du même tuilage, à
11 niveaux, sur 12 Mio. Tout ce qui est demandé existe déjà dans l'image en cache ; seule la taille déclarée diffère, de
quelques mégaoctets de remplissage. L'image en cache sert alors de vue, au lieu d'atteindre le cas « impossible ».

**0016 — les boucles des shaders bornées, sur demande** (`#770`, `emit_spirv.cpp`, `vk_pipeline_serialization.cpp`). Le
pixel shader d'un dessin d'*EA Sports FC 27* boucle « tant que i < compte », le compte lu dans un tampon de constantes ;
au chargement qui suit l'écran-titre, il y vaut une valeur immense et le GPU ne répond plus (`ring gfx timeout`, rapport
de RADV : `docs/reports/2026-11-19-fc27-blocage-gpu/`). Avec `GAMEFLEX_SHADER_LOOP_LIMIT=N`, chaque boucle traduite reçoit
un compteur, remis à zéro à son entrée et vérifié à son retour : elle s'arrête après N tours. Sans la variable, rien ne
change. Un shader compilé avec une borne se range sous un autre nom dans le cache de shadPS4. Le réglage « Borne des
boucles de shaders » la pose, par jeu.

**0017 — une table de sauts dont la lecture écrase son propre index** (`#770`, `control_flow_graph.cpp`). Un compute
shader d'*FC 27*, à l'écran du choix des maillots, fait un `switch` par une table de sauts :
`s_load_dwordx2 s[20:21], s[22:23], s20` lit l'adresse du cas dans le registre même qui portait son numéro. La
reconnaissance de la table (0012) exigeait que ce numéro reste vivant jusqu'au saut, refusait, et shadPS4 s'arrêtait sur
« S_SETPC_B64 without a resolvable offset ». Cette lecture n'est jamais traduite (la table est lue à la compilation) :
dans le shader traduit, le registre garde le numéro jusqu'au saut, et la forme est désormais acceptée. Quand un saut
reste inconnu, le journal donne aussi toutes les instructions qui y mènent, avec les champs de leurs lectures scalaires.

**0018 — un tampon de texels qui commence à l'intérieur d'une image** (`#770`, `buffer_cache.cpp`,
`SynchronizeMemoryFromImage`). Au « Start Match » d'*FC 27*, un tampon de texels commence 0x30 octets après le début
d'une image en cache, et shadPS4 s'arrêtait sur « Texel buffer aliases image subresources ». Les octets qui le précèdent
appartiennent à la même image : elle est réécrite en entier, à sa propre adresse, tant que la zone mémoire du tampon la
contient (sinon rien n'est recopié, avec un avertissement). Avec 0016 et 0017, le match d'*FC 27* se joue.

## Les fichiers

| Fichier | Rôle |
|---|---|
| `build.sh` | construit le dépôt officiel à `BASE` plus `patches/`, dans un conteneur trixie gardé |
| `patches/` | nos correctifs, un patch par numéro, réexportés par `tools/emulator-patches.sh export shadps4` |
| `bench.sh`, `bench/` | l'essai sur x86_64 dans l'image PS4 du serveur : plage du téléphone (320 Gio) contre témoin, ou les plages de `RUNS` ; le jeu déballé, ou son `.pkg` |

## Changer shadPS4

1. **Un correctif** : un commit sur la branche `gameflex` du clone de travail (titre `gameflex 00NN: …`, message
   en anglais qui dit pourquoi), puis `tools/emulator-patches.sh export shadps4` (la série dans `patches/`,
   vérifiée), et son entrée ici.
2. **Une nouvelle source de l'amont** : dans le clone, `git fetch upstream`, rebaser la branche sur le nouveau commit
   de `main` (ou une étiquette), commit par commit — un conflit se règle dans le commit concerné. Pour 0001 : ses
   deux idées (le `Setting<int>` à côté de `extra_fmem_in_mbytes`, la borne de `user_size` dans le constructeur
   POSIX de `AddressSpace::Impl`). Un commit que l'amont a repris disparaît, et son numéro avec (comme 0008). Puis
   `BASE` dans `build.sh`, et l'export.
3. **Construire** : `tools/shadps4/build.sh <sortie>`.
4. **Essayer** : `tools/shadps4/bench.sh <sortie>/shadps4 "<bibliothèque>/roms/ps4/Sonic Mania Plus"`
   — même écran et même journal à 256 qu'au témoin.
5. **Livrer au serveur** : copier `<sortie>/shadps4` dans `<bibliothèque>/cores/ps4/` sous le nom
   `shadps4-gameflex-<version>`, puis relancer une partie : l'image le pose au lancement par-dessus
   celui de l'éditeur s'il est là (`consoles/unpack-emulator.sh`), et le journal de la partie le
   dit. Aucune reconstruction (chantier 29).
6. **Sa source** : une ligne dans `tools/gpl-sources/manifest.tsv` (le dépôt officiel à `BASE`, et le commit de
   ce dépôt dont `patches/` l'a construit). Le registre et les patchs partent dans le dépôt public `gameflex` ;
   le code de shadPS4 reste chez l'éditeur : aucun dépôt à nous ne le porte (chantier 63).

**Le passage à `main` du 2026-11-17** (`#770`, [rapport](../../docs/reports/2026-11-17-correctifs-amont/README.md)) :
la branche rebasée sur `7c0b846d`, 50 commits de l'amont de plus (instructions de shaders, `CarvePhysArea`, le TSC de
l'invité, fichiers et sauvegardes, son). Onze commits passent tels quels, et 0014 s'ajoute (sans lui, *FF7 Remake* et *KH3* s'arrêtent avant leur
écran-titre) ; 0010 seul entre en conflit, avec la fonction
`FindSubrectView` que l'amont ajoute juste avant (`#4953`) : les deux gardées, le défaut des 128 Mo est toujours chez
eux. 0003 reste juste : le nouveau patch `RDTSC` (`#5715`) passe par `PrePatchInstructions`, que 0003 ne saute pas.
0007 et 0011 inchangés (l'amont n'a corrigé ni l'un ni l'autre), 0012 reste (`#5580` toujours ouverte). La branche
d'avant reste joignable par sa révision (`3caec584`) ; la nouvelle est d'abord poussée sous `gameflex-2026-11-17`,
le temps des épreuves.

**Le passage à la 0.20.0** (2026-10-26, `#487`) : 0001 rebasé — `shad_net_enabled` avait quitté
`GeneralSettings` pour une section réseau ; le réglage et sa borne inchangés. Sur la dev, *Kingdom Hearts III*
jusqu'à son premier écran ; au banc, à 320 Gio (la plage du téléphone), le même écran.

Le conteneur de build (`gameflex-shadps4-build`) garde dépendances, sources et objets : une nouvelle série se
reconstruit en quelques minutes. Un `/src` qui n'est pas un clone du dépôt officiel (l'ancien clone de la
fourche) est recloné. `docker rm -f gameflex-shadps4-build` pour repartir de zéro.
