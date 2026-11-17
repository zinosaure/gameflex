import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { can, useData } from "../libs/data.jsx";
import { useInputKind } from "../libs/glyphs.js";
import Shortcuts from "./Shortcuts.jsx";
import { lastTabPath, TABS } from "../libs/tabs.js";
import { BAR_EVENTS, focusHeader } from "../libs/input.js";
import { ICON_BUTTON, useBarToggle } from "../libs/ui.js";
import { useFederatedSessions } from "../libs/SessionPool.js";

// LA BARRE DU HAUT, la même sur chaque écran : les quatre onglets (L2 / R2), la
// marque « Beta », les aides des boutons, la cloche, l'engrenage, l'heure.
// Aucun écran ne dessine la sienne.

const TOAST_EVENT = "gameflex-edge:toast";

export const toast = (text) => window.dispatchEvent(new CustomEvent(TOAST_EVENT, { detail: text }));

export function Toast() {
    const [state, setState] = useState({ text: "", on: false });

    useEffect(() => {
        let timer = 0;
        const show = (event) => {
            setState({ text: event.detail, on: true });
            clearTimeout(timer);
            timer = setTimeout(() => setState((previous) => ({ ...previous, on: false })), 3200);
        };
        window.addEventListener(TOAST_EVENT, show);

        return () => {
            clearTimeout(timer);
            window.removeEventListener(TOAST_EVENT, show);
        };
    }, []);

    // Le toast de Bootstrap, posé en haut au centre ; caché tant qu'il n'a rien à dire. Il DESCEND jusqu'à sa place et
    // y remonte (le porteur, 2026-11-14 : « les toasts avec slide down vers le middle-top » ; `.gf-toast`, launcher.css).
    return (
        <div className="toast-container position-fixed top-0 start-50 translate-middle-x mt-5 p-3">
            <div className={`toast gf-toast ${state.on ? "show" : ""}`} role="status" aria-live="polite"><div className="toast-body">{state.text}</div></div>
        </div>
    );
}

function Clock() {
    const { i18n } = useTranslation();
    const [now, setNow] = useState(() => new Date());

    useEffect(() => {
        const timer = setInterval(() => setNow(new Date()), 20000);

        return () => clearInterval(timer);
    }, []);

    return <span className="clock small fw-semibold text-body-secondary">{now.toLocaleTimeString(i18n.language === "fr" ? "fr-FR" : "en-GB", { hour: "2-digit", minute: "2-digit" })}</span>;
}

// L'EN-TÊTE : « GameFlex », et la pastille « staging » quand l'hôte se déclare banc
// d'essai (capacité `staging`, comme l'étiquette de la webapp) ; rien d'autre. PLUS DE
// VERSION (le porteur, 2026-11-16 : « Dans le header, pas de version. Juste le pastille
// staging quand c'est du staging. ») : elle est dans le pied des Réglages (Settings.jsx).
// Plus de « bêta » : edge est l'interface depuis la bascule (chantier 56, P5), une
// interface principale qui se dirait en essai mentirait.
function Brand() {
    const { t } = useTranslation();
    const { capabilities } = useData();

    return (
        <span className="brand">
            GameFlex{can(capabilities, "staging") && <span className="badge text-bg-warning text-uppercase">{t("brand.staging")}</span>}
        </span>
    );
}

// L'ENGRENAGE DU HAUT EST UNE BASCULE (le porteur, 2026-11-12) : il ouvre les
// réglages en retenant d'où l'on vient, et les referme vers cet écran — la même
// console, la même fiche avec sa recherche et ses filtres (dans l'adresse). Select
// fait de même ; B, depuis les réglages, aussi.
// Depuis une page des réglages, l'écran d'avant est autant de pas en arrière qu'on en a ouvert depuis la
// liste (`settingsDepth`, SettingsPage.jsx), plus un.
export const leaveSettings = (navigate, location) => {
    if (location.state?.fromScreen) {
        navigate(-((location.state.settingsDepth || 0) + 1));
    } else {
        navigate(lastTabPath());
    }
};

const toggleSettings = (navigate, location, tab) => {
    if (tab === "settings") {
        leaveSettings(navigate, location);
    } else {
        navigate("/settings", { state: { fromScreen: `${location.pathname}${location.search}` } });
    }
};

