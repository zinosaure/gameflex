# GameFlex binary license

Copyright (c) 2026 Gino D.

Most of GameFlex is open source: the source code in this repository is under the
[MIT License](LICENSE). A few parts are distributed only as compiled programs, without their source
code. This license covers those parts, and only them:

| Binary | Where it is |
|---|---|
| The video gateway | `/app/gatewaywebrtc` in the `zinosaure/gameflex-gateway-webrtc` image |
| Remote Play | `/app/remoteplay` in the `zinosaure/gameflex-remoteplay` image |
| streamex, the program that runs and streams the recent consoles | `/streamex` in the `zinosaure/gameflex-streamex-bin` image, and `/usr/local/bin/streamex` in each console image |
| The Android app | `gameflex.apk` |

The third-party software inside the images and the app keeps its own license: see
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

## You may

- **Use the binaries**, free of charge.
- **Copy and redistribute them unmodified**, including inside images you build from this
  repository, as long as this license and the notices go with them.

## You may not

- **Sell them**, rent them, or charge a fee for them, alone or as part of something else.
- **Modify them**, or distribute modified copies.
- **Reverse engineer, decompile or disassemble them**, including to extract the keys they carry,
  except where the law that applies to you allows it despite this clause, or where the license of a
  library they use requires it to be allowed.

That last exception matters: some of these binaries use libraries under the GNU Lesser General
Public License (LGPL), such as FFmpeg, PulseAudio and the GNU C library. The LGPL lets you modify
those libraries, use your modified versions with the binaries, and reverse engineer the binaries to
debug such modifications. Nothing in this license restricts what the LGPL allows. The source code of
those libraries is listed in [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

## No warranty

THE BINARIES ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE BINARIES OR THE USE OR OTHER DEALINGS IN THE BINARIES.

## Earlier versions

GameFlex 0.3.0 to 0.8.0 were published under the MIT License alone, their binaries included, and
they remain so. This license applies to the binaries of the versions published with this file.
