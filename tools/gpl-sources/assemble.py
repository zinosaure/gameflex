#!/usr/bin/env python3
"""Assemble src/ next to cores/ in a GameFlex library: for every emulator file
we distribute, where its source is (chantier 34, P1, #488).

    tools/gpl-sources/assemble.py <library> [--out DIR]

We publish modified builds of GPL emulators (shadPS4, KytyPS5, Eden, with Box64
on the phone) and repackaged publisher binaries on the mirror, which serves the
release library. The GPL wants their source next to them: for each file of
cores/ (libretro cores aside), src/<console>/<file>/ gets a SOURCE.md naming the
exact upstream revision, and the patches and scripts of this repository as they
were at the commit that produced the file (manifest.tsv), not as they are today.

A file of cores/ missing from manifest.tsv is an error, and nothing is written:
a delivery without its source line is caught here, not by a reader.

Without --out, src/ of the library is updated in place (rsync --delete), so the
mirror's bind mount keeps its directory.
"""
import argparse
import io
import os
import pathlib
import subprocess
import sys
import tarfile
import tempfile
import zipfile

HERE = pathlib.Path(__file__).resolve().parent
REPOSITORY = HERE.parent.parent
LIBRETRO_SUFFIXES = ("_libretro.so", "_libretro_android.so")
INDEX_TITLE = "# Sources of the emulators on this mirror"
WRITTEN_OFFER = ("**Written offer.** For at least three years after we last distribute a file of this mirror's "
                 "`cores/`, we will give anyone who asks a complete, machine-readable copy of the corresponding "
                 "source code of any GPL, LGPL or AGPL software it contains (for our builds: the upstream source "
                 "at the revision named on its page, with our patches), for no more than our cost of physically "
                 "performing this distribution. Ask on the [issues page](https://github.com/zinosaure/gameflex/issues), "
                 "naming the file.")

KINDS = {
    "ours": "A GameFlex build: the upstream source at the revision below, with our patches, "
            "built by the scripts in this folder.",
    "repackaged": "The publisher's binary, unmodified, repackaged for GameFlex's phone app with "
                  "the libraries it expects from a desktop Linux.",
    "upstream": "The publisher's file, unmodified, as published.",
}

# What a phone package carries besides the emulator, by folder — said only for the
# folders the zip really has.
LIBRARY_FOLDERS = {
    "usr/lib/": "under `usr/lib`, unmodified Debian binaries from the phone runtime's Debian image "
                "(named in `assemble-glibc.sh`)",
    "usr/x86_64/lib/": "under `usr/x86_64/lib`, unmodified Debian binaries from the Debian trixie the "
                       "x86_64 emulator was built on",
    "usr/x86_64/box64lib/": "under `usr/x86_64/box64lib`, Box64's own `x64lib`, part of its source tree",
}


def read_manifest():
    entries = {}

    for line in (HERE / "manifest.tsv").read_text(encoding="utf-8").splitlines():
        if not line.strip() or line.startswith("#"):
            continue

        file_name, kind, upstreams, repository_paths, commands = line.split("\t")

        if kind not in KINDS:
            sys.exit(f"manifest.tsv: {file_name}: unknown kind {kind!r}")

        entries[file_name] = {
            "kind": kind,
            "upstreams": [item.rsplit("@", 1) for item in upstreams.split()],
            "repository": [] if repository_paths == "-" else [item.rsplit("@", 1) for item in repository_paths.split()],
            "commands": [] if commands == "-" else [command.strip() for command in commands.split(";")],
        }

    return entries


def distributed_files(library):
    cores = library / "cores"

    return sorted(
        str(path.relative_to(cores))
        for path in cores.rglob("*")
        if path.is_file() and not path.name.endswith(LIBRETRO_SUFFIXES) and not path.name.startswith(".")
    )


def export_from_repository(commit, path, destination):
    # git archive gives the path as it was at that commit, whatever the working tree holds.
    archive = subprocess.run(["git", "-C", str(REPOSITORY), "archive", "--format=tar", commit, "--", path],
                             check=True, capture_output=True).stdout

    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        tar.extractall(destination, filter="data")


def browse_link(url, revision):
    # GitHub shows a commit or a tag at /tree/ ; Forgejo (git.eden-emu.dev) tells them apart.
    if "github.com" in url:
        return f"{url}/tree/{revision}"

    is_commit = len(revision) >= 7 and all(character in "0123456789abcdef" for character in revision)

    return f"{url}/src/{'commit' if is_commit else 'tag'}/{revision}"


