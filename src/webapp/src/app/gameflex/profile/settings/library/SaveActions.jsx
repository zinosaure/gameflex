// LES GESTES SUR DES SAUVEGARDES, selon d'où on les regarde (chantier 38, le
// porteur, 2026-10-29) :
//
//   le principal (« Sur cet appareil ») — Backup, Envoyer vers… ;
//   une copie (les backups, ou celle d'un serveur fédéré) — Restaurer, Supprimer ;
//   un serveur fédéré — Télécharger.
//
// LES MÊMES POUR TOUT CE QU'ON PEUT CHOISIR : un emplacement du rétro ou tous
// ceux d'un jeu (`slots`), un jeu d'une console récente ou toute la console
// (`games`). L'appelant passe la liste ; une seule écriture des gestes, sinon
// un écran finit par en oublier un.
//
// Envoyer et Télécharger n'écrivent que dans une COPIE de l'autre côté
// (libs/save-sync.js) : rien ne remplace des sauvegardes. Seul Restaurer écrit
// dans le principal, ici, après avoir mis le sien dans les backups.
import React, { useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { FormButton } from "../../../shared/templates/Essentials.jsx";
import { sourceOf } from "../../../libs/sources.js";
import { peerHost, copySlot, copyStreamexGame } from "../../../libs/save-sync.js";
import {
    listRoms,
    backupStates,
    restoreStates,
    deleteSaveSlot,
    backupStreamexSaves,
    restoreStreamexSaves,
    deleteStreamexSave,
} from "../../../api/retrogaming.js";

const Form = Skeleton.Form;

// Ici, le principal.
const HERE = { host: null, source: "" };

// Une réponse `{results: [{error}]}` : combien ont échoué.
const failuresOf = (answer) => (answer.ok ? (answer.data?.results || []).filter((result) => result.error).length : 1);

/**
 * La référence d'ICI d'un jeu du rétro qu'une copie range sous la sienne : la
 * même, puis le même titre dans la bibliothèque. Une copie venue d'ailleurs
 * peut porter un jeu qu'ici n'a pas — rien à restaurer alors : la sauvegarde
 * irait sous un jeu que la bibliothèque ne montre pas.
 */
async function localReference({ consoleId, reference, title }, source) {
    const found = await listRoms(consoleId, title && title !== reference ? title : "");
    const rows = found.ok ? (found.data || []).filter((row) => sourceOf(row).source === "local") : [];
    const match = rows.find((row) => row.reference === reference) || rows.find((row) => title && row.title === title);
    if (match) {
        return match.reference;
    }

    // NOS PROPRES BACKUPS viennent d'ici : leur référence est la nôtre, même
    // pour un jeu d'un disque interne que la bibliothèque ne liste pas.
    return source === "local" ? reference : null;
}

/**
 * `where` — `{kind: "principal" | "copy" | "peer", source, host, server}`.
 * `peers` — les serveurs fédérés (`{id, name, link}`), pour Envoyer.
 * `selfName` — le nom que ce serveur annonce : celui de la copie qu'un envoi
 * crée chez l'autre. `label` — ce que la question de confirmation nomme.
 * `canDelete` — Supprimer, pour une copie, et pour un emplacement du principal
 * (le geste qu'avait déjà ce panneau).
 */
export function SaveActions({ where, peers = [], selfName = "", slots = null, games = null, label = "", canDelete = false, t, context, onDone = null }) {
    const [isPicking, setPicking] = useState(false);
    const [chosen, setChosen] = useState({ servers: [] });
    const [isBusy, setBusy] = useState(false);
    const items = slots || games || [];

    // `done` — ce qu'on dit quand tout a marché : « Copié. », « Restauré. »,
    // « Supprimé. ».
    const finish = (failed, message = null, done = "profile.saveStates.syncDone") => {
        context.toast.open(message || t(failed ? "profile.saveStates.syncFailed" : done),
            { variant: failed ? "error" : "success" });
        if (onDone) {
            onDone();
        }
    };

    const run = async (work) => {
        setBusy(true);
        try {
            await work();
        } catch (failure) {
            console.error("sauvegardes : le geste a échoué", failure);
            finish(1);
        } finally {
            setBusy(false);
        }
    };

    const copy = (item, from, to) => (slots ? copySlot(item, from, to) : copyStreamexGame(item, from, to));

    const backup = () => run(async () => {
        const answer = slots
            ? await backupStates(slots.map(({ consoleId, reference, slot }) => ({ consoleId, reference, slot })))
            : await backupStreamexSaves(games[0].console, games.map((game) => game.id));
        finish(failuresOf(answer));
    });

    // ENVOYER : dans la copie qui porte notre nom, chez chaque serveur choisi,
    // sous le compte qu'on y a. Sans compte là-bas, on le dit : c'est en le
    // choisissant comme source qu'on colle son jeton.
    const send = () => run(async () => {
        let failed = 0;
        for (const id of chosen.servers) {
            const server = peers.find((peer) => String(peer.id) === String(id));
            const opened = server ? await peerHost(server) : { ok: false };
            if (!opened.ok) {
                context.toast.open(t("profile.saveStates.noAccountThere", { name: server?.name || id }), { variant: "error" });
                failed += 1;

                continue;
            }
            for (const item of items) {
                if (!await copy(item, HERE, { host: opened.host, source: selfName })) {
                    failed += 1;
                }
            }
        }
        setPicking(false);
        setChosen({ servers: [] });
        finish(failed);
    });

    // TÉLÉCHARGER : dans notre copie de ce serveur, sous le nom que notre
    // fédération lui donne.
    const download = () => run(async () => {
        let failed = 0;
        for (const item of items) {
            if (!await copy(item, { host: where.host, source: "" }, { host: null, source: where.server.name })) {
                failed += 1;
            }
        }
        finish(failed);
    });

    const restore = () => context.modal.open(t("profile.saveStates.restoreConfirm", { title: label }), {
        variant: "confirm",
        title: t("profile.saveStates.restore"),
        confirmLabel: t("profile.saveStates.restore"),
        cancelLabel: t("app.cancel"),
        onConfirm: () => run(async () => {
            if (games) {
                const answer = await restoreStreamexSaves(where.source, games[0].console, games.map((game) => game.id));
                if (answer.status === 409) {
                    finish(1, t("profile.saveStates.gameRunning"));

                    return;
                }
                finish(failuresOf(answer), null, "profile.saveStates.restored");

                return;
            }
            const restorable = [];
            for (const item of slots) {
                const toReference = await localReference(item, where.source);
                if (!toReference) {
                    context.toast.open(t("profile.saveStates.notHere", { title: item.title }), { variant: "error" });

                    continue;
                }
                restorable.push({ consoleId: item.consoleId, reference: item.reference, toReference, slot: item.slot });
            }
            if (restorable.length > 0) {
                finish(failuresOf(await restoreStates(where.source, restorable)) + slots.length - restorable.length,
                    null, "profile.saveStates.restored");
            }
        }),
    });

    const remove = () => context.modal.open(t("profile.saveStates.deleteConfirm", { title: label }), {
        variant: "confirm",
        title: t("app.delete"),
        confirmLabel: t("app.delete"),
        cancelLabel: t("app.cancel"),
        onConfirm: () => run(async () => {
            let failed = 0;
            for (const item of items) {
                const answer = slots
                    ? await deleteSaveSlot(item.consoleId, item.reference, item.slot, where.source)
                    : await deleteStreamexSave(item.console, item.id, where.source);
                if (!answer.ok) {
                    failed += 1;
                }
            }
            finish(failed, null, "profile.saveStates.deleted");
        }),
    });

    const button = (key, variant, text, onClick) => (
        <bs.Button key={key} variant={variant} className="flex-fill" disabled={isBusy} onClick={onClick}>
            {text}
        </bs.Button>
    );
    const buttons = {
        principal: [
            button("backup", "secondary", t("profile.saveStates.backup"), backup),
            peers.length > 0 && button("send", "primary", t("profile.saveStates.send"), () => setPicking(!isPicking)),
        ],
        copy: [button("restore", "warning", t("profile.saveStates.restore"), restore)],
        peer: [button("download", "primary", t("profile.saveStates.download"), download)],
    }[where.kind] || [];
    if (canDelete && where.kind !== "peer") {
        buttons.push(button("delete", "danger", t("app.delete"), remove));
    }

    return (
        <div>
            <div className="d-flex gap-2">{buttons.filter(Boolean)}</div>
            {isBusy ? (
                <div className="d-flex align-items-center gap-2 mt-2 text-muted small">
                    <bs.Spinner animation="border" size="sm" />
                    <span>{t("profile.saveStates.working")}</span>
                </div>
            ) : null}
            {isPicking ? (
                <div className="mt-3">
                    <Form.Group label={t("profile.saveStates.sendPick")} helpText={t("profile.saveStates.sendHelp")}>
                        <Form.Multicheckbox
                            name="servers"
                            context={{ values: chosen, setValues: (values) => setChosen(values) }}
                            options={peers.map((peer) => ({ value: String(peer.id), label: peer.name }))} />
                    </Form.Group>
                    <FormButton
                        callback={send}
                        label={t("profile.saveStates.sendNow")}
                        isDisabled={isBusy || (chosen.servers || []).length === 0}
                        isPending={isBusy} />
                </div>
            ) : null}
        </div>
    );
}
