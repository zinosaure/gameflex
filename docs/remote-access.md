# Play away from home

**Goal:** reach your GameFlex from outside your home, on a phone's mobile data or at a friend's.

## Two ways

| | The WireGuard VPN (recommended) | An HTTPS proxy |
|---|---|---|
| GameFlex on the Internet | no: only the devices you add can reach it | yes: anyone can reach its sign-in page |
| On the router | one port: UDP `51820` | UDP `9201`–`9300`, and the proxy's HTTPS port |
| For each player | the WireGuard app, and a device you add for them | a browser, nothing else |
| You need | your home's public address, or a name pointing to it | a domain name and a certificate |

Two things travel between a player and your server: **the page and its controls** (port `10200`),
and **the game's picture and sound**, over WebRTC (UDP ports `9201` to `9300`). The VPN carries
both inside one encrypted tunnel; the proxy way opens both to the Internet.

## With the WireGuard VPN

GameFlex comes with an optional [WireGuard](https://www.wireguard.com) server,
[wg-easy](https://github.com/wg-easy/wg-easy). It is not part of GameFlex: it is downloaded from
its own publisher, under its own license. Once a phone or a laptop joins it, it opens GameFlex at
the same address as at home.

**Before you start:**

- GameFlex runs at home (see [Install GameFlex](install.md)).
- Your home's public address, or a domain name pointing to it. Most home connections change their
  address from time to time: a free dynamic DNS name keeps up with it.
- Access to your router's port forwarding settings.
- **Already running a WireGuard server at home?** You don't need this one: let its devices reach
  the address of GameFlex (`GAMEFLEX_PUBLIC_HOST`) in their *Allowed IPs*.

### 1. Turn it on

```bash
./launch.sh wireguard
```

Without the script: add `wireguard` to `COMPOSE_PROFILES` in `.env`, then run `docker compose up -d`.

### 2. Set it up

From your home network, open `http://<GAMEFLEX_PUBLIC_HOST>:51821`, for example
`http://192.168.1.50:51821`:

1. Create its administrator. Its password needs at least 12 characters.
2. When it asks about an existing setup, answer **No**.
3. **Host:** your home's public address, or the domain name pointing to it. **Port:** keep `51820`.

This page uses plain HTTP: keep it on your home network.

### 3. Forward one port on your router

Forward **UDP port 51820** to the address of your GameFlex computer (`GAMEFLEX_PUBLIC_HOST`).
Nothing else: never forward `51821`, `10200` or the video ports.

### 4. Add a device

1. On the VPN's page, create a client for each device, with a name you will recognize (*Anna's
   phone*).
2. On a phone or a tablet, install the WireGuard app (Android, iOS), tap **+**, then scan the
   client's QR code. On a computer, install WireGuard, then import the client's configuration file.

Each device only sends what goes to GameFlex through your home: the rest of its traffic stays on its
own network. To give a device your whole home network instead, change its *Allowed IPs* on the
VPN's page.

### 5. Play

Turn the tunnel on in the WireGuard app, then open GameFlex at its home address,
`http://<GAMEFLEX_PUBLIC_HOST>:10200`.

### What you should see

From a phone on mobile data, with Wi-Fi off and the tunnel on: the WireGuard app shows a recent
*handshake*, the page opens at the home address, and a game shows its picture within a few seconds.

## With an HTTPS proxy

**Before you start:**

- GameFlex runs at home (see [Install GameFlex](install.md)).
- A domain name pointing to your home's public address, such as `games.example.com`.
- Access to your router's port forwarding settings.

GameFlex does not include a relay (TURN) server yet. Most home connections work; some networks
cannot reach your server directly (some mobile carriers, company networks). There, the page
connects but the game stays on *Connecting to the stream*.

### 1. Put an HTTPS reverse proxy in front of GameFlex

Any reverse proxy works (Nginx Proxy Manager, Caddy, Traefik, nginx), as long as it:

- forwards `https://games.example.com` to `http://<GAMEFLEX_PUBLIC_HOST>:10200`;
- lets **WebSockets** through (GameFlex keeps several open during a game);
- keeps long connections open.

With **Caddy**, for example:

```
games.example.com {
    reverse_proxy 192.168.1.50:10200
}
```

With **Nginx Proxy Manager**, add a proxy host to `http://192.168.1.50:10200`, turn on *Websockets
Support*, and request a certificate.

### 2. Forward the video ports on your router

Forward **UDP ports 9201 to 9300** to the address of your GameFlex computer (`GAMEFLEX_PUBLIC_HOST`).
Nothing else: never forward ports 10201, 10202, 10205 or 9001 to 9100.

### 3. Open GameFlex through its public address

Sign in at `https://games.example.com` rather than at the local address. The links GameFlex gives
you (shared games, federation tokens) use the address you opened.

### What you should see

From a phone on mobile data, with Wi-Fi off: the page opens at `https://games.example.com`, and a
game shows its picture within a few seconds.

## Keep it safe

- **With the VPN,** only the devices you added reach GameFlex. When a device is lost or given away,
  remove its client from the VPN's page.
- **With the proxy,** anyone can reach the sign-in page. After repeated failed sign-ins, GameFlex
  makes the next attempt wait, up to 15 minutes (*Too many failed sign-ins*).
- **Use strong passwords**, for the administrator and every member.
- **The administrator password in `.env` only creates the account,** at the first start. Change it
  afterwards in GameFlex (**Settings › Profile**). Lost it? `./launch.sh reset-password` sets it
  back to the one in `.env`.
- **Create accounts only for people you know**, and remove the ones you no longer need
  (**Settings › Members**).
- **Keep GameFlex up to date** (see [Update GameFlex](update.md)).
- **HTTPS also unlocks** push notifications and the system share sheet (**Send link**), which
  browsers refuse on plain HTTP. Through the VPN, GameFlex stays on plain HTTP, as at home.

## If something goes wrong

| What you see | What to check |
|---|---|
| The WireGuard app never shows a handshake | UDP 51820 is forwarded to the right computer, and the VPN's *Host* is your home's current public address. Some providers share one public address between several homes (CGNAT): there, no port forwarding works; ask your provider for a public IPv4 address. |
| The tunnel is up, but the page does not open | You opened the home address (`GAMEFLEX_PUBLIC_HOST`), and the device's *Allowed IPs* include it. |
| The VPN does not start (`./launch.sh logs wireguard`) | `Cannot find device "wg0"`: this computer's system has no WireGuard. It comes with Linux 5.6 and later. |
| The page does not open from outside (proxy) | The domain points to your home, the proxy forwards to port 10200, and its certificate is valid. |
| The page opens, but the game stays on *Connecting to the stream* (proxy) | UDP 9201–9300 is forwarded to the right computer. If it still fails on mobile data only, that network blocks direct connections: try another network, or the VPN. |
| Signed out, or the menu does not react (proxy) | The proxy must let WebSockets through, and keep connections open. |
| Shared links point to a local address | Open GameFlex through its public address before sharing. |
