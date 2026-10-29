# KytyPS5 construit par GameFlex

Le Kyty de l'éditeur, plus dix-sept patchs, appliqués dans l'ordre de leur numéro.

**0001 — une option de compilation** : `KYTY_COMPACT_ADDRESS_SPACE`. Elle range tout l'espace
d'adresses du jeu sous 512 Gio — la plage du jeu jusqu'à 256 Gio, la région étendue (128 Gio) juste
après au lieu de 512 Gio posés à 8 Tio, le point de pliage du GPU égal à la base étendue — pour
qu'il tienne dans les 39 bits d'un téléphone (chantier 17, `#367`). Chez Kyty ces valeurs servent
dans des contextes `constexpr` : c'est une option de compilation, pas un réglage. Le serveur peut
prendre cette construction : *Richman 11* y joue à l'identique.

**0002 — l'extension barycentrique optionnelle** (`#373`). Kyty exigeait
`VK_KHR_fragment_shader_barycentric` : son recompilateur lit chaque paramètre interpolé d'un
pixel shader comme trois entrées *per-vertex* qu'il mélange lui-même avec `gl_BaryCoordKHR`. Turnip
(Mesa 26.1.6 comme `main`) n'a pas cette extension — l'Adreno 830 du S25 était refusé au choix du
périphérique. Avec le patch, l'extension est demandée si elle existe ; sinon chaque paramètre
devient une entrée interpolée par le matériel (le chemin que Kyty réservait déjà aux paramètres
*flat*, et celui de shadPS4 pour tout), et les VGPR I/J du shader lisent zéro — `v_interp_p2` ne
s'en sert pas. L'arrivée est automatique (`GraphicContext::fragment_shader_barycentric_enabled`
→ `ShaderRecompiler::SetFragmentBarycentricSupported`) ; `KYTY_NO_FRAGMENT_BARYCENTRIC=1` dans
l'environnement force ce chemin sur un GPU qui a l'extension, pour l'éprouver sur x86_64 (le banc,
avec `FORCE_NO_BARYCENTRIC=1`). Ce que ça perd : les shaders qui calculent leurs propres I/J
(interpolation personnalisée) — ils lisent l'interpolation du matériel à la place. Depuis
`KytyPS5-2026-10-22` (`890393f`), Kyty démarre seul sans l'extension, mais un shader qui en a
besoin échoue toujours : le patch ne garde que le chemin du recompilateur, la variable
`KYTY_NO_FRAGMENT_BARYCENTRIC` et la ligne de la console.

**0003 — les lignes du chargeur sur la console** (`#373`). Kyty tait ses lignes LOGF par défaut,
et le journal de l'application ne reprend que la console : sur le téléphone, on ne voyait pas quels
modules un jeu charge. `Loading: <module>` passe donc sur la console, et surtout le refus de la
libc du jeu : Kyty ne charge `sce_module/libc.prx` que si `IsFileExisting` répond oui (un
`stat()`), et l'écartait **sans rien dire** — ses imports deviennent alors des bouchons, et le
premier appel plante loin de la cause. La ligne `libc.prx not loaded: <chemin> is not a file
(errno …)` dit le chemin résolu et l'erreur.

**0004 — les choix GPU et la pile d'un planificateur utilisé trop tôt** (`#373`). Une ligne
`Vulkan device paths: mesh_shader=… subgroup=défaut/min/max wave64=… shared_int64_atomics=…
depth_feedback=… provoking_vertex_last=…` : les chemins que Kyty prend sur ce GPU (au banc,
RADV : `true 64/32/64 true true true true`). Et quand `CommandScheduler::CheckActive` échoue —
le planificateur servi avant son premier `Begin` —, les adresses de retour de la pile
(`_Unwind_Backtrace`), à retrouver par `addr2line -f -C -e kyty_emulator <adresse − base>` ; la
base sous Box64 est `BOX64_LOAD_ADDR` (`0x6800000000`).

