# Changelog

All notable changes to GameFlex are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.15.0] - 2026-11-19

### Added

- **Switch a console off for everyone**: **Settings › Consoles**, a console, **Console enabled**. Turned off, nobody
  launches it on your server, members and friends alike, for instance while your computer needs its graphics card.
- **Choose the consoles of a delivered access**: **Settings › Federation › Delivered accesses**, an access, **Console
  access** — the same switches as a member's (all the retro consoles, each recent console, each Remote Play console).
  A friend's server only launches what its access includes, on top of its players' own accounts.
- **A switched-off console says so**: its games show **Console disabled** instead of **Play**.
- **Take the controller back on another device**: opening your game on your phone while your computer holds the
  controller asks the computer to release it; without an answer within 15 seconds, it stays where it is.
- **Discovery mode leads to the forms**: while it is on, **Create a new access** and **Pairing code** show right under
  it.

### Changed

- **Settings › Profile › Federation › See my connection token** (it was **Create my token**, although the token exists
  already).
- **A refused request for the controller is said to the one who asked**, who was not told before.

### Removed

- **Also share the consoles**, the switch of a delivered access that only showed the Remote Play consoles: they show
  with the games now, and **Console access** decides what can be launched.

### Fixed

- **gameflex-federation no longer reports itself unhealthy**: its health check looked at the wrong port.

## [0.14.2] - 2026-11-18

### Fixed

- **The documentation follows the interface again**: the recent consoles' emulators are installed from **Settings ›
  Consoles**, on each console's page (there is no **Settings › Emulators** anymore); `GAMEFLEX_SECRET` encrypts the
  metadata providers' keys, no longer network shares; gameflex-federation's games show only in the catalog.

## [0.14.1] - 2026-11-18

### Fixed

- **The list of tested games in the README**: Grand Theft Auto V, Kingdom Hearts III and Final Fantasy XV Pocket
  Edition play on PlayStation 4, and FIFA 22 is not playable yet; the games tried on the phone itself; the recent
  consoles' emulators come from the gameflex-streamex packages.

## [0.14.0] - 2026-11-18

### Added

- **The Android app updates its interface without a new app**: **About › Update › Interface** installs the latest
  interface made for your version of the app (from the `edge-versions` release), or one of your own from a .zip file,
  and goes back to the app's own interface.
- **gameflex-federation makes the sheets of its games**: **More › Metadata providers** takes your IGDB and SteamGridDB
  accounts. A game on a disk that already has its sheet keeps it; the games of a network share, and those of a disk
  without a sheet, get one made by the service.
- **Who uses your token**: **Settings › Profile › Federation › Connections with my token** lists each server or device
  that opened a session with your account token: the name it gives itself, the account that pasted the token there,
  whether it is a GameFlex server or the app, the address it came from (**From your current network** when it is
  yours), and the first and last time. **Revoke the connections** regenerates the token, to cut someone who should not have it.

### Changed

- **gameflex-federation has the same Federation pages as GameFlex**: **Delivered accesses** (invitations with their QR
  code, tokens, pairing codes in discovery mode, each access with when it was last used, and revoking it), with its
  disks and shares under **Internal resources**. A pairing code can be cancelled, and its window says who used it.
  **Also share my metadata providers** lends its IGDB and SteamGridDB accounts to an access, as in GameFlex.
- **The recent consoles' emulators come in streamex packages**: **Settings › Consoles** installs the console's
  package from the [gameflex-streamex](https://github.com/zinosaure/gameflex-streamex) releases (the version tested
  with GameFlex, and for shadPS4 and KytyPS5 our build of it), after checking its fingerprint, instead of downloading
  from each emulator's publisher. A publisher who removes a version or moves no longer breaks the download.
- **A game's patch has an official source**: on its page, **Get it** offers GameFlex's streamex packages first, before
  internal disks and federated servers, for the games whose settings we have tuned.
- **Our builds of shadPS4 and KytyPS5 are patches on the official repositories**: `tools/shadps4/patches/` and
  `tools/kyty/patches/` (`patches-phone/` for the phone), applied by `build.sh` to the official source at a pinned commit.
- **The Android app learns its emulators' versions from the same index**: it reads the streamex packages' index when it
  starts and before installing an emulator, installs the version listed there after checking its fingerprint, and keeps
  the last one it knew when offline. A new emulator package no longer needs a new app.

### Fixed

- **The games of gameflex-federation only appear in the catalog**: the service only lends its games and their files
  to download, so its games no longer show as playable in the console menus or the home screen, and its patches,
  settings, BIOS, emulators and sheets stay in the catalog (**Settings › Games**).

### Removed

- **Adding a network share directly to a GameFlex server or to the app** (WebDAV, kDrive, Samba), a feature without a
  screen since the old interface left: shares go into **gameflex-federation**, which every device and server joins
  like a friend's server. The shares registered the old way, and their cached listings, are deleted at startup.

## [0.13.1] - 2026-11-18

### Fixed

- **The on-screen controller in portrait sits lower**: it no longer covers the compact performance view and the
  « GameFlex » line under the video, on phones whose visible screen is shorter (in the Android app, for one).
- **The touchpad of the PlayStation controller in landscape is a little narrower** (345 px instead of 360): it no longer
  almost touches the buttons beside it.

## [0.13.0] - 2026-11-18

### Added

- **gameflex-federation**, a new service in `docker-compose.yml` (port 10206): one address that shares a library with
  every device and every GameFlex linked to it — games, emulators, BIOS files, patches and emulator settings. It runs no
  game, so a NAS or a small computer can host it, on its own with `./launch.sh federation`. It shares your library
  folder, other folders you add to its volumes, and network shares (Samba, WebDAV, kDrive, FTP, SFTP); its page lists
  them under **More › Resources**. Devices join it with a token, an invitation link and its QR code, or a pairing code
  in discovery mode; **Connections** lists them, and **More › Logs** shows what it did. Its code is open source, under
  `src/microservices/federation/`.
- **A game found in several places shows its sources**: on a federated game's page, **Sources** lists where it is, the
  recommended one first (an internal disk before a network share; Samba or WebDAV for a big game, FTP or SFTP for a
  small file; then the fastest), and you can choose another one before you copy it.

### Changed

- **Two new settings in `.env`**: `GAMEFLEX_FEDERATION_ROOT_PASSWORD` (required) and `GAMEFLEX_FEDERATION_ROOT_USERNAME`
  (default `root`), the account of gameflex-federation's page. `./launch.sh setup` asks for them, and `./launch.sh` or
  `./launch.sh update` makes up the password, and shows it once, for a `.env` that has none.
