# Play away from home

**Goal:** reach your GameFlex from outside your home, on a phone's mobile data or at a friend's.

## How it works

Two things travel between a player and your server:

- **The page and its controls**, over HTTPS, through one port: the web port of GameFlex (`10200`).
  You put a reverse proxy in front of it, with your domain name and a certificate.
- **The game's picture and sound**, over WebRTC, through UDP ports `9201` to `9300`, straight to
  your server. Your router forwards them.

GameFlex does not include a relay (TURN) server yet. Most home connections work; some networks
cannot reach your server directly (some mobile carriers, company networks). There, the page
connects but the game stays on *Connecting to the stream*.

**Before you start:**

- GameFlex runs at home (see [Install GameFlex](install.md)).
- A domain name pointing to your home's public address, such as `games.example.com`.
- Access to your router's port forwarding settings.

## Steps

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

## What you should see

From a phone on mobile data, with Wi-Fi off: the page opens at `https://games.example.com`, and a
game shows its picture within a few seconds.

## Keep it safe

Once GameFlex is on the Internet, anyone can reach its sign-in page.

- **Use strong passwords**, for the administrator and every member. After repeated failed
  sign-ins, GameFlex makes the next attempt wait, up to 15 minutes (*Too many failed sign-ins*).
- **The administrator password lives in `.env`.** GameFlex sets it again from there at every
  start: change it in `.env`, then run `docker compose up -d`.
- **Create accounts only for people you know**, and remove the ones you no longer need
  (**Settings › Members**).
- **Keep GameFlex up to date** (see [Update GameFlex](update.md)).
- **HTTPS also unlocks** push notifications and the system share sheet (**Send link**), which
  browsers refuse on plain HTTP.

## If something goes wrong

| What you see | What to check |
|---|---|
| The page does not open from outside | The domain points to your home, the proxy forwards to port 10200, and its certificate is valid. |
| The page opens, but the game stays on *Connecting to the stream* | UDP 9201–9300 is forwarded to the right computer. If it still fails on mobile data only, that network blocks direct connections: try another network. |
| Signed out, or the menu does not react | The proxy must let WebSockets through, and keep connections open. |
| Shared links point to a local address | Open GameFlex through its public address before sharing. |
