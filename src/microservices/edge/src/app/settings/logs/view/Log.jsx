// Route : /settings/logs/view/* — `*` : le chemin du journal, tel que le kernel le nomme (`q`, `nolevel`, `page` dans
// l'adresse).
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { deleteLog, fetchLogs, fetchLogTail, logFileUrl } from "../../../libs/api.js";
import { fact } from "../../../libs/format.js";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { switchGroup } from "../../../bricks/FilterGroups.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Listing } from "../../../bricks/Listing.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { PlaceholderRow } from "../../../bricks/PlaceholderRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { hiddenOf, toggleHidden } from "../../../globals/filters.js";
import { LOGS } from "../../Logs.jsx";

// UN JOURNAL : ses 100 dernières lignes (cherchées, filtrées par niveau, 25 par page), le télécharger, le supprimer.

// Une ligne sans niveau (la sortie brute d'un émulateur).
const RAW_LEVEL = "__raw__";

// LE TÉLÉCHARGER, comme le lien qu'il était (un onglet à part, que le serveur sert en pièce jointe) : une ligne
// n'est pas un lien, elle en ouvre un (le même geste que GameSettings.jsx).
const openDownload = (href) => {
    const link = document.createElement("a");
    link.href = href;
    link.target = "_blank";
    link.rel = "noreferrer";
    document.body.appendChild(link);
    link.click();
    link.remove();
};

function LogLines({ log }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { back } = useSettingsNavigation();
    const [entries, setEntries] = useState(null);
    const [failed, setFailed] = useState("");
    const [read, update] = useQueryState({ q: "", nolevel: "", page: "1" });
    const hiddenLevels = hiddenOf(read("nolevel"));

    useEffect(() => {
        fetchLogTail(log.path).then((answer) => {
            if (answer.ok) {
                setEntries(answer.data?.entries || []);
            } else {
                setFailed(answer.errorMessage || t("logs.loadFailed"));
            }
        });
    }, [log.path, t]);

    const present = useMemo(() => [...new Set((entries || []).map((entry) => entry.level || RAW_LEVEL))].sort(), [entries]);
    const needle = read("q").trim().toLowerCase();
    const filtered = (entries || []).filter((entry) => !hiddenLevels.includes(entry.level || RAW_LEVEL)
        && (!needle || [entry.message, entry.details, entry.raw, entry.clock].some((field) => (field || "").toLowerCase().includes(needle))));

    // Les niveaux en filtre, un interrupteur chacun (ceux que ce journal contient), comme les statuts d'une liste de jeux ;
    // en basculer un remet la page 1.
    // Une ligne du journal : son message (petit, et coupé où il faut : un chemin d'API n'a pas d'espace), puis
    // l'heure et le niveau en chasse fixe.
    return (
        <>
            <Section title={t("details")} help={t("logs.detailsHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-file-earmark-text" label={t("resources.filePath")} value={log.path} mono />
                </Rows>
            </Section>
            <Section title={t("logs.lastLines")} help={t("logs.lastLinesHelp")} sense="modify">
                {/* Un journal illisible : la raison en ligne atténuée, plus en alerte (le porteur, 2026-11-16). */}
                {failed ? <Note className="mt-0">{failed}</Note> : (
                    <Listing items={filtered} overall={(entries || []).length} loading={entries === null} search={read("q")} onSearch={(q) => update({ q, page: 1 })} placeholder={t("logs.searchLine")} pageSize={25}
                        placeholderRow={() => <PlaceholderRow lead={null} chevron={false} />}
                        page={Number(read("page")) || 1} onPage={(page) => update({ page })} found={t("logs.linesFound", { count: filtered.length })} empty={t("logs.noLine")}
                        filterCount={hiddenLevels.length} filters={[switchGroup({ key: "levels", title: t("logs.filterLevel"), id: "lg-filter-level", hidden: hiddenLevels,
                            onChange: (value, on) => update({ nolevel: toggleHidden(hiddenLevels, value, on), page: 1 }),
                            choices: present.map((value) => ({ value, label: value === RAW_LEVEL ? t("logs.rawLine") : t(`logs.level.${value.toLowerCase()}`, { defaultValue: value }) })) })]}
                        render={(entry, index) => (
                            <InfoRow key={`${entry.time || entry.clock || ""}-${index}`} mono
                                label={<span className="small fw-normal text-break">{[entry.message || entry.raw, entry.details].filter(Boolean).join(" — ")}</span>}
                                value={[fact(t("facts.time"), entry.clock), fact(t("facts.level"), entry.level || t("logs.rawLine"))]} />
                        )} />
                )}
            </Section>
            {/* « Plus » APRÈS la liste de ses lignes (le porteur, 2026-11-14 : « Les sections sont toujours après les
                listings ») : le télécharger. */}
            <Section title={t("more")} help={t("logs.moreHelp")} sense="play">
                <Rows>
                    <NavRow id="lg-download" icon="bi-download" label={t("logs.download")} help={t("logs.downloadHelp")} opens="action" onOpen={() => openDownload(logFileUrl(log.path))} />
                </Rows>
            </Section>
            {/* LE SUPPRIMER, ici et plus sur sa ligne de la liste ; fait, retour à la liste. */}
            <DangerSection id="lg-del" help={t("logs.deleteHelp")} label={t("logs.delete")} question={t("logs.deleteConfirm")} onConfirm={async () => {
                const answer = await deleteLog(log.path);
                feedback.result(answer.ok, t("logs.deleted", { name: log.name }), answer.errorMessage || t("logs.deleteFailed"));

                if (answer.ok) {
                    back(LOGS);
                }
            }} />
        </>
    );
}

// Le journal que la liste a passé, sinon relu dans la liste du kernel.
export default function Log() {
    const location = useLocation();
    const path = useParams()["*"] || "";
    const given = location.state?.log?.path === path ? location.state.log : null;
    const [log, setLog] = useState(given || undefined);

    useEffect(() => {
        if (given) {
            return undefined;
        }
        let cancelled = false;
        fetchLogs().then((answer) => {
            if (!cancelled) {
                setLog((answer.ok ? answer.data?.logs || [] : []).find((entry) => entry.path === path) || null);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [path]);

    return (
        <SettingsPage title={log?.name || path.split("/").pop()} parent={LOGS}>
            {log === undefined ? <Section><Loading /></Section> : log === null ? <NotFound /> : <LogLines log={log} />}
            <GuideSection guides={[["storage", "logs"], ["troubleshooting", "find-out-what-happened"]]} />
        </SettingsPage>
    );
}

