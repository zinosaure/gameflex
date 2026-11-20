import React, { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { toRemote, useRemoteAdmin } from "../libs/remoteAdmin.js";
import { focusElement, useScreenInput } from "../libs/input.js";
import { ButtonPlace, MUTED } from "../libs/ui.js";
import { PageHeader, Section } from "../ui/Bricks.jsx";
import { usePadReach } from "../ui/Forms.jsx";
import { leaveSettings, Topbar } from "../shell/Shell.jsx";

// UNE PAGE DES RÉGLAGES (le porteur, 2026-11-13 : « dans Settings, chaque lien doit ouvrir une ROUTE, plus un
// offcanvas » ; « le format de Settings doit être une box, max-width 600px, margin auto, comme l'ancienne
// interface ») : la liste des réglages et chacune de ses pages tiennent dans une boîte centrée de 600 px au
// plus ; une page a son en-tête — le rond de Retour et son titre, le fil d'Ariane dessous (PageHeader) —, puis ses
// sections (Section), à même la boîte. Ce sont des ROUTES, jamais des panneaux : aucune page des Réglages ne s'ouvre en
// offcanvas (le porteur, 2026-11-14 : « les pages de settings sont des routes et non pas des offcanvas »).
//
// LE RETOUR (B, le rond de la page, celui du navigateur) remonte d'un niveau : vers l'entrée d'où l'on est
// venu (l'historique), ou vers la page parente quand on est arrivé par un lien direct ; la liste rend alors la
// lumière à l'entrée qui avait ouvert la page. Select et l'engrenage quittent les réglages d'un geste, d'aussi
// loin qu'on y soit (Shell.jsx : `settingsDepth` compte les pages ouvertes depuis la liste).
//
// Une page est un calque de la croix (`data-nav-layer`), comme l'était le panneau : GAUCHE et DROITE y
// changent l'option d'un sélecteur, HAUT et BAS y font défiler un long texte avant la commande suivante.

// L'entrée qui a ouvert chaque page, par page : celle à qui rendre la lumière au retour.
const openedFrom = new Map();

// LE FIL D'ARIANE D'UNE PAGE (le porteur, 2026-11-13 : « rajouter des fils d'Ariane dans les titres ») : les niveaux
// au-dessus d'elle, de la liste des Réglages à sa page parente (`parent`). Chaque page retient son titre à son adresse
// (`titles`) : un niveau traversé se nomme comme sa page l'était. La liste se nomme « Réglages », une entrée par son
// libellé (`/settings/profile/saves` → settings.items.mySaves.label) ; un niveau qu'on n'a pas traversé (un lien
// direct) se nomme par son segment, ou se saute quand ce n'est pas une page — sauf quand la page le nomme elle-même
// (`levels` de SettingsPage : les jeux d'une console nomment la console, « Jeux », la source et le jeu).
const titles = new Map();

// PROFIL ET SES PAGES ENFANTS (chantier 53, P3) : le mot de passe, le jeton et Mes sauvegardes vivent sous lui.
export const PROFILE = "/settings/profile";

// Le titre d'une page, et l'aide de son entrée dans la liste (une page d'un seul geste la reprend en tête) : celles
// du compte, d'Application et d'À propos (settings/account, settings/application, settings/about).
export const usePageText = (key) => {
    const { t } = useTranslation();

    return { title: t(`settings.items.${key}.label`), help: t(`settings.items.${key}.help`) };
};

// Les niveaux dont chaque segment nomme une entrée : sous la liste, et sous Profil. Le segment se lit comme la clé
// de l'entrée (`remote-play` → remotePlay) ; `saves`, sous Profil, est Mes sauvegardes, `favorites` Mes favoris.
const ENTRY_LEVELS = ["/settings", PROFILE];
const ENTRY_OF_SEGMENT = { saves: "mySaves", favorites: "myFavorites" };
const entryKey = (segment) => ENTRY_OF_SEGMENT[segment] || segment.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());

function useTrail(parent) {
    const { t, i18n } = useTranslation();
    const [parentPath] = parent.split("?");
    const segments = parentPath.split("/").filter(Boolean);
    const trail = [{ label: t("settings.title"), to: "/settings" }];

    for (let depth = 2; depth <= segments.length; depth += 1) {
        const path = `/${segments.slice(0, depth).join("/")}`;
        const last = path === parentPath;
        const segment = segments[depth - 1];
        const entry = ENTRY_LEVELS.includes(`/${segments.slice(0, depth - 1).join("/")}`) ? `settings.items.${entryKey(segment)}.label` : null;
        const label = (entry && i18n.exists(entry) ? t(entry) : null) || titles.get(path) || (last ? decodeURIComponent(segment) : null);

        if (label) {
            trail.push({ label, to: last ? parent : path });
        }
    }

    return trail;
}

export function useSettingsNavigation() {
    const navigate = useNavigate();
    const location = useLocation();
    const depth = location.state?.settingsDepth || 0;
    // Chez un serveur fédéré (chantier 73), chaque adresse des Réglages reste chez lui.
    const remote = useRemoteAdmin();

    return {
        // Ouvrir une page : retenir l'entrée (`key`, son data-key), garder l'écran d'où l'on est venu aux réglages.
        open: (path, key = null, state = {}) => {
            if (key) {
                openedFrom.set(location.pathname, key);
            }
            navigate(toRemote(path, remote), { state: { ...state, fromScreen: location.state?.fromScreen, settingsDepth: depth + 1 } });
        },
        back: (parent) => (depth > 0
            ? navigate(-1)
            : navigate(toRemote(parent, remote), { replace: true, state: { fromScreen: location.state?.fromScreen } })),
    };
}

