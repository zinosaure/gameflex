import { useEffect, useRef } from "react";
import { playSound, wakeAudio } from "./sounds.js";
import { padFamily, setInputKind } from "./glyphs.js";

// UNE SEULE NAVIGATION, pour le clavier, la croix d'une manette (Gamepad API)
// et la télécommande d'un téléviseur — le doigt et la souris passent par les
// clics ordinaires.
//
// L'algorithme est celui de la maquette E (et de Skeleton.navigateFocus) :
// l'élément focalisable le plus proche dans la direction, rangée par rangée,
// avec des sauts déclarés (`data-up`, `data-down`, `data-left`, `data-right`)
// que l'écran résout. Chaque écran pose son gestionnaire (`useScreenInput`) :
// il voit la flèche (`direction`) et A (`activate`) avant l'algorithme — les
// consoles et la bibliothèque gardent HAUT et BAS pour changer de page — et dit
// ce que font Retour, X, Y, L2/R2, Select et Start. Un geste qu'aucun écran ne
// prend va à la barre du haut (Shell.jsx) : L2/R2 parcourent l'en-tête, Select ouvre
// et referme les réglages, Start ramène à En direct — sur tous les écrans.
//
// UNE TOUCHE PAR GESTE (le porteur, 2026-11-12, à la vraie manette). La manette suit
// la disposition standard de la Gamepad API ; le clavier a un équivalent pour chacune :
//
//   geste                 manette (indice)        clavier
//   ───────────────────   ─────────────────────   ──────────────────────────────
//   haut, bas, gauche,    croix (12, 13, 14, 15)  flèches
//   droite
//   OK / A                bouton sud (0) — Croix  Entrée, Espace
//   Retour / B            bouton est (1) — Rond   Échap, Retour arrière
//   X                     bouton ouest (2) —      X
//                         Carré
//   Y                     bouton nord (3) —       Y
//                         Triangle
//   parcourir l'en-tête   L2, R2 (6, 7)           Q, E — PageUp, PageDown
//   (onglets, raccourcis,
//   réglages ; sans boucler)
//   réglages (bascule)    Select (8)              M, Menu
//   En direct (l'accueil) Start (9)               S
//   le menu d'une partie  L2 + Home (6 + 16),     Échap
//   (la page de jeu)      tenus 3 s (libs/play.js)
//
// Ce que font A, B, X et Y selon l'écran :
//   - les consoles : A ouvre la bibliothèque, où que soit la lumière (leurs réglages
//     sont dans Réglages › Administration › Consoles : Y n'y fait rien) ;
//   - la fiche d'un jeu : A lance le jeu (Jouer, Continuer, Reprendre — comme le
//     bouton), B ferme le panneau ouvert, sinon éteint la lumière, sinon revient
//     aux consoles ; Y ouvre les réglages du jeu, X la recherche ;
//   - un panneau : A active, B le referme.
// Les notifications n'ont plus de touche : elles restent sur la cloche.
//
// L1 et R1 (4, 5) ne font rien. Une croix tenue se répète (380 ms, puis toutes
// les 110 ms). Chaque geste a son son (libs/sounds.js) : un déplacement, valider
// (A), annuler (B). Les raccourcis se lisent
// selon la dernière entrée — la famille de la manette ou le clavier (libs/glyphs.js) :
// le bouton manette de l'en-tête les liste (Shortcuts.jsx). L1, R1, L3 et R3 sont libres.

const stack = [];
const root = () => document.documentElement;

export function useScreenInput(handler) {
    const ref = useRef(handler);
    ref.current = handler;

    useEffect(() => {
        const entry = { get: () => ref.current };
        stack.push(entry);

        return () => {
            const index = stack.indexOf(entry);

            if (index >= 0) {
                stack.splice(index, 1);
            }
        };
    }, []);
}

// La même chose hors d'un composant : la fenêtre de confirmation de Skeleton (Bricks.jsx, useConfirm), posée par le
// code de Skeleton et non par React. Rend de quoi la retirer.
export function pushScreenInput(handler) {
    const entry = { get: () => handler };
    stack.push(entry);

    return () => {
        const index = stack.indexOf(entry);

        if (index >= 0) {
            stack.splice(index, 1);
        }
    };
}

const top = () => stack.length ? stack[stack.length - 1].get() : {};

// Ce qui se focalise : visible, actif, hors d'un calque inerte, et dans le
// calque du dessus quand il y en a un (« Toutes les consoles »).
export const navigable = () => {
    const layers = document.querySelectorAll("[data-nav-layer]");
    const scope = layers.length ? layers[layers.length - 1] : document.getElementById("root");

    return Array.from(scope.querySelectorAll("[data-nav]"))
        .filter((element) => !element.disabled && element.offsetParent !== null && !element.closest("[inert]"));
};

