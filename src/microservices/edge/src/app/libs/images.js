import { useEffect, useState } from "react";
import { gameImagePath, readToken } from "./api.js";

// LES IMAGES D'UN JEU — `GET /api/v1/retrogaming/{console}/roms/{ref}/images/{kind}?w=`
// (chantier 49), `kind` ∈ icon (carrée), background (16:9), logo (transparent) — et
// celles d'une console : les fonds de sa bande (`…/consoles/{id}/backgrounds/{nom}?w=`), ses huit emplacements.
// Un seul chargeur, par adresse.
//
// La route demande le jeton : une balise <img> ne peut pas le porter, donc
// `fetch`, puis `URL.createObjectURL` (plan du chantier 51, « Le chargement »).
// L'image reste en cache dans la page tant qu'un écran s'en sert, et se
// libère (`revokeObjectURL`) un moment après le dernier. Un 404 — le jeu n'a
// pas cette image, ou la route n'existe pas encore — se retient aussi : on ne
// redemande pas, et l'affiche générée prend la place.

const LINGER_MS = 20000;
const PARALLEL = 6;
const entries = new Map();
const queue = [];
let running = 0;

const pump = () => {
    while (running < PARALLEL && queue.length) {
        const entry = queue.shift();

        if (entry.users === 0 && entry.state === "queued") {
            entry.state = "idle";
            continue;
        }
        running += 1;
        entry.state = "loading";
        const token = readToken();
        // `no-cache` : toujours revalider (If-None-Match ou If-Modified-Since), comme la route le demande — y compris une
        // copie gardée du temps où elle répondait `max-age=86400`, qu'un navigateur aurait sinon resservie jusqu'à un jour.
        fetch(entry.path, { cache: "no-cache", headers: token ? { Authorization: `Bearer ${token}` } : {} })
            .then(async (response) => {
                if (!response.ok) {
                    // 404 : pas d'image. Une autre erreur (un 504 pendant que le
                    // kernel redémarre) se retente, trois fois, de plus en plus tard.
                    entry.state = response.status === 404 || response.status === 405 ? "none" : "idle";

                    if (entry.state === "idle") {
                        retryLater(entry);
                    }

                    return;
                }
                const blob = await response.blob();

                if (!blob.type.startsWith("image/")) {
                    entry.state = "none";

                    return;
                }
                entry.url = URL.createObjectURL(blob);
                entry.type = blob.type;
                entry.state = "ok";
            })
            .catch(() => {
                entry.state = "idle";
                retryLater(entry);
            })
            .finally(() => {
                running -= 1;
                entry.listeners.forEach((listener) => listener());
                pump();
            });
    }
};

const retryLater = (entry) => {
    entry.failures = (entry.failures || 0) + 1;

    if (entry.failures > 3) {
        entry.state = "none";

        return;
    }
    setTimeout(() => {
        if (entry.users > 0 && entry.state === "idle") {
            entry.state = "queued";
            queue.push(entry);
            pump();
        }
    }, 2000 * entry.failures);
};

// Une entrée par adresse.
const entryFor = (path) => {
    if (!entries.has(path)) {
        entries.set(path, { path, state: "idle", url: null, type: "", users: 0, listeners: new Set(), timer: null });
    }

    return entries.get(path);
};

// UNE IMAGE CHOISIE GARDE L'ADRESSE DE CELLE QU'ELLE REMPLACE. Deux choses, chacune à sa place (chantier 53, P12 ; le
// porteur : « Ça retourne toujours à l'image Astro Bot ») :
//   - LE NAVIGATEUR : la route répond `no-cache` et un ETag (api/console_images.go, serveChangingImage) — `fetch` revalide
//     à chaque lecture, 304 tant que rien n'a changé, la nouvelle image sinon, rechargement compris. Elle répondait
//     `max-age=86400`, et un numéro dans l'adresse (`&v=`) ne tenait que le temps de la page : rechargée, elle reprenait
//     l'ancienne image dans le cache du navigateur, un jour durant ;
//   - CETTE PAGE : ses entrées sont gardées par adresse, et une image affichée garde la sienne tant qu'un écran la montre.
//     Après un remplacement, un numéro propre à chaque jeu et à chaque console, dans un fragment (`#v=`, qui ne part pas
//     au serveur et n'entre pas dans la clé du cache HTTP), donne une nouvelle entrée aux écrans qui la montrent ensuite :
//     la carte, la fiche, la bande, l'apparence.
const versions = new Map();
const versioned = (path, key) => (versions.get(key) ? `${path}#v=${versions.get(key)}` : path);
const bump = (key) => versions.set(key, (versions.get(key) || 0) + 1);

