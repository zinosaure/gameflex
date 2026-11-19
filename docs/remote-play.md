# Remote Play: your PS4 or PS5

**Goal:** play your own PlayStation 4 or PlayStation 5 through GameFlex, from a browser or the app,
at home or away. The console runs its own games; GameFlex brings its picture to you and sends your
controller back.

**Before you start:**

- **The console and the GameFlex server are on the same home network.** Players can be anywhere.
- **Remote Play is turned on in the console's settings.** To let GameFlex wake the console, let it
  stay connected to the Internet in rest mode (in its power-saving settings).
- **Your PSN account ID**, a short code in Base64 (not your PSN user name). The steps below show how
  to find it.

## Add your console

An administrator adds it once, in **Settings › Remote Play**:

1. Press **Add a console**.
2. **Console name:** any name, such as *Living room PS5*.
3. **IP address:** press **Discover consoles on the network** and touch yours, or type its address
   (shown in the console's network settings).
4. **PSN ID (Base64):** your account ID. **Get it by signing in to PSN** opens a website, outside GameFlex, that
   finds it from your PSN sign-in; copy the ID it gives you.
5. **PIN code:** on the console, open **Settings › Remote Play › Add Device**. The console shows an
   8-digit code: type it here while it is on screen.
6. Press **Register the console**. GameFlex shows *Console registered.*

The console's page, in the same place, shows its details, lets you change its **Background image**,
and **Unlink this console**.

## Play

The **Remote Play** tab, at the top, shows each console with its state: **Online**, **Standby** or
**Offline**.

- **Turn on** wakes it from rest mode.
- **Play** starts the stream. The picture quality is in the console's settings (the gear): 540p at
  30 frames per second by default, up to 1080p at 60, with an automatic bitrate that follows your
  network. It is kept in this browser only.
- **Busy · Watch** looks on while someone else plays.

One player holds the controller at a time; the others watch, and ask for it to play (see
[Controllers and playing together](controllers.md)).

**To finish,** **Quit the game** offers **Put the console to sleep, then quit**: you watch the
console go to sleep, and the game ends by itself once it is off, as it does whenever the console is
turned off. **Quit and leave the game running** leaves it on for the others.

**Share it with a friend's GameFlex:** your Remote Play consoles show on a server you give access to. Which
ones it can play is set in that access's **Console access** (see
[Choose the consoles of an access](federation.md#choose-the-consoles-of-an-access)). To play on it, they also need
an account on your server.

## Limitations

- The picture is SDR, in H.264. HDR is not supported.
- Motion sensors are not sent. The touchpad of a real controller only clicks; the on-screen
  touchpad works.
- DualSense haptics and adaptive triggers are simplified to plain rumble.
- One player at a time: there is no split play.
- Remote Play has been tested with a PlayStation 5 only; the PlayStation 4 is untested.

## If something goes wrong

| What you see | What to do |
|---|---|
| *No console on the network.* | The console must be on and on the same network as the GameFlex server. Type its IP address instead. |
| *The console could not be registered.* | Type the PIN while the console still shows it (it expires), and check the PSN ID. |
| **Turn on** does nothing | In the console's power-saving settings, let it stay connected to the Internet in rest mode. |
| *The console just closed a session: try again in a few seconds.* | Wait a few seconds, then press **Play** again. |
| Black picture away from home | See [Play away from home](remote-access.md). |
