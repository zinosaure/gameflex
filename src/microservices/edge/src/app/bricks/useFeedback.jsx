import React, { useCallback, useContext, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { focusElement, pushScreenInput } from "../libs/input.js";
import Skeleton from "../skeleton/skeleton.js";
import { escapeHtml } from "./useConfirm.jsx";

// CE QUE RÉPOND UN FORMULAIRE (chantier 81 ; le porteur, 2026-11-25 : « Les form boutons avec feedback de skeleton.modal.
// Actuellement c'est des toasts qu'on voit quand les callback des boutons sont des success, failed ou autres. Il faudra
// donc les mettre avec ce que skeleton.Modal propose. […] On garde les toasts pour les autres endroits. ») : la fenêtre de
// Skeleton dans sa variante — sa grande icône colorée, le texte, « OK » (`context.modal.open(…, { variant })`, sa copie
// skeleton/skeleton.js). Seulement après le geste d'un FormButton ; un interrupteur, une ligne qui agit, un avis de partie
// gardent le toast (shell/Shell.jsx).
//
//   const feedback = useFeedback();
//   feedback.success(t("settings.saved"));  feedback.failed(result.detail);  feedback.info(…);  feedback.warning(…);
//   feedback.result(answer.ok, t("…saved"), answer.errorMessage || t("…failed"));  — l'un ou l'autre selon `ok`.
//
// `title` (facultatif) : un titre en gras au-dessus du texte. À LA MANETTE, comme une confirmation (useConfirm) : la
// fenêtre est le calque du dessus, la lumière y entre sur OK, A ou B la referme ; refermée, la lumière revient sur ce qui
// l'a ouverte.
const VARIANTS = { success: "success", failed: "error", info: "info", warning: "warning" };

export function useFeedback() {
    const skeleton = useContext(Skeleton.context);
    const { t } = useTranslation();

    const open = useCallback((variant, text, title = null, opener = document.activeElement) => {
        let release = () => {};
        const id = skeleton.modal.open(<span>{text}</span>, {
            variant,
            title: title ? escapeHtml(title) : null,
            okLabel: escapeHtml(t("feedback.ok")),
            onClose: () => {
                release();
                requestAnimationFrame(() => opener?.isConnected && focusElement(opener, true));
            },
        });
        const node = id && document.getElementById(id);

        if (!node) {
            return;
        }
        node.setAttribute("data-nav-layer", "");
        const ok = node.querySelector("[data-bs-dismiss]");
        ok?.setAttribute("data-nav", "");
        release = pushScreenInput({ back: () => ok?.click() });
        node.addEventListener("shown.bs.modal", () => focusElement(ok, true), { once: true });
    }, [skeleton, t]);

    return useMemo(() => {
        const feedback = Object.fromEntries(Object.entries(VARIANTS).map(([name, variant]) => [name, (text, title = null) => open(variant, text, title)]));
        feedback.result = (ok, success, failed) => (ok ? feedback.success(success) : feedback.failed(failed));

        return feedback;
    }, [open]);
}
