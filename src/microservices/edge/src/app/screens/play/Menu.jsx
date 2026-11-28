import { useConsoles } from "../../libs/data.jsx";
import React, { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { isOwnSession, nameIn } from "../../libs/SessionPool.js";
import { useNames } from "../../libs/displayNames.js";
import { can, capabilities } from "../../libs/capabilities.js";
import { canInvite, shareOrigin, streamHostOrigin } from "../../libs/serverBase.js";
import { copyTextToClipboard, canShareLink, qrImage, shareLink } from "../../libs/share-link.js";
import { deleteSaveSlot, fetchStateScreenshot, listMembers, listSaveSlots, playedHost, sendInvite } from "../../libs/api.js";
import { dateTime, fact, formatDuration } from "../../libs/format.js";
import { focusElement } from "../../libs/input.js";
import { setHapticFeedback, setInvisibleSticks, setRumbleEnabled, useGamepadPreferences } from "../../engine/libs/gamepadPreferences.js";
import { useDevicePads } from "../../engine/libs/gamepadPresence.js";
import { DEFAULT_PAD_OPACITY, setPadOpacity, usePadOpacity } from "../../engine/stream/onscreen/padOpacity.js";
import i18n from "../../i18n.js";
import SidePanel from "../../shell/SidePanel.jsx";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import { slotName } from "../library/GameSettings.jsx";
import { Avatar } from "../../bricks/Avatar.jsx";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { CopyField } from "../../bricks/CopyField.jsx";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import { EmptyState } from "../../bricks/EmptyState.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { RecentNotices, RequestRow } from "../../bricks/RecentNotices.jsx";
import { InvisibleSticksRow } from "../../bricks/InvisibleSticksRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { OrientationFailed } from "../../bricks/OrientationFailed.jsx";
import { offersOrientation, OrientationRows } from "../../bricks/OrientationRows.jsx";
import { PadOpacityRow } from "../../bricks/PadOpacityRow.jsx";
import { QuickTiles } from "../../bricks/QuickTiles.jsx";
import { RadioRow } from "../../bricks/RadioRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SelectRow } from "../../bricks/SelectRow.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";

// Le sens de chaque groupe du menu : ce qui se règle (modifier), qui joue (info), inviter (ajouter), sortir (danger).
const GROUP_SENSES = { game: "play", sound: "modify", picture: "modify", pad: "modify", players: "info", pads: "modify", together: "add", leave: "danger" };

// LE MENU D'UNE PARTIE (#408, #409 ; chantier 52, P3) — un panneau du launcher, la moitié de l'écran en
// paysage, tout l'écran en portrait, en direction C (validée par le porteur le 2026-11-13, les modèles de
// docs/reports/2026-11-13-launcher-modeles/) : des tuiles en tête (Reprendre, Manette à l'écran, Son du jeu,
// Plein écran ; les sauvegardes d'état dans « La partie », le retour et l'avance rapides à l'écran depuis le chantier 69), puis les groupes du regroupement proposé — La partie, La
// manette (ce que touche le pouce de CET appareil), À plusieurs (ce qui concerne d'autres personnes : la manette
// qui passe de main en main, le partage), L'image et le son (les mesures à l'écran en un choix à trois cases, qui
// dit qu'elles s'excluent), Sortir (remettre à zéro et quitter, à part, en bas). Il s'ouvre au bouton rond, par
// L2 + Home tenus trois secondes, ou par Échap ; B ou Échap le referment. Une entrée qui a un sous-écran l'ouvre
// dans un second panneau ; un interrupteur se règle sur place, le menu reste ouvert. UN BOUTON QUI ACHÈVE UN GESTE —
// une tuile qui règle, une ligne qui agit — FERME LE MENU juste après (le porteur, 2026-11-14 : « quand on clique sur les
// boutons en fin d'action. Tu peux faire en sorte qu'il ferme le menu juste après ? »), pour qu'on voie l'effet sur le jeu.
// LA TUILE « REPRENDRE » DIT LE TEMPS DE JEU (le porteur : « rajouter le temps de jeux dans la case "Resume" ») : depuis
// le début de la partie (`startedAt`), à la seconde.
//
// Ce qu'il propose vient du moteur, jamais d'une liste tenue ici — « absent = pas d'entrée », la règle
// du menu de la webapp (FloatingPanel.jsx) : `input.extraProps` dit ce que la plateforme sait faire
// (sauvegardes, disques, triches, remise à zéro, clavier, voyage dans le temps), l'hôte ce qu'il permet
// (`can`), le child qui tient quoi (la place, le prêt, qui regarde).
//
// Les pouvoirs du PROPRIÉTAIRE qui JOUE (sauvegardes, triches, retour, disque, remise à zéro) : un invité
// ne réécrit pas les sauvegardes d'un autre, et qui regarde ne rembobine pas le jeu sous les joueurs.
//
// REGARDER SANS COMPTE (`watchOnly`, `#747` ; le porteur, 2026-11-16 : « Ce watch ne permet que de voir. Le menu à le
// strict minimum comme information, son, Fullscreen, inviter d'autres personnes, (qualité vidéo si on peut) et
// quitter. ») : le même menu, réduit à ces entrées — les tuiles Reprendre, Son et Plein écran, une section
// d'informations (la console, qui l'a lancée, depuis quand, le serveur qui la tient), le lien pour regarder (copier ou
// envoyer, le QR code), la vidéo, et Quitter. Ni manette, ni prise de manette, ni sauvegarde, ni triche.

// LA MANETTE NE CHANGE DE MAINS QU'APRÈS UNE DEMANDE (chantier 69 ; le porteur, 2026-11-19 : « on ne peut pas savoir à
// qui prêter s'il y a 10 personnes qui regardent. Pour passer la manette, il faut une demande avant ») : la section
// Joueurs la demande (playersRows), la cloche et les Notifications y répondent. « Prêter ma manette » est supprimé.

// Les sous-panneaux, par leur nom dans la pile de la page (PlayShell.jsx : `panels`).
const SUBPANELS = ["states", "disk", "cheats", "invite", "keyboard", "video", "link", "qr"];

export function PlayMenu({ session, user, input, isWatching, switchesStore, sessionSend, players, roster, departed, notifications, loan, context, panels, openPanel, closePanel, closeMenu, onQuit, watchOnly = false, hostName = "", voice = null, seats = { pads: [], waiting: [] } }) {
    const { t } = useTranslation();
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);
    // Les réglages de la manette de CET APPAREIL : leur seule source, celle de Réglages › Manettes (#758).
    const device = useGamepadPreferences();
    // L'opacité de la manette à l'écran : celle de sa disposition en jeu (padOpacity.js), lue par la manette dessinée.
    const padOpacity = usePadOpacity();
    // L'orientation : l'application pivote son activité ; un navigateur, le plein écran et le verrou. Refusée, la ligne le dit.
    const turnsTheApp = can("screenOrientation");
    const [turnFailed, setTurnFailed] = useState(false);
    const displayNames = useNames();
    const nameOf = (username) => displayNames[username] || username;
    // Mon nom DANS CETTE PARTIE (celui de mon compte chez l'hôte quand elle vit ailleurs) : les joueurs
    // et les spectateurs viennent de son child et portent ses noms à lui.
    const self = nameIn(session, user?.username);
    const isOwner = !watchOnly && (session.me?.role === "owner" || isOwnSession(session, user?.username));
    const consoleEntry = useConsoles().find((entry) => entry.id === session.consoleId);
    const consoleName = consoleEntry?.name || String(session.consoleId || "").toUpperCase();
    const consoleMaxPlayers = consoleEntry?.maxPlayers || 0;
    const extra = input.extraProps || {};
    const set = (values) => switchesStore.setValues(values);
    // UNE QUESTION À CONFIRMER (la remise à zéro, donner sa place, inviter) : la fenêtre de Skeleton, comme toute
    // confirmation du launcher (chantier 53) — par-dessus le menu, qui reste ouvert derrière elle.
    const confirmWith = useConfirm();
    const ask = ({ title, message, confirm, cancelLabel = null, onConfirm, onCancel = null }) => confirmWith({ title, message, confirmLabel: confirm, cancelLabel, onConfirm, onCancel });

    // LES DISQUES, demandés à l'ouverture : rien quand la réponse en annonce un seul ou aucun.
    const listDisks = extra.disks?.list;
    const [disks, setDisks] = useState({ count: 0, current: -1 });
    useEffect(() => {
        if (!listDisks) {
            return undefined;
        }
        let cancelled = false;
        Promise.resolve(listDisks()).then((found) => !cancelled && found && setDisks(found));

        return () => {
            cancelled = true;
        };
    }, [listDisks]);

    // Le temps de jeu de la tuile « Reprendre », à la seconde, tant que le menu est ouvert.
    const [now, setNow] = useState(Date.now());
    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), 1000);

        return () => clearInterval(timer);
    }, []);
    const playTime = formatDuration(session.startedAt, now);

    const playing = isOwner && !isWatching;
    const emulates = session.platform === "retrogaming" || session.platform === "device";
    const freeSlots = session.openSlots ?? 0;
    const invitesAccounts = capabilities().invites === "accounts";
    // Le lien d'une partie : sa page dans le launcher — l'ouvrir, c'est la regarder. CHEZ SON HÔTE : une partie jouée chez
    // un serveur fédéré (ou chez un autre téléphone) y vit sous l'identifiant qu'il lui donne, et c'est son adresse qui
    // l'ouvre ; celle de cet appareil menait à une partie qu'il n'a pas.
    const shareUrl = `${streamHostOrigin() || shareOrigin()}/play/${encodeURIComponent(session.id)}`;
    const top = panels[panels.length - 1];

    // Le menu redevient le panneau du dessus : la lumière revient sur l'entrée qui avait ouvert le sous-panneau
    // (PlayShell.jsx s'en charge) ; une entrée qui a disparu entre-temps la laisse au début du menu.
    useEffect(() => {
        if (top === "menu" && !document.activeElement?.closest(".npanel")) {
            focusElement(document.querySelector(".npanel [data-nav]"), true);
        }
    }, [top]);

    // UN GESTE FAIT REFERME LE MENU (le porteur, 2026-11-26 : « certaines actions doivent automatiquement fermer le menu
    // après […] Même les switches. […] c'est seulement sur les invités, QR code et envoyer une demande de manette que ça
    // ne se ferme pas ») : les interrupteurs, les choix, une sauvegarde d'état, un disque, un code activé, la vidéo,
    // l'orientation, le clavier, la manette prise ou rendue, une notification répondue. Restent ouverts : inviter, le QR
    // code, demander la manette d'un joueur — et l'opacité de la manette, qu'on règle en la faisant glisser.
    const act = (gesture) => {
        gesture();
        closeMenu();
    };
    const then = (gesture) => (...values) => act(() => gesture(...values));
    // Le siège demandé (chantier 84) : un joueur peut en tenir plusieurs, un par manette.
    const askForPad = (username, slot = null) => {
        sessionSend({ type: "requestslot", to: username, slot });
        context.toast.open(t("play.handover.sent"));
    };
    const copyLink = async () => {
        if (await copyTextToClipboard(shareUrl)) {
            act(() => context.toast.open(t("play.share.copied"), { variant: "success" }));

            return;
        }
        // Le presse-papiers est refusé (une origine http) : le lien à copier à la main.
        openPanel("link");
    };
    // ENVOYER LE LIEN : la feuille de partage de l'appareil, puis le menu se ferme ; quand elle ne s'ouvre pas, le lien se
    // copie (ou se montre à copier) au lieu de ne rien faire.
    const sendLink = async () => {
        const outcome = await shareLink({ url: shareUrl, title: session.title || t("play.menu.title"), text: t("play.share.copyHelp") });

        if (outcome === "failed") {
            await copyLink();

            return;
        }
        closeMenu();
    };

    // LES TUILES (direction C) : Reprendre, puis la manette — à prendre, à récupérer, ou à rendre.
    // PRENDRE OU LIBÉRER LA MANETTE, UNE TUILE (chantier 69 ; le porteur, 2026-11-19 : « quand le même joueur a 2 ou
    // plusieurs onglets ouverts, le lien pour "prendre la manette" ou "libérer la manette" doit être dans une tuile ») :
    // cet onglet regarde et peut jouer (sa partie, une place libre, ou sa place tenue par un autre de ses onglets — qui
    // la libère alors, chantier 65) ; ou il joue alors qu'un autre de ses onglets regarde, et la rend pour lui.
    // SON TEXTE DIT CE QUI ARRIVE (le porteur, 2026-11-19 : « le texte de la tuile "Prendre la manette" porte confusion »)
    // : « Récupérer la manette » quand un autre onglet du même compte la tient ; « Prendre la manette (2/4) » pour qui
    // regarde, la manette que le child lui donnera (la plus petite libre, la 1 gardée au propriétaire) ; grisée, « Pas de
    // manette disponible », quand il n'y en a plus.
    const mineElsewhere = players.some((player) => player.username === self);
    const padsMax = roster.maxPlayers || consoleMaxPlayers || 0;
    const held = new Set(players.map((player) => player.slot));
    let firstFree = 0;
    for (let slot = isOwner ? 1 : 2; slot <= padsMax && !firstFree; slot += 1) {
        if (!held.has(slot)) {
            firstFree = slot;
        }
    }
    const watchingHere = isWatching && !watchOnly && !loan.canReturn;
    const releases = !isWatching && !loan.canReclaim && loan.otherTabs > 0;
    const take = () => act(() => sessionSend({ type: "requestslot" }));
    // RENDRE LA MANETTE (le porteur, 2026-11-23 : « valable que quand on a pris une manette. Si c'est une manette empruntée
    // alors on rend à la personne. Mais si c'est un slot alors on libère le slot. La tuile est grisée quand on est owner ») :
    // l'emprunteur la rend à qui l'a prêtée ; un joueur libère sa place ; le propriétaire ne rend pas la sienne.
    const myPlace = players.find((player) => player.username === self)?.slot;
    const giveBackTile = loan.canReturn
        ? { id: "pm-give-back", icon: "bi-arrow-return-left", tone: "green", label: t("play.handover.return"), value: t("play.giveBack.toOwner", { name: nameOf(roster.owner || session.owner) }), onClick: () => act(() => sessionSend({ type: "returnslot" })) }
        : !isWatching && !watchOnly
            ? isOwner
                ? { id: "pm-give-back", icon: "bi-arrow-return-left", tone: "slate", label: t("play.handover.return"), value: t("play.giveBack.owner"), disabled: true }
                : { id: "pm-give-back", icon: "bi-arrow-return-left", tone: "green", label: t("play.handover.return"), value: t("play.giveBack.place", { slot: myPlace ?? "?", max: padsMax || "?" }), onClick: () => act(() => sessionSend({ type: "releaseslot" })) }
            : null;
    const padTile = watchingHere
        ? mineElsewhere
            ? { id: "pm-take", icon: "bi-controller", tone: "green", label: t("play.retrieve"), value: t("play.retrieveTile"), primary: true, onClick: take }
            : firstFree
                ? { id: "pm-take", icon: "bi-controller", tone: "green", label: t("play.takeSlot", { slot: firstFree, max: padsMax }), value: t("play.joinTile"), primary: true, onClick: take }
                : { id: "pm-take", icon: "bi-controller", tone: "slate", label: t("play.noPad"), value: t("play.noPadTile"), disabled: true }
        : loan.canReclaim
            ? { id: "pm-reclaim-tile", icon: "bi-arrow-counterclockwise", tone: "green", label: t("play.handover.reclaim"), value: t("play.giveBack.lentTo", { name: nameOf(loan.lentTo) }), primary: true, onClick: () => act(() => sessionSend({ type: "reclaimslot" })) }
            : releases
                ? { id: "pm-release", icon: "bi-box-arrow-right", tone: "green", label: t("play.release"), value: t("play.releaseTile"), onClick: () => act(() => sessionSend({ type: "releaseslot" })) }
                : giveBackTile;
    // LES TUILES : reprendre la partie, et prendre ou récupérer la manette (le porteur, 2026-11-23). La manette à l'écran,
    // le son du jeu et le plein écran sont des interrupteurs de leur section (Manette, Son, Vidéo).
    const tiles = [
        { id: "pm-resume", icon: "bi-play-fill", tone: "indigo", label: t("play.menu.resume"), value: playTime ? fact(t("play.info.duration"), playTime) : t("play.menu.resumeTile"), primary: true, onClick: closeMenu },
        padTile,
    ];
    // LES MESURES À L'ÉCRAN : les statistiques et la vue discrète s'excluent — un choix à trois cases le dit.
    const measures = switches.isResourceMonitoringVisible ? "panel" : switches.isCompactStatsVisible ? "compact" : "none";

    // Les entrées de chaque groupe, dans l'ordre ; un groupe sans entrée ne s'affiche pas.
    // LA PARTIE (chantier 69 ; le porteur, 2026-11-19) : les sauvegardes d'état y viennent des tuiles ; le retour et
    // l'avance rapides sont partis à l'écran (PlayShell.jsx), là où l'on voit ce qu'ils font.
    const game = [
        playing && emulates && extra.saves && <NavRow key="states" id="pm-states" icon="bi-floppy" label={t("play.states.title")} help={t("play.states.tile")} onOpen={() => openPanel("states")} />,
        playing && emulates && disks.count > 1 && <NavRow key="disk" id="pm-disk" icon="bi-disc" label={t("play.disk.title")} help={t("play.disk.help")} value={t("play.disk.number", { number: disks.current + 1 })} onOpen={() => openPanel("disk")} />,
        playing && emulates && extra.cheats?.cheatsSupported && <NavRow key="cheats" id="pm-cheats" icon="bi-braces" label={t("play.cheats.title")} help={t("play.cheats.help")} onOpen={() => openPanel("cheats")} />,
    ].filter(Boolean);
    // LA MANETTE : seulement ce que touche le pouce de CET appareil, dont les sticks invisibles, la même ligne que dans
    // Réglages › Manettes.
    // LA MANETTE À L'ÉCRAN COUPÉE, SES RÉGLAGES RESTENT, GRISÉS (le porteur, 2026-11-24 : « on doit quand même voir les
    // autres paramètres qui sont liés à la manette […] au pire tu les mets en disabled mais jamais tu les caches ») —
    // SAUF L'OPACITÉ, qui reste active : elle règle aussi le pavé tactile resté seul (« ça concerne aussi le touchpad sur
    // l'écran quand la manette est cachée »), et la manette la retrouve (`lastName`, padOpacity.js).
    // L'OPACITÉ EN DERNIER, APRÈS LES VIBRATIONS (le porteur, 2026-11-24 : « tu peux mettre le range opacité après le
    // switch de la vibration, en dernier dans la section ») ; le 2026-11-23 elle venait en premier.
    const padHidden = !switches.isControllerVisible;
    const pad = [
        !isWatching && <SwitchRow key="pad" id="pm-pad" label={t("play.pad.label")} texts="shownF" help={t("play.pad.help")} checked={Boolean(switches.isControllerVisible)} onChange={then((on) => set({ isControllerVisible: on }))} />,
        !isWatching && <SwitchRow key="haptic" id="pm-haptic" label={t("play.haptic.label")} texts="enabled" help={t("play.haptic.help")} checked={device.hapticFeedback} disabled={padHidden} onChange={then(setHapticFeedback)} />,
        !isWatching && <InvisibleSticksRow key="invisible-sticks" id="pm-invisible-sticks" disabled={padHidden} onChange={then(setInvisibleSticks)} />,
        !isWatching && session.consoleId === "ps1" && <SwitchRow key="dualshock" id="pm-dualshock" label={t("play.dualShock.label")} help={t("play.dualShock.help")} checked={switches.isDualShockController} onChange={then((on) => set({ isDualShockController: on }))} />,
        !watchOnly && <SwitchRow key="rumble" id="pm-rumble" label={t("play.rumble.label")} texts="enabled" help={t("play.rumble.help")} checked={device.rumble} onChange={then(setRumbleEnabled)} />,
        // L'OPACITÉ (#758) : celle de « Modifier la disposition », pour la disposition en jeu ; la manette la suit aussitôt.
        !isWatching && (padOpacity.name || padOpacity.lastName) && <PadOpacityRow key="opacity" id="pm-pad-opacity" value={padOpacity.opacity ?? DEFAULT_PAD_OPACITY} onChange={setPadOpacity} />,
        // LE CLAVIER, déclaré par la plateforme (#420) : streamex seul tape dans son émulateur.
        !isWatching && extra.keyboard && <NavRow key="keyboard" id="pm-keyboard" icon="bi-keyboard" label={t("play.keyboard.title")} help={t("play.keyboard.help")} onOpen={() => openPanel("keyboard")} />,
    ].filter(Boolean);
    // LES JOUEURS (chantier 68 ; le porteur, 2026-11-19) : une ligne par manette de la console — son joueur (connecté, et
    // depuis quand), un joueur parti (« déconnecté », 30 s, PlayShell.jsx), le propriétaire absent (sa place l'attend ; le
    // décompte de l'arrêt quand plus personne ne joue), ou une place libre. UN COMPTE QUI NE JOUE PAS la demande : une
    // place tenue, la manette de ce joueur ; une place libre, rejoindre la partie — après confirmation.
    // DEMANDER UNE MANETTE : qui regarde, ou qui a une manette en attente (chantier 84 : la demande va à cette manette).
    const canAsk = !watchOnly && (isWatching || seats.waiting.length > 0) && !loan.canReturn;
    // LES MANETTES CONNECTÉES À CET APPAREIL (chantier 84 ; le porteur, 2026-11-26 : « une nouvelle section dans le menu
    // "Manettes connectées" qui va lister les manettes et quand on clique sur une manette ça demande si on veut rendre la
    // place […] à condition que la manette connectée soit aussi attribuée à un slot. Cette liste devient active que si
    // manettes connectées > 1 ») : chacune, son numéro et ce qu'elle tient — sa place, en attente, sans place, désactivée.
    // Aucune n'est prioritaire : la manette 1 peut rendre la sienne et laisser la 2 en jeu.
    const connectedPads = useDevicePads();
    const padRows = connectedPads.length === 0 ? [<EmptyRow key="none" icon="bi-controller" text={t("play.pads.none")} />] : connectedPads.map((entry) => {
        const seat = entry.number ? seats.pads.find((one) => one.pad === entry.number)?.slotId : null;
        const state = !entry.enabled ? t("play.pads.off") : seat ? t("play.players.place", { slot: seat, max: padsMax || "?" })
            : seats.waiting.includes(entry.number) ? t("play.pads.waiting") : t("play.pads.free");
        const label = entry.number ? t("play.pads.pad", { number: entry.number }) : entry.name;

        return (
            <NavRow key={entry.key} id={`pm-pad-${entry.number || entry.key}`} icon="bi-controller" label={label} help={[entry.name, fact(t("facts.state"), state)]}
                opens="confirm" disabled={connectedPads.length < 2 || !seat}
                onOpen={() => ask({ title: t("play.pads.releaseTitle", { label }), message: t("play.pads.releaseMessage"), confirm: t("play.pads.release"),
                    onConfirm: () => act(() => sessionSend({ type: "releaseslot", pad: entry.number })) })} />
        );
    });
    const playerRows = !watchOnly ? playersRows({ players, roster, departed, consoleMax: consoleMaxPlayers, now, self, t, nameOf, canAsk, freeSlots, ask, askForPad, join: () => act(() => sessionSend({ type: "requestslot" })) }) : [];
    // À PLUSIEURS : ce qui concerne d'autres personnes — la manette qui passe de main en main, puis le partage.
    const together = [
        // PRENDRE LA MANETTE : tout le monde arrive en spectateur, le propriétaire aussi ; c'est le geste qui démarre
        // une partie, et le retour après un rechargement. Les autres rejoignent par une place libre de la section Joueurs.
        loan.canReclaim && <NavRow key="reclaim" id="pm-reclaim" icon="bi-arrow-counterclockwise" label={t("play.handover.reclaim")} help={t("play.handover.reclaimHelp")} opens="action" onOpen={() => act(() => sessionSend({ type: "reclaimslot" }))} />,
        loan.canReturn && <NavRow key="return" id="pm-return" icon="bi-arrow-return-left" label={t("play.handover.return")} help={t("play.handover.returnHelp")} opens="action" onOpen={() => act(() => sessionSend({ type: "returnslot" }))} />,
        // LE PARTAGE, dès que quelqu'un peut rejoindre : sans adresse sur le réseau, le lien pointerait vers le
        // localhost de qui le reçoit (`canInvite`). Copier OU envoyer, jamais les deux.
        canInvite() && invitesAccounts && !watchOnly && <NavRow key="invite" id="pm-invite" icon="bi-person-plus" label={t("play.invite.title")} help={t("play.invite.help")} onOpen={() => openPanel("invite")} />,
        canInvite() && (canShareLink()
            ? <NavRow key="send" id="pm-send" icon="bi-send" label={t("play.share.send")} help={t("play.share.sendHelp")} opens="action" onOpen={sendLink} />
            : <NavRow key="copy" id="pm-copy" icon="bi-link-45deg" label={t("play.share.copy")} help={t("play.share.copyHelp")} opens="action" onOpen={copyLink} />),
        // LE QR CODE DU MÊME LIEN, pour un téléphone à côté : l'ouvrir, c'est regarder la partie, puis la rejoindre s'il
        // reste une manette. Son panneau (`qr`) le montre en grand, seul : le lien se copie par sa propre ligne (le
        // porteur, 2026-11-14 : « Dans QR code pas besoin de mettre le lien. Car on l'a déjà dans copy the link »).
        canInvite() && <NavRow key="qr" id="pm-qr" icon="bi-qr-code" label={t("play.share.qr")} help={t("play.share.qrHelp")} onOpen={() => openPanel("qr")} />,
    ].filter(Boolean);
    // LA VOIX (chantier 71 ; le porteur, 2026-11-23 : « Le micro et le son des joueurs, c'est eux qui décident. Le micro
    // et le son des spectateurs, c'est le owner qui décide s'ils peuvent interagir ») : mon micro et entendre les autres,
    // à chacun ; la conversation ouverte aux spectateurs, au propriétaire seul. Chacun désactivé avec sa raison, dite
    // par la file : seul, conversation fermée aux spectateurs — et le micro sur une page non sécurisée. Pas pour le
    // spectateur sans compte : la file ne le connaît pas.
    const voiceYou = !watchOnly ? voice?.you : null;
    const voiceWhy = voiceYou && !voiceYou.usable ? t(`play.voice.why.${voiceYou.reason}`) : null;
    const voiceRows = voiceYou ? [
        <SwitchRow key="voice-mic" id="pm-voice-mic" label={t("play.voice.mic")} texts="open" help={voiceWhy || t(voice.microphone ? "play.voice.micHelp" : "play.voice.why.insecure")}
            checked={Boolean(voiceYou.mic)} disabled={!voiceYou.usable || !voice.microphone} onChange={then(voice.toggleMic)} />,
        <SwitchRow key="voice-hear" id="pm-voice-hear" label={t("play.voice.hear")} help={voiceWhy || t("play.voice.hearHelp")}
            checked={voice.hearing} disabled={!voiceYou.usable} onChange={then(voice.toggleHear)} />,
        isOwner && <SwitchRow key="voice-spectators" id="pm-voice-spectators" label={t("play.voice.spectators")} help={t(voiceYou.canOpen ? "play.voice.spectatorsHelp" : "play.voice.why.nobody")}
            checked={Boolean(voice.room?.spectators)} disabled={!voiceYou.canOpen} onChange={then(voice.toggleSpectators)} />,
    ].filter(Boolean) : [];
    // LE SON (le porteur, 2026-11-23 : « rajouter dans la section son, le son du jeu. C'est un switch qui n'est pas
    // grisé ») : le son du jeu, puis la voix.
    const sound = [
        can("pageAudio") && <SwitchRow key="sound" id="pm-sound" label={t("play.sound.label")} texts="open" help={t("play.sound.help")} checked={Boolean(switches.isSoundEnabled)} onChange={then((on) => set({ isSoundEnabled: on }))} />,
        ...voiceRows,
    ].filter(Boolean);
    const picture = [
        // LA VIDÉO (chantier 58) : toujours là (le porteur, 2026-11-16 : « Le second flux s'affiche toujours ») — une
        // partie qui n'a qu'un flux le dit dans son panneau.
        <NavRow key="video" id="pm-video" icon="bi-badge-hd" label={t("play.video.title")} help={t("play.video.short")} value={videoChoiceLabel(t, switches)} onOpen={() => openPanel("video")} />,
        // LE PLEIN ÉCRAN (le porteur, 2026-11-23 : « dans la section vidéo, rajoute le switch Fullscreen ») ; sans objet dans
        // l'application, déjà en plein écran : l'hôte le dit (`fullscreen`).
        can("fullscreen") && <SwitchRow key="fullscreen" id="pm-fullscreen" label={t("play.fullscreen.label")} texts="fullscreen" help={t("play.fullscreen.help")} checked={Boolean(switches.isFullscreen)} onChange={then((on) => set({ isFullscreen: on }))} />,
        // L'ORIENTATION DE L'ÉCRAN (#758) : les lignes de Réglages › Application, le même réglage ; choisie ici, elle vaut
        // tout de suite, la partie continue. Le spectateur sans compte ne l'a pas (son menu est au strict minimum).
        !watchOnly && offersOrientation(turnsTheApp) && <OrientationRows key="orientation" idPrefix="pm" turnsTheApp={turnsTheApp} onFailed={setTurnFailed} onApplied={closeMenu} />,
        can("castToTv") && !watchOnly && switches.castApplicationId && <SwitchRow key="cast" id="pm-cast" label={t("play.cast.label")} help={t("play.cast.help")} checked={switches.isChromeCasting} onChange={then((on) => set({ isChromeCasting: on }))} />,
        !watchOnly && <ChoiceRow key="measures" id="pm-measures" label={t("play.stats.label")} help={t("play.stats.help")} value={measures}
            options={[["none", t("play.stats.none")], ["compact", t("play.stats.compact")], ["panel", t("play.stats.panel")]]}
            onChange={then((next) => set({ isResourceMonitoringVisible: next === "panel", isCompactStatsVisible: next === "compact" }))} />,
    ].filter(Boolean);
    // LES INFORMATIONS DU SPECTATEUR SANS COMPTE : ce que la partie dit d'elle ; le menu d'un joueur les a ailleurs (la
    // tuile Reprendre, la page du jeu).
    const info = watchOnly ? [
        <InfoRow key="console" icon="bi-controller" label={t("play.info.console")} value={consoleName} />,
        <InfoRow key="owner" icon="bi-person" label={t("play.info.owner")} value={session.owner || ""} />,
        <InfoRow key="duration" icon="bi-clock" label={t("play.info.duration")} value={playTime || ""} />,
        <InfoRow key="server" icon="bi-hdd-network" label={t("play.info.server")} value={hostName} />,
    ] : [];
    // `after` : ce qui se lit sous les lignes d'un groupe (l'avis d'une orientation refusée).
    // CHAQUE GROUPE, SA DESCRIPTION ET SON SENS (chantier 81 ; le porteur, 2026-11-25 : « ces modifications s'appliquent aussi
    // côté front office, sur les panneaux latéraux comme le menu du streaming ») — `play.menuHelp.<clé>`, GROUP_SENSES.
    const group = (key, title, rows, after = null) => rows.length > 0 && (
        <Section key={key} title={title} help={t(`play.menuHelp.${key}`)} sense={GROUP_SENSES[key] || "modify"}><Rows>{rows}</Rows>{after}</Section>
    );

    return (
        <>
            <SidePanel label={input.title || session.title || t("play.menu.title")} firstKey="pm-resume" onClose={closeMenu}>
                <QuickTiles items={tiles} />
                {/* LES NOTIFICATIONS DE LA PARTIE (chantier 68, Bell.jsx), en tête : les cinq dernières (RecentNotices ; le
                    porteur, 2026-11-25) ; une demande en attente se répond sur sa ligne. TOUJOURS LÀ, même vide (le porteur,
                    2026-11-19 : « fais comme sur la page d'accueil ») : sa ligne « Aucune notification. ». Pas pour le spectateur
                    sans compte. */}
                {notifications && !watchOnly && (
                    <Section title={t("play.notifications.title")} help={t("play.notifications.help")} sense="info">
                        <RecentNotices limit={5} notices={notifications.list.map((entry) => ({
                            key: entry.id, changedAt: entry.endedAt || entry.at, endedAt: entry.endedAt,
                            row: <NotificationRow entry={entry} nameOf={nameOf} answer={(yes) => act(() => notifications.answer(entry.id, yes))} />,
                        }))} />
                    </Section>
                )}
                {watchOnly && <Section title={t("details")} help={t("play.info.watchOnly")} sense="info"><Rows>{info}</Rows></Section>}
                {group("game", t("play.menu.game"), game)}
                {/* LE SON, PUIS LA VIDÉO, AVANT LA MANETTE (le porteur, 2026-11-23 : « dans le menu de la session, tu peux mettre
                    son et vidéo au dessus de la section manette », puis « la section son doit être en haut avant la section
                    vidéo »). */}
                {group("sound", t("play.menu.sound"), sound)}
                {group("picture", t("play.menu.picture"), picture, turnFailed && <OrientationFailed turnsTheApp={turnsTheApp} />)}
                {group("pad", t("play.menu.controller"), pad)}
                {group("players", t("play.players.title"), playerRows)}
                {!watchOnly && group("pads", t("play.pads.title"), padRows)}
                {group("together", t("play.menu.together"), together)}
                {/* SORTIR : les deux gestes qui mettent fin à ce qu'on fait, à part, en bas — loin des réglages qu'on
                    touche en jouant. Remettre à zéro se confirme. */}
                {group("leave", t("play.menu.leave"), [
                    playing && emulates && extra.reset && (
                        <NavRow key="reset" id="pm-reset" icon="bi-arrow-counterclockwise" label={t("play.reset.title")} help={t("play.reset.help")} opens="confirm"
                            onOpen={() => ask({ title: t("play.reset.confirmTitle"), message: t("play.reset.confirmMessage"), confirm: t("play.reset.confirm"), danger: true, onConfirm: () => act(() => extra.reset()) })} />
                    ),
                    <NavRow key="quit" id="pm-quit" icon="bi-box-arrow-left" label={t("play.menu.quit")} help={t(isWatching ? "play.menu.quitHelpWatching" : "play.menu.quitHelp")} opens="confirm" onOpen={onQuit} />,
                ].filter(Boolean))}
            </SidePanel>

            {panels.filter((name) => SUBPANELS.includes(name)).map((name) => {
                switch (name) {
                    case "states":
                        return <SaveStates key={name} session={session} saves={extra.saves} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "disk":
                        return <DiskPanel key={name} disks={disks} change={extra.disks?.change} context={context} onChange={setDisks} onClose={closePanel} onDone={closeMenu} />;
                    case "cheats":
                        return <CheatsPanel key={name} cheats={extra.cheats} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "invite":
                        return <InvitePanel key={name} session={session} self={self} ask={ask} context={context} onClose={closePanel} />;
                    case "keyboard":
                        return <KeyboardPanel key={name} typeText={extra.keyboard?.typeText} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "video":
                        return <VideoPanel key={name} switches={switches} set={set} platform={session.platform} onClose={closePanel} onDone={closeMenu} />;
                    case "qr":
                        return (
                            <SidePanel key={name} label={t("play.share.qr")} onClose={closePanel}>
                                <Section title={t("play.share.qrTitle")} help={t("play.share.qrHelp")} sense="modify">
                                    <div className="bg-white rounded-4 p-3 mx-auto gf-qr">
                                        <img className="d-block w-100" src={qrImage(shareUrl)} alt={t("play.share.qr")} />
                                    </div>
                                </Section>
                            </SidePanel>
                        );
                    case "link":
                        return (
                            <SidePanel key={name} label={t("play.share.copyFailedTitle")} firstKey="pk-link-copy" onClose={closePanel}>
                                <Section title={t("invitations.linkTitle")} help={t("play.share.copyFailedMessage")} sense="modify">
                                    <Rows><CopyField id="pk-link" value={shareUrl} /></Rows>
                                </Section>
                            </SidePanel>
                        );
                    default:
                        return null;
                }
            })}
        </>
    );
}

