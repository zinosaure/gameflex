// LES RESSOURCES : LES DISQUES INTERNES SEULEMENT (le porteur, 2026-11-14 : « on gère que les ressources internes,
// c'est-à-dire les disques ») — InternalDisks.jsx de la webapp. Une page, /settings/resources (`?disksPage=`), la liste
// feuilletée (Listing ; « Rajouter des pagination pour les listes asynchrone ») : les disques déclarés au compose
// (`/mnt/<nom>`) ; un disque s'ouvre sur sa page (/settings/resources/disks/<nom>) — son nombre de jeux en sous-titre,
// ses jeux par console, le relire.
//
// LES RESSOURCES EXTERNES (Samba, WebDAV, kDrive, FTP, SFTP) REVIENNENT ICI (le porteur, 2026-11-20 : gameflex-federation
// s'intègre à GameFlex, « Un serveur Gameflex peut faire office de serveur fédéré seul », `#808`) : leur liste, « Ajouter
// une ressource externe » dans « Plus », la page de leurs genres, puis le formulaire d'un genre — les pages de
// gameflex-federation, branchées sur le kernel (resources_api.go) : leurs sections s'ajoutent à ce que ce serveur prête à
// ses amis, derrière la bibliothèque et les disques. Une ressource s'ouvre sur sa page (/settings/resources/external/<id>) :
// son état, la relire, la modifier, la retirer.
//
// Les pages se composent des briques de la direction C (bricks/) ; relire un disque est une ligne de « Plus ».

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";
import { listDisks, readToken } from "../libs/api.js";
import { useData } from "../libs/data.jsx";
import { counted, fact } from "../libs/format.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../libs/kernelChannel.js";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";
import { KINDS } from "./people/externalResources.jsx";
import { hostGuide } from "../libs/capabilities.js";

// LES DISQUES INTERNES : une ligne par disque, qui ouvre sa page ; une page à la fois (`?disksPage=`).
export function Disks() {
    const { t } = useTranslation();
    const { capabilities } = useData();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1", q: "" });
    const disks = useDisks();
    const needle = (read("q") || "").trim().toLowerCase();
    const device = capabilities.internalDisks === "device";
    const text = (key, values) => t(`resources.disks.${key}${device ? "Device" : ""}`, values);

    return (
        <Section title={t("resources.internalTitle")} help={text("intro")} sense="modify">
            <Listing items={(disks || []).filter((disk) => !needle || disk.name.toLowerCase().includes(needle))} loading={disks === null} empty={text("empty")}
                search={read("q")} onSearch={(q) => update({ q, page: "1" })} placeholder={t("resources.searchDisk")} searchKey="dk-search"
                page={read("page")} onPage={(page) => update({ page })}
                render={(disk) => (
                    <NavRow key={disk.name} id={`dk-${disk.name}`} icon={disk.present ? "bi-device-hdd" : "bi-exclamation-triangle"} label={disk.name}
                        help={disk.present ? counted(t, "games", diskGames(disk)) : text("absent")}
                        onOpen={() => open(`/settings/resources/disks/${encodeURIComponent(disk.name)}`, `dk-${disk.name}`, { disk })} />
                )} />
        </Section>
    );
}

export const diskGames = (disk) => Object.values(disk.consoles || {}).reduce((sum, count) => sum + count, 0);

// LE SUJET `resources` DU CENTRE DE MESSAGES : la liste des ressources externes et l'état de leur parcours, repoussée à
// chaque changement (jamais relue à intervalle) ; undefined le temps qu'elle arrive.
export function useExternalResources() {
    const token = readToken();
    const channel = useKernelChannel(kernelReachable(token) ? kernelTarget(null, token) : null);
    const [list, setList] = useState(undefined);

    useKernelTopic(channel, "resources", null, (data) => setList(Array.isArray(data) ? data : []));

    return list;
}

export const KIND_OF = Object.fromEntries(KINDS.map((entry) => [entry.kind, entry]));
export const when = (value, language) => (value && !value.startsWith("0001") ? new Date(value).toLocaleString(language) : "");

// L'état d'une ressource en une phrase : en cours de lecture, injoignable, pas encore lue, ou ses jeux.
export const stateText = (t, language, state) => {
    if (state?.scanning) {
        return t("fedService.connections.scanning");
    }
    if (state?.error) {
        return t("fedService.connections.failed", { error: state.error });
    }
    if (!state?.scannedAt || state.scannedAt.startsWith("0001")) {
        return t("fedService.connections.never");
    }
    if (state.empty) {
        return noLibraryText(t, state);
    }

    return [fact(t("facts.games"), counted(t, "games", state.games)), fact(t("fedService.connections.lastScan"), when(state.scannedAt, language))];
};

// LUE, MAIS VIDE (le porteur, 2026-11-23 : un kDrive rangé sous /gameflex/roms, « Dossier » laissé vide, « je ne trouve
// aucun jeu ») : aucun dossier de bibliothèque là où elle commence ; le serveur nomme ceux, un niveau plus bas, qui en
// ont un — ce que « Dossier » devrait dire.
export const noLibraryText = (t, state) => (state.suggest?.length
    ? t("resources.external.noLibrarySuggest", { folders: state.suggest.join(", ") })
    : t("resources.external.noLibrary"));