- `./launch.sh update` keeps the disks you added to gameflex-federation, as it keeps those of the kernel.
- **The Android app downloads its emulator packages from their own repository**,
  [gameflex-streamex](https://github.com/zinosaure/gameflex-streamex/releases/tag/phone-emulators), the same packages
  as before.

## [0.12.0] - 2026-11-17

### Added

- **The Android app shares like a server**: catalog tokens, federation accesses given to other servers, and its Remote
  Play consoles lent to them — a lent console is for every account with a valid token there, as on a server.
- **The Android app looks for its updates by itself**, when its home screen comes back: *Update* shows the version and
  its date on two lines, and an orange exclamation mark when a newer one exists.
- **A second, smaller stream for every game**: a game launched in 1080p also offers 720p and 540p, one in 720p offers
  540p, at the frame rate it was launched with. The top one follows the emulator's own settings. *Auto* moves down a
  step when the network weakens, and back up when it is steady.
- **Invisible sticks** (**Settings › Controllers › This device**, and the game menu): the on-screen sticks are only
  drawn under your thumb while you use them.
- **L3 and R3 on the on-screen Switch, Switch 2 and Wii U controllers**, and ZL from a physical controller.

- **Pairing codes**: join a server, or sign in with your account there, by typing six digits that the other server
  shows, instead of passing a link or a token around. A code works once, for 2 minutes, and only while its server (or
  your account) is in **discovery mode**, which lasts 5 minutes and can be extended. In discovery mode, a server shows
  itself on the local network, and *Join a server* lists the servers found there; you can always type an address
  instead. Five wrong codes close discovery mode. Invitation links, QR codes and tokens work as before.
- **Watch a federated server's games without an account there**: *Live* now shows the games running on every federated
  server, even one where you have not pasted an account — those only offer **Watch**. The tab's counter counts them too.
  A server shares its running games with whoever holds its link, and updates them as they start and end.
- **Watch a game from its link, signed in or not** (the public watch page, also what a Chromecast shows): the video,
  with a short menu — information, sound, fullscreen, the link and QR code to invite others to watch, the video quality,
  and Quit. No controller, no saves, no cheats.

- **Your favorites on Live**: under the live games, *My favorites*, ten per page, each opening its game's page in the
  library; a favorite whose game is gone opens its page in *My favorites*, which says why. With none yet, the box stays,
  with a greyed line and a link to the Catalog.
- **Settings › Invite**, for every account: this server's address, to open GameFlex on another device — a QR code to
  scan with a phone's or a tablet's camera, and the link to copy. It creates no account: everyone signs in with their
  own. A phone that is on no network says so instead of showing a link nobody can open.
- **A download queue for games**: games downloaded into the library from a federated server wait their turn and download
  one after the other, in the order asked for. Each shows its place in the queue on its page and in the source's list;
  **Live › Downloads** lists them all, the running one with its progress; **Remove from the queue** or **Cancel** stops
  one. The server keeps the queue, so you can close the page. Emulators, BIOS files and patches still download at once
  on a server; the Android app downloads one thing at a time.
- **Settings › Profile › Federation** (it replaces *Token*): **Create my token**, and **Sign in at federated servers** —
  each user pastes the invitation or the token of their own account at a federated server (a valid one lets them play
  on its consoles), and sees which account they play under there.
- **Download the metadata into the library** (a federated game's settings page, **More**): its sheet alone — texts and
  images — copied into your metadata under its game id, without the game. A game already here is refused; a server
  running an older version does not lend its sheets yet.
- **Choose which federated servers' games you see** (**Settings › Application › Federated servers**): one switch per
  federated server, on by default, a server added later too. Off, its games leave your libraries and their counts on
  every device you sign in on — the choice is kept with your account. The Catalog and **Settings › Consoles** still show
  every game.
- **Guide sections**: the settings pages that explain something (Controllers, a pad layout, Remote Play and adding a
  console, Consoles, an emulator's settings, Internal resources, Cache, Logs, Rescan everything, Android app, a token)
  link to the matching guide of the public repository, at the right heading.
- **The catalog** (**Settings › Catalog**, a tile at the top), for every account whatever its role, even one that
  cannot launch a console: every game of this server (its library and its internal disks) and of your federated servers.
  Search by name; filters for your favorites, the sources, the status, a minimum size and the consoles. A game's page
  shows its information, a **Download** section — the game and its metadata (its `metadata/<console>/<game id>/`
  folder as a `.zip`), both downloaded by your browser — on the Android app, in the phone's browser —, never into the
  library — and a **More** section: its sheet in the library, its saves, adding or removing it from your favorites. A
  federated game's metadata archive is read at its server (a server running an older version does not lend it).
- **Favorites**, per account: they follow you on every device. Add or remove one from a game's settings (the gear,
  section **More**) or from its page in the catalog. **Settings › Profile › My favorites** lists them, a federated
  server's games included: a favorite opens the game's page of the catalog, the same one. A favorite whose game is gone
  (removed, its console removed, its server no longer federated) says so, and can be removed.
- **See its saves**, on a game's page in the catalog and in My favorites: the game's page in **My saves**, on this
  machine or at its federated server under your account there; greyed out, with the reason, when it has none or when
  your account token for that server is missing.
- **A note on a backup** (**Settings › Profile › My saves › Backups**): why you set a save aside, and when. It is
  written when making the backup (*Backup* now opens a small form: an optional note, then *Back up*), shown in the
  backups' list and on the backup's page (the note, when it was written, by which account), and edited or removed with
  *Write a note* / *Edit the note* in its *More*. It is kept in a `note.json` beside the backup, and goes with it when a
  newer backup (or a restore) replaces it, or when the backup is deleted.
- **Federated servers are a source for your own games' metadata** (chantier 57): **Choose an image** offers, after
  IGDB, SteamGridDB and libretro, the images of the same game (same identifier) or of a game with that title at each
  federated server; **Not the right game?** has a group per federated server, and picking one of its games copies its
  sheet and images into this server, where they stay when it goes offline (**Forget a source › Federated server** removes
  the copy). A game downloaded from a federated server brings its sheet along. **Settings › Metadata provider** shows how
  many federated servers there are.
- **Share my metadata providers**, a new option of a federation access (**Settings › Federation**, when giving access
  and on a token's page), off by default: with it, for a title this server does not have, the other server's search also
  asks this server's IGDB and SteamGridDB accounts. The keys never leave this server; the searches count against its
  quota.
- **A federated game's sheet and images come from its server** (chantier 57): in the library, in Live and on the
  game's page (**Settings › Consoles › a console › Games › a federated server › a game**), its release date,
  publishers, developers, genres, series, identifier and images are read at the server that lends it, read-only — they
  are corrected on that server. When the server refuses our token, does not answer, or runs a version that does not lend
  sheets yet, the page says so, and nothing is taken from this server's own providers instead.
- **The Android app shows a federated game's sheet and images too**: the covers in a federated server's game list, the
  background and logo of the game in the library, and its information (the gear, its settings page) are read at the
  server that lends them, with the same access as its games. A server running an older version does not lend them yet,
  and the app says so.
- **The Android app has its own metadata provider** (**Settings › Metadata provider**, back on the phone): its own IGDB
  and SteamGridDB accounts, kept on the phone. Its own games get their sheet and images (what the game carries,
  libretro, then those accounts), written next to them — in the app's library when *Write the metadata to the game's
  disk* is off —, and every gesture of a game's page works there too: correct the text, choose an image (its federated
  servers are offered after the providers), *Not the right game?*, forget a source, reset, and *Metadata without a
  game*.
- **The Android app has its consoles' background images**: the strip behind a console and its **Appearance** (**Settings
  › Consoles › a console**), filled from the phone's own metadata providers (libretro without an account, then its IGDB
  and SteamGridDB accounts); an image is replaced from a search, and **Reset the default images** downloads them again
  from those providers. They said *Not yet on this device*.
