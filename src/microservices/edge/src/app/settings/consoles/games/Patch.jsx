// Route : /settings/consoles/<console>/games/<source>/patch?ref= — son correctif.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { DangerRow } from "../../../bricks/DangerRow.jsx";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { DownloadRow } from "../../../bricks/DownloadRow.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../../bricks/Listing.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { useConfirm } from "../../../bricks/useConfirm.jsx";
import { deleteInstalled, gamePatch } from "../../../libs/api.js";
import { downloadOutcome, runDownload, useDownloads } from "../../../libs/downloads.js";
import { fact, formatMegabytes } from "../../../libs/format.js";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { gamePagePath, gameSourceOf, isLocal, serverName, useAddressedGame, useGameTitle, useLevels } from "../Games.jsx";

// « LE RÉCUPÉRER » pour un correctif absent (le modèle 26) : chaque disque interne, puis chaque serveur fédéré, comme le
// serveur les rend (storage_patches_api.go) — grisés quand ils ne l'ont pas, avec leur raison. Toucher une source ouvre
// la fenêtre, puis la commande `download` (route `disk` ou `server`, section `patches`) ; sa ligne montre la
// progression, les autres sont grisées le temps qu'il dure. Elles se feuillettent, la page tenue par la liste.
function PatchSources({ consoleId, view, gameTitle, running }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    // LA SOURCE OFFICIELLE (chantier 63, P6) : le paquet des correctifs de l'index des paquets streamex.
    const keyOf = (source) => (source.source === "official" ? "official" : source.source === "disk" ? `disk:${source.disk}` : `server:${source.serverId}`);
    const labelOf = (source) => (source.source === "official" ? t("consoles.sourceStreamex")
        : source.source === "disk" ? source.disk : serverName({ hostname: source.name }, source.serverId, t));
    // Les sources, nombreuses quand les serveurs fédérés le sont : un listing qui se cherche.
    const found = useListSearch(view.sources, labelOf);
    // La raison d'un serveur qui ne répond pas comme il faut est le message de son refus (remote.Refusal) : notre jeton
    // refusé (« does not allow access »), ou le serveur muet.
    const reasonOf = (source) => (!source.reason ? t("consoles.sourceLacks")
        : t(source.source !== "official" && /allow/i.test(source.reason) ? "consoles.sourceDenied" : "consoles.sourceUnreachable"));

    const start = async (source) => {
        const body = { consoleId, name: view.gameId };
        const command = source.source === "official" ? { route: "official", section: "patches", body }
            : source.source === "disk" ? { route: "disk", disk: source.disk, section: "patches", body }
                : { route: "server", server: String(source.serverId), section: "patches", body };
        const answer = await runDownload({
            key: `patches/${consoleId}/${view.gameId}`, label: gameTitle || view.gameId, source: keyOf(source),
            command: { ...command, ...(source.sizeBytes ? { total: source.sizeBytes } : {}) }
        });
        toast(downloadOutcome(answer, view.gameId, t));
    };

    const ask = (source) => {
        const size = formatMegabytes(source.sizeBytes);
        const from = labelOf(source);

        confirm({
            title: t("consoles.fetchFrom", { source: from }),
            message: t(size ? "gameAdmin.patchConfirmSized" : "gameAdmin.patchConfirm", { name: gameTitle || view.gameId, size, source: from }),
            confirmLabel: t("consoles.download"),
            onConfirm: () => start(source),
        });
    };

    const row = (source) => {
        const own = running?.source === keyOf(source);
        const disk = source.source === "disk";
        const official = source.source === "official";
        const facts = [fact(t("facts.type"), t(official ? "gameAdmin.officialKind" : disk ? "gameAdmin.diskKind" : "consoles.federatedServer")),
        source.available ? fact(t("game.size"), formatMegabytes(source.sizeBytes)) : reasonOf(source)];

        return (
            own ? <DownloadRow key={keyOf(source)} entry={running} id={`gp-source-${keyOf(source)}`} label={labelOf(source)} />
                : <NavRow key={keyOf(source)} id={`gp-source-${keyOf(source)}`} icon={official ? "bi-patch-check" : disk ? "bi-hdd" : "bi-arrow-left-right"}
                    label={labelOf(source)}
                    help={facts}
                    disabled={!source.available || Boolean(running)} opens="confirm" onOpen={() => ask(source)} />
        );
    };

    return (
        <Section title={t("consoles.fetchTitle")} help={view.sources.length ? t("gameAdmin.patchFetchHelp") : null} sense="modify">
            <Listing items={found.found} empty={t("gameAdmin.patchNoSource")} render={row}
                search={found.search} onSearch={found.onSearch} placeholder={t("gameAdmin.searchSource")} searchKey="gp-source-search" />
        </Section>
    );
}