// LE STATUT D'UNE RESSOURCE EN BADGE, ET SA RAISON (le porteur, 2026-11-23 : « une ligne "Status" pour voir le statut de
// la Resources comme badge : online, offline, unreachable, etc ... Et en deuxième ligne : la raison ») — d'après son
// dernier parcours et le genre de son erreur, que le serveur range (`state.problem`). Une raison seulement quand il y a
// quelque chose à dire (une erreur, une bibliothèque introuvable) : le nombre de jeux et la date ont leurs lignes.
export const STATUS_BADGE = { online: "success", scanning: "info", never: "secondary", empty: "warning", folder: "warning", offline: "danger", refused: "danger", error: "danger" };
export const statusOfResource = (t, state) => {
    if (state.scanning) {
        return { key: "scanning", reason: null };
    }
    if (!state.scannedAt || state.scannedAt.startsWith("0001")) {
        return { key: "never", reason: null };
    }
    if (state.error) {
        return { key: STATUS_BADGE[state.problem] ? state.problem : "error", reason: state.error };
    }
    if (state.empty) {
        return { key: "empty", reason: noLibraryText(t, state) };
    }

    return { key: "online", reason: null };
};

// Les consoles où une ressource a des jeux, et combien, par le nom de la console.
export const consolesOfResource = (resource, consoles) => Object.entries(resource.consoles || {}).filter(([, count]) => count > 0)
    .map(([id, count]) => ({ id, count, name: (consoles || []).find((entry) => entry.id === id)?.name || id }))
    .sort((left, right) => left.name.localeCompare(right.name));
// La page des jeux d'une console qui vient de cette ressource (Réglages › Consoles › la console › Jeux).
export const resourceGamesPath = (consoleId, resource) => `/settings/consoles/${encodeURIComponent(consoleId)}/games/resource-${resource.id}`;

// LES RESSOURCES EXTERNES : une ligne par connexion, dix par page (`?externalPage=`) — son genre et son état.
export function ExternalResources() {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1", q: "" });
    const list = useExternalResources();
    const needle = (read("q") || "").trim().toLowerCase();

    return (
        <Section title={t("resources.external.title")} help={t("resources.external.intro")} sense="modify">
            <Listing items={(list || []).filter((resource) => !needle || [resource.name, t(`fedService.kinds.${resource.kind}.label`)].some((text) => String(text || "").toLowerCase().includes(needle)))}
                loading={list === undefined} empty={t("resources.external.empty")} search={read("q")} onSearch={(q) => update({ q, page: "1" })}
                placeholder={t("resources.searchExternal")} searchKey="rx-search" page={read("page")} onPage={(page) => update({ page })}
                render={(resource) => (
                    <NavRow key={resource.id} id={`rx-${resource.id}`} icon={KIND_OF[resource.kind]?.icon || "bi-hdd-network"}
                        label={resource.name} help={[t(`fedService.kinds.${resource.kind}.label`), [].concat(stateText(t, i18n.language, resource.state))[0]]}
                        pending={Boolean(resource.state?.scanning)} onOpen={() => open(`/settings/resources/external/${resource.id}`, `rx-${resource.id}`)} />
                )} />
        </Section>
    );
}

// LE GUIDE DES RESSOURCES EXTERNES, au bas des pages d'ajout et de modification (le porteur, 2026-11-23 : « Dans la
// page Edit the connection il faut rajouter les guides ») : *Games on a network share* du guide du stockage — ajouter,
// lire, télécharger ses jeux. Le « Bon à savoir » qui l'accompagnait annonçait pour plus tard ce qui est fait (le
// téléchargement) : retiré le même jour (« Supprime-le pour le moment »).
export const EXTERNAL_GUIDES = [["storage", "games-on-a-network-share"]];

// Route : /settings/resources — les disques et les ressources externes.
// La liste des disques, puis le guide : comment un disque s'ajoute — au compose d'un serveur, ou sur le téléphone.
// LES DISQUES DE CE SERVEUR, lus une fois par page (leur listing, leur nombre sur l'index) ; `null` le temps de la lecture.
export function useDisks() {
    const { t } = useTranslation();
    const [disks, setDisks] = useState(null);

    useEffect(() => {
        listDisks().then((answer) => {
            if (!answer.ok) {
                toast(answer.errorMessage || t("resources.disks.loadFailed"));
            }
            setDisks(answer.ok ? answer.data || [] : []);
        });
    }, [t]);

    return disks;
}

// UN SEUL LISTING PAR ROUTE (le porteur, 2026-11-25 : « mettre en liens pour ensuite aller sur la route des listings. Ça
// évitera d'avoir 2 listings sur une seule page ») : les disques (resources/Disks.jsx) et les ressources externes
// (resources/External.jsx), une ligne chacun, leur nombre en pastille.
export default function Resources() {
    const { t } = useTranslation();
    const { capabilities } = useData();
    const { open } = useSettingsNavigation();
    const disks = useDisks();
    const external = useExternalResources();
    const device = capabilities.internalDisks === "device";

    return (
        <SettingsPage title={t("settings.items.resources.label")} guides={[hostGuide("disks", ["storage", "more-disks-of-games"])]}>
            <Section title={t("resources.storageTitle")} help={t("resources.storageHelp")} sense="modify">
                <Rows>
                    <NavRow id="rs-disks" icon="bi-device-hdd" label={t("resources.internalTitle")} value={disks ? counted(t, "disks", disks.length) : null}
                        help={t(`resources.disks.intro${device ? "Device" : ""}`)} onOpen={() => open("/settings/resources/disks", "rs-disks")} />
                    <NavRow id="rs-external" icon="bi-hdd-network" label={t("resources.external.title")} value={external ? counted(t, "resources", external.length) : null}
                        help={t("resources.external.intro")} onOpen={() => open("/settings/resources/external", "rs-external")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