- **A console with no background image shows those of your first federated server**: when its folder has none — no
  image dropped by hand, and nothing found by the metadata providers (no account, or a console libretro does not have)
  —, its strip and its **Appearance** show the images of the first federated server, in the order of your servers, that
  lends some, and say where they come from; they are read at that server, nothing is copied. A server that does not
  answer, refuses our token or runs an older version is skipped. Replacing an image, or resetting them, acts on this
  device's images.
- **What the server writes to its logs** (**Settings › Logs › What gets written**, administrators): the kernel log on
  or off; when on, its levels (Debug, Info, Warn, Error) and *HTTP requests* apart, whatever levels are on; game
  session logs on or off; the reports players send from an error screen (`reports.log`) on or off — off, *Report the
  error* is no longer offered. Applied at once, without a restart, a running game included; only the files change, the
  services' output (`docker logs`) shows everything as before. By default the kernel log writes Info, Warn and Error but
  not the HTTP requests, which made up nearly all of a day's file, and game sessions write their logs.
- **The launcher keeps what the current interface had** (chantier 56):
  - a game status now takes a **note**, and the game's **history** of reports (status, emulators, note, who, when,
    emulator versions) is listed under it (**Settings › Consoles › a console › Games › a game › Information**);
  - the **status of a whole console**, which every game without its own takes (**Settings › Consoles › a console ›
    More › Console status**);
  - a **federation invitation link opened in a browser** (`/invite/<code>`) shows where to paste it, signed in or not;
  - a console's warnings (still experimental, heavy games, firmware, the GPU of the machine that will run it) are in
    the **Information and settings** panel of a game;
  - a **minimum size filter** and an optional **heaviest first** order in the games list of a source, to find the
    heaviest games before deleting them.
- **On the phone**, the launcher follows the phone's rules: only an administrator starts a game, a recent console
  needs its emulator on the device, and a game download opens in the phone's browser.
- **Member invitations** (**Settings › Members**, administrators): a new member is created without a password; the
  administrator gets a one-time link (`/join/<code>`, valid 24 hours, with its QR code) and the member chooses their
  own password on the launcher's public join page, then is signed in. A pending member's page shows the link's
  expiry and author, and *New invitation link*; the members filter *Invitation accepted* works.
- **Password reset by an administrator** (**Settings › Members › a member › Danger**): every session of the account
  ends on every device, its running games end, and a new link lets the member choose a new password.
- **Log out other devices / all devices** (**Settings › Log out**): two new rows next to *Log out*, each with a
  confirmation. *Log out all devices* also ends the account's running games (saved), like a reset; *Log out other
  devices* and changing your password leave them running.
- **Metadata without a game** (**Settings › Consoles › a console**, administrators): the metadata folders, on the server
  and on each internal disk, that no reachable game carries any more — a removed disk, a deleted game —, each deletable
  after a confirmation; the server checks again before deleting, and never touches a console's own images.
- **A new web interface** (the launcher of the previous notes, now named edge), at the web address itself
  (`GAMEFLEX_PORT`) — it replaces the previous interface, see *Changed*. Tabs Live, Retrogaming, Streamex and Remote Play; one console per
  screen and one game per screen, scrolled vertically (finger, wheel, Up/Down); a search panel
  with source and status switches and a paged game list; a game settings panel (preset — disc,
  emulator, BIOS —, stream, saves); *Continue* when the game has saves, *Resume* when one of your games is
  running (any other game is then greyed out). Built for the gamepad: L2/R2 walk the header,
  A plays or opens, B goes back, X searches, Y opens the settings, Start goes to Live, Select
  opens the settings; a shortcuts panel shows the buttons of the connected pad or keyboard;
  optional navigation sounds. *Play* opens the launcher's own game page: the picture, the
  on-screen pad, and a menu (L2 + Home held 3 s, Escape or the round button) with the players,
  the saves, the stream settings and *Quit*. Friends' games and Remote Play devices open there too. Its settings are pages, each with its own address (a
  link opens it, a reload keeps it, B goes back one level), in a centred box, their sections in
  cards; a list row only opens its page, where its actions are; the forms, the searches and the
  pagination are those of the current interface; buttons and fields share a 3 px corner.
- **Game information**: publisher, developer, genre, release date and series, from the
  libretro-database for retro consoles (downloaded once per console, no account) and from IGDB
  once its account is entered in **Settings › Metadata**. Kept on the server's disk, so they stay
  visible offline once fetched.
- **Multi-disc games are grouped**: the game list tells which entries are the discs of one game
  (`[Disc1of4]`, `(Disc 1)`, named discs such as GDI/NOD) and which series each game belongs to.
- **A game's status and release date in the launcher** (**Settings › Consoles › a console › Games › a game › Fix the
  information**, administrators): a *Status* section, one choice per emulator of the console (*Playable*, *Crashes in
  game*, *Not playable*, *Untested*), shown at once on the game's sheet and in the library's search; the release date
  is either *The year only* (a list of years) or *The full date* (a calendar).

### Changed

