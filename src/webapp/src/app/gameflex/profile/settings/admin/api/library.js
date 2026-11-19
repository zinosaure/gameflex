// « Ma bibliothèque » — ce qui est ICI, et rien d'autre.
//
// Les trois écrans (Jeux, Coeurs, BIOS) posent la même question à deux hôtes
// très différents : un serveur, qui a un administrateur et un disque, et un
// téléphone, dont la place est celle qu'on a dans la poche. Ce fichier est
// l'endroit où cette différence se règle UNE fois, pour que les écrans
// n'aient pas à la connaître.
//
// Ce qui change entre les deux, et c'est tout :
//
//   la source     le kernel du serveur, ou celui de l'appareil
//   la suppression  possible sur l'appareil SEULEMENT (décision 2026-09-01)
//
// Les jeux DISTANTS (fédération, SMB…) ne figurent nulle part ici : « ma
// bibliothèque » désigne ce qu'on possède. Les jeux des autres se voient
// dans la liste des jeux d'une console, où ils portent leur origine.
import {
    listRoms,
    listConsoles,
    deleteRom,
} from "../../../../api/retrogaming.js";
import {
    hasLocalLibrary,
    deviceRemoveGame,
    rescanLocalLibrary,
} from "../../../../libs/localLibrary.js";
import { fetchPeerRoms } from "./storage-admin.js";

/**
 * Les consoles ACTIVES de cet hôte : celles pour lesquelles il a des jeux.
 *
 * Le registre en connaît une quarantaine ; en proposer quarante dans un
 * filtre revient à ne pas filtrer. Ce qui compte est ce qu'on a réellement —
 * et c'est aussi la bonne question pour les coeurs et les BIOS : « ai-je ce
 * qu'il faut pour les consoles dont je possède des jeux ? »
 *
 * RELUE À CHAQUE FOIS, plus retenue (2026-10-14). « La réponse ne change pas
 * pendant qu'on navigue » était faux : un jeu téléchargé, importé ou trouvé
 * par un nouveau parcours ajoute sa console. Retenue pour la vie de la page,
 * elle laissait Réglages › Jeux sur les consoles de son premier affichage —
 * NES, PS3 et PSP sur le S25 du porteur, qui ne voyait ni sa Switch, ni sa
 * Wii U, ni sa Xbox, même après « Rescan all ». Une seule requête, légère.
 */
export async function activeConsoleIds() {
    // Les deux hôtes servent `/api/v1/retrogaming/consoles` avec le compte de
    // jeux par console (chantier 8, C8-P2) — il n'y a plus deux façons de
    // savoir ce qu'on a.
    const answer = await listConsoles();

    return answer.ok
        ? (answer.data || []).filter((entry) => entry.count > 0).map((entry) => entry.id)
        : [];
}

// Le nom d'une console ne se demande pas ici : ConsolesContext le sert déjà
// aux deux hôtes (registre du serveur, ou module Go embarqué sur
// l'appareil), et les écrans l'ont sous la main via useConsoles(). Une
// seconde source aurait fini par ne pas dire la même chose.

// ── Les jeux ────────────────────────────────────────────────────────────

/**
 * Les jeux de ces consoles, ceux d'ICI uniquement.
 *
 * Une requête par console — c'est la forme des routes, et le filtre de
 * l'écran est justement une liste de consoles. Les deux hôtes la servent
 * pareil depuis C8-P2 ; ce qui vient d'ailleurs est écarté ici, une fois.
 */
export async function fetchRoms(consoleIds, query = "", serverId = null, disk = null) {
    const wanted = consoleIds || [];

    // CHEZ UN PAIR : un seul appel, qui rend toutes ses consoles. Sa liste n'a
    // pas de champ `kind` — le pair ne relaie pas les jeux d'un tiers.
    if (serverId) {
        const answer = await fetchPeerRoms(serverId);
        const needle = (query || "").trim().toLowerCase();

        return (answer.consoles || [])
            .filter((section) => wanted.length === 0 || wanted.includes(section.consoleId))
            .flatMap((section) => (section.roms || []).map((rom) => ({
                consoleId: section.consoleId,
                reference: rom.reference,
                title: rom.title,
                sizeBytes: rom.sizeBytes || 0,
                // LES FICHIERS DU JEU, portés jusqu'ici parce que le
                // rapatriement en dépend : un jeu d'un seul fichier a besoin
                // de son extension pour être rangé (sans « .nes », aucun cœur
                // ne le charge), et plusieurs fichiers annoncent un dossier.
                files: rom.files || [],
                // Et sa FORME : un dossier d'un seul `.iso` (tous les jeux
                // PSP d'ici) arrive zippé quand même, et doit se déplier.
                folder: Boolean(rom.folder),
                // Son statut CHEZ L'AMI (chantier 30), établi sur sa machine.
                status: rom.status || null,
            })))
            // La recherche se fait ici : le relais ne lit pas de `?q=`.
            .filter((rom) => !needle || rom.title.toLowerCase().includes(needle));
    }

    const pages = await Promise.all(wanted.map((consoleId) => listRoms(consoleId, query, null, null, disk)));

    return pages.flatMap((page, index) => {
        const consoleId = wanted[index];

        // La bibliothèque d'ici, et elle seule : les deux kernels n'y mêlent
        // plus rien (chantier 14).
        return (page.ok ? page.data || [] : [])
            .map((rom) => ({
                consoleId,
                reference: rom.reference,
                title: rom.title,
                sizeBytes: rom.sizeBytes || 0,
                // Le disque interne d'où vient la ligne (chantier 23) : c'est
                // SA copie que la corbeille efface.
                disk: rom.disk || null,
                // Son statut (chantier 30) : le badge, et le panneau qui le
                // règle.
                status: rom.status || null,
            }));
    });
}

/**
 * Retirer un jeu — sur l'appareil, ou sur le serveur quand on l'administre.
 *
 * DEUX CHEMINS, parce que ce ne sont pas les mêmes disques : l'appareil passe
 * par son kernel embarqué et doit relire son scan (retenu en mémoire, sinon le
 * jeu reste affiché), le serveur par une route qui résout la référence dans
 * son catalogue — lequel se rafraîchit tout seul sur l'âge des dossiers.
 *
 * Rend une erreur, ou rien. La forme vient de `deviceRemoveGame`, que trois
 * écrans lisent déjà ainsi.
 */
export async function removeRom(item) {
    if (!hasLocalLibrary()) {
        const answer = await deleteRom(item.consoleId, item.reference, item.disk || null);

        return answer.ok ? null : (answer.errorMessage || "suppression refusée");
    }

    const error = await deviceRemoveGame(item.reference, item.disk || null);
    if (!error) {
        await rescanLocalLibrary();
    }

    return error;
}

// ── La taille, telle qu'on la lit ───────────────────────────────────────

export function formatSize(bytes) {
    if (!bytes) {
        return "—";
    }

    const units = ["B", "KB", "MB", "GB"];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }

    return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}
