# Changelog

All notable changes to GameFlex are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
