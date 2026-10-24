# Consoles

**Goal:** know, for each console, which folder to use, which game formats it accepts, and which
files it needs before it can start.

Each console has an **ID**. It is the name of its folders in your library: games go in
`roms/<ID>/`, BIOS and firmware in `bios/<ID>/`, the emulator in `cores/<ID>/`. See
[Organize your library](library.md) for the layout.

In every table, the BIOS and firmware files go in `bios/<ID>/`, **with exactly the names shown**.

## Retro consoles

Retro consoles run on libretro cores, which GameFlex downloads for you from the
[libretro buildbot](https://buildbot.libretro.com/), in **Settings › Emulators**. Every console
below also accepts a `.zip` archive that contains a single game file.

You can find the BIOS files below on [RetroBIOS](https://abdess.github.io/retrobios/) or on the
[mirror](../README.md#the-mirror). Rename them if needed: GameFlex looks for the exact names in this
table.

| Console | ID | Game formats | BIOS |
|---|---|---|---|
| Nintendo Entertainment System (NES) | `nes` | `.nes` `.fds` `.unf` `.unif` | none |
| Super Nintendo (SNES) | `snes` | `.smc` `.sfc` `.swc` `.fig` `.bs` | none |
| Game Boy / Game Boy Color | `gbc` | `.gb` `.gbc` `.sgb` | none |
| Game Boy Advance | `gba` | `.gba` `.agb` | none |
| Virtual Boy | `vb` | `.vb` `.vboy` | none |
| Nintendo 64 | `n64` | `.n64` `.z64` `.v64` `.u64` `.ndd` | none |
| Nintendo DS | `nds` | `.nds` `.dsi` | optional: `bios7.bin`, `bios9.bin`, `firmware.bin` |
| Nintendo GameCube | `gc` | `.iso` `.gcm` `.rvz` `.gcz` `.ciso` `.tgc` `.wia` `.dol` `.elf` | none (see below) |
| Nintendo Wii | `wii` | `.iso` `.rvz` `.wbfs` `.wad` `.gcz` `.ciso` `.wia` `.dol` `.elf` | `dolphin-emu/Sys/` (see below) |
| Sega Master System / Game Gear | `sms` | `.sms` `.gg` `.sg` | none |
| Sega Mega Drive / Genesis / 32X | `megadrive` | `.md` `.gen` `.smd` `.bin` `.32x` | none |
| Sega Saturn | `saturn` | `.cue` `.chd` `.iso` `.ccd` `.mds` | one of `sega_101.bin` (Japan), `mpr-17933.bin` (America, Europe), `saturn_bios.bin` |
| Sega Dreamcast | `dreamcast` | `.cdi` `.gdi` `.chd` `.cue` | `dc_boot.bin`, `dc_flash.bin` |
| PlayStation | `ps1` | `.cue` `.bin` `.chd` `.pbp` `.img` `.ecm` `.m3u` `.iso` | optional: `scph5500.bin` (Japan), `scph5501.bin` (America), `scph5502.bin` (Europe), `scph1001.bin`, `scph7001.bin`, `psxonpsp660.bin` |
| PlayStation 2 | `ps2` | `.iso` `.chd` `.cso` `.bin` `.cue` | a PlayStation 2 BIOS file (any name) |
| PlayStation Portable (PSP) | `psp` | `.iso` `.cso` `.pbp` `.chd` | `PPSSPP/` (see below) |
| Atari 2600 | `atari2600` | `.a26` `.bin` | none |
| Atari 7800 | `atari7800` | `.a78` `.bin` | none |
| Atari Lynx | `lynx` | `.lnx` `.o` | none |
| Atari Jaguar | `jaguar` | `.j64` `.jag` `.rom` `.abs` `.cof` | none |
| PC Engine / TurboGrafx-16 | `pce` | `.pce` `.cue` `.chd` `.sgx` | none |
| PC-FX | `pcfx` | `.cue` `.chd` `.ccd` | `pcfx.rom` |
| Neo Geo | `neogeo` | `.zip` `.chd` | `neogeo.zip` |
| Neo Geo Pocket | `ngp` | `.ngp` `.ngc` | none |
| Capcom CPS-1 | `cps1` | `.zip` | none |
| Capcom CPS-2 | `cps2` | `.zip` | none |
| WonderSwan | `wswan` | `.ws` `.wsc` | none |
| ColecoVision | `coleco` | `.col` `.cv` `.rom` `.bin` | `colecovision.rom` |
| Vectrex | `vectrex` | `.vec` `.bin` | none |
| Commodore 64 | `c64` | `.d64` `.t64` `.prg` `.crt` `.tap` `.g64` | none |
| Amstrad CPC | `amstrad` | `.dsk` `.sna` `.cdt` `.kcr` | none |
| ZX Spectrum | `zxspectrum` | `.tzx` `.tap` `.z80` `.sna` `.szx` | none |
| Doom | `doom` | `.wad` `.iwad` `.pwad` | none |

**Choosing a BIOS.** When a console has several BIOS files, the game's page lets you pick one
before you play. *Default* lets the emulator choose among the files you have. If a game shows a
black screen, pick the BIOS of the game's region.

**Folders named after an emulator.** Three consoles read a folder named after their emulator,
in `bios/`:

- **PSP** reads `bios/PPSSPP/`. Put `ppge_atlas.zim` and `ppge_atlas.meta` there, from the
  `assets` folder of PPSSPP. Without them, games run but their system dialogs have no text.
- **GameCube and Wii** read `bios/dolphin-emu/Sys/`: copy the whole `Sys` folder of Dolphin there
  (about 15 MB). The Wii shows nothing without it.

## Recent consoles

Recent consoles run on a full emulator, in `cores/<ID>/`: the file its publisher releases, under
the name GameFlex expects (**Settings › Emulators** shows it). The simplest is the download button in **Settings › Emulators**,
which gets the version tested with your version of GameFlex. A friend's server and the
[mirror](../README.md#the-mirror) have them too. If `cores/<ID>/` holds several versions, a game
starts with the most recent file (see [Organize your library](library.md)).

*Experimental* means: many games work, some crash or need a particular firmware, and things change
quickly.

| Console | ID | Emulator | Game formats | Firmware and keys |
|---|---|---|---|---|
| PlayStation 3 | `ps3` | [RPCS3](https://rpcs3.net) | `.iso` (a file, or in a folder), `.pkg` | `PS3UPDAT.PUP`, the official PlayStation 3 system update file (required) |
| PlayStation 4 *(experimental)* | `ps4` | [shadPS4](https://shadps4.net) | a folder with `eboot.bin` at its root (recommended), or `.pkg` | optional system modules (`.sprx` files, such as `libSceLibcInternal.sprx`), directly in `bios/ps4/` |
| PlayStation 5 *(experimental)* | `ps5` | [KytyPS5](https://github.com/KytyPS5/KytyPS5) | a folder with `eboot.bin` at its root, `.zar`, `.elf` | none |
| Nintendo Switch *(experimental)* | `switch` | [Eden](https://eden-emu.dev), or [Citron](https://github.com/citron-neo/emulator) | `.xci` `.nsp` | `prod.keys` (required), `title.keys` (optional) |
| Nintendo Wii U *(experimental)* | `wiiu` | [Cemu](https://cemu.info) | `.wux` `.wud` `.wua` (a file, or in a folder named after the game), or an extracted game (a folder with `code/`, `content/` and `meta/`) | `keys.txt` (required) |
| Xbox | `xbox` | [xemu](https://xemu.app) | `.iso` in XISO format (a file, or in a folder) | `mcpx_1.0.bin`, `Complex_4627.bin`, and the hard disk image `xbox_hdd.qcow2` |
| Xbox 360 *(experimental)* | `xbox360` | [Xenia Canary](https://github.com/xenia-canary/xenia-canary) | `.iso` (a file, or in a folder) | none |

**PlayStation 4 packages.** A `.pkg` is unpacked the first time you play it. It takes a while, and
the unpacked game takes as much space again. Keep your games unpacked when you can: put the
unpacked folder, with its `eboot.bin`, in `roms/ps4/`. **Settings › Cache** shows the unpacked
copies and lets you delete them.

**Wii U graphic packs.** Some games need a graphic pack to run well. Put each pack in its own
folder, in `patches/wiiu/<pack name>/` (the folder holds a `rules.txt`).

## Remote Play

Remote Play brings **your own PlayStation 4 or PlayStation 5** to GameFlex. It plays the console's
own games: nothing goes in your library.

| Console | ID |
|---|---|
| PlayStation 4 | `rp-ps4` |
| PlayStation 5 | `rp-ps5` |

## Known limitations in this version

- **PSP, GameCube and Wii:** the latest cores on the official libretro server may not start
  (PPSSPP stops when a game loads; Dolphin needs a newer system library). Take these cores from
  the [mirror](../README.md#the-mirror) instead.
- **Neo Geo, CPS-1 and CPS-2:** arcade game sets (`.zip`) do not start yet.
- **PlayStation 3:** extracted game folders (`PS3_GAME/…`) are not supported. Use an `.iso` or a
  `.pkg`.
- **Switch:** GameFlex does not install the console's firmware, and does not install updates or
  downloadable content. An extra `.nsp` shows up as a separate game.
- **Xbox 360:** extracted games (`default.xex`) and GOD containers are not supported. Use an
  `.iso`.
- **PlayStation:** some European games are protected and need a subchannel file (`.sbi` or `.sub`)
  next to the disc image.
