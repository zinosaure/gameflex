# Notre Eden (téléphone arm64)

Le serveur joue la nocturne officielle d'Eden (chantier 25). Le téléphone joue la même, construite
ici avec un patch : sans lui, un gros jeu s'y fige au bout de quelques minutes (`#434`).

## Pourquoi

Le fastmem d'Eden plaque chaque morceau de la mémoire de la Switch dans une réserve de 256 Gio, et
chaque morceau est une zone du processus. Android en permet 65 530 (`vm.max_map_count`). Sur le
S25, *Hogwarts Legacy* les remplit en trois minutes, puis Eden écrit « mmap: Cannot allocate
memory » en rafale et le jeu se fige (relevé le 2026-10-22). Le serveur en permet un million.

L'Eden d'Android connaît cette limite : son `HeapTracker` (`src/common/heap_tracker.cpp`) compte
les zones du tas du jeu, en rend la moitié quand il approche du plafond, et remet une page en
place quand le jeu y touche. Mais il ne s'active que sous `#ifdef __ANDROID__`. Le téléphone, lui,
lance l'Eden **Linux** arm64 : son chargeur et son serveur X ne savent pas faire tourner une
application Android.

| Fichier | Rôle |
|---|---|
| `0001-heap-tracker-on-linux-arm64.patch` | le `HeapTracker` partout où Eden a l'exécution native (`HAS_NCE` : arm64 sous Android **ou** Linux) — quatre lignes de `src/core/memory.cpp` |
| `0002-assembler-guard-for-new-kernel-headers.patch` | les en-têtes du noyau récents (linux-api-headers 7.3) testent `__ASSEMBLER__` et non plus `__ASSEMBLY__` : sans lui, `asm-generic/signal.h` redéfinit `sigset_t` (`src/core/arm/nce/arm_nce_asm_definitions.h`) |
| `0003-frame-rate-in-the-window-title.patch` | la cadence d'Eden (« FPS: … \| Speed: …% \| Frame: … ms ») ajoutée au titre de sa fenêtre à chaque mise à jour de sa barre d'état (500 ms) : le téléphone lit les titres (`sessionstatus.TitleFPS`), et son relevé échantillonne l'écran à 30 par seconde (`#439`) |
| `build.sh` | construit Eden aarch64 au commit de la nocturne, avec les patchs, et le livre en AppDir archivé |

Le téléphone l'exécute nativement par défaut, sur tous les cœurs (`GlibcGuest.stageEdenConstraints`).
Le paramètre « Native execution » à `0` rend le recompilateur, sur un cœur et sans fastmem :
l'exécution native, elle, ne va pas sans le fastmem.

**Essayé et retiré** (2026-10-23) : un patch `0004` qui envoyait au registre zéro un accès
vectoriel débordant au-delà de R254, au lieu d'abandonner le shader (« Overflow on register
arithmetic »). Servi au S25 (`eden-…-r3`) : l'erreur disparaît, les aplats noirs de la grotte
d'*Hogwarts Legacy* restent. Retiré, et l'invité revient à 0001-0003 (`eden-…-r4`).

## Construire

```bash
tools/eden/build.sh
# → src/webapp/src/public/downloads/eden-<commit>-gameflex-aarch64.AppDir.tar.gz
```

La construction reprend celle de l'éditeur (sa CI : `mirror/Workflow` sur git.eden-emu.dev) :
- l'image `ghcr.io/pkgforge-dev/archlinux` ;
- les paquets de `.ci/deps/arch.sh` ;
- clang et son profil PGO (`github.com/Eden-CI/PGO`) ;
- pas de LTO en aarch64 ;
- les options de cmake de `.ci/build/*.sh`.

Seul le binaire `eden` change. Il prend la place de celui de l'AppImage officielle, dont les
bibliothèques restent. Le script s'arrête si notre binaire réclame une bibliothèque que l'AppDir
officiel n'apporte pas.

**Émulé** : l'hôte n'a pas d'arm64 et rien n'y est installé. L'image arm64 est exportée sans être
lancée, puis tournée sous `proot` et `qemu-user`, en priorité basse. Le volume `eden-arm64` garde
le système, la source et les objets : une seconde passe ne recompile que ce qui a changé.

Pacman, sous proot, demande deux réglages que le script pose dans `pacman.conf` :
- `CheckSpace` retiré : proot ne sait pas dire la place libre des montages ;
- `DisableSandbox` : le bac à sable de pacman ne passe pas sous qemu-user.

## Livrer au téléphone

1. **L'assembleur** : dans `src/mobileapp/tools/assemble-appimage.sh`, l'adresse aarch64 de
   `switch` passe à `file:///dl/<l'archive>`. Puis `tools/assemble-appimage.sh switch aarch64`.
2. **La version de l'invité** : dans `GlibcGuest`, `eden-…` change de nom. Sinon le téléphone
   garde son paquet.
3. **L'APK** : la reconstruire.

## Changer de nocturne

1. `COMMIT` et `NIGHTLY` dans `build.sh`, à la même valeur que l'adresse du serveur
   (`runtimes/streamex/consoles/emulators.sh`).
2. Si un patch ne s'applique plus, le refaire sur la nouvelle source (`git diff > 000N-….patch`).
3. Avant de garder le patch 0001, vérifier que l'éditeur n'a pas lui-même étendu le
   `HeapTracker` à Linux.

## État (2026-10-23)

**Le premier émulateur de la Switch au téléphone** (`#438`), en exécution native sur tous les
cœurs par défaut (`eden-de7e6e01d4-gameflex-r1`, patch 0002 ajouté). Sur le S25, *Hogwarts
Legacy* :
- 25-30 images/s dans les cinématiques (journal 223) ;
- 12-25 images/s en jeu, la cinématique passée (journal 224) ;
- ses zones mémoire entre 37 000 et 46 500, sans gel.

Le premier échec ci-dessous.

**La construction échouait.** Le patch s'applique, la configuration passe, et
900 objets sur 1 554 se compilent (une heure). Puis `src/core/hle/kernel/k_process.cpp` casse sur
les en-têtes du noyau de l'image arm64 : `asm-generic/signal.h` (inclus par
`core/arm/nce/arm_nce_asm_definitions.h`) redéfinit `sigset_t` déjà posé par la glibc. Le code
d'Eden n'y est pour rien : c'est l'environnement de construction, et la CI de l'éditeur passe. Ce
n'est pas notre patch non plus.

La pause de quelques heures qui a suivi : Citron venait de repasser premier (chantier 26). Eden
a repris la première place le jour même (`#438`).