def library_note(package):
    names = zipfile.ZipFile(package).namelist()
    present = [text for prefix, text in LIBRARY_FOLDERS.items() if any(name.startswith(prefix) and name != prefix for name in names)]

    if not present:
        return None

    return ("Besides the emulator, it carries " + "; ".join(present) + ". Debian's source packages: "
            "<https://sources.debian.org>.")


def write_source_page(file_name, entry, folder, package):
    lines = [f"# {pathlib.PurePath(file_name).name}", "", KINDS[entry["kind"]], "", "## Upstream", ""]

    for url, revision in entry["upstreams"]:
        lines.append(f"- {url} at `{revision}` — [browse]({browse_link(url, revision)})")

    if entry["kind"] == "ours":
        patches = sorted(path.relative_to(folder) for path in folder.rglob("*.patch"))
        lines += ["", "## Our changes", "",
                  "Each patch applies to the upstream revision above. The build command below says which ones it "
                  "applies, in the order of their number. Our patches are distributed under the license of the "
                  "project they modify.", ""]
        lines += [f"- [`{patch}`]({patch})" for patch in patches]

    if entry["commands"]:
        lines += ["", "## How it was built", "", "From the root of the GameFlex repository, with the files of this folder:", "", "```"]
        lines += entry["commands"]
        lines += ["```"]

    if entry["repository"]:
        lines += ["", "## Files in this folder", "",
                  "From the GameFlex repository, as they were when this file was produced:", ""]
        lines += [f"- `{path}` (commit `{commit}`)" for path, commit in entry["repository"]]

    note = library_note(package) if file_name.endswith(".zip") and entry["kind"] != "upstream" else None

    if note:
        lines += ["", "## Libraries", "", note]

    (folder / "SOURCE.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_index(files, entries, root):
    lines = [INDEX_TITLE, "",
             "One folder per emulator file of `cores/`: where its source is, and for our builds, our patches "
             "and build scripts.", "",
             "| File | What it is | Upstream |", "|---|---|---|"]

    for file_name in files:
        entry = entries[file_name]
        upstreams = ", ".join(f"[{url.rsplit('/', 1)[-1]} `{revision[:12]}`]({browse_link(url, revision)})"
                              for url, revision in entry["upstreams"])
        lines.append(f"| [`{file_name}`]({file_name}/SOURCE.md) | {entry['kind']} | {upstreams} |")

    # The pages point to the upstream source; the GPL also wants it given on request
    # (decided 2026-10-28) — the same offer as THIRD-PARTY-NOTICES.md.
    lines += ["", WRITTEN_OFFER]

    (root / "README.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description="Assemble src/ next to cores/ in a GameFlex library.")
    parser.add_argument("library", type=pathlib.Path)
    parser.add_argument("--out", type=pathlib.Path, help="write there instead of <library>/src")
    arguments = parser.parse_args()
    entries = read_manifest()
    files = distributed_files(arguments.library)
    unknown = [file_name for file_name in files if file_name not in entries]

    if unknown:
        sys.exit("No source line in tools/gpl-sources/manifest.tsv for:\n  " + "\n  ".join(unknown))

    target = arguments.out or arguments.library / "src"
    index = target / "README.md"

    # The sync deletes what it did not write: only into an empty folder, or one it wrote before.
    if target.exists() and any(target.iterdir()) and not (index.is_file() and index.read_text(encoding="utf-8").startswith(INDEX_TITLE)):
        sys.exit(f"{target} is not empty and was not written by this script: nothing done")

    with tempfile.TemporaryDirectory() as staging:
        root = pathlib.Path(staging)

        for file_name in files:
            folder = root / file_name
            folder.mkdir(parents=True)

            for path, commit in entries[file_name]["repository"]:
                export_from_repository(commit, path, folder)

            write_source_page(file_name, entries[file_name], folder, arguments.library / "cores" / file_name)

        write_index(files, entries, root)
        # rsync carries the staging folder's mode (700) onto src/: the mirror's nginx
        # runs as another user and must read it.
        root.chmod(0o755)
        target.mkdir(exist_ok=True)
        subprocess.run(["rsync", "-rcp", "--delete", f"{root}/", f"{target}/"], check=True)

    print(f"{len(files)} files, their sources in {target}")


if __name__ == "__main__":
    main()
