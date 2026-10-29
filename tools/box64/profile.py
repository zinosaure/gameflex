"""Où shadPS4 passe son temps sur le téléphone.

Lit un enregistrement simpleperf (perf.data) et la carte du code traduit que
Box64 écrit (BOX64_DYNAREC_PERFMAP : une ligne par instruction x86 traduite,
« adresse taille module/fonction + décalage:instruction », les lignes longues
tronquées et collées). Chaque échantillon tombe :
  - dans un binaire natif (box64, le pilote Vulkan, la libc…) : son DSO et son
    symbole, tels que simpleperf les nomme ;
  - dans le code traduit : le module x86 et la fonction que la carte donne ;
  - ailleurs dans de la mémoire anonyme : « traduit, hors carte ».

La carte fait plus d'un gigaoctet : on relève d'abord les adresses des
échantillons, puis on la lit au fil de l'eau en ne gardant que les entrées qui
en contiennent une (la dernière écrite l'emporte, un bloc retraduit réécrit
son entrée).

usage : python3 profile.py perf.data perf-PID.map [symfs]

Où le lancer : là où sont les scripts de simpleperf du NDK (le conteneur qui
porte le SDK Android et adb), avec perf.data et la carte rapatriés du
téléphone ; symfs, un dossier qui reprend le chemin du téléphone du `box64`
du paquet (il porte ses symboles). Le relevé lui-même, sur le téléphone :
un `.box64rc` dans le dossier de l'invité (`[shadps4]`,
`BOX64_DYNAREC_PERFMAP=1`), puis `run-as <paquet> simpleperf record -p <pid>
--duration 30 -e cpu-clock -f 1000` — voir le journal, entrée 302 (#506).
"""
import bisect
import collections
import os
import re
import sys

# Les scripts de simpleperf du NDK (SIMPLEPERF_SCRIPTS), là où le SDK est monté.
sys.path.insert(0, os.environ.get("SIMPLEPERF_SCRIPTS", "/sdk/ndk/26.3.11579264/simpleperf"))
from simpleperf_report_lib import ReportLib  # noqa: E402

ENTRY = re.compile(rb"(0x[0-9a-f]{6,16}) (\d+) ")


def read_samples(path, symfs):
    lib = ReportLib()
    lib.SetRecordFile(path)
    if symfs:
        lib.SetSymfs(symfs)
    lib.ShowIpForUnknownSymbol()
    samples = []
    while True:
        sample = lib.GetNextSample()
        if sample is None:
            break
        symbol = lib.GetSymbolOfCurrentSample()
        samples.append((sample.ip, sample.thread_comm, symbol.dso_name, symbol.symbol_name))
    lib.Close()
    return samples


def name_jit(ips, map_path):
    """{ip: nom} pour chaque ip échantillonnée que la carte couvre."""
    wanted = sorted(set(ips))
    names = {}
    tail = b""
    with open(map_path, "rb") as stream:
        while True:
            chunk = stream.read(64 << 20)
            if not chunk:
                break
            data = tail + chunk
            matches = list(ENTRY.finditer(data))
            # La dernière entrée du morceau peut être coupée : on la reporte.
            keep = matches[:-1] if len(matches) > 1 else []
            for index, match in enumerate(keep):
                start = int(match.group(1), 16)
                size = max(int(match.group(2)), 4)
                at = bisect.bisect_left(wanted, start)
                if at < len(wanted) and wanted[at] < start + size:
                    end = matches[index + 1].start()
                    name = data[match.end():end].split(b"\n")[0].decode("utf-8", "replace")
                    while at < len(wanted) and wanted[at] < start + size:
                        names[wanted[at]] = name
                        at += 1
            tail = data[matches[-1].start():] if matches else data
    return names


def module_and_function(name):
    """« /…/bin/shadps4/_ZN…fi + 0x12:MOV » → (« shadps4 », « _ZN…fi »)."""
    if name.startswith("box64/"):
        return "box64 (spécial)", name.split(":")[0]
    head = name.split(" + ")[0]
    for marker in ("/bin/shadps4", "/lib/", "/x64lib/", "/box64lib/"):
        at = head.find(marker)
        if at >= 0:
            parts = head[at + 1:].split("/")
            if marker == "/bin/shadps4":
                return "shadps4", "/".join(parts[2:]) or "?"
            return parts[1], "/".join(parts[2:]) or "?"
    return "autre : " + head.split("/")[0][:50], "/".join(head.split("/")[1:])[:80] or "?"


def main():
    samples = read_samples(sys.argv[1], sys.argv[3] if len(sys.argv) > 3 else None)
    total = len(samples)
    anonymous = [ip for ip, _, dso, _ in samples if dso.startswith("[anon") or dso.startswith("//anon") or dso in ("", "unknown")]
    names = name_jit(anonymous, sys.argv[2])
    by_kind = collections.Counter()
    by_thread = collections.Counter()
    by_native = collections.Counter()
    by_module = collections.Counter()
    by_function = collections.Counter()
    by_thread_kind = collections.Counter()
    by_dso_raw = collections.Counter()
    for ip, thread, dso, symbol in samples:
        by_thread[thread] += 1
        by_dso_raw[dso[:60]] += 1
        if ip in names:
            module, function = module_and_function(names[ip])
            kind = "traduit"
            by_module["traduit : " + module] += 1
            by_function[module + " | " + function] += 1
        elif dso.startswith("[anon") or dso.startswith("//anon") or dso in ("", "unknown"):
            kind = "traduit, hors carte"
            by_module["(anonyme hors carte) " + dso[:30]] += 1
        else:
            short = dso.rsplit("/", 1)[-1]
            kind = "noyau" if short.startswith("[kernel") else "natif"
            by_native[short + " | " + symbol[:80]] += 1
            by_module[kind + " : " + short] += 1
        by_kind[kind] += 1
        by_thread_kind[thread + " | " + kind] += 1

    def show(title, counter, limit):
        print(f"\n== {title}")
        for key, count in counter.most_common(limit):
            print(f"{100.0 * count / total:6.1f} %  {key}")

    print(f"échantillons : {total} ; adresses traduites nommées : {len(names)}")
    show("nature", by_kind, 10)
    show("DSO bruts", by_dso_raw, 12)
    show("modules", by_module, 25)
    show("fils", by_thread, 20)
    show("fil × nature", by_thread_kind, 30)
    show("natif et noyau : fonctions", by_native, 45)
    show("traduit : fonctions", by_function, 60)


main()
