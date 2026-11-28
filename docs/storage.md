# Storage: disks, shares, saves and logs

**Goal:** add more disks of games, read games from a network share, and keep an eye on saves, the
cache and the logs.

Most of it is in **Settings**, for administrators:

| Screen | What it shows |
|---|---|
| **Resources** | The other disks of games plugged into the server, and the network shares and cloud folders it reads. |
| **Consoles** | Each console's emulators, BIOS files, games and settings: see [Organize your library](library.md) and [Emulator settings](emulator-settings.md). |
| **Members**, then a member, **Their saves** | The room an account's saves take. |
| **Profile › My saves** | Your own saves, for every account. |
| **Cache** | What a game's first launch installed, and its size. |
| **Logs** | What the server writes, and what happened, without a terminal. |
| **Rescan everything** | Refreshes the list of everything in your library, after you copy files by hand. |

## More disks of games

Your games don't all fit on one disk? Add others, as many as you like.

1. Lay out the disk like a library: `roms/<console>/`, and if you want `bios/`, `cores/`,
   `patches/`, `params/` — at its root, or in a `gameflex` folder if the disk holds other things.
2. Mount your disks on the server's computer inside one folder, one mount point per disk (for
   example with `/etc/fstab`): `/mnt/gameflex/games`, `/mnt/gameflex/archive`… Each mount point is a
   disk, under its name (`games` here: it starts with a letter or a digit, and uses letters, digits,
   `.`, `-` and `_`).
3. Tell GameFlex where that folder is, in `.env`, then restart (`docker compose up -d`):

   ```
   GAMEFLEX_DISKS=/mnt/gameflex
   ```

   This is the only setting: a disk mounted there later shows up by itself.

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
- **A disk shown as *Missing*** is unplugged, or has no games folder. For an external disk, prefer a
  fixed mount point (`/etc/fstab`) to the one your desktop creates when you sign in.

### Unplug a disk

Open the disk in **Settings › Resources** and turn off **Disk enabled**: GameFlex stops using it,
its games leave the lists, and you can unmount and unplug it. GameFlex refuses while a game in
progress plays from it. Plug it back in and mount it at the same place: it shows up by itself; turn
**Disk enabled** back on and its games return. Always turn the disk off first — the computer itself
lets you unplug a disk that a game is reading.

## Games on a network share

GameFlex reads games from a NAS, a server or a cloud folder by itself: **Settings › Resources ›
Add an external resource**, then its kind — Samba, WebDAV, kDrive, FTP or SFTP.

1. Give it a name, its address, and the folder that holds your games, if they are not at the root.
   That folder holds a `roms` folder, laid out by console like your library: `roms/<console>/`.
2. Choose **Anonymous**, or **With an account** and give its name and password. The password stays
   on the server, encrypted.
3. Save: GameFlex reads its list of games in the background.

**What you should see:** the resource in **Settings › Resources**, with its status and its number
of games. Open it to see its games by console; **Read again** reads it after you add games to it.
**Edit the connection** changes its address, folder or account.

- **Its games are downloaded, not played from the share:** in **Settings › Consoles**, a console,
  **Games**, choose the resource as the source, then download a game into your library. A game you
  already have says so.
- **The Catalog** lists its games too, to download onto your device.
- **Your friends** see the games only your resources hold, with the rest of your library, and can
  download them.
- **A resource that can't be read** says why on its page: the server does not answer, the account is
  refused, or the folder is not found. An empty one suggests a folder where it found games.

You can also mount a share on the server's computer yourself (for example NFS, with a line in
`/etc/fstab`), inside your disks folder (see above): it is then a disk, and its games play from it.
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
game with that ID in your library or on an internal disk, and shows its title. Every line starts
with its console, then where its game is (the disk, or *Library*).

The name stays with the save. Each place that holds saves keeps a `titles.json` file, with the
title, the ID and the path of each save's game, written when you play, when the list names a save,
and when a save is backed up, restored, sent or downloaded. A save whose game has left your
library, or a copy from a server that does not have the game, keeps its name, and its page shows
the game's old **Path**. With nothing kept, the save shows the title it gives itself, if any, then
its ID.

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
`gameflex-remoteplay`) log to Docker: `./launch.sh logs <service>`.

## Back up

Everything that is yours is in your library folder (`GAMEFLEX_LIBRARY`). Back up at least
`databases/` (accounts, settings) and `saves/`; `roms/`, `bios/` and `cores/` if you can't download
them again; `metadata/` and `status/` if you corrected sheets or set statuses. `cache/` and `logs/`
don't need a backup.
