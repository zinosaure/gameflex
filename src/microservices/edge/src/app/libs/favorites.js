import { useEffect, useSyncExternalStore } from "react";
import { addFavorite, listFavorites, removeFavorite } from "./api.js";
import { useAuth } from "./data.jsx";

// MES FAVORIS (le porteur, 2026-11-16 : « dans les fiches game info, remplace le bouton download this rom par add to my
// favorite ») — ceux du COMPTE, gardés au service des comptes (`/api/v1/auth/me/favorites`, favorites.go) : ils
// suivent le joueur d'un appareil à l'autre, au serveur comme au téléphone. Lus une fois pour la page et pour le compte
// connecté, puis tenus ici à chaque geste : l'engrenage d'un jeu et le Catalogue des Réglages lisent le même état, et
// un geste sur l'un se voit sur l'autre sans rien relire. Un autre compte se connecte : ils se relisent.
//
// Un favori : `{consoleId, reference, serverId, title}` — la référence que la liste des jeux donne ; un jeu d'un serveur
// fédéré garde son serveur (`serverId`, `rom.peer`).

let state = { owner: null, loaded: false, failed: false, items: [] };
let reading = null;
const listeners = new Set();

const notify = () => listeners.forEach((listener) => listener());
const subscribe = (listener) => {
    listeners.add(listener);

    return () => listeners.delete(listener);
};

const fromRow = (row) => ({ consoleId: row.console_id, reference: row.reference, serverId: row.server_id ?? null, title: row.title || "" });

// La clé d'un jeu parmi les favoris : sa console et sa référence, comme le service des comptes les tient.
export const favoriteKey = (consoleId, reference) => `${consoleId}|${reference}`;

const read = (owner) => {
    if (reading?.owner === owner) {
        return reading.promise;
    }
    const promise = listFavorites().then((answer) => {
        if (state.owner !== owner) {
            return;
        }
        state = { owner, loaded: true, failed: !answer.ok, items: answer.ok ? (answer.items || []).map(fromRow) : [] };
        notify();
    });
    reading = { owner, promise };

    return promise;
};

// `{loaded, failed, items}` pour le compte connecté (vide tant qu'ils se lisent).
export function useFavorites() {
    const { user } = useAuth();
    const owner = user?.username || null;
    const current = useSyncExternalStore(subscribe, () => state);

    useEffect(() => {
        if (!owner) {
            return;
        }

        if (state.owner !== owner) {
            state = { owner, loaded: false, failed: false, items: [] };
            reading = null;
            notify();
        }
        read(owner);
    }, [owner]);

    return current.owner === owner ? current : { owner, loaded: false, failed: false, items: [] };
}

// Le jeu est-il un de mes favoris ?
export const isFavorite = (favorites, consoleId, reference) => favorites.items.some((item) => item.consoleId === consoleId && item.reference === reference);

// POSER (`on`) OU RETIRER UN FAVORI : le service des comptes d'abord, puis l'état de la page. Rend la réponse
// (`credential`, libs/api.js), que l'appelant dit à l'utilisateur.
export async function setFavorite({ consoleId, reference, serverId = null, title = "" }, on) {
    const answer = on ? await addFavorite({ consoleId, reference, serverId, title }) : await removeFavorite(consoleId, reference);

    if (answer.ok) {
        const others = state.items.filter((item) => item.consoleId !== consoleId || item.reference !== reference);
        state = { ...state, items: on ? [...others, answer.item ? fromRow(answer.item) : { consoleId, reference, serverId, title }] : others };
        notify();
    }

    return answer;
}
