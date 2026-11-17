import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { can } from "../../libs/capabilities.js";
import { streamHostOrigin } from "../../libs/serverBase.js";
import { describeResources } from "../../libs/hostResources.js";

// LES PERFORMANCES D'UNE PARTIE — le panneau (toutes les mesures, en haut de l'écran sur un voile) et la
// vue discrète (les chiffres principaux, en petit, sur deux lignes : l'émulateur, puis le flux). Le
// relevé vient du moteur (`input.onMonitoringResource`, use-session-stats.js de la webapp) ; ce fichier
// ne fait que l'écrire. Les deux s'excluent (le menu, Menu.jsx).
//
// LES FORMATS S'ÉCRIVENT UNE FOIS pour les deux (la règle de l'homogénéité) : « 2.5 Go (+300 Mo) / 8.0 Go »
// se lit pareil aux deux endroits. Un chiffre absent est un tiret ; NaN et undefined aussi.

const finite = (value) => (typeof value === "number" && Number.isFinite(value) ? value : null);
const rate = (kbps) => (finite(kbps) != null ? `${Math.round(kbps)} kbps` : "—");
const frameRate = (fps) => (finite(fps) != null ? Math.round(fps) : "—");
const latency = (ms) => (finite(ms) != null ? `${Math.round(ms)} ms` : "—");
const streamResolution = (resolution) => (finite(resolution?.width) != null && finite(resolution?.height) != null ? `${resolution.width}x${resolution.height}` : "—");

// Mo sous le gigaoctet, Go au-dessus.
function bytes(value) {
    const amount = finite(value);

    if (amount == null) {
        return null;
    }

    return amount >= 1024 ** 3 ? `${(amount / 1024 ** 3).toFixed(1)} Go` : `${Math.round(amount / 1024 ** 2)} Mo`;
}

// L'ÉCART DEPUIS LE DÉBUT DE LA PARTIE (le départ gardé par le serveur, `stats.start`) : « (+14 °C) ».
// Le signe suit le chiffre affiché : « ±0 » pour un écart qui s'arrondit à zéro.
function delta(from, field, format) {
    const now = finite(from?.[field]);
    const then = finite(from?.start?.[field]);

    if (now == null || then == null) {
        return "";
    }
    const difference = Math.round(now - then);

    return ` (${difference > 0 ? "+" : difference < 0 ? "−" : "±"}${format(Math.abs(difference))})`;
}

function bytesDelta(stats, field) {
    const now = finite(stats?.[field]);
    const then = finite(stats?.start?.[field]);

    if (now == null || then == null || then === 0) {
        return "";
    }
    const difference = bytes(Math.abs(now - then));

    return ` (${difference === bytes(0) ? "±" : now < then ? "−" : "+"}${difference})`;
}

// Une mémoire, son écart, puis son plafond quand il y en a un — en deux morceaux (la vue discrète écrit
// la suite plus petit).
function memory(stats, usedField, totalField = null) {
    const used = bytes(stats?.[usedField]);
    const total = totalField ? bytes(stats?.[totalField]) : null;

    return used == null ? null : { used, suffix: `${bytesDelta(stats, usedField)}${total ? ` / ${total}` : ""}` };
}

// La scène que l'émulateur fait calculer, et sa réduction par rapport à l'image du jeu.
function emulatorResolution(stats) {
    const scene = stats?.emulatorResolution;

    if (!scene) {
        return null;
    }
    const sceneHeight = Number(String(scene).split("x")[1]);
    const gameHeight = Number(String(stats?.gameResolution || "").split("x")[1]);

    if (!(sceneHeight > 0) || !(gameHeight > 0)) {
        return { scene, reduction: "" };
    }
    const percent = Math.round((sceneHeight / gameHeight - 1) * 100);

    return { scene, reduction: ` (${percent > 0 ? "+" : percent < 0 ? "−" : ""}${Math.abs(percent)} %)` };
}

