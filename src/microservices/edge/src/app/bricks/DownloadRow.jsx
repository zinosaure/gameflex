import React, { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { SUBTITLES } from "../globals/subtitles.jsx";
import { cancelDownload } from "../libs/downloads.js";
import { fact, formatSize } from "../libs/format.js";
import { focusElement } from "../libs/input.js";
import { toast } from "../shell/Shell.jsx";
import { Chip } from "./Chip.jsx";
import NavRow from "./NavRow.jsx";
import { SplitRow } from "./SplitRow.jsx";

// ANNULER UN TÉLÉCHARGEMENT EN COURS (le porteur, 2026-11-14, « 3. A »), OU LE RETIRER DE LA FILE (2026-11-16) : une
// ligne juste sous la ligne de ce qui télécharge, la même partout où l'on télécharge (un émulateur, un BIOS, un
// correctif, un jeu d'un serveur fédéré, En direct). Pas de fenêtre de confirmation : annuler ne détruit rien de ce qui
// était là. La commande `download.cancel` part (cancelDownload, libs/downloads.js) ; le kernel arrête la copie, ou sort
// le jeu de la file, le sujet finit « annulé », et la ligne de la source redevient elle-même — l'avis le dit
// (downloadOutcome). Un jeu qui attend son tour (`state: queued`) dit « Retirer de la file ». Partie avec lui, elle rend
// la lumière à la ligne de la source. `id` : sa clé, quand une page en montre plusieurs (En direct).
// Le geste « Annuler » / « Retirer de la file » d'un téléchargement (DownloadRow). `id` : la clé du bouton qui le porte.
export function useDownloadCancel(running, id) {
    const { t } = useTranslation();
    const [pending, setPending] = useState(false);
    const touched = useRef(false);

    // Touchée, elle se grise le temps de la réponse, et un bouton grisé perd la lumière : on retient qu'elle l'avait.
    useLayoutEffect(() => {
        const row = document.querySelector(`[data-key="${id}"]`);

        return () => {
            const source = row?.previousElementSibling;

            if (row && source && (row === document.activeElement || (touched.current && document.activeElement === document.body))) {
                requestAnimationFrame(() => source.isConnected && focusElement(source, true));
            }
        };
    }, [id]);

    const cancel = async () => {
        touched.current = document.activeElement?.dataset?.key === id;
        setPending(true);
        const answer = await cancelDownload(running);

        if (!answer.ok) {
            setPending(false);
            toast(answer.status === 404 ? t("downloads.alreadyOver") : answer.errorMessage || t("settings.actionFailed"));
        }
    };

    return { pending, cancel };
}

// UN TÉLÉCHARGEMENT SUR UNE LIGNE (En direct ; le porteur, 2026-11-17 : « la progression à gauche à la place de l'icône et
// à droite le bouton annuler […] actuellement sur 2 lignes ça fait très moche », et « pareil pour les téléchargements en
// attente ») : la pastille de gauche est sa progression (le pourcentage dans un anneau qui se remplit ; le tourniquet
// tant que le total n'est pas connu ; le sablier d'un jeu de la file), le sous-titre son ÉTAPE et « reçu sur total »
// (« Téléchargement », puis « Décompression » pour un jeu reçu en archive), et le bouton du bord droit « Annuler » ou
// « Retirer de la file ». La ligne elle-même n'ouvre rien. LA SEULE LIGNE D'UN TÉLÉCHARGEMENT, partout où il se voit (En
// direct, la page d'un jeu, d'un correctif, d'un émulateur) : un écart se définit une fois. `help` remplace le sous-titre.
export const PHASE_TEXTS = { download: "downloads.phaseDownload", unpack: "downloads.phaseUnpack", sheet: "downloads.phaseSheet", library: "downloads.phaseLibrary", copy: "downloads.phaseCopy", remove: "downloads.phaseRemove" };

// `onCancel` : un transfert que le navigateur mène lui-même (une sauvegarde d'un serveur fédéré, Saves.jsx) — la même
// ligne, son annulation à lui (le porteur, 2026-11-23 : « le download doit avoir la progression comme add to library »).
export function DownloadRow({ entry, id, label, help = null, onCancel = null, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey }) {
    const { t } = useTranslation();
    const queued = entry.state === "queued";
    const received = formatSize(entry.received) || "0 B";
    const subtitle = help ?? (queued ? fact(t("facts.queue"), t("facts.position", { position: entry.position }))
        : fact(t(PHASE_TEXTS[entry.phase] || "downloads.phaseDownload"), entry.total
            ? t("consoles.receivedOf", { received, total: formatSize(entry.total) }) : t("consoles.received", { received })));
    const { pending, cancel } = useDownloadCancel(entry, `${id}-cancel`);
    const percent = !queued && entry.total ? Math.min(100, Math.round((entry.received / entry.total) * 100)) : null;
    const lead = queued ? <Chip icon="bi-hourglass-split" tone="slate" />
        : percent === null ? <span className="gf-chip tone-teal" aria-hidden="true"><span className="spinner-border spinner-border-sm" role="status"></span></span>
            : <span className="gf-chip gf-chip-progress tone-teal" style={{ "--p": percent }} role="progressbar" aria-label={label}
                aria-valuenow={percent} aria-valuemin="0" aria-valuemax="100"><span>{percent}</span></span>;

    // UNE SUPPRESSION (`kind: remove`) ne s'annule pas : un jeu à moitié effacé ne servirait plus. Sa ligne n'a pas de bouton.
    if (entry.kind === "remove") {
        return <NavRow id={id} lead={lead} label={label} help={subtitle} opens="action" separator={separator} noKey={noKey} />;
    }

    return (
        <SplitRow id={id} lead={lead} label={label} help={subtitle} separator={separator} noKey={noKey} onOpen={() => { }}
            // Annuler, c'est arrêter : la sorte « danger » (le porteur, 2026-11-17 : « Le bouton annuler du téléchargement n'est
            // pas danger mais primary »).
            action={{ id: `${id}-cancel`, kind: "danger", icon: queued ? "bi-dash-lg" : "bi-x-lg", label: t(queued ? "downloads.dequeue" : "downloads.cancel"),
                // Le téléchargement fini, on ne revient plus en arrière : la décompression et la fiche ne s'annulent pas (le
                // porteur, 2026-11-17 : « Il faut aussi grisé le bouton annuler après le téléchargement »).
                pending: onCancel ? false : pending, disabled: (!onCancel && !entry.downloadId) || (entry.phase && entry.phase !== "download"), onClick: onCancel || cancel }} />
    );
}