// LES SAUVEGARDES D'ÉTAT DE LA PARTIE : dix emplacements, le premier réservé à « sauvegarder en partant ».
// Chacun est une entrée (sa vignette, sa date) qui ouvre son panneau — aucun geste sur la ligne (bricks/) :
// sauver ici, ou écraser, charger, chaque remplacement confirmé sur place, et la suppression en bas. Sauver ou
// charger ferme le menu, pour qu'on voie le jeu repartir.
function SaveStates({ session, saves, context, onClose, onDone }) {
    const { t, i18n } = useTranslation();
    const confirmWith = useConfirm();
    const [slots, setSlots] = useState(null);
    const [open, setOpen] = useState(null);
    const refresh = () => listSaveSlots(session.consoleId, session.reference, "", playedHost()).then((answer) => setSlots(answer.ok ? answer.data || [] : []));

    useEffect(() => {
        refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const when = (info) => (info?.exists && info.timestamp ? dateTime(info.timestamp, i18n.language) : t("play.states.empty"));
    const nameOf = (slot) => slotName(slot, t);
    const save = (slot) => {
        onDone();
        saves.saveState(slot, (error) => context.toast.open(error || t("play.states.saved"), { variant: error ? "error" : undefined }));
    };
    const load = (slot) => {
        onDone();
        saves.loadState(slot, (error, _info, answer) => {
            // Un état d'un autre cœur : le refus le dit (un état ne se charge que dans l'émulateur qui l'a écrit).
            const refused = answer?.code === "state-refused" ? t("play.states.refused", { core: answer.stateCore || answer.core || "" }) : null;
            context.toast.open(error ? (refused || error) : t("play.states.loaded"), { variant: error ? "error" : undefined });
        });
    };
    // Le panneau d'un emplacement se referme : la lumière revient sur sa ligne.
    const closeSlot = (slot) => {
        setOpen(null);
        setTimeout(() => focusElement(document.querySelector(`.npanel [data-key="ps-slot-${slot}"]`), true), 0);
    };
    const remove = async (slot) => {
        const answer = await deleteSaveSlot(session.consoleId, session.reference, slot, "", playedHost());

        if (!answer.ok) {
            context.toast.open(t("play.states.deleteFailed"), { variant: "error" });

            return;
        }
        await refresh();
        closeSlot(slot);
    };
    const info = (slot) => (slots || []).find((entry) => entry.slot === slot);

    return (
        <>
            <SidePanel label={t("play.states.title")} onClose={onClose}>
                <Section title={t("saves.slotsTitle")} help={t("play.states.help")} sense="modify">
                    {slots === null ? <Note className="mt-0">{t("play.loading")}</Note> : (
                        <Rows>
                            {Array.from({ length: 10 }, (_, slot) => (
                                <NavRow key={slot} id={`ps-slot-${slot}`} label={nameOf(slot)} help={info(slot)?.exists && info(slot).timestamp ? fact(t("facts.date"), when(info(slot))) : when(info(slot))} onOpen={() => setOpen(slot)}
                                    lead={<SlotThumbnail session={session} slot={slot} info={info(slot)} />} />
                            ))}
                        </Rows>
                    )}
                </Section>
            </SidePanel>
            {open !== null && (
                <SidePanel key={open} label={nameOf(open)} subtitle={when(info(open))} firstKey={info(open)?.exists ? "ps-load-ask" : "ps-save"} onClose={() => closeSlot(open)}>
                    <Section title={t("saves.stateTitle")} sense="info">
                        <SlotThumbnail session={session} slot={open} info={info(open)} />
                    </Section>
                    {/* Ses gestes en lignes, dans « Plus » (le porteur, 2026-11-13 : un geste orphelin est une ligne) ;
                        charger et écraser se confirment dans la fenêtre de Skeleton. Le premier emplacement s'écrit
                        seul, en quittant la partie : on n'y sauvegarde pas. */}
                    {(info(open)?.exists || open !== 0) && (
                        <Section title={t("more")} sense="modify">
                            <Rows>
                                {info(open)?.exists && (
                                    <NavRow id="ps-load-ask" icon="bi-box-arrow-in-down" label={t("play.states.load")} help={t("play.states.loadConfirm")} opens="confirm"
                                        onOpen={() => confirmWith({ title: t("play.states.load"), message: t("play.states.loadConfirm"), onConfirm: () => load(open) })} />
                                )}
                                {open !== 0 && (info(open)?.exists
                                    ? <NavRow id="ps-save-ask" icon="bi-floppy" label={t("play.states.overwrite")} help={t("play.states.overwriteConfirm")} opens="confirm"
                                        onOpen={() => confirmWith({ title: t("play.states.overwrite"), message: t("play.states.overwriteConfirm"), onConfirm: () => save(open) })} />
                                    : <NavRow id="ps-save" icon="bi-floppy" label={t("play.states.save")} opens="action" onOpen={() => save(open)} />)}
                            </Rows>
                        </Section>
                    )}
                    {info(open)?.exists && <DangerSection id="ps-delete" label={t("play.states.delete")} question={t("play.states.deleteConfirm")} onConfirm={() => remove(open)} />}
                </SidePanel>
            )}
        </>
    );
}

// La vignette d'un emplacement, avec le jeton (la route le demande) — celui de l'hôte quand la partie vit
// chez un serveur fédéré (la page de jeu vise alors son adresse : libs/serverBase.js).
function SlotThumbnail({ session, slot, info }) {
    const [url, setUrl] = useState(null);

    useEffect(() => {
        if (!info?.exists) {
            setUrl(null);

            return undefined;
        }
        let cancelled = false;
        let made = null;
        fetchStateScreenshot(session.consoleId, session.reference, slot, info.timestamp || "", playedHost())
            .then(({ blob }) => {
                if (!cancelled && blob) {
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
    }, [session.consoleId, session.reference, slot, info?.exists, info?.timestamp]);

    return (
        <span className="thumb ratio ratio-4x3 flex-shrink-0 rounded overflow-hidden bg-body-tertiary">
            {url ? <img src={url} alt="" className="object-fit-cover" /> : <span className="d-flex align-items-center justify-content-center text-body-tertiary"><i className="bi bi-camera"></i></span>}
        </span>
    );
}

// LE DISQUE INSÉRÉ : optimiste à l'affichage, puis recalé sur ce que l'hôte dit avoir monté.
function DiskPanel({ disks, change, context, onChange, onClose, onDone }) {
    const { t } = useTranslation();
    const choose = async (index) => {
        onChange({ ...disks, current: index });
        onDone();
        const state = await change(index);

        if (state?.error) {
            context.toast.open(t("play.disk.failed"), { variant: "error" });
        }
        if (state) {
            onChange({ count: state.count, current: state.current });
        }
    };

    return (
        <SidePanel label={t("play.disk.title")} firstKey={`pd-disk-${disks.current}`} onClose={onClose}>
            <Section title={t("play.disk.listTitle")} sense="modify">
                <Rows>
                    <RadioRow id="pd-disk" help={t("play.disk.help")} value={disks.current} onChange={choose}
                        options={Array.from({ length: disks.count }, (_, index) => ({ value: index, icon: "bi-disc", label: t("play.disk.number", { number: index + 1 }), help: index === disks.current ? t("play.disk.inserted") : null }))} />
                </Rows>
            </Section>
        </SidePanel>
    );
}

// LES CODES DE TRICHE : enregistrés sur le compte par le moteur (`setCheats`), appliqués en direct.
// Un code s'ajoute (nom, code) ; chacun est une entrée de la liste qui ouvre son panneau — l'activer, le
// modifier, le retirer (aucun geste sur la ligne : bricks/).
function CheatsPanel({ cheats, context, onClose, onDone }) {
    const { t } = useTranslation();
    const [entries, setEntries] = useState(cheats?.initialCheats || []);
    const [values, setValues] = useState({ "pt-name": "", "pt-code": "" });
    const [open, setOpen] = useState(null);
    const [pending, setPending] = useState(false);
    const push = (next, done) => {
        setEntries(next);
        setPending(true);
        cheats.setCheats(next, (error) => {
            setPending(false);

            if (error) {
                context.toast.open(error, { variant: "error" });
            }
            done?.();
        });
    };
    const add = (event) => {
        event.preventDefault();
        const code = (values["pt-code"] || "").trim();

        if (!code) {
            return;
        }
        push([...entries, { code, enabled: true, description: (values["pt-name"] || "").trim() }], () => setValues({ "pt-name": "", "pt-code": "" }));
    };
    // Le panneau d'un code se referme : la lumière revient sur sa ligne, ou au formulaire s'il a été retiré.
    const closeEntry = (index) => {
        setOpen(null);
        setTimeout(() => focusElement(document.querySelector(`.npanel [data-key="pt-cheat-${index}"]`) || document.querySelector('.npanel [data-key="pt-name"]'), true), 0);
    };

    return (
        <>
            <SidePanel label={t("play.cheats.title")} firstKey="pt-name" onClose={onClose}>
                <Form props={{ onSubmit: add }}>
                    <Section title={t("play.cheats.addTitle")} help={t("play.cheats.help")} sense="modify">
                    <Rows>
                        <TextRow id="pt-name" label={t("play.cheats.name")} context={formContext(values, setValues)} props={{ placeholder: t("play.cheats.namePlaceholder") }} />
                        <TextRow id="pt-code" kind="longtext" label={t("play.cheats.code")} help={t("play.cheats.codeHelp")} context={formContext(values, setValues)}
                            props={{ rows: 3, className: "font-monospace", placeholder: t("play.cheats.codePlaceholder") }} />
                        <SubmitRow>
                            <FormButton id="pt-submit" label={t("play.cheats.add")} isReplaced={Boolean((values["pt-name"] || "").trim() || (values["pt-code"] || "").trim())} pending={pending}
                                disabled={!(values["pt-code"] || "").trim()} />
                        </SubmitRow>
                    </Rows>
                    </Section>
                </Form>
                <Section title={t("play.cheats.list")} sense="modify">
                    {entries.length ? (
                        <Rows>
                            {entries.map((entry, index) => (
                                <NavRow key={index} id={`pt-cheat-${index}`} icon={entry.enabled ? "bi-magic" : "bi-slash-circle"} label={entry.description || entry.code}
                                    help={[entry.description ? fact(t("play.cheats.code"), entry.code) : null, fact(t("facts.state"), t(entry.enabled ? "settings.switchOn" : "settings.switchOff"))]} onOpen={() => setOpen(index)} />
                            ))}
                        </Rows>
                    ) : <EmptyState icon="bi-braces" text={t("play.cheats.none")} />}
                </Section>
            </SidePanel>
            {open !== null && entries[open] && (
                <CheatPanel key={open} entry={entries[open]} pending={pending} onClose={() => closeEntry(open)}
                    onSave={(next) => push(entries.map((item, at) => (at === open ? next : item)), () => closeEntry(open))}
                    onToggle={(enabled) => push(entries.map((item, at) => (at === open ? { ...item, enabled } : item)), onDone)}
                    onDelete={() => push(entries.filter((_, at) => at !== open), () => closeEntry(open))} />
            )}
        </>
    );
}

// UN CODE DE TRICHE : l'activer sur place, modifier son code (son nom reste celui donné à l'ajout), le retirer.
function CheatPanel({ entry: cheat, pending, onClose, onSave, onToggle, onDelete }) {
    const { t } = useTranslation();
    const [values, setValues] = useState({ "pc-code": cheat.code });
    const save = (event) => {
        event.preventDefault();
        const code = (values["pc-code"] || "").trim();
        code && onSave({ ...cheat, code });
    };

    return (
        <SidePanel label={cheat.description || t("play.cheats.title")} firstKey="pc-enabled" onClose={onClose}>
            <Section title={t("play.cheats.usageTitle")} sense="info">
                <Rows>
                    <SwitchRow id="pc-enabled" label={t("play.cheats.apply")} help={t("play.cheats.applyHelp")} checked={Boolean(cheat.enabled)} onChange={onToggle} />
                </Rows>
            </Section>
            <Form props={{ onSubmit: save }}>
                <Section title={t("play.cheats.code")} sense="modify">
                    <Rows>
                        <TextRow id="pc-code" kind="longtext" label={t("play.cheats.code")} help={t("play.cheats.codeHelp")} context={formContext(values, setValues)} props={{ rows: 3, className: "font-monospace" }} />
                        <SubmitRow>
                            <FormButton id="pc-save" label={t("play.cheats.update")} isReplaced={(values["pc-code"] || "").trim() !== cheat.code} pending={pending} disabled={!(values["pc-code"] || "").trim()} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <DangerSection id="pc-delete" label={t("play.cheats.delete")} question={t("play.cheats.deleteConfirm")} disabled={pending} onConfirm={onDelete} />
        </SidePanel>
    );
}

// DES PERSONNES À QUI DEMANDER LA MANETTE, OU À QUI LA PASSER — `people` se relit à chaque rendu : quelqu'un
// peut prendre ou lâcher une manette pendant que le panneau est ouvert. `picks` : ce que fait une ligne touchée (NavRow,
// `opens`) — demander part aussitôt, passer la manette se confirme.
// LE TEMPS QUI RESTE avant une échéance, « -HH:MM:SS ».
// Le délai d'une partie où plus personne ne joue (kernel, `unifiedIdleTimeout`) : passé, elle s'arrête.
const IDLE_STOP_MS = 5 * 60 * 1000;

function playersRows({ players, roster, departed, consoleMax, now, self, t, nameOf, canAsk, freeSlots, ask, askForPad, join }) {
    const max = roster.maxPlayers || consoleMax || players.length;
    const ownerAway = roster.owner && !players.some((player) => player.username === roster.owner);
    const idleEnd = roster.idleSince ? new Date(roster.idleSince).getTime() + IDLE_STOP_MS : 0;
    const rows = [];

    for (let slot = 1; slot <= max; slot += 1) {
        const held = players.find((player) => player.slot === slot);
        const gone = !held && departed.find((entry) => entry.slot === slot);
        const owned = !held && !gone && slot === 1 && ownerAway;
        // UN JOUEUR À PLUSIEURS MANETTES (chantier 84) : « Gino (Pad 2) ».
        const several = held && players.filter((player) => player.username === held.username).length > 1;
        const name = held ? (several ? t("play.pads.named", { name: nameOf(held.username), pad: held.pad }) : nameOf(held.username))
            : gone ? nameOf(gone.username) : owned ? nameOf(roster.owner) : t("play.players.nobody");
        const place = t("play.players.place", { slot, max });
        const seat = fact(t("facts.place"), place);
        const help = held ? [seat, fact(t("facts.state"), t("play.players.connected")), fact(t("play.info.duration"), formatDuration(held.joinedAt, now))]
            : gone ? [seat, fact(t("facts.state"), t("play.players.disconnected"))]
                : owned ? [seat, fact(t("facts.state"), t("play.players.disconnected")), idleEnd ? fact(t("facts.comeBack"), formatDuration(now, idleEnd)) : null]
                    : [seat, fact(t("facts.state"), t("play.players.free"))];
        const icon = held ? "bi-person-fill" : gone || owned ? "bi-person-dash" : "bi-controller";
        const askable = canAsk && held && held.username !== self && !held.lentTo;
        const joinable = canAsk && !held && !gone && !owned && freeSlots > 0;

        if (askable) {
            rows.push(<NavRow key={slot} id={`pm-player-${slot}`} icon={icon} label={name} help={help} opens="confirm"
                onOpen={() => ask({ title: t("play.players.askTitle"), message: t("play.players.askMessage", { name }), confirm: t("play.players.ask"), onConfirm: () => askForPad(held.username, slot) })} />);
        } else if (joinable) {
            // Une place libre : « > » (le porteur, 2026-11-19 : « "?" quand c'est déjà pris par un joueur, et ">" quand
            // c'est un free slot ») ; elle se confirme quand même.
            rows.push(<NavRow key={slot} id={`pm-player-${slot}`} icon={icon} label={name} help={help} opens="page"
                onOpen={() => ask({ title: t("play.players.joinTitle"), message: t("play.players.joinMessage"), confirm: t("play.join"), onConfirm: join })} />);
        } else {
            rows.push(<InfoRow key={slot} icon={icon} label={name} value={help} />);
        }
    }

    return rows;
}

// UNE NOTIFICATION DU MENU : une demande en attente se touche (la confirmation : céder, ou garder) ; le reste se lit,
// une demande réglée avec son issue.
// Son sous-titre, quand il nomme quelqu'un, se lit à l'affichage (`subtitle(nameOf)`) : le nom affiché arrive parfois après
// la notification.
// PLUS DE « IL Y A 5 SECONDES » (le porteur, 2026-11-25 : « tu peux enlever le timer. Ça ne sert plus à rien ») : une
// notification finie part d'elle-même 1 min 30 après (RecentNotices).
const OUTCOME_TONES = { accepted: "success", refused: "danger", expired: "secondary" };

function NotificationRow({ entry, nameOf, answer }) {
    const { t } = useTranslation();
    const subtitle = typeof entry.subtitle === "function" ? entry.subtitle(nameOf) : entry.subtitle;

    // UNE DEMANDE EN ATTENTE SE RÉPOND SUR SA LIGNE (le porteur, 2026-11-25) : Ignorer, puis son geste.
    if (entry.asks && entry.state === "pending") {
        return (
            <RequestRow id={`pm-notification-${entry.id}`} label={entry.title} help={subtitle} acceptLabel={entry.confirm.confirmLabel}
                onIgnore={() => answer(false)} onAccept={() => answer(true)} />
        );
    }

    // RÉGLÉE, SON ISSUE EST UN ÉTAT : en pastille colorée, comme un statut (acceptée, refusée, sans réponse).
    return entry.asks
        ? <InfoRow icon="bi-controller" label={entry.title} value={t(`play.notifications.state.${entry.state}`)} pill={OUTCOME_TONES[entry.state] || "secondary"} help={subtitle} />
        : <InfoRow icon="bi-bell" label={entry.title} value={subtitle} />;
}

// INVITER UN MEMBRE : une notification (en direct s'il est connecté, en push sinon) qui ouvre la partie.
function InvitePanel({ session, self, ask, context, onClose }) {
    const { t } = useTranslation();
    const [members, setMembers] = useState(null);

    useEffect(() => {
        let cancelled = false;
        listMembers().then((answer) => !cancelled && setMembers(answer.ok ? answer.item?.members || [] : []));

        return () => {
            cancelled = true;
        };
    }, []);

    const invite = (member) => {
        const name = member.display_name || member.username;
        ask({
            title: t("play.invite.title"),
            message: t("play.invite.confirm", { name }),
            confirm: t("play.invite.send"),
            onConfirm: async () => {
                const answer = await sendInvite({ username: member.username, platform: session.platform, consoleId: session.consoleId, reference: session.reference, sessionId: session.id });
                context.toast.open(t(answer.ok ? "play.invite.sent" : "play.invite.failed", { name }), { variant: answer.ok ? "success" : "error" });
            },
        });
    };
    const visible = (members || []).filter((member) => member.username !== self);

    return (
        <SidePanel label={t("play.invite.title")} onClose={onClose}>
            <Section title={t("members.listTitle")} help={t("play.invite.help")} sense="modify">
            {members === null ? <Note className="mt-0">{t("play.loading")}</Note> : visible.length ? (
                <Rows>
                    {visible.map((member) => <NavRow key={member.username} id={`pi-${member.username}`} lead={<Avatar name={member.display_name || member.username} />} label={member.display_name || member.username} help={member.username} opens="confirm" onOpen={() => invite(member)} />)}
                </Rows>
            ) : <EmptyState icon="bi-people" text={t("play.invite.none")} />}
            </Section>
        </SidePanel>
    );
}

// LE CLAVIER (#420) : un texte tapé dans le jeu lettre par lettre, puis Entrée ; le menu se ferme pour
// qu'on le voie arriver.
function KeyboardPanel({ typeText, context, onClose, onDone }) {
    const { t } = useTranslation();
    const [text, setText] = useState("");
    const send = () => {
        const typed = text;
        onDone();
        typeText(typed).then(({ missing }) => {
            if (missing.length) {
                context.toast.open(t("play.keyboard.missing", { characters: missing.join(" ") }), { variant: "warning" });
            }
        });
    };

    return (
        <SidePanel label={t("play.keyboard.title")} firstKey="pk-text" onClose={onClose}>

            <Form props={{ onSubmit: (event) => {
                event.preventDefault();
                text && send();
            } }}>
                <Section title={t("play.keyboard.text")} help={t("play.keyboard.help")} sense="modify">
                <Rows>
                    <TextRow id="pk-text" label={t("play.keyboard.text")} help={t("play.keyboard.textHelp")} value={text} onChange={(value) => setText(value || "")}
                        props={{ id: "pk-text", autoCapitalize: "off", spellCheck: false }} />
                    <SubmitRow><FormButton id="pk-send" label={t("play.keyboard.send")} isReplaced={Boolean(text)} /></SubmitRow>
                </Rows>
                </Section>
            </Form>
        </SidePanel>
    );
}

// LE FLUX DE CET ÉCRAN (chantiers 31 et 58) : automatique, ou l'un des flux de la partie — le sien, puis ses paliers,
// plus petits, à la même cadence. Les cartes d'option de Skeleton (RadioRow), chacune sa taille et sa cadence, lues dans
// le canal de contrôle (rien d'inventé ici). Une partie qui n'a qu'un flux le montre, coché, et dit pourquoi.
const streamLabel = (t, stream) => t("play.video.stream", { height: stream.height, fps: stream.fps });
// Le badge d'une carte suit la taille du flux, pas son rôle : HD à partir de 720p, SD en dessous.
const streamIcon = (stream) => (stream.height >= 720 ? "bi-badge-hd" : "bi-badge-sd");

// Le résumé de la ligne « Vidéo » du menu : « Automatique », ou le flux fixé.
function videoChoiceLabel(t, switches) {
    const streams = switches.videoStreams;
    if (streams.length === 1) {
        return streamLabel(t, streams[0]);
    }
    const fixed = streams[Number(switches.videoQuality)];

    return switches.videoQuality !== "auto" && fixed ? streamLabel(t, fixed) : t("play.video.auto");
}

// Pourquoi une partie n'a qu'un flux : déjà au plus petit (streamex), l'image encodée par la console (Remote Play),
// ou un type de partie sans paliers (le rétro, le téléphone).
const singleReason = (platform) => (platform === "streamex" ? "single" : platform === "remoteplay" ? "singleConsole" : "singleKind");

function VideoPanel({ switches, set, platform, onClose, onDone }) {
    const { t } = useTranslation();
    const streams = switches.videoStreams;
    const single = streams.length < 2;
    const options = single
        ? streams.map((stream) => ({ value: "0", icon: streamIcon(stream), label: streamLabel(t, stream), help: t("play.video.hostHelp") }))
        : [
            { value: "auto", icon: "bi-magic", label: t("play.video.auto"), help: t("play.video.autoHelp") },
            ...streams.map((stream, index) => ({
                value: String(index),
                icon: streamIcon(stream),
                label: streamLabel(t, stream),
                help: t(index === 0 ? "play.video.hostHelp" : "play.video.tierHelp"),
            })),
        ];
    const value = single ? "0" : switches.videoQuality;
    const current = streams[switches.videoCurrent];

    return (
        <SidePanel label={t("play.video.title")} firstKey={`pv-quality-${Math.max(0, options.findIndex((option) => option.value === value))}`} onClose={onClose}>
            <Section title={t("play.video.streamTitle")} sense="modify">
                {options.length > 0 && (
                    <Rows>
                        <RadioRow id="pv-quality" help={t("play.video.short")} value={value} onChange={(next) => {
                            set({ videoQuality: next });
                            onDone();
                        }} options={options} disabled={single} />
                    </Rows>
                )}
                {single ? <Note>{t(`play.video.${singleReason(platform)}`)}</Note> : current && <Note>{t("play.video.now", { stream: streamLabel(t, current) })}</Note>}
            </Section>
        </SidePanel>
    );
}

// QUITTER UNE PARTIE : LA FENÊTRE DE CONFIRMATION DE SKELETON (useConfirm ; le porteur, 2026-11-16 : « le modal quand on
// quitte une session doit être avec confirmation. Utilise toujours ce que propose skeleton »), son choix dans son corps
// (`body`, comme la note d'un backup) — et toujours UN SÉLECTEUR, jamais trois boutons côte à côte (le porteur,
// 2026-11-14, chantier 53, P12 : « avoir les 3 boutons avant de quitter n'est pas ergonomique et c'est moche. On peut
// facilement cliquer sur l'autre bouton sans faire exprès. Il faut un selectbox qui demande quoi faire. Et par défaut
// quitter et tuer la session car j'ai remarqué que beaucoup de gens ne tue pas la session après avoir quitté. ») :
//   - le propriétaire qui joue : « Que faire de la partie ? » — quitter et l'arrêter (le défaut, `store`), quitter et la
//     laisser tourner, quitter et la mettre en pause (un cœur seulement : une PS5 ou un conteneur streamex ne se gèlent
//     pas) —, l'état à sauver en partant (rétro) ; puis Annuler et Quitter, les boutons de Skeleton ;
//   - qui ne peut pas arrêter la partie (il regarde, il a emprunté une manette, il joue chez un autre) : sa seule issue,
//     dite en question, et son bouton — ni sélecteur ni choix ;
//   - Annuler, B, Échap ou un toucher sur le voile referment sans rien faire ; la lumière revient sur « Quitter la
//     partie ».
// À LA MANETTE : la lumière arrive sur Annuler (toute confirmation de Skeleton), HAUT mène au sélecteur, GAUCHE et
// DROITE y changent l'option. La partie ne se quitte qu'une fois la fenêtre refermée (`whenClosed`) : rien d'elle ne
// reste dans la page de jeu qui s'en va (en plein écran, Skeleton la pose, voile compris, dans l'élément en plein écran :
// skeleton.js).
// UNE CONSOLE REMOTE PLAY SE MET EN VEILLE en partant, en deuxième choix (chantier 69 ; le porteur, 2026-11-19 : « envoyer
// un signal d'arrêt à la console […] et le mettre en seconde position »).
// REMOTE PLAY N'A PAS « ARRÊTER LA PARTIE » (le porteur, 2026-11-19 : « c'est exactement ce que fait "Mettre la console en
// veille et quitter" ») : la veille d'abord, puis partir en la laissant tourner.
const quitChoices = (retro, remote = false) => (remote ? ["standby", "leave"] : ["kill", "leave", ...(retro ? ["pause"] : [])]);
const quitChoiceOf = (values, retro, remote = false) => (quitChoices(retro, remote).includes(values.onQuit) ? values.onQuit : quitChoices(retro, remote)[0]);

// Le corps de la fenêtre : le choix, et l'état à sauver d'un jeu rétro — retenus dans `store`, lu à la confirmation.
function QuitChoice({ retro, remote, store }) {
    const { t } = useTranslation();
    const values = useSyncExternalStore(store.subscribe, store.getSnapshot);
    const choice = quitChoiceOf(values, retro, remote);

    return (
        <div className="text-start">
            <Rows>
                <SelectRow id="pq-choice" label={t("play.quit.what")} help={t(`play.quit.help.${choice}`)} value={choice} onChange={(next) => store.setValues({ onQuit: next })}
                    options={quitChoices(retro, remote).map((name) => ({ value: name, label: t(`play.quit.choice.${name}`) }))} />
                {retro && <SwitchRow id="pq-save" label={t("play.quit.save")} help={t("play.quit.saveHelp")} checked={Boolean(values.saveStateOnQuit)} onChange={(on) => store.setValues({ saveStateOnQuit: on })} />}
            </Rows>
        </div>
    );
}

// `useQuitFlow(options)` rend la fonction qui ouvre la sortie. `options` : la partie (`session`), qui je suis
// (`isWatching`, `isOwner`, `borrowing`), le choix retenu (`store`), et ce qui quitte (`onConfirm({ onQuit,
// saveStateOnQuit })`, PlayShell.jsx) — lus à l'ouverture, jamais figés.
export function useQuitFlow(options) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const latest = useRef(options);
    latest.current = options;

    return useCallback(() => {
        const { session, isWatching, isOwner, borrowing, othersPlay, store } = latest.current;
        const canEnd = isOwner && !isWatching && !borrowing;
        const retro = session.platform === "retrogaming" || session.platform === "device";
        const remote = session.platform === "remoteplay";
        // D'AUTRES JOUENT : le choix s'ouvre sur « Quitter et laisser la partie tourner » (le porteur, 2026-11-19 : « ça
        // permettra de ne pas couper tout de suite la session des autres ») ; sinon sur le premier choix.
        if (canEnd) {
            store.setValues({ onQuit: othersPlay ? "leave" : quitChoices(retro, remote)[0] });
        }
        const kind = borrowing ? "return" : isWatching ? "watch" : "leave";

        confirm({
            title: t(canEnd ? "play.menu.quit" : `play.quit.title.${kind}`),
            body: canEnd ? <QuitChoice retro={retro} remote={remote} store={store} /> : null,
            message: canEnd || !i18n.exists(`play.quit.help.${kind}`) ? null : t(`play.quit.help.${kind}`),
            confirmLabel: t(canEnd ? "play.quit.submit" : `play.quit.confirm.${kind}`),
            whenClosed: true,
            onConfirm: () => {
                const values = store.getSnapshot();
                latest.current.onConfirm({ ...values, onQuit: canEnd ? quitChoiceOf(values, retro, remote) : kind });
            },
        });
    }, [confirm, t]);
}
