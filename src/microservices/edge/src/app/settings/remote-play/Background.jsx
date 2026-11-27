// Route : /settings/remote-play/<id>/background — son fond d'écran.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { resetConsoleBackgroundSlot, searchConsoleBackgrounds, setConsoleBackground } from "../../libs/api.js";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { ImageSearch } from "../server/consoles/ImageSearch.jsx";
import { backgroundConsole, deviceName, forgetRemoteBackground, RemoteBackgroundImage, ROOT, useRemoteBackground } from "../RemotePlay.jsx";
import { DevicePage } from "./Device.jsx";

// /settings/remote-play/<id>/background : L'IMAGE DE FOND D'UNE CONSOLE — « L'image actuelle », puis la recherche chez
// les fournisseurs (comme un emplacement de l'apparence d'une console : une image choisie, la fenêtre, puis le serveur la
// récupère dans le premier emplacement), puis « Danger » : la remettre par défaut (le remplissage par défaut la refait
// à la prochaine visite). Une image prêtée par un serveur fédéré dit sa provenance dans « Détails », comme la page d'un
// emplacement de l'apparence d'une console (Consoles.jsx, SlotPage ; #748).
function BackgroundView({ device }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { back } = useSettingsNavigation();
    const consoleId = backgroundConsole(device);
    const [epoch, setEpoch] = useState(0);
    const background = useRemoteBackground(device, epoch);
    const name = background?.name;
    const parent = `${ROOT}/${encodeURIComponent(device.id)}`;

    const reset = async () => {
        const answer = await resetConsoleBackgroundSlot(consoleId, 1);

        if (!answer.ok) {
            feedback.failed(answer.errorMessage || t("settings.actionFailed"));

            return;
        }
        forgetRemoteBackground(consoleId);
        feedback.success(t("rp.backgroundResetDone"));
        setEpoch((value) => value + 1);
    };

    return (
        <SettingsPage title={t("rp.background")} parent={parent} levels={{ [parent]: deviceName(device) }} guides={[["remotePlay"]]}>
            {name && (
                <Section title={t("appearance.current")} help={t("rp.backgroundHelp", { console: device.hostType })} sense="info">
                    <RemoteBackgroundImage consoleId={consoleId} name={name} server={background.server} />
                </Section>
            )}
            {name && background.server && (
                <Section title={t("details")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-arrow-left-right" label={t("game.origin")} value={background.serverName} />
                    </Rows>
                </Section>
            )}
            <Section title={t("appearance.searchTitle")} help={t("appearance.searchConsoleHelp")} sense="modify">
                <ImageSearch fit="cover" search={(query) => searchConsoleBackgrounds(consoleId, query)}
                    confirmOf={(image) => ({
                        title: t("rp.backgroundConfirmTitle"),
                        message: t("appearance.chosenConfirm", { game: image.game, source: t(`images.services.${image.source}`) }),
                        confirmLabel: t("appearance.replaceAction"),
                    })}
                    success={() => t("rp.backgroundSet")}
                    onChoose={async (image) => {
                        const answer = await setConsoleBackground(consoleId, 1, image);

                        if (answer.ok) {
                            forgetRemoteBackground(consoleId);
                            back(parent);
                        }

                        return answer;
                    }} />
            </Section>
            <DangerSection id="rp-background-reset" icon="bi-arrow-counterclockwise" label={t("rp.backgroundReset")} question={t("rp.backgroundResetConfirm")}
                confirmLabel={t("rp.backgroundResetAction")} onConfirm={reset} />
        </SettingsPage>
    );
}

export default function Background() {
    return <DevicePage View={BackgroundView} />;
}
