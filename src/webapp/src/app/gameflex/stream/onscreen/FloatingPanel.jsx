import React, { useContext, useEffect, useState, useSyncExternalStore } from "react";
import Skeleton from "../../../skeleton/skeleton.js";
import { isOwnSession, nameIn, useDisplayNames } from "../../libs/SessionPool.js";
import { useAuth } from "../../profile/auth/AuthContext.jsx";
import {
    OffCanvas,
    SectionPanel,
    ListElement,
} from "../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
import { InvitePanel } from "../onscreen/menu/InvitePanel.jsx";
import {
    buildStreamingShareUrl,
    buildDeviceShareUrl,
    isDeviceSession,
} from "../../libs/sessionLinks.js";
import { canInvite, streamHostOrigin } from "../../libs/serverBase.js";
import { can, capabilities } from "../../libs/capabilities.js";
import { copyTextToClipboard, canShareLink, shareLink } from "../../shared/utils/share-link.js";
import { CheatsPanel } from "../onscreen/menu/CheatsPanel.jsx";
import { KeyboardPanel } from "../onscreen/menu/KeyboardPanel.jsx";
import { SaveStatesPanel } from "../onscreen/menu/SaveStatesPanel.jsx";
import { HandoverPanel, handoverCandidates, giveCandidates } from "../onscreen/menu/HandoverPanel.jsx";
import { describeResources } from "../../libs/hostResources.js";


