import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { deleteCacheEntry, fetchCache } from "../../libs/api.js";
import { formatSize } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { DangerSection, GuideSection, InfoRow, Listing, Note, Rows, Section } from "../../ui/Bricks.jsx";

// LES PAGES D'ADMINISTRATION PORTÉES DE LA WEBAPP (profile/settings/admin) : le même
// comportement et les mêmes routes d'API, à la manière du launcher — une page des réglages
// chacune (SettingsPage.jsx), composée des briques de la direction C (Bricks.jsx) : ses groupes, ses
// lignes, ses champs, sa recherche et sa pagination ceux de Skeleton (Forms.jsx), la croix y circule, FR/EN.
// Un geste qui retire quelque chose est une ligne de « Danger », en bas de sa page, et se confirme dans la
// fenêtre de Skeleton, comme dans la webapp ; un geste qui n'appartient à aucun groupe est une ligne de « Plus »
// (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens »).

// ─── Cache (storage/Cache.jsx) ───────────────────────────────────────────────
// Ce que le premier lancement d'un jeu installe, une fois pour tous les comptes
// (`cache/installs/<console>/`). Tout s'y refait : un seul geste, retirer une entrée —
// refusé pendant une partie de sa console, et la confirmation dit quand un déballé est
// la seule copie d'un jeu dont le .pkg a disparu. Une entrée de la liste s'ouvre
// (/settings/cache/<console>/<entrée>) : le geste est dans sa page, jamais sur la ligne.
const CACHE_KINDS = ["unpacked", "firmware", "game-data", "compilation", "shaders"];

const readCache = () => fetchCache().then((answer) => ({
    ok: answer.ok,
    errorMessage: answer.errorMessage,
    total: answer.data?.sizeBytes || 0,
    entries: (answer.data?.consoles || []).flatMap((console) => (console.entries || []).map((entry) => ({ ...entry, consoleId: console.consoleId, console: console.console }))),
}));
const cacheTitle = (entry) => entry.label.split("\n")[0];
const cacheKind = (entry, t) => t(`settings.cacheKinds.${CACHE_KINDS.includes(entry.kind) ? entry.kind : "other"}`);

function CacheListPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState();
    const [entries, setEntries] = useState([]);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const search = read("q");

    useEffect(() => {
        readCache().then((answer) => {
            if (!answer.ok) {
                toast(answer.errorMessage || t("settings.loadFailed"));
            }
            setEntries(answer.entries);
            setTotal(answer.total);
            setLoading(false);
        });
    }, [t]);

    const filtered = useMemo(() => {
        const needle = search.trim().toLowerCase();

        return needle ? entries.filter((entry) => `${entry.label} ${entry.name} ${entry.console}`.toLowerCase().includes(needle)) : entries;
    }, [entries, search]);

    return (
        <SettingsPage title={t("settings.items.cache.label")}>
            <Section title={t("settings.cacheEntries")}>
                <Listing items={filtered} loading={loading} search={search} onSearch={(q) => update({ q, page: null })} page={read("page")} onPage={(page) => update({ page: page > 1 ? page : null })}
                    placeholder={t("settings.searchCache")} found={t("settings.cacheFound", { count: filtered.length, size: formatSize(total) })} empty={t("settings.noCache")}
                    render={(entry) => {
                        const key = `cache-open-${entry.consoleId}-${entry.name}`;

                        return (
                            <NavRow key={key} id={key} icon="bi-hdd-stack" tone="slate" label={cacheTitle(entry)} help={[entry.console, cacheKind(entry, t), formatSize(entry.sizeBytes), entry.inUse ? t("settings.cacheInUse") : null].filter(Boolean).join(" · ")}
                                onOpen={() => open(`/settings/cache/${encodeURIComponent(entry.consoleId)}/${encodeURIComponent(entry.name)}`, key, { entry })} />
                        );
                    }} />
            </Section>
            <GuideSection guides={[["storage", "cache"]]} />
        </SettingsPage>
    );
}

function CacheEntryPage() {
    const { t } = useTranslation();
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
        <SettingsPage title={entry ? cacheTitle(entry) : name} parent={parent}>
            {entry === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : entry === null ? <NotFound /> : (
                <>
                    <Section title={t("details")}>
                        <Rows>
                            <InfoRow icon="bi-joystick" label={t("settings.cacheConsole")} value={entry.console} />
                            <InfoRow icon="bi-tag" label={t("settings.cacheKind")} value={cacheKind(entry, t)} />
                            <InfoRow icon="bi-hdd" label={t("settings.cacheSize")} value={formatSize(entry.sizeBytes)} />
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
                                toast(t("settings.cacheDeleted", { name: cacheTitle(entry) }));
                                back(parent);
                            } else {
                                toast(answer.errorMessage || t("settings.actionFailed"));
                            }
                        }} />
                </>
            )}
        </SettingsPage>
    );
}

export function CacheRoutes() {
    return (
        <Routes>
            <Route index element={<CacheListPage />} />
            <Route path=":consoleId/:name" element={<CacheEntryPage />} />
            <Route path="*" element={<Navigate to="/settings/cache" replace />} />
        </Routes>
    );
}
