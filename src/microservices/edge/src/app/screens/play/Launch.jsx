import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { LaunchBackdrop } from "../../engine/stream/shared/SessionStream.jsx";
import { LAUNCH_SCREEN_BACKGROUND } from "../../engine/stream/shared/launch-backdrop.js";
import { reportLaunchFailure } from "../../libs/api.js";
import { can, useData } from "../../libs/data.jsx";
import { useScreenInput } from "../../libs/input.js";
import { useButton } from "../../libs/ui.js";

// L'ÉCRAN DU LANCEMENT (#191, #287, chantier 35) — le même partout où l'on attend une partie : dès le
// clic sur « Jouer » (la réponse du kernel peut prendre des minutes), puis sur la page de jeu jusqu'à
// la première image du JEU, pas celle de l'intro.
//
// Le fond est la vidéo de l'intro, que la passerelle tient figée pendant le chargement
// (`backdrop.launch`, LaunchBackdrop de la webapp : sa balise <video> passe d'un écran à l'autre sans
// s'interrompre), ou la vidéo de la partie pour qui la rejoint (`backdrop.stream`) ; dessous, le fond
// GameFlex. La ligne du bas dit l'étape en cours — la dernière reçue — et ce que seul le serveur
// savait (le cœur, la géométrie) ; un échec se dit en entier, et « Signaler l'erreur » prend la place
// du tourniquet. La croix en haut à droite annule, toujours (le porteur, 2026-10-10 et 2026-10-29) ;
// B aussi.
//
// `conflict` — la partie qu'on joue déjà ailleurs (« tu joues déjà ») : y retourner (`onConflict`).
export default function PlayLaunch({ label, steps = [], failure = null, backdrop = null, onCancel, conflict = null, onConflict = null }) {
    const { t } = useTranslation();
    const button = useButton();
    const { capabilities } = useData();
    const [report, setReport] = useState(null);
    const current = steps.length ? steps[steps.length - 1] : null;
    const stepLabel = (step) => (step ? t(`streaming.connection.step.${step}`) : null);

    useScreenInput({ back: onCancel, switchTab: () => {}, select: () => {}, start: () => {} });

    const sendReport = async () => {
        setReport("sending");
        const result = await reportLaunchFailure({ context: `lancement (${label})`, steps, detail: failure });
        setReport(result.ok ? "sent" : "failed");
    };

    return (
        <div className="play-launch" style={{ background: LAUNCH_SCREEN_BACKGROUND }} data-nav-layer>
            {backdrop && <LaunchBackdrop {...backdrop} />}
            <button type="button" className="play-round play-cancel" data-nav data-key="pl-cancel" title={t("play.cancel")} aria-label={t("play.cancel")} onClick={onCancel}>
                <i className="bi bi-x-lg"></i>
            </button>
            <div className="play-line">
                {failure ? (
                    <div className="d-flex flex-column align-items-center gap-1" data-step={failure.step || current?.step} data-state="error" style={{ maxWidth: "35rem" }}>
                        {conflict ? (
                            <>
                                <span className="fw-semibold">{t("play.conflict", { title: conflict.title || "" })}</span>
                                <button type="button" className={`${button("main")} mt-2`} data-nav data-key="pl-conflict" onClick={onConflict}>{t("play.conflictGo")}</button>
                            </>
                        ) : (
                            <>
                                {/* « Signaler » pour une étape en erreur seulement (#191) : un refus (la console qui vient
                                    de fermer une session, une partie déjà en cours) n'est pas une panne à signaler. Ni
                                    quand les signalements sont éteints (Réglages › Journaux : la capacité `reports`). */}
                                {failure.step && can(capabilities, "reports") && (
                                    <button type="button" className={`${button("warning")} mb-2`} data-nav data-key="pl-report" disabled={report === "sending" || report === "sent"} onClick={sendReport}>
                                        {t(report === "sent" ? "play.reported" : report === "failed" ? "play.reportFailed" : "play.report")}
                                    </button>
                                )}
                                <span className="fw-semibold text-danger">{t("play.failed")}</span>
                                {(stepLabel(failure.step) || failure.error) && <span className="small text-break">{[stepLabel(failure.step), failure.error].filter(Boolean).join(" — ")}</span>}
                            </>
                        )}
                    </div>
                ) : (
                    <div className="d-flex flex-column align-items-center gap-1 mw-100" data-step={current?.step} data-state={current?.state}>
                        <div className="spinner-border mb-2" role="status"></div>
                        <span className="fw-semibold text-break">{stepLabel(current?.step) || label}</span>
                        {current?.detail && <span className="small text-white-50 text-break">{current.detail}</span>}
                    </div>
                )}
            </div>
        </div>
    );
}