- **Game and console statuses are kept in the database**, on a server and on the Android app, instead of one file per
  console (`status/<console>.json`). The files are moved into the database at the first start, then removed.
- **The Android app downloads its emulator packages from the project's GitHub Releases** (`phone-emulators`), its
  official source, whatever server it was built for.
- **Emulators updated**: RPCS3 0.0.43-20607, Xenia Canary `f9431eb`, the Eden nightly of 2026-11-17 (`d85bad8924`) and
  Lichii build-316 (Switch 2), on the server and in the Android app (RPCS3, Eden, Lichii, and KytyPS5 with the touchpad
  fix). Download them from **Settings › Emulators**. Citron stays at its current version.
- **The on-screen controller, redrawn**: the sticks are always floating — touch anywhere in the lower part of the
  screen, under the buttons, and the stick starts there; the buttons sit higher, the menu row lower; in landscape, the
  PlayStation touchpad takes most of the middle of the screen. The button opacity goes from 1 % to 100 %.
- **The game menu button sits in the bottom right corner, and opens by sliding it to the left**: a tap does nothing, so
  a thumb on the screen never opens the menu by accident. It fades to 10 % while you play, and stays there to be grabbed.
  Escape, Enter (with a controller) and L2 + Home still open the menu.
- **The Android app's settings pages**: *Guide* and *Good to know* are plain text sections, as on a server; *How it
  works* moved into the Guide; *Root account* shows the root account and how many administrators and players there
  are, and says to sign in as root first when there is no administrator; the GameFlex banner shows **Open**.
- **Portrait, half screen** can only be chosen with a physical controller connected.
- **The Android app's home screen looks like GameFlex's settings**: GameFlex at the top, with a controller button that
  opens GameFlex itself, as does the GameFlex banner below it (both greyed out while the server is stopped); four tiles —
  *Start / Stop the server*, *Restart*, *Invite to join* (the phone's address as a QR code and a link, *Share the link*)
  and *Join a server* (scan or type a link; the servers you joined, to open again or remove); then *Storage* (*Location*,
  *External disk*), *Root account* (reset the root password, shown once with *Copy*), *About* (*Update*, and *The
  phone*: its model, processor, memory and GPU, and whether it can run the recent consoles), and the settings' footer.
  Each line opens its own page; the Back button comes back. Removing a disk or a joined server now asks first.
- **The default stream is 540p at 30 frames per second, for every console**: retro games, recent consoles and Remote
  Play, on a server as on the Android app, whenever nothing is chosen. It was 720p at 60 for Remote Play and recent
  consoles, and the console's own size at 60 for retro games. The choice is still kept per console in each browser.
- **An automatic bitrate now follows your network**: at each launch, your browser measures its throughput to the
  server that runs the game — this one, or the federated server it starts on — for a second at most, and the stream
  takes 60 % of it (between 0.5 and 15 Mbps), kept for the whole game. It was a fixed bitrate (per resolution for Remote
  Play, 10 Mbps on the Android app). The stream panel's fields each say what they change.
- **The video stream choice** (a game's menu › Video) shows its three options as cards, each with its icon and what it
  does: *Automatic*, *Current stream*, *Low quality*. A multi-disc game's discs are chosen the same way.
- **A federated game's settings** (the gear): *Forget my token* is gone — the account you play under at its server is
  managed in **Settings › Profile › Federation**, which the section *At <server>* now links to.
- **Remote Play stream settings are each user's choice**: a Remote Play console's gear (on the Remote Play screen)
  opens the same stream panel as a game's (resolution, frame rate, bitrate, network auto-detection), for every account,
  and for a federated server's console as for one of this server. The choice is kept for that console in this browser
  and sent with each connection, and the server checks it. With no choice, the service's defaults (720p, 60 FPS, auto
  bitrate). **Settings › Remote Play** no longer sets the stream.
- **Settings › Federation** (administrators) now only creates the connection between servers, and starts with two
  lines: **Delivered accesses** (the friends this server shares its games and consoles with; **Create a new access**,
  **Pending delivered accesses**) and **Joined servers** (the friends who share theirs; **Join a server**). A joined
  server's page shows its information in sections (the server, with its address opening in a new tab; its machine;
  what its access opens); an access's page shows **Games**, **Consoles** and **Metadata**, each *Allowed* or *Not
  allowed*. Playing on a friend's consoles is set in **Settings › Profile › Federation**. The `alert` boxes are gone;
  section help texts and a **Guide** section explain federated servers and accounts.
- The federation guide (`docs/federation.md`) explains federated servers and federated accounts, and follows the
  current settings.
- **The top bar**: the controller-shortcuts and settings buttons are outlined, and filled while their panel or the
  Settings are open. Next to *GameFlex*, only the *staging* badge of a test install — no more version (it stays in the
  Settings footer).
- **Remote Play**: a console's screen says where it comes from, above its name — *This server*, or the federated server
  that lends it.
- **Settings**: *App home* (in the Android app) moves from *About* to *Session*, next to *Sign out*; the groups only an
  administrator sees (*The server*, *People and sharing*) carry the green administrator badge; the Android app page
  drops *What this build does*, and on a test install its download reads *Download the test version*.
- **No more alert boxes**: why a game cannot start here is the first line of its *Information*; an emulator settings
  file that is missing or unreadable is a *State* line; with no controller, the *Controller* line says *None*; a member
  who cannot be managed here says why in the help of the page's first group; the other notices are a muted line, and a
  refused sign-in a red line.
- **A member's console access saves at each switch** (**Settings › Members › a member › Console access**): no more
  *Save* button; a refused change goes back, with a notice.
- **A game's settings** (the gear): **Download the ROM** is replaced by **Add to my favorites**; downloading a game
  moved to the catalog.
- **My saves starts with its sources**, in sections, like a console's games: *On this server* — this machine (the
  saves your games play), the backups, and the copies a federated server sent you — then *Federated servers*, each with
  your saves there; each source opens its list. Internal disks have no section: saves stay with each account on the
  server, whatever disk a game is on.
- **The phone's log is set from Settings › Logs**, like a server's: *What gets written* shows the one switch the phone
  has, the kernel log, applied at once and kept by the phone's kernel for its next starts. Settings › Logs shows only
  what a host can set.
- **The launcher's Live tab, reorganised**: your game in progress first (*Resume*, and a square stop button on the
  right, after one confirmation), then the other players' live games, shown by their muted live video (*Join* while a
  controller is free, *Watch*; an administrator can stop someone else's game after two confirmations, the second saying
  who is cut off). When no game is running at all, a carousel of five library games that have a picture (*See the game*
  opens its page), or the GameFlex intro picture when the library is empty. Notifications and downloads come below,
  each keeping its box with a greyed line when empty.
