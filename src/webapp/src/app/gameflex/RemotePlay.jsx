import React, { useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { buildStreamingUrl } from "./libs/sessionLinks.js";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../skeleton/skeleton.js";
import { useConsoles } from "./contexts/ConsolesContext.jsx";
import {
    deleteDevice,
    listDevices,
    listFederatedDevices,
    registerDevice,
    wakeDevice,
} from "./api/remoteplay.js";
import { can } from "./libs/capabilities.js";
import { federatedId, parseFederatedId } from "./libs/federatedHost.js";
import { forgetFederationToken, useFederationSession } from "./libs/federationSession.js";
import { allows } from "./libs/consoleGrants.js";
import { FederatedAccount, FederatedRefusal, FederatedTokenForm } from "./shared/FederatedAccountPanel.jsx";
import { handleLaunchConflict } from "./shared/LaunchConflict.jsx";
import { playStationService } from "./stream/remoteplay/service/playstation.service.js";
import { StreamSettings } from "./stream/shared/StreamSettings.jsx";
import { streamingService } from "@/gameflex/stream/remoteplay/service/streaming.service";
import { useAuth } from "./profile/auth/AuthContext.jsx";
import { isOwnSession, useSession } from "./libs/SessionPool.js";
import {
    loadStreamingSettings,
    saveStreamingSettings,
} from "./libs/streamingSettingsStorage.js";
import { useDeviceStatus } from "./stream/remoteplay/hooks/use-device-status.js";
import { useFederatedDeviceStatus } from "./stream/remoteplay/hooks/use-federated-device-status.js";
// LE MÊME SÉLECTEUR QUE LA LUDOTHÈQUE ET QUE L'ADMINISTRATION, pas un troisième.
// « D'où je liste » se pose ici exactement comme ailleurs ; en écrire un autre
// finirait par le nommer autrement ou oublier son aide.
import SourceSelector from "./profile/settings/admin/storage/SourceSelector.jsx";
import {
    Page,
    SectionPanel,
    ListElement,
    AsyncSectionPanel,
    OffCanvas,
    ListingElement,
} from "./shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Only PlayStation is backed by a real implementation (golang/src/internal/
// remote/playstation) — other remoteplay-method consoles (internal/consoles
// registry) are listed for the sidebar but have no server-side support yet.
// LES IDENTIFIANTS DU JEU À DISTANCE PORTENT `rp-` (2026-09-10).
//
// Le préfixe distingue la console jouée à distance de son émulation, qui garde
// l'identifiant simple — c'est la variante à distance qui est provisoire, voir
// `shared/consoles/registry.go` pour le raisonnement.
const PLATFORM_BY_CONSOLE = {
    "rp-ps4": "playstation",
    "rp-ps5": "playstation",
    "rp-xboxseries": "xbox",
    "rp-steam": "steam",
    "rp-switch2": "nintendo",
};

// D'OÙ VIENT CETTE CONSOLE — d'ici, ou de la machine de quelqu'un d'autre.
//
// Deux PS5 portent souvent le même nom et toujours le même type : ce qui
// les distingue est le serveur qui les tient. Sans cette mention, choisir
// entre elles revient à choisir au hasard (demandé le 2026-09-07).
//
// `source` est le nom du serveur fédéré, tel qu'il a été ajouté ici. Son
// absence veut dire « cette machine », et c'est le seul cas qui se traduit :
// le nom d'un serveur, lui, ne se traduit pas.
function deviceSourceLabel(device, t) {
    return device.source || t("remoteplay.device.sourceLocal");
}

function DeviceCard({ device, statusMap, t, onClick }) {
    // hasActiveSession (from DeviceOut, golang/src/internal/api/handlers.go)
    // overrides the plain online/standby/offline badge — someone else is
    // already streaming this console, distinct from just being reachable.
    // Same amber accent + wording as the source app's DeviceCard.jsx.
    //
    // CORROBORATED against the kernel's session pool — see the detail panel's
    // `busy`. A badge that says "in use" about a free console is not a
    // cosmetic problem: it is the reason the owner stops trying.
    const liveSession = useSession(device.id);
    const st = device.hasActiveSession && liveSession
        ? { bg: "warning", label: t("remoteplay.device.sessionActive") }
        : statusMap[device.status] || { bg: "secondary", label: device.status };
    return (
        <ListElement
            onClick={onClick}
            icon="bi bi-playstation"
            label={device.hostName || device.hostId}
            helpText={`${device.hostType} · ${device.ipAddress} (${deviceSourceLabel(device, t)})`}
            next={
                <div className="d-flex justify-content-end align-items-end gap-2">
                    <bs.Badge
                        bg={st.bg}
                        className="rounded-pill px-3 py-2"
                        style={{ fontSize: "0.75rem" }}
                    >
                        {st.label}
                    </bs.Badge>
                    <i className="bi bi-chevron-right text-muted fs-5 flex-shrink-0" />
                </div>
            }
        />
    );
}

// How long the Turn-on button stays out of reach after a wake packet. Wake-on-
// LAN gets no acknowledgement, so this is not a protocol timeout — it is how
// long we are willing to say "wait" before admitting the console is not coming.
const WAKE_TIMEOUT_MS = 30_000;

// ── Offcanvas: détail d'un appareil + actions ──────────────────────────────
function DeviceDetailPanel({
    device: initialDevice,
    statusMap,
    t,
    context,
    onChanged,
}) {
    const navigate = useNavigate();
    const { user, isAdmin, token } = useAuth();
    const [device, setDevice] = useState(initialDevice);
    const [pendingAction, setPendingAction] = useState(null);
    // A DEADLINE, not a boolean: "waking until" survives re-renders, makes the
    // 30s cap explicit, and cannot be left stuck on by a missed reset. A
    // console that never answers must not disable its own button for good.
    const [wakingUntil, setWakingUntil] = useState(0);
    const isAwake = device.status === "online" || device.status === "awake";
    const isWaking = wakingUntil > 0 && !isAwake;
    const [isLoading, setIsLoading] = useState(true);
    // CETTE CONSOLE EST-ELLE CHEZ QUELQU'UN D'AUTRE ?
    //
    // `serverId` ne vient QUE de la fédération (voir handleRemoteDevices) :
    // une console d'ici n'en a pas, et n'en aura jamais. C'est donc lui qui
    // décide de tout ce qui suit — pas un test de plateforme, pas une
    // devinette sur l'adresse.
    const federatedServer = device.serverId
        ? { id: device.serverId, link: device.link, name: device.source }
        : null;
    // La session CHEZ LUI, au nom de NOTRE compte de chez lui : notre kernel
    // l'échange contre le jeton qu'on y a reçu et collé ici une fois (chantier
    // 20). Tant qu'elle n'est pas là — pas de jeton, ou il le refuse —, on ne
    // touche pas à sa console : le panneau montre où coller le jeton.
    const {
        session: account,
        code: refusal,
        isLoading: isOpening,
    } = useFederationSession(federatedServer?.id);
    // CE COMPTE-LÀ LANCE-T-IL CETTE CONSOLE CHEZ LUI : ses consoles (`rp-ps5`…).
    // Le lien nous montre toutes ses consoles à distance ; jouer sur l'une
    // d'elles reste une affaire de compte.
    const grantedThere = Boolean(account)
        && allows(account.consoles, { id: `rp-${String(device.hostType || "").toLowerCase()}` });
    const needsSignIn = Boolean(federatedServer) && (!account || !grantedThere);
    // CE QU'IL FAUT POUR TOUCHER CETTE CONSOLE : chez qui, et avec quoi.
    //
    // Null pour une console d'ici — les appels partent alors comme avant, sur
    // notre propre kernel. Null AUSSI tant qu'on n'a pas de session là-bas :
    // un appel sans elle se ferait refuser, et le panneau dit déjà pourquoi.
    const remoteHost = federatedServer && account
        ? { origin: federatedServer.link, token: account.token }
        : null;
    // L'identifiant tel que L'HÔTE le connaît. Le préfixe `srv<id>:` est à
    // nous seuls — chez lui, cette console porte son nom d'origine, et lui
    // envoyer notre écriture ne rendrait jamais rien.
    const remoteDeviceId = federatedServer
        ? (parseFederatedId(device.id)?.id || device.id)
        : device.id;
    const [values, setValues] = useState({
        resolution: "720p",
        frameRate: "60",
        bitrate: "auto",
    });
    const formContext = {
        values,
        setValues,
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    // The panel is opened with a snapshot of `device` (a plain prop, not a
    // live subscription — it's rendered through an offcanvas portal, not as
    // a normal re-rendering child of DeviceList) — without this, waking the
    // console never updates the Power/Connect button here, only a fresh
    // fetch would. Same "device-status" SignalR hub DeviceList itself uses
    // below, filtered down to just this device by hostId.
    const onDevicesUpdate = useCallback(
        (devices) => {
            const match = devices.find((d) => d.hostId === initialDevice.hostId);
            if (match) setDevice((prev) => ({ ...prev, ...match }));
        },
        [initialDevice.hostId],
    );
    // LE HUB NE CONNAÎT QUE LES CONSOLES D'ICI, et il apparie par `hostId`.
    //
    // Laissé branché sur une console fédérée, il lui collerait l'état d'une
    // console LOCALE qui porte le même hostId — cas très ordinaire, puisque
    // deux salons ont chacun leur 192.168.1.x. Le panneau montrerait alors la
    // veille d'une autre machine.
    useDeviceStatus({
        onDevicesUpdate,
        onStatusUpdate: onDevicesUpdate,
        enabled: !federatedServer && can("spawnsSessions"),
    });
    // … et une console fédérée se relit chez son serveur, par la liste
    // fédérée : sans quoi « Allumer » la laissait sur « Veille » ici.
    const onFederatedDevices = useCallback(
        (devices) => {
            const match = devices.find((d) => d.id === initialDevice.id);
            if (match) setDevice((prev) => ({ ...prev, ...match }));
        },
        [initialDevice.id],
    );
    useFederatedDeviceStatus({
        enabled: Boolean(federatedServer),
        onDevices: onFederatedDevices,
    });

    useEffect(() => {
        let cancelled = false;

        // LES RÉGLAGES APPARTIENNENT À CELUI QUI TIENT LA CONSOLE.
        //
        // Tant qu'on n'a pas de session chez lui, il n'y a rien à demander :
        // les demander à NOTRE serveur ne rendrait qu'une erreur, et une toast
        // rouge à chaque ouverture de panneau. Le panneau dit déjà pourquoi.
        if (federatedServer && !remoteHost) {
            setIsLoading(false);

            return undefined;
        }

        (async () => {
            // `apiRequest` LANCE, il ne rend pas un résultat — contrairement au
            // helper `api()` de `api/remoteplay.js`, qui attrape tout et rend
            // `{success:false}`. Deux conventions dans le même écran, et celui
            // d'ici avait été lu comme l'autre : un appel qui échoue sautait
            // `setIsLoading(false)`, et le panneau restait sur son spinner POUR
            // TOUJOURS.
            //
            // Ça ne se voyait que sur une console FÉDÉRÉE : c'est le seul cas
            // où l'appel quitte notre origine, et donc le seul qui puisse
            // échouer autrement que par un statut HTTP (serveur éteint, CORS,
            // réponse qui n'est pas du JSON). Une console d'ici répondait
            // toujours (signalé le 2026-09-10).
            try {
                const result = await playStationService.getDeviceSettings(
                    remoteDeviceId, remoteHost,
                );
                if (cancelled) return;
                if (result.success && result.data) {
                    // Local (per-user, per-hostType) preference wins over the
                    // server's device-wide row — see streamingSettingsStorage.js.
                    // Falls back to whatever's saved server-side, then hardcoded
                    // defaults, so the form is never empty on first visit.
                    const local = loadStreamingSettings(device.hostType);
                    setValues({
                        resolution:
                            local?.resolution || result.data.settings.resolution || "720p",
                        frameRate: local?.frameRate || result.data.settings.frameRate || "60",
                        bitrate: local?.bitrate || result.data.settings.bitrate || "auto",
                    });
                } else {
                    context.toast.open(
                        result.errorMessage || t("remoteplay.settings.loadFailed"),
                        { variant: "error" },
                    );
                }
            } catch (error) {
                if (!cancelled) {
                    // UN REFUS DE L'AMI REDEMANDE LA SESSION, il n'affiche pas
                    // une erreur : elle vaut un temps, et il la juge lui-même.
                    // Oubliée, notre kernel la ré-échange — ou dit, par le
                    // panneau, que l'ami refuse désormais notre jeton.
                    if (federatedServer && isUnauthorized(error)) {
                        forgetFederationToken(account?.token);
                    } else {
                        context.toast.open(
                            error?.message || t("remoteplay.settings.loadFailed"),
                            { variant: "error" },
                        );
                    }
                }
            } finally {
                // LE PANNEAU S'OUVRE, QUOI QU'IL ARRIVE. Ses réglages ont leurs
                // valeurs par défaut, et le reste — réveiller, se connecter,
                // oublier la console — reste à portée de main. Un serveur muet
                // ne doit pas emprisonner l'écran.
                if (!cancelled) setIsLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [device.id, Boolean(remoteHost)]);

    // Two ways out, and both must exist: the console turns up (isAwake), or
    // the clock runs out. Bound to wakingUntil alone so a status poll landing
    // every second cannot keep rebuilding the timer and pushing the deadline
    // away — a console that never answers must not disable its button for good.
    useEffect(() => {
        if (wakingUntil === 0) {
            return undefined;
        }

        const timer = setTimeout(() => setWakingUntil(0), Math.max(0, wakingUntil - Date.now()));
        return () => clearTimeout(timer);
    }, [wakingUntil]);

    useEffect(() => {
        if (isAwake) {
            setWakingUntil(0);
        }
    }, [isAwake]);

    const handleWake = async () => {
        setPendingAction("wake");

        try {
            const result = await wakeDevice(remoteDeviceId, remoteHost);

            // Wake-on-LAN is a shout into the dark: the packet leaves, and the
            // console answers by turning up as "online" in the status poll — or
            // never. So the button stays out of reach until one of those two
            // happens, and 30s is the point where we stop waiting rather than
            // leave someone staring at a control they cannot press.
            if (result.ok) {
                setWakingUntil(Date.now() + WAKE_TIMEOUT_MS);
            }

            context.toast.open(
                result.ok
                    ? t("remoteplay.device.wakeSent")
                    : result.errorMessage || t("remoteplay.device.wakeFailed"),
                { variant: result.ok ? "success" : "error" },
            );
        } catch (failure) {
            console.error("échec du réveil de la console", failure);
            context.toast.open(
                t("remoteplay.device.wakeFailed"),
                { variant: "error" },
            );
        } finally {
            setPendingAction(null);
        }
    };
    const handleConnect = async () => {
        // The backend has no per-session settings override — ConnectWebRTC's
        // launch_spec always reads the device's saved settings row (see
        // resolveDeviceStreamSettings in playstation_handlers.go), so saving
        // here (right before launch) is the only way the form's current
        // values actually reach the stream. Saved to localStorage in
        // parallel so *this* browser's preference survives independently of
        // whatever the device row ends up holding after someone else connects.
        setPendingAction("connect");
        saveStreamingSettings(device.hostType, values);
        // Même piège qu'à l'ouverture du panneau : un `apiRequest` qui lance
        // sautait tout le reste, et le bouton gardait son « … » sans jamais
        // revenir. Un enregistrement raté n'est pas fatal — on le dit, et on
        // tente quand même la session, qui a sa propre issue juste en dessous.
        try {
            const result = await playStationService.updateDeviceSettings(
                remoteDeviceId,
                values,
                remoteHost,
            );
            if (!result.success) {
                context.toast.open(
                    result.errorMessage || t("remoteplay.settings.saveFailed"),
                    { variant: "error" },
                );
            }
        } catch (error) {
            context.toast.open(
                error?.message || t("remoteplay.settings.saveFailed"),
                { variant: "error" },
            );
        }
        // Crée la session côté serveur D'ABORD (voir la doc de la page de
        // streaming unifiée, ../Streaming.jsx) — la porte unique est
        // idempotente : `launchRemoteplay` reconnaît qu'une session tourne
        // déjà pour cet appareil et la rend telle quelle (`existing: true`)
        // au lieu de refaire la poignée de main avec la PS5. Un Connect
        // rafraîchi ou double-cliqué est donc sans danger.
        let sessionResponse;
        try {
            sessionResponse = await streamingService.startSession(
                device.hostId,
                // LE NOM SOUS LEQUEL ON EST CONNU LÀ-BAS, pas celui d'ici.
                // C'est lui qui prendra la place dans la file de joueurs de
                // l'hôte : y envoyer notre nom local ferait entrer dans sa
                // file quelqu'un qui n'existe pas chez lui.
                remoteHost ? account.username : user?.username,
                remoteHost,
            );
        } catch (error) {
            setPendingAction(null);
            // UNE AUTRE PARTIE EST LÀ : le même choix que sur le catalogue
            // (shared/LaunchConflict.jsx) — y retourner, la terminer, ou
            // arrêter celle où l'on joue déjà.
            if (handleLaunchConflict({
                result: error?.result, context, t, token, host: remoteHost, navigate,
                retryLaunch: () => handleConnect(),
            })) {
                return;
            }
            // The PS5 itself refuses a handshake for a few seconds after the
            // previous session closed — session.go rejects at two different
            // points depending on timing ("init rejected: HTTP 403" or
            // "ctrl rejected: HTTP 403", session.go:126/191) — the backend
            // wraps either as a technical 500, but for the user it's just a
            // cooldown, so show that instead of the generic failure message.
            // Confirmed live (2026-08-22): a "ctrl rejected" case was falling
            // through to the generic message because this only matched
            // "init rejected".
            const rawMessage = error instanceof Error ? error.message : "";
            context.toast.open(
                rawMessage.includes("init rejected") || rawMessage.includes("ctrl rejected")
                    ? t("remoteplay.connect.cooldown")
                    : t("streaming.connection.errors.sessionCreateFailed"),
                { variant: "error" },
            );
            return;
        }
        setPendingAction(null);
        if (!sessionResponse.success) {
            context.toast.open(
                sessionResponse.errorMessage ||
                t("streaming.connection.errors.sessionCreateFailed"),
                { variant: "error" },
            );
            return;
        }
        const sessionData = sessionResponse.data || sessionResponse.result || {};
        const sessionId =
            sessionData.id ||
            sessionData.Id ||
            sessionData.sessionId ||
            sessionData.session_id;
        // A plain navigation, not an offcanvas — see Streaming.jsx for why
        // (a fullscreen offcanvas kept growing skeleton.js's generic
        // offcanvas API with streaming-only concerns for a single caller).
        // offcanvas.close() first — unlike Wake (you stay on this panel to
        // see the result), Connect leaves it entirely for a fullscreen page;
        // without closing it explicitly it just stays open underneath,
        // reappearing once the streaming page's own Exit navigates back here.
        context.offcanvas.close();
        // Connect IS the intent to play (see buildStreamingUrl). Without it
        // the page lands in watch on a session nobody holds — for remoteplay
        // that means no controller at all, since the child's queue is now the
        // only thing that opens the input path.
        // L'IDENTIFIANT REPART PRÉFIXÉ quand la session vit ailleurs.
        //
        // L'hôte l'a frappé dans SON espace, et deux serveurs y frappent les
        // mêmes ; sans le préfixe, la page de streaming irait chercher cette
        // session dans notre propre façade — au mieux elle ne trouverait
        // rien, au pire elle trouverait l'homonyme d'ici. Même raison, même
        // écriture que pour un jeu fédéré depuis le chantier 6.
        navigate(buildStreamingUrl({
            sessionId: federatedServer
                ? federatedId(federatedServer.id, sessionId)
                : sessionId,
            play: true,
        }));
    };
    // Someone else already has an active session on this device
    // (hasActiveSession) — join as a spectator instead of starting a new
    // session (la porte rendrait simplement la session existante). Le
    // serveur indexe les sessions par identifiant d'APPAREIL — c'est le
    // contrat que `launchRemoteplay` respecte, et c'est pourquoi
    // `device.id` doubles as the session id to attach to here — no separate
    // lookup needed.
    // Is the live session on this device MINE? A device with an active
    // session offers WATCH — correct for somebody else's game, wrong for my
    // own, which is how "I connected, saw nothing, left, and could not get
    // back in" happened: the only way back was a watch link, and a watch
    // link is inert by design. The Dashboard has had this rule since
    // chantier-3 ("take my session back"); this page never got it.
    const liveSession = useSession(device.id);
    const isMySession = isOwnSession(liveSession, user?.username);
    // hasActiveSession must be CORROBORATED. It comes from a map in the
    // remoteplay service that nothing reliably cleans (its own comment says
    // so): the entry outlives the stream, stays hidden while the console is
    // in standby, and reappears the moment it wakes — so a console nobody is
    // using answers "someone is connected" and never offers Play again
    // (reported live 2026-08-25: "j'ai rallumé la PS5 et le bouton n'est
    // jamais passé à play").
    //
    // The kernel's session pool is the registry of REAL sessions. A device
    // claiming one the kernel has never heard of is a leak, not a session.
    // Deliberately not the reverse check (trusting the device over the pool):
    // being wrong this way offers Play on a console somebody else is using,
    // and starting a session is idempotent per device, while being wrong the
    // other way locks the owner out of their own console for good.
    const busy = !!device.hasActiveSession && !!liveSession;

    const handleResume = () => {
        context.offcanvas.close();
        navigate(buildStreamingUrl({ sessionId: device.id, play: true }));
    };

    const handleWatch = () => {
        // Same reason as handleConnect above — this also leaves the panel
        // for a fullscreen page. No more separate /watch route — a single
        // /streaming decides owner/player/spectate itself from
        // session.me.role (see Streaming.jsx's own doc); deviceName/
        // consoleId still come from the session itself (libs/SessionPool.js).
        context.offcanvas.close();
        // Watch carries no intent — deliberately. This is the path for a
        // device someone else is already using.
        navigate(buildStreamingUrl({ sessionId: device.id }));
    };
    const handleDelete = () => {
        context.modal.open(
            t("remoteplay.device.deleteConfirmMessage", {
                name: device.hostName || device.hostId,
            }),
            {
                variant: "confirm",
                title: t("remoteplay.device.deleteConfirmTitle"),
                confirmLabel: t("remoteplay.device.deleteConfirmLabel"),
                cancelLabel: t("remoteplay.device.deleteCancelLabel"),
                onConfirm: async () => {
                    setPendingAction("delete");

                    try {
                        const result = await deleteDevice(device.id);

                        if (result.ok) {
                            context.offcanvas.close();
                            onChanged({ removed: device.id });
                        } else {
                            context.toast.open(
                                result.errorMessage || t("remoteplay.device.deleteFailed"),
                                { variant: "error" },
                            );
                        }
                    } catch (failure) {
                        console.error("échec de la suppression de l'appareil", failure);
                        context.toast.open(
                            t("remoteplay.device.deleteFailed"),
                            { variant: "error" },
                        );
                    } finally {
                        setPendingAction(null);
                    }
                },
            },
        );
    };
    const st = device.hasActiveSession
        ? { bg: "warning", label: t("remoteplay.device.sessionActive") }
        : statusMap[device.status] || { bg: "secondary", label: device.status };

    if (isLoading) {
        return <AsyncSectionPanel isLoading />;
    }

    return (
        <OffCanvas
            icon="bi bi-playstation"
            label={device.hostName || device.hostId}
            helpText={`${device.hostType} · ${device.ipAddress} · (${deviceSourceLabel(device, t)})`}
            callback={(() => {
                // TANT QU'ON N'A PAS DE SESSION LÀ-BAS, il n'y a rien à
                // faire : une tentative se ferait refuser par le serveur d'en
                // face, ce qui est vrai mais illisible. Avec elle, les appels
                // partent chez lui (C9-P4b) et le bouton reprend son sens.
                if (needsSignIn) {
                    return null;
                }
                if (busy) {
                    return isMySession ? handleResume : handleWatch;
                } else if (device.status === "standby" || device.status === "offline") {
                    return handleWake;
                } else if (isAwake) {
                    return handleConnect;
                }

                return null;
            })()}
            buttonLabel={(() => {
                if (needsSignIn) {
                    return isOpening ? "•••" : t("federation.notPlayable");
                }
                if (busy) {
                    return isMySession ? t("dashboard.mySessions.resume") : t("remoteplay.device.watch");
                } else if (isWaking) {
                    return t("remoteplay.device.turningOn");
                } else if (device.status === "standby" || device.status === "offline") {
                    return t("remoteplay.device.turnOn");
                } else if (isAwake) {
                    return t("remoteplay.device.play");
                }

                return t("remoteplay.device.busy");
            })()}
            isDisabled={needsSignIn || pendingAction !== null || isWaking}
            isPending={
                pendingAction === "connect" ||
                pendingAction === "wake" ||
                pendingAction === "delete"
            }
            helpButton={isAdmin ? (
                <bs.Button variant="danger" onClick={handleDelete}>
                    <i className="bi bi-trash"></i>
                </bs.Button>
            ) : null}
        >
            {/* QUI JOUE CHEZ LUI : sans jeton (ou refusé), le formulaire pour
                le coller ; avec, sous quel nom — et, si ce compte-là ne lance
                pas cette console, pourquoi. */}
            {federatedServer && !isOpening && (account
                ? <>
                    <FederatedAccount server={federatedServer} account={account} t={t} />
                    {!grantedThere && <FederatedRefusal server={federatedServer} code="remoteLaunchRefused" t={t} />}
                </>
                : refusal === "noFederatedAccount" || refusal === "accountRefused"
                    ? <FederatedTokenForm
                        server={federatedServer}
                        refused={refusal === "accountRefused"}
                        t={t}
                        context={context} />
                    : <FederatedRefusal server={federatedServer} code={refusal} t={t} />)}

            <SectionPanel
                label={t("remoteplay.device.details")}
                helpText={t("remoteplay.device.detailsHelp")}>
                <div className="d-flex flex-column gap-2">
                    <DetailRow
                        label={t("remoteplay.device.type")}
                        value={device.hostType}
                    />
                    <DetailRow
                        label={t("remoteplay.device.ip")}
                        value={device.ipAddress}
                    />
                    <DetailRow
                        label={t("remoteplay.device.status")}
                        value={
                            <bs.Badge
                                bg={st.bg}
                                className="rounded-pill px-3 py-1"
                                style={{ fontSize: "0.75rem" }}
                            >
                                {st.label}
                            </bs.Badge>
                        }
                    />
                </div>
                {isAdmin && (
                    <div className="mt-3 border-top pt-3">
                        <bs.Button
                            variant="danger"
                            onClick={handleDelete}
                            className="d-flex justify-content-center w-100"
                        >
                            {t("remoteplay.device.delete")}
                        </bs.Button>
                    </div>
                )}
            </SectionPanel>

            {/* LES RÉGLAGES DE FLUX SONT CEUX DE CELUI QUI TIENT LA CONSOLE.
                Tant qu'on n'est pas connu chez lui on ne les a pas, et trois
                listes vides valent moins que rien.

                LES OPTIONS, ELLES, SONT LES NÔTRES — et communes aux trois
                plateformes, 480p en moins depuis le 2026-09-20. C'est la
                console qui a tranché : alignée sur streamex le 2026-09-10, la
                liste offrait un « 480p » (854×480) que la PS5 ne sait pas
                ouvrir, et la partie mourait 0,7 s après une poignée de main
                pourtant réussie — « AvCap failed to initialize video ». Le
                mode a été retiré partout plutôt que d'entretenir deux
                échelles. */}
            {!needsSignIn && (
                <StreamSettings t={t} context={formContext} toast={context.toast} />
            )}
        </OffCanvas>
    );
}

// UNE LIGNE SANS VALEUR NE S'AFFICHE PAS.
//
// « PSN ID » et « Status » restaient à l'écran, vides, sur une console
// appairée depuis le téléphone : le premier n'est délibérément pas publié par
// son kernel, le second manquait (2026-09-10). Un libellé suivi de rien se lit
// comme un défaut, alors que c'est parfois simplement une chose qu'on ne sait
// pas — la même règle « pas de section vide » que le reste du menu.
function DetailRow({ label, value }) {
    if (value === null || value === undefined || value === "") {
        return null;
    }

    return (
        <div className="d-flex justify-content-between align-items-center">
            <span className="text-muted small">{label}</span>
            <span className="text-body fw-semibold small">{value}</span>
        </div>
    );
}

// ── Offcanvas: enregistrer un nouvel appareil ──────────────────────────────
// Platform/model are fixed by the console the user picked in the sidebar —
// no need to ask again here (unlike a generic cross-platform form).
const IDENTIFIER_LABEL_FALLBACK = {
    playstation: "PSN ID",
    xbox: "Gamertag",
    steam: "Steam",
};

// Le site qui donne l'identifiant de compte PSN (Base64) qu'attend
// l'appairage d'une console.
const PSN_ACCOUNT_ID_URL = "https://psn.flipscreen.games/";

function RegisterPanel({
    platform,
    hostType,
    consoleName,
    t,
    context,
    onRegistered,
}) {
    // Hooks must run unconditionally — the "unsupported platform" early
    // return happens further down, after they're declared.
    const [values, setValues] = useState({
        name: "",
        ip: "",
        identifier: "",
        pin: "",
    });
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isDiscovering, setIsDiscovering] = useState(false);
    const [discovered, setDiscovered] = useState([]);

    const title = t("remoteplay.register.title", { console: consoleName });


    // Temporary: only PlayStation is supported for now, so the other consoles just show a "not supported yet" message.
    // The form is still rendered for them, but the submit button is disabled.
    if (platform !== "playstation") {
        return (
            <div className="d-flex flex-column h-100">
                <div className="d-flex align-items-center justify-content-between gap-3 mb-3 border-bottom pb-3">
                    <h5 className="fw-bold mb-0">{title}</h5>
                </div>
                <p className="text-muted">
                    {t("remoteplay.notSupportedYet", { platform: consoleName })}
                </p>

                <div className="d-flex gap-2 mt-auto border-top pt-3">
                    <bs.Button
                        variant="secondary"
                        className="w-25 py-2"
                        onClick={() => context.offcanvas.close()}
                    >
                        <i className="bi bi-arrow-left"></i>
                    </bs.Button>
                </div>
            </div>
        );
    }

    const rulesets = {
        name: (data) =>
            !data.name || data.name.length < 2
                ? Ruleset.isInvalid(t("form.required"))
                : null,
        ip: (data) => (!data.ip ? Ruleset.isInvalid(t("form.required")) : null),
        identifier: (data) =>
            !data.identifier ? Ruleset.isInvalid(t("form.required")) : null,
        pin: (data) => {
            if (data.pin === null || data.pin === undefined || data.pin === "")
                return Ruleset.isInvalid(t("form.required"));
            if (!/^\d{1,8}$/.test(String(data.pin)))
                return Ruleset.isInvalid(t("remoteplay.register.pinInvalid"));
            return null;
        },
    };
    const formContext = {
        values,
        setValues,
        files: {},
        setFiles: () => { },
        rulesets,
    };
    const identifierLabel = t(
        `remoteplay.register.identifierLabel.${platform}`,
        IDENTIFIER_LABEL_FALLBACK[platform] || "ID",
    );

    const handleDiscover = async () => {
        setIsDiscovering(true);
        setDiscovered([]);
        let result;
        try {
            result = await playStationService.discoverDevices();
        } catch (error) {
            // Troisième fois le même piège. Ici l'appel reste chez nous, mais
            // la diffusion est refusée à une application Android
            // (`netlinkrib: permission denied`) — un cas très réel.
            setIsDiscovering(false);
            context.toast.open(
                error?.message || t("remoteplay.register.discoverFailed"),
                { variant: "error" },
            );

            return;
        }
        setIsDiscovering(false);
        if (result.success) {
            const found = result.data || [];
            setDiscovered(found);
            if (found.length === 0) {
                context.toast.open(t("remoteplay.register.discoverEmpty"), {
                    variant: "normal",
                });
            }
        } else {
            context.toast.open(
                result.errorMessage || t("remoteplay.register.discoverFailed"),
                { variant: "error" },
            );
        }
    };

    const applyDiscovered = (device) => {
        setValues({
            ...values,
            ip: device.ip,
            name: values.name || device.name || "",
        });
        setDiscovered([]);
    };

    const handleSubmit = async () => {
        setIsSubmitting(true);

        try {
            const result = await registerDevice({ ...values, hostType });

            if (result.ok) {
                context.toast.open(t("remoteplay.register.success"), {
                    variant: "success",
                });
                context.offcanvas.close();
                onRegistered();
            } else {
                context.toast.open(
                    result.errorMessage || t("remoteplay.register.failure"),
                    { variant: "error" },
                );
            }
        } catch (failure) {
            console.error("échec de l'enregistrement de l'appareil", failure);
            context.toast.open(
                t("remoteplay.register.failure"),
                { variant: "error" },
            );
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <OffCanvas
            icon="bi bi-playstation"
            label={title}
            helpText={t("remoteplay.register.helpText")}
            isDisabled={isSubmitting || isDiscovering}
            buttonLabel={isSubmitting ? <bs.Spinner animation="border" size="sm" /> : t("remoteplay.register.submit")}
            callback={handleSubmit}
        >
            <SectionPanel
                label={t("remoteplay.register.formTitle")}
                helpText={t("remoteplay.register.formHelp")}
            >
                <Form context={formContext} />
                <Form.Group
                    label={t("remoteplay.register.name")}
                    helpText={t("remoteplay.register.nameHelp")}>
                    <Form.Text
                        name="name"
                        context={formContext}
                        props={{ placeholder: t("remoteplay.register.namePlaceholder") }}
                    />
                </Form.Group>

                <Form.Group
                    label={t("remoteplay.register.ip")}
                    helpText={t("remoteplay.register.ipHelp")}>
                    <div className="d-flex gap-2 align-items-start">
                        <div className="flex-grow-1">
                            <Form.Text
                                name="ip"
                                context={formContext}
                                props={{ placeholder: t("remoteplay.register.ipPlaceholder") }}
                            />
                        </div>
                        <bs.Button
                            variant="secondary"
                            title={t("remoteplay.register.discover")}
                            aria-label={t("remoteplay.register.discover")}
                            disabled={isDiscovering}
                            onClick={handleDiscover}
                        >
                            {isDiscovering ? (
                                <bs.Spinner animation="border" size="sm" />
                            ) : (
                                <i className="bi bi-broadcast"></i>
                            )}
                        </bs.Button>
                    </div>
                    {discovered.length > 0 && (
                        <bs.ListGroup className="mt-2">
                            <div className="text-muted small px-2 pt-1 pb-2">
                                {t("remoteplay.register.discoverResultsLabel")}
                            </div>
                            {discovered.map((device) => (
                                <bs.ListGroup.Item
                                    key={device.ip}
                                    action
                                    onClick={() => applyDiscovered(device)}
                                    className="d-flex justify-content-between align-items-center"
                                >
                                    <span>{device.name || device.ip}</span>
                                    <span className="text-muted small">
                                        {device.hostType} &middot; {device.ip}
                                    </span>
                                </bs.ListGroup.Item>
                            ))}
                        </bs.ListGroup>
                    )}
                </Form.Group>

                <Form.Group
                    label={identifierLabel}
                    helpText={t("remoteplay.register.identifierHint")}
                >
                    <Form.Text name="identifier" context={formContext} />
                    {/* L'IDENTIFIANT PSN S'OBTIENT AILLEURS (décidé par le porteur le
                        2026-10-10). Ce bouton appelait une connexion PSN que le
                        service n'a jamais eue — un bouchon qui ouvrait une page
                        d'erreur de Sony, et une route absente du téléphone. Il
                        mène au site qui donne cet identifiant ; on le recopie
                        dans le champ juste au-dessus. Sur le téléphone, l'adresse
                        s'ouvre dans le navigateur (MainActivity). */}
                    <bs.Button
                        as="a"
                        href={PSN_ACCOUNT_ID_URL}
                        target="_blank"
                        rel="noopener noreferrer"
                        variant="link"
                        size="sm"
                        className="mt-2 p-0 text-decoration-none"
                    >
                        <div className="d-flex align-items-center justify-content-end">
                            {t("remoteplay.register.getViaPsnLogin")}
                        </div>
                    </bs.Button>
                </Form.Group>

                <Form.Group
                    label={t("remoteplay.register.pin")}
                    helpText={t("remoteplay.register.pinHelp")}
                >
                    <Form.IntFloat
                        name="pin"
                        context={formContext}
                        min={0}
                        max={99999999}
                        step={1}
                        props={{ placeholder: t("remoteplay.register.pinPlaceholder") }}
                    />
                </Form.Group>
            </SectionPanel>
        </OffCanvas>
    );
}

// ── Liste des appareils d'une console donnée ───────────────────────────────
function DeviceList({ platform, hostType, consoleName, t, context }) {
    // Keys must match the Go backend's actual status strings exactly
    // (listDevicesWithLiveStatus / handleWakeUp in playstation_handlers.go
    // only ever emit "awake"/"standby"/"offline", never "online") — a stale
    // "online" key here meant every genuinely-awake device fell through to
    // the {bg:"secondary", label: device.status} fallback, showing the raw
    // untranslated word "awake" instead of a proper badge.
    const statusMap = {
        awake: { bg: "success", label: t("remoteplay.status.awake") },
        standby: { bg: "warning", label: t("remoteplay.status.standby") },
        offline: { bg: "secondary", label: t("remoteplay.status.offline") },
    };
    const [devices, setDevices] = useState([]);
    // LES CONSOLES D'AILLEURS VIVENT À PART, et ce n'est pas un détail de
    // rangement : le hub de statut ci-dessous pousse la liste LOCALE en
    // entier, à chaque changement. Fusionner les deux dans un seul état
    // ferait disparaître les fédérées au premier réveil de console d'ici.
    const [federatedDevices, setFederatedDevices] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    // Cet hôte peut-il appairer une console de SON réseau — et donc en tenir
    // une liste à lui ?
    //
    // C'était `spawnsSessions`, ce qui revenait à dire « un hôte qui n'ouvre
    // pas de session ne peut pas appairer ». Vrai par accident tant qu'un
    // téléphone ne parlait pas aux consoles ; faux depuis qu'il sert
    // lui-même les routes remote-play (C9-P6). Une capacité doit dire ce
    // qu'elle est.
    //
    // Et ces consoles sont celles de QUI JOUE ICI : sur un téléphone, de son
    // propriétaire (`playsHere`). Un membre n'a ni à les voir — le carnet
    // porte leurs clés — ni à en ajouter (2026-10-08).
    const { playsHere } = useAuth();
    const hostsDevices = can("pairsConsoles") && playsHere;

    // Explicit one-shot re-fetch for actions this component itself triggers
    // (register/delete) — snappier than waiting for the next hub push.
    // Background status changes (wake, session start/end, the server's own
    // 5s TCP-reachability poll) instead arrive live via the "device-status"
    // hub below — without it, e.g. waking a console never flips its card
    // from Standby to Online without a manual reload.
    const refresh = async () => {
        if (hostsDevices) {
            const result = await listDevices();
            if (result.ok) {
                setDevices(result.data || []);
            } else {
                context.toast.open(result.errorMessage || t("remoteplay.loadFailed"), {
                    variant: "error",
                });
            }
        }

        // UN SERVEUR FÉDÉRÉ MUET NE FAIT PAS D'ERREUR À L'ÉCRAN.
        //
        // Le kernel a déjà sauté ceux qui ne répondent pas ; ce qui arrive
        // ici est ce qu'on a pu joindre. Une toast rouge parce que la machine
        // d'un ami est éteinte serait du bruit — on ne peut rien y faire
        // depuis cet écran.
        const federated = await listFederatedDevices();
        setFederatedDevices(federated.ok ? (federated.data || []) : []);
        setIsLoading(false);
    };

    const onDevicesUpdate = useCallback((updated) => {
        setDevices(updated);
        setIsLoading(false);
    }, []);
    useDeviceStatus({
        onDevicesUpdate,
        onStatusUpdate: onDevicesUpdate,
        enabled: hostsDevices,
    });
    // Les consoles d'ailleurs n'ont pas ce WebSocket : elles se relisent.
    const onFederatedDevices = useCallback((updated) => setFederatedDevices(updated), []);
    useFederatedDeviceStatus({
        enabled: federatedDevices.length > 0,
        onDevices: onFederatedDevices,
    });

    useEffect(() => {
        refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // UNE SOURCE À LA FOIS, comme dans la ludothèque (#166).
    //
    // Les deux listes étaient FUSIONNÉES — d'ici puis d'ailleurs — ce qui
    // mêlait dans un même défilement des consoles qu'on allume d'un geste et
    // des consoles qu'il faut d'abord emprunter à quelqu'un. La question qu'on
    // se pose devant cet écran est « qu'est-ce que J'AI » ou « qu'est-ce que
    // MON AMI prête », jamais les deux à la fois.
    //
    // Le descripteur est celui de la ludothèque, mot pour mot (`local` ou
    // `peer:<id>`) : deux écrans qui posent la même question doivent y répondre
    // dans la même langue, sinon chacun invente la sienne.
    const toutesLesConsoles = platform === "playstation"
        ? [...devices, ...federatedDevices]
            .filter((device) => String(device.hostType).toUpperCase() === hostType)
        : [];

    // LE LOCAL OUVRE TOUJOURS LA LISTE, même seul : le sélecteur ne sert pas
    // qu'à choisir, il dit aussi d'où vient ce qu'on regarde.
    const availableSources = useMemo(() => {
        const trouves = new Map();
        for (const device of toutesLesConsoles) {
            if (!device.serverId) {
                continue;
            }
            const valeur = `peer:${device.serverId}`;
            if (!trouves.has(valeur)) {
                trouves.set(valeur, device.source || t("remoteplay.device.sourceLocal"));
            }
        }

        return [
            { value: "local", label: t("remoteplay.device.sourceLocal") },
            ...[...trouves.entries()]
                .map(([value, label]) => ({ value, label }))
                .sort((a, b) => a.label.localeCompare(b.label)),
        ];
    }, [toutesLesConsoles, t]);

    const [source, setSource] = useState("local");
    // Un pair oublié depuis le dernier affichage retombe sur le local : laisser
    // le sélecteur sur un choix absent montrerait une liste vide en prétendant
    // filtrer.
    const selectedSource = availableSources.some((option) => option.value === source)
        ? source
        : "local";

    const devicesForConsole = toutesLesConsoles.filter((device) => (
        device.serverId
            ? selectedSource === `peer:${device.serverId}`
            : selectedSource === "local"
    ));

    return (
        <>
            {/* D'ABORD LA SOURCE, ENSUITE CE QU'ON PEUT EN FAIRE. On choisit
                d'où on regarde, puis on regarde — et le geste d'ajout, qui ne
                vaut que pour SA propre source, vient après elle. Pas de bouton
                de relecture ici : les routes de remise en cache sont réservées
                aux administrateurs alors que cet écran est ouvert à tous les
                rôles, et un bouton visible qui répond 401 est pire que pas de
                bouton. */}
            <SourceSelector
                t={t}
                value={selectedSource}
                onChange={setSource}
                options={availableSources}
                canRefresh={false}
                label={t("remoteplay.device.sourceTitle")}
                helpText={t("remoteplay.device.sourceHelp")}
            />
            {/* DEUX CONDITIONS, ET ELLES NE DISENT PAS LA MÊME CHOSE.
                `hostsDevices` (`can("pairsConsoles")`) est une CAPACITÉ de
                l'hôte : appairer une console de son réseau. `selectedSource`
                est ce qu'on REGARDE en ce moment. Les confondre serait refaire
                l'erreur que ce fichier a déjà corrigée une fois, quand la
                capacité s'appelait `spawnsSessions`.
                Appairer n'a de sens que sur SA propre source : devant les
                consoles d'un pair, elles sont déjà enregistrées chez lui, et
                un bouton d'ajout y promettrait un geste qui n'existe pas
                (demandé par l'utilisateur le 2026-09-19). */}
            {hostsDevices && selectedSource === "local" && <ListElement
                light={true}
                onClick={() => context.offcanvas.open(
                    <RegisterPanel
                        platform={platform}
                        hostType={hostType}
                        consoleName={consoleName}
                        t={t}
                        context={context}
                        onRegistered={refresh}
                    />,
                    { size: 600 },
                )}
                icon="bi bi-plus-lg"
                label={t("remoteplay.addDevice")}
                helpText={t("remoteplay.addDeviceDesc")}
            />}
            <ListingElement
                callback={(item) => (
                    <DeviceCard
                        key={item.id}
                        device={item}
                        statusMap={statusMap}
                        t={t}
                        onClick={() => context.offcanvas.open(
                            <DeviceDetailPanel
                                device={item}
                                statusMap={statusMap}
                                t={t}
                                context={context}
                                onChanged={({ removed }) => setDevices((current) => (
                                    // L'appareil retiré part de la liste, SUR
                                    // PLACE (#295) : elle ne se relit pas.
                                    current.filter((other) => other.id !== removed)))}
                            />,
                            { size: 600 },
                        )}
                    />
                )}
                items={devicesForConsole}
                itemCount={devicesForConsole.length}
                isLoading={isLoading}
                texts={{
                    placeholderSearch: null,
                    itemsFound: t("remoteplay.devicesFound"),
                    noItemsFound: t("remoteplay.noDevicesFound"),
                }}
            />
        </>
    );
}

// « Ce serveur ne nous connaît pas » se lit dans le STATUT que porte l'erreur
// d'`apiRequest`. On fouillait son message tant que les services Go refusaient
// en texte brut et que la lecture JSON échouait avant qu'on regarde le 401.
function isUnauthorized(error) {
    return error?.status === 401 || error?.status === 403;
}

// ── Page: liste des appareils d'une console donnée ─────────────────────────
export default function RemotePlay() {
    const { t } = useTranslation();
    const { playsHere } = useAuth();
    const location = useLocation();
    const context = useContext(Skeleton.context);
    const consoles = useConsoles();
    const consoleId = location.pathname.split("/").filter(Boolean)[1] || "";
    const consoleMeta = consoles.find(
        (c) => c.id === consoleId && c.method === "remoteplay",
    );
    const consoleName = consoleMeta?.name || consoleId.toUpperCase();
    const platform = PLATFORM_BY_CONSOLE[consoleId] || "playstation";
    // LE PRÉFIXE N'EST PAS UN TYPE DE MATÉRIEL.
    //
    // `hostType` est ce que la CONSOLE dit d'elle-même (« PS5 »), et c'est
    // sur lui que la liste d'appareils filtre. Le tirer de l'identifiant tel
    // quel donnerait « RP-PS5 », qui ne correspond à aucune console — la
    // liste serait vide sans que rien ne le dise (2026-09-10, en préfixant
    // les identifiants du jeu à distance).
    const hostType = consoleId.replace(/^rp-/, "").toUpperCase();



    return (
        <Page
            label={consoleName}
            /* Le sous-titre PROMETTAIT d'ajouter un appareil sur un hôte qui
               n'a pas le bouton — le téléphone, dont la même capacité
               `pairsConsoles` masque l'ajout plus bas. Une page qui annonce
               un geste impossible se lit comme un bug de l'application, et
               c'en est un : signalé le 2026-09-09 (« je fais comment pour
               tester ? »). Elle dit maintenant ce qui est vrai chez elle.
               NUANCE DEPUIS LE 2026-09-19 : le bouton dépend AUSSI de la
               source regardée (local seulement). Ce sous-titre ne suit pas
               cette seconde condition, délibérément — il décrit ce que l'hôte
               SAIT faire, pas ce que la vue courante propose, et le faire
               changer au fil du sélecteur le rendrait mouvant sans rien
               apprendre. */
            helpText={can("pairsConsoles") && playsHere
                ? t("remoteplay.devicesDescription")
                : t("remoteplay.devicesDescriptionRemote")}>
            <DeviceList
                platform={platform}
                hostType={hostType}
                consoleName={consoleName}
                t={t}
                context={context}
            />
        </Page>
    );
}
