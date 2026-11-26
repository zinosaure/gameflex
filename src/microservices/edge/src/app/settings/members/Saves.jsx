// Route : /settings/members/<id>/saves — ses sauvegardes, de loin.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { deleteUserSaves, fetchUserSaves, userSavesArchiveUrl } from "../../libs/api.js";
import { counted, formatSize } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import { useSettingsNavigation } from "../SettingsPage.jsx";
import { MEMBERS, MemberSubPage } from "../Members.jsx";

// UN TÉLÉCHARGEMENT, comme le lien qu'il était (un onglet à part, que le serveur sert en pièce jointe) : une ligne
// n'est pas un lien, elle en ouvre un (le même geste que GameSettings.jsx).
const openDownload = (href) => {
    const link = document.createElement("a");
    link.href = href;
    link.target = "_blank";
    link.rel = "noreferrer";
    document.body.appendChild(link);
    link.click();
    link.remove();
};

// /settings/members/<id>/saves : SES SAUVEGARDES (storage/Saves.jsx de la webapp ; le porteur, 2026-11-14 : « rajoute
// aussi un lien pour voir les sauvegardes de l'utilisateur (ce qui fait qu'on enlève "user's saves" dans settings) ») —
// de loin, volontairement : la place qu'occupe son dossier (`saves/<identifiant>`), et deux gestes — l'emporter
// (l'archive), ou la reprendre (tout supprimer ; le serveur efface en fond, d'où « lancée »). Elles ne dépendent pas du
// compte : la page s'ouvre aussi pour root et pour soi.
function MemberSaves({ member }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { back } = useSettingsNavigation();
    const [saves, setSaves] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchUserSaves().then((answer) => {
            if (cancelled) {
                return;
            }

            if (!answer.ok) {
                toast(answer.errorMessage || t("settings.loadFailed"));
            }
            setSaves((answer.data?.users || []).find((entry) => entry.username === member.username) || null);
        });

        return () => {
            cancelled = true;
        };
    }, [member.username, t]);

    if (saves === undefined) {
        return <Section><Loading /></Section>;
    }
    const none = !saves || saves.files === 0;
    const unavailable = none ? t("members.savesNone") : null;

    return (
        <>
            <Section title={t("details")} help={t("members.savesDetailsHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-files" label={t("members.savesFiles")} value={counted(t, "files", saves?.files || 0)} />
                    <InfoRow icon="bi-hdd" label={t("members.savesSize")} value={formatSize(saves?.sizeBytes || 0)} />
                    {saves?.deleting && <InfoRow icon="bi-hourglass-split" label={t("members.savesState")} value={t("settings.savesDeleting")} />}
                </Rows>
            </Section>
            <Section title={t("more")} help={t("members.savesMoreHelp")} sense="play">
                <Rows>
                    <NavRow id="saves-download" icon="bi-download" label={t("settings.download")} help={unavailable || t("settings.savesDownloadHelp")} opens="action"
                        disabled={none} onOpen={() => openDownload(userSavesArchiveUrl(member.username))} />
                </Rows>
            </Section>
            {/* Rien à supprimer, ou un effacement en cours : grisée, et pourquoi. */}
            <DangerSection id="saves-delete" help={saves?.deleting ? t("settings.savesDeleting") : unavailable || t("settings.deleteSavesHelp")} label={t("settings.deleteAll")}
                question={t("settings.deleteSavesConfirm")} disabled={none || Boolean(saves?.deleting)} onConfirm={async () => {
                    const answer = await deleteUserSaves(member.username);

                    if (!answer.ok) {
                        feedback.failed(answer.errorMessage || t("settings.actionFailed"));

                        return;
                    }
                    feedback.success(t(answer.data?.finished ? "settings.savesDeleted" : "settings.savesDeleteStarted", { user: member.username }));
                    back(`${MEMBERS}/${member.id}`);
                }} />
        </>
    );
}

export default function MemberSavesPage() {
    const { t } = useTranslation();

    return <MemberSubPage title={t("members.saves")} render={(member) => <MemberSaves member={member} />} />;
}
