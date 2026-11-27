// Route : /settings/logs — LES JOURNAUX (storage/Logs.jsx de la webapp) : ceux de cette machine, un par jour et par
// partie, en listing d'abord (chantier 81) : la recherche par nom, l'intervalle de jours (`q`, `from`, `to`, `page` dans
// l'adresse) ; un journal s'ouvre sur sa page (logs/view/Log.jsx) ; ses réglages ont la leur (logs/Settings.jsx) ; « Danger »
// supprime ceux que le filtre laisse voir — jamais tout le dossier.
import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteLog, fetchLogs } from "../libs/api.js";
import { can, useData } from "../libs/data.jsx";
import { fact, formatSize } from "../libs/format.js";
import { toast } from "../shell/Shell.jsx";
import { useFeedback } from "../bricks/useFeedback.jsx";
import { Form, useCalendarDays } from "../ui/Forms.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";
import { BlockRow } from "../bricks/BlockRow.jsx";
import { DangerSection } from "../bricks/DangerSection.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";

export const LOGS = "/settings/logs";

// « 20261112 » → « 2026-11-12 », pour le comparer à un champ de date.
const dayOf = (value) => (value ? value.replaceAll("-", "") : null);

// Le chemin d'un journal dans l'adresse : chaque segment encodé, les « / » gardés (la route est une étoile).
const logPath = (path) => `${LOGS}/view/${path.split("/").map(encodeURIComponent).join("/")}`;

// L'intervalle des jours : le `Form.DateRange` de Skeleton (le porteur, 2026-11-13), ses deux bornes gardées
// dans l'adresse (`from`, `to`, au format AAAA-MM-JJ que rend son calendrier), dans une ligne empilée du groupe
// des filtres (la forme de SelectRow : son Form.Group, le contrôle sur toute la largeur, sans icône). Ses deux
// calendriers (`Form.DateRange.components`, bornés l'un par l'autre comme dans DateRange) l'un SOUS l'autre,
// comme le modèle les dessine : côte à côte, sur un téléphone, ses sélecteurs d'année et de mois se tronquent
// (« 2026 » coupé, « N » pour novembre ; la webapp l'a constaté aussi, storage/Logs.jsx). Chacun dans le cadre
// que Skeleton donne à Form.Date. Elle vit dans le panneau des filtres de la liste (Listing ; le porteur, 2026-11-14 :
// « met les calendriers dans le offcanvas du filtre »). La page (SettingsPage, usePadReach) mène la manette jusqu'aux
// sélecteurs d'année et de mois que Skeleton dessine lui-même ; SES JOURS, des `div` que Skeleton rend touchables
// (`cursor: pointer`), sont marqués pour la croix (useCalendarDays, Forms.jsx : `data-nav`, `role="button"`) — et
// démarqués quand une borne les grise.
function DateRangeRow({ from, to, onChange }) {
    const days = useRef(null);
    const name = "lg-dates";
    const context = {
        values: { [name]: [from || null, to || null] },
        setValues: (next) => {
            const [start, end] = next[name] || [null, null];
            onChange(start || "", end || "");
        },
        files: {}, setFiles: () => {}, rulesets: {},
    };
    const { Left, Right } = Form.DateRange.components({ name, context });
    const today = new Date();
    useCalendarDays(days);

    return (
        <BlockRow>
            <Form.Group label={null} props={{ className: "mb-0 w-100" }}>
                <div ref={days} className="d-grid gap-2">
                    <div className="border rounded p-2"><Left maxDate={today} /></div>
                    <div className="border rounded p-2"><Right maxDate={today} /></div>
                </div>
            </Form.Group>
        </BlockRow>
    );
}

