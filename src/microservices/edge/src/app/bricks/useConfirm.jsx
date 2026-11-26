import React, { useCallback, useContext } from "react";
import { useTranslation } from "react-i18next";
import { focusElement, pushScreenInput } from "../libs/input.js";
import Skeleton from "../skeleton/skeleton.js";

// Le texte d'une fenêtre de Skeleton : son titre et ses boutons s'écrivent en HTML dans la page (skeleton.js) ;
// un nom de jeu ou de membre n'y doit jamais devenir du balisage.
export const escapeHtml = (text) => String(text ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char]));

// LA CONFIRMATION : la fenêtre de Skeleton, `context.modal.open(…, { variant: "confirm" })` (sa copie,
// skeleton/skeleton.js) — TOUTE confirmation du launcher (le porteur, 2026-11-13 : « utilise skeleton.Feedback pour
// les fenêtres de confirmation partout dans le launcher »). `confirm({ title, message, confirmLabel, onConfirm,
// onCancel })`. À LA MANETTE : la fenêtre est le calque du dessus, la lumière y entre sur Annuler, la croix passe
// d'un bouton à l'autre, A active, B annule ; refermée, la lumière revient sur ce qui l'a ouverte. Les sons sont
// ceux de la croix (libs/input.js).
//
// UNE CONFIRMATION EN DEUX TEMPS : `then`, les mêmes champs pour une seconde fenêtre, plus explicite — arrêter la partie
// d'un autre (Live.jsx ; le porteur, 2026-10-25 : deux confirmations « pour éviter les erreurs »). La seconde s'ouvre une
// fois la première refermée, jamais par-dessus elle : la lumière ne se perd pas entre les deux, et revient à la fin sur
// ce qui avait ouvert la première. Seul le `onConfirm` de la dernière agit.
export function useConfirm() {
    const skeleton = useContext(Skeleton.context);
    const { t } = useTranslation();

    // `body` : un contenu à la place du message — un champ à remplir avant de confirmer (la note d'un backup ; le porteur,
    // 2026-11-16 : « mets le champ note dans un modal skeleton au lieu de l'afficher plus bas »), un choix (quitter une
    // partie, Menu.jsx). Il vit dans le portail de Skeleton, sous son fournisseur : son propre état, et ce qu'il garde se
    // lit à la confirmation.
    //
    // `whenClosed` : le geste attend que la fenêtre soit refermée — quitter une partie démonte la page de jeu, et la
    // fenêtre posée dans son élément en plein écran (skeleton.js) partirait avec elle avant d'avoir fini de se refermer.
    // `cancelLabel` : le mot du refus quand « Annuler » ne dit pas ce qu'il fait (une demande de manette : « Garder »).
    return useCallback(function confirm({ title, message = null, body = null, confirmLabel, cancelLabel = null, onConfirm, onCancel = null, then = null, whenClosed = false }, opener = document.activeElement) {
        let release = () => {};
        let confirmed = false;
        const id = skeleton.modal.open(body || <span>{message || title}</span>, {
            variant: "confirm",
            title: message || body ? escapeHtml(title) : null,
            confirmLabel: escapeHtml(confirmLabel || title),
            cancelLabel: escapeHtml(cancelLabel || t("settings.cancel")),
            onConfirm: then || whenClosed ? () => { confirmed = true; } : onConfirm,
            onCancel,
            onClose: () => {
                release();

                if (confirmed && then) {
                    confirm(then, opener);

                    return;
                }
                if (confirmed) {
                    onConfirm();

                    return;
                }
                requestAnimationFrame(() => opener?.isConnected && focusElement(opener, true));
            },
        });
        const node = id && document.getElementById(id);

        if (!node) {
            return;
        }
        node.setAttribute("data-nav-layer", "");
        node.querySelectorAll("button, textarea, input, select").forEach((control) => control.setAttribute("data-nav", ""));
        const cancel = node.querySelector('[data-role="cancel"]');
        release = pushScreenInput({ back: () => cancel?.click() });
        // La fenêtre de Bootstrap prend le focus à la fin de son entrée : la lumière va sur Annuler à ce moment-là.
        node.addEventListener("shown.bs.modal", () => focusElement(cancel, true), { once: true });
    }, [skeleton, t]);
}
