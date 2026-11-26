// Route : /settings/profile/saves (la page des sources, `Saves`) ; ce fichier tient aussi ce que les pages de
// settings/profile/saves/ partagent. MES SAUVEGARDES, PAR SOURCE (profile/settings/library de la webapp : SavedStates, StreamexSaves,
// SaveActions ; chantier 38). ON CHOISIT D'ABORD LA SOURCE, en sections, comme les jeux d'une console (Réglages ›
// Consoles › Jeux, Games.jsx ; le porteur, 2026-11-16 : « Mes sauvegardes, comme pour jeux dans consoles, on choisit
// d'abord la source avec des sections ») :
//
//   « Sur ce serveur » — cette machine (le PRINCIPAL, ce que les parties jouent : Backup, Envoyer vers…), les backups,
//     et les COPIES qu'un serveur fédéré nous a envoyées ou qu'on y a téléchargées (Restaurer, Supprimer) ;
//   « Serveurs fédérés » — chacun : SES sauvegardes, sous notre compte de là-bas (Télécharger).
//
// Un disque interne n'a pas de sauvegardes : elles restent par compte, dans le dossier du serveur, quel que soit le
// disque d'un jeu (chantier 23). Il n'a donc pas de section ici.
//
// Rien de ce qui voyage n'écrase une sauvegarde : un envoi arrive dans une copie d'en face, un
// téléchargement dans une copie d'ici ; seul Restaurer écrit dans le principal, après avoir mis
// le sien dans les backups. Le rétro se range par jeu (ses emplacements), les consoles récentes
// par console (un dossier par jeu). Un geste qui remplace ou retire quelque chose se confirme dans la fenêtre
// de Skeleton ; supprimer est en bas de sa page (« Danger »). Les pages se composent des briques de la direction C.
//
// UN BACKUP A SA NOTE (le porteur, 2026-11-16 : « Dans backup de mes sauvegardes, avoir la possibilité de rajouter une
// note textuelle dans un fichier note.json. Cela permettra de se rappeler pourquoi et quand on a backup la sauvegarde. »)
// — un note.json à côté de lui (saves/notes.go : la note, sa date, le compte). Elle s'écrit au moment du Backup (son
// formulaire, sous « Plus »), se lit dans la liste des backups et sur la page d'un backup (« La note »), et se corrige
// ou se retire par « Écrire une note » / « Modifier la note » de son « Plus », qui ouvre sa page. Un backup qui en
// remplace un autre emporte la note de l'ancien.
//
// LES PAGES (des routes, le porteur, 2026-11-13). LA SOURCE EST UN SEGMENT DE L'ADRESSE, comme dans Games.jsx : `here`
// (cette machine), `copy-<nom>` (une copie ; `copy-local`, les backups), `server-<id>` (un serveur fédéré) :
//
//   /settings/profile/saves                               les sources (une page enfant de Profil)
//   …/<source>                                            ses sauvegardes, cherchées et filtrées par console
//   …/<source>/game/<console>/<jeu>                       un jeu du rétro : ses emplacements
//   …/<source>/game/<console>/<jeu>/slot/<n>, …/all       un emplacement, ou tous : leurs gestes
//   …/<source>/console/<console>                          une console récente : ses jeux
//   …/<source>/console/<console>/game/<jeu>, …/all        la sauvegarde d'un jeu, ou toutes : leurs gestes
//   …/copy-local/…/slot/<n>/note, …/game/<jeu>/note       la note d'un backup
//
// La liste garde sa recherche, ses consoles et sa page dans l'adresse (`q`, `noconsole`, `page`).
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : la liste, les copies (`?copiesPage=`), les serveurs (`?serversPage=`), les emplacements d'un jeu
// (`?slotsPage=`), les jeux d'une console récente (`?gamesPage=`) — dans l'adresse, la lumière y revient ; et ce qu'un
// geste emportera (tous les emplacements, toute la console), la page tenue par la liste.

import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { DownloadRow } from "../../bricks/DownloadRow.jsx";
import { switchGroup } from "../../bricks/FilterGroups.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../bricks/Listing.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { hiddenOf, toggleHidden } from "../../globals/filters.js";
import { backupStates, backupStreamexSaves, deleteSaveSlot, deleteStreamexSave, fetchStateFile, fetchStreamexSave, listRoms, listSaveNotes, listSaveSlots, listSaveSources, listStreamexSaves, listUserStates, restoreStates, restoreStreamexSaves, uploadStateFile, uploadStreamexSave } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { peerHost, useFederationSession } from "../../libs/federation.js";
import { counted, dateTime, fact, formatSize, originOf } from "../../libs/format.js";
import { gameScreenPath } from "../../libs/tabs.js";
import { FederatedRefusal, FederatedTokenForm } from "../../screens/access/FederatedAccount.jsx";
import { slotName, StateThumbnail } from "../../screens/library/GameSettings.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Form, FormButton } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, PROFILE, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { CONSOLE_ICON } from "../../globals/icons.js";

export const HERE = { host: null, source: "" };
// Le guide des sauvegardes, en bas de chacune de leurs pages.
export const SAVES_GUIDES = [["storage", "my-saves"]];
export const BASE = `${PROFILE}/saves`;
export const PRINCIPAL = "here";

// `{kind: "principal"}`, `{kind: "copy", name}`, `{kind: "peer", id}`, ou null pour un segment qui ne désigne rien.
export const sourceOf = (slug) => {
    const match = /^(copy|server)-(.+)$/.exec(slug || "");

    if (slug === PRINCIPAL) {
        return { kind: "principal" };
    }

    return match ? (match[1] === "copy" ? { kind: "copy", name: match[2] } : { kind: "peer", id: match[2] }) : null;
};
export const sourcePath = (slug) => `${BASE}/${encodeURIComponent(slug)}`;
export const copyName = (name, t) => (name === "local" ? t("saves.backups") : name);

