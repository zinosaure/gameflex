import { useEffect, useSyncExternalStore } from "react";
import { resolveUsers } from "./api.js";

// LES NOMS AFFICHÉS DES COMPTES, pour toute l'application : une table, remplie au besoin par le service des comptes
// (`POST /users/resolve`). En direct, la page de jeu, son menu et ses notifications la lisent — chacun demande les noms
// qu'il montre (`useNames`), seuls ceux qu'on n'a pas encore demandés partent. UNE DEMANDE EN ÉCHEC LES REND À DEMANDER
// (le porteur, 2026-11-19 : « Sous la vidéo du direct c'est le display name qu'il faut afficher. Pas le username » — la
// table vivait dans SessionPoolProvider, monté sur la page de jeu seulement : En direct ne la voyait pas).
let names = {};
const asked = new Set();
const listeners = new Set();

const subscribe = (listener) => {
    listeners.add(listener);

    return () => listeners.delete(listener);
};
const snapshot = () => names;

export function requestNames(list) {
    const missing = [...new Set(list)].filter((username) => username && username !== "anonymous" && !asked.has(username));

    if (!missing.length) {
        return;
    }
    missing.forEach((username) => asked.add(username));
    resolveUsers(missing).then((answer) => {
        if (answer.ok && answer.item) {
            names = { ...names, ...answer.item };
            listeners.forEach((listener) => listener());
        } else {
            missing.forEach((username) => asked.delete(username));
        }
    });
}

// La table entière ; `usernames` : ceux que l'appelant va montrer, demandés s'il le faut.
export function useNames(usernames = []) {
    const table = useSyncExternalStore(subscribe, snapshot);
    const key = [...new Set(usernames.filter(Boolean))].sort().join(",");

    useEffect(() => {
        if (key) {
            requestNames(key.split(","));
        }
    }, [key]);

    return table;
}
