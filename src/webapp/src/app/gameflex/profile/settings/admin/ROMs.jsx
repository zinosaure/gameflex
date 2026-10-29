import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    DownloadRing,
} from "../../../shared/templates/Essentials.jsx";
import { useConsoles } from "../../../contexts/ConsolesContext.jsx";
import { useAuth } from "../../auth/AuthContext.jsx";
import {
    activeConsoleIds,
    fetchRoms,
    removeRom,
    formatSize,
} from "./api/library.js";
import {
    parseSource,
    importFromSource,
    fetchMountRoms,
    importFromMount,
} from "./api/storage-admin.js";
import { listRemoteMounts } from "../../api.js";
import SourceSelector, { useDiskSources } from "./storage/SourceSelector.jsx";
import { ConsoleStatusPanel, GameStatusBadge, GameStatusPanel, StatusFilter, matchesStatuses } from "../../../games/GameStatus.jsx";
import { federatedServers } from "../../../libs/federatedHost.js";
import { can } from "../../../libs/capabilities.js";
import { publishDownloadProgress } from "../../../libs/deviceStream.js";
import { hasLocalLibrary, onLocalLibraryReloaded } from "../../../libs/localLibrary.js";

const ITEMS_PER_PAGE = 10;

export default function ROMs({ context, token, t }) {
    // La console de départ : la PREMIÈRE que cet hôte connaît, pas « nes »
    // écrit en dur. Un serveur qui n'a pas de NES affichait sinon une liste
    // vide en s'ouvrant, comme si sa bibliothèque l'était.
    const [values, setValues] = useState({ q: "", consoles: [], gameSize: 0, statuses: [] });

    const [items, setItems] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [pending, setPending] = useState(null);

    // LA SOURCE, sous forme de descripteur (`local` | `peer:<id>` |
    // `mount:<id>`), comme les écrans des cœurs et des BIOS — une seule
    // grammaire pour les trois.
    //
    // Les montages sont là depuis #157 : la matrice #154 les autorise pour les
    // ROMs, et le serveur sait maintenant les rapatrier
    // (`POST /api/v1/mounts/{id}/import`). Ils ne fournissent QUE des jeux —
    // un partage Samba, un dossier WebDAV ou un kDrive ne porte ni cœur ni
    // BIOS, d'où leur absence des deux autres écrans.
    const [source, setSource] = useState("local");
    const [peers, setPeers] = useState([]);
    const [mounts, setMounts] = useState([]);
    // Les disques internes de ce serveur (chantier 23) : leurs jeux sont les
    // nôtres — la corbeille, pas le rapatriement.
    const diskSources = useDiskSources(t);
    // Ce que NOTRE bibliothèque porte déjà, en clés « console/UUID ». `null`
    // quand on regarde notre propre disque : la question ne se pose pas.
    //
    // PAR L'UUID, PLUS PAR LE TITRE (chantier 14) : l'identité d'un jeu est
    // sa console et son nom sur le disque, extension comprise — la même chez
    // un pair, sur un partage et ici. Un nom écrit autrement est un autre jeu.
    const [here, setHere] = useState(null);
    // L'UUID d'une ligne : la référence d'un pair EN EST un ; un partage, qui
    // désigne ses jeux par leur chemin, le donne à côté (`id`).
    const idOf = (item) => item.id || item.reference;

    const { isAdmin } = useAuth();

    useEffect(() => {
        let cancelled = false;
        federatedServers()
            .then((found) => {
                if (!cancelled) {
                    setPeers(found || []);
                }
            })
            // Pas de fédération n'est pas une panne : le choix ne s'affiche pas.
            .catch(() => undefined);

        // Les partages enregistrés, même raison : aucun n'est un cas normal.
        listRemoteMounts(token)
            .then((answer) => {
                if (!cancelled && answer.ok) {
                    setMounts(answer.item || answer.data || []);
                }
            })
            .catch(() => undefined);

        return () => {
            cancelled = true;
        };
    }, [token]);

    // Le filtre ne propose que les consoles ACTIVES — celles pour lesquelles
    // cet hôte a des jeux. Le registre en connaît une quarantaine ; les
    // proposer toutes revient à ne pas filtrer (demandé le 2026-09-01).
    //
    // UNE SOURCE DISTANTE A SES PROPRES CONSOLES (chantier 14) : les jeux d'un
    // pair ou d'un partage ne comptent plus parmi ceux d'ici, et c'est par cet
    // écran qu'on les ajoute — une console qui n'existe que chez eux doit s'y
    // lire. Elle se lit donc dans tout le registre, et le filtre propose celles
    // où la source a des jeux.
    const registry = useConsoles();
    const [activeIds, setActiveIds] = useState([]);
    const [sourceIds, setSourceIds] = useState([]);
    const fromElsewhere = parseSource(source).kind !== "local";
    const consoles = useMemo(
        () => registry.filter((console) => (fromElsewhere ? sourceIds : activeIds).includes(console.id)),
        [registry, activeIds, sourceIds, fromElsewhere],
    );
    const allIds = useMemo(() => registry.map((console) => console.id), [registry]);
    const nameOf = (consoleId) =>
        registry.find((console) => console.id === consoleId)?.name || consoleId;

    // RELUES QUAND LA BIBLIOTHÈQUE DE L'APPAREIL L'EST : un téléchargement ou
    // un parcours (« Rescan all ») peut faire entrer une console nouvelle.
    const [libraryEpoch, setLibraryEpoch] = useState(0);

    useEffect(() => {
        if (!hasLocalLibrary()) {
            return undefined;
        }

        return onLocalLibraryReloaded(() => setLibraryEpoch((epoch) => epoch + 1));
    }, []);

    useEffect(() => {
        let cancelled = false;
        activeConsoleIds().then((found) => {
            if (cancelled) {
                return;
            }
            setActiveIds(found);
        }).catch((failure) => {
            console.error("Lecture des consoles actives", failure);
        });

        return () => {
            cancelled = true;
        };
    }, [libraryEpoch]);

    // RIEN DE COCHÉ VEUT DIRE TOUTES, pas aucune. Le catalogue se lit console
    // par console (une requête chacune), donc il faut bien une liste ici —
    // mais c'est un détail de la lecture, pas ce que le filtre raconte.
    const wanted = values.consoles.length > 0 ? values.consoles : (fromElsewhere ? allIds : activeIds);
    const selected = wanted.join(",");

    // CHEZ UN PAIR, ON CHARGE AUSSI NOTRE PROPRE LISTE.
    //
    // Sans elle, impossible de dire quel jeu on possède déjà — et proposer de
    // rapatrier un jeu qu'on a déjà est le meilleur moyen de remplir un disque
    // pour rien. Même croisement que pour les cœurs et les BIOS ; la
    // différence est qu'ici la liste AFFICHÉE est celle de la source, puisque
    // personne ne déclare d'avance la liste des jeux qui existent.
    const reload = useCallback(() => {
        if (wanted.length === 0) {
            setItems([]);
            setHere(null);
            setLoading(false);

            return Promise.resolve();
        }

        const { kind, id } = parseSource(source);
        setLoading(true);

        // Un montage se lit par son listing, un pair par son catalogue, et
        // notre disque par le nôtre — trois routes, une seule liste à l'écran.
        const listing = kind === "mount"
            ? fetchMountRoms(id, wanted, values.q)
            : fetchRoms(wanted, values.q, kind === "peer" ? id : null, kind === "disk" ? id : null);
        // UN DISQUE INTERNE EST À NOUS (chantier 23) : rien à croiser avec la
        // bibliothèque, il ne s'y rapatrie pas.
        const own = kind === "local" || kind === "disk";

        return Promise.all([
            listing,
            own ? null : fetchRoms(wanted, values.q, null),
        ]).then(([found, mine]) => {
            setItems(found);
            if (kind !== "local" && values.consoles.length === 0) {
                setSourceIds([...new Set(found.map((item) => item.consoleId))]);
            }
            setHere(mine
                ? new Set(mine.map((rom) => `${rom.consoleId}/${rom.reference}`))
                : null);
            setPage(1);
        }).catch((failure) => {
            console.error("Lecture du catalogue des jeux", failure);
        }).finally(() => {
            setLoading(false);
        });
    }, [selected, values.q, source]);

    useEffect(() => {
        reload();
    }, [reload]);

    const handleDelete = async (item) => {
        try {
            const error = await removeRom(item);

            context.toast.open(
                error ? t("admin.library.removeFailed") : t("admin.library.removed", { name: item.title }),
                { variant: error ? "error" : "success" },
            );

            // LA LIGNE PART, ET RIEN D'AUTRE NE BOUGE. Relire coûtait une
            // requête par console et ramenait en page 1 : on perdait le fil
            // (relevé par le porteur le 2026-10-10).
            if (!error) {
                setItems((current) => current.filter((entry) => (
                    entry.consoleId !== item.consoleId || entry.reference !== item.reference
                )));
            }
        } catch (failure) {
            console.error("Suppression d'un jeu", failure);
            context.toast.open(t("admin.library.removeFailed"), { variant: "error" });
        }
    };

    // RAPATRIER un jeu dans notre bibliothèque.
    //
    // C'est le serveur qui va le chercher, et qui le RANGE : il dérive le nom
    // du fichier du titre, prend l'extension sur le seul fichier quand le jeu
    // n'en a qu'un, et déplie l'archive en dossier quand il en a plusieurs.
    // La page n'a donc rien à décider — elle transmet ce que le catalogue du
    // pair a annoncé.
    const handleImport = async (item) => {
        const { kind, id } = parseSource(source);
        const key = `${item.consoleId}/${item.reference}`;

        setPending(key);
        // Un jeu pèse parfois un gigaoctet : on n'attend pas devant cet écran.
        // La cloche le porte donc aussi, et l'anneau y tourne — c'est le
        // serveur qui transfère, la page ne compte aucun octet.
        publishDownloadProgress({ label: item.title, done: 0, total: 0, percent: -1 });
        // Un montage se désigne par un CHEMIN, un pair par une RÉFÉRENCE de
        // catalogue : deux charges utiles, parce que ce sont deux façons de
        // retrouver un jeu — et c'est le serveur qui, dans les deux cas,
        // décide de sa forme.
        try {
            const result = kind === "mount"
                ? await importFromMount(id, item.consoleId, item.reference)
                : await importFromSource(source, "roms", {
                    consoleId: item.consoleId,
                    reference: item.reference,
                    title: item.title,
                    files: item.files || [],
                    folder: Boolean(item.folder),
                });

            context.toast.open(
                result.ok
                    ? t("admin.storage.downloaded", { name: item.title })
                    : (result.errorMessage || t("admin.storage.notAtSource", { name: item.title })),
                { variant: result.ok ? "success" : "error" },
            );

            // LA LIGNE PREND SA COCHE, SUR PLACE. Relire était le pire des
            // cas : le catalogue d'un pair entier (10 000 jeux et plus), plus
            // une requête par console d'ici, et un retour en page 1 (relevé par
            // le porteur le 2026-10-10). Le jeu se range sous le nom qu'il a
            // là-bas — la règle du rapatriement —, donc sous le même UUID.
            if (result.ok) {
                setHere((current) => new Set(current || []).add(`${item.consoleId}/${idOf(item)}`));
            }
        } catch (failure) {
            console.error("Rapatriement d'un jeu", failure);
            context.toast.open(t("app.error"), { variant: "error" });
        } finally {
            // La cloche est GLOBALE : sans cette remise à zéro, un rejet la
            // laisse tourner sur tous les écrans jusqu'au rechargement.
            publishDownloadProgress(null);
            setPending(null);
        }
    };

    // Le filtre de taille s'applique ICI et non à la source : c'est un
    // curseur qu'on bouge, et refaire une requête à chaque pixel ferait
    // clignoter la liste pour rien.
    // Le statut aussi (chantier 30) : il est déjà sur chaque ligne.
    const filtered = useMemo(() => {
        const floor = (values.gameSize || 0) * 1024 * 1024;

        return items.filter((item) => (floor <= 0 || item.sizeBytes >= floor) && matchesStatuses(item, values.statuses));
    }, [items, values.gameSize, values.statuses]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    // Le dernier jeu d'une page supprimé : on recule d'une page plutôt que de
    // montrer une page vide — et surtout pas de retour en page 1.
    useEffect(() => {
        const last = Math.max(1, Math.ceil(filtered.length / ITEMS_PER_PAGE));
        if (page > last) {
            setPage(last);
        }
    }, [filtered.length, page]);

    const searchContext = { values, setValues };
    const sourceKind = parseSource(source).kind;

    // SUPPRIMER EST OUVERT À L'ADMINISTRATEUR depuis le 2026-09-17, et la
    // raison compte : le rapatriement l'a rendu réversible. On sait reprendre
    // un jeu chez un pair, et il se range tout seul au bon endroit — effacer
    // pour faire de la place redevient donc un geste normal.
    //
    // L'ADMINISTRATEUR, sur les deux hôtes : sur un téléphone c'est d'ordinaire
    // son propriétaire, et la page de l'appareil l'est. Un rôle
    // `user` n'efface jamais — un membre d'un téléphone non plus, alors que
    // `canRemove()` le lui proposait : la machine est à son propriétaire, pas
    // à qui la regarde (2026-10-08).
    const mayDelete = isAdmin;

    // DEUX CONFIRMATIONS, ET CE N'EST PAS UNE COQUETTERIE. La première prévient
    // que le fichier quitte le disque ; la seconde fait NOMMER le jeu qu'on
    // efface, pour qu'on ne puisse pas y arriver par deux clics d'habitude. Une
    // ROM supprimée ne se retrouve pas — sauf à la rapatrier d'un pair, ce qui
    // suppose qu'un pair l'ait.
    //
    // Enchaîner est sûr : `modal.open` fabrique un élément ET une instance
    // NEUFS à chaque appel (`skeleton.js`), et le `hide()` qui suit `onConfirm`
    // ne ferme que la première — la seconde a son propre z-index et devient la
    // survivante de la pile.
    const askDelete = (item) => {
        context.modal.open(
            <div className="text-center">
                {t("admin.library.removeConfirm")} <br />
                <strong>{item.title}</strong><br />
                <span className="text-muted small">{t("admin.library.removeIrreversible")}</span>
            </div>,
            {
                variant: "confirm",
                onConfirm: () => context.modal.open(
                    <div className="text-center">
                        {t("admin.library.removeAssume", { title: item.title })}
                    </div>,
                    {
                        variant: "confirm",
                        onConfirm: () => handleDelete(item),
                        confirmLabel: t("app.delete"),
                        cancelLabel: t("app.cancel"),
                    },
                ),
                confirmLabel: t("admin.library.removeContinue"),
                cancelLabel: t("app.cancel"),
            },
        );
    };

    // CE QUE LA LIGNE OFFRE DÉPEND DE LA SOURCE.
    //
    //   local   → la corbeille, et rien d'autre : c'est notre disque.
    //   un pair → le rapatriement, sauf si on l'a déjà — auquel cas la coche
    //             le dit, sans bouton, puisqu'il n'y a rien à faire.
    const actionFor = (item) => {
        if (sourceKind === "local" || sourceKind === "disk") {
            return mayDelete ? (
                <bs.Button
                    variant="danger"
                    title={t("app.delete")}
                    onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        askDelete(item);
                    }}
                >
                    <i className="bi bi-trash"></i>
                </bs.Button>
            ) : null;
        }

        // L'anneau prend la place du bouton pour que la ligne ne bouge pas, et
        // `percent: null` le fait TOURNER : c'est le serveur qui transfère, la
        // page ne compte aucun octet.
        if (pending === `${item.consoleId}/${item.reference}`) {
            return <DownloadRing percent={null} label={t("admin.storage.download")} />;
        }

        if (here && here.has(`${item.consoleId}/${idOf(item)}`)) {
            return <i className="bi bi-check-circle-fill text-success fs-4"
                title={t("admin.storage.installed")}></i>;
        }

        return (
            <bs.Button
                variant="primary"
                disabled={pending !== null}
                title={t("admin.storage.download")}
                onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    handleImport(item);
                }}
            >
                <i className="bi bi-download"></i>
            </bs.Button>
        );
    };

    const sourceOptions = [
        { value: "local", label: t("admin.storage.sourceHere") },
        ...diskSources,
        ...peers.map((peer) => ({
            value: `peer:${peer.id}`,
            label: peer.hostname || peer.link,
        })),
        ...mounts.map((mount) => ({
            value: `mount:${mount.id}`,
            label: mount.name || mount.kind,
        })),
    ];

    return (
        <SectionPanel>
            {/* Le sélecteur SE VOIT TOUJOURS, même sans pair : il ne sert pas
                qu'à choisir, il dit aussi d'où vient ce qu'on regarde. Et sa
                remise en cache — celle d'un listage — est à côté de lui. */}
            <SourceSelector
                t={t}
                value={source}
                onChange={setSource}
                options={sourceOptions}
                onRefreshed={reload}
                section="roms"
                helpText={t("admin.library.sourceHelp")} />
            {/* LE STATUT DE TOUTE UNE CONSOLE (chantier 30), pour nos jeux —
                ceux de la bibliothèque comme ceux d'un disque interne, qui ont
                le leur dans le dossier de ce serveur. Ceux d'un ami ont le
                sien, établi chez lui. */}
            {isAdmin && can("gameStatus") && (sourceKind === "local" || sourceKind === "disk") && consoles.length > 0 && (
                <ListElement
                    light={true}
                    icon="bi bi-collection"
                    label={t("gameStatus.consoleTitle")}
                    helpText={t("gameStatus.consoleOpenHelp")}
                    onClick={() => context.offcanvas.open(
                        <ConsoleStatusPanel consoles={consoles} t={t} context={context} onChanged={reload} />,
                        { size: 600 },
                    )}
                />
            )}
            <ListingElement
                callback={(item) => (
                    <ListElement
                        key={`${item.consoleId}::${item.reference}`}
                        icon="bi bi-disc"
                        label={<>{item.title}<GameStatusBadge status={item.status} t={t} /></>}
                        helpText={`${nameOf(item.consoleId)} · ${formatSize(item.sizeBytes)}`}
                        // LE STATUT D'UN JEU D'ICI se règle en touchant sa
                        // ligne ; la corbeille reste à droite.
                        onClick={isAdmin && can("gameStatus") && (sourceKind === "local" || sourceKind === "disk")
                            ? () => context.offcanvas.open(
                                <GameStatusPanel item={item} t={t} context={context} onChanged={reload} />,
                                { size: 600 },
                            )
                            : null}
                        next={actionFor(item)}
                    />
                )}
                items={visible}
                itemCount={filtered.length}
                itemsPerPage={ITEMS_PER_PAGE}
                currentPage={page}
                onPositionChange={setPage}
                isLoading={isLoading}
                search={{
                    context: searchContext,
                    placeholder: t("admin.library.searchGame"),
                    filters: {
                        Component: () => (
                            <div>
                                <Skeleton.Form.Group
                                    label={t("admin.library.filterConsole")}
                                    helpText={t("admin.library.filterConsoleHelp")}
                                >
                                    <Skeleton.Form.Multicheckbox
                                        name="consoles"
                                        context={searchContext}
                                        options={consoles.map((console) => ({
                                            value: console.id,
                                            label: console.name,
                                        }))} />
                                </Skeleton.Form.Group>
                                <StatusFilter context={searchContext} t={t} />
                                <Skeleton.Form.Group
                                    label={t("admin.library.filterSize")}
                                    helpText={t("admin.library.filterSizeHelp")}
                                >
                                    <Skeleton.Form.Range
                                        name="gameSize"
                                        context={searchContext}
                                        min={0}
                                        max={1000}
                                        step={10} />
                                </Skeleton.Form.Group>
                            </div>
                        ),
                    },
                }}
                texts={{
                    itemsFound: t("admin.library.gamesFound"),
                    noItemsFound: t("admin.library.noGame"),
                }}
            />
        </SectionPanel>
    );
}
