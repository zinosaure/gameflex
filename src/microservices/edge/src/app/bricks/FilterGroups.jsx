import React from "react";
import { useTranslation } from "react-i18next";
import { GAME_STATUSES } from "../libs/format.js";
import { InFilters } from "../globals/filters.js";
import { RangeRow } from "./RangeRow.jsx";
import { Rows } from "./Rows.jsx";
import { Section } from "./Section.jsx";
import { SwitchRow } from "./SwitchRow.jsx";

// LES GROUPES DE FILTRES (le porteur, 2026-11-23 : « Tous les filtres doivent avoir des sections avec des titres comme
// celui des consoles dans le frontend ») : chaque groupe sa section, sous son titre — le panneau des filtres d'une liste
// (Listing) comme celui de la bibliothèque (Library.jsx). `groups` : `[{ key, title, help?, rows }]` ; un groupe faux est
// sauté.
export function FilterGroups({ groups }) {
    return groups.filter(Boolean).map((group) => (
        <Section key={group.key} title={group.title} help={group.help || null}>
            <InFilters.Provider value={true}>
                <Rows>{group.rows}</Rows>
            </InFilters.Provider>
        </Section>
    ));
}

// UN GROUPE D'INTERRUPTEURS, tous allumés au départ (les sources, les statuts, les consoles, les niveaux d'un journal) :
// un par choix (`choices` : `[{ value, label, help? }]`, l'identifiant de chaque ligne `<id>-<value>`) ; en éteindre un
// écarte sa valeur (`hidden`, `onChange(value, on)`). Un seul choix ne filtre rien : pas de groupe.
// EN TÊTE, « TOUT » (le porteur, 2026-11-25 : « rajouter un switch qui permet de sélectionner/désélectionner le tout ») :
// allumé quand tous le sont ; le basculer allume ou éteint tout le groupe d'un geste (`onChange(valeurs, on)`, que
// toggleHidden, globals/filters.js, prend comme une seule).
function AllSwitch({ id, choices, hidden, onChange }) {
    const { t } = useTranslation();
    const values = choices.map((choice) => choice.value);

    return <SwitchRow id={`${id}-all`} label={t("filters.all")} checked={values.every((value) => !hidden.includes(value))} onChange={(on) => onChange(values, on)} />;
}

export function switchGroup({ key, title, help = null, id, choices, hidden, onChange }) {
    return choices.length > 1 && {
        key, title, help, rows: (
            <>
                <AllSwitch id={id} choices={choices} hidden={hidden} onChange={onChange} />
                {choices.map((choice) => (
                    <SwitchRow key={choice.value} id={`${id}-${choice.value}`} label={choice.label} help={choice.help || null} checked={!hidden.includes(choice.value)}
                        onChange={(on) => onChange(choice.value, on)} />
                ))}
            </>
        ),
    };
}

// LA TAILLE, le groupe de filtres des listes de jeux des Réglages (le Catalogue, les jeux d'une console) : une taille
// minimum en Mo (`?minSize=`, 0 : tous) et « Les plus lourds d'abord » (`?sort=size`).
export const sizeGroup = ({ t, id, minSize, onMinSize, heaviest, onHeaviest }) => ({
    key: "size", title: t("catalog.filters.size"), rows: (
        <>
            <RangeRow id={`${id}-min-size`} min={0} max={1000} step={10} label={t("gameAdmin.minSize")} help={t("gameAdmin.minSizeHelp")}
                value={minSize} onChange={(value) => onMinSize(Number(value) > 0 ? Math.round(Number(value)) : null)} />
            <SwitchRow id={`${id}-heaviest`} label={t("gameAdmin.heaviest")} help={t("gameAdmin.heaviestHelp")} checked={heaviest} onChange={onHeaviest} />
        </>
    ),
});

// LES STATUTS D'UN JEU, le groupe de filtres de toute liste de jeux (la bibliothèque, le Catalogue, les jeux d'une console).
export const statusGroup = ({ t, id, hidden, onChange }) => switchGroup({
    key: "statuses", title: t("games.status"), id, hidden, onChange, choices: GAME_STATUSES.map((status) => ({ value: status, label: t(`status.${status}`) })),
});
