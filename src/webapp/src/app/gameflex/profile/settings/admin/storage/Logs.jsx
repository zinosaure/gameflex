import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../../skeleton/skeleton.js";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    FormButton,
    OffCanvas,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import { formatSize } from "../api/library.js";
import { fetchLogs, fetchLogTail, deleteLog, logFileUrl } from "../api/storage-admin.js";
import { hasLocalLibrary } from "../../../../libs/localLibrary.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;
// Une ligne de journal tient sur deux lignes d'écran, pas plus : on en
// affiche davantage qu'on n'affiche de fichiers.
const LINES_PER_PAGE = 25;
// Le « niveau » d'une ligne qu'on n'a pas su découper. Un identifiant, pas
// un libellé : il sert de valeur de filtre, et le texte affiché est traduit.
const RAW_LEVEL = "__raw__";

// LES JOURNAUX — les relire sans ouvrir un terminal.
//
// Écrire un journal ne sert que si on peut le relire, et jusqu'ici la seule
// façon de le faire était d'avoir un accès SSH à la machine. Sur un
// téléphone il n'y en a pas ; sur un serveur, l'administrateur qui veut
// comprendre une partie finie n'en a pas toujours un non plus.
//
// Le rangement porte déjà l'information — `<jour>/<nom>.log` pour un
// service, `<jour>/<heure>/<conteneur>.log` pour une partie — donc l'écran
// n'a rien à deviner du contenu pour dire de quoi il parle.

// Les cent dernières lignes. La FIN, jamais le début : on ouvre cet écran
// après qu'une chose vient de mal se passer. Le fichier entier se télécharge
// pour qui veut le reste.
//
// UNE LISTE, PAS UN BLOC DE TEXTE. Le premier jet rendait le journal brut
// dans un `<pre>` — c'est-à-dire ce qu'un terminal donne déjà, accolades
// comprises, et sur un téléphone chaque entrée se repliait sur quatre lignes
// illisibles. Le serveur les découpe maintenant (`logfile.ParseTail`), donc
// l'écran peut en faire ce qu'il fait de toutes ses listes : le message en
// titre, l'heure et le niveau dessous, et de quoi filtrer les trois.
function LogView({ path, t }) {
    const [values, setValues] = useState({ q: "", levels: [] });
    const [entries, setEntries] = useState(null);
    const [page, setPage] = useState(1);
    const [error, setError] = useState(null);

    useEffect(() => {
        fetchLogTail(path).then((result) => {
            if (result.ok) {
                setEntries(result.entries || []);
            } else {
                setError(result.errorMessage || t("admin.storage.loadFailed"));
            }
        });
    }, [path, t]);

    // Les niveaux PRÉSENTS dans ce journal, pas la liste théorique : proposer
    // ERROR à un journal qui n'en a aucune est une case qui vide la liste.
    // Une ligne qu'on n'a pas su découper (un accès HTTP, une sortie
    // d'émulateur) n'a pas de niveau et se filtre sous son propre libellé —
    // la taire serait cacher précisément celle qui sort de l'ordinaire.
    const levels = useMemo(() => {
        const seen = new Set((entries || []).map((entry) => entry.level || RAW_LEVEL));

        return [...seen].sort();
    }, [entries]);

    const filtered = useMemo(() => {
        const needle = (values.q || "").trim().toLowerCase();

        return (entries || []).filter((entry) => {
            if (values.levels.length > 0
                && !values.levels.includes(entry.level || RAW_LEVEL)) {
                return false;
            }
            if (!needle) {
                return true;
            }
            // L'heure est cherchable comme le reste : « ce qui s'est passé à
            // 00:59:32 » est la question qu'on se pose devant un journal.
            return [entry.message, entry.details, entry.raw, entry.clock]
                .some((field) => (field || "").toLowerCase().includes(needle));
        });
    }, [entries, values]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * LINES_PER_PAGE, page * LINES_PER_PAGE),
        [filtered, page],
    );

    if (error) {
        return <bs.Alert variant="danger">{error}</bs.Alert>;
    }
    if (entries === null) {
        return <div className="text-center py-5"><bs.Spinner animation="border" /></div>;
    }

    const searchContext = { values, setValues };

    return (
        <SectionPanel>
            <div className="d-flex justify-content-between align-items-center mb-3">
                <span className="text-muted small">{t("admin.storage.lastLines")}</span>
                {/* Sur le téléphone, le journal EST déjà dans `gameflex/logs` :
                    rien ne se télécharge hors du dossier gameflex (le porteur,
                    2026-10-12). */}
                {!hasLocalLibrary() && (
                    <bs.Button variant="secondary" href={logFileUrl(path)} target="_blank" rel="noreferrer">
                        <i className="bi bi-download me-2"></i>
                        {t("admin.storage.download")}
                    </bs.Button>
                )}
            </div>
            <ListingElement
                callback={(entry, index) => (
                    <ListElement
                        key={`${entry.clock || ""}-${index}`}
                        icon="bi bi-chevron-right"
                        label={[entry.message || entry.raw, entry.details]
                            .filter(Boolean).join(" — ")}
                        helpText={[entry.clock, entry.level || t("admin.storage.rawLine")]
                            .filter(Boolean).join(" · ")}
                    />
                )}
                items={visible}
                itemCount={filtered.length}
                itemsPerPage={LINES_PER_PAGE}
                currentPage={page}
                onPositionChange={setPage}
                search={{
                    context: searchContext,
                    placeholder: t("admin.storage.searchLine"),
                    filters: {
                        Component: () => (
                            <Form.Group
                                label={t("admin.storage.filterLevel")}
                                helpText={t("admin.storage.filterLevelHelp")}
                            >
                                <Form.Multicheckbox
                                    name="levels"
                                    context={searchContext}
                                    options={levels.map((level) => ({
                                        value: level,
                                        label: level === RAW_LEVEL
                                            ? t("admin.storage.rawLine")
                                            : level,
                                    }))} />
                            </Form.Group>
                        ),
                    },
                }}
                texts={{
                    itemsFound: t("admin.storage.linesFound"),
                    noItemsFound: t("admin.storage.noLine"),
                }}
            />
        </SectionPanel>
    );
}

