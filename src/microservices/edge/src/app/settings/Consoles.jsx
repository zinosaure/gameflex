// LES CONSOLES (Réglages › Administration ; là où allait l'engrenage d'une console, le porteur, 2026-11-12) : la
// liste, puis une page par console. Dessinées au chantier 53, P7 (#676), sur les modèles validés par le porteur le
// 2026-11-13 (docs/reports/2026-11-13-launcher-modeles/, captures 12 à 20) :
//
//   /settings/consoles/<console>                ses ÉMULATEURS et ses BIOS, deux listes lues à part, sans onglet ni
//                                               source en tête ; puis « Plus » : ses jeux, ses paramètres, son
//                                               apparence ;
//   …/<console>/games/…                         ses jeux : les sources, la liste d'une source, un jeu
//                                               (Games.jsx, chantier 53, P8) ;
//   …/<console>/cores/<nom>, …/bios/<nom>       un émulateur, un BIOS : ce qu'on en sait ; « Émulateur par défaut »,
//                                               « Utiliser cette version » ou « BIOS par défaut » ; le supprimer
//                                               (« Danger ») ; absent, « Le récupérer » d'une source ;
//   …/<console>/params/<émulateur>              le formulaire de ses paramètres (les consoles Streamex), et ses
//                                               préréglages : ceux d'ici, puis ce que prête chaque autre source ;
//   …/<console>/appearance                      son apparence : ses informations, les huit emplacements de sa
//                                               bande (…/appearance/<n>), la recherche qui en remplit un
//                                               (…/appearance/search?slot=) — chantier 53, P9.
//
// LA PAGE À ONGLETS D'AVANT EST PARTIE (AGENTS.md : aucune rétro compatibilité) : ses quatre onglets (`?tab=`), le
// choix d'une source en tête de chacun, et la liste d'un onglet qu'on cherchait et feuilletait. Une console montre
// désormais tout ce qu'elle a ICI ; ce qui manque se récupère depuis sa propre page, où chaque source dit si elle l'a.
//
// L'OFFRE DE PARAMÈTRES EST PARTIE AUSSI (…/offers/…, chantier 53 après la P7 ; le porteur, 2026-11-14 : « au lieu de
// copier le fichier on va juste copier les données comme preset ») : ce qu'un disque interne ou un serveur fédéré
// prête est un préréglage de plus dans le sélecteur des paramètres, dans le groupe de sa source, et ne fait que
// remplir le formulaire ; l'import qui le copiait en préréglage d'ici est parti du serveur.
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les émulateurs et les BIOS d'une console, les sources de « Le récupérer ». La page d'une liste
// dont les lignes ouvrent une page est dans l'adresse (`?coresPage=`, `?biosPage=`) : au retour, la lumière la
// retrouve.
//
// UNE LIGNE OUVRE, ELLE N'AGIT PAS (le porteur, 2026-11-13) : un émulateur, un BIOS, un emplacement de la bande ont
// chacun leur page, où sont leurs gestes.
//
// LES RÈGLES DES BOUTONS (le porteur, 2026-11-13) : chaque section a un titre ; le bouton d'un formulaire est dans
// l'encadré de son (dernier) groupe ; un geste qui n'agit sur aucun groupe est une ligne — ce qui détruit dans la
// section « Danger », en bas (DangerSection), le reste dans le groupe « Plus » ; toute confirmation est la fenêtre de
// Skeleton (useConfirm).

import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { switchGroup } from "../bricks/FilterGroups.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Note } from "../bricks/Note.jsx";
import { Section } from "../bricks/Section.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { Thumb } from "../bricks/Thumb.jsx";
import { hiddenOf, toggleHidden } from "../globals/filters.js";
import { consoleDefaults, fetchRequired, listConsoleBackgrounds } from "../libs/api.js";
import { useData } from "../libs/data.jsx";
import { counted, fact } from "../libs/format.js";
import { consoleBackgroundPath, forgetConsoleBackgrounds, useImage } from "../libs/images.js";
import { forgetBand } from "../screens/library/ConsoleVisual.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";
import { pill } from "../bricks/Pill.jsx";
import { CONSOLE_ICON } from "../globals/icons.js";

// Les consoles émulées (le Remote Play n'a ni cœur, ni BIOS, ni paramètres), par nom.
export function useEmulated() {
    const { consoles } = useData();

    return useMemo(() => (consoles || []).filter((entry) => entry.method !== "remoteplay").sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" })), [consoles]);
}

export const consolePath = (consoleId) => `/settings/consoles/${encodeURIComponent(consoleId)}`;
export const byName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });

