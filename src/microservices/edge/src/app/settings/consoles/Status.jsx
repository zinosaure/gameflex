// Route : /settings/consoles/<console>/status — le statut de toute la console.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SelectRow } from "../../bricks/SelectRow.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { fetchConsoleStatus, setConsoleStatus } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { GAME_STATUSES, STATUS_VARIANT } from "../../libs/format.js";
import { Form, FormButton } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound } from "../SettingsPage.jsx";
import { consolePath, useEmulated } from "../Consoles.jsx";

// /settings/consoles/<console>/status : LE STATUT DE TOUTE LA CONSOLE (chantier 30 ; chantier 56, A2 — le porteur,
// 2026-11-14 : « Oui ») — ConsoleStatusPanel de la webapp. Chaque jeu sans statut à lui prend celui-ci, y compris un jeu
// ajouté plus tard ; un jeu signalé garde le sien. D'abord ce qui vaut aujourd'hui (posé par qui et quand, ou le défaut
// de la console), puis le sélecteur et son bouton dans l'encadré, « ••• » tant que rien ne change. La liste gardée de la
// console s'oublie (forgetHere) : ses jeux se relisent à la prochaine bibliothèque.
export default function ConsoleStatus() {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { consoleId } = useParams();
    const { consoles, forgetHere } = useData();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [answer, setAnswer] = useState(undefined);
    const [draft, setDraft] = useState(null);
    const [pending, setPending] = useState(false);

    useEffect(() => {
        let cancelled = false;
        setAnswer(undefined);
        setDraft(null);
        fetchConsoleStatus(consoleId).then((read) => !cancelled && setAnswer(read));

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    const current = answer?.ok ? answer.data || {} : null;
    const effective = current ? current.status || current.default || "untested" : "untested";
    const chosen = draft ?? effective;
    const dated = (at) => (at ? new Date(at).toLocaleString(i18n.language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");
    const badge = (status) => <span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>;

    const save = async () => {
        setPending(true);
        const written = await setConsoleStatus(consoleId, chosen);
        setPending(false);

        if (!written.ok) {
            feedback.failed(written.errorMessage || t("settings.actionFailed"));

            return;
        }
        setAnswer(written);
        setDraft(null);
        forgetHere(consoleId);
        feedback.success(t("consoles.statusSaved", { status: t(`status.${chosen}`) }));
    };

    return (
        <SettingsPage title={t("consoles.status")} parent={consolePath(consoleId)} levels={{ [consolePath(consoleId)]: entry?.name }} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            {consoles && !entry ? <NotFound /> : !current ? (
                <Section><Note className="mt-0">{answer === undefined ? t("settings.loading") : answer.errorMessage || t("settings.actionFailed")}</Note></Section>
            ) : (
                <>
                    <Section title={t("consoles.statusNow")} sense="modify">
                        <Rows>
                            <InfoRow icon="bi-patch-check" label={t("games.status")} value={badge(effective)}
                                help={current.status ? t("consoles.statusSetBy", { by: current.by, at: dated(current.at) }) : t("consoles.statusDefault")} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.statusChange")} help={t("consoles.statusHelp")} sense="modify">
                        <Form props={{ onSubmit: (event) => {
                            event.preventDefault();

                            if (chosen !== effective && !pending) {
                                save();
                            }
                        } }}>
                            <Rows>
                                <SelectRow id="cst-status" label={t("games.status")} value={chosen} disabled={pending} onChange={(value) => setDraft(value)}
                                    options={GAME_STATUSES.map((status) => ({ value: status, label: t(`status.${status}`) }))} />
                                <SubmitRow>
                                    <FormButton id="cst-save" label={t("settings.save")} isReplaced={chosen !== effective} pending={pending} />
                                </SubmitRow>
                            </Rows>
                        </Form>
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}