**0005 — un défaut « lecture » sur une page lisible est une écriture** (`#373`). Kyty suit la
mémoire du GPU par défauts de page (`RenderContext::HandleFault`) : une écriture invalide ses
caches, une lecture relit le GPU. Une page n'est illisible que si un *read watcher* l'a rendue
inaccessible ; ailleurs elle reste lisible, et seule une écriture peut y faire défaut. Sous Box64,
le type d'accès est reconstruit — et une écriture rapportée en lecture partait relire le GPU,
créait un tampon avant toute commande et arrêtait Kyty. `PageManager::IsReadWatched` le dit ;
les huit premiers défauts sont tracés (`GPU page fault at … reported …, handled as …`). Au banc
(x86) : uniquement des écritures, sur trois pages qui reviennent.

**0006 — le suivi GPU garde le droit d'exécution** (`#373`). Le gestionnaire de pages protège en
`Mode::Read` puis rend en `Mode::ReadWrite` : `PROT_READ`, puis `PROT_READ|PROT_WRITE` — le droit
d'exécution disparaissait de toute page suivie. Une page où du code du jeu côtoie des données du
GPU devenait non exécutable ; Box64, qui vérifie la protection avant de traduire du x86, s'arrêtait
dessus (S25 : `GPU page fault at 0x901f9c000: reported execute`). `ProtectTransient` (son seul
appelant est le gestionnaire de pages) garde l'exécution — `ExecuteRead`, `ExecuteReadWrite` —,
seule une page inaccessible le reste. Et un défaut d'exécution sur une page lisible n'est plus
relu comme une lecture : il n'est pas du suivi, Kyty le laisse à ses autres gestionnaires. La
trace dit aussi les compteurs de la page (`read watchers`, `write watchers`).

**0007 — les lectures formatées d'un tampon choisi par le GPU, de toute largeur** (`#431`,
2026-10-24). *Final Fantasy VII Rebirth* (PPSA08668) s'arrêtait à 38 s, avant sa première image :
« shader resource tracking: … buffer descriptor is not a valid runtime value; GPU-selected access
requires a scalar, raw 8/16-bit, raw DWORD x1/x2/x3/x4, or formatted X load ». L'instruction est
`BUFFER_LOAD_FORMAT_D16_XY` (MUBUF `0x81`, à `pc 0x37c` — celle que l'issue
[#1796](https://github.com/KytyPS5/KytyPS5/issues/1796) avait vue avant que Kyty sache la
décoder), et son descripteur se calcule dans le shader. Par ce chemin (`IndirectBuffer`, l'accès
par adresse), Kyty ne savait lire qu'**une** composante formatée de 32 bits
(`LoadIndirectFormattedX`) ; un descripteur fixe, lui, lit toutes les largeurs
(`LoadFormattedPrepared`). Le patch :
- accepte les lectures formatées X, XY, XYZ, XYZW et leurs variantes D16
  (`MemoryInfo::SupportsIndirectBufferLoad`) ;
- convertit chaque composante que son `DST_SEL` choisit (mot 3 du descripteur, trois bits par
  composante), par la fonction de conversion que Kyty avait déjà pour X ;
- pour D16, range deux composantes par mot comme `LoadFormattedPrepared` : demi-flottants pour un
  format flottant ou normalisé, entiers bornés sinon. Le format n'étant connu qu'à l'exécution,
  une fonction de plus (`indirect_buffer_format_integer`) dit s'il est entier.

Une lecture X de 32 bits suit exactement l'ancien calcul. Vérifié sur le serveur (Kyty de
l'éditeur + ce seul patch, sans `KYTY_COMPACT_ADDRESS_SPACE`) : *FF7 Rebirth* atteint son écran
titre en une minute, puis le menu principal et « Nouvelle partie » — les couleurs fausses, comme
*FF7 Remake* (textures BC6H).

**0008 à 0012 — ce qu'il manquait ensuite à *FF7 Rebirth*** (2026-10-24, joué par le porteur
sur la dev, un arrêt de Kyty à chaque fois, un patch à chaque fois) :
- **0008, le compteur des « observateurs d'écriture » d'une page sur 15 bits** au lieu de 7
  (`pageManager.cpp`) : passé le menu, une page de 4 Kio portait plus de 127 tampons du GPU —
  « PageManager fatal: write-watcher overflow », sortie 66. L'état d'une page passe d'un à deux
  octets. Le diagnostic de 0006 (`WatcherCounts`) range désormais les deux compteurs sur 16 bits.
