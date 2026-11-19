// LES QUATRE ONGLETS DE LA BARRE, dans l'ordre et avec les mots choisis par le
// porteur (2026-11-12) : En direct, Retrogaming, Streamex, Remote Play — puis
// la cloche et l'engrenage. Chaque onglet de consoles a sa route, et retient
// la console choisie ; le dernier onglet ouvert se retrouve au retour (dans
// CE navigateur seulement, rien n'est écrit sur le serveur).

export const TABS = [
    { id: "live", label: "tab.live", path: "/live" },
    { id: "retrogaming", label: "tab.retrogaming", path: "/retrogaming", family: "retro" },
    { id: "streamex", label: "tab.streamex", path: "/streamex", family: "recent" },
    { id: "remoteplay", label: "tab.remoteplay", path: "/remoteplay", family: "remote" },
];

const TAB_KEY = "gameflex-edge-tab";
const consoleKey = (tab) => `gameflex-edge-console-${tab}`;

const read = (key) => {
    try {
        return localStorage.getItem(key);
    } catch {
        return null;
    }
};

const write = (key, value) => {
    try {
        localStorage.setItem(key, value);
    } catch {
        // Sans stockage, l'onglet et la console ne sont retenus que le temps de la page.
    }
};

export const tabById = (id) => TABS.find((tab) => tab.id === id) || null;

// LA FICHE D'UN JEU DANS LA BIBLIOTHÈQUE : l'onglet de sa console (rétro ou streamex), sa console, le jeu.
export const gameScreenPath = (console, reference) => `/${console.method === "streamex" ? "streamex" : "retrogaming"}/${encodeURIComponent(console.id)}/games/${encodeURIComponent(reference)}`;

export const rememberTab = (id) => write(TAB_KEY, id);

export const lastTabId = () => tabById(read(TAB_KEY))?.id || "retrogaming";

export const lastTabPath = () => tabById(lastTabId()).path;

export const rememberConsole = (tab, id) => write(consoleKey(tab), id);

export const recalledConsole = (tab) => read(consoleKey(tab));

