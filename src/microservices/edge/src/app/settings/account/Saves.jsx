import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import {
    backupStates, backupStreamexSaves, deleteGameSaves, deleteSaveSlot, deleteStreamexSave, fetchStateFile, fetchStreamexSave, listRoms, listSaveNotes,
    listSaveSlots, listSaveSources, listStreamexSaves, listUserStates, restoreStates, restoreStreamexSaves, uploadStateFile, uploadStreamexSave, writeSaveNote,
} from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { peerHost, useFederationSession } from "../../libs/federation.js";
import { formatSize } from "../../libs/format.js";
import { FederatedAccount, FederatedRefusal, FederatedTokenForm } from "../../screens/access/FederatedAccount.jsx";
import { slotName, StateThumbnail } from "../../screens/library/GameSettings.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Form, FormButton } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, PROFILE, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { DangerSection, GuideSection, InfoRow, Listing, Note, PlaceholderRow, Rows, Section, SubmitRow, SwitchRow, TextRow, useConfirm } from "../../ui/Bricks.jsx";

// MES SAUVEGARDES, PAR SOURCE (profile/settings/library de la webapp : SavedStates, StreamexSaves,
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

const HERE = { host: null, source: "" };
const BASE = `${PROFILE}/saves`;
const PRINCIPAL = "here";

// `{kind: "principal"}`, `{kind: "copy", name}`, `{kind: "peer", id}`, ou null pour un segment qui ne désigne rien.
const sourceOf = (slug) => {
    const match = /^(copy|server)-(.+)$/.exec(slug || "");

    if (slug === PRINCIPAL) {
        return { kind: "principal" };
    }

    return match ? (match[1] === "copy" ? { kind: "copy", name: match[2] } : { kind: "peer", id: match[2] }) : null;
};
const sourcePath = (slug) => `${BASE}/${encodeURIComponent(slug)}`;
const copyName = (name, t) => (name === "local" ? t("saves.backups") : name);

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
const failuresOf = (answer) => (answer.ok ? (answer.data?.results || []).filter((result) => result.error).length : 1);

const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "—");

// Le nom d'un jeu d'une console récente : son titre (celui de son jeu dans la bibliothèque, que le kernel trouve par son
// identifiant, sinon celui que la sauvegarde se donne : metadata.NameSaves), sinon son identifiant ; la Xbox n'a que son
// disque.
const streamexName = (game, t) => (game.console === "xbox" ? t("saves.xboxDisk") : game.title || game.id);

// LA LIGNE D'UNE SAUVEGARDE DIT SA CONSOLE (le porteur, 2026-11-16 : « Oui il faut les nom des jeux et la console dans
// mes sauvegardes »), comme les autres listes (« console · … », le Catalogue, Mes favoris) : le nom du registre d'abord,
// puis ce que la ligne ajoute. Définie ici, une fois, pour chaque ligne de ces pages.
function useConsoleName() {
    const { consoles } = useData();

    return (consoleId) => consoles?.find((entry) => entry.id === consoleId)?.name || consoleId;
}
const saveHelp = (consoleName, ...parts) => [consoleName, ...parts].filter(Boolean).join(" · ");

// La provenance du jeu qui nomme une sauvegarde d'une console récente, comme celle d'un jeu du Catalogue (originOf) : son
// disque interne, sinon la bibliothèque ; rien quand aucun jeu ne la nomme.
const streamexOrigin = (game, t) => (game.reference ? game.disk || t("games.source.library") : null);

// LES NOTES DES BACKUPS (saves/notes.go) : la plus longue, celle que le kernel revalide ; la clé d'une sauvegarde, comme
// la liste des notes la nomme ; une note dans une ligne, son début entre guillemets.
const NOTE_MAX = 500;
const NOTE_EXCERPT = 80;
const slotNoteItem = (consoleId, reference, slot) => ({ kind: "states", console: consoleId, reference, slot: Number(slot) });
const gameNoteItem = (consoleId, id) => ({ kind: "streamex", console: consoleId, id });
const noteKey = (item) => (item.kind === "streamex" ? `streamex/${item.console}/${item.id}` : `states/${item.console}/${item.reference}/${item.slot}`);
const isBackups = (where) => where?.kind === "copy" && where.source === "local";
const excerpt = (text, t) => t("saves.noteQuoted", { note: text.length > NOTE_EXCERPT ? `${text.slice(0, NOTE_EXCERPT).trimEnd()}…` : text });
const withNote = (help, note, t) => [help, note ? excerpt(note.note, t) : null].filter(Boolean).join(" · ");

