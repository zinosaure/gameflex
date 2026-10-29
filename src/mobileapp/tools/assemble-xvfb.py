#!/usr/bin/env python3
# Assemble le bundle Xvfb (serveur X « framebuffer virtuel ») pour Android,
# depuis les dépôts Termux — route B de streamex mobile. L'architecture est le
# premier argument : aarch64 (l'appareil) ou x86_64 (l'émulateur).
#
# Ce que fait le script :
#  1. part de xorg-server-xvfb (+ xkb : xkeyboard-config, xorg-xkbcomp) ;
#  2. tire la FERMETURE transitive des .so via l'index Contents de Termux ;
#  3. copie les arbres usr/{lib,bin,share/X11} en préservant leur structure —
#     Xvfb réclame des données (protocole X, keymaps), pas que des .so ;
#  4. RELOCALISE le préfixe : les binaires Termux ont /data/data/com.termux/
#     files/usr codé en dur (tmp, lib, xkb…), un dossier qui n'est pas le nôtre.
#     On réécrit ce préfixe vers /data/data/app.gameflex.mobile (plus COURT,
#     donc réécriture sur place, sans décaler le binaire) dans chaque ELF ;
#  5. empaquette bin/ + lib/ + share/ + lic/ en ZIP (Java le lit nativement).
#
# Reproductible : relancer quand Termux met à jour. Le bundle ne va PAS dans le
# dépôt (10 Mo) — il se pose sur le dossier de téléchargements de la webapp,
# récupéré par l'appareil au premier usage (comme Citron).
import gzip, os, re, shutil, subprocess, sys, urllib.request

# L'ARCHITECTURE EST UN PARAMÈTRE, pas une constante : le même bundle sert
# l'appareil (aarch64) et l'émulateur (x86_64), et Termux publie les deux aux
# mêmes versions (vérifié le 2026-09-12 : xorg-server-xvfb 21.1.16-3 des deux
# côtés). Sans bundle x86_64, tout ce qui touche l'affichage exigeait l'appareil.
ARCH = sys.argv[1] if len(sys.argv) > 1 else "aarch64"
MACHINE = {"aarch64": "AArch64", "x86_64": "Advanced Micro Devices X86-64"}[ARCH]

REPOS = [("x11", "https://packages.termux.dev/apt/termux-x11",
          f"dists/x11/main/binary-{ARCH}/Packages", f"dists/x11/Contents-{ARCH}.gz"),
         ("main", "https://packages.termux.dev/apt/termux-main",
          f"dists/stable/main/binary-{ARCH}/Packages", f"dists/stable/Contents-{ARCH}.gz")]
W = os.environ.get("XBUNDLE_WORK", f"/tmp/xbundle-{ARCH}")
ROOT = W + "/root"            # miroir du préfixe : lib/, bin/, share/
# GNU readelf lit toutes les architectures ; llvm-readelf (NDK) aussi, même format.
READELF = os.environ.get("READELF", "readelf")
# mesa : le pilote DRI logiciel (lib/dri/swrast_dri.so) qu'Xvfb dlopen pour
# ACTIVER GLX. Sans lui, l'extension est compilée mais aucun écran ne l'offre —
# et un client stock (le SDL2 statique de xemu) répond « GLX is not supported »
# (banc, 2026-09-12). Le prix : sa fermeture tire libllvm (176 Mo installés,
# ~45 Mo de zip). C'est le coût du GLX serveur pour un exécutable qu'on ne
# recompile pas ; un invité construit avec EGL s'en passerait.
SEED = ["xorg-server-xvfb", "xkeyboard-config", "xorg-xkbcomp", "mesa"]

# Le préfixe Termux codé en dur → le nôtre. NOTRE chemin est plus court (30 <
# 31), ce qui permet la réécriture sur place, chaîne par chaîne.
OLD_PREFIX = b"/data/data/com.termux/files/usr"
NEW_PREFIX = b"/data/data/app.gameflex.mobile"

# Fournies par Android — à NE PAS embarquer.
SYS = set("""libc.so libm.so libdl.so liblog.so libandroid.so libz.so libEGL.so
libGLESv1_CM.so libGLESv2.so libGLESv3.so libvulkan.so libnativewindow.so libsync.so
libjnigraphics.so libmediandk.so libOpenSLES.so libcamera2ndk.so libc++.so
libc++_shared.so ld-android.so libneuralnetworks.so""".split())

