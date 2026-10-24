# Troubleshooting

**Goal:** find what goes wrong, fix the common problems, and report the others.

## Find out what happened

- **The state of the services:** `./launch.sh status` (or `docker compose ps`). Each one should be
  running.
- **A service's log:** `./launch.sh logs kernel` (or `edge`, `credential`, `gateway`,
  `remoteplay`).
- **A game's log:** **Settings › Logs**, one log per game, by day.
- **The connection:** in a game, the menu's **Show connection statistics** gives the frame rate,
  the bitrate and the machine's load.

## Common problems

| What you see | Where to look |
|---|---|
| GameFlex does not start, or a variable is missing | [Install GameFlex](install.md#if-something-goes-wrong) |
| A game does not appear in the library | [Organize your library](library.md#if-something-is-missing) |
| A console says its firmware is required, or a game shows a black screen | [Consoles](consoles.md) (the files each console needs), then the game's log |
| A console says it is not installed | Download its image: `./launch.sh consoles <name>` ([Install GameFlex](install.md)) |
| **Play** stays greyed out | The console's emulator is missing: **Settings › Emulators** ([Organize your library](library.md)) |
| The game stays on *Connecting to the stream* | At home: a firewall blocks UDP 9201–9300. Away: [Play away from home](remote-access.md) |
| The controller does nothing, or buttons are swapped | [Controllers and playing together](controllers.md#if-something-goes-wrong) |
| A recent console has no controller | Load the `uinput` module ([Install GameFlex](install.md)) |
| A friend's server refuses access | [Federation](federation.md#if-something-goes-wrong) |
| The PS4 or PS5 console does not answer | [Remote Play](remote-play.md#if-something-goes-wrong) |
| The disk is full | **Settings › Cache** and **Settings › Logs** ([Storage](storage.md)) |
| The administrator password is lost | `./launch.sh reset-password` sets it back to the one in `.env` (without the script: `docker compose exec gameflex-credential /app/auth reset-root-password`) |

Each console also has its known limitations: see [Consoles](consoles.md#known-limitations-in-this-version).

## Report a problem

Issues are welcome on the [issues page](https://github.com/zinosaure/gameflex/issues/new/choose).
Choose the form that fits, and give:

- your GameFlex version (`GAMEFLEX_VERSION` in `.env`, or `docker compose images`);
- where it happens: a browser (which one), the Android app, or the server;
- what you did, and what you saw, error messages word for word;
- the log: the game's log from **Settings › Logs**, or `./launch.sh logs kernel`;
- a screenshot if it helps.

**Before you paste anything,** remove passwords, tokens and your public address. Never share your
`.env` file, nor games, BIOS files or links to them.

A security problem? Don't open a public issue: see [SECURITY.md](../SECURITY.md).
