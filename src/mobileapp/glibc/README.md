# La couche glibc de l'application

Ce qui permet à un binaire **glibc** — xemu, RPCS3, un client X Debian — de vivre dans une
application Android ordinaire, dont la libc est bionic. Le récit complet, mesures à l'appui, est
dans `docs/roadmap/streamex-mobile-exploration.md` ; ce fichier dit ce qu'il y a ici et d'où vient
ce qui n'y est pas.

## Le principe

Un binaire glibc se lance par **son chargeur, invoqué explicitement** — ce qui court-circuite le
`PT_INTERP` codé en dur, qu'Android n'a pas :

    <chargeur> --inhibit-cache --library-path <dossier> <binaire>

Le chargeur démarre, projette `libc.so.6`… et meurt : le **filtre seccomp** de l'application,
hérité du zygote et impossible à élargir, tue par `SIGSYS` tout appel qu'il ne connaît pas — sans
un mot, glibc n'ayant pas le gestionnaire de bionic. D'où le **traceur** (`ptracer.c`) : engendré par
l'application, il exécute l'invité sous `PTRACE`, attrape chaque `SIGSYS`, et répond à la place du
noyau — `-ENOSYS` quand glibc a un repli (`set_robust_list`, `rseq`, `clone3`), **réécriture** en
l'équivalent moderne quand elle n'en a pas (sur x86_64 : `poll`→`ppoll`, `access`→`faccessat`,
`open`→`openat`…), **« fait »** pour un `set*id` qui ne change rien (PulseAudio « abandonne ses
privilèges » ainsi, et ces appels sont refusés à une application ; `EPERM` s'il changerait quelque
chose), et il **nomme** tout ce qu'il ne sait pas traiter. Il suit aussi les threads.

