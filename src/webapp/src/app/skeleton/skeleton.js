
import React, { useState, useMemo, Suspense, lazy, useEffect, useRef, useSyncExternalStore, createContext, useContext } from 'react';
import { createPortal } from 'react-dom';
import { Routes, Route, Link, Navigate, useLocation, matchPath, useNavigate } from "react-router-dom";
import * as bs from 'react-bootstrap';
import { Offcanvas, Toast, Modal, Alert } from 'bootstrap';
import { Form, Ruleset } from './libs/Form.jsx';
import { Table } from './libs/Table.jsx';
import { usePagination } from './libs/Pagination.jsx';
import { useTranslation } from 'react-i18next';

import '../i18n/index.js';
import './skeleton.css';


let offcanvasStack = [];
let offcanvasCounter = 0;
let modalStack = [];
let modalCounter = 0;
let toastStack = [];
let toastCounter = 0;
let alertCounter = 0;
let overlayOrderCounter = 0;

// QUI VEUT SAVOIR QU'UN PANNEAU S'OUVRE OU SE FERME — la page de jeu, qui
// suspend les entrées du jeu tant qu'il y en a un (#408). Prévenus après
// chaque changement des deux piles.
const overlayListeners = new Set();
const notifyOverlays = () => overlayListeners.forEach((listener) => listener());

// LA NAVIGATION À LA CROIX, SUSPENDUE PENDANT QU'ON JOUE (#407). Une partie
// en cours prend la croix, les flèches, A et B ; le pont manette-clavier et la
// navigation spatiale ne doivent alors rien en faire — ils déplaçaient le
// focus sur les boutons de la page de jeu, et leurs touches fabriquées
// partaient au jeu. Posé par la page de jeu (Skeleton.suspendNavigation).
let navigationSuspended = false;

// Where overlays are mounted.
//
// The Fullscreen API renders the fullscreen element and its DESCENDANTS —
// nothing else. Every overlay here used to be appended to <body>, so entering
// fullscreen on anything smaller than the document made the whole menu, its
// modals and its toasts vanish: they were still in the DOM, simply outside the
// only subtree the browser was painting (reported live 2026-08-25 on the
// streaming page, whose fullscreen switch lives INSIDE the menu that
// disappeared).
//
// So overlays follow the fullscreen element when there is one. Bootstrap's own
// backdrop follows for free — it is created with `rootElement:
// this._element.parentNode` (5.3) — which is why re-parenting the panel is
// enough, and why it must be the panel that moves, not a copy.
const overlayRoot = () => document.fullscreenElement || document.body;

// Fullscreen can be entered or left WHILE something is open — the streaming
// menu's own switch does exactly that. Move what is already mounted rather
// than leaving it behind in a subtree nobody is painting.
if (typeof document !== "undefined") {
    const followFullscreen = () => {
        const root = overlayRoot();
        for (const selector of [".offcanvas", ".modal", ".offcanvas-backdrop", ".modal-backdrop", ".toast-container", "[data-skeleton-alerts]"]) {
            for (const node of document.querySelectorAll(selector)) {
                if (node.parentNode !== root) {
                    root.appendChild(node);
                }
            }
        }
    };
    for (const event of ["fullscreenchange", "webkitfullscreenchange", "MSFullscreenChange"]) {
        document.addEventListener(event, followFullscreen);
    }
}

const containers = new Map();
const components = {};
const dPadMap = {
    0: "Enter",
    1: "Escape",
    12: "ArrowUp",
    13: "ArrowDown",
    14: "ArrowLeft",
    15: "ArrowRight",
};
const TEXT_BG_CLASS = {
    normal: "",
    info: "text-bg-info",
    success: "text-bg-success",
    warning: "text-bg-warning",
    error: "text-bg-danger",
    confirm: "text-bg-primary",
};
const WHITE_CLOSE_VARIANTS = new Set(["success", "error", "confirm"]);
// Shared by toast (small inline icon) and modal (big SweetAlert2-style icon).
const VARIANT_ICON_CLASS = {
    normal: "",
    info: "bi bi-info-circle-fill",
    success: "bi bi-check-circle-fill",
    warning: "bi bi-exclamation-triangle-fill",
    error: "bi bi-x-circle-fill",
    confirm: "bi bi-question-circle-fill",
};
const MODAL_ICON_COLOR_CLASS = {
    normal: "",
    info: "text-info",
    success: "text-success",
    warning: "text-warning",
    error: "text-danger",
    confirm: "text-primary",
};
const TOAST_POSITION_CLASS = {
    "top-start": "top-0 start-0",
    "top-center": "top-0 start-50 translate-middle-x",
    "top-end": "top-0 end-0",
    "middle-start": "top-50 start-0 translate-middle-y",
    "middle-center": "top-50 start-50 translate-middle",
    "middle-end": "top-50 end-0 translate-middle-y",
    "bottom-start": "bottom-0 start-0",
    "bottom-center": "bottom-0 start-50 translate-middle-x",
    "bottom-end": "bottom-0 end-0",
};
const nextOverlayZIndex = () => 3010 + (offcanvasStack.length + modalStack.length) * 20;
const FOCUSABLE_SELECTOR = 'a[href], button:not(:disabled), input:not(:disabled):not([type="hidden"]), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';
const NAV_DIRECTIONS = {
    ArrowUp: { dx: 0, dy: -1 },
    ArrowDown: { dx: 0, dy: 1 },
    ArrowLeft: { dx: -1, dy: 0 },
    ArrowRight: { dx: 1, dy: 0 },
};
// Les <input> qui ne sont pas des champs de saisie : la croix doit en sortir,
// et A les actionne.
const NON_TEXT_INPUTS = new Set(["checkbox", "radio", "button", "submit", "reset", "range", "color", "file", "image"]);
// Un champ où l'on ÉCRIT — qui garde ses flèches (le curseur) et ses touches.
const isTextEntry = (element) => !!element && (
    (element.tagName === "INPUT" && !NON_TEXT_INPUTS.has(element.type))
    || element.tagName === "TEXTAREA"
    || element.isContentEditable
);
const context = createContext(null);