export const gamePath = (consoleId, reference, kind, width, game) => versioned(gameImagePath(consoleId, reference, kind, width, game), `game:${consoleId}/${reference}`);

// Oublier les images gardées d'un jeu (après qu'un administrateur les a changées) : la prochaine
// carte, fiche ou bande qui les montre les redemande. Celles affichées à l'instant restent jusqu'à
// ce qu'on les quitte.
export function forgetGameImages(consoleId, reference) {
    bump(`game:${consoleId}/${reference}`);
    const prefix = `/api/v1/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/images/`;

    for (const [path, entry] of entries) {
        if (path.startsWith(prefix) && entry.users === 0) {
            if (entry.url) {
                URL.revokeObjectURL(entry.url);
            }
            entries.delete(path);
        }
    }
}
// `server` : le serveur fédéré qui prête le fond d'une console qui n'a aucune image ici (sa liste le nomme, `#740`) —
// le kernel relaie l'image de chez lui.
export const consoleBackgroundPath = (consoleId, name, width, server = null) => versioned(`/api/v1/retrogaming/consoles/${encodeURIComponent(consoleId)}/backgrounds/${encodeURIComponent(name)}?w=${width}${server ? `&server=${encodeURIComponent(server)}` : ""}`, `band:${consoleId}`);
// Un emplacement du fond d'une console changé : ses images se relisent (et la bande relit sa liste, ConsoleVisual.jsx).
export const forgetConsoleBackgrounds = (consoleId) => bump(`band:${consoleId}`);

const acquire = (entry) => {
    entry.users += 1;
    clearTimeout(entry.timer);

    if (entry.state === "idle") {
        entry.state = "queued";
        queue.push(entry);
        pump();
    }
};

const release = (entry) => {
    entry.users -= 1;

    if (entry.users > 0) {
        return;
    }
    clearTimeout(entry.timer);
    entry.timer = setTimeout(() => {
        if (entry.users === 0 && entry.url) {
            URL.revokeObjectURL(entry.url);
            entry.url = null;
            entry.state = "idle";
        }
    }, LINGER_MS);
};

// Les largeurs que la route sait rendre : une carte en demande 256 ou 512,
// un fond 1280 ou 1920.
export const widthFor = (cssWidth) => {
    const wanted = cssWidth * Math.min(2, window.devicePixelRatio || 1);

    return [256, 512, 1280, 1920].find((width) => width >= wanted) || 1920;
};

// L'état d'une image : `undefined` tant qu'on ne sait pas, `null` sans image,
// sinon `{url, type}` (l'adresse locale, le type reçu). `path` nul n'en demande rien.
export function useImage(path) {
    const [, redraw] = useState(0);
    const entry = path ? entryFor(path) : null;

    useEffect(() => {
        if (!entry) {
            return undefined;
        }
        const listener = () => redraw((value) => value + 1);
        entry.listeners.add(listener);
        acquire(entry);

        return () => {
            entry.listeners.delete(listener);
            release(entry);
        };
    }, [entry]);

    if (!entry) {
        return undefined;
    }

    return entry.state === "ok" ? { url: entry.url, type: entry.type } : entry.state === "none" ? null : undefined;
}

// L'image d'un jeu : `undefined`, `null` ou l'adresse locale. `enabled` à faux n'en demande rien (une carte hors de vue).
// `game` — la source du jeu (libs/api.js, romSource) : `disk` ou `server`.
export function useGameImage(consoleId, reference, kind, width, enabled = true, game = {}) {
    const image = useImage(consoleId && reference && enabled ? gamePath(consoleId, reference, kind, width, game) : null);

    return image ? image.url : image;
}

// UN JEU A-T-IL CETTE IMAGE ? Demandée hors d'un écran (la vitrine d'En direct choisit ses jeux parmi ceux qui en ont
// une, Live.jsx) : rend vrai ou faux une fois la réponse arrivée. Le même chargeur et la même entrée que `useGameImage` :
// l'écran qui la montre ensuite ne la redemande pas.
export function probeGameImage(consoleId, reference, kind, width, game = {}) {
    const entry = entryFor(gamePath(consoleId, reference, kind, width, game));

    return new Promise((resolve) => {
        const settle = () => {
            if (entry.state !== "ok" && entry.state !== "none") {
                return;
            }
            entry.listeners.delete(settle);
            release(entry);
            resolve(entry.state === "ok");
        };
        entry.listeners.add(settle);
        acquire(entry);
        settle();
    });
}
