import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { can, useData } from "../../libs/data.jsx";
import { setLanguage } from "../../i18n.js";
import { playSound, setSoundsEnabled, soundsEnabled } from "../../libs/sounds.js";
import { PushSection, usePush } from "./Push.jsx";
import SettingsPage, { usePageText } from "../SettingsPage.jsx";
import { EmptyState, OrientationFailed, OrientationRows, QuickTiles, Rows, SelectRow, Section, SwitchRow, offersOrientation } from "../../ui/Bricks.jsx";
import { toast } from "../../shell/Shell.jsx";

// APPLICATION (Settings.jsx en fait l'entrée et la route) : une seule page. Les Manettes ont leur entrée
// (Gamepads.jsx), leur disposition sa page (Layout.jsx) ; les notifications de cet appareil viennent de Push.jsx.
// Ce qui manque encore dans une page le dit dans la page même ; aucun faux contrôle. En direction C
// (Bricks.jsx) : des groupes titrés, leurs lignes, le bouton d'un formulaire dans l'encadré de son groupe (SubmitRow),
// un geste seul en ligne.

// LE PLEIN ÉCRAN : l'API Fullscreen, préfixée webkit en repli.
const fullscreenElement = () => document.fullscreenElement || document.webkitFullscreenElement || null;

const enterFullscreen = async () => {
    const root = document.documentElement;
    const request = root.requestFullscreen || root.webkitRequestFullscreen;

    if (!fullscreenElement() && request) {
        await request.call(root);
    }
};

const toggleFullscreen = async () => {
    try {
        if (fullscreenElement()) {
            await (document.exitFullscreen || document.webkitExitFullscreen)?.call(document);
        } else {
            await enterFullscreen();
        }
    } catch {
        // Un navigateur qui refuse : l'état affiché reste le vrai (fullscreenchange ne vient pas).
    }
};

function useFullscreen() {
    const [on, setOn] = useState(() => Boolean(fullscreenElement()));

    useEffect(() => {
        const update = () => setOn(Boolean(fullscreenElement()));
        document.addEventListener("fullscreenchange", update);
        document.addEventListener("webkitfullscreenchange", update);

        return () => {
            document.removeEventListener("fullscreenchange", update);
            document.removeEventListener("webkitfullscreenchange", update);
        };
    }, []);

    return on;
}

// LES SERVEURS FÉDÉRÉS DONT CE COMPTE VOIT LES JEUX (le porteur, 2026-11-16 : « Je demandais à ce qu'on voit la liste
// des serveurs fédérés en format switch pour ensuite choisir quel serveur on veut voir ou pas les jeux. […] Ça fait que
// chaque utilisateur choisit quel serveur fédéré il a envie de voir les jeux. ») : un interrupteur par serveur fédéré de
// ce GameFlex (`GET /servers`, son nom affiché), allumé par défaut — un serveur ajouté plus tard aussi. Éteint, ses jeux
// quittent les bibliothèques et leurs nombres (data.jsx, `shownServers`) ; le Catalogue et les Réglages des consoles les
// montrent toujours. Chaque interrupteur s'enregistre à son changement, sous le COMPTE (le service des comptes,
// hidden_servers.go) : le choix suit le joueur d'un appareil à l'autre ; refusé, il revient, et l'avis dit pourquoi.
// Sans serveur, la boîte reste, et sa ligne grisée le dit.
function FederatedServers() {
    const { t } = useTranslation();
    const { servers, hiddenServers, setServerShown } = useData();
    const change = async (server, on) => {
        const answer = await setServerShown(server.id, on);

        if (!answer.ok) {
            toast(answer.detail || t("settings.saveFailed"));
        }
    };

    return (
        <Section title={t("settings.federatedServersTitle")} help={t("settings.federatedServersHelp")}>
            {servers.length ? (
                <Rows>
                    {servers.map((server) => (
                        <SwitchRow key={server.id} id={`ap-server-${server.id}`} label={server.hostname || server.link} help={server.hostname ? server.link : null}
                            checked={!hiddenServers.has(String(server.id))} onChange={(on) => change(server, on)} />
                    ))}
                </Rows>
            ) : <EmptyState icon="bi-hdd-network" text={t("settings.federatedServersNone")} />}
        </Section>
    );
}

