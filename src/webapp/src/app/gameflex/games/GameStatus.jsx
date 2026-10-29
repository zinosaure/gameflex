import React, { useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { OffCanvas, SectionPanel } from "../shared/templates/Essentials.jsx";
import { fetchConsoleStatus, fetchGameStatus, reportGameStatus, setConsoleStatus } from "../api/retrogaming.js";

const Form = Skeleton.Form;

// LE STATUT D'UN JEU (chantier 30) — jouable, plante en jeu, injouable, non
// testé. Le serveur le calcule (`rom.status`, gamestatus.Resolve) ; la page ne
// fait que le montrer, et un administrateur le régler.
//
// UN SEUL BADGE pour tous les écrans — la liste des jeux, Réglages › Jeux, la
// fiche : un écart se définit une fois. Variantes pleines.

/** Les statuts, dans l'ordre où les écrans les proposent. */
export const GAME_STATUSES = ["playable", "crashes", "unplayable", "untested"];

const VARIANTS = { playable: "success", crashes: "warning", unplayable: "danger", untested: "secondary" };

/** « Eden (streamex) » → « Eden » : le nom court d'un émulateur, pour un badge. */
export function emulatorLabel(name) {
    return (name || "").replace(/\s*\(streamex\)$/, "");
}

// yyyy-mm-dd hh:mm, comme Membres et Profil.
const formatDateTime = (iso) => {
    if (!iso) return "—";
    const d = new Date(iso);
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

/**
 * Le badge d'un jeu. `status` est la vue du serveur ({status, emulator}) ;
 * rien quand il n'y en a pas — un jeu d'un partage monté, que personne ne
 * signale.
 */
export function GameStatusBadge({ status, t }) {
    if (!status?.status || !VARIANTS[status.status]) {
        return null;
    }
    const label = t(`gameStatus.${status.status}`);

    return (
        <bs.Badge
            bg={VARIANTS[status.status]}
            text={status.status === "crashes" ? "dark" : undefined}
            className="ms-2 align-middle fw-normal">
            {status.emulator ? `${label} · ${emulatorLabel(status.emulator)}` : label}
        </bs.Badge>
    );
}

/** Les notes d'un jeu, de la plus récente à la plus ancienne. */
function StatusNotes({ notes, t }) {
    if (!notes?.length) {
        return null;
    }

    return (
        <div className="d-flex flex-column gap-2 mt-3">
            {notes.map((note, index) => (
                <div key={`${note.at}-${index}`} className="small">
                    <GameStatusBadge status={{ status: note.status }} t={t} />
                    <span className="ms-2">{note.note}</span>
                    <div className="text-muted">
                        {t("gameStatus.reportedBy", { by: note.by, at: formatDateTime(note.at) })}
                        {note.emulators?.length ? ` · ${note.emulators.map(emulatorLabel).join(", ")}` : ""}
                    </div>
                </div>
            ))}
        </div>
    );
}

/**
 * Le statut dans la fiche d'un jeu, pour tous : par émulateur quand la
 * console en a plusieurs, et les notes. Jamais la version : un joueur n'en
 * ferait rien (le porteur, 2026-10-26).
 */
export function GameStatusDetails({ status, t }) {
    if (!status?.status) {
        return null;
    }
    const emulators = Object.entries(status.emulators || {});

    return (
        <SectionPanel label={t("gameStatus.title")} helpText={t("gameStatus.detailHelp")}>
            {emulators.length > 1 ? (
                <div className="d-flex flex-column gap-1">
                    {emulators.map(([name, value]) => (
                        <div key={name} className="d-flex align-items-center justify-content-between">
                            <span>{emulatorLabel(name)}</span>
                            <GameStatusBadge status={{ status: value }} t={t} />
                        </div>
                    ))}
                </div>
            ) : (
                <GameStatusBadge status={{ status: status.status }} t={t} />
            )}
            <StatusNotes notes={status.notes} t={t} />
        </SectionPanel>
    );
}

/**
 * LE PANNEAU D'UN JEU, dans Réglages › Jeux : le statut, les émulateurs qu'il
 * concerne (tous cochés par défaut — le porteur : « par défaut le tout »), une
 * note, puis l'historique avec les versions, que seul l'administrateur lit.
 *
 * Il tient son propre état : les props d'un panneau ouvert sont figées
 * (même patron que AddServerPanel).
 */
export function GameStatusPanel({ item, t, context, onChanged }) {
    const [answer, setAnswer] = useState(null);
    const [values, setValues] = useState({ status: item.status?.status || "playable", emulators: [], note: "" });
    const [isPending, setPending] = useState(false);

    const installed = (answer?.emulators || []).filter((emulator) => emulator.installed).map((emulator) => emulator.name);

    useEffect(() => {
        let cancelled = false;
        fetchGameStatus(item.consoleId, item.reference).then((result) => {
            if (cancelled || !result.ok) {
                return;
            }
            setAnswer(result.data);
            const names = (result.data.emulators || []).filter((emulator) => emulator.installed).map((emulator) => emulator.name);
            setValues((previous) => ({ ...previous, emulators: names }));
        });

        return () => {
            cancelled = true;
        };
    }, [item.consoleId, item.reference]);

    const save = async () => {
        setPending(true);
        try {
            // TOUS COCHÉS = TOUS, et pas « ces deux-là » : un émulateur
            // installé plus tard suivra ce signalement.
            const chosen = values.emulators.length === installed.length ? [] : values.emulators;
            const result = await reportGameStatus(item.consoleId, item.reference, {
                status: values.status, emulators: chosen, note: values.note,
            });
            if (!result.ok) {
                context.toast.open(result.errorMessage || t("gameStatus.failed"), { variant: "error" });

                return;
            }
            setAnswer(result.data);
            setValues((previous) => ({ ...previous, note: "" }));
            context.toast.open(t("gameStatus.saved"), { variant: "success" });
            onChanged?.();
        } finally {
            setPending(false);
        }
    };

    const formContext = { values, setValues };
    const reports = [...(answer?.reports || [])].reverse();

    return (
        <OffCanvas
            icon="bi bi-clipboard-check"
            label={item.title}
            helpText={t("gameStatus.help")}
            callback={save}
            buttonLabel={t("gameStatus.save")}
            isDisabled={isPending || !answer || (installed.length > 1 && values.emulators.length === 0)}
            isPending={isPending}>
            <SectionPanel>
                <div className="mb-3">
                    <GameStatusBadge status={answer?.view || item.status} t={t} />
                </div>
                <Form.Group label={t("gameStatus.statusLabel")}>
                    <Form.Select
                        name="status"
                        context={formContext}
                        options={GAME_STATUSES.map((value) => ({ value, label: t(`gameStatus.${value}`) }))} />
                </Form.Group>
                {installed.length > 1 && (
                    <Form.Group label={t("gameStatus.emulatorsLabel")} helpText={t("gameStatus.emulatorsHelp")}>
                        <Form.Multicheckbox
                            name="emulators"
                            context={formContext}
                            options={installed.map((name) => ({ value: name, label: emulatorLabel(name) }))} />
                    </Form.Group>
                )}
                <Form.Group label={t("gameStatus.noteLabel")} helpText={t("gameStatus.noteHelp")}>
                    <Form.Longtext name="note" context={formContext} props={{ maxLength: 500, rows: 3 }} />
                </Form.Group>
            </SectionPanel>
            <SectionPanel label={t("gameStatus.history")}>
                {reports.length === 0 && <div className="text-muted">{t("gameStatus.historyEmpty")}</div>}
                <div className="d-flex flex-column gap-2">
                    {reports.map((report, index) => (
                        <div key={`${report.at}-${index}`} className="small">
                            <GameStatusBadge status={{ status: report.status }} t={t} />
                            <span className="ms-2">
                                {report.emulators?.length
                                    ? report.emulators.map(emulatorLabel).join(", ")
                                    : t("gameStatus.allEmulators")}
                            </span>
                            {report.note && <div>{report.note}</div>}
                            <div className="text-muted">
                                {t("gameStatus.reportedBy", { by: report.by, at: formatDateTime(report.at) })}
                                {report.versions && Object.keys(report.versions).length > 0 && ` · ${Object.entries(report.versions)
                                    .map(([name, version]) => `${emulatorLabel(name)} ${version}`).join(", ")}`}
                            </div>
                        </div>
                    ))}
                </div>
            </SectionPanel>
        </OffCanvas>
    );
}

/**
 * TOUTE UNE CONSOLE D'UN GESTE (le porteur : « on va update les statuts pour
 * faire passer à jouable pour tous […] ce serait difficile d'aller tester les
 * ~23000 jeux un par un »). Chaque jeu sans statut à lui prend celui-ci, y
 * compris un jeu ajouté plus tard ; un jeu signalé garde le sien.
 */
export function ConsoleStatusPanel({ consoles, t, context, onChanged }) {
    const [values, setValues] = useState({ console: consoles[0]?.id || "", status: "playable" });
    const [current, setCurrent] = useState(null);
    const [isPending, setPending] = useState(false);

    useEffect(() => {
        if (!values.console) {
            return undefined;
        }
        let cancelled = false;
        setCurrent(null);
        fetchConsoleStatus(values.console).then((result) => {
            if (!cancelled && result.ok) {
                setCurrent(result.data);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [values.console]);

    const nameOf = (consoleId) => consoles.find((console) => console.id === consoleId)?.name || consoleId;

    const save = async () => {
        setPending(true);
        try {
            const result = await setConsoleStatus(values.console, values.status);
            if (!result.ok) {
                context.toast.open(result.errorMessage || t("gameStatus.failed"), { variant: "error" });

                return;
            }
            setCurrent(result.data);
            context.toast.open(t("gameStatus.consoleSaved", {
                console: nameOf(values.console), status: t(`gameStatus.${values.status}`),
            }), { variant: "success" });
            onChanged?.();
        } finally {
            setPending(false);
        }
    };

    const formContext = { values, setValues };

    return (
        <OffCanvas
            icon="bi bi-collection"
            label={t("gameStatus.consoleTitle")}
            helpText={t("gameStatus.consoleHelp")}
            callback={save}
            buttonLabel={t("gameStatus.save")}
            isDisabled={isPending || !values.console}
            isPending={isPending}>
            <SectionPanel>
                <Form.Group label={t("gameStatus.consoleLabel")}>
                    <Form.Select
                        name="console"
                        context={formContext}
                        options={consoles.map((console) => ({ value: console.id, label: console.name }))} />
                </Form.Group>
                <div className="text-muted small mb-3">
                    {current?.status && t("gameStatus.consoleCurrent", {
                        status: t(`gameStatus.${current.status}`), by: current.by, at: formatDateTime(current.at),
                    })}
                    {current && !current.status && t("gameStatus.consoleNone", { status: t(`gameStatus.${current.default}`) })}
                </div>
                <Form.Group label={t("gameStatus.statusLabel")}>
                    <Form.Select
                        name="status"
                        context={formContext}
                        options={GAME_STATUSES.map((value) => ({ value, label: t(`gameStatus.${value}`) }))} />
                </Form.Group>
            </SectionPanel>
        </OffCanvas>
    );
}

/**
 * Le filtre par statut, pour le panneau de recherche d'une liste : les mêmes
 * cases sur les deux écrans.
 */
export function StatusFilter({ context, t }) {
    return (
        <Form.Group label={t("gameStatus.filterLabel")} helpText={t("gameStatus.filterHelp")}>
            <Form.Multicheckbox
                name="statuses"
                context={context}
                options={GAME_STATUSES.map((value) => ({ value, label: t(`gameStatus.${value}`) }))} />
        </Form.Group>
    );
}

/** Le jeu passe-t-il le filtre ? Rien de coché : tous. */
export function matchesStatuses(item, statuses) {
    return !statuses?.length || statuses.includes(item.status?.status || "");
}
