import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage from "./SettingsPage.jsx";
import { Listing, useListSearch } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";
import { fact } from "../libs/format.js";

// LES CRÉDITS (le porteur, 2026-11-12 : « une page dédiée » ; 2026-11-14 : « retire console icons. Et rajoute les
// crédits pour les créateurs des émulateurs. ») — une page des réglages (/settings/credits ; 32-credits des modèles) :
// les émulateurs que GameFlex fait tourner, leurs auteurs, leur licence et l'adresse de leur code ; puis les
// informations des jeux. Les icônes des consoles sont parties de l'interface (le porteur, 2026-11-12), leur crédit aussi.
//
// RIEN N'Y EST INVENTÉ : chaque ligne dit ce que sa source dit, et seulement ça (relevé le 2026-11-14).
//   - LES CONSOLES RÉCENTES : l'émulateur de chacune et l'adresse de son code, déclarés par
//     src/microservices/runtimes/streamex/consoles/emulators.sh et tools/gpl-sources/manifest.tsv ; l'auteur est le
//     compte qui le publie, la licence celle que son dépôt déclare (son fichier LICENSE, ou ce qu'en dit l'API de
//     GitHub) — sauf Citron, dont le dépôt ne répond plus : sa licence et son auteur sont ceux que dit l'émulateur lui-même
//     (sa fenêtre « À propos » : « licensed under GPLv3.0+ » ; son fichier .desktop : « citron Emulator Project »), son
//     adresse le site qu'il nomme. KytyPS5 « is based on a heavily modified version of Kyty » (son README), dont
//     l'auteur d'origine, InoriRus, se crédite à part (LICENSES/Kyty-MIT.txt : « Copyright (c) 2021 InoriRus ») ; de
//     même Xenia, dont Xenia Canary est la fourche (son LICENSE : « Copyright (c) 2015, Ben Vanik »). Box64 fait tourner
//     la PS4 et la PS5 sur le téléphone (tools/box64). La fourche de KytyPS5 de GameFlex n'est pas un crédit : c'est
//     notre code. Remote Play est notre code aussi (pas d'émulateur).
//   - LES CŒURS LIBRETRO : ceux du registre des consoles (src/microservices/shared/consoles/registry.go, consoles
//     actives), leur nom, leurs auteurs et leur licence tels que libretro les déclare dans son registre des cœurs
//     (github.com/libretro/libretro-core-info, au commit eac3a278 du 2026-11-02 : `corename`, `authors`, `license`) —
//     chaque ligne mène à son fichier.
//   - LES INFORMATIONS DES JEUX : libretro-database, CC BY-SA 4.0 (la ligne Sources de chaque fiche nomme déjà la base
//     pour ce jeu-là).
// Une console ajoutée au registre ajoute sa ligne ici, relevée à sa source.

const RECENT = [
    { name: "RPCS3", consoles: "PlayStation 3", authors: "RPCS3", license: "GPL-2.0", href: "https://github.com/RPCS3/rpcs3" },
    { name: "shadPS4", consoles: "PlayStation 4", authors: "shadps4-emu", license: "GPL-2.0", href: "https://github.com/shadps4-emu/shadPS4" },
    { name: "KytyPS5", consoles: "PlayStation 5", authors: "KytyPS5", license: "GPL-2.0-only", href: "https://github.com/KytyPS5/KytyPS5" },
    { name: "Kyty", origin: "KytyPS5", authors: "InoriRus", license: "MIT", href: "https://github.com/InoriRus/Kyty" },
    { name: "Eden", consoles: "Nintendo Switch", authors: "eden-emu", license: "GPL-3.0", href: "https://git.eden-emu.dev/eden-emu/eden" },
    { name: "Citron", consoles: "Nintendo Switch", authors: "citron Emulator Project", license: "GPL-3.0-or-later", href: "https://citron-neo.org/" },
    { name: "Lichii", consoles: "Nintendo Switch 2", authors: "zortos293", license: "GPL-3.0", href: "https://github.com/zortos293/Lichii" },
    { name: "Cemu", consoles: "Wii U", authors: "cemu-project", license: "MPL-2.0", href: "https://github.com/cemu-project/Cemu" },
    { name: "xemu", consoles: "Xbox", authors: "xemu-project", license: "GPL-2.0", href: "https://github.com/xemu-project/xemu" },
    { name: "Xenia Canary", consoles: "Xbox 360", authors: "xenia-canary", license: "BSD-3-Clause", href: "https://github.com/xenia-canary/xenia-canary" },
    { name: "Xenia", origin: "Xenia Canary", authors: "Ben Vanik", license: "BSD-3-Clause", href: "https://github.com/xenia-project/xenia" },
    { name: "Box64", phone: true, authors: "ptitSeb", license: "MIT", href: "https://github.com/ptitSeb/box64" },
];

