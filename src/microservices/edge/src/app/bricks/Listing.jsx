import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { focusElement } from "../libs/input.js";
import { useButton } from "../libs/ui.js";
import SidePanel from "../shell/SidePanel.jsx";
import { Form, PadReach, usePagination } from "../ui/Forms.jsx";
import { BlockRow } from "./BlockRow.jsx";
import { EmptyRow } from "./EmptyRow.jsx";
import { FilterGroups } from "./FilterGroups.jsx";
import { InListing } from "../globals/listing.js";
import { Note } from "./Note.jsx";
import { PlaceholderRow } from "./PlaceholderRow.jsx";
import { Rows } from "./Rows.jsx";

// UNE RECHERCHE DANS UNE LISTE DÉJÀ LUE (chantier 81, la passe de vérification ; le porteur, 2026-11-25 : « certains listing
// n'ont pas de search bar ») — écrite ICI une fois pour les listings qui filtrent ce qu'ils ont : `textOf(item)` dit ce qui
// se cherche, sans accents ni majuscules. Rend { search, onSearch, found } pour <Listing>.
export const foldText = (text) => String(text ?? "").normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();

export function useListSearch(items, textOf) {
    const [search, setSearch] = useState("");
    const needle = foldText(search.trim());
    const list = items || [];
    const found = needle ? list.filter((item) => foldText(textOf(item)).includes(needle)) : list;

    return { search, onSearch: setSearch, found };
}

