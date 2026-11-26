import React, { useEffect, useState } from "react";
import { useFeedback } from "../bricks/useFeedback.jsx";
import { useTranslation } from "react-i18next";

import { CopyField } from "../bricks/CopyField.jsx";
import { DangerSection } from "../bricks/DangerSection.jsx";
import { Loading } from "../bricks/Loading.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";
import { fetchCastSettings, saveCastSettings } from "../libs/api.js";
import { useShareOrigin } from "../screens/access/Invitation.jsx";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { useSettingsNavigation } from "./SettingsPage.jsx";

// GOOGLE CAST, DANS L'ADMINISTRATION (le porteur, 2026-11-24 : « D'ailleurs ça devrait être dans l'administration
// directement et non plus dans le launch.sh ») : l'identifiant de l'application réceptrice enregistrée chez Google, gardé
// par le service des comptes (cast.go, `/api/v1/cast`) — sur un serveur comme sur le PC.
//   - /settings/cast : le récepteur — son identifiant (la ligne ouvre sa page), l'adresse à enregistrer chez Google ;
//     puis « Danger » : retirer l'identifiant ;
//   - /settings/cast/application : le champ, « Enregistrer ».
// Sans identifiant, la page d'une partie ne propose pas le cast.
export const CAST = "/settings/cast";

export function useCastApplication() {
    const { t } = useTranslation();
    const [applicationId, setApplicationId] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchCastSettings().then((answer) => {
            if (cancelled) {
                return;
            }
            if (!answer.ok) {
                toast(t("settings.actionFailed"));
            }
            setApplicationId(answer.ok ? answer.item?.application_id || "" : "");
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return applicationId;
}

// Route : /settings/cast — Google Cast.
export default function Cast() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { open } = useSettingsNavigation();
    const applicationId = useCastApplication();
    const [removed, setRemoved] = useState(false);
    const current = removed ? "" : applicationId;
    // L'adresse que la TV joint : celle que l'hôte déclare pour être partagée (`shareOrigin` — sur le téléphone, son
    // adresse sur le réseau, jamais le 127.0.0.1 de sa WebView), comme Inviter.
    const receiver = `${useShareOrigin()}/cast-receiver.html`;

    return (
        <SettingsPage title={t("settings.items.cast.label")} guides={[["remoteAccess"]]}>
            {current === undefined ? <Section><Loading /></Section> : (
                <>
                    <Section title={t("settings.cast.receiver")} help={t("settings.cast.receiverHelp")} sense="modify">
                        <Rows>
                            <NavRow id="cast-application" icon="bi-cast" label={t("settings.cast.applicationId")}
                                value={current || t("settings.notConfigured")} onOpen={() => open(`${CAST}/application`, "cast-application")} />
                            <CopyField id="cast-receiver-url" label={t("settings.cast.receiverUrl")} help={t("settings.cast.receiverUrlHelp")} value={receiver} />
                        </Rows>
                    </Section>
                    <DangerSection id="cast-remove" label={t("settings.cast.remove")} help={t(current ? "settings.cast.removeHelp" : "settings.cast.nothingStored")}
                        question={t("settings.cast.removeConfirm")} disabled={!current} onConfirm={async () => {
                            const answer = await saveCastSettings("");

                            if (answer.ok) {
                                setRemoved(true);
                                feedback.success(t("settings.cast.removed"));
                            } else {
                                feedback.failed(t("settings.actionFailed"));
                            }
                        }} />
                </>
            )}
        </SettingsPage>
    );
}