const LIBRETRO = [
    { core: "cap32", name: "Caprice32", authors: "Ulrich Doewich, dantoine", license: "GPLv2" },
    { core: "desmume", name: "DeSmuME", authors: "YopYop156, Zeromus", license: "GPLv2" },
    { core: "dolphin", name: "Dolphin", authors: "Team Dolphin", license: "GPLv2+" },
    { core: "fbalpha2012_cps1", name: "FB Alpha 2012 CPS-1", authors: "Team FB Alpha", license: "Non-commercial" },
    { core: "fbalpha2012_cps2", name: "FB Alpha 2012 CPS-2", authors: "Team FB Alpha", license: "Non-commercial" },
    { core: "fbalpha2012_neogeo", name: "FB Alpha 2012 Neo Geo", authors: "Team FB Alpha", license: "Non-commercial" },
    { core: "fceumm", name: "FCEUmm", authors: "FCEU Team, CaH4e3", license: "GPLv2" },
    { core: "flycast", name: "Flycast", authors: "flyinghead", license: "GPLv2" },
    { core: "fuse", name: "Fuse", authors: "Team Fuse", license: "GPLv3" },
    { core: "gambatte", name: "Gambatte", authors: "Sinamas", license: "GPLv2" },
    { core: "gearcoleco", name: "Gearcoleco", authors: "Nacho Sanchez", license: "GPLv3" },
    { core: "genesis_plus_gx", name: "Genesis Plus GX", authors: "Charles McDonald, Eke-Eke", license: "Non-commercial" },
    { core: "handy", name: "Handy", authors: "K. Wilkins", license: "Zlib" },
    { core: "kronos", name: "Kronos", authors: "Guillaume Duhammel, Theo Berkau, Anders Montonen, devmiyax, FCare, barbudreadmon", license: "GPLv2" },
    { core: "mednafen_ngp", name: "Beetle NeoPop", authors: "neopop_uk, Mednafen Team", license: "GPLv2" },
    { core: "mednafen_pce_fast", name: "Beetle PCE Fast", authors: "Mednafen Team", license: "GPLv2" },
    { core: "mednafen_pcfx", name: "Beetle PC-FX", authors: "Mednafen Team", license: "GPLv2" },
    { core: "mednafen_saturn", name: "Beetle Saturn", authors: "Mednafen Team", license: "GPLv2" },
    { core: "mednafen_vb", name: "Beetle VB", authors: "Mednafen Team", license: "GPLv2" },
    { core: "mednafen_wswan", name: "Beetle WonderSwan", authors: "Dox, Mednafen Team", license: "GPLv2" },
    { core: "mgba", name: "mGBA", authors: "endrift", license: "MPLv2.0" },
    { core: "nestopia", name: "Nestopia", authors: "Martin Freij, R. Danbrook, Rupert Carmichael", license: "GPLv2" },
    { core: "parallel_n64", name: "ParaLLEl N64", authors: "Hacktarux, Mupen64Plus Team, TinyTiger, Libretro", license: "GPLv2" },
    { core: "pcsx2", name: "LRPS2", authors: "PCSX2 Team", license: "GPLv3" },
    { core: "pcsx_rearmed", name: "PCSX-ReARMed", authors: "PCSX Team, notaz, Exophase", license: "GPLv2" },
    { core: "picodrive", name: "PicoDrive", authors: "notaz, fdave, irixxxx", license: "MAME" },
    { core: "ppsspp", name: "PPSSPP", authors: "Henrik Hrydgard", license: "GPLv2" },
    { core: "prboom", name: "PrBoom", authors: "Florian Schulze", license: "GPLv2" },
    { core: "prosystem", name: "ProSystem", authors: "Greg Stanton, Brian Berlin, Leonis, Greg DeMent", license: "GPLv2" },
    { core: "snes9x", name: "Snes9x", authors: "Snes9x Team", license: "Non-commercial" },
    { core: "stella2014", name: "Stella 2014", authors: "Stephen Anthony, Bradford Mott, Eckhard Stolberg, Brian Watson", license: "GPLv2" },
    { core: "vecx", name: "vecx", authors: "Valavan Manohararajah, John Hawthorn, Nikita Zimin, Demeth", license: "GPLv3" },
    { core: "vice_x64sc", name: "VICE x64sc", authors: "VICE Team", license: "GPLv2" },
    { core: "virtualjaguar", name: "Virtual Jaguar", authors: "Joseph Mattiello, David Raingeard, Shamus", license: "GPLv3" },
    { core: "yabause", name: "Yabause", authors: "Guillaume Duhammel, Theo Berkau, Anders Montonen", license: "GPLv2" },
];