// The floating settings button + offcanvas — built entirely from
// input.options/input.controller/input.resourceMonitoringRef. A field left
// undefined/null simply doesn't render its section (same "undefined = no
// section" contract every menu in this app already uses).
export function FloatingPanel({ t, session, input, title = null, isWatching, switchesStore, controller, sessionSend, players, watchers = [], loan = {}, onQuit = null }) {
    const { user, token } = useAuth();
    // Mon nom DANS CETTE PARTIE : celui de mon compte chez l'hôte quand elle
    // vit ailleurs (voir nameIn). Les joueurs et les spectateurs viennent de
    // son child et portent ses noms à lui.
    const self = nameIn(session, user?.username);
    const context = useContext(Skeleton.context);
    // Read live from the store (see createSwitchesStore's own doc above) —
    // NOT from a props snapshot, which would be frozen to whatever it was
    // when this panel was opened.
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);

    // LES DISQUES, demandés à l'ouverture du menu.
    //
    // Un jeu PS1 en plusieurs disques est un .m3u et ses .cue : sans ce
    // sélecteur, arriver au « insérez le disque 2 » signifiait la fin de la
    // partie. On DEMANDE, et on n'affiche rien quand la réponse annonce zéro
    // ou un disque — ce qui couvre du même geste le jeu à disque unique, le
    // core sans interface de disques, et l'hôte trop ancien pour savoir
    // répondre. Aucun drapeau de capacité à tenir à jour des deux côtés,
    // donc aucun qui puisse finir par mentir.
    //
    // C'est le hook de plateforme qui sait à QUI poser la question — le
    // serveur par sa WS, le téléphone par son API locale. Ce panneau ne le
    // sait pas, et c'est le contrat.
    const askDisks = input.extraProps?.disks;
    // La FONCTION en dépendance, pas l'objet qui la porte : `extraProps` est
    // reconstruit à chaque rendu du hook, et s'y accrocher relancerait la
    // question à chaque rendu — une requête en boucle pour une réponse qui ne
    // change pas.
    const listDisks = askDisks?.list;
    const [disks, setDisks] = useState({ count: 0, current: -1 });
    useEffect(() => {
        if (!listDisks) {
            return undefined;
        }
        let cancelled = false;
        Promise.resolve(listDisks()).then((found) => {
            if (!cancelled && found) setDisks(found);
        });

        return () => { cancelled = true; };
    }, [listDisks]);
    const contextForm = {
        values: switches, setValues: (values) => {
            context.offcanvas.close();
            switchesStore.setValues(values);
        }
    };
    // The handover request rides the façade WS (`requestslot`); the kernel
    // routes the prompt to the holder and the result back. `freeSlots` and
    // `swapDevice` lived here until v1 removed join/takeover from this panel —
    // see the comment on the participate section below for where to find them.
    // Everyone who could be asked, not just the first one found. With four
    // pads in play "the current player" is not a single person, and picking
    // the first match left the other three unreachable.
    const askable = handoverCandidates(players, self);
    // The other direction. `watchers` comes from the child alone, so on a v1
    // session it is empty and this section simply is not there.
    const giveable = giveCandidates(watchers, self);
    // A username is an account; a display name is what people call each other.
    // Anything addressed to a human names them the way they chose.
    const displayNames = useDisplayNames();
    const givePad = (username) => {
        sessionSend({ type: "giveslot", to: username });
        context.toast.open(
            t(lending ? "streaming.handover.lent" : "streaming.handover.given", { username: displayNames[username] || username }),
            { variant: "info" },
        );
        context.offcanvas.close();
    };
    const askForPad = (username) => {
        sessionSend({ type: "requestslot", to: username });
        context.toast.open(t("streaming.handover.sent"), { variant: "info" });
        context.offcanvas.close();
    };
    // Owner-only powers (2026-08-25). A JOINER is a guest on somebody
    // else's session: they play, they do not get to end it, overwrite its
    // save states, rewind it under the other players, or turn on cheats.
    // `!isWatching` was the wrong gate — it is true for every player, and a
    // joiner is a player.
    const isOwner = session.me?.role === "owner" || isOwnSession(session, user?.username);
    // DEUX GESTES pour passer sa manette (#274) : le propriétaire la PRÊTE —
    // il garde sa place et la reprend quand il veut — ; un invité DONNE sa
    // place, et la perd.
    const lending = isOwner;
    const giveKey = (key) => `streaming.handover.${lending ? key.replace("give", "lend") : key}`;
    const freeSlots = session.openSlots ?? 0;
    const saveLoad = input.extraProps?.saves;
    const resetConsole = input.extraProps?.reset;
    // Un jeu qui tourne SUR CET APPAREIL n'a pas les mêmes réglages qu'un
    // flux venu d'un serveur, et afficher un interrupteur qui ne fait rien est
    // pire que ne rien afficher — signalé le 2026-08-27.
    //
    // La liste s'est BEAUCOUP réduite le 2026-09-01, et c'était une faute de
    // ma part : ce test comparait un nom de plateforme supprimé depuis
    // C7-P3, il était donc TOUJOURS FAUX et ne masquait rien. En le
    // réparant, j'ai rendu vivants cinq masquages écrits pour un autre
    // état du produit — et le menu du téléphone a perdu d'un coup le
    // partage par lien et les performances, sans que personne l'ait demandé.
    //
    // Ce qui tombe encore, et seulement ça :
    //   son          — il sort du haut-parleur, pas du flux (le flux n'a pas
    //                  encore de piste audio : voir AudioCapture) ;
    //   cast         — il n'y a pas de page à envoyer sur un téléviseur, et
    //                  le partage se fait par LIEN, comme sur le web.
    //
    // Le reste — partage, performances, sauvegardes, voyage dans le temps —
    // est le même menu que le web. La règle est là : l'application montre
    // l'interface du web, à quelques fonctions près, et chaque exception se
    // justifie ligne à ligne ou n'existe pas.
    // SIX GARDES, SIX FAITS — plus un seul « suis-je un téléphone ? ».
    //
    // Il y avait ici `const onDevice = isNativeHost()`, qui cachait à lui
    // seul le son, le plein écran, le cast, le rembobinage et l'invitation.
    // Un test de plateforme employé six fois pour six questions différentes :
    // le jour où l'une d'elles change, les cinq autres changent avec.
    //
    // Et il avait DÉJÀ menti une fois. Le test lisait
    // `session.platform === "device"` ; cette plateforme a été supprimée par
    // C7-P3, donc il était toujours faux, et les gardes ne gardaient plus
    // rien — le menu du téléphone réaffichait tout. Une garde qui ne garde
    // plus est pire qu'une garde absente : elle donne l'impression que la
    // question est traitée.
    //
    // Chacune est maintenant DÉCLARÉE par l'hôte (C8-P5), et se lit toute
    // seule : `pageAudio` dit que couper le son de la page couperait
    // quelque chose, `rewind` que le cœur tient un historique d'états.
    // « Inviter quelqu'un » n'a pas la même forme partout : sur un serveur on
    // invite un COMPTE, sur un appareil on partage un lien que qui l'ouvre
    // rejoint. `invites` porte déjà cette différence depuis toujours.
    const invitesAccounts = capabilities().invites === "accounts";
    // Le voyage dans le temps appartient aux plateformes qui émulent — une
    // question de session, pas d'hôte. Les deux n'en feront plus qu'une
    // après C8-P4.
    const timeTravelPlatform = session.platform === "retrogaming"
        || session.platform === "device";
    // Le lien à envoyer. Une partie du téléphone n'a pas d'identifiant de
    // session à partager — voir buildDeviceShareUrl.
    const shareUrl = isDeviceSession(session)
        ? buildDeviceShareUrl()
        : buildStreamingShareUrl({ sessionId: session.id });
    const cheats = input.extraProps?.cheats?.cheatsSupported;
    const keyboard = input.extraProps?.keyboard;
    const [isTyping, setTyping] = useState(false);
    // DEUX questions, pas une. `can("rewind")` dit si l'HÔTE tient un
    // historique d'états (le runtime du téléphone, lui, n'en tient aucun) ;
    // `rewindSupported` dit si CE CŒUR-LÀ peut le faire sans tout casser —
    // un état PS2 pèse 44 Mio, le sérialiser chaque seconde coûtait une
    // image par seconde, et donc des coupures de son (2026-09-08). La
    // réponse vient de la partie elle-même (message "started"), jamais d'une
    // liste de consoles tenue ici.
    const rewindSupported = input.extraProps?.timeTravelProps?.rewindSupported;
    // L'AVANCE RAPIDE AUSSI, déclarée par la partie : un invité de bureau du
    // téléphone (RPCS3, xemu…) ne la sait pas, et sa section ne proposait plus
    // qu'un interrupteur sans effet (le porteur, 2026-10-28 : « je peux voir le
    // rewind alors que c'est pas possible sur streamex »).
    const fastForwardSupported = input.extraProps?.timeTravelProps?.fastForwardSupported;
    const rewindOffered = can("rewind") && rewindSupported;

    return (
        // LE TITRE DU JEU EST ICI (#408) : il a quitté l'écran de jeu, où il
        // tenait la barre du bas. Le porteur, le 2026-10-19 : « on l'enlève
        // pour le moment, peut-être le mettre dans menu ».
        <OffCanvas icon="bi bi-controller" label={title} helpText={title ? t("streaming.menu.help") : null}>
            <div style={{ minHeight: 0, overflowY: "auto" }}>
                <SectionPanel
                    label={t("streaming.settings.general.title")}
                    helpText={t("streaming.settings.general.help")}
                >
                    {/* chantier-4 — TAKE THE PAD. In v4 everybody arrives watching,
                        the owner included: opening a link grants nothing. So the
                        one deliberate act that starts a session has to be visible
                        HERE, or nobody ever plays.

                        Offered whenever this tab is watching and the session has
                        room for us. For the owner that is always true — their slot
                        is reserved and cannot be taken — so it doubles as their way
                        back after a refresh, without a second concept. */}
                    {isWatching && (isOwner || freeSlots > 0) && (
                        <ListElement
                            light={true}
                            icon="bi bi-controller"
                            label={t("streaming.settings.join.action")}
                            helpText={t("streaming.settings.join.help")}
                            onClick={() => {
                                sessionSend({ type: "requestslot" });
                                context.offcanvas.close();
                            }}
                            next={null}
                        />
                    )}
                    {/* v1 (chantier-3-modele-v1-release.md) — JOIN and TAKEOVER are
                        gone from this panel:
                        - join moves to the Dashboard, where every intent is now
                            sent (R5). It is not lost, it changes address.
                        - takeover is deleted outright (R7): the kernel no longer
                            arbitrates identity across sockets, so there is nothing
                            left for it to ask for.
                        Both are DELETED rather than commented out: git keeps a
                        version that still compiles, a comment would only rot. To
                        restore them, look at this file in commit f3592c8 — the last
                        one before the v1 model, where the whole three-entry
                        "participate" section is intact along with `swapDevice` /
                        `freeSlots` and their i18n keys.

                        HANDOVER stays, deliberately: FloatingPanel.jsx is the ONLY
                        place that emits `requestslot`, so removing it too would
                        leave V1-P3 (the 15 s pad reservation) with no entry point at
                        all, and the holder's accept/deny prompt in Streaming.jsx
                        dead. It moves only once the Dashboard can send the request
                        instead (V1-P5) — never remove the sole entry point before
                        its replacement exists. */}
                    {/* LES DEUX BOUTS D'UN PRÊT (2026-10-09). Une passation prête
                        la manette, elle ne donne pas la partie : le
                        propriétaire peut la reprendre, l'emprunteur la rendre.
                        Le child dit à CET onglet lequel des deux il peut faire
                        — la page ne le déduit pas. */}
                    {loan.canReclaim && (
                        <ListElement
                            light={true}
                            icon="bi bi-arrow-counterclockwise"
                            label={t("streaming.handover.reclaim")}
                            helpText={t("streaming.handover.reclaimHelp")}
                            onClick={() => {
                                sessionSend({ type: "reclaimslot" });
                                context.offcanvas.close();
                            }}
                            next={null}
                        />
                    )}
                    {loan.canReturn && (
                        <ListElement
                            light={true}
                            icon="bi bi-arrow-return-left"
                            label={t("streaming.handover.return")}
                            helpText={t("streaming.handover.returnHelp")}
                            onClick={() => {
                                sessionSend({ type: "returnslot" });
                                context.offcanvas.close();
                            }}
                            next={null}
                        />
                    )}
                    {isWatching && askable.length > 0 && (
                        <ListElement
                            light={true}
                            icon="bi bi-controller"
                            label={t("streaming.participate.handover")}
                            helpText={t("streaming.participate.handoverHelp")}
                            onClick={() => {
                                // One holder, no question to ask: opening a
                                // list of one to tap the only row is a step
                                // that exists purely to be got through.
                                if (askable.length === 1) {
                                    askForPad(askable[0].username);
                                    return;
                                }

                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-controller"
                                        label={t("streaming.participate.handover")}
                                        helpText={t("streaming.handover.chooseHelp")}
                                    >
                                        {/* `players` and not a snapshot: somebody can
                                            take or drop a pad while this is open, and the
                                            list must show who can actually be asked NOW. */}
                                        <HandoverPanel
                                            players={players}
                                            self={self}
                                            onAsk={askForPad}
                                            context={context}
                                            t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                    )}
                    {/* The OTHER direction, and the reason hand-over exists at
                        all. Everything above starts from the person who WANTS
                        the pad; this starts from the person who HAS it — the
                        gesture of holding a controller out to someone.

                        It matters most exactly where it was missing: remote-play
                        has one pad and JOIN is impossible there (a non-owner's
                        smallest slot is 2), so this is the only way the pad ever
                        moves. Offered to whoever holds a pad, whenever there is
                        somebody watching to hand it to. */}
                    {!isWatching && !loan.canReclaim && giveable.length > 0 && (
                        <ListElement
                            light={true}
                            icon="bi bi-hand-index-thumb"
                            label={t(giveKey("giveTitle"))}
                            helpText={t(giveKey("giveHelp"))}
                            onClick={() => {
                                // One watcher, no list: opening a list of one
                                // to tap its only row is a step that exists
                                // purely to be got through. The confirmation
                                // still happens — giving away the pad mid-game
                                // is not a thing to do by brushing a menu.
                                if (giveable.length === 1) {
                                    context.modal.open(
                                        t(giveKey("giveMessage"), {
                                            username: displayNames[giveable[0].username] || giveable[0].username,
                                        }),
                                        {
                                            variant: "confirm",
                                            title: t(giveKey("giveTitle")),
                                            confirmLabel: t(giveKey("give")),
                                            cancelLabel: t("app.cancel"),
                                            onConfirm: () => givePad(giveable[0].username),
                                        },
                                    );
                                    return;
                                }
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-hand-index-thumb"
                                        label={t(giveKey("giveTitle"))}
                                        helpText={t(giveKey("giveHelp"))}
                                    >
                                        <HandoverPanel
                                            direction="give"
                                            lending={lending}
                                            watchers={watchers}
                                            self={self}
                                            onGive={givePad}
                                            context={context}
                                            t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                    )}
                    {can("pageAudio") && (
                        <Form.SkeletonSwitch
                            reverse
                            variant="md"
                            name="isSoundEnabled"
                            context={contextForm}
                            label={t("streaming.settings.sound.label")}
                            helpText={t("streaming.settings.sound.help")}
                            offText={t("streaming.settings.switch.muted")}
                        />
                    )}
                    {/* SANS OBJET DANS L'APPLICATION : elle est déjà en plein
                        écran (le porteur, 2026-10-29). L'hôte le dit faux à
                        sa WebView — la barre d'état et celle de navigation
                        du 2026-09-01 ne sont plus là. Un navigateur le
                        garde : il y range sa barre d'adresse. */}
                    {can("fullscreen") && (
                        <Form.SkeletonSwitch
                            reverse
                            variant="md"
                            name="isFullscreen"
                            context={contextForm}
                            label={t("streaming.settings.fullscreen.label")}
                            helpText={t("streaming.settings.fullscreen.help")}
                        />
                    )}
                    {can("castToTv") && (
                        <Form.SkeletonSwitch
                            reverse
                            variant="md"
                            name="isChromeCasting"
                            context={contextForm}
                            label={t("streaming.settings.chromecast.label")}
                            helpText={t("streaming.settings.chromecast.help")}
                            onText={t("streaming.settings.switch.casting")}
                        />
                    )}
                    {!isWatching && (
                        <>
                            <Form.SkeletonSwitch
                                reverse
                                variant="md"
                                name="isControllerVisible"
                                context={contextForm}
                                label={
                                    controller.label ? t("streaming.settings.controller.label", {
                                        name: controller.label,
                                    }) : t("streaming.settings.controller.labelGeneric")
                                }
                                helpText={t("streaming.settings.controller.help")}
                                onText={t("streaming.settings.switch.visible")}
                                offText={t("streaming.settings.switch.hidden")}
                            />
                            {switches.isControllerVisible && (
                                <Form.SkeletonSwitch
                                    reverse
                                    variant="md"
                                    name="isHapticFeedback"
                                    context={contextForm}
                                    label={t("streaming.settings.hapticFeedback.label")}
                                    helpText={t("streaming.settings.hapticFeedback.help")}
                                />
                            )}
                            {/* SEULEMENT SI LA PLATEFORME L'ÉCOUTE. La fenêtre
                                d'enchaînement est un réglage de streamex ; le
                                rétro et le jeu à distance ne branchent pas
                                `onChainWindowChange`, et l'interrupteur y basculait
                                sans rien changer (relevé en essai le 2026-09-11). */}
                            {input.extraProps?.onChainWindowChange && (
                                <Form.SkeletonSwitch
                                    reverse
                                    variant="md"
                                    name="isChainButtons300"
                                    context={contextForm}
                                    label={t("streaming.settings.chainButtons300.label")}
                                    helpText={t("streaming.settings.chainButtons300.help")}
                                    onText="300 ms"
                                    offText="100 ms"
                                />
                            )}
                            {session.consoleId === "ps1" && (
                                <Form.SkeletonSwitch
                                    reverse
                                    variant="md"
                                    name="isDualShockController"
                                    context={contextForm}
                                    label={t("streaming.settings.dualShock.label")}
                                    helpText={t("streaming.settings.dualShock.help")}
                                    onText={t("streaming.settings.switch.dualshock")}
                                    offText={t("streaming.settings.switch.digitalPad")}
                                />
                            )}
                        </>
                    )}
                    <Form.SkeletonSwitch
                        reverse
                        variant="md"
                        name="isRumbleEnabled"
                        context={contextForm}
                        label={t("streaming.settings.rumble.label")}
                        helpText={t("streaming.settings.rumble.help")}
                    />
                </SectionPanel>
                {/* LE CLAVIER, DÉCLARÉ PAR LA PLATEFORME (#420) : streamex seul
                    injecte des touches dans son émulateur. SA PROPRE SECTION,
                    comme le partage ou la sortie (le porteur, 2026-10-29) :
                    une ligne au milieu des interrupteurs n'avait ni sa place
                    ni ses marges. Il s'ouvre sur place, sous la ligne. */}
                {!isWatching && keyboard && (
                    <SectionPanel
                        label={t("streaming.settings.keyboard.title")}
                        helpText={t("streaming.settings.keyboard.help")}
                    >
                        <ListElement
                            icon="bi bi-keyboard"
                            label={t("streaming.settings.keyboard.open")}
                            helpText={t("streaming.settings.keyboard.openHelp")}
                            onClick={() => setTyping(true)}
                        >
                            {isTyping ? <KeyboardPanel typeText={keyboard.typeText} context={context} t={t} /> : null}
                        </ListElement>
                    </SectionPanel>
                )}
                {/* TWO conditions because they answer two different
                    questions, and each alone lets something through.
                    isOwner: "is this my session?" — a joiner must not
                    overwrite my saves or turn on cheats. !isWatching: "do I
                    hold the pad right now?" — since chantier-4 an owner
                    ARRIVES watching, and rewinding a game you are not
                    playing changes it under whoever is. Reported live
                    2026-08-25: time travel offered in watch mode. */}
                {/* Une question de SESSION, pas d'hôte : ce sont ces deux
                    plateformes-là qui savent voyager dans le temps. Elles
                    n'en feront plus qu'une après C8-P4. */}
                {isOwner && !isWatching && timeTravelPlatform && (
                    <>
                        {/* Le voyage dans le temps, des deux côtés, et
                            chacun DÉCLARÉ par la partie (message "started").
                            L'AVANCE RAPIDE est celle du moteur GameFlex — au
                            serveur comme au téléphone ; un invité de bureau
                            (RPCS3, xemu…) ne l'a pas. Le REMBOBINAGE demande
                            en plus au cœur de tenir un historique d'états
                            sans coûter le son. Sans l'un ni l'autre, pas de
                            section : un réglage sans effet ne se propose
                            pas. */}
                        {(fastForwardSupported || rewindOffered) && (
                            <SectionPanel
                                label={t("streaming.settings.timeTravel.title")}
                                helpText={t("streaming.settings.timeTravel.help")}
                            >
                                {rewindOffered && (
                                    <Form.SkeletonSwitch
                                        reverse
                                        variant="md"
                                        name="isRewinding"
                                        context={contextForm}
                                        label={t("streaming.settings.timeTravel.rewind")}
                                        helpText={t("streaming.settings.timeTravel.rewindHelp")}
                                        onText={t("streaming.settings.switch.rewinding")}
                                    />
                                )}
                                {fastForwardSupported && (
                                    <Form.SkeletonSwitch
                                        reverse
                                        variant="md"
                                        name="isFastForwarding"
                                        context={contextForm}
                                        label={t("streaming.settings.timeTravel.fastForward")}
                                        helpText={t("streaming.settings.timeTravel.fastForwardHelp")}
                                        onText={t("streaming.settings.switch.forwarding")}
                                    />
                                )}
                            </SectionPanel>
                        )}
                        <SectionPanel
                            label={t("streaming.settings.session.title")}
                            helpText={t("streaming.settings.session.help")}
                        >
                            {disks.count > 1 && (
                                <Form.Group
                                    label={t("streaming.settings.disk.title")}
                                    helpText={t("streaming.settings.disk.help")}
                                >
                                    <Form.Select
                                        name="disk"
                                        context={{
                                            values: { disk: String(disks.current) },
                                            setValues: async (value) => {
                                                const index = Number(value.disk);
                                                // Optimiste à l'affichage, puis recalé sur
                                                // ce que l'hôte dit avoir RÉELLEMENT monté :
                                                // un choix refusé revient tout seul au
                                                // disque précédent.
                                                setDisks((state) => ({ ...state, current: index }));
                                                const state = await askDisks.change(index);
                                                if (state?.error) {
                                                    context.toast.open(
                                                        t("streaming.settings.disk.failed"),
                                                        { variant: "error" },
                                                    );
                                                }
                                                if (state) {
                                                    setDisks({
                                                        count: state.count,
                                                        current: state.current,
                                                    });
                                                }
                                            },
                                            files: {}, setFiles: () => { }, rulesets: {},
                                        }}
                                        options={Array.from({ length: disks.count }, (unused, index) => ({
                                            value: String(index),
                                            label: t("streaming.settings.disk.number", { number: index + 1 }),
                                        }))}
                                    />
                                </Form.Group>
                            )}
                            {saveLoad && (
                                <ListElement
                                    icon="bi bi-database"
                                    label={t("streaming.settings.saveStates.title")}
                                    helpText={t("streaming.settings.saveStates.help")}
                                    onClick={() => {
                                        context.offcanvas.open(
                                            <OffCanvas
                                                icon="bi bi-database"
                                                label={t("streaming.settings.saveStates.title")}
                                                helpText={t("streaming.settings.saveStates.help")}
                                            >
                                                <SaveStatesPanel
                                                    session={session}
                                                    username={self}
                                                    saveState={saveLoad.saveState}
                                                    loadState={saveLoad.loadState}
                                                    context={context}
                                                    t={t}
                                                />
                                            </OffCanvas>, {
                                            size: 600,
                                        });
                                    }}
                                />
                            )}
                            {cheats && (
                                <ListElement
                                    icon="bi bi-braces"
                                    label={t("streaming.settings.cheats.title")}
                                    helpText={t("streaming.settings.cheats.help")}
                                    onClick={() => {
                                        context.offcanvas.open(
                                            <OffCanvas
                                                icon="bi bi-braces"
                                                label={t("streaming.settings.cheats.title")}
                                                helpText={t("streaming.settings.cheats.help")}
                                            >
                                                <CheatsPanel
                                                    setCheats={input.extraProps.cheats.setCheats}
                                                    initialCheats={input.extraProps.cheats.initialCheats}
                                                    context={context}
                                                    t={t}
                                                />
                                            </OffCanvas>, {
                                            size: 600,
                                        });
                                    }}
                                />
                            )}
                            {/* LE BOUTON RESET DE LA CONSOLE (chantier 13), confirmé
                                d'abord : ce qui n'est pas sauvegardé est perdu. */}
                            {resetConsole && (
                                <ListElement
                                    icon="bi bi-arrow-counterclockwise"
                                    label={t("streaming.settings.reset.title")}
                                    helpText={t("streaming.settings.reset.help")}
                                    onClick={() => {
                                        context.modal.open(t("streaming.settings.reset.confirmMessage"), {
                                            variant: "confirm",
                                            title: t("streaming.settings.reset.confirmTitle"),
                                            confirmLabel: t("streaming.settings.reset.confirm"),
                                            cancelLabel: t("app.cancel"),
                                            onConfirm: () => resetConsole(),
                                        });
                                    }}
                                />
                            )}
                        </SectionPanel>
                    </>
                )}
                {/* Le partage est possible dès que quelqu'un peut REJOINDRE,
                    et depuis C7-P4 c'est le cas sur un téléphone aussi. Ce
                    qu'il faut vérifier n'est donc plus « sommes-nous sur un
                    appareil » mais « cet hôte est-il joignable » : sans
                    adresse sur le réseau, le lien pointerait vers le
                    localhost de celui qui le reçoit. */}
                {canInvite() && shareUrl && (
                    <SectionPanel
                        label={t("streaming.settings.share.title")}
                        helpText={t("streaming.settings.share.help")}
                    >
                        {/* L'UN OU L'AUTRE, jamais les deux : la feuille de
                            partage propose déjà « copier », et deux entrées
                            pour le même geste ne font qu'obliger à choisir
                            entre deux choses identiques (2026-09-02). */}
                        {!canShareLink() && (
                            <ListElement
                                icon="bi bi-link-45deg"
                                label={t("streaming.settings.share.copy")}
                                helpText={t("streaming.settings.share.copyHelp")}
                                onClick={async () => {
                                    const url = shareUrl;

                                    if (await copyTextToClipboard(url)) {
                                        context.toast.open(t("streaming.settings.share.copied"), { variant: "success" });

                                        return;
                                    }
                                    // Never fail silently: on a plain-HTTP origin the
                                    // clipboard is unavailable by design, so show the
                                    // link and let the user copy it by hand.
                                    context.modal.open(
                                        <>
                                            <p>{t("streaming.settings.share.copyFailedMessage")}</p>
                                            <p className="user-select-all text-break small mb-0">{url}</p>
                                        </>,
                                        { title: t("streaming.settings.share.copyFailedTitle") },
                                    );
                                }}
                                next={null}
                            />
                        )}
                        {/* Only offered where the OS share sheet actually exists
                            (mobile, secure context) — a button that can never work
                            is worse than no button. */}
                        {canShareLink() && (
                            <ListElement
                                icon="bi bi-send"
                                label={t("streaming.settings.share.send")}
                                helpText={t("streaming.settings.share.sendHelp")}
                                onClick={() => {
                                    shareLink({
                                        url: shareUrl,
                                        title: session.title || t("streaming.settings.share.title"),
                                        text: t("streaming.settings.share.copyHelp"),
                                    });
                                }}
                                next={null}
                            />
                        )}
                        {/* INVITER QUELQU'UN — sur un serveur seulement.
                            Elle ouvre la partie à un COMPTE nommé : un
                            téléphone n'en a qu'un, celui de son porteur, et
                            l'écran n'y proposerait donc d'inviter personne.
                            Là-bas, partager le lien EST l'invitation.

                            Retirée partout par erreur le 2026-09-02 — la
                            remarque portait sur le téléphone, pas sur le
                            web. */}
                        {/* Inviter un COMPTE, ou partager un lien : deux
                            manières d'inviter, et l'hôte dit laquelle est la
                            sienne (`invites`). Là-bas, partager le lien EST
                            l'invitation. */}
                        {invitesAccounts && (
                            <ListElement
                                icon="bi bi-person-plus"
                                label={t("streaming.settings.invite.title")}
                                helpText={t("streaming.settings.invite.help")}
                                onClick={() => {
                                    context.offcanvas.open(
                                        <OffCanvas
                                            icon="bi bi-person-plus"
                                            label={t("streaming.settings.invite.title")}
                                            helpText={t("streaming.settings.invite.help")}
                                        >
                                            <InvitePanel
                                                session={session}
                                                token={token}
                                                context={context}
                                                t={t} />
                                        </OffCanvas>, {
                                        size: 600,
                                    });
                                }}
                            />
                        )}
                    </SectionPanel>
                )}
                {/* LA CATÉGORIE « VIDÉO » (chantier 31, le porteur : « il pourra
                    choisir entre flux actuel […] ou flux basse qualité ») —
                    seulement quand la partie a un second flux : un flux déjà
                    en 540p à 30 n'en a pas. */}
                {switches.videoAvailable && (
                    <SectionPanel
                        label={t("streaming.settings.video.title")}
                        helpText={t("streaming.settings.video.help")}
                    >
                        <Form.Group
                            label={t("streaming.settings.video.label")}
                            helpText={t(`streaming.settings.video.now.${switches.videoCurrent === "sd" ? "sd" : "hd"}`)}>
                            <Form.Select
                                name="videoQuality"
                                context={contextForm}
                                options={["auto", "current", "low"].map((value) => ({
                                    value, label: t(`streaming.settings.video.${value}`),
                                }))} />
                        </Form.Group>
                    </SectionPanel>
                )}
                {(
                    <SectionPanel
                        label={t("streaming.settings.performance.title")}
                        helpText={t("streaming.settings.performance.help")}
                    >
                        <Form.SkeletonSwitch
                            reverse
                            variant="md"
                            name="isResourceMonitoringVisible"
                            context={contextForm}
                            label={t("streaming.settings.performance.label")}
                            onText={t("streaming.settings.switch.visible")}
                            offText={t("streaming.settings.switch.hidden")}
                        />
                    </SectionPanel>
                )}
                {/* QUITTER, À LA FIN DU MENU (#408) — le bouton qui tenait le
                    coin gauche de la barre du bas. La fenêtre qu'il ouvre est
                    la même : elle dit ce que devient la partie. */}
                {onQuit && (
                    <SectionPanel
                        label={t("streaming.menu.quit.title")}
                        helpText={t(isWatching ? "streaming.menu.quit.helpWatching" : "streaming.menu.quit.help")}
                    >
                        <ListElement
                            light={true}
                            icon="bi bi-box-arrow-left"
                            label={t("streaming.menu.quit.label")}
                            onClick={() => {
                                context.offcanvas.close();
                                onQuit();
                            }}
                        />
                    </SectionPanel>
                )}
            </div>
        </OffCanvas>
    );
}

// The stats panel is generic across all 3 platforms and watch mode, 
// so it lives here instead of being duplicated inside each useXInput. 
// useElapsedSince — « En cours depuis 00:07:12 », qui AVANCE.
//
// Le serveur envoie l'INSTANT de démarrage, jamais une durée déjà calculée :
// une durée se périme en une seconde, et l'afficher telle quelle ferait
// sauter le compteur au rythme du sondage, deux secondes à la fois. Un
// instant, lui, ne se périme jamais — la page compte à sa propre cadence, et
// le chiffre reste juste même si le réseau bafouille.
function useElapsedSince(startedAt) {
    const started = React.useMemo(() => {
        const parsed = startedAt ? new Date(startedAt).getTime() : NaN;

        return Number.isFinite(parsed) ? parsed : null;
    }, [startedAt]);
    const [, forceTick] = React.useState(0);

    React.useEffect(() => {
        if (started == null) {
            return undefined;
        }
        const timer = setInterval(() => forceTick((n) => n + 1), 1000);

        return () => clearInterval(timer);
    }, [started]);

    if (started == null) {
        return null;
    }
    // Une horloge de client peut être en avance sur celle du serveur : un
    // compteur qui démarrerait à « -00:00:03 » serait plus troublant que faux.
    const seconds = Math.max(0, Math.floor((Date.now() - started) / 1000));
    const pad = (n) => String(n).padStart(2, "0");

    return `${pad(Math.floor(seconds / 3600))}:${pad(Math.floor(seconds / 60) % 60)}:${pad(seconds % 60)}`;
}

export function StreamingStatsPanel({ stats, session, t }) {
    // != null is not enough for these: NaN and undefined both pass it, and
    // the panel then printed "NaN" and "undefinedxundefined" (React even warns
    // about the NaN child). A stat is shown when it is a real finite number,
    // and every other case is the same em dash the rest of the rows use.
    const num = (value) => (typeof value === "number" && Number.isFinite(value) ? value : null);
    const rate = (kbps) => (num(kbps) != null ? `${Math.round(kbps)} kbps` : "—");
    // Mo en dessous du gigaoctet, Go au-dessus : « 4341 Mo » se relit deux
    // fois, « 4,2 Go » se lit une.
    const bytes = (value) => {
        const v = num(value);
        if (v == null) return null;

        return v >= 1024 ** 3 ? `${(v / 1024 ** 3).toFixed(1)} Go` : `${Math.round(v / 1024 ** 2)} Mo`;
    };
    // L'ÉCART DEPUIS LE DÉBUT DE LA PARTIE, entre parenthèses (le porteur,
    // 2026-10-28 : « commencé à 20°, 34° vingt minutes plus tard : (+14°) »).
    // Le départ est la première mesure de la partie, gardée par le serveur
    // (sessionstatus.Baselines) : la même pour tous, et un rechargement ne la
    // remet pas à zéro. Rien sans départ connu.
    //
    // `from` — le relevé dont on parle : celui de la partie, ou celui du
    // téléphone qu'on tient (`stats.device`), chacun avec son propre départ.
    const start = stats?.start || {};
    const delta = (field, format, from = stats) => {
        const now = num(from?.[field]);
        const then = num(from?.start?.[field]);
        if (now == null || then == null) return "";
        const difference = Math.round(now - then);
        const sign = difference > 0 ? "+" : difference < 0 ? "−" : "±";

        return ` (${sign}${format(Math.abs(difference))})`;
    };
    const bytesDelta = (field) => {
        const now = num(stats?.[field]);
        const then = num(start[field]);
        if (now == null || then == null || then === 0) return "";
        const difference = now - then;

        return ` (${difference < 0 ? "−" : "+"}${bytes(Math.abs(difference))})`;
    };
    const uptime = useElapsedSince(stats?.startedAt);
    // LA CHALEUR ET LA BATTERIE, DANS L'APPLICATION SEULEMENT (le porteur,
    // 2026-10-29 : sur un navigateur, « on ne voit pas les 2 autres lignes et
    // les icônes ») : celles du téléphone qu'on tient, puis celles de la
    // machine quand la partie tourne ailleurs — un serveur, ou le téléphone
    // d'un autre : « un téléphone peut être un serveur aussi ». Sur une
    // partie de ce téléphone, la machine EST lui : ses lignes seraient les
    // mêmes deux fois.
    const inApp = can("devicePower");
    const machinePower = inApp && Boolean(streamHostOrigin());
    // Players is internal server-side (json:"-") — the wire serves the
    // console's own maximum, and this tab's own pad via me.padSlot.
    //
    // `maxPlayers` et non la somme : `openSlots` réserve le siège 1 tant que
    // le propriétaire ne joue pas, donc l'addition perd ce siège. Même
    // compteur que Dashboard.jsx, même raison.
    const maxSlots = session?.maxPlayers
        || ((session?.playerCount ?? 0) + (session?.openSlots ?? 0));
    const padSlot = session?.me?.padSlot;

    return (
        // EN COLONNES SUR TOUTE LA LARGEUR (le porteur, 2026-10-28) : une
        // seule colonne de vingt lignes débordait la hauteur d'un téléphone
        // couché ; la largeur entière les range côte à côte.
        <div style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))",
            columnGap: "2rem",
            rowGap: "0.25rem",
        }}>
            {/* LE TÉLÉPHONE QU'ON TIENT, EN PREMIER, dans l'application
                seulement (le porteur, 2026-10-29 : « les joueurs préfèrent
                avoir l'info de leur téléphone aussi (et en premier) »). Une
                ligne le sépare de ce qui suit : la machine qui fait tourner la
                partie. Voir useSessionStats. */}
            {inApp && (
                <>
                    <div className="d-flex justify-content-between gap-3">
                        <span className="text-white text-opacity-75">{t("streaming.page.stats.phoneTemperature")}</span>
                        <span>
                            {num(stats?.device?.temperatureCelsius) != null
                                ? `${Math.round(stats.device.temperatureCelsius)} °C${delta("temperatureCelsius", (v) => `${v} °C`, stats.device)}`
                                : "—"}
                        </span>
                    </div>
                    <div className="d-flex justify-content-between gap-3">
                        <span className="text-white text-opacity-75">{t("streaming.page.stats.phoneBattery")}</span>
                        <span>
                            {num(stats?.device?.batteryPercent) != null
                                ? `${Math.round(stats.device.batteryPercent)} %${delta("batteryPercent", (v) => `${v} %`, stats.device)}`
                                : "—"}
                        </span>
                    </div>
                    <hr className="my-1" style={{ gridColumn: "1 / -1" }} />
                </>
            )}
            {/* L'émulateur et sa version, tels que la session les déclare
                (sessionstatus.Status.Emulator) : un cœur libretro par son nom,
                une console streamex par son image, un invité du téléphone par
                son paquet. Absent pour Remote Play, qui n'émule rien. */}
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.emulator")}</span>
                <span className="text-end">{stats?.emulator || "—"}</span>
            </div>
            {/* La machine qui fait tourner la partie — celle d'un serveur
                fédéré quand on joue chez lui : c'est lui qui répond
                (sessionstatus.Status.Host). */}
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.host")}</span>
                <span className="text-end">{describeResources(stats?.host, t) || "—"}</span>
            </div>
            {/* La chaleur et la batterie de cette machine — 100 % pour un
                serveur sur secteur (sessionstatus.ServerPower ; un téléphone
                donne les siennes). */}
            {machinePower && (
                <>
                    <div className="d-flex justify-content-between gap-3">
                        <span className="text-white text-opacity-75">{t("streaming.page.stats.temperature")}</span>
                        <span>
                            {num(stats?.temperatureCelsius) != null
                                ? `${Math.round(stats.temperatureCelsius)} °C${delta("temperatureCelsius", (v) => `${v} °C`)}`
                                : "—"}
                        </span>
                    </div>
                    <div className="d-flex justify-content-between gap-3">
                        <span className="text-white text-opacity-75">{t("streaming.page.stats.battery")}</span>
                        <span>
                            {num(stats?.batteryPercent) != null
                                ? `${Math.round(stats.batteryPercent)} %${delta("batteryPercent", (v) => `${v} %`)}`
                                : "—"}
                        </span>
                    </div>
                </>
            )}
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.uptime")}</span>
                <span style={{ fontVariantNumeric: "tabular-nums" }}>{uptime ?? "—"}</span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">
                    {t("streaming.page.stats.resolution")}
                </span>
                <span>
                    {num(stats?.resolution?.width) != null && num(stats?.resolution?.height) != null
                        ? `${stats.resolution.width}x${stats.resolution.height}`
                        : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.fps")}</span>
                <span>
                    {num(stats?.fps) != null ? Math.round(stats.fps) : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">
                    {t("streaming.page.stats.downloadRate")}
                </span>
                <span>
                    {rate(stats?.downloadKbps)}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">
                    {t("streaming.page.stats.uploadRate")}
                </span>
                <span>
                    {rate(stats?.uploadKbps)}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.bitrate")}</span>
                <span>
                    {rate(stats?.videoBitrateKbps)}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.latency")}</span>
                <span>
                    {num(stats?.latencyMs) != null ? `${Math.round(stats.latencyMs)} ms` : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.cpuEmulator")}</span>
                <span>
                    {num(stats?.cpuPercent) != null
                        ? `${Math.round(stats.cpuPercent)} %${delta("cpuPercent", (v) => `${v} %`)}`
                        : "—"}
                </span>
            </div>

            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.ram")}</span>
                <span>{bytes(stats?.rssBytes) != null ? `${bytes(stats.rssBytes)}${bytesDelta("rssBytes")}` : "—"}</span>
            </div>
            {/* Le conteneur ENTIER, tel que Docker le compte : l'émulateur
                plus l'encodeur, le serveur X, l'audio. Avec son plafond quand
                il y en a un — sans lui, « 900 Mo » ne dit pas s'il reste de la
                marge. Voir sessionstatus.Status côté serveur. */}
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.containerRam")}</span>
                <span>
                    {bytes(stats?.containerRssBytes) != null
                        ? `${bytes(stats.containerRssBytes)}${bytes(stats?.containerLimitBytes) ? ` / ${bytes(stats.containerLimitBytes)}` : ""}`
                        : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.gpuBusyPercent")}</span>
                <span>
                    {num(stats?.gpuBusyPercent) != null ? `${Math.round(stats.gpuBusyPercent)} %` : "—"}
                </span>
            </div>

            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.vramUsedBytes")}</span>
                <span>
                    {bytes(stats?.vramUsedBytes) != null
                        ? `${bytes(stats.vramUsedBytes)}${bytes(stats?.vramTotalBytes) ? ` / ${bytes(stats.vramTotalBytes)}` : ""}${bytesDelta("vramUsedBytes")}`
                        : "—"}
                </span>
            </div>
            {/* Ce que l'émulateur lit sur le disque, par seconde
                (sessionstatus.Status.DiskReadBytesPerSec) : la vitesse à
                laquelle le jeu se charge. Une décimale sous 10 Mo/s, où
                l'arrondi effacerait une lecture lente. */}
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.diskRead")}</span>
                <span>
                    {num(stats?.diskReadBytesPerSec) != null
                        ? `${(stats.diskReadBytesPerSec / 1024 ** 2).toFixed(stats.diskReadBytesPerSec < 10 * 1024 ** 2 ? 1 : 0)} Mo/s`
                        : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.emulationFps")}</span>
                <span>
                    {num(stats?.emulationFps) != null ? `${Math.round(stats.emulationFps)}` : "—"}
                </span>
            </div>
            <div className="d-flex justify-content-between gap-3">
                <span className="text-white text-opacity-75">{t("streaming.page.stats.slot")}</span>
                <span>
                    {padSlot != null && maxSlots > 0 ? `${padSlot}/${maxSlots}` : "—"}
                </span>
            </div>

        </div>
    );
}