def load_packages():
    pkgs = {}
    for _, base, pk, _ in REPOS:
        raw = urllib.request.urlopen(f"{base}/{pk}").read().decode("utf-8", "replace")
        for para in raw.split("\n\n"):
            m = re.search(r"^Package: (\S+)", para, re.M)
            f = re.search(r"^Filename: (\S+)", para, re.M)
            if m and f and m.group(1) not in pkgs:
                pkgs[m.group(1)] = (base, f.group(1))
    return pkgs

def load_contents():
    provides, LIBDIR = {}, "com.termux/files/usr/lib/"
    for _, base, _, cf in REPOS:
        data = gzip.decompress(urllib.request.urlopen(f"{base}/{cf}").read()).decode("utf-8", "replace")
        for line in data.splitlines():
            parts = line.split()
            if len(parts) < 2:
                continue
            path, so = parts[0], os.path.basename(parts[0])
            # Uniquement le dossier lib DIRECT du préfixe : pas une copie
            # nichée (usr/lib/aws-cli/libcrypto.so.3 → awscli, le piège de 135 Mo).
            if path.endswith(LIBDIR + so) and (so.endswith(".so") or ".so." in so):
                provides.setdefault(so, parts[-1].split("/")[-1])
    return provides

def copytree(src, dst):
    if not os.path.isdir(src):
        return
    os.makedirs(dst, exist_ok=True)
    for name in os.listdir(src):
        s, d = os.path.join(src, name), os.path.join(dst, name)
        if os.path.isdir(s) and not os.path.islink(s):
            copytree(s, d)
        elif os.path.isfile(s):  # copy2 déréférence les symlinks → fichiers réguliers, ZIP-sûr
            shutil.copy2(s, d)

def extract(pkg, base, filename):
    deb = f"{W}/dl/{pkg}.deb"
    urllib.request.urlretrieve(f"{base}/{filename}", deb)
    d = f"{W}/x/{pkg}"; os.makedirs(d, exist_ok=True)
    subprocess.run(["ar", "x", deb], cwd=d, check=True)
    tar = next(t for t in os.listdir(d) if t.startswith("data.tar"))
    subprocess.run(["tar", "xf", os.path.join(d, tar)], cwd=d, check=True)
    usr = os.path.join(d, "data/data/com.termux/files/usr")
    if not os.path.isdir(usr):
        return
    copytree(os.path.join(usr, "lib"), ROOT + "/lib")
    copytree(os.path.join(usr, "bin"), ROOT + "/bin")
    copytree(os.path.join(usr, "share/X11"), ROOT + "/share/X11")
    # LES KEYMAPS NE SONT PAS OÙ XVFB LES CHERCHE.
    #
    # Termux installe xkeyboard-config dans share/xkeyboard-config-2/, alors
    # que Xvfb lit share/X11/xkb (chemin patché). Copier seulement share/X11
    # ramassait les données de libX11 et ratait TOUTES les keymaps : Xvfb
    # mourait sur « XKB: Failed to compile keymap » puis « Failed to activate
    # virtual core keyboard » — fatal (constat A35 du 2026-09-12). On les remet
    # donc à l'endroit attendu.
    copytree(os.path.join(usr, "share/xkeyboard-config-2"), ROOT + "/share/X11/xkb")
    doc = os.path.join(usr, "share/doc", pkg, "copyright")
    if os.path.isfile(doc):
        shutil.copy2(doc, f"{W}/lic/{pkg}.copyright")

def elves(root):
    for dirpath, _, files in os.walk(root):
        for f in files:
            p = os.path.join(dirpath, f)
            if os.path.islink(p):
                continue
            with open(p, "rb") as fh:
                if fh.read(4) == b"\x7fELF":
                    yield p

def needed(path):
    out = subprocess.run([READELF, "-d", path], capture_output=True, text=True).stdout
    return re.findall(r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]", out)

def have_sonames():
    return {os.path.basename(p) for p in elves(ROOT + "/lib")}