// Faire défiler le conteneur (`data-scroller`) pour que l'élément choisi reste en vue.
const reveal = (element, instant) => {
    const scroller = element.closest("[data-scroller]");

    if (!scroller) {
        return;
    }
    const box = element.getBoundingClientRect(), frame = scroller.getBoundingClientRect();
    const margin = Math.min(box.height * 0.6, frame.height * 0.2);
    const behavior = instant ? "auto" : "smooth";
    // Le haut de l'élément reste toujours visible : sur un écran bas, une
    // jaquette plus haute que la place n'est jamais coupée par le haut.
    const edge = Math.min(margin, 16);

    if (box.top < frame.top + edge) {
        scroller.scrollBy({ top: box.top - frame.top - edge, behavior });
    } else if (box.bottom > frame.bottom - margin) {
        scroller.scrollBy({ top: Math.min(box.bottom - frame.bottom + margin, box.top - frame.top - edge), behavior });
    }
};

export const focusElement = (element, instant = false) => {
    if (!element) {
        return false;
    }
    element.focus({ preventScroll: true });
    reveal(element, instant);

    return true;
};

const resolve = (override) => {
    const custom = top().resolve?.(override);

    if (custom !== undefined) {
        return custom;
    }

    return document.querySelector(`[data-key="${CSS.escape(override)}"]`);
};

// Un sélecteur natif ne s'ouvre pas à la manette : GAUCHE et DROITE y passent à
// l'option suivante qui n'est pas grisée (l'événement `change` part comme au doigt).
export const cycleSelect = (select, step) => {
    const options = Array.from(select.options);
    let index = select.selectedIndex + step;

    while (index >= 0 && index < options.length && options[index].disabled) {
        index += step;
    }

    if (index < 0 || index >= options.length) {
        return false;
    }
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, options[index].value);
    select.dispatchEvent(new Event("change", { bubbles: true }));

    return true;
};

// Une glissière (`input[type=range]`) : GAUCHE et DROITE la déplacent d'un pas, comme les flèches du
// clavier (l'événement `input` part comme au doigt) ; à son bout, la croix sort vers le champ voisin.
export const stepRange = (range, step) => {
    const before = range.value;

    if (step > 0) {
        range.stepUp();
    } else {
        range.stepDown();
    }
    if (range.value === before) {
        return false;
    }
    const value = range.value;
    range.value = before;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(range, value);
    range.dispatchEvent(new Event("input", { bubbles: true }));

    return true;
};

// UN PANNEAU QUI DÉFILE À LA CROIX : quand la commande suivante est loin sous le
// bord (du texte entre les deux), ou qu'il n'y en a plus, HAUT et BAS font défiler
// le panneau d'un pas — le texte se lit — avant d'y mettre la lumière.
const scrollStep = (scroller, sign) => {
    const before = scroller.scrollTop;
    scroller.scrollBy({ top: sign * scroller.clientHeight * 0.6, behavior: "smooth" });

    return scroller.scrollHeight > scroller.clientHeight && (sign > 0 ? before + scroller.clientHeight < scroller.scrollHeight - 1 : before > 0);
};

const scopeScroller = () => {
    const layers = document.querySelectorAll("[data-nav-layer]");
    const scope = layers.length ? layers[layers.length - 1] : document.getElementById("root");

    return Array.from(scope.querySelectorAll("[data-scroller]")).find((element) => element.offsetParent !== null && element.scrollHeight > element.clientHeight + 1) || null;
};

// UN BOUTON DE L'EN-TÊTE ATTEINT PAR L2 / R2 (Shell.jsx) : il a la lumière, A l'ouvre ; B ou
// une direction la rendent à l'écran (le point neutre).
let headerFocus = null;

export const focusHeader = (element) => {
    headerFocus = element || null;
    focusElement(element);
};

// Quitter l'en-tête : le bouton atteint par L2 / R2, ou tout élément de l'en-tête qui a la lumière
// (un clic, Tab) — une direction n'y circule jamais, elle rend la lumière à l'écran.
const leaveHeader = (direction = false) => {
    const current = document.activeElement;

    if ((headerFocus && current === headerFocus) || (direction && current?.closest?.("[data-nav-skip]"))) {
        current.blur();
        headerFocus = null;

        return true;
    }
    headerFocus = null;

    return false;
};