// LES ÉMULATEURS OU LES BIOS D'UNE CONSOLE, ICI (`/retrogaming/storage/cores|bios`, filtré sur elle) : les cœurs de
// son registre, installés ou non ; les fichiers des émulateurs de bureau (`package` : « server » ou « phone ») ; le
// fichier qu'attend son image, s'il manque ; ses BIOS. `null` quand la liste n'a pas pu se lire.
export const loadSection = (section, consoleId) => fetchRequired(section).then((answer) => (answer.ok
    ? (answer.data?.consoles || []).filter((found) => found.consoleId === consoleId).flatMap((found) => found.items || [])
    : null));

// CE QUE LA CONSOLE PREND SANS CHOIX (`inUse` de son défaut, console_defaults.go) : en Streamex, LE fichier en
// service ; en rétro, le cœur par défaut ; le BIOS par défaut ; au téléphone, l'émulateur de bureau par défaut. Un seul
// par console et par liste. Chaque ligne dit ce qu'elle règle (`choose`, écrit par les deux kernels, chantier 54) :
// une ligne qui ne règle rien n'est jamais « en service ».
export const inService = (section, item, inUse) => Boolean(inUse && item.installed && item.choose && item.name === inUse[item.choose]);

// L'ordre d'une liste : celui en service, ceux d'ici, ceux du serveur avant les paquets du téléphone, puis le nom.
export const ordered = (rows, section, inUse) => [...rows].sort((a, b) => Number(inService(section, b, inUse)) - Number(inService(section, a, inUse))
    || Number(b.installed) - Number(a.installed) || Number(a.package === "phone") - Number(b.package === "phone") || byName(a, b));

// UNE LIGNE NE DIT QUE « installé ou non · version · taille en MB » (le porteur, 2026-11-13).
export const rowFacts = (item, t) => [pill(t(item.installed ? "consoles.installed" : "consoles.missing"), item.installed ? "success" : "danger"), item.installed ? fact(t("consoles.version"), item.version) : null];

// « Paramètres d'Eden », « Paramètres de Citron » : l'élision du français, par le contexte d'i18next. Le fichier d'un
// émulateur appelle `system` l'émulateur (« Eden ») et `name` sa console.
export const paramsLabel = (emulator, t) => t("consoles.paramsOf", { name: emulator.system, context: /^[aeiouyàâäéèêëîïôöùûü]/i.test(emulator.system || "") ? "elided" : undefined });

// Les paramètres d'un émulateur : sa page.
export const paramsPath = (consoleId, emulator) => `${consolePath(consoleId)}/params/${encodeURIComponent(emulator)}`;

// UNE LISTE D'UNE CONSOLE (ses émulateurs, ses émulateurs mobiles, ses BIOS) : un listing (le porteur, 2026-11-25 :
// « dans consoles faut enlever les icônes des listings », « la même chose pour bios ») — sa recherche sur le nom et la
// version (`q` : `search`/`onSearch`, dans l'adresse), une page à la fois (`page`) ; chaque ligne ouvre sa page.
// `disabled` : la raison d'une liste que cet hôte ne fait pas tourner — ses lignes désactivées.
export function ItemRows({ section, consoleId, rows, inUse, page, onPage, search, onSearch, empty, disabled = null, searchKey }) {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();

    if (rows === null) {
        return <Note className="mt-0">{t("settings.actionFailed")}</Note>;
    }
    const needle = search.trim().toLowerCase();
    const found = rows ? ordered(rows, section, inUse).filter((item) => !needle || [item.name, item.version].some((text) => String(text || "").toLowerCase().includes(needle))) : [];

    return (
        <Listing items={found} loading={rows === undefined} empty={empty} search={search} onSearch={onSearch} placeholder={t("consoles.searchItem")} searchKey={searchKey}
            page={page} onPage={onPage} render={(item) => {
                const key = `cs-${section}-${item.package || "here"}-${item.name}`;

                return (
                    <NavRow key={key} id={key} label={item.name} help={rowFacts(item, t)} checked={inService(section, item, inUse)} disabled={Boolean(disabled)}
                        onOpen={() => open(`${consolePath(consoleId)}/${section}/${encodeURIComponent(item.name)}`, key)} />
                );
            }} />
    );
}

