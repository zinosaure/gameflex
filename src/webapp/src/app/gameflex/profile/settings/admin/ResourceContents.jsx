import React, { useState, useEffect, useMemo } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { SectionPanel, ListElement, ListingElement } from "../../../shared/templates/Essentials.jsx";
import {
    mountListing,
    rescanRemoteMount,
    checkRemoteMount,
    mountFileUrl,
} from "../../api.js";
import { authHeaders } from "../../auth/AuthContext.jsx";
import { formatSize } from "./api/library.js";
import { DownloadRing } from "../../../shared/templates/Essentials.jsx";
import { readWithProgress } from "../../../shared/utils/read-with-progress.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../../libs/kernelChannel.js";
import { hasLocalLibrary } from "../../../libs/localLibrary.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 25;

/**
 * CE QU'IL Y A DANS UNE RESSOURCE DISTANTE, et de quoi en tirer un fichier.
 *
 * Trois gestes, ceux qu'on fait devant un partage : vérifier qu'il répond,
 * le relire quand il a changé, et prendre un élément à la main.
 *
 * Le dernier est le moins évident et le plus utile : jusqu'ici, un BIOS ou un
 * core chez quelqu'un d'autre ne se récupérait qu'à travers le jeu qui en
 * avait besoin. Ici on le prend directement (demandé le 2026-09-02).
 *
 * TOUT VIENT DU CACHE. Parcourir un drive coûte un aller-retour par dossier —
 * plus de deux minutes sur un vrai kDrive. Le parcours est donc un geste
 * explicite (« Relire »), qui tourne en fond ; l'affichage, lui, lit des
 * fichiers JSON et répond en quelques millisecondes.
 */
