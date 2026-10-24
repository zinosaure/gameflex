# Security

## Report a vulnerability

Please **don't open a public issue** for a security problem. Report it privately, from the
repository's **Security** tab: **Report a vulnerability**.

Give what you can of:

- the GameFlex version;
- what an attacker can do, and what they need first (an account, being on the same network…);
- the steps to reproduce it.

You will get an answer as soon as possible. GameFlex is a personal project: there is no bounty, and
no fixed delay, but security reports come first.

## Supported versions

Fixes go into the latest release. Keep GameFlex up to date (see [Update GameFlex](docs/update.md)).

## Running GameFlex safely

GameFlex is made for a home network. Some things to know:

- **The server controls Docker.** The `gameflex-kernel` service uses the Docker socket to start a
  container for each game, which gives it full control of Docker on that computer. Run GameFlex on a
  machine you trust, and never expose port 10201.
- **Keep GameFlex off the Internet if you can.** To play away from home, the optional WireGuard VPN
  needs a single port on your router (UDP 51820), and only the devices you add get in. Never forward
  its settings page (TCP 51821).
- **Without the VPN, only the web port is meant to be reached from outside**, through an HTTPS
  reverse proxy, plus the video ports (UDP 9201–9300). See
  [Play away from home](docs/remote-access.md).
- **Passwords:** after 5 failed sign-ins for an account from one address, GameFlex makes the next
  attempt wait a minute, then longer each time, up to 15 minutes; after 20 failures from any
  address, the account itself waits. Use strong passwords all the same.
- **Your secrets** are in `.env`: the administrator password and `GAMEFLEX_SECRET`. Keep the file
  private (`./launch.sh` writes it readable by you only), and never paste it in an issue.
- **Federation tokens** open your catalog to whoever holds them: games, BIOS files, emulators,
  patches, emulator settings and, if you tick it, your Remote Play consoles. Give them to people
  you trust, and revoke the ones you no longer need.
- **The Android app as a server** lets anyone on the same Wi-Fi read its list of games and download
  its game and BIOS files, without an account. Use it on a network you trust.
- **Network shares:** the passwords and tokens of Samba, WebDAV and kDrive shares are stored
  encrypted with `GAMEFLEX_SECRET`. Keep that secret private: with it and a copy of `databases/`,
  they can be read.
