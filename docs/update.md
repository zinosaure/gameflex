# Update GameFlex

**Goal:** move to a new version of GameFlex, and go back if needed.

**Before you start:**

- **Back up** `databases/` and `saves/` from your library folder (see
  [Storage](storage.md#back-up)).
- **Finish the games in progress:** the update restarts every service.
- Read what changed: the [Releases page](https://github.com/zinosaure/gameflex/releases) and the
  `CHANGELOG.md`.

## Steps

With the setup script:

```bash
./launch.sh update
```

Without it:

```bash
docker compose pull
docker compose up -d
```

Docker downloads the new images, including those of the recent consoles listed in
`COMPOSE_PROFILES`, and restarts GameFlex. At start, GameFlex updates its database by itself.

## What you should see

- `./launch.sh status` shows every service running.
- The page opens as before, with your accounts, games and saves.

## Stay on a version

By default (`GAMEFLEX_VERSION=latest`), an update takes the newest version. To stay on one, set it
in `.env`, such as `GAMEFLEX_VERSION=0.3.0`, then run `docker compose up -d`.

## Go back to the previous version

1. Set the previous version in `.env`: `GAMEFLEX_VERSION=<previous version>`.
2. Run `docker compose up -d`.
3. If GameFlex misbehaves, restore the `databases/` folder you backed up: a newer version may have
   changed it.

## The Android app

The app updates itself: on its home screen, **Update** › **Check for an update**. See [GameFlex on Android](android.md).

## If something goes wrong

| What you see | What to do |
|---|---|
| `pull` fails | Check the Internet connection, and the version name in `GAMEFLEX_VERSION`. |
| A service keeps restarting after the update | Read its log (`./launch.sh logs kernel`), then go back to the previous version and report it (see [Troubleshooting](troubleshooting.md)). |
| The disk fills up | `./launch.sh update` removes the old images; with plain Docker, run `docker image prune`. |