const CORE_INFO = "https://github.com/libretro/libretro-core-info/blob/master";
const GAMES = { work: "libretro-database", license: "CC BY-SA 4.0", href: "https://github.com/libretro/libretro-database" };

// Ouvrir l'adresse dans un autre onglet (la ligne est un bouton, que la croix atteint).
const follow = (href) => window.open(href, "_blank", "noopener,noreferrer");

// Une ligne de crédit : le nom, à quoi il sert et ses auteurs, la licence au bord droit, l'adresse qui sort (`opens="link"`).
function CreditRow({ id, name, help, license, href }) {
    const { t } = useTranslation();

    return <NavRow id={id} icon="bi-cpu" label={name} help={help} value={fact(t("facts.licence"), license)} opens="link" onOpen={() => follow(href)} />;
}

export default function Credits() {
    const { t } = useTranslation();
    const cores = useListSearch(LIBRETRO, (entry) => [entry.name, entry.core, entry.authors, entry.license].join(" "));

    return (
        <SettingsPage title={t("settings.items.credits.label")} guides={[["library", "3-get-the-emulators"]]}>
            <Section title={t("credits.recent")} help={t("credits.recentHelp")} sense="optional">
                <Rows>
                    {RECENT.map((entry) => (
                        <CreditRow key={entry.name} id={`cr-${entry.name}`} name={entry.name} license={entry.license} href={entry.href}
                            help={[entry.origin ? t("credits.originOf", { work: entry.origin }) : entry.phone ? t("credits.phone") : fact(t("catalog.filters.consoles"), entry.consoles),
                                fact(t("facts.authors"), entry.authors)]} />
                    ))}
                </Rows>
            </Section>
            <Section title={t("credits.libretro")} help={t("credits.libretroHelp")} sense="optional">
                <Listing items={cores.found} search={cores.search} onSearch={cores.onSearch} placeholder={t("credits.searchCore")} searchKey="cr-search"
                    render={(entry) => (
                    <CreditRow key={entry.core} id={`cr-${entry.core}`} name={entry.name} license={entry.license} href={`${CORE_INFO}/${entry.core}_libretro.info`}
                        help={fact(t("facts.authors"), entry.authors)} />
                )} />
            </Section>
            <Section title={t("credits.games")} sense="optional">
                <Rows>
                    <CreditRow id="cr-games" name={GAMES.work} license={GAMES.license} href={GAMES.href} help={t("credits.gamesHelp")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