// « En cours depuis 00:07:12 », qui avance : le serveur donne l'instant du départ, la page compte.
function useElapsed(startedAt) {
    const started = useMemo(() => {
        const parsed = startedAt ? new Date(startedAt).getTime() : NaN;

        return Number.isFinite(parsed) ? parsed : null;
    }, [startedAt]);
    const [, tick] = useState(0);

    useEffect(() => {
        if (started == null) {
            return undefined;
        }
        const timer = setInterval(() => tick((n) => n + 1), 1000);

        return () => clearInterval(timer);
    }, [started]);

    if (started == null) {
        return null;
    }
    const seconds = Math.max(0, Math.floor((Date.now() - started) / 1000));
    const pad = (n) => String(n).padStart(2, "0");

    return `${pad(Math.floor(seconds / 3600))}:${pad(Math.floor(seconds / 60) % 60)}:${pad(seconds % 60)}`;
}

function Section({ title, first = false }) {
    return <div className={`play-stats-section fw-bold text-uppercase small text-white text-opacity-50 ${first ? "" : "border-top border-secondary pt-2 mt-2"}`}>{title}</div>;
}

function Line({ label, children }) {
    return (
        <div className="d-flex justify-content-between gap-3">
            <span className="text-white text-opacity-75">{label}</span>
            <span className="text-end">{children}</span>
        </div>
    );
}

// LE PANNEAU, en colonnes sur toute la largeur et en sections : le téléphone qu'on tient (dans
// l'application seulement), la partie, le flux, l'émulateur, la machine.
export function StatsPanel({ stats, session }) {
    const { t } = useTranslation();
    const uptime = useElapsed(stats?.startedAt);
    const inApp = can("devicePower");
    const machinePower = inApp && Boolean(streamHostOrigin());
    const maxSlots = session?.maxPlayers || ((session?.playerCount ?? 0) + (session?.openSlots ?? 0));
    const padSlot = session?.me?.padSlot;
    const scene = emulatorResolution(stats);
    const emulatorMemory = memory(stats, "rssBytes");
    const containerMemory = memory(stats, "containerRssBytes", "containerLimitBytes");
    const videoMemory = memory(stats, "vramUsedBytes", "vramTotalBytes");
    const percent = (value) => `${value} %`;
    const degrees = (value) => `${value} °C`;

    return (
        <div className="play-stats position-absolute top-0 start-0 w-100 p-3" data-stats-panel="">
            <div className="play-stats-grid">
                {inApp && (
                    <>
                        <Section title={t("play.stats.sections.phone")} first />
                        <Line label={t("play.stats.phoneTemperature")}>{finite(stats?.device?.temperatureCelsius) != null ? `${Math.round(stats.device.temperatureCelsius)} °C${delta(stats.device, "temperatureCelsius", degrees)}` : "—"}</Line>
                        <Line label={t("play.stats.phoneBattery")}>{finite(stats?.device?.batteryPercent) != null ? `${Math.round(stats.device.batteryPercent)} %${delta(stats.device, "batteryPercent", percent)}` : "—"}</Line>
                    </>
                )}
                <Section title={t("play.stats.sections.session")} first={!inApp} />
                <Line label={t("play.stats.uptime")}><span className="font-monospace">{uptime ?? "—"}</span></Line>
                <Line label={t("play.stats.slot")}>{padSlot != null && maxSlots > 0 ? `${padSlot}/${maxSlots}` : "—"}</Line>

                <Section title={t("play.stats.sections.gateway")} />
                <Line label={t("play.stats.resolution")}>{streamResolution(stats?.resolution)}</Line>
                <Line label={t("play.stats.fps")}>{frameRate(stats?.fps)}</Line>
                <Line label={t("play.stats.downloadRate")}>{rate(stats?.downloadKbps)}</Line>
                <Line label={t("play.stats.uploadRate")}>{rate(stats?.uploadKbps)}</Line>
                <Line label={t("play.stats.bitrate")}>{rate(stats?.videoBitrateKbps)}</Line>
                <Line label={t("play.stats.latency")}>{latency(stats?.latencyMs)}</Line>

                <Section title={t("play.stats.sections.emulator")} />
                <Line label={t("play.stats.emulator")}>{stats?.emulator || "—"}</Line>
                <Line label={t("play.stats.gameResolution")}>{stats?.gameResolution || "—"}</Line>
                <Line label={t("play.stats.emulatorResolution")}>{scene ? scene.scene + scene.reduction : "—"}</Line>
                <Line label={t("play.stats.emulationFps")}>{frameRate(stats?.emulationFps)}</Line>
                <Line label={t("play.stats.cpuEmulator")}>{finite(stats?.cpuPercent) != null ? `${Math.round(stats.cpuPercent)} %${delta(stats, "cpuPercent", percent)}` : "—"}</Line>
                <Line label={t("play.stats.ram")}>{emulatorMemory ? emulatorMemory.used + emulatorMemory.suffix : "—"}</Line>
                <Line label={t("play.stats.containerRam")}>{containerMemory ? containerMemory.used + containerMemory.suffix : "—"}</Line>
                <Line label={t("play.stats.diskRead")}>{finite(stats?.diskReadBytesPerSec) != null ? `${(stats.diskReadBytesPerSec / 1024 ** 2).toFixed(stats.diskReadBytesPerSec < 10 * 1024 ** 2 ? 1 : 0)} Mo/s` : "—"}</Line>

                <Section title={t("play.stats.sections.host")} />
                <Line label={t("play.stats.host")}>{describeResources(stats?.host, t) || "—"}</Line>
                {machinePower && (
                    <>
                        <Line label={t("play.stats.temperature")}>{finite(stats?.temperatureCelsius) != null ? `${Math.round(stats.temperatureCelsius)} °C${delta(stats, "temperatureCelsius", degrees)}` : "—"}</Line>
                        <Line label={t("play.stats.battery")}>{finite(stats?.batteryPercent) != null ? `${Math.round(stats.batteryPercent)} %${delta(stats, "batteryPercent", percent)}` : "—"}</Line>
                    </>
                )}
                <Line label={t("play.stats.gpuBusyPercent")}>{finite(stats?.gpuBusyPercent) != null ? `${Math.round(stats.gpuBusyPercent)} %` : "—"}</Line>
                <Line label={t("play.stats.vramUsedBytes")}>{videoMemory ? videoMemory.used + videoMemory.suffix : "—"}</Line>
            </div>
        </div>
    );
}