Sur l'appareil (arm64) il n'y a que deux appels à neutraliser et un repli natif : c'est le cas
facile. Le banc x86_64 (l'émulateur) est plus exigeant que la cible — ce qu'il prouve vaut pour
l'appareil *a fortiori*, GPU et vitesse exceptés.

## Ce qui est ici

| fichier | rôle |
|---|---|
| `ptracer.c` | le traceur. Compilé par le NDK pour chaque ABI dans `docker-entrypoint.sh`, déposé en `jniLibs/<abi>/libptracer.so` — le motif `lib*.so` est le seul qu'Android installe là d'où l'app peut exécuter un binaire. Le même source compile en glibc pour le banc. |
| `bench/run.sh` | le banc Linux : reproduit le filtre de l'émulateur dans le Debian jetable du runtime (`RUNTIME_IMAGE`) et vérifie, avec un vrai Xvfb, contrôle négatif → threads → client X. Une seconde par essai. |
| `bench/seccomp_trap.c` | installe sur lui-même un filtre BPF rendant `SIGSYS` pour la liste d'appels refusés par Android x86_64, puis exécute l'invité. |
| `bench/draw_xcb.c` | client X glibc minimal qui ouvre lui-même la socket (le `/tmp/.X11-unix` codé en dur de libxcb n'existe pas sur Android) et peint la racine de couleurs franches : la capture les relit, et le pixel central prouve la chaîne. |
| `bench/witness.c` | le témoin minimal, le même pour toutes les architectures : deux mots écrits = libc initialisée, `main` atteint. |
| `bench/witness_mt.c` | témoin multi-thread : chaque thread refait `set_robust_list`/`rseq` à sa naissance. |
| `bench/arm64-guest.sh` | un invité **aarch64** sans téléphone : son bundle et le runtime aarch64 lancés comme `GlibcGuest` le fait (le chargeur de `jniLibs`, le shim, l'environnement), sous qemu-aarch64 **limité à 39 bits d'adresses** comme le noyau d'un Android, face à un Xvfb, avec des captures toutes les 30 s. `BIOS=`/`PATCHES=` mettent en scène les clés et correctifs de Cemu. Ni seccomp ni traceur (qemu ne trace pas), ni GPU. |
| `../tools/assemble-glibc.sh` | assemble `glibc-<arch>.zip` (libc + témoins + client X + fermeture libxcb) dans le Debian qu'il déclare (`RUNTIME_IMAGE`, forky aujourd'hui), et pose le chargeur dans `jniLibs/<abi>/libglibcloader.so`. |

## Ce qui n'est PAS versionné, et d'où ça vient

Des binaires étrangers, sous leurs propres licences, servis à l'appareil par la webapp (son
dossier `downloads/`) ou posés dans `jniLibs/` (git-ignoré) :

- **`jniLibs/<abi>/libglibcloader.so`** — le chargeur glibc de Debian forky (`ld-linux-aarch64.so.1`
  / `ld-linux-x86-64.so.2`, paquet `libc6` 2.43), renommé pour tenir dans `jniLibs`. Le chargeur
  DOIT être dans le dossier des bibliothèques natives (exécution), la libc NON (elle doit garder
  son nom `libc.so.6`, qu'Android n'y installe pas) — d'où le zip ci-dessous. Les deux viennent
  du MÊME assemblage : un chargeur et une libc de constructions différentes ne partagent pas
  leurs symboles `GLIBC_PRIVATE`.

  > **Pourquoi 2.43 et plus 2.36 (2026-10-02).** Mesuré par `objdump -T` sur les fichiers que le
  > serveur emploie : Cemu 2.6 exige `GLIBC_2.34`, shadPS4 0.18.0 et Kyty `GLIBC_2.38`, le Citron
  > nightly `GLIBC_2.43` (une AppImage « sharun » construite sur Arch). Trixie (2.41) ne suffisait
  > pas ; forky, si. Le banc du traceur passe sous 2.43. Eden, qui remplace Citron (chantier 25) :
  > sa nocturne aarch64 y tient, l'amd64 réclame `GLIBC_2.44` — l'émulateur Android prend la 0.2.1.
- **`jniLibs/<abi>/libxkbcomp.so`** — le `bin/xkbcomp` du bundle Xvfb de la même ABI : Xvfb
  l'exécute par un `sh -c`, et un lien `bin/xkbcomp → nativeLibraryDir/libxkbcomp.so` est le seul
  chemin d'exécution permis.
- **`downloads/glibc-<arch>.zip`** — le runtime d'un invité glibc, à plat (sauf `fonts/`) : le
  jeu **complet** de `libc6` (Debian forky, glibc 2.43 — `libm.so.6` n'est pas dans libc, et les
  stubs `libpthread`/`libdl`/`librt` doivent exister par leur nom pour un binaire lié sur une
  glibc plus ancienne), les témoins, `draw-xcb`, la fermeture de libxcb, et ce qu'une AppImage
  attend d'un système alors qu'Android n'a rien de tel : **Mesa client** (glvnd,
  `libGLX_mesa`/`libEGL_mesa`, `libgallium` = llvmpipe, `libLLVM`), **Vulkan** (chargeur + ICD
  lavapipe, écrit `lvp_icd.<arch>.json` quel que soit le nom que Mesa lui donne ; en aarch64,
  **Turnip compilé pour KGSL** — `libvulkan_freedreno_kgsl.so` et `turnip_icd.aarch64.json`, le
  GPU d'un Adreno, produit par `tools/build-turnip.sh` et donné aux invités par
  `GlibcGuest.useTurnip` quand `/dev/kgsl-3d0` existe ; voir le chantier 11), **polices**
  (DejaVu, `fonts.conf` à préfixe relatif), fontconfig/freetype, `libusb`, et les **extensions
  X11** qu'un client ouvre par `dlopen` (XRandR, XInput2, XCursor, XFixes, xkbcommon…, que SDL
  et Qt chargent par nom), la **base MIME** de freedesktop (`share/mime`, sans laquelle GTK ne
  reconnaît aucune image), et **PulseAudio** (`pulse/` : le démon et ses trois modules — puits
  nul, socket Unix, puits permanent —, la sortie audio que chaque image de console du serveur lève
  et sans laquelle un jeu Wii U n'avance pas ; voir `GlibcGuest.startAudio`). L'invité les trouve
  par `--library-path`, `LIBGL_DRIVERS_PATH`,
  `__GLX_VENDOR_LIBRARY_NAME=mesa`, `VK_ICD_FILENAMES`, `FONTCONFIG_FILE` (voir
  `GlibcGuest.builderFor`). Le fichier `RUNTIME` du zip nomme l'image Debian d'où il vient.
  Assemblé par `tools/assemble-glibc.sh <arch>`, qui pose aussi le chargeur dans `jniLibs`. Après
  un changement de contenu : bumper `GLIBC_VERSION` dans `Xserver.java` pour que l'appareil
  re-télécharge, et **réassembler les invités** (leur fermeture a été calculée contre l'ancien).
- **`downloads/<invité>-<arch>.zip`** — l'émulateur de bureau d'une console, tel que son projet
  le publie et **à la version du serveur** (`consoles/emulators.sh`), par
  `tools/assemble-appimage.sh <console> [arch]` : l'AppImage extraite **sans rien exécuter**
  (squashfs ou DwarFS selon le contenu), ramenée à `usr/bin/<binaire>` + `usr/lib` quelle que soit
  sa mise en page (linuxdeploy, sharun, archive à plat), ses `checkrt/` fusionnés dans `usr/lib`,
  **la plateforme retirée** (libc et pile graphique : c'est le runtime qui les porte), et complétée
  de ce qu'elle suppose du système (clôture des NEEDED contre le Debian **du runtime**, lu dans son
  fichier `RUNTIME` — en multiarch pour aarch64). `Xserver.ensureGuest` la pose sous
  `guests/<nom>` en gardant l'arborescence (RUNPATH `$ORIGIN/../lib`, `qt.conf` à côté du
  binaire) ; `GlibcGuest` la lance sous le traceur, son dossier AVANT le runtime dans le chemin, le
  shim préchargé par `ld.so --preload`.

  | console | invité | binaire | x86_64 | aarch64 |
  |---|---|---|---|---|
  | xbox | `xemu` | `xemu` | ✔ 0.8.139 | ✔ 0.8.139 |
  | ps3 | `rpcs3` | `rpcs3` | ✔ 0.0.43-20404 (celui du serveur) | ✔ même build |
  | wiiu | `cemu` | `Cemu` | ✔ 2.6 | ✔ fced55a — l'AppImage arm64 de sa CI : aucune release arm64, et la 2.6 n'a pas de recompilateur AArch64 ; artefact authentifié (`GITHUB_TOKEN`), qui expire le 2027-01-04 |
  | switch | `eden` | `eden` | ✔ 0.2.1 — la nocturne du serveur réclame `GLIBC_2.44` | ✔ nocturne de7e6e01d4 (celle du serveur) |
  | ps4 | `shadps4` | `shadps4` (+ `pkgextract`) | ✔ 0.18.0 — le serveur est en 0.20.0, qui exige (depuis la 0.19.0) des tampons clairsemés que lavapipe ne fournit pas | — exige ≈ 85 Tio d'adresses, un noyau Android en donne 512 Gio (voir le chantier 11, `#254`) |
  | ps5 | `kyty` | `kyty_emulator` | ✔ 2026-10-26 (celui du serveur) | — mémoire étendue fixée à 8 Tio : même mur |

  `GlibcGuest` porte la même liste d'architectures : une console sans bundle pour l'ABI de
  l'appareil n'y est jamais proposée.

  > Cette ligne a longtemps dit « `rpcs3` : x86_64 seulement — pas d'AppImage arm64 ». C'était
  > faux, et la cause était chez nous : l'AppImage arm64 de RPCS3 vit dans un dépôt **séparé**
  > (`RPCS3/rpcs3-binaries-linux-arm64`), alors qu'on interrogeait celui du x86_64. Assemblée et
  > vérifiée le 2026-09-23 — `usr/bin/rpcs3` : ELF aarch64, 95,4 Mo, bundle de 104 Mo.
- **`downloads/xvfb-bundle-<arch>.zip`** — le serveur X bionic de Termux, assemblé par
  `tools/assemble-xvfb.py <arch>` (voir son en-tête).

Le contenu de ces zips est choisi par **ABI primaire** (`Xserver.abiTag()`), et leurs marqueurs
portent l'ABI : un bundle d'une autre architecture ne passe jamais pour installé.
