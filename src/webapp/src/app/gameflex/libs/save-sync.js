// LES TRANSFERTS DE SAUVEGARDES, PORTÉS PAR LA PAGE (chantier 38, le porteur,
// 2026-10-29).
//
// Aucun kernel ne parle à un autre : la page lit d'un côté et écrit de
// l'autre, avec la session qu'elle a de chaque côté — la nôtre ici, celle de
// notre compte chez un serveur fédéré là-bas (`federationSession`). ET ELLE
// N'ÉCRIT QUE DANS UNE COPIE : « chaque upload qu'on fera sur des serveurs
// fédérés, ça ne va pas remplacer la sauvegarde principale, ça va juste aller
// dans le sous-dossier sources/{nom serveur} ». Envoyer range chez lui sous
// le nom que notre serveur annonce ; Télécharger range ici sous le nom que
// notre fédération lui donne. Remettre une copie dans le principal est un
// autre geste, sur place (`restoreStates`, `restoreStreamexSaves`).
//
// La règle de remplissage qui vivait ici (`planSlotTransfer`, 2026-10-16 :
// remplir les emplacements libres d'en face avant de demander) servait à
// écrire dans le principal d'un autre ; elle est partie avec lui.
import { federationSession } from "./federationSession.js";
import {
    fetchSaveStateFile,
    uploadSaveStateFile,
    fetchStreamexSave,
    uploadStreamexSave,
} from "../api/retrogaming.js";

/**
 * Un serveur fédéré comme destination ou comme source : son adresse, et la
 * session de notre compte chez lui. `{ok: false, code}` sans compte là-bas
 * (`noFederatedAccount`) ou quand il ne répond pas.
 */
export async function peerHost(server) {
    const opened = await federationSession(server.id);
    if (!opened.ok) {
        return { ok: false, code: opened.code };
    }

    return { ok: true, host: { origin: server.link, token: opened.session.token } };
}

/**
 * Un emplacement du rétro, de `from` vers la copie `to.source` de `to.host`
 * (`{host, source}` des deux côtés ; `host` nul : ici). L'état, puis sa
 * vignette quand il en a une. Vrai quand l'état est passé.
 */
export async function copySlot({ consoleId, reference, slot, title, timestamp }, from, to) {
    const state = await fetchSaveStateFile(consoleId, reference, slot, from.host, from.source);
    if (!state.ok) {
        return false;
    }
    const written = await uploadSaveStateFile(consoleId, reference, slot, state.blob,
        { host: to.host, source: to.source, timestamp, title });
    if (!written.ok) {
        return false;
    }
    const shot = await fetchSaveStateFile(consoleId, reference, slot, from.host, from.source, true);
    if (shot.ok) {
        await uploadSaveStateFile(consoleId, reference, slot, shot.blob,
            { host: to.host, source: to.source, timestamp, screenshot: true });
    }

    return true;
}

/** La sauvegarde d'un jeu d'une console récente, de `from` vers la copie `to.source`. */
export async function copyStreamexGame({ console, id }, from, to) {
    const archive = await fetchStreamexSave(console, id, from.source, from.host);
    if (!archive.ok) {
        return false;
    }
    const written = await uploadStreamexSave(console, id, archive.blob, to.source, to.host);

    return Boolean(written.ok);
}
