import { useEffect, useState } from "react";
import { gameMetadataPath, gameQuery, readToken } from "./api.js";

// LES INFORMATIONS D'UN JEU — `GET /api/v1/retrogaming/{console}/roms/{ref}/metadata`
// (chantier 49) : `releaseDate`, `publishers`, `developers`, `genres`, et
// `sources` (quel service a fourni chaque champ). Le rétro les tient de
// libretro-database, sans clé ; les jeux PS, d'IGDB une fois ses clés saisies.
//
// La route demande le jeton : `fetch`, comme les images (libs/images.js). Un
// 404 ou une erreur — le jeu n'est pas reconnu, ou la route n'existe pas encore —
// veut dire « rien de connu » : la fiche s'affiche sans. Retenu pour la page ;
// trois requêtes à la fois au plus (la fiche courante et ses deux voisines). Le
// statut d'une lecture refusée se garde (gameMetadataStatus) : la page d'un jeu
// fédéré dit pourquoi son serveur ne prête pas sa fiche (chantier 57).

const PARALLEL = 3;
const entries = new Map();
const queue = [];
let running = 0;

const pump = () => {
    while (running < PARALLEL && queue.length) {
        const entry = queue.shift();
        running += 1;
        const token = readToken();
        fetch(entry.path, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
            .then(async (response) => {
                if (!response.ok) {
                    entry.data = null;
                    entry.status = response.status;

                    return;
                }
                const body = await response.json().catch(() => null);
                // L'enveloppe du kernel (`{success, data}`), ou l'objet nu.
                const data = body && typeof body === "object" && "data" in body ? body.data : body;
                entry.data = data && typeof data === "object" ? data : null;
            })
            .catch(() => {
                entry.data = null;
                entry.status = 0;
            })
            .finally(() => {
                running -= 1;
                entry.done = true;
                entry.listeners.forEach((listener) => listener());
                pump();
            });
    }
};

// La clé d'une fiche : le jeu ET sa source (un même jeu sur la bibliothèque et sur un disque a deux fiches).
const keyOf = (consoleId, reference, game) => `${consoleId}|${reference}|${gameQuery(game)}`;

const entryFor = (consoleId, reference, game) => {
    const key = keyOf(consoleId, reference, game);

    if (!entries.has(key)) {
        const entry = {
            path: gameMetadataPath(consoleId, reference, game),
            done: false, data: null, listeners: new Set(),
        };
        entries.set(key, entry);
        queue.push(entry);
        pump();
    }

    return entries.get(key);
};

// La fiche d'un jeu remplacée (la réponse d'une correction, qui la rend fusionnée), ou oubliée (une
// source retirée, un autre jeu désigné) : ceux qui la montrent la relisent.
export function setGameMetadata(consoleId, reference, game, data) {
    const entry = entries.get(keyOf(consoleId, reference, game));

    if (entry) {
        entry.data = data && typeof data === "object" ? data : null;
        entry.done = true;
        entry.listeners.forEach((listener) => listener());
    }
}

export function forgetGameMetadata(consoleId, reference, game) {
    const key = keyOf(consoleId, reference, game);
    const entry = entries.get(key);
    entries.delete(key);
    entry?.listeners.forEach((listener) => listener());
}

// LE STATUT D'UNE FICHE QUI NE S'EST PAS LUE (0 : rien n'a répondu), null sinon — à lire après useGameMetadata, qui
// redessine quand la lecture finit : 403, 404 ou 502 pour un jeu fédéré dont le serveur ne la prête pas.
export function gameMetadataStatus(consoleId, reference, game = {}) {
    const entry = entries.get(keyOf(consoleId, reference, game));

    return entry?.done && entry.data === null && entry.status !== undefined ? entry.status : null;
}

// `undefined` tant qu'on ne sait pas, `null` sans information, sinon l'objet.
// `game` — la source du jeu (libs/api.js, romSource) : `disk` ou `server`.
export function useGameMetadata(consoleId, reference, game = {}) {
    const [, redraw] = useState(0);
    const entry = consoleId && reference ? entryFor(consoleId, reference, game) : null;

    useEffect(() => {
        if (!entry || entry.done) {
            return undefined;
        }
        const listener = () => redraw((value) => value + 1);
        entry.listeners.add(listener);

        return () => entry.listeners.delete(listener);
    }, [entry]);

    if (!entry) {
        return null;
    }

    return entry.done ? entry.data : undefined;
}

// Ce que la fiche en montre : le premier éditeur, deux genres au plus, l'année ; le nombre de joueurs, 0 quand aucune
// source ne le sait (le kernel ne le rend alors pas : libretro-database seule le donne, chantier 53, P12).
export const summaryOf = (metadata) => ({
    publisher: metadata?.publishers?.[0] || "",
    developer: (metadata?.developers || []).join(", "),
    genres: (metadata?.genres || []).slice(0, 2),
    year: String(metadata?.releaseDate || "").slice(0, 4),
    players: Number(metadata?.players) > 0 ? Number(metadata.players) : 0,
});

// LES SOURCES À CITER, avec leur lien : libretro-database est sous licence
// CC BY-SA, la citer est une obligation. Une source inconnue s'affiche par son nom.
const SOURCE_LINKS = {
    "libretro-database": { label: "libretro-database", href: "https://github.com/libretro/libretro-database" },
    igdb: { label: "IGDB", href: "https://www.igdb.com" },
    steamgriddb: { label: "SteamGridDB", href: "https://www.steamgriddb.com" },
};

export const sourcesOf = (metadata) => {
    const names = [...new Set(Object.values(metadata?.sources || {}).flat().filter(Boolean).map((name) => String(name)))];

    return names.map((name) => SOURCE_LINKS[name.toLowerCase()] || { label: name, href: "" });
};
