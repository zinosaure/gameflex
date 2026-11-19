import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useParams, useSearchParams } from "react-router-dom";
import {
    consoleDefaults, deleteEmulatorPreset, deleteInstalled, deleteMetadataOrphan, fetchAvailability, fetchConsoleStatus, fetchRequired, listConsoleBackgrounds, listEmulatorParams,
    listMetadataOrphans, metadataOrphanCoverPath, readToken, resetConsoleBackgrounds, searchConsoleBackgrounds, setConsoleBackground, setConsoleDefaults, setConsoleStatus, setConsoleSwitch,
    writeEmulatorParams,
} from "../../../libs/api.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../../libs/kernelChannel.js";
import { useData } from "../../../libs/data.jsx";
import { downloadOutcome, runDownload, useDownloads } from "../../../libs/downloads.js";
import { formatMegabytes, GAME_STATUSES, STATUS_VARIANT } from "../../../libs/format.js";
import { consoleBackgroundPath, forgetConsoleBackgrounds, useImage } from "../../../libs/images.js";
import { forgetBand } from "../../../screens/library/ConsoleVisual.jsx";
import { toast } from "../../../shell/Shell.jsx";
import { fieldContext, Form, FormButton, Ruleset } from "../../../ui/Forms.jsx";
import { GamesRoutes } from "./Games.jsx";
import { ImageSearch } from "./ImageSearch.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import NavRow, {
    DangerRow, DangerSection, DownloadRow, EmptyState, GuideSection, InfoRow, Listing, Note, PlaceholderRow, Rows, SubmitRow, useConfirm, Section, SelectRow, SwitchRow, TextRow, Thumb,
} from "../../../ui/Bricks.jsx";

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

// Les consoles émulées (le Remote Play n'a ni cœur, ni BIOS, ni paramètres), par nom.
function useEmulated() {
    const { consoles } = useData();

    return useMemo(() => (consoles || []).filter((entry) => entry.method !== "remoteplay").sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" })), [consoles]);
}

const consolePath = (consoleId) => `/settings/consoles/${encodeURIComponent(consoleId)}`;
const byName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });

// LES ÉMULATEURS OU LES BIOS D'UNE CONSOLE, ICI (`/retrogaming/storage/cores|bios`, filtré sur elle) : les cœurs de
// son registre, installés ou non ; les fichiers des émulateurs de bureau (`package` : « server » ou « phone ») ; le
// fichier qu'attend son image, s'il manque ; ses BIOS. `null` quand la liste n'a pas pu se lire.
const loadSection = (section, consoleId) => fetchRequired(section).then((answer) => (answer.ok
    ? (answer.data?.consoles || []).filter((found) => found.consoleId === consoleId).flatMap((found) => found.items || [])
    : null));

// CE QUE LA CONSOLE PREND SANS CHOIX (`inUse` de son défaut, console_defaults.go) : en Streamex, LE fichier en
// service ; en rétro, le cœur par défaut ; le BIOS par défaut ; au téléphone, l'émulateur de bureau par défaut. Un seul
// par console et par liste. Chaque ligne dit ce qu'elle règle (`choose`, écrit par les deux kernels, chantier 54) :
// une ligne qui ne règle rien n'est jamais « en service ».
const inService = (section, item, inUse) => Boolean(inUse && item.installed && item.choose && item.name === inUse[item.choose]);

// L'ordre d'une liste : celui en service, ceux d'ici, ceux du serveur avant les paquets du téléphone, puis le nom.
const ordered = (rows, section, inUse) => [...rows].sort((a, b) => Number(inService(section, b, inUse)) - Number(inService(section, a, inUse))
    || Number(b.installed) - Number(a.installed) || Number(a.package === "phone") - Number(b.package === "phone") || byName(a, b));

// LA PASTILLE D'UNE LIGNE : le paquet d'un émulateur (`bi-phone` pour le téléphone, `bi-pc-display` pour le serveur ;
// le porteur, 2026-11-13), l'empreinte d'un BIOS, en couleur s'il est là.
const lookOf = (section, item) => (section === "bios" ? ["bi-fingerprint", item.installed ? "amber" : "slate"]
    : item.package === "phone" ? ["bi-phone", "violet"] : ["bi-pc-display", "indigo"]);

// UNE LIGNE NE DIT QUE « installé ou non · version · taille en MB » (le porteur, 2026-11-13).
const rowFacts = (item, t) => [t(item.installed ? "consoles.installed" : "consoles.missing"), item.installed ? item.version : null,
    item.installed ? formatMegabytes(item.sizeBytes) : null].filter(Boolean).join(" · ");

// « Paramètres d'Eden », « Paramètres de Citron » : l'élision du français, par le contexte d'i18next. Le fichier d'un
// émulateur appelle `system` l'émulateur (« Eden ») et `name` sa console.
const paramsLabel = (emulator, t) => t("consoles.paramsOf", { name: emulator.system, context: /^[aeiouyàâäéèêëîïôöùûü]/i.test(emulator.system || "") ? "elided" : undefined });

// Les paramètres d'un émulateur : sa page.
const paramsPath = (consoleId, emulator) => `${consolePath(consoleId)}/params/${encodeURIComponent(emulator)}`;

// UNE LISTE D'UNE CONSOLE (ses émulateurs, ses BIOS), une page à la fois (`page`, dans l'adresse) : chaque ligne
// ouvre sa page.
function ItemRows({ section, consoleId, rows, inUse, page, onPage }) {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();

    if (rows === null) {
        return <Note className="mt-0">{t("settings.actionFailed")}</Note>;
    }

    return (
        <Listing items={rows ? ordered(rows, section, inUse) : []} loading={rows === undefined} empty={t(section === "cores" ? "consoles.noCores" : "consoles.noBios")}
            page={page} onPage={onPage} render={(item) => {
                const [icon, tone] = lookOf(section, item);
                const key = `cs-${section}-${item.name}`;

                return (
                    <NavRow key={item.name} id={key} icon={icon} tone={tone} label={item.name} help={rowFacts(item, t)} emphasis={inService(section, item, inUse)}
                        onOpen={() => open(`${consolePath(consoleId)}/${section}/${encodeURIComponent(item.name)}`, key)} />
                );
            }} />
    );
}