- **The web interface is the new one** (chantier 56): the launcher, out of beta, replaces the previous interface at
  the web address — no more second port, no more *beta* badge. It can be installed as an application from the browser
  (its manifest and icons, without an offline cache: nothing plays offline). Signing in again is needed once, and the
  settings this browser kept for the launcher (language, sounds, last tab and console, a console's stream settings)
  start from their defaults; what this browser kept for the previous interface (vibration, screen orientation) carries
  over, and the on-screen pad layouts are kept on the server. Its sources live in the `edge` service
  (`src/microservices/edge`), next to the web front that serves the page in a public install: one service for the
  interface and its front, and the one table of the API domains. The `gameflex-edge` image of a public install serves
  it (a production build, with `build-info.json`), and is built from the repository root. The development stack's page
  port is `GAMEFLEX_PORT`, as in a public install (formerly `GAMEFLEX_WEBAPP_PORT`; `GAMEFLEX_LAUNCHER_PORT` is gone).
- **The Android app carries the new interface** (chantier 56): the page it opens is the new web interface. What the
  phone's server does not do yet — downloading an emulator, a BIOS or a patch, a console's default emulator, game
  artwork and metadata, console backgrounds, cartridge saves — says *Not on this device yet* on its page instead of a
  bare failure. A game download from the phone still opens in the phone's browser.
- **Strong passwords**: a password a person chooses (the join page, **Settings › Profile › Password**) must be 8 to
  20 characters long with an uppercase letter, a lowercase letter, a digit and a special character; the form shows
  each rule as it is typed, and the server enforces the same rule. Changing your password logs out your other devices.
- **Administrators no longer set a member's password**: `password` is refused by `POST` and `PATCH /api/v1/users`.
- **Deleting a member also deletes their saves** (**Settings › Members › a member › Danger**): their whole saves folder
  on the server (retro states and cartridge saves, recent consoles, backups and copies received from federated servers),
  after their running games end; the confirmation says so. Before, the saves stayed on the server. Not yet on the
  phone.
- **The launcher's panels and settings pages, redesigned** ("tiles and groups"): the game menu,
  the game settings and every settings page open with tiles for the most used actions, then
  rounded groups; a button carries a text or an icon, never both. The game menu groups what
  concerns other people under *With others*, and the on-screen stats become one three-way choice.
- **A console's settings in the launcher, on one page** (**Settings › Consoles**, administrators):
  its emulators and its BIOS (installed or not, version, size in MB; the one in use highlighted),
  then its games, its emulator settings and its appearance. An emulator or a BIOS has its page:
  *Use this version*, *Default emulator* or *Default BIOS*, *Delete*; when it is missing, *Get it*
  from the official source or a federated server — each source says whether it has it, and the
  download shows its progress. The tabbed console page and its source selector are gone.
- **A console's games in the launcher, under the console** (**Settings › Consoles › a console › Games**,
  administrators): its sources (this machine, each internal disk, each federated server, with their game counts),
  a source's games, and a game — its information, then its patch, its appearance and its information fixes, each on
  its own page, and removing it. A patch is deleted, or fetched from an internal disk or a federated server; a
  federated server's game downloads into the library; both show their progress. The **Settings › Games** page is
  gone, and remote resources (Samba, WebDAV, kDrive) are no longer a source of games.
- **Images are chosen, no longer uploaded** (**Settings › Consoles › a console › Appearance**, and a game's *Appearance*,
  administrators): a search among the metadata services (IGDB, SteamGridDB, libretro-thumbnails — the last one needs no
  account) lists their images; touching one asks for confirmation, then the server fetches it from that service's image
  host only. A console's strip has eight fixed slots: a free one is filled, a taken one is replaced or emptied; images
  already in its folder beyond the eight are listed as extra, never shown nor erased. A game lists one image per kind; an
  image chosen by hand can be deleted, and *Reset the default images* removes them all. A running download (an emulator,
  a BIOS, a patch, a federated server's game) now has a *Cancel* row under its source.
- **The metadata services in the launcher, one page each** (**Settings › Metadata**, administrators): the list of the
  services the server reads — IGDB and SteamGridDB (configured or not), libretro-database, libretro-thumbnails and the
  game's own files (no account needed) — then a page per account service: its keys (never shown back; an empty field
  keeps its value, only the fields typed are sent) and *Remove this account*, confirmed in a window.
- **The launcher's settings pages, after a round on the phone**: each page opens with the round *Back*
  button and its title, as on a console's games, the breadcrumb below, no line under it; a row says at its
  right edge what it does (a chevron for a page, *?* for a question, *i* for information, an arrow for an
  outside link, nothing for an action done on the spot); lists come first, then *More*, then *Danger*.
  **Federation** has no more tabs: its servers and accesses, then *Join a server* and *Give access to this
  server*. **Resources** only manage the internal disks (remote resources leave the launcher; a disk shows its
  game count under its name). **Credits** name the emulators, their authors, licences and source addresses,
  instead of the console icons. **Application** groups language and orientation under *Appearance*; in a
  browser, choosing a landscape orientation turns fullscreen on and locks the screen, or says the browser
  refused. Navigation sounds also play on touch and mouse.
- **The launcher's members and token pages, after the same round**: **Profile › Token** opens on its details,
  with the real expiry date the server keeps; no duration is pre-selected any more; *Create my QR* moves to
  *More*, *Disable the token* to *Danger* with a confirmation, and the account invitation no longer offers
  *Share*. **Members** puts *New member* under the list, filters it from a button next to the search
  (administrators, players, token enabled; *invitation accepted* is not available yet), marks administrators
  with a green shield and shows only *@username* under each name, like the *My account* card. A member's
  account (username, name, an *Administrator* switch, password) is one group; their console access and their
  saves are pages of their own, and **Settings › Users' saves** is gone. Every list filter (members, the logs'
  period, a log's level, the saves' console) now opens in a side panel from the button next to the search, and a
  calendar day can be picked with the gamepad. **Give access to this server** chooses *rather the link and the
  token* with a switch, under one button. A page header carries no subtitle any more; an empty list keeps its box
  with one greyed row; a group of controls has no dividers, saved or not, and switches carry no icon.
