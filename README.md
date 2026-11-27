# GameFlex

**Self-hosted cloud gaming. Play anywhere, with anyone, on any device with a web browser.**

[![Buy me a coffee](https://img.shields.io/badge/Buy%20me%20a%20coffee-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/zinosaure)

> **GameFlex is still in development.** Expect bugs, rough edges and changes between versions,
> especially on the consoles marked *experimental*. It is a personal project, provided as is, with
> no warranty. Please read the [disclaimer](DISCLAIMER.md) before you install it.

GameFlex turns your own computer into a cloud gaming service. It runs the emulators at home and
streams the game, picture and sound, to whatever screen you have at hand: a phone, a tablet, a
laptop, a TV, a friend's computer. Your controller presses travel back the other way. There is
nothing to install to play: a web browser is enough, and an Android app is there if you prefer.

## Why GameFlex

Free and open source, with no subscription and no account with us: your games, on your server, shared with the
people you choose.

### Play together, even far apart

- **Share your games and consoles in six digits.** Pair two GameFlex with a six-digit code, or a QR code, and each
  of you sees the other's games and consoles ([federation](docs/federation.md)).
- **Play on a friend's server, from home.** Their machine runs the game and streams it to you: you play their
  consoles without owning them.
- **Talk while you play.** Voice chat in the game menu, for the players and, if the owner allows it, for the people
  watching.
- **Pass the controller.** Lend yours to a friend who is watching and take it back, or give them your place for
  good; someone watching can ask for it. Up to four controllers in the same game (eight on the Switch), each
  player on their own device: a Super Nintendo party game becomes an online game.
- **Join or watch any game.** **Live** shows the games being played right now, ready to join or watch. Send a link:
  friends watch in their browser, even without an account. Members you invite get a notification.

### Your consoles, anywhere, on any screen

- **Fifty years of consoles in one place,** from the Atari 2600 to the PlayStation 5, behind one address, with the
  same accounts and the same saves.
- **Your real PS4 or PS5, from anywhere.** Remote Play goes through your GameFlex: no port to open on your router,
  and once the console is paired, every member you allow can play it.
- **Nothing to install to play.** A web browser is enough: Windows, macOS, Linux, Android, iOS, a smart TV. The
  screen in your hands only shows a video, so a modest phone plays a PlayStation 3 game.
- **On an Android TV.** The Android app installs on the TV box: it runs the retro consoles itself, and the heavy
  ones play from your server, or a friend's, through federation. Several controllers on the same device each take
  their own place, for a couch full of players.
- **Low latency.** The picture travels by WebRTC, straight from your server to your screen; at home, it takes a few
  milliseconds. Up to 1440p and 120 frames per second, and the picture adapts when the network weakens.
- **On the TV.** Send a game to a Chromecast, from Chrome, the Android app or the PC.
- **Away from home, without opening your server to the Internet.** An optional WireGuard VPN lets your phone or
  laptop join your home network from wherever it is ([Play away from home](docs/remote-access.md)).

### You stay in control

- **Who plays what.** Choose the consoles each member can start, and what each friend's server may do with yours;
  switch a console off for everyone in one move.
- **Run all your servers from one.** With **Remote administration** on the access you give your other GameFlex — a
  second home, a mini PC, the phone — you manage its consoles, federation and Remote Play from your own Settings,
  without opening its address. One place for all of them, and nothing more to expose.
- **An account for each person.** Give a member their password, or send them an invitation link where they choose
  it. Two-step verification (an authenticator app) can be required for a member or for everyone.
- **Saves follow you, even across servers.** Your saves belong to your account, on every device. Back them up, send
  them to a friend's server and fetch them back from there.
- **Your games stay yours.** Your library is a folder on your own disk: no online catalog, nothing that disappears
  when a service closes.

### Easy to run, your way

- **An app, or a full server.** The [Android app](#the-android-app) (APK) or the Linux PC app (AppImage) run a whole
  GameFlex in one install; a server runs it with Docker Compose.
- **No Internet needed with the phone.** The phone is the server: friends nearby open its address in their browser
  and play with you. On a train or a plane, it shares its own hotspot.
- **Your games, wherever they are.** Add a disk of games in one click on the PC and the phone, and read the games of
  your Samba, WebDAV, FTP, SFTP and kDrive shares
  ([external resources](docs/federation.md#share-your-network-shares-external-resources)); more sources are coming.
- **Emulators patched for GameFlex.** The recent consoles run emulators carrying our own fixes, for more games that
  start and play. Each console has its settings and presets, which friends can share
  ([emulator settings](docs/emulator-settings.md)).
- **In your language.** The interface speaks English and French, and the consoles start in the app's language.
- **Make it yours.** Each release ships its web interface as `gameflex-interface.zip`: change it and load it back in
  the Android app.
- **The little things that matter.** Save states, rewind and fast forward, and cheat codes on the retro consoles;
  covers and game sheets fetched for you; an on-screen controller whose buttons you place yourself, and a half-screen
  mode for a controller clipped under the phone.
- **Know what plays.** Each game shows whether it plays on each emulator, from what the players report; a fix or a
  setting that makes a game work travels to your friends with it. **Report the error** writes a failed launch to
  the server's logs, for the administrator.

### Coming soon

- **Plugins**, in their own space.
- **PC ports** of games, playable from GameFlex.
- **Watching sessions and data** from the administration.

### How it compares

| | Subscription cloud gaming (Xbox Cloud Gaming, GeForce NOW) | Desktop streaming (Moonlight with Sunshine) | GameFlex |
|---|---|---|---|
| Where games run | Their data centers | Your PC | Your server, PC or phone |
| Cost | A monthly subscription | Free | Free, open source |
| Your games | Their catalog | Your PC games | Your own files, a folder on your disk |
| To play | Their app or a browser | An app on each screen | A web browser, or the app |
| Emulators | — | Each one installed and set up by hand | Built in, one interface for every console |
| Play together | Depends on the game | — | Watch, join, pass the controller, talk |
| A phone as the server | — | — | Yes, even without Internet |

| | |
|---|---|
| ![A NES game playing in the browser, with the on-screen controller](docs/images/playing-nes.png) | ![Live: your game in progress, and the games being played, ready to join or watch](docs/images/home-live.png) |
| ![A console in the Retrogaming tab](docs/images/library.jpg) | ![A game's page: its information, its status and Play](docs/images/game.jpg) |
| ![The game menu: save states, rewind, cheats, sharing](docs/images/game-menu.png) | ![Settings](docs/images/settings.png) |

<p align="center"><img src="docs/images/phone-playing.png" alt="The same NES game on a phone, with the on-screen controller" width="260"></p>

On a phone or a tablet, the same interface: the consoles, their games, and a game page ready to play.

| | |
|---|---|
| ![The Retrogaming tab on a tablet: the PlayStation 2](docs/images/tablet-ps2.jpg) | ![The Retrogaming tab on a tablet: the PlayStation Portable](docs/images/tablet-psp.jpg) |
| ![The Streamex tab on a tablet: the PlayStation 5](docs/images/tablet-ps5.jpg) | ![The Streamex tab on a tablet: the PlayStation 4](docs/images/tablet-ps4.jpg) |
| ![A PlayStation 4 game page on a tablet](docs/images/tablet-ps4-ff15.jpg) | ![A PlayStation 4 game page on a tablet, with Continue](docs/images/tablet-ps4-ff7-remake.jpg) |
| ![A PlayStation 4 game page on a tablet](docs/images/tablet-ps4-octopath.jpg) | ![A PlayStation 4 game page on a tablet](docs/images/tablet-ps4-gta5.jpg) |
| ![Live on a tablet: a Switch 2 game in progress, ready to join or watch](docs/images/tablet-live.jpg) | ![My favorites on a tablet: games from every console and every server, on all your devices](docs/images/tablet-favorites.jpg) |
| ![The Remote Play tab on a tablet: a PlayStation 5 in rest mode](docs/images/tablet-remoteplay-ps5.jpg) | |

<p align="center">
  <img src="docs/images/phone-ps5-astro-bot.jpg" alt="A PlayStation 5 game page on a phone" width="200">
  <img src="docs/images/phone-ps5-ff7-rebirth.jpg" alt="A PlayStation 5 game page on a phone" width="200">
  <img src="docs/images/phone-playing-ps5.jpg" alt="A PlayStation 5 game playing on a phone, with the on-screen controller" width="200">
  <img src="docs/images/phone-playing-ps5-ff7-remake.jpg" alt="Final Fantasy VII Remake Intergrade on the PlayStation 5 (KytyPS5), played on a phone with the on-screen controller and the compact performance view" width="200">
</p>

**Recent consoles in game, with Streamex** — on a phone held sideways, each with its own on-screen controller:

| | |
|---|---|
| ![Astro Bot on the PlayStation 5 (KytyPS5), with the PlayStation on-screen controller](docs/images/playing-ps5-astro-bot.jpg) | ![Final Fantasy VII Remake on the PlayStation 4 (shadPS4): Midgar at night](docs/images/playing-ps4-ff7-remake.jpg) |
| ![The Legend of Zelda: Ocarina of Time on the Switch 2 (Lichii): Link in the Kokiri Forest, with the Nintendo on-screen controller](docs/images/playing-switch2-ocarina.jpg) | ![EA Sports FC 27 on the Switch (Citron)](docs/images/playing-switch-fc27.jpg) |
| ![Final Fantasy XIII on the PlayStation 3 (RPCS3)](docs/images/playing-ps3-ff13.jpg) | |

**Remote Play in action** — a PlayStation 5 woken from rest mode and played from a phone, *Black Myth: Wukong*
with the on-screen controller (1 min 13 s video):

<p align="center"><a href="docs/images/remoteplay-demo.mp4"><img src="docs/images/remoteplay-demo.jpg" alt="Remote Play: Black Myth: Wukong on a PlayStation 5, played from a phone — open the video" width="540"></a></p>

**The same game, emulated** — *Black Myth: Wukong* running on the server itself in KytyPS5, the PlayStation 5
emulator, and played from a phone with the on-screen controller and its floating stick (1 min 10 s video). No
console here: the emulator is still experimental, and this shows how far it has come. It was recorded on a modest
mini PC, far below a gaming computer: an **AMD Ryzen 7 255** (8 cores, 16 threads) with its integrated **Radeon 780M**
graphics, **8 GB of VRAM** taken from its **48 GB of RAM**, and no dedicated graphics card. A more powerful computer
runs it faster.

<p align="center"><a href="docs/images/kytyps5-wukong-demo.mp4"><img src="docs/images/kytyps5-wukong-demo.jpg" alt="Black Myth: Wukong emulated on the server by KytyPS5, played from a phone — open the video" width="540"></a></p>

## What it can play

| | Consoles |
|---|---|
| **Retro** | NES, Super Nintendo, Game Boy and Game Boy Color, Game Boy Advance, Virtual Boy, Nintendo 64, Nintendo DS, GameCube, Wii, Master System, Mega Drive / Genesis, Saturn, Dreamcast, PlayStation, PlayStation 2, PSP, Neo Geo, Neo Geo Pocket, Capcom CPS-1 and CPS-2, PC Engine, PC-FX, Atari 2600, 7800, Lynx and Jaguar, WonderSwan, ColecoVision, Vectrex, Commodore 64, Amstrad CPC, ZX Spectrum, Doom |
| **Recent, with Streamex** | PlayStation 3, Xbox, and, still experimental, PlayStation 4, PlayStation 5, Nintendo Switch, Wii U and Xbox 360 |
| **Remote Play** | your own PlayStation 4 or PlayStation 5 |

The full list, with the files each console needs, is in [Consoles](docs/consoles.md).

Retro consoles run on [libretro](https://www.libretro.com) cores. Recent consoles run on their own
Linux emulators: RPCS3, xemu, shadPS4, KytyPS5, Eden, Citron, Cemu and Xenia Canary.

**Streamex** is what plays them: GameFlex's own engine for the recent consoles, the same skeleton in
every console's image. It starts the console's emulator, captures its picture and sound, encodes the
video on your graphics card and streams it all to the players' browsers. Their controllers come
back as virtual controllers that the emulator takes for its own. A new console is a new emulator in
the same skeleton.

## Good to know before you start

- **GameFlex ships no game, no BIOS and no firmware.** It plays the files you put in your library. The recent
  consoles' emulators come inside streamex packages, which GameFlex installs from **Settings › Consoles**,
  on each console's page (**Emulators**, then **Get it**).
- **A server needs Docker,** with Compose, on an x86-64 computer; a recent graphics card is recommended
  for the recent consoles ([Install GameFlex](docs/install.md)). Without Docker, the Android app or the
  Linux PC app (AppImage) run GameFlex on their own.
- **It is made for your home network.** To play away from home, turn on its WireGuard VPN, or put
  GameFlex behind an HTTPS proxy ([Play away from home](docs/remote-access.md)). Without the VPN,
  some networks block the video: GameFlex does not include a relay (TURN) server.
- **The administrator password is yours.** GameFlex refuses to start without it; the setup
  script can make one up and shows it to you once.
- **It is still in development, a personal project with no warranty.** Read the
  [disclaimer](DISCLAIMER.md).

## What has been tested

GameFlex is tested on one setup so far. What follows has been seen working there; anything else
should work, but has not been tried.

- **The server:** a Linux PC with an AMD Ryzen 7 255 and its Radeon 780M graphics, which encode
  the video (VA-API). NVIDIA and Intel graphics have not been tried, nor Windows and macOS (Docker
  Desktop).
- **The browser:** Google Chrome, on Linux and on Android.
- **Recent consoles,** games played in game on the server, with the on-screen controller or a real one:
  - **PlayStation 5:** Astro Bot, Black Myth: Wukong, Clair Obscur: Expedition 33, Final Fantasy VII
    Remake Intergrade, Kingdom Hearts III, Tekken 8, Richman 11. They run at about 15 to 30 frames per
    second at 540p, depending on the game; big games take a few minutes to start the first time (their
    shaders compile).
  - **PlayStation 4:** Final Fantasy VII Remake, Final Fantasy XV Pocket Edition, Grand Theft Auto
    V, Kingdom Hearts III, Kingdom Hearts HD 2.8 Final Chapter Prologue, Driveclub, FIFA 22, EA
    Sports FC 27 (it plays, with minor glitches), Sonic Mania Plus. FIFA 22 and FC 27 need their
    patch, which GameFlex offers.
  - **PlayStation 3:** Final Fantasy XIII. **Switch:** Mini Metro, EA Sports FC 27. **Switch 2:** The
    Legend of Zelda: Ocarina of Time, with its saves. **Wii U:** New Super Mario Bros. U (as a disc
    image and extracted). **Xbox:** Dead or Alive 3. **Xbox 360:** Halo 3.

  Not yet playable: Final Fantasy XVI (it stays on its loading screen), the Marvel games. Every game is different: see the
  known limitations in [Consoles](docs/consoles.md).
- **Remote Play:** with a PlayStation 5 (Black Myth: Wukong), from a phone. The PlayStation 4 has
  not been tried: we have none.
- **A friend's server (federation):** Final Fantasy VII Remake in 720p, played smoothly from a phone
  on mobile data.
- **The WireGuard VPN:** a retro game played through it, from a client on the same computer. Not
  yet from a phone on mobile data.
- **The Android app:** on a Samsung Galaxy S25 (Snapdragon 8 Elite, Adreno 830 graphics, 12 GB of
  RAM). The recent consoles on the phone need a Snapdragon (Adreno) graphics chip. Played on the
  phone itself:
  - **PlayStation 4:** Final Fantasy XV Pocket Edition, Grand Theft Auto V, Kingdom Hearts III,
    Sonic Mania Plus.
  - **PlayStation 5:** Neptunia ReVerse at a steady 30 frames per second, Kingdom Hearts III at
    about 15, Richman 11.

  The other games that play on the server have not been tried on the phone yet. Played from the
  phone on the server, or on a friend's server, games run like in a browser.
- **The PC (AppImage):** on Ubuntu 24.04, with the same Radeon 780M. Retro games, and the recent consoles:
  Final Fantasy XIII (PlayStation 3), Final Fantasy VII Remake (PlayStation 4), Astro Bot (PlayStation 5),
  Dead or Alive 3 (Xbox), New Super Mario Bros. U (Wii U), Super Mario Odyssey (Switch), The Legend of
  Zelda: Ocarina of Time (Switch 2), and Halo 3 (Xbox 360) up to its menu, without a controller. Not tried
  yet on a real desktop with a double-click, nor on Wayland.
- **Send to the TV (Google Cast):** not tried up to a TV yet. The Android app opens its TV picker, and the PC
  window opens the cast page in the browser.
- **Android TV:** the Android app on a friend's TV box, the heavy consoles played through federation. Several
  controllers on one device: tried with simulated controllers in a browser, not yet with real ones on a TV.
- **Not tried yet:** playing with friends over the phone's own hotspot, without Internet.

## Quick start

On a computer with Docker:

```bash
mkdir ~/gameflex && cd ~/gameflex
curl -fLO https://raw.githubusercontent.com/zinosaure/gameflex/main/launch.sh
chmod +x launch.sh
./launch.sh
```

The script asks a few questions, then starts GameFlex and tells you which address to open. The
details, and the way to do it by hand, are in [Install GameFlex](docs/install.md).

**Installing with an AI agent?** Point it to [AGENTS.md](AGENTS.md): it explains how to install,
check, troubleshoot and report an issue on your behalf, and what to ask you first.

## Guides

1. [Install GameFlex](docs/install.md): the compose file to copy, its settings, volumes and ports.
2. [Organize your library](docs/library.md): where your games, BIOS files and emulators go.
3. [Consoles](docs/consoles.md): every console, the game formats it accepts, and the files it needs.
4. [GameFlex on Android](docs/android.md): the app, playing on the phone, and the phone as a server
   for your friends.
5. [Controllers and playing together](docs/controllers.md): the on-screen controller, real
   controllers, the keyboard, and sharing a game.
6. [Play away from home](docs/remote-access.md): the WireGuard VPN, or an HTTPS proxy and the
   router, and keeping it safe.
7. [Remote Play](docs/remote-play.md): your own PS4 or PS5.
8. [Share with friends: federation](docs/federation.md): give a friend access to your server, add
   theirs, copy their games or play them on their server, step by step with screenshots; and
   your network shares (Samba, WebDAV, kDrive, FTP, SFTP) lent with your library.
9. [Emulator settings](docs/emulator-settings.md): tune the recent consoles' emulators, keep and
   share presets.
10. [Storage](docs/storage.md): more disks, network shares, saves, cache and logs.
11. [Update GameFlex](docs/update.md): new versions, and going back.
12. [Troubleshooting](docs/troubleshooting.md): find what goes wrong, and report it.

## The Android app

The GameFlex app turns an Android phone or tablet into a GameFlex of its own:

- **It plays on the phone itself:** retro consoles on most phones, and, on a powerful one, recent
  consoles too.
- **It becomes a server for the people around you.** Friends on the same Wi-Fi open the phone's
  address in their browser, and play with you or watch. Nothing to install for them.
- **It connects to your GameFlex server**, to play its games from anywhere.

Download the latest `gameflex.apk` from the
[Releases page](https://github.com/zinosaure/gameflex/releases/latest). The app checks the same
page for updates and installs them itself.

Its interface (GameFlex's pages) also updates **without a new app**: **About › Update › Interface**
checks the official source for the latest interface made for your version of the app, or installs
one of your own from a **.zip file** — built from GameFlex's open sources (`src/microservices/edge`).
**Back to the APK's interface** returns to the one the app came with.

Everything else is in [GameFlex on Android](docs/android.md).

## Source code

Most of GameFlex is open source, in this repository, under the [MIT License](LICENSE):

- **the services:** the kernel that runs the sessions (`src/microservices/kernel/`), the accounts
  service (`kernel/extras/credential/`), the retro engine (`runtimes/retrogaming/`), the front
  server and the web interface (`edge/`), and the modules they share (`shared/`);
- **the console recipes:** each recent console's Dockerfile, entry point and settings
  (`runtimes/streamex/consoles/`, `profiles/`);
- **our emulator builds:** the patches and build scripts for shadPS4, KytyPS5, Eden and Box64
  (`tools/`). Those patches are under the license of the emulator they change.

A few parts are only published as binaries, under the [GameFlex binary license](LICENSE-BINARIES.md):
the video gateway, Remote Play, streamex (the program that runs and streams the recent consoles)
and the Android app. They are free to use and to redistribute unmodified.

**Rebuild the images yourself.** Check out the tag of a version, then:

```bash
GAMEFLEX_VERSION=x.y.z docker compose -f docker-compose.build.yml build
```

The open services build from this repository; the closed binaries come from the images published
for the same version. The rebuilt images take the names that `docker-compose.yml` runs. To start
them, set `GAMEFLEX_VERSION=x.y.z` in `.env`, then run `docker compose up -d`: `./launch.sh` and
`./launch.sh update` download the published images again, over yours. The details are at the top
of [docker-compose.build.yml](docker-compose.build.yml).

**Contribute.** Pull requests on the open part are welcome: [CONTRIBUTING.md](CONTRIBUTING.md) says
how they travel to a release.

## Where to find emulators and BIOS files

GameFlex ships none of them, but you can find them here. None of these sites is part of GameFlex,
and what you download from them is your responsibility.

- **Retro cores:** the [libretro buildbot](https://buildbot.libretro.com/). GameFlex downloads the
  cores it needs from there for you, from **Settings › Consoles**.
- **BIOS and firmware:** [RetroBIOS](https://abdess.github.io/retrobios/). Use the files from its
  RetroArch pack, under the names listed in [Consoles](docs/consoles.md).
- **Emulators for the recent consoles:** GameFlex installs them for you too, from **Settings ›
  Consoles**, on each console's page: the version tested with GameFlex, from the
  [gameflex-streamex](https://github.com/zinosaure/gameflex-streamex/releases) packages. Each
  emulator's website is linked from [Consoles](docs/consoles.md).

## Thanks

GameFlex stands on the work of the emulation community. It runs these emulators, each under its own
license; none of these projects is associated with GameFlex.

- [libretro](https://github.com/libretro) and its cores, for the retro consoles: FCEUmm, Nestopia,
  Snes9x, Gambatte, mGBA, ParaLLEl N64, DeSmuME, Dolphin, Genesis Plus GX,
  PicoDrive, Yabause, Kronos, Flycast, PCSX ReARMed, PCSX2 (LRPS2), PPSSPP, Stella, ProSystem, Handy,
  Virtual Jaguar, the Beetle (Mednafen) cores, FinalBurn Alpha, Gearcoleco, vecx, VICE, Caprice32,
  Fuse and PrBoom.
- [RPCS3](https://github.com/RPCS3/rpcs3), for the PlayStation 3.
- [shadPS4](https://github.com/shadps4-emu/shadPS4), for the PlayStation 4.
- [KytyPS5](https://github.com/KytyPS5/KytyPS5), for the PlayStation 5.
- [Eden](https://eden-emu.dev) and [Citron](https://github.com/citron-neo/emulator), for the
  Nintendo Switch.
- [Cemu](https://github.com/cemu-project/Cemu), for the Wii U.
- [xemu](https://github.com/xemu-project/xemu), for the Xbox.
- [Xenia Canary](https://github.com/xenia-canary/xenia-canary), for the Xbox 360.
- [Box64](https://github.com/ptitSeb/box64), which runs the recent consoles' emulators on Android
  phones.
- [Pion](https://github.com/pion/webrtc), the WebRTC library that carries the video.

## Help and feedback

Issues are welcome: bug reports, games that don't work, ideas. Open one on the
[issues page](https://github.com/zinosaure/gameflex/issues/new/choose) and pick the form that fits:

- **Bug report:** something in GameFlex does not work as it should.
- **A game or a console does not work:** a game does not appear, does not start, or plays wrong.
- **Idea or request:** something GameFlex could do.

The bug and game forms ask for what helps us find the cause: your GameFlex version, where it happens (browser,
app or server), what you saw, and the logs (`./launch.sh logs kernel`, or **Settings › Logs**).
Screenshots help too.

Please never paste a password, your `.env` file or a token, and never share games, BIOS files or
links to them. A security problem goes through [SECURITY.md](SECURITY.md), not a public issue.

## Support GameFlex

GameFlex is a personal project, built in spare time. If you enjoy it and want to support its
development, you can buy me a coffee:

<a href="https://buymeacoffee.com/zinosaure"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy me a coffee" height="50"></a>

## License

GameFlex's source code is released under the [MIT License](LICENSE). The parts published only as
binaries (the video gateway, Remote Play, streamex and the Android app) are under the
[GameFlex binary license](LICENSE-BINARIES.md): free to use and to redistribute unmodified, not for
sale. Versions 0.3.0 to 0.8.0 were published under the MIT License alone, and remain so.

The Docker images and the Android app also contain software by others, each part under its own
license: see [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md), and the full texts in
[THIRD-PARTY-LICENSES.txt](THIRD-PARTY-LICENSES.txt). GameFlex is not affiliated with any console
maker or game publisher. See the [disclaimer](DISCLAIMER.md).
