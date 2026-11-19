import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    DownloadRing,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import { formatSize } from "../api/library.js";
import { parseSource, importFromSource, fetchPatches, deletePatch } from "../api/storage-admin.js";
import SourceSelector, { useDiskSources } from "./SourceSelector.jsx";
import MirrorNotice from "./MirrorNotice.jsx";
import { federatedServers } from "../../../../libs/federatedHost.js";

const ITEMS_PER_PAGE = 20;

// LES CORRECTIFS (chantier 15, décidé par le porteur le 2026-10-13) —
// `patches/<console>/<nom>` : d'ordinaire le dossier d'un jeu, qui porte ses
// packs (`wiiu/NewSuperMarioBrosU/CrashFix/`).
//
// Le gabarit de Jeux, Émulateurs et Firmwares : le sélecteur de source, puis
// la liste. Ici, la corbeille ; chez un serveur fédéré ou sur un disque
// interne, le rapatriement — ou la coche quand on l'a déjà. Rien n'écrase
// rien : pour reprendre une version, on supprime d'abord. Lus dans le listing
// de la source (celui d'ici, la copie de celui d'un ami, ou le disque).
export default function Patches({ t }) {
    // Le contexte du squelette change d'un rendu à l'autre : `reload` lit le
    // dernier, sans en dépendre (sinon il relirait en boucle).
    const latestContext = useLatestContext();
    const [source, setSource] = useState("local");
    const [peers, setPeers] = useState([]);
    const diskSources = useDiskSources(t);
    const [items, setItems] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [pending, setPending] = useState(null);
    const [page, setPage] = useState(1);
    const [values, setValues] = useState({ q: "" });
    const { kind } = parseSource(source);

    useEffect(() => {
        let cancelled = false;
        federatedServers()
            .then((found) => {
                if (!cancelled) {
                    setPeers(found || []);
                }
            })
            .catch(() => undefined);

        return () => {
            cancelled = true;
        };
    }, []);

    const reload = useCallback(() => {
        setLoading(true);

        return fetchPatches(source)
            .then((answer) => {
                if (!answer.ok) {
                    latestContext.current.toast.open(answer.errorMessage || t("admin.storage.loadFailed"), { variant: "error" });
                }
                setItems((answer.consoles || []).flatMap((section) => section.items.map((item) => ({
                    ...item, consoleId: section.consoleId, console: section.console,
                }))));
                setPage(1);
            })
            .finally(() => setLoading(false));
    }, [source, latestContext, t]);

    useEffect(() => {
        reload();
    }, [reload]);

    // LA LIGNE CHANGE SUR PLACE (#295) : supprimer la retire, rapatrier lui
    // donne sa coche — rien à relire.
    const askDelete = (item) => latestContext.current.modal.open(
        <div className="text-center">
            {t("admin.storage.deletePatchConfirm")} <br />
            <strong>{item.name}</strong>
        </div>,
        {
            variant: "confirm",
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                const answer = await deletePatch(item.consoleId, item.name);
                latestContext.current.toast.open(
                    answer.ok ? t("admin.storage.patchDeleted", { name: item.name }) : (answer.errorMessage || t("app.error")),
                    { variant: answer.ok ? "success" : "error" },
                );
                if (answer.ok) {
                    setItems((current) => current.filter(
                        (other) => !(other.consoleId === item.consoleId && other.name === item.name)));
                }
            },
        },
    );

    const handleImport = async (item) => {
        const key = `${item.consoleId}/${item.name}`;
        setPending(key);
        try {
            const answer = await importFromSource(source, "patches", { consoleId: item.consoleId, name: item.name });
            latestContext.current.toast.open(
                answer.ok
                    ? t("admin.storage.downloaded", { name: item.name })
                    : (answer.errorMessage || t("admin.storage.notAtSource", { name: item.name })),
                { variant: answer.ok ? "success" : "error" },
            );
            if (answer.ok) {
                setItems((current) => current.map((other) => (
                    other.consoleId === item.consoleId && other.name === item.name ? { ...other, installed: true } : other)));
            }
        } finally {
            setPending(null);
        }
    };

    const actionFor = (item) => {
        if (kind === "local") {
            return (
                <bs.Button
                    variant="danger"
                    title={t("app.delete")}
                    onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        askDelete(item);
                    }}>
                    <i className="bi bi-trash"></i>
                </bs.Button>
            );
        }
        if (pending === `${item.consoleId}/${item.name}`) {
            return <DownloadRing percent={null} label={t("admin.storage.download")} />;
        }
        if (item.installed) {
            return <i className="bi bi-check-circle-fill text-success fs-4" title={t("admin.storage.installed")}></i>;
        }

        return (
            <bs.Button
                variant="primary"
                disabled={pending !== null}
                title={t("admin.storage.download")}
                onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    handleImport(item);
                }}>
                <i className="bi bi-download"></i>
            </bs.Button>
        );
    };

    const filtered = useMemo(() => {
        const needle = values.q.trim().toLowerCase();

        return needle ? items.filter((item) => item.name.toLowerCase().includes(needle)) : items;
    }, [items, values.q]);
    const visible = filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE);

    const sourceOptions = [
        { value: "local", label: t("admin.storage.sourceHere") },
        ...diskSources,
        ...peers.map((peer) => ({ value: `peer:${peer.id}`, label: peer.hostname || peer.link })),
    ];

    return (
        <SectionPanel>
            <MirrorNotice t={t} section="patches" />
            <SourceSelector
                t={t}
                value={source}
                onChange={setSource}
                options={sourceOptions}
                onRefreshed={reload}
                section="patches"
                helpText={t("admin.storage.patchesSourceHelp")} />
            <ListingElement
                callback={(item) => (
                    <ListElement
                        key={`${item.consoleId}::${item.name}`}
                        icon="bi bi-bandaid"
                        label={item.name}
                        helpText={[item.console, formatSize(item.sizeBytes), item.dir && item.files.join(", ")]
                            .filter(Boolean).join(" · ")}
                        next={actionFor(item)}
                    />
                )}
                items={visible}
                itemCount={filtered.length}
                itemsPerPage={ITEMS_PER_PAGE}
                currentPage={page}
                onPositionChange={setPage}
                isLoading={isLoading}
                search={{
                    context: { values, setValues },
                    placeholder: t("admin.storage.searchPatch"),
                    filters: null,
                }}
                texts={{
                    itemsFound: t("admin.storage.patchesFound"),
                    noItemsFound: t("admin.storage.noPatches"),
                }} />
        </SectionPanel>
    );
}
