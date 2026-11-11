#!/usr/bin/env python3
# compare-so.py <paquet> <construit> : le témoin de build-radv.sh.
#
# Compare deux .so ELF64 section par section (adresse, taille, contenu) et sort
# en erreur dès qu'une section diffère, sauf ce que le paquet de Debian a de plus
# sans que le code change :
#   - .note.gnu.build-id : il dépend aussi des informations de débogage, donc des
#     en-têtes des bibliothèques (expat, zlib, linux-libc-dev), que trixie met à
#     jour après le passage du buildd ;
#   - .gnu_debuglink et .gnu_debugaltlink, le lien vers le paquet -dbgsym, que
#     dh_strip et dh_dwz ajoutent ; et .shstrtab, la table qui les nomme.
# Tout le reste (code, données, symboles, .dynamic) doit être identique à l'octet.
#
# Il lit les en-têtes ELF lui-même : aucun module externe.
import struct
import sys

MAY_DIFFER = {".note.gnu.build-id", ".shstrtab"}
ONLY_IN_PACKAGE = {".gnu_debuglink", ".gnu_debugaltlink"}


def read_sections(data):
    assert data[:4] == b"\x7fELF" and data[4] == 2, "ELF64 attendu"
    section_offset, = struct.unpack_from("<Q", data, 0x28)
    entry_size, entry_count, names_index = struct.unpack_from("<HHH", data, 0x3A)
    headers = [struct.unpack_from("<IIQQQQIIQQ", data, section_offset + index * entry_size) for index in range(entry_count)]
    names_offset = headers[names_index][4]
    sections = {}

    for (name, kind, flags, address, offset, size, link, info, align, entsize) in headers:
        label = data[names_offset + name:data.index(b"\0", names_offset + name)].decode()
        content = b"" if kind == 8 else data[offset:offset + size]  # SHT_NOBITS : rien dans le fichier
        sections[label] = (address, size, content)

    sections.pop("", None)

    return sections


package_path, built_path = sys.argv[1:3]
package = read_sections(open(package_path, "rb").read())
built = read_sections(open(built_path, "rb").read())
problems = []

for label in sorted(set(built) - set(package)):
    problems.append(f"{label} : seulement dans {built_path}")

for label in sorted(set(package) - set(built) - ONLY_IN_PACKAGE):
    problems.append(f"{label} : seulement dans le paquet")

for label in sorted(set(package) & set(built) - MAY_DIFFER):
    (package_address, package_size, package_content), (built_address, built_size, built_content) = package[label], built[label]

    if package_address != built_address or package_size != built_size or package_content != built_content:
        differing = sum(1 for left, right in zip(package_content, built_content) if left != right) + abs(len(package_content) - len(built_content))
        problems.append(f"{label} : adresse {package_address:#x}/{built_address:#x}, taille {package_size}/{built_size}, {differing} octets différents")

print(f"{len(set(package) & set(built) - MAY_DIFFER)} sections comparées, {len(problems)} écarts")

for problem in problems:
    print("  " + problem)

sys.exit(1 if problems else 0)
