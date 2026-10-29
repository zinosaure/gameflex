# Storage: disks, shares, saves and logs

**Goal:** add more disks of games, read games from a network share, and keep an eye on saves, the
cache and the logs.

Everything here is in **Settings**, for administrators:

| Screen | What it shows |
|---|---|
| **Games** | The games of your library, console by console; copy games from another source. |
| **Emulators**, **Firmware**, **Patches**, **Emulator settings** | See [Organize your library](library.md) and [Emulator settings](emulator-settings.md). |
| **Saves** | The room each account's saves take. |
| **Cache** | What a game's first launch installed, and its size. |
| **Logs** | What happened, without a terminal. |
| **Internal resources** | Your other disks of games. |
| **Remote resources** | Network shares: Samba, WebDAV, kDrive. |
| **Rescan all** | Refreshes the list of everything in your library, after you copy files by hand. |

## More disks of games

Your games don't all fit on one disk? Add others, as many as you like.

1. Lay out the disk like a library: `roms/<console>/`, and if you want `bios/`, `cores/`,
   `patches/`, `params/`. GameFlex reads it as it is, and never writes to it, except to delete a
   game you ask it to.
2. Add one volume line per disk to the `gameflex-kernel` service in `docker-compose.yml`, under
   `/mnt/<name>`:

   ```yaml
       volumes:
           - /media/games:/mnt/games
   ```

   The name (`games` here) starts with a letter or a digit, and uses letters, digits, `.`, `-` and `_`.
   `./launch.sh update` keeps these lines when it brings a new `docker-compose.yml`.
3. Restart: `docker compose up -d`.

**What you should see:** the disk in **Settings › Internal resources**, with its number of games.
On the games page, choose it in **Source**.

- **Its games play from the disk.** Its emulators, BIOS files, patches and settings are copied into
  your library from their screens, with the disk as the source.
- **A game on two disks** plays from your library first, then from the disks in the order of their
  names. It keeps its saves.
- **A disk shown as *Missing*** was unplugged, or not mounted when GameFlex started. For an external
  disk, prefer a fixed mount point (`/etc/fstab`) to the one your desktop creates when you sign in.

## Games on a network share

**Settings › Remote resources › Add a server** reads games from:

- a **Samba** share (Windows sharing, most NAS): its address, the share name, the folder, and a user
  name and password, or anonymous access;
- a **WebDAV** folder: its address, the folder, a user name and password;
- **kDrive**: the drive ID, the folder, and an API token created in the Infomaniak manager.

GameFlex tests the share, then reads its list of games. **A share brings games only**, and **a game
is copied to your library before you play it**: in **Settings › Games**, choose the share as the
**Source**, then download the game. Passwords and tokens are stored encrypted with your
`GAMEFLEX_SECRET`: if you change it, enter them again.

## Saves

**Settings › Saves** shows, for each account, how many files its saves hold and their size.
**Download** gives them as a zip; deleting removes every save of that account, games in progress
included. Each player also sees their own in **My saves**.

## Cache

The first launch of some games installs data, once for every account: an unpacked PlayStation 4
game, the PlayStation 3 firmware and game data, compiled shaders. **Settings › Cache** lists each
entry with its size, and deletes it. Everything there is rebuilt when needed, but rebuilding takes
time. An entry can't be deleted while a game of its console is running.

## Logs

**Settings › Logs** lists the server's log for each day, and one log for each game. Open one to read
its last lines, search it, or filter it by level; **Download** gives the whole file, with tokens
hidden. Logs can be deleted by date.

When a game fails to start, **Report error** saves what happened in `logs/reports.log`. It stays on
your server: to send it to us, attach it to an issue (see [Troubleshooting](troubleshooting.md)).

The other services (`gameflex-edge`, `gameflex-credential`, `gameflex-gateway`,
`gameflex-remoteplay`) log to Docker: `./launch.sh logs <service>`.

## Back up

Everything that is yours is in your library folder (`GAMEFLEX_LIBRARY`). Back up at least
`databases/` (accounts, settings) and `saves/`; `roms/`, `bios/` and `cores/` if you can't download
them again. `cache/` and `logs/` don't need a backup.