// /…/games/<source>/patch?ref= : LE CORRECTIF D'UN JEU, comme un émulateur (le modèle 26) — `patches/<console>/<id>/`,
// lu par `…/roms/{ref}/patch` (storage_patches_api.go, handleGamePatch).
//   - Détails : l'identifiant du jeu, son statut, son dossier ; installé, ses fichiers et sa taille ;
//   - installé : le supprimer (« Danger »), puis retour au jeu ;
//   - absent : « Le récupérer » (PatchSources). Fini, la page se relit.
export default function Patch() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const { back } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);
    const [view, setView] = useState(undefined);
    // UNE ROUTE QUE L'HÔTE NE SERT PAS (le téléphone, libs/api.js) : sa raison, au lieu d'un « Introuvable » qui mentirait.
    const [unserved, setUnserved] = useState("");
    const [epoch, setEpoch] = useState(0);
    const running = useDownloads().find((entry) => entry.key === `patches/${consoleId}/${view?.gameId || ""}`) || null;
    const wasRunning = useRef(false);
    const parent = gamePagePath(consoleId, slug, reference);
    const installed = view?.installed || null;

    useEffect(() => {
        if (!isLocal(source)) {
            setView(null);

            return undefined;
        }
        let cancelled = false;
        gamePatch(consoleId, reference, gameSourceOf(source)).then((answer) => {
            if (!cancelled) {
                setView(answer.ok ? answer.data : null);
                setUnserved(answer.errorCode === "not_served" ? answer.errorMessage : "");
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, reference, epoch]);

    useEffect(() => {
        if (wasRunning.current && !running) {
            setEpoch((value) => value + 1);
        }
        wasRunning.current = Boolean(running);
    }, [running]);

    return (
        <SettingsPage title={t("gameAdmin.patch")} parent={parent} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {view === undefined ? <Section title={t("details")} sense="info"><Loading /></Section>
                : unserved ? <Section title={t("details")} sense="info"><Note className="mt-0">{unserved}</Note></Section>
                    : view === null || game === null ? <NotFound />
                        : !view.gameId ? <Section title={t("details")} sense="info"><Note className="mt-0">{t("gameAdmin.noGameId")}</Note></Section> : (
                            <>
                                <Section title={t("details")} sense="info">
                                    <Rows>
                                        <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={view.gameId} />
                                        <InfoRow icon={installed ? "bi-check2-circle" : "bi-x-circle"} label={t("games.status")}
                                            value={t(installed ? "consoles.installed" : "consoles.missing")} />
                                        <InfoRow icon="bi-folder2" label={t("gameAdmin.patchFolder")} value={view.folder} mono />
                                        {installed && <InfoRow icon="bi-file-earmark-code" label={t("game.files")} mono
                                            value={(installed.files || []).length ? installed.files.map((file) => <span key={file} className="d-block">{file}</span>) : null} />}
                                        {installed && <InfoRow icon="bi-hdd" label={t("game.size")} value={formatMegabytes(installed.sizeBytes)} />}
                                    </Rows>
                                </Section>
                                {installed ? (
                                    <DangerSection>
                                        <DangerRow id="gm-patch-remove" label={t("gameAdmin.patchRemove")} help={t("gameAdmin.patchRemoveHelp")} question={t("gameAdmin.patchRemoveConfirm", { name: view.gameId })}
                                            onConfirm={async () => {
                                                const answer = await deleteInstalled("patches", consoleId, view.gameId);
                                                feedback.result(answer.ok, t("gameAdmin.patchRemoved", { name: view.gameId }), answer.errorMessage || t("settings.actionFailed"));

                                                if (answer.ok) {
                                                    back(parent);
                                                }
                                            }} />
                                    </DangerSection>
                                ) : <PatchSources consoleId={consoleId} view={view} gameTitle={title} running={running} />}
                            </>
                        )}
        </SettingsPage>
    );
}