export default function ResourceContents({ item, token, context, t }) {
    const [summary, setSummary] = useState(null);
    const [isLoading, setLoading] = useState(true);
    const [isChecking, setChecking] = useState(false);
    // PAS D'ÉTAT DE SECTION : un montage ne porte que des ROMs (#153).
    //
    // Samba, WebDAV et kDrive ne peuvent fournir ni cœurs ni BIOS — ceux-là
    // ne viennent que d'un serveur fédéré. Le choix existait, et un kDrive
    // pouvait donc être parcouru pour des BIOS. La synchronisation des
    // sauvegardes, plus tard, ajoutera une section légitime : c'est le seul
    // motif d'en réintroduire une.
    const [consoleId, setConsoleId] = useState("");
    const [items, setItems] = useState([]);
    const [page, setPage] = useState(1);
    const [values, setValues] = useState({ q: "" });
    // L'avancement PAR ÉLÉMENT : plusieurs téléchargements peuvent courir en
    // même temps, et chacun doit parler pour lui.
    const [progress, setProgress] = useState({});

    // LE RÉSUMÉ EST POUSSÉ, jamais relu : le sujet `mount/<id>` du centre de
    // messages part à l'abonnement, puis au début et à la fin de chaque
    // parcours. « Relire » se voit donc tout de suite, et sa fin aussi, sans
    // les relectures toutes les 4 s qui duraient autant qu'un vrai drive.
    const channel = useKernelChannel(kernelReachable(token) ? kernelTarget(null, token) : null);
    useKernelTopic(channel, `mount/${item.id}`, null, (seen) => {
        setSummary(seen || {});
        setLoading(false);
    });

    // Les consoles que la ressource contient, triées par ce qu'elles ont —
    // celles qui portent le plus de jeux d'abord, c'est là qu'on va.
    const consoles = useMemo(() => {
        const found = (summary?.consoles || []).filter((entry) => entry.roms > 0);

        return [...found].sort((a, b) => b.roms - a.roms);
    }, [summary]);

    useEffect(() => {
        // La première console qui a quelque chose : ouvrir sur une liste vide
        // ferait croire que la ressource l'est.
        setConsoleId((current) => (
            consoles.some((entry) => entry.id === current) ? current : (consoles[0]?.id || "")
        ));
    }, [consoles]);

    useEffect(() => {
        if (!summary?.scanned || !consoleId) {
            setItems([]);

            return;
        }

        let cancelled = false;
        mountListing(token, item.id, { console: consoleId }).then((answer) => {
            if (!cancelled) {
                setItems(answer.item || answer.data || []);
                setPage(1);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [token, item.id, consoleId, summary?.scanned]);

    const check = async () => {
        setChecking(true);
        try {
            const answer = await checkRemoteMount(token, { id: item.id });
            const seen = answer.item || answer.data || {};
            context.toast.open(
                seen.reachable
                    ? t("admin.servers.checkFolders", {
                        shown: (seen.folders || []).join(", ") || "—",
                        files: seen.files || 0,
                    })
                    : seen.detail || t("app.error"),
                { variant: seen.reachable ? "success" : "error" },
            );
        } finally {
            setChecking(false);
        }
    };

    const rescan = async () => {
        const answer = await rescanRemoteMount(token, item.id);
        const started = answer.item || answer.data || {};
        context.toast.open(
            started.scanning ? t("admin.servers.rescanStarted") : t("app.error"),
            { variant: started.scanning ? "success" : "error" },
        );
    };

    /**
     * Récupérer un élément DANS LE NAVIGATEUR.
     *
     * Par fetch et non par un lien nu : la route exige le jeton, qu'un
     * `<a href>` ne sait pas porter. On passe donc par un blob — le même
     * détour que la fiche d'un jeu fait déjà, et pour la même raison.
     *
     * ET ON MONTRE L'AVANCEMENT. Le fichier vient d'un drive distant : la
     * requête peut mettre plusieurs secondes à seulement démarrer, et un
     * bouton qui ne change pas se lit comme une panne (signalé le
     * 2026-09-02). L'avancement est suivi PAR ÉLÉMENT — on peut en lancer
     * plusieurs, et chacun doit dire où il en est, pas le dernier cliqué.
     */
    const download = async (entry) => {
        // ON PART DE ZÉRO QUAND ON CONNAÎT LA TAILLE.
        //
        // Le listing la porte, donc l'anneau peut annoncer un pourcentage dès
        // le premier instant plutôt que de tourner en attendant le premier
        // morceau — et sur un petit fichier, ce premier morceau est parfois
        // le seul, si bien qu'on n'aurait jamais vu autre chose qu'une
        // rotation.
        setProgress((current) => ({
            ...current,
            [entry.path]: { percent: entry.sizeBytes > 0 ? 0 : null },
        }));
        try {
            const response = await fetch(mountFileUrl(item.id, entry.path), {
                headers: authHeaders(token),
            });
            if (!response.ok) {
                throw new Error(String(response.status));
            }

            // LA TAILLE VIENT DU LISTING. La réponse est diffusée en
            // morceaux et n'annonce donc pas de longueur — l'anneau aurait
            // tourné dans le vide alors qu'on connaît le poids du fichier
            // depuis le scan.
            const blob = await readWithProgress(response, ({ percent }) => {
                setProgress((current) => ({ ...current, [entry.path]: { percent } }));
            }, entry.sizeBytes);

            const url = URL.createObjectURL(blob);
            const link = document.createElement("a");
            link.href = url;
            link.download = entry.path.split("/").pop() || entry.name;
            document.body.appendChild(link);
            link.click();
            link.remove();
            URL.revokeObjectURL(url);
        } catch {
            context.toast.open(t("admin.servers.downloadFailed"), { variant: "error" });
        } finally {
            // Toujours, y compris après un échec : l'anneau ne doit pas
            // tourner indéfiniment sur un téléchargement qui n'aura pas lieu.
            setProgress((current) => {
                const next = { ...current };
                delete next[entry.path];

                return next;
            });
        }
    };

    const shown = useMemo(() => {
        const needle = values.q.trim().toLowerCase();

        return needle ? items.filter((entry) => entry.name.toLowerCase().includes(needle)) : items;
    }, [items, values.q]);

    return (
        <>
            <SectionPanel
                label={t("admin.servers.contentsTitle")}
                helpText={summary?.scanned
                    ? t("admin.servers.contentsScannedAt", {
                        at: new Date(summary.scannedAt).toLocaleString(),
                    })
                    : t("admin.servers.contentsNeverScanned")}>
                <div className="d-flex gap-2">
                    <bs.Button
                        variant="secondary"
                        onClick={check}
                        disabled={isChecking}
                        className="w-50">
                        {isChecking
                            ? <bs.Spinner animation="border" size="sm" />
                            : <><i className="bi bi-plug me-2"></i>{t("admin.servers.check")}</>}
                    </bs.Button>
                    <bs.Button
                        variant="primary"
                        onClick={rescan}
                        disabled={summary?.scanning}
                        className="w-50">
                        {summary?.scanning
                            ? <><bs.Spinner animation="border" size="sm" className="me-2" />
                                {t("admin.servers.rescanRunning")}</>
                            : <><i className="bi bi-arrow-clockwise me-2"></i>{t("admin.servers.rescan")}</>}
                    </bs.Button>
                </div>
                {summary?.error && (
                    <div className="text-danger small mt-2">{summary.error}</div>
                )}
            </SectionPanel>
            <SectionPanel>
                <ListingElement
                    callback={(entry) => (
                        <ListElement
                            key={entry.path}
                            // Toujours un disque : un montage ne porte que des
                            // ROMs (#153). Ces deux lignes choisissaient encore
                            // entre trois icônes selon `section`, état que sa
                            // suppression avait laissé sans définition — un
                            // ReferenceError au premier rendu, que webpack ne
                            // signale pas (il ne résout pas les identifiants).
                            icon="bi bi-disc"
                            label={entry.name}
                            helpText={formatSize(entry.sizeBytes)}
                            onClick={null}
                            // RIEN NE SE TÉLÉCHARGE HORS DU DOSSIER GAMEFLEX sur
                            // un téléphone (le porteur, 2026-10-12) : ce bouton
                            // y mènerait aux Téléchargements de l'appareil.
                            next={hasLocalLibrary() ? null : progress[entry.path] ? (
                                <DownloadRing
                                    percent={progress[entry.path].percent}
                                    label={t("admin.servers.download")} />
                            ) : (
                                <bs.Button
                                    variant="primary"
                                    title={t("admin.servers.download")}
                                    onClick={(event) => {
                                        event.preventDefault();
                                        event.stopPropagation();
                                        download(entry);
                                    }}
                                >
                                    <i className="bi bi-download"></i>
                                </bs.Button>
                            )}
                        />
                    )}
                    items={shown.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE)}
                    itemCount={shown.length}
                    itemsPerPage={ITEMS_PER_PAGE}
                    currentPage={page}
                    onPositionChange={setPage}
                    isLoading={isLoading}
                    search={{
                        context: { values, setValues },
                        placeholder: t("admin.servers.searchContents"),
                        filters: summary?.scanned ? {
                            Component: () => (
                                <>
                                    {/* Aucun choix de section : voir l'état
                                        supprimé en tête du composant (#153). */}
                                    {consoles.length > 0 && (
                                        <Form.Group label={t("admin.servers.consoleLabel")} helpText={t("admin.servers.consoleHelp")}>
                                            <Form.Select
                                                name="console"
                                                context={{
                                                    values: { console: consoleId },
                                                    setValues: (v) => setConsoleId(v.console),
                                                }}
                                                options={consoles.map((entry) => ({
                                                    value: entry.id,
                                                    label: `${entry.id} (${entry.roms})`,
                                                }))}
                                            />
                                        </Form.Group>
                                    )}
                                </>
                            ),
                        } : null,
                    }}
                    texts={{
                        itemsFound: t("admin.servers.contentsFound"),
                        noItemsFound: summary?.scanned
                            ? t("admin.servers.contentsEmpty")
                            : t("admin.servers.contentsNeverScanned"),
                    }}
                />
            </SectionPanel>
        </>
    );
}