export default function Logs() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { open } = useSettingsNavigation();
    const { capabilities } = useData();
    const [logs, setLogs] = useState([]);
    const [loading, setLoading] = useState(true);
    const [read, update] = useQueryState({ q: "", from: "", to: "", page: "1" });
    const search = read("q");
    const from = read("from");
    const to = read("to");

    useEffect(() => {
        fetchLogs().then((answer) => {
            if (!answer.ok) {
                toast(answer.errorMessage || t("logs.loadFailed"));
            }
            setLogs(answer.data?.logs || []);
            setLoading(false);
        });
    }, [t]);

    const needle = search.trim().toLowerCase();
    const floor = dayOf(from);
    const ceiling = dayOf(to);
    const filtered = logs.filter((log) => (!needle || log.path.toLowerCase().includes(needle))
        && (!log.day || ((!floor || log.day >= floor) && (!ceiling || log.day <= ceiling))));

    const removeAll = async () => {
        const results = await Promise.all(filtered.map((log) => deleteLog(log.path)));
        const deleted = results.filter((answer) => answer.ok).length;
        const failedCount = results.length - deleted;
        if (failedCount) {
            feedback.warning(t("logs.deletedPartial", { deleted, failed: failedCount }));
        } else {
            feedback.success(t("logs.deletedMany", { count: deleted }));
        }
        const gone = new Set(filtered.filter((_, index) => results[index].ok).map((log) => log.path));
        setLogs((current) => current.filter((log) => !gone.has(log.path)));
    };

    return (
        <SettingsPage title={t("settings.items.logs.label")}>
            <Section title={t("logs.listTitle")} help={t("logs.listHelp")} sense="modify">
                <Listing items={filtered} overall={logs.length} loading={loading} search={search} onSearch={(q) => update({ q, page: 1 })} placeholder={t("logs.searchLog")} page={Number(read("page")) || 1} onPage={(page) => update({ page })}
                    found={t("logs.logsFound", { count: filtered.length })} empty={t(logs.length ? "logs.noMatch" : "logs.noLogs")} noMatch={t("logs.noMatch")}
                    filterCount={from || to ? 1 : 0} filters={[{ key: "dates", title: t("logs.filterDates"), rows: <DateRangeRow from={from} to={to} onChange={(start, end) => update({ from: start, to: end, page: 1 })} /> }]}
                    render={(log) => (
                        <NavRow key={log.path} id={`lg-open-${log.path}`} icon="bi-journal-text" label={log.name} onOpen={() => open(logPath(log.path), `lg-open-${log.path}`, { log })}
                            help={[fact(t("facts.date"), [log.day, log.session].filter(Boolean).join(" ")), fact(t("game.size"), formatSize(log.sizeBytes))]} />
                    )} />
            </Section>
            {/* SES RÉGLAGES, SUR LEUR PROPRE ROUTE (logs/Settings.jsx) : ce qui modifie, en orange. Un hôte qui ne les sert pas
                n'a pas la ligne (une capacité : la seule exception à « désactiver plutôt que cacher »). */}
            {can(capabilities, "logSettings") && (
                <Section title={t("logs.settingsSection")} help={t("logs.settingsSectionHelp")} sense="modify">
                    <Rows>
                        <NavRow id="lg-settings" icon="bi-sliders" label={t("logs.settingsTitle")} help={t("logs.settingsRowHelp")} onOpen={() => open(`${LOGS}/settings`, "lg-settings")} />
                    </Rows>
                </Section>
            )}
            {/* Rien d'affiché : grisée, et pourquoi (ce que la liste dit déjà). */}
            <DangerSection id="lg-all" label={t("logs.deleteAll")} help={!loading && !filtered.length ? t(logs.length ? "logs.noMatch" : "logs.noLogs") : t("logs.deleteAllHelp")}
                question={t("logs.deleteAllConfirm", { count: filtered.length })} disabled={loading || !filtered.length} onConfirm={removeAll} />
            <GuideSection guides={[["storage", "logs"], ["troubleshooting", "find-out-what-happened"]]} />
        </SettingsPage>
    );
}