// /settings/consoles/<console> : ses émulateurs et ses BIOS, chacun sa lecture ; celui en service en couleur ; puis
// « Plus » — ses jeux (Games.jsx), ses paramètres (un par émulateur, grisés hors Streamex : seuls les
// émulateurs de bureau en ont), son apparence.
function ConsolePage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { consoles, countOf, markConsoleDisabled } = useData();
    const { open } = useSettingsNavigation();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const registered = (consoles || []).find((item) => item.id === consoleId);
    const [switching, setSwitching] = useState(false);
    // COUPER LA CONSOLE POUR TOUT LE SERVEUR (chantier 66) : personne ne la lance plus, ni d'ici ni par la fédération.
    const switchConsole = async (enabled) => {
        setSwitching(true);
        const answer = await setConsoleSwitch(consoleId, enabled);
        setSwitching(false);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        markConsoleDisabled(consoleId, !enabled);
        toast(t(enabled ? "consoles.switchedOn" : "consoles.switchedOff", { name: entry?.name || consoleId }));
    };
    const [cores, setCores] = useState(undefined);
    const [bios, setBios] = useState(undefined);
    const [inUse, setInUse] = useState(null);
    const [params, setParams] = useState([]);
    const [read, update] = useQueryState({ coresPage: "1", biosPage: "1" });
    const streamex = entry?.method === "streamex";
    const count = countOf(consoleId);

    useEffect(() => {
        let cancelled = false;
        const keep = (setter) => (value) => !cancelled && setter(value);

        setCores(undefined);
        setBios(undefined);
        loadSection("cores", consoleId).then(keep(setCores));
        loadSection("bios", consoleId).then(keep(setBios));
        consoleDefaults(consoleId).then((answer) => keep(setInUse)(answer.ok ? answer.data?.inUse || {} : {}));
        listEmulatorParams().then((answer) => keep(setParams)(answer.ok ? (answer.data?.emulators || []).filter((emulator) => emulator.consoleId === consoleId).sort((a, b) => String(a.system).localeCompare(String(b.system))) : []));

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    const biosDefault = !streamex && (bios || []).some((item) => inService("bios", item, inUse));

    return (
        <SettingsPage title={entry?.name || consoleId} parent="/settings/consoles">
            {consoles && !entry ? <NotFound /> : !entry ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : (
                <>
                    {/* « UN INTERRUPTEUR POUR COUPER LA CONSOLE DIRECTEMENT » (le porteur, 2026-11-19, chantier 66) : pour tout le
                        serveur — ses joueurs, administrateur compris, et ceux de la fédération. Son bouton Jouer se grise
                        partout (« Console désactivée »). */}
                    <Section title={t("consoles.switchTitle")}>
                        <Rows>
                            <SwitchRow id={`cs-switch-${consoleId}`} label={t("consoles.switchLabel")} help={t("consoles.switchHelp")}
                                checked={!registered?.disabled} disabled={switching || !registered} onChange={switchConsole} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.emulators")} help={t(streamex ? "consoles.emulatorsHelpStreamex" : "consoles.emulatorsHelp")}>
                        <ItemRows section="cores" consoleId={consoleId} rows={cores} inUse={inUse} page={read("coresPage")} onPage={(page) => update({ coresPage: page })} />
                    </Section>
                    <Section title={t("consoles.bios")} help={biosDefault ? t("consoles.biosHelp") : null}>
                        <ItemRows section="bios" consoleId={consoleId} rows={bios} inUse={inUse} page={read("biosPage")} onPage={(page) => update({ biosPage: page })} />
                    </Section>
                    <Section title={t("more")}>
                        <Rows>
                            <NavRow id="cs-games" icon="bi-collection-play" tone="indigo" label={t("consoles.games")} value={count}
                                onOpen={() => open(`${consolePath(consoleId)}/games`, "cs-games")} />
                            <NavRow id="cs-orphans" icon="bi-file-earmark-x" tone="slate" label={t("consoles.orphans")} help={t("consoles.orphansRowHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/metadata-orphans`, "cs-orphans")} />
                            {streamex ? params.map((emulator) => (
                                <NavRow key={emulator.emulator} id={`cs-params-${emulator.emulator}`} icon="bi-sliders" tone="sky" label={paramsLabel(emulator, t)}
                                    onOpen={() => open(paramsPath(consoleId, emulator.emulator), `cs-params-${emulator.emulator}`)} />
                            )) : <NavRow id="cs-params" icon="bi-sliders" tone="slate" label={t("consoles.tabParams")} help={t("consoles.paramsStreamexOnly")} opens="info" disabled />}
                            <NavRow id="cs-appearance" icon="bi-palette" tone="violet" label={t("consoles.appearance")} help={t("consoles.appearanceHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/appearance`, "cs-appearance")} />
                            <NavRow id="cs-status" icon="bi-patch-check" tone="green" label={t("consoles.status")} help={t("consoles.statusRowHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/status`, "cs-status")} />
                        </Rows>
                    </Section>
                    <GuideSection guides={[["consoles"], ["library", "3-get-the-emulators"]]} />
                </>
            )}
        </SettingsPage>
    );
}

// /settings/consoles/<console>/status : LE STATUT DE TOUTE LA CONSOLE (chantier 30 ; chantier 56, A2 — le porteur,
// 2026-11-14 : « Oui ») — ConsoleStatusPanel de la webapp. Chaque jeu sans statut à lui prend celui-ci, y compris un jeu
// ajouté plus tard ; un jeu signalé garde le sien. D'abord ce qui vaut aujourd'hui (posé par qui et quand, ou le défaut
// de la console), puis le sélecteur et son bouton dans l'encadré, « ••• » tant que rien ne change. La liste gardée de la
// console s'oublie (forgetHere) : ses jeux se relisent à la prochaine bibliothèque.
function ConsoleStatusPage() {
    const { t, i18n } = useTranslation();
    const { consoleId } = useParams();
    const { consoles, forgetHere } = useData();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [answer, setAnswer] = useState(undefined);
    const [draft, setDraft] = useState(null);
    const [pending, setPending] = useState(false);

    useEffect(() => {
        let cancelled = false;
        setAnswer(undefined);
        setDraft(null);
        fetchConsoleStatus(consoleId).then((read) => !cancelled && setAnswer(read));

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    const current = answer?.ok ? answer.data || {} : null;
    const effective = current ? current.status || current.default || "untested" : "untested";
    const chosen = draft ?? effective;
    const dated = (at) => (at ? new Date(at).toLocaleString(i18n.language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");
    const badge = (status) => <span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>;

    const save = async () => {
        setPending(true);
        const written = await setConsoleStatus(consoleId, chosen);
        setPending(false);

        if (!written.ok) {
            toast(written.errorMessage || t("settings.actionFailed"));

            return;
        }
        setAnswer(written);
        setDraft(null);
        forgetHere(consoleId);
        toast(t("consoles.statusSaved", { status: t(`status.${chosen}`) }));
    };

    return (
        <SettingsPage title={t("consoles.status")} parent={consolePath(consoleId)} levels={{ [consolePath(consoleId)]: entry?.name }}>
            {consoles && !entry ? <NotFound /> : !current ? (
                <Section><Note className="mt-0">{answer === undefined ? t("settings.loading") : answer.errorMessage || t("settings.actionFailed")}</Note></Section>
            ) : (
                <>
                    <Section title={t("consoles.statusNow")}>
                        <Rows>
                            <InfoRow icon="bi-patch-check" tone="green" label={t("games.status")} value={badge(effective)}
                                help={current.status ? t("consoles.statusSetBy", { by: current.by, at: dated(current.at) }) : t("consoles.statusDefault")} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.statusChange")} help={t("consoles.statusHelp")}>
                        <Form props={{ onSubmit: (event) => {
                            event.preventDefault();

                            if (chosen !== effective && !pending) {
                                save();
                            }
                        } }}>
                            <Rows>
                                <SelectRow id="cst-status" label={t("games.status")} value={chosen} disabled={pending} onChange={(value) => setDraft(value)}
                                    options={GAME_STATUSES.map((status) => ({ value: status, label: t(`status.${status}`) }))} />
                                <SubmitRow>
                                    <FormButton id="cst-save" label={t("settings.save")} isReplaced={chosen !== effective} pending={pending} />
                                </SubmitRow>
                            </Rows>
                        </Form>
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}

// Une source de « Le récupérer », sa clé (celle du registre des téléchargements) et son nom.
const sourceKey = (source) => (source.source === "official" ? "official" : `server:${source.serverId}`);
const sourceLabel = (source, item, t) => (source.source === "official" ? t(item.package ? "consoles.sourceStreamex" : "consoles.sourceOfficial") : source.name || t("consoles.serverNamed", { id: source.serverId }));
// La raison d'une source qui ne l'a pas : injoignable, notre jeton refusé, ou elle ne l'a pas.
const reasonOf = (reason, t) => t(reason === "unreachable" ? "consoles.sourceUnreachable" : reason === "denied" ? "consoles.sourceDenied" : "consoles.sourceLacks");
const hostOf = (address) => {
    try {
        return new URL(address).host;
    } catch {
        return null;
    }
};

// « LE RÉCUPÉRER » (le porteur, 2026-11-13) : la source officielle (un cœur rétro chez libretro, un émulateur de
// bureau chez son éditeur), puis chaque serveur fédéré, demandés à la source au moment où la page s'ouvre
// (storage_availability.go) ; grisés quand ils ne l'ont pas, avec leur raison. Toucher une source ouvre la fenêtre de
// confirmation, puis le téléchargement part en tâche de fond (libs/downloads.js) : la ligne de la source devient celle
// du téléchargement (DownloadRow : sa progression, son étape, « Annuler ») ; les autres sources grisées le temps qu'il
// dure. La taille qu'un
// serveur fédéré annonce se lit dans la copie de son listing. Les sources se feuillettent (Listing), la page tenue par
// la liste : une ligne n'ouvre pas de page ; demandées, elles sont des lignes fantômes.
function FetchSources({ section, consoleId, item }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const key = `${section}/${consoleId}/${item.name}`;
    const running = useDownloads().find((entry) => entry.key === key) || null;
    const [sources, setSources] = useState(undefined);
    const [failure, setFailure] = useState("");
    const [offered, setOffered] = useState({});

    useEffect(() => {
        let cancelled = false;

        fetchAvailability(consoleId, section, item.name).then((answer) => {
            if (cancelled) {
                return;
            }
            const found = answer.ok ? answer.data?.sources || [] : null;
            setSources(found);
            // La raison, dite (« Pas encore sur cet appareil » au téléphone, libs/api.js), plutôt qu'un échec brut.
            setFailure(answer.ok ? "" : answer.errorMessage);

            for (const source of (found || []).filter((candidate) => candidate.source === "server" && candidate.available)) {
                fetchRequired(section, `peer:${source.serverId}`).then((listed) => {
                    const theirs = (listed.ok ? listed.data?.consoles || [] : []).filter((listedConsole) => listedConsole.consoleId === consoleId).flatMap((listedConsole) => listedConsole.items || [])
                        .find((candidate) => candidate.name === item.name && candidate.installed);

                    if (!cancelled && theirs) {
                        setOffered((current) => ({ ...current, [source.serverId]: theirs }));
                    }
                });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [section, consoleId, item.name]);

    const start = async (source) => {
        const total = Number(offered[source.serverId]?.sizeBytes) || 0;
        const command = source.source === "official"
            ? { route: "cores", body: { consoleId, core: item.name } }
            : { route: "server", server: String(source.serverId), section, body: { consoleId, name: item.name }, ...(total ? { total } : {}) };
        const answer = await runDownload({ key, label: item.name, source: sourceKey(source), command });
        toast(downloadOutcome(answer, item.name, t));
    };

    const ask = (source) => {
        const size = formatMegabytes(offered[source.serverId]?.sizeBytes);
        const from = sourceLabel(source, item, t);

        confirm({
            title: t("consoles.fetchFrom", { source: from }),
            message: t(size ? "consoles.fetchConfirmSized" : "consoles.fetchConfirm", { name: item.name, source: from, size }),
            confirmLabel: t("consoles.download"),
            onConfirm: () => start(source),
        });
    };

    const row = (source) => {
        const own = running?.source === sourceKey(source);
        const theirs = offered[source.serverId];
        const official = source.source === "official";
        const facts = !source.available ? [official ? null : t("consoles.federatedServer"), reasonOf(source.reason, t)]
            : official ? [hostOf(item.source)]
                : [t("consoles.federatedServer"), theirs?.version, formatMegabytes(theirs?.sizeBytes)];

        return (
            own ? <DownloadRow key={sourceKey(source)} entry={running} id={`cs-source-${sourceKey(source)}`} label={sourceLabel(source, item, t)} />
                : <NavRow key={sourceKey(source)} id={`cs-source-${sourceKey(source)}`} icon={official ? "bi-globe2" : "bi-arrow-left-right"}
                    tone={!source.available || running ? "slate" : official ? "indigo" : "green"} label={sourceLabel(source, item, t)}
                    help={facts.filter(Boolean).join(" · ")}
                    disabled={!source.available || Boolean(running)} opens="confirm" onOpen={() => ask(source)} />
        );
    };

    return (
        <Section title={t("consoles.fetchTitle")} help={t(section === "bios" ? "consoles.fetchHelpBios" : "consoles.fetchHelp")}>
            {sources === null ? <Note className="mt-0">{failure || t("settings.actionFailed")}</Note>
                : <Listing items={sources || []} loading={sources === undefined} empty={t("consoles.noSource")} render={row} />}
        </Section>
    );
}

// /settings/consoles/<console>/cores|bios/<nom> : un émulateur ou un BIOS.
//   - Détails : son statut, son paquet (un émulateur), sa version, sa taille ;
//   - Utilisation, installé : « Utiliser cette version » (un émulateur de bureau : la version en service, qui fait de
//     son émulateur celui par défaut), « Émulateur par défaut » (une console rétro qui en a plusieurs), « BIOS par
//     défaut » (une console rétro qui en a plusieurs) — confirmé, enregistré par la console (console_defaults.go). La
//     fiche d'un jeu choisit toujours pour un lancement (le porteur, 2026-11-13, réponse 4 du chantier 53) ;
//   - Danger, installé : le supprimer de cette machine, puis retour à la console ;
//   - absent : « Le récupérer » (FetchSources).
// Un téléchargement fini relit la page : l'élément est là.
function RequiredPage({ section }) {
    const { t } = useTranslation();
    const { consoleId, name } = useParams();
    const { back } = useSettingsNavigation();
    const confirm = useConfirm();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const running = useDownloads().some((download) => download.key === `${section}/${consoleId}/${name}`);
    const wasRunning = useRef(running);
    const [rows, setRows] = useState(undefined);
    const [defaults, setDefaults] = useState(null);
    const [epoch, setEpoch] = useState(0);
    const parent = consolePath(consoleId);
    const streamex = entry?.method === "streamex";
    const item = rows === undefined ? undefined : (rows || []).find((row) => row.name === name) || null;

    useEffect(() => {
        let cancelled = false;

        Promise.all([loadSection(section, consoleId), consoleDefaults(consoleId)]).then(([read, answer]) => {
            if (!cancelled) {
                setRows(read);
                setDefaults(answer.ok ? answer.data : null);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [section, consoleId, name, epoch]);

    useEffect(() => {
        if (wasRunning.current && !running) {
            setEpoch((value) => value + 1);
        }
        wasRunning.current = running;
    }, [running]);

    // LE CHOIX DE LA CONSOLE que cette page règle : ce que la ligne dit régler (`choose`, écrit par le kernel — chantier
    // 54 : au téléphone, la Switch est une console Streamex dont le choix est l'émulateur), et quand il a un sens. Une
    // version d'un émulateur de bureau du serveur (`emulatorFile`) se met en service, si l'image la reconnaît
    // (`tested`) ; un cœur, un émulateur du
    // téléphone ou un BIOS, seulement quand la console en a plusieurs — ou quand c'est déjà lui qui est enregistré.
    const field = item?.choose || null;
    const stored = (field && defaults?.stored?.[field]) || "";
    const several = (rows || []).filter((row) => row.choose === field && (section === "bios" ? row.installed : true)).length > 1;
    const usage = !item?.installed || !entry || !field ? null
        : field === "emulatorFile" ? { label: "consoles.useVersion", help: "consoles.useVersionHelp", known: item.tested !== undefined }
            : several || stored === item.name ? { label: section === "bios" ? "consoles.defaultBios" : "consoles.defaultEmulator", help: "consoles.defaultHelp", known: true } : null;
    const chosen = Boolean(item) && inService(section, item, defaults?.inUse);
    const implicit = chosen && stored !== item.name;

    const choose = (on) => confirm({
        title: t(usage.label),
        message: on ? t(section === "bios" ? "consoles.defaultBiosConfirm" : field === "emulatorFile" ? "consoles.useVersionConfirm" : "consoles.defaultEmulatorConfirm", { name: item.name }) : t("consoles.defaultClearConfirm"),
        confirmLabel: on ? t("consoles.defaultSet") : t("consoles.defaultClear"),
        onConfirm: async () => {
            const answer = await setConsoleDefaults(consoleId, { [field]: on ? item.name : "" });
            toast(answer.ok ? t(on ? "consoles.defaultSaved" : "consoles.defaultCleared", { name: item.name }) : answer.errorMessage || t("settings.actionFailed"));

            if (answer.ok) {
                setEpoch((value) => value + 1);
            }
        },
    });

    const facts = item ? [
        t(item.installed ? "consoles.installed" : "consoles.missing"),
        item.tested === true ? t("consoles.tested") : null,
        item.tested === false ? t("consoles.untested") : null,
        item.installed && item.scoped === false ? t("consoles.unfiled") : null,
    ].filter(Boolean) : [];
    const phone = item?.package === "phone";

    return (
        <SettingsPage title={name} parent={parent} enter={usage ? "cs-default" : null}>
            {item === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : item === null ? <NotFound /> : (
                <>
                    <Section title={t("details")}>
                        <Rows>
                            <InfoRow icon={item.installed ? "bi-check2-circle" : "bi-x-circle"} tone={item.installed ? "green" : "slate"} label={t("games.status")} value={facts.join(" · ")} />
                            {section === "cores" && <InfoRow icon={phone ? "bi-phone" : "bi-pc-display"} tone={phone ? "violet" : "indigo"} label={t("resources.typeLabel")} value={t(phone ? "consoles.packagePhone" : "consoles.packageServer")} />}
                            {item.installed && <InfoRow icon="bi-tag" label={t("consoles.version")} value={item.version} />}
                            {item.installed && <InfoRow icon="bi-hdd" label={t("game.size")} value={formatMegabytes(item.sizeBytes)} />}
                        </Rows>
                    </Section>
                    {usage && (
                        <Section title={t("consoles.usage")}>
                            <Rows>
                                <SwitchRow id="cs-default" label={t(usage.label)} checked={chosen}
                                    disabled={!usage.known || implicit || defaults === null} onChange={choose}
                                    help={!usage.known ? t("consoles.useVersionUnknown") : implicit ? t("consoles.defaultImplicit") : t(usage.help)} />
                            </Rows>
                        </Section>
                    )}
                    {!item.installed && <FetchSources section={section} consoleId={consoleId} item={item} />}
                    {item.installed && (
                        <DangerSection>
                            <DangerRow id="cs-remove" label={t(section === "bios" ? "consoles.removeBios" : "consoles.removeEmulator")} help={t("consoles.removeHelp")}
                                question={t("consoles.removeConfirm", { name: item.name })} confirmLabel={t("consoles.remove")} onConfirm={async () => {
                                    const answer = await deleteInstalled(section, consoleId, item.name);
                                    toast(answer.ok ? t("consoles.removed", { name: item.name }) : answer.errorMessage || t("consoles.removeFailed"));

                                    if (answer.ok) {
                                        back(parent);
                                    }
                                }} />
                        </DangerSection>
                    )}
                </>
            )}
        </SettingsPage>
    );
}

// Les textes d'un émulateur dans la langue de la page (son fichier les porte en anglais, et ses traductions).
const inLanguage = (emulator, language) => {
    const own = (item) => item?.translations?.[language] || {};

    return {
        ...emulator,
        description: own(emulator).description || emulator.description,
        params: (emulator.params || []).map((param) => ({
            ...param,
            label: own(param).label || param.label,
            description: own(param).description || param.description,
            options: param.options && param.options.map((option) => ({ ...option, label: own(param).options?.[option.value] || option.label })),
        })),
        presets: (emulator.presets || []).map((preset) => ({ ...preset, name: own(preset).name || preset.name, description: own(preset).description || preset.description })),
    };
};
const valuesOf = (emulator) => Object.fromEntries((emulator.params || []).map((param) => [param.id, param.value]));

// CE QUE PRÊTENT LES AUTRES SOURCES pour cet émulateur (le porteur, 2026-11-14 : « rajouter les paramètres venant des
// serveurs fédérés dans presets. On rajoute des option group si possible. ») : chaque disque interne, puis chaque
// serveur fédéré, lus à part — ses valeurs (« Ses valeurs ») et ses préréglages perso, chacun avec ses valeurs, déjà
// passées par les paramètres d'ici (storage_params_api.go, lentPreset). `items` : undefined en lecture, null quand la
// source n'a pas répondu. Les disques d'abord, comme les sources des jeux d'une console (Games.jsx).
function useLentPresets(consoleId, emulator) {
    const { t } = useTranslation();
    const { disks, servers } = useData();
    const sources = useMemo(() => [
        ...(disks || []).map((disk) => ({ key: `disk:${disk.name}`, name: disk.name })),
        ...(servers || []).map((server) => ({ key: `peer:${server.id}`, name: server.hostname || server.link || t("consoles.serverNamed", { id: server.id }) })),
    ], [disks, servers, t]);
    const [lent, setLent] = useState({});

    useEffect(() => {
        let cancelled = false;
        setLent({});

        for (const source of sources) {
            fetchRequired("params", source.key, { console: consoleId, emulator }).then((answer) => !cancelled && setLent((current) => ({
                ...current,
                [source.key]: answer.ok ? (answer.data?.consoles || []).flatMap((found) => found.items || []).filter((item) => item.emulator === emulator) : null,
            })));
        }

        return () => {
            cancelled = true;
        };
    }, [consoleId, emulator, sources]);

    return useMemo(() => sources.map((source) => ({ ...source, items: lent[source.key] })), [sources, lent]);
}

// Un prêt dans le sélecteur : sa valeur (sa source et son préréglage — un préréglage d'ici n'a ni « : » ni « / »), son
// nom (« Ses valeurs », ou celui de son préréglage).
const lentId = (source, item) => `${source.key}/${item.preset || ""}`;
const lentName = (item, t) => (item.preset ? item.name : t("consoles.paramsTheirValues"));
const enginesOf = (emulator) => (emulator.engines || []).map(({ engine, name, preset, enabled, error }) => ({ engine, name, preset, enabled, error }));
const textInvalid = (value) => String(value ?? "").trim() === "" || /[\r\n]/.test(String(value ?? ""));

// LE FORMULAIRE D'UN ÉMULATEUR (EmulatorParams de la webapp), construit depuis son fichier : sa présentation, son
// fichier (une ligne) et son interrupteur, son préréglage, ses paramètres, ses moteurs, l'enregistrer comme
// préréglage — un groupe chacun, Enregistrer dans l'encadré du dernier (le formulaire couvre tous ses groupes) ;
// supprimer le préréglage choisi (un des siens), dans « Danger ». Le préréglage se choisit parmi ceux de ce serveur,
// puis ce que prête chaque disque interne et chaque serveur fédéré (useLentPresets), un groupe d'options chacun ;
// choisi, il remplit les champs, et « Enregistrer » écrit ICI.
function EmulatorForm({ initial }) {
    const { t, i18n } = useTranslation();
    const language = (i18n.language || "en").split("-")[0];
    const [emulator, setEmulator] = useState(() => inLanguage(initial, language));
    const [values, setValues] = useState(() => valuesOf(initial));
    const [enabled, setEnabled] = useState(initial.exists ? initial.enabled : true);
    const [engines, setEngines] = useState(() => enginesOf(initial));
    const [preset, setPreset] = useState("");
    const [saveAs, setSaveAs] = useState({ on: false, name: "" });
    const [pending, setPending] = useState(false);
    const lent = useLentPresets(initial.consoleId, initial.emulator);
    const lentById = useMemo(() => new Map(lent.flatMap((source) => (source.items || []).filter((item) => !item.error).map((item) => [lentId(source, item), { source, item }]))), [lent]);
    const borrowed = lentById.get(preset) || null;
    const saved = valuesOf(emulator);
    const savedEngines = enginesOf(emulator);
    const changed = !emulator.exists || Boolean(emulator.error) || enabled !== emulator.enabled
        || emulator.params.some((param) => values[param.id] !== saved[param.id])
        || engines.some((setting, index) => setting.preset !== savedEngines[index]?.preset || setting.enabled !== savedEngines[index]?.enabled);
    const selected = emulator.presets.find((item) => item.id === preset) || null;
    const readOnlyNames = new Set(emulator.presets.filter((item) => !item.override).map((item) => String(item.name).trim().toLowerCase()));
    const textParams = emulator.params.filter((param) => param.type === "text");
    const anyTextInvalid = textParams.some((param) => textInvalid(values[param.id]));
    const presetNameProblem = !saveAs.on ? "" : !saveAs.name.trim() ? t("consoles.paramsPresetNameInvalid") : readOnlyNames.has(saveAs.name.trim().toLowerCase()) ? t("consoles.paramsPresetNameReadOnly") : "";

    const apply = (answer) => {
        const next = inLanguage(answer.data, language);
        setEmulator(next);
        setValues(valuesOf(next));
        setEnabled(next.enabled);
        setEngines(enginesOf(next));
    };

    // Un préréglage ne fait que REMPLIR les champs ; rien n'est écrit avant « Enregistrer ». Celui d'une autre source
    // n'apporte que ses valeurs (le porteur, 2026-11-14 : « on charge que les données de celui-ci ») : rien ne le
    // garde ici, sauf « Enregistrer comme préréglage », sous le nom qu'on lui donne.
    const choose = (id) => {
        setPreset(id);
        const from = lentById.get(id);

        if (from) {
            setSaveAs({ on: false, name: "" });
            setValues((current) => ({ ...current, ...from.item.values }));
            toast([t("consoles.presetFilledFrom", { name: lentName(from.item, t), source: from.source.name }),
                from.item.ignored?.length ? t("consoles.presetIgnored", { list: from.item.ignored.join(", ") }) : null].filter(Boolean).join(" "));

            return;
        }
        const chosen = emulator.presets.find((item) => item.id === id);
        setSaveAs(chosen && chosen.override && !chosen.error ? { on: true, name: chosen.name } : { on: false, name: "" });

        if (chosen && !chosen.error) {
            setValues((current) => ({ ...current, ...chosen.values }));
            toast([t("consoles.presetFilled", { name: chosen.name }), chosen.ignored?.length ? t("consoles.presetIgnored", { list: chosen.ignored.join(", ") }) : null].filter(Boolean).join(" "));
        }
    };

    const save = async () => {
        setPending(true);
        const presetName = saveAs.on ? saveAs.name.trim() : "";
        const answer = await writeEmulatorParams(emulator.consoleId, emulator.emulator, {
            enabled, values, preset: presetName,
            ...(engines.length ? { engines: engines.map(({ engine, preset: chosen, enabled: on }) => ({ engine, preset: chosen, enabled: on })) } : {}),
        });
        setPending(false);
        toast(answer.ok ? (answer.data?.savedPreset ? t("consoles.paramsSavedWithPreset", { name: presetName }) : t("consoles.paramsSaved")) : answer.errorMessage || t("consoles.paramsFailed"));

        // La réponse est l'émulateur relu (son fichier tel qu'enregistré).
        if (answer.ok && answer.data?.params) {
            apply(answer);
            setPreset(answer.data.savedPreset || "");

            if (!answer.data.savedPreset) {
                setSaveAs({ on: false, name: "" });
            }
        }
    };

    const setParam = (id, value) => setValues((current) => ({ ...current, [id]: value }));
    const setEngine = (engine, patch) => setEngines((current) => current.map((setting) => (setting.engine === engine ? { ...setting, ...patch } : setting)));

    // LES PARAMÈTRES TEXTE, un contexte de Skeleton qui porte leur règle (une ligne, non vide) : leurs noms
    // préfixés (`pm-<id>`), pour que leurs identifiants dans la page ne croisent ceux d'aucun autre champ.
    const textContext = {
        values: Object.fromEntries(textParams.map((param) => [`pm-${param.id}`, values[param.id] ?? ""])),
        setValues: (next) => setValues((current) => ({
            ...current,
            ...Object.fromEntries(textParams.filter((param) => `pm-${param.id}` in next).map((param) => [param.id, next[`pm-${param.id}`]])),
        })),
        files: {},
        setFiles: () => {},
        rulesets: Object.fromEntries(textParams.map((param) => [`pm-${param.id}`, (data) => (textInvalid(data[`pm-${param.id}`]) ? Ruleset.isInvalid(t("consoles.paramTextInvalid")) : null)])),
    };
    const presetNameContext = {
        ...fieldContext("pm-preset-name", saveAs.name, (name) => setSaveAs((current) => ({ ...current, name }))),
        rulesets: { "pm-preset-name": () => (presetNameProblem ? Ruleset.isInvalid(presetNameProblem) : null) },
    };

    // Un paramètre, sa ligne : un interrupteur, un sélecteur (des chaînes, comme le fichier), un champ.
    const row = (param) => {
        const id = `pm-${param.id}`;
        const help = [param.description, param.config].filter(Boolean).join(" — ") || null;

        return param.type === "switch" ? <SwitchRow key={id} id={id} label={param.label} help={help} checked={Boolean(values[param.id])} onChange={(on) => setParam(param.id, on)} />
            : param.type === "select" ? <SelectRow key={id} id={id} label={param.label} help={help} value={String(values[param.id] ?? "")} onChange={(value) => setParam(param.id, value)}
                options={(param.options || []).map((option) => ({ value: String(option.value), label: option.label }))} />
                : <TextRow key={id} id={id} label={param.label} help={help} context={textContext} props={{ autoCapitalize: "off" }} />;
    };
    const intro = Boolean(emulator.description || emulator.requirements?.length);
    // LE SÉLECTEUR DES PRÉRÉGLAGES, un groupe par source (`<optgroup>`, Form.Select) : ce serveur, puis chaque disque
    // interne et chaque serveur fédéré — sa lecture, sa raison quand il ne répond pas, ou qu'il ne prête rien.
    const unavailable = (source, label) => [{ value: `${source.key}/`, label, attributes: { disabled: true } }];
    const presetOptions = [
        { value: "", label: t("consoles.paramsPresetNone") },
        { label: t("consoles.presetsHere"), options: emulator.presets.length ? emulator.presets.map((item) => ({ value: item.id, label: item.error ? `${item.name} — ${t("consoles.presetBroken")}` : item.name }))
            : [{ value: "here/", label: t("consoles.presetsNone"), attributes: { disabled: true } }] },
        ...lent.map((source) => ({
            label: source.name,
            options: source.items === undefined ? unavailable(source, t("settings.loading"))
                : source.items === null ? unavailable(source, reasonOf("unreachable", t))
                    : !source.items.length ? unavailable(source, t("consoles.presetsLendsNothing"))
                        : source.items.map((item) => ({ value: lentId(source, item), label: item.error ? `${lentName(item, t)} — ${reasonOf(item.error, t)}` : lentName(item, t), attributes: item.error ? { disabled: true } : undefined })),
        })),
    ];
    const presetHelp = borrowed ? t("consoles.paramsPresetLentHelp", { source: borrowed.source.name }) : selected && !selected.override ? t("consoles.paramsPresetReadOnly") : t("consoles.paramsPresetHelp");

    return (
        <Form props={{ onSubmit: (event) => {
            event.preventDefault();

            if (changed && !anyTextInvalid && !presetNameProblem && !pending) {
                save();
            }
        } }}>
            {/* LE FICHIER EST UNE LIGNE (le porteur, 2026-11-14, remarque b), comme « Le dossier » de l'Apparence : son
                icône, son chemin en chasse fixe — plus collé dans le paragraphe. */}
            <Section title={t("consoles.paramsFileTitle")}>
                {intro && (
                    <Note className="mt-0">
                        {emulator.description && <span className="d-block">{emulator.description}</span>}
                        {emulator.requirements?.length > 0 && <span className="d-block">{t("consoles.paramsRequirements", { list: emulator.requirements.join(", ") })}</span>}
                    </Note>
                )}
                {/* LE FICHIER ET SON ÉTAT, des renseignements (InfoRow) : son chemin, puis, s'il manque ou ne se lit pas, ce
                    qu'il en est et ce que fera l'enregistrement — plus des alertes (le porteur, 2026-11-16 : « retirer les
                    alerts »). */}
                {(emulator.file || emulator.error || !emulator.exists) && (
                    <Rows className={intro ? "mt-3" : ""}>
                        {emulator.file && <InfoRow icon="bi-file-earmark-code" label={t("consoles.paramsFileLabel")} value={emulator.file} mono />}
                        {emulator.error && <InfoRow icon="bi-exclamation-triangle" tone="rose" label={t("consoles.paramsFileState")} value={t("consoles.paramsFileUnreadable")}
                            help={t("consoles.paramsBroken", { error: emulator.error })} />}
                        {!emulator.exists && <InfoRow icon="bi-file-earmark" label={t("consoles.paramsFileState")} value={t("consoles.paramsFileAbsent")} help={t("consoles.paramsMissing")} />}
                    </Rows>
                )}
                <Rows className={intro || emulator.file || emulator.error || !emulator.exists ? "mt-3" : ""}>
                    <SwitchRow id="pm-enabled" label={t("consoles.paramsEnabled")} help={t("consoles.paramsEnabledHelp")} checked={Boolean(enabled)} onChange={setEnabled} />
                </Rows>
            </Section>
            {(emulator.presets.length > 0 || lent.length > 0) && (
                <Section title={t("consoles.paramsPresetsTitle")}>
                    <Rows>
                        <SelectRow id="pm-preset" label={t("consoles.paramsPreset")} help={presetHelp} value={preset} onChange={choose} options={presetOptions} />
                    </Rows>
                </Section>
            )}
            {emulator.params.length > 0 && <Section title={t("consoles.tabParams")}><Rows>{emulator.params.map(row)}</Rows></Section>}
            {engines.length > 0 && (
                <Section title={t("consoles.paramsEngines")} help={t("consoles.paramsEnginesHelp")}>
                    <Rows>
                        {engines.map((setting) => (
                            <React.Fragment key={setting.engine}>
                                <SelectRow id={`pm-engine-${setting.engine}`} label={setting.name || setting.engine} value={setting.preset || ""} onChange={(chosen) => setEngine(setting.engine, { preset: chosen })}
                                    help={setting.error ? t("consoles.paramsEngineBroken", { error: setting.error }) : t("consoles.paramsEngineHelp")}
                                    options={[{ value: "", label: t("consoles.paramsEngineNone") }, ...emulator.presets.filter((item) => !item.error).map((item) => ({ value: item.id, label: item.name }))]} />
                                <SwitchRow id={`pm-engine-on-${setting.engine}`} label={t("consoles.paramsEngineEnabled")} help={setting.name || setting.engine}
                                    checked={Boolean(setting.enabled)} onChange={(on) => setEngine(setting.engine, { enabled: on })} />
                            </React.Fragment>
                        ))}
                    </Rows>
                </Section>
            )}
            <Section title={t("consoles.paramsSaveTitle")}>
                <Rows>
                    <SwitchRow id="pm-save-as" label={t("consoles.paramsSaveAsPreset")} help={t("consoles.paramsSaveAsPresetHelp")} checked={saveAs.on}
                        onChange={(on) => setSaveAs((current) => ({ ...current, on }))} />
                    {saveAs.on && <TextRow id="pm-preset-name" label={t("consoles.paramsPresetName")} help={t("consoles.paramsPresetNameHelp")} context={presetNameContext} />}
                    <SubmitRow>
                        <FormButton id="pm-save" label={t("settings.save")} isReplaced={Boolean(changed)} pending={pending} disabled={anyTextInvalid || Boolean(presetNameProblem)} />
                    </SubmitRow>
                </Rows>
            </Section>
            {selected?.override && (
                <DangerSection id="pm-preset-del" label={t("consoles.paramsPresetDelete")} question={t("consoles.paramsPresetDeleteConfirm", { name: selected.name })} onConfirm={async () => {
                    const answer = await deleteEmulatorPreset(emulator.consoleId, emulator.emulator, selected.id);
                    toast(answer.ok ? t("consoles.paramsPresetDeleted", { name: selected.name }) : answer.errorMessage || t("consoles.paramsFailed"));

                    if (answer.ok && answer.data?.params) {
                        setEmulator(inLanguage(answer.data, language));
                        setPreset("");
                        setSaveAs({ on: false, name: "" });
                    }
                }} />
            )}
            <GuideSection guides={[["emulatorSettings", "change-an-emulators-settings"]]} />
        </Form>
    );
}

// /settings/consoles/<console>/params/<émulateur> (« Paramètres d'Eden », une ligne du « Plus » de la console) :
// l'émulateur relu à chaque ouverture dans la liste des paramètres de ce serveur — jamais pris dans l'historique,
// qui garderait, après un rechargement, le fichier d'avant le dernier enregistrement.
function ParamsPage() {
    const { t } = useTranslation();
    const { consoleId, emulator: emulatorId } = useParams();
    const [emulator, setEmulator] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        listEmulatorParams().then((answer) => {
            if (!cancelled) {
                setEmulator((answer.ok ? (answer.data?.emulators || []) : []).find((entry) => entry.consoleId === consoleId && entry.emulator === emulatorId) || null);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, emulatorId]);

    return (
        <SettingsPage title={emulator ? paramsLabel(emulator, t) : emulatorId} parent={consolePath(consoleId)}>
            {emulator === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>
                : emulator === null ? <NotFound /> : <EmulatorForm key={`${emulator.consoleId}/${emulator.emulator}`} initial={emulator} />}
        </SettingsPage>
    );
}

// L'APPARENCE D'UNE CONSOLE (chantier 53, P9, #678 ; la décision 11 et le modèle 30, puis les remarques d et e du
// porteur, 2026-11-14) — /settings/consoles/<console>/appearance :
//   - ses informations (le nom, le constructeur, l'année), le dossier de ses images ;
//   - SES HUIT EMPLACEMENTS (« gardons maximum 8 images par console. On peut donc remplacer des images dans les 1 à 8
//     slots seulement ») : la bande de la console les montre dans cet ordre. Un emplacement pris ouvre sa page ; un
//     libre, la recherche qui le remplit ;
//   - les images du dossier EN TROP, quand il y en a : signalées, ni ouvertes ni effacées — ce sont ses fichiers ;
//   - « Plus » : « Ajouter une image » (le premier emplacement libre ; grisée quand les huit sont pris) et
//     « Réinitialiser les images par défaut » — qui ne détruit rien : le serveur redemande les images par défaut à la
//     visite suivante, dans les emplacements libres (console_backgrounds.go). C'est pourquoi elle est dans « Plus » et
//     non dans « Danger » (la règle des boutons : « Danger » pour ce qui détruit).
// …/appearance/<n>          un emplacement pris : l'image, la remplacer (« Plus ») — une image ne se supprime pas
//                           seule (le porteur, 2026-11-14 : « Il est préférable de ne pas laisser le choix de supprimer
//                           une image. On ne peut que la remplacer. ») ;
// …/appearance/search?slot= la recherche qui remplit ou remplace l'emplacement (ImageSearch, ImageSearch.jsx),
//                           l'image actuelle au-dessus quand il est pris.
// L'envoi d'un fichier (Form.File) et l'ancienne grille des « Fonds » sont partis (AGENTS.md : aucune rétro
// compatibilité).

const appearancePath = (consoleId) => `${consolePath(consoleId)}/appearance`;
const slotSearchPath = (consoleId, slot) => `${appearancePath(consoleId)}/search?${new URLSearchParams({ slot })}`;

// Les emplacements d'une console, lus : `{slots, extra, server, serverName}` ; undefined en lecture ; `failure` (sa
// raison, peut-être vide) quand le serveur ne les a pas rendus. `server` : le serveur fédéré qui les prête, quand la
// console n'a aucune image ici (`#740`) — ses images se demandent avec lui.
function useBackgroundSlots(consoleId, epoch = 0) {
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
function useAppearanceLevels(consoleId, slot = null) {
    const { t } = useTranslation();
    const entry = useEmulated().find((item) => item.id === consoleId);

    return {
        [consolePath(consoleId)]: entry?.name,
        [appearancePath(consoleId)]: t("consoles.appearance"),
        ...(slot ? { [`${appearancePath(consoleId)}/${slot}`]: t("appearance.slot", { slot }) } : {}),
    };
}

// Un emplacement changé : ses images et la liste de la bande se relisent.
const forgetSlots = (consoleId) => {
    forgetConsoleBackgrounds(consoleId);
    forgetBand(consoleId);
};

// La vignette d'une image du dossier (chargée avec le jeton, libs/images.js), ou du serveur fédéré qui les prête.
function BackgroundThumb({ consoleId, name, server = null }) {
    const image = useImage(name ? consoleBackgroundPath(consoleId, name, 256, server) : null);

    return <Thumb url={image?.url || null} fit="cover" />;
}

// L'image d'un emplacement pris, en grand : sa page (« Aperçu ») et sa recherche (« L'image actuelle »).
function SlotPreview({ consoleId, name, server = null }) {
    const image = useImage(consoleBackgroundPath(consoleId, name, 1280, server));

    return <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-body-secondary">{image ? <img className="object-fit-cover" src={image.url} alt="" /> : <span></span>}</div>;
}

// L'APPARENCE D'UNE CONSOLE (chantier 53, P9, et les choix du porteur qui suivent) : ses huit images, toujours là —
// « on n'a pas besoin du lien "Ajouter une image" car on aura toujours 8 images. On ne peut que les remplacer. » —, une
// ligne chacune, qui ouvre sa page (Remplacer). Un emplacement que les fournisseurs n'ont pas pu remplir n'a pas de
// ligne : ni « Libre », ni « Ajouter ». En « Danger », LA VRAIE REMISE À ZÉRO (« le serveur vide les 8 emplacements, puis
// les remplit des images par défaut »), derrière la fenêtre de confirmation ; la liste se relit, et sa lecture refait le
// remplissage par défaut (metadata/console_backgrounds.go). UNE CONSOLE SANS AUCUNE IMAGE ICI montre celles du premier
// serveur fédéré qui en prête (`#740` ; le porteur, 2026-11-16 : « On prend le premier fédéré pour ne pas se prendre la
// tête ») : « Le dossier » dit leur provenance, et l'aide des images ce qu'un geste change — remplacer en écrit une ici,
// et la console montre alors les siennes ; réinitialiser redemande celles des fournisseurs de cette machine.
function AppearancePage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [epoch, setEpoch] = useState(0);
    const read = useBackgroundSlots(consoleId, epoch);
    const filled = (read?.slots || []).filter((slot) => slot.name);
    const levels = useAppearanceLevels(consoleId);

    const reset = async () => {
        const answer = await resetConsoleBackgrounds(consoleId);
        toast(answer.ok ? t("appearance.resetConsoleDone") : answer.errorMessage || t("settings.actionFailed"));

        if (answer.ok) {
            forgetSlots(consoleId);
            setEpoch((value) => value + 1);
        }
    };

    return (
        <SettingsPage title={t("consoles.appearance")} parent={consolePath(consoleId)} levels={levels}>
            {consoles && !entry ? <NotFound /> : (
                <>
                    <Section title={t("appearance.consoleTitle")}>
                        {!entry ? <Note className="mt-0">{t("settings.loading")}</Note> : (
                            <Rows>
                                <InfoRow icon="bi-joystick" label={t("appearance.name")} value={entry.name} />
                                <InfoRow icon="bi-building" label={t("appearance.maker")} value={entry.publisher} />
                                <InfoRow icon="bi-calendar3" label={t("appearance.released")} value={entry.year ? String(entry.year) : null} />
                            </Rows>
                        )}
                    </Section>
                    <Section title={t("consoles.backgroundsFolderTitle")} help={t("consoles.backgroundsFolderHelp")}>
                        <Rows>
                            <InfoRow icon="bi-folder2" label={t("consoles.backgroundsFolder")} value={`metadata/${consoleId}/background/`} mono />
                            {read?.server && <InfoRow icon="bi-arrow-left-right" tone="green" label={t("game.origin")} value={read.serverName} />}
                        </Rows>
                    </Section>
                    <Section title={t("appearance.slotsTitle")} help={read?.server ? t("appearance.slotsLentHelp", { server: read.serverName }) : t("appearance.slotsHelp")}>
                        {read?.failure !== undefined ? <Note className="mt-0">{read.failure || t("settings.actionFailed")}</Note> : (
                            <Listing items={filled} loading={read === undefined} empty={t("appearance.slotsNone")} placeholderRow={() => <PlaceholderRow lead="thumb" />}
                                render={(slot) => {
                                    const key = `ap-slot-${slot.slot}`;

                                    return (
                                        <NavRow key={slot.slot} id={key} lead={<BackgroundThumb consoleId={consoleId} name={slot.name} server={read.server} />} label={t("appearance.slot", { slot: slot.slot })}
                                            help={slot.name} onOpen={() => open(`${appearancePath(consoleId)}/${slot.slot}`, key)} />
                                    );
                                }} />
                        )}
                    </Section>
                    {read?.extra.length > 0 && (
                        <Section title={t("appearance.extraTitle")} help={t("appearance.extraHelp")}>
                            <Listing items={read.extra} render={(name) => (
                                <InfoRow key={name} lead={<BackgroundThumb consoleId={consoleId} name={name} />} label={name} value={t("appearance.extraValue")} />
                            )} />
                        </Section>
                    )}
                    <DangerSection id="ap-reset" icon="bi-arrow-counterclockwise" label={t("appearance.reset")} help={t("appearance.resetConsoleHelp")}
                        question={t("appearance.resetConsoleConfirm")} confirmLabel={t("appearance.resetAction")} onConfirm={reset} />
                </>
            )}
        </SettingsPage>
    );
}

// /settings/consoles/<console>/appearance/<n> : UN EMPLACEMENT PRIS (le modèle 33) — l'image en grand, ses détails ;
// « Remplacer cette image » (la recherche). Pas de « Supprimer » ni de section « Danger » : une image ne se supprime pas
// seule, elle se remplace (le porteur, 2026-11-14). Vide ou hors de 1 à 8, la page n'existe pas.
function SlotPage() {
    const { t } = useTranslation();
    const { consoleId, slot: slotParam } = useParams();
    const { open } = useSettingsNavigation();
    const read = useBackgroundSlots(consoleId);
    const slot = Number(slotParam);
    const name = read?.slots.find((item) => item.slot === slot)?.name || "";
    const parent = appearancePath(consoleId);
    const levels = useAppearanceLevels(consoleId);

    return (
        <SettingsPage title={t("appearance.slot", { slot: slotParam })} parent={parent} levels={levels}>
            {read === undefined ? <Section title={t("appearance.preview")}><Note className="mt-0">{t("settings.loading")}</Note></Section> : !name ? <NotFound /> : (
                <>
                    <Section title={t("appearance.preview")}>
                        <SlotPreview consoleId={consoleId} name={name} server={read.server} />
                    </Section>
                    <Section title={t("details")}>
                        <Rows>
                            <InfoRow icon="bi-collection" label={t("appearance.slotLabel")} value={t("appearance.slotOf", { slot })} />
                            <InfoRow icon="bi-file-earmark-image" label={t("consoles.paramsFileLabel")} value={name} mono />
                            {read.server && <InfoRow icon="bi-arrow-left-right" tone="green" label={t("game.origin")} value={read.serverName} />}
                        </Rows>
                    </Section>
                    <Section title={t("more")}>
                        <Rows>
                            <NavRow id="ap-replace" icon="bi-search" tone="indigo" label={t("appearance.replace")} help={t("appearance.replaceSlotHelp")}
                                onOpen={() => open(slotSearchPath(consoleId, slot), "ap-replace")} />
                        </Rows>
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}

// /settings/consoles/<console>/appearance/search?slot=<n> : REMPLACER L'IMAGE D'UN EMPLACEMENT (le modèle 34, la
// recherche à la place de l'envoi) — « L'image actuelle » d'abord (le porteur, 2026-11-14 : « rajoute l'image de base en
// haut pour savoir quelle image on est en train de remplacer ») ; puis un titre de jeu de la console, ses images chez les
// fournisseurs ; une image choisie, la fenêtre, puis le serveur la récupère dans l'emplacement ; retour d'où l'on venait.
// Un emplacement vide n'a pas de page : on ne fait que remplacer (« On ne peut que les remplacer »).
function SlotSearchPage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const [params] = useSearchParams();
    const { back } = useSettingsNavigation();
    const read = useBackgroundSlots(consoleId);
    const slot = Number(params.get("slot"));
    const valid = Number.isInteger(slot) && slot >= 1 && slot <= 8;
    const name = read?.slots.find((item) => item.slot === slot)?.name || "";
    const parent = `${appearancePath(consoleId)}/${slot}`;
    const levels = useAppearanceLevels(consoleId, slot);

    return (
        <SettingsPage title={t("appearance.replaceSlotTitle", { slot })} parent={parent} levels={levels}>
            {!valid ? <NotFound /> : read === undefined ? <Section title={t("appearance.searchTitle")}><Note className="mt-0">{t("settings.loading")}</Note></Section> : !name ? <NotFound /> : (
                <>
                    <Section title={t("appearance.current")}>
                        <SlotPreview consoleId={consoleId} name={name} server={read.server} />
                    </Section>
                    <Section title={t("appearance.searchTitle")} help={t("appearance.searchConsoleHelp")}>
                        <ImageSearch fit="cover" search={(query) => searchConsoleBackgrounds(consoleId, query)}
                            confirmOf={(image) => ({
                                title: t("appearance.replaceSlotConfirmTitle", { slot }),
                                message: t("appearance.chosenConfirm", { game: image.game, source: t(`images.services.${image.source}`) }),
                                confirmLabel: t("appearance.replaceAction"),
                            })}
                            onChoose={async (image) => {
                                const answer = await setConsoleBackground(consoleId, slot, image);

                                if (answer.ok) {
                                    forgetSlots(consoleId);
                                    toast(t("appearance.slotSet", { slot }));
                                    back(parent);
                                }

                                return answer;
                            }} />
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}

// /settings/consoles : les consoles émulées, cherchées (`?q=`), une page à la fois (`?page=`).
function ConsolesPage() {
    const { t } = useTranslation();
    const { consoles, countOf } = useData();
    const { open } = useSettingsNavigation();
    const emulated = useEmulated();
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const needle = search.trim().toLowerCase();
    const shown = emulated.filter((entry) => !needle || `${entry.name} ${entry.id}`.toLowerCase().includes(needle));

    return (
        <SettingsPage title={t("settings.items.consoles.label")}>
            <Section title={t("consoles.listTitle")}>
            <Listing items={shown} loading={!consoles} search={search} onSearch={(q) => update({ q })} page={read("page")} onPage={(page) => update({ page })}
                placeholder={t("consoles.search")} found={t("consoles.found", { count: shown.length })} empty={t("consoles.none")}
                render={(entry) => (
                    <NavRow key={entry.id} id={`cs-open-${entry.id}`} icon={entry.method === "streamex" ? "bi-display" : "bi-joystick"} tone={entry.method === "streamex" ? "rose" : "indigo"} label={entry.name}
                        onOpen={() => open(`/settings/consoles/${encodeURIComponent(entry.id)}`, `cs-open-${entry.id}`)}
                        help={[entry.method === "streamex" ? t("tab.streamex") : t("tab.retrogaming"), countOf(entry.id) !== null ? t("console.libraryCount", { count: countOf(entry.id) }) : null].filter(Boolean).join(" · ")} />
                )} />
            </Section>
            <GuideSection guides={[["consoles"], ["library"]]} />
        </SettingsPage>
    );
}

// /settings/consoles/* (Settings.jsx).
// LA JAQUETTE QUE GARDE UN DOSSIER SANS JEU, en tête de sa ligne ; l'icône d'une image s'il n'en garde pas.
function OrphanCover({ consoleId, orphan }) {
    const image = useImage(orphan.cover ? metadataOrphanCoverPath(consoleId, orphan.gameId, orphan.disk) : null);

    return <Thumb url={image?.url || null} fit="contain" />;
}

// /settings/consoles/<console>/metadata-orphans : LES MÉTADONNÉES SANS JEU (le porteur, 2026-11-14 : « on peut se
// retrouver avec des metadata des jeux qu'on n'a plus accès. Ce qui fera des métadata orphelin. […] Cette page listera
// que les metadata qui non pas jeux et quand on clique dessus ça propose de le supprimer avec confirmation. ») — une
// ligne de « Plus » de la console, sous ses jeux. Chaque dossier qu'aucun jeu accessible ne porte : son titre gardé
// (sinon son identifiant), sa jaquette, où il vit (le serveur, ou le disque) ; le toucher demande, puis le retire — le
// kernel revérifie qu'il est toujours sans jeu (un disque rebranché entre-temps le garde). Une liste lue en asynchrone :
// feuilletée, ses fantômes pendant la lecture, sa ligne grisée quand elle est vide.
//
// LES JEUX DE LA CONSOLE ENCORE EN LECTURE (chantier 53, 2026-11-14) : le kernel ne juge un dossier orphelin qu'avec
// l'empreinte de chaque jeu en mémoire, et la lit en fond quand elle manque (après un redémarrage : une minute et plus
// pour une grande bibliothèque) — il répond 503 `games_not_ready` sans attendre. La liste le dit d'une ligne grisée, et
// s'abonne au sujet `metadata-orphans/<console>` : le kernel lui pousse l'instant où les empreintes sont lues, et elle
// se relit alors, une fois (#751 : elle se relisait toutes les quatre secondes). Une SUPPRESSION confirmée attend sa réponse sur sa
// ligne (`pending` : grisée, son tourniquet ; un second geste ne repart pas) : partie (ou déjà partie, 404), la ligne
// s'en va ; gardée — un jeu la porte de nouveau (409), les jeux pas encore lus (503) —, l'avis le dit et la ligne revient.
function OrphansPage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const confirm = useConfirm();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [read, update] = useQueryState({ page: "1" });
    const [orphans, setOrphans] = useState(undefined);
    const [reading, setReading] = useState(false);
    const [readAgain, setReadAgain] = useState(0);
    const [removing, setRemoving] = useState([]);
    const removingNow = useRef(new Set());
    const keyOf = (orphan) => `or-${orphan.disk || "server"}-${orphan.gameId}`;

    // Tant que le kernel lit les empreintes : son sujet dit quand c'est fait.
    const token = readToken();
    const channel = useKernelChannel(reading && kernelReachable(token) ? kernelTarget(null, token) : null);
    useKernelTopic(channel, reading ? `metadata-orphans/${consoleId}` : null, null, (state) => {
        if (state?.ready) {
            setReadAgain((count) => count + 1);
        }
    });

    useEffect(() => {
        let cancelled = false;
        const fetchList = () => listMetadataOrphans(consoleId).then((answer) => {
            if (cancelled) {
                return;
            }

            if (answer.errorCode === "games_not_ready") {
                setReading(true);

                return;
            }

            if (!answer.ok) {
                toast(answer.errorMessage || t("settings.loadFailed"));
            }
            setReading(false);
            setOrphans(answer.ok ? answer.data?.orphans || [] : []);
        });

        setOrphans(undefined);
        fetchList();

        return () => {
            cancelled = true;
        };
    }, [consoleId, readAgain, t]);

    const where = (orphan) => (orphan.disk ? t("consoles.orphanOnDisk", { disk: orphan.disk }) : t("consoles.orphanOnServer"));
    const remove = async (orphan, name) => {
        const key = keyOf(orphan);

        if (removingNow.current.has(key)) {
            return;
        }
        removingNow.current.add(key);
        setRemoving((current) => [...current, key]);
        const answer = await deleteMetadataOrphan(consoleId, orphan.gameId, orphan.disk);

        removingNow.current.delete(key);
        setRemoving((current) => current.filter((other) => other !== key));

        if (!answer.ok && answer.status !== 404) {
            toast(answer.errorMessage || t("settings.actionFailed"));

            return;
        }
        setOrphans((current) => (current || []).filter((other) => keyOf(other) !== key));
        toast(t("consoles.orphanRemoved", { name }));
    };
    const ask = (orphan) => {
        const name = orphan.title || orphan.gameId;

        confirm({
            title: t("consoles.orphanRemove"),
            message: t("consoles.orphanRemoveConfirm", { name }),
            confirmLabel: t("consoles.orphanRemove"),
            onConfirm: () => remove(orphan, name),
        });
    };

    return (
        <SettingsPage title={t("consoles.orphans")} parent={consolePath(consoleId)} levels={{ [consolePath(consoleId)]: entry?.name }}>
            <Section title={t("consoles.orphansTitle")} help={t("consoles.orphansHelp")}>
                {reading ? <EmptyState icon="bi-hourglass-split" text={t("consoles.orphansReading")} /> : (
                    <Listing items={orphans || []} loading={orphans === undefined} page={read("page")} onPage={(page) => update({ page })}
                        found={t("consoles.orphansFound", { count: (orphans || []).length })} empty={t("consoles.orphansNone")}
                        placeholderRow={() => <PlaceholderRow lead="thumb" />}
                        render={(orphan) => {
                            const key = keyOf(orphan);

                            return (
                                <NavRow key={key} id={key} lead={<OrphanCover consoleId={consoleId} orphan={orphan} />} label={orphan.title || orphan.gameId}
                                    help={orphan.title ? `${where(orphan)} · ${orphan.gameId}` : where(orphan)} opens="confirm" pending={removing.includes(key)}
                                    onOpen={() => ask(orphan)} />
                            );
                        }} />
                )}
            </Section>
        </SettingsPage>
    );
}

export function ConsolesRoutes() {
    return (
        <Routes>
            <Route index element={<ConsolesPage />} />
            <Route path=":consoleId" element={<ConsolePage />} />
            <Route path=":consoleId/params/:emulator" element={<ParamsPage />} />
            <Route path=":consoleId/cores/:name" element={<RequiredPage key="cores" section="cores" />} />
            <Route path=":consoleId/bios/:name" element={<RequiredPage key="bios" section="bios" />} />
            <Route path=":consoleId/appearance" element={<AppearancePage />} />
            <Route path=":consoleId/appearance/search" element={<SlotSearchPage />} />
            <Route path=":consoleId/appearance/:slot" element={<SlotPage />} />
            <Route path=":consoleId/games/*" element={<GamesRoutes />} />
            <Route path=":consoleId/metadata-orphans" element={<OrphansPage />} />
            <Route path=":consoleId/status" element={<ConsoleStatusPage />} />
            <Route path="*" element={<Navigate to="/settings/consoles" replace />} />
        </Routes>
    );
}