export function move(direction) {
    const current = document.activeElement;

    if (leaveHeader(true)) {
        return;
    }

    if (top().direction?.(direction, current)) {
        return;
    }

    // Au bout de ses options, la croix sort du sélecteur : vers le bouton qui lui est accolé (Bricks.jsx, IconAddon),
    // ou la page suivante d'une liste — sinon ils resteraient hors d'atteinte.
    if (current?.tagName === "SELECT" && (direction === "left" || direction === "right") && current.closest("[data-nav-layer]")
        && cycleSelect(current, direction === "right" ? 1 : -1)) {
        return;
    }

    if (current?.type === "range" && (direction === "left" || direction === "right") && stepRange(current, direction === "right" ? 1 : -1)) {
        return;
    }
    // L'EN-TÊTE N'EST JAMAIS UNE CIBLE DE LA CROIX (`data-nav-skip`, Shell.jsx) : seuls L2 / R2 y vont.
    const candidates = navigable().filter((element) => !element.closest("[data-nav-skip]"));

    if (!candidates.includes(current)) {
        focusElement(top().fallback?.() || candidates[0]);

        return;
    }
    const override = current.dataset[direction];

    if (override) {
        const element = resolve(override);

        if (element && !element.disabled) {
            focusElement(element);

            return;
        }
    }
    const from = current.getBoundingClientRect();
    const horizontal = direction === "left" || direction === "right";
    const sign = direction === "right" || direction === "down" ? 1 : -1;
    const options = [];

    for (const element of candidates) {
        if (element === current) {
            continue;
        }
        const to = element.getBoundingClientRect();
        const gap = horizontal ? (sign > 0 ? to.left - from.right : from.left - to.right) : (sign > 0 ? to.top - from.bottom : from.top - to.bottom);
        const centre = horizontal
            ? sign * ((to.left + to.width / 2) - (from.left + from.width / 2))
            : sign * ((to.top + to.height / 2) - (from.top + from.height / 2));

        if (centre <= 2 || gap < -Math.min(from.width, from.height) / 2) {
            continue;
        }
        const separation = horizontal
            ? Math.max(0, Math.max(to.top, from.top) - Math.min(to.bottom, from.bottom))
            : Math.max(0, Math.max(to.left, from.left) - Math.min(to.right, from.right));
        const offAxis = separation + (horizontal
            ? Math.abs((to.top + to.height / 2) - (from.top + from.height / 2))
            : Math.abs((to.left + to.width / 2) - (from.left + from.width / 2))) * 0.15;
        options.push({ element, gap: Math.max(0, gap), offAxis, aligned: separation === 0 });
    }

    // Le conteneur qui défile : celui de l'élément en lumière, sinon celui de l'écran (ou du panneau
    // du dessus) — depuis un onglet, BAS fait encore défiler une liste sans commande (En direct).
    const scroller = horizontal ? null : current.closest("[data-scroller]") || scopeScroller();

    if (!options.length) {
        if (!(scroller && scrollStep(scroller, sign))) {
            top().edge?.(direction);
        }

        return;
    }
    // LA MÊME RANGÉE D'ABORD : à droite d'une jaquette, la voisine de la rangée
    // — pas un onglet de la barre qui déborde en diagonale (vu au téléviseur).
    const sameRow = options.filter((option) => option.aligned);
    const pool = sameRow.length ? sameRow : options;
    const nearest = Math.min(...pool.map((option) => option.gap));
    const band = pool.filter((option) => option.gap <= nearest + 24);
    band.sort((a, b) => a.offAxis - b.offAxis || a.gap - b.gap);

    if (scroller && scroller.closest("[data-nav-layer]")) {
        const frame = scroller.getBoundingClientRect();
        const to = band[0].element.getBoundingClientRect();
        const beyond = sign > 0 ? to.bottom - frame.bottom : frame.top - to.top;

        if (beyond > frame.height * 0.6 && scrollStep(scroller, sign)) {
            return;
        }
    }
    focusElement(band[0].element);
}

const activate = () => {
    const current = document.activeElement;
    playSound("enter");
    headerFocus = null;

    if (top().activate?.(current)) {
        return;
    }

    // Un bouton, un lien, un interrupteur ou une case (à cocher, d'option), le choix d'un fichier :
    // comme au doigt. L'étiquette d'un choix segmenté (ChoiceRow, Bricks.jsx : la croix s'arrête sur elle, pas sur sa
    // case) aussi — son clic de A (`detail` 0) choisit sa case ; sans elle, A n'y faisait rien.
    if (current && current.matches("button, a, [role=button], input[type=checkbox], input[type=radio], input[type=file], label[data-nav]") && !current.disabled) {
        current.click();
    }
};