// LA PAGE DES SAUVEGARDES D'UN JEU, pour la fiche qui y mène (le Catalogue et Mes favoris, catalog/Games.jsx) : chez
// cette machine, ou chez un serveur fédéré (`serverId`) ; un jeu du rétro par sa référence (là où il se joue), un jeu
// d'une console récente par son identifiant (`gameId`).
export const savesPathOf = ({ serverId = null, consoleId, reference = null, gameId = null }) => {
    const base = sourcePath(serverId != null ? `server-${serverId}` : PRINCIPAL);

    return gameId != null
        ? `${base}/console/${encodeURIComponent(consoleId)}/game/${encodeURIComponent(gameId)}`
        : `${base}/game/${encodeURIComponent(consoleId)}/${encodeURIComponent(reference)}`;
};

// Une réponse `{results: [{error}]}` : combien ont échoué.
export const failuresOf = (answer) => (answer.ok ? (answer.data?.results || []).filter((result) => result.error).length : 1);


// Le nom d'un jeu d'une console récente : son titre (celui de son jeu dans la bibliothèque, que le kernel trouve par son
// identifiant, sinon celui que la sauvegarde se donne : metadata.NameSaves), sinon son identifiant ; la Xbox n'a que son
// disque.
export const streamexName = (game, t) => (game.console === "xbox" ? t("saves.xboxDisk") : game.title || game.id);

// LA LIGNE D'UNE SAUVEGARDE DIT SA CONSOLE (le porteur, 2026-11-16 : « Oui il faut les nom des jeux et la console dans
// mes sauvegardes »), comme les autres listes (« console · … », le Catalogue, Mes favoris) : le nom du registre d'abord,
// puis ce que la ligne ajoute. Définie ici, une fois, pour chaque ligne de ces pages.
export function useConsoleName() {
    const { consoles } = useData();

    return (consoleId) => consoles?.find((entry) => entry.id === consoleId)?.name || consoleId;
}

// La provenance du jeu qui nomme une sauvegarde d'une console récente, comme celle d'un jeu du Catalogue (originOf) : son
// disque interne, sinon la bibliothèque ; rien quand aucun jeu ne la nomme.
export const streamexOrigin = (game, t) => (game.reference ? originOf(game, [], t) : null);

// LES NOTES DES BACKUPS (saves/notes.go) : la plus longue, celle que le kernel revalide ; la clé d'une sauvegarde, comme
// la liste des notes la nomme ; une note dans une ligne, son début entre guillemets.
export const NOTE_MAX = 500;
export const NOTE_EXCERPT = 80;
export const slotNoteItem = (consoleId, reference, slot) => ({ kind: "states", console: consoleId, reference, slot: Number(slot) });
export const gameNoteItem = (consoleId, id) => ({ kind: "streamex", console: consoleId, id });
export const noteKey = (item) => (item.kind === "streamex" ? `streamex/${item.console}/${item.id}` : `states/${item.console}/${item.reference}/${item.slot}`);
export const isBackups = (where) => where?.kind === "copy" && where.source === "local";
export const excerpt = (text, t) => t("saves.noteQuoted", { note: text.length > NOTE_EXCERPT ? `${text.slice(0, NOTE_EXCERPT).trimEnd()}…` : text });
export const withNote = (help, note, t) => [help, note ? fact(t("saves.note"), excerpt(note.note, t)) : null];

// Les notes des backups, par sauvegarde (noteKey) — seulement quand on regarde les backups : `{}` ailleurs, null tant
// qu'elles se lisent.
export function useBackupNotes(where) {
    const backups = isBackups(where);
    const [notes, setNotes] = useState(backups ? null : {});

    useEffect(() => {
        if (!backups) {
            setNotes({});

            return undefined;
        }
        let cancelled = false;
        listSaveNotes().then((answer) => {
            if (!cancelled) {
                setNotes(Object.fromEntries((answer.ok ? answer.data?.notes || [] : []).map((note) => [noteKey(note), note])));
            }
        });

        return () => {
            cancelled = true;
        };
    }, [backups]);

    return notes;
}

// Un emplacement du rétro, de `from` vers la copie `to.source` de `to.host` : l'état, puis sa vignette.
async function copySlot({ consoleId, reference, slot, title, timestamp }, from, to, { onProgress = null, onStore = null, signal = undefined } = {}) {
    const state = await fetchStateFile(consoleId, reference, slot, from.source, from.host, false, { onProgress, signal });

    if (!state.ok) {
        return false;
    }
    onStore?.();
    const written = await uploadStateFile(consoleId, reference, slot, state.blob, { host: to.host, source: to.source, timestamp, title });

    if (!written.ok) {
        return false;
    }
    const shot = await fetchStateFile(consoleId, reference, slot, from.source, from.host, true);

    if (shot.ok) {
        await uploadStateFile(consoleId, reference, slot, shot.blob, { host: to.host, source: to.source, timestamp, screenshot: true });
    }

    return true;
}

// La sauvegarde d'un jeu d'une console récente, de `from` vers la copie `to.source`.
// Le nom de son jeu voyage avec elle (chantier 76).
async function copyStreamexGame({ console, id, title, reference, path }, from, to, { onProgress = null, onStore = null, signal = undefined } = {}) {
    const archive = await fetchStreamexSave(console, id, from.source, from.host, { onProgress, signal });

    if (!archive.ok) {
        return false;
    }
    onStore?.();

    return Boolean((await uploadStreamexSave(console, id, archive.blob, to.source, to.host, { title, reference, path })).ok);
}

// La référence d'ICI d'un jeu qu'une copie range sous la sienne : la même, puis le même titre dans
// la bibliothèque ; nos propres backups viennent d'ici (un jeu d'un disque interne compris).
async function localReference({ consoleId, reference, title }, source) {
    const found = await listRoms(consoleId, { query: title && title !== reference ? title : "" });
    const rows = found.ok ? found.data || [] : [];
    const match = rows.find((row) => row.reference === reference) || rows.find((row) => title && row.title === title);

    return match ? match.reference : source === "local" ? reference : null;
}

