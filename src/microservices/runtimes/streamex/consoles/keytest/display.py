#!/usr/bin/env python3
"""KeyTest display — fullscreen tkinter window rendered on the session's X11
screen (captured + streamed over WebRTC by streamex).

It receives input events from the streamex keytest adapter over localhost
and renders a live log: every press lands on the top row with its receive
time and the press→receive latency — both on the server clock, so the
streamed video itself shows how long a key really took to arrive. Rows are
prefixed with the player's port (P1..P4) so multiplayer latency is measured
per player, a key that stays down produces a "(Released)" row when it comes
up (so a held key never hides the chain it belongs to), hall-effect trigger
pressure lands on its own rows (R2 (91%)), and stick positions are shown
live above the table.

Header shows the server time plus live stream stats polled from streamex's
/status: current/target FPS and the capture resolution.
"""

import json
import math
import os
import queue
import subprocess
import threading
import time
import tkinter as tk
import urllib.request
import zlib
from http.server import BaseHTTPRequestHandler, HTTPServer

MAX_ROWS = 20
# Display-only: a key must stay down at least this long to get a "(Released)"
# row — quick taps would spam the table with useless pairs. The real input
# pipeline is untouched (presses AND releases are always forwarded).
RELEASED_ROW_MIN_HOLD_MS = 1000
# A stick position not refreshed for longer than this disappears from the
# screen (a released stick sends one final 0,0 — this fades idle players).
STICK_STALE_MS = 5000
# Presses closer than this are ONE roll — a d-pad quarter-circle arrives as
# DOWN + DOWN_RIGHT + RIGHT on a single line. Overridable at runtime through
# POST /config (the client's "forgiving chain window" toggle, 100/300 ms
# today — see Blackscreen.jsx's CHAIN_WINDOW_OPTIONS_MS). Matches that
# toggle's own default (300ms): the client now sends its initial value on
# connect (Player.jsx's "session" handler), but this stays in sync with it
# regardless, rather than silently defaulting to something the UI never
# actually offers.
CHAIN_WINDOW_MS = 300

# Pad skins: the display labels follow the pad skin selected in the client
# (the "sk,<skin>" wire command → POST /config). PS3 shows PlayStation
# names (CROSS, CIRCLE, L1…), Switch shows Nintendo names (A, B, ZL…).
# Keyed by the profile KEY (gp:A, gp:L2…) — unknown keys fall back to the
# profile label. Canonical KeyLab codes (CROSS…) are not keys and render
# unchanged.
SKIN_LABELS = {
    "ps3": {
        "gp:A": "CROSS", "gp:B": "CIRCLE", "gp:X": "SQUARE", "gp:Y": "TRIANGLE",
        "gp:L1": "L1", "gp:L2": "L2", "gp:L3": "L3",
        "gp:R1": "R1", "gp:R2": "R2", "gp:R3": "R3",
        "gp:Start": "OPTIONS", "gp:Select": "SHARE", "gp:PS": "PS",
        "gp:Touchpad": "TOUCHPAD",
        "gp:Up": "DPAD_UP", "gp:Down": "DPAD_DOWN",
        "gp:Left": "DPAD_LEFT", "gp:Right": "DPAD_RIGHT",
        "gp:UpLeft": "DPAD_UP_LEFT", "gp:UpRight": "DPAD_UP_RIGHT",
        "gp:DownLeft": "DPAD_DOWN_LEFT", "gp:DownRight": "DPAD_DOWN_RIGHT",
    },
    "switch": {
        "gp:A": "A", "gp:B": "B", "gp:X": "X", "gp:Y": "Y",
        "gp:L1": "L", "gp:L2": "ZL", "gp:L3": "LS",
        "gp:R1": "R", "gp:R2": "ZR", "gp:R3": "RS",
        "gp:Start": "+", "gp:Select": "−", "gp:PS": "HOME",
        "gp:Up": "UP", "gp:Down": "DOWN",
        "gp:Left": "LEFT", "gp:Right": "RIGHT",
        "gp:UpLeft": "UP_LEFT", "gp:UpRight": "UP_RIGHT",
        "gp:DownLeft": "DOWN_LEFT", "gp:DownRight": "DOWN_RIGHT",
    },
}
SKIN = "ps3"  # active pad skin (client-driven at runtime)

