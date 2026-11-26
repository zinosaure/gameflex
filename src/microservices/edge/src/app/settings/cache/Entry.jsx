// Route : /settings/cache/<console>/<nom> — une entrée du cache.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { deleteCacheEntry } from "../../libs/api.js";
import { formatSize } from "../../libs/format.js";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { cacheKind, cacheTitle, readCache } from "../Cache.jsx";
import { CONSOLE_ICON } from "../../globals/icons.js";

export default function CacheEntry() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId, name } = useParams();
    const location = useLocation();
    const { back } = useSettingsNavigation();
    const [entry, setEntry] = useState(() => location.state?.entry || undefined);
    const parent = "/settings/cache";

    // Un lien direct, un rechargement : l'entrée relue dans le cache.
    useEffect(() => {
        if (entry === undefined) {
            readCache().then((answer) => setEntry(answer.entries.find((other) => other.consoleId === consoleId && other.name === name) || null));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, name]);

    return (
        <SettingsPage title={entry ? cacheTitle(entry) : name} parent={parent} guides={[["storage", "cache"]]}>
            {entry === undefined ? <Section><Loading /></Section> : entry === null ? <NotFound /> : (
                <>
                    <Section title={t("details")} sense="info">
                        <Rows>
                            <InfoRow icon={CONSOLE_ICON} label={t("settings.cacheConsole")} value={entry.console} />
                            <InfoRow icon="bi-tag" label={t("settings.cacheKind")} value={cacheKind(entry, t)} />
                            <InfoRow icon="bi-hdd" label={t("settings.cacheSize")} value={formatSize(entry.sizeBytes)} />
                            {entry.inUse && <InfoRow icon="bi-activity" label={t("facts.state")} value={t("settings.cacheInUse")} />}
                            <InfoRow icon="bi-folder2" label={t("settings.cacheFolder")} value={entry.name} mono />
                        </Rows>
                        {entry.label.split("\n").slice(1).filter(Boolean).map((line) => <Note key={line}>{line}</Note>)}
                        {entry.source === "present" && <Note>{t("settings.cacheSourcePresent")}</Note>}
                        {entry.source === "missing" && <Note>{t("settings.cacheSourceMissing")}</Note>}
                    </Section>
                    <DangerSection id="cache-remove" help={entry.inUse ? t("settings.cacheInUse") : t("settings.cacheRemoveHelp")} label={t("settings.remove")}
                        disabled={entry.inUse} question={entry.source === "missing" ? t("settings.cacheLastCopy") : t("settings.cacheConfirm")}
                        onConfirm={async () => {
                            const answer = await deleteCacheEntry(entry.consoleId, entry.name);

                            if (answer.ok) {
                                feedback.success(t("settings.cacheDeleted", { name: cacheTitle(entry) }));
                                back(parent);
                            } else {
                                feedback.failed(answer.errorMessage || t("settings.actionFailed"));
                            }
                        }} />
                </>
            )}
        </SettingsPage>
    );
}