// L'ÉTAT D'UNE PAGE DANS SON ADRESSE (une recherche, un filtre, une page, une source) : rechargée, ou
// retrouvée au retour d'une page qu'elle a ouverte, elle est telle qu'on l'a laissée. Remplacé, jamais empilé :
// le retour ne repasse pas par chaque lettre tapée. Chaque mise à jour part de l'adresse du moment
// (`window.location`), pas de celle du dernier rendu : deux mises à jour d'un même geste (une recherche, et la
// page qu'elle ramène à 1) s'additionnent au lieu que la seconde efface la première. Une valeur vide, ou celle
// par défaut (`defaults`), sort de l'adresse.
export function useQueryState(defaults = {}) {
    const [params, setParams] = useSearchParams();
    const location = useLocation();
    const read = (name) => params.get(name) ?? defaults[name] ?? "";

    const update = (patch) => {
        const next = new URLSearchParams(window.location.search);

        for (const [name, value] of Object.entries(patch)) {
            if (value === null || value === undefined || value === "" || String(value) === String(defaults[name] ?? "")) {
                next.delete(name);
            } else {
                next.set(name, String(value));
            }
        }
        setParams(next, { replace: true, state: location.state });
    };

    return [read, update];
}

// Un seul de ces états.
export function useSearchParam(name, fallback = "") {
    const [read, update] = useQueryState({ [name]: fallback });

    return [read(name), (value) => update({ [name]: value })];
}

// LA LUMIÈRE À L'OUVERTURE : l'entrée d'où l'on revient quand elle paraît (une liste se lit après coup), sinon
// le retour de la page (un bouton : rien ne fait surgir le clavier d'un téléphone), sinon la première entrée.
function useInitialFocus(box) {
    const location = useLocation();

    useEffect(() => {
        const root = box.current;
        const key = openedFrom.get(location.pathname);
        const initial = root?.querySelector('[data-key="sp-back"]') || root?.querySelector("[data-nav]");
        focusElement(initial, true);

        if (!root || !key) {
            return undefined;
        }
        const restore = () => {
            const row = root.querySelector(`[data-key="${CSS.escape(key)}"]`);

            // Seulement si la lumière n'a pas bougé entre-temps.
            if (row && (document.activeElement === initial || document.activeElement === document.body)) {
                focusElement(row, true);
            }

            return Boolean(row);
        };

        if (restore()) {
            return undefined;
        }
        const observer = new MutationObserver(() => restore() && observer.disconnect());
        observer.observe(root, { childList: true, subtree: true });
        const timer = setTimeout(() => observer.disconnect(), 10000);

        return () => {
            observer.disconnect();
            clearTimeout(timer);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [location.pathname]);
}

// `title` : le titre de la page. `parent` : la page que B rouvre ; `null` pour la liste des réglages, que B quitte
// (sans fil d'Ariane : rien n'est au-dessus d'elle). Pas de sous-titre : l'en-tête ne porte que le rond, le titre et le
// fil (PageHeader). `enter` : où BAS mène depuis Retour (PageHeader). `levels` : `{ adresse: nom }`, les niveaux
// au-dessus d'elle que la page sait nommer — le fil d'Ariane les dit même quand on ne les a pas traversés.
export default function SettingsPage({ title, parent = "/settings", enter = null, levels = {}, children }) {
    const navigate = useNavigate();
    const location = useLocation();
    const { back } = useSettingsNavigation();
    const box = useRef(null);
    const goBack = () => (parent ? back(parent) : leaveSettings(navigate, location));

    for (const [path, label] of Object.entries(levels)) {
        if (label) {
            titles.set(path, label);
        }
    }
    // Chez un serveur fédéré : pas de fil d'Ariane (ses adresses sont les nôtres), la pastille à sa place.
    const remote = useRemoteAdmin();
    const localTrail = useTrail(parent || "");
    const trail = remote ? [] : localTrail;

    if (parent) {
        titles.set(location.pathname, title);
    }

    usePadReach(box);
    useInitialFocus(box);
    useScreenInput({ back: goBack });

    return (
        <main className="screen tabbed" data-nav-layer>
            <Topbar tab="settings" />
            <div className="gbody d-flex flex-column">
                <div className="lcol px-0 flex-grow-1" data-scroller>
                    {/* LE LIEU DES BOUTONS (libs/ui.js, ButtonPlace) : tout ce qui est dans la boîte est « dans les
                        Réglages » — primary pour valider, danger pour ce qui est désactivé ou dangereux. La barre du
                        haut, la même sur chaque écran, reste un écran. */}
                    <ButtonPlace.Provider value="settings">
                        <div ref={box} className="settings-box w-100 mx-auto">
                            <PageHeader title={title} onBack={goBack} trail={parent ? trail : []} enter={enter} remote={remote?.name || ""} />
                            {children}
                        </div>
                    </ButtonPlace.Provider>
                </div>
            </div>
        </main>
    );
}

// Ce qu'une page montre quand son adresse ne désigne plus rien (une console retirée, un lien périmé).
export function NotFound() {
    const { t } = useTranslation();

    return <Section><p className={`${MUTED} m-0`}>{t("settings.notFound")}</p></Section>;
}