# Canonical KeyLab codes → per-skin label. The gateway sends CANONICAL
# codes (not profile keys), so the switch skin needs its own translation —
# PS3 canonical names already render as-is.
CANON_SKIN_LABELS = {
    "switch": {
        "CROSS": "A", "CIRCLE": "B", "SQUARE": "X", "TRIANGLE": "Y",
        "L1": "L", "L2": "ZL", "L3": "LS",
        "R1": "R", "R2": "ZR", "R3": "RS",
        "START": "+", "SELECT": "−", "PS": "HOME",
        "DPAD_UP": "UP", "DPAD_DOWN": "DOWN",
        "DPAD_LEFT": "LEFT", "DPAD_RIGHT": "RIGHT",
        "DPAD_UP_LEFT": "UP_LEFT", "DPAD_UP_RIGHT": "UP_RIGHT",
        "DPAD_DOWN_LEFT": "DOWN_LEFT", "DPAD_DOWN_RIGHT": "DOWN_RIGHT",
    },
}

LABELS = {}
EVENTS = queue.Queue()
ROWS = []  # closed chains, newest-last {slot, time_ms, diff_ms, keys}
PENDING = {}  # slot → chain being built (rendered live on top)
PRESSED = {}  # (slot, code) → press recv_ms, to know if the key was held long
STICKS = {}  # (slot, stick) → (x, y) in -1000..1000
STICK_ACTIVITY = {}  # (slot, stick) → last recv_ms
STATUS = {
    "fps": "—",
    "target_fps": os.environ.get("FPS", "60"),
    "resolution": "—",
}


# LE PORT DE STREAMEX VIENT DE L'ENVIRONNEMENT, jamais d'une constante.
#
# `127.0.0.1:8080` était écrit en dur ici. Or un conteneur de session tourne
# en `network_mode: host` : cette adresse n'est pas celle de streamex, c'est
# celle de la MACHINE. Si un autre service y écoute — c'était le cas — ces
# deux boucles l'interrogent LUI, indéfiniment, et reçoivent 404. Repéré le
# 2026-09-09 dans le journal du voisin : des milliers d'appels à /status et
# /profile.json en Python-urllib, plus de la moitié de son trafic.
#
# Le kernel passe `STREAMEX_PORT` au conteneur, et c'est déjà ce que les
# healthchecks des cinq autres consoles lisent. Le repli sur 8080 garde le
# cas où streamex tourne seul dans son propre réseau, là où 8080 est vraiment
# le sien.
STREAMEX_BASE = "http://127.0.0.1:" + os.environ.get("STREAMEX_PORT", "8080")


def fetch_json(path, timeout=1):
    with urllib.request.urlopen(STREAMEX_BASE + path, timeout=timeout) as response:
        return json.load(response)


def fetch_labels():
    """Resolves button key → label from streamex's /profile.json (retries
    until the server is up — the display may start first)."""
    for _ in range(120):
        try:
            profile = fetch_json("/profile.json")
            for button in profile.get("buttons", []):
                LABELS[button["key"]] = button.get("label") or button["id"]
            return
        except Exception:
            time.sleep(1)


def poll_status():
    """Live stream stats for the header (FPS and resolution)."""
    while True:
        try:
            status = fetch_json("/status")
            fps = status.get("fps") or {}
            encoded = fps.get("encoded")
            if isinstance(encoded, (int, float)):
                STATUS["fps"] = str(int(round(encoded)))
            res = status.get("resolution") or {}
            if res.get("capture"):
                STATUS["resolution"] = res["capture"]
            elif res.get("target"):
                STATUS["resolution"] = res["target"]
        except Exception:
            pass
        time.sleep(1)


def label_for(code):
    skin_labels = SKIN_LABELS.get(SKIN, {})
    if code in skin_labels:
        return skin_labels[code]
    canon = CANON_SKIN_LABELS.get(SKIN, {})
    if code in canon:
        return canon[code]
    return LABELS.get(code, code)