// Les notes des backups, par sauvegarde (noteKey) — seulement quand on regarde les backups : `{}` ailleurs, null tant
// qu'elles se lisent.
function useBackupNotes(where) {
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
async function copySlot({ consoleId, reference, slot, title, timestamp }, from, to) {
    const state = await fetchStateFile(consoleId, reference, slot, from.source, from.host);

    if (!state.ok) {
        return false;
    }
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
async function copyStreamexGame({ console, id }, from, to) {
    const archive = await fetchStreamexSave(console, id, from.source, from.host);

    return archive.ok ? Boolean((await uploadStreamexSave(console, id, archive.blob, to.source, to.host)).ok) : false;
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
function usePeers() {
    const { servers } = useData();

    return useMemo(() => (servers || []).map((entry) => ({ ...entry, name: entry.hostname || entry.link })).sort((a, b) => a.name.localeCompare(b.name)), [servers]);
}

// L'adresse d'une page enfant d'une liste : la sienne, puis la requête de la page sans la page de la liste (`pageKey`)
// — la page de la liste reste à la liste.
const childPath = (location, tail, pageKey) => {
    const params = new URLSearchParams(location.search);
    params.delete(pageKey);
    const query = params.toString();

    return `${location.pathname}/${tail}${query ? `?${query}` : ""}`;
};

// Les copies de ce compte, et le nom sous lequel ce serveur se présente aux autres (`self`). `loading` : la liste se
// lit encore.
function useSaveSources() {
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
function usePlace() {
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
// colle son jeton, ou on dit pourquoi il refuse ; avec sa session, ce qu'on y voit — sous quel compte d'abord.
function PeerGate({ place, children }) {
    const { t } = useTranslation();
    const { peer, federation } = place;

    if (!peer || federation.loading) {
        return <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>;
    }

    // Chacun est un groupe à lui (FederatedAccount.jsx), posé tel quel.
    if (!federation.session) {
        return federation.code === "noFederatedAccount" || federation.code === "accountRefused"
            ? <FederatedTokenForm server={peer} refused={federation.code === "accountRefused"} />
            : <FederatedRefusal server={peer} code={federation.code} />;
    }

    return (
        <>
            <FederatedAccount server={peer} account={federation.session} />
            {children}
        </>
    );
}

// Une page d'une source : rien pour un segment qui ne désigne rien, la porte d'un serveur fédéré, puis ce qu'elle montre.
function PlaceBody({ place, children }) {
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
function BackupNote({ id, holder }) {
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

function SaveActions({ id, where, peers, selfName, slots = null, games = null, label, canDelete, notePath = null, note = null, onDone, onRemoved = null }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const { open } = useSettingsNavigation();
    const [opened, setOpened] = useState(null);
    const [chosen, setChosen] = useState([]);
    const [busy, setBusy] = useState(false);
    const items = slots || games || [];
    const pick = (serverId, on) => setChosen((previous) => (on ? [...previous, serverId] : previous.filter((other) => other !== serverId)));
    const toggle = (form) => setOpened((current) => (current === form ? null : form));

    const finish = (failed, message = null, done = "saves.syncDone") => {
        toast(message || t(failed ? "saves.syncFailed" : done));
        onDone?.();
    };

    const run = async (work) => {
        setBusy(true);

        try {
            await work();
        } catch {
            finish(1);
        } finally {
            setBusy(false);
        }
    };

    const copy = (item, from, to) => (slots ? copySlot(item, from, to) : copyStreamexGame(item, from, to));

    // BACKUP : dans `local`, à la place du précédent, avec la note écrite (facultative). LA NOTE S'ÉCRIT DANS LA FENÊTRE DE
    // SKELETON (le porteur, 2026-11-16 : « mets le champ note dans un modal skeleton au lieu de l'afficher plus bas. Ça
    // porte confusion comme ça ») : « Faire le backup » la confirme.
    const backup = (written) => run(async () => {
        const answer = slots
            ? await backupStates(slots.map(({ consoleId, reference, slot }) => ({ consoleId, reference, slot })), written)
            : await backupStreamexSaves(games[0].console, games.map((game) => game.id), written);
        finish(failuresOf(answer), null, "saves.backedUp");
    });
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
        finish(failed, reason);
    });

    // TÉLÉCHARGER : dans notre copie de ce serveur, sous le nom que notre fédération lui donne.
    const download = () => run(async () => {
        let failed = 0;

        for (const item of items) {
            if (!await copy(item, { host: where.host, source: "" }, { host: null, source: where.server.name })) {
                failed += 1;
            }
        }
        finish(failed);
    });

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
    });

    const remove = () => run(async () => {
        let failed = 0;

        for (const item of items) {
            const answer = slots ? await deleteSaveSlot(item.consoleId, item.reference, item.slot, where.source) : await deleteStreamexSave(item.console, item.id, where.source);

            if (!answer.ok) {
                failed += 1;
            }
        }
        finish(failed, null, "saves.deleted");

        if (!failed) {
            onRemoved?.();
        }
    });

    // Envoyer vers… sans serveur fédéré : grisé, et pourquoi.
    const rows = {
        principal: [
            <NavRow key="backup" id={`${id}-backup`} icon="bi-archive" tone="indigo" label={t("saves.backup")} help={t("saves.backupHelp")} disabled={busy} opens="confirm" onOpen={askBackup} />,
            <NavRow key="send" id={`${id}-send`} icon="bi-send" tone="indigo" label={t("saves.send")} help={t(peers.length ? "saves.sendRowHelp" : "saves.sendNoServer")}
                disabled={busy || !peers.length} opens="action" onOpen={() => toggle("send")} />,
        ],
        copy: [
            <NavRow key="restore" id={`${id}-restore-ask`} icon="bi-arrow-counterclockwise" tone="amber" label={t("saves.restore")} help={t("saves.restoreHelp")} disabled={busy} opens="confirm"
                onOpen={() => confirm({ title: t("saves.restore"), message: t("saves.restoreConfirm", { title: label }), onConfirm: restore })} />,
            notePath && (
                <NavRow key="note" id={`${id}-note`} icon="bi-pencil-square" tone="teal" label={t(note ? "saves.noteEdit" : "saves.noteWrite")} help={t("saves.noteRowHelp")} disabled={busy}
                    onOpen={() => open(notePath, `${id}-note`)} />
            ),
        ].filter(Boolean),
        peer: [
            <NavRow key="download" id={`${id}-download`} icon="bi-download" tone="green" label={t("saves.download")} help={t("saves.downloadHelp", { name: where.server?.name })} disabled={busy} opens="action" onOpen={download} />,
        ],
    }[where.kind] || [];

    return (
        <>
            <Section title={t("more")}>
                <Rows>{rows}</Rows>
                {busy && <Note><span className="spinner-border spinner-border-sm me-2" role="status"></span>{t("saves.working")}</Note>}
            </Section>
            {opened === "send" && (
                <Section title={t("saves.sendPick")} help={t("saves.sendHelp")}>
                    <Rows>
                        {peers.map((peer) => (
                            <SwitchRow key={peer.id} id={`${id}-to-${peer.id}`} label={peer.name} checked={chosen.includes(String(peer.id))}
                                onChange={(on) => pick(String(peer.id), on)} />
                        ))}
                        <SubmitRow>
                            <FormButton id={`${id}-send-now`} type="button" label={t("saves.sendNow")} isReplaced={chosen.length > 0} pending={busy} onClick={send} />
                        </SubmitRow>
                    </Rows>
                </Section>
            )}
            {canDelete && where.kind !== "peer" && (
                <DangerSection id={`${id}-delete`} label={t("saves.delete")} help={t(where.kind === "copy" ? "saves.deleteCopyHelp" : "saves.deleteHelp")}
                    question={t("saves.deleteConfirm", { title: label })} disabled={busy} onConfirm={remove} />
            )}
        </>
    );
}

// LA NOTE D'UN BACKUP, en renseignement (InfoRow) : ce qu'elle dit, puis quand et sous quel compte elle a été écrite.
function NoteSection({ note }) {
    const { t, i18n } = useTranslation();

    return note ? (
        <Section title={t("saves.noteTitle")}>
            <Rows>
                <InfoRow icon="bi-sticky" tone="teal" label={t("saves.note")} value={note.note} />
                <InfoRow icon="bi-clock-history" tone="slate" label={t("saves.noteWritten")} value={`${dateTime(note.date, i18n.language)} · @${note.account}`} />
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
function SaveList({ where, base }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const [loading, setLoading] = useState(true);
    const [slots, setSlots] = useState([]);
    const [streamex, setStreamex] = useState([]);
    const [read, update] = useQueryState({ q: "", noconsole: "", page: "1" });
    const hidden = read("noconsole").split(",").filter(Boolean);
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

    const items = useMemo(() => {
        const byGame = new Map();

        for (const slot of slots) {
            const key = `${slot.consoleId}::${slot.reference}`;
            const existing = byGame.get(key);

            if (existing) {
                existing.slots.push(slot);
            } else {
                byGame.set(key, { kind: "retro", key, consoleId: slot.consoleId, reference: slot.reference, title: slot.title || slot.reference, slots: [slot] });
            }
        }
        const byConsole = new Map();

        for (const game of streamex) {
            const existing = byConsole.get(game.console);

            if (existing) {
                existing.games.push(game);
            } else {
                byConsole.set(game.console, { kind: "streamex", key: `streamex::${game.console}`, consoleId: game.console, title: nameOf(game.console), games: [game] });
            }
        }

        return [...byGame.values(), ...byConsole.values()].sort((a, b) => nameOf(a.consoleId).localeCompare(nameOf(b.consoleId)));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [slots, streamex, consoles]);

    const savedConsoles = useMemo(() => [...new Set(items.map((item) => item.consoleId))].map((id) => ({ id, name: nameOf(id) })).sort((a, b) => a.name.localeCompare(b.name)),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [items, consoles]);

    const needle = read("q").trim().toLowerCase();
    const filtered = items.filter((item) => (!needle || item.title.toLowerCase().includes(needle)
        || (item.games || []).some((game) => `${game.title || ""} ${game.id}`.toLowerCase().includes(needle)))
        && !hidden.includes(item.consoleId));
    // Un interrupteur basculé : la liste des consoles éteintes gagne ou perd la sienne, et la liste repart de sa page 1.
    const toggle = (consoleId) => (on) => update({ noconsole: (on ? hidden.filter((other) => other !== consoleId) : [...hidden, consoleId]).join(","), page: 1 });

    const pathOf = (item) => (item.kind === "streamex"
        ? `${base}/console/${encodeURIComponent(item.consoleId)}`
        : `${base}/game/${encodeURIComponent(item.consoleId)}/${encodeURIComponent(item.reference)}`);

    return (
        <Listing items={filtered} loading={loading} search={read("q")} onSearch={(q) => update({ q, page: 1 })} placeholder={t("saves.search")}
            page={Number(read("page")) || 1} onPage={(page) => update({ page })} found={t("saves.found", { count: filtered.length })} empty={t("saves.empty")}
            filterCount={savedConsoles.filter((entry) => hidden.includes(entry.id)).length} filterTitle={t("saves.filterTitle")} filterHelp={t("saves.filterHelp")}
            filters={savedConsoles.length > 1 && savedConsoles.map((entry) => (
                <SwitchRow key={entry.id} id={`sv-console-${entry.id}`} label={entry.name} checked={!hidden.includes(entry.id)} onChange={toggle(entry.id)} />
            ))}
            render={(item) => (
                <NavRow key={item.key} id={`sv-open-${item.key}`} icon={item.kind === "streamex" ? "bi-controller" : "bi-shield-check"} tone={item.kind === "streamex" ? "rose" : "teal"} label={item.title}
                    onOpen={() => open(pathOf(item), `sv-open-${item.key}`, { title: item.title })}
                    help={item.kind === "streamex" ? t("saves.gameCount", { count: item.games.length }) : saveHelp(nameOf(item.consoleId), t("saves.slotCount", { count: item.slots.length }))} />
            )} />
    );
}

// Combien de jeux ont une sauvegarde dans chaque rangement d'ici (`""` : le principal ; sinon une copie) — un jeu du
// rétro compté une fois, quels que soient ses emplacements ; un jeu d'une console récente. Absent tant qu'il se lit, nul
// quand il ne se lit pas.
function useSaveCounts(sources) {
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

// /settings/profile/saves : LES SOURCES (Games.jsx, SourcesPage) — « Sur ce serveur » : cette machine et les backups en
// tête de chaque page, puis les copies reçues d'un serveur fédéré ; « Serveurs fédérés » : chacun. Leur nombre de jeux
// sauvegardés au bord droit, ici ; celui d'un serveur fédéré se lit chez lui, sous notre compte, une fois ouvert.
function SourcesPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const peers = usePeers();
    const copies = useSaveSources();
    const others = copies.sources.filter((name) => name !== "local");
    const counts = useSaveCounts(["", "local", ...others]);
    const [read, update] = useQueryState({ copiesPage: "1", serversPage: "1" });
    const opener = (slug) => () => open(sourcePath(slug), `svs-${slug}`);
    const head = (
        <>
            <NavRow id={`svs-${PRINCIPAL}`} icon="bi-pc-display" tone="indigo" label={t("saves.principal")} help={t("saves.principalHelp")} value={counts[""] ?? null} onOpen={opener(PRINCIPAL)} />
            <NavRow id="svs-copy-local" icon="bi-archive" tone="slate" label={t("saves.backups")} help={t("saves.backupsHelp")} value={counts.local ?? null} onOpen={opener("copy-local")} />
        </>
    );

    return (
        <SettingsPage title={t("settings.items.mySaves.label")} parent={PROFILE}>
            <Section title={t("saves.hereTitle")} help={t("saves.hereHelp")}>
                <Listing items={others} loading={copies.loading} head={head} page={read("copiesPage")} onPage={(page) => update({ copiesPage: page })}
                    render={(name) => (
                        <NavRow key={name} id={`svs-copy-${name}`} icon="bi-hdd-network" tone="green" label={name} help={t("saves.copyFromHelp", { name })} value={counts[name] ?? null}
                            onOpen={opener(`copy-${name}`)} />
                    )} />
            </Section>
            <Section title={t("saves.serversTitle")} help={peers.length ? t("saves.serversHelp") : null}>
                <Listing items={peers} empty={t("saves.noServer")} page={read("serversPage")} onPage={(page) => update({ serversPage: page })}
                    render={(peer) => (
                        <NavRow key={peer.id} id={`svs-server-${peer.id}`} icon="bi-arrow-left-right" tone="green" label={peer.name} onOpen={opener(`server-${peer.id}`)} />
                    )} />
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={[["storage", "my-saves"]]} />
        </SettingsPage>
    );
}

// /settings/profile/saves/<source> : SES SAUVEGARDES — un serveur fédéré, sous quel compte d'abord.
function SourcePage() {
    const { t } = useTranslation();
    const place = usePlace();

    return (
        <SettingsPage title={place.name} parent={BASE} levels={place.levels}>
            <PlaceBody place={place}>
                <Section title={t("saves.listTitle")}>
                    <SaveList where={place.where} base={place.base} />
                </Section>
            </PlaceBody>
        </SettingsPage>
    );
}

// Les emplacements d'un jeu du rétro, ici, dans une copie ou chez un serveur fédéré.
function useGameSlots(consoleId, reference, where) {
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

// UN JEU DU RÉTRO : tous ses emplacements d'un geste, en tête de chaque page, puis chacun (sa vignette, sa date ; dans
// les backups, le début de sa note) — chaque ligne ouvre sa page, où sont ses gestes (aucun geste sur la ligne,
// NavRow.jsx). Une page à la fois (`?slotsPage=`).
function SaveGameSlots({ consoleId, reference, title, where }) {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const [slots] = useGameSlots(consoleId, reference, where);
    const notes = useBackupNotes(where);
    const consoleName = useConsoleName()(consoleId);
    const [read, update] = useQueryState({ slotsPage: "1" });
    const pathOf = (tail) => childPath(location, tail, "slotsPage");
    const all = (slots || []).length > 1
        ? <NavRow id="sv-all" icon="bi-collection" tone="teal" label={t("saves.wholeGame")} help={saveHelp(consoleName, t("saves.slotCount", { count: slots.length }))} onOpen={() => open(pathOf("all"), "sv-all", { title })} />
        : null;

    return (
        <Listing items={slots || []} loading={slots === null || notes === null} empty={t("saves.empty")} head={all} page={read("slotsPage")} onPage={(page) => update({ slotsPage: page })}
            placeholderRow={() => <PlaceholderRow lead="thumb" />}
            render={(slot) => (
                <NavRow key={slot.slot} id={`sv-${slot.slot}`} label={slotName(slot.slot, t)} help={withNote(saveHelp(consoleName, dateTime(slot.timestamp, i18n.language)), notes?.[noteKey(slotNoteItem(consoleId, reference, slot.slot))], t)}
                    onOpen={() => open(pathOf(`slot/${slot.slot}`), `sv-${slot.slot}`, { title })}
                    lead={<StateThumbnail consoleId={consoleId} reference={reference} slot={slot.slot} source={where.source} host={where.host} version={slot.timestamp} />} />
            )} />
    );
}

// /settings/profile/saves/<source>/game/<console>/<jeu>/slot/<n> et …/all : UN emplacement, ou tous ceux du jeu
// (une ligne chacun : sa vignette, sa date ; seul, le titre du jeu, que le titre de la page ne dit pas), et leurs
// gestes ; un backup seul, sa note. Retiré, la page n'a plus rien à montrer : retour au jeu.
function SaveSlotsPage() {
    const { t, i18n } = useTranslation();
    const { consoleId, reference, slot: slotParam } = useParams();
    const place = usePlace();
    const { self } = useSaveSources();
    const { back } = useSettingsNavigation();
    const found = useGameTitle(consoleId, reference, place.where);
    const [slots, refresh] = useGameSlots(consoleId, reference, place.where);
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const parent = `${place.base}/game/${encodeURIComponent(consoleId)}/${encodeURIComponent(reference)}`;
    const title = found?.title || reference;
    const all = slotParam === undefined;
    const chosen = (slots || []).filter((entry) => all || String(entry.slot) === slotParam);
    const items = chosen.map((entry) => ({ consoleId, reference, slot: entry.slot, title, timestamp: entry.timestamp }));
    const label = all ? t("saves.wholeGame") : slotName(Number(slotParam), t);
    const noteOf = (slot) => notes?.[noteKey(slotNoteItem(consoleId, reference, slot))] || null;
    const backupNote = !all && isBackups(place.where) ? { path: `${parent}/slot/${slotParam}/note`, note: noteOf(slotParam) } : null;
    const body = slots === null || notes === null || found === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : !chosen.length ? <NotFound /> : (
        <>
            <Section title={t(all ? "saves.slotsTitle" : "saves.stateTitle")}>
                <Listing items={chosen} render={(entry) => (
                    <InfoRow key={entry.slot} label={all ? slotName(entry.slot, t) : title} value={withNote(saveHelp(consoleName, dateTime(entry.timestamp, i18n.language)), all ? noteOf(entry.slot) : null, t)}
                        lead={<StateThumbnail consoleId={consoleId} reference={reference} slot={entry.slot} source={place.where.source} host={place.where.host} version={entry.timestamp} />} />
                )} />
            </Section>
            {backupNote && <NoteSection note={backupNote.note} />}
            <SaveActions id={all ? "sv-all" : `sv-${slotParam}`} where={place.where} peers={place.peers} selfName={self} label={`${title} — ${label}`} slots={items}
                canDelete={all ? place.where.kind === "copy" : place.where.kind !== "peer"} notePath={backupNote?.path} note={backupNote?.note} onDone={refresh} onRemoved={() => back(parent)} />
        </>
    );

    return (
        <SettingsPage title={label} parent={parent} levels={{ ...place.levels, [parent]: found?.title }}>
            <PlaceBody place={place}>{body}</PlaceBody>
        </SettingsPage>
    );
}

// Le titre d'un jeu du rétro : celui que la liste a passé, sinon relu dans les sauvegardes de cet endroit.
function useGameTitle(consoleId, reference, where) {
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

// /settings/profile/saves/<source>/game/<console>/<jeu>
function SaveGamePage() {
    const { t } = useTranslation();
    const { consoleId, reference } = useParams();
    const place = usePlace();
    const { back } = useSettingsNavigation();
    const found = useGameTitle(consoleId, reference, place.where);
    const parent = place.base;
    const body = found === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : found === null ? <NotFound /> : (
        <>
            <Section title={t("saves.slotsTitle")}>
                <SaveGameSlots consoleId={consoleId} reference={reference} title={found.title} where={place.where} />
            </Section>
            {/* TOUTES SES SAUVEGARDES D'UN GESTE (la ligne de la liste ne porte plus de geste) — ici ou dans une
                copie ; jamais chez un serveur fédéré. Fait, la page n'a plus rien à montrer : retour à la liste. */}
            {place.where?.kind !== "peer" && (
                <DangerSection id="sv-del-game" help={t("saves.deleteGameHelp")} label={t("saves.deleteGame")} question={t("saves.deleteGameConfirm", { title: found.title })}
                    onConfirm={async () => {
                        const answer = await deleteGameSaves(consoleId, reference, place.where.source);

                        if (answer.ok) {
                            toast(t("saves.deleted"));
                            back(parent);
                        } else {
                            toast(t("saves.deleteFailed"));
                        }
                    }} />
            )}
        </>
    );

    return (
        <SettingsPage title={found?.title || reference} parent={parent} levels={place.levels}>
            <PlaceBody place={place}>{body}</PlaceBody>
        </SettingsPage>
    );
}

// Les jeux d'une console récente qui ont une sauvegarde, ici, dans une copie ou chez un serveur fédéré.
function useConsoleGames(consoleId, where) {
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
const streamexHelp = (game, consoleName, t, language) => saveHelp(consoleName, streamexOrigin(game, t), game.console === "xbox" ? t("saves.xboxDiskHelp") : game.title ? game.id : null,
    dateTime(game.updatedAt, language), formatSize(game.sizeBytes));

// UNE CONSOLE RÉCENTE : toute la console d'un geste, en tête de chaque page, puis jeu par jeu (un dossier par jeu ; dans
// les backups, le début de sa note) — chaque ligne ouvre sa page, où sont ses gestes. Une page à la fois (`?gamesPage=`).
function SaveConsoleGames({ consoleId, title, where }) {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const [games] = useConsoleGames(consoleId, where);
    const notes = useBackupNotes(where);
    const consoleName = useConsoleName()(consoleId);
    const [read, update] = useQueryState({ gamesPage: "1" });
    const pathOf = (tail) => childPath(location, tail, "gamesPage");
    const all = (games || []).length > 1
        ? <NavRow id="sx-all" icon="bi-collection" tone="teal" label={t("saves.wholeConsole", { name: title })} help={t("saves.gameCount", { count: games.length })} onOpen={() => open(pathOf("all"), "sx-all")} />
        : null;

    return (
        <Listing items={games || []} loading={games === null || notes === null} empty={t("saves.empty")} head={all} page={read("gamesPage")} onPage={(page) => update({ gamesPage: page })}
            render={(game) => (
                <NavRow key={game.id} id={`sx-${game.id}`} icon="bi-shield-check" tone="teal" label={streamexName(game, t)} help={withNote(streamexHelp(game, consoleName, t, i18n.language), notes?.[noteKey(gameNoteItem(consoleId, game.id))], t)}
                    onOpen={() => open(pathOf(`game/${encodeURIComponent(game.id)}`), `sx-${game.id}`)} />
            )} />
    );
}

// /settings/profile/saves/<source>/console/<console>/game/<jeu> et …/all : la sauvegarde d'UN jeu, ou celles de
// toute la console (une ligne chacune), et leurs gestes ; un backup seul, sa note. Retirée, retour à la console.
function SaveGamesPage() {
    const { t, i18n } = useTranslation();
    const { consoleId, gameId } = useParams();
    const place = usePlace();
    const { self } = useSaveSources();
    const { back } = useSettingsNavigation();
    const [games, refresh] = useConsoleGames(consoleId, place.where);
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const parent = `${place.base}/console/${encodeURIComponent(consoleId)}`;
    const all = gameId === undefined;
    const chosen = (games || []).filter((game) => all || game.id === gameId);
    const label = all ? t("saves.wholeConsole", { name: consoleName }) : chosen[0] ? streamexName(chosen[0], t) : gameId;
    const noteOf = (id) => notes?.[noteKey(gameNoteItem(consoleId, id))] || null;
    const backupNote = !all && isBackups(place.where) ? { path: `${parent}/game/${encodeURIComponent(gameId)}/note`, note: noteOf(gameId) } : null;
    const body = games === null || notes === null ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : !chosen.length ? <NotFound /> : (
        <>
            <Section title={t(all ? "saves.gamesTitle" : "saves.saveTitle")}>
                <Listing items={chosen} render={(game) => (
                    <InfoRow key={game.id} icon="bi-shield-check" tone="teal" label={streamexName(game, t)} value={withNote(streamexHelp(game, consoleName, t, i18n.language), all ? noteOf(game.id) : null, t)} />
                )} />
            </Section>
            {backupNote && <NoteSection note={backupNote.note} />}
            <SaveActions id={all ? "sx-all" : `sx-${gameId}`} where={place.where} peers={place.peers} selfName={self} games={chosen} label={label}
                canDelete={place.where.kind === "copy"} notePath={backupNote?.path} note={backupNote?.note} onDone={refresh} onRemoved={() => back(parent)} />
        </>
    );

    return (
        <SettingsPage title={label} parent={parent} levels={{ ...place.levels, [parent]: consoleName }}>
            <PlaceBody place={place}>{body}</PlaceBody>
        </SettingsPage>
    );
}

// /settings/profile/saves/<source>/console/<console>
function SaveConsolePage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const place = usePlace();
    const title = useConsoleName()(consoleId);

    return (
        <SettingsPage title={title} parent={place.base} levels={place.levels}>
            <PlaceBody place={place}>
                <Section title={t("saves.gamesTitle")} help={t("saves.consoleHelp")}>
                    <SaveConsoleGames consoleId={consoleId} title={title} where={place.where} />
                </Section>
            </PlaceBody>
        </SettingsPage>
    );
}

// /settings/profile/saves/copy-local/…/slot/<n>/note et …/game/<jeu>/note : LA NOTE D'UN BACKUP, écrite ou corrigée ;
// vidée, elle part. Le formulaire de Skeleton (Form.Longtext), son bouton au repos tant que rien n'a changé
// (`isReplaced`) ; enregistrée, retour au backup. Ailleurs que dans les backups, la page n'existe pas.
function NotePage() {
    const { t } = useTranslation();
    const { consoleId, reference, slot, gameId } = useParams();
    const place = usePlace();
    const { back } = useSettingsNavigation();
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const [draft, setDraft] = useState(null);
    const [pending, setPending] = useState(false);
    const streamex = gameId !== undefined;
    const item = streamex ? gameNoteItem(consoleId, gameId) : slotNoteItem(consoleId, reference, slot);
    const found = useGameTitle(consoleId, streamex ? null : reference, streamex ? null : place.where);
    const [games] = useConsoleGames(consoleId, streamex ? place.where : null);
    const savedGame = streamex ? (games || []).find((game) => game.id === gameId) : null;
    const consolePath = `${place.base}/console/${encodeURIComponent(consoleId)}`;
    const gamePath = `${place.base}/game/${encodeURIComponent(consoleId)}/${encodeURIComponent(reference)}`;
    const parent = streamex ? `${consolePath}/game/${encodeURIComponent(gameId)}` : `${gamePath}/slot/${slot}`;
    // Les niveaux au-dessus, nommés même par un lien direct : la console et le jeu (son titre, sinon son identifiant), ou
    // le jeu et l'emplacement.
    const levels = streamex
        ? { ...place.levels, [consolePath]: consoleName, [parent]: savedGame ? streamexName(savedGame, t) : gameId }
        : { ...place.levels, [gamePath]: found?.title, [parent]: slotName(Number(slot), t) };
    const saved = notes?.[noteKey(item)]?.note || "";
    const value = draft ?? saved;
    const isReplaced = value.trim() !== saved;

    const save = async () => {
        setPending(true);
        const answer = await writeSaveNote(item, value);
        setPending(false);

        if (answer.ok) {
            toast(t(value.trim() ? "saves.noteSaved" : "saves.noteRemoved"));
            back(parent);
        } else {
            toast(answer.status === 404 ? t("saves.noteNoBackup") : answer.errorMessage || t("settings.saveFailed"));
        }
    };

    const body = !isBackups(place.where) ? <NotFound /> : notes === null ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : (
        <Section title={t("saves.noteTitle")} help={t("saves.notePageHelp")}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (isReplaced && !pending) {
                    save();
                }
            } }}>
                <Rows>
                    <TextRow id="sv-note" kind="longtext" label={t("saves.note")} help={t("saves.noteFieldHelp")} value={value}
                        onChange={(next) => setDraft(String(next || "").slice(0, NOTE_MAX))} props={{ maxLength: NOTE_MAX, rows: 5, disabled: pending }} />
                    <SubmitRow>
                        <FormButton id="sv-note-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} />
                    </SubmitRow>
                </Rows>
            </Form>
        </Section>
    );

    return (
        <SettingsPage title={t(saved ? "saves.noteEdit" : "saves.noteWrite")} parent={parent} levels={levels}>
            {body}
        </SettingsPage>
    );
}

// /settings/profile/saves/* (Settings.jsx).
export function MySavesRoutes() {
    return (
        <Routes>
            <Route index element={<SourcesPage />} />
            <Route path=":source" element={<SourcePage />} />
            <Route path=":source/game/:consoleId/:reference" element={<SaveGamePage />} />
            <Route path=":source/game/:consoleId/:reference/all" element={<SaveSlotsPage />} />
            <Route path=":source/game/:consoleId/:reference/slot/:slot" element={<SaveSlotsPage />} />
            <Route path=":source/game/:consoleId/:reference/slot/:slot/note" element={<NotePage />} />
            <Route path=":source/console/:consoleId" element={<SaveConsolePage />} />
            <Route path=":source/console/:consoleId/all" element={<SaveGamesPage />} />
            <Route path=":source/console/:consoleId/game/:gameId" element={<SaveGamesPage />} />
            <Route path=":source/console/:consoleId/game/:gameId/note" element={<NotePage />} />
            <Route path="*" element={<Navigate to={BASE} replace />} />
        </Routes>
    );
}