def relocate_prefix(path):
    data = bytearray(open(path, "rb").read())
    i, changed = 0, 0
    while True:
        j = data.find(OLD_PREFIX, i)
        if j < 0:
            break
        k = data.find(b"\x00", j)               # fin de la chaîne
        if k < 0:
            break
        full = bytes(data[j:k])
        repl = full.replace(OLD_PREFIX, NEW_PREFIX)  # TOUTES les occurrences de la chaîne
        repl += b"\x00" * (len(full) - len(repl))    # pad à la même longueur (NEW plus court)
        data[j:k] = repl
        changed += full.count(OLD_PREFIX)
        i = k
    if changed:
        open(path, "wb").write(data)
    return changed

def point_shell_to_system(path):
    """Fait pointer le shell d'Xvfb vers celui d'Android.

    Xvfb compile son keymap en forkant un shell (`fork` + `execl` importés, et
    un gabarit « "%s%sxkbcomp" -w … » dont le chemin entre guillemets trahit le
    passage par `sh -c`). Le shell qu'il connaît est celui de son préfixe —
    inexistant chez nous, et de toute façon dans le dossier de données, donc
    interdit à l'exécution par Android (W^X).

    Android a le sien à /system/bin/sh, autorisé à l'exécution et toujours
    présent. 14 octets contre 37 : la réécriture tient en place.

    À lancer APRÈS relocate_prefix, qui a déjà converti com.termux → le nôtre.
    """
    data = bytearray(open(path, "rb").read())
    changed = 0
    for suffix in (b"/bin/sh", b"/bin/bash"):
        old = NEW_PREFIX + suffix
        i = 0
        while True:
            j = data.find(old, i)
            if j < 0:
                break
            k = data.find(b"\x00", j)
            if k < 0:
                break
            repl = b"/system/bin/sh"
            data[j:k] = repl + b"\x00" * (k - j - len(repl))
            changed += 1
            i = k
    if changed:
        open(path, "wb").write(data)
    return changed


def point_socket_to_standard_name(path):
    """Ramène le nom de la socket X au nom STANDARD — celui que tout client essaie.

    Le préfixe relocalisé donne « <prefix>/tmp/.X11-unix/X0 », et xtrans
    dérive de la même chaîne le nom de la socket ABSTRAITE
    (« @<prefix>/tmp/.X11-unix/X0 », lu dans /proc/net/unix de l'émulateur le
    2026-09-12). Or libxcb — donc SDL2, Qt, libX11, tout client stock — cherche
    « @/tmp/.X11-unix/X0 » puis « /tmp/.X11-unix/X0 », et DISPLAY n'accepte
    aucun chemin (libxcb 1.15 : six formes essayées, six refus).

    L'abstraite n'a besoin d'aucun dossier : avec ces littéraux ramenés à
    « /tmp/.X11-unix » et « -nolisten unix » (la socket-FICHIER) au lancement,
    Xvfb écoute sur le nom standard sans toucher au système de fichiers —
    mesuré sur le Xvfb Debian : abstraite seule, /tmp/.X11-unix jamais créé,
    xdpyinfo se connecte. Le verrou (« <prefix>/tmp », littéral distinct)
    reste dans le dossier de l'app, seul endroit où Xvfb peut écrire.

    Plus court : réécriture en place. À lancer APRÈS relocate_prefix.
    """
    data = bytearray(open(path, "rb").read())
    old = NEW_PREFIX + b"/tmp/.X11-unix"
    i, changed = 0, 0
    while True:
        j = data.find(old, i)
        if j < 0:
            break
        k = data.find(b"\x00", j)
        if k < 0:
            break
        full = bytes(data[j:k])
        repl = full[len(NEW_PREFIX):]                # « /tmp/.X11-unix… »
        data[j:k] = repl + b"\x00" * (len(full) - len(repl))
        changed += 1
        i = k
    if changed:
        open(path, "wb").write(data)
    return changed


