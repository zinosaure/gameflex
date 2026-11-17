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

1. Open **Settings › Consoles**, then the console. Under **More**, open its emulator's settings
   (such as **RPCS3 settings**); retro emulators have none. The **File** section shows its state:
   *Missing* (nothing is applied yet) or *Unreadable* (nothing is applied, see below).
2. **Apply these settings** turns them on or off.
3. To start from a ready-made set, choose it in **Preset**: it fills the fields.
4. Change what you want. The form lists the settings, with one sentence under each.
5. **Settings per game engine** applies a preset, on top of these values, to every game of an engine
   (Unreal Engine, Unity…).
6. To keep these values for later, turn on **Save as preset** and give it a **Preset name**.
7. Press **Save**. The settings apply at the next game you start.

**Danger › Delete this preset** deletes the chosen preset, if it is one of yours.

## Share settings

Settings travel like the rest of the library. The **Preset** list also shows, in a group of their
own, the values and presets each internal disk and each federated server lends for this emulator
(see [Federation](federation.md)). Choosing one only fills the form; **Save** writes it here,
nothing changes over there.

## The settings in detail

The form says in one sentence what each setting changes. What follows is the rest: when to change
it, what it costs, and how it works.

### Settings found in several emulators

- **Follow the stream** (Render resolution): the emulator draws at the resolution chosen for the
  game. For RPCS3, whose games are usually 720p, 540p gives 75 % and 360p gives 50 %. For Eden,
  Citron and Lichii, whose console draws 1080p docked and 720p handheld, 540p gives 0.5× and 720p
  gives 0.75×. For xemu, it is ×1 up to 720p and ×2 for 1080p.
- **Adreno driver workaround (Turnip)** and **Adreno shader compiler (Turnip)**: these only act on
  devices whose graphics driver is Turnip. Try a driver workaround when a game's log ends with
  “Device lost”. *nopreamble* works around a known geometry bug of some emulators, RPCS3 among
  them, on Turnip.
- **Sound** (shadPS4, KytyPS5): when off, the emulator still decodes the audio but plays none of
  it, which is only slightly lighter.

### Box64 (shadPS4 and KytyPS5 on ARM devices)

Box64 is the translator that runs these x86-64 emulators on phones and other ARM devices. A server
runs the emulator directly and ignores every Box64 setting.

- **Translator (DynaRec)**: turning it off is useful to tell a translation fault from a fault in
  the game itself; the **Interpreter (exact)** preset does just that.
- **Processor shown to the game** (shadPS4): the PlayStation 4 has an AMD processor. Reporting one
  avoids the instruction rewriting the emulator otherwise performs on every game.
- **Game code in shared memory** (shadPS4): the emulator loads the game, its code included, into
  shared memory. By default Box64 checks every piece of that code each time it runs it, because it
  cannot see changes made through another view of the memory. When on, it protects that code like
  ordinary memory. PlayStation 4 games do not modify their own code.
- **Memory ordering**: ARM processors relax the x86 memory ordering; the translator restores it, more
  or less strictly.
- **Flags safety**: safer is slightly slower. With less than *Safest*, some games compare their
  memory wrongly and stop shortly after they start.
- **Aligned atomics**: off by default.
- **Translator, safest** preset: strong memory ordering, safest flags, smallest blocks and exact
  floating-point maths, the first thing to try when a game stops unexpectedly.
- **Default** preset (shadPS4): Box64's own values except four. The safest flags, which some games
  need to start; the call/return optimisation off, which makes games start faster; an AMD processor
  shown to the game, as on the console; and the game's code in shared memory protected instead of
  checked every time it runs.
- **Default** preset (KytyPS5): Kyty's occlusion queries, shader cache, unified memory and coarse
  shading on, and Box64's own values except two: the safest flags, which games need to start, and
  the call/return optimisation off.

### RPCS3 (PlayStation 3)

- **Console resolution**: games are told this resolution and choose their output from it. 720p, the
  resolution of most games, is the lightest; 1080p only helps the few games that offer it.
