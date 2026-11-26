// REMOTE PLAY (administration, « Le serveur ») — la page Remote Play de la webapp (RemotePlay.jsx), portée dans les
// Réglages en direction C (le porteur, 2026-11-13 : « Pour Remote Play tu peux prendre l'exemple dans webapp. C'est
// déjà fait mais ce n'était pas dans le settings auparavant. ») : les mêmes étapes, les mêmes champs, les mêmes appels.
//
//   /settings/remote-play         les consoles enregistrées sur CE serveur (DeviceList), une ligne chacune ;
//   /settings/remote-play/new     en enregistrer une (RegisterPanel) — le seul endroit d'où l'on enregistre une
//                                 console : l'écran Remote Play ne montre que les consoles enregistrées (chantier 53, P12) ;
//   /settings/remote-play/<id>    une console (DeviceDetailPanel) : ses détails, son image de fond, la dissocier ;
//   /settings/remote-play/<id>/background   son image de fond : la remplacer par une recherche, la remettre par défaut.
//
// SEULEMENT LES CONSOLES D'ICI : celles d'un serveur fédéré s'appairent et se règlent chez lui ; le sélecteur de
// source de la webapp (ici / un pair) n'a donc rien à choisir dans un coin d'administration. Jouer une console reste
// l'affaire de l'écran Remote Play (se connecter, l'allumer) ; ni « Jouer » ni « Regarder » ici.
//
// LES RÈGLES DES BOUTONS (le porteur, 2026-11-13) : chaque section a un titre ; Enregistrer est dans l'encadré de son
// (dernier) groupe ; un geste qui n'agit sur aucun groupe est une ligne — en enregistrer une : le groupe « Plus » ; la
// dissocier : la section « Danger ».
//
// LA PAGE D'UNE CONSOLE N'A NI LE FLUX NI LE RÉVEIL (le porteur, 2026-11-14 : « Supprime la section "stream settings"
// et "more" (on n'est pas supposé allumer la console depuis l'admin). Et remplace le texte "Delete this console" par
// "unlink this console" dans la section danger. ») :
//   - le réveil reste là où un joueur lance sa partie : l'écran Remote Play (Platforms.jsx, Allumer) ;
//   - LE FLUX N'EST PAS UNE AFFAIRE D'ADMINISTRATION : chaque utilisateur le choisit dans l'engrenage de la console, sur
//     l'écran Remote Play (Platforms.jsx), il est retenu dans son navigateur et part avec sa connexion (le porteur,
//     2026-11-16 : « C'est l'utilisateur qui choisit »). Le réglage au serveur, sur la ligne de la console, « pour tout
//     le monde », est supprimé avec sa route (`/playstation/device-settings/{id}`) et sa colonne (`config_json`).
//
// L'IMAGE DE FOND (le porteur, 2026-11-14, chantier 53, P12 : « celui-ci affiche un background qu'on choisit dans
// settings / remote play / la console. Ce serait le même mécanisme que pour les autres consoles sauf que là on choisit une
// seule grande image. ») : le premier emplacement du fond de sa console (`rp-ps5`…), rangé et servi comme ceux des autres
// consoles (metadata/console_backgrounds.go), cherché chez les fournisseurs de métadonnées (settings/server/consoles/ImageSearch.jsx) ; la
// « Danger » le remet par défaut. Il vaut pour toutes les consoles de ce type sur ce serveur : le fond est rangé par
// console, pas par appareil, et la page le dit.
//
// L'ÉTAT DES CONSOLES EN DIRECT, comme la webapp : la liste lue une fois (`my-devices`, qui sonde chaque console),
// puis le canal d'état du service (use-device-status.js, pas de sondage) là où l'hôte ouvre des sessions — c'est lui
// qui fait passer une console de « Veille » à « En ligne » quand on l'allume.
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les consoles d'ici (leur page dans l'adresse, `?page=`), celles que la recherche sur le réseau
// a trouvées — et, le temps de leur lecture ou de la recherche, des lignes fantômes.

