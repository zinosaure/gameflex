import React, { useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { hasLocalLibrary, onLocalLibraryReloaded } from "../libs/localLibrary.js";
import { federatedServerNow, federatedServers, parseFederatedId } from "../libs/federatedHost.js";
import { describeResources } from "../libs/hostResources.js";
import SourceSelector from "../profile/settings/admin/storage/SourceSelector.jsx";
import { capabilities } from "../libs/capabilities.js";
import { useConsoles } from "../contexts/ConsolesContext.jsx";
import { listRoms, listInternalDisks } from "../api/retrogaming.js";
import { Page, ListingElement } from "../shared/templates/Essentials.jsx";
import { GameCard } from "./GameCard.jsx";
import { GameDetailPanel } from "./GameDetailPanel.jsx";
import { StatusFilter, matchesStatuses } from "./GameStatus.jsx";
import { ITEMS_PER_PAGE, SEARCH_DEBOUNCE_MS } from "./helpers.js";

export default function Games() {
    const { t } = useTranslation();
    const location = useLocation();
    const context = useContext(Skeleton.context);
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


    // PLUS DE « JOUER » ICI (chantier 52, P8, décision du porteur du 2026-11-13) : la page de jeu est
    // celle du launcher, et la webapp n'en a plus. La fiche d'un jeu dit ce qu'il est, d'où il vient, et
    // le télécharge.

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
        />,
        { size: 600 },
    );

    // OUVRIR UN JEU DÉSIGNÉ PAR L'ADRESSE — `?game=<référence>`.
    //
    // Une adresse qui désigne un jeu le rouvre : on atterrit ICI, la page qui
    // tient la liste où il se trouve.
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
        </Page>
    );
}
