import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { deleteLog, fetchLogs, fetchLogSettings, fetchLogTail, logFileUrl, saveLogSettings } from "../../libs/api.js";
import { can, useData } from "../../libs/data.jsx";
import { formatSize } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import { Form, useCalendarDays } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { BlockRow, DangerSection, GuideSection, InfoRow, Listing, Note, PlaceholderRow, Rows, SelectRow, Section, SwitchRow } from "../../ui/Bricks.jsx";

// LES JOURNAUX (storage/Logs.jsx de la webapp) : ceux de cette machine, un par jour et par partie ;
// la recherche par nom, l'intervalle de jours ; ouvrir un journal montre ses 100 dernières lignes
// (cherchées, filtrées par niveau, 25 par page), le télécharge ou le supprime ; la liste supprime tous
// ceux que le filtre laisse voir — jamais tout le dossier. Une ligne de la liste ne fait qu'ouvrir ; les
// suppressions sont des lignes de « Danger », en bas de leur page, confirmées dans la fenêtre de Skeleton ;
// télécharger, une ligne de « Plus » (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être
// des boutons mais des liens »).
//
// LA DISPOSITION (le porteur, 2026-11-14 : « Le "What gets written" doit être en premier et en dessous on voit dans
// more, show logs. Et c'est sur cette page qu'on voit le listing des logs ») : la page des journaux dit d'abord ce qui
// s'écrit (LogWriting) — le journal du kernel, ses niveaux et les requêtes HTTP, les journaux des parties —, puis
// « Plus » : « Voir les journaux », la liste, sur sa page.
//
//   /settings/logs                  ce qui s'écrit ; « Plus » : voir les journaux
//   /settings/logs/list             la liste (`q`, `from`, `to`, `page` dans l'adresse)
//   /settings/logs/view/<chemin>    un journal (son chemin, tel que le kernel le nomme ; `q`, `level`, `page`)

const RAW_LEVEL = "__raw__";
const BASE = "/settings/logs";
const LIST = `${BASE}/list`;
// Les niveaux, un interrupteur chacun, dans l'ordre du kernel (gameflex/logfile, LevelNames).
const LEVELS = ["debug", "info", "warn", "error"];

// « 20261112 » → « 2026-11-12 », pour le comparer à un champ de date.
const dayOf = (value) => (value ? value.replaceAll("-", "") : null);

// Le chemin d'un journal dans l'adresse : chaque segment encodé, les « / » gardés (la route est une étoile).
const logPath = (path) => `${BASE}/view/${path.split("/").map(encodeURIComponent).join("/")}`;

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
    const { back } = useSettingsNavigation();
    const [entries, setEntries] = useState(null);
    const [failed, setFailed] = useState("");
    const [read, update] = useQueryState({ q: "", level: "", page: "1" });
    const level = read("level");

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
    const filtered = (entries || []).filter((entry) => (!level || (entry.level || RAW_LEVEL) === level)
        && (!needle || [entry.message, entry.details, entry.raw, entry.clock].some((field) => (field || "").toLowerCase().includes(needle))));

    // Le niveau en filtre : « Tous les niveaux », puis ceux que ce journal contient ; le choisir remet la page 1.
    // Une ligne du journal : son message (petit, et coupé où il faut : un chemin d'API n'a pas d'espace), puis
    // l'heure et le niveau en chasse fixe.
    return (
        <>
            <Section title={t("details")}>
                <Rows>
                    <InfoRow icon="bi-file-earmark-text" label={t("resources.filePath")} value={log.path} mono />
                </Rows>
            </Section>
            <Section title={t("logs.lastLines")}>
                {/* Un journal illisible : la raison en ligne atténuée, plus en alerte (le porteur, 2026-11-16). */}
                {failed ? <Note className="mt-0">{failed}</Note> : (
                    <Listing items={filtered} loading={entries === null} search={read("q")} onSearch={(q) => update({ q, page: 1 })} placeholder={t("logs.searchLine")} pageSize={25}
                        placeholderRow={() => <PlaceholderRow lead={null} chevron={false} />}
                        page={Number(read("page")) || 1} onPage={(page) => update({ page })} found={t("logs.linesFound", { count: filtered.length })} empty={t("logs.noLine")}
                        filterCount={level ? 1 : 0} filters={present.length > 1 && (
                            <SelectRow id="lg-level" label={t("logs.filterLevel")} help={t("logs.filterLevelHelp")} value={level} onChange={(next) => update({ level: next, page: 1 })}
                                options={[{ value: "", label: t("logs.allLevels") }, ...present.map((value) => ({ value, label: value === RAW_LEVEL ? t("logs.rawLine") : value }))]} />
                        )}
                        render={(entry, index) => (
                            <InfoRow key={`${entry.time || entry.clock || ""}-${index}`} mono
                                label={<span className="small fw-normal text-break">{[entry.message || entry.raw, entry.details].filter(Boolean).join(" — ")}</span>}
                                value={[entry.clock, entry.level || t("logs.rawLine")].filter(Boolean).join(" · ")} />
                        )} />
                )}
            </Section>
            {/* « Plus » APRÈS la liste de ses lignes (le porteur, 2026-11-14 : « Les sections sont toujours après les
                listings ») : le télécharger. */}
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="lg-download" icon="bi-download" tone="indigo" label={t("logs.download")} help={t("logs.downloadHelp")} opens="action" onOpen={() => openDownload(logFileUrl(log.path))} />
                </Rows>
            </Section>
            {/* LE SUPPRIMER, ici et plus sur sa ligne de la liste ; fait, retour à la liste. */}
            <DangerSection id="lg-del" help={t("logs.deleteHelp")} label={t("logs.delete")} question={t("logs.deleteConfirm")} onConfirm={async () => {
                const answer = await deleteLog(log.path);
                toast(answer.ok ? t("logs.deleted", { name: log.name }) : answer.errorMessage || t("logs.deleteFailed"));

                if (answer.ok) {
                    back(LIST);
                }
            }} />
        </>
    );
}

