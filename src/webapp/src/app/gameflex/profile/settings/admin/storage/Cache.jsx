import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import { formatSize } from "../api/library.js";
import { fetchCache, deleteCacheEntry } from "../api/storage-admin.js";

const ITEMS_PER_PAGE = 10;

// La nature d'une entrée, telle que l'écran la nomme.
const KIND_LABELS = {
    "unpacked": "admin.storage.cacheKindUnpacked",
    "firmware": "admin.storage.cacheKindFirmware",
    "game-data": "admin.storage.cacheKindGameData",
    "compilation": "admin.storage.cacheKindCompilation",
    "shaders": "admin.storage.cacheKindShaders",
    "other": "admin.storage.cacheKindOther",
};

// LE CACHE DU DOSSIER (#448) — ce que le premier lancement d'un jeu installe,
// une fois pour tous les comptes (`cache/installs/<console>/`) : un jeu PS4
// déballé, le firmware et les données installées de la PS3. Le porteur,
// 2026-10-24 : « ça évitera de grossir le stockage sans savoir d'où ça vient ».
//
// Tout s'y refait, d'où un seul geste : retirer une entrée. Il est refusé
// pendant une partie de sa console (elle lit ce cache), et la confirmation
// dit quand un déballé est la seule copie d'un jeu dont le .pkg a disparu.
export default function Cache({ t }) {
    const [values, setValues] = useState({ q: "" });
    const [entries, setEntries] = useState([]);
    const [total, setTotal] = useState(0);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);

    const latestContext = useLatestContext();
    const context = latestContext.current;

    const reload = useCallback(() => {
        setLoading(true);

        return fetchCache()
            .then((result) => {
                if (!result.ok) {
                    latestContext.current.toast.open(result.errorMessage || t("admin.storage.loadFailed"),
                        { variant: "error" });
                }
                // Une ligne par entrée, sa console avec elle.
                setEntries((result.consoles || []).flatMap((console) => (console.entries || []).map((entry) => ({
                    ...entry,
                    consoleId: console.consoleId,
                    console: console.console,
                }))));
                setTotal(result.sizeBytes || 0);
            })
            .catch((failure) => {
                console.error("Chargement du cache", failure);
                latestContext.current.toast.open(t("admin.storage.loadFailed"), { variant: "error" });
            })
            .finally(() => setLoading(false));
    }, [t, latestContext]);

    useEffect(() => {
        reload();
    }, [reload]);

    const filtered = useMemo(() => {
        const needle = (values.q || "").trim().toLowerCase();

        return needle
            ? entries.filter((entry) => `${entry.label} ${entry.name} ${entry.console}`.toLowerCase().includes(needle))
            : entries;
    }, [entries, values.q]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    useEffect(() => {
        const last = Math.max(1, Math.ceil(filtered.length / ITEMS_PER_PAGE));
        if (page > last) {
            setPage(last);
        }
    }, [filtered.length, page]);

    const handleDelete = async (entry) => {
        const result = await deleteCacheEntry(entry.consoleId, entry.name);
        context.toast.open(
            result.ok
                ? t("admin.storage.cacheDeleted", { name: entry.label })
                : (result.errorMessage || t("admin.storage.deleteFailed")),
            { variant: result.ok ? "success" : "error" },
        );
        if (result.ok) {
            setEntries((current) => current.filter((other) => !(other.consoleId === entry.consoleId
                && other.name === entry.name)));
            setTotal((current) => Math.max(0, current - entry.sizeBytes));
        }
    };

    // Ce qu'on lit d'une entrée : sa console, sa nature, sa taille, puis ce
    // qui la refait, ou ce qui empêche de la retirer.
    const describe = (entry) => [
        entry.console,
        t(KIND_LABELS[entry.kind] || KIND_LABELS.other),
        formatSize(entry.sizeBytes),
        entry.source === "present" ? t("admin.storage.cacheSourcePresent") : null,
        entry.source === "missing" ? t("admin.storage.cacheSourceMissing") : null,
        entry.inUse ? t("admin.storage.cacheInUse") : null,
    ].filter(Boolean).join(" · ");

    const searchContext = { values, setValues };

    return (
        <SectionPanel>
            <ListingElement
                callback={(entry) => (
                    <ListElement
                        key={`${entry.consoleId}/${entry.name}`}
                        icon="bi bi-hdd-stack-fill"
                        label={entry.label}
                        helpText={describe(entry)}
                        next={(
                            <bs.Button
                                variant="danger"
                                disabled={entry.inUse}
                                title={entry.inUse ? t("admin.storage.cacheInUse") : t("app.delete")}
                                onClick={(e) => {
                                    e.preventDefault();
                                    e.stopPropagation();
                                    context.modal.open(
                                        <div className="text-center">
                                            {t("admin.storage.deleteCacheConfirm")} <br />
                                            {entry.source === "missing" && (
                                                <>{t("admin.storage.cacheLastCopy")} <br /></>
                                            )}
                                            <strong>
                                                {entry.label} — {formatSize(entry.sizeBytes)}
                                            </strong>
                                        </div>,
                                        {
                                            variant: "confirm",
                                            onConfirm: () => handleDelete(entry),
                                            confirmLabel: t("app.delete"),
                                            cancelLabel: t("app.cancel"),
                                        },
                                    );
                                }}
                            >
                                <i className="bi bi-trash"></i>
                            </bs.Button>
                        )}
                    />
                )}
                items={visible}
                itemCount={filtered.length}
                itemsPerPage={ITEMS_PER_PAGE}
                currentPage={page}
                onPositionChange={setPage}
                isLoading={isLoading}
                search={{
                    context: searchContext,
                    placeholder: t("admin.storage.searchCache"),
                }}
                texts={{
                    itemsFound: t("admin.storage.cacheFound", { size: formatSize(total) }),
                    noItemsFound: t("admin.storage.noCache"),
                }}
            />
        </SectionPanel>
    );
}
