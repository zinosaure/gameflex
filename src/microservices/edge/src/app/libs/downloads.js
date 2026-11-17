import { useSyncExternalStore } from "react";
import { explained, kernelCommand, subscribeTopic } from "./api.js";
import { can } from "./data.jsx";
import { followLink } from "./links.js";
import { handToBrowser } from "../engine/api/retrogaming.js";

// UN TÉLÉCHARGEMENT PAR LE NAVIGATEUR, JAMAIS DANS LA BIBLIOTHÈQUE (le porteur, 2026-11-16 : « Les téléchargement sont
// par navigateur et non directement dans la librairie ») — un jeu, l'archive de ses métadonnées (le Catalogue des
// Réglages) : le navigateur le range dans ses Téléchargements, avec sa barre et sa reprise. `url` : l'adresse d'un ticket
// à usage unique (ticketDownloadUrl, metadataArchiveUrl). SUR LE TÉLÉPHONE, SON NAVIGATEUR (chantier 56, T2 ;
// `nativeBrowse`) : la WebView ne télécharge pas, et elle consommerait le ticket — le kernel du téléphone demande à
// l'application de l'ouvrir dans Chrome (handToBrowser, `POST /device/browse`). Ailleurs, un vrai lien `<a download>`.
// Rend vrai quand le téléphone l'a confié à Chrome (la page le dit).
export async function browserDownload(url, capabilities) {
    if (can(capabilities, "nativeBrowse") && await handToBrowser(url)) {
        return true;
    }
    followLink(url, { download: "", rel: "noopener" });

    return false;
}

// LES TÉLÉCHARGEMENTS DE CE COMPTE, TELS QUE LE KERNEL LES GARDE (le sujet `downloads` de son canal, la salle partagée
// gameflex/facade) : ceux qui tournent, puis la file des jeux — leur rang et leur progression (le porteur, 2026-11-16 :
// « c'est possible de mettre les jeux à télécharger en queue ? »). LE KERNEL GARDE LA FILE : une page fermée puis
// rouverte la relit, et En direct montre aussi ce qu'une autre page a lancé. Un seul abonnement pour toute la page,
// ouvert par le premier lecteur, refermé avec le dernier. Une ligne : `{id, downloadId, key, label, source, state,
// position, received, total}` — `state` vaut `queued` (il attend son tour, `position` : 1 pour le prochain) ou
// `running` ; `key` : ce qu'on télécharge, `source` : d'où.
let current = [];
const listeners = new Set();
let stopFollowing = null;

const notify = () => listeners.forEach((listener) => listener());

// Un hôte qui ne sert pas le sujet (un téléphone d'une version d'avant) : aucune ligne, plutôt qu'une erreur.
const follow = () => subscribeTopic("downloads", (list) => {
    current = (Array.isArray(list) ? list : []).map((entry) => ({ ...entry, id: entry.downloadId }));
    notify();
}, () => {
    current = [];
    notify();
});

// UN TÉLÉCHARGEMENT EN TÂCHE DE FOND (chantier 53, P7 ; la file le 2026-11-16) : la commande `download` du canal du
// kernel (`command` : `{route, server?, disk?, section?, body, total?}`, facade_ws.go), avec ce que la liste en montrera
// (`key`, `label`, `source`) ; le kernel rend son identifiant aussitôt — un jeu prend la file, le reste part tout de
// suite au serveur ; au téléphone, tout attend son tour. Puis son sujet `download/<id>` : `queued` (son rang :
// `onQueued(rang)`, une fois, quand il attend derrière d'autres), `progress`, enfin `done` ou `failed`. Le geste tourne
// chez le kernel, détaché de la page : la quitter ne l'arrête pas. Rend la forme de `kernel` : `{ok, data,
// errorMessage}`, `data` étant ce qu'aurait rendu la route ; `cancelled` quand il a été annulé ou retiré de la file
// (cancelDownload) ; `pending` quand la même chose attendait déjà ou tournait (409).
export async function runDownload({ key, label, source, command, onQueued = null }) {
    const accepted = await kernelCommand("download", { ...command, key, label, source });
    const downloadId = accepted.data?.downloadId;

    if (!accepted.ok || !downloadId) {
        return { ok: false, pending: accepted.status === 409, data: null, errorMessage: accepted.errorMessage || "" };
    }

    return followDownload(downloadId, onQueued);
}

// LA FIN D'UNE TÂCHE SUIVIE DÉJÀ LANCÉE (un téléchargement, ou la suppression d'un jeu : DELETE …/roms/{ref} rend
// l'identifiant de la sienne) : son sujet `download/<id>` jusqu'à `done` ou `failed`. Même forme que runDownload.
export function followDownload(downloadId, onQueued = null) {
    return new Promise((resolve) => {
        let told = false;
        const stop = subscribeTopic(`download/${downloadId}`, (step) => {
            if (!step) {
                return;
            }

            if (step.type === "queued" && !told && step.position > 0) {
                told = true;
                onQueued?.(step.position);
            }

            if (step.type === "done" || step.type === "failed") {
                stop?.();
                resolve(step.type === "done" ? { ok: true, data: step.result || null, errorMessage: "" }
                    : { ok: false, cancelled: step.error === CANCELLED, data: null, errorMessage: step.error === CANCELLED ? "" : explained(step.errorCode || "", step.error || "") });
            }
        }, (reason) => resolve({ ok: false, data: null, errorMessage: explained("", reason) }));
    });
}

// ANNULER UN TÉLÉCHARGEMENT (le porteur, 2026-11-14, « 3. A »), OU LE RETIRER DE LA FILE (2026-11-16) : la commande
// `download.cancel` du canal du kernel, par le compte qui l'a lancé. Le kernel arrête la copie et retire le fichier
// partiel, ou sort le jeu de la file ; le sujet finit en `failed`, « cancelled » — runDownload rend alors `{ok: false,
// cancelled: true}`, et la ligne part. Rend la réponse de la commande (404 : il venait de finir).
const CANCELLED = "cancelled";

export const cancelDownload = (entry) => kernelCommand("download.cancel", { downloadId: entry.downloadId });

// L'AVIS DE FIN D'UN TÉLÉCHARGEMENT, le même partout où l'on télécharge : fini, annulé, déjà demandé, ou son échec et
// sa raison.
export const downloadOutcome = (answer, name, t) => (answer.ok ? t("consoles.downloaded", { name })
    : answer.cancelled ? t("downloads.cancelled", { name }) : answer.pending ? t("downloads.pending", { name })
        : answer.errorMessage || t("consoles.downloadFailed", { name }));

// L'avis d'un jeu qui attend son tour (onQueued de runDownload).
export const queuedNotice = (name, position, t) => t("downloads.queued", { name, position });

// Où en est un téléchargement, en un mot (la ligne d'un jeu dans une liste) : son rang dans la file, ou son pourcentage.
export const downloadState = (entry, t) => (!entry ? null : entry.state === "queued" ? t("downloads.queuedShort", { position: entry.position })
    : entry.total ? t("downloads.percent", { percent: Math.min(100, Math.round((entry.received / entry.total) * 100)) }) : t("downloads.running"));

const subscribe = (listener) => {
    listeners.add(listener);

    if (!stopFollowing) {
        stopFollowing = follow();
    }

    return () => {
        listeners.delete(listener);

        if (!listeners.size && stopFollowing) {
            stopFollowing();
            stopFollowing = null;
            current = [];
        }
    };
};

export const useDownloads = () => useSyncExternalStore(subscribe, () => current);
