import React from "react";
import { useTranslation } from "react-i18next";
import { GLYPHS, KEYBOARD_EN } from "../libs/glyphs.js";
import SidePanel from "./SidePanel.jsx";
import { InfoRow } from "../bricks/InfoRow.jsx";
import { Note } from "../bricks/Note.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";

// LES RACCOURCIS (le porteur, 2026-11-12) : le bouton manette de l'en-tête ouvre ce
// panneau, qui dit chaque geste avec la touche de l'entrée en cours — la famille de
// la manette ou le clavier (la table de libs/glyphs.js), recalculée à chaque entrée.
// Le contenu suit libs/input.js (la table des touches, ce que fait chaque écran).

// Les sections : [titre, [[gestes, libellé]]] ; un geste est une clé de la table
// (`a`, `b`, `x`, `y`, `l2`, `r2`, `select`, `start`) ou une croix (`ud`, `lr`, `dpad`).
const SECTIONS = [
    ["shortcuts.global", [[["l2", "r2"], "shortcuts.tabs"], [["start"], "shortcuts.home"], [["select"], "shortcuts.settings"], [["dpad"], "shortcuts.move"]]],
    ["shortcuts.consoles", [[["ud"], "shortcuts.console"], [["a"], "shortcuts.library"]]],
    ["shortcuts.sheet", [[["ud"], "shortcuts.gameNext"], [["a"], "shortcuts.play"], [["y"], "shortcuts.gameSettings"], [["x"], "shortcuts.search"], [["b"], "shortcuts.back"]]],
    ["shortcuts.panels", [[["dpad"], "shortcuts.move"], [["lr"], "shortcuts.select"], [["a"], "shortcuts.activate"], [["b"], "shortcuts.close"]]],
    // En partie (libs/gameInput.js, menuGestureOf) : Home en rétro ; L2 + R2 + Home tenus deux secondes ailleurs ; au
    // clavier, Échap partout.
    ["shortcuts.game", [[["menuHome"], "shortcuts.gameMenuRetro"], [["menuChord"], "shortcuts.gameMenuOthers"], [["menuKey"], "shortcuts.gameMenu"]]],
];

// UNE PASTILLE PAR TOUCHE, TOUTES PAREILLES (le porteur, 2026-11-26 : « mettre les pastilles des touches cohérentes et
// une par une ») : la croix en ses flèches, les mêmes au clavier et à la manette ; une combinaison (le menu d'une partie)
// en ses touches, liées par « + ».
const DIRECTIONS = { ud: ["▲", "▼"], lr: ["◀", "▶"], dpad: ["▲", "▼", "◀", "▶"] };

export default function Shortcuts({ kind, onClose }) {
    const { t, i18n } = useTranslation();
    const key = (action) => {
        const text = GLYPHS[kind][action];

        return kind === "keyboard" && i18n.language === "en" ? KEYBOARD_EN[text] || text : text;
    };
    // Les touches d'un geste, et si elles se tiennent ensemble.
    const keysOf = (action) => {
        if (action === "menuHome") {
            return { keys: [GLYPHS[kind].home] };
        }
        if (action === "menuChord") {
            return { keys: [GLYPHS[kind].l2, GLYPHS[kind].r2, GLYPHS[kind].home], together: true };
        }
        if (action === "menuKey") {
            return { keys: [key("b")] };
        }

        return { keys: DIRECTIONS[action] || [key(action)] };
    };
    const pills = (actions) => actions.flatMap((action) => {
        const { keys, together } = keysOf(action);

        return keys.flatMap((text, index) => [
            together && index > 0 ? <span key={`${action}-plus-${index}`} className="align-self-center">+</span> : null,
            <span key={`${action}-${index}`} className="badge text-bg-secondary fs-6 gf-key">{text}</span>,
        ]).filter(Boolean);
    });

    return (
        <SidePanel label={t("shortcuts.title")} className="shortcuts" firstKey="p-close" onClose={onClose}>
            <Note>{t(`shortcuts.family.${kind}`)}</Note>
            {/* En direction C (bricks/) : un groupe par famille de gestes, chaque ligne dit ce qu'elle fait, ses
                touches dessous, à gauche, comme toute valeur d'une InfoRow — elles se rangent à la suite et passent à la
                ligne plutôt que de sortir du cadre. */}
            {SECTIONS.map(([title, rows]) => (
                <Section key={title} title={t(title)}>
                    <Rows>
                        {rows.filter(([actions]) => (kind === "keyboard" ? !["menuHome", "menuChord"].includes(actions[0]) : actions[0] !== "menuKey")).map(([actions, label]) => (
                            <InfoRow key={label} label={t(label)} value={(
                                <span className="d-flex flex-wrap gap-1">{pills(actions)}</span>
                            )} />
                        ))}
                    </Rows>
                </Section>
            ))}
        </SidePanel>
    );
}
