import React, { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { listSaveSlots, deleteSaveSlot, screenshotUrl } from "../../engine/api/retrogaming.js";
import { listMembers, sendInvite } from "../../libs/profileApi.js";
import { isOwnSession, nameIn } from "../../libs/SessionPool.js";
import { useNames } from "../../libs/displayNames.js";
import { can, capabilities } from "../../libs/capabilities.js";
import { canInvite, shareOrigin, streamAuthToken, streamHostOrigin } from "../../libs/serverBase.js";
import { copyTextToClipboard, canShareLink, qrImage, shareLink } from "../../libs/share-link.js";
import { readToken } from "../../libs/api.js";
import { formatDuration } from "../../libs/format.js";
import { authHeaders, useConsoles } from "../../libs/sharedHost.js";
import { focusElement } from "../../libs/input.js";
import { setHapticFeedback, setRumbleEnabled, useGamepadPreferences } from "../../engine/libs/gamepadPreferences.js";
import { DEFAULT_PAD_OPACITY, setPadOpacity, usePadOpacity, useTouchpadAlone } from "../../engine/stream/onscreen/padOpacity.js";
import i18n from "../../i18n.js";
import SidePanel from "../../shell/SidePanel.jsx";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import { slotName } from "../library/GameSettings.jsx";
import NavRow, {
    Avatar, ChoiceRow, CopyField, Listing, DangerSection, EmptyState, InfoRow, InvisibleSticksRow, Note, OrientationFailed, OrientationRows, PadOpacityRow, QuickTiles, RadioRow, Rows, Section, SelectRow, SubmitRow,
    SwitchRow, TextRow, offersOrientation, useConfirm,
} from "../../ui/Bricks.jsx";

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

// À QUI PASSER LA MANETTE : ceux qui regardent — la liste vient du child seul. À qui la DEMANDER : la section Joueurs
// (PlayersRows), une ligne par manette.
const giveableOf = (watchers, self) => (watchers || []).filter((username) => username && username !== self).map((username) => ({ username }));

// Les sous-panneaux, par leur nom dans la pile de la page (PlayShell.jsx : `panels`).
const SUBPANELS = ["states", "disk", "cheats", "give", "invite", "keyboard", "video", "link", "qr"];

export function PlayMenu({ session, user, input, isWatching, switchesStore, sessionSend, players, watchers, roster, departed, notifications, loan, context, panels, openPanel, closePanel, closeMenu, onQuit, watchOnly = false, hostName = "" }) {
    const { t } = useTranslation();
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);
    // Les réglages de la manette de CET APPAREIL : leur seule source, celle de Réglages › Manettes (#758).
    const device = useGamepadPreferences();
    // L'opacité de la manette à l'écran : celle de sa disposition en jeu (padOpacity.js), lue par la manette dessinée.
    const padOpacity = usePadOpacity();
    // Une vraie manette : la manette à l'écran se cache, mais le pavé tactile (PS4, PS5) reste — sa glissière aussi.
    const touchpadAlone = useTouchpadAlone();
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
    const giveable = giveableOf(watchers, self);
    const lending = isOwner;
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

    const act = (gesture) => {
        gesture();
        closeMenu();
    };
    const askForPad = (username) => act(() => {
        sessionSend({ type: "requestslot", to: username });
        context.toast.open(t("play.handover.sent"));
    });
    const givePad = (username) => act(() => {
        sessionSend({ type: "giveslot", to: username });
        context.toast.open(t(lending ? "play.handover.lent" : "play.handover.given", { name: nameOf(username) }));
    });
    const confirmGive = (username) => ask({
        title: t(lending ? "play.handover.lendTitle" : "play.handover.giveTitle"),
        message: t(lending ? "play.handover.lendMessage" : "play.handover.giveMessage", { name: nameOf(username) }),
        confirm: t(lending ? "play.handover.lend" : "play.handover.give"),
        onConfirm: () => givePad(username),
    });
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

    const tone = (on) => t(on ? "settings.switchOn" : "settings.switchOff");
    // LES TUILES (direction C) : Reprendre, puis ce qu'on touche le plus en jouant — chacune là où son entrée
    // existait (mêmes conditions) ; une tuile qui règle s'allume avec son réglage, puis ferme le menu (`act`).
    const tiles = [
        { id: "pm-resume", icon: "bi-play-fill", tone: "indigo", label: t("play.menu.resume"), value: playTime ? t("play.menu.resumeTileTime", { time: playTime }) : t("play.menu.resumeTile"), primary: true, onClick: closeMenu },
        !isWatching && {
            // Le libellé court d'une tuile : la manette qu'elle montre (« Super Nintendo… ») allongeait sa rangée.
            id: "pm-pad", icon: "bi-phone-landscape", tone: "violet", label: t("play.pad.label"),
            value: tone(switches.isControllerVisible), on: Boolean(switches.isControllerVisible), onClick: () => act(() => set({ isControllerVisible: !switches.isControllerVisible })),
        },
        can("pageAudio") && { id: "pm-sound", icon: "bi-volume-up", tone: "sky", label: t("play.sound.label"), value: tone(switches.isSoundEnabled), on: Boolean(switches.isSoundEnabled), onClick: () => act(() => set({ isSoundEnabled: !switches.isSoundEnabled })) },
        // Sans objet dans l'application, déjà en plein écran : l'hôte le dit (`fullscreen`).
        can("fullscreen") && { id: "pm-fullscreen", icon: "bi-arrows-fullscreen", tone: "slate", label: t("play.fullscreen.label"), value: tone(switches.isFullscreen), on: Boolean(switches.isFullscreen), onClick: () => act(() => set({ isFullscreen: !switches.isFullscreen })) },
    ];
    // LES MESURES À L'ÉCRAN : les statistiques et la vue discrète s'excluent — un choix à trois cases le dit.
    const measures = switches.isResourceMonitoringVisible ? "panel" : switches.isCompactStatsVisible ? "compact" : "none";

    // Les entrées de chaque groupe, dans l'ordre ; un groupe sans entrée ne s'affiche pas.
    // LA PARTIE (chantier 69 ; le porteur, 2026-11-19) : les sauvegardes d'état y viennent des tuiles ; le retour et
    // l'avance rapides sont partis à l'écran (PlayShell.jsx), là où l'on voit ce qu'ils font.
    const game = [
        playing && emulates && extra.saves && <NavRow key="states" id="pm-states" icon="bi-floppy" tone="teal" label={t("play.states.title")} help={t("play.states.tile")} onOpen={() => openPanel("states")} />,
        playing && emulates && disks.count > 1 && <NavRow key="disk" id="pm-disk" icon="bi-disc" tone="teal" label={t("play.disk.title")} help={t("play.disk.help")} value={t("play.disk.number", { number: disks.current + 1 })} onOpen={() => openPanel("disk")} />,
        playing && emulates && extra.cheats?.cheatsSupported && <NavRow key="cheats" id="pm-cheats" icon="bi-braces" tone="amber" label={t("play.cheats.title")} help={t("play.cheats.help")} onOpen={() => openPanel("cheats")} />,
    ].filter(Boolean);
    // LA MANETTE : seulement ce que touche le pouce de CET appareil ; les réglages de la manette à l'écran tant qu'elle est
    // montrée — dont les sticks invisibles, la même ligne que dans Réglages › Manettes.
    const pad = [
        !isWatching && switches.isControllerVisible && <SwitchRow key="haptic" id="pm-haptic" label={t("play.haptic.label")} help={t("play.haptic.help")} checked={device.hapticFeedback} onChange={setHapticFeedback} />,
        !isWatching && switches.isControllerVisible && <InvisibleSticksRow key="invisible-sticks" id="pm-invisible-sticks" />,
        // L'OPACITÉ (#758) : celle de « Modifier la disposition », pour la disposition en jeu ; la manette la suit aussitôt.
        !isWatching && (switches.isControllerVisible || touchpadAlone) && padOpacity.name && <PadOpacityRow key="opacity" id="pm-pad-opacity" value={padOpacity.opacity ?? DEFAULT_PAD_OPACITY} onChange={setPadOpacity} />,
        !isWatching && session.consoleId === "ps1" && <SwitchRow key="dualshock" id="pm-dualshock" label={t("play.dualShock.label")} help={t("play.dualShock.help")} checked={switches.isDualShockController} onChange={(on) => set({ isDualShockController: on })} />,
        !watchOnly && <SwitchRow key="rumble" id="pm-rumble" label={t("play.rumble.label")} help={t("play.rumble.help")} checked={device.rumble} onChange={setRumbleEnabled} />,
        // LE CLAVIER, déclaré par la plateforme (#420) : streamex seul tape dans son émulateur.
        !isWatching && extra.keyboard && <NavRow key="keyboard" id="pm-keyboard" icon="bi-keyboard" tone="violet" label={t("play.keyboard.title")} help={t("play.keyboard.help")} onOpen={() => openPanel("keyboard")} />,
    ].filter(Boolean);
    // LES JOUEURS (chantier 68 ; le porteur, 2026-11-19) : une ligne par manette de la console — son joueur (connecté, et
    // depuis quand), un joueur parti (« déconnecté », 30 s, PlayShell.jsx), le propriétaire absent (sa place l'attend ; le
    // décompte de l'arrêt quand plus personne ne joue), ou une place libre. UN COMPTE QUI NE JOUE PAS la demande : une
    // place tenue, la manette de ce joueur ; une place libre, rejoindre la partie — après confirmation.
    const canAsk = !watchOnly && isWatching && !loan.canReturn;
    const playerRows = !watchOnly ? playersRows({ players, roster, departed, consoleMax: consoleMaxPlayers, now, self, t, nameOf, canAsk, freeSlots, ask, askForPad, join: () => act(() => sessionSend({ type: "requestslot" })) }) : [];
    // À PLUSIEURS : ce qui concerne d'autres personnes — la manette qui passe de main en main, puis le partage.
    const together = [
        // PRENDRE LA MANETTE : tout le monde arrive en spectateur, le propriétaire aussi ; c'est le geste qui démarre
        // une partie, et le retour après un rechargement. Les autres rejoignent par une place libre de la section Joueurs.
        isWatching && !watchOnly && isOwner && <NavRow key="join" id="pm-join" icon="bi-controller" tone="green" label={t("play.join")} help={t("play.joinHelp")} opens="action" onOpen={() => act(() => sessionSend({ type: "requestslot" }))} />,
        loan.canReclaim && <NavRow key="reclaim" id="pm-reclaim" icon="bi-arrow-counterclockwise" tone="green" label={t("play.handover.reclaim")} help={t("play.handover.reclaimHelp")} opens="action" onOpen={() => act(() => sessionSend({ type: "reclaimslot" }))} />,
        loan.canReturn && <NavRow key="return" id="pm-return" icon="bi-arrow-return-left" tone="green" label={t("play.handover.return")} help={t("play.handover.returnHelp")} opens="action" onOpen={() => act(() => sessionSend({ type: "returnslot" }))} />,
        !isWatching && !loan.canReclaim && giveable.length > 0 && (
            <NavRow key="give" id="pm-give" icon="bi-hand-index-thumb" tone="green" label={t(lending ? "play.handover.lendTitle" : "play.handover.giveTitle")} help={t(lending ? "play.handover.lendHelp" : "play.handover.giveHelp")}
                opens={giveable.length === 1 ? "confirm" : "page"} onOpen={() => (giveable.length === 1 ? confirmGive(giveable[0].username) : openPanel("give"))} />
        ),
        // LE PARTAGE, dès que quelqu'un peut rejoindre : sans adresse sur le réseau, le lien pointerait vers le
        // localhost de qui le reçoit (`canInvite`). Copier OU envoyer, jamais les deux.
        canInvite() && invitesAccounts && !watchOnly && <NavRow key="invite" id="pm-invite" icon="bi-person-plus" tone="green" label={t("play.invite.title")} help={t("play.invite.help")} onOpen={() => openPanel("invite")} />,
        canInvite() && (canShareLink()
            ? <NavRow key="send" id="pm-send" icon="bi-send" tone="green" label={t("play.share.send")} help={t("play.share.sendHelp")} opens="action" onOpen={sendLink} />
            : <NavRow key="copy" id="pm-copy" icon="bi-link-45deg" tone="green" label={t("play.share.copy")} help={t("play.share.copyHelp")} opens="action" onOpen={copyLink} />),
        // LE QR CODE DU MÊME LIEN, pour un téléphone à côté : l'ouvrir, c'est regarder la partie, puis la rejoindre s'il
        // reste une manette. Son panneau (`qr`) le montre en grand, seul : le lien se copie par sa propre ligne (le
        // porteur, 2026-11-14 : « Dans QR code pas besoin de mettre le lien. Car on l'a déjà dans copy the link »).
        canInvite() && <NavRow key="qr" id="pm-qr" icon="bi-qr-code" tone="green" label={t("play.share.qr")} help={t("play.share.qrHelp")} onOpen={() => openPanel("qr")} />,
    ].filter(Boolean);
    const picture = [
        // LA VIDÉO (chantier 58) : toujours là (le porteur, 2026-11-16 : « Le second flux s'affiche toujours ») — une
        // partie qui n'a qu'un flux le dit dans son panneau.
        <NavRow key="video" id="pm-video" icon="bi-badge-hd" tone="sky" label={t("play.video.title")} help={t("play.video.short")} value={videoChoiceLabel(t, switches)} onOpen={() => openPanel("video")} />,
        // L'ORIENTATION DE L'ÉCRAN (#758) : les lignes de Réglages › Application, le même réglage ; choisie ici, elle vaut
        // tout de suite, la partie continue. Le spectateur sans compte ne l'a pas (son menu est au strict minimum).
        !watchOnly && offersOrientation(turnsTheApp) && <OrientationRows key="orientation" idPrefix="pm" turnsTheApp={turnsTheApp} onFailed={setTurnFailed} />,
        can("castToTv") && !watchOnly && <SwitchRow key="cast" id="pm-cast" label={t("play.cast.label")} help={t("play.cast.help")} checked={switches.isChromeCasting} onChange={(on) => set({ isChromeCasting: on })} />,
        !watchOnly && <ChoiceRow key="measures" id="pm-measures" label={t("play.stats.label")} help={t("play.stats.help")} value={measures}
            options={[["none", t("play.stats.none")], ["compact", t("play.stats.compact")], ["panel", t("play.stats.panel")]]}
            onChange={(next) => set({ isResourceMonitoringVisible: next === "panel", isCompactStatsVisible: next === "compact" })} />,
    ].filter(Boolean);
    // LES INFORMATIONS DU SPECTATEUR SANS COMPTE : ce que la partie dit d'elle ; le menu d'un joueur les a ailleurs (la
    // tuile Reprendre, la page du jeu).
    const info = watchOnly ? [
        <InfoRow key="console" icon="bi-controller" tone="indigo" label={t("play.info.console")} value={consoleName} />,
        <InfoRow key="owner" icon="bi-person" tone="green" label={t("play.info.owner")} value={session.owner || ""} />,
        <InfoRow key="duration" icon="bi-clock" tone="amber" label={t("play.info.duration")} value={playTime || ""} />,
        <InfoRow key="server" icon="bi-hdd-network" tone="sky" label={t("play.info.server")} value={hostName} />,
    ] : [];
    // `after` : ce qui se lit sous les lignes d'un groupe (l'avis d'une orientation refusée).
    const group = (key, title, rows, after = null) => rows.length > 0 && <Section key={key} title={title}><Rows>{rows}</Rows>{after}</Section>;

    return (
        <>
            <SidePanel label={input.title || session.title || t("play.menu.title")} firstKey="pm-resume" onClose={closeMenu}>
                <QuickTiles items={tiles} />
                {/* LES NOTIFICATIONS DE LA PARTIE (chantier 68, Bell.jsx), en tête : toute la partie, la plus récente en haut, 5
                    par page ; une demande en attente se répond ici. TOUJOURS LÀ, même vide (le porteur, 2026-11-19 : « fais
                    comme sur la page d'accueil ») : sa ligne « Aucune notification. ». Pas pour le spectateur sans compte. */}
                {notifications && !watchOnly && (
                    <Section title={t("play.notifications.title")}>
                        {notifications.list.length > 0
                            ? <Listing items={notifications.list} pageSize={5} render={(entry) => <NotificationRow key={entry.id} entry={entry} nameOf={nameOf} ask={ask} answer={(yes) => act(() => notifications.answer(entry.id, yes))} />} />
                            : <EmptyState icon="bi-bell" text={t("notifications.none")} />}
                    </Section>
                )}
                {watchOnly && <Section title={t("play.menu.info")} help={t("play.info.watchOnly")}><Rows>{info}</Rows></Section>}
                {group("game", t("play.menu.game"), game)}
                {group("pad", t("play.menu.controller"), pad)}
                {group("players", t("play.players.title"), playerRows)}
                {group("together", t("play.menu.together"), together)}
                {group("picture", t("play.menu.picture"), picture, turnFailed && <OrientationFailed turnsTheApp={turnsTheApp} />)}
                {/* SORTIR : les deux gestes qui mettent fin à ce qu'on fait, à part, en bas — loin des réglages qu'on
                    touche en jouant. Remettre à zéro se confirme. */}
                {group("leave", t("play.menu.leave"), [
                    playing && emulates && extra.reset && (
                        <NavRow key="reset" id="pm-reset" icon="bi-arrow-counterclockwise" tone="rose" label={t("play.reset.title")} help={t("play.reset.help")} opens="confirm"
                            onOpen={() => ask({ title: t("play.reset.confirmTitle"), message: t("play.reset.confirmMessage"), confirm: t("play.reset.confirm"), danger: true, onConfirm: () => act(() => extra.reset()) })} />
                    ),
                    <NavRow key="quit" id="pm-quit" icon="bi-box-arrow-left" tone="rose" label={t("play.menu.quit")} help={t(isWatching ? "play.menu.quitHelpWatching" : "play.menu.quitHelp")} opens="confirm" onOpen={onQuit} />,
                ].filter(Boolean))}
            </SidePanel>

            {panels.filter((name) => SUBPANELS.includes(name)).map((name) => {
                switch (name) {
                    case "states":
                        return <SaveStates key={name} session={session} saves={extra.saves} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "disk":
                        return <DiskPanel key={name} disks={disks} change={extra.disks?.change} context={context} onChange={setDisks} onClose={closePanel} />;
                    case "cheats":
                        return <CheatsPanel key={name} cheats={extra.cheats} context={context} onClose={closePanel} />;
                    case "give":
                        return <PeoplePanel key={name} label={t(lending ? "play.handover.lendTitle" : "play.handover.giveTitle")} help={t(lending ? "play.handover.lendHelp" : "play.handover.giveHelp")} empty={t("play.handover.noneToGive")}
                            people={giveable.map((watcher) => ({ username: watcher.username, help: t("play.handover.isWatching") }))} nameOf={nameOf} onPick={confirmGive} picks="confirm" onClose={closePanel} />;
                    case "invite":
                        return <InvitePanel key={name} session={session} self={self} ask={ask} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "keyboard":
                        return <KeyboardPanel key={name} typeText={extra.keyboard?.typeText} context={context} onClose={closePanel} onDone={closeMenu} />;
                    case "video":
                        return <VideoPanel key={name} switches={switches} set={set} platform={session.platform} onClose={closePanel} />;
                    case "qr":
                        return (
                            <SidePanel key={name} label={t("play.share.qr")} onClose={closePanel}>
                                <Section title={t("play.share.qrTitle")} help={t("play.share.qrHelp")}>
                                    <div className="bg-white rounded-4 p-3 mx-auto gf-qr">
                                        <img className="d-block w-100" src={qrImage(shareUrl)} alt={t("play.share.qr")} />
                                    </div>
                                </Section>
                            </SidePanel>
                        );
                    case "link":
                        return (
                            <SidePanel key={name} label={t("play.share.copyFailedTitle")} firstKey="pk-link-copy" onClose={closePanel}>
                                <Section title={t("invitations.linkTitle")} help={t("play.share.copyFailedMessage")}>
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
// Chacun est une entrée (sa vignette, sa date) qui ouvre son panneau — aucun geste sur la ligne (Bricks.jsx) :
// sauver ici, ou écraser, charger, chaque remplacement confirmé sur place, et la suppression en bas. Sauver ou
// charger ferme le menu, pour qu'on voie le jeu repartir.
function SaveStates({ session, saves, context, onClose, onDone }) {
    const { t, i18n } = useTranslation();
    const confirmWith = useConfirm();
    const [slots, setSlots] = useState(null);
    const [open, setOpen] = useState(null);
    const refresh = () => listSaveSlots(session.consoleId, session.reference).then((answer) => setSlots(answer.ok ? answer.data || [] : []));

    useEffect(() => {
        refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const when = (info) => (info?.exists && info.timestamp ? new Date(info.timestamp).toLocaleString(i18n.language) : t("play.states.empty"));
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
        const answer = await deleteSaveSlot(session.consoleId, session.reference, slot);

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
                <Section title={t("saves.slotsTitle")} help={t("play.states.help")}>
                    {slots === null ? <Note className="mt-0">{t("play.loading")}</Note> : (
                        <Rows>
                            {Array.from({ length: 10 }, (_, slot) => (
                                <NavRow key={slot} id={`ps-slot-${slot}`} label={nameOf(slot)} help={when(info(slot))} onOpen={() => setOpen(slot)}
                                    lead={<SlotThumbnail session={session} slot={slot} info={info(slot)} />} />
                            ))}
                        </Rows>
                    )}
                </Section>
            </SidePanel>
            {open !== null && (
                <SidePanel key={open} label={nameOf(open)} subtitle={when(info(open))} firstKey={info(open)?.exists ? "ps-load-ask" : "ps-save"} onClose={() => closeSlot(open)}>
                    <Section title={t("saves.stateTitle")}>
                        <SlotThumbnail session={session} slot={open} info={info(open)} />
                    </Section>
                    {/* Ses gestes en lignes, dans « Plus » (le porteur, 2026-11-13 : un geste orphelin est une ligne) ;
                        charger et écraser se confirment dans la fenêtre de Skeleton. Le premier emplacement s'écrit
                        seul, en quittant la partie : on n'y sauvegarde pas. */}
                    {(info(open)?.exists || open !== 0) && (
                        <Section title={t("more")}>
                            <Rows>
                                {info(open)?.exists && (
                                    <NavRow id="ps-load-ask" icon="bi-box-arrow-in-down" tone="teal" label={t("play.states.load")} help={t("play.states.loadConfirm")} opens="confirm"
                                        onOpen={() => confirmWith({ title: t("play.states.load"), message: t("play.states.loadConfirm"), onConfirm: () => load(open) })} />
                                )}
                                {open !== 0 && (info(open)?.exists
                                    ? <NavRow id="ps-save-ask" icon="bi-floppy" tone="teal" label={t("play.states.overwrite")} help={t("play.states.overwriteConfirm")} opens="confirm"
                                        onOpen={() => confirmWith({ title: t("play.states.overwrite"), message: t("play.states.overwriteConfirm"), onConfirm: () => save(open) })} />
                                    : <NavRow id="ps-save" icon="bi-floppy" tone="teal" label={t("play.states.save")} opens="action" onOpen={() => save(open)} />)}
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
        fetch(`${screenshotUrl(session.consoleId, session.reference, slot)}&v=${encodeURIComponent(info.timestamp || "")}`, { headers: authHeaders(streamAuthToken() || readToken()) })
            .then((response) => (response.ok ? response.blob() : null))
            .then((blob) => {
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
function DiskPanel({ disks, change, context, onChange, onClose }) {
    const { t } = useTranslation();
    const choose = async (index) => {
        onChange({ ...disks, current: index });
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
            <Section title={t("play.disk.listTitle")}>
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
// modifier, le retirer (aucun geste sur la ligne : Bricks.jsx).
function CheatsPanel({ cheats, context, onClose }) {
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
                    <Section title={t("play.cheats.addTitle")} help={t("play.cheats.help")}>
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
                <Section title={t("play.cheats.list")}>
                    {entries.length ? (
                        <Rows>
                            {entries.map((entry, index) => (
                                <NavRow key={index} id={`pt-cheat-${index}`} icon={entry.enabled ? "bi-magic" : "bi-slash-circle"} tone={entry.enabled ? "amber" : "slate"} label={entry.description || entry.code}
                                    help={`${entry.description ? `${entry.code} · ` : ""}${t(entry.enabled ? "settings.switchOn" : "settings.switchOff")}`} onOpen={() => setOpen(index)} />
                            ))}
                        </Rows>
                    ) : <EmptyState icon="bi-braces" text={t("play.cheats.none")} />}
                </Section>
            </SidePanel>
            {open !== null && entries[open] && (
                <CheatPanel key={open} entry={entries[open]} pending={pending} onClose={() => closeEntry(open)}
                    onSave={(next) => push(entries.map((item, at) => (at === open ? next : item)), () => closeEntry(open))}
                    onToggle={(enabled) => push(entries.map((item, at) => (at === open ? { ...item, enabled } : item)))}
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
            <Section title={t("play.cheats.usageTitle")}>
                <Rows>
                    <SwitchRow id="pc-enabled" label={t("play.cheats.apply")} help={t("play.cheats.applyHelp")} checked={Boolean(cheat.enabled)} onChange={onToggle} />
                </Rows>
            </Section>
            <Form props={{ onSubmit: save }}>
                <Section title={t("play.cheats.code")}>
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
const countdown = (until, now) => `-${formatDuration(now, until)}`;
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
        const name = held ? nameOf(held.username) : gone ? nameOf(gone.username) : owned ? nameOf(roster.owner) : t("play.players.nobody");
        const place = t("play.players.place", { slot, max });
        const help = held ? [place, t("play.players.connected"), formatDuration(held.joinedAt, now)].filter(Boolean).join(" • ")
            : gone ? [place, t("play.players.disconnected")].join(" • ")
                : owned ? [place, t("play.players.disconnected"), idleEnd ? countdown(idleEnd, now) : ""].filter(Boolean).join(" • ")
                    : [place, t("play.players.free")].join(" • ");
        const icon = held ? "bi-person-fill" : gone || owned ? "bi-person-dash" : "bi-controller";
        const tone = held ? "green" : "slate";
        const askable = canAsk && held && held.username !== self && !held.lentTo;
        const joinable = canAsk && !held && !gone && !owned && freeSlots > 0;

        if (askable) {
            rows.push(<NavRow key={slot} id={`pm-player-${slot}`} icon={icon} tone={tone} label={name} help={help} opens="confirm"
                onOpen={() => ask({ title: t("play.players.askTitle"), message: t("play.players.askMessage", { name }), confirm: t("play.players.ask"), onConfirm: () => askForPad(held.username) })} />);
        } else if (joinable) {
            // Une place libre : « > » (le porteur, 2026-11-19 : « "?" quand c'est déjà pris par un joueur, et ">" quand
            // c'est un free slot ») ; elle se confirme quand même.
            rows.push(<NavRow key={slot} id={`pm-player-${slot}`} icon={icon} tone={tone} label={name} help={help} opens="page"
                onOpen={() => ask({ title: t("play.players.joinTitle"), message: t("play.players.joinMessage"), confirm: t("play.join"), onConfirm: join })} />);
        } else {
            rows.push(<InfoRow key={slot} icon={icon} tone={tone} label={name} value={help} />);
        }
    }

    return rows;
}

// UNE NOTIFICATION DU MENU : une demande en attente se touche (la confirmation : céder, ou garder) ; le reste se lit,
// une demande réglée avec son issue.
// Son sous-titre, quand il nomme quelqu'un, se lit à l'affichage (`subtitle(nameOf)`) : le nom affiché arrive parfois après
// la notification.
function NotificationRow({ entry, nameOf, ask, answer }) {
    const { t } = useTranslation();
    const outcome = entry.asks && entry.state !== "pending" ? t(`play.notifications.state.${entry.state}`) : "";
    const subtitle = typeof entry.subtitle === "function" ? entry.subtitle(nameOf) : entry.subtitle;
    const help = [subtitle, outcome].filter(Boolean).join(" • ");

    if (entry.asks && entry.state === "pending") {
        return (
            <NavRow id={`pm-notification-${entry.id}`} icon="bi-controller" tone="green" label={entry.title} help={help} opens="confirm"
                onOpen={() => ask({ ...entry.confirm, confirm: entry.confirm.confirmLabel, onConfirm: () => answer(true), onCancel: () => answer(false) })} />
        );
    }

    return <InfoRow icon={entry.asks ? "bi-controller" : "bi-bell"} tone={entry.asks ? "green" : "sky"} label={entry.title} value={help} />;
}

function PeoplePanel({ label, help, empty, people, nameOf, onPick, picks, onClose }) {
    const { t } = useTranslation();

    return (
        <SidePanel label={label} onClose={onClose}>
            <Section title={t("play.handover.peopleTitle")} help={help}>
            {people.length ? (
                <Rows>
                    {people.map((person) => <NavRow key={person.username} id={`pp-${person.username}`} lead={<Avatar name={nameOf(person.username)} />} label={nameOf(person.username)} help={person.help} opens={picks} onOpen={() => onPick(person.username)} />)}
                </Rows>
            ) : <EmptyState icon="bi-people" text={empty} />}
            </Section>
        </SidePanel>
    );
}

// INVITER UN MEMBRE : une notification (en direct s'il est connecté, en push sinon) qui ouvre la partie.
function InvitePanel({ session, self, ask, context, onClose, onDone }) {
    const { t } = useTranslation();
    const [members, setMembers] = useState(null);

    useEffect(() => {
        let cancelled = false;
        listMembers(readToken()).then((answer) => !cancelled && setMembers(answer.ok ? answer.item?.members || [] : []));

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
                const answer = await sendInvite(readToken(), member.username, session.platform, session.consoleId, session.reference, session.id);
                onDone();
                context.toast.open(t(answer.ok ? "play.invite.sent" : "play.invite.failed", { name }), { variant: answer.ok ? "success" : "error" });
            },
        });
    };
    const visible = (members || []).filter((member) => member.username !== self);

    return (
        <SidePanel label={t("play.invite.title")} onClose={onClose}>
            <Section title={t("members.listTitle")} help={t("play.invite.help")}>
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
                <Section title={t("play.keyboard.text")} help={t("play.keyboard.help")}>
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

function VideoPanel({ switches, set, platform, onClose }) {
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
            <Section title={t("play.video.streamTitle")}>
                {options.length > 0 && (
                    <Rows>
                        <RadioRow id="pv-quality" help={t("play.video.short")} value={value} onChange={(next) => set({ videoQuality: next })} options={options} disabled={single} />
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
const quitChoices = (retro, remote = false) => ["kill", ...(remote ? ["standby"] : []), "leave", ...(retro ? ["pause"] : [])];
const quitChoiceOf = (values, retro, remote = false) => (quitChoices(retro, remote).includes(values.onQuit) ? values.onQuit : "kill");

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
        const { session, isWatching, isOwner, borrowing, store } = latest.current;
        const canEnd = isOwner && !isWatching && !borrowing;
        const retro = session.platform === "retrogaming" || session.platform === "device";
        const remote = session.platform === "remoteplay";
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
