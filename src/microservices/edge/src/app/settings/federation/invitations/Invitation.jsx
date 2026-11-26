// Route : /settings/federation/invitations/<id> — une invitation en attente.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { revokeInvitation } from "../../../libs/api.js";
import SettingsPage, { useSettingsNavigation } from "../../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../../profile/Federation.jsx";
import { dateTime, invitationKind, loadInvitations, PENDING, useLevels, usePassed, Waiting } from "../../Federation.jsx";

// UNE INVITATION EN ATTENTE : pour qui, ce qu'elle ouvre, jusqu'à quand ; la révoquer.
export default function Invitation() {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { invitationId } = useParams();
    const { back } = useSettingsNavigation();
    const invitation = usePassed("invitation", invitationId, () => loadInvitations());
    const levels = useLevels();

    if (!invitation) {
        return <Waiting item={invitation} title={t("fed.pendingTitle")} parent={PENDING} />;
    }
    const label = invitation.label || t("invitations.kindAccount");

    return (
        <SettingsPage title={label} parent={PENDING} levels={levels} guides={FEDERATION_GUIDES}>
            <Section title={t("invitations.detailTitle")} help={t("fed.invitationDetailHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-person" label={t("fed.forWho")} value={invitation.label} />
                    <InfoRow icon="bi-unlock" label={t("invitations.kindLabel")} value={invitationKind(invitation, t)} />
                    <InfoRow icon="bi-hourglass-split" label={t("invitations.expiresLabel")} value={dateTime(invitation.expires_at, i18n.language)} />
                </Rows>
            </Section>
            <DangerSection id={`fd-inv-revoke-${invitation.id}`} help={t("invitations.revokeHelp")} label={t("invitations.revokeTitle")} confirmLabel={t("fed.revoke")}
                question={`${t("invitations.revokeConfirm")} ${invitation.label || ""}`} onConfirm={async () => {
                    const answer = await revokeInvitation(invitation.id);
                    feedback.result(answer.ok, t("invitations.revoked", { name: invitation.label }), answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        back(PENDING);
                    }
                }} />
        </SettingsPage>
    );
}
