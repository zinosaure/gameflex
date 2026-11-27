import React from "react";
import { helpLines, isFact, roomAfter, SUBTITLES } from "../globals/subtitles.jsx";
import { linesOf } from "../globals/subtitles.jsx";
import { useInListing } from "../globals/listing.js";
import { useInSettings } from "../globals/backoffice.js";
import { isStateTone, Pill } from "./Pill.jsx";

// DANS UN LISTING, LE SOUS-TITRE EST EN PASTILLES (le porteur, 2026-11-25 : « Au lieu de mettre les sous-titres des éléments
// du listing en "a • b" tu peux les mettre en 2 pastilles à la place ? La couleur par défaut c'est le primary mais si par
// exemple c'est une information comme status, ou autres alors ça prend des couleurs ») : la valeur et l'aide, deux au plus,
// chacune sa pastille (Pill) — bleue, ou celle que l'appelant donne (`pill(texte, tone)` : un statut, installé, manquant).
// Une phrase (une raison, un refus) garde sa ligne de texte, sous les pastilles.
const LISTED_MAX = 2;
const isPhrase = (item) => typeof item === "string" && item.length > 40;

function ListedLine({ items }) {
    const shown = items.slice(0, LISTED_MAX);
    const pills = shown.filter((item) => !isPhrase(item));
    const phrases = shown.filter(isPhrase);

    return (
        <>
            {pills.length > 0 && (
                <span className="gf-row-pills">
                    {pills.map((item, index) => (React.isValidElement(item) ? (item.type === Pill ? React.cloneElement(item, { key: index, index }) : <React.Fragment key={index}>{item}</React.Fragment>)
                        : <Pill key={index} index={index}>{isFact(item) ? item.value : item}</Pill>))}
                </span>
            )}
            {phrases.map((item, index) => <small key={index} className="gf-row-help d-block">{item}</small>)}
        </>
    );
}

// Le texte d'une ligne : son libellé, sa valeur DESSOUS (comme une ligne d'informations, InfoRow ; le porteur, 2026-11-16 :
// « Le lien Vidéo n'est pas affiché comme sur 2 lignes comme on s'est dit »), puis son aide.
// `pill` : la valeur en pastille, sur sa ligne, au-dessus de l'aide (Pill : success, danger, info) — une ligne qui ouvre
// une page (NavRow) l'a toujours.
export function RowText({ label, value = null, help, checked = false, max = Infinity, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey, pill = null, children = null }) {
    // Avec un séparateur, la valeur et l'aide ne font qu'UNE ligne de sous-titre, la valeur d'abord.
    // Hors des Réglages, un listing garde son sous-titre en texte, « a • b » (globals/backoffice.js). Une ligne qui demande
    // une information par ligne (`separator` à null : l'historique d'un statut) garde sa forme, même dans un listing.
    const inSettings = useInSettings();
    const listed = useInListing() && inSettings && separator !== null;
    const joined = separator !== null;
    const items = listed ? [...linesOf(value), ...linesOf(help)].filter((item) => !isFact(item) || (item.value !== "" && item.value !== null && item.value !== undefined)) : [];
    const pills = pill && !listed && (inSettings || isStateTone(pill)) ? linesOf(value).map((item) => (isFact(item) ? item.value : item)).filter((item) => item !== null && item !== undefined && item !== "") : [];
    const lines = pills.length ? helpLines(help, Math.max(0, max - 1), separator, noKey)
        : joined ? helpLines([...linesOf(value), ...linesOf(help)], max, separator, noKey) : helpLines(help, roomAfter(value, max), null, noKey);
    const shown = joined ? null : Array.isArray(value) || isFact(value) ? helpLines(value, max, null, noKey) : value;

    return (
        <span className="gf-row-text">
            <span className="gf-row-label navrow-title">
                {label}
                {/* CHOISI (le porteur, 2026-11-25 : « au lieu de mettre en bold et couleur primary, on mettra un petit icône
                    check en vert à côté du titre ») : l'émulateur en service, une console activée, un jeu déjà ici. */}
                {checked && <i className="bi bi-check-circle-fill text-success ms-2" role="img" aria-label="✓"></i>}
            </span>
            {listed ? items.length > 0 && <ListedLine items={items} /> : (
                <>
                    {pills.length > 0 && <span className="gf-row-pills">{pills.map((item, index) => <Pill key={index} tone={pill} index={index}>{item}</Pill>)}</span>}
                    {!pills.length && shown !== null && shown !== undefined && shown !== "" && <span className="gf-row-value text-wrap text-break">{shown}</span>}
                    {lines && <small className="gf-row-help">{lines}</small>}
                </>
            )}
            {children}
        </span>
    );
}