// L'ORIENTATION, POUR TOUT L'APPAREIL (#406, #410 ; chantier 52 P7) : les lignes de Bricks.jsx (OrientationRows), les
// mêmes que dans le menu d'une partie (#758). Ce sont des lignes du groupe « Apparence » (le porteur, 2026-11-14 :
// « language et orientation doivent être dans la même section "appearance" »).

// /settings/application (09-application des modèles) : en tuiles, ce qui s'allume et s'éteint — les sons, le
// plein écran, les consoles sans jeu, les notifications de cet appareil ; puis « Apparence » — la langue et
// l'orientation, dans le même groupe (le porteur, 2026-11-14) —, les serveurs fédérés dont on voit les jeux, et
// l'autorisation des notifications. Les Manettes n'y sont plus : une entrée des Réglages à elle (le porteur,
// 2026-11-13).
export function ApplicationPage() {
    const { t, i18n } = useTranslation();
    const page = usePageText("application");
    const { capabilities, showEmpty, setShowEmpty } = useData();
    const [sounds, setSounds] = useState(soundsEnabled);
    const [turnFailed, setTurnFailed] = useState(false);
    const fullscreen = useFullscreen();
    const turnsTheApp = can(capabilities, "screenOrientation");
    const notifies = can(capabilities, "accounts") && can(capabilities, "pushNotifications");
    const push = usePush(notifies);
    const state = (on) => t(on ? "settings.switchOn" : "settings.switchOff");
    const tiles = [
        {
            id: "ap-sounds", icon: "bi-music-note-beamed", tone: "sky", label: t("settings.items.sounds.label"), value: t(sounds ? "settings.soundsOn" : "settings.soundsOff"), on: sounds,
            onClick: () => {
                setSoundsEnabled(!sounds);
                setSounds(!sounds);
                playSound("enter");
            },
        },
        // Le plein écran : dans un navigateur ; l'application Android l'occupe déjà (l'hôte le dit faux).
        can(capabilities, "fullscreen") && { id: "ap-fullscreen", icon: "bi-arrows-fullscreen", tone: "slate", label: t("settings.items.fullscreen.label"), value: state(fullscreen), on: fullscreen, onClick: toggleFullscreen },
        // Les consoles sans jeu (Platforms.jsx) ; le choix est gardé dans ce navigateur (data.jsx).
        { id: "ap-show-empty", icon: "bi-grid", tone: "indigo", label: t("settings.showEmpty"), value: state(showEmpty), on: showEmpty, onClick: () => setShowEmpty(!showEmpty) },
        notifies && push.device !== null && {
            id: "ap-push", icon: "bi-bell", tone: "amber", label: t("settings.items.push.label"), value: state(push.device === true), on: push.device === true, disabled: push.blocked,
            onClick: () => push.toggle(push.device !== true),
        },
    ].filter(Boolean);

    return (
        <SettingsPage title={page.title}>
            <QuickTiles items={tiles} />
            <Section title={t("settings.items.appearance.label")}>
                <Rows>
                    <SelectRow id="ap-language" label={t("settings.items.language.label")} help={t("settings.items.language.help")} value={i18n.language} onChange={setLanguage}
                        options={["fr", "en"].map((language) => ({ value: language, label: t(`settings.languages.${language}`) }))} />
                    {offersOrientation(turnsTheApp) && <OrientationRows idPrefix="ap" turnsTheApp={turnsTheApp} onFailed={setTurnFailed} />}
                </Rows>
                {turnFailed && <OrientationFailed turnsTheApp={turnsTheApp} />}
            </Section>
            <FederatedServers />
            {notifies && <PushSection push={push} />}
        </SettingsPage>
    );
}
