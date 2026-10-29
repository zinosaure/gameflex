# Changelog

All notable changes to GameFlex are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.0] - 2026-10-29

### Added

- **The source code of GameFlex**, in this repository, under the MIT License: the kernel, the
  accounts service, the retro engine, the front server, their shared modules, the web interface,
  the console recipes and the patches of our emulator builds, with what it takes to rebuild every
  image (see *Source code* in the README). The video gateway, Remote Play, streamex and the Android
  app stay published as binaries only, free to use and to redistribute unmodified, under the new
  [GameFlex binary license](LICENSE-BINARIES.md). Pull requests are welcome: see
  [CONTRIBUTING.md](CONTRIBUTING.md).
- **Saves by source in *My saves***, for the retro consoles and the PlayStation 3, 4 and 5, Switch,
  Wii U and Xbox: *Saves from* shows this device's saves or the ones you have on a federated
  server. **Backup** sets a game's saves aside, **Send to…** copies them to a federated server,
  where they land in its copies and replace nothing, **Download** brings a copy home, and
  **Restore** puts a copy back, one game at a time (one slot for a retro console); what it
  replaces goes into the backups first. On the server and in the Android app.
- **Write some text, in the game menu** of the recent consoles: a field that opens the phone's
  keyboard, typed into the game letter by letter, then Enter — a profile name, for instance.
- **The temperature and the battery in the Android app**: the phone's own, at the top of the
  Performance panel and in two circles next to the menu button, wherever the game runs; then,
  when it runs on a server or on another phone, that machine's.
- **The change since the game started**, in brackets in the Performance panel: the temperature,
  the battery, the processor, the memory and the video memory.
- **`GAMEFLEX_GPU`, the graphics card games run on**, on a machine that has several (a chip in the
  processor and a card added over OCuLink, for example): its PCI address. Empty, the first one.
  A game now receives that card only, and the Performance panel reads that card's load.

### Changed

- **The Android app is a release build**: 25 MB instead of 45 MB. It installs over the previous
  version.
- **The game menu opens with L2, then Home**, held for a second, instead of Select + Start: holding
  Start turns some controllers off.
- **A controller labelled like Nintendo's** (A on the right, such as the GameSir Pocket Taco) gets
  each button by its position. A mapping already saved for it stays as it was until **Auto** in
  the controller mapping settings.
- **The launch screen**: the step under a spinner in the middle, and a round cross at the top
  right that closes the game — also once a step has failed, with *Report error* in place of the
  spinner. The Android app no longer says a game takes two to three minutes to start.
- **The Performance panel spans the whole width**, in columns, over a lighter veil.
- **The Android app has no full-screen switch any more**: it is always full screen.

### Removed

- **Favourites**: the switches in the game menu and on a game's sheet, and the page that listed
  them. The favourites saved so far are deleted at the first start.
- **Synchronise with…** in *My saves*, replaced by the sources above.

### Fixed

- **No keyboard reached the recent consoles that use virtual controllers**: streamex dropped every
  key. A plugged-in keyboard, and *Write some text*, now reach the emulator.
- **The Xbox 360 signs in the account's profile at every game** when it has only one, so the game
  can save: Xenia forgot the signed-in profile, because a game ends by being stopped.
- **The Android app offered Rewind and Fast forward for a recent console**, where they did nothing.

### Security

- **What an account can send is bounded**: a save state over 512 MB is refused, error reports
  stop after 60 an hour, and wrong invitation codes are slowed down after five.
- **A token naming an unknown account is refused**, and a new account's name is limited to
  letters, digits, `.`, `_` and `-`, 3 to 32 characters: the names the kernel accepts for a game.
- **A retro game's title comes from the library**, not from the request that launches it: it
  showed in *Live now*.
- **A game's container has a process limit.**

## [0.8.0] - 2026-10-28

### Added

- **The game engine on a game's page**: Unity, Unreal, Frostbite or RAGE, recognised when the
  library is scanned, for the PlayStation 4 and 5 games.
- **Settings per game engine**: the PlayStation 4 and 5 emulator panels get *Settings per game
  engine*, a preset applied at launch to every game of that engine.
