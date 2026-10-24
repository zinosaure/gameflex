# Emulator settings

**Goal:** tune an emulator (its resolution, its performance options, its compatibility switches)
without rebuilding anything, and keep or share the settings that work.

Emulator settings exist for the recent consoles' emulators: RPCS3 (PlayStation 3), shadPS4
(PlayStation 4), KytyPS5 (PlayStation 5), Eden and Citron (Switch), Cemu (Wii U), xemu (Xbox) and
Xenia Canary (Xbox 360). They work the same on a server and in the Android app.

## How it works

- **One file per emulator,** in `params/<console>/`, such as `params/ps4/shadps4.json`. It lists
  the emulator's settings, each with its label, its explanation, its value and its choices.
- **GameFlex applies it before every game** of that emulator, as long as **Apply these settings** is
  on. Turned off, the emulator goes back to its own configuration at the next launch.
- **Presets** are ready-made sets of values, in `params/<console>/presets/`. The ones GameFlex
  provides are read-only; you can save your own.
- **A preset only fills the form.** Nothing changes until you press **Save**, and a change applies
  at the next game you start.

## Change an emulator's settings

1. Open **Settings › Emulator settings**. Each emulator shows its state:
   - *Settings applied before every game*;
   - *Settings disabled*: the emulator keeps its own configuration;
   - *No file*: nothing is applied yet;
   - *Unreadable file*: nothing is applied (see below).
2. Choose an emulator. The form lists its settings, with an explanation under each one.
3. To start from a ready-made set, choose it in **Preset**: it fills the fields.
4. Change what you want.
5. To keep these values for later, turn on **Save as preset** and give it a name.
6. Press **Save**. The settings apply at the next game you start.

## Share settings

Settings travel like the rest of the library. In **Settings › Emulator settings**, choose an
internal disk or a friend's server as the **Source** (see [Federation](federation.md)): it shows
their values and their own presets. **Import** one: it becomes a preset of yours, named
`<source>-<preset>`, and replaces nothing.

## If something goes wrong

| What you see | What to do |
|---|---|
| A game no longer starts, or looks wrong, after a change | Choose another preset and save, or turn off **Apply these settings** to go back to the emulator's own configuration. |
| *Unreadable file* | The file in `params/<console>/` was damaged, for example by a manual edit. The form shows the default list: **Save** replaces the file. |
| A preset can't be deleted or replaced | It is one of GameFlex's read-only presets: start from it, and save yours under another name. |
