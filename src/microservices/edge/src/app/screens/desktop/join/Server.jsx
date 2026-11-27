// Route : /desktop/join/:origin — UN SERVEUR REJOINT : son adresse, le rouvrir, l'oublier (« Danger »).
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { EmptyRow } from "../../../bricks/EmptyRow.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { call, DesktopPage, useDesktopState, useJoin } from "../shared.jsx";

export default function JoinedServer() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const navigate = useNavigate();
    const { state, setState } = useDesktopState();
    const origin = decodeURIComponent(useParams().origin || "");
    const server = (state.joined || []).find((candidate) => candidate.origin === origin);
    const join = useJoin();
    const [pending, setPending] = useState(false);
    const guides = [["remoteAccess"]];

    if (!server) {
        return (
            <DesktopPage title={t("desktop.join")} parent="/desktop/join" guides={guides}>
                <Section title={t("desktop.details")} help={t("desktop.joinedServerHelp")} sense="info"><EmptyRow icon="bi-hdd-network" text={t("desktop.joinedNone")} /></Section>
            </DesktopPage>
        );
    }

    return (
        <DesktopPage title={server.name} parent="/desktop/join" guides={guides}>
            <Section title={t("desktop.details")} help={t("desktop.joinedServerHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-link-45deg" label={t("desktop.address")} value={server.origin} mono />
                </Rows>
            </Section>
            <Section title={t("desktop.more")} help={t("desktop.joinedMoreHelp")} sense="play">
                <Rows>
                    <NavRow id="dc-joined-open" icon="bi-box-arrow-in-right" label={t("desktop.joinReopen")} help={t("desktop.joinReopenHelp")} opens="action" pending={pending}
                        onOpen={async () => {
                            setPending(true);
                            await join(server.origin);
                            setPending(false);
                        }} />
                </Rows>
            </Section>
            <DangerSection id="dc-joined-remove" label={t("desktop.joinRemove")} help={t("desktop.joinRemoveHelp")} question={t("desktop.joinRemoveConfirm")}
                confirmLabel={t("desktop.remove")} onConfirm={async () => {
                    const answer = await call("/join", "DELETE", { origin: server.origin });

                    feedback.result(answer.ok, t("desktop.joinRemoved"), t("settings.actionFailed"));
                    if (answer.ok) {
                        setState(answer);
                        navigate("/desktop/join");
                    }
                }} />
        </DesktopPage>
    );
}
