import React, { useContext, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { hasLocalLibrary, onLocalLibraryReloaded } from "../libs/localLibrary.js";
import { federatedId, federatedServerNow, federatedServers, parseFederatedId } from "../libs/federatedHost.js";
import { describeResources } from "../libs/hostResources.js";
import SourceSelector from "../profile/settings/admin/storage/SourceSelector.jsx";
import { can, capabilities } from "../libs/capabilities.js";
import { useConsoles } from "../contexts/ConsolesContext.jsx";
import { listRoms, listInternalDisks, createSession, reportLaunchFailure } from "../api/retrogaming.js";
import { launchSession, stopSession } from "../api/spawnSessions.js";
import { LaunchScreen } from "../stream/shared/SessionStream.jsx";
import { handleLaunchConflict } from "../shared/LaunchConflict.jsx";
import { useAuth, getStoredUsername } from "../profile/auth/AuthContext.jsx";
import { Page, ListingElement } from "../shared/templates/Essentials.jsx";
import { GameCard } from "./GameCard.jsx";
import { GameDetailPanel } from "./GameDetailPanel.jsx";
import { StatusFilter, matchesStatuses } from "./GameStatus.jsx";
import { ITEMS_PER_PAGE, SEARCH_DEBOUNCE_MS } from "./helpers.js";
import { withStep } from "../stream/shared/hooks/use-launch-progress.js";

export default function Games() {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const context = useContext(Skeleton.context);
    const { token, user } = useAuth();
    const consoles = useConsoles();
    const consoleId = location.pathname.split("/").filter(Boolean)[1] || "";
    const consoleMeta = consoles.find((c) => c.id === consoleId);
    const consoleName = consoleMeta?.name || consoleId.toUpperCase();
    const [items, setItems] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    // The URL's own ?page= is the source of truth (not a separate useState)
    // — that's what makes a refresh, or coming back from a game via the
    // streaming page's Exit, land back on the same page instead of
    // resetting to 1. setSearchParams pushes a new history entry by
    // default (not `replace`), so the browser Back button steps through
    // pagination too, same as any other in-app navigation.
    const [searchParams, setSearchParams] = useSearchParams();
    const currentPage = parseInt(searchParams.get("page"), 10) || 1;
    // L'ADRESSE D'À PRÉSENT, pas celle du rendu qui a créé la fonction : le
    // chargement de la liste l'appelle après un `await`, et le `prev` de
    // setSearchParams y réécrivait des paramètres déjà retirés — un `?game=`
    // servi revenait, et sa fiche s'ouvrait une seconde fois, restée ensuite
    // par-dessus la partie (2026-10-15).
    const setCurrentPage = (page) => {
        const next = new URLSearchParams(window.location.search);

        if (page > 1) {
            next.set("page", String(page));
        } else {
            next.delete("page");
        }

        setSearchParams(next);
    };
    // Seeded from the URL's own ?q= (same reasoning as currentPage above) —
    // coming back from a game via Exit, or a refresh, restores the actual
    // search results list instead of the unfiltered one.
    const initialQuery = searchParams.get("q") || "";
    const [searchInput, setSearchInput] = useState(initialQuery);
    // LE FILTRE PAR STATUT (chantier 30) : sur la liste déjà là, sans
    // requête — le statut est sur chaque jeu.
    const [statuses, setStatuses] = useState([]);
    const [searchQuery, setSearchQuery] = useState(initialQuery);

    // Debounce the search box before it triggers a new backend fetch.
    useEffect(() => {
        const timeout = setTimeout(
            () => setSearchQuery(searchInput),
            SEARCH_DEBOUNCE_MS,
        );
        return () => clearTimeout(timeout);
    }, [searchInput]);

    // Mirrors searchQuery into the URL — `replace` (not push, unlike
    // setCurrentPage) since this fires on every debounced keystroke; a
    // history entry per letter typed would make the Back button useless.
    useEffect(() => {
        setSearchParams((prev) => {
            const next = new URLSearchParams(prev);

            if (searchQuery) {
                next.set("q", searchQuery);
            } else {
                next.delete("q");
            }

            return next;
        },
            { replace: true },
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchQuery]);

    // Skips resetting to page 1 on the very first load — the whole point of
    // reading `page` from the URL above is to restore whatever page the
    // user was on, which this effect would otherwise immediately stomp on
    // mount (it also runs once for the initial consoleId/searchQuery).
    // Later runs (an actual new search) still reset to page 1 as before.
    const isFirstLoad = useRef(true);
    // La recherche de la liste affichée : une liste RELUE avec la même
    // recherche (`libraryEpoch`, la bibliothèque du téléphone qui change) garde
    // sa page — sinon chaque relecture y ramenait à la page 1, retour d'une
    // partie compris.
    const listedQuery = useRef(searchQuery);
    // Bumped by the native "library changed" event below: a game downloaded
    // during play just entered the library, and the open list must reclassify
    // it from remote to local without a reload (2026-08-30).
    const [libraryEpoch, setLibraryEpoch] = useState(0);

    useEffect(() => {
        if (!hasLocalLibrary()) {
            return undefined;
        }

        // Idem : la liste se redessine quand elle a été RELUE, pas quand on
        // annonce qu'elle va l'être.
        return onLocalLibraryReloaded(() => setLibraryEpoch((epoch) => epoch + 1));
    }, []);

    // LES SOURCES : la bibliothèque d'ici, et les serveurs fédérés (chantier
    // 14, 2026-10-12). Les partages montés n'y sont plus : ils ne servent qu'à
    // AJOUTER des jeux, dans Réglages › Jeux. Un serveur fédéré se choisit ici
    // pour voir SES jeux — les jouer chez lui, ou les télécharger.
    //
    // Null tant que la liste n'est pas arrivée : une adresse `?source=peer:3`
    // ne doit pas retomber sur le local au premier rendu.
    const [servers, setServers] = useState(null);

    useEffect(() => {
        let cancelled = false;
        federatedServers().then((found) => {
            if (!cancelled) {
                setServers(found || []);
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    // LES DISQUES INTERNES DE CETTE MACHINE (chantier 23 ; ceux du téléphone
    // depuis le chantier 27) : chacun est une source à part, et ses jeux se
    // jouent ici. Null tant que la liste n'est pas arrivée, pour la même
    // raison que les serveurs ; une machine sans disque interne en a zéro.
    const [disks, setDisks] = useState(null);

    useEffect(() => {
        let cancelled = false;
        listInternalDisks().then((answer) => {
            if (!cancelled) {
                setDisks(answer.ok ? (answer.data || []).filter((disk) => disk.present) : []);
            }
        }).catch(() => {
            if (!cancelled) {
                setDisks([]);
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);
    // Les disques qui ont des jeux de CETTE console : une source vide pour elle
    // n'aurait rien à montrer.
    const consoleDisks = useMemo(
        () => (disks || []).filter((disk) => (disk.consoles?.[consoleId] || 0) > 0),
        [disks, consoleId],
    );

    // DANS L'ADRESSE, comme la page et la recherche : revenir d'une partie
    // jouée chez un serveur fédéré ramène à SA liste.
    const requestedSource = searchParams.get("source") || "local";
    const setSource = (value) => {
        setSearchParams((prev) => {
            const next = new URLSearchParams(prev);

            if (value && value !== "local") {
                next.set("source", value);
            } else {
                next.delete("source");
            }
            next.delete("page");

            return next;
        });
    };
    // LE LOCAL OUVRE TOUJOURS LA LISTE, même quand il est seul : le sélecteur
    // ne sert pas qu'à choisir, il dit aussi d'où vient ce qu'on regarde
    // (demandé le 2026-09-18). La même grammaire que les écrans
    // d'administration : `local` | `peer:<id>`.
    const sourceOptions = useMemo(() => [
        { value: "local", label: t("retrogaming.origin.local") },
        ...consoleDisks.map((disk) => ({ value: `disk:${disk.name}`, label: t("retrogaming.diskSource", { name: disk.name }) })),
        ...(servers || [])
            .map((server) => ({ value: `peer:${server.id}`, label: server.hostname || server.link }))
            .sort((a, b) => a.label.localeCompare(b.label)),
    ], [servers, consoleDisks, t]);
    // Un serveur qui n'est plus enregistré retombe sur le local : laisser le
    // sélecteur sur un choix absent afficherait une liste vide en prétendant
    // filtrer.
    const selectedSource = servers === null || disks === null || sourceOptions.some((option) => option.value === requestedSource)
        ? requestedSource
        : "local";
    const peerId = selectedSource.startsWith("peer:") ? selectedSource.slice("peer:".length) : null;
    const diskName = selectedSource.startsWith("disk:") ? selectedSource.slice("disk:".length) : null;
    // UNE CONSOLE DONT LES JEUX NE SONT QUE SUR UN DISQUE INTERNE s'ouvre sur
    // ce disque, une fois par visite : la bibliothèque n'aurait rien à montrer.
    // Choisir ensuite « bibliothèque » à la main n'y ramène pas.
    const diskPicked = useRef(new Set());

    // POURQUOI UN SERVEUR FÉDÉRÉ NE MONTRE RIEN : « denied » quand il nous
    // refuse l'accès, « offline » quand il ne répond pas. Une liste vide ne
    // dirait ni l'un ni l'autre.
    const [refusal, setRefusal] = useState(null);
    // La source dont `items` est la liste : un lien `?game=` attend la bonne.
    const [listedSource, setListedSource] = useState(null);

    // LES RESSOURCES DU SERVEUR FÉDÉRÉ CHOISI, telles qu'il les déclare
    // (relayées par notre kernel, `GET /api/v1/servers/{id}`) : une console
    // de bureau lancée chez lui joue avec les siennes, pas les nôtres. Null
    // tant qu'on ne les a pas, ou s'il n'en déclare pas.
    const [peerResources, setPeerResources] = useState(null);
    useEffect(() => {
        setPeerResources(null);
        if (!peerId || consoleMeta?.method !== "streamex") {
            return undefined;
        }
        let cancelled = false;
        federatedServerNow(peerId).then((known) => {
            if (!cancelled) setPeerResources(known?.resources || null);
        });

        return () => {
            cancelled = true;
        };
    }, [peerId, consoleMeta?.method]);

    useEffect(() => {
        if ((peerId && servers === null) || (diskName && disks === null)) {
            return undefined;
        }
        let cancelled = false;
        (async () => {
            setIsLoading(true);
            // UNE SOURCE, UNE LISTE (chantier 14) : la bibliothèque, ou les
            // jeux d'UN serveur fédéré. Le kernel ne mêle plus rien — ni les
            // partages montés, ni les serveurs fédérés — à la bibliothèque.
            const result = await listRoms(consoleId, searchQuery, null, peerId, diskName);
            if (cancelled) return;
            if (result.ok && selectedSource === "local" && !searchParams.get("source") && !searchQuery
                && (result.data || []).length === 0 && consoleDisks.length > 0
                && !diskPicked.current.has(consoleId)) {
                diskPicked.current.add(consoleId);
                setSource(`disk:${consoleDisks[0].name}`);

                return;
            }
            if (result.ok) {
                setItems(result.data || []);
                setRefusal(null);
                if (!isFirstLoad.current && searchQuery !== listedQuery.current) {
                    setCurrentPage(1);
                }
                listedQuery.current = searchQuery;
                isFirstLoad.current = false;
            } else if (peerId && (result.status === 403 || result.status === 502)) {
                setItems([]);
                setRefusal(result.status === 403 ? "denied" : "offline");
            } else {
                setItems([]);
                setRefusal(null);
                context.toast.open(result.errorMessage || t("retrogaming.loadFailed"), {
                    variant: "error",
                });
            }
            setListedSource(selectedSource);
            setIsLoading(false);
        })();
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, searchQuery, libraryEpoch, selectedSource, servers === null, disks === null]);


    // Un lancement refusé parce qu'une autre partie est là : le choix proposé
    // est le même sur toutes les pages qui lancent (shared/LaunchConflict.jsx).
    const handleConflict = (result, retryLaunch, host = null) =>
        handleLaunchConflict({ result, context, t, token, host, navigate, retryLaunch });

    // L'ÉCRAN DU LANCEMENT, DÈS LE CLIC (#287, demandé par le porteur le
    // 2026-10-10 : « dès qu'on clique sur Play on doit voir l'écran de toutes
    // étapes qui défilent »). La réponse du lancement peut prendre des
    // minutes — un jeu distant descend d'abord, un conteneur démarre — et on
    // l'attendait ici, sur la fiche, sans rien montrer. Les étapes arrivent
    // pourtant déjà sur le canal de la commande (`onProgress`) : on les
    // affiche tout de suite, sur le même écran que la page de jeu, qui le
    // reprend à la réponse et le garde jusqu'à la première image.
    //
    // ANNULER pendant cette attente ne peut pas interrompre le kernel : son
    // lancement va au bout (le jeu descendu reste en cache). La partie qui en
    // naît est arrêtée à sa réponse, dès qu'elle a un identifiant.
    const [launchScreen, setLaunchScreen] = useState(null);
    const launchRunRef = useRef(null);

    // `host` — le serveur d'un ami quand le jeu se lance chez lui : le flux du
    // lancement, et donc le fond de l'écran, est sur SA passerelle.
    const openLaunchScreen = (title, host = null) => {
        const run = { cancelled: false };
        launchRunRef.current = run;
        context.offcanvas.close();
        setLaunchScreen({ title, steps: [], failure: null, reportState: null, backdrop: null });
        const update = (change) => {
            if (launchRunRef.current === run) {
                setLaunchScreen((screen) => (screen ? { ...screen, ...change(screen) } : screen));
            }
        };

        return {
            run,
            // La même lecture qu'useLaunchProgress, sur le canal de la
            // commande plutôt que sur le sujet du lancement.
            //
            // LE FOND D'ABORD (chantier 35) : le premier message, sans étape,
            // donne l'identifiant du lancement — l'écran se branche sur son
            // flux, où la passerelle tient l'image de l'intro. Sous le même
            // compte que la page de jeu qui suivra : l'intro ne se joue
            // qu'une fois par compte, et les deux la jouent ensemble.
            onProgress: (message, frame) => {
                if (frame?.launchId) {
                    update((screen) => (screen.backdrop ? {} : {
                        backdrop: {
                            launch: {
                                streamId: frame.launchId,
                                host: host ? { origin: host.origin, token: host.token } : null,
                                username: (host ? host.username : getStoredUsername()) || null,
                            },
                        },
                    }));
                }
                if (!message) {
                    return;
                }
                if (message.type === "failed") {
                    run.failure = { step: message.step, error: message.error || null };
                }
                update((screen) => ({
                    steps: withStep(screen.steps, {
                        step: message.step,
                        detail: message.detail || null,
                        state: message.type === "failed" ? "error" : "done",
                        at: message.at || null,
                    }),
                    failure: message.type === "failed"
                        ? { step: message.step, error: message.error || null }
                        : screen.failure,
                }));
            },
            // Une étape en erreur garde l'écran (#191 : la croix et
            // « signaler ») ; un refus sans étape se dit par un message.
            failed: () => Boolean(run.failure),
            close: () => {
                if (launchRunRef.current === run) {
                    launchRunRef.current = null;
                    setLaunchScreen(null);
                }
            },
        };
    };
    const launchScreenRef = useRef(null);
    launchScreenRef.current = launchScreen;

    const cancelLaunchScreen = () => {
        if (launchRunRef.current) {
            launchRunRef.current.cancelled = true;
        }
        launchRunRef.current = null;
        setLaunchScreen(null);
    };

    // La partie née d'un lancement annulé s'arrête aussitôt — chez son hôte
    // quand elle vit ailleurs.
    const stopCancelledLaunch = (platform, result, host) => {
        const sessionId = result?.ok ? (result.data?.sessionId || result.data?.session_id) : null;
        if (sessionId) {
            stopSession(platform, sessionId, host ? host.token : token, host).catch(() => {});
        }
    };

    const sendLaunchReport = async () => {
        const screen = launchScreenRef.current;
        if (!screen) {
            return;
        }
        setLaunchScreen((current) => (current ? { ...current, reportState: "sending" } : current));
        const result = await reportLaunchFailure({
            context: `lancement (${screen.title})`,
            steps: screen.steps,
            detail: screen.failure,
        });
        setLaunchScreen((current) => (current ? { ...current, reportState: result.ok ? "sent" : "failed" } : current));
        context.toast.open(
            result.ok ? t("streaming.launch.reported") : t("streaming.launch.reportFailed"),
            { variant: result.ok ? "success" : "error" },
        );
    };

    // Creates the session server-side FIRST (see createSession's doc,
    // api/retrogaming.js) — the returned sessionId is the ONLY thing the
    // unified Streaming page (../Streaming.jsx) needs; a copy-pasted or
    // refreshed streaming link re-attaches to this same session instead of
    // spawning a duplicate.
    const handlePlay = async (rom, core, bios, streamSettings, host = null) => {
        // UN SEUL GESTE (chantier 8, C8-P3) : on demande une session, on
        // reçoit son identifiant, on va la regarder. Ce qui se passe derrière
        // — un conteneur sur un serveur, un cœur dans le processus de
        // l'application — ne regarde pas cette page.
        //
        // Avant, le téléphone partait par un chemin à lui : l'adresse portait
        // `device=1&path=…&kind=…&source=…&files=…`, et la page de streaming
        // lançait elle-même le jeu au moment de négocier. Elle devait donc
        // savoir d'où vient un fichier et s'il tourne déjà — deux questions
        // dont le serveur a la réponse, et que la page se posait à sa place.
        // Les deux s'étaient déjà trompées (2026-09-02).

        // External consoles (Switch/spawn-gaming) don't go through the retro
        // pipeline: the sessions service spawns a dedicated container and
        // returns play/watch URLs. The game opens in the streamer's own
        // embedded UI until the
        // unified Streaming page learns this platform (next frontend phase).
        // UNE CONSOLE STREAMEX LANCÉE ICI, SUR UN HÔTE SANS CONTENEURS.
        //
        // Le lancement streamex du serveur SPAWNE un conteneur : ce chemin
        // n'existe pas sur le téléphone, qui répondait 405 dès qu'on
        // choisissait l'émulateur local (signalé le 2026-09-08). L'entrée
        // locale venait d'être ouverte pour la Switch ; son lancement, lui,
        // partait encore chez le serveur.
        //
        // Depuis #195 les deux hôtes servent la MÊME porte
        // (la commande `launch` du centre de messages) : on y envoie
        // `platform: "retrogaming"`, et
        // l'hôte de session du téléphone sait reconnaître une console à
        // émulateur embarqué (StreamingHost).
        // Même geste, donc, et c'est le PROPRE de ce chantier : ce qui tourne
        // derrière ne regarde pas cette page.
        if (consoleMeta?.method === "streamex" && !host && !can("spawnsSessions")) {
            const screen = openLaunchScreen(rom.title);
            // LES RÉGLAGES DE FLUX PARTENT AUSSI D'ICI. Ce chemin passait
            // `null` : le panneau s'affichait, se sauvegardait, et le
            // téléphone ouvrait son écran X en 720p à 60 images quoi qu'on
            // choisisse (signalé par le porteur le 2026-10-11).
            const result = await createSession(
                consoleId, rom.reference, rom.title, core || "", bios || "", user?.username,
                null, streamSettings, { onProgress: screen.onProgress },
            );
            if (screen.run.cancelled) {
                stopCancelledLaunch("retrogaming", result, null);

                return;
            }
            if (!result.ok) {
                if (!screen.failed()) {
                    screen.close();
                    context.toast.open(result.errorMessage || t("retrogaming.loadFailed"), {
                        variant: "error",
                    });
                }

                return;
            }
            context.offcanvas.close();
            const backParams = new URLSearchParams({
                sessionId: result.data.sessionId,
                page: String(currentPage),
            });
            // Le fond de l'intro suit sur la page de jeu (chantier 35).
            if (result.launchId) {
                backParams.set("launchId", result.launchId);
            }
            if (searchQuery) backParams.set("q", searchQuery);
            if (selectedSource !== "local") backParams.set("source", selectedSource);
            backParams.set("play", "1");
            navigate(`/streaming?${backParams.toString()}`);

            return;
        }

        if (consoleMeta?.method === "streamex") {
            // `host` — LE JEU EST CHEZ QUELQU'UN, C'EST LUI QUI LE LANCE.
            //
            // Rien d'autre ne change : même route, même corps de requête,
            // même conteneur au bout. Ce qui change est à qui on le demande,
            // et avec quel jeton — le nôtre ne veut rien dire chez lui.
            const screen = openLaunchScreen(rom.title, host);
            const result = await launchSession(
                rom.title,
                consoleId,
                token,
                streamSettings,
                host,
                { onProgress: screen.onProgress, core },
            );
            if (screen.run.cancelled) {
                stopCancelledLaunch("streamex", result, host);

                return;
            }
            // L'IDENTIFIANT DU LANCEMENT VIENT DU KERNEL, avec la réponse — et
            // SEULEMENT pour un lancement d'ici : jouer chez un pair lance sur
            // SON kernel, dont la page de jeu ne suivra pas le canal.
            const launchId = host ? null : result.launchId || null;
            // One player-session per user now covers streamex too (kernel
            // étape 4) — same switch-confirm flow as the retro path below.
            //
            // UN CONFLIT N'EST PAS UN ÉCHEC À SIGNALER : sa fenêtre prend la
            // main, et l'écran des étapes s'efface, même avec une étape en
            // erreur. Depuis #289 il passe AU-DESSUS des fenêtres de Skeleton,
            // et cachait le choix « Y retourner » (relevé par flows.js le
            // 2026-10-11).
            if (handleConflict(
                result,
                () => handlePlay(rom, core, bios, streamSettings, host),
                host,
            )) {
                screen.close();

                return;
            }
            if (!result.ok && !screen.failed()) {
                screen.close();
            }
            if (!result.ok || !result.data?.session_id) {
                if (!screen.failed()) {
                    screen.close();
                    context.toast.open(
                        result.errorMessage ||
                        t("streaming.connection.errors.sessionCreateFailed"),
                        { variant: "error" },
                    );
                }
                return;
            }
            // Même appel que les chemins rétro plus bas : la fiche
            // panel this was opened from must not stay open underneath the
            // streaming page (it would reappear on Exit).
            context.offcanvas.close();
            // Same unified page as every other platform — the session is
            // resolved via SessionPool (the sessions service now pushes its
            // spawned streamex sessions on the façade).
            // L'IDENTIFIANT REPART PRÉFIXÉ quand la session vit ailleurs :
            // l'hôte l'a frappé dans SON espace, et deux serveurs y frappent
            // les mêmes. Sans le préfixe, la page de streaming irait chercher
            // cette session dans notre propre façade — au mieux elle ne
            // trouverait rien, au pire l'homonyme d'ici. Même écriture que
            // pour une console fédérée (C9-P4) et pour un jeu (chantier 6).
            const backParams = new URLSearchParams({
                sessionId: host
                    ? federatedId(host.serverId, result.data.session_id)
                    : result.data.session_id,
                page: String(currentPage),
            });
            if (launchId) {
                backParams.set("launchId", launchId);
            }
            if (searchQuery) backParams.set("q", searchQuery);
            if (selectedSource !== "local") backParams.set("source", selectedSource);
            // Launching a game is an intent to play — carry it, or the
            // player lands on a black watch screen of a session whose
            // emulator has not started, with no idea why.
            backParams.set("play", "1");
            navigate(`/streaming?${backParams.toString()}`);
            return;
        }

        if (!core) {
            context.toast.open(t("retrogaming.noCoreConfigured"), {
                variant: "error",
            });
            return;
        }

        // ÉMULER CHEZ CELUI QUI A LE JEU (C9-P5).
        //
        // Même route, même corps de requête, même conteneur au bout : ce qui
        // change est à qui on le demande. Deux détails, et les deux sont des
        // défauts si on les oublie — la référence doit être LA SIENNE (notre
        // préfixe ne veut rien dire chez lui) et le nom doit être celui qu'on
        // porte chez lui (sa file de joueurs est indexée par nom).
        const remoteReference = host ? parseFederatedId(rom.reference)?.id : null;
        const screen = openLaunchScreen(rom.title, host);
        const result = await createSession(
            consoleId,
            remoteReference || rom.reference,
            rom.title,
            core,
            bios || "",
            host ? host.username : user?.username,
            host,
            // #150 — la résolution choisie part avec la demande de lancement,
            // comme pour streamex. Vide : la console garde sa géométrie.
            streamSettings,
            { onProgress: screen.onProgress },
        );
        if (screen.run.cancelled) {
            stopCancelledLaunch("retrogaming", result, host);

            return;
        }
        // L'IDENTIFIANT DU LANCEMENT (#191) vient du kernel, avec la réponse,
        // et SEULEMENT pour un lancement d'ici : émuler chez un pair lance
        // sur SON kernel — un écran d'étapes vide serait pire que pas
        // d'écran. Le téléphone en rend un depuis le chantier 35, pour le fond
        // de l'intro ; ses étapes, elles, passent par son flux de nouvelles.
        const launchId = host ? null : result.launchId || null;

        if (!result.ok) {
            // Un conflit : sa fenêtre prend la main, l'écran s'efface (voir
            // le chemin streamex plus haut).
            if (handleConflict(
                result,
                () => handlePlay(rom, core, bios, streamSettings, host),
                host,
            )) {
                screen.close();

                return;
            }
            if (!screen.failed()) {
                screen.close();
                context.toast.open(result.errorMessage || t("retrogaming.loadFailed"), {
                    variant: "error",
                });
            }
            return;
        }
        // A plain navigation, not an offcanvas — same call made for remote-
        // play's Streaming/Watch pages: a fullscreen offcanvas kept growing
        // skeleton.js's generic offcanvas API with streaming-only concerns
        // for a single caller.
        // offcanvas.close() first — the game detail panel this was opened
        // from otherwise just stays open underneath, reappearing once the
        // streaming page's own Exit navigates back to /retrogaming/{consoleId}.
        context.offcanvas.close();
        // Carries the current page + search query along so Streaming.jsx's
        // Exit can send us back to the exact same filtered/paginated list
        // instead of resetting to the unfiltered page 1 (see currentPage/
        // searchQuery above) — pure UI navigation state, not part of the
        // session itself.
        // Préfixé quand la session vit ailleurs — même raison que pour
        // streamex et pour une console fédérée : deux serveurs frappent les
        // mêmes identifiants, et notre façade ne connaît pas la sienne.
        const backParams = new URLSearchParams({
            sessionId: host
                ? federatedId(host.serverId, result.data.sessionId)
                : result.data.sessionId,
            page: String(currentPage),
        });

        // L'IDENTIFIANT VOYAGE AVEC LA SESSION (#191).
        //
        // On arrive ici APRÈS la réponse du lancement — volontairement : c'est cette
        // attente qui permet de garder le traitement du conflit « tu joues déjà
        // ailleurs », dont le bouton de reprise appartient à ce composant.
        // L'écran d'étapes n'est donc pas perdu pour autant : le kernel garde
        // le salon une minute après la fin, et la page de streaming rejoue tout
        // le lancement en s'abonnant.
        if (launchId) {
            backParams.set("launchId", launchId);
        }

        if (searchQuery) backParams.set("q", searchQuery);
        if (selectedSource !== "local") backParams.set("source", selectedSource);
        // Launching a game is an intent to play — carry it, or the
        // player lands on a black watch screen of a session whose
        // emulator has not started, with no idea why.
        backParams.set("play", "1");
        navigate(`/streaming?${backParams.toString()}`);
    };

    // Une seule façon d'ouvrir la fiche d'un jeu — le clic d'une vignette,
    // et le lien qui arrive d'ailleurs (voir l'effet `?game=` plus bas). Les
    // deux doivent passer par le même chemin, sinon l'un des deux finit par
    // manquer un prop.
    const openGameDetail = (rom) => context.offcanvas.open(
        <GameDetailPanel
            rom={rom}
            consoleId={consoleId}
            consoleMeta={consoleMeta}
            t={t}
            context={context}
            onPlay={handlePlay}
        />,
        { size: 600 },
    );

    // OUVRIR UN JEU DÉSIGNÉ PAR L'ADRESSE — `?game=<référence>`.
    //
    // Une adresse qui désigne un jeu le rouvre : on atterrit ICI — la page
    // qui possède déjà `handlePlay`, les variantes, le choix du cœur et du
    // BIOS.
    //
    // On attend que le catalogue soit chargé : la référence seule ne suffit
    // pas à construire une fiche, il faut le jeu entier. Et on retire le
    // paramètre une fois servi, sinon revenir en arrière rouvrirait le
    // panneau indéfiniment.
    //
    // UN JEU D'UN SERVEUR FÉDÉRÉ vit dans la liste de SON serveur (chantier
    // 14) : on la choisit d'abord, puis on attend qu'elle soit arrivée.
    const requestedGame = searchParams.get("game");
    useEffect(() => {
        if (!requestedGame) {
            return;
        }
        const remote = parseFederatedId(requestedGame);
        // Un lien qui désigne un disque interne (chantier 23) y reste : le jeu
        // a la même référence partout, c'est la source demandée qui dit où.
        const wanted = remote
            ? `peer:${remote.serverId}`
            : (requestedSource.startsWith("disk:") ? requestedSource : "local");
        const forget = () => setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.delete("game");

            return next;
        }, { replace: true });

        if (requestedSource !== wanted) {
            setSource(wanted);

            return;
        }
        if (servers === null || disks === null || isLoading) {
            return;
        }
        // Son serveur n'est plus enregistré : le jeu n'a plus de liste.
        if (selectedSource !== wanted) {
            forget();
            context.toast.open(t("retrogaming.gameGone"), { variant: "error" });

            return;
        }
        if (listedSource !== wanted) {
            return;
        }
        const rom = items.find((item) => item.reference === requestedGame);
        forget();
        if (rom) {
            openGameDetail(rom);
        } else if (!remote && consoleDisks.length > 0) {
            // UN JEU QUI N'EST PLUS QUE SUR UN DISQUE INTERNE (chantier 23) :
            // sa référence est la même partout, on le cherche donc sur les
            // disques avant de le dire perdu — un lien doit le rouvrir.
            (async () => {
                for (const disk of consoleDisks) {
                    const found = await listRoms(consoleId, "", null, null, disk.name);
                    const onDisk = (found.ok ? found.data || [] : []).find((item) => item.reference === requestedGame);
                    if (onDisk) {
                        openGameDetail(onDisk);

                        return;
                    }
                }
                context.toast.open(t("retrogaming.gameGone"), { variant: "error" });
            })();
        } else {
            context.toast.open(t("retrogaming.gameGone"), { variant: "error" });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [requestedGame, requestedSource, selectedSource, listedSource, servers, isLoading, items]);

    const searchContext = {
        values: { q: searchInput, statuses },
        setValues: (v) => {
            setSearchInput(v.q ?? searchInput);
            if (v.statuses && v.statuses !== statuses) {
                setStatuses(v.statuses);
                setCurrentPage(1);
            }
        },
    };
    const shown = useMemo(() => items.filter((item) => matchesStatuses(item, statuses)), [items, statuses]);

    return (
        <Page
            label={t("retrogaming.gamesTitle", { console: consoleName })}
            helpText={t("retrogaming.gamesDescription")}>
            {/* Prévenir AVANT la partie, pas après le plantage. Le registre des
                consoles dit lesquelles, et pourquoi (`experimental`, D9) :
                shadPS4 (PS4), KytyPS5 (PS5) et Xenia (Xbox 360) sont
                expérimentaux de bout en bout ; sur Switch, Eden ne lâche que
                sur quelques jeux lourds ; sur Wii U, certains jeux demandent un
                autre firmware que celui installé (ajouté le 2026-09-15, à la
                demande de l'utilisateur). La liste était écrite ici en dur :
                une console ajoutée ne prévenait pas. */}
            {consoleMeta?.experimental && (
                <bs.Alert variant="warning" className="py-2 small d-flex align-items-start gap-2">
                    <i className="bi bi-exclamation-triangle-fill mt-1"></i>
                    <span>
                        {consoleMeta.experimental === "heavy"
                            ? t("retrogaming.experimentalHeavyWarning")
                            : consoleMeta.experimental === "firmware"
                                ? t("retrogaming.experimentalFirmwareWarning")
                                : t("retrogaming.experimentalWarning")}
                    </span>
                </bs.Alert>
            )}
            {/* AVANT LA BARRE DE RECHERCHE, et toujours affiché — même quand
                le local est la seule source (demandé le 2026-09-18). Il ne
                sert pas qu'à choisir : il dit d'où vient ce qu'on regarde.
                `ListingElement` rendant lui-même sa recherche, « avant » veut
                dire ici, au-dessus de lui.
                Pas de bouton de remise en cache : ses deux routes sont
                réservées aux administrateurs, et cette page est ouverte à
                tous les rôles (voir SourceSelector). */}
            <SourceSelector
                t={t}
                value={selectedSource}
                onChange={setSource}
                options={sourceOptions}
                canRefresh={false}
                label={t("retrogaming.origin.title")}
                helpText={t("retrogaming.filterSourceHelp")} />
            {/* LA MACHINE QUI FERAIT TOURNER UNE CONSOLE DE BUREAU, dite avant
                la partie, APRÈS le choix de la source (le porteur,
                2026-10-23) : « Sur cet appareil » et un disque interne jouent
                avec les ressources d'ici, un serveur fédéré avec les siennes.
                Le téléphone garde ses deux messages (2026-10-15) ; un serveur,
                le nôtre ou un fédéré, dit ses ressources. Rien tant qu'on ne
                les connaît pas. */}
            {consoleMeta?.method === "streamex" && (() => {
                const here = !peerId;
                const resources = here ? capabilities().resources : peerResources;
                if (!resources) {
                    return null;
                }
                const phone = here && capabilities().platform === "android";
                const host = here ? t("retrogaming.thisServer") : (servers || []).find((server) => String(server.id) === peerId)?.hostname || t("retrogaming.thatServer");
                const described = describeResources(resources, t);
                const text = phone
                    ? (resources.gpu ? t("retrogaming.gpuAvailable") : t("retrogaming.noGpuWarning"))
                    : (resources.gpu
                        ? t("retrogaming.hostResources", { host, resources: described })
                        : t("retrogaming.hostNoGpu", { host, resources: described }));

                return (
                    <bs.Alert variant={resources.gpu ? "success" : "warning"} className="py-2 small d-flex align-items-start gap-2">
                        <i className="bi bi-gpu-card mt-1"></i>
                        <span>{text}</span>
                    </bs.Alert>
                );
            })()}
            {/* UN SERVEUR FÉDÉRÉ QUI REFUSE, OU QUI SE TAIT : un message à la
                place de la liste (demandé par le porteur, chantier 14). */}
            {refusal && (
                <bs.Alert variant="secondary" className="small d-flex align-items-start gap-2">
                    <i className={`bi ${refusal === "denied" ? "bi-shield-lock" : "bi-wifi-off"} mt-1`}></i>
                    <span>{t(refusal === "denied" ? "retrogaming.peerDenied" : "retrogaming.peerOffline")}</span>
                </bs.Alert>
            )}
            {!refusal && <ListingElement
                callback={(item) => (
                    <GameCard
                        key={item.reference}
                        rom={item}
                        t={t}
                        onClick={() => openGameDetail(item)}
                    />
                )}
                items={shown.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)}
                itemCount={shown.length}
                itemsPerPage={ITEMS_PER_PAGE}
                isLoading={isLoading}
                currentPage={currentPage}
                onPositionChange={setCurrentPage}
                search={{
                    context: searchContext,
                    placeholder: t("retrogaming.searchPlaceholder"),
                    // LE PANNEAU DE FILTRES REVIENT POUR LE STATUT (chantier
                    // 30), et seulement pour lui : la source se choisit
                    // toujours au sélecteur au-dessus (2026-09-18).
                    filters: {
                        Component: () => <StatusFilter context={searchContext} t={t} />,
                    },
                }}
                texts={{
                    itemsFound: t("retrogaming.gamesFound"),
                    noItemsFound: t("retrogaming.noGames"),
                }}
            />}
            {/* AU-DESSUS DE LA FICHE QUI SE REFERME, DÈS LE CLIC.
                Les panneaux de Skeleton vivent à la racine du document, à
                partir de 3010 (`nextOverlayZIndex`), et mettent 500 ms à
                sortir de l'écran. À 1050 dans la page, l'écran des étapes
                passait dessous : sur un téléphone, où la fiche prend toute la
                largeur, on voyait encore la fiche et son « ••• » au lieu des
                étapes (signalé le 2026-10-10). D'où le portail, à côté des
                panneaux, et 3400 : au-dessus d'eux, sous les toasts (3500)
                qui doivent rester lisibles par-dessus. */}
            {launchScreen && createPortal(
                <LaunchScreen
                    t={t}
                    fixed
                    zIndex={3400}
                    label={launchScreen.failure ? t("streaming.launch.failed") : t("streaming.launch.title")}
                    steps={launchScreen.steps.length > 0 ? launchScreen.steps : null}
                    backdrop={launchScreen.backdrop}
                    failed={Boolean(launchScreen.failure)}
                    error={launchScreen.failure?.error || null}
                    onCancel={cancelLaunchScreen}
                    onReport={sendLaunchReport}
                    reportState={launchScreen.reportState}
                />,
                document.body,
            )}
        </Page>
    );
}