- **The sources of the emulators on the mirror**: a `src/` folder next to `cores/` says, for each
  emulator file, which upstream version it comes from and, for GameFlex's own builds, the patches
  and scripts exactly as they were when the file was built. A written offer covers the rest.
- **A script to copy the mirror at home**, `gameflex-mirror.py`: it only downloads what changed,
  resumes a cut transfer and never deletes anything.

### Changed

- **PlayStation 4 and 5 games start much sooner in the Android app**: Box64's call/return
  optimisation is now off by default. On a Galaxy S25, *Kingdom Hearts III* reaches its first
  pictures in 42 s instead of 159 s, *Final Fantasy VII Remake* in 40 s instead of 109 s, and a
  PlayStation 5 game uses about a third less processor for the same picture.
- **The Android app keeps the desktop emulators' saves under the account that plays**, in the same
  place as the server does, instead of a private folder of the app.
- **The launch line shows on every path**, and in the Android app the intro waits for the emulator
  to draw its first picture; for a PlayStation 4 or 5 game it says that loading takes two to three
  minutes.
- **My QR is at the top of the account token** in **Settings › Profile**, instead of below every
  other action, and it shows greyed out while the token is disabled.

### Fixed

- **PlayStation 4 games made with Unity no longer freeze at their first garbage collection**, on the
  server and in the Android app: *Kingdom Hearts Melody of Memory* stopped for good on its loading
  screen. shadPS4 now delivers a game's signal only where the console would.
- **PlayStation 4 games no longer stop at their first video**, on the server and in the Android
  app: shadPS4's video player kept a lock while it told the game a video had started, and the
  game's answer took it again. *Kingdom Hearts Melody of Memory* is now playable.
- **FIFA 22 no longer hangs at boot one launch out of two** on the PlayStation 4, on the server and in
  the Android app: shadPS4 lost a timer the game had just set when its service thread was going
  back to sleep, and the game waited for it forever.
- **FIFA 22 no longer stops before its menu** on the PlayStation 4: shadPS4 could not copy an image
  with several mipmap levels, and it stopped when the game's sound engine finished a batch for a
  sound it had already dropped. The game still stops after a few minutes of a match, on other
  limits of shadPS4's renderer.
- **The PlayStation 4 of the server runs the latest shadPS4 under development** instead of its last
  release, with GameFlex's fixes on top: *Grand Theft Auto V*, *Kingdom Hearts III*, *Final Fantasy
  VII Remake* and *Melody of Memory* play as before.
- **The PlayStation 4 of the Android app takes four players**, as on the server, instead of one:
  shadPS4 now reads the app's controllers as four real gamepads, with analog sticks, where it only
  had one set of keyboard keys.
- **Options, Share and the touchpad did nothing on the PlayStation 4 and 5 of the Android app**,
  from the controller on screen or a controller plugged in: Options now acts as Start, and Share
  and the touchpad press the touchpad, as on the server.
- **A game whose emulator exits by itself ends the game**, and the page says the game stopped,
  instead of showing a frozen picture. KytyPS5 no longer crashes when a game exits.
- **The Android app refuses an emulator package that is not the expected version**, instead of
  running a stale one without controllers.
- **Floating panels fit narrow screens.**

### Security

- **Everyone has to sign in again once after this update.** Tokens are now signed with Ed25519,
  and a game's container only receives the key that checks a token, never the server's secrets.
  A token pasted for a friend's server has to be pasted again once that server is updated too.
- **The desktop consoles and Remote Play only stream to the GameFlex video gateway**: their
  connection to it is sealed, and a gateway that cannot prove it is GameFlex's gets nothing.
- **A player could read, overwrite or delete another account's save states.** Fixed.
- **Importing a game from a friend's server could write outside the library** (administrators
  only). Fixed.
- **An account could launch a console that was not given to it.** Fixed.
- **A Remote Play game could be stopped by anyone who reached its container.** It now asks for a
  token, and only its owner or an administrator ends it.
- **A session opened from a friend's server can only play** here: it no longer reaches the server
  list, the shared folders or the invitations.
- **Checking a server before adding it is for administrators**, as adding it is.
- **An emulator core that the console does not declare is refused at launch.**

## [0.7.0] - 2026-10-27

### Added