// /settings/logs/view/<chemin> : le journal que la liste a passé, sinon relu dans la liste du kernel.
function LogPage() {
    const { t } = useTranslation();
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
        <SettingsPage title={log?.name || path.split("/").pop()} parent={LIST} levels={{ [LIST]: t("logs.showLogs") }}>
            {log === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : log === null ? <NotFound /> : <LogLines log={log} />}
        </SettingsPage>
    );
}

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
    const { t } = useTranslation();
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
            <Form.Group label={t("logs.filterDates")} helpText={t("logs.filterDatesHelp")} props={{ className: "mb-0 w-100" }}>
                <div ref={days} className="d-grid gap-2">
                    <div className="border rounded p-2"><Left maxDate={today} /></div>
                    <div className="border rounded p-2"><Right maxDate={today} /></div>
                </div>
            </Form.Group>
        </BlockRow>
    );
}

// CE QUI S'ÉCRIT (le porteur, 2026-11-14 : « Il faut que dans logs on puisse avoir la possibilité de l'activer ou pas et
// aussi choisir les level à afficher si on active le log. ») — deux interrupteurs qui s'appliquent aussitôt, le kernel les
// suivant dès sa ligne suivante (log_settings.go) : le journal du kernel, et, allumé, ses niveaux et les requêtes HTTP à
// part, quels que soient les niveaux allumés ; les journaux des parties, sans niveaux (la sortie d'un émulateur n'en a pas).
// LES SIGNALEMENTS (reports.log), leur interrupteur aussi (le porteur, 2026-11-14 : « Oui pour le reports.log ») :
// éteints, l'écran d'erreur d'un lancement ne propose plus « Signaler l'erreur » (la capacité `reports`).
// LES NIVEAUX, UN INTERRUPTEUR CHACUN (le porteur, 2026-11-14 : « change les checkbox en switch pour le choix des niveaux
// de log »), dans leur section, avec les requêtes HTTP — toujours là, grisés quand le journal du kernel est éteint (le
// porteur : « Au lieu de faire disparaitre le level to write, le mieux ce serait de mettre les switch en disabled et
// enabled quand c'est activé »).
// Les fichiers seulement : la sortie standard du kernel ne change pas. Grisé tant qu'il se lit ; ce qu'on touche part
// seul, et revient en arrière si le kernel refuse. Un hôte qui ne le sert pas ne le montre pas (`logSettings`).
// CE QUE L'HÔTE RÈGLE, ET RIEN D'AUTRE : une ligne pour chaque réglage que sa réponse porte — le téléphone a le journal
// de son kernel, ses niveaux et ses requêtes HTTP (`{kernel, levels, http}`, mobileapp/kernel/logging.go, 2026-11-17),
// pas les journaux des parties ni les signalements. Pendant la lecture, toutes, grisées.
function LogWriting() {
    const { t } = useTranslation();
    const [settings, setSettings] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchLogSettings().then((answer) => !cancelled && setSettings(answer.ok ? answer.data : null));

        return () => {
            cancelled = true;
        };
    }, []);

    const change = async (patch) => {
        const undo = Object.fromEntries(Object.keys(patch).map((key) => [key, settings[key]]));
        setSettings((current) => ({ ...current, ...patch }));
        const answer = await saveLogSettings(patch);

        if (!answer.ok) {
            setSettings((current) => ({ ...current, ...undo }));
            toast(answer.errorMessage || t("settings.saveFailed"));
        }
    };

    const known = Boolean(settings);
    const offers = (name) => !known || name in settings;
    const levels = known ? settings.levels || [] : [];
    const failed = settings === null && <Note className="mt-0">{t("logs.loadFailed")}</Note>;

    return (
        <>
            <Section title={t("logs.writingTitle")} help={t("logs.writingHelp")}>
                {failed || (
                    <Rows>
                        <SwitchRow id="lg-kernel" label={t("logs.kernel")} help={t("logs.kernelHelp")} checked={known && settings.kernel} disabled={!known}
                            onChange={(on) => change({ kernel: on })} />
                        {offers("sessions") && (
                            <SwitchRow id="lg-sessions" label={t("logs.sessions")} help={t("logs.sessionsHelp")} checked={known && settings.sessions} disabled={!known}
                                onChange={(on) => change({ sessions: on })} />
                        )}
                        {offers("reports") && (
                            <SwitchRow id="lg-reports" label={t("logs.reports")} help={t("logs.reportsHelp")} checked={known && settings.reports} disabled={!known}
                                onChange={(on) => change({ reports: on })} />
                        )}
                    </Rows>
                )}
            </Section>
            {known && "levels" in settings && (
                <Section title={t("logs.levels")} help={t("logs.levelsHelp")}>
                    <Rows>
                        {LEVELS.map((level) => (
                            <SwitchRow key={level} id={`lg-level-${level}`} label={t(`logs.level.${level}`)} checked={levels.includes(level)} disabled={!settings.kernel}
                                onChange={(on) => change({ levels: LEVELS.filter((other) => (other === level ? on : levels.includes(other))) })} />
                        ))}
                        <SwitchRow id="lg-http" label={t("logs.http")} help={t("logs.httpHelp")} checked={settings.http} disabled={!settings.kernel} onChange={(on) => change({ http: on })} />
                    </Rows>
                </Section>
            )}
        </>
    );
}

