# Storage: disks, shares, saves and logs

**Goal:** add more disks of games, read games from a network share, and keep an eye on saves, the
cache and the logs.

Most of it is in **Settings**, for administrators:

| Screen | What it shows |
|---|---|
| **Resources** | The other disks of games plugged into the server. |
| **Consoles** | Each console's emulators, BIOS files, games and settings: see [Organize your library](library.md) and [Emulator settings](emulator-settings.md). |
| **Members**, then a member, **Their saves** | The room an account's saves take. |
| **Profile › My saves** | Your own saves, for every account. |
| **Cache** | What a game's first launch installed, and its size. |
| **Logs** | What the server writes, and what happened, without a terminal. |
| **Rescan everything** | Refreshes the list of everything in your library, after you copy files by hand. |

## More disks of games

Your games don't all fit on one disk? Add others, as many as you like.

1. Lay out the disk like a library: `roms/<console>/`, and if you want `bios/`, `cores/`,
   `patches/`, `params/`.
2. Add one volume line per disk to the `gameflex-kernel` service in `docker-compose.yml`, under
   `/mnt/<name>`:

   ```yaml
       volumes:
           - /media/games:/mnt/games
   ```

   The name (`games` here) starts with a letter or a digit, and uses letters, digits, `.`, `-` and `_`.
   `./launch.sh update` keeps these lines when it brings a new `docker-compose.yml`.
3. Restart: `docker compose up -d`.

**What you should see:** the disk in **Settings › Resources**, with its number of games. Open it
to see its games; **Rescan the disk** finds what you added to it by hand. Its games show in the
library of their console, next to the others.

- **Its games play from the disk,** with the emulators and BIOS files of your library. Its emulator
  settings show as presets in your settings forms, and its games' patches can be downloaded into
  your library from a game's **Patch** page.
- **GameFlex writes on a disk only** to remove a game you ask it to remove, and to keep the sheets
  and images of its games in its `metadata/` folder. For a read-only disk, turn off
  **Settings › Metadata provider › Keep metadata on the games' disk**: they then go in your library.
- **A game on two disks** plays from your library first, then from the disks in the order of their
  names. It keeps its saves.
- **Your friends** see the games of your disks with the rest of your library.
- **A disk shown as *Missing*** was unplugged, or not mounted when GameFlex started. For an external
  disk, prefer a fixed mount point (`/etc/fstab`) to the one your desktop creates when you sign in.

## Games on a network share

GameFlex reads a network share (Samba, NFS, WebDAV…) as one more disk: mount it on the server's
computer first, then declare the mount point like any disk (see above).

1. Mount the share on the computer that runs GameFlex, at a fixed place, for example with a line in
   `/etc/fstab`. Your system's documentation, or your NAS's, explains how.
2. Add its volume line to `gameflex-kernel`, as for a disk: `- /media/nas-games:/mnt/nas`.
3. Restart: `docker compose up -d`.

Mount it before GameFlex starts: a share mounted later shows as *Missing* until the next restart.
If the share is read-only, turn off **Keep metadata on the games' disk** (above).

## Saves

**Settings › Members**, then a member, **Their saves**, shows how many files that account's saves
hold and their size. **Download** gives them as one archive; deleting removes every save of that
account, games in progress included. Each player also sees their own in **My saves**.

### My saves

**Settings › Profile › My saves** starts with where your saves are:

- **On this server**: *This server* — the saves your games use —, *Backups*, and a copy for each
  federated server that sent you saves or that you downloaded from;
- **Federated servers**: your saves on each of them, under your account there.

Each save is named after its game. An emulator stores the save of a recent console's game (Switch,
PlayStation 3, 4 and 5) under the game's title ID, such as `01000EA014150000`; GameFlex finds the
game with that ID in your library or on an internal disk, and shows its title. When no game has it,
the save keeps the title it gives itself, if any, and its ID last. Every line starts with its
console, then where its game is (the disk, or *Library*).

From *This server*, **Backup** sets a save aside in *Backups*, in place of the previous backup of
that save. You can add a short **Note** to it — why you made it, before which part of the game —,
kept in a `note.json` beside the backup with its date and your account. The note shows in the
backups' list and on the backup's page, where **Edit the note** changes or removes it. A newer
backup of the same save replaces the old one and its note.

**Restore** puts a backup or a copy back into your saves; the saves it replaces go into *Backups*
first. **Send to…** copies a save to a federated server, where it lands in a copy under your
account: nothing there replaces your saves. A game only ever plays the saves of *This server*.

## Cache

The first launch of some games installs data, once for every account: an unpacked PlayStation 4
game, the PlayStation 3 firmware and game data, compiled shaders. **Settings › Cache** lists each
entry with its console, its kind and its size, and removes it. Everything there is rebuilt by the
next launch that needs it, but rebuilding takes time. An entry can't be removed while a game of its
console is running.

## Logs

**Settings › Logs** starts with **Writing**, what the server writes from now on:

- **Server log**: what the server does — games launched, downloads, errors —, one file per day,
  with the **Levels to write** (Debug, Info, Warn, Error) and, if you need them, the
  **HTTP requests**;
- **Game session logs**: what the emulator writes during a game, one file per game;
- **Reports**: what players send with **Report the error** when a game fails to start, in
  `logs/reports.log`. Turned off, the button goes away.

**Show logs** lists them, one per day and one per game. Open one to read its last lines, search
them, or filter them by level; **Download** gives the whole file. Logs can be deleted one by one,
or all those a period filter leaves visible.

A report stays on your server: to send it to us, attach it to an issue (see
[Troubleshooting](troubleshooting.md)).

The other services (`gameflex-edge`, `gameflex-credential`, `gameflex-gateway`,
`gameflex-remoteplay`) log to Docker: `./launch.sh logs <service>`. gameflex-federation shows its
own on its page, under **More › Logs**.

## Back up

Everything that is yours is in your library folder (`GAMEFLEX_LIBRARY`). Back up at least
`databases/` (accounts, settings) and `saves/`; `roms/`, `bios/` and `cores/` if you can't download
them again; `metadata/` and `status/` if you corrected sheets or set statuses. `cache/` and `logs/`
don't need a backup.