import React, { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing, useListSearch } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Note } from "../bricks/Note.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";
import { Thumb } from "../bricks/Thumb.jsx";
import { useDeviceStatus } from "../engine/stream/remoteplay/hooks/use-device-status.js";
import { listConsoleBackgrounds, listDevices } from "../libs/api.js";
import { can, useData } from "../libs/data.jsx";
import { fact } from "../libs/format.js";
import { consoleBackgroundPath, forgetConsoleBackgrounds, useImage } from "../libs/images.js";
import { focusElement } from "../libs/input.js";
import { forgetBand } from "../screens/library/ConsoleVisual.jsx";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";

export const ROOT = "/settings/remote-play";

// Seule la PlayStation a une implémentation (le service `/api/v1/playstation`) : ses deux types, ceux que le service
// sait appairer.
export const HOST_TYPES = ["PS4", "PS5"];

// Le site qui donne l'identifiant de compte PSN (Base64) qu'attend l'appairage (RemotePlay.jsx, décidé le 2026-10-10).
export const PSN_ACCOUNT_ID_URL = "https://psn.flipscreen.games/";

export const STATUS_LOOK = { awake: ["bi-power", "green"], standby: ["bi-moon", "amber"], offline: ["bi-power", "slate"] };
export const deviceName = (device) => device.hostName || device.hostId;

// LA LUMIÈRE NE TOMBE PAS : un bouton grisé le temps d'un appel la perd (la recherche, Enregistrer) ; elle
// revient au premier de ces contrôles qui l'accepte, si rien d'autre ne l'a prise entre-temps.
export const restoreLight = (...keys) => requestAnimationFrame(() => {
    if (document.activeElement && document.activeElement !== document.body) {
        return;
    }
    focusElement(keys.map((key) => document.querySelector(`[data-key^="${CSS.escape(key)}"]`)).find((element) => element && !element.disabled));
});

// L'état d'une console : sa pastille et son libellé. Une partie en cours (la ligne le dit, le pool des parties le
// confirme) l'emporte sur « En ligne » : quelqu'un y joue.
export function useStatus(device) {
    const { t } = useTranslation();
    const { sessions } = useData();
    const busy = Boolean(device?.hasActiveSession) && sessions.some((session) => session.id === device.id);

    if (!device) {
        return { busy: false, label: "", icon: "bi-playstation", tone: "slate" };
    }

    if (busy) {
        return { busy, label: t("remoteplay.device.sessionActive"), icon: "bi-people", tone: "amber" };
    }
    const [icon, tone] = STATUS_LOOK[device.status] || ["bi-question-circle", "slate"];

    return { busy, label: t(`remoteplay.status.${device.status}`, { defaultValue: device.status || "" }), icon, tone };
}

