// Route : /desktop/update — LA MISE À JOUR, COMME L'APK (UpdatePage) : la version en service ; chercher, puis — plus
// récente — l'installer, sa progression sur la ligne (update.go suit le téléchargement dans sa réponse) ; installée,
// « Redémarrer GameFlex ».
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { formatSize } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { call, DesktopPage, useDesktopState } from "./shared.jsx";

export default function Update() {
    const { state } = useDesktopState();
    const { t } = useTranslation();
    const [check, setCheck] = useState(null);
    const [busy, setBusy] = useState(false);
    const [progress, setProgress] = useState(null);
    const [note, setNote] = useState(null);
    const [installed, setInstalled] = useState(null);

    const search = async () => {
        setBusy(true);
        setNote(t("desktop.updateSearching"));
        const answer = await call("/update");
        setBusy(false);
        setCheck(answer);
        setNote(!answer.ok || answer.error ? t("desktop.updateFailed", { detail: answer.error || answer.status })
            : answer.newer ? t("desktop.updateAvailable", { version: answer.offer.version, size: formatSize(answer.offer.size) }) : t("desktop.updateLatest"));
    };

    const install = async () => {
        setBusy(true);
        setProgress(-1);
        try {
            const response = await fetch("/api/v1/desktop/update", { method: "POST" });

            if (!response.ok) {
                const answer = await response.json().catch(() => ({}));
                throw new Error(answer.code === "update_not_appimage" ? t("desktop.updateNotAppImage") : answer.detail || answer.code || String(response.status));
            }
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffered = "";
            for (;;) {
                const { value, done } = await reader.read();

                if (done) {
                    break;
                }
                buffered += decoder.decode(value, { stream: true });
                const lines = buffered.split("\n");
                buffered = lines.pop();
                for (const line of lines.filter(Boolean)) {
                    const event = JSON.parse(line);

                    if (event.error) {
                        throw new Error(event.error);
                    }
                    if (event.done) {
                        setInstalled(event.version);
                    } else {
                        setProgress(event.total ? Math.min(100, Math.round((event.received / event.total) * 100)) : -1);
                        setNote(event.total ? t("desktop.updateDownloadingOf", { received: formatSize(event.received), total: formatSize(event.total) })
                            : t("desktop.updateDownloading", { received: formatSize(event.received) }));
                    }
                }
            }
        } catch (error) {
            setNote(t("desktop.updateFailed", { detail: error.message }));
        }
        setProgress(null);
        setBusy(false);
    };

    const offered = check?.newer && !installed;

    return (
        <DesktopPage title={t("desktop.update")} guides={[["update"]]}>
            <Section title={t("desktop.thisApp")} help={t("desktop.thisAppHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-tag" label={t("desktop.version")} value={state.version} />
                </Rows>
            </Section>
            <Section title={t("desktop.update")} help={check && !check.appimage ? t("desktop.updateNotAppImage") : t("desktop.updateSectionHelp")} sense="play">
                <Rows>
                    {installed ? (
                        <NavRow id="dc-relaunch" icon="bi-arrow-clockwise" label={t("desktop.relaunch")} help={t("desktop.relaunchHelp", { version: installed })} opens="action"
                            onOpen={async () => {
                                const answer = await call("/relaunch", "POST", {});

                                if (!answer.ok) {
                                    toast(t("settings.actionFailed"));
                                }
                            }} />
                    ) : (
                        <NavRow id="dc-update-action" icon={offered ? "bi-download" : "bi-cloud-arrow-down"}
                            label={offered ? t("desktop.updateInstall", { version: check.offer.version }) : t("desktop.updateCheck")} help={note} opens="action"
                            pending={busy && progress === null} progress={progress} disabled={offered && !check.appimage} onOpen={offered ? install : search} />
                    )}
                </Rows>
            </Section>
        </DesktopPage>
    );
}
