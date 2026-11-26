import React from "react";
import { SUBTITLES } from "../globals/subtitles.jsx";
import { SQUARE_ICON_BUTTON, useButton } from "../libs/ui.js";
import { Chip } from "./Chip.jsx";
import { RowText } from "./RowText.jsx";
import { useInListing } from "../globals/listing.js";
import { useSenseTone } from "../globals/sections.js";

// UNE LIGNE QUI OUVRE, ET UN GESTE À SON BORD DROIT (le porteur, 2026-11-16, « Ma partie en cours » d'En direct : « Juste
// un élément avec le titre, le sous-titre […] Et à droite le bouton power off ») : la ligne d'une NavRow — ses classes, sa
// tête (`lead` ou la pastille), son texte, la lumière de la croix — sauf son bord droit, où se pose un bouton carré à icône
// seule (SQUARE_ICON_BUTTON, la règle des boutons : son nom en aria-label et en title). Un bouton dans un bouton n'existe
// pas : la ligne est une boîte de Bootstrap qui tient DEUX boutons côte à côte, la NavRow d'abord, qui prend la place et
// toute la hauteur (le voile de la manette la couvre de haut en bas, jusqu'au geste), puis le geste. Toucher la ligne fait `onOpen` ;
// toucher le bouton, `action.onClick`. À LA MANETTE : la croix atteint la ligne (A l'ouvre), DROITE le bouton (A agit),
// GAUCHE revient. Le bord droit étant le bouton, la ligne n'y dit rien (ROW_ENDS). `action` : { id, icon, label, kind
// (la sorte du bouton, useButton), pending (le tourniquet à sa place), disabled, onClick }.
export function SplitRow({ id, icon = "bi-chevron-right", tone = null, lead = null, label, help = null, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey, onOpen, action }) {
    tone = useSenseTone(tone);
    const listed = useInListing();
    const button = useButton();

    return (
        <div className="list-group-item gf-row p-0 pe-3">
            <button type="button" className="list-group-item list-group-item-action gf-row navrow align-self-stretch min-w-0" data-nav data-key={id} data-opens="action" onClick={onOpen}>
                {lead || (!listed && <Chip icon={icon} tone={tone} />)}
                <RowText label={label} help={help} separator={separator} noKey={noKey} />
            </button>
            <button type="button" className={`${SQUARE_ICON_BUTTON} ${button(action.kind || "other")}`} data-nav data-key={action.id} disabled={action.disabled || action.pending}
                aria-label={action.label} title={action.label} onClick={action.onClick}>
                {action.pending ? <span className="spinner-border spinner-border-sm" role="status"></span> : <i className={`bi ${action.icon}`}></i>}
            </button>
        </div>
    );
}
