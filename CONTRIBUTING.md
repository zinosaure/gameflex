# Contributing to GameFlex

Thank you for wanting to help. Bug reports, ideas and pull requests are all welcome.

## What you can change

The open part of GameFlex is in this repository: the services and the modules they share
(`src/microservices/`), the web interface (`src/webapp/`), the console recipes
(`src/microservices/runtimes/streamex/consoles/` and `profiles/`), our emulator patches (`tools/`)
and the documentation.

The video gateway, Remote Play, streamex and the Android app are published only as binaries: their
source code is not here, so a pull request cannot change them. Open an issue instead.

## How a pull request travels

This repository is generated from the maintainer's source repository at each release. When a pull
request is accepted:

1. it is merged here, as usual;
2. before the next release, its commits are carried into the source repository, with their
   author, date and message;
3. the release publishes them here again, with everything else.

Your name stays on your commits. Between the merge and the next release, `main` has your change
but the published images do not yet.

## Before you open one

- **One subject per pull request.** Say what it fixes or adds, and how you checked it.
- **Build what you changed.** The images rebuild from this repository (see *Source code* in the
  [README](README.md)):

  ```bash
  GAMEFLEX_VERSION=x.y.z docker compose -f docker-compose.build.yml build gameflex-kernel
  ```

- **Run the tests of the Go module you changed.** The modules expect `src/microservices/shared/` at
  `/shared`, as their Dockerfiles build them. For the kernel:

  ```bash
  docker run --rm -v "$PWD/src/microservices/shared:/shared:ro" \
      -v "$PWD/src/microservices/kernel/src:/src:ro" -w /src \
      -e GOFLAGS=-mod=readonly -e GOCACHE=/tmp/gocache golang:1.23-bookworm go test ./...
  ```

- **Follow the code around you:** its style, its naming, its comments. The documentation is in
  English.
- **No new folder under `src/` or `tools/`** without asking in an issue first: only the folders
  already here are published, and a file outside them would not survive the next release.
- **Never include games, BIOS files, firmware or keys,** nor anything else you have no right to
  share.

## License of contributions

By contributing, you agree that your contribution is published under the license of the files it
changes: the [MIT License](LICENSE) for GameFlex's code and documentation, and the license of the
emulator concerned for the patches in `tools/<emulator>/`.

## Reporting a problem

Use the forms of the [issue tracker](https://github.com/zinosaure/gameflex/issues/new/choose):
*Bug report*, *A game or a console does not work*, *Idea or request*. Read
[Troubleshooting](docs/troubleshooting.md) first: it says what to gather. An issue is public:
remove passwords, tokens and personal addresses from what you paste.
