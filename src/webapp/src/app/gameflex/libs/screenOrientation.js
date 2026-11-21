import { useSyncExternalStore } from "react";
import { can } from "./capabilities.js";
import { homeOrigin } from "./serverBase.js";
import { hasPhysicalGamepad, onGamepadPresence } from "./gamepadPresence.js";

// L'ORIENTATION DE L'ÉCRAN — une préférence de l'APPAREIL (#406, 2026-10-19).
//
// Elle se réglait dans le panneau d'une partie, pour cette partie seulement,
// et ne se gardait nulle part. Le porteur la veut pour toute l'application,
// réglée dans Réglages › Paramètres de l'application. Comme les préférences
// de manette (gamepadPreferences.js), elle décrit l'appareil qu'on tient en
// main, pas un compte : elle vit dans ce navigateur.
//
// Ce qu'elle fait dépend de l'hôte :
//   - dans l'APPLICATION (`can("screenOrientation")`), l'activité Android
//     pivote : toute l'application, menus et console d'accueil compris, et
//     l'activité la garde d'un lancement à l'autre ;
//   - dans un NAVIGATEUR, rien ne peut tourner la page entière : pendant une
//     partie seulement, le plein écran et `screen.orientation.lock()`, ou à
//     défaut l'image tournée en CSS (use-screen-rotation.js).
//
// Quatre choix (le porteur, 2026-10-21) :
//   - « auto », SUIVRE LE TÉLÉPHONE — le défaut. Il respecte le verrou de
//     rotation d'Android : qui l'a mis ne voit rien basculer ;
//   - les deux paysages, nommés par le sens où l'on TOURNE le téléphone :
//     « landscape », sens antihoraire (le haut à gauche — `landscape-primary`,
//     SCREEN_ORIENTATION_LANDSCAPE), et « landscape-reverse », sens horaire ;
//   - « portrait-half », le demi-écran des manettes clipsées (plus bas).
// Le portrait verrouillé n'en est plus : le verrou de rotation du téléphone
// fait la même chose.
const KEY = "gameflex_screen_orientation";
const DEFAULT_ORIENTATION = "auto";

export const SCREEN_ORIENTATIONS = ["auto", "landscape-reverse", "landscape", "portrait-half"];

let current = read();
const listeners = new Set();

function read() {
    try {
        const stored = typeof window === "undefined" ? null : window.localStorage.getItem(KEY);

        return SCREEN_ORIENTATIONS.includes(stored) ? stored : DEFAULT_ORIENTATION;
    } catch {
        return DEFAULT_ORIENTATION;
    }
}

export function getScreenOrientation() {
    return current;
}

export function setScreenOrientation(mode) {
    if (!SCREEN_ORIENTATIONS.includes(mode)) {
        return Promise.resolve(false);
    }
    try {
        window.localStorage.setItem(KEY, mode);
    } catch {
        /* stockage indisponible — la valeur en mémoire vaut pour cette visite */
    }
    current = mode;
    listeners.forEach((listener) => listener());
    watchHalfScreen();

    return applyScreenOrientation();
}

// L'application pivote d'elle-même — c'est l'activité qui tourne, et elle
// garde le réglage. Rendu : vrai si l'hôte l'a pris. Hors de l'application,
// rien à demander : la page de jeu s'en charge pendant une partie.
//
// Vers NOTRE serveur (homeOrigin), jamais vers l'hôte d'une partie fédérée
// qu'une page de jeu aurait posé en surcharge : c'est ce téléphone qui tourne.
export function applyScreenOrientation() {
    if (!can("screenOrientation")) {
        return Promise.resolve(false);
    }

    return fetch(`${homeOrigin()}/api/v1/device/orientation`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mode: current }),
    }).then((response) => response.ok).catch(() => false);
}