// LES CONSOLES D'ICI : `null` le temps de la lecture, `false` si elle a échoué, puis la liste — tenue à jour par le
// canal d'état du service.
export function useDevices() {
    const { t } = useTranslation();
    const { capabilities } = useData();
    const [devices, setDevices] = useState(null);
    const onDevices = useCallback((updated) => setDevices(updated || []), []);

    useEffect(() => {
        let cancelled = false;
        listDevices().then((answer) => {
            if (cancelled) {
                return;
            }

            if (answer.ok) {
                setDevices(answer.data || []);
            } else {
                setDevices((current) => current || false);
                toast(answer.errorMessage || t("remoteplay.loadFailed"));
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useDeviceStatus({ onDevicesUpdate: onDevices, onStatusUpdate: onDevices, enabled: can(capabilities, "spawnsSessions") });

    return devices;
}

// Une console de la liste : son nom, puis son type, son adresse et son état, en « clé : valeur ».
export function DeviceRow({ device, onOpen }) {
    const { t } = useTranslation();
    const status = useStatus(device);

    return (
        <NavRow id={`rp-${device.id}`} icon="bi-playstation" label={deviceName(device)}
            help={[fact(t("remoteplay.device.type"), device.hostType), fact(t("remoteplay.device.status"), status.label)]} onOpen={onOpen} />
    );
}

// Route : /settings/remote-play — les consoles PlayStation.
// Les consoles d'ici, puis « Plus » : en enregistrer une, la même ligne sous la liste et sous la liste vide (l'état
// vide n'a plus de bouton : son remède est une ligne).
export default function RemotePlay() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const devices = useDevices();
    const [read, update] = useQueryState({ page: "1" });
    const found = useListSearch(devices || [], (device) => [deviceName(device), device.hostType, device.ip, device.id].join(" "));

    return (
        <SettingsPage title={t("settings.items.remotePlay.label")}>
            {devices === false ? <Note>{t("remoteplay.loadFailed")}</Note> : (
                <>
                    <Section title={t("rp.devicesTitle")} sense="modify">
                        <Listing items={found.found} loading={devices === null} found={t("consoles.found", { count: found.found.length })} empty={t("remoteplay.noDevicesFound")}
                            search={found.search} onSearch={(q) => { found.onSearch(q); update({ page: null }); }} placeholder={t("rp.searchDevices")} searchKey="rp-search"
                            page={read("page")} onPage={(page) => update({ page })}
                            render={(device) => <DeviceRow key={device.id} device={device} onOpen={() => open(`${ROOT}/${encodeURIComponent(device.id)}`, `rp-${device.id}`, { device })} />} />
                    </Section>
                    <Section title={t("more")} sense="modify">
                        <Rows>
                            <NavRow id="rp-add" icon="bi-plus-lg" label={t("remoteplay.addDevice")} help={t("remoteplay.addDeviceDesc")} onOpen={() => open(`${ROOT}/new`, "rp-add")} />
                        </Rows>
                    </Section>
                </>
            )}
            <GuideSection guides={[["remotePlay"]]} />
        </SettingsPage>
    );
}

// LA CONSOLE DU REGISTRE QUI RANGE LE FOND D'UN APPAREIL : `rp-ps5` pour une PS5.
export const backgroundConsole = (device) => `rp-${String(device?.hostType || "").toLowerCase()}`;

// LE PREMIER EMPLACEMENT DU FOND D'UNE CONSOLE REMOTE PLAY : `{name, server, serverName}` — son nom (« 01.jpg », "" s'il
// est libre) et le serveur fédéré qui le prête quand cette console n'a aucune image ici (`#740`), avec son nom —,
// `undefined` tant qu'on ne sait pas, `null` si la lecture a échoué. `epoch` : le relire après un geste.
export function useRemoteBackground(device, epoch = 0) {
    const consoleId = backgroundConsole(device);
    const [background, setBackground] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        setBackground(undefined);
        listConsoleBackgrounds(consoleId).then((answer) => {
            if (!cancelled) {
                setBackground(answer.ok
                    ? { name: (answer.data?.slots || []).find((slot) => slot.slot === 1)?.name || "", server: answer.data?.server || null, serverName: answer.data?.serverName || "" }
                    : null);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId, epoch]);

    return background;
}

// L'image en grand (« L'image actuelle »), ou sa vignette en tête d'une ligne.
export function RemoteBackgroundImage({ consoleId, name, server = null, width = 1280, thumb = false }) {
    const image = useImage(name ? consoleBackgroundPath(consoleId, name, width, server) : null);

    if (thumb) {
        return <Thumb url={image?.url || null} fit="cover" />;
    }

    return <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-body-secondary">{image ? <img className="object-fit-cover" src={image.url} alt="" /> : <span></span>}</div>;
}

// Le fond changé : ses images, et celle de l'écran Remote Play, se relisent.
export const forgetRemoteBackground = (consoleId) => {
    forgetConsoleBackgrounds(consoleId);
    forgetBand(consoleId);
};