export default function Skeleton({ init, topRightNavigation = null }) {
    const navigate = useNavigate();
    // LA MANETTE DANS L'INTERFACE (#407) — la croix et le stick gauche
    // déplacent le focus, A ACTIVE ce qui l'a, B ferme le panneau du dessus.
    Skeleton.useGamepad((key) => {
        // A ne fabrique plus une touche Entrée : un événement fabriqué ne
        // déclenche AUCUNE action par défaut, donc ni un <button>, ni un
        // interrupteur, ni une ligne de menu ne réagissaient — seuls les
        // éléments qui écoutaient eux-mêmes Entrée. On active, comme un doigt.
        if (key === 'Enter') {
            Skeleton.activateFocus();

            return;
        }

        const activeElement = document.activeElement;

        // Dans un champ de saisie, la croix ne sert pas le curseur (pas de
        // clavier à l'écran pour écrire à la manette) : haut et bas en
        // SORTENT, B le quitte.
        if (isTextEntry(activeElement)) {
            if (key === 'ArrowUp' || key === 'ArrowDown') {
                Skeleton.navigateFocus(key);
            } else if (key === 'Escape') {
                activeElement.blur();
            }

            return;
        }

        // Dispatched from the focused element (falling back to document) with
        // bubbles:true so it propagates exactly like a real keypress would —
        // through any focused-element/component listeners, up to document
        // (where Bootstrap's own Escape-closes-offcanvas/modal listener lives)
        // and finally to window. A bare `window.dispatchEvent(...)` never
        // reaches document-level listeners at all: dispatching directly on
        // window only notifies window's own listeners, it doesn't propagate
        // down into document.
        const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
        (activeElement || document).dispatchEvent(event);
    });
    Skeleton.useKeyboard((event) => {
        // This fires for real key presses AND for the synthetic keydown events
        // the gamepad hook dispatches (they bubble to window like any other
        // keydown) — one implementation drives both input sources, no need to
        // duplicate this logic in the gamepad hook above.
        const key = event.key;

        if (!NAV_DIRECTIONS[key] || navigationSuspended) {
            return;
        }

        const activeElement = document.activeElement;

        // UNE LISTE DÉROULANTE, À LA MANETTE : gauche et droite changent sa
        // valeur, haut et bas en sortent. Elle gardait toutes les flèches, et
        // une fois dedans on n'en sortait plus qu'en fermant le panneau. Au
        // vrai clavier (événement de confiance), elle garde les siennes.
        if (activeElement?.tagName === 'SELECT' && !event.isTrusted) {
            if (key === 'ArrowLeft' || key === 'ArrowRight') {
                Skeleton.stepSelect(activeElement, key === 'ArrowRight' ? 1 : -1);
            } else {
                Skeleton.navigateFocus(key);
            }

            return;
        }

        // Let native widgets keep arrow keys for their own purpose (moving a
        // text cursor, changing a <select>/range value, navigating a Bootstrap
        // dropdown menu) instead of stealing them for page-level navigation.
        // An interrupteur (case à cocher) n'en fait rien : il n'est plus compté
        // comme un champ, sinon la croix restait bloquée dessus.
        if (isTextEntry(activeElement) || activeElement?.tagName === 'SELECT' || activeElement?.closest('.dropdown-menu, .dropdown')) {
            return;
        }

        Skeleton.navigateFocus(key);
    });

    // Return value unused on purpose — calling the hook is what makes this
    // component (and MenuPanel's active-link check below) re-render on route
    // change; reading location.pathname directly keeps that dependency explicit.
    const location = useLocation();
    const { i18n, t } = useTranslation();
    const [currentTheme, setCurrentTheme] = useState(() => {
        const initThemeMode = init.themeMode == "light" ? "light" : "dark";

        try {
            return localStorage.getItem("html.theme") || initThemeMode;
        } catch {
            return initThemeMode;
        }
    });
    const [portals, setPortals] = useState({ alerts: [], toasts: [], modals: [], offcanvas: [], });
    const importFallback = React.isValidElement(init.importFallback) ? init.importFallback : null;
    const homePage = init.homePage || { to: "/", component: null };
    const mappedRoutes = useMemo(() => {
        const routes = [];
        const setRoutes = (items) => {
            items.forEach(item => {
                if (item.path?.to && item.path?.component) {
                    routes.push({
                        path: item.path.to,
                        component: item.path.component,
                        noTemplate: item.template === false,
                    });
                    components[item.path.to] = Skeleton.resolvePath(item.path.component);
                }

                setRoutes(item.children || []);
            });
        };

        if (homePage && homePage.component) {
            routes.push({
                path: homePage.to || "/",
                component: homePage.component,
            });

            components[homePage.to || "/"] = Skeleton.resolvePath(homePage.component);
        }

        setRoutes(init.menuItems || []);

        return routes;
    }, [init]);

    // Whether the CURRENT path is a noTemplate route — decides which of the
    // two <Routes> trees below is actually "live" for this render. Each one
    // has its own catch-all (path="*" -> Navigate to home), which is exactly
    // right for whichever tree matches the current path, but wrong for the
    // one that doesn't: an unconditional catch-all Navigate fires from
    // *inside its own tree* regardless of what the other tree is rendering,
    // so mounting both at once means whichever tree has no match for the
    // current path immediately redirects home — undoing a perfectly valid
    // match in the other tree. Rendering only one tree (the matching one,
    // chrome or bare) at a time avoids ever having two catch-alls live
    // simultaneously.
    //
    // `init.routesPending`: the app builds some routes from menu items that
    // arrive asynchronously (the console list). Until they do, a path that
    // matches nothing may simply not be routed YET — redirecting home at that
    // moment (with `replace`) threw away every reloaded console page. While
    // pending, the catch-all renders the import fallback and waits.
    const hasFullscreenRoutes = mappedRoutes.some(
        (route) => route.noTemplate === true && matchPath({ path: route.path, end: true }, location.pathname)
    );

    // Sync the DOM to the persisted theme on first mount — <html data-bs-theme>
    // is hardcoded in index.html, so without this the page briefly renders in
    // that hardcoded theme before flashing to the user's actual saved choice.
    useEffect(() => {
        $("html").attr("data-bs-theme", currentTheme);
    }, []);

    const app = {
        theme: currentTheme,
        toggleTheme: (theme) => {
            try {
                setCurrentTheme(theme);
                localStorage.setItem("html.theme", theme);
                $("html").attr("data-bs-theme", theme);
            } catch {
                app.toast.open(t('theme.switch_failed'), { variant: "error", duration: 3000, title: t('theme.switch_failed_title') })
            }
        },
        // lockOrientation (default true) — the in-game menu's manual toggle
        // button still wants the old landscape-lock behavior; the auto
        // fullscreen-on-mount used by Streaming/Watch on mobile/tablet
        // (le refacto GameFlex, voir GAMEFLEX_JOURNAL.md) passes false, since
        // that one must only strip the browser's own toolbars/nav bars, not
        // force a rotation the user never asked for — orientation stays
        // whatever it already was.
        toggleFullscreen: ({ lockOrientation = true } = {}) => {
            if (!document.fullscreenElement) {
                document.documentElement.requestFullscreen().then(() => {
                    // Best-effort — only actually applies on mobile/tablet (most desktop
                    // browsers reject orientation lock outright), and only in fullscreen.
                    if (lockOrientation && screen.orientation && screen.orientation.lock) {
                        screen.orientation.lock("landscape").catch(() => { });
                    }
                }).catch(() => { });
            } else {
                document.exitFullscreen();
            }
        },
        currentLanguage: i18n.language,
        availableLanguages: init.languages || [
            { value: "en", label: "🇬🇧 - English" },
            { value: "fr", label: "🇫🇷 - Français" },
        ],
        toggleLanguage: (lang) => {
            i18n.changeLanguage(lang).catch(() => { });
        },
        offcanvas: {
            // Rend le NŒUD du corps du panneau.
            //
            // Pourquoi : ce qu'on passe ici est un INSTANTANÉ. L'élément est
            // rangé dans un état et rendu par portail depuis CE fournisseur —
            // il ne re-rend donc jamais avec l'appelant, et tout ce qu'il a
            // capturé à l'ouverture reste figé. Un panneau de filtres montrait
            // ainsi des cases qui ne bougeaient plus, alors que les valeurs
            // changeaient bien derrière (signalé le 2026-09-02).
            //
            // C'est sans remède ici : la fraîcheur ne peut venir que de
            // l'arbre de l'appelant. Rendre ce nœud lui permet d'y porter son
            // propre sous-arbre avec createPortal — le panneau re-rend alors
            // comme n'importe quel enfant de sa page. Voir ListingElement,
            // qui s'en sert.
            open: (component = null, { position = "end", size = null, speed = 500, onClose = null } = {}) => {
                if (!component || !React.isValidElement(component)) {
                    return null;
                }

                const isVertical = position === "top" || position === "bottom";
                const viewport = isVertical ? window.innerHeight : window.innerWidth;
                const id = `skeleton-offcanvas-${++offcanvasCounter}`;
                const zIndex = nextOverlayZIndex();
                const $offcanvas = $(`
                    <div class="offcanvas offcanvas-${position}" tabindex="-1" id="${id}" style="z-index: ${zIndex}; transition-duration: ${speed}ms; display: block; overflow-y: auto;">
                        <div class="offcanvas-body" style="overflow-y: visible;"></div>
                    </div>
                `).appendTo(overlayRoot());

                size = size === null ? viewport : (Math.abs(size) > 0 && Math.abs(size) < 1 ? viewport * size : size);
                $offcanvas.css(isVertical ? { height: size } : { width: size });

                const instance = new Offcanvas($offcanvas[0], { scroll: false });
                const item = { id, instance, $offcanvas, $backdrop: null, onClose };

                [...offcanvasStack, ...modalStack].forEach(prev => prev.$backdrop?.css({ opacity: 0, visibility: "hidden" }));
                item.order = ++overlayOrderCounter;
                offcanvasStack.push(item);
                notifyOverlays();
                const body = $offcanvas.find(".offcanvas-body")[0];
                setPortals(prev => ({ ...prev, offcanvas: [...prev.offcanvas, { id, node: body, component }] }));

                $offcanvas.on("hidden.bs.offcanvas", () => {
                    offcanvasStack = offcanvasStack.filter(e => e.id !== id);
                    notifyOverlays();
                    instance.dispose();
                    $offcanvas.remove();

                    const survivor = [...offcanvasStack, ...modalStack].sort((a, b) => b.order - a.order)[0];
                    survivor?.$backdrop?.css({ opacity: "", visibility: "" });

                    if (offcanvasStack.length + modalStack.length > 0) {
                        document.body.style.overflow = "hidden";
                    }

                    setPortals(prev => ({ ...prev, offcanvas: prev.offcanvas.filter(panel => panel.id !== id) }));
                    item.onClose?.();
                });

                instance.show();
                item.$backdrop = $(".offcanvas-backdrop").last().css({ zIndex: zIndex - 1 });

                return body;
            },
            close: () => {
                offcanvasStack[offcanvasStack.length - 1]?.instance.hide();

                if (offcanvasStack.length + modalStack.length <= 0) {
                    document.body.style.overflow = "auto";
                }
            },
        },
        modal: {
            open: (component = null, {
                size = null,
                variant = "normal",
                title = null,
                onClose = null,
                onConfirm = null,
                onCancel = null,
                okLabel = "OK",
                confirmLabel = "Confirm",
                cancelLabel = "Cancel",
            } = {}) => {
                if (!component || (typeof component !== "string" && !React.isValidElement(component))) {
                    return;
                }

                const isConfirm = variant === "confirm";
                const id = `skeleton-modal-${++modalCounter}`;
                const zIndex = nextOverlayZIndex();
                const sizeClass = size ? `modal-${size}` : "";
                // SweetAlert2/iOS-alert look: no header bar at all — a big colored icon
                // (reusing the same variant → icon glyph map as the toast) stands in for
                // it instead. Only variants that map to a real icon (info/success/warning
                // /error/confirm) get one and the centered treatment; "normal" (used for
                // arbitrary content — forms, nested modals, long content, see
                // examples/Feedbacks.jsx) gets a plain bold title and keeps its natural,
                // non-centered layout so those demos aren't forced into a look that only
                // makes sense for short alert-style messages.
                const iconClass = VARIANT_ICON_CLASS[variant] || "";
                const iconColorClass = MODAL_ICON_COLOR_CLASS[variant] || "";
                const hasIcon = !!iconClass;
                const footerHtml = isConfirm ? `
                    <div class="modal-footer p-0 gap-0">
                        <button type="button" class="btn btn-link text-decoration-none flex-fill rounded-0 m-0 py-3 border-end" data-role="cancel">
                            ${cancelLabel}
                        </button>
                        <button type="button" class="btn btn-link text-decoration-none flex-fill rounded-0 m-0 py-3 fw-semibold" data-role="confirm">
                            ${confirmLabel}
                        </button>
                    </div>` : hasIcon ? `
                    <div class="modal-footer p-0 gap-0">
                        <button type="button" class="btn btn-link text-decoration-none flex-fill rounded-0 m-0 py-3 fw-semibold" data-bs-dismiss="modal">
                            ${okLabel}
                        </button>
                    </div>` : "";
                const $modal = $(`
                    <div class="modal fade modal-zoom" id="${id}" tabindex="-1" style="z-index: ${zIndex};">
                        <div class="modal-dialog modal-dialog-centered ${sizeClass}">
                            <div class="modal-content">
                                <div class="modal-body pt-4 ${hasIcon ? "text-center" : ""}">
                                    ${hasIcon ? `<i class="${iconClass} ${iconColorClass} d-block mb-3" style="font-size: 4.5rem;"></i>` : ""}
                                    ${title ? `<h5 class="modal-title fw-bold mb-2">${title}</h5>` : ""}
                                    <div class="modal-body-content"></div>
                                </div>
                                ${footerHtml}
                            </div>
                        </div>
                    </div>
                `).appendTo(overlayRoot());

                // Portal target is this inner wrapper, not .modal-body itself — .modal-body
                // also holds the jQuery-inserted icon/title markup above, and a React portal
                // takes full ownership of its target node's children, which would wipe that
                // static markup out on the first render.
                const $body = $modal.find(".modal-body-content");

                if (React.isValidElement(component)) {
                    setPortals(prev => ({ ...prev, modals: [...prev.modals, { id, node: $body[0], component }] }));
                } else {
                    $body.html(component);
                }

                const instance = new Modal($modal[0]);
                const item = { id, instance, $modal, $backdrop: null, onClose };

                // Let go of the focus BEFORE hiding. Bootstrap marks the modal
                // aria-hidden while it closes, and a button inside it that
                // still holds focus is then hidden from assistive technology
                // while remaining reachable by keyboard — the browser says so
                // out loud ("Blocked aria-hidden on an element because its
                // descendant retained focus"). Blurring first is what the
                // warning asks for, and it costs one line per exit.
                const hide = () => {
                    const focused = document.activeElement;
                    if (focused && $modal[0].contains(focused)) {
                        focused.blur();
                    }
                    instance.hide();
                };
                item.hide = hide;

                if (isConfirm) {
                    $modal.find('[data-role="confirm"]').on("click", () => { onConfirm?.(); hide(); });
                    $modal.find('[data-role="cancel"]').on("click", () => { onCancel?.(); hide(); });
                }

                [...offcanvasStack, ...modalStack].forEach(prev => prev.$backdrop?.css({ opacity: 0, visibility: "hidden" }));
                item.order = ++overlayOrderCounter;
                modalStack.push(item);
                notifyOverlays();

                $modal.on("hidden.bs.modal", () => {
                    modalStack = modalStack.filter(e => e.id !== id);
                    notifyOverlays();
                    instance.dispose();
                    $modal.remove();

                    const survivor = [...offcanvasStack, ...modalStack].sort((a, b) => b.order - a.order)[0];
                    survivor?.$backdrop?.css({ opacity: "", visibility: "" });

                    if (offcanvasStack.length + modalStack.length > 0) {
                        document.body.style.overflow = "hidden";
                    }

                    setPortals(prev => ({ ...prev, modals: prev.modals.filter(m => m.id !== id) }));
                    item.onClose?.();
                });

                instance.show();
                item.$backdrop = $(".modal-backdrop").last().css({ zIndex: zIndex - 1 });

                return id;
            },
            close: (id = null) => {
                const item = id === null ? modalStack[modalStack.length - 1] : modalStack.find(m => m.id === id);
                (item?.hide ?? item?.instance.hide.bind(item?.instance))?.();
            },
        },
        toast: {
            open: (content = null, {
                position = "top-end",
                duration = 5000,
                variant = "normal",
                title = null,
                onConfirm = null,
                onCancel = null,
                confirmLabel = "Confirm",
                cancelLabel = "Cancel",
            } = {}) => {
                if (!content || (typeof content !== "string" && !React.isValidElement(content))) {
                    return;
                }

                let $container = containers.get(position);

                if (!$container) {
                    $container = $(`<div class="toast-container position-fixed p-3 ${TOAST_POSITION_CLASS[position] || TOAST_POSITION_CLASS["top-end"]}" style="z-index: 3500;"></div>`).appendTo(overlayRoot());
                    containers.set(position, $container);
                }

                const id = `skeleton-toast-${++toastCounter}`;
                const isConfirm = variant === "confirm";
                // A confirm toast waits for an explicit answer — like the modal's confirm
                // variant, it never autohides and skips the close X (Cancel is the escape
                // hatch) so a decision can't be dismissed by accident.
                const effectiveDuration = isConfirm ? 0 : duration;
                // TEXT_BG_CLASS.normal is "" (originally blank on purpose for the old
                // modal header background, which is gone now — modals no longer have a
                // header — but TEXT_BG_CLASS is still shared with the confirm-toast
                // variant, so it's kept). A toast has no other backing surface, so ""
                // here fell through to Bootstrap's own default .toast background:
                // 85%-opacity body color, i.e. visibly transparent over whatever's
                // behind it. bg-body/text-body are the opaque equivalent for the theme.
                const bgClass = TEXT_BG_CLASS[variant] || "bg-body text-body";
                const borderClass = variant === "normal" ? "border" : "border-0";
                const closeClass = WHITE_CLOSE_VARIANTS.has(variant) ? "btn-close-white" : "";
                const iconClass = VARIANT_ICON_CLASS[variant] || "";
                // Icon glyph sits in a white badge circle, colored with the same solid
                // brand color the modal uses (MODAL_ICON_COLOR_CLASS) — reads correctly
                // regardless of bgClass's own text color, unlike the plain inline icon
                // this replaced.
                const iconColorClass = MODAL_ICON_COLOR_CLASS[variant] || "";
                // confirm always maps to TEXT_BG_CLASS.confirm (text-bg-primary, solid
                // dark blue + white text), so the footer can hardcode light styling
                // instead of branching per-variant like MODAL_ICON_COLOR_CLASS does.
                const footerHtml = isConfirm ? `
                    <div class="d-flex border-top border-light border-opacity-25">
                        <button type="button" class="btn btn-link text-white text-decoration-none flex-fill rounded-0 m-0 py-2 border-end border-light border-opacity-25" data-role="cancel">
                            ${cancelLabel}
                        </button>
                        <button type="button" class="btn btn-link text-white text-decoration-none flex-fill rounded-0 m-0 py-2 fw-semibold" data-role="confirm">
                            ${confirmLabel}
                        </button>
                    </div>` : "";
                const progressHtml = effectiveDuration > 0 ? `<div class="notif-progress"><div style="animation-duration: ${effectiveDuration}ms;"></div></div>` : "";
                const $toast = $(`
                    <div class="toast ${bgClass} ${borderClass} rounded-3 shadow overflow-hidden" role="alert" aria-live="assertive" aria-atomic="true" id="${id}">
                        <div class="d-flex align-items-center p-3">
                            ${iconClass ? `
                            <div class="bg-white rounded-circle d-flex align-items-center justify-content-center flex-shrink-0 me-3" style="width: 2.75rem; height: 2.75rem;">
                                <i class="${iconClass} ${iconColorClass}" style="font-size: 1.375rem;"></i>
                            </div>` : ""}
                            <div class="flex-grow-1">
                                ${title ? `<div class="fw-bold">${title}</div>` : ""}
                                <div class="toast-body p-0"></div>
                            </div>
                            ${isConfirm ? "" : `<button type="button" class="btn-close ${closeClass} ms-2 align-self-start" data-bs-dismiss="toast" aria-label="Close"></button>`}
                        </div>
                        ${footerHtml}
                        ${progressHtml}
                    </div>
                `);

                $container.append($toast);

                const $body = $toast.find(".toast-body");

                if (React.isValidElement(content)) {
                    setPortals(prev => ({ ...prev, toasts: [...prev.toasts, { id, node: $body[0], component: content }] }));
                } else {
                    $body.html(content);
                }

                const instance = new Toast($toast[0], { autohide: effectiveDuration > 0, delay: effectiveDuration });
                const item = { id, position, instance, $toast };

                if (isConfirm) {
                    $toast.find('[data-role="confirm"]').on("click", () => { onConfirm?.(); instance.hide(); });
                    $toast.find('[data-role="cancel"]').on("click", () => { onCancel?.(); instance.hide(); });
                }

                toastStack.push(item);

                $toast.on("hidden.bs.toast", () => {
                    toastStack = toastStack.filter(e => e.id !== id);
                    instance.dispose();
                    $toast.remove();
                    setPortals(prev => ({ ...prev, toasts: prev.toasts.filter(t => t.id !== id) }));

                    if ($container.children().length === 0) {
                        $container.remove();
                        containers.delete(position);
                    }
                });

                instance.show();

                return id;
            },
            close: (id) => {
                toastStack.find(t => t.id === id)?.instance.hide();
            },
        },
        alert: (content = null, { variant = "normal", duration = null, title = null } = {}) => {
            if (!content || (typeof content !== "string" && !React.isValidElement(content))) {
                return;
            }

            if (variant === "confirm") {
                variant = "normal";
            }

            let $container = containers.get("alert");

            if (!$container) {
                const headerEl = document.querySelector("#skeleton-html > div");
                const top = headerEl ? headerEl.getBoundingClientRect().bottom : 0;
                $container = $(`<div data-skeleton-alerts class="position-fixed start-0 end-0 d-flex flex-column" style="top: ${top}px; z-index: 3500;"></div>`).appendTo(overlayRoot());
                containers.set("alert", $container);
            }

            const id = `skeleton-alert-${++alertCounter}`;
            // Same solid-card look as the toast (TEXT_BG_CLASS), not Bootstrap's
            // subtle/tinted `alert-*` classes — bg-body/text-body + border is the
            // "normal" fallback since TEXT_BG_CLASS.normal is "" (see toast for why).
            const bgClass = TEXT_BG_CLASS[variant] || "bg-body text-body border";
            const closeClass = WHITE_CLOSE_VARIANTS.has(variant) ? "btn-close-white" : "";
            const iconClass = VARIANT_ICON_CLASS[variant] || "";
            const iconColorClass = MODAL_ICON_COLOR_CLASS[variant] || "";
            const progressHtml = duration > 0 ? `<div class="notif-progress"><div style="animation-duration: ${duration}ms;"></div></div>` : "";
            const $alert = $(`
                <div class="alert ${bgClass} border-0 fade show mt-2 mx-3 rounded-3 shadow overflow-hidden p-0" role="alert" id="${id}">
                    <div class="d-flex align-items-center p-3">
                        ${iconClass ? `
                        <div class="bg-white rounded-circle d-flex align-items-center justify-content-center flex-shrink-0 me-3" style="width: 2.75rem; height: 2.75rem;">
                            <i class="${iconClass} ${iconColorClass}" style="font-size: 1.375rem;"></i>
                        </div>` : ""}
                        <div class="flex-grow-1">
                            ${title ? `<div class="fw-bold">${title}</div>` : ""}
                            <div class="alert-content"></div>
                        </div>
                        <button type="button" class="btn-close ${closeClass} ms-2 align-self-start" data-bs-dismiss="alert" aria-label="Close"></button>
                    </div>
                    ${progressHtml}
                </div>
            `).appendTo($container);

            const $body = $alert.find(".alert-content");

            if (React.isValidElement(content)) {
                setPortals(prev => ({ ...prev, alerts: [...prev.alerts, { id, node: $body[0], component: content }] }));
            } else {
                $body.html(content);
            }

            const instance = new Alert($alert[0]);
            let timeoutId = duration ? setTimeout(() => instance.close(), duration) : null;

            $alert.on("closed.bs.alert", () => {
                clearTimeout(timeoutId);
                setPortals(prev => ({ ...prev, alerts: prev.alerts.filter(a => a.id !== id) }));

                if ($container.children().length === 0) {
                    $container.remove();
                    containers.delete("alert");
                }
            });
        },
    };

    return (
        <context.Provider value={{ ...app }}>
            <div id="app">
                {/* Only one of these two ever renders — see hasFullscreenRoutes's
                    comment above for why mounting both at once (each with its
                    own catch-all) was fighting itself. */}
                {hasFullscreenRoutes && (
                    <Routes>
                        {mappedRoutes.map((route, i) => {
                            if (route?.noTemplate === true) {
                                return (
                                    <Route
                                        key={i}
                                        path={route.path}
                                        element={Skeleton.importResolvedPath(route.path, importFallback)}
                                    />
                                );
                            }

                            return null;
                        })}
                        <Route path="*" element={init.routesPending ? importFallback : <Navigate to={homePage?.to || "/"} replace />} />
                    </Routes>
                )}
                {!hasFullscreenRoutes && (
                    <div id="skeleton-html">
                        <div
                            className="d-flex align-items-center justify-content-between px-3 py-1 mb-4 border-bottom position-sticky"
                            style={{ top: 0, zIndex: 3000, backgroundColor: 'var(--bs-body-bg)' }}>
                            <div className="d-flex align-items-center vertical-align-middle">
                                <a
                                    type="button"
                                    role="button"
                                    tabIndex={0}
                                    title="Menu"
                                    onClick={() => app.offcanvas.open(<Skeleton.MenuPanel menuItems={init.menuItems} side="start" />, {
                                        position: "start",
                                        size: 450,
                                    })}
                                    onKeyDown={Skeleton.keyActivate(() => app.offcanvas.open(<Skeleton.MenuPanel menuItems={init.menuItems} side="start" />, {
                                        position: "start",
                                        size: 450,
                                    }))}
                                    className="me-2 fs-1 text-decoration-none text-reset"
                                >
                                    <i className={"bi bi-list"}></i>
                                </a>
                                <div className="fs-3 mb-0 fw-bold">
                                    <a
                                        className="text-decoration-none text-muted"
                                        style={{ cursor: "pointer" }}
                                        tabIndex={0}
                                        role="button"
                                        onKeyDown={Skeleton.keyActivate(() => navigate(homePage?.to || "/"))}
                                        onClick={() => navigate(homePage?.to || "/")}>{init.name || "Skeleton"}</a>
                                    {/* UNE MARQUE À CÔTÉ DU NOM (« staging ») : une
                                        installation dit ce qu'elle est, sans changer de
                                        nom ni de couleurs. */}
                                    {init.nameTag ? <span className="badge text-bg-warning ms-2 align-middle fs-6">{init.nameTag}</span> : null}
                                </div>
                            </div>
                            <div className="d-flex align-items-center vertical-align-middle gap-3">
                                {topRightNavigation ? topRightNavigation : null}
                            </div>
                        </div>
                        <div id="skeleton-html-content">
                            <Routes>
                                {mappedRoutes.map((route, i) => {
                                    if (route?.noTemplate !== true || route?.noTemplate === undefined) {
                                        return (
                                            <Route
                                                key={i}
                                                path={route.path}
                                                element={Skeleton.importResolvedPath(route.path, importFallback)}
                                            />
                                        );
                                    }

                                    return null;
                                })}
                                <Route path="*" element={init.routesPending ? importFallback : <Navigate to={homePage?.to || "/"} replace />} />
                            </Routes>
                        </div>
                        <div id="skeleton-html-footer" className="px-3 py-4 mt-5 border-top d-flex flex-wrap align-items-center justify-content-between gap-2" style={{ zIndex: 3000, backgroundColor: 'var(--bs-body-bg)' }}>
                            <div className="d-flex align-items-center">
                                <span className="text-muted fw-bold">
                                    {/* UNE SEULE DATE, même dans l'APK : sa
                                        page est construite avec elle et
                                        copiée depuis ses assets, ce tampon
                                        date donc aussi l'application. Le
                                        suffixe « · app <version — date> »
                                        répétait la même ligne (le porteur,
                                        2026-10-26). */}
                                    {init.name || "Skeleton"} v{process.env.GAMEFLEX_VERSION} ({process.env.GITHUB_COMMIT_SHA} · {process.env.BUILD_STAMP})
                                </span>
                            </div>
                            {/* Les liens que l'application déclare
                                (`init.footerLinks` : href, icon, label),
                                ouverts hors de la page. */}
                            {init.footerLinks?.length > 0 && (
                                <div className="d-flex align-items-center gap-3">
                                    {init.footerLinks.map((link) => (
                                        <a key={link.href} href={link.href} target="_blank" rel="noopener noreferrer" className="text-muted text-decoration-none">
                                            {link.icon && <i className={`${link.icon} me-1`} />}
                                            {link.label}
                                        </a>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>
                )}
            </div>
            {Object.keys(portals).map(name => {
                return portals[name].map(item => createPortal(item.component, item.node, item.id));
            })}
        </context.Provider>
    );
};

Skeleton.context = context;

Skeleton.MenuPanel = function ({ menuItems, side = "start" }) {
    const location = useLocation();
    const context = useContext(Skeleton.context);
    const RecursiveMenu = ({ items, level = 0 }) => {
        return (
            <div className={`nav nav-pills flex-column gap-0 ${level > 0 ? "ps-4" : ""}`}>
                {items.map((item, i) => {
                    if (item.hidden) {
                        return null;
                    }
                    // PAR SEGMENT ENTIER : `/streamex/xbox360` commence par
                    // `/streamex/xbox`, et la Xbox s'allumait avec la 360
                    // (relevé par le porteur le 2026-10-22).
                    const to = item.path.to;
                    const active = Boolean(to) && (location.pathname === to ||
                        location.pathname.startsWith(to.endsWith("/") ? to : `${to}/`));

                    return (
                        <div key={i}>
                            <Link
                                to={to}
                                onClick={() => context.offcanvas.close()}
                                className={`nav-link d-flex align-items-center gap-3 ${active ? "active" : "text-body"}`}
                            >
                                <i className={item?.icon || "bi bi-circle"}></i>
                                <span>{item.label}</span>
                                {/* `badge` — un complément court, à droite du
                                    libellé, que le libellé n'a pas à porter :
                                    l'année d'une console (demandé le
                                    2026-10-11). Facultatif ; le même gris
                                    que les autres badges d'information. */}
                                {item.badge ? (
                                    <bs.Badge bg="secondary" className="ms-auto fw-normal">{item.badge}</bs.Badge>
                                ) : null}
                            </Link>
                            {item.children && item.children.length > 0 && (
                                <RecursiveMenu items={item.children} level={level + 1} />
                            )}
                        </div>
                    );
                })}
            </div>
        );
    };

    return (
        <div>
            <div className="text-end px-0 pt-0 pb-2 mb-4 border-bottom">
                <a
                    type="button"
                    role="button"
                    tabIndex={0}
                    title="Close"
                    onClick={() => context.offcanvas.close()}
                    onKeyDown={Skeleton.keyActivate(() => context.offcanvas.close())}
                    className="fs-5 text-decoration-none text-reset"
                >
                    <i className={side === "start" ? "bi bi-arrow-left" : "bi bi-arrow-right"}></i>
                </a>
            </div>
            {menuItems.map((item, i) => {
                if (item.hidden) {
                    return null;
                }

                return (
                    <div key={i} className="mb-4">
                        <div className="d-flex align-items-center gap-2 mb-2 text-uppercase fw-bold text-primary">
                            <span>{item.label}</span>
                        </div>
                        {item.children && item.children.length > 0 && (
                            <RecursiveMenu items={item.children} level={0} />
                        )}
                    </div>
                );
            })}
        </div>
    );
};

Skeleton.resolvePath = (componentPath) => {
    return lazy(() =>
        // `catch` ATTRAPE TOUT, y compris ce qui n'est pas une absence.
        //
        // Un module présent mais qui lève à l'évaluation (import circulaire,
        // chunk périmé servi par un service worker) tombait ici et
        // s'annonçait « was not found » — on cherche alors un fichier qui est
        // pourtant là. La bannière reste la même pour qui regarde l'écran ;
        // la vraie erreur va dans la console, pour qui la cherche.
        import(`../${componentPath.replaceAll('.', '/')}.jsx`).catch((error) => (
            console.error(`Skeleton.resolvePath(${componentPath})`, error),
            {
            default: () => (
                <div className="text-danger">
                    Lazy load failed: {componentPath} (filename: <b>./{componentPath.replaceAll('.', '/')}.jsx</b>) — voir la console pour l'erreur réelle.
                </div>
            )
        }))
    );
};

Skeleton.importResolvedPath = (componentPath, fallback = null) => {
    fallback = fallback || (
        <div className="d-flex align-items-center justify-content-center" style={{ height: 'var(--gameflex-visible-height, 100vh)' }}>
            <bs.Spinner animation="border" role="status">
                <span className="visually-hidden">Loading...</span>
            </bs.Spinner>
        </div>
    );
    const LazyComponent = components[componentPath] || null;

    return (
        <Suspense fallback={fallback}>
            {LazyComponent ? <LazyComponent /> : null}
        </Suspense>
    );
};

Skeleton.useResponsive = function ({ threshold = 1.3, autoResize = false } = {}) {
    const get = () => {
        const [width, height, deviceRatio] = [window.innerWidth, window.innerHeight, window.devicePixelRatio];

        return {
            width,
            height,
            deviceRatio,
            isPortrait: width < (height * threshold),
            isLandscape: width >= (height * threshold),
            isMobile: width < 768,
            isTablet: width >= 768 && width < 1024,
            isDesktop: width >= 1024,
        };
    };
    const [dimensions, setDimensions] = useState(get());

    useEffect(() => {
        if (!autoResize) {
            return;
        }

        const handleResize = () => setDimensions(get());
        window.addEventListener('resize', handleResize);

        return () => window.removeEventListener('resize', handleResize);
    }, []);

    return dimensions;
};

Skeleton.useGamepad = function (dispatchKey) {
    // LE RAPPEL DANS UNE RÉFÉRENCE, et la boucle montée UNE fois. Il était
    // en dépendance de l'effet ; c'est une flèche neuve à chaque rendu de
    // Skeleton — chaque panneau ouvert ou fermé —, donc la boucle repartait,
    // oubliait les boutons tenus, et renvoyait un bouton encore enfoncé.
    const dispatchRef = useRef(dispatchKey);
    dispatchRef.current = dispatchKey;

    useEffect(() => {
        let raf;
        const pressed = new Map();
        const repeatDelay = 380, repeatRate = 90;
        // Pendant une partie, la manette est au jeu (#407) : on suit encore
        // les boutons — un bouton tenu à la reprise n'est pas un nouvel appui —
        // mais on ne dit rien.
        const dispatch = (key) => {
            if (!navigationSuspended) {
                dispatchRef.current(key);
            }
        };
        const tick = () => {
            const pads = navigator.getGamepads?.() || [];
            const now = performance.now();

            for (const pad of pads) {
                if (!pad) {
                    continue;
                }

                pad.buttons.forEach((b, i) => {
                    const key = dPadMap[i];

                    if (!key) {
                        return;
                    }

                    const id = `${pad.index}-${i}`;

                    if (b.pressed) {
                        const state = pressed.get(id);

                        if (!state) {
                            pressed.set(id, { downAt: now, nextAt: now + repeatDelay });
                            dispatch(key);
                        } else if (now >= state.nextAt) {
                            state.nextAt = now + repeatRate;
                            dispatch(key);
                        }
                    } else {
                        pressed.delete(id);
                    }
                });

                const ax = pad.axes[0] || 0, ay = pad.axes[1] || 0;
                const stickKey = Math.abs(ax) > 0.55 ? (ax > 0 ? 'ArrowRight' : 'ArrowLeft') :
                    Math.abs(ay) > 0.55 ? (ay > 0 ? 'ArrowDown' : 'ArrowUp') : null;
                const id = `${pad.index}-stick`;

                if (stickKey) {
                    const state = pressed.get(id);

                    if (!state || state.key !== stickKey) {
                        pressed.set(id, { key: stickKey, downAt: now, nextAt: now + repeatDelay });
                        dispatch(stickKey);
                    } else if (now >= state.nextAt) {
                        state.nextAt = now + repeatRate;
                        dispatch(stickKey);
                    }
                } else {
                    pressed.delete(id);
                }
            }

            raf = requestAnimationFrame(tick);
        };

        raf = requestAnimationFrame(tick);

        return () => cancelAnimationFrame(raf);
    }, []);
};

// L'ÉVÉNEMENT ENTIER, et non sa seule touche : la navigation doit savoir
// s'il vient d'un vrai clavier (`isTrusted`) ou du pont de la manette — une
// liste déroulante ne se pilote pas pareil (#407). Le rappel est tenu dans une
// référence, pour la même raison que celui de useGamepad.
Skeleton.useKeyboard = function (handleKey) {
    const handlerRef = useRef(handleKey);
    handlerRef.current = handleKey;

    useEffect(() => {
        const handleKeyDown = (e) => handlerRef.current(e);
        window.addEventListener('keydown', handleKeyDown);

        return () => window.removeEventListener('keydown', handleKeyDown);
    }, []);
};

// TV/console-style spatial navigation: moves focus to the nearest focusable
// element in the pressed direction, so gamepad d-pad/stick and physical arrow
// keys can drive the whole app the same way Tab does, without requiring every
// page to hand-wire its own navigation order.
Skeleton.navigateFocus = function (key) {
    const direction = NAV_DIRECTIONS[key];

    if (!direction) {
        return;
    }

    // Scope navigation to the topmost open overlay (modal/offcanvas), matching
    // what's actually visible/interactive on screen — otherwise arrow nav could
    // "escape" into page content sitting behind an open dialog's backdrop.
    const topOverlay = [...offcanvasStack, ...modalStack].sort((a, b) => b.order - a.order)[0];
    const scope = topOverlay ? document.getElementById(topOverlay.id) : document;

    if (!scope) {
        return;
    }

    const candidates = Array.from(scope.querySelectorAll(FOCUSABLE_SELECTOR)).filter((el) => {
        return el.offsetParent !== null && !el.closest('[aria-hidden="true"]');
    });

    if (candidates.length === 0) {
        return;
    }

    const current = document.activeElement;
    const currentInScope = current && scope.contains(current) && candidates.includes(current);

    if (!currentInScope) {
        focusForNavigation(candidates[0]);
        return;
    }

    const currentRect = current.getBoundingClientRect();
    const cx = currentRect.left + currentRect.width / 2;
    const cy = currentRect.top + currentRect.height / 2;
    let best = null, bestScore = Infinity;

    candidates.forEach((el) => {
        if (el === current) {
            return;
        }

        const rect = el.getBoundingClientRect();
        const ex = rect.left + rect.width / 2;
        const ey = rect.top + rect.height / 2;
        const dx = ex - cx, dy = ey - cy;

        // Displacement along the pressed axis must be positive (the element is
        // actually "ahead" in that direction); drift on the perpendicular axis
        // is penalized so navigation prefers elements roughly in line with the
        // current one over ones that are merely closer as the crow flies.
        const primary = dx * direction.dx + dy * direction.dy;

        if (primary <= 0) {
            return;
        }

        const perpendicular = Math.abs(dx * direction.dy - dy * direction.dx);
        const score = primary + perpendicular * 2;

        if (score < bestScore) {
            bestScore = score;
            best = el;
        }
    });

    // RIEN DANS CETTE DIRECTION : on reste. On repartait au PREMIER élément
    // de la page — un appui de trop vers le bas ramenait tout en haut.
    if (best) {
        focusForNavigation(best);
    }
};

// LE FOCUS SE VOIT (#407). Un focus posé par programme n'allume pas toujours
// `:focus-visible` — le navigateur ne le montre qu'après un vrai clavier —, et
// à la manette on ne savait plus où l'on était. La classe s'en va avec le
// focus. Le style est dans skeleton.css.
function focusForNavigation(element) {
    element.focus();
    element.classList.add("skeleton-nav-focus");
    element.addEventListener("blur", () => element.classList.remove("skeleton-nav-focus"), { once: true });
}

// A, À LA MANETTE (#407) : ce que ferait un doigt sur l'élément qui a le
// focus. Rien n'a le focus dans ce qu'on voit : il va au premier élément.
// Une liste déroulante passe à l'option suivante (on ne peut pas l'ouvrir
// d'ici) ; un champ de saisie garde le focus, sans plus.
Skeleton.activateFocus = function () {
    const element = document.activeElement;
    const topOverlay = [...offcanvasStack, ...modalStack].sort((a, b) => b.order - a.order)[0];
    const scope = (topOverlay && document.getElementById(topOverlay.id)) || document.body;

    // Le panneau lui-même (Bootstrap le focalise à l'ouverture, tabindex -1)
    // n'est pas une chose qu'on actionne : A va au premier élément, comme la
    // croix.
    if (!element || element === document.body || !scope.contains(element) || !element.matches(FOCUSABLE_SELECTOR)) {
        Skeleton.navigateFocus("ArrowDown");

        return;
    }
    if (element.tagName === "SELECT") {
        Skeleton.stepSelect(element, 1);

        return;
    }
    if (isTextEntry(element)) {
        return;
    }

    element.click();
};

// Une liste déroulante avance (ou recule) d'une option, comme si on l'avait
// choisie : la valeur passe par le setter natif, puis un vrai `change` —
// c'est ce que React écoute.
Skeleton.stepSelect = function (select, step) {
    const options = [...select.options].filter((option) => !option.disabled);

    if (options.length < 2) {
        return;
    }

    const index = Math.max(0, options.indexOf(select.selectedOptions[0]));
    const next = options[(index + step + options.length) % options.length];
    const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set;

    setValue.call(select, next.value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
};

// Les panneaux ouverts (offcanvas et fenêtres), et qui veut le savoir — la
// page de jeu suspend les entrées du jeu tant qu'il y en a un (#408).
Skeleton.overlayCount = function () {
    return offcanvasStack.length + modalStack.length;
};

Skeleton.subscribeOverlays = function (listener) {
    overlayListeners.add(listener);

    return () => overlayListeners.delete(listener);
};

Skeleton.useOverlayCount = function () {
    return useSyncExternalStore(Skeleton.subscribeOverlays, Skeleton.overlayCount);
};

// Suspendre la navigation à la croix pendant qu'une partie a la main (#407).
Skeleton.suspendNavigation = function (suspended) {
    navigationSuspended = !!suspended;
};

// Standard ARIA "button" keyboard pattern for elements that visually act as a
// button (an icon, an <a> without href, an <h5>) but aren't a native <button>
// or a <a href>, so they don't get Enter/Space activation for free — pair with
// tabIndex={0} on the element.
Skeleton.keyActivate = function (onClick) {
    return function (e) {
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
            e.preventDefault();
            onClick(e);
        }
    };
};

Skeleton.Form = Form;
Skeleton.Ruleset = Ruleset;
Skeleton.Table = Table;
Skeleton.usePagination = usePagination;