def main():
    shutil.rmtree(W, ignore_errors=True)
    for sub in ("dl", "x", "lic"):
        os.makedirs(f"{W}/{sub}", exist_ok=True)
    os.makedirs(ROOT, exist_ok=True)
    print("… index"); pkgs = load_packages(); prov = load_contents()
    print(f"  {len(pkgs)} paquets, {len(prov)} bibliothèques cartographiées")

    pulled, unresolved = set(), set()
    def pull(pkg):
        if pkg in pulled:
            return
        if pkg not in pkgs:
            unresolved.add(pkg); return
        base, fn = pkgs[pkg]; extract(pkg, base, fn); pulled.add(pkg)
        print(f"  + {pkg}")

    print("… fermeture transitive"); [pull(s) for s in SEED]
    for _ in range(30):
        have = have_sonames()
        missing = set()
        for p in list(elves(ROOT + "/bin")) + list(elves(ROOT + "/lib")):
            for n in needed(p):
                if n not in SYS and n not in have:
                    missing.add(n)
        if not missing:
            break
        progressed = False
        for so in sorted(missing):
            pkg = prov.get(so)
            if pkg and pkg not in pulled:
                pull(pkg); progressed = True
            elif not pkg:
                unresolved.add(so)
        if not progressed:
            break

    # On ne garde que les binaires utiles au runtime. xkbcomp reste par
    # prudence, mais Xvfb ne l'exécute PAS : il compile le keymap en interne
    # (XkbCompileKeymapFromString, d'où libxkbfile dans la fermeture) — vérifié
    # aux strings, aucun littéral « xkbcomp » dans le binaire. Le verrou W^X
    # sur l'execve ne concerne donc pas xkb.
    keep_bin = {"Xvfb", "xkbcomp"}
    for b in list(os.listdir(ROOT + "/bin")):
        if b not in keep_bin:
            os.remove(ROOT + "/bin/" + b)

    # Les .pc de pkg-config ne servent qu'à COMPILER contre ces bibliothèques :
    # rien à faire dans un bundle de runtime téléchargé par un téléphone.
    shutil.rmtree(ROOT + "/lib/pkgconfig", ignore_errors=True)

    print("… relocalisation du préfixe dans les ELF")
    total = sum(relocate_prefix(p) for p in elves(ROOT))
    print(f"  {total} chaînes réécrites vers {NEW_PREFIX.decode()}")

    print("… le shell d'Xvfb pointe vers celui d'Android")
    shells = sum(point_shell_to_system(p) for p in elves(ROOT))
    print(f"  {shells} chemins de shell réécrits vers /system/bin/sh")

    print("… la socket X reprend son nom standard (abstraite @/tmp/.X11-unix/X0)")
    sockets = sum(point_socket_to_standard_name(p) for p in elves(ROOT))
    print(f"  {sockets} littéraux ramenés à /tmp/.X11-unix")

    # verdict + ZIP
    have = sorted(have_sonames())
    foreign = [os.path.basename(p) for p in elves(ROOT + "/lib")
               if MACHINE not in subprocess.run([READELF, "-h", p], capture_output=True, text=True).stdout]
    still = set()
    for p in list(elves(ROOT + "/bin")) + list(elves(ROOT + "/lib")):
        for n in needed(p):
            if n not in SYS and n not in set(have):
                still.add(n)
    size = sum(os.path.getsize(os.path.join(dp, f)) for dp, _, fs in os.walk(ROOT) for f in fs)

    import zipfile
    zpath = W + f"/xvfb-bundle-{ARCH}.zip"
    with zipfile.ZipFile(zpath, "w", zipfile.ZIP_DEFLATED) as z:
        for base in (ROOT, W + "/lic"):
            for dp, _, fs in os.walk(base):
                for f in fs:
                    full = os.path.join(dp, f)
                    arc = "lic/" + f if base.endswith("lic") else os.path.relpath(full, ROOT)
                    z.write(full, arc)

    print(f"\n===== BUNDLE {ARCH} =====")
    print(f"paquets ({len(pulled)}) : {', '.join(sorted(pulled))}")
    print(f"bibliothèques {len(have)}, taille {size/1024/1024:.1f} Mo")
    print(f"binaires : {sorted(os.listdir(ROOT + '/bin'))}")
    print(f"étrangères à {MACHINE} : {foreign or 'aucune'}")
    print(f"NEEDED non résolus : {sorted(still) or 'AUCUN — clôture atteinte'}")
    print(f"non résolus (paquets) : {sorted(unresolved) or 'aucun'}")
    print(f"ZIP : {zpath} ({os.path.getsize(zpath)/1024/1024:.1f} Mo)")

main()
