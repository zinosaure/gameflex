import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { consoleDefaults, fetchServer, listBios, listCores, listStreamexSaves, listUserStates, readToken, romSource, saveRAMOf, stateThumbnailPath } from "../../libs/api.js";
import { formatSize, STATUS_VARIANT, statusOf } from "../../libs/format.js";
import { discName } from "../../libs/games.js";
import { sourcesOf, summaryOf } from "../../libs/metadata.js";
import { splitTitle } from "../../libs/visuals.js";
import { probeNetworkAndSuggestPreset } from "../../engine/stream/shared/libs/network-probe.js";
import { useFederationSession } from "../../libs/federation.js";
import { FederatedRefusal } from "../access/FederatedAccount.jsx";
import { federatedAccountPath } from "../../settings/account/MyFederation.jsx";
import { allows, hostRefusal, useAuth, useData } from "../../libs/data.jsx";
import { isFavorite, setFavorite, useFavorites } from "../../libs/favorites.js";
import { focusElement } from "../../libs/input.js";
import { toast } from "../../shell/Shell.jsx";
import SidePanel from "../../shell/SidePanel.jsx";
import { FormButton } from "../../ui/Forms.jsx";
import { GameBanner } from "./HeroArt.jsx";
import NavRow, { ChoiceRow, EmptyState, InfoRow, Note, QuickTiles, Rows, Section, SelectRow, SubmitRow } from "../../ui/Bricks.jsx";

// L'ENGRENAGE D'UN JEU : « Informations et réglages » — ce que la fiche verrouillée
// montrait, et les réglages de la fiche d'un jeu de la webapp (GameDetailPanel.jsx),
// recopiés et rien de plus : le disque, l'émulateur, le BIOS, les réglages du flux, les
// sauvegardes — et, dans « Plus », le jeu parmi mes favoris (le porteur, 2026-11-16 : il
// remplace le téléchargement de la ROM, parti au Catalogue des Réglages). Puis « Continuer » :
// le choix de l'état à reprendre.
//
// OÙ VIT CHAQUE RÉGLAGE, ET COMMENT IL ATTEINT LE LANCEMENT. Jouer ouvre toujours
// la page du jeu de la webapp (`?game=`), une autre origine : le launcher n'écrit
// pas son stockage. Ce qui est réglé ici voyage donc dans le lien
// (`core`, `bios`, `resolution`, `frameRate`, `bitrate`, `state`), que la page lit
// (Games.jsx, `presetFromAddress`) et passe au lancement. L'émulateur et le BIOS ne
// sont gardés nulle part ailleurs (un choix de la fiche, pour un lancement) ; la fiche
// part du défaut de la console, que l'administrateur règle (chantier 53) ;
// le flux est retenu par console dans CE navigateur, comme la webapp le fait chez elle.

// Le choix de l'émulateur et du BIOS, par jeu, pour la page (la webapp ne les garde pas non plus).
const choices = new Map();

export const choiceOf = (gameKey) => choices.get(gameKey) || {};

export const setChoice = (gameKey, change) => choices.set(gameKey, { ...choiceOf(gameKey), ...change });

// LE FLUX QUE L'UTILISATEUR CHOISIT, par console, DANS CE NAVIGATEUR — jamais au compte ni au serveur (le porteur,
// 2026-11-16 : « le choix de flux doit être par navigateur car jouer sur un téléphone dans un bus avec des données
// mobiles n'est pas le même débit que joué sur son ordinateur sur le même réseau à la maison »). Le navigateur ne
// garde que ce qu'il a touché. La clé est celle d'une console à jeux (`ps5`, `snes`…) ou d'une console Remote Play,
// une par appareil (`device-…`, l'identifiant de son écran, Platforms.jsx), qu'elle soit d'ici ou d'un serveur
// fédéré (le porteur, 2026-11-16 : « C'est l'utilisateur qui choisit. »). Chaque lancement envoie le flux entier
// (`streamOf` : le choix, et le défaut pour le reste), son débit « Auto » mesuré à ce moment-là
// (network-probe.js, `withNetworkBitrate`) ; le kernel le revalide.
const STREAM_KEY = (key) => `gameflex-edge-stream:${key}`;

const streamChoiceOf = (key) => {
    try {
        return JSON.parse(localStorage.getItem(STREAM_KEY(key)) || "{}") || {};
    } catch {
        return {};
    }
};

const saveStreamChoice = (key, value) => {
    try {
        localStorage.setItem(STREAM_KEY(key), JSON.stringify(value));
    } catch {
        // Sans stockage, le choix ne vaut que pour cette page.
    }
};

