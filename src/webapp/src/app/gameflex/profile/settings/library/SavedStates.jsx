// MES SAUVEGARDES, PAR SOURCE (chantier 38, le porteur, 2026-10-28 et 29) :
// « on peut faire comme pour les jeux/émulateurs/bios. On choisit la source,
// local et les autres serveurs fédérés. »
//
//   Sur cet appareil — le PRINCIPAL, ce que les parties jouent : Backup,
//     Envoyer vers… ; puis les COPIES (les backups, et une par serveur
//     fédéré), d'où l'on Restaure ;
//   un serveur fédéré — SES sauvegardes, sous le compte qu'on y a :
//     Télécharger.
//
// Rien de ce qui voyage n'écrase une sauvegarde : un envoi arrive dans une
// copie de l'autre côté, un téléchargement dans une copie d'ici. « Ce serait
// donc à l'utilisateur ensuite de choisir de faire la synchronisation entre
// le dossier principal et les sources » : c'est Restaurer, et ce que le
// principal avait part d'abord dans les backups.
//
// Le panneau « Synchroniser avec… » (SaveSyncPanel), qui écrivait dans le
// principal d'un serveur fédéré emplacement par emplacement, est remplacé :
// il est supprimé.
import React, { useState, useEffect, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { useConsoles } from "../../../contexts/ConsolesContext.jsx";
import {
    listUserSaves,
    deleteGameSaves,
    listSaveSources,
    listStreamexSaves,
} from "../../../api/retrogaming.js";
import { SaveStatesPanel } from "../../../stream/onscreen/menu/SaveStatesPanel.jsx";
import { federatedServers } from "../../../libs/federatedHost.js";
import { useFederationSession } from "../../../libs/federationSession.js";
import { FederatedAccount, FederatedRefusal, FederatedTokenForm } from "../../../shared/FederatedAccountPanel.jsx";
import SourceSelector from "../admin/storage/SourceSelector.jsx";
import { SaveActions } from "./SaveActions.jsx";
import { StreamexSaves } from "./StreamexSaves.jsx";
import {
    SectionPanel,
    ListElement,
    OffCanvas,
    ListingElement,
    AsyncSectionPanel,
} from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;

// TOUS LES EMPLACEMENTS D'UN JEU, d'un geste : la ligne du haut de sa fiche.
function WholeGame({ where, peers, selfName, slots, title, t, context, onDone }) {
    const [isOpen, setOpen] = useState(false);

    return (
        <ListElement
            light={true}
            icon="bi bi-collection"
            label={t("profile.saveStates.wholeGame")}
            helpText={t("profile.saveStates.slotCount", { count: slots.length })}
            onClick={() => setOpen(true)}
        >
            {isOpen ? (
                <div className="pt-2 mt-2 border-top">
                    <SaveActions
                        where={where}
                        peers={peers}
                        selfName={selfName}
                        slots={slots}
                        label={title}
                        canDelete={where.kind === "copy"}
                        t={t}
                        context={context}
                        onDone={onDone} />
                </div>
            ) : null}
        </ListElement>
    );
}

/**
 * Les sauvegardes d'un rangement — `where` : le principal, une copie, ou un
 * serveur fédéré (`{kind, source, host, server}`). Le rétro par jeu, les
 * consoles récentes par console. `paging` — la page dans l'adresse (la liste
 * de la page) ; sans lui, la sienne (une copie, dans sa fiche).
 */
function SaveList({ where, peers, selfName, user, t, context, epoch = 0, onChanged = null, paging = null }) {
    const consoles = useConsoles();
    const [isLoading, setLoading] = useState(true);
    const [slots, setSlots] = useState([]);
    const [streamex, setStreamex] = useState([]);
    // Le filtre par console vit dans le même objet que la recherche.
    const [formValues, setFormValues] = useState({ q: "", consoles: [] });
    const searchContext = {
        values: formValues,
        setValues: (values) => setFormValues(values),
    };
    const query = formValues.q || "";
    const [ownPage, setOwnPage] = useState(1);
    const currentPage = paging ? paging.page : ownPage;
    const setCurrentPage = paging ? paging.setPage : setOwnPage;
    // Le nom d'une console, ou son identifiant quand le registre ne la connaît
    // pas — un seul endroit.
    const nameOf = (consoleId) =>
        consoles.find((entry) => entry.id === consoleId)?.name || consoleId;

    const reload = () => {
        setLoading(true);
        Promise.all([listUserSaves(where.host, where.source), listStreamexSaves(where.source, where.host)])
            .then(([retro, recent]) => {
                setSlots(retro.ok ? (retro.data || []) : []);
                setStreamex(recent.ok ? (recent.data?.games || []) : []);
            })
            .catch((failure) => {
                console.error("Chargement des sauvegardes", failure);
            })
            .finally(() => setLoading(false));
    };
    useEffect(reload, [epoch, where.kind, where.source, where.host]);

    const changed = () => {
        reload();
        if (onChanged) {
            onChanged();
        }
    };

    // UNE LIGNE PAR JEU du rétro (`savestate/user` en rend une par
    // emplacement), UNE PAR CONSOLE récente. Triées par console seulement
    // (l'utilisateur, 2026-09-18) : le `sort` de JS est stable, l'ordre de
    // l'API tient à l'intérieur d'une console.
    const items = useMemo(() => {
        const byGame = new Map();
        for (const slot of slots) {
            const key = `${slot.consoleId}::${slot.reference}`;
            const existing = byGame.get(key);
            if (existing) {
                existing.slots.push(slot);
            } else {
                byGame.set(key, {
                    kind: "retro",
                    key,
                    consoleId: slot.consoleId,
                    reference: slot.reference,
                    title: slot.title,
                    slots: [slot],
                });
            }
        }
        const byConsole = new Map();
        for (const game of streamex) {
            const existing = byConsole.get(game.console);
            if (existing) {
                existing.games.push(game);
            } else {
                byConsole.set(game.console, {
                    kind: "streamex",
                    key: `streamex::${game.console}`,
                    consoleId: game.console,
                    title: nameOf(game.console),
                    games: [game],
                });
            }
        }

        return [...byGame.values(), ...byConsole.values()]
            .sort((a, b) => nameOf(a.consoleId).localeCompare(nameOf(b.consoleId)));
    }, [slots, streamex, consoles]);

    // Les consoles PRÉSENTES dans les sauvegardes : proposer les quarante du
    // registre pour six parties en cours ferait chercher.
    const savedConsoles = useMemo(() => {
        const seen = new Map();
        for (const item of items) {
            if (!seen.has(item.consoleId)) {
                seen.set(item.consoleId, nameOf(item.consoleId));
            }
        }

        return [...seen.entries()]
            .map(([value, label]) => ({ value, label }))
            .sort((a, b) => a.label.localeCompare(b.label));
    }, [items, consoles]);

    // Rien de coché = rien d'écarté. Un jeu d'une console récente se cherche
    // aussi par son titre.
    const picked = formValues.consoles || [];
    const needle = query.toLowerCase();
    const filtered = items.filter((item) =>
        (!needle || item.title.toLowerCase().includes(needle)
            || (item.games || []).some((game) => `${game.title || ""} ${game.id}`.toLowerCase().includes(needle)))
        && (picked.length === 0 || picked.includes(item.consoleId)));

    // Une recherche qui rétrécit la liste peut laisser la page courante après
    // sa fin : on revient à la première.
    useEffect(() => {
        if (currentPage > 1 && (currentPage - 1) * ITEMS_PER_PAGE >= filtered.length) {
            setCurrentPage(1);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [query, picked.join(",")]);

    const pageItems = filtered.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE);

    // savedOnesOnly — pas d'émulateur à qui sauver ou charger hors d'une
    // partie : les emplacements remplis, et sous celui qu'on touche, les
    // gestes de ce rangement.
    const openGame = (game) => {
        context.offcanvas.open(
            <OffCanvas
                icon="bi bi-lightning-charge"
                label={game.title}
                helpText={nameOf(game.consoleId)}
            >
                {game.slots.length > 1 ? (
                    <SectionPanel>
                        <WholeGame
                            where={where}
                            peers={peers}
                            selfName={selfName}
                            slots={game.slots.map((slot) => ({ ...slot, title: game.title }))}
                            title={game.title}
                            t={t}
                            context={context}
                            onDone={changed} />
                    </SectionPanel>
                ) : null}
                <SaveStatesPanel
                    session={{ consoleId: game.consoleId, reference: game.reference }}
                    username={user?.username}
                    context={context}
                    t={t}
                    savedOnesOnly={true}
                    source={where.source}
                    host={where.host}
                    slotActions={(slot, info, refresh) => (
                        <SaveActions
                            where={where}
                            peers={peers}
                            selfName={selfName}
                            slots={[{
                                consoleId: game.consoleId,
                                reference: game.reference,
                                slot,
                                title: game.title,
                                timestamp: info?.timestamp,
                            }]}
                            label={`${game.title} #${slot + 1}`}
                            canDelete={where.kind !== "peer"}
                            t={t}
                            context={context}
                            onDone={() => {
                                refresh();
                                changed();
                            }} />
                    )}
                />
            </OffCanvas>,
            { size: 600 },
        );
    };

    const openConsole = (item) => {
        context.offcanvas.open(
            <OffCanvas
                icon="bi bi-controller"
                label={item.title}
                helpText={t("profile.saveStates.consoleHelp")}
            >
                <StreamexSaves
                    where={where}
                    consoleId={item.consoleId}
                    consoleName={item.title}
                    peers={peers}
                    selfName={selfName}
                    t={t}
                    context={context}
                    onChanged={changed} />
            </OffCanvas>,
            { size: 600 },
        );
    };

    // LES DEUX APPELS, ET PAS L'UN OU L'AUTRE : la carte porte le `onClick`
    // de la ligne et ce bouton en est un enfant — `stopPropagation` empêche la
    // fiche de s'ouvrir par-dessus la modale, `preventDefault` l'action du
    // bouton.
    const handleDeleteGame = (event, game) => {
        event.preventDefault();
        event.stopPropagation();
        context.modal.open(t("profile.saveStates.deleteGameConfirm", { title: game.title }), {
            variant: "confirm",
            title: t("profile.saveStates.deleteGame"),
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                const result = await deleteGameSaves(game.consoleId, game.reference, where.source);
                if (result.ok) {
                    // Ses emplacements partent, SUR PLACE (#295).
                    setSlots((current) => current.filter(
                        (slot) => !(slot.consoleId === game.consoleId && slot.reference === game.reference)));
                } else {
                    context.toast.open(t("profile.saveStates.deleteFailed"), { variant: "error" });
                }
            },
        });
    };

    return (
        <ListingElement
            callback={(item) => (item.kind === "streamex" ? (
                <ListElement
                    key={item.key}
                    icon="bi bi-controller"
                    label={item.title}
                    helpText={t("profile.saveStates.gameCount", { count: item.games.length })}
                    onClick={() => openConsole(item)}
                />
            ) : (
                <ListElement
                    key={item.key}
                    icon="bi bi-shield-check"
                    label={item.title}
                    // La console d'abord : trier par console sans la montrer
                    // rendrait l'ordre illisible.
                    helpText={`${nameOf(item.consoleId)} · ${t("profile.saveStates.slotCount", { count: item.slots.length })}`}
                    onClick={() => openGame(item)}
                    next={where.kind === "peer" ? null : (
                        <bs.Button
                            variant="danger"
                            title={t("profile.saveStates.deleteGame")}
                            onClick={(e) => handleDeleteGame(e, item)}
                        >
                            <i className="bi bi-trash"></i>
                        </bs.Button>
                    )}
                />
            ))}
            items={pageItems}
            itemCount={filtered.length}
            itemsPerPage={ITEMS_PER_PAGE}
            currentPage={currentPage}
            onPositionChange={setCurrentPage}
            isLoading={isLoading}
            search={{
                context: searchContext,
                placeholder: t("profile.saveStates.search"),
                filters: {
                    Component: () => (
                        <div>
                            <Form.Group
                                label={t("admin.library.filterConsole")}
                                helpText={t("profile.saveStates.filterConsoleHelp")}
                            >
                                <Form.Multicheckbox
                                    name="consoles"
                                    context={searchContext}
                                    options={savedConsoles} />
                            </Form.Group>
                        </div>
                    )
                }
            }}
            texts={{
                itemsFound: t("profile.saveStates.found"),
                noItemsFound: t("profile.saveStates.empty"),
            }}
        />
    );
}

export default function SavedStates({ context, user, t }) {
    const [searchParams, setSearchParams] = useSearchParams();
    // Null tant que la liste n'est pas arrivée : `?source=peer:3` ne doit pas
    // retomber sur le local au premier rendu.
    const [servers, setServers] = useState(null);
    // `self` — le nom que ce serveur annonce : celui de la copie qu'un envoi
    // crée chez l'autre. `sources` — nos copies.
    const [copies, setCopies] = useState({ self: "", sources: [] });
    // Relire le principal quand une restauration l'a changé depuis une copie.
    const [epoch, setEpoch] = useState(0);

    useEffect(() => {
        let cancelled = false;
        federatedServers().then((found) => {
            if (!cancelled) {
                // Les panneaux de compte lisent `name`, la route rend `hostname`.
                setServers((found || []).map((entry) => ({ ...entry, name: entry.hostname || entry.link })));
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    const refreshCopies = () => {
        listSaveSources().then((answer) => {
            if (answer.ok) {
                setCopies({ self: answer.data?.self || "", sources: answer.data?.sources || [] });
            }
        });
    };
    useEffect(refreshCopies, []);

    // DANS L'ADRESSE, comme la page des jeux.
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
    const currentPage = parseInt(searchParams.get("page"), 10) || 1;
    const setCurrentPage = (page) => {
        setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            if (page > 1) next.set("page", String(page));
            else next.delete("page");
            return next;
        });
    };

    // La même grammaire que la page des jeux : `local` | `peer:<id>`.
    const peers = servers || [];
    const sourceOptions = useMemo(() => [
        { value: "local", label: t("retrogaming.origin.local") },
        ...peers
            .map((server) => ({ value: `peer:${server.id}`, label: server.name }))
            .sort((a, b) => a.label.localeCompare(b.label)),
    ], [servers, t]);
    const selectedSource = servers === null || sourceOptions.some((option) => option.value === requestedSource)
        ? requestedSource
        : "local";
    const peer = selectedSource.startsWith("peer:")
        ? peers.find((server) => `peer:${server.id}` === selectedSource) || null
        : null;

    // SES SAUVEGARDES SONT CELLES DE NOTRE COMPTE CHEZ LUI (chantier 20) : sans
    // le jeton de ce compte — ou s'il le refuse —, le formulaire pour le
    // coller, une fois.
    const { session: account, code: refusal, isLoading: isOpening } = useFederationSession(peer?.id);
    const host = useMemo(
        () => (peer && account ? { origin: peer.link, token: account.token } : null),
        [peer, account],
    );

    const changed = () => {
        setEpoch((value) => value + 1);
        refreshCopies();
    };

    const openCopy = (name) => {
        const label = name === "local" ? t("profile.saveStates.backups") : name;
        context.offcanvas.open(
            <OffCanvas
                icon={name === "local" ? "bi bi-archive" : "bi bi-hdd-network"}
                label={label}
                helpText={name === "local" ? t("profile.saveStates.backupsHelp") : t("profile.saveStates.copyFromHelp", { name })}
            >
                <SaveList
                    where={{ kind: "copy", source: name, host: null }}
                    peers={peers}
                    selfName={copies.self}
                    user={user}
                    t={t}
                    context={context}
                    onChanged={changed} />
            </OffCanvas>,
            { size: 600 },
        );
    };

    const peerView = () => {
        if (account) {
            return (
                <>
                    <FederatedAccount server={peer} account={account} t={t} />
                    {/* SON PROPRE PANNEAU : l'écart entre deux panneaux est
                        déjà écrit une fois (app.css) ; sans lui, « Oublier mon
                        jeton » touchait la recherche. */}
                    <SectionPanel>
                        <SaveList
                            where={{ kind: "peer", source: "", host, server: peer }}
                            peers={peers}
                            selfName={copies.self}
                            user={user}
                            t={t}
                            context={context}
                            onChanged={changed}
                            paging={{ page: currentPage, setPage: setCurrentPage }} />
                    </SectionPanel>
                </>
            );
        }
        if (isOpening) {
            return <AsyncSectionPanel isLoading={true} />;
        }

        return refusal === "noFederatedAccount" || refusal === "accountRefused"
            ? <FederatedTokenForm server={peer} refused={refusal === "accountRefused"} t={t} context={context} />
            : <FederatedRefusal server={peer} code={refusal} t={t} />;
    };

    return (
        <>
            <SectionPanel>
                <SourceSelector
                    t={t}
                    value={selectedSource}
                    onChange={setSource}
                    options={sourceOptions}
                    canRefresh={false}
                    label={t("profile.saveStates.sourceLabel")}
                    helpText={t("profile.saveStates.sourceHelp")} />
                {peer ? peerView() : (
                    <SaveList
                        where={{ kind: "principal", source: "", host: null }}
                        peers={peers}
                        selfName={copies.self}
                        user={user}
                        t={t}
                        context={context}
                        epoch={epoch}
                        onChanged={refreshCopies}
                        paging={{ page: currentPage, setPage: setCurrentPage }} />
                )}
            </SectionPanel>
            {peer ? null : (
                <SectionPanel label={t("profile.saveStates.copies")} helpText={t("profile.saveStates.copiesHelp")}>
                    {copies.sources.length === 0
                        ? <div className="text-muted">{t("profile.saveStates.noCopies")}</div>
                        : copies.sources.map((name) => (
                            <ListElement
                                key={name}
                                icon={name === "local" ? "bi bi-archive" : "bi bi-hdd-network"}
                                label={name === "local" ? t("profile.saveStates.backups") : name}
                                helpText={name === "local"
                                    ? t("profile.saveStates.backupsHelp")
                                    : t("profile.saveStates.copyFromHelp", { name })}
                                onClick={() => openCopy(name)}
                            />
                        ))}
                </SectionPanel>
            )}
        </>
    );
}
