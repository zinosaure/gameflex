# GameFlex dans un conteneur

L'AppImage de GameFlex, lancée telle quelle dans une Ubuntu 24.04, sans fenêtre. Pour ceux qui n'ont pas de PC Linux :
**Windows avec Docker Desktop**, un NAS, un serveur sans bureau. Rien de GameFlex n'est construit ici : l'image prend
l'AppImage de la dernière Release (ou celle posée dans `local/`) et ce qu'un bureau Linux a déjà (le pilote graphique
de Mesa, xkbcomp, les polices, les bibliothèques des émulateurs).

```bash
docker compose -f tools/appimage/docker/docker-compose.yml up -d --build
docker exec gameflex gameflex-root-password      # le mot de passe de root, montré une fois
```

Puis `http://<adresse de la machine>:10200` dans un navigateur. Les jeux, BIOS, émulateurs et sauvegardes vont dans
`./library` (ou le dossier nommé par `GAMEFLEX_LIBRARY`) ; les émulateurs des consoles récentes s'installent depuis
**Réglages › Émulateurs**, comme dans l'AppImage.

## Le réseau

La vidéo passe par WebRTC, qui annonce les adresses de la machine. Le compose utilise **le réseau de la machine**
(`network_mode: host`) : rien à publier.
- Linux : rien à faire.
- Docker Desktop (Windows, macOS) : version 4.34 ou plus récente, « Enable host networking » coché dans Settings ›
  Resources › Network.

Sans le réseau de la machine, retirer `network_mode` et décommenter dans le compose : le port, un bloc UDP pour la
vidéo (`ICE_PORT_MIN`, `ICE_PORT_MAX`) et `NAT_1TO1_IP`, l'adresse de la machine que les autres appareils joignent.

## Le GPU

- **Linux** (Intel, AMD) : `/dev/dri` est passé au conteneur ; les consoles récentes rendent sur le GPU, avec le Mesa
  de l'image.
- **Windows** : retirer `devices`. Docker Desktop ne donne pas le GPU à Vulkan comme Linux le fait : les consoles
  rétro jouent, les consoles récentes rendraient sur le processeur, donc lentement. **Pas éprouvé sous Windows.**
- NVIDIA : son pilote n'est pas dans l'image ; une AppImage lancée directement sur le bureau s'en sert.

## Une autre version

```bash
cp GameFlex-x86_64.AppImage tools/appimage/docker/local/    # celle-ci plutôt que la Release
docker build -t gameflex-appimage --build-arg APPIMAGE_URL=https://…/GameFlex-x86_64.AppImage tools/appimage/docker
```

Éprouvé le 2026-11-25 sur Linux : le conteneur en réseau de la machine, sur un autre port (`GAMEFLEX_PORT=10300`),
*Dead or Alive 3* (Xbox) joué depuis un autre navigateur par l'adresse du réseau local, image et son.