// /settings/logs : ce qui s'écrit, puis « Plus » : voir les journaux. Un hôte qui ne sert pas le réglage n'a que
// « Plus ».
function LogsPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { capabilities } = useData();
    const writing = can(capabilities, "logSettings");

    // BAS depuis Retour : le premier interrupteur, que l'alignement de la croix sauterait (il est au bord droit, Retour
    // et « Voir les journaux » au bord gauche).
    return (
        <SettingsPage title={t("settings.items.logs.label")} enter={writing ? "lg-kernel" : null}>
            {writing && <LogWriting />}
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="lg-list" icon="bi-journal-text" tone="slate" label={t("logs.showLogs")} help={t("logs.showLogsHelp")} onOpen={() => open(LIST, "lg-list")} />
                </Rows>
            </Section>
            <GuideSection guides={[["storage", "logs"], ["troubleshooting", "find-out-what-happened"]]} />
        </SettingsPage>
    );
}

// /settings/logs/list : la liste des journaux, et supprimer ceux que le filtre laisse voir.
function ListPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
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
        toast(failedCount ? t("logs.deletedPartial", { deleted, failed: failedCount }) : t("logs.deletedMany", { count: deleted }));
        const gone = new Set(filtered.filter((_, index) => results[index].ok).map((log) => log.path));
        setLogs((current) => current.filter((log) => !gone.has(log.path)));
    };

    return (
        <SettingsPage title={t("logs.showLogs")} parent={BASE}>
            <Section title={t("logs.listTitle")}>
                <Listing items={filtered} loading={loading} search={search} onSearch={(q) => update({ q, page: 1 })} placeholder={t("logs.searchLog")} page={Number(read("page")) || 1} onPage={(page) => update({ page })}
                    found={t("logs.logsFound", { count: filtered.length })} empty={t(logs.length ? "logs.noMatch" : "logs.noLogs")} noMatch={t("logs.noMatch")}
                    filterCount={from || to ? 1 : 0} filters={<DateRangeRow from={from} to={to} onChange={(start, end) => update({ from: start, to: end, page: 1 })} />}
                    render={(log) => (
                        <NavRow key={log.path} id={`lg-open-${log.path}`} icon="bi-journal-text" tone="slate" label={log.name} onOpen={() => open(logPath(log.path), `lg-open-${log.path}`, { log })}
                            help={[log.session ? t("logs.sessionAt", { day: log.day, time: log.session }) : log.day, formatSize(log.sizeBytes)].filter(Boolean).join(" · ")} />
                    )} />
            </Section>
            {/* Rien d'affiché : grisée, et pourquoi (ce que la liste dit déjà). */}
            <DangerSection id="lg-all" label={t("logs.deleteAll")} help={!loading && !filtered.length ? t(logs.length ? "logs.noMatch" : "logs.noLogs") : t("logs.deleteAllHelp")}
                question={t("logs.deleteAllConfirm", { count: filtered.length })} disabled={loading || !filtered.length} onConfirm={removeAll} />
        </SettingsPage>
    );
}

// /settings/logs/* (Settings.jsx).
export function LogsRoutes() {
    return (
        <Routes>
            <Route index element={<LogsPage />} />
            <Route path="list" element={<ListPage />} />
            <Route path="view/*" element={<LogPage />} />
            <Route path="*" element={<Navigate to={BASE} replace />} />
        </Routes>
    );
}