// L'échelle unique du flux (StreamSettings.jsx de la webapp), et ses défauts quand rien n'est choisi : 540p à 30, le
// débit « Auto », pour TOUTES les consoles — les jeux comme le Remote Play, d'ici comme d'un serveur fédéré (le
// porteur, 2026-11-16 : « La valeur par défaut ça toujours été 540p@30 pour tout le stream settings de toutes les
// consoles »). Les deux kernels ont le même (`consoles.DefaultStreamResolution`).
const STREAM_OPTIONS = {
    resolution: ["360p", "540p", "720p", "1080p"],
    frameRate: ["30", "60"],
    bitrate: ["auto", "2000", "4000", "8000", "15000"],
};
const STREAM_DEFAULTS = { resolution: "540p", frameRate: "30", bitrate: "auto" };
const streamLabel = (name, value) => (name === "frameRate" ? `${value} FPS` : name === "bitrate" ? (value === "auto" ? "Auto" : `${Number(value) / 1000} Mbps`) : value);

// Le flux d'une console dans ce navigateur : son choix, et le défaut pour ce qu'il n'a pas touché — ce que sa tuile
// résume et ce que son lancement envoie.
export const streamOf = (key) => ({ ...STREAM_DEFAULTS, ...streamChoiceOf(key) });

// Le résumé d'un flux, sur sa tuile : « 540p · 30 FPS · Auto ».
export const streamSummaryOf = (stream) => Object.keys(STREAM_OPTIONS).map((name) => streamLabel(name, stream[name])).join(" · ");

// LES CHAMPS DU FLUX — la définition et la cadence en choix segmentés, le débit en sélecteur (cinq débits : au-delà de
// quatre choix, un sélecteur, Bricks.jsx). Des lignes de `Rows`, que StreamPanel complète de son bouton.
function StreamFields({ values, onChange }) {
    const { t } = useTranslation();

    return (
        <>
            {["resolution", "frameRate"].map((name) => (
                <ChoiceRow key={name} id={name} label={t(`gear.${name}`)} help={t(`gear.${name}Help`)} value={values[name]} onChange={(value) => onChange(name, value)}
                    options={STREAM_OPTIONS[name].map((value) => [value, streamLabel(name, value)])} />
            ))}
            <SelectRow id="bitrate" label={t("gear.bitrate")} help={t("gear.bitrateHelp")} value={values.bitrate} onChange={(value) => onChange("bitrate", value)}
                options={STREAM_OPTIONS.bitrate.map((value) => ({ value, label: streamLabel("bitrate", value) }))} />
        </>
    );
}

// LE PANNEAU DU FLUX, LE MÊME PARTOUT : celui de l'engrenage d'un jeu (`choiceKey`, sa console) et celui de l'engrenage
// d'une console Remote Play (son appareil, d'ici ou d'un serveur fédéré ; Platforms.jsx). Chaque choix est retenu
// aussitôt dans ce navigateur (le porteur, 2026-11-14 : « c'était automatiquement sauvegarde au changement ») ; puis
// l'auto-détection (StreamSettings.jsx de la webapp) : la latence et le débit vers ce serveur
// (engine/stream/shared/libs/network-probe.js), qui proposent une définition et une cadence. Ce qui n'est pas choisi
// montre le défaut, le même pour toutes les consoles (`streamOf`) ; `onChanged` : la tuile qui le résume se redessine.
export function StreamPanel({ choiceKey, onChanged, onClose }) {
    const { t } = useTranslation();
    const [, redraw] = useState(0);
    const [probing, setProbing] = useState(false);
    const stream = streamOf(choiceKey);
    const keep = (change) => {
        saveStreamChoice(choiceKey, { ...streamChoiceOf(choiceKey), ...change });
        redraw((value) => value + 1);
        onChanged();
    };
    const autoDetect = async () => {
        setProbing(true);

        try {
            const { suggestion } = await probeNetworkAndSuggestPreset();

            if (suggestion) {
                keep({ resolution: suggestion.resolution, frameRate: suggestion.frameRate });
                toast(t("gear.probeApplied", { resolution: suggestion.resolution, frameRate: suggestion.frameRate }));
            } else {
                toast(t("gear.probeUnreachable"));
            }
        } catch {
            toast(t("gear.probeFailed"));
        }
        setProbing(false);
    };

    return (
        <SidePanel label={t("gear.stream")} firstKey={`resolution-${stream.resolution}`} onClose={onClose}>
            <Section title={t("gear.stream")} help={t("gear.streamHelp")}>
                <Rows>
                    <StreamFields values={stream} onChange={(name, value) => keep({ [name]: value })} />
                    {/* L'auto-détection règle les champs de ce groupe : son bouton est dans l'encadré (le porteur,
                        2026-11-13). Un geste, rien à comparer : `isReplaced` tel quel. */}
                    <SubmitRow>
                        <FormButton id="gear-probe" type="button" kind="other" isReplaced pending={probing} label={t("gear.autoDetect")} onClick={autoDetect} />
                    </SubmitRow>
                </Rows>
                <Note>{t("gear.autoDetectHelp")}</Note>
            </Section>
        </SidePanel>
    );
}