// /settings/consoles/<console>/cores, …/mobile-cores, …/bios : UN LISTING D'UNE CONSOLE, SUR SA PAGE (le porteur,
// 2026-11-25) — ses émulateurs (ceux de ce serveur, les cœurs du rétro), ses émulateurs mobiles (les paquets du
// téléphone), ses BIOS ; la recherche et la page dans l'adresse. Celle que cet hôte ne fait pas tourner (`emulatorPackage`) a ses
// lignes désactivées, la raison dans l'aide de la section.
export const ITEM_LISTS = {
    cores: { section: "cores", title: "consoles.emulators", empty: "consoles.noCores", keep: (item) => item.package !== "phone" },
    "mobile-cores": { section: "cores", title: "consoles.mobileEmulators", empty: "consoles.noMobileCores", keep: (item) => item.package === "phone" },
    bios: { section: "bios", title: "consoles.bios", empty: "consoles.noBios", keep: () => true },
};

export function ItemsPage({ list }) {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { capabilities } = useData();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const kind = ITEM_LISTS[list];
    const [rows, setRows] = useState(undefined);
    const [inUse, setInUse] = useState(null);
    const [read, update] = useQueryState({ page: "1", q: "" });
    const onPhone = capabilities?.emulatorPackage === "phone";
    const streamex = entry?.method === "streamex";
    const notHere = list === "cores" && onPhone ? t("consoles.emulatorsNotHere") : list === "mobile-cores" && !onPhone ? t("consoles.mobileEmulatorsNotHere") : null;
    const help = notHere || (list === "bios" ? t("consoles.biosHelp") : list === "mobile-cores" ? t("consoles.mobileEmulatorsHelp")
        : t(streamex ? "consoles.emulatorsHelpStreamex" : "consoles.emulatorsHelp"));

    useEffect(() => {
        let cancelled = false;
        setRows(undefined);
        loadSection(kind.section, consoleId).then((read_) => !cancelled && setRows(read_ === null ? null : read_.filter(kind.keep)));
        consoleDefaults(consoleId).then((answer) => !cancelled && setInUse(answer.ok ? answer.data?.inUse || {} : {}));

        return () => {
            cancelled = true;
        };
    }, [consoleId, list]);

    return (
        <SettingsPage title={t(kind.title)} parent={consolePath(consoleId)} levels={{ [consolePath(consoleId)]: entry?.name }} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            <Section title={t(kind.title)} help={help} sense="modify">
                <ItemRows section={kind.section} consoleId={consoleId} rows={rows} inUse={inUse} page={read("page")} onPage={(page) => update({ page })}
                    search={read("q")} onSearch={(q) => update({ q, page: "1" })} searchKey={`cs-${list}-search`} empty={t(kind.empty)} disabled={notHere} />
            </Section>
        </SettingsPage>
    );
}

export const appearancePath = (consoleId) => `${consolePath(consoleId)}/appearance`;
export const slotSearchPath = (consoleId, slot) => `${appearancePath(consoleId)}/search?${new URLSearchParams({ slot })}`;

// Les emplacements d'une console, lus : `{slots, extra, server, serverName}` ; undefined en lecture ; `failure` (sa
// raison, peut-être vide) quand le serveur ne les a pas rendus. `server` : le serveur fédéré qui les prête, quand la
// console n'a aucune image ici (`#740`) — ses images se demandent avec lui.
export function useBackgroundSlots(consoleId, epoch = 0) {
    const [answer, setAnswer] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        setAnswer(undefined);
        listConsoleBackgrounds(consoleId).then((read) => !cancelled && setAnswer(read.ok
            ? { slots: read.data?.slots || [], extra: read.data?.extra || [], server: read.data?.server || null, serverName: read.data?.serverName || "" }
            : { slots: [], extra: [], server: null, failure: read.errorMessage || "" }));

        return () => {
            cancelled = true;
        };
    }, [consoleId, epoch]);

    return answer;
}

// LES NIVEAUX AU-DESSUS D'UNE PAGE DE L'APPARENCE, nommés (SettingsPage, `levels`) : la console par son nom, l'apparence,
// l'emplacement — le fil d'Ariane les dit même par un lien direct.
export function useAppearanceLevels(consoleId, slot = null) {
    const { t } = useTranslation();
    const entry = useEmulated().find((item) => item.id === consoleId);

    return {
        [consolePath(consoleId)]: entry?.name,
        [appearancePath(consoleId)]: t("consoles.appearance"),
        ...(slot ? { [`${appearancePath(consoleId)}/${slot}`]: t("appearance.slot", { slot }) } : {}),
    };
}

// Un emplacement changé : ses images et la liste de la bande se relisent.
export const forgetSlots = (consoleId) => {
    forgetConsoleBackgrounds(consoleId);
    forgetBand(consoleId);
};

