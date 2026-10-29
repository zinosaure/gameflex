# Disclaimer

## What GameFlex is, and is not

GameFlex is **self-hosted game streaming software**. It provides no game, no BIOS, no firmware and
no emulator. It plays the files you put in your own library.

**Still in development.** GameFlex is a work in progress. Features may be incomplete, change or
disappear between versions, and some consoles are experimental. Back up your library and your
saves.

**A personal project, with no warranty.** GameFlex is provided as is, without warranty of any kind:
its source code under the MIT License (see [LICENSE](LICENSE)), the parts published only as binaries
under the [GameFlex binary license](LICENSE-BINARIES.md). Problems are reported through the
repository's issues; nothing commits anyone to a delay or a fix.

## Your own builds

Part of GameFlex is open source, and you may build it yourself, change it or fork it. **What you
build is yours:** the authors answer neither for how it behaves nor for its security. Only the
images published on Docker Hub as `zinosaure/gameflex-*` and the app on this repository's Releases
page are releases of GameFlex. A modified or rebuilt version must not present itself as one.

## Not affiliated

GameFlex is not affiliated with, endorsed by or sponsored by **any console maker or game
publisher**. Sony, Nintendo, Microsoft, SEGA and the other names that appear in this software or
its documentation are used **only to name the hardware an emulator reproduces**. They are
trademarks of their respective owners, used without their authorization.

No brand logo is shown in the interface.

## Your games are your business

Whether you may use a backup copy, a disc image or a firmware **depends on your country and on what
you own**. GameFlex checks nothing and authorizes nothing: it opens the files you give it.

**You alone are responsible** for the files you put in your library, how you obtained them, and
how you use them.

## Federation: everyone answers for what they share

GameFlex lets several servers share their catalogs. This feature is a **transport**: it moves files
between machines that their owners have deliberately connected.

**Every server administrator answers for what they make available** and for what they take.
Opening your catalog to someone is a decision, made with an explicit access token. The authors of
GameFlex see, host and broker no content: there is no central catalog.

## Emulators do not come from GameFlex

GameFlex does not distribute emulators or cores. Your server downloads them from their publishers,
or you bring them yourself, from a federated server or a mirror. Each one keeps its own license and
terms. Emulation projects are named in the documentation to tell you where to find them; none of
them is associated with GameFlex.

**Our patched builds are ours.** The builds of shadPS4, KytyPS5, Eden and Box64 made with the
patches in `tools/` are not those of their projects, which neither made nor support them. Report
their problems to GameFlex, not to the upstream projects.

**Remote Play** talks to a PlayStation console you own, registered with your own account, the way
other independent Remote Play clients do. Sony neither makes nor supports it, and may change what
it relies on at any time.

The mirror mentioned in the [README](README.md) is not part of the GameFlex project. There is no
guarantee that it stays online or unchanged, and you remain responsible for what you download from
it.

## Network exposure

GameFlex is made for a **home network**. If you open it to the Internet, its security is your
responsibility: put it behind an HTTPS proxy, and give it a strong administrator password. You
choose that password, or let the setup script make one up for you, and keep it.