- **An intro before every game**: three silent seconds with the GameFlex name and the address of
  its repository, played by the video gateway to everyone who opens a game — the player and every
  spectator, each time they open it. Live now thumbnails skip it, and so does a page that only
  reconnects.
- **Disk read in the Performance panel**: how many megabytes per second the emulator reads from
  its disk — how fast the game loads. For the desktop consoles on the server, and for every game
  on the Android app.
- **Two PlayStation 4 settings in the Android app**: *Adreno driver workaround (Turnip)*, for a
  game whose log ends with “Device lost”, and *CPU shown to the game*.

### Changed

- **The launch screen shows the intro's first picture** as its background while the game starts,
  and the steps on a single line at the bottom; a failed launch says its whole error. On the recent
  consoles, whose emulator window stays black while the game loads, the intro waits for the game's
  first real picture (five minutes at most) and the screen says *The game is loading*. The screen
  lifts on the game's first picture, not the intro's.
- **A PlayStation 4 game starts sooner in the Android app**: the processor shown to shadPS4 is now
  an AMD one, like the console's, and shadPS4 no longer scans the whole game at every start for
  instructions it has nothing to change on it. *Final Fantasy VII Remake* on a Galaxy S25 reaches
  its first videos in 2 min 06 instead of about 3 min 45.
- **The Android app spends less processor time on a slow game**: its screen capture no longer
  converts a picture that has not changed since the previous one.
- **PlayStation 4 games need far less processor in the Android app**: Box64 no longer checks the
  game's code each time it runs it. That check took 62 % of the processor time of *Final Fantasy
  VII Remake* on a Galaxy S25. The setting is *Game code in shared memory*, on by default.

### Fixed

- **Final Fantasy VII Remake** died at its start in the Android app: Box64 lost the x86 flags
  around an empty `rep cmps`, and the game's own `memcmp` compared wrong. Box64 now keeps the flags
  safest by default for the PlayStation 4 (*Flags safety*, 2).

## [0.6.1] - 2026-10-26

### Changed

- **Retro games are *Playable* by default**: a console of libretro cores that no administrator
  has set reads *Playable*; the recent consoles stay *Untested*. **Settings › Games › Status of a
  whole console** shows the default that applies.
- **Emulators updated**: shadPS4 0.20.0, KytyPS5 2026-10-26 (`597137e`), RPCS3 0.0.43-20404,
  xemu 0.8.139 and Xenia Canary `291f0c9`, on the server and in the Android app. Download them
  from **Settings › Emulators**; the previous files stay and can be removed there.
- **KytyPS5 keeps each game's shaders**: they are compiled again when the game starts, before its
  first frame, instead of the first time a scene appears. **Shader cache** in its settings, on by
  default.

### Fixed

- RPCS3 0.0.43-20404 crashed before every game: its Qt 6.12 looked for PipeWire, which GameFlex
  does not run. Qt's sound now goes through PulseAudio.

## [0.6.0] - 2026-10-26

### Added

- `GAMEFLEX_STAGING`: a test server writes *staging* next to its name and in the browser tab, and
  its app download gives the APK it builds itself.
- **A status for every game**: *Playable*, *Crashes in game*, *Not playable* or *Untested*, shown
  next to its name, with a filter on the **Games** page and the notes in its panel. Administrators
  set it in **Settings › Games**, for one game (per emulator, with a note) or for a whole console
  at once. Friends see your statuses through federation.
  The Android app has the same, for its own library.
- **Invitations instead of tokens**: **Generate a federation token** and **Settings › Profile ›
  My QR** now give an invitation, a link and a QR code that work once, within 12 hours. **Add a
  server** and **Play at…** accept it, pasted or read from the image of the QR; your server
  exchanges it with your friend's for the token, which no longer goes through a messaging app.
  Pending invitations are listed and can be revoked. The link and the token still work, as before.
- **A 540p stream for weak connections** on the recent consoles (PS3 to PS5, Switch, Wii U, Xbox,
  Xbox 360): when a screen's network weakens, it moves to a 540p stream at 30 frames per second,
  and back once the network is stable. The game encodes it only while someone watches it. The game
  menu's new **Video** category chooses **Automatic**, **Current stream** or **Low quality**; the
  **Live now** thumbnails always use it.
