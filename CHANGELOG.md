# Changelog

All notable changes to GameFlex are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