// Les serveurs fédérés : la route rend `hostname`, les pages lisent `name`.
export function usePeers() {
    const { servers } = useData();

    return useMemo(() => (servers || []).map((entry) => ({ ...entry, name: entry.hostname || entry.link })).sort((a, b) => a.name.localeCompare(b.name)), [servers]);
}

// L'adresse d'une page enfant d'une liste : la sienne, puis la requête de la page sans la page de la liste (`pageKey`)
// — la page de la liste reste à la liste.
export const childPath = (location, tail, pageKey) => {
    const params = new URLSearchParams(location.search);
    params.delete(pageKey);
    const query = params.toString();

    return `${location.pathname}/${tail}${query ? `?${query}` : ""}`;
};

// Les copies de ce compte, et le nom sous lequel ce serveur se présente aux autres (`self`). `loading` : la liste se
// lit encore.
export function useSaveSources() {
    const [copies, setCopies] = useState({ self: "", sources: [], loading: true });

    useEffect(() => {
        let cancelled = false;
        listSaveSources().then((answer) => {
            if (!cancelled) {
                setCopies(answer.ok ? { self: answer.data?.self || "", sources: answer.data?.sources || [], loading: false } : { self: "", sources: [], loading: false });
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    return copies;
}

// OÙ L'ON REGARDE, d'après le segment de la source (`:source`) : le principal, une copie, ou un serveur fédéré (sous
// notre compte de là-bas). `where` est nul tant que le serveur n'a pas de session ; `source`, pour un segment qui ne
// désigne rien. `levels` nomme la source au fil d'Ariane, même par un lien direct.
export function usePlace() {
    const { t } = useTranslation();
    const peers = usePeers();
    const { source: slug } = useParams();
    const source = sourceOf(slug);
    const wantsPeer = source?.kind === "peer";
    const peer = wantsPeer ? peers.find((entry) => String(entry.id) === source.id) || null : null;
    const federation = useFederationSession(peer ? peer.id : null);
    const host = useMemo(() => (peer && federation.session ? { origin: peer.link, token: federation.session.token } : null), [peer, federation.session]);
    const base = sourcePath(slug);
    let where = null;
    let name = slug || "";

    if (source?.kind === "principal") {
        where = { kind: "principal", source: "", host: null };
        name = t("saves.principal");
    } else if (source?.kind === "copy") {
        where = { kind: "copy", source: source.name, host: null };
        name = copyName(source.name, t);
    } else if (wantsPeer) {
        where = host ? { kind: "peer", source: "", host, server: peer } : null;
        name = peer?.name || t("consoles.serverNamed", { id: source.id });
    }

    return { where, source, peer, wantsPeer, federation, peers, base, name, levels: { [base]: name } };
}

// UN SERVEUR FÉDÉRÉ SANS SESSION : on l'attend (la liste des serveurs ou la session se lisent encore), ou on
// colle son jeton, ou on dit pourquoi il refuse ; avec sa session, ce qu'on y voit.
export function PeerGate({ place, children }) {
    const { t } = useTranslation();
    const { peer, federation } = place;

    if (!peer || federation.loading) {
        return <Section><Loading /></Section>;
    }

    // Chacun est un groupe à lui (FederatedAccount.jsx), posé tel quel.
    if (!federation.session) {
        return federation.code === "noFederatedAccount" || federation.code === "accountRefused"
            ? <FederatedTokenForm server={peer} refused={federation.code === "accountRefused"} />
            : <FederatedRefusal server={peer} code={federation.code} />;
    }

    // Connecté : ce qu'on y voit, sans plus. Oublier son jeton se fait dans Réglages › Profil › Fédération › Mes comptes
    // chez les serveurs fédérés, et nulle part ailleurs (le porteur, 2026-11-23 : « enlève le forget my token ici »).
    return children;
}

// Une page d'une source : rien pour un segment qui ne désigne rien, la porte d'un serveur fédéré, puis ce qu'elle montre.
export function PlaceBody({ place, children }) {
    if (!place.source) {
        return <NotFound />;
    }

    return place.wantsPeer ? <PeerGate place={place}>{place.where && children}</PeerGate> : children;
}

// LES GESTES, selon d'où l'on regarde (`where.kind`) : le principal — Backup, Envoyer vers… ; une
// copie — Restaurer (et Supprimer), et pour un backup seul, sa note (`notePath`, `note`) ; un serveur fédéré — Télécharger.
// Les mêmes pour un emplacement ou tous ceux d'un jeu (`slots`), un jeu récent ou toute sa console (`games`). Ce sont des
// LIGNES du groupe « Plus », pas des boutons (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent
// pas être des boutons mais des liens » ; 08 des modèles) ; une ligne indisponible est grisée, sa raison
// dessous. Backup ouvre la fenêtre de Skeleton — la note, facultative, puis « Faire le backup » ; Envoyer vers… le choix des serveurs (un
// interrupteur par serveur), son bouton dans son encadré : UN FORMULAIRE À LA FOIS, jamais deux boutons l'un sous
// l'autre. Restaurer remplace les sauvegardes d'ici (les siennes passent d'abord dans les backups) : il se
// confirme dans la fenêtre de Skeleton. Supprimer est seul en bas, dans « Danger ».
// LA NOTE D'UN BACKUP, dans la fenêtre de confirmation (useConfirm, `body`) : son aide, puis le champ long de Skeleton
// (Form.Longtext, par TextRow). Ce qu'on écrit va dans `holder`, que « Faire le backup » lit.
export function BackupNote({ id, holder }) {
    const { t } = useTranslation();
    const [written, setWritten] = useState("");

    return (
        <div className="text-start">
            <p className="small text-body-secondary">{t("saves.backupFormHelp")}</p>
            <Rows>
                <TextRow id={id} kind="longtext" label={t("saves.note")} help={t("saves.noteHelp")} value={written}
                    onChange={(value) => {
                        const next = String(value || "").slice(0, NOTE_MAX);
                        setWritten(next);
                        holder.current = next;
                    }} props={{ maxLength: NOTE_MAX, rows: 3 }} />
            </Rows>
        </div>
    );
}

// `game` : le jeu que la sauvegarde nomme (`{ consoleId, reference }`) — « Voir sa fiche dans la bibliothèque » en dernier
// dans « Plus », comme la page d'un jeu des Réglages ; grisée, avec sa raison, quand aucun jeu d'ici ne la porte (le
// porteur, 2026-11-23 : « certains jeux n'ont pas de titre, il faudra rajouter le lien vers la fiche du jeu »).
export function SaveActions({ id, where, peers, selfName, slots = null, games = null, label, canDelete, notePath = null, note = null, game = null, onDone, onRemoved = null }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const navigate = useNavigate();
    const { consoles } = useData();
    const gameConsole = game ? (consoles || []).find((entry) => entry.id === game.consoleId) : null;
    const confirm = useConfirm();
    const { open } = useSettingsNavigation();
    const [opened, setOpened] = useState(null);
    const [chosen, setChosen] = useState([]);
    // LE GESTE EN COURS (« backup », « send », « restore », « download », « delete ») : sa ligne prend la roue (NavRow
    // `pending`), les autres se grisent — jamais une note sous le groupe (le porteur, 2026-11-23 : « le texte copying…
    // avec le spinner est hors section »).
    const [running, setRunning] = useState(null);
    const busy = running !== null;
    // Le téléchargement en cours depuis un serveur fédéré (`{ state, phase, received, total }`), et de quoi l'arrêter.
    const [transfer, setTransfer] = useState(null);
    const stopTransfer = useRef(null);
    const items = slots || games || [];
    const pick = (serverId, on) => setChosen((previous) => (on ? [...previous, serverId] : previous.filter((other) => other !== serverId)));
    const toggle = (form) => setOpened((current) => (current === form ? null : form));
    // Les serveurs où l'envoyer, qui peuvent être nombreux : un listing qui se cherche et se feuillette.
    const targets = useListSearch(peers, (peer) => peer.name);

    // `viaForm` : le geste de son FormButton (« Envoyer maintenant »), qui répond par la fenêtre de Skeleton (useFeedback).
    const finish = (failed, message = null, done = "saves.syncDone", viaForm = false) => {
        (viaForm ? (failed ? feedback.failed : feedback.success) : toast)(message || t(failed ? "saves.syncFailed" : done));
        onDone?.();
    };

    const run = async (work, gesture) => {
        setRunning(gesture);

        try {
            await work();
        } catch {
            finish(1);
        } finally {
            setRunning(null);
        }
    };

    const copy = (item, from, to, options) => (slots ? copySlot(item, from, to, options) : copyStreamexGame(item, from, to, options));

    // BACKUP : dans `local`, à la place du précédent, avec la note écrite (facultative). LA NOTE S'ÉCRIT DANS LA FENÊTRE DE
    // SKELETON (le porteur, 2026-11-16 : « mets le champ note dans un modal skeleton au lieu de l'afficher plus bas. Ça
    // porte confusion comme ça ») : « Faire le backup » la confirme.
    const backup = (written) => run(async () => {
        const answer = slots
            ? await backupStates(slots.map(({ consoleId, reference, slot }) => ({ consoleId, reference, slot })), written)
            : await backupStreamexSaves(games[0].console, games.map((game) => game.id), written);
        finish(failuresOf(answer), null, "saves.backedUp");
    }, "backup");
    const askBackup = () => {
        const holder = { current: "" };
        confirm({
            title: t("saves.backupTitle"),
            body: <BackupNote id={`${id}-backup-note`} holder={holder} />,
            confirmLabel: t("saves.backupNow"),
            onConfirm: () => backup(holder.current.trim()),
        });
    };

    // ENVOYER : dans la copie qui porte notre nom, chez chaque serveur choisi, sous le compte
    // qu'on y a ; sans compte là-bas, on le dit.
    const send = () => run(async () => {
        let failed = 0;
        let reason = null;

        for (const serverId of chosen) {
            const server = peers.find((peer) => String(peer.id) === serverId);
            const opened = server ? await peerHost(server) : { ok: false };

            if (!opened.ok) {
                reason = t("saves.noAccountThere", { name: server?.name || serverId });
                failed += 1;
                continue;
            }

            for (const item of items) {
                if (!await copy(item, HERE, { host: opened.host, source: selfName })) {
                    failed += 1;
                }
            }
        }
        setOpened(null);
        setChosen([]);
        finish(failed, reason, "saves.syncDone", true);
    }, "send");

    // TÉLÉCHARGER : dans notre copie de ce serveur, sous le nom que notre fédération lui donne.
    // TÉLÉCHARGER, LA LIGNE DE TOUT TÉLÉCHARGEMENT (DownloadRow, le porteur, 2026-11-23 : « le download doit avoir la
    // progression comme add to library pour rester cohérent ») : l'anneau et « reçu sur total » pendant la lecture, puis
    // le rangement dans nos copies ; Annuler l'arrête tant qu'il lit.
    const download = () => run(async () => {
        let failed = 0;
        const aborter = new AbortController();
        stopTransfer.current = () => aborter.abort();

        for (const item of items) {
            if (aborter.signal.aborted) {
                break;
            }
            setTransfer({ state: "running", phase: "download", received: 0, total: 0 });
            const copied = await copy(item, { host: where.host, source: "" }, { host: null, source: where.server.name }, {
                signal: aborter.signal,
                // L'archive se fait à la volée, sans taille annoncée : celle de la sauvegarde la remplace quand on la connaît.
                onProgress: (received, total) => setTransfer({ state: "running", phase: "download", received, total: total || item.sizeBytes || 0 }),
                onStore: () => setTransfer((current) => ({ ...current, phase: "copy" })),
            });
            if (!copied) {
                failed += 1;
            }
        }
        stopTransfer.current = null;
        setTransfer(null);
        finish(aborter.signal.aborted ? 0 : failed, aborter.signal.aborted ? t("downloads.cancelled", { name: label }) : null);
    }, "download");

    const restore = () => run(async () => {
        if (games) {
            const answer = await restoreStreamexSaves(where.source, games[0].console, games.map((game) => game.id));
            finish(answer.status === 409 ? 1 : failuresOf(answer), answer.status === 409 ? t("saves.gameRunning") : null, "saves.restored");

            return;
        }
        const restorable = [];

        for (const item of slots) {
            const toReference = await localReference(item, where.source);

            if (toReference) {
                restorable.push({ consoleId: item.consoleId, reference: item.reference, toReference, slot: item.slot });
            } else {
                toast(t("saves.notHere", { title: item.title }));
            }
        }

        if (restorable.length) {
            finish(failuresOf(await restoreStates(where.source, restorable)) + slots.length - restorable.length, null, "saves.restored");
        }
    }, "restore");

    const remove = () => run(async () => {
        let failed = 0;
        let reason = null;

        for (const item of items) {
            const answer = slots ? await deleteSaveSlot(item.consoleId, item.reference, item.slot, where.source) : await deleteStreamexSave(item.console, item.id, where.source);

            if (!answer.ok) {
                failed += 1;
                // Le refus du serveur, dit tel quel (une partie de cette console en cours : « arrêtez-la d'abord »).
                reason = reason || answer.errorMessage || null;
            }
        }
        finish(failed, failed ? reason : null, "saves.deleted");

        if (!failed) {
            onRemoved?.();
        }
    }, "delete");

    // Envoyer vers… sans serveur fédéré : grisé, et pourquoi.
    const rows = {
        principal: [
            <NavRow key="backup" id={`${id}-backup`} icon="bi-archive" label={t("saves.backup")} help={t("saves.backupHelp")} disabled={busy} pending={running === "backup"} opens="confirm" onOpen={askBackup} />,
            <NavRow key="send" id={`${id}-send`} icon="bi-send" label={t("saves.send")} help={t(peers.length ? "saves.sendRowHelp" : "saves.sendNoServer")}
                disabled={busy || !peers.length} pending={running === "send"} opens="action" onOpen={() => toggle("send")} />,
        ],
        copy: [
            <NavRow key="restore" id={`${id}-restore-ask`} icon="bi-arrow-counterclockwise" label={t("saves.restore")} help={t("saves.restoreHelp")} disabled={busy} pending={running === "restore"} opens="confirm"
                onOpen={() => confirm({ title: t("saves.restore"), message: t("saves.restoreConfirm", { title: label }), onConfirm: restore })} />,
            notePath && (
                <NavRow key="note" id={`${id}-note`} icon="bi-pencil" label={t(note ? "saves.noteEdit" : "saves.noteWrite")} help={t("saves.noteRowHelp")} disabled={busy}
                    onOpen={() => open(notePath, `${id}-note`)} />
            ),
        ].filter(Boolean),
        peer: [
            transfer ? <DownloadRow key="download" entry={transfer} id={`${id}-download`} label={t("saves.download")} onCancel={() => stopTransfer.current?.()} />
                : <NavRow key="download" id={`${id}-download`} icon="bi-download" label={t("saves.download")} help={t("saves.downloadHelp", { name: where.server?.name })} disabled={busy} opens="action" onOpen={download} />,
        ],
    }[where.kind] || [];
    const gameRow = game && where.kind !== "peer" && gameConsole && (
        <NavRow key="library" id={`${id}-library`} icon="bi-collection-play" label={t("gameAdmin.openInLibrary")}
            help={game.reference ? t("gameAdmin.openInLibraryHelp") : t("saves.noGameHere")} disabled={!game.reference}
            onOpen={() => navigate(gameScreenPath(gameConsole, game.reference))} />
    );

    return (
        <>
            <Section title={t("more")} help={t("saves.moreHelp")} sense="modify">
                <Rows>{[...rows, gameRow].filter(Boolean)}</Rows>
            </Section>
            {opened === "send" && (
                <Section title={t("saves.sendPick")} help={t("saves.sendHelp")} sense="modify">
                    <Listing items={targets.found} search={targets.search} onSearch={targets.onSearch} placeholder={t("fed.searchServer")} searchKey={`${id}-to-search`}
                        render={(peer) => (
                            <SwitchRow key={peer.id} id={`${id}-to-${peer.id}`} label={peer.name} checked={chosen.includes(String(peer.id))}
                                onChange={(on) => pick(String(peer.id), on)} />
                        )} />
                    <Rows className="mt-3">
                        <SubmitRow>
                            <FormButton id={`${id}-send-now`} type="button" label={t("saves.sendNow")} isReplaced={chosen.length > 0} pending={busy} onClick={send} />
                        </SubmitRow>
                    </Rows>
                </Section>
            )}
            {canDelete && where.kind !== "peer" && (
                <DangerSection id={`${id}-delete`} label={t("saves.delete")} help={t(where.kind === "copy" ? "saves.deleteCopyHelp" : "saves.deleteHelp")}
                    question={t(where.kind === "copy" ? "saves.deleteConfirm" : "saves.deleteHereConfirm", { title: label })} disabled={busy} pending={running === "delete"} onConfirm={remove} />
            )}
        </>
    );
}

// LA NOTE D'UN BACKUP, en renseignement (InfoRow) : ce qu'elle dit, puis quand et sous quel compte elle a été écrite.
export function NoteSection({ note }) {
    const { t, i18n } = useTranslation();

    return note ? (
        <Section title={t("saves.noteTitle")} help={t("saves.noteSectionHelp")} sense="info">
            <Rows>
                <InfoRow icon="bi-sticky" label={t("saves.note")} value={note.note} />
                <InfoRow icon="bi-clock-history" label={t("saves.noteWritten")} value={[dateTime(note.date, i18n.language), `@${note.account}`]} />
            </Rows>
        </Section>
    ) : null;
}

// LES SAUVEGARDES D'UN RANGEMENT (`where`) : une ligne par jeu du rétro, une par console récente,
// triées par console ; la recherche, et les consoles en filtre — UN INTERRUPTEUR PAR CONSOLE qui a des sauvegardes ici,
// tous allumés au départ, comme les sources et les statuts de la recherche de la bibliothèque (le porteur, 2026-11-14,
// chantier 53, P13 : « dans le filtre qui se trouve dans profile > my saves, il faut que les consoles soit en switch et
// non pas en selectbox ») ; l'adresse garde les consoles éteintes (`?noconsole=`), avec la page. Une ligne ne fait
// qu'ouvrir sa page (`base` : la source) ; ses gestes, la suppression comprise, sont dans la page.
export function SaveList({ where, base }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const [loading, setLoading] = useState(true);
    const [slots, setSlots] = useState([]);
    const [streamex, setStreamex] = useState([]);
    const [read, update] = useQueryState({ q: "", nokind: "", page: "1" });
    const nameOf = useConsoleName();

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        Promise.all([listUserStates(where.source, where.host), listStreamexSaves(where.source, where.host)]).then(([retro, recent]) => {
            if (!cancelled) {
                setSlots(retro.ok ? retro.data || [] : []);
                setStreamex(recent.ok ? recent.data?.games || [] : []);
                setLoading(false);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [where.kind, where.source, where.host]);

    // LES CONSOLES SEULEMENT (le porteur, 2026-11-23 : « classe toujours par consoles avant. On doit voir que les
    // consoles ») : une ligne par console qui a des sauvegardes, rétro comme récente ; ses jeux sont sur sa page.
    const items = useMemo(() => {
        const byConsole = new Map();
        const entryOf = (consoleId, kind) => {
            if (!byConsole.has(consoleId)) {
                byConsole.set(consoleId, { key: consoleId, consoleId, kind, title: nameOf(consoleId), games: new Set(), titles: [] });
            }

            return byConsole.get(consoleId);
        };

        for (const slot of slots) {
            const entry = entryOf(slot.consoleId, "retro");
            if (!entry.games.has(slot.reference)) {
                entry.games.add(slot.reference);
                entry.titles.push(slot.title || slot.reference);
            }
        }
        for (const game of streamex) {
            const entry = entryOf(game.console, "streamex");
            entry.games.add(game.id);
            entry.titles.push(game.title || game.id);
        }

        return [...byConsole.values()].sort((a, b) => a.title.localeCompare(b.title));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [slots, streamex, consoles]);

    const hiddenKinds = hiddenOf(read("nokind"));
    const needle = read("q").trim().toLowerCase();
    const filtered = items.filter((item) => (!needle || `${item.title} ${item.titles.join(" ")}`.toLowerCase().includes(needle)) && !hiddenKinds.includes(item.kind));
    const pathOf = (item) => `${base}/console/${encodeURIComponent(item.consoleId)}`;

    return (
        <Listing items={filtered} loading={loading} search={read("q")} onSearch={(q) => update({ q, page: 1 })} placeholder={t("saves.search")}
            page={Number(read("page")) || 1} onPage={(page) => update({ page })} found={t("saves.found", { count: filtered.length })} empty={t("saves.empty")}
            filterCount={hiddenKinds.length}
            filters={[switchGroup({
                key: "kinds", title: t("facts.type"), id: "sv-kind", hidden: hiddenKinds, onChange: (kind, on) => update({ nokind: toggleHidden(hiddenKinds, kind, on), page: 1 }),
                choices: [{ value: "retro", label: t("tab.retrogaming") }, { value: "streamex", label: t("tab.streamex") }].filter((choice) => items.some((item) => item.kind === choice.value))
            })]}
            render={(item) => (
                <NavRow key={item.key} id={`sv-open-${item.key}`} icon={item.kind === "streamex" ? "bi-display" : CONSOLE_ICON} label={item.title}
                    onOpen={() => open(pathOf(item), `sv-open-${item.key}`, { title: item.title })}
                    help={[fact(t("facts.type"), t(item.kind === "streamex" ? "tab.streamex" : "tab.retrogaming")), fact(t("facts.games"), counted(t, "games", item.games.size))]} />
            )} />
    );
}

// Combien de jeux ont une sauvegarde dans chaque rangement d'ici (`""` : le principal ; sinon une copie) — un jeu du
// rétro compté une fois, quels que soient ses emplacements ; un jeu d'une console récente. Absent tant qu'il se lit, nul
// quand il ne se lit pas.
export function useSaveCounts(sources) {
    const [counts, setCounts] = useState({});
    const key = sources.join("\n");

    useEffect(() => {
        let cancelled = false;

        for (const source of sources) {
            Promise.all([listUserStates(source), listStreamexSaves(source)]).then(([retro, recent]) => {
                const games = new Set((retro.ok ? retro.data || [] : []).map((slot) => `${slot.consoleId}::${slot.reference}`));
                const count = retro.ok || recent.ok ? games.size + (recent.ok ? recent.data?.games || [] : []).length : null;

                if (!cancelled) {
                    setCounts((current) => ({ ...current, [source]: count }));
                }
            });
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);

    return counts;
}

// Route : /settings/profile/saves — LES SOURCES (Games.jsx, SourcesPage) — « Sur ce serveur » : cette machine et les backups en
// tête de chaque page, puis les copies reçues d'un serveur fédéré ; « Serveurs fédérés » : chacun. Leur nombre de jeux
// sauvegardés au bord droit, ici ; celui d'un serveur fédéré se lit chez lui, sous notre compte, une fois ouvert.
export default function Saves() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const peers = usePeers();
    const copies = useSaveSources();
    const others = copies.sources.filter((name) => name !== "local");
    const counts = useSaveCounts(["", "local", ...others]);
    const [read, update] = useQueryState({ page: "1" });
    const places = useListSearch([...others.map((name) => ({ name })), ...peers.map((peer) => ({ peer }))],
        (place) => (place.peer ? `${place.peer.name} ${t("saves.kindServer")}` : `${place.name} ${t("saves.kindCopy")}`));
    const opener = (slug) => () => open(sourcePath(slug), `svs-${slug}`);
    // DEUX SECTIONS (le porteur, 2026-11-23 : « je diviserais la section on this server en 2 ») : les sauvegardes en
    // service — celles que les parties jouent — puis les sauvegardes copiées : les backups, et les copies venues des
    // serveurs fédérés.
    return (
        <SettingsPage title={t("settings.items.mySaves.label")} parent={PROFILE}>
            <Section title={t("saves.inUseTitle")} help={t("saves.inUseHelp")} sense="play">
                <Rows>
                    <NavRow id={`svs-${PRINCIPAL}`} icon="bi-pc-display" label={t("saves.principal")} help={t("saves.principalHelp")} value={fact(t("facts.games"), counted(t, "games", counts[""] ?? null))} onOpen={opener(PRINCIPAL)} />
                </Rows>
            </Section>
            {/* UN SEUL LISTING PAR PAGE (le porteur, 2026-11-25 : « pas plus d'un listing par route ») : les sauvegardes copiées
                et celles des serveurs fédérés, ensemble — chaque ligne dit ce qu'elle est —, qui se cherchent. */}
            <Section title={t("saves.copiedTitle")} help={t("saves.copiedAndServersHelp")} sense="modify">
                <Listing items={places.found} loading={copies.loading} page={read("page")} onPage={(page) => update({ page })}
                    search={places.search} onSearch={(q) => { places.onSearch(q); update({ page: null }); }} placeholder={t("saves.searchPlace")} searchKey="svs-search"
                    head={<NavRow id="svs-copy-local" icon="bi-archive" label={t("saves.backups")} value={fact(t("facts.games"), counted(t, "games", counts.local ?? null))} onOpen={opener("copy-local")} />}
                    render={(place) => (place.peer ? (
                        <NavRow key={`server-${place.peer.id}`} id={`svs-server-${place.peer.id}`} icon="bi-arrow-left-right" label={place.peer.name}
                            value={t("saves.kindServer")} onOpen={opener(`server-${place.peer.id}`)} />
                    ) : (
                        <NavRow key={place.name} id={`svs-copy-${place.name}`} icon="bi-hdd-network" label={place.name}
                            value={[fact(t("facts.games"), counted(t, "games", counts[place.name] ?? null)), t("saves.kindCopy")]} onOpen={opener(`copy-${place.name}`)} />
                    ))} />
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}

// Les emplacements d'un jeu du rétro, ici, dans une copie ou chez un serveur fédéré.
export function useGameSlots(consoleId, reference, where) {
    const [slots, setSlots] = useState(null);
    const refresh = () => {
        if (where) {
            listSaveSlots(consoleId, reference, where.source, where.host).then((answer) => setSlots(answer.ok ? (answer.data || []).filter((slot) => slot.exists) : []));
        }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    useEffect(refresh, [consoleId, reference, where?.kind, where?.source, where?.host]);

    return [slots, refresh];
}

// UN JEU DU RÉTRO : ses emplacements, la liste de toute liste (Listing) — cherchés par leur nom ou leur note (`?q=`), une
// page à la fois (`?slotsPage=`) ; chacun (sa vignette, sa date ; dans les backups, le début de sa note) ouvre sa page, où
// sont ses gestes. « Tous les emplacements » est dans « Plus », après la liste (SaveGamePage ; le porteur, 2026-11-23 :
// « les pages my saves ne suivent pas le pattern […] la ligne The whole doit être dans une autre section more »).
export function SaveGameSlots({ consoleId, reference, title, where, slots }) {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const notes = useBackupNotes(where);
    const [read, update] = useQueryState({ q: "", slotsPage: "1" });
    const pathOf = (tail) => childPath(location, tail, "slotsPage");
    const noteOf = (slot) => notes?.[noteKey(slotNoteItem(consoleId, reference, slot.slot))];
    const needle = read("q").trim().toLowerCase();
    const shown = (slots || []).filter((slot) => !needle || `${slotName(slot.slot, t)} ${noteOf(slot)?.note || ""}`.toLowerCase().includes(needle));

    return (
        <Listing items={shown} loading={slots === null || notes === null} search={read("q")} onSearch={(q) => update({ q, slotsPage: null })} placeholder={t("saves.search")} searchKey="sv-search"
            found={t("saves.found", { count: shown.length })} empty={t("saves.empty")} page={read("slotsPage")} onPage={(page) => update({ slotsPage: page })}
            placeholderRow={() => <PlaceholderRow lead="thumb" />}
            render={(slot) => (
                <NavRow key={slot.slot} id={`sv-${slot.slot}`} label={slotName(slot.slot, t)} help={withNote(fact(t("facts.date"), dateTime(slot.timestamp, i18n.language)), noteOf(slot), t)}
                    onOpen={() => open(pathOf(`slot/${slot.slot}`), `sv-${slot.slot}`, { title })}
                    lead={<StateThumbnail consoleId={consoleId} reference={reference} slot={slot.slot} source={where.source} host={where.host} version={slot.timestamp} />} />
            )} />
    );
}

// Le titre d'un jeu du rétro : celui que la liste a passé, sinon relu dans les sauvegardes de cet endroit.
export function useGameTitle(consoleId, reference, where) {
    const location = useLocation();
    const given = location.state?.title || null;
    const [found, setFound] = useState(given ? { title: given } : undefined);

    useEffect(() => {
        if (given || !where) {
            return undefined;
        }
        let cancelled = false;
        listUserStates(where.source, where.host).then((answer) => {
            const slot = (answer.ok ? answer.data || [] : []).find((entry) => entry.consoleId === consoleId && entry.reference === reference);

            if (!cancelled) {
                setFound(slot ? { title: slot.title || slot.reference } : null);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [given, consoleId, reference, where?.kind, where?.source, where?.host]);

    return found;
}

// Les jeux d'une console récente qui ont une sauvegarde, ici, dans une copie ou chez un serveur fédéré.
export function useConsoleGames(consoleId, where) {
    const [games, setGames] = useState(null);
    const refresh = () => {
        if (where) {
            listStreamexSaves(where.source, where.host).then((answer) => setGames(answer.ok ? (answer.data?.games || []).filter((game) => game.console === consoleId) : []));
        }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    useEffect(refresh, [consoleId, where?.kind, where?.source, where?.host]);

    return [games, refresh];
}

// La ligne d'un jeu d'une console récente : sa console, la provenance de son jeu, son identifiant quand un titre le
// nomme, sa date, sa taille.
export const streamexHelp = (game, note, t, language) => [fact(t("facts.date"), dateTime(game.updatedAt, language)),
note ? fact(t("saves.note"), excerpt(note.note, t)) : fact(t("game.size"), formatSize(game.sizeBytes))];

// UNE CONSOLE RÉCENTE : ses jeux, la liste de toute liste (Listing) — cherchés par leur nom (`?q=`), filtrés par la
// provenance de leur jeu (`?nosource=`, quand il y en a plusieurs), une page à la fois (`?gamesPage=`) ; chacun (sa date,
// puis sa note sinon sa taille) ouvre sa page, où sont ses gestes. « Toute la console » est dans « Plus », après la liste
// (SaveConsolePage ; le porteur, 2026-11-23).
export function SaveConsoleGames({ consoleId, where, games }) {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const notes = useBackupNotes(where);
    const [read, update] = useQueryState({ q: "", nosource: "", gamesPage: "1" });
    const pathOf = (tail) => childPath(location, tail, "gamesPage");
    const sourceOf = (game) => streamexOrigin(game, t) || t("saves.noOrigin");
    const hidden = hiddenOf(read("nosource"));
    const sources = [...new Set((games || []).map(sourceOf))].sort().map((value) => ({ value, label: value }));
    const needle = read("q").trim().toLowerCase();
    const shown = (games || []).filter((game) => (!needle || `${streamexName(game, t)} ${game.id}`.toLowerCase().includes(needle)) && !hidden.includes(sourceOf(game)));

    return (
        <Listing items={shown} loading={games === null || notes === null} search={read("q")} onSearch={(q) => update({ q, gamesPage: null })} placeholder={t("saves.search")} searchKey="sx-search"
            found={t("saves.found", { count: shown.length })} empty={t("saves.empty")} page={read("gamesPage")} onPage={(page) => update({ gamesPage: page })}
            filterCount={hidden.length} filters={[switchGroup({
                key: "sources", title: t("library.sourcesTitle"), id: "sx-src", choices: sources, hidden,
                onChange: (value, on) => update({ nosource: toggleHidden(hidden, value, on), gamesPage: null })
            })]}
            render={(game) => (
                <NavRow key={game.id} id={`sx-${game.id}`} icon="bi-shield-check" label={streamexName(game, t)} help={streamexHelp(game, notes?.[noteKey(gameNoteItem(consoleId, game.id))], t, i18n.language)}
                    onOpen={() => open(pathOf(`game/${encodeURIComponent(game.id)}`), `sx-${game.id}`)} />
            )} />
    );
}

// UNE CONSOLE DU RÉTRO : ses jeux qui ont des emplacements, la liste de toute liste (Listing) — cherchés par leur titre
// (`?q=`), une page à la fois (`?gamesPage=`) ; chacun (ses emplacements, son dernier) ouvre la page de ses emplacements.
export function SaveRetroGames({ consoleId, where }) {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [states, setStates] = useState(null);
    const [read, update] = useQueryState({ q: "", gamesPage: "1" });

    useEffect(() => {
        let cancelled = false;
        listUserStates(where.source, where.host).then((answer) => !cancelled && setStates(answer.ok ? (answer.data || []).filter((slot) => slot.consoleId === consoleId) : []));

        return () => {
            cancelled = true;
        };
    }, [consoleId, where.kind, where.source, where.host]);

    const games = useMemo(() => {
        const byGame = new Map();
        for (const slot of states || []) {
            const game = byGame.get(slot.reference) || { reference: slot.reference, title: slot.title || slot.reference, slots: 0, latest: "" };
            game.slots += 1;
            game.latest = String(slot.timestamp || "") > game.latest ? String(slot.timestamp || "") : game.latest;
            byGame.set(slot.reference, game);
        }

        return [...byGame.values()].sort((a, b) => a.title.localeCompare(b.title));
    }, [states]);
    const needle = read("q").trim().toLowerCase();
    const shown = games.filter((game) => !needle || game.title.toLowerCase().includes(needle));
    const base = `${where.base}/game/${encodeURIComponent(consoleId)}`;

    return (
        <Listing items={shown} loading={states === null} search={read("q")} onSearch={(q) => update({ q, gamesPage: null })} placeholder={t("saves.search")} searchKey="sr-search"
            found={t("saves.found", { count: shown.length })} empty={t("saves.empty")} page={read("gamesPage")} onPage={(page) => update({ gamesPage: page })}
            render={(game) => (
                <NavRow key={game.reference} id={`sr-${game.reference}`} icon="bi-shield-check" label={game.title}
                    help={[fact(t("saves.slotsTitle"), counted(t, "slots", game.slots)), fact(t("facts.date"), game.latest ? dateTime(game.latest, i18n.language) : null)]}
                    onOpen={() => open(`${base}/${encodeURIComponent(game.reference)}`, `sr-${game.reference}`, { title: game.title })} />
            )} />
    );
}