- **A game's sheet no longer says what was changed by hand**: an administrator can always change the metadata folder
  directly, so the sheet, its images and the game list never claim a value was "set by hand". One **Reset the sheet**
  row, on the game's page, brings back what the services give, corrected texts and replaced images alike; *Forget a
  source* starts with no source chosen and no longer offers the hand correction.
- **Settings › Metadata provider** (formerly *Metadata*) has a **Write metadata on the game's disk** switch, on by
  default. Off, the metadata of a game on an internal disk live in the server's folder — useful for a read-only disk,
  such as an NTFS disk on the phone; a game already known there keeps its own. A write refused by a read-only disk says
  so.
- **Remote Play consoles are added, set and removed in the launcher**, **Settings › Remote Play**
  (administrators): the list, pairing (discovery, name, address, PSN account ID, PIN), a console's
  stream settings, waking it, removing it.
- **The launcher's screens, after a round on the phone**: every side panel opens with a round close button on the left
  and its title, without subtitle nor line. A game's settings panel shows its background with its logo or icon on top,
  two tiles side by side — **Preset** (disc, emulator and BIOS in one form) and **Stream** — and, in *Information*, its
  status and, when the libretro-database knows it, its number of players (also shown on the game's sheet). The library
  search keeps one switch per source and per status, all on, and the search bar (a single letter keeps the titles that
  start with it); the letter grid and the series headers are gone, and the games are the paged list of the settings
  pages, each row saying its source, status and series. The **Remote Play** tab is greyed out without a console; a
  console's screen shows one large image chosen in **Settings › Remote Play › a console › Background image**, and its
  button follows the console: *Turn on* in standby, *Play* once online, *Busy · Watch* when someone else plays, greyed
  out when unreachable; its gear shows its information and its stream settings (saved on the server by an
  administrator). Only registered Remote Play consoles have a screen (consoles are registered in **Settings › Remote
  Play** only), and touching a console's image no longer opens or wakes anything: only its buttons act.
- **Quitting a game in the launcher asks for confirmation**, in the launcher's confirmation window: *What to do with the
  game?* — *Quit and end the game* by default, or leave it running, or pause it (retro) — and *Save the state when
  leaving* in its body, then *Cancel* and *Quit*; no second window. *Cancel*, B or a tap outside cancels.
- **The launcher's searchable lists** show the search bar and its result count in a box of their own, a line, then the
  list; the library search panel titles its switches *Sources* and *Status*.
- **The launcher's pages, another round**: pages without an image (Settings, Live) have a soft gradient from the top
  of the screen, under a transparent top bar with the same veil as on the console screens; a page's breadcrumb sits
  under its title, smaller, and a long one wraps instead of being cut; administrators are marked with a green check
  badge. **About › GameFlex** links to the public GitHub repository and each of its documents (README, guides,
  licences, contributing) and shows GameFlex's licence in full; **About** also links to *Buy me a coffee*, and the
  settings footer reads *GameFlex · GitHub · Buy me a coffee*; the *Licence* entry is hidden for now. **About ›
  Android app** explains the installation step by step. **Log out** keeps *Log out* alone, in a normal colour, and puts
  *Log out other devices* and *Log out all devices* in their own group. A disk's consoles (**Resources**) open the
  console's page; **My saves** filters with one switch per console; a console's *More* says *Games*.
- **A console's eight images** (**Settings › Consoles › a console › Appearance**, administrators): *Reset the default
  images* moves to *Danger*, asks for confirmation, and really empties the eight slots before filling them again with
  the default images; slots are only replaced — no more *Free* slot nor *Add an image*.
- **My saves names each game**: a save of a recent console (Switch, PlayStation 3, 4 and 5) shows the title of its game
  in the library or on an internal disk, found by its title ID, instead of the bare ID (`01000EA014150000`); without
  such a game, the title the save gives itself, its ID last. Every line of **My saves** starts with its console, then
  where its game is (the disk, or *Library*), like the other lists; a save's page is titled after its game.

### Removed

- **The per-console Remote Play stream setting** that an administrator set for everyone: its
  `/api/v1/playstation/device-settings/{id}` route, on servers and on the Android app, and the column that stored it
  (dropped when the service starts). A console that was set to another stream (1080p…) starts from the defaults until
  its user chooses.
- **The game in progress and the log switch on the Android app's home screen**: the game is seen and stopped from the
  web interface (home, *Live*), the log is set in **Settings › Logs**.
- **The previous web interface** is no longer served: the new one has taken its address (*Changed*).
- **`GAMEFLEX_FILE_LOGS_OFF`**: the log settings above replace it.
- **The phone's free self-registration** (`Register`), which only tests still called: an account on the phone is
  created by an administrator and joined through its link, as on a server.
- **The game page of the old interface**: the launcher replaces it. The current interface no longer
  plays, watches or joins a game; its game list, its settings and the Remote Play device pages stay.
- **The key test of Settings › Gamepads**, in both interfaces: nobody used it.
- **The custom TURN server**: the **Settings › TURN server** screen (coturn, eturnal, Cloudflare),
  its `/api/v1/turn-config` routes and the encrypted settings behind them, and the
  `GAMEFLEX_PASSPHRASE_SALT` variable. Games use STUN only, as before: on a network that cannot
  reach the server directly, play through the WireGuard VPN.

### Fixed

- **The touchpad of the on-screen controller on PlayStation 5**: KytyPS5 ignored it; a finger, a slide and a click on it
  now reach the game.
- **The Video panel said “0p · 0 fps”** for a retro or a Remote Play game: it now says the stream it really receives.
- **The *Auto* bitrate is capped by the resolution** (1080p 15 Mbps, 720p 6.7, 540p 3.8, 360p 1.7), and *Video bitrate*
  counts the video alone.
- **Sound after reloading a game page, or when watching one**: the picture came back without the sound, and only
  leaving the game and resuming it brought it back. The sound now starts at the first touch, click or key press on the
  page, which says so, and at once in the Android app.
- **The link of a federated game opens that game**: opening the address of a game from a federated server (a link,
  a reload) showed the first game of the library instead when this console also had games here — they arrive first.
  The game page now waits for every source while the requested game has not arrived.
- **A federated account pasted or forgotten in Settings applies everywhere at once**: *Live* and the game page kept
  their own copy of the session at the federated server, and went on using a forgotten account until it expired.
- **The background of a Remote Play console says where it comes from**: when a federated server lends it, its page
  now shows the **Source** line, like the other background pages.
- **Send the link, in the Android app**: during a game played at a federated server, the game menu's *Send the link*
  did nothing — the request for the phone's share sheet went to that server, which does not have it. It now always
  goes to the phone, and when the sheet cannot open, the link is copied (or shown to copy) instead of nothing
  happening. The link of such a game now points to the server that hosts it, where it can be opened.
- **Launching a game no longer leaves the browser's fullscreen**: started from a fullscreen launcher (**Settings ›
  Application › Fullscreen**), the game page exited fullscreen as it opened. A game now keeps the state it finds:
  fullscreen stays fullscreen, and a page that is not fullscreen does not become so on its own. The Android app,
  always fullscreen, is unchanged.