// LE DEMI-ÉCRAN DES MANETTES CLIPSÉES (#410, le porteur, 2026-10-21).
//
// Une manette comme la GameSir Pocket Taco se clipse sur le bas du téléphone
// tenu debout, façon Game Boy : elle cache la moitié basse de l'écran. En
// « portrait-half », dès qu'une manette est branchée — n'importe laquelle :
// la Taco n'est pas la seule de ce format —, l'interface se range dans la
// moitié haute ; la manette partie, elle reprend tout l'écran. Une page ne
// voit une manette qu'après un premier appui sur l'un de ses boutons : c'est
// à ce moment-là que l'interface remonte.
//
// Toute la page suit d'un seul geste : `data-half-screen` sur <html>, et la
// hauteur cachée dans `--gameflex-covered-height`. css-js/app.css fait le
// reste — panneaux, fenêtres, toasts et plein écran compris.
//
// La part de l'interface se règle, de 30 à 70 % de l'écran, 50 par défaut (le
// porteur, 2026-10-21) : la manette ne cache pas la même part sur tous les
// téléphones. Comme l'orientation, le réglage vit dans ce navigateur.
//
// Elle se compte sur l'ÉCRAN, pas sur la page : dans un navigateur, la barre
// d'adresse pousse la page vers le bas, et la moitié de la PAGE déborderait
// sous la manette. On retire donc la part cachée de l'écran depuis le bas de
// la page, qui est le bas de l'écran à la barre de navigation d'Android près :
// l'interface s'arrête un peu au-dessus de la manette, jamais dessous. En plein
// écran, la mesure est exacte.
const HEIGHT_KEY = "gameflex_half_screen_height";
export const HALF_SCREEN_MIN = 30;
export const HALF_SCREEN_MAX = 70;
const HALF_SCREEN_DEFAULT = 50;

let halfScreenHeight = readHalfScreenHeight();
let unwatchGamepads = null;

function isHalfScreenHeight(value) {
    return Number.isFinite(value) && value >= HALF_SCREEN_MIN && value <= HALF_SCREEN_MAX;
}

function readHalfScreenHeight() {
    try {
        const stored = typeof window === "undefined" ? NaN : Number(window.localStorage.getItem(HEIGHT_KEY));

        return isHalfScreenHeight(stored) ? stored : HALF_SCREEN_DEFAULT;
    } catch {
        return HALF_SCREEN_DEFAULT;
    }
}

export function getHalfScreenHeight() {
    return halfScreenHeight;
}

// En pourcentage de l'écran. Rendu : faux pour une valeur hors bornes, qui
// n'est ni gardée ni appliquée — un chiffre en cours de frappe, par exemple.
export function setHalfScreenHeight(value) {
    const height = Number(value);
    if (!isHalfScreenHeight(height)) {
        return false;
    }
    try {
        window.localStorage.setItem(HEIGHT_KEY, String(height));
    } catch {
        /* stockage indisponible — la valeur en mémoire vaut pour cette visite */
    }
    halfScreenHeight = height;
    applyHalfScreen();

    return true;
}

function applyHalfScreen() {
    const root = document.documentElement;
    const portrait = window.matchMedia?.("(orientation: portrait)").matches ?? true;

    if (current === "portrait-half" && portrait && hasPhysicalGamepad()) {
        // Debout, la hauteur de l'écran est son grand côté — quel que soit le
        // sens dans lequel le navigateur rapporte ses dimensions.
        const screenHeight = Math.max(window.screen.width, window.screen.height);
        const covered = Math.round((screenHeight * (100 - halfScreenHeight)) / 100);
        root.style.setProperty("--gameflex-covered-height", `${covered}px`);
        root.setAttribute("data-half-screen", "");
    } else {
        root.style.removeProperty("--gameflex-covered-height");
        root.removeAttribute("data-half-screen");
    }
}

// Les manettes ne se surveillent que dans ce mode : les autres n'en ont pas
// besoin, et la surveillance sonde toutes les deux secondes.
function watchHalfScreen() {
    if (current === "portrait-half" && !unwatchGamepads) {
        unwatchGamepads = onGamepadPresence(applyHalfScreen);
        window.addEventListener("resize", applyHalfScreen);
    } else if (current !== "portrait-half" && unwatchGamepads) {
        unwatchGamepads();
        unwatchGamepads = null;
        window.removeEventListener("resize", applyHalfScreen);
    }
    applyHalfScreen();
}

if (typeof window !== "undefined") {
    watchHalfScreen();
}

function subscribe(listener) {
    listeners.add(listener);

    return () => listeners.delete(listener);
}

export function useScreenOrientation() {
    return useSyncExternalStore(subscribe, getScreenOrientation);
}