- **0009, des pools de descripteurs à la taille des ensembles** (`descriptorHeap.cpp`,
  `shaders.cpp`) : un pool neuf qui ne pouvait pas tenir un seul ensemble arrêtait Kyty
  (`descriptorHeap.cpp:57`). La création d'un ensemble trop grand pour être poussé note son besoin
  par type (`RequireDescriptorPoolSize`), les pools suivants le couvrent, et la console le dit
  (« Descriptor pools grow: … »). Pas revu à l'essai suivant : non confirmé.
- **0010, DCC en blocs indépendants de 64 et de 128 octets à la fois** (`pm4Handlers.cpp`) : un
  réglage valide sur RDNA2, traité en « Not implemented ». Kyty ne garde la valeur que pour son
  vidage de débogage ; 64 octets, la contrainte la plus stricte.
- **0011, CMASK et DCC alignés différemment** (`pm4Handlers.cpp`), et trois drapeaux voisins
  (indices VRS dans CMASK, combinateur DCC désactivé, clés d'effacement DCC) dans la vérification
  des cibles (`debug.cpp`) : Kyty dessine sans ces métadonnées ; une ligne au journal au lieu d'un
  arrêt.
- **0012, une cible de format COLOR_INVALID est éteinte** (`renderDraw.cpp`) : le bloc couleur
  ignore ce que le shader y écrit ; Kyty voulait la dessiner (« unsupported render-target format
  combination: layout=0 »).

**Où *FF7 Rebirth* s'arrête maintenant** : à la première scène, « unknown PS5 render-target tile
mode: 0x0000000c ». Kyty — l'éditeur compris — ne connaît que sept agencements d'image en mémoire
(`Prospero::TileMode`) ; le jeu en emploie un huitième. Le gérer veut dire apprendre au découpeur
(`tiler.cpp`) un agencement de plus : un autre ordre de travail que les patchs ci-dessus.

**0013 — les vidéos d'un décodeur ouvert en 10 bits sortent en P010** (`#431`, 2026-10-24 ;
`videoDec2Decoder.cpp/.h`, `libVideoDec2.cpp`, `libVdecsw.cpp`). La vidéo d'ouverture de
*FF7 Remake* restait « en deux et verte ». Le jeu ouvre le décodeur VP9 (`Videodec2`) en
**profil 2** (10 bits), puis recopie chaque image dans ses propres textures (luminance R16,
chrominance R16G16) comme du **P010** : 16 bits par échantillon, des lignes de deux fois le pas
en octets, la chrominance entrelacée juste après la luminance (`pas en octets × hauteur`). Kyty
rendait toujours du NV12 8 bits : le jeu lisait la moitié de chaque image par ligne, et sa
chrominance dans une zone jamais écrite — des zéros, donc du vert. Mesuré en remplissant le reste du
tampon de pages numérotées : la ligne r de la chrominance du jeu venait de la page 1080 + r.
Avec le patch, un décodeur VP9 de profil 2 ou 3, ou HEVC Main 10 (`HighBitDepthProfile`), rend du
P010 (FFmpeg convertit, même un flux 8 bits — celui de *FF7 Remake* l'est), et
`frame_pitch_in_bytes` dit le double du pas. Les autres profils gardent le NV12. Vérifié sur la
dev : la vidéo d'ouverture en couleurs, plein écran.

**0014 — le découpeur GPU choisit ses constantes par des branches** (`#431`, 2026-10-24 ;
`gpu_tiler_standard64_3d.inc`, `gpu_tiler_prt.comp`, `gpu_tiler_prt_3d.comp`). Le menu et tout
*FF7 Remake* sortaient en aplats sarcelle cernés d'orange. Une capture RenderDoc (`--rd`) l'a
situé dans la dernière passe, et dans ses tables de couleurs 3D (32×32×32, R16G16B16A16,
« standard 64 Ko ») : leurs colonnes 16 à 31 recopiaient les colonnes 0 à 15. La même table, lue
dans la mémoire du jeu et redécoupée selon addrlib (Mesa), est propre ; texel par texel, Kyty
mettait le bit 3 de x au bit 15 de l'adresse, là où AMD met le bit 4. La source de Kyty est
juste ; c'est sa compilation : `ELEMENT_BYTES == 8 ? uvec4(3, 3, 3, 4) : … uvec4(2, 3, 3, 3)`
sur une constante de spécialisation devient un `OpSpecConstantOp Select` de vecteurs à condition
scalaire, et Mesa 26.1 ne l'applique qu'à la première composante — les trois autres prennent le
dernier cas. Touché : toute texture 3D « standard 64 Ko » de 1, 2, 4 ou 8 octets par texel (pas
16 : le BC6H était donc juste), et les deux découpeurs PRT. Le patch écrit ces choix en `if`
(plus aucun `Select` de vecteurs dans le SPIR-V). Vérifié sur la dev : le menu dans ses vraies
couleurs (l'épée bleutée, la lumière blanche au-dessus).

**0015 — les requêtes d'occlusion se demandent aussi par l'environnement** (`#445`,
2026-10-24 ; `main.cpp`). Depuis `KytyPS5-2026-10-23-6aaa533`, Kyty sait compter sur le GPU ce que
le jeu demande des objets cachés (`--occlusion-queries`) ; sans l'option, tout compte comme
visible. *FF7 Remake* en jeu (aux portillons, mode « Résolution ») : **13,5 i/s au lieu de 10**,
un tiers des appels de dessin (330 par image au lieu de ~1 070) ; rien au menu, qui ne cache rien.
La fiche des paramètres de GameFlex ne passe que des variables d'environnement : le patch fait
valoir `KYTY_OCCLUSION_QUERIES=1` (`0` l'éteint) comme l'option, que la fiche règle par
l'interrupteur « Occlusion queries » (allumé par défaut, `shipped/ps5/kyty.json`).

**Le passage à `6aaa533`** (2026-10-24) : 0001 et 0003 à 0014 s'appliquent tels quels ; **0002**
est rebasé — l'éditeur a ajouté des champs à `GraphicContext` (requêtes d'occlusion) et un
paramètre au constructeur de `PipelineCache`, ce qui déplaçait trois contextes. `build.sh` reclone
désormais un arbre qui n'est pas à la version demandée (`.gameflex-tag`) : le clone est superficiel
et ne peut pas changer d'étiquette.

**Notre construction du téléphone ne passe pas ce jeu au serveur** : avec 0001 à 0007 et
`KYTY_COMPACT_ADDRESS_SPACE`, Kyty s'arrête avant, au chargement de la libc du jeu
(`sce_module/libc.prx`, « Unhandled host exception … address=0xffffffffffffe908 »,
`runtimeLinker.cpp:716`). **Isolé le 2026-10-28 : c'est 0001, l'espace compact** — pour
*Kingdom Hearts III + Re Mind* et *Clair Obscur* au moins, qui s'arrêtent au même endroit. Les jeux
Unreal PS5 réservent 512 Gio d'un seul tenant (`sceKernelReserveVirtualRange`, `len =
0x8000000000` à partir de 64 Gio), plus que les 256 Gio du jeu en espace compact. Kyty refuse
(`0x8002000c`), et le jeu s'arrête sur son erreur fatale. Le même arbre, 0005 et 0006 compris, mais
compilé sans l'espace compact, amène ces deux jeux à leur premier écran
([rapport](../../docs/reports/2026-10-28-unreal-ps5-telephone/README.md), `#524`). *FF7 Rebirth*
s'arrête à la même adresse que *Clair Obscur* : très probablement la même cause, non vérifiée.
Le serveur prend donc sa propre construction, `build.sh --server` : l'éditeur et les
patchs 0007 et suivants, sans l'espace compact, dans `/src-server` du même conteneur. C'est elle qui
est dans `cores/ps5/` de la dev et de la release depuis le 2026-10-24.

**0013 révisé** (2026-10-24, la revue de [KytyPS5#1877](https://github.com/KytyPS5/KytyPS5/pull/1877),
où il est proposé) : `QueryDecoderMemoryInfo` annonce la taille d'image du P010 (elle restait à
16 Mio, trop peu en 4K 10 bits), et Vdecsw, qui ne décode que de l'AVC, n'est plus touché. Le
patch est identique à la PR. Les binaires de `cores/ps5/` et le paquet du téléphone portent encore
la version d'avant : rien ne change pour *FF7 Remake* (2048×1088), ils la prendront à la prochaine
livraison.

**0016 — le cache de shaders, par l'environnement et marqué de la révision de Kyty** (`#487`,
2026-10-26 ; `main.cpp`, `pipelineSerialization.cpp`). Depuis `KytyPS5-2026-10-24`, Kyty peut
garder les shaders et les pipelines d'un jeu (`_PipelineCache/<jeu>.shaders`) et les recompiler au
démarrage, avant la première image (`--shader-cache`) ; son lanceur l'allume par défaut, la ligne
de commande non. Le patch fait valoir `KYTY_SHADER_CACHE=1` (`0` l'éteint) comme l'option ; la
fiche de GameFlex a l'interrupteur « Shader cache », allumé, et le lanceur le pose quand aucun
paramètre n'en décide (`kytyAuto`). Kyty reconnaît son cache à la date de son exécutable
(`/proc/self/exe`) : sous Box64, c'est Box64, dont la date ne bouge pas quand Kyty change — un cache
écrit par un autre Kyty serait relu. Le patch ajoute à l'en-tête la révision de Kyty
(`KYTY_GIT_REVISION`). Côté serveur, `unpack-emulator.sh` recopie désormais notre construction avec
sa date (`cp -p`) : recopiée à neuf à chaque partie, elle jetait le cache. Vérifié sur la dev :
*FF7 Remake*, la première partie écrit 0,8 Mo, la seconde relit 87 programmes, 91 modules et 65
pipelines en 11 ms.

**0017 — un jeu qui sort de lui-même ne fait plus planter Kyty** (`#522`, 2026-10-28 ;
`libC.cpp`, `libKernel.cpp`). Le `exit()` du jeu appelait le `::exit()` de l'hôte, qui détruit les
objets statiques alors que les fils du jeu et la boucle de la fenêtre tournent encore. Le fil de
l'image lisait alors `Config::GetVblankFrequency()` sur un `g_config` détruit, et une sortie propre
finissait en « Unhandled host exception », exit 65. C'était le cas de *FF XVI* (`return from main
= 0`) et de *Tekken 8* au serveur ([rapport](../../docs/reports/2026-10-27-ps5-serveur/README.md)).
Le patch sort comme Kyty le fait déjà quand on ferme sa fenêtre (`Emulator::Execute`) : par
`std::quick_exit`, après avoir vidé la console, pour que les lignes qui disent pourquoi le jeu est
parti ne se perdent pas. Le code de sortie est celui du jeu, et GameFlex dit « le jeu s'est arrêté
de lui-même » (`#504`).

**Le passage à `597137e`** (2026-10-26, `#487`) : 57 commits de l'éditeur. 0002 est rebasé
(le constructeur de `PipelineCache` préchauffe désormais le cache de shaders : le drapeau
barycentrique arrive avant), 0006 aussi (`ProtectTransient` prend un verrou partagé), et 0012
suit son code, passé de `renderDraw.cpp` à `PipelineCache::GetGraphicsPrograms`. Les autres
s'appliquent tels quels. *FF7 Remake* jusqu'à son écran-titre sur la dev ; au banc, par le chemin du
téléphone, *Neptunia ReVerse* jusqu'à son menu.

**Le téléphone, à `6aaa533`** (2026-10-24) : `build.sh` sans option — les quinze patchs et
l'espace compact, appliqués tels quels. Au banc, par le chemin du téléphone
(`FORCE_NO_BARYCENTRIC=1`), *Neptunia ReVerse* arrive à son premier écran. Le bundle est
`kyty-2026-10-23+box64-d653d0ebe-forky-r1` : la même construction de Box64 qu'avant, seul
`kyty_emulator` change. `tools/box64/bundle.sh` copie quelques bibliothèques x86_64 d'un trixie
(`SHADPS4_BUILD_CONTAINER`) : le conteneur de construction de Kyty en est un.

| Fichier | Rôle |
|---|---|
| `0001-compact-address-space.patch` | l'option (27 lignes : `CMakeLists.txt`, `src/kernel/memory.h`, `src/kernel/memoryAddressSpace.inc`, `src/graphics/host_gpu/regionDefinitions.h`) |
| `0002-fragment-barycentric-optional.patch` | le chemin sans l'extension (56 lignes : `vulkanWindow.cpp`, `graphicContext.h`, `pipelineCache.cpp`, `ShaderRecompiler.h/.cpp`, `ShaderInfoCollection.cpp`) |
| `0003-loader-console-lines.patch` | les lignes du chargeur sur la console (13 lignes : `runtimeLinker.cpp`) |
| `0004-gpu-console-diagnostics.patch` | les choix GPU et la pile du planificateur inactif (38 lignes : `vulkanWindow.cpp`, `commandScheduler.cpp`) |
| `0005-fault-read-on-write-watched-page.patch` | le défaut « lecture » sur une page lisible traité en écriture (35 lignes : `pageManager.h/.cpp`, `renderContext.cpp`) |
| `0006-gpu-tracking-keeps-execute.patch` | le suivi GPU garde le droit d'exécution (31 lignes : `memoryAddressSpace.inc`, `pageManager.h/.cpp`, `renderContext.cpp`) |
| `0007-indirect-formatted-buffer-loads.patch` | les lectures formatées d'un tampon choisi par le GPU, de toute largeur (191 lignes : `ShaderIR.h`, `spirvEmitterInternal.h`, `spirvEmitterMemory.cpp`) |
| `0008-page-watchers-16-bits.patch` | le compteur d'observateurs d'une page sur 15 bits (`pageManager.cpp`) |
| `0009-descriptor-pools-fit-layouts.patch` | les pools de descripteurs à la taille des ensembles (`descriptorHeap.h/.cpp`, `shaders.cpp`) |
| `0010-dcc-both-independent-blocks.patch` | DCC en blocs indépendants de 64 et 128 octets (`pm4Handlers.cpp`) |
| `0011-color-metadata-flags-accepted.patch` | les drapeaux CMASK/DCC que Kyty n'émule pas, acceptés (`pm4Handlers.cpp`, `debug.cpp`) |
| `0012-invalid-color-targets-off.patch` | une cible COLOR_INVALID éteinte (`renderDraw.cpp`) |
| `0013-videodec-p010-for-10-bit-profiles.patch` | P010 pour un décodeur vidéo ouvert en 10 bits (`videoDec2Decoder.cpp/.h`, `libVideoDec2.cpp`, `libVdecsw.cpp`) |
| `0014-tiler-no-vector-selects-on-spec-constants.patch` | les constantes du découpeur GPU choisies par des branches (`gpu_tiler_standard64_3d.inc`, `gpu_tiler_prt.comp`, `gpu_tiler_prt_3d.comp`) |
| `0015-occlusion-queries-from-environment.patch` | `KYTY_OCCLUSION_QUERIES` dans l'environnement vaut `--occlusion-queries` (`main.cpp`) |
| `0016-shader-cache-from-environment.patch` | `KYTY_SHADER_CACHE` dans l'environnement vaut `--shader-cache`, et l'en-tête du cache porte la révision de Kyty (`main.cpp`, `pipelineSerialization.cpp`) |
| `0017-guest-exit-without-static-destructors.patch` | le `exit()` d'un jeu sort par `quick_exit`, comme une fenêtre fermée : les fils du jeu tournent encore (`libC.cpp`, `libKernel.cpp`, `#522`) |
| `build.sh` | construit `kyty_emulator` à la version du serveur, avec les patchs, sans lanceur Qt, dans un conteneur trixie gardé ; `--server` : la construction du serveur (0007 et suivants, sans l'espace compact) |
| `bench.sh`, `bench/` | l'essai sur x86_64 dans l'image PS5 du serveur : notre construction contre le Kyty officiel |

## Mettre à jour quand l'éditeur publie une version

1. **La version** : `ARG KYTY_RELEASE=…` dans
   `src/microservices/runtimes/streamex/consoles/ps5/Dockerfile` (le script la lit là).
2. **Construire** : `tools/kyty/build.sh <sortie>`. Si un patch ne s'applique plus, le script
   tente une fusion à trois voies et le dit ; s'il s'arrête, rebaser ce patch-là (un clone à la
   version, les autres patchs appliqués avant lui, `git diff > <son nom>.patch`) :
   - 0001 : les quatre constantes (`kExtendedMemoryBase`/`Size`, `HOST_USER_MAX`,
     `TRACKER_ADDRESS_SIZE`, `LOWER_ADDRESS_SIZE`) gardent leur règle — **base étendue = point de
     pliage**, tout sous 2³⁹ ;
   - 0002 : le drapeau (`graphics.fragment_shader_barycentric_enabled`, la valeur que Kyty
     calcule déjà dans `VulkanCreateDevice`) arrive au recompilateur avant le premier shader ;
     sans lui `CollectPixelInputs` ne marque aucun paramètre *per-vertex*, et la boucle des
     entrées intégrées n'ajoute pas `gl_BaryCoord*` ;
3. **Essayer** : `tools/kyty/bench.sh <sortie>/kyty_emulator "<bibliothèque>/roms/ps5/Richman 11"`
   — même écran que le Kyty officiel. Puis `FORCE_NO_BARYCENTRIC=1 tools/kyty/bench.sh …` : le
   même écran encore, par le chemin du téléphone (le parcours `compact` seul). Pour lire les lignes
   LOGF de Kyty (« Select device », « Vulkan fragment shader barycentric: … »), muettes par défaut :
   `KYTY_ARGS="--printf-direction Console"` (le nom de l'énumération, majuscule comprise).
4. **Livrer au serveur** : `tools/kyty/build.sh --server <sortie>`, copier `<sortie>/kyty_emulator`
   dans `<bibliothèque>/cores/ps5/` sous le nom `kyty_emulator-gameflex-<KYTY_RELEASE>`, puis
   relancer une partie : l'image le pose au lancement par-dessus celui de l'éditeur s'il est là
   (`consoles/unpack-emulator.sh`), et le journal de la partie le dit. Aucune reconstruction.
5. **Sa source** : une ligne dans `tools/gpl-sources/manifest.tsv` (le commit amont, celui de ce
   dépôt qui l'a construit), puis `tools/gpl-sources/assemble.py <bibliothèque>` une fois livré dans
   la release, que le miroir sert (chantier 34). Sans sa ligne, le script refuse.