# ───────────────────────────── Button sounds ─────────────────────────────
# One pitch per button (a C-major-ish ladder), streamed through pacat into
# the Pulse sink the session streams (keytest_sink.monitor) — the sound
# STAYS while the key is held: press starts a continuous tone, release
# stops it. Quick taps get a minimum blip so they stay audible. Server-side
# only, off-thread: it never perturbs the latency measurement itself.

SOUND_ENABLED = True
SOUND_VOLUME = 26000  # pacat --volume, 0..65535 (~40%)
SOUND_RATE = 22050
SOUND_MIN_BLIP_MS = 60
SOUND_CHUNK = 1024  # samples per stdin write

# Keyed by the PS3-skin canonical name (gp:* profile keys resolve through
# SKIN_LABELS["ps3"], canonical KeyLab codes pass through unchanged).
SOUND_FREQ = {
    "CROSS": 523.25, "CIRCLE": 587.33, "SQUARE": 659.25, "TRIANGLE": 698.46,
    "L1": 783.99, "R1": 880.00,
    "L2": 220.00, "R2": 246.94,
    "UP": 392.00, "DOWN": 329.63, "LEFT": 349.23, "RIGHT": 369.99,
    "START": 466.16, "SELECT": 415.30, "PS": 261.63, "TOUCHPAD": 293.66,
    "L3": 196.00, "R3": 207.65,
    "DPAD_UP_LEFT": 311.13, "DPAD_UP_RIGHT": 329.63,
    "DPAD_DOWN_LEFT": 277.18, "DPAD_DOWN_RIGHT": 293.66,
}


def canonical_for(code):
    return SKIN_LABELS["ps3"].get(code, code)


def freq_for(code):
    canonical = canonical_for(code)
    if canonical in SOUND_FREQ:
        return SOUND_FREQ[canonical]
    # Stable per-code fallback (hash() is salted per process — crc32 is not).
    return 280 + (zlib.crc32(code.encode()) % 440)


class TonePlayer:
    """One continuous tone per (slot, code) — mixed by Pulse, captured by
    the stream. A held key keeps sounding until its release (or a retrigger)."""

    def __init__(self):
        self._processes = {}  # (slot, code) → (Popen, start_ms)
        self._lock = threading.Lock()

    def start(self, slot, code):
        if not SOUND_ENABLED:
            return
        key = (slot, code)
        self.stop(key)
        process = subprocess.Popen(
            [
                "pacat",
                f"--rate={SOUND_RATE}",
                "--channels=1",
                "--format=s16le",
                f"--volume={SOUND_VOLUME}",
            ],
            stdin=subprocess.PIPE,
        )
        with self._lock:
            self._processes[key] = (process, now_ms())

        freq = freq_for(code)

        def feed():
            step = 2.0 * math.pi * freq / SOUND_RATE
            phase = 0.0
            samples = bytearray()
            try:
                while True:
                    samples.clear()
                    for _ in range(SOUND_CHUNK):
                        phase += step
                        value = int(32767 * math.sin(phase))
                        samples += value.to_bytes(2, "little", signed=True)
                    process.stdin.write(samples)
                    process.stdin.flush()
            except (BrokenPipeError, OSError, ValueError):
                pass  # killed on release — expected

        threading.Thread(target=feed, daemon=True).start()

    def stop(self, key):
        with self._lock:
            entry = self._processes.pop(key, None)
        if entry is None:
            return
        process, started = entry
        # A quick tap would otherwise be silent (the tone dies the instant
        # the release lands) — give it a minimum audible blip.
        def kill():
            try:
                process.terminate()
                process.wait(timeout=0.5)
            except Exception:
                try:
                    process.kill()
                except Exception:
                    pass

        held = now_ms() - started
        if held < SOUND_MIN_BLIP_MS:
            threading.Timer((SOUND_MIN_BLIP_MS - held) / 1000.0, kill).start()
        else:
            kill()

    def stop_all(self):
        with self._lock:
            keys = list(self._processes)
        for key in keys:
            self.stop(key)


SOUNDS = TonePlayer()


class EventHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        # Debug endpoint: exposes the current display state so external
        # tests can assert events really reached the screen app.
        body = json.dumps(
            {
                "rows": all_rows(),
                "sent_rows": sent_rows(),
                "chain_window_ms": CHAIN_WINDOW_MS,
                "skin": SKIN,
                "sticks": stick_lines(),
                "sound_enabled": SOUND_ENABLED,
            }
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        try:
            payload = json.loads(self.rfile.read(length))
        except Exception:
            self.send_response(400)
            self.end_headers()
            return
        if self.path == "/config":
            window = payload.get("chain_window_ms")
            if isinstance(window, int) and 10 <= window <= 2000:
                global CHAIN_WINDOW_MS
                CHAIN_WINDOW_MS = window
            skin = payload.get("skin")
            if skin == "ps5":      # ancien nom du gabarit PS3
                skin = "ps3"
            if skin in SKIN_LABELS:
                global SKIN
                SKIN = skin
            sound = payload.get("sound")
            if isinstance(sound, bool):
                global SOUND_ENABLED
                SOUND_ENABLED = sound
                if not sound:
                    SOUNDS.stop_all()
            self.send_response(204)
            self.end_headers()
            return
        # UN ÉVÉNEMENT SANS TOUCHE N'EST PAS UN ÉVÉNEMENT.
        #
        # `/event` prenait n'importe quel JSON. Un corps sans `code` passait
        # quand même : `kind` valait « press » par défaut, la touche « ? » et
        # l'horloge zéro — d'où la ligne « P1 00:00:00.000 — ? » qui restait
        # sous les vraies jusqu'à ce que vingt appuis l'en chassent. Elle
        # n'apprenait rien et faisait douter du reste (signalée le
        # 2026-09-09 ; reproduite en postant un corps de /config sur /event,
        # qui rend cette ligne au caractère près).
        #
        # Aucun expéditeur légitime n'est concerné : les deux chemins Go
        # (adapter/keytest.go et keylab/display.go) portent toujours `code`
        # et `recv_ms`, et le relais CSV écarte déjà les codes vides. Ce qui
        # arrivait venait donc d'ailleurs — le conteneur tourne en
        # `network_mode: host`, ce port est celui de la MACHINE et n'importe
        # quelle sonde locale l'atteint. On refuse, et on DIT quoi : la
        # prochaine fois le journal nomme le corps reçu, au lieu de laisser
        # une ligne muette à l'écran.
        code = payload.get("code") if isinstance(payload, dict) else None
        if not isinstance(code, str) or not code:
            print(f"[keytest] événement sans touche, ignoré : {payload}", flush=True)
            self.send_response(400)
            self.end_headers()
            return
        EVENTS.put(payload)
        self.send_response(204)
        self.end_headers()

    def log_message(self, *args):
        pass


# UN PORT PAR SESSION, ET NON UN PORT FIXE.
#
# `26761` était écrit en dur des deux côtés. Les conteneurs de session
# tournent en `network_mode: host` : ce port est celui de la MACHINE, donc une
# seule session keytest pouvait le prendre. La deuxième voyait sa liaison
# refusée — dans un fil démon, sans que rien ne le dise — et son affichage ne
# recevait plus AUCUNE entrée : le conteneur restait « healthy », l'image
# tournait, et seule la table des appuis restait vide (constaté le 2026-09-09
# en lançant deux sessions de test, puis signalé par l'utilisateur).
#
# Le décalage est le même que côté Go (adapter/keytest.go) : le port de
# session (9001-9100) place le serveur en 26761-26860, hors de la plage des
# sessions, et la première session garde le port historique.
DISPLAY_PORT_BASE = 17760
DISPLAY_PORT = int(os.environ.get("STREAMEX_PORT", "9001")) + DISPLAY_PORT_BASE


def start_server():
    def serve():
        # Et si la liaison échoue, le DIRE. Un fil démon qui meurt en silence
        # est ce qui a rendu ce défaut invisible pendant qu'il durait.
        try:
            HTTPServer(("127.0.0.1", DISPLAY_PORT), EventHandler).serve_forever()
        except OSError as error:
            print(f"[keytest] impossible d'écouter sur {DISPLAY_PORT} : {error}", flush=True)

    threading.Thread(target=serve, daemon=True).start()


def now_ms():
    return int(time.time() * 1000)


def clock_text(ms):
    return time.strftime("%H:%M:%S", time.localtime(ms / 1000)) + f".{ms % 1000:03d}"


def format_row(row):
    port = f"P{row['slot'] + 1}"
    diff = f"+{row['diff_ms']} ms" if row["diff_ms"] is not None else "—"
    return f"{port:<4} {clock_text(row['time_ms']):<14} {diff:<8} {' + '.join(row['keys'])}"


def append_row(row):
    ROWS.append(row)
    if len(ROWS) > MAX_ROWS:
        del ROWS[0 : len(ROWS) - MAX_ROWS]


def sent_rows():
    """Structured rows for the frontend key-log panel: the SEND time (the
    client's press/release timestamp on the server clock) and the keys —
    no diff, the panel is about when inputs were sent, not latency."""

    def entry(row):
        return {
            "port": f"P{row['slot'] + 1}",
            "sent_ms": row.get("sent_ms"),
            "recv_ms": row["time_ms"],
            "keys": " + ".join(row["keys"]),
            "raw": " + ".join(row.get("raws") or ["—"]),
        }

    rows = [entry(row) for row in reversed(ROWS)]
    pending = [chain for chain in PENDING.values() if chain["keys"]]
    pending.sort(key=lambda chain: chain["time_ms"], reverse=True)
    rows[0:0] = [entry(chain) for chain in pending]
    return rows


# ROWS holds CLOSED chains; the chains being built show live on top.
def all_rows():
    rows = [format_row(row) for row in reversed(ROWS)]
    pending = [chain for chain in PENDING.values() if chain["keys"]]
    pending.sort(key=lambda chain: chain["time_ms"], reverse=True)
    rows[0:0] = [format_row(chain) for chain in pending]
    return rows


def stick_lines():
    """Live stick positions, one line per active player (P1 L/R)."""
    now = now_ms()
    lines = []
    for slot in sorted({slot for slot, _ in STICK_ACTIVITY}):
        entries = []
        for stick in ("l", "r"):
            position = STICKS.get((slot, stick))
            activity = STICK_ACTIVITY.get((slot, stick), 0)
            if position is None or now - activity > STICK_STALE_MS:
                continue
            entries.append(f"{stick.upper()}: {position[0] / 1000:+.2f},{position[1] / 1000:+.2f}")
        if entries:
            lines.append(f"P{slot + 1}  " + "   ".join(entries))
    return lines


def main():
    threading.Thread(target=fetch_labels, daemon=True).start()
    threading.Thread(target=poll_status, daemon=True).start()
    start_server()

    root = tk.Tk()
    root.title("keytest")
    root.attributes("-fullscreen", True)
    root.configure(bg="#0d0f14")

    tk.Label(
        root,
        text="KEYTEST — input → screen",
        font=("DejaVu Sans", 22, "bold"),
        bg="#0d0f14",
        fg="#ffffff",
    ).pack(pady=(24, 2))

    info = tk.Label(
        root,
        text="",
        font=("DejaVu Sans Mono", 14),
        bg="#0d0f14",
        fg="#8b96a8",
    )
    info.pack(pady=(0, 12))

    sticks = tk.Label(
        root,
        text="",
        font=("DejaVu Sans Mono", 15),
        bg="#0d0f14",
        fg="#7ee787",
        justify="left",
        anchor="w",
    )
    sticks.pack(fill="x", padx=70, pady=(0, 8))

    header = tk.Label(
        root,
        text=f"{'Port':<5}{'Received at':<14}{'Diff':<8}Key(s)",
        font=("DejaVu Sans Mono", 15, "bold"),
        bg="#0d0f14",
        fg="#4da3ff",
        justify="left",
        anchor="w",
    )
    header.pack(fill="x", padx=70, pady=(0, 4))

    history = tk.Label(
        root,
        text="",
        font=("DejaVu Sans Mono", 15),
        bg="#0d0f14",
        fg="#e6e9f0",
        justify="left",
        anchor="nw",
    )
    history.pack(fill="both", expand=True, padx=70, pady=(0, 30))

    def poll():
        while True:
            try:
                event = EVENTS.get_nowait()
            except queue.Empty:
                break

            kind = event.get("kind", "press")
            slot = int(event.get("slot") or 0)
            # do_POST écarte déjà un événement sans touche ; ce repli
            # n'est là que pour ne jamais interrompre la boucle d'affichage.
            code = event.get("code") or "?"
            # Et sans horodatage, l'heure de RÉCEPTION est celle-ci — mieux
            # qu'un 00:00:00.000 qui se lit comme une donnée alors qu'il ne
            # dit que « absent ». Le diff, lui, reste « — » : il demande le
            # temps d'envoi du client, qu'on n'invente pas.
            recv = event.get("recv_ms") or now_ms()
            sent = event.get("sent_ms")
            diff = recv - sent if sent else None
            raw = event.get("raw") or "—"

            if kind == "stick":
                STICKS[(slot, code)] = (int(event.get("x") or 0), int(event.get("y") or 0))
                STICK_ACTIVITY[(slot, code)] = recv
                continue

            name = label_for(code)

            if kind == "pressure":
                # A trigger's pressure lands on its OWN row (the percent is
                # part of the label) — it never merges into a key chain.
                pending = PENDING.get(slot)
                if pending is not None and pending["keys"]:
                    append_row(pending)
                    del PENDING[slot]
                value = max(0, min(1000, int(event.get("value") or 0)))
                append_row({
                    "slot": slot,
                    "time_ms": recv,
                    "diff_ms": diff,
                    "sent_ms": sent,
                    "keys": [f"{name} ({round(value / 10)}%)"],
                    "raws": [raw],
                })
                continue

            if kind == "release":
                # A key held at least RELEASED_ROW_MIN_HOLD_MS comes up on
                # its own "(Released)" row — like a RemotePlay input log,
                # press/release are paired and a held key never hides the
                # chain it belongs to. Quick taps close silently.
                since = PRESSED.pop((slot, code), None)
                if since is not None and recv - since >= RELEASED_ROW_MIN_HOLD_MS:
                    pending = PENDING.get(slot)
                    if pending is not None and pending["keys"]:
                        append_row(pending)
                        del PENDING[slot]
                    append_row({
                        "slot": slot,
                        "time_ms": recv,
                        "diff_ms": diff,
                        "sent_ms": sent,
                        "keys": [f"{name} (Released)"],
                        "raws": [raw],
                    })
                SOUNDS.stop((slot, code))
                continue

            # Press — inside the SAME PLAYER's roll window it extends the
            # pending chain; otherwise the chain closes and a new one opens.
            # Holding a key re-emits nothing here (streamex forwards real
            # presses only), so the chain stays pending until the window
            # expires below.
            PRESSED[(slot, code)] = recv
            SOUNDS.start(slot, code)
            pending = PENDING.get(slot)
            if (
                pending is not None
                and recv - pending["time_ms"] <= CHAIN_WINDOW_MS
                and pending["keys"]
            ):
                if pending["keys"][-1] == name:
                    # Fast double-tap of the same key is folded into the roll.
                    pending["time_ms"] = recv
                    pending["diff_ms"] = diff
                    continue
                pending["keys"].append(name)
                pending["raws"].append(raw)
                pending["time_ms"] = recv
                pending["diff_ms"] = diff
            else:
                if pending is not None and pending["keys"]:
                    append_row(pending)
                PENDING[slot] = {
                    "slot": slot,
                    "keys": [name],
                    "raws": [raw],
                    "time_ms": recv,
                    "diff_ms": diff,
                    "sent_ms": sent,
                }

        # Close chains whose roll window expired, so a held key settles as a
        # fixed row (its xN counter keeps ticking in place).
        now = now_ms()
        for slot, pending in list(PENDING.items()):
            if now - pending["time_ms"] > CHAIN_WINDOW_MS:
                append_row(pending)
                del PENDING[slot]

        history.config(text="\n".join(all_rows()))
        sticks.config(text="\n".join(stick_lines()))
        info.config(
            text=(
                f"Time: {time.strftime('%H:%M:%S')}"
                f" - FPS: {STATUS['fps']}/{STATUS['target_fps']}"
                f" - Resolution: {STATUS['resolution']}"
            )
        )
        root.after(100, poll)

    root.after(100, poll)
    root.mainloop()


if __name__ == "__main__":
    main()
