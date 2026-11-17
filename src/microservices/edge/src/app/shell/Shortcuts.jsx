import React from "react";
import { useTranslation } from "react-i18next";
import { GLYPHS, KEYBOARD_EN } from "../libs/glyphs.js";
import SidePanel from "./SidePanel.jsx";
import { InfoRow, Note, Rows, Section } from "../ui/Bricks.jsx";

// LES RACCOURCIS (le porteur, 2026-11-12) : le bouton manette de l'en-tête ouvre ce
// panneau, qui dit chaque geste avec la touche de l'entrée en cours — la famille de
// la manette ou le clavier (la table de libs/glyphs.js), recalculée à chaque entrée.
// Le contenu suit libs/input.js (la table des touches, ce que fait chaque écran).

// Les sections : [titre, [[gestes, libellé]]] ; un geste est une clé de la table
// (`a`, `b`, `x`, `y`, `l2`, `r2`, `select`, `start`) ou une croix (`ud`, `lr`, `dpad`).
const SECTIONS = [
    ["shortcuts.global", [[["l2", "r2"], "shortcuts.tabs"], [["start"], "shortcuts.home"], [["select"], "shortcuts.settings"], [["dpad"], "shortcuts.move"]]],
    ["shortcuts.consoles", [[["ud"], "shortcuts.console"], [["a"], "shortcuts.library"]]],
    ["shortcuts.sheet", [[["ud"], "shortcuts.game"], [["a"], "shortcuts.play"], [["y"], "shortcuts.gameSettings"], [["x"], "shortcuts.search"], [["b"], "shortcuts.back"]]],
    ["shortcuts.panels", [[["dpad"], "shortcuts.panelMove"], [["lr"], "shortcuts.select"], [["a"], "shortcuts.activate"], [["b"], "shortcuts.close"]]],
    // En partie : L2 + Home tenus trois secondes ; au clavier, Échap (libs/play.js, PlayShell.jsx).
    ["shortcuts.game", [[["menu"], "shortcuts.gameMenu"]]],
];

// La croix : des flèches pour tous (le clavier a les mêmes).
const DIRECTIONS = { ud: "▲ ▼", lr: "◀ ▶", dpad: "✚" };

export default function Shortcuts({ kind, onClose }) {
    const { t, i18n } = useTranslation();
    const glyph = (action) => {
        if (action === "menu") {
            return kind === "keyboard" ? glyph("b") : `${GLYPHS[kind].l2} + ${GLYPHS[kind].home}`;
        }
        if (DIRECTIONS[action]) {
            return kind === "keyboard" && action === "dpad" ? "← ↑ → ↓" : DIRECTIONS[action];
        }
        const text = GLYPHS[kind][action];

        return kind === "keyboard" && i18n.language === "en" ? KEYBOARD_EN[text] || text : text;
    };

    return (
        <SidePanel label={t("shortcuts.title")} className="shortcuts" firstKey="p-close" onClose={onClose}>
            <Note>{t(`shortcuts.family.${kind}`)}</Note>
            {/* En direction C (Bricks.jsx) : un groupe par famille de gestes, chaque ligne dit ce qu'elle fait, ses
                touches dessous, à gauche, comme toute valeur d'une InfoRow — elles se rangent à la suite et passent à la
                ligne plutôt que de sortir du cadre. */}
            {SECTIONS.map(([title, rows]) => (
                <Section key={title} title={t(title)}>
                    <Rows>
                        {rows.map(([actions, label]) => (
                            <InfoRow key={label} label={t(label)} value={(
                                <span className="d-flex flex-wrap gap-1">{actions.map((action) => <span key={action} className={`badge text-bg-secondary ${kind === "playstation" ? "fs-6" : ""}`}>{glyph(action)}</span>)}</span>
                            )} />
                        ))}
                    </Rows>
                </Section>
            ))}
        </SidePanel>
    );
}
