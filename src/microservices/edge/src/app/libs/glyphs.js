import { useEffect, useState } from "react";

// LES TOUCHES DE CHAQUE ENTRÉE (le porteur, 2026-11-12) : le panneau des raccourcis
// (Shortcuts.jsx, le bouton manette de l'en-tête) nomme chaque geste par la touche de
// la DERNIÈRE entrée utilisée : la famille de la manette (par son `Gamepad.id`), ou le
// clavier. Au doigt ou à la souris seuls, le bouton n'apparaît pas. Plus de ligne
// d'aide en haut à droite.
//
// LA TABLE, définie une fois. Les gestes sont ceux de libs/input.js : `a` (sud),
// `b` (est), `x` (ouest), `y` (nord), `l2`, `r2`, `select`, `start`. Des formes ou du
// texte simples, jamais un logo de marque.
//
//   famille       reconnue par l'id                          a   b   x   y   l2  r2  select  start    home
//   ───────────   ────────────────────────────────────────   ──  ──  ──  ──  ──  ──  ──────  ───────  ─────
//   playstation   054c, Sony, DualShock, DualSense,          ✕   ○   □   △   L2  R2  Create  Options  PS
//                 PlayStation
//   nintendo      057e, Nintendo, Switch, Joy-Con, Pro       B   A   Y   X   ZL  ZR  −       +        Home
//                 Controller — les étiquettes Nintendo, PAR
//                 POSITION : au sud, B ; à l'est, A
//   xbox          tout le reste (XInput, Xbox, inconnue)     A   B   X   Y   LT  RT  View    Menu     Guide
//   keyboard      une touche du clavier                      Entrée, Échap, X, Y, Q, E, M, S (pas de home)
//
// `home` ne sert qu'au menu d'une partie : L2 + Home tenus trois secondes (libs/play.js).
//
// La manette clipsée du porteur (GameSir, étiquettes Nintendo) n'est reconnue Nintendo
// que si son id le dit ; sinon elle reçoit les étiquettes Xbox.

export const GLYPHS = {
    playstation: { a: "✕", b: "○", x: "□", y: "△", l2: "L2", r2: "R2", select: "Create", start: "Options", home: "PS" },
    nintendo: { a: "B", b: "A", x: "Y", y: "X", l2: "ZL", r2: "ZR", select: "−", start: "+", home: "Home" },
    xbox: { a: "A", b: "B", x: "X", y: "Y", l2: "LT", r2: "RT", select: "View", start: "Menu", home: "Guide" },
    keyboard: { a: "Entrée", b: "Échap", x: "X", y: "Y", l2: "Q", r2: "E", select: "M", start: "S" },
};

// Les mots du clavier, en anglais.
export const KEYBOARD_EN = { Entrée: "Enter", Échap: "Esc" };

export const padFamily = (id = "") => {
    if (/054c|sony|dualshock|dualsense|playstation/i.test(id)) {
        return "playstation";
    }

    if (/057e|nintendo|switch|joy-?con|pro controller/i.test(id)) {
        return "nintendo";
    }

    return "xbox";
};

// La dernière entrée qui a une touche : une famille de la table, ou `null` (rien que le doigt
// ou la souris jusqu'ici). Toucher ou cliquer ne la change pas : une manette branchée, ou le
// clavier une fois servi, gardent leur bouton.
const EVENT = "gameflex-edge:input-kind";
let kind = null;

export const setInputKind = (next) => {
    if (next !== kind) {
        kind = next;
        document.documentElement.dataset.input = next || "pointer";
        window.dispatchEvent(new Event(EVENT));
    }
};

export function useInputKind() {
    const [current, setCurrent] = useState(kind);

    useEffect(() => {
        const update = () => setCurrent(kind);
        window.addEventListener(EVENT, update);
        update();

        return () => window.removeEventListener(EVENT, update);
    }, []);

    return current;
}
