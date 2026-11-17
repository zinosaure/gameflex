// LES MÉMOIRES DE PAGE : d'où l'on vient (pour remonter), et le dernier jeu regardé par console.

// REMONTER D'UN NIVEAU : si l'écran d'avant est le parent, le Retour du
// navigateur (et celui d'Android) y ramène — l'historique reste celui qu'on a
// parcouru. Sinon (une adresse ouverte directement), on remplace par le parent.
export const goUp = (navigate, location, parent) => {
    const from = location.state?.from || "";

    if (from && from.split("?")[0] === parent.split("?")[0]) {
        navigate(-1);
    } else {
        navigate(parent, { replace: true });
    }
};

// Descendre d'un niveau en retenant d'où l'on vient (pour `goUp`).
export const goDown = (navigate, location, path) => {
    navigate(path, { state: { from: location.pathname + location.search } });
};

// LE DERNIER JEU REGARDÉ DANS LA BIBLIOTHÈQUE D'UNE CONSOLE : y revenir rouvre
// la même fiche (une mémoire de page, perdue au rechargement).
const gameByConsole = new Map();

export const rememberGame = (consoleId, reference) => {
    if (reference) {
        gameByConsole.set(consoleId, reference);
    }
};

export const recalledGame = (consoleId) => gameByConsole.get(consoleId) || null;