export default function Logs({ t }) {
    // DEUX BORNES PLUTÔT QU'UN `DateRange`, et c'est la largeur qui décide :
    // le panneau de filtres fait 450 px, et `Form.DateRange` y pose deux
    // calendriers côte à côte — les sélecteurs d'année et de mois s'y
    // tronquent à « 202€ » et « Septen ». Deux `Form.Date` empilés (300 px
    // chacun) tiennent, et se bornent l'un l'autre pour retrouver ce qu'un
    // intervalle garantit : la fin ne peut pas précéder le début.
    //
    // Vide des deux côtés = rien d'écarté. Un filtre qui n'a pas servi ne
    // doit pas cacher la moitié des journaux.
    const [values, setValues] = useState({ q: "", from: null, to: null });
    const [logs, setLogs] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [isBusy, setBusy] = useState(false);

    // Le contexte À JOUR, sans que la lecture en dépende : il change d'identité
    // à chaque fenêtre qui s'ouvre (voir `useLatestContext`).
    const latestContext = useLatestContext();
    const context = latestContext.current;

    const reload = useCallback(() => {
        setLoading(true);

        return fetchLogs()
            .then((result) => {
                if (!result.ok) {
                    latestContext.current.toast.open(result.errorMessage || t("admin.storage.loadFailed"),
                        { variant: "error" });
                }
                setLogs(result.logs || []);
            })
            .catch((failure) => {
                console.error("Chargement de la liste des journaux", failure);
                latestContext.current.toast.open(t("admin.storage.loadFailed"), { variant: "error" });
            })
            .finally(() => setLoading(false));
    }, [t, latestContext]);

    useEffect(() => {
        reload();
    }, [reload]);

    const filtered = useMemo(() => {
        const needle = (values.q || "").trim().toLowerCase();
        // Le jour d'un journal EST son dossier (`20260909`) — on compare
        // donc des chaînes de huit chiffres, sans jamais construire de Date.
        // Passer par un objet Date ferait entrer un fuseau horaire dans une
        // comparaison qui n'en a pas : un journal du 9 septembre est du 9
        // septembre, où qu'on le lise.
        const floor = values.from ? values.from.replaceAll("-", "") : null;
        const ceiling = values.to ? values.to.replaceAll("-", "") : null;

        return logs.filter((log) => {
            if (needle && !log.path.toLowerCase().includes(needle)) {
                return false;
            }
            // UN JOURNAL SANS DATE ÉCHAPPE AUX BORNES — il ne s'y dérobe pas.
            // `reports.log` (#191) vit à la racine, hors des dossiers datés, et
            // n'a donc pas de jour. Le comparer à une borne le ferait
            // disparaître dès qu'on en pose une, puisque la comparaison porte
            // sur des chaînes et que le vide est plus petit que tout — un
            // fichier qu'on cherche précisément quand quelque chose a échoué.
            if (!log.day) {
                return true;
            }
            if (floor && log.day < floor) {
                return false;
            }
            if (ceiling && log.day > ceiling) {
                return false;
            }

            return true;
        });
    }, [logs, values.q, values.from, values.to]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    // Une ligne supprimée sur la dernière page : on recule d'une page plutôt
    // que de montrer une page vide.
    useEffect(() => {
        const last = Math.max(1, Math.ceil(filtered.length / ITEMS_PER_PAGE));
        if (page > last) {
            setPage(last);
        }
    }, [filtered.length, page]);

    const open = (log) => {
        context.offcanvas.open(
            <OffCanvas icon="bi bi-journal-text" label={log.name} helpText={log.path}>
                <LogView path={log.path} t={t} />
            </OffCanvas>, { size: 800 });
    };

    const handleDelete = async (log) => {
        const result = await deleteLog(log.path);
        context.toast.open(
            result.ok
                ? t("admin.storage.logDeleted", { name: log.name })
                : (result.errorMessage || t("admin.storage.deleteFailed")),
            { variant: result.ok ? "success" : "error" },
        );
        // La ligne part, sur place : pas de relecture (2026-10-10).
        if (result.ok) {
            setLogs((current) => current.filter((entry) => entry.path !== log.path));
        }
    };

    // SUPPRIMER CE QUE LE FILTRE MONTRE, ET RIEN D'AUTRE.
    //
    // Le serveur ne connaît pas ces filtres : il rend TOUS les journaux et
    // c'est l'écran qui les trie (voir `filtered` plus haut). Une route « tout
    // supprimer » côté kernel devrait donc réimplémenter la même logique en
    // Go — et le téléphone sert les mêmes routes, ce qui en ferait aussitôt
    // une troisième copie à garder d'accord avec les deux autres. On envoie
    // donc N suppressions unitaires : le navigateur les sérialise déjà
    // lui-même (six connexions par origine), il n'y a pas de file à écrire.
    //
    // Et on compte les échecs plutôt que de les taire : supprimer trente
    // journaux sur trente-deux n'est pas un succès, et l'écran doit le dire.
    const handleDeleteAll = async () => {
        setBusy(true);

        try {
            const results = await Promise.all(filtered.map((log) => deleteLog(log.path)));
            const deleted = results.filter((result) => result.ok).length;
            const failed = results.length - deleted;

            context.toast.open(
                failed === 0
                    ? t("admin.storage.logsDeleted", { count: deleted })
                    : t("admin.storage.logsDeletedPartial", { deleted, failed }),
                { variant: failed === 0 ? "success" : "error" },
            );
            // Partent les lignes supprimées, restent celles qui ont résisté —
            // la réponse de chaque suppression le dit, rien à relire.
            const gone = new Set(filtered.filter((_, index) => results[index].ok).map((log) => log.path));
            setLogs((current) => current.filter((entry) => !gone.has(entry.path)));
        } catch (failure) {
            console.error("Suppression de tous les journaux filtrés", failure);
            context.toast.open(t("admin.storage.deleteFailed"), { variant: "error" });
            // Une suppression en lot qui jette a déjà supprimé une partie des
            // fichiers : sans cette relecture, l'écran continue de lister des
            // journaux qui n'existent plus, et les ouvrir échoue.
            reload();
        } finally {
            setBusy(false);
        }
    };

    const confirmDeleteAll = () => context.modal.open(
        <div className="text-center">
            {t("admin.storage.deleteAllLogsConfirm", { count: filtered.length })}
        </div>,
        {
            variant: "confirm",
            onConfirm: handleDeleteAll,
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
        },
    );

    const searchContext = { values, setValues };

    return (
        <SectionPanel>
            <ListingElement
                callback={(log) => (
                    <ListElement
                        key={log.path}
                        icon="bi bi-journal-text"
                        label={log.name}
                        helpText={[
                            log.session
                                ? t("admin.storage.sessionAt", { day: log.day, time: log.session })
                                : log.day,
                            formatSize(log.sizeBytes),
                        ].filter(Boolean).join(" · ")}
                        onClick={() => open(log)}
                        next={(
                            <bs.Button
                                variant="danger"
                                onClick={(e) => {
                                    e.preventDefault();
                                    e.stopPropagation();
                                    context.modal.open(
                                        <div className="text-center">
                                            {t("admin.storage.deleteLogConfirm")} <br />
                                            <strong>{log.path}</strong>
                                        </div>,
                                        {
                                            variant: "confirm",
                                            onConfirm: () => handleDelete(log),
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
                    placeholder: t("admin.storage.searchLog"),
                    filters: {
                        Component: () => (
                            <div>
                                <Form.Group
                                    label={t("admin.storage.filterFrom")}
                                    helpText={t("admin.storage.filterDatesHelp")}
                                >
                                    <Form.Date
                                        name="from"
                                        context={searchContext}
                                        maxDate={values.to || null} />
                                </Form.Group>
                                <Form.Group label={t("admin.storage.filterTo")} helpText={t("admin.storage.filterToHelp")}>
                                    <Form.Date
                                        name="to"
                                        context={searchContext}
                                        minDate={values.from || null} />
                                </Form.Group>
                            </div>
                        ),
                    },
                }}
                texts={{
                    itemsFound: t("admin.storage.logsFound"),
                    noItemsFound: t("admin.storage.noLogs"),
                }}
            />
            <Form.Group
                label={t("admin.storage.deleteAllLabel", { count: filtered.length })}
                helpText={t("admin.storage.deleteAllHelp")}
                props={{ style: { marginBottom: 0, marginTop: "1rem" } }}>
                <FormButton
                    variant="danger"
                    callback={confirmDeleteAll}
                    label={t("admin.storage.deleteAllLogs")}
                    isDisabled={isLoading || isBusy || filtered.length === 0}
                    isPending={isBusy} />
            </Form.Group>
        </SectionPanel>
    );
}