- **Join a friend's GameFlex from the Android app**: the home screen's **Join on the network** now
  has two tabs. **Share** shows your QR code and link, as before; **Join** scans another GameFlex's
  QR code (Google's reader) or takes its link, checks that a GameFlex answers there, and opens it
  in the app. **Settings › Back to the app's home screen**, or the Back button, returns to your home
  screen; the servers you joined stay listed.

### Changed

- **Settings › Download the GameFlex app** now appears on every server, and leads to the GitHub
  Release unless `GAMEFLEX_STAGING` is set.
- The Android app's page footer gives its version and build date once, instead of twice.
- On a phone server, the share and download-through-the-browser buttons are offered only in the
  phone's own app, no longer to the people who join it.

### Fixed

- After a lost packet, the picture no longer stays damaged until the next keyframe (up to two
  seconds on the recent consoles and the Android app): the browser's request for a fresh keyframe
  now reaches the game, which sends one at once.

### Removed

- The **360p** resolution of the stream settings: 540p is now the smallest. A saved 360p choice
  falls back to the default.

## [0.5.0] - 2026-10-25

### Added

- The page footer links to the GitHub repository and to Buy Me a Coffee.
- **Live now** shows how long each game has been running.
- An administrator can end somebody else's game from **Live now**, after two confirmations.
- The federation guide shows every step with screenshots, and how to play on a friend's server.

### Fixed

- Remote Play: starting a stream after opening the console list stopped the session container
  (the server answered 502). Introduced in 0.4.3.
- Two players in the same game no longer crash the video gateway: their inputs were written to the
  game at the same time, and every crash cut the picture for everyone watching.
- A game no longer stays without a picture for good when the video gateway restarts: a failed
  reconnection is now retried.
- **Settings › Federations › Add a server** no longer says that a friend's game is fetched to your
  server to be played: it runs on theirs.

### Changed

- The Android app's server listens on port **10200**, like a GameFlex server, instead of 18400. The
  addresses, QR codes and federation tokens a phone gave before must be given again.
- The menu says **Retro gaming** instead of *Embedded consoles*, translated like the rest.

## [0.4.3] - 2026-10-25

### Fixed

- Remote Play: after **Turn on**, a console stayed on *Standby* until you left the page and came
  back. A console lent by a federated server never updated; one of your own stopped updating once
  the page's connection had dropped (a proxy closes a quiet connection after a minute).

## [0.4.2] - 2026-10-25

Version 0.4.1 was skipped: its changes come with this one.

### Fixed

- `./launch.sh update` only downloaded the new images: an install never received the new
  services and settings of `docker-compose.yml`. It now also downloads the `docker-compose.yml` and
  `launch.sh` of the version in `.env`. The disks you added (the `/mnt/<name>` lines) move into the
  new compose file, and the previous one stays in `docker-compose.yml.old`. **Installed 0.4.0?**
  Download `launch.sh` once more before updating:
  `curl -fLO https://raw.githubusercontent.com/zinosaure/gameflex/main/launch.sh`
- The Android app showed an old version (0.2.0) at the bottom of its pages: its built-in
  interface took the version from where it was built, not from the release.

## [0.4.0] - 2026-10-24

### Added

- An optional WireGuard VPN ([wg-easy](https://github.com/wg-easy/wg-easy)) in
  `docker-compose.yml`, to play away from home without putting GameFlex on the Internet: one UDP
  port on the router, and only the devices you add get in. `./launch.sh wireguard` turns it on;
  [Play away from home](docs/remote-access.md) explains the steps.

### Security

- The passwords and tokens of network shares (Samba, WebDAV, kDrive) are stored encrypted with
  `GAMEFLEX_SECRET`; they were stored in plain text. Existing ones are encrypted at the first start.

### Changed

- The administrator password in `.env` only creates the administrator account, at the first start.
  A password changed in GameFlex is no longer set back at the next restart;
  `./launch.sh reset-password` sets it back on purpose, if you lost it.
- GameFlex asks for Docker only: the README no longer says it needs Linux. It has only been tried
  on Linux so far.

### Fixed

- `launch.sh` downloads `docker-compose.yml` when `.env` already exists, instead of failing
  with "no configuration file provided".
- `./launch.sh setup` offers the consoles already chosen instead of dropping them.

## [0.3.0] - 2026-10-24

### Added

- First public release.
