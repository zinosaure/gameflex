// LES JEUX D'UNE BIBLIOTHÈQUE, tels que le porteur les veut (2026-11-12) :
// une fiche par jeu, ses disques dedans ; et l'ordre par série, puis par titre.
//
// Les faits viennent du kernel, dans chaque entrée de la liste
// (`GET /api/v1/retrogaming/{console}/roms`) : `discGroup` (partagé par les
// disques d'un même jeu, présent dès deux disques), `disc` (à partir de 1),
// `discCount`, `discLabel` (« GDI », « NOD », « A »… quand les disques ont un nom
// plutôt qu'un numéro), `series`. Le launcher ne lit RIEN dans les noms de
// fichiers : la webapp et le téléphone partageront les mêmes faits. Sans ces
// champs, chaque entrée est un jeu, et l'ordre est alphabétique.

const compare = (a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });

const discOrder = (a, b) => (a.disc || 0) - (b.disc || 0) || compare(a.title, b.title);

// D'OÙ VIENT UNE ENTRÉE : la bibliothèque (`local`), un disque interne (`disk:<nom>`), un serveur fédéré (`peer:<id>`) —
// la valeur de son interrupteur dans la liste, et ce que `?nosource=` retient (Library.jsx). Les disques d'un jeu se
// groupent DANS leur source : le même jeu à deux disques chez moi et chez un ami fait deux jeux de deux disques, et non
// un seul de quatre (vu le 2026-11-16 au téléphone, `#716`, quand sa liste a reçu `discGroup` pour toutes ses sources).
export const sourceOf = (rom) => (rom.peer != null ? `peer:${rom.peer}` : rom.disk ? `disk:${rom.disk}` : "local");

// Les entrées (déjà filtrées) → les jeux : `{ key, discs, series, seriesSize, sortKey }`.
export function groupGames(roms) {
    const groups = new Map();
    const games = [];

    for (const rom of roms) {
        if (rom.discGroup) {
            const group = `${sourceOf(rom)}|${rom.discGroup}`;

            if (!groups.has(group)) {
                const game = { key: `g:${group}`, discs: [] };
                groups.set(group, game);
                games.push(game);
            }
            groups.get(group).discs.push(rom);
        } else {
            games.push({ key: rom.reference, discs: [rom] });
        }
    }

    const seriesSize = new Map();

    for (const game of games) {
        game.discs.sort(discOrder);
        game.title = game.discs[0].title;
        game.series = game.discs.find((rom) => rom.series)?.series || "";

        if (game.series) {
            seriesSize.set(game.series, (seriesSize.get(game.series) || 0) + 1);
        }
    }

    for (const game of games) {
        game.seriesSize = game.series ? seriesSize.get(game.series) : 0;
        // Une série d'un seul jeu ne regroupe rien : le jeu se range à son propre titre.
        game.sortKey = game.seriesSize >= 2 ? game.series : game.title;
    }
    games.sort((a, b) => compare(a.sortKey, b.sortKey) || compare(a.title, b.title));

    return games;
}

// L'index du jeu qui contient ce disque (par sa référence).
export const gameIndexOf = (games, reference) => games.findIndex((game) => game.discs.some((rom) => rom.reference === reference));

// Le nom d'un disque dans le sélecteur : son étiquette, sinon son numéro.
export const discName = (rom, index) => rom.discLabel || String(rom.disc || index + 1);
