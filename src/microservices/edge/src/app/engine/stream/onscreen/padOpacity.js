import { useSyncExternalStore } from "react";
import { saveButtonPositioning } from "../../../libs/profileApi.js";

// L'OPACITÉ DE LA MANETTE À L'ÉCRAN, RÉGLÉE DEPUIS LE MENU D'UNE PARTIE (#758 ; le porteur, 2026-11-17 : « Tu peux rajouter
// un skeletonRange pour changer l'opacité des boutons ? Des fois je trouve que c'est trop visible. », puis « C'est déjà dans
// Settings/Gamepads > edit boutons »).
//
// Ce n'est pas un réglage de plus : c'est l'opacité de la DISPOSITION EN JEU de la manette, celle de Réglages › Manettes ›
// Modifier la disposition — gardée sur le compte, par manette virtuelle, dans l'emplacement « en jeu » (`is_default`,
// `/key-positioning`, le service des comptes). La manette dessinée (ControllerLayout, non éditable) publie ici ce qu'elle a
// lu : sa manette, son opacité, son emplacement en jeu. Le menu lit et écrit ici ; la manette suit tout de suite, et
// l'écriture part au compte quand la glissière s'arrête.
//
// SANS DISPOSITION EN JEU, la glissière en crée une, dans le premier emplacement libre, avec une disposition VIDE : la
// manette garde celle par défaut (mergeSavedLayout, Controllers.jsx), et suit ses changements à venir ; l'éditeur la montre
// comme celle par défaut (Layout.jsx). Les cinq emplacements pris, aucun en jeu : l'opacité ne vaut que pour cette page.
const SAVE_AFTER_MS = 600;

// L'OPACITÉ PAR DÉFAUT (chantier 68 ; le porteur, 2026-11-19 : « bouton manette virtuelle par défaut : 40% opacité ») :
// celle d'une manette dont la disposition en jeu n'en a pas enregistré — la page de jeu, son menu, et l'éditeur de
// disposition. Une seule valeur, lue partout.
export const DEFAULT_PAD_OPACITY = 0.4;

let current = { name: null, opacity: null, slot: null, slots: [], token: null };
let timer = null;
const listeners = new Set();

function publish(next) {
    current = next;
    listeners.forEach((listener) => listener());
}

function subscribe(listener) {
    listeners.add(listener);

    return () => listeners.delete(listener);
}

// La manette dessinée dit ce qu'elle a lu (`slot` : l'emplacement en jeu, ou null ; `slots` : tous ceux de cette manette).
export function publishPadOpacity({ name, opacity, slot, slots, token }) {
    publish({ name, opacity, slot, slots: slots || [], token });
}

// Elle part (fin de partie, autre manette) : plus rien à régler.
export function forgetPadOpacity(name) {
    if (current.name === name) {
        clearTimeout(timer);
        publish({ name: null, opacity: null, slot: null, slots: [], token: null });
    }
}

// LE PAVÉ TACTILE SEUL, à côté d'une vraie manette (TouchpadSurface `alone`) : il suit la même opacité, et le menu montre
// alors sa glissière (le porteur, 2026-11-18 : « pour PS4 et PS5, on ne peut pas diminuer l'opacité du touchpad qui est
// sur l'écran »). Compté : un pavé qui part pendant qu'un autre arrive ne l'efface pas.
let alonePads = 0;
const aloneListeners = new Set();

export function markTouchpadAlone(on) {
    alonePads = Math.max(0, alonePads + (on ? 1 : -1));
    aloneListeners.forEach((listener) => listener());
}

export function useTouchpadAlone() {
    return useSyncExternalStore((listener) => {
        aloneListeners.add(listener);

        return () => aloneListeners.delete(listener);
    }, () => alonePads > 0);
}

export function usePadOpacity() {
    return useSyncExternalStore(subscribe, () => current);
}

// Régler : la manette suit aussitôt ; le compte, une fois la glissière arrêtée.
export function setPadOpacity(opacity) {
    if (!current.name) {
        return;
    }
    publish({ ...current, opacity });
    clearTimeout(timer);
    timer = setTimeout(save, SAVE_AFTER_MS);
}

async function save() {
    const { name, opacity, slot, slots, token } = current;
    const used = new Set(slots.map((entry) => entry.slot_index));
    const index = slot ? slot.slot_index : [1, 2, 3, 4, 5].find((candidate) => !used.has(candidate));

    if (!name || !token || !index) {
        return;
    }
    const answer = await saveButtonPositioning(token, name, index, slot ? slot.layout : [], true, opacity);

    // L'emplacement créé devient celui en jeu : la prochaine écriture le réécrit au lieu d'en créer un autre.
    if (answer.ok && answer.item && current.name === name) {
        publish({ ...current, slot: answer.item, slots: [...slots.filter((entry) => entry.slot_index !== index), answer.item] });
    }
}
