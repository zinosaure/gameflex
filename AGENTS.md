# AGENTS.md: GameFlex for AI agents

You are helping someone install, run or troubleshoot **GameFlex**, a self-hosted cloud gaming
server. This repository is the **distribution only**: Docker images on Docker Hub
(`zinosaure/gameflex-*`), a compose file, a setup script and documentation. There is no source code
to build here.

The human guides are in `docs/`. This file tells you how to do the same work safely on someone's
behalf.

## Ground rules

- **Ask, don't invent,** for anything that belongs to the person: where their games are, their
  administrator password, which consoles they want.
- **Never print, log, commit or paste** the administrator password, `GAMEFLEX_SECRET` or `.env`.
  If you generate the password, show it to the person once and tell them to keep it.
- **Never download games, BIOS or firmware for the person,** and never search for them. Point to
  [Where to find emulators and BIOS files](README.md#where-to-find-emulators-and-bios-files) and
  let them decide.
- **Ask before anything that changes the machine beyond this folder:** installing Docker, loading a
  kernel module, opening firewall or router ports, using `sudo`.
- **Do not expose GameFlex to the Internet** unless the person asks; then follow their reverse
  proxy setup. Never publish port 10201.

## Install

1. **Check the machine** (Linux, x86-64):

   ```bash
   uname -m                      # x86_64
   docker compose version        # Docker Compose v2 is required
   docker info >/dev/null        # Docker answers without sudo
   ls /dev/dri                   # a graphics card for video encoding
   ls /dev/uinput                # controllers for the recent consoles (else: sudo modprobe uinput)
   ```

   If something is missing, explain it and ask before fixing it. The details are in
   [docs/install.md](docs/install.md).

2. **Ask the person** for:
   - the folder for their library (games, BIOS, saves), as a full path;
   - an administrator password, or their agreement to generate one;
   - the recent consoles they want, among `ps3 ps4 ps5 switch wiiu xbox xbox360` (none is fine).

3. **Find the machine's address** on the local network:

   ```bash
   ip -4 route get 1.1.1.1 | awk '{for (i = 1; i < NF; i++) if ($i == "src") print $(i + 1)}'
   ```

4. **Run the setup without questions.** `launch.sh` reads its answers from the environment when
   stdin is not a terminal. Pass the password through the environment, never as a visible
   argument:

   ```bash
   mkdir -p ~/gameflex && cd ~/gameflex
   curl -fLO https://raw.githubusercontent.com/zinosaure/gameflex/main/launch.sh
   chmod +x launch.sh
   GAMEFLEX_LIBRARY=/path/to/library \
   GAMEFLEX_PUBLIC_HOST=<address from step 3> \
   GAMEFLEX_ROOT_PASSWORD=<password> \
   COMPOSE_PROFILES=<consoles, comma-separated, or empty> \
   ./launch.sh < /dev/null
   ```

   The script writes `.env` (mode 600), generates `GAMEFLEX_SECRET`, downloads the images and
   starts GameFlex. Without a password it generates one and prints it: hand it to the person.

5. **Verify:**

   ```bash
   ./launch.sh status          # gameflex-edge, -kernel, -credential, -gateway, -remoteplay: running
   curl -fsS -o /dev/null http://127.0.0.1:10200/ && echo ok
   ```

   Then tell the person to open `http://<address>:10200` and sign in as `root`. The administrator
   password is set again from `.env` at every start: to change it, edit `.env` and run
   `docker compose up -d`, never only in the web page.

## Everyday tasks

| Task | Command |
|---|---|
| Start | `./launch.sh` |
| Add recent consoles | `./launch.sh consoles ps3 switch` |
| Update | `./launch.sh update` |
| Stop (data stays) | `./launch.sh stop` |
| What runs | `./launch.sh status` |
| Read a log | `docker compose logs --tail 200 gameflex-kernel` (services: `gameflex-edge`, `gameflex-kernel`, `gameflex-credential`, `gameflex-gateway`, `gameflex-remoteplay`) |
| Change a setting | edit `.env`, then `docker compose up -d` |

`./launch.sh logs` follows the log forever: prefer `docker compose logs --tail` in a script.

## Where things are

| You need | Look in |
|---|---|
| Settings, volumes, ports | [docs/install.md](docs/install.md), the *Reference* section |
| Where games, BIOS and emulators go | [docs/library.md](docs/library.md) |
| Each console's folder name, formats and required files | [docs/consoles.md](docs/consoles.md) |
| The Android app | [docs/android.md](docs/android.md) |
| Sharing with another GameFlex (tokens, servers) | [docs/federation.md](docs/federation.md) |
| Emulator settings and presets | [docs/emulator-settings.md](docs/emulator-settings.md) |
| Common problems | the *If something goes wrong* tables of each guide |
| The library on disk | `GAMEFLEX_LIBRARY` in `.env`: `roms/<console>/`, `bios/<console>/`, `cores/<console>/`, `saves/`, `logs/` |

After the person copies files into the library by hand, GameFlex needs a refresh: **Settings ›
Rescan all** in the web page.

## Report an issue

Only with the person's agreement: an issue is public.

1. **Gather:** the GameFlex version (`grep GAMEFLEX_VERSION .env`, `docker compose images`), the
   machine (distribution, CPU, graphics card), what the person did and saw, and the relevant log
   (`docker compose logs --tail 200 gameflex-kernel`).
2. **Redact:** remove passwords, tokens, `GAMEFLEX_SECRET`, public IP addresses and personal paths.
   Never attach `.env`. Never mention or link game or BIOS files.
3. **Follow the forms** in `.github/ISSUE_TEMPLATE/`: *Bug report*, *A game or a console does not
   work*, *Idea or request*. Use their fields as the headings of the body.
4. **Submit:** give the person
   <https://github.com/zinosaure/gameflex/issues/new/choose> with the text ready to paste, or, if
   they want you to and `gh` is signed in:

   ```bash
   gh issue create --repo zinosaure/gameflex --title "<short summary>" --label bug --body-file issue.md
   ```

   Show them the final text first.