// Le gestionnaire de l'écran d'abord ; sinon la barre du haut (un événement).
const toScreenOr = (name, event, detail) => {
    const handler = top()[name];

    if (handler) {
        handler(detail);
    } else {
        window.dispatchEvent(new CustomEvent(event, { detail }));
    }
};

export const BAR_EVENTS = { tab: "gameflex-edge:tab", settings: "gameflex-edge:settings", home: "gameflex-edge:home" };

const back = () => {
    playSound("back");

    if (!leaveHeader()) {
        top().back?.();
    }
};
const switchTab = (delta) => {
    playSound("move");
    toScreenOr("switchTab", BAR_EVENTS.tab, delta);
};
const selectButton = () => {
    playSound("enter");
    toScreenOr("select", BAR_EVENTS.settings);
};
const startButton = () => {
    playSound("enter");
    toScreenOr("start", BAR_EVENTS.home);
};
const yButton = () => {
    if (top().y) {
        playSound("enter");
        top().y();
    }
};
const xButton = () => {
    if (top().x) {
        playSound("enter");
        top().x();
    }
};
const step = (direction) => {
    playSound("move");
    move(direction);
};

// Une touche ou la manette : les aides des boutons reviennent, et le focus se montre partout.
const keyInput = () => {
    root().classList.remove("touch", "pointing");
};

const typing = (element) => element && (element.tagName === "TEXTAREA" || (element.tagName === "INPUT" && !["button", "checkbox", "radio"].includes(element.type)));

function onKeyDown(event) {
    if (event.altKey || event.ctrlKey || event.metaKey || navigationSuspended) {
        return;
    }
    wakeAudio();
    keyboardUsed = true;
    setInputKind("keyboard");
    keyInput();
    const directions = { ArrowUp: "up", ArrowDown: "down", ArrowLeft: "left", ArrowRight: "right" };
    const inField = typing(document.activeElement);

    // DANS UN CHAMP DE SAISIE, les lettres s'écrivent et gauche/droite
    // déplacent le curseur — jusqu'au bord du texte, d'où elles sortent du
    // champ ; haut/bas en sortent aussi, et Échap le quitte.
    if (inField) {
        const field = document.activeElement;
        const atStart = field.selectionStart === 0 && field.selectionEnd === 0;
        const atEnd = field.selectionStart === field.value.length;

        if (event.key === "ArrowUp" || event.key === "ArrowDown" || (event.key === "ArrowLeft" && atStart) || (event.key === "ArrowRight" && atEnd)) {
            event.preventDefault();
            step(directions[event.key]);
        } else if (event.key === "Escape") {
            event.preventDefault();
            field.blur();
        }

        return;
    }

    if (directions[event.key]) {
        event.preventDefault();
        step(directions[event.key]);
    } else if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        activate();
    } else if (["Escape", "Backspace", "GoBack", "BrowserBack"].includes(event.key)) {
        event.preventDefault();
        back();
    } else if (event.key === "y" || event.key === "Y") {
        yButton();
    } else if (event.key === "x" || event.key === "X") {
        xButton();
    } else if (event.key === "q" || event.key === "Q" || event.key === "PageUp") {
        switchTab(-1);
    } else if (event.key === "e" || event.key === "E" || event.key === "PageDown") {
        switchTab(1);
    } else if (event.key === "m" || event.key === "M" || event.key === "ContextMenu") {
        selectButton();
    } else if (event.key === "s" || event.key === "S") {
        startButton();
    }
}

// LA MANETTE : la disposition standard de la Gamepad API (la table en tête de fichier).
const padState = new Map();
// Le clavier a-t-il servi ? Le bouton des raccourcis reste alors, même manette débranchée.
let keyboardUsed = false;
let padLoop = 0;
// LA MANETTE SUSPENDUE pendant qu'on écoute un accord (Réglages › Manettes) : ses boutons servent
// à l'accord, pas à la navigation. Un bouton encore tenu à la reprise ne déclenche rien.
let padsSuspended = false;
export const suspendPads = (on) => {
    padsSuspended = on;
};
// LA NAVIGATION SUSPENDUE PENDANT QU'ON JOUE (la page de jeu, screens/play/PlayShell.jsx) : la
// manette ET le clavier vont au jeu, et plus rien n'a le focus — une touche Entrée du jeu
// n'actionnerait pas un bouton de la page. Le menu ouvert, elle reprend.
let navigationSuspended = false;
export const suspendNavigation = (on) => {
    navigationSuspended = on;
};