// UNE LISTE QU'ON CHERCHE ET QU'ON FEUILLETTE (10 par page, `pageSize`), comme ListingElement de la webapp : la
// recherche de Skeleton (Form.SearchText) au bord gauche, les filtres (des lignes, dans leur groupe), le nombre
// trouvé, les lignes dans leur groupe, puis la pagination de Skeleton (usePagination). `page` et `onPage` : la page
// tenue par l'appelant (dans l'adresse), pour la retrouver au retour d'une ligne ouverte — la lumière y revient ;
// sinon la liste la tient. `total` : une liste que le SERVEUR feuillette (les membres) — `items` est alors la page
// lue, et `total` le nombre de tous.
//
// TOUTE LISTE LUE EN ASYNCHRONE DES RÉGLAGES EN EST UNE (le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») — les émulateurs et les BIOS d'une console, les sources, les sauvegardes, les disques, les
// consoles Remote Play… Celle qu'on ne cherche pas n'a pas de recherche (`onSearch` absent), et celle qui ne se compte
// pas, pas de nombre (`found` absent) : elle n'est que ses lignes, et sa pagination dès qu'elle dépasse une page.
// `head` : des lignes en tête de chaque page, dans le même groupe, qui ne comptent pas dans la page (« Toutes ses
// sauvegardes », « Cette machine »).
//
// PENDANT QU'ELLE SE LIT (`loading`), DES LIGNES FANTÔMES (le porteur, 2026-11-14 : « il faut aussi utiliser ces
// placeholder pendant que ça charge. Un placeholder du même nombre d'éléments du ITEM PER PAGE. Skeleton.Table propose
// déjà le placeholder. ») — la règle de la Table de Skeleton (handlePendingState), écrite ICI une fois :
//   - autant que les lignes qu'elle montrait (elles se changent en fantômes : une page relue, une recherche refaite),
//     sinon autant qu'une page (la première lecture) ;
//   - de la forme de ses vraies lignes : PlaceholderRow, ou celle que la liste donne (`placeholderRow`, une fonction
//     de l'index : des jaquettes, des vignettes, des renseignements) ;
//   - la pagination reste à l'écran, inerte (`isPending`, et `inert` : la croix ne s'y pose pas, ni sur un fantôme).
//
// VIDE, ELLE GARDE SA BOÎTE (le porteur, 2026-11-14 : « Pour toutes les listes vides, il faut afficher au moins un lien
// (un placeholder) grisé avec le message ») : une ligne grisée (EmptyRow) — sous ses lignes de tête s'il y en a — qui dit
// `empty` (« Aucun jeu. ») ; quand une recherche n'a rien trouvé, la loupe et `noMatch`, sinon « Rien ne correspond à
// « … ». » (`null` : aucune ligne, la liste dit autrement pourquoi). Le nombre trouvé ne se dit pas : la ligne le dit.
//
// SES FILTRES (`filters` : leurs lignes, des contrôles — interrupteurs, sélecteurs, la période d'un journal) NE SONT
// JAMAIS SOUS LA RECHERCHE (le porteur, 2026-11-14 : « Remplace le modal filtre par un offcanvas, c'est plus simple pour
// la navigation » ; « Dans logs, rajoute le bouton filtre avec le search bar et met les calendriers dans le offcanvas du
// filtre » ; « Oui, c'est mieux de faire partout comme ça ») : comme ListingElement de la webapp, un bouton à icône
// accolé à la recherche, dans son input-group, ouvre le panneau latéral (SidePanel, l'offcanvas du launcher) qui les
// tient — un formulaire, sans traits ; ses groupes de filtres (`filters` : `[{ key, title, help?, rows }]`, FilterGroups)
// y ont chacun leur section, sous leur titre ; le panneau s'appelle « Filtres », partout. La pastille du bouton dit combien
// sont posés (`filterCount`) : un bouton enfoncé ne change pas de couleur (libs/ui.js). Ils s'appliquent à mesure, et chaque page les garde dans son adresse. À la manette, la lumière entre dans le panneau sur
// son premier filtre, B le referme et la rend au bouton ; au doigt, son fond le referme. Le panneau n'est pas une page :
// un filtre posé sur la liste.
export function Listing({ items, render, loading = false, search = "", onSearch = null, placeholder = "", found = null, empty = null, noMatch = undefined, filters = null, filterCount = 0, head = null, pageSize = 10, page = null, onPage = null, total = null, placeholderRow = null, searchKey = "sc-search" }) {
    const { t } = useTranslation();
    const [ownPage, setOwnPage] = useState(1);
    const pager = useRef(null);
    const pressed = useRef(-1);
    const lastShown = useRef(0);
    const served = total !== null;
    const pages = Math.max(1, Math.ceil((served ? total : items.length) / pageSize));
    const shown = Math.min(Math.max(1, Number(page ?? ownPage) || 1), pages);
    const setPage = (next) => (onPage || setOwnPage)(next);
    const rows = served ? items : items.slice((shown - 1) * pageSize, shown * pageSize);
    const ghosts = !loading ? 0 : rows.length > 0 ? rows.length : lastShown.current > 0 ? lastShown.current : pageSize;
    // La liste suit la recherche et sa ligne (puis un trait), ou elle est le premier contenu de son groupe : pas d'écart
    // au-dessus dans les deux cas.
    const groups = (filters || []).filter(Boolean);
    const searched = (search || "").trim();
    // LA RECHERCHE ET LES FILTRES, SEULEMENT AU-DELÀ D'UNE PAGE (le porteur, 2026-11-25 : « afficher la barre de recherche
    // que quand il y a plus d'une page en pagination […] ça dépend avant tout du nombre d'éléments venant du backend » ;
    // le bouton des filtres « suit la règle ») — et tant qu'une recherche est tapée ou un filtre posé : ce qu'ils ont
    // réduit à une page ne les retire pas sous la main.
    const searching = Boolean(onSearch || groups.length) && ((served ? total : items.length) > pageSize || searched !== "" || filterCount > 0);
    const first = "mt-0";
    const blank = !loading && !rows.length && !items.length;
    const blankText = !searched ? empty : noMatch === undefined ? t("settings.noMatch", { search: searched }) : noMatch;
    // Le nombre trouvé est le résultat de la recherche : une recherche cachée (une seule page) ne le dit pas.
    const note = loading || blank || ((onSearch || groups.length) && !searching) ? null : found;
    const button = useButton();
    const [filtering, setFiltering] = useState(false);
    const filterButton = useRef(null);
    const closeFilters = () => {
        setFiltering(false);
        requestAnimationFrame(() => filterButton.current?.isConnected && focusElement(filterButton.current, true));
    };
    const searchForm = {
        values: { q: search },
        setValues: (next) => {
            setPage(1);
            onSearch(next.q || "");
        },
        files: {},
        setFiles: () => {},
        rulesets: {},
    };
    const pagination = usePagination({
        isPending: loading,
        currentPosition: shown,
        itemCount: served ? total : items.length,
        itemsPerPage: pageSize,
        maxButtons: 5,
        // Skeleton redessine sa pagination à chaque page : la lumière revient au bouton qu'on vient d'appuyer.
        onPositionChange: (next) => {
            pressed.current = Array.from(pager.current?.querySelectorAll("button, select") || []).indexOf(document.activeElement);
            setPage(next);
        },
    });

    // Le panneau ouvert, la lumière va sur son premier filtre (le panneau la pose d'abord sur sa croix de fermeture).
    useEffect(() => {
        if (filtering) {
            requestAnimationFrame(() => focusElement(document.querySelector(".gf-filters .offcanvas-body [data-nav]"), true));
        }
    }, [filtering]);

    // Les lignes montrées, retenues : relue, la liste en fait autant de fantômes.
    useEffect(() => {
        if (!loading) {
            lastShown.current = rows.length;
        }
    });

    // La lumière rendue au bouton de la pagination qu'on vient d'appuyer. Une page que le serveur donne se LIT d'abord
    // (la pagination est inerte entre-temps, la lumière la quitte) : rendue quand la lecture finit.
    const wasLoading = useRef(loading);
    useEffect(() => {
        const settled = wasLoading.current && !loading;
        wasLoading.current = loading;

        if (loading || (served && !settled)) {
            return;
        }
        const controls = Array.from(pager.current?.querySelectorAll("button, select") || []);
        const target = controls[pressed.current];
        pressed.current = -1;

        if (target) {
            focusElement(target.disabled ? controls.find((control) => control.tagName === "SELECT") || target : target);
        }
    }, [shown, loading]);

    // LA RECHERCHE ET SON RÉSULTAT, PUIS UN TRAIT, PUIS LA LISTE (le porteur, 2026-11-14, chantier 53, P12 : « les search
    // bar rattaché à des listings doit être dans une section. La section aura donc le search bar (et des fois le bouton
    // filtre) et en dessous du search bar, le résultat de la recherche. Puis entre les 2 section, search bar et listing,
    // rajouter un hr pour séparer. ») — une fois, ici, pour toute liste qui se cherche ou se filtre : LA BOÎTE de la
    // recherche (`.gf-listing-search`, le conteneur arrondi des groupes : le champ, le bouton des filtres accolé, la ligne
    // « N trouvés » dessous — le porteur : « Le search bar n'a pas encore de section... »), un `hr`, puis la boîte des
    // lignes et leur pagination. Le titre du groupe (la Section de l'appelant) reste au-dessus de tout : il nomme la liste,
    // sa recherche comprise. Une liste sans recherche ne change pas : sa ligne de nombre, puis ses lignes.
    return (
        <>
            {searching && (
                <Rows className="gf-listing-search">
                    <BlockRow>
                        <div className={`gf-search ${groups.length && onSearch ? "input-group flex-nowrap" : ""}`}>
                            {groups.length > 0 && (
                                <button ref={filterButton} type="button" className={`${button("other")} position-relative flex-shrink-0`} data-nav data-key={`${searchKey}-filter`}
                                    aria-label={t("settings.filters")} title={t("settings.filters")} aria-pressed={filterCount > 0} aria-expanded={filtering}
                                    onClick={() => setFiltering(true)}>
                                    <i className="bi bi-funnel"></i>
                                    {filterCount > 0 && <span className="position-absolute top-0 start-100 translate-middle badge rounded-pill text-bg-danger">{filterCount}</span>}
                                </button>
                            )}
                            {onSearch && <Form.SearchText name="q" context={searchForm} props={{ placeholder, autoComplete: "off", "data-nav": true, "data-key": searchKey, "aria-label": placeholder }} />}
                        </div>
                        {note && <Note className="mt-2">{note}</Note>}
                    </BlockRow>
                </Rows>
            )}
            {searching && <hr className="gf-listing-rule my-3" />}
            {groups.length > 0 && filtering && (
                <SidePanel label={t("settings.filters")} className="gf-filters" onClose={closeFilters}>
                    <FilterGroups groups={groups} />
                </SidePanel>
            )}
            {!searching && note && <Note className={first}>{note}</Note>}
            {(rows.length > 0 || head || ghosts > 0 || (blank && blankText)) && (
                <InListing.Provider value={Boolean(onSearch)}>
                    <Rows className={!searching && note ? "mt-3" : first}>
                        {head}
                        {loading
                            ? Array.from({ length: ghosts }, (_, index) => <React.Fragment key={`ghost-${index}`}>{placeholderRow ? placeholderRow(index) : <PlaceholderRow />}</React.Fragment>)
                            : rows.length ? rows.map(render) : blankText && <EmptyRow icon={searched ? "bi-search" : "bi-inbox"} text={blankText} />}
                    </Rows>
                </InListing.Provider>
            )}
            {pages > 1 && (
                <div ref={pager} className="gf-pager mt-3" inert={loading ? "" : undefined} aria-busy={loading || undefined}>
                    {/* LA MANETTE JUSQU'À LA PAGINATION, partout où une liste se feuillette (le porteur, 2026-11-16 : « Avec une
                        manette, la pagination dans recherche ne fonctionne pas ») : Skeleton dessine ses boutons sans
                        `data-nav` ; une page des Réglages les marquait (SettingsPage, usePadReach), pas un panneau latéral
                        (la recherche d'une bibliothèque) — la liste les marque donc elle-même. */}
                    <PadReach>
                        <pagination.PrevNext>
                            <pagination.Selectbox />
                        </pagination.PrevNext>
                    </PadReach>
                </div>
            )}
        </>
    );
}
