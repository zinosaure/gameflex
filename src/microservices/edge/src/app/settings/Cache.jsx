// LES PAGES D'ADMINISTRATION PORTÉES DE LA WEBAPP (profile/settings/admin) : le même
// comportement et les mêmes routes d'API, à la manière du launcher — une page des réglages
// chacune (SettingsPage.jsx), composée des briques de la direction C (bricks/) : ses groupes, ses
// lignes, ses champs, sa recherche et sa pagination ceux de Skeleton (Forms.jsx), la croix y circule, FR/EN.
// Un geste qui retire quelque chose est une ligne de « Danger », en bas de sa page, et se confirme dans la
// fenêtre de Skeleton, comme dans la webapp ; un geste qui n'appartient à aucun groupe est une ligne de « Plus »
// (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens »).

import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Section } from "../bricks/Section.jsx";
import { fetchCache } from "../libs/api.js";
import { fact, formatSize } from "../libs/format.js";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";

// ─── Cache (storage/Cache.jsx) ───────────────────────────────────────────────
// Ce que le premier lancement d'un jeu installe, une fois pour tous les comptes
// (`cache/installs/<console>/`). Tout s'y refait : un seul geste, retirer une entrée —
// refusé pendant une partie de sa console, et la confirmation dit quand un déballé est
// la seule copie d'un jeu dont le .pkg a disparu. Une entrée de la liste s'ouvre
// (/settings/cache/<console>/<entrée>) : le geste est dans sa page, jamais sur la ligne.
export const CACHE_KINDS = ["unpacked", "firmware", "game-data", "compilation", "shaders"];

export const readCache = () => fetchCache().then((answer) => ({
    ok: answer.ok,
    errorMessage: answer.errorMessage,
    total: answer.data?.sizeBytes || 0,
    entries: (answer.data?.consoles || []).flatMap((console) => (console.entries || []).map((entry) => ({ ...entry, consoleId: console.consoleId, console: console.console }))),
}));
export const cacheTitle = (entry) => entry.label.split("\n")[0];
export const cacheKind = (entry, t) => t(`settings.cacheKinds.${CACHE_KINDS.includes(entry.kind) ? entry.kind : "other"}`);

// Route : /settings/cache — le cache.
export default function Cache() {
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
            <Section title={t("settings.cacheEntries")} sense="info">
                <Listing items={filtered} loading={loading} search={search} onSearch={(q) => update({ q, page: null })} page={read("page")} onPage={(page) => update({ page: page > 1 ? page : null })}
                    placeholder={t("settings.searchCache")} found={t("settings.cacheFound", { count: filtered.length, size: formatSize(total) })} empty={t("settings.noCache")}
                    render={(entry) => {
                        const key = `cache-open-${entry.consoleId}-${entry.name}`;

                        return (
                            <NavRow key={key} id={key} icon="bi-hdd-stack" label={cacheTitle(entry)} checked={Boolean(entry.inUse)} help={[fact(t("settings.cacheConsole"), entry.console), fact(t("settings.cacheSize"), formatSize(entry.sizeBytes))]}
                                onOpen={() => open(`/settings/cache/${encodeURIComponent(entry.consoleId)}/${encodeURIComponent(entry.name)}`, key, { entry })} />
                        );
                    }} />
            </Section>
            <GuideSection guides={[["storage", "cache"]]} />
        </SettingsPage>
    );
}
