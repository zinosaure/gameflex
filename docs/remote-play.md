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

1. Open **Remote Play** in the menu, choose your console (PlayStation 4 or PlayStation 5), and press
   **Add a console**.
2. **Device name:** any name, such as *Living Room PS5*.
3. **IP address:** press **Discover consoles on the network** and pick yours, or type its address
   (shown in the console's network settings).
4. **PSN ID (Base64):** your account ID. **Get via PSN Login** opens a website, outside GameFlex, that
   finds it from your PSN sign-in; copy the ID it gives you.
5. **PIN code:** on the console, open the Remote Play settings and choose to add (link) a device. The
   console shows an 8-digit code: type it here while it is on screen.
6. Press **Register device**. GameFlex shows *Device registered successfully.*

## Play

Your console shows its state: **Online**, **Standby** or **Offline**.

- **Turn on** wakes it from rest mode.
- **Play** starts the stream. Choose the picture quality first: 720p at 60 frames per second by
  default, from 360p to 1080p; **Auto** adapts the bitrate to your connection.
- **Watch** looks on while someone else plays.

One player holds the controller at a time; the others watch, and the controller can be lent or
given (see [Controllers and playing together](controllers.md)).

**Share it with a friend's GameFlex:** when you create their token, tick **Also share the consoles**
(see [Federation](federation.md)). To play on it, they also need an account on your server.

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
| *No consoles found on the network* | The console must be on and on the same network as the GameFlex server. Type its IP address instead. |
| *Failed to register the device.* | Type the PIN while the console still shows it (it expires), and check the PSN ID. |
| **Turn on** does nothing | In the console's power-saving settings, let it stay connected to the Internet in rest mode. |
| *The console just closed a session* | Wait a few seconds, then press **Play** again. |
| Black picture away from home | See [Play away from home](remote-access.md). |
