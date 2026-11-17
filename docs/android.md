# GameFlex on Android

The GameFlex app turns an Android phone or tablet into two things at once:

- **A GameFlex of its own.** The phone keeps a library, runs the emulators itself, and plays your
  games without any computer. Retro consoles run on most phones; recent consoles need a powerful
  one.
- **A server for the people around you.** While the app runs, the phone serves GameFlex on your
  Wi-Fi. Friends open its address in their browser, on their own phone, tablet or computer, and
  play with you or watch. Nothing to install for them.

It also connects to your GameFlex server at home, to play its games and get its emulators.

<p align="center"><img src="images/phone-playing.png" alt="A NES game on a phone, with the on-screen controller" width="300"></p>

## What you need

- **Android 8.0 or later, 64-bit.**
- **For the recent consoles** (PlayStation 3 to 5, Switch, Wii U, Xbox): a phone with a Qualcomm
  Snapdragon chip (Adreno graphics) and plenty of memory. On other phones they run far too slowly:
  play them from your GameFlex server instead.
- **Space for your games**, on the phone, an SD card or a USB key.
- Optional: a Bluetooth or USB controller.

## Install the app

1. Download `gameflex.apk` from the
   [Releases page](https://github.com/zinosaure/gameflex/releases/latest), on the phone.
2. Open it. Android asks, the first time, to allow installing apps from your browser or file
   manager: allow it.
3. Open GameFlex. It asks for two permissions:
   - **notifications**, to show that the server is running;
   - **access to all files**, to read the games in your library. Without it the library stays
     empty.

The app updates itself: on the home screen, **About** › **Update** › **Check for an update**. It also
checks when it opens, and when you come back to it after a few hours: an orange exclamation mark at
the end of the **Update** line means a newer version is ready to install.

## The home screen

The app opens on its home screen, in English, or in French on a phone set to French. It looks like
GameFlex's settings: the controller button at the top right and the GameFlex banner open GameFlex
itself, your games and your settings (both are greyed out while the server is stopped). Each line
opens its own page; the Back button comes back.

| Part | What it does |
|---|---|
| **The four tiles** | **Start / Stop the server**, **Restart**. **Invite to join**: the phone's address, as a QR code and as text, and **Share the address**. **Join a server**: open another GameFlex in the app, by its QR code or its address; the servers you joined stay listed. |
| **Storage › Location** | Where your library is, and how much space is left. **Change location** uses another folder: accounts, saves and logs are copied there; games, BIOS files and cores are not. **How it works**, in its Guide section, explains the folders. |
| **Storage › External disk** | SD cards and USB keys with games. **Add a disk**; a disk's page removes it. |
| **Root account** | The root username, how many administrator and player accounts the server has, and **Reset the root password**, shown once in a window with **Copy**. With no administrator yet, it asks you to sign in as root first to create one. |
| **About › Update** | The app's version and build date, and installs the latest version; an orange exclamation mark means one is available. |
| **About › Phone** | The phone's model, Android, processor, memory and GPU, and whether it can run the recent consoles. |

## First start

1. In **Root account**, press **Reset the root password**, then **Generate**. Copy the password
   shown: it is shown only once.
2. Press the controller button at the top right and sign in as `root` with this password.
3. In **Settings › Members**, create your own administrator account, and accounts for the people
   who will play with you.

## Put your games on the phone

Your library is the `gameflex` folder of the phone's storage, unless you chose another place in
**Storage**. It is organized like on a server: `roms/<console>/` for the games,
`bios/<console>/` for the BIOS files. See [Organize your library](library.md) and
[Consoles](consoles.md).

**Games on an SD card or a USB key:** on the home screen, **External disk** › **Add a disk**,
pick the disk, and give it a name. The disk needs a `roms/` folder (or `bios/`, `cores/`…) laid out
the same way. A read-only disk, such as an NTFS USB key, works: GameFlex never writes to it, except
to delete a game you ask it to.

## Get the emulators

In GameFlex, open **Settings › Consoles**, then the console, and open the emulator marked
*Not installed*. Under **Get it**, choose a source:

- **Retro consoles:** the **Official source**, the
  [libretro buildbot](https://buildbot.libretro.com/), or your GameFlex server if you added one.
- **Recent consoles:** your GameFlex server. It gives the version for phones of the same emulator
  your server uses.

**Play** stays greyed out while a console's emulator is missing.

## Play on the phone

Open a game and press **Play**.

- **The on-screen controller** appears on the touchscreen. It hides when you connect a Bluetooth or
  USB controller; map your controller in **Settings › Gamepads**.
- **The game menu:** slide the menu button, at the bottom right of the screen, to the left, or, on a
  controller, hold **L2** and **Home** together for three seconds.
- **A controller that clips onto the phone:** in **Settings › Application › Orientation** (or **Orientation** in the game menu), choose **Portrait, half screen**. The game fills the top half, above the
  controller.

## Play together: your phone as the server

While the app runs, other devices on **the same Wi-Fi** can join the games of your phone.

**To invite someone to a game:**

1. Start the game on your phone.
2. Open the game menu, then **Send the link** (or **Copy the link**, or show the **QR code**).
3. Your friend opens the link in a browser. They arrive as a spectator, with no account needed. To
   play, they sign in, then press **Take the controller**, or **Ask for the controller** when all
   places are taken: you answer from your phone.

**To let someone use GameFlex on their own device:** create an account for them in **Settings ›
Members**. They open the address shown under **Invite to join** on the home screen, or in **Settings ›
Invite**, such as `http://192.168.1.40:10200`, or scan its QR code, and sign in. They can then join or watch the
games in progress from the dashboard.

**If they have the app too**, they join from it rather than from a browser: on their home screen,
**Join a server**, then **Scan a QR code** (Google's reader, which needs Google Play services and
installs itself the first time), or type its address and press **Join**. The app checks that a GameFlex
answers there, then opens it: they play under their account on your phone, as in a browser. To come
back to their own home screen: the Back button, or **Settings › App home**. The servers they joined
stay listed under **Servers you joined**, where a server's page opens it again or removes it; the
controller button at the top right brings back their own GameFlex. Someone without the app, or with an iPhone, scans the same QR code
with the camera and joins in the browser.

Good to know:

- **Only an administrator starts a game** on the phone, and one game runs at a time. The others
  join it: the number of players depends on the console.
- **Sound for the others:** the first time someone joins from another device, Android asks you to
  allow screen recording. Allow it, or they hear nothing.
- **The phone keeps serving with the screen off.** A notification, *GameFlex — server*, shows it
  runs; its **Stop** button stops everything.
- **Use a network you trust.** On your Wi-Fi, anyone can read the list of your games and their
  files from the phone, without an account.

## Connect to your GameFlex server

1. On your server, create an invitation for your phone (**Settings › Federation › Delivered
   accesses › Create a new access**).
2. On the phone, open **Settings › Federation › Joined servers › Join a server**, and paste the
   **Invitation link**, or choose **Address and token** and fill in the **Address** and the
   **Access token**. Press **Save**.

The server's games appear in your libraries and in the catalog. To play them on the server, which
streams them to the phone, sign in at that server once: **Settings › Profile › Federation › Sign in
at federated servers**, with the invitation or the token of your account over there (**Settings ›
Profile › Federation › Create my token** on the server). You can also copy a game to the phone, to
play it without the server.

## Remote Play: your PS4 or PS5

The phone connects to your own PlayStation 4 or 5 on the same network. Open **Settings › Remote
Play › Add a console**, and follow the steps: the console's address (or **Discover consoles on the
network**), your PSN ID (**Get it by signing in to PSN**), and the PIN code the console shows in its
**Settings › Remote Play › Add Device**. Then, in the **Remote Play** tab, **Turn on** and **Play**.
See [Remote Play](remote-play.md).

## Known limitations

- The Xbox 360 is not available on the phone: play it from your GameFlex server.
- The recent consoles are experimental on the phone, and need a Snapdragon phone.
