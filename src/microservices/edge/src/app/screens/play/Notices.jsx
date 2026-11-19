import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useButton } from "../../libs/ui.js";

// LES AVIS DE LA PAGE DE JEU — les toasts de Bootstrap, en haut au centre, par-dessus la vidéo.
//
// Le moteur de la webapp les ouvre comme il ouvrait ceux de Skeleton (`context.toast.open(texte,
// options)`) : ses hooks reçoivent ce `context`, et le Remote Play le lit par `Skeleton.context`
// (sa copie de Skeleton, app/skeleton/skeleton.js, que Play.jsx lui fournit). Un avis s'efface seul (`duration`, 4 s ; 0 : il reste) ; une QUESTION
// (`variant: "confirm"`) attend sa réponse, deux boutons (« Toujours là ? »). Les demandes de manette n'en sont
// plus : elles vont à la cloche et au menu (Bell.jsx, chantier 68).

const VARIANTS = { error: "text-bg-danger", warning: "text-bg-warning", success: "text-bg-success" };

export function useNotices() {
    const [list, setList] = useState([]);
    const seq = useRef(0);

    const dismiss = useCallback((id) => setList((current) => current.filter((notice) => notice.id !== id)), []);
    const open = useCallback((content, options = {}) => {
        seq.current += 1;
        const notice = { id: seq.current, content, ...options };
        setList((current) => [...current, notice].slice(-4));

        return notice.id;
    }, []);
    const context = useMemo(() => ({ toast: { open, close: dismiss } }), [open, dismiss]);

    return { list, dismiss, context };
}

function Notice({ notice, dismiss }) {
    const { t } = useTranslation();
    const button = useButton();
    const asks = notice.variant === "confirm";

    useEffect(() => {
        if (asks) {
            return undefined;
        }
        const timer = notice.duration === 0 ? null : setTimeout(() => dismiss(notice.id), notice.duration || 4000);

        return () => clearTimeout(timer);
    }, [asks, notice, dismiss]);

    const answer = (yes) => {
        dismiss(notice.id);
        (yes ? notice.onConfirm : notice.onCancel)?.();
    };

    return (
        <div className={`toast show border-0 ${VARIANTS[notice.variant] || "text-bg-dark bg-opacity-75"}`} role={asks ? "alertdialog" : "status"} aria-live="polite">
            <div className="toast-body">
                <div>{notice.content}</div>
                {asks && (
                    <div className="d-flex gap-2 justify-content-end mt-2">
                        <button type="button" className={`${button("main")} btn-sm`} data-nav data-key={`pn-yes-${notice.id}`} onClick={() => answer(true)}>{notice.confirmLabel || t("play.yes")}</button>
                        <button type="button" className={`${button()} btn-sm`} data-nav data-key={`pn-no-${notice.id}`} onClick={() => answer(false)}>{notice.cancelLabel || t("play.dismiss")}</button>
                    </div>
                )}
            </div>
        </div>
    );
}

export default function PlayNotices({ notices }) {
    const { list, dismiss } = notices;

    if (!list.length) {
        return null;
    }

    return (
        <div className="toast-container position-absolute top-0 start-50 translate-middle-x p-3" style={{ zIndex: 1020 }}>
            {list.map((notice) => <Notice key={notice.id} notice={notice} dismiss={dismiss} />)}
        </div>
    );
}
