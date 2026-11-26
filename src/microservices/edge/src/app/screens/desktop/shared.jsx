import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { toast } from "../../shell/Shell.jsx";
import { Footer } from "../../bricks/Footer.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { PageHeader } from "../../bricks/PageHeader.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { Form, formContext, FormButton } from "../../ui/Forms.jsx";

// CE QUE LES PAGES DE LA CONSOLE DU PC PARTAGENT (chantier 70 ; une route, un fichier : chantier 81) — l'appel au
// lanceur, l'état de la console, le cadre d'une page, le formulaire d'un réglage. Pas une route.

// UN APPEL AU LANCEUR (cmd/desktop/console.go) : `/api/v1/desktop…`, servi à la machine seule.
export const call = async (path, method = "GET", body = undefined) => {
    try {
        const response = await fetch(`/api/v1/desktop${path}`, {
            method, headers: body ? { "Content-Type": "application/json" } : undefined, body: body ? JSON.stringify(body) : undefined,
        });

        return { ok: response.ok, status: response.status, ...(await response.json()) };
    } catch {
        return { ok: false, status: 0 };
    }
};

// L'ÉTAT DE LA CONSOLE, lu une fois et relu après chaque geste, jamais par sondage : `{ state, setState, act, save }`.
export const DesktopState = createContext(null);
export const useDesktopState = () => useContext(DesktopState);

export function useDesktop() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const [state, setState] = useState(null);
    const reload = useCallback(async () => setState(await call("")), []);

    useEffect(() => {
        reload();
    }, [reload]);

    // Un geste : sa réponse est le nouvel état ; un refus se dit — par la fenêtre de Skeleton quand le geste vient d'un
    // FormButton (`viaForm`, useFeedback), sinon un toast.
    const act = async (path, body, method = "POST", viaForm = false) => {
        const answer = await call(path, method, body);

        if (answer.ok) {
            setState(answer);
        } else {
            (viaForm ? feedback.failed : toast)(answer.code === "start_failed" ? t("desktop.startFailed", { detail: answer.detail || "" }) : t("settings.actionFailed"));
        }

        return answer.ok;
    };
    const save = (patch) => act("/settings", { port: state.port, library: state.library, ...patch }, "PUT", true);

    return { state, setState, act, save };
}

// LE CADRE D'UNE PAGE : la boîte des Réglages ; l'accueil n'a pas de retour. Le guide de la page (`guides`) et le pied
// de page sous chacune (chantier 81 : « Guide et pied de page sur toutes les pages »).
export function DesktopPage({ title = null, parent = "/desktop", guides = null, children }) {
    const navigate = useNavigate();

    return (
        <main className="screen">
            <div className="gbody d-flex flex-column">
                <div className="lcol px-0 flex-grow-1" data-scroller>
                    <div className="settings-box w-100 mx-auto">
                        {title ? <PageHeader title={title} onBack={() => navigate(parent)} /> : (
                            <header className="gf-page-header d-flex align-items-center gap-2">
                                <h1 className="ltitle m-0">GameFlex</h1>
                            </header>
                        )}
                        {children}
                        {guides && <GuideSection guides={guides} />}
                        <Footer />
                    </div>
                </div>
            </div>
        </main>
    );
}

// UN RÉGLAGE DU SERVEUR (le port, le dossier) : un champ, Enregistrer ; le serveur redémarre avec lui.
export function SettingForm({ field, title, help, kind = "text", guides, apply }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { state } = useDesktopState();
    const [values, setValues] = useState({ [field]: String(state[field] ?? "") });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const changed = values[field].trim() !== String(state[field] ?? "");

    return (
        <DesktopPage title={title} guides={guides}>
            <Form props={{ onSubmit: async (event) => {
                event.preventDefault();

                if (!changed || pending) {
                    return;
                }
                setPending(true);
                const ok = await apply(values[field].trim());
                setPending(false);

                if (ok) {
                    navigate("/desktop");
                }
            } }}>
                <Section title={title} help={help} sense="modify">
                    <Rows>
                        <TextRow id={field} label={title} context={form} props={{ inputMode: kind === "number" ? "numeric" : undefined, "data-key": `dc-${field}` }} />
                        <SubmitRow>
                            <FormButton id={`dc-save-${field}`} label={t("settings.save")} isReplaced={changed} pending={pending} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
        </DesktopPage>
    );
}

// REJOINDRE UN SERVEUR (join.go) : son adresse vérifiée, puis il s'ouvre dans la fenêtre, sous son compte de là-bas.
// `viaForm` : depuis le formulaire du lien (son FormButton), qui répond par la fenêtre de Skeleton (useFeedback).
const JOIN_ERRORS = { join_not_link: "desktop.joinNotLink", join_not_gameflex: "desktop.joinNotGameflex" };

export function useJoin() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { setState } = useDesktopState();

    return async (link, viaForm = false) => {
        const answer = await call("/join", "POST", { link });

        if (!answer.ok) {
            (viaForm ? feedback.failed : toast)(t(JOIN_ERRORS[answer.code] || "settings.actionFailed", { origin: answer.detail || link }));

            return false;
        }
        setState(answer);
        window.location.href = `${answer.joined[0].origin}/`;

        return true;
    };
}
