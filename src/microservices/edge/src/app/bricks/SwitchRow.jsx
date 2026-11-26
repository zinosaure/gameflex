import React from "react";
import { useTranslation } from "react-i18next";
import { fieldContext, Form } from "../ui/Forms.jsx";
import { helpLines, SUBTITLES } from "../globals/subtitles.jsx";
import { InFilters } from "../globals/filters.js";

// UNE LIGNE QUI SE RÈGLE SUR PLACE : son libellé sur toute la largeur, puis, dessous, sa description à gauche et
// l'interrupteur de Skeleton au bord droit (le porteur, 2026-11-25 : « pour skeletonSwitch, le titre soit à 100% et que
// les textes et switch soient juste en dessous ») — Form.SkeletonSwitch (« les switches par skeletonSwitch »), ses textes dedans —
// la paire de la ligne (`texts`, SWITCH_TEXTS ; « Oui » / « Non » par défaut) —, à la largeur `md` de
// Skeleton, la même pour toutes les lignes (« Désactivé » y tient). Toucher la ligne le bascule aussi. À la manette, la
// lumière est sur l'interrupteur, A le bascule. PAS D'ICÔNE : un interrupteur est un contrôle, son groupe est donc
// toujours un formulaire (Rows), et « évite de mettre l'icône à gauche quand c'est un Switch dans un formulaire » (le
// porteur, 2026-11-14). Désactivée : grisée, sa raison dans `help` (chantier 81 : jamais cachée).
// LES TEXTES D'UN INTERRUPTEUR DÉPENDENT DE SON LIBELLÉ (le porteur, 2026-11-25 : « C'est mixte, ça va dépendre du label.
// Car on peut avoir yes/no, on/off, enabled/disabled, normal/fullscreen, etc. ») — les paires, écrites ICI une fois, que
// chaque ligne nomme (`texts`) :
//   yesNo       Oui / Non — une question, une préférence (par défaut) ;
//   enabled     Activé / Désactivé — une fonction qu'on allume (les sons, les vibrations, un journal) ;
//   shown       Affiché / Caché, shownF au féminin (la manette à l'écran) ;
//   allowed     Autorisé / Refusé — un droit (l'accès à une console, l'administration à distance) ;
//   open        Ouvert / Coupé — un son, un micro ;
//   fullscreen  Plein écran / Normal.
export const SWITCH_TEXTS = { yesNo: ["on", "off"], enabled: ["enabled", "disabled"], shown: ["shown", "hidden"], shownF: ["shownF", "hiddenF"],
    allowed: ["allowed", "refused"], open: ["open", "muted"], fullscreen: ["fullscreen", "normal"] };

export function SwitchRow({ id, label, help = null, checked, disabled = false, texts = null, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey, onChange }) {
    const { t } = useTranslation();
    const [onText, offText] = Array.isArray(texts) ? texts : (SWITCH_TEXTS[texts] || SWITCH_TEXTS.yesNo).map((key) => t(`switches.${key}`));
    // SANS DESCRIPTION, LE TITRE EN PREND LA PLACE (le porteur, 2026-11-25 : « forcer le help-text (sous-titres) ; si pas
    // de help alors c'est le titre qui remplace ») : une seule ligne, le titre à gauche, l'interrupteur à droite.
    const lines = React.useContext(InFilters) ? null : helpLines(help, Infinity, separator, noKey);
    const flip = (event) => {
        if (!disabled && !event.target.closest(".form-switch-pill, label")) {
            onChange(!checked);
        }
    };

    return (
        <div className={`list-group-item gf-row gf-row-switch ${disabled ? "opacity-50" : ""}`} onClick={flip}>
            {lines && <span className="gf-row-label navrow-title">{label}</span>}
            <span className="gf-row-switch-line">
                {lines ? <small className="gf-row-help">{lines}</small> : <span className="gf-row-label navrow-title">{label}</span>}
                <Form.SkeletonSwitch name={id} context={fieldContext(id, Boolean(checked), (on) => onChange(Boolean(on)), disabled)} reverse variant="md"
                    onText={onText} offText={offText} props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
            </span>
        </div>
    );
}
