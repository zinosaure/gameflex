import React from "react";
import { useTranslation } from "react-i18next";

// UN SOUS-TITRE, UNE INFORMATION PAR LIGNE (le porteur, 2026-11-23 : « remplacer tous les sous-titres qui utilisent le
// · comme séparateur pour les mettre sur plusieurs lignes ») : `help` est un texte, ou une liste dont chaque élément non
// vide a sa ligne — plus jamais « a · b · c » : une page passe la liste (des listes dans la liste s'aplatissent). Celui des
// lignes (RowText, InfoRow), des cartes d'un choix (RadioRow) et d'un téléchargement (DownloadRow) ; LA VALEUR d'une ligne
// suit la même règle.
// DEUX LIGNES AU PLUS POUR UNE LIGNE QUI MÈNE À UNE PAGE (le porteur, 2026-11-23 : « pour tous les sous-titres, mets que
// 2 sous-titres max », puis « Normalement le max 2 sous-titres c'est quand c'est des éléments d'un listing qui mène vers
// une page. Si pas de page ou de fiche après alors tu fais l'exception ») : une NavRow qui ouvre une page (`opens` :
// "page") montre les deux premières informations qu'on lui passe — la page les range de la plus utile à la moins utile,
// le reste est dans la fiche (Détails). Une ligne qui ne mène nulle part (un renseignement, une action, un lien, une
// question) les montre toutes. La valeur et l'aide se partagent la limite, la valeur d'abord. Posée ici, une fois.
export const MAX_LINES = 2;

export const linesOf = (items) => (Array.isArray(items) ? items.flat(Infinity) : [items]).filter((line) => line !== null && line !== undefined && line !== false && line !== "");

export const roomAfter = (value, max) => Math.max(0, max - Math.min(linesOf(value).length, max));

// LE SÉPARATEUR (le porteur, 2026-11-23 : « rajouter un argument separator qui par défaut null. S'il a une valeur alors tous
// les sous-titres passent en mode une seule ligne et minuscule […] Et si null alors c'est un saut de ligne ») : `separator`
// à null, une information par ligne ; « • », « - »… : toutes sur UNE ligne, jointes par lui, chaque clé commençant par
// une minuscule (« nombre de jeux : 566 jeux • console : PlayStation 1 ») ; une phrase garde sa ligne. Toutes les briques qui portent un sous-titre le
// prennent (NavRow, SplitRow, SwitchRow, InfoRow, RadioRow, DownloadRow, QuickTiles) ; une ligne le fond avec sa valeur.
// SANS LES CLÉS (le porteur, 2026-11-23 : « rajoute aussi l'argument noKey, qui par défaut c'est false. Et si c'est true
// alors on affiche que les valeurs ») : `noKey` à true, un `fact` ne montre que sa valeur (« PlayStation 1 », « 711 Mo »),
// une par ligne, ou sur une ligne avec un séparateur (« PlayStation 1 • 711 Mo »). Les mêmes briques le prennent.
// LES SOUS-TITRES DE TOUT LE FRONTEND, PAR DÉFAUT (le porteur, 2026-11-23 : « Tu peux ensuite mettre tout en noKey et
// separator • pour voir », puis « On garde comme définitif avec le • ») : les valeurs seules, sur une ligne, jointes par
// « • », pour toute brique qui ne dit rien. À null et false, ce serait une information « clé : valeur » par ligne. Une
// page qui veut autre chose (une fiche, InfoRow avec ses clés) le passe elle-même.
export const SUBTITLES = { separator: "•", noKey: true };

export const helpLines = (help, max = Infinity, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey) => {
    const lines = linesOf(help).slice(0, max);

    if (lines.length === 0) {
        return null;
    }

    if (separator === null) {
        return lines.map((line, index) => (isFact(line) ? <FactLine key={index} label={line.label} value={line.value} noKey={noKey} /> : <span key={index} className="d-block">{line}</span>));
    }

    // LES PHRASES GARDENT LEUR LIGNE (le porteur, 2026-11-23 : « les textes info tu les mets toujours en retour de la ligne ») :
    // les « clé : valeur » qui se suivent se joignent par le séparateur ; une phrase (une aide, une raison) a sa ligne à elle.
    const groups = lines.reduce((all, line) => {
        const last = all[all.length - 1];
        if (isFact(line) && last?.facts) {
            last.items.push(line);
        } else {
            all.push({ facts: isFact(line), items: [line] });
        }

        return all;
    }, []);

    return groups.map((group, index) => (
        <span key={index} className="d-block">
            {group.facts ? group.items.map((line, position) => (
                <React.Fragment key={position}>
                    {position > 0 && ` ${separator} `}
                    <FactLine label={line.label} value={line.value} inline noKey={noKey} />
                </React.Fragment>
            )) : group.items[0]}
        </span>
    ));
};

// UNE INFORMATION EN « CLÉ : VALEUR » (le porteur, 2026-11-23 : « je te propose de les mettre en key : value […] que sur les
// sous-titres dont le titre n'est pas déjà une clé » ; « ça ne sert à rien de mettre No. of games: {value} games » ; « la
// valeur des sous-titres en gras », puis « au lieu de mettre les valeurs en gras, mets plutôt la clé en gras ») : un
// élément d'un sous-titre, `fact(clé, valeur)` — la clé en gras, la valeur seule, sans répéter la clé (« Nombre de jeux :
// 494 ») ; rien
// quand elle est vide. Une ligne d'informations (InfoRow : son libellé EST la clé) n'en a pas besoin. La ponctuation est
// celle de la langue (`colon` : « : » précédé d'une espace en français). `fact` vit dans libs/format.js (les bibliothèques
// sans JSX s'en servent aussi : downloadState).
export const isFact = (line) => line !== null && typeof line === "object" && !Array.isArray(line) && line.$$typeof === undefined && "label" in line && "value" in line;

// Une clé qui commence par une minuscule, pour le mode une ligne (« Adresse IP » → « adresse IP »).
export const lowerFirst = (text) => (typeof text === "string" && text ? text.charAt(0).toLocaleLowerCase() + text.slice(1) : text);

export function FactLine({ label, value, inline = false, noKey = false }) {
    const { t } = useTranslation();

    return (
        <span className={`${inline ? "" : "d-block "}gf-fact`}>
            {!noKey && <><span className="fw-bold">{inline ? lowerFirst(label) : label}</span>{t("colon")}</>}{value}
        </span>
    );
}
