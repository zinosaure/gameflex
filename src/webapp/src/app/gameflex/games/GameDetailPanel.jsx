import React, { useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import {
    hasLocalLibrary,
    isRemoteSource,
    sourceOf,
    deviceRemoveGame,
    rescanLocalLibrary,
    onLocalLibraryReloaded,
} from "../libs/localLibrary.js";
import { parseFederatedId } from "../libs/federatedHost.js";
import { GameStatusDetails } from "./GameStatus.jsx";
import {
    listRoms,
    downloadTicket,
    handToBrowser,
    ticketDownloadUrl,
} from "../api/retrogaming.js";
import { useAuth } from "../profile/auth/AuthContext.jsx";
import { SectionPanel, OffCanvas } from "../shared/templates/Essentials.jsx";
import { GameSource } from "./GameSource.jsx";
import { formatSize } from "./helpers.js";

const Form = Skeleton.Form;

// ── Offcanvas : la fiche d'un jeu ──────────────────────────────────────────
//
// PLUS DE « JOUER » (chantier 52, P8, décision du porteur du 2026-11-13) : la page de jeu est celle du
// launcher, et la webapp n'en a plus — l'émulateur, le BIOS, les réglages du flux et la décision de
// lancement sont partis avec elle. La fiche dit d'où vient le jeu, son moteur et son statut, et le
// télécharge (ou retire la copie du téléphone, pour son propriétaire).
export function GameDetailPanel({
    rom,
    consoleId,
    consoleMeta,
    t,
    context,
}) {
    const { playsHere } = useAuth();
    // EN TÊTE : les effets plus bas le lisent dans leurs tableaux de dépendances.
    const onDevice = hasLocalLibrary();

    // LA COPIE LOCALE, indépendamment de la source affichée.
    //
    // Elle répond à « ce jeu est-il déjà dans la bibliothèque d'ici ? », par
    // l'UUID (chantier 14). Pour un jeu d'ici, elle pilote le bouton
    // télécharger/retirer. Pour celui d'un serveur fédéré, elle ne change RIEN
    // à ce qui part : un jeu de la liste d'un ami ne se lance que chez lui (le
    // porteur, 2026-10-13) ; elle rend seulement juste le refus d'« Ici »
    // (`also-in-library` plutôt que « pas dans la bibliothèque »).
    // La prop date de l'ouverture du panneau et ne peut pas connaître une
    // copie posée depuis : on la cherche donc nous-mêmes, à chaque relecture
    // de la bibliothèque (signalé le 2026-09-02).
    const [localCopy, setLocalCopy] = useState(null);

    const [isDownloading, setIsDownloading] = useState(false);
    const [isRemoving, setIsRemoving] = useState(false);
    // Un jeu déjà chez soi n'a rien à retélécharger : sur un appareil, le
    // bouton se ferme au lieu de proposer une copie qui ne servira jamais.
    //
    // Réévalué à chaque changement de bibliothèque, pas seulement à
    // l'ouverture : le jeu peut ENTRER pendant que la fiche est ouverte —
    // c'est même le cas normal, puisque c'est cette fiche qui l'a demandé.
    // « Déjà chez moi » n'est plus un état à tenir à jour : c'est la copie
    // locale, ou son absence. Deux états pour une seule vérité finissent
    // toujours par ne plus dire la même chose.
    const alreadyLocal = Boolean(localCopy);
    // L'UUID DU JEU, sans le préfixe d'un serveur fédéré (`srv<id>:<uuid>`) :
    // c'est par lui, et plus par le titre, qu'une copie d'ici se reconnaît
    // (chantier 14). Un nom écrit autrement est un autre jeu.
    const gameId = parseFederatedId(rom.reference)?.id || rom.reference;

    useEffect(() => {
        let cancelled = false;
        // Même objet tant que rien n'a changé : sans cette garde, chaque
        // relecture rend une référence neuve et relance le rendu.
        const settle = (found) => {
            if (!cancelled) {
                setLocalCopy((current) => (
                    current?.reference === found?.reference ? current : found
                ));
            }
        };

        // SUR LE SERVEUR, un jeu d'ici est sa propre copie ; un jeu fédéré se
        // cherche dans la liste d'ici, une fois, à l'ouverture de la fiche.
        if (!onDevice) {
            if (!parseFederatedId(rom.reference)) {
                settle(sourceOf(rom).source === "local" ? rom : null);

                return () => {
                    cancelled = true;
                };
            }
            listRoms(consoleId)
                .then((answer) => settle(
                    (answer.ok ? answer.data || [] : []).find((game) => game.reference === gameId) || null,
                ))
                .catch(() => settle(null));

            return () => {
                cancelled = true;
            };
        }

        // SUR LE TÉLÉPHONE, LA MÊME LISTE QU'AU SERVEUR (chantier 15) : celle
        // de son listing, que le kernel refait avant d'annoncer qu'un jeu est
        // entré ou sorti — la fiche la relit alors. Elle ne lisait plus un
        // scan gardé par la page (`/api/v1/library/games`, supprimée). La
        // garde reste : un jeu d'un partage s'était reconnu LUI-MÊME comme
        // présent, et la fiche proposait de retirer ce qui n'y avait jamais
        // été (2026-09-02).
        const look = () => listRoms(consoleId)
            .then((answer) => settle((answer.ok ? answer.data || [] : []).find(
                (game) => !isRemoteSource(game.kind) && game.reference === gameId,
            ) || null))
            .catch(() => settle(null));

        look();

        // Le signal « RELUE », pas « changée » : entre les deux il y a une
        // lecture, et répondre trop tôt fait relire l'ancien cache. Le jeu
        // venait d'entrer dans la bibliothèque et la fiche n'en savait
        // rien — il fallait la fermer et la rouvrir (signalé le 2026-09-02).
        // Le menu des consoles avait eu exactement ce défaut.
        const unsubscribe = onLocalLibraryReloaded(look);

        return () => {
            cancelled = true;
            unsubscribe();
        };
    }, [onDevice, consoleId, gameId]);

    // LE NAVIGATEUR TÉLÉCHARGE, PLUS LA PAGE.
    //
    // Elle le faisait elle-même — `fetch` avec l'en-tête, puis un blob remis au
    // navigateur — parce qu'un lien ne peut pas porter d'`Authorization` et que
    // toute route `/api/v1/retrogaming` en exige un. Ça marchait, et ça perdait
    // tout ce que le navigateur sait faire : sa barre de progression, la
    // reprise après coupure, le dossier Téléchargements — et un jeu de
    // plusieurs gigaoctets passait d'abord ENTIER par la mémoire de l'onglet.
    //
    // On demande donc un TICKET, dont l'émission est authentifiée, et on ouvre
    // l'adresse qu'il déverrouille. Le ticket ne porte aucune identité, ne vaut
    // que pour CE jeu, expire en une minute et se consomme au premier usage :
    // voir `retro_download_ticket.go`, qui dit pourquoi c'est un ticket tenu
    // par le serveur et non une signature rejouable.
    //
    // UN VRAI LIEN `<a download>` (chantier 14, demandé par le porteur) : c'est
    // l'utilisateur qui décide, dans son navigateur, où le jeu atterrit. Le nom
    // vient du serveur (`Content-Disposition`), d'où un `download` vide. Un jeu
    // d'un serveur fédéré passe par notre kernel, qui le relaie : le jeton de
    // ce serveur ne sort pas d'ici.
    const handleDownload = async () => {
        setIsDownloading(true);
        try {
            const answer = await downloadTicket(consoleId, rom.reference);
            const ticket = answer?.data?.ticket;
            if (!answer?.ok || !ticket) {
                throw new Error("no ticket");
            }
            const url = ticketDownloadUrl(ticket);
            // SUR LE TÉLÉPHONE, SON NAVIGATEUR : la WebView ne télécharge
            // pas, et elle consommerait le ticket (`handToBrowser`).
            if (await handToBrowser(url)) {
                context.toast.open(t("retrogaming.detail.downloadInBrowser"), { variant: "success" });

                return;
            }
            const link = document.createElement("a");
            link.href = url;
            link.download = "";
            link.rel = "noopener";
            document.body.appendChild(link);
            link.click();
            link.remove();
        } catch {
            context.toast.open(t("retrogaming.detail.downloadFailed"), { variant: "error" });
        } finally {
            setIsDownloading(false);
        }
    };

    const handleRemoveLocal = () => {
        context.modal.open(t("retrogaming.detail.removeConfirm"), {
            variant: "confirm",
            title: rom.title,
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                setIsRemoving(true);
                try {
                    const error = await deviceRemoveGame(rom.reference);
                    if (error) {
                        context.toast.open(t("retrogaming.detail.removeFailed"), { variant: "error" });

                        return;
                    }
                    // La relecture prévient la fiche elle-même (voir l'effet
                    // plus haut) : la copie locale disparaît, donc le bouton
                    // et le sélecteur suivent. Le faire aussi à la main
                    // donnait deux chemins pour la même vérité.
                    await rescanLocalLibrary();
                    context.toast.open(t("retrogaming.detail.removed"), { variant: "success" });
                } catch {
                    context.toast.open(t("retrogaming.detail.removeFailed"), { variant: "error" });
                } finally {
                    setIsRemoving(false);
                }
            },
        });
    };

    // `handleAddToLibrary` a été SUPPRIMÉE le 2026-09-18. Elle faisait ranger un
    // jeu distant dans la LUDOTHÈQUE par le kernel (route `/import`), depuis
    // l'écran d'un joueur — or la ludothèque est partagée et un rôle `user` n'a
    // rien à y écrire. Un téléchargement va chez la personne, jamais dans la
    // bibliothèque ; c'est un administrateur qui l'y ajoute, par Réglages ›
    // Jeux (chantier 14).

    // RETIRER écrit sur la machine qui tient le jeu : c'est au propriétaire du
    // téléphone seul (`playsHere`), pour une copie qui est sur le téléphone.
    // Tous les autres TÉLÉCHARGENT — un membre, un invité, le serveur : le
    // fichier va dans leur navigateur, rien ne s'écrit ici (chantier 14).
    // Pour un jeu d'ICI seulement : la liste d'un serveur fédéré ne fait que
    // télécharger, même quand le même fichier est aussi sur le téléphone.
    const removable = onDevice && alreadyLocal && playsHere && sourceOf(rom).source === "local";
    const downloadLabel = removable
        ? t("retrogaming.detail.remove")
        : isDownloading
            ? t("retrogaming.detail.downloading")
            : `${t("retrogaming.detail.downloadRom")} (${formatSize(rom.sizeBytes)})`;

    return (
        <OffCanvas
            icon="bi bi-controller"
            label={rom.title}
            helpText={consoleMeta?.publisher + " · " + consoleMeta?.name}
        >
            <SectionPanel>
                <Form.Group label={t("retrogaming.origin.title")} helpText={t("retrogaming.origin.titleHelp")}>
                    <GameSource rom={rom} t={t} />
                </Form.Group>
                {/* LE MOTEUR DU JEU (chantier 36), reconnu à ses fichiers au
                    scan — absent quand il ne l'est pas : rien à en dire. */}
                {rom.engine && (
                    <Form.Group label={t("retrogaming.engine.title")} helpText={t("retrogaming.engine.titleHelp")}>
                        <div className="d-flex align-items-center gap-2">
                            <i className="bi bi-cpu"></i>
                            <span>{t(`retrogaming.engine.names.${rom.engine}`)}</span>
                        </div>
                    </Form.Group>
                )}
                <Form.Group>
                    {/* UN SEUL BOUTON, qui fait l'inverse une fois le jeu là.
                        Il y en avait deux empilés : « déjà dans la
                        bibliothèque », grisé et muet, puis « retirer » —
                        c'est-à-dire un bouton qui occupe la place sans rien
                        offrir, au-dessus de celui qui agit. Un jeu est
                        dedans ou dehors ; le bouton dit lequel des deux
                        gestes reste possible (signalé le 2026-09-01).

                        UN TÉLÉCHARGEMENT VA CHEZ LA PERSONNE, jamais dans la
                        bibliothèque — « un user normal peut télécharger sans
                        problème mais ça ne va pas dans la bibliothèque »
                        (2026-09-18) ; un jeu y entre par Réglages › Jeux
                        (chantier 14). */}
                    <bs.Button
                        className="w-100 d-inline-flex align-items-center justify-content-center gap-2"
                        variant={removable ? "danger" : "secondary"}
                        onClick={removable ? handleRemoveLocal : handleDownload}
                        disabled={isDownloading || isRemoving}
                        title={removable
                            ? t("retrogaming.detail.removeHelp")
                            : t("retrogaming.detail.download")}>
                        {removable && <i className="bi bi-trash"></i>}
                        {downloadLabel}
                    </bs.Button>
                </Form.Group>
            </SectionPanel>

            {/* LE STATUT DU JEU (chantier 30) : par émulateur et ses notes,
                tel que le serveur du jeu l'a établi — sans version. */}
            <GameStatusDetails status={rom.status} t={t} />
        </OffCanvas>
    );
}
