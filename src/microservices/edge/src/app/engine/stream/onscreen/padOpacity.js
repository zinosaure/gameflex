import { useSyncExternalStore } from "react";
import { saveButtonPositioning } from "../../../libs/api.js";

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

let current = { name: null, opacity: null, slot: null, slots: [], saves: false, lastName: null };
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

// La manette dessinée dit ce qu'elle a lu (`slot` : l'emplacement en jeu, ou null ; `slots` : tous ceux de cette manette ;
// `saves` : la disposition s'est lue, l'opacité s'enregistre).
export function publishPadOpacity({ name, opacity, slot, slots, saves }) {
    publish({ name, opacity, slot, slots: slots || [], saves: Boolean(saves), lastName: name });
}

// Elle part (fin de partie, autre manette, manette à l'écran coupée) : elle n'est plus dessinée (`name` à null), MAIS SA
// DISPOSITION RESTE (`lastName`, son emplacement) : la manette à l'écran coupée, la glissière du menu règle
// toujours son opacité, celle du pavé tactile resté seul à côté d'une vraie manette (PS4, PS5) et celle que la manette
// retrouvera (le porteur, 2026-11-24 : « tu peux laisser l'opacité activée car ça concerne aussi le touchpad sur l'écran
// quand la manette est cachée »). Une écriture en attente part quand même.
export function forgetPadOpacity(name) {
    if (current.name === name) {
        publish({ ...current, name: null, lastName: name });
    }
}

export function usePadOpacity() {
    return useSyncExternalStore(subscribe, () => current);
}

// Régler : la manette suit aussitôt ; le compte, une fois la glissière arrêtée.
export function setPadOpacity(opacity) {
    if (!current.name && !current.lastName) {
        return;
    }
    publish({ ...current, opacity });
    clearTimeout(timer);
    timer = setTimeout(save, SAVE_AFTER_MS);
}

async function save() {
    const { opacity, slot, slots, saves } = current;
    const name = current.name || current.lastName;
    const used = new Set(slots.map((entry) => entry.slot_index));
    const index = slot ? slot.slot_index : [1, 2, 3, 4, 5].find((candidate) => !used.has(candidate));

    if (!name || !saves || !index) {
        return;
    }
    const answer = await saveButtonPositioning(name, index, slot ? slot.layout : [], true, opacity);

    // L'emplacement créé devient celui en jeu : la prochaine écriture le réécrit au lieu d'en créer un autre.
    if (answer.ok && answer.item && (current.name || current.lastName) === name) {
        publish({ ...current, slot: answer.item, slots: [...slots.filter((entry) => entry.slot_index !== index), answer.item] });
    }
}
