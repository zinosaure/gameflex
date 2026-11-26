// Route : /desktop — L'ACCUEIL DE LA CONSOLE DU PC, comme celui de l'APK : la bannière qui ouvre GameFlex, les tuiles
// (démarrer ou arrêter le serveur, le redémarrer, Inviter, Rejoindre), puis Serveur (le port — ce qu'un téléphone n'a
// pas), Stockage, Compte root, À propos ; chaque section son sens et sa description (chantier 81).
import React from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { LAUNCH_BACKGROUND } from "../../engine/stream/shared/launch-backdrop.js";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { QuickTiles } from "../../bricks/QuickTiles.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { DesktopPage, useDesktopState } from "./shared.jsx";

export default function Desktop() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { state, act } = useDesktopState();
    const running = Boolean(state.running);
    const accounts = state.accounts || {};

    return (
        <DesktopPage guides={[["install"], ["remoteAccess"]]}>
            {/* LA BANNIÈRE : elle ouvre GameFlex, comme le rond de la barre du haut de l'APK. */}
            <button type="button" className="btn p-0 border-0 w-100 mt-3 position-relative" data-nav data-key="dc-open" disabled={!running}
                onClick={() => { window.location.href = state.localUrl; }}>
                <div className="gbanner ratio ratio-16x9 rounded-4 overflow-hidden">
                    <img className="object-fit-cover" src={LAUNCH_BACKGROUND} alt="" />
                </div>
                <span className="position-absolute bottom-0 start-50 translate-middle-x mb-3 fs-3 fw-bold text-white">
                    <i className="bi bi-play-fill me-2" aria-hidden="true"></i>{t("desktop.open")}
                </span>
            </button>
            <QuickTiles items={[
                { id: "dc-power", icon: running ? "bi-stop-circle" : "bi-play-circle", tone: running ? "rose" : "green",
                    label: t(running ? "desktop.stop" : "desktop.start"), value: t(running ? "desktop.running" : "desktop.stopped"), onClick: () => act("/power", { on: !running }) },
                { id: "dc-restart", icon: "bi-arrow-repeat", tone: "amber", label: t("desktop.restart"), value: t("desktop.restartHelp"), disabled: !running, onClick: () => act("/restart", {}) },
                { id: "dc-invite", icon: "bi-qr-code", tone: "green", label: t("desktop.invite"), value: t("desktop.inviteHelp"), disabled: !running, onClick: () => navigate("/desktop/invite") },
                { id: "dc-join", icon: "bi-box-arrow-in-right", tone: "sky", label: t("desktop.join"), value: t("desktop.joinHelp"), onClick: () => navigate("/desktop/join") },
            ]} />
            <Section title={t("desktop.server")} help={t("desktop.serverHelp")} sense="modify">
                <Rows>
                    <NavRow id="dc-port" icon="bi-ethernet" label={t("desktop.port")} value={String(state.port)} onOpen={() => navigate("/desktop/port")} />
                </Rows>
            </Section>
            <Section title={t("desktop.storage")} help={t("desktop.storageHelp")} sense="modify">
                <Rows>
                    <NavRow id="dc-location" icon="bi-folder2-open" label={t("desktop.location")} value={state.library} onOpen={() => navigate("/desktop/location")} />
                    <NavRow id="dc-disks" icon="bi-device-hdd" label={t("desktop.disks")}
                        value={(state.disks || []).length ? t("desktop.disksCount", { count: state.disks.length }) : t("desktop.disksNone")} onOpen={() => navigate("/desktop/disks")} />
                </Rows>
            </Section>
            <Section title={t("desktop.rootAccount")} help={t("desktop.rootAccountHelp")} sense="modify">
                <Rows>
                    <NavRow id="dc-root" icon="bi-key" label={t("desktop.rootPassword")}
                        value={running ? [`@${accounts.root || "root"}`, t("desktop.adminsCount", { count: accounts.admins || 0 })] : t("desktop.stopped")}
                        disabled={!running} onOpen={() => navigate("/desktop/account")} />
                </Rows>
            </Section>
            <Section title={t("desktop.about")} help={t("desktop.aboutHelp")} sense="optional">
                <Rows>
                    <NavRow id="dc-update" icon="bi-cloud-arrow-down" label={t("desktop.update")} value={t("desktop.updateValue", { version: state.version })}
                        onOpen={() => navigate("/desktop/update")} />
                    <InfoRow icon="bi-pc-display" label={t("desktop.machine")} value={state.device} />
                    <InfoRow icon="bi-tag" label={t("desktop.version")} value={state.version} />
                </Rows>
            </Section>
        </DesktopPage>
    );
}