// LES CŒURS ET LES BIOS D'UNE CONSOLE, et son défaut (l'émulateur en service, le BIOS par défaut — chantier 53), lus
// une fois pour la page.
const consoleFacts = new Map();

export function useConsoleFacts(consoleId) {
    const [facts, setFacts] = useState(() => consoleFacts.get(consoleId) || null);

    useEffect(() => {
        if (consoleFacts.has(consoleId)) {
            setFacts(consoleFacts.get(consoleId));

            return undefined;
        }
        let cancelled = false;
        Promise.all([listCores(consoleId), listBios(consoleId), consoleDefaults(consoleId)]).then(([cores, bios, defaults]) => {
            const found = {
                cores: cores.ok ? cores.data || [] : [],
                bios: bios.ok ? (bios.data || []).map((entry) => (typeof entry === "string" ? entry : entry.name)).filter(Boolean) : [],
                defaults: defaults.ok ? defaults.data?.inUse || {} : {},
            };
            consoleFacts.set(consoleId, found);

            if (!cancelled) {
                setFacts(found);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    return facts;
}

// LES SAUVEGARDES DU COMPTE : les états de tout le rétro et les jeux des consoles
// récentes qui en ont une — deux lectures pour la bibliothèque. `null` tant qu'on ne sait pas.
export function useUserSaves() {
    const [saves, setSaves] = useState(null);

    useEffect(() => {
        let cancelled = false;
        Promise.all([listUserStates(), listStreamexSaves()]).then(([states, streamex]) => {
            if (!cancelled) {
                setSaves({ states: states.ok ? states.data || [] : [], streamex: streamex.ok ? streamex.data?.games || [] : [] });
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    return saves;
}

// La SRAM d'un disque (sa carte mémoire, sa cartouche), pour ce compte : `undefined` tant qu'on ne sait pas.
const sramCache = new Map();

export function useSaveRAM(consoleId, reference, enabled) {
    const key = `${consoleId}|${reference}`;
    const [, redraw] = useState(0);

    useEffect(() => {
        if (!enabled || !reference || sramCache.has(key)) {
            return undefined;
        }
        let cancelled = false;
        sramCache.set(key, undefined);
        saveRAMOf(consoleId, reference).then((answer) => {
            sramCache.set(key, answer.ok ? answer.data : null);

            if (!cancelled) {
                redraw((value) => value + 1);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [key, enabled, consoleId, reference]);

    return sramCache.get(key);
}

// Les cœurs qui peuvent tourner ici, et le choisi : celui de la fiche (pour ce lancement, rien n'est enregistré), sinon
// l'émulateur par défaut de la console (le porteur, 2026-11-13 : « dans la fiche de jeu avant de lancer le jeu on peut
// toujours choisir quel émulateur on veut »), sinon le premier installé, sinon le premier.
export const coreChoice = (console, facts, game) => {
    const names = console.cores || [];
    const installed = (name) => !facts?.cores?.length || facts.cores.some((core) => core.installed && core.name === name);
    const chosen = choiceOf(game.key).core;

    return {
        names,
        installed,
        selected: names.includes(chosen) ? chosen
            : names.includes(facts?.defaults?.core) && installed(facts.defaults.core) ? facts.defaults.core
                : names.find(installed) || names[0] || "",
    };
};

// LES BIOS d'une console : ceux qu'elle réclame, ceux qu'elle accepte, puis ceux
// du disque (comme `biosOptions` dans la webapp) ; « Par défaut » en tête.
export const biosChoice = (console, facts, game) => {
    const present = new Set((facts?.bios || []).map((name) => name.toLowerCase()));
    const names = [];
    const seen = new Set();

    for (const name of [...(console.bios || []), ...(console.biosOptional || []), ...(facts?.bios || [])]) {
        if (!seen.has(name.toLowerCase())) {
            seen.add(name.toLowerCase());
            names.push(name);
        }
    }
    const required = console.bios || [];

    return {
        names,
        isPresent: (name) => present.has(name.toLowerCase()),
        missing: required.length > 0 && facts !== null && !required.some((name) => present.has(name.toLowerCase())),
        selected: choiceOf(game.key).bios || "",
        // « Par défaut » : le BIOS par défaut de la console, quand l'administrateur en a choisi un (chantier 53).
        fallback: facts?.defaults?.bios || "",
    };
};

// LE NOM D'UN EMPLACEMENT DE SAUVEGARDE, le même partout (cette fiche, le menu de la partie, Mes sauvegardes) :
// le premier est celui qu'écrit « sauvegarder en partant ».
export const slotName = (slot, t) => (slot === 0 ? t("gear.quitSlot") : t("gear.slot", { slot }));

// Une vignette d'état, chargée avec le jeton (la route le demande) ; d'une copie (`source`), ou
// chez un serveur fédéré (`host`, sous notre compte de là-bas).
export function StateThumbnail({ consoleId, reference, slot, source = "", host = null, version = "" }) {
    const [url, setUrl] = useState(null);

    useEffect(() => {
        let cancelled = false;
        let made = null;
        const token = host ? host.token : readToken();
        const path = `${stateThumbnailPath(consoleId, reference, slot, source)}${version ? `&v=${encodeURIComponent(version)}` : ""}`;
        fetch(host ? `${host.origin}${path}` : path, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
            .then((response) => (response.ok ? response.blob() : null))
            .then((blob) => {
                if (!cancelled && blob && blob.type.startsWith("image/")) {
                    made = URL.createObjectURL(blob);
                    setUrl(made);
                }
            })
            .catch(() => {});

        return () => {
            cancelled = true;

            if (made) {
                URL.revokeObjectURL(made);
            }
        };
    }, [consoleId, reference, slot, source, host, version]);

    return (
        <span className="thumb ratio ratio-4x3 rounded overflow-hidden bg-body-secondary flex-shrink-0">
            {url ? <img className="object-fit-cover" src={url} alt="" /> : <span className="d-flex align-items-center justify-content-center text-body-tertiary"><i className="bi bi-image"></i></span>}
        </span>
    );
}

// Le nom d'un serveur fédéré, tel que l'annuaire le dit (son nom, sinon son adresse) — celui de PeerAccount.
const peerName = (servers, serverId) => {
    const server = (servers || []).find((entry) => String(entry.id) === String(serverId));

    return server ? server.hostname || server.link : "";
};

const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

// « INFORMATIONS ET RÉGLAGES » — l'engrenage.
// LE COMPTE CHEZ LE SERVEUR FÉDÉRÉ D'UN JEU (chantier 20) : on y joue sous son compte de là-bas. Il SE GÈRE DANS RÉGLAGES ›
// PROFIL › FÉDÉRATION (le porteur, 2026-11-16 : « Enleve le lien "oublier mon jeton" car cela ce gére dans
// settings/profile/fédération. Et non pas ici. ») : l'engrenage dit sous quel compte on y joue — sans lien (le porteur,
// 2026-11-16 : « On retire quand on est déjà connecté sur le serveur fédéré. Si on ne l'est pas alors on affiche le
// lien. »). Sans compte, ou avec un jeton refusé, la ligne mène à la page de ce compte, où l'on colle son invitation ou son
// jeton ; une autre raison de ne pas y être se dit (FederatedAccount.jsx). Chacun de ces blocs est son propre groupe,
// titre compris : rien autour, sinon deux intitulés se suivent.
function PeerAccount({ serverId }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { servers } = useData();
    const server = (servers || []).map((entry) => ({ ...entry, name: entry.hostname || entry.link })).find((entry) => String(entry.id) === String(serverId));
    const { session, code, loading } = useFederationSession(server ? server.id : null);

    if (!server) {
        return null;
    }
    if (session) {
        return <Section title={t("federation.playingAtTitle", { server: server.name })} help={t("federation.playingAtHelp", { name: session.displayName || session.username, server: server.name })} />;
    }
    if (loading) {
        return <Section title={t("gear.peerAccount")}><Note className="mt-0">{t("settings.loading")}</Note></Section>;
    }
    if (code === "noFederatedAccount" || code === "accountRefused") {
        return (
            <Section title={t("gear.peerAccount")} help={t(`federation.errors.${code}`, { server: server.name })}>
                <Rows>
                    <NavRow id="g-peer-account" icon="bi-person-plus" tone="green" label={t("gear.peerAccountManage")} help={t("gear.peerAccountManageHelp")}
                        onOpen={() => navigate(federatedAccountPath(server.id))} />
                </Rows>
            </Section>
        );
    }

    return <FederatedRefusal server={server} code={code} />;
}

// L'ENGRENAGE EN DIRECTION C (validée par le porteur le 2026-11-13 ; docs/reports/2026-11-13-launcher-modeles/), tel
// que le porteur l'a revu le 2026-11-14 (chantier 53, P12) :
//   - EN TÊTE, LE VISUEL DU JEU (« en premier on va rajouter le background et par dessus le logo/icône du jeu au centre
//     de celui-ci ») : GameBanner, HeroArt.jsx ;
//   - ce qui change ce que « Jouer » lancera, en DEUX tuiles côte à côte (« regrouper "Disc", "Emulator" et "BIOS" dans un
//     seul formulaire. Comme ça on aura préréglage et flux côte à côte. ») : « Préréglage » — l'émulateur, et le disque
//     choisi quand il y en a plusieurs — ouvre UN formulaire, le disque, l'émulateur et le BIOS, ceux qui valent pour la
//     console ; « Flux » ouvre le sien. Chacun s'applique à mesure (rien ne s'enregistre : un choix de lancement) ;
//   - puis les sauvegardes, puis les informations, qu'on consulte, en bas : « Statut » à la place de « Émulateur »
//     (« dans section "Information" remplace la ligne "Emulator" par "Status" ou "State" du jeu »), et « Joueurs » quand
//     une source le sait (« si possible, si on peut avoir l'info, rajouter le nombre de joueurs possible »).
// LA MACHINE QUI FERA TOURNER UNE CONSOLE DE BUREAU, en informations, dans sa section « Serveur » (le porteur, 2026-11-14 :
// « Le message du serveur doit être comme information. Tu créer une section serveur ») — plus en alerte en tête du
// panneau : cet appareil (un disque interne) ou le serveur fédéré du jeu, son GPU, ses cœurs, sa mémoire, tels qu'il les
// déclare (`resources` des capacités, ou de `GET /servers/{id}`) ; l'aide dit si l'émulateur dessine sur ce GPU ou s'il
// tournerait sur le seul processeur. Rien tant qu'on ne les connaît pas. L'avertissement « expérimental » est parti :
// la fiche le dit déjà par sa pastille (le porteur : « Le message expérimental on l'a déjà avec les pastilles »).
// `peer` : les ressources du serveur fédéré du jeu, null tant qu'on ne les a pas.
export function serverInfo(console, rom, capabilities, peer, t) {
    if (console?.method !== "streamex") {
        return null;
    }
    const here = !rom?.peer;
    const resources = here ? capabilities?.resources : peer?.resources;

    if (!resources) {
        return null;
    }
    const phone = here && capabilities?.platform === "android";
    const rows = [
        ["machine", "bi-hdd-rack", t("gear.server.machine"), phone ? t("gear.server.thisPhone") : here ? t("gear.server.thisServer") : peer?.hostname || t("gear.server.thatServer")],
        ["gpu", "bi-gpu-card", t("gear.server.gpu"), resources.gpu || t("gear.server.noGpu")],
        // LA MÉMOIRE VIDÉO, sa ligne à elle (le porteur, 2026-11-14 : « Dans serveur, si tu peux mettre le VRAM aussi »).
        resources.gpu && resources.vramBytes > 0 && ["vram", "bi-display", t("gear.server.vram"), formatSize(resources.vramBytes)],
        // LE PROCESSEUR PAR SON NOM, puis ses cœurs (le porteur, 2026-11-14 : « tu peux aussi rajouter le nom du processus ? »).
        (resources.cpu || resources.cpuThreads > 0) && ["cpu", "bi-cpu", t("gear.server.cpu"),
            [resources.cpu, resources.cpuThreads > 0 ? t("gear.server.threads", { count: resources.cpuThreads }) : ""].filter(Boolean).join(" · ")],
        resources.memoryBytes > 0 && ["memory", "bi-memory", t("gear.server.memory"), formatSize(resources.memoryBytes)],
    ].filter(Boolean);

    return { help: t(resources.gpu ? "gear.server.rendersOnGpu" : phone ? "gear.server.phoneNoGpu" : "gear.server.noGpuHelp"), rows };
}

export function GearPanel({ console, game, rom, metadata, facts, playable, states, sram, streamexSave, disc, discsLocked, onDisc, onChanged, onClose }) {
    const { t, i18n } = useTranslation();
    const { capabilities, servers } = useData();
    const { user, isAdmin } = useAuth();
    // Les ressources du serveur fédéré d'un jeu de console de bureau, lues à l'ouverture (serverInfo).
    const [peer, setPeer] = useState(null);

    useEffect(() => {
        setPeer(null);

        if (!rom?.peer || console?.method !== "streamex") {
            return undefined;
        }
        let cancelled = false;
        fetchServer(rom.peer).then((answer) => !cancelled && answer.ok && setPeer(answer.data || null));

        return () => {
            cancelled = true;
        };
    }, [rom?.peer, console?.method]);
    const [, redraw] = useState(0);
    const favorites = useFavorites();
    const [marking, setMarking] = useState(false);
    // Le sous-panneau ouvert par une tuile : preset, stream.
    const [sub, setSub] = useState(null);
    const known = summaryOf(metadata);
    const sources = sourcesOf(metadata);
    const cores = coreChoice(console, facts, game);
    const bios = biosChoice(console, facts, game);
    const stream = streamOf(console.id);
    // Le statut du jeu, celui de la liste et de la fiche ; sans statut, il n'a pas été essayé.
    const status = statusOf(rom) || "untested";
    // Pourquoi la partie ne partirait pas d'ici (les cas de la webapp que le launcher sait voir).
    let noLaunch = "";
    const refused = hostRefusal(console, rom, capabilities, isAdmin);
    const server = serverInfo(console, rom, capabilities, peer, t);

    if (!playable || !allows(user?.consoles, console)) {
        // Le compte ne lance pas cette console, ou l'hôte la refuse (un téléphone : hostRefusal).
        noLaunch = allows(user?.consoles, console) && refused ? refused : "consoleNotGranted";
    } else if (!rom.peer && console.disabled) {
        // Coupée pour tout le serveur (chantier 66, Réglages › Consoles).
        noLaunch = "consoleDisabled";
    } else if (!rom.peer && facts && cores.selected && !cores.installed(cores.selected)) {
        noLaunch = "coreMissing";
    } else if (!rom.peer && bios.missing) {
        noLaunch = "biosMissing";
    }
    const details = [
        ["developer", "bi-building", t("game.developer"), known.developer],
        // La provenance d'un jeu d'un serveur fédéré : le NOM de ce serveur (le porteur, 2026-11-17 : « Pourquoi la provenance
        // dans GamesSettings ne mets pas le nom du serveur fédéré ? »), comme le dit l'annuaire ; « Un serveur fédéré » le temps
        // qu'il se lise.
        ["origin", "bi-hdd", t("game.origin"), rom.peer ? peerName(servers, rom.peer) || t("game.originPeer") : rom.disk ? t("games.onDisk", { name: rom.disk }) : t("game.originLibrary")],
        ["engine", "bi-boxes", t("gear.engine"), rom.engine ? t(`gear.engines.${rom.engine}`, { defaultValue: rom.engine }) : ""],
        ["size", "bi-file-earmark-zip", t("game.size"), formatSize(rom.sizeBytes)],
        ["files", "bi-files", t("game.files"), (rom.files || []).length ? t("game.fileCount", { count: rom.files.length }) : ""],
    ].filter(([, , , value]) => value);
    const change = (update) => {
        setChoice(game.key, update);
        redraw((value) => value + 1);
        onChanged();
    };
    // AJOUTER À MES FAVORIS, OU EN RETIRER (le porteur, 2026-11-16 : « dans les fiches game info, remplace le bouton
    // download this rom par add to my favorite ») : le jeu par son premier disque — celui que la fiche montre, quel que
    // soit le disque choisi —, et le serveur d'un jeu fédéré. Le favori suit le compte (libs/favorites.js) ; le
    // téléchargement de la ROM est parti dans le Catalogue des Réglages, par le navigateur.
    const first = game.discs[0];
    const favorite = isFavorite(favorites, console.id, first.reference);
    const toggleFavorite = async () => {
        setMarking(true);
        const answer = await setFavorite({ consoleId: console.id, reference: first.reference, serverId: first.peer ?? null, title: first.title }, !favorite);
        setMarking(false);
        toast(answer.ok ? t(favorite ? "favorites.removed" : "favorites.added", { name: splitTitle(first.title).main }) : answer.detail || t("favorites.failed"));
    };
    // Un sous-panneau se referme : la lumière revient sur sa tuile.
    const closeSub = () => {
        const key = `gt-${sub}`;
        setSub(null);
        setTimeout(() => focusElement(document.querySelector(`.npanel [data-key="${CSS.escape(key)}"]`), true), 0);
    };
    // LE PRÉRÉGLAGE, CE QUE « JOUER » LANCERA : ses choix, ceux qui valent pour ce jeu — le disque d'un jeu de plusieurs
    // disques (grisé quand une autre partie tourne), l'émulateur, NOS BIOS pour une partie d'ici (chez un ami, ce sont
    // les siens).
    const pickDisc = game.discs.length > 1;
    const pickBios = !rom.peer && bios.names.length > 0;
    const discLabel = pickDisc ? t("library.discOf", { name: discName(game.discs[disc], disc), count: game.discs.length }) : "";
    const tiles = [
        // Sa tuile dit l'essentiel : l'émulateur, et le disque quand il y en a plusieurs.
        { id: "gt-preset", icon: "bi-sliders", tone: "indigo", label: t("gear.preset"), value: [cores.selected || t("gear.noEmulator"), discLabel].filter(Boolean).join(" · "), onClick: () => setSub("preset") },
        { id: "gt-stream", icon: "bi-broadcast", tone: "sky", label: t("gear.streamTile"), value: streamSummaryOf(stream), onClick: () => setSub("stream") },
    ];
    const presetFirst = pickDisc ? "g-disc" : cores.names.length ? "g-core" : pickBios ? "g-bios" : "p-close";

    return (
        <>
            {/* LES PASTILLES SOUS LE TITRE (le porteur, 2026-11-14 : « met le pastille "expimental" en sous-titre dans le
                titre du offcanvas », puis « rajoutes le pastille status du jeu en sous titres. Le status en premier et
                ensuite l'expérimental ») : celles de la fiche — le statut du jeu, de la couleur de son badge, puis
                « Expérimental » pour une console encore expérimentale. */}
            <SidePanel label={splitTitle(rom.title).main} className="gear" firstKey="p-close" onClose={onClose}
                badges={(
                    <>
                        <span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>
                        {console.experimental && <span className="badge rounded-pill text-bg-primary">{t("console.experimental")}</span>}
                    </>
                )}>
                <GameBanner consoleId={console.id} reference={game.discs[0].reference} source={romSource(game.discs[0])} title={splitTitle(game.discs[0].title).main} />
                <QuickTiles items={tiles} />

                <Section title={t("gear.saves")} help={t(console.method === "retrogaming" ? "gear.savesHelp" : "gear.streamexSavesHelp")}>
                    {console.method === "retrogaming" ? (
                        states.length === 0 && !sram?.exists ? <EmptyState icon="bi-sd-card" text={t("gear.noSaves")} /> : (
                            <Rows>
                                {sram?.exists && <InfoRow icon="bi-sd-card" tone="green" label={t("gear.sramLabel")} value={dateTime(sram.timestamp, i18n.language)} />}
                                {states.map((state) => (
                                    <InfoRow key={state.slot} lead={<StateThumbnail consoleId={console.id} reference={rom.reference} slot={state.slot} />}
                                        label={slotName(state.slot, t)} value={dateTime(state.timestamp, i18n.language)} />
                                ))}
                            </Rows>
                        )
                    ) : streamexSave
                        ? <Rows><InfoRow icon="bi-sd-card" tone="green" label={t("gear.streamexSaveLabel")} value={dateTime(streamexSave.updatedAt, i18n.language)} /></Rows>
                        : <EmptyState icon="bi-sd-card" text={t("gear.noSaves")} />}
                </Section>

                <Section title={t("gear.informations")}>
                    <Rows>
                        {/* POURQUOI LE JEU NE PARTIRAIT PAS D'ICI, la première des informations — plus une alerte en tête du
                            panneau (le porteur, 2026-11-16 : « retirer les alerts » ; un renseignement est une ligne). */}
                        {noLaunch && <InfoRow icon="bi-slash-circle" tone="rose" label={t("gear.launch")} value={t(`noLaunch.${noLaunch}`)} />}
                        {details.map(([key, icon, label, value]) => <InfoRow key={key} icon={icon} label={label} value={value} />)}
                        {/* Le statut, de la couleur de son badge (celle de la liste et de la fiche). */}
                        <InfoRow icon="bi-patch-check" label={t("games.status")} value={<span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>} />
                        {/* Les joueurs, seulement quand une source le sait (libretro-database ; rien n'est déduit de la console). */}
                        {known.players > 0 && <InfoRow icon="bi-people" label={t("game.players")} value={String(known.players)} />}
                    </Rows>
                    {sources.length > 0 && (
                        <Note>
                            <b className="text-body">{t("game.sources")}</b>{" "}
                            {sources.map((source, index) => (
                                <React.Fragment key={source.label}>
                                    {index > 0 && " · "}
                                    {source.href ? <a href={source.href} target="_blank" rel="noopener noreferrer" data-nav>{source.label}</a> : source.label}
                                </React.Fragment>
                            ))}
                        </Note>
                    )}
                </Section>
                {server && (
                    <Section title={t("gear.server.title")} help={server.help}>
                        <Rows>{server.rows.map(([key, icon, label, value]) => <InfoRow key={key} icon={icon} label={label} value={value} />)}</Rows>
                    </Section>
                )}
                {/* Le compte chez le serveur fédéré du jeu : un renseignement et son lien, avant « Plus » (l'ordre d'une page). */}
                {rom.peer && <PeerAccount serverId={rom.peer} />}
                {/* Un geste orphelin, donc une ligne du groupe « Plus » (le porteur, 2026-11-13). */}
                <Section title={t("more")}>
                    <Rows>
                        <NavRow id="g-favorite" icon={favorite ? "bi-star-fill" : "bi-star"} tone="amber" label={t(favorite ? "favorites.remove" : "favorites.add")}
                            help={t("favorites.help")} pending={marking || !favorites.loaded} opens="action" onOpen={toggleFavorite} />
                    </Rows>
                </Section>
            </SidePanel>

            {sub === "preset" && (
                <SidePanel label={t("gear.preset")} firstKey={presetFirst} onClose={closeSub}>
                    <Section title={t("gear.launchTitle")}>
                        {!pickDisc && !cores.names.length && !pickBios ? <EmptyState icon="bi-cpu" text={t("gear.noEmulator")} /> : (
                            <Rows>
                                {pickDisc && (
                                    <SelectRow id="g-disc" label={t("gear.disc")} help={t("gear.discHelp")} value={String(disc)} disabled={discsLocked} onChange={(value) => onDisc(Number(value))}
                                        options={game.discs.map((entry, index) => ({ value: String(index), label: t("library.disc", { name: discName(entry, index) }) }))} />
                                )}
                                {cores.names.length > 0 && (
                                    <SelectRow id="g-core" label={t("gear.emulator")} help={t("gear.emulatorHelp")} value={cores.selected} onChange={(value) => change({ core: value })}
                                        options={cores.names.map((name) => ({ value: name, label: name, attributes: { disabled: !rom.peer && !cores.installed(name) } }))} />
                                )}
                                {/* Les BIOS absents restent listés, grisés, comme dans la webapp. */}
                                {pickBios && (
                                    <SelectRow id="g-bios" label={t("gear.bios")} help={t("gear.biosHelp")} value={bios.selected} onChange={(value) => change({ bios: value })}
                                        options={[{ value: "", label: bios.fallback ? t("gear.biosDefaultNamed", { name: bios.fallback }) : t("gear.biosDefault") }, ...bios.names.map((name) => ({ value: name, label: name, attributes: { disabled: !bios.isPresent(name) } }))]} />
                                )}
                            </Rows>
                        )}
                    </Section>
                </SidePanel>
            )}
            {sub === "stream" && (
                <StreamPanel choiceKey={console.id} onClose={closeSub} onChanged={() => {
                    redraw((value) => value + 1);
                    onChanged();
                }} />
            )}
        </>
    );
}

// « CONTINUER » : l'état à reprendre — d'abord « Démarrer le jeu » (la carte mémoire
// se charge d'elle-même), puis chaque état, comme la webapp les montre (son
// emplacement, sa date, sa vignette).
export function ContinuePanel({ console, rom, states, onChoose, onClose }) {
    const { t, i18n } = useTranslation();

    return (
        <SidePanel label={t("continue.title")} className="continue" firstKey="cs-start" onClose={onClose}>
            <Section title={t("continue.whereTitle")}>
            <Rows>
                <NavRow id="cs-start" label={t("continue.start")} help={t("continue.startHelp")} opens="action" onOpen={() => onChoose(null)}
                    lead={<span className="thumb ratio ratio-4x3 rounded overflow-hidden bg-body-secondary flex-shrink-0"><span className="d-flex align-items-center justify-content-center"><i className="bi bi-play-fill"></i></span></span>} />
                {states.map((state) => (
                    <NavRow key={state.slot} id={`cs-${state.slot}`} label={slotName(state.slot, t)} help={dateTime(state.timestamp, i18n.language)} opens="action" onOpen={() => onChoose(state.slot)}
                        lead={<StateThumbnail consoleId={console.id} reference={rom.reference} slot={state.slot} />} />
                ))}
            </Rows>
            </Section>
        </SidePanel>
    );
}
