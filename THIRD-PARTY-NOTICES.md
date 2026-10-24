# Third-party notices

GameFlex's own code is released under the MIT License (see [LICENSE](LICENSE)). The artifacts
published for GameFlex also contain software written by other people, each part under its own
license. This file lists that software, artifact by artifact, with the version found in the
artifact, its license and where its source code can be found. The full license texts and copyright
notices are in [THIRD-PARTY-LICENSES.txt](THIRD-PARTY-LICENSES.txt).

**No emulator, BIOS, firmware, game or libretro core is included in any GameFlex artifact.**
The console images unpack the emulator file that *you* place in `cores/<console>/` when a game is
launched; cores, BIOS files and games are always supplied by the user.

This inventory was made on the builds that became GameFlex **0.3.0** by reading the artifacts themselves: the Go build
information embedded in each binary, the Debian package database of each image, the webpack
module list of the web interface, the Gradle dependency tree and the files inside the APK, and a
byte-for-byte comparison of the runtime archives with the Debian packages they come from. When a
newer build changes a version, the artifact itself is authoritative (see
[Finding the licenses and sources yourself](#finding-the-licenses-and-sources-yourself)).

License names are [SPDX identifiers](https://spdx.org/licenses/). "LGPL-2.1+" means "LGPL-2.1 or
any later version", and so on.

## Contents

- [At a glance](#at-a-glance)
- [1. Docker images: services](#1-docker-images-services)
- [2. Docker images: consoles](#2-docker-images-consoles)
- [3. Android app](#3-android-app)
- [4. Runtime archives for the Android app](#4-runtime-archives-for-the-android-app)
- [Copyleft components and their source code](#copyleft-components-and-their-source-code)
- [Finding the licenses and sources yourself](#finding-the-licenses-and-sources-yourself)
- [Notices reproduced as required](#notices-reproduced-as-required)

## At a glance

| Artifact | Third-party content |
|---|---|
| `zinosaure/gameflex-edge` | Debian 12 base (91 packages), Go standard library, web interface (31 npm packages) |
| `zinosaure/gameflex-kernel`, `-credential`, `-gateway-webrtc`, `-remoteplay` | Debian 12 base (93 packages), Go modules (13 to 26 per binary) |
| `zinosaure/gameflex-retrogaming` | Debian 12 base (293 packages, including FFmpeg and Mesa), 2 Go modules, `libretro.h` |
| `zinosaure/gameflex-ps3`, `-ps4`, `-ps5`, `-switch`, `-switch-eden`, `-wiiu`, `-xbox`, `-xbox360` | Debian 13 base (411 to 436 packages, Mesa from trixie-backports), 4 Go modules, FFmpeg 9.0.2 (shared libraries, LGPL, with its source), `pkgextract` (PS4 image only). **No emulator.** |
| `gameflex.apk` | AndroidX, Kotlin, ZXing (Apache-2.0), 35 Go modules, the web interface, the glibc dynamic loader (LGPL), `xkbcomp` (MIT) |
| `glibc-x86_64.zip`, `glibc-aarch64.zip` | ~115 libraries from Debian forky (glibc, Mesa, LLVM, PulseAudio, X11…), DejaVu fonts, MIME database |
| `xvfb-bundle-x86_64.zip`, `xvfb-bundle-aarch64.zip` | Xvfb and its libraries from the Termux package repository (X.Org, Mesa, LLVM, ICU, OpenSSL, GNU libiconv…) |

The Go standard library, linked into every GameFlex binary, is under the
[BSD-3-Clause license of the Go project](https://go.dev/LICENSE). Toolchains found in the
binaries: go1.23.12 (edge, kernel, gateway-webrtc, remoteplay, retrogaming), go1.25.14
(credential), go1.24.13 (streamex, sdiag-guid), go1.26.0 (Android app).

---

## 1. Docker images: services

### Base image and Debian packages

All six service images are built `FROM debian:bookworm-slim` (Debian 12.15 in the inventoried
build). The build stages (`golang:1.23-bookworm`, `golang:1.25-bookworm`, `node:20-alpine`) are
not part of the published images.

| Image | Debian packages | Added on top of the base system |
|---|---|---|
| `gameflex-edge` | 91 | `ca-certificates`, `tzdata` |
| `gameflex-kernel`, `gameflex-credential`, `gameflex-gateway-webrtc`, `gameflex-remoteplay` | 93 | `ca-certificates`, `tzdata`, `wget` |
| `gameflex-retrogaming` | 293 | `ca-certificates`, `tzdata`, `ffmpeg`, `libegl1`, `libgles2`, `libgbm1`, `libegl-mesa0`, `libgl1-mesa-dri` and their dependencies |

Notable components (Debian source package and version):

| Component | Version | License | Image(s) |
|---|---|---|---|
| GNU C Library (`glibc`) | 2.36-9+deb12u14 | LGPL-2.1+ | all |
| Base system (`bash`, `coreutils`, `dpkg`, `apt`, `tar`, `util-linux`, `perl-base`…) | Debian 12 | mostly GPL-2+ / GPL-3+ | all |
| OpenSSL | 3.0.22-1~deb12u1 | Apache-2.0 | all |
| `ca-certificates` | 20250419~deb12u2 | MPL-2.0 (certificate data), GPL-2+ (scripts) | all |
| `tzdata` | 2026e-0+deb12u1 | public domain | all |
| GNU Wget | 1.21.3-1+deb12u1 | GPL-3+ | kernel, credential, gateway-webrtc, remoteplay |
| GnuTLS | 3.7.9-2+deb12u7 | LGPL-2.1+ | kernel, credential, gateway-webrtc, remoteplay, retrogaming |
| FFmpeg (Debian build, configured with `--enable-gpl`) | 7:5.1.9-0+deb12u1 | GPL-2+ as built | retrogaming |
| x264 / x265 / Xvid / Rubber Band / vid.stab (FFmpeg dependencies) | 2:0.164.3095+gitbaee400-3 / 3.5-2 / 2:1.3.7-1 / 3.1.2+dfsg0-1 / 1.1.0-2 | GPL-2+ | retrogaming |
| Mesa | 22.3.6-1+deb12u2 | MIT | retrogaming |
| LLVM 15 (Mesa dependency) | 1:15.0.6-4 | Apache-2.0 WITH LLVM-exception | retrogaming |
| libglvnd / libdrm / libva | 1.6.0-1 / 2.4.114-1 / 2.17.0-1 | MIT-style / MIT / MIT | retrogaming |
| SDL 2 | 2.26.5+dfsg-1 | Zlib | retrogaming |
| PulseAudio client library | 16.1+dfsg1-2 | LGPL-2.1+ | retrogaming |
| LAME / Blu-ray / libssh (FFmpeg dependencies) | 3.100-6 / 1:1.3.4-1 / 0.10.6-0+deb12u3 | LGPL-2+ / LGPL-2.1+ / LGPL-2.1+ | retrogaming |
| Opus, Vorbis, libvpx, AOM, dav1d (FFmpeg dependencies) | Debian 12 | BSD-style | retrogaming |

Every package in an image has its license at `/usr/share/doc/<package>/copyright` inside that
image (checked: no package lacks one). The corresponding source is the Debian source package of
the same version, see [below](#finding-the-licenses-and-sources-yourself).

### GameFlex programs and the Go modules they link

| Image | Program | Third-party Go modules linked |
|---|---|---|
| `gameflex-edge` | `/app/edge` | none (Go standard library only) |
| `gameflex-kernel` | `/app/sessions` | 26 |
| `gameflex-credential` | `/app/auth` | 13 |
| `gameflex-gateway-webrtc` | `/app/gatewaywebrtc` | 23 |
| `gameflex-remoteplay` | `/app/remoteplay` | 13 |
| `gameflex-retrogaming` | `/app/retrogaming` | 2, plus `libretro.h` (see below) |

52 distinct modules in total: 22 MIT, 14 Apache-2.0, 12 BSD-3-Clause, 3 BSD-2-Clause, 1 ISC.

| Module | Version | License | Linked into |
|---|---|---|---|
| [github.com/distribution/reference](https://github.com/distribution/reference) | v0.6.0 | Apache-2.0 | kernel |
| [github.com/docker/docker](https://github.com/moby/moby) | v27.3.1+incompatible | Apache-2.0 (with a [NOTICE](#docker-githubcomdockerdocker)) | kernel |
| [github.com/docker/go-connections](https://github.com/docker/go-connections) | v0.5.0 | Apache-2.0 | kernel |
| [github.com/docker/go-units](https://github.com/docker/go-units) | v0.5.0 | Apache-2.0 | kernel |
| [github.com/go-logr/logr](https://github.com/go-logr/logr) | v1.3.0 | Apache-2.0 | kernel |
| [github.com/go-logr/stdr](https://github.com/go-logr/stdr) | v1.2.2 | Apache-2.0 | kernel |
| [github.com/moby/docker-image-spec](https://github.com/moby/docker-image-spec) | v1.3.1 | Apache-2.0 | kernel |
| [github.com/moby/term](https://github.com/moby/term) | v0.5.2 | Apache-2.0 | kernel |
| [github.com/opencontainers/go-digest](https://github.com/opencontainers/go-digest) | v1.0.0 | Apache-2.0 | kernel |
| [github.com/opencontainers/image-spec](https://github.com/opencontainers/image-spec) | v1.1.1 | Apache-2.0 | kernel |
| [go.opentelemetry.io/otel](https://github.com/open-telemetry/opentelemetry-go), `/metric`, `/trace` | v1.21.0 | Apache-2.0 | kernel |
| [go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib) | v0.44.0 | Apache-2.0 | kernel |
| [github.com/dustin/go-humanize](https://github.com/dustin/go-humanize) | v1.0.1 | MIT | credential, remoteplay |
| [github.com/felixge/httpsnoop](https://github.com/felixge/httpsnoop) | v1.0.3 | MIT | kernel |
| [github.com/go-chi/chi/v5](https://github.com/go-chi/chi) | v5.3.1 (kernel), v5.1.0 (gateway-webrtc, remoteplay) | MIT | kernel, gateway-webrtc, remoteplay |
| [github.com/go-chi/cors](https://github.com/go-chi/cors) | v1.2.1 | MIT | remoteplay |
| [github.com/golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) | v5.3.1 | MIT | kernel, credential, remoteplay |
| [github.com/morikuni/aec](https://github.com/morikuni/aec) | v1.1.0 | MIT | kernel |
| [github.com/pion/webrtc/v4](https://github.com/pion/webrtc) and its 15 companion modules: `datachannel` v1.5.9, `dtls/v3` v3.0.3, `ice/v4` v4.0.2, `interceptor` v0.1.37, `logging` v0.2.2, `mdns/v2` v2.0.7, `randutil` v0.1.0, `rtcp` v1.2.14, `rtp` v1.8.9, `sctp` v1.8.33, `sdp/v3` v3.0.9, `srtp/v3` v3.0.4, `stun/v3` v3.0.0, `transport/v3` v3.0.7, `turn/v4` v4.0.0 | v4.0.0 | MIT | gateway-webrtc |
| [github.com/geoffgarside/ber](https://github.com/geoffgarside/ber) | v1.1.0 | BSD-3-Clause | kernel, credential |
| [github.com/gogo/protobuf](https://github.com/gogo/protobuf) | v1.3.2 | BSD-3-Clause | kernel |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause | kernel, credential, gateway-webrtc, remoteplay, retrogaming |
| [github.com/remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft) | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | credential, remoteplay |
| [github.com/wlynxg/anet](https://github.com/wlynxg/anet) | v0.0.3 | BSD-3-Clause | gateway-webrtc |
| [golang.org/x/crypto](https://go.googlesource.com/crypto) | v0.27.0 (kernel), v0.28.0 (gateway-webrtc), v0.32.0 (credential) | BSD-3-Clause | kernel, credential, gateway-webrtc |
| [golang.org/x/net](https://go.googlesource.com/net) | v0.29.0 | BSD-3-Clause | gateway-webrtc |
| [golang.org/x/sys](https://go.googlesource.com/sys) | v0.26.0 (kernel, gateway-webrtc, remoteplay), v0.47.0 (credential) | BSD-3-Clause | kernel, credential, gateway-webrtc, remoteplay |
| [modernc.org/libc](https://gitlab.com/cznic/libc) | v1.74.4 (credential), v1.55.3 (remoteplay) | BSD-3-Clause; contains code derived from musl libc (MIT) and Go (BSD-3-Clause) | credential, remoteplay |
| [modernc.org/mathutil](https://gitlab.com/cznic/mathutil) | v1.7.1 (credential), v1.6.0 (remoteplay) | BSD-3-Clause | credential, remoteplay |
| [modernc.org/memory](https://gitlab.com/cznic/memory) | v1.11.0 (credential), v1.8.0 (remoteplay) | BSD-3-Clause | credential, remoteplay |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | v1.57.0 (credential), v1.33.1 (remoteplay) | BSD-3-Clause; contains SQLite (public domain) | credential, remoteplay |
| [github.com/gorilla/websocket](https://github.com/gorilla/websocket) | v1.5.3 | BSD-2-Clause | kernel, credential, gateway-webrtc, remoteplay, retrogaming |
| [github.com/hirochachacha/go-smb2](https://github.com/hirochachacha/go-smb2) | v1.1.0 | BSD-2-Clause | kernel, credential |
| [github.com/pkg/errors](https://github.com/pkg/errors) | v0.9.1 | BSD-2-Clause | kernel |
| [github.com/decred/dcrd/dcrec/secp256k1/v4](https://github.com/decred/dcrd) | v4.4.1 | ISC | remoteplay |

`/app/retrogaming` is also compiled with
[`libretro.h`](https://github.com/libretro/RetroArch/blob/master/libretro-common/include/libretro.h)
(Copyright The RetroArch team, MIT) and links dynamically to Mesa's EGL/GBM and libglvnd from the
Debian packages listed above.

### Web interface (`gameflex-edge`)

`gameflex-edge` serves the web interface from `/app/web`, a webpack bundle. The bundle and its
`*.LICENSE.txt` files contain code from 31 npm packages; nothing else from `node_modules` is in the
image. Versions are those resolved when the image was built (no lockfile is used), so they may
differ slightly from one build to the next; the inventoried build of the image contained:

| Package (declared dependency) | Version | License |
|---|---|---|
| [@fortawesome/fontawesome-free](https://github.com/FortAwesome/Font-Awesome) | 6.7.2 | Icons CC-BY-4.0, fonts OFL-1.1, code MIT |
| [bootstrap](https://github.com/twbs/bootstrap) | 5.3.8 | MIT |
| [bootstrap-icons](https://github.com/twbs/icons) (icon fonts) | 1.13.2 | MIT |
| [i18next](https://github.com/i18next/i18next) | 23.16.8 | MIT |
| [jquery](https://github.com/jquery/jquery) | 3.7.1 | MIT |
| [popper.js](https://github.com/floating-ui/floating-ui/tree/v1.x) | 1.16.1 | MIT |
| [react](https://github.com/facebook/react) | 18.3.1 | MIT |
| [react-dom](https://github.com/facebook/react) | 18.3.1 | MIT |
| [react-bootstrap](https://github.com/react-bootstrap/react-bootstrap) | 2.10.10 | MIT |
| [react-i18next](https://github.com/i18next/react-i18next) | 14.1.3 | MIT |
| [react-router-dom](https://github.com/remix-run/react-router) | 6.30.6 | MIT |

The 20 other packages bundled with them (dependencies of the above, and the runtime helpers of
the webpack loaders):

| License | Count | Packages |
|---|---|---|
| MIT | 17 | `@babel/runtime` 7.29.10, `@popperjs/core` 2.11.8, `@remix-run/router` 1.23.4, `@restart/hooks` 0.4.16, `@restart/ui` 1.9.4, `classnames` 2.5.1, `css-loader` 6.11.0 (runtime), `dequal` 2.0.3, `dom-helpers` 5.2.1, `invariant` 2.2.4, `prop-types` 15.8.1, `react-lifecycles-compat` 3.0.4, `react-router` 6.30.6, `scheduler` 0.23.2, `style-loader` 3.3.4 (runtime), `uncontrollable` 7.2.1, `warning` 4.0.3 |
| Apache-2.0 | 2 | `react-aria` 3.53.1, `@react-aria/ssr` 3.10.1 |
| BSD-3-Clause | 1 | `react-transition-group` 4.4.5 |

Four declared dependencies (`@microsoft/signalr`, `axios`, `process`, `react-bootstrap-icons`)
are not imported by the interface and are not in the bundle.

---

## 2. Docker images: consoles

### No emulator inside

None of the eight console images contains an emulator. Each one unpacks, at launch, the emulator
file that the user has placed in `cores/<console>/`; the image only carries the name and the
official download address of the version it was tested with (image labels
`gameflex.emulator.file` and `gameflex.emulator.url`), plus configuration files.

| Image | Emulator it expects (not included) |
|---|---|
| `gameflex-ps3` | RPCS3 |
| `gameflex-ps4` | shadPS4 |
| `gameflex-ps5` | KytyPS5 |
| `gameflex-switch` | Citron |
| `gameflex-switch-eden` | Eden |
| `gameflex-wiiu` | Cemu |
| `gameflex-xbox` | xemu |
| `gameflex-xbox360` | Xenia Canary |

Each emulator is distributed by its own authors under its own license; GameFlex does not
redistribute it.

### Base image and Debian packages

All eight images are built `FROM debian:trixie-slim` (Debian 13.7; 13.6 for `gameflex-ps5` and
`gameflex-xbox360` in the inventoried build), with Mesa taken from `trixie-backports`. They hold
411 to 436 Debian packages each; 274 source packages are common to all eight. The build stages
(`debian:trixie-slim` for FFmpeg, `golang:1.24-trixie`) are not part of the published images.

Notable components common to all eight images:

| Component | Version | License |
|---|---|---|
| GNU C Library (`glibc`) | 2.41-12+deb13u4 (deb13u3 in ps5, xbox360) | LGPL-2.1+ |
| Mesa (OpenGL, Vulkan, VA-API drivers; trixie-backports) | 26.1.6-1~bpo13+1 | MIT |
| LLVM 19 (Mesa dependency) | 1:19.1.7-3 | Apache-2.0 WITH LLVM-exception |
| Vulkan loader | 1.4.309.0-1 | Apache-2.0 |
| libglvnd / libdrm / libva | 1.7.0-1 / 2.4.124-2 / 2.22.0-3 | MIT-style / MIT / MIT |
| Weston | 14.0.2-1 | MIT |
| Xwayland (and `xserver-common` from `xorg-server`) | 2:24.1.6-1 (2:21.1.16-1.3+deb13u4) | MIT/X11 |
| Wayland | 1.23.1-3 | MIT |
| Openbox | 3.6.1-12 | GPL-2+ |
| PulseAudio | 17.0+dfsg1-2 | LGPL-2.1+ (some parts GPL-2+) |
| D-Bus | 1.16.2-2 | AFL-2.1 or GPL-2+ |
| OpenH264 (Debian build) | 2.6.0+dfsg-2 | BSD-2-Clause |
| Opus | 1.5.2-2 | BSD-3-Clause |
| FFmpeg libraries (Debian build, configured with `--enable-gpl`), pulled in by Weston's VNC/RDP backends and the ALSA plugins | 7:7.1.5-0+deb13u1 | GPL-2+ as built |
| x264 / x265 / Xvid / Rubber Band / vid.stab (FFmpeg dependencies) | 2:0.164.3108+git31e19f9-2 / 4.1-2 / 2:1.3.7-1 / 3.3.0+dfsg-2 / 1.1.0-2 | GPL-2+ |
| Ghostscript library (`libgs10`, pulled in by Openbox through imlib2 and libspectre) | 10.05.1~dfsg-1+deb13u3 | AGPL-3+ |
| FreeRDP 3 / neatvnc (Weston backends) | 3.15.0+dfsg-2.1+deb13u3 / 0.9.1+dfsg-1 | Apache-2.0 / ISC |
| PipeWire libraries / GStreamer | 1.4.2-1 / 1.26.2-2 | MIT / LGPL-2.1+ |
| GLib, Pango, Cairo, HarfBuzz, Fontconfig, FreeType | Debian 13 | LGPL-2.1+ / LGPL-2+ / LGPL-2.1 or MPL-1.1 / MIT / MIT-style / FTL or GPL-2+ |
| DejaVu fonts | 2.37-8 | Bitstream Vera license, public domain |
| xdotool / `x11-xserver-utils` / `xauth` | 1:3.20160805.1-5.1 / 7.7+11 / 1:1.1.2-1.1 | BSD-3-Clause / MIT / MIT |
| curl / OpenSSL / GnuTLS | 8.14.1-2+deb13u7 / 3.5.7-1~deb13u3 / 3.8.9-3+deb13u4 | curl / Apache-2.0 / LGPL-2.1+ |
| Base system (`bash`, `coreutils`, `procps`, `perl-base`…) | Debian 13 | mostly GPL-2+ / GPL-3+ |

Additional packages per image:

| Image | Additional notable components |
|---|---|
| `gameflex-ps3` | ImageMagick 8:7.1.1.43+dfsg1-1+deb13u12 (ImageMagick license), libc++ 19 (Apache-2.0 WITH LLVM-exception), wmctrl 1.07+git20240228.1105759-1 (GPL-2+) |
| `gameflex-ps4` | SDL 3 3.2.10+ds-1 (Zlib), UnZip 6.0-29+deb13u1 (Info-ZIP), `pkgextract` (see below) |
| `gameflex-ps5` | XCB utility libraries (MIT) |
| `gameflex-switch`, `gameflex-switch-eden` | ImageMagick 8:7.1.1.43+dfsg1-1+deb13u12, libc++ 19, AT-SPI2 2.56.2-1+deb13u2 (LGPL-2.1+) |
| `gameflex-wiiu` | GTK 3 3.24.49-3 and GTK 4 4.18.6+ds-2 (LGPL-2.1+), SDL 2 2.32.4+dfsg-1 (Zlib), BlueZ library `libbluetooth3` 5.82-1.1 (GPL-2+), libzip 1.11.3-2 (BSD-3-Clause), GLU 9.0.2-1.1 (SGI-B-2.0), AT-SPI2 2.56.2-1+deb13u2 |
| `gameflex-xbox` | SDL 2 2.32.4+dfsg-1 (Zlib), libusb 2:1.0.28-1 (LGPL-2.1+) |
| `gameflex-xbox360` | no additional notable package |

As in the service images, every package has its license at `/usr/share/doc/<package>/copyright`
(checked: no package lacks one).

### GameFlex programs and FFmpeg

| Program | Images | Third-party content |
|---|---|---|
| `/usr/local/bin/streamex` (identical in all eight images) | all | Go modules below; links dynamically to FFmpeg 9.0.2 (below), OpenH264, Opus, the PulseAudio client library, libva and libdrm from the Debian packages above |
| `/usr/local/bin/sdiag-guid` | ps4, ps5, xbox, xbox360 | Go standard library only; loads SDL 2 dynamically when present |
| `/usr/local/bin/unpack-emulator` | all | GameFlex shell script (MIT) |

Go modules linked into `streamex`:

| Module | Version | License |
|---|---|---|
| [github.com/godbus/dbus/v5](https://github.com/godbus/dbus) | v5.2.2 | BSD-2-Clause |
| [github.com/gorilla/websocket](https://github.com/gorilla/websocket) | v1.5.3 | BSD-2-Clause |
| [github.com/jezek/xgb](https://github.com/jezek/xgb) | v1.1.1 | BSD-3-Clause |
| [golang.org/x/sys](https://go.googlesource.com/sys) | v0.30.0 | BSD-3-Clause |

FFmpeg in `streamex`:

| Component | Version | License | Source |
|---|---|---|---|
| [FFmpeg](https://ffmpeg.org) `libavcodec` (63.1.102) and `libavutil`, shared libraries in `/usr/local/lib` | 9.0.2, unmodified | LGPL-2.1+ (built without `--enable-gpl` and without `--enable-nonfree`) | in each console image: `/usr/share/doc/gameflex/sources/ffmpeg-9.0.2.tar.xz`; <https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz> |

It was configured with `--enable-shared --disable-static --disable-programs
--disable-doc --disable-debug --disable-everything --disable-avdevice --disable-avformat
--disable-avfilter --disable-swscale --disable-swresample --disable-network --enable-vaapi
--enable-encoder=h264_vaapi`: only the VA-API H.264 encoder is compiled in. `streamex` loads these
libraries at run time: replacing `/usr/local/lib/libavcodec.so.63` and `libavutil.so.61` with
your own build of the same FFmpeg version is enough to run it with a modified FFmpeg.

### `pkgextract` (`gameflex-ps4` only)

| Component | Version | License | Source |
|---|---|---|---|
| [ps4-pkg-extractor](https://github.com/paulomanrique/ps4-pkg-extractor) (`/usr/local/bin/pkgextract`), downloaded as released by its author | v0.0.1 | Unlicense | <https://github.com/paulomanrique/ps4-pkg-extractor/releases/tag/v0.0.1> |

It is a Rust program; according to its `Cargo.lock` at v0.0.1 it is built with the crates
`orbis-pkg-util`, `orbis-pkg` and `orbis-pfs` 0.1.0 ([source](https://github.com/videobitva/orbis),
MIT OR Apache-2.0), `clap`, `anyhow`, `rsa`, `aes`, `sha2`, `flate2`, `rayon`, `indicatif` and
their dependencies, under permissive licenses (mostly MIT OR Apache-2.0; also BSD-3-Clause for
`subtle`, Unicode-3.0 for `unicode-ident`, Zlib as an option for `miniz_oxide`).

---

## 3. Android app

`gameflex.apk` (version 0.2.0, ABIs `arm64-v8a` and `x86_64`). Emulators, libretro cores and the
runtime archives of [section 4](#4-runtime-archives-for-the-android-app) are not in the APK; the
app downloads them only when a feature that needs them is used.

### Java and Kotlin libraries

Resolved from the app's release runtime classpath; all are Apache-2.0.

| Library | Version | License | Source |
|---|---|---|---|
| AndroidX: `appcompat`, `appcompat-resources` | 1.7.0 | Apache-2.0 | <https://developer.android.com/jetpack/androidx> |
| AndroidX: `core`, `core-ktx` | 1.13.0 | Apache-2.0 | same |
| AndroidX: `activity` 1.7.0, `fragment` 1.5.4, `lifecycle-*` 2.6.2, `arch.core` 2.2.0, `savedstate` 1.2.1, `emoji2`, `emoji2-views-helper` 1.3.0, `annotation` 1.6.0, `annotation-experimental` 1.4.0, `collection` 1.1.0, `concurrent-futures` 1.1.0, `profileinstaller` 1.3.1, `startup-runtime` 1.1.1, `tracing` 1.0.0, `vectordrawable`, `vectordrawable-animated` 1.1.0, `versionedparcelable` 1.1.1, `documentfile` 1.0.1, `drawerlayout`, `cursoradapter`, `customview`, `interpolator`, `loader`, `viewpager` 1.0.0, `resourceinspection-annotation` 1.0.1 | as listed | Apache-2.0 | same |
| Kotlin standard library (`kotlin-stdlib`, `-common`, `-jdk7`, `-jdk8`) | 1.8.22 | Apache-2.0 | <https://github.com/JetBrains/kotlin> |
| kotlinx.coroutines (`core`, `android`) | 1.6.4 | Apache-2.0 | <https://github.com/Kotlin/kotlinx.coroutines> |
| JetBrains annotations | 13.0 | Apache-2.0 | <https://github.com/JetBrains/java-annotations> |
| Guava `listenablefuture` | 1.0 | Apache-2.0 | <https://github.com/google/guava> |
| ZXing `core` | 3.5.3 | Apache-2.0 | <https://github.com/zxing/zxing> |

### Go code (`lib/<abi>/libgojni.so`)

The app's server runs in `libgojni.so`, built with go1.26.0 and `gomobile bind`
([golang.org/x/mobile](https://go.googlesource.com/mobile), BSD-3-Clause, which also provides the
Java glue classes `go.Seq`). Both ABIs link the same 35 modules: 18 MIT, 14 BSD-3-Clause,
2 BSD-2-Clause, 1 ISC. The code is also compiled with `libretro.h` (The RetroArch team, MIT).

| Module | Version | License |
|---|---|---|
| [github.com/pion/webrtc/v4](https://github.com/pion/webrtc) and its 15 companion modules: `datachannel` v1.6.2, `dtls/v3` v3.1.8, `ice/v4` v4.4.2, `interceptor` v0.1.48, `logging` v0.2.4, `mdns/v2` v2.2.0, `randutil` v0.1.0, `rtcp` v1.2.17, `rtp` v1.10.5, `sctp` v1.11.1, `sdp/v3` v3.0.19, `srtp/v3` v3.0.13, `stun/v4` v4.0.0, `transport/v4` v4.1.0, `turn/v5` v5.1.0 | v4.2.20 | MIT |
| [github.com/dustin/go-humanize](https://github.com/dustin/go-humanize) | v1.0.1 | MIT |
| [github.com/golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) | v5.3.1 | MIT |
| [github.com/geoffgarside/ber](https://github.com/geoffgarside/ber) | v1.1.0 | BSD-3-Clause |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause |
| [github.com/jezek/xgb](https://github.com/jezek/xgb) | v1.1.1 | BSD-3-Clause |
| [github.com/remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft) | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| [github.com/wlynxg/anet](https://github.com/wlynxg/anet) | v0.0.5 | BSD-3-Clause |
| [golang.org/x/crypto](https://go.googlesource.com/crypto) | v0.55.0 | BSD-3-Clause |
| [golang.org/x/mobile](https://go.googlesource.com/mobile) | v0.0.0-20260821190718-4776eadac327 | BSD-3-Clause |
| [golang.org/x/net](https://go.googlesource.com/net) | v0.58.0 | BSD-3-Clause |
| [golang.org/x/sys](https://go.googlesource.com/sys) | v0.47.0 | BSD-3-Clause |
| [golang.org/x/time](https://go.googlesource.com/time) | v0.14.0 | BSD-3-Clause |
| [modernc.org/libc](https://gitlab.com/cznic/libc) | v1.74.4 | BSD-3-Clause; contains code derived from musl libc (MIT) and Go (BSD-3-Clause) |
| [modernc.org/mathutil](https://gitlab.com/cznic/mathutil) | v1.7.1 | BSD-3-Clause |
| [modernc.org/memory](https://gitlab.com/cznic/memory) | v1.11.0 | BSD-3-Clause |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | v1.57.0 | BSD-3-Clause; contains SQLite (public domain) |
| [github.com/gorilla/websocket](https://github.com/gorilla/websocket) | v1.5.3 | BSD-2-Clause |
| [github.com/hirochachacha/go-smb2](https://github.com/hirochachacha/go-smb2) | v1.1.0 | BSD-2-Clause |
| [github.com/decred/dcrd/dcrec/secp256k1/v4](https://github.com/decred/dcrd) | v4.4.1 | ISC |

### Web interface (`assets/web`)

The app embeds the same web interface as `gameflex-edge`, built from the same 31 npm packages
(see [the table above](#web-interface-gameflex-edge)). In the inventoried APK the only version
that differs is `bootstrap-icons` 1.13.1. The app also carries `assets/fonts/bootstrap-icons.ttf`
(Bootstrap Icons, MIT).

### Native libraries (`lib/<abi>/`)

| File | What it is | Version | License | Source |
|---|---|---|---|---|
| `libglibcloader.so` | The GNU C Library dynamic loader (`ld-linux-*.so`), unmodified, from Debian forky's `libc6` | 2.43-7 (`arm64-v8a`), 2.43-6 (`x86_64`) | LGPL-2.1+ | Debian source package `glibc` of that version |
| `libxkbcomp.so` | `xkbcomp`, the X keymap compiler, from the Termux package `xorg-xkbcomp`; identical to `bin/xkbcomp` of the Xvfb archive (see [section 4](#xvfb-bundle-x86_64zip-and-xvfb-bundle-aarch64zip)) | 1.5.0 | MIT / HPND (X.Org) | <https://gitlab.freedesktop.org/xorg/app/xkbcomp>, build recipe <https://github.com/termux/termux-packages> |
| `libgojni.so` | GameFlex (Go), see above | — | MIT + the licenses above | — |
| `libptracer.so`, `libspawner.so`, `libgfudev.so`, `libahbprobe.so` | GameFlex's own C code | — | MIT | — |

---

## 4. Runtime archives for the Android app

These release assets are downloaded by the app on demand, to run Linux (glibc) programs and an
X server on Android.

### `glibc-x86_64.zip` and `glibc-aarch64.zip`

Assembled from **unmodified Debian forky (testing) binary packages**. Every library (116 in the
x86_64 archive, 115 in the aarch64 one), the PulseAudio daemon and its modules, the three fonts
and the compiled MIME cache were compared byte for byte with the Debian packages below and are
identical to them. `glibc-x86_64.zip` matches the Debian archive as of 2026-10-02,
`glibc-aarch64.zip` as of 2026-10-19 (file `RUNTIME` in each archive:
`debian:forky-20260918-slim`, the image used to assemble them).

Versions are those of the binary packages; a `+bN` suffix marks a rebuild of the source version
without that suffix (for `libtdb1`, the source is `samba` 2:4.25.0+dfsg-1).

| Debian source package | Files in the archives | Version (x86_64) | Version (aarch64) | License |
|---|---|---|---|---|
| `glibc` | `libc.so.6`, `libm.so.6`, `libpthread.so.0`, `libdl.so.2`, `librt.so.1`, `libresolv.so.2`, `libnss_*`, `libmvec`, `libanl`, `libnsl`, `libutil`, `libthread_db`, `libBrokenLocale`, `libmemusage`, `libpcprofile`, `libc_malloc_debug` | 2.43-6 | 2.43-7 | LGPL-2.1+ |
| `gcc-16` | `libstdc++.so.6`, `libgcc_s.so.1`, `libgomp.so.1` | 16.2.0-3 | 16.2.0-3 | GPL-3+ WITH GCC-exception-3.1 |
| `mesa` | `libgallium-26.1.6-1.so`, `swrast_dri.so`, `libvulkan_lvp.so`, `libEGL_mesa.so.0`, `libGLX_mesa.so.0`, `libgbm.so.1` | 26.1.6-1 | 26.1.6-1 | MIT |
| `llvm-toolchain-21` | `libLLVM.so.21.1` | 1:21.1.8-13 | 1:21.1.8-13 | Apache-2.0 WITH LLVM-exception |
| `libglvnd` | `libGL`, `libGLX`, `libEGL`, `libOpenGL`, `libGLdispatch` | 1.7.0-3+b1 | 1.7.0-3+b1 | MIT-style |
| `vulkan-loader` | `libvulkan.so.1` | 1.4.357.0-1 | 1.4.363.0-1 | Apache-2.0 |
| `pulseaudio` | `pulse/pulseaudio`, five modules in `pulse/modules/`, `libpulse.so.0`, `libpulsecommon-17.0.so`, `libpulsecore-17.0.so`, `libpulsedsp.so`, `libprotocol-native.so`, `libprotocol-simple.so` | 17.0+dfsg1-3 | 17.0+dfsg1-3 | LGPL-2.1+ |
| `libsndfile` | `libsndfile.so.1` | 1.2.2-4+b1 | 1.2.2-4+b1 | LGPL-2.1+ |
| `lame` | `libmp3lame.so.0` | 4.0-1 | 4.0-1 | LGPL-2+ |
| `mpg123` | `libmpg123.so.0` | 1.33.7-1 | 1.33.7-1 | LGPL-2.1 |
| `libsoxr` | `libsoxr.so.0` | 0.1.3-4.1+b1 | 0.1.3-4.1+b1 | LGPL-2.1+ |
| `libasyncns` | `libasyncns.so.0` | 0.8-8 | 0.8-8 | LGPL-2.1+ |
| `systemd` | `libsystemd.so.0`, `libudev.so.1` | 261.2-1 | 262-1 | LGPL-2.1+ |
| `lm-sensors` | `libsensors.so.5` | 1:3.6.2-2+b2 | 1:3.6.2-2+b2 | LGPL-2.1+ |
| `libusb-1.0` | `libusb-1.0.so.0` | 2:1.0.30-1 | 2:1.0.30-1 | LGPL-2.1+ |
| `samba` (`libtdb1`) | `libtdb.so.1` | 2:1.4.15+samba4.25.0+dfsg-1 | 2:1.4.15+samba4.25.0+dfsg-1 | LGPL-3+ |
| `elfutils` | `libelf.so.1` | 0.196-1 | 0.196-1 | LGPL-3+ or GPL-2+ |
| `libtool` | `libltdl.so.7` | 2.6.2-2 | 2.6.2-3 | LGPL-2+ with a linking exception (as stated in the libltdl sources; Debian's copyright file lists GPL-2+) |
| `dbus` | `libdbus-1.so.3` | 1.16.2-5+b1 | 1.16.2-8 | AFL-2.1 or GPL-2+ |
| `libcap2` | `libcap.so.2` | 1:2.78-1+b1 | 1:2.78-1+b1 | BSD-3-Clause or GPL-2 |
| `libzstd` | `libzstd.so.1` | 1.5.7+dfsg-4 | 1.5.7+dfsg-4 | BSD-3-Clause or GPL-2 |
| `freetype` | `libfreetype.so.6` | 2.14.3+dfsg-2 | 2.14.3+dfsg-3 | FTL or GPL-2+ |
| `flac`, `libogg`, `libvorbis`, `opus`, `speexdsp`, `orc` | `libFLAC.so.14`, `libogg.so.0`, `libvorbis.so.0`, `libvorbisenc.so.2`, `libopus.so.0`, `libspeexdsp.so.1`, `liborc-0.4.so.0` | 1.5.0+ds-5+b1, 1.3.6-2+b1, 1.3.7-3+b2, 1.6.1-1+b1, 1.2.1-4, 1:0.4.44-1 | same | BSD-3-Clause (orc: BSD-2-Clause) |
| X11 client libraries: `libx11`, `libxcb`, `libxau`, `libxdmcp`, `libxext`, `libxfixes`, `libxi`, `libxinerama`, `libxrandr`, `libxrender`, `libxcursor`, `libxss`, `libxxf86vm`, `libxshmfence` | `libX11`, `libX11-xcb`, `libxcb` and 8 extensions, `libXau`, `libXdmcp`, `libXext`, `libXfixes`, `libXi`, `libXinerama`, `libXrandr`, `libXrender`, `libXcursor`, `libXss`, `libXxf86vm`, `libxshmfence` | 2:1.8.13-1, 1.17.0-2+b2, 1:1.0.11-1+b2, 1:1.1.5-2+b1, 2:1.3.4-1+b4, 1:6.0.0-2+b5, 2:1.8.3-1, 2:1.1.4-3+b5, 2:1.5.4-1+b4, 1:0.9.12-1+b2, 1:1.2.3-1+b2, 1:1.2.3-1+b4, 1:1.1.4-2+b1, 1.3.3-1+b2 | same | MIT/X11 |
| `libxkbcommon` | `libxkbcommon.so.0`, `libxkbcommon-x11.so.0` | 1.13.1-1 | 1.13.1-1 | MIT |
| `wayland` | `libwayland-client.so.0` | 1.26.0-1 | 1.26.0-1 | MIT |
| `libdrm` | `libdrm.so.2`, `libdrm_amdgpu.so.1`, `libdrm_intel.so.1` (x86_64 only) | 2.4.134-3 | 2.4.134-3 | MIT |
| `libpciaccess` | `libpciaccess.so.0` (x86_64 only) | 0.19-2 | — | MIT |
| `libdisplay-info` | `libdisplay-info.so.3` | 0.3.0-1+b1 | 0.3.0-1+b1 | MIT |
| `expat` | `libexpat.so.1` | 2.8.4-2 | 2.9.0-1 | MIT |
| `libffi` | `libffi.so.8` | 3.8.0-2 | 3.8.0-2 | MIT |
| `libxml2` | `libxml2.so.16` | 2.15.4+dfsg-1 | 2.15.4+dfsg-1 | MIT |
| `z3` | `libz3.so.4` | 4.13.3-1.1 | 4.13.3-1.1 | MIT |
| `brotli` | `libbrotlicommon.so.1`, `libbrotlidec.so.1` | 1.2.0-4+b1 | 1.2.0-4+b1 | MIT |
| `fontconfig` | `libfontconfig.so.1` | 2.17.1-5 | 2.17.1-5 | MIT-style (HPND) |
| `ncurses` | `libtinfo.so.6` | 6.6+20260608-2 | 6.6+20260608-2 | MIT/X11 |
| `libedit` | `libedit.so.2` | 3.1-20260512-1 | 3.1-20260512-1 | BSD-3-Clause |
| `libbsd` | `libbsd.so.0` | 0.12.2-3 | 0.12.2-3 | BSD-3-Clause and other BSD-style |
| `libmd` | `libmd.so.0` | 1.2.0-2 | 1.3.0-1 | BSD-style, ISC, public domain |
| `tcp-wrappers` | `libwrap.so.0` | 7.6.q-37 | 7.6.q-37 | TCP Wrappers license (BSD-style) |
| `bzip2` | `libbz2.so.1.0` | 1.0.8-6+b2 | 1.0.8-6+b2 | bzip2-1.0.6 |
| `zlib` | `libz.so.1` | 1:1.3.dfsg+really1.3.2-3 | 1:1.3.dfsg+really1.3.2-3 | Zlib |
| `libpng1.6` | `libpng16.so.16` | 1.6.58-1 | 1.6.59-1 | libpng-2.0 |
| `fonts-dejavu` | `fonts/DejaVuSans.ttf`, `DejaVuSans-Bold.ttf`, `DejaVuSansMono.ttf` | 2.37-9 | 2.37-9 | Bitstream Vera license, public domain |
| `shared-mime-info` | `share/mime/` (compiled MIME database) | 2.4-5+b3 | 2.4-5+b3 | GPL-2+ |

Not from Debian packages:

| File | What it is | License | Source |
|---|---|---|---|
| `libvulkan_freedreno_kgsl.so` (aarch64 only) | Mesa's Turnip Vulkan driver, built for Android's KGSL interface from unmodified Mesa 26.2.5 (`-Dfreedreno-kmds=kgsl`) | MIT | <https://archive.mesa3d.org/mesa-26.2.5.tar.xz> |
| `lvp_icd.<arch>.json`, `turnip_icd.aarch64.json`, `50_mesa.json` | Driver manifests copied from the Debian Mesa / libglvnd packages, with the library path made relative | MIT | as above |
| `witness`, `witness-mt`, `witness-vk`, `draw-xcb`, `libpathshim.so`, `fonts.conf`, `overcommit_memory`, `RUNTIME` | GameFlex's own files | MIT | — |

The archives do not contain the Debian copyright files; the license of each package is in its
Debian source package (`debian/copyright`), reproduced in
[THIRD-PARTY-LICENSES.txt](THIRD-PARTY-LICENSES.txt), see also
[below](#finding-the-licenses-and-sources-yourself).

### `xvfb-bundle-x86_64.zip` and `xvfb-bundle-aarch64.zip`

Xvfb (the X.Org virtual framebuffer X server) and the libraries it needs, taken from the binary
packages of the [Termux](https://termux.dev) repositories (`packages.termux.dev`) on 2026-09-13.
Both archives carry the same packages, for their own architecture.

**These binaries were modified after download** by GameFlex's assembly script: in every ELF file,
the hard-coded Termux installation prefix (`/data/data/com.termux/files/usr`) was rewritten in
place to the app's own directory, Xvfb's path to its shell was rewritten to `/system/bin/sh`, and
the path of the X socket was rewritten to the standard `/tmp/.X11-unix`. No code was changed.

The archives include, in `lic/`, the copyright files that the Termux packages ship (21 packages,
marked "yes" below).

| Termux package | Files | Version found | License | `lic/` |
|---|---|---|---|---|
| `xorg-server-xvfb` | `bin/Xvfb` | 21.1.16 | MIT/X11 | no |
| `xorg-xkbcomp` | `bin/xkbcomp` | 1.5.0 | MIT / HPND | yes |
| `xkeyboard-config` | `share/X11/xkb/` | — | MIT-style | yes |
| `mesa` | `lib/libgallium-26.2.2.so`, `lib/dri/*`, `lib/gbm/dri_gbm.so`, `libEGL_mesa`, `libGLX_mesa`, `libgbm` | 26.2.2 | MIT | no |
| `libglvnd` | `libGL`, `libGLX`, `libEGL`, `libGLES*`, `libOpenGL`, `libGLdispatch` | 1.7.0 | MIT-style | no |
| `libllvm` | `lib/libLLVM-21.so`, `libLTO.so`, `libRemarks.so`, `lib/cmake/llvm/` | 21 | Apache-2.0 WITH LLVM-exception | no |
| `libdrm` | `libdrm.so` (aarch64 also `libdrm_freedreno`, `libdrm_etnaviv`) | — | MIT | no |
| `libx11` | `libX11.so`, `libX11-xcb.so`, `share/X11/` data | — | MIT/X11 | no |
| `libxcb` | `libxcb.so` and 27 extension libraries | — | MIT | yes |
| `libxau`, `libxdmcp`, `libxext`, `libxxf86vm`, `libxshmfence`, `libxfont2`, `libfontenc`, `libxkbfile` | the corresponding `lib*.so` | — | MIT/X11 | yes |
| `libwayland` | `libwayland-client`, `-server`, `-cursor`, `-egl` | — | MIT | yes |
| `libpixman` | `libpixman-1.so` | 0.46.4 | MIT | yes |
| `libexpat` | `libexpat.so.1.12.4` | 2.8.4 | MIT | yes |
| `libffi` | `libffi.so` | 3.8.0 | MIT | yes |
| `libxml2` | `libxml2.so.16.1.4` | 2.15.4 | MIT | yes |
| `brotli` | `libbrotlicommon`, `libbrotlidec`, `libbrotlienc` | — | MIT | yes |
| `libpng` | `libpng16.so` | 1.6.58 | libpng-2.0 | yes |
| `zlib` | `libz.so.1.3.2` | 1.3.2 | Zlib | yes |
| `libbz2` | `libbz2.so.1.0.8` | 1.0.8 | bzip2-1.0.6 | yes |
| `libandroid-shmem` | `libandroid-shmem.so` | — | BSD-3-Clause | yes |
| `libandroid-support` | `libandroid-support.so` | — | Apache-2.0, MIT | no |
| `libicu` | `libicu*.so.78.3` | 78.3 | Unicode-3.0 (ICU) | no |
| `libiconv` | `libiconv.so`, `libcharset.so` | 1.19 | LGPL-2.1+ | no |
| `liblzma` | `liblzma.so.5.8.4` | 5.8.4 | 0BSD | no |
| `zstd` | `libzstd.so.1.5.7` | 1.5.7 | BSD-3-Clause or GPL-2.0 | no |
| `freetype` | `libfreetype.so` | — | FTL or GPL-2+ | no |
| `openssl` | `libcrypto.so.3`, `libssl.so.3`, `engines-3/`, `ossl-modules/` | 3.6.3 | Apache-2.0 | no |

Termux build recipes and package sources: <https://github.com/termux/termux-packages>. Upstream
sources: X.Org <https://gitlab.freedesktop.org/xorg>, Mesa <https://mesa3d.org>, LLVM
<https://llvm.org>, ICU <https://icu.unicode.org>, GNU libiconv
<https://www.gnu.org/software/libiconv/>, OpenSSL <https://www.openssl.org>.

---

## Copyleft components and their source code

GameFlex's own code is MIT. The following copyleft-licensed components are redistributed in
binary form in the artifacts above:

| Component | License | Artifacts | Corresponding source |
|---|---|---|---|
| GNU C Library | LGPL-2.1+ | every Docker image; `glibc-*.zip`; the APK (`libglibcloader.so`) | Debian source package `glibc` of the version listed for each artifact |
| FFmpeg 9.0.2 (shared libraries, loaded by `streamex`) | LGPL-2.1+ | the eight console images | inside each console image (`/usr/share/doc/gameflex/sources/`), and <https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz> (unmodified; configuration above) |
| FFmpeg (Debian builds, `--enable-gpl`), x264, x265, Xvid, Rubber Band, vid.stab | GPL-2+ | `gameflex-retrogaming`, the eight console images | Debian source packages, versions above |
| Ghostscript library | AGPL-3+ | the eight console images | Debian source package `ghostscript` 10.05.1~dfsg-1+deb13u3 |
| Openbox, BlueZ library, wmctrl, Wget, base system tools | GPL-2+ / GPL-3+ | console images; Wget in four service images | Debian source packages, versions above |
| PulseAudio, GTK, GLib, GStreamer, GnuTLS, AT-SPI2, libusb, other LGPL libraries | LGPL-2.1+ (and others as listed) | Docker images; `glibc-*.zip` | Debian source packages, versions above |
| libstdc++, libgcc_s, libgomp | GPL-3+ WITH GCC-exception-3.1 | Docker images; `glibc-*.zip` | Debian source package `gcc-16` 16.2.0-3 (archives), `gcc-12` / `gcc-14` (images) |
| shared-mime-info database | GPL-2+ | `glibc-*.zip` | Debian source package `shared-mime-info` 2.4-5+b3 |
| GNU libiconv | LGPL-2.1+ | `xvfb-bundle-*.zip` | <https://ftp.gnu.org/pub/gnu/libiconv/libiconv-1.19.tar.gz> and the Termux recipe |

Debian source packages are kept, for every version ever published, at
`https://snapshot.debian.org/package/<source>/<version>/` (for example
<https://snapshot.debian.org/package/glibc/>).

**Written offer.** For at least three years after we last distribute a given version of a GameFlex
artifact, we will give anyone who asks a complete, machine-readable copy of the corresponding
source code of any GPL, LGPL or AGPL component it contains, for no more than our cost of
physically performing this distribution. Ask on the
[issues page](https://github.com/zinosaure/gameflex/issues), naming the artifact, its version and
the component.

**The licenses travel with the artifacts.** Every GameFlex Docker image carries this file,
[THIRD-PARTY-LICENSES.txt](THIRD-PARTY-LICENSES.txt) and GameFlex's [LICENSE](LICENSE) in
`/usr/share/doc/gameflex/`, next to the Debian copyright files in `/usr/share/doc/<package>/`. The
Android app carries them in its web assets (served by the phone at `/licenses/`), and the runtime
archives under `lic/gameflex/`.

---

## Finding the licenses and sources yourself

- **All the texts in one file.** [THIRD-PARTY-LICENSES.txt](THIRD-PARTY-LICENSES.txt) reproduces
  the license and notice files of every Go module, npm package, Android library and Rust crate
  listed here, FFmpeg's, and the copyright files of the Debian and Termux packages of the runtime
  archives. Like this file, it is in every Docker image at `/usr/share/doc/gameflex/`.
- **Debian packages in a Docker image.** The list of packages, with the source package and
  version each one was built from:

  ```sh
  docker run --rm --entrypoint dpkg-query zinosaure/gameflex-ps3 \
      -W -f='${Package} ${Version} ${source:Package} ${source:Version}\n'
  ```

  The license of each package is in `/usr/share/doc/<package>/copyright` inside the image. Its
  source is the Debian source package of the same version, at
  `https://snapshot.debian.org/package/<source>/<version>/`, or with `apt-get source` from a
  Debian system.
- **Go modules in a GameFlex binary.** `go version -m <binary>` prints every module and version
  linked into it. Each module's license is in its source at that version, also shown at
  `https://pkg.go.dev/<module>@<version>`.
- **npm packages in the web interface.** The license comments kept by the minifier are in the
  `*.LICENSE.txt` files next to each bundle file. Each package's full license is in its published
  package, `https://www.npmjs.com/package/<name>/v/<version>`.
- **Android libraries.** Each AndroidX, Kotlin and ZXing library is published with its license on
  Google's Maven repository and Maven Central.

## Notices reproduced as required

### Docker (`github.com/docker/docker`)

`github.com/docker/docker` v27.3.1+incompatible, linked into `gameflex-kernel`, is licensed under
the Apache License 2.0 and includes the following NOTICE:

```text
Docker
Copyright 2012-2017 Docker, Inc.

This product includes software developed at Docker, Inc. (https://www.docker.com).

This product contains software (https://github.com/creack/pty) developed
by Keith Rarick, licensed under the MIT License.

The following is courtesy of our legal counsel:


Use and transfer of Docker may be subject to certain restrictions by the
United States and other governments.
It is your responsibility to ensure that your use and/or transfer does not
violate applicable laws.

For more information, please see https://www.bis.doc.gov

See also https://www.apache.org/dev/crypto.html and/or seek legal counsel.
```
