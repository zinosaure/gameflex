# Assistant d’émulateurs

Le dépôt possède déjà un moteur libretro et un installateur de cores. La distribution
originale vise Linux x86-64 pour les téléchargements côté serveur. Sur ce Pi ARM64,
seul QuickNES était installé ; les jeux GBA et Game Boy n’avaient aucun core.

Le nouveau parcours, dans **Paramètres › Émulateurs de votre bibliothèque**, dans
la fenêtre de lancement et directement sur la fiche du système, analyse les jeux
présents, la plateforme Linux et l’architecture réelle du serveur. Il recommande
notamment mGBA pour GBA, Gambatte pour Game Boy / Game Boy Color, ParaLLEl N64
pour Nintendo 64 et DeSmuME pour Nintendo DS. Lorsqu’un système contient des jeux
mais aucun core, le bouton **Télécharger mon émulateur** déclenche l’installation
explicite. Le bouton Jouer reste indisponible lorsque le core connu manque.

Les cores tournent sur le serveur GameFlex. Le téléphone et la TV affichent le flux
existant : il ne s’agit pas d’installer un APK d’émulation sur la TV. Le téléchargement
passe par le buildbot officiel Libretro, avec un chemin `linux/aarch64/latest/` ou
`linux/x86_64/latest/`. Aucune URL arbitraire n’est acceptée. L’assistant n’installe
aucun paquet système, jeu, BIOS ou firmware et ne remplace jamais un core existant.

Le traitement est limité à une installation à la fois. Avant publication atomique,
il vérifie la taille, le nom du fichier dans le ZIP, le format ELF, l’architecture,
le chargement des dépendances et l’API libretro dans un sous-processus séparé. La provenance,
la licence et l’empreinte SHA-256 sont conservées dans
`.local/pi-tv/emulator-installations/`. Une incompatibilité laisse les fichiers
existants intacts. Les systèmes non validés sont signalés comme nécessitant une
configuration manuelle plutôt que de télécharger un moteur au hasard.

API authentifiée : `GET /experience-api/v1/emulators` et
`POST /experience-api/v1/emulators/<console>/install` (administrateur uniquement).
Le suivi d’installation figure dans l’inventaire. Le listing des cores du kernel est
rechargé après installation avant de relancer le jeu.

Vérifications : tests locaux d’architecture, d’autorisation, d’archive et de
non-écrasement ; dépendances dans l’image ARM64 réelle de jeu. Un programme GBA
original CC0, sans logo ni BIOS propriétaire, a produit 90 images 240 × 160 avec
mGBA. Les ROMs commerciales de la bibliothèque ne servent pas aux tests automatisés.
Ce test ne constitue pas un essai physique de télécommande ou de manette sur la TV.

Références :
- https://docs.libretro.com/guides/download-cores/
- https://github.com/mgba-emu/mgba
- https://github.com/libretro/gambatte-libretro
- https://github.com/libretro/parallel-n64
- https://github.com/libretro/desmume2014
