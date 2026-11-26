import React from "react";
import { helpLines, isFact, linesOf, SUBTITLES } from "../globals/subtitles.jsx";

// Au-delà, une phrase : elle garde sa ligne de texte (comme dans un listing, RowText).
const PHRASE = 40;
import { Chip } from "./Chip.jsx";
import { DEFAULT_PILL, isStateTone, Pill } from "./Pill.jsx";
import { useInSettings } from "../globals/backoffice.js";
import { useInListing } from "../globals/listing.js";
import { useSenseTone } from "../globals/sections.js";

// UNE LIGNE DE RENSEIGNEMENT, comme « À propos du téléphone » d'Android (le porteur, 2026-11-16 : « tu peux remplacer le
// style des fiches pour les mettre comme sur l'image. Au lieu de les mettre en 2 colonnes. » ; « on garde le même ui on a
// déjà, c'est juste la disposition qui change ») : sa pastille, son libellé, et SA VALEUR SOUS LE LIBELLÉ, à gauche, sur
// toute la largeur qui reste — une valeur longue (un chemin, une adresse, l'éditeur d'un jeu) passe à la ligne sans rien
// pousser, un badge (« Jouable ») ou des touches (les raccourcis) s'y rangent de même. Aucun geste. Une seule disposition :
// aucune page ne remet la valeur au bord droit. `mono` : la valeur en chasse fixe, plus petite (un chemin, un fichier,
// l'heure d'une ligne de journal). `help` : ce que la ligne désigne, sous sa valeur, plus petit, comme l'aide d'une NavRow — un service de métadonnées sans compte,
// qui n'a pas de page à ouvrir (chantier 53, P10), un statut et qui l'a posé.
// `mono` : sans effet — une valeur a la police de toutes les autres (le porteur, 2026-11-25 : « le font pour le texte
// "fichier" est différent ») ; un chemin se coupe où il faut (text-break).
// `pill` : la couleur de ses pastilles (Pill : success, danger, info…) — un état (actif, installé, manquant).
// UNE VALEUR EST UNE PASTILLE (le porteur, 2026-11-25 : « Les sous-titres avec valeur n'est pas en pastille. Ça doit être
// pareille pour tous les sous-titres qui sont des valeurs (donnée dynamique) ») : chaque valeur (un tableau en donne
// plusieurs, sur une ligne) a sa pastille, primary puis secondary par défaut (Pill). Une phrase (plus de 40 caractères : une raison, une
// explication) ou un élément déjà composé (un badge, des touches) garde sa forme. `pill={false}` : du texte, exprès.
export function InfoRow({ icon = null, tone = null, lead = null, label, value: given, help = null, mono = false, pill = DEFAULT_PILL, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey }) {
    tone = useSenseTone(tone);
    const listed = useInListing();
    const items = linesOf(given).map((item) => (isFact(item) ? item.value : item));
    // Hors des Réglages, une valeur reste du texte ; un état coloré garde sa pastille (globals/backoffice.js).
    const inSettings = useInSettings();
    const pills = pill !== false && (inSettings || isStateTone(pill)) && items.length > 0
        && items.every((item) => (typeof item === "string" || typeof item === "number") && String(item).length <= PHRASE) ? items : null;
    const value = Array.isArray(given) || isFact(given) ? helpLines(given, Infinity, separator, noKey) : given;

    if (value === null || value === undefined || value === "") {
        return null;
    }

    return (
        <div className="list-group-item gf-row">
            {lead || (icon && !listed && <Chip icon={icon} tone={tone} />)}
            <span className="gf-row-text">
                <span className="gf-row-label">{label}</span>
                {pills ? <span className="gf-row-pills">{pills.map((item, index) => <Pill key={index} tone={pill} index={index}>{String(item)}</Pill>)}</span>
                    : <span className="gf-row-value text-wrap text-break">{value}</span>}
                {helpLines(help, Infinity, separator, noKey) && <small className="gf-row-help">{helpLines(help, Infinity, separator, noKey)}</small>}
            </span>
        </div>
    );
}