- **No occlusion queries**: the effects that can go wrong are lens flares and shadows.
- **Single-change presets** (Relaxed ZCULL, RPCS3 thread scheduling, SPU blocks: Giga, SPU loop
  detection, Two SPU threads at once): each differs from **Default** by one setting. Try one when a
  game runs too slowly, and compare its frame rate with that of **Default**.

### shadPS4 (PlayStation 4)

- **Pipeline cache**: a game still compiles everything the first time it runs.
- **Render height (Unreal Engine)**: the height is written to the game's saved configuration and
  applied when the game starts. The engine scales the image up to its output. Games made with other
  engines are not affected.
- **GPU readbacks**: a game that needs them shows a black or broken picture without them; its patch
  can turn them on.

### KytyPS5 (PlayStation 5)

- **Occlusion queries**: the game has to wait for the graphics processor's answer, which can slow
  it down on devices without unified memory. When off, every object is drawn: more graphics work,
  no waiting.
- **Shader cache**: the shaders are prepared again before the game's first image, so the game takes
  longer to start, and stutters less the first time a scene appears.
- **Compile shaders in the background**: compute shaders are still compiled immediately.
- **Unified memory (experimental)**: it needs the system's dma-buf heap (`/dev/dma_heap/system`),
  found on Linux servers; without it, the usual memory is used.
- **Console resolution**: games made with Unreal Engine draw their output images at this size.
  2160p (4K) costs far more for an image that the stream scales down anyway.
- **Coarse shading**: the colour of one pixel in four is copied to the three others. Turn it off for
  a sharper image when the stream is as large as the game's images.
- **Frame rate limit**: a game that runs faster than the stream gives its images unevenly, some are
  never seen, and the motion jumps. Without a limit, the game runs as fast as it can.
- **Run-ahead of the graphics processor**: counted at the points where a game marks its frames.
  With 1, a game waits there until the graphics processor has finished what came before. Higher,
  the emulator prepares the next image while the graphics processor finishes the current one: some
  games get faster, others stop working or show images that do not belong together. A game's patch
  raises it for a game whose images were checked with it.
- **Render height (Unreal Engine)**: the engine scales the image up to the screen and its dynamic
  resolution is turned off. Games made with other engines, and games that choose their own size,
  are not affected.
- **Render height reference**: a game's patch can set another value for a game that draws below its
  output, for example at half of it.
- **Render height variable**: a game that sets *r.ScreenPercentage* itself may need the other
  variable, through its patch.
- **Engine settings (Unreal Engine)**: lighter settings raise the frame rate of many games at the
  cost of detail. A game's patch can provide its own list.
- **Game code patch**: the format is that of GoldHEN patches, their JSON on one line. It applies
  only if the bytes it replaces are found. A game's patch provides it.

### Eden, Citron and Lichii (Switch and Switch 2)

- **Multi-core CPU emulation** (Eden): on ARM devices, it applies to native execution only. The
  recompiler always runs a single thread there, because games can break at start with several.
- **Native execution (ARM devices)**: the recompiler fallback runs one core, without fast memory
  access. Ignored on other processors.
- **Asynchronous presentation**, **Extended dynamic state** and **Dynamic vertex input** (Eden): on
  Android, Eden turns the first on and the other two off by default, as some drivers handle dynamic
  state badly.
- **Lichii** is derived from Eglantine and Eden.
- **Android** presets: Eden's values on Android devices (low GPU accuracy, no reactive flushing,
  asynchronous presentation, dynamic state off, handheld mode).
- **Performance** presets: handheld mode at 0.5×, shaders in the background, low GPU accuracy, no
  reactive flushing.

### Xenia (Xbox 360)

- **GPU readback**: games use it for automatic exposure or save thumbnails.
- **Occlusion queries**: they serve lens flares, culling and automatic exposure.
- **Performance** preset: fake occlusion queries, no readback, framebuffers.

## If something goes wrong

| What you see | What to do |
|---|---|
| A game no longer starts, or looks wrong, after a change | Choose another preset and save, or turn off **Apply these settings** to go back to the emulator's own configuration. |
| *The file is unreadable* | The file in `params/<console>/` was damaged, for example by a manual edit. The form shows the default list: **Save** replaces the file. |
| A preset can't be deleted or replaced | It is one of GameFlex's read-only presets: start from it, and save yours under another name. |
