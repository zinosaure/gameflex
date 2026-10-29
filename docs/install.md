# Install GameFlex

**Goal:** run the GameFlex server on your computer, and sign in from a browser.

**Before you start:**

- **A computer, x86-64**, that stays on while you play. It does the emulation, so the more recent
  the consoles you want, the more powerful it must be.
- **Docker with Compose v2**: it is all there is to install. Check with `docker compose version`.
  If it is missing, follow [Install Docker Engine](https://docs.docker.com/engine/install/).
  GameFlex has only been tried on Linux. On Windows and macOS, Docker Desktop runs the containers
  in a virtual machine; whether the graphics card, the virtual controllers and the video reach
  them there has not been tried.
- **A graphics card that Docker can hand to the containers, in `/dev/dri`** (AMD or Intel
  recommended). GameFlex uses it to encode the video.
- **For the recent consoles, `/dev/uinput`**, which gives them virtual controllers. Load it with
  `sudo modprobe uinput`, and keep it loaded after a restart with
  `echo uinput | sudo tee /etc/modules-load.d/uinput.conf`.
- **Disk space:** about 5 GB for GameFlex, about 1.3 GB more per recent console (plus its emulator,
  around 100 MB, in your library), and your games.

## The quick way: `launch.sh`

```bash
mkdir ~/gameflex && cd ~/gameflex
curl -fLO https://raw.githubusercontent.com/zinosaure/gameflex/main/launch.sh
chmod +x launch.sh
./launch.sh
```

The script checks your computer, then asks four questions: your library folder, the address of
this computer (it suggests one), an administrator password (or makes one up and shows it once), and the recent
consoles you want. It keeps your answers in `.env`, downloads GameFlex and starts it. At the end it
gives you the address to open.

Afterwards, the same script runs GameFlex for you:

| Command | What it does |
|---|---|
| `./launch.sh` | Starts GameFlex. |
| `./launch.sh consoles ps3 switch` | Adds recent consoles and downloads their images. |
| `./launch.sh update` | Downloads the latest version, with its `docker-compose.yml` (your disks kept) and `launch.sh`, and restarts (see [Update GameFlex](update.md)). |
| `./launch.sh stop` | Stops GameFlex. Your library and settings stay. |
| `./launch.sh status` | Shows what is running. |
| `./launch.sh logs kernel` | Follows the log of a service: `edge`, `kernel`, `credential`, `gateway`, `remoteplay` or `wireguard`. |
| `./launch.sh wireguard` | Turns on the optional WireGuard VPN, to play away from home (see [Play away from home](remote-access.md)). |
| `./launch.sh setup` | Asks the questions again. |

## Step by step, without the script

### 1. Make a folder for GameFlex

```bash
mkdir ~/gameflex && cd ~/gameflex
```

### 2. Get the two files

```bash
curl -fLO https://raw.githubusercontent.com/zinosaure/gameflex/main/docker-compose.yml
curl -fL -o .env https://raw.githubusercontent.com/zinosaure/gameflex/main/.env.example
```

Or copy [the compose file](#the-compose-file) and [the settings file](#the-settings-file) from
the end of this page.

### 3. Fill in `.env`

Open `.env` in a text editor and fill in the four required values:

| Setting | What to put |
|---|---|
| `GAMEFLEX_LIBRARY` | The folder for your games and saves, as a full path, such as `/srv/gameflex`. Create it if needed: `sudo mkdir -p /srv/gameflex`. |
| `GAMEFLEX_PUBLIC_HOST` | The address of this computer on your home network. `ip -4 route get 1.1.1.1` shows it after `src`, such as `192.168.1.50`. |
| `GAMEFLEX_ROOT_PASSWORD` | The password of the administrator account, `root`. |
| `GAMEFLEX_SECRET` | A random secret. Make one with `openssl rand -hex 32` and paste it. |

### 4. Start GameFlex

```bash
docker compose up -d
```

The first time, Docker downloads the images: a few minutes.

### 5. Add the recent consoles you want

Each recent console has its own image, downloaded only if you ask for it. List the ones you want in
`.env`, separated by commas, then download them. For example, for the PlayStation 3 and the Switch:

```bash
COMPOSE_PROFILES=ps3,switch
```

```bash
docker compose pull
```

The names are `ps3`, `ps4`, `ps5`, `switch`, `wiiu`, `xbox` and `xbox360`. The images hold no
emulator: once signed in, download each console's emulator from **Settings › Emulators** (see
[Organize your library](library.md#3-get-the-emulators)). Retro consoles need no extra image; their cores also come from **Settings › Emulators**.

### 6. Sign in

Open `http://<GAMEFLEX_PUBLIC_HOST>:10200` in a browser, for example `http://192.168.1.50:10200`,
and sign in as `root` with your password.

## What you should see

- `docker compose ps` lists `gameflex-edge`, `gameflex-kernel`, `gameflex-credential`,
  `gameflex-gateway`, `gameflex-remoteplay` and `gameflex-retrogaming` as running.
  (`gameflex-retrogaming` only keeps its image at hand: after a reboot it stays stopped, and that is
  fine.)
- The sign-in page opens, and then the GameFlex home page.
- Your library folder now holds `roms/`, `bios/`, `cores/`, `saves/` and the other folders.

Next: [organize your library](library.md).

## If something goes wrong

| What you see | What to do |
|---|---|
| `required variable … is missing a value` | One of the four required settings is empty in `.env`. |
| A service keeps restarting | Read its log: `docker compose logs gameflex-kernel` (or `gameflex-edge`, `gameflex-credential`, `gameflex-gateway`, `gameflex-remoteplay`). |
| The page does not open from another device | A firewall blocks port 10200 on this computer. |
| The game page stays black, or says it cannot connect | The video goes through UDP ports 9201 to 9300: let them through the firewall. If this computer has several network cards, set `GAMEFLEX_ICE_INTERFACE`. |
| A console says it is not installed | Download its image: `./launch.sh consoles <name>`, or step 5. |
| A recent console has no controller | `/dev/uinput` is missing: load the `uinput` module (see *Before you start*). |

## Reference

### Settings

All settings live in `.env`.

| Setting | Required | Default | What it does |
|---|---|---|---|
| `GAMEFLEX_LIBRARY` | yes | | Your library folder on this computer. |
| `GAMEFLEX_PUBLIC_HOST` | yes | | The address of this computer on your home network. Players and the services reach GameFlex through it. |
| `GAMEFLEX_ROOT_PASSWORD` | yes | | The administrator's password. GameFlex does not start without it (`launch.sh` can make one up). It only creates the administrator account, at the first start: change the password afterwards in GameFlex (**Settings › Profile**). Lost it? `./launch.sh reset-password` sets it back to this one. |
| `GAMEFLEX_SECRET` | yes | | The secret the services share to sign sessions; it also encrypts the passwords of network shares. Changing it signs everyone out, and those passwords must be entered again. |
| `GAMEFLEX_VERSION` | no | `latest` | The version of the images, such as `0.3.0`. |
| `GAMEFLEX_PORT` | no | `10200` | The port of the web page. |
| `GAMEFLEX_ICE_INTERFACE` | no | every physical interface | The network interfaces the video may use, such as `eth0`, separated by commas. Empty, Docker bridges and VPN tunnels (`wg0`, `tun0`) are skipped. |
| `GAMEFLEX_ROOT_USERNAME` | no | `root` | The administrator's user name. |
| `GAMEFLEX_MAX_PLAYERS` | no | `4` | Virtual controllers created for a recent console's game, never fewer than the console's own player count. How many play is set per console (see [Consoles](consoles.md)). |
| `GAMEFLEX_CONSOLE_LIMITS` | no | one of each | How many games of each recent console can run at once, as `console:number` pairs. |
| `GAMEFLEX_GPU` | no | the first one | The graphics card games run on, when the machine has several (a chip in the processor and a card added over OCuLink, for example): its PCI address, such as `0000:03:00.0`. `ls /dev/dri/by-path` lists them, and the kernel log says at start-up which one games use. |
| `GAMEFLEX_MIRROR_URL` | no | | A mirror for emulators and BIOS files. When set, the settings screens show a link to it. It must be a full `http://` or `https://` address, or GameFlex does not start. |
| `GAMEFLEX_STAGING` | no | | `1` marks a test server: GameFlex writes *staging* next to its name, and **Settings › Download the GameFlex app** gives the APK this server builds (in `/downloads`) instead of the GitHub Release. Leave empty for a normal server. |
| `COMPOSE_PROFILES` | no | none | The recent consoles whose images you want, separated by commas: `ps3`, `ps4`, `ps5`, `switch`, `wiiu`, `xbox`, `xbox360`. Add `wireguard` for the optional VPN (see [Play away from home](remote-access.md)). |
| `GAMEFLEX_NAME` | no | `gameflex` | The name of the containers (`gameflex-kernel`…). Change it only to run a second GameFlex on the same computer, which then also needs other ports: `GAMEFLEX_PORT`, and the fixed ports in `docker-compose.yml`. |

### Volumes

| On this computer | In the container | What it holds |
|---|---|---|
| `GAMEFLEX_LIBRARY` | `/gameflex` (gameflex-kernel) | Your library: games, BIOS files, emulators, saves, logs, cache. |
| `GAMEFLEX_LIBRARY/databases` | `/data` (gameflex-credential, gameflex-remoteplay) | Accounts, settings and paired consoles. |
| `GAMEFLEX_LIBRARY/cache/profiles` | `/profiles` (gameflex-kernel) | Controller layouts, written by GameFlex when it starts. |
| `/var/run/docker.sock` | same (gameflex-kernel) | Lets GameFlex start a container for each game. |
| any other disk | `/mnt/<name>` (gameflex-kernel) | More games: add one line per disk to the volumes of `gameflex-kernel`. |
| the Docker volume `gameflex_wireguard` | `/etc/wireguard` (gameflex-wireguard) | The optional VPN's keys and devices, once you turn it on. |

**Back up `GAMEFLEX_LIBRARY`**: it holds everything that is yours, saves and accounts included.

**About the Docker socket:** the kernel needs it to start games, and this gives it full control of
Docker on this computer. Run GameFlex on a machine you trust, and don't expose port 10201.

### Ports

| Port | Protocol | Used for | Open it to |
|---|---|---|---|
| `10200` | TCP | the web page | your network (or your HTTPS proxy) |
| `9201`–`9300` | UDP | the game video | your network; the Internet only to play away from home without the VPN |
| `10201` | TCP | the server, for the game containers | nobody: this computer only |
| `10202`, `10205` | TCP | the video gateway and Remote Play | nobody on the Internet: never forward them on your router. If you firewall this computer, let the web container reach them |
| `9001`–`9100` | TCP | the running games | nobody on the Internet: never forward them on your router |
| `51820` | UDP | the optional WireGuard VPN | the Internet: forward it on your router, once you turn the VPN on |
| `51821` | TCP | the VPN's settings page | your network only: never forward it on your router |

### The compose file

```yaml
# GameFlex: self-hosted cloud gaming.
#
# Put this file and `.env` in the same folder, fill in `.env` (start from
# `.env.example`), then run:
#
#   docker compose up -d
#
# The guide: docs/install.md.

name: ${GAMEFLEX_NAME:-gameflex}

services:
    # The web page, and the door to every other service. Open
    # http://<GAMEFLEX_PUBLIC_HOST>:<GAMEFLEX_PORT> in a browser.
    gameflex-edge:
        image: zinosaure/gameflex-edge:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-edge
        restart: unless-stopped
        ports:
            - "${GAMEFLEX_PORT:-10200}:8080"
        environment:
            - KERNEL_BACKEND_PROXY_TARGET=http://gameflex-kernel:8080
            - CREDENTIAL_BACKEND_PROXY_TARGET=http://gameflex-credential:8080
            - GATEWAY_WEBRTC_PROXY_TARGET=http://${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}:10202
            - REMOTEPLAY_BACKEND_PROXY_TARGET=http://${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}:10205

    # The server: your library, the games, and the containers that run them.
    gameflex-kernel:
        image: zinosaure/gameflex-kernel:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-kernel
        restart: unless-stopped
        ports:
            # This computer only: the game containers reach it here.
            - "127.0.0.1:10201:8080"
        environment:
            - GAMEFLEX_JWT_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_INTERNAL_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_PUBLIC_HOST=${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}
            # Its games carry this name: two GameFlex on one machine keep theirs apart.
            - GAMEFLEX_STACK_NAME=${GAMEFLEX_NAME:-gameflex}
            # The same folders as the volumes below, seen from this machine.
            - GAMEFLEX_STORAGE_PATH=${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}
            - GAMEFLEX_PROFILES_HOST_DIR=${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}/cache/profiles
            - GAMEFLEX_CREDENTIAL_URL=http://gameflex-credential:8080
            - GAMEFLEX_KERNEL_URL=http://127.0.0.1:10201
            - GAMEFLEX_GATEWAY_URL=http://127.0.0.1:10202
            # Where the kernel itself reaches the video gateway (host network):
            # it opens a launch's stream there, the intro the game page shows
            # while the game starts.
            - GAMEFLEX_GATEWAY_CONTROL_URL=http://${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}:10202
            - GAMEFLEX_REMOTEPLAY_CONTROL_URL=http://${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}:10205
            - GAMEFLEX_PORT_RANGE=9001-9100
            - GAMEFLEX_MAX_PLAYERS=${GAMEFLEX_MAX_PLAYERS:-4}
            - GAMEFLEX_CONSOLE_LIMITS=${GAMEFLEX_CONSOLE_LIMITS:-switch:1,ps3:1,xbox:1,ps4:1,ps5:1,wiiu:1,xbox360:1}
            - GAMEFLEX_MIRROR_URL=${GAMEFLEX_MIRROR_URL:-}
            - GAMEFLEX_GPU=${GAMEFLEX_GPU:-}
            - GAMEFLEX_STAGING=${GAMEFLEX_STAGING:-}
            - GAMEFLEX_RETRO_IMAGE=zinosaure/gameflex-retrogaming:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_REMOTEPLAY_IMAGE=zinosaure/gameflex-remoteplay:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_PS3_IMAGE=zinosaure/gameflex-ps3:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_PS4_IMAGE=zinosaure/gameflex-ps4:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_PS5_IMAGE=zinosaure/gameflex-ps5:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_SWITCH_IMAGE=zinosaure/gameflex-switch:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_SWITCH_EDEN_IMAGE=zinosaure/gameflex-switch-eden:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_WIIU_IMAGE=zinosaure/gameflex-wiiu:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_XBOX_IMAGE=zinosaure/gameflex-xbox:${GAMEFLEX_VERSION:-latest}
            - GAMEFLEX_XBOX360_IMAGE=zinosaure/gameflex-xbox360:${GAMEFLEX_VERSION:-latest}
        volumes:
            # GameFlex starts the games in their own containers.
            - /var/run/docker.sock:/var/run/docker.sock
            - ${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}:/gameflex
            - ${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}/cache/profiles:/profiles
            # More disks with games: one line each, <folder on this machine>:/mnt/<name>.
            # - /media/games:/mnt/games

    # Accounts and sign-in.
    gameflex-credential:
        image: zinosaure/gameflex-credential:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-credential
        restart: unless-stopped
        environment:
            - GAMEFLEX_JWT_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_INTERNAL_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_ROOT_USERNAME=${GAMEFLEX_ROOT_USERNAME:-root}
            - GAMEFLEX_ROOT_PASSWORD=${GAMEFLEX_ROOT_PASSWORD:?set GAMEFLEX_ROOT_PASSWORD in .env}
        volumes:
            - ${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}/databases:/data

    # The video: carries the picture and sound of every game to the players.
    gameflex-gateway:
        image: zinosaure/gameflex-gateway-webrtc:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-gateway
        restart: unless-stopped
        network_mode: host
        environment:
            - HTTP_PORT=10202
            - GAMEFLEX_INTERNAL_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_ICE_INTERFACE=${GAMEFLEX_ICE_INTERFACE:-}
            - GATEWAY_ICESERVERS_URL=http://127.0.0.1:10205/api/v1/remoteplay/streaming/webrtc/config
            - ICE_PORT_MIN=9201
            - ICE_PORT_MAX=9300

    # Remote Play: your own PlayStation 4 or 5.
    gameflex-remoteplay:
        image: zinosaure/gameflex-remoteplay:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-remoteplay
        restart: unless-stopped
        network_mode: host
        environment:
            - HTTP_PORT=10205
            - GAMEFLEX_KERNEL_URL=http://127.0.0.1:10201
            - GAMEFLEX_INTERNAL_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_JWT_SECRET=${GAMEFLEX_SECRET:?set GAMEFLEX_SECRET in .env}
            - GAMEFLEX_PUBLIC_HOST=${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}
        volumes:
            - ${GAMEFLEX_LIBRARY:?set GAMEFLEX_LIBRARY in .env}/databases:/data
        healthcheck:
            test: [ "CMD-SHELL", "wget -qO- http://localhost:10205/api/v1/playstation/health || exit 1" ]
            interval: 30s
            timeout: 5s
            retries: 3
            start_period: 30s

    # Retro consoles. Kept here so that its image is downloaded with the others;
    # GameFlex starts one of these for every retro game.
    gameflex-retrogaming:
        image: zinosaure/gameflex-retrogaming:${GAMEFLEX_VERSION:-latest}
        container_name: ${GAMEFLEX_NAME:-gameflex}-retrogaming
        restart: "no"

    # Recent consoles: one image each, downloaded only when you ask for it,
    # for example `docker compose --profile ps3 pull`. These never run on their
    # own: GameFlex starts them for each game. They hold no emulator: download
    # it from Settings › Emulators, into cores/<console>/.
    gameflex-ps3:
        image: zinosaure/gameflex-ps3:${GAMEFLEX_VERSION:-latest}
        profiles: [ "ps3" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-ps4:
        image: zinosaure/gameflex-ps4:${GAMEFLEX_VERSION:-latest}
        profiles: [ "ps4" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-ps5:
        image: zinosaure/gameflex-ps5:${GAMEFLEX_VERSION:-latest}
        profiles: [ "ps5" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-switch:
        image: zinosaure/gameflex-switch:${GAMEFLEX_VERSION:-latest}
        profiles: [ "switch" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-switch-eden:
        image: zinosaure/gameflex-switch-eden:${GAMEFLEX_VERSION:-latest}
        profiles: [ "switch" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-wiiu:
        image: zinosaure/gameflex-wiiu:${GAMEFLEX_VERSION:-latest}
        profiles: [ "wiiu" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-xbox:
        image: zinosaure/gameflex-xbox:${GAMEFLEX_VERSION:-latest}
        profiles: [ "xbox" ]
        entrypoint: [ "true" ]
        restart: "no"
    gameflex-xbox360:
        image: zinosaure/gameflex-xbox360:${GAMEFLEX_VERSION:-latest}
        profiles: [ "xbox360" ]
        entrypoint: [ "true" ]
        restart: "no"

    # Optional: a WireGuard VPN (wg-easy), to play away from home without
    # putting GameFlex on the Internet. Turn it on with `wireguard` in
    # COMPOSE_PROFILES, then open its page at http://<GAMEFLEX_PUBLIC_HOST>:51821.
    # The guide: docs/remote-access.md. Forward only UDP 51820 on your router,
    # never 51821.
    gameflex-wireguard:
        image: ghcr.io/wg-easy/wg-easy:15
        container_name: ${GAMEFLEX_NAME:-gameflex}-wireguard
        profiles: [ "wireguard" ]
        restart: unless-stopped
        environment:
            # Its page is for your home network, without HTTPS.
            - INSECURE=true
            - DISABLE_IPV6=true
            # The devices you connect send only what goes to GameFlex through
            # your home; the rest of their traffic stays on their own network.
            - INIT_ENABLED=true
            - INIT_ALLOWED_IPS=${GAMEFLEX_PUBLIC_HOST:?set GAMEFLEX_PUBLIC_HOST in .env}/32
        volumes:
            - wireguard:/etc/wireguard
            - /lib/modules:/lib/modules:ro
        ports:
            - "51820:51820/udp"
            - "51821:51821/tcp"
        cap_add:
            - NET_ADMIN
            - SYS_MODULE
        sysctls:
            - net.ipv4.ip_forward=1
            - net.ipv4.conf.all.src_valid_mark=1
        # The image routes with the old iptables (legacy), which recent Linux
        # systems no longer load: there it switches to nftables, which Docker
        # itself uses.
        entrypoint:
            - sh
            - -c
            - |
              iptables-legacy -t nat -S >/dev/null 2>&1 || {
                  ln -sf iptables-nft /usr/sbin/iptables
                  ln -sf iptables-nft-save /usr/sbin/iptables-save
                  ln -sf iptables-nft-restore /usr/sbin/iptables-restore
              }
              exec docker-entrypoint.sh /usr/bin/dumb-init node server/index.mjs

volumes:
    # The VPN's keys and devices.
    wireguard:
```

### The settings file

```bash
# GameFlex settings. Copy this file to `.env`, next to docker-compose.yml, and
# fill in the four required values. Every setting is explained in
# docs/install.md.

# ── Required ─────────────────────────────────────────────────────────

# Your library folder: games, BIOS files, emulators, saves. GameFlex creates
# what it needs inside. Use a full path.
GAMEFLEX_LIBRARY=/srv/gameflex

# The address of this computer on your home network, for example 192.168.1.50.
# Find it with: ip -4 route get 1.1.1.1
GAMEFLEX_PUBLIC_HOST=

# The password of the administrator account (user name: root). Choose a strong
# one: GameFlex refuses to start without it. Put it between single quotes if it
# holds spaces or signs such as $ or #.
GAMEFLEX_ROOT_PASSWORD=

# A long random secret that the GameFlex services share. Make one with:
#   openssl rand -hex 32
# Keep it: changing it signs everyone out.
GAMEFLEX_SECRET=

# ── Optional ─────────────────────────────────────────────────────────

# The GameFlex version to run, such as 0.3.0. "latest" follows new releases.
GAMEFLEX_VERSION=latest

# The port of the web page.
GAMEFLEX_PORT=10200

# The network interfaces the video may use, such as eth0 or enp5s0, separated by
# commas. Empty: every physical interface (Docker bridges and VPN tunnels such as
# wg0 are skipped). Set it if this computer has several networks.
GAMEFLEX_ICE_INTERFACE=

# The administrator's user name.
GAMEFLEX_ROOT_USERNAME=root

# Virtual controllers created for a recent console's game (never fewer than the
# console's own player count). How many play is set per console.
GAMEFLEX_MAX_PLAYERS=4

# How many games of each recent console can run at the same time.
GAMEFLEX_CONSOLE_LIMITS=switch:1,ps3:1,xbox:1,ps4:1,ps5:1,wiiu:1,xbox360:1

# A mirror where you download emulators and BIOS files yourself. When set,
# GameFlex shows a link to it in its settings.
GAMEFLEX_MIRROR_URL=

# The graphics card games run on, when the machine has several: its PCI
# address, such as 0000:03:00.0 (ls /dev/dri/by-path lists them). Empty: the
# first one.
GAMEFLEX_GPU=

# A test server: GameFlex writes "staging" next to its name, and its app
# download gives the APK this server builds instead of the GitHub Release.
# Leave empty for a normal server.
GAMEFLEX_STAGING=

# The recent consoles whose images you want, separated by commas: ps3, ps4,
# ps5, switch, wiiu, xbox, xbox360. Add wireguard for the optional VPN that lets
# you play away from home (docs/remote-access.md). Docker Compose reads this
# setting itself.
COMPOSE_PROFILES=

# The name of this installation's containers (gameflex-kernel…). Change it only
# if this computer already runs another GameFlex under that name; it then also
# needs other ports: GAMEFLEX_PORT, and the fixed ports in docker-compose.yml.
GAMEFLEX_NAME=gameflex
```
