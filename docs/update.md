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

It downloads the new images, including those of the recent consoles listed in `COMPOSE_PROFILES`,
and the new `docker-compose.yml` and `launch.sh`, then restarts GameFlex. At start, GameFlex
updates its database by itself.

- **The disks you added** (the `/mnt/<name>` lines, see [Storage](storage.md)) move into the new
  `docker-compose.yml` by themselves. Your previous file stays next to it, in
  `docker-compose.yml.old`: if you changed anything else in it, copy that change over.
- **If the script cannot place your disks,** it leaves your `docker-compose.yml` as it is and saves
  the new one as `docker-compose.yml.new`. Copy your changes into it, then rename it
  `docker-compose.yml`.

Without the script: download the new `docker-compose.yml` (see [Install GameFlex](install.md)),
copy your disk lines into it, then:

```bash
docker compose pull
docker compose up -d
```

## What you should see

- `./launch.sh status` shows every service running.
- The page opens as before, with your accounts, games and saves.

## Stay on a version

By default (`GAMEFLEX_VERSION=latest`), an update takes the newest version. To stay on one, set it
in `.env`, such as `GAMEFLEX_VERSION=0.4.0`: `./launch.sh update` then takes the images and the
files of that version.

## Go back to the previous version

1. Set the previous version in `.env`: `GAMEFLEX_VERSION=<previous version>`.
2. Run `./launch.sh update`: it takes the images and the files of that version.
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