- **The controller-shortcuts panel fits a phone's screen**: a long label pushed its buttons out of the frame; it now
  wraps, and the buttons of one gesture stack when space runs out. Every information line wraps its label the same way.
- **The Android app offers the federated servers again in *Get it*** (an emulator's or a BIOS's page, when it is
  missing): the official source (the core for this device, at libretro's buildbot) and each federated server, asked when
  the page opens, greyed with their reason when they do not have it — it said only *Not yet on this device*.
- **The Android app downloads a game into its library** from a federated server (it said *Not yet on this device*):
  its kernel now serves the download command, one download at a time, with the app's progress and cancelling.
- A download refused by the federated server, or cancelled, no longer leaves an empty console folder in the library.
- A list of several thousand pages (the catalog) no longer turns the page blank after a search: its page selector
  overflowed the browser's stack.
- **A federated server no longer looks offline while its disk wakes up**: before showing a server's games, a phone or a
  server asks it whether its listing changed, and gives it five seconds. The server answered by reading the folders of
  its internal disks every time, and a sleeping hard disk took up to half a minute to wake: the server was then shown as
  *not responding* for a minute, and its games were missing. It now answers from memory, without touching a disk; what
  changes its listing (a scan, *Rescan* of a disk, an import, a deletion, a disk added or removed) moves the date
  forward. What you drop on an internal disk by hand reaches your friends once a list of that disk shows it, or after
  its *Rescan*. A phone serving its games to a friend does the same.
- **A console's library counts, and opens, the games of your federated servers**: its **Library (N)** button counted
  only the games of this device or server, so a phone without games of its own showed *Library (0)*, greyed out, on
  every console, and none of its servers' games could be reached. The count now adds each federated server's games (as
  the copy of its listing counts them), *Show consoles with no games* off no longer hides a console whose games are all
  on a server, a library still reading its servers says *Loading…* instead of *No games found*, and one whose servers
  did not answer says so.
- **The save of a Switch game without its title ID in its name is found again** (« Crash Bandicoot.xci »): the server
  reads the title ID inside the `.xci` or `.nsp` itself — the ticket of a package, otherwise the header of its program,
  with the console keys already installed (`bios/switch/prod.keys`) — so the game's page lists the console save made
  with it. The same ID now names its per-game settings (`patches/switch/<title id>/`) and its metadata folder: the
  folder it had under its library reference moves there by itself, the first time the game is shown, without
  overwriting a folder already there.
- **An empty save folder no longer counts as a save** (recent consoles): a game stopped before it wrote anything used to
  offer Continue instead of Play, and showed a save dated 1 January of year 1.
- **A game's page now offers Resume for the recent-console game you are playing** (Switch, PS3, PS4, PS5, Xbox): it
  used to grey Play out instead, as if another game were running. The server's GPU is also shown by its full name
  (« AMD Radeon 780M Graphics » rather than « AMD ») in a game's settings panel.
- **The kernel log no longer keeps writing to yesterday's file** after midnight, nor to a log deleted from
  **Settings › Logs** while it was being written (today's kernel log, a running game): the next line starts a new file.
- **PAL games ran 20 % too fast, with their sound lagging** (PlayStation 2, and any emulator that changes its
  frame rate once the game has started, such as a Dreamcast or GameCube/Wii game switching between 50 and 60 Hz): the
  game now runs at its own frame rate, and its sound stays in time with the picture.
- **Remote Play: a controller input is no longer lost** when the gateway's input channel is not open yet (or any
  more): it goes through the input socket instead, under the same player queue.
- **Remote Play at 60 frames per second**: each picture is now dated at the frame rate asked of the console (16.7 ms at
  60), instead of 33 ms whatever the rate; on the server and on the phone.
- **Reloading a page whose address ends with a file name** (a game of a console, a log) no longer answers 404 when the
  page is served by the web front of a public install or by the phone: only a missing file of the page itself
  (a script, an image, a download) does.
- **Deleting an administrator who had configured a metadata provider** no longer fails (an error 500): the provider
  keeps its settings and loses only its author.
- **The date pickers** (Settings › Logs) wrote "???" when no day was chosen, and named the months
  in the browser's language instead of the app's.
- **Launcher game sessions**: the launch screen now covers the game sheet's buttons; opening a game no longer turns the
  screen to landscape by itself (only **Settings › Application › Orientation** decides); quitting a game played in
  fullscreen no longer stops the page with an error.
- **A replaced image came back to the old one** after a reload (a console's image, its background slots, a game's images,
  a Remote Play console's background): the browser kept the old one for a day. Images are now revalidated on every read.
- **A PlayStation 4 save named "Saved Data"**, a generic label some games write, no longer shows it as the game's name:
  it takes its game's title from the library, or its ID.

### Security

- **Sessions can now be revoked**: every token carries the account's session generation; a reset, a password
  change or *Log out other/all devices* invalidates older tokens at once — the launcher returns to the sign-in page
  as soon as its channels close (code 1008) or the accounts service answers 401.
- A member link code can never be redeemed as a federation invitation, whatever kind the other server claims.
- **Only an administrator manages the server's Remote Play consoles**: discovering, pairing, setting
  the stream of, or removing a console now answers 403 to a member, and to a session opened from
  another server. Members still list, wake and play them. Pairing keeps one route,
  `POST /api/v1/playstation/bind`; `POST /devices/register`, `POST /discover-and-bind`,
  `POST /devices/discover` and `PUT /devices/{id}/settings` are gone.

## [0.11.0] - 2026-11-11

### Added

- **Nintendo Switch 2, a new console** *(experimental)*, through the Lichii emulator: keep the games
  in `roms/switch2/` (`.xci`, `.nsp`, or `.nup`, a game packed with its updates by Lichii) and
  `prod.keys` in `bios/switch2/`, download Lichii in **Settings › Emulators**, and add `switch2` to
  `COMPOSE_PROFILES`. Only a few games start yet. A Switch 2 game can use up to 20 GB of memory. Its
  image carries the same mesh shader fix of the AMD Vulkan driver as the PlayStation 5 one, and its
  *Mesh shaders* setting uses Lichii's compute fallback by default (faster in our tests, and lighter
  on the graphics driver); *Native* hands them to the driver. See [Consoles](docs/consoles.md).
- **A compact view of the game's figures**: a second switch in the menu's Performance section
  shows them in small type on two lines, the emulator's then the stream's, under the picture on an
  upright screen or along the top on a sideways one; it and the connection statistics replace each
  other.
- **New PlayStation 5 settings**: *Coarse shading* (on by default) computes one pixel in four in
  the images a game draws at least twice as tall as its window, which the stream shows smaller
  anyway; *Run-ahead of the graphics processor* (1 by default) lets a game prepare its next image
  while the graphics processor finishes the current one, for the games whose own settings ask for
  it.

### Changed

- **PlayStation 5 games run much faster**, with our own build of KytyPS5 (offered by the
  [mirror](README.md#the-mirror), in **Settings › Emulators**): its command thread waits for the
  graphics processor far less and does less work per draw, and the shaders it translates run
  faster. On a Radeon 780M, *Astro Bot*'s Celestial Garden went from about 5 to about 20 frames per
  second, *Clair Obscur: Expedition 33* reaches 20 (with its own settings, from the mirror), and
  *Black Myth: Wukong* holds its 30.
- **KytyPS5 keeps its shader cache across its own updates**, as long as they leave its shaders
  alone: a new version no longer recompiles every game from scratch.
- **The Performance panel writes video memory as used (change since the game started) / total**,
  and a memory change that rounds to zero as ±0, like the other figures.
- **PlayStation 5 games that draw with mesh shaders spend much less time handing their work to the
  GPU** (55 to 70 µs per submission instead of 210 to 270 in *Tekken 8*, *Clair Obscur*, *Astro Bot*
  and *Wukong*): the PlayStation 5 image carries its own build of Mesa's AMD Vulkan driver (RADV),
  Debian's with one patch, which no longer rebuilds the compute queue's preambles at every
  submission. The first session of each game after the update compiles its pipelines again.

### Fixed

- **A slow screen capture no longer slows the stream down**: the capture runs on its own thread,
  and the stream keeps the emulator's frame rate.
- **An encoder that fails to open no longer brings the whole streaming service down.**
- **A GPU hang that could reset the whole graphics processor, after some mesh shaders**, on AMD
  Radeon RX 7000 graphics and newer (and their integrated relatives): the PlayStation 5 image's
  Vulkan driver also carries Mesa's own fix for it (Mesa 26.2.5), backported to the Mesa it is
  built on.

## [0.10.0] - 2026-11-08

### Added

- **PC Ports, a new console**: Nintendo 64 games remade for the PC by their community, which read
  only their data from your own ROM — widescreen, 60 frames per second, high definition. The first
  is *The Legend of Zelda: Majora's Mask*, through Zelda 64: Recompiled: keep its US 1.0 ROM in
  `roms/n64/`, download the port in **Settings › Emulators**, and add `ports` to
  `COMPOSE_PROFILES`. See *PC Ports* in [Consoles](docs/consoles.md).
- **New PlayStation 5 settings**, in the emulator panel: *Render height (Unreal Engine)*, the
  height an Unreal Engine game draws its 3D scene at, its menus and text staying sharp (540p by
  default; 720p, 360p, or the game's own); *Frame rate limit*, which follows the stream by
  default, so that a faster game does not give its pictures unevenly; and *Console resolution*,
  1080p by default instead of 4K. The PlayStation 4 gets *Render height* too.
- **A game's own settings**, for PlayStation 4 and 5 games: `patches/<console>/<game ID>/settings.json`,
  in the library or on an internal disk, applied at launch over the console's settings and those
  of the game's engine. The [mirror](README.md#the-mirror) has the ones we use.
- **The Performance panel in sections** (session, gateway, emulator, machine), with the game's
  resolution and the emulator's, and the emulator's frame rate next to the gateway's.
- **The game page in portrait**: GameFlex under the video, the controller above the menu, and a
  smaller touchpad that stays next to a real controller.
- **A small spinner while a PlayStation 5 game compiles its shaders**, at the top left, with their
  number inside, instead of a line of text.
- **360p is back** among the stream sizes.
- **The emulator settings in French**, when the page is in French.

### Changed

- **Emulators updated**: KytyPS5 2026-11-04 (`91c5820`), shadPS4 0.20.0, RPCS3 0.0.43-20508 and
  Xenia Canary `57cddf7`, on the server and in the Android app (Xenia: the server only). Download
  them from **Settings › Emulators**. GameFlex's own builds of KytyPS5 and shadPS4, with the
  patches this version relies on (the PlayStation 5 settings above among them), are on the
  [mirror](README.md#the-mirror).
- **PlayStation 5 games run much faster** with GameFlex's build of KytyPS5: the game's memory is
  shared with the graphics card, which also writes the occlusion counters the game waits for.
  *Final Fantasy VII Remake* goes from about 11 to 22 frames per second, on the same picture. A
  PlayStation 5 game gets up to 20 GB of memory, instead of 12.
- **The sound is 60 ms behind the picture instead of 120**, in the browser and in the Android app.
- **The launch screen** keeps GameFlex's backdrop until the game's first picture.
- **The French interface says « vous »**.

### Fixed

- **Jumps in the picture of the recent consoles**, in the browser and in the Android app. Three
  causes: the page showed each frame as soon as it was decoded, so two frames arriving together
  fell into the same screen refresh; the server's capture took one picture twice every second;
  and it could read a picture the emulator had not finished drawing, so an older picture came back
  for a moment. The page now plays the video 50 ms behind (about 20 ms more latency), and the
  capture takes each picture when the emulator has finished it.
- **PlayStation 5 games**: *Kingdom Hearts III* no longer stops at its first video; *Final Fantasy
  VII Rebirth* no longer crashes when it reads its cache again; *Final Fantasy VII Remake*'s
  opening video is no longer pink, and its lighting and its sound are right; *Black Myth: Wukong*
  has its sound, keeps running past its memory clean-up and runs in its performance mode; *Tekken
  8* starts. With GameFlex's build of KytyPS5.
- **PlayStation 4 games**: *Final Fantasy VII Remake* passes its title screen, and *EA Sports FC 27*
  saves and reaches its menu. With GameFlex's build of shadPS4.
- **The PlayStation 5 sticks, L2 and R2 in the Android app.**
- ***My saves* names the PlayStation 4 games**: it showed their `CUSA…` code only.

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
