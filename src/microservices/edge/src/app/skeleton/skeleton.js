// SKELETON, COPIÉ DANS LE LAUNCHER (chantier 53, P1, `#662` ; le porteur, 2026-11-13 : « Du coup faudra copier la
// lib depuis skeleton »). La copie de `src/webapp/src/app/skeleton/skeleton.js`, réduite à ce que le launcher en
// emploie : la COUCHE DES FENÊTRES — `context.modal.open(contenu, { variant: "confirm", title, confirmLabel,
// onConfirm, onCancel })`, la fenêtre de confirmation que le porteur veut partout (« utilise skeleton.Feedback pour
// les fenêtres de confirmation partout dans le launcher »). Son code est celui de Skeleton, tel quel : le balisage,
// les classes, la pile, la fenêtre de Bootstrap (son JavaScript) et jQuery.
//
// CE QUI N'EST PAS COPIÉ, parce que le launcher a les siens : la coque de l'application (les routes, l'en-tête, le
// pied, le menu), la navigation à la manette et au clavier (`libs/input.js`), le thème et la langue, les panneaux
// latéraux (`shell/SidePanel.jsx`), les avis (`toast` de `shell/Shell.jsx` ; ceux de la page de jeu,
// `screens/play/Notices.jsx`). Les libs de formulaires (Form, Table, Pagination) sont copiées entières, dans `libs/`.
//
// Le Remote Play venu de la webapp (`engine/hooks/use-gamepad.jsx`) importe celui-ci ; il n'en lit que
// `Skeleton.context`, pour ses avis, que la page de jeu lui fournit (Play.jsx).

import React, { useState, useSyncExternalStore, createContext } from 'react';
import { createPortal } from 'react-dom';
import { Modal } from 'bootstrap';
import $ from 'jquery';
import { Form, Ruleset } from './libs/Form.jsx';
import { Table } from './libs/Table.jsx';
import { usePagination } from './libs/Pagination.jsx';

import './skeleton.css';

// jQuery EN GLOBAL, comme la webapp le pose (src/webapp/src/app/index.js) : la fenêtre de Bootstrap ne déclenche ses
// événements jQuery (`hidden.bs.modal`, que ce code écoute avec `$modal.on`) que si `window.jQuery` existe — sans lui,
// une fenêtre refermée restait dans la page, et sa pile ne se vidait jamais (vu dans le launcher, 2026-11-13).
if (typeof window !== "undefined") {
    window.$ = window.jQuery = $;
}

// Les panneaux latéraux de Skeleton ne sont pas copiés : cette pile reste vide (le code des fenêtres, tel quel, la lit
// pour placer ses fonds et sa hauteur).
const offcanvasStack = [];
let modalStack = [];
let modalCounter = 0;
let overlayOrderCounter = 0;

// QUI VEUT SAVOIR QU'UNE FENÊTRE S'OUVRE OU SE FERME — la page de jeu, qui suspend les entrées du jeu tant qu'il y en
// a une (#408). Prévenus après chaque changement de la pile.
const overlayListeners = new Set();
const notifyOverlays = () => overlayListeners.forEach((listener) => listener());

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
//
// DANS LE LAUNCHER, SEULEMENT CE QUE SKELETON A POSÉ LUI-MÊME : ses fenêtres (jQuery les ajoute au corps de la page) et
// le fond que Bootstrap leur donne. Les panneaux (`.offcanvas`, leur fond) et les avis (`.toast-container`) du launcher
// sont à React — SidePanel.jsx, le Toast de Shell.jsx, screens/play/Notices.jsx —, rendus dans l'arbre, et cette copie n'a ni
// panneaux ni avis à elle (voir l'en-tête). Les déplacer cassait React : le plein écran d'une partie emportait l'avis
// de #app dans la page de jeu, React le retirait avec elle en quittant, puis insérait l'écran suivant DEVANT ce nœud
// disparu — « NotFoundError: Failed to execute 'insertBefore' on 'Node' » (chantier 53, P12, bug 3).
if (typeof document !== "undefined") {
    const followFullscreen = () => {
        const root = overlayRoot();
        for (const selector of [".modal", ".modal-backdrop"]) {
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
const nextOverlayZIndex = () => 3010 + (offcanvasStack.length + modalStack.length) * 20;

const context = createContext(null);

// LE FOURNISSEUR : autour de l'application (index.js), il tient les portails des fenêtres et donne `context.modal`.
export default function Skeleton({ children }) {
    const [portals, setPortals] = useState({ modals: [] });

    const app = {
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
                // LE FOND SUIT LA FENÊTRE EN PLEIN ÉCRAN : Bootstrap le pose dans <body>, que le navigateur ne peint plus
                // quand un autre élément est en plein écran — une fenêtre ouverte pendant une partie en plein écran
                // (Quitter, chantier 53, P12) restait sans voile. Il rejoint la fenêtre là où elle est.
                if (item.$backdrop[0] && item.$backdrop[0].parentNode !== $modal[0].parentNode) {
                    item.$backdrop.appendTo($modal[0].parentNode);
                }

                return id;
            },
            close: (id = null) => {
                const item = id === null ? modalStack[modalStack.length - 1] : modalStack.find(m => m.id === id);
                (item?.hide ?? item?.instance.hide.bind(item?.instance))?.();
            },
        },
    };

    return (
        <context.Provider value={{ ...app }}>
            {children}
            {portals.modals.map(item => createPortal(item.component, item.node, item.id))}
        </context.Provider>
    );
};

Skeleton.context = context;

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

Skeleton.Form = Form;
Skeleton.Ruleset = Ruleset;
Skeleton.Table = Table;
Skeleton.usePagination = usePagination;
