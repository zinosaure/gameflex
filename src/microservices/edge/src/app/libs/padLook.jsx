import React from "react";

// LA MANETTE À L'ÉCRAN AU STYLE DU LAUNCHER (chantier 52, option A du porteur) : le moteur de
// placement, les dispositions enregistrées et l'éditeur restent ceux de la webapp
// (stream/onscreen/Controllers.jsx) ; seul le dessin des boutons change, par son PadLookContext. La
// même apparence dans une partie (PlayShell.jsx) et dans l'éditeur (settings/gamepads/layout/Controller.jsx) : ce qu'on
// y voit est ce qu'on aura en jeu.
//
// Un disque sombre et translucide, un liseré clair, le glyphe en blanc ; l'appui prend l'indigo du
// thème. Les glyphes sont ceux de Bootstrap Icons, ou une lettre : les gabarits (controllers/presets.js)
// nomment des icônes Font Awesome, traduites ici — une icône qui n'y est pas montre le texte du bouton.

const ICONS = {
    "fa-play": "bi-play-fill",
    "fa-stop": "bi-stop-fill",
    "fa-plus": "bi-plus-lg",
    "fa-minus": "bi-dash-lg",
    "fa-bars": "bi-list",
    "fa-xmark": "bi-x-lg",
    "fa-circle": "bi-circle",
    "fa-square": "bi-square",
    "fa-arrow-up": "bi-caret-up-fill",
    "fa-arrow-down": "bi-caret-down-fill",
    "fa-arrow-left": "bi-caret-left-fill",
    "fa-arrow-right": "bi-caret-right-fill",
};
const LETTERS = { "fa-a": "A", "fa-b": "B", "fa-x": "X", "fa-y": "Y", "fa-z": "Z", "fa-l": "L", "fa-r": "R" };

function glyph(button) {
    const classes = String(button.icon || "").split(/\s+/);
    const bootstrap = classes.find((name) => name.startsWith("bi-"));
    const awesome = classes.find((name) => ICONS[name] || LETTERS[name]);

    if (bootstrap || ICONS[awesome]) {
        return <i className={`bi ${bootstrap || ICONS[awesome]}`}></i>;
    }

    return <span className="pad-letter">{LETTERS[awesome] || button.text || button.name}</span>;
}

const ACCENT = (share) => `color-mix(in srgb, var(--accent) ${share}%, transparent)`;
// Le disque, son liseré et son ombre, définis une fois : les boutons ET le pavé tactile (#758 ; le porteur, 2026-11-17 :
// « la couleur des bordures n'est pas comme les autres »).
const DISC = "rgba(14, 16, 26, .42)";
const EDGE = "rgba(255, 255, 255, .38)";
const SHADOW = "0 2px 6px rgba(0, 0, 0, .35)";

export const PAD_LOOK = {
    face: ({ pressed, selected, diagonal }) => (diagonal
        ? { color: pressed ? "#fff" : "rgba(255, 255, 255, .6)" }
        : {
            borderRadius: "50%",
            color: pressed ? "#fff" : "rgba(255, 255, 255, .88)",
            background: pressed ? ACCENT(85) : DISC,
            border: selected ? "2px solid var(--accent-2)" : `1px solid ${pressed ? "var(--accent-2)" : EDGE}`,
            boxShadow: SHADOW,
            textShadow: "0 1px 2px rgba(0, 0, 0, .5)",
        }),
    // Le pavé tactile (TouchpadSurface.jsx) : le même disque et le même liseré qu'un bouton, l'appui en indigo.
    surface: ({ touched }) => ({
        background: touched ? ACCENT(55) : DISC,
        border: `1px solid ${touched ? "var(--accent-2)" : EDGE}`,
        boxShadow: SHADOW,
    }),
    glyph,
    thumb: (dragging) => (dragging ? ACCENT(80) : "rgba(255, 255, 255, .28)"),
};