// La vignette d'une image du dossier (chargée avec le jeton, libs/images.js), ou du serveur fédéré qui les prête.
export function BackgroundThumb({ consoleId, name, server = null }) {
    const image = useImage(name ? consoleBackgroundPath(consoleId, name, 256, server) : null);

    return <Thumb url={image?.url || null} fit="cover" />;
}

// L'image d'un emplacement pris, en grand : sa page (« Aperçu ») et sa recherche (« L'image actuelle »).
export function SlotPreview({ consoleId, name, server = null }) {
    const image = useImage(consoleBackgroundPath(consoleId, name, 1280, server));

    return <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-body-secondary">{image ? <img className="object-fit-cover" src={image.url} alt="" /> : <span></span>}</div>;
}

// LA LISTE DES CONSOLES DES RÉGLAGES, UNE SEULE : Réglages › Consoles, et les jeux par console d'un disque interne ou
// d'une ressource externe (Réglages › Ressources ; le porteur, 2026-11-23 : « doit être comme pour settings/consoles »).
// Les consoles émulées, cherchées (`?q=`), une page à la fois (`?page=`) ; chaque ligne dit son genre et le nombre de
// jeux de la source (`countOf(id)`, rien s'il est inconnu), en « clé : valeur » ; une console activée (chantier 66) a
// la validation verte à côté de son titre (`checked` ; le porteur, 2026-11-25, au lieu du titre en couleur du 2026-11-23) ; la ligne ouvre
// ce que dit `onOpen(console, rowId)`.
//
// SES FILTRES (le porteur, 2026-11-23), dans le panneau de la liste comme ceux des jeux d'une source : les consoles sans
// jeu (`?empty=1`, éteint par défaut : la liste montre d'abord les consoles qui en ont), les consoles activées et les
// désactivées (`?nostate=`, toutes deux montrées par défaut).
export function ConsoleList({ countOf, onOpen }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const emulated = useEmulated();
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const needle = search.trim().toLowerCase();
    const showEmpty = read("empty") === "1";
    const hiddenStates = hiddenOf(read("nostate"));
    const stateOf = (entry) => (entry.disabled ? "disabled" : "enabled");
    const shown = emulated.filter((entry) => (!needle || `${entry.name} ${entry.id}`.toLowerCase().includes(needle))
        && (showEmpty || countOf(entry.id) === null || countOf(entry.id) > 0) && !hiddenStates.includes(stateOf(entry)));

    return (
        <Section title={t("consoles.listTitle")} sense="modify">
            <Listing items={shown} loading={!consoles} search={search} onSearch={(q) => update({ q })} page={read("page")} onPage={(page) => update({ page })}
                placeholder={t("consoles.search")} found={t("consoles.found", { count: shown.length })} empty={t("consoles.none")}
                filterCount={(showEmpty ? 1 : 0) + hiddenStates.length} filters={[
                    {
                        key: "games", title: t("consoles.games"), rows: (
                            <SwitchRow id="cs-filter-empty" label={t("settings.showEmpty")} checked={showEmpty} onChange={(on) => update({ empty: on ? "1" : null, page: null })} />
                        ),
                    },
                    switchGroup({ key: "states", title: t("games.status"), id: "cs-filter", hidden: hiddenStates, onChange: (state, on) => update({ nostate: toggleHidden(hiddenStates, state, on), page: null }),
                        choices: [{ value: "enabled", label: t("consoles.filterEnabled") }, { value: "disabled", label: t("consoles.filterDisabled") }] }),
                ]}
                render={(entry) => (
                    <NavRow key={entry.id} id={`cs-open-${entry.id}`} icon={entry.method === "streamex" ? "bi-display" : CONSOLE_ICON} label={entry.name}
                        onOpen={() => onOpen(entry, `cs-open-${entry.id}`)}
                        help={[fact(t("facts.type"), entry.method === "streamex" ? t("tab.streamex") : t("tab.retrogaming")), fact(t("facts.games"), counted(t, "games", countOf(entry.id)))]} />
                )} />
        </Section>
    );
}

// Route : /settings/consoles — les consoles.
// /settings/consoles : la liste, chaque console vers sa page, avec le nombre de jeux de sa bibliothèque.
export default function Consoles() {
    const { t } = useTranslation();
    const { countOf } = useData();
    const { open } = useSettingsNavigation();

    return (
        <SettingsPage title={t("settings.items.consoles.label")}>
            <ConsoleList countOf={countOf}
                onOpen={(entry, rowId) => open(`/settings/consoles/${encodeURIComponent(entry.id)}`, rowId)} />
            <GuideSection guides={[["consoles"], ["library"]]} />
        </SettingsPage>
    );
}