export function Topbar({ tab }) {
    const { t } = useTranslation();
    const toggle = useBarToggle();
    const navigate = useNavigate();
    const location = useLocation();
    const { sessions, invites, devices } = useData();
    // L'ONGLET REMOTE PLAY SANS CONSOLE EST GRISÉ (le porteur, 2026-11-14, chantier 53, P12 : « quand il n'y a pas de
    // console, le lien "Remote Play" dans le header devient disabled ») : aucune console enregistrée, ici ou chez un
    // serveur fédéré — rien à y allumer ni à y jouer. Il ne se touche plus, ne prend plus la lumière, et L2 / R2 le
    // sautent ; une console s'enregistre dans Réglages › Remote Play.
    const off = (id) => id === "remoteplay" && !devices.length;
    // PLUS DE CLOCHE (le porteur, 2026-11-12) : son compte — les parties en direct et les
    // invitations, comme avant — est sur l'onglet En direct, qui porte la section Notifications.
    // Les parties des serveurs fédérés comptent aussi (`#747`) : En direct les montre (useFederatedSessions).
    const federatedSessions = useFederatedSessions();
    const count = sessions.length + federatedSessions.length + invites.length;
    // À sa place, la manette : les raccourcis de l'entrée en cours, visible seulement quand une
    // manette est branchée ou que le clavier a servi (rien au doigt ou à la souris seuls).
    const inputKind = useInputKind();
    const [shortcutsOpen, setShortcutsOpen] = useState(false);
    const stripRef = useRef(null);
    // Sur un écran étroit, la bande des onglets défile : l'onglet actif, puis
    // celui qui reçoit le focus, y restent visibles.
    const revealTab = (element) => {
        const strip = stripRef.current;

        if (strip && element) {
            strip.scrollTo({ left: element.offsetLeft - (strip.clientWidth - element.offsetWidth) / 2, behavior: "smooth" });
        }
    };

    // Les gestes qu'aucun écran ne prend (libs/input.js) : L2/R2 parcourent
    // l'en-tête, Select ouvre et referme les réglages, Start ramène à En direct.
    useEffect(() => {
        // L2 / R2 PARCOURENT L'EN-TÊTE (le porteur, 2026-11-12), dans l'ordre où on le voit : les
        // onglets, le bouton des raccourcis (quand il est là), l'engrenage — sans boucler. Un onglet
        // s'ouvre ; un bouton ne reçoit que la lumière (A l'ouvre). D'où l'on part : le bouton en
        // lumière, sinon l'écran ouvert (les réglages : l'engrenage ; sinon son onglet).
        const toTab = (event) => {
            const stops = [...TABS.map((item) => item.id).filter((id) => !off(id)), ...(document.querySelector('.topbar [data-key="shortcuts"]') ? ["shortcuts"] : []), "gear"];
            const active = document.activeElement;
            const focused = active?.closest?.(".topbar") ? active.dataset.key : null;
            const from = focused === "shortcuts" || focused === "gear" ? focused : tab === "settings" ? "gear" : tab;
            const next = stops[stops.indexOf(from) + event.detail];

            if (!next) {
                return;
            }

            if (next === "shortcuts" || next === "gear") {
                focusHeader(document.querySelector(`.topbar [data-key="${next}"]`));
            } else {
                // L'onglet s'ouvre, et le bouton de l'en-tête rend la lumière : la suite part de l'onglet.
                if (focused) {
                    active.blur();
                }
                navigate(TABS.find((item) => item.id === next).path);
            }
        };
        const toSettings = () => toggleSettings(navigate, location, tab);
        const toHome = () => navigate("/live");
        window.addEventListener(BAR_EVENTS.tab, toTab);
        window.addEventListener(BAR_EVENTS.settings, toSettings);
        window.addEventListener(BAR_EVENTS.home, toHome);

        return () => {
            window.removeEventListener(BAR_EVENTS.tab, toTab);
            window.removeEventListener(BAR_EVENTS.settings, toSettings);
            window.removeEventListener(BAR_EVENTS.home, toHome);
        };
    }, [tab, navigate, location, devices]);

    useEffect(() => {
        const strip = stripRef.current;
        const active = strip?.querySelector(".nav-link.active");

        if (strip && active) {
            strip.scrollLeft = active.offsetLeft - (strip.clientWidth - active.offsetWidth) / 2;
        }
    }, [tab]);

    // `data-nav-skip` : la croix et les flèches n'entrent jamais dans l'en-tête — seuls L2 / R2 le
    // parcourent (le porteur, 2026-11-12) ; le doigt, la souris et Tab y vont toujours.
    return (
        <header className="topbar" data-nav-skip>
            <Brand />
            {/* Les onglets : la navigation soulignée de Bootstrap ; sur un écran étroit, la rangée défile. */}
            <nav className="tabs nav nav-underline flex-nowrap overflow-x-auto" ref={stripRef}>
                {TABS.map((item) => (
                    <button key={item.id} type="button" className={`nav-link text-nowrap d-inline-flex align-items-center gap-2 ${tab === item.id ? "active" : ""}`} data-nav data-key={`t-${item.id}`}
                        disabled={off(item.id) && tab !== item.id} aria-current={tab === item.id ? "page" : undefined} onFocus={(event) => revealTab(event.currentTarget)} onClick={() => navigate(item.path)}>
                        {t(item.label)}
                        {item.id === "live" && <span className="badge rounded-pill text-bg-danger">{count}</span>}
                    </button>
                ))}
            </nav>
            <div className="spacer"></div>
            {/* LES DEUX RONDS SONT DES BASCULES (libs/ui.js, useBarToggle) : à contour, pleins quand les raccourcis ou les
                Réglages sont ouverts. */}
            {inputKind && (
                <button type="button" className={`${ICON_BUTTON} ${toggle(shortcutsOpen)}`} data-nav data-key="shortcuts" aria-label={t("shortcuts.title")} title={t("shortcuts.title")}
                    aria-pressed={shortcutsOpen} onClick={() => setShortcutsOpen((open) => !open)}>
                    <i className="bi bi-controller"></i>
                </button>
            )}
            <button type="button" className={`${ICON_BUTTON} ${toggle(tab === "settings")}`} data-nav data-key="gear" aria-label={t("settings.title")} aria-pressed={tab === "settings"} onClick={() => toggleSettings(navigate, location, tab)}>
                <i className="bi bi-gear"></i>
            </button>
            <Clock />
            {shortcutsOpen && inputKind && <Shortcuts kind={inputKind} onClose={() => {
                setShortcutsOpen(false);
                // La lumière revient sur le bouton, comme atteinte par L2 / R2 : B ou une direction la rendent à l'écran.
                requestAnimationFrame(() => focusHeader(document.querySelector('[data-key="shortcuts"]')));
            }} />}
        </header>
    );
}