// LA VUE DISCRÈTE : deux lignes, l'émulateur puis le flux, séparées par « • » ; sans voile (un voile
// teinterait ce que l'émulateur dessine dans l'image), l'ombre seule détache le texte. Remote Play
// n'émule rien : la ligne du flux seule. En portrait, sous la vidéo, à hauteur fixe ; en paysage, en haut.
export const STATS_STRIP_HEIGHT = 32;

export function StatsStrip({ stats, session, layout }) {
    const { t } = useTranslation();
    const value = (text) => <span className="text-white text-opacity-75">{text}</span>;
    const note = (text) => (text ? <small className="text-white text-opacity-50">{text}</small> : null);
    const frames = (fps) => <><span className="text-white fw-semibold">{frameRate(fps)}</span>{value(` ${t("play.stats.fpsUnit")}`)}</>;
    const joined = (parts) => parts.map((part, index) => (
        <React.Fragment key={index}>
            {index > 0 && <span className="text-white text-opacity-50">{" • "}</span>}
            {part}
        </React.Fragment>
    ));
    const scene = emulatorResolution(stats);
    const emulatorMemory = memory(stats, "rssBytes");
    const videoMemory = memory(stats, "vramUsedBytes", "vramTotalBytes");
    const lines = [
        session?.platform !== "remoteplay" && {
            key: "emulator",
            content: joined([
                scene ? <>{value(scene.scene)}{note(scene.reduction)}</> : value(stats?.gameResolution || "—"),
                frames(stats?.emulationFps),
                <>{value(emulatorMemory?.used ?? "—")}{note(emulatorMemory?.suffix)}</>,
                <>{value(videoMemory?.used ?? "—")}{note(videoMemory?.suffix)}</>,
            ]),
        },
        {
            key: "gateway",
            content: joined([value(streamResolution(stats?.resolution)), frames(stats?.fps), value(latency(stats?.latencyMs)), value(rate(stats?.videoBitrateKbps))]),
        },
    ].filter(Boolean);

    return (
        <div className="play-strip d-flex flex-column justify-content-center px-1 text-center" data-stats-strip={layout} style={layout === "portrait" ? { height: STATS_STRIP_HEIGHT } : undefined}>
            {lines.map((line) => <div key={line.key} className="text-truncate">{line.content}</div>)}
        </div>
    );
}