const padTick = () => {
    const pads = navigator.getGamepads ? Array.from(navigator.getGamepads()).filter(Boolean) : [];
    const now = performance.now();
    const actions = { 12: () => step("up"), 13: () => step("down"), 14: () => step("left"), 15: () => step("right"), 0: activate, 1: back, 2: xButton, 3: yButton, 6: () => switchTab(-1), 7: () => switchTab(1), 8: selectButton, 9: startButton };

    for (const pad of pads) {
        for (const index of Object.keys(actions)) {
            const id = pad.index + ":" + index;
            const down = pad.buttons[index] && pad.buttons[index].pressed;
            const held = padState.get(id);

            if (!down) {
                padState.delete(id);
                continue;
            }

            if (padsSuspended || navigationSuspended) {
                padState.set(id, Infinity);
                continue;
            }
            keyInput();
            setInputKind(padFamily(pad.id));

            if (!held) {
                padState.set(id, now + 380);
                actions[index]();
            } else if (Number(index) >= 12 && now >= held) {
                padState.set(id, now + 110);
                actions[index]();
            }
        }
    }
    padLoop = pads.length ? requestAnimationFrame(padTick) : 0;
};

// Une manette branchée fait paraître le bouton des raccourcis (Shell.jsx), à sa famille ;
// la dernière débranchée le retire, si rien d'autre n'a servi depuis.
const onPadConnected = (event) => {
    setInputKind(padFamily(event.gamepad?.id));
    startPads();
};

const onPadDisconnected = () => {
    const pads = navigator.getGamepads ? Array.from(navigator.getGamepads()).filter(Boolean) : [];

    if (!pads.length && document.documentElement.dataset.input !== "keyboard") {
        setInputKind(keyboardUsed ? "keyboard" : null);
    }
};

const startPads = () => {
    if (!padLoop) {
        padLoop = requestAnimationFrame(padTick);
    }
};

// Au doigt ou à la souris, le focus ne reste pas affiché sur ce qu'on a touché
// (styles : `pointing`) ; au doigt, les aides des boutons disparaissent.
function onPointerDown(event) {
    wakeAudio();
    root().classList.add("pointing");

    if (event.pointerType === "touch" && !root().classList.contains("tv")) {
        root().classList.add("touch");
    }
}

// LES SONS AU DOIGT ET À LA SOURIS (le porteur, 2026-11-14 : « les sound effects doivent être jouables même si on
// n'utilise pas de manette/clavier physique ») : toucher ce que la croix atteint (`data-nav` : un bouton, une ligne,
// une tuile, un onglet, un lien du fil d'Ariane, un interrupteur) sonne comme A, et un retour (`data-sound="back"` : le
// rond de Retour, la croix d'un panneau ; Annuler d'une fenêtre de confirmation) comme B. Écrit ICI une fois, pour
// tout le launcher. Le clic qu'Entrée, Espace ou A produisent (`detail` 0) a déjà sonné ; un champ qu'on touche pour
// écrire ne sonne pas ; la partie en cours non plus (la manette à l'écran n'est pas une navigation). L'interrupteur
// des sons décide comme pour la manette (libs/sounds.js).
const BACK_CONTROLS = '[data-sound="back"], .modal [data-role="cancel"]';

function onPointerClick(event) {
    const target = event.detail > 0 && !navigationSuspended && event.target instanceof Element ? event.target : null;
    // La ligne d'un interrupteur se touche tout entière (SwitchRow) : c'est son interrupteur qui sonne.
    const control = target && (target.closest("[data-nav]") || target.closest(".gf-row-switch")?.querySelector("[data-nav]"));

    if (!control || control.disabled || control.getAttribute("aria-disabled") === "true" || typing(control) || control.tagName === "SELECT") {
        return;
    }
    playSound(control.matches(BACK_CONTROLS) ? "back" : "enter");
}

// Le focus visible : la classe `is-focus`, une seule forme pour le clavier, la croix et le doigt.
function onFocusIn(event) {
    document.querySelectorAll(".is-focus").forEach((node) => node.classList.remove("is-focus"));
    const element = event.target.closest("[data-nav]");

    if (element) {
        element.classList.add("is-focus");
        // Un élément qui se redessine en prenant la lumière (son `onFocus` change son état, React
        // réécrit son `className`) perdait la classe aussitôt posée : on la remet après le rendu.
        requestAnimationFrame(() => {
            if (element.contains(document.activeElement)) {
                element.classList.add("is-focus");
            }
        });
    }
}

function onFocusOut(event) {
    event.target.closest?.("[data-nav]")?.classList.remove("is-focus");
}

export function installInput() {
    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("click", onPointerClick, true);
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);
    window.addEventListener("gamepadconnected", onPadConnected);
    window.addEventListener("gamepaddisconnected", onPadDisconnected);
    startPads();
}
