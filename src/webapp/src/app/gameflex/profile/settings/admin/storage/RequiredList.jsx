import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../../skeleton/skeleton.js";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    DownloadRing,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import { formatSize } from "../api/library.js";
import { parseSource, importFromSource, deleteInstalled } from "../api/storage-admin.js";
import { useAuth } from "../../../auth/AuthContext.jsx";
import SourceSelector, { useDiskSources } from "./SourceSelector.jsx";
import { publishDownloadProgress } from "../../../../libs/deviceStream.js";
import { federatedServers } from "../../../../libs/federatedHost.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;

// CE QU'IL FAUT AVOIR, ET CE QU'ON A.
//
// La liste vient du REGISTRE, jamais du disque. C'est tout l'écran : un
// dossier vide doit se lire « rien d'installé », pas « cette console
// n'existe pas ». Jusqu'ici un core ou un BIOS manquant se manifestait par
// un émulateur qui refuse de démarrer, sans dire ce qu'il cherchait.
//
// Cores et firmwares partagent cette liste parce qu'ils posent la même
// question et n'ont qu'une différence : un core manquant se télécharge, un
// firmware non — personne ne les distribue légitimement. Cette différence
// tient dans `onDownload`, absent pour les BIOS. Le reste serait deux fois
// le même fichier.

// Les deux états, comme valeurs de filtre. Des identifiants, pas des
// libellés : le texte affiché est traduit.
const STATE_INSTALLED = "installed";
const STATE_MISSING = "missing";

export default function RequiredList({
    t, load, onDownload = null, texts,
    // « cores » ou « bios » — la section que cet écran gère, pour que le
    // rapatriement depuis un pair sache où déposer le fichier.
    section,
    // Les natures de source que CETTE section accepte, dans l'ordre d'affichage
    // (matrice #154). « peers » se déplie en une entrée par serveur enregistré,
    // « disks » en une entrée par disque interne présent (chantier 24).
    sources = ["local"],
}) {
    // Deux filtres, la même paire que « Ma bibliothèque » proposait déjà :
    // LESQUELLES (consoles) et DANS QUEL ÉTAT. Rien de coché = rien
    // d'écarté — un filtre vide n'a pas encore servi.
    const [values, setValues] = useState({ q: "", consoles: [], states: [] });
    const [sections, setSections] = useState([]);
    // LES LIGNES, DANS L'ORDRE FIXÉ À LA LECTURE (voir `reload`). Un geste sur
    // une ligne la change SUR PLACE : télécharger ou supprimer ne relit plus
    // la liste, et la ligne ne saute pas ailleurs (relevé par le porteur le
    // 2026-10-10 — « on perd le fil car toute la liste se recharge »).
    const [rows, setRows] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [pending, setPending] = useState(null);

    // LA SOURCE, sous forme de descripteur (`local` | `official` | `peer:<id>`).
    //
    // Les sources OFFERTES viennent de l'appelant (`sources`), parce qu'elles
    // dépendent de la section : la matrice #154 dit que les cœurs se trouvent
    // en local, chez un pair ou à la source officielle, alors que les BIOS
    // n'ont pas de source officielle — personne ne les distribue. Mettre ce
    // choix ici aurait redonné la même liste aux deux écrans, ce qui était
    // précisément le défaut du premier jet (#148).
    //
    // Un montage Samba/WebDAV/kDrive n'apparaît jamais dans CETTE liste : il ne
    // porte que des ROMs, donc il ne concerne que l'écran des jeux.
    const [source, setSource] = useState("local");
    const [peers, setPeers] = useState([]);
    const diskSources = useDiskSources(t);
    // Ce que la source CHOISIE peut fournir, en clés « console/nom ». `null`
    // quand la source est notre propre disque : la question ne se pose pas.
    const [available, setAvailable] = useState(null);

    // Le contexte À JOUR, sans que la lecture en dépende : il change d'identité
    // à chaque fenêtre qui s'ouvre (voir `useLatestContext`).
    const latestContext = useLatestContext();
    const context = latestContext.current;
    const { isAdmin } = useAuth();

    useEffect(() => {
        let cancelled = false;
        federatedServers()
            .then((found) => {
                if (!cancelled) {
                    setPeers(found || []);
                }
            })
            // Pas de fédération configurée n'est pas une panne : le choix ne
            // s'affiche simplement pas.
            .catch(() => undefined);

        return () => {
            cancelled = true;
        };
    }, []);

    const reload = useCallback(() => {
        setLoading(true);

        // LA LISTE NE CHANGE PAS AVEC LA SOURCE (#154, tranché le 2026-09-17).
        //
        // Pour les cœurs et les BIOS, le registre sait d'avance ce qu'il faut :
        // la liste connue RESTE la liste, et chaque ligne dit si l'élément est
        // présent ici. La source choisie ne sert qu'à répondre, ligne par
        // ligne, « celle-là peut-elle me le fournir ? ».
        //
        // Le premier jet remplaçait la liste par celle du pair : on perdait
        // alors ce qu'on sait devoir avoir, qui est justement l'intérêt de cet
        // écran.
        const { kind } = parseSource(source);

        return Promise.all([
            load("local"),
            kind === "local" || kind === "official" ? null : load(source),
        ]).then(([connu, ailleurs]) => {
            if (!connu.ok) {
                latestContext.current.toast.open(connu.errorMessage || t("admin.storage.loadFailed"),
                    { variant: "error" });
            }
            setSections(connu.consoles || []);
            // LES FICHIERS D'ÉMULATEUR DE LA SOURCE (chantier 29) : une archive
            // d'éditeur, notre construction, un paquet du téléphone. Le registre
            // ne les connaît pas — leur nom porte leur version —, donc ce qu'ici
            // n'a pas s'ajoute à la liste, absent, pour être rapatrié.
            const ici = new Set((connu.consoles || []).flatMap((found) =>
                (found.items || []).map((item) => `${found.consoleId}/${item.name}`)));
            const leurs = (ailleurs && ailleurs.ok ? ailleurs.consoles || [] : [])
                .flatMap((found) => (found.items || [])
                    .filter((item) => item.package && !ici.has(`${found.consoleId}/${item.name}`))
                    .map((item) => ({
                        // « Utilisé » ne vaut que chez lui : ici, rien ne le prend encore.
                        ...item, installed: false, inUse: false, consoleId: found.consoleId, consoleName: found.console,
                    })));
            // UNE SEULE LISTE, et la console dans la ligne plutôt qu'en titre
            // de groupe. Un regroupement par console la ferait apparaître deux
            // fois — une fois en en-tête, une fois sous le nom du fichier — et
            // découperait en quarante blocs une liste qu'on parcourt justement
            // pour trouver les trous. Ce qui manque passe donc devant.
            //
            // L'ORDRE SE FIXE ICI, À LA LECTURE, et nulle part ailleurs : trié
            // à chaque rendu, un cœur qu'on vient de télécharger partait en fin
            // de liste sous les yeux de qui l'avait demandé.
            setRows((connu.consoles || [])
                .flatMap((found) => (found.items || []).map((item) => ({
                    ...item,
                    consoleId: found.consoleId,
                    consoleName: found.console,
                })))
                .concat(leurs)
                .sort((left, right) => Number(left.installed) - Number(right.installed)));
            // Un pair muet vaut « rien de disponible », jamais une erreur : la
            // liste connue doit rester lisible même quand la source ne répond
            // pas (même règle que le relais côté kernel).
            //
            // CE QU'IL A, PAS CE QU'IL LISTE. Un pair rend la même liste que
            // nous — tout ce que le registre attend, chacun avec son
            // `installed`. La compter entière activait le bouton sur ce qu'il
            // n'a pas lui-même, et le rapatriement répondait 404 : 35 firmwares
            // sur 58 chez un serveur ami (relevé le 2026-10-10).
            setAvailable(ailleurs && ailleurs.ok
                ? new Set((ailleurs.consoles || []).flatMap((section) =>
                    (section.items || [])
                        .filter((item) => item.installed)
                        .map((item) => `${section.consoleId}/${item.name}`)))
                : (ailleurs ? new Set() : null));
        }).catch((failure) => {
            console.error("Lecture de la liste des éléments requis", failure);
            latestContext.current.toast.open(t("admin.storage.loadFailed"), { variant: "error" });
        }).finally(() => {
            setLoading(false);
        });
    }, [load, source, t, latestContext]);

    useEffect(() => {
        reload();
    }, [reload]);

    // Une ligne changée sur place, sans relire la liste.
    const updateRow = (item, change) => setRows((current) => current.map((row) => (
        row.consoleId === item.consoleId && row.name === item.name ? { ...row, ...change } : row
    )));

    const filtered = useMemo(() => {
        const needle = (values.q || "").trim().toLowerCase();

        return rows.filter((item) => {
            if (needle && !item.name.toLowerCase().includes(needle)
                && !item.consoleName.toLowerCase().includes(needle)) {
                return false;
            }
            if (values.consoles.length > 0 && !values.consoles.includes(item.consoleId)) {
                return false;
            }
            if (values.states.length > 0
                && !values.states.includes(item.installed ? STATE_INSTALLED : STATE_MISSING)) {
                return false;
            }

            return true;
        });
    }, [rows, values]);

    // Pagination locale : la liste entière est déjà en mémoire — le registre
    // en déclare quelques dizaines, pas des milliers.
    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    // Une descente réussie change la LIGNE, et elle seule : « présent », avec
    // la taille que le serveur vient d'écrire — sa réponse la porte
    // (`sizeBytes`), il n'y a rien à deviner ni à relire.
    // D'OÙ ON LE PREND DÉPEND DE LA SOURCE CHOISIE.
    //
    // Un pair ou un disque interne : on le rapatrie par notre kernel. La
    // source officielle : le téléchargement buildbot déjà en place. Et l'échec se DIT — on ne peut
    // pas connaître l'inventaire de buildbot d'avance, donc son bouton reste
    // actif et c'est la réponse qui tranche.
    const download = async (item) => {
        const { kind } = parseSource(source);

        setPending(`${item.consoleId}/${item.name}`);
        // La cloche de l'application doit le dire aussi : on quitte souvent cet
        // écran pendant qu'un fichier descend, et l'anneau de la ligne part
        // avec lui. `percent: -1` parce que c'est le serveur qui transfère —
        // la page ne compte aucun octet, donc l'anneau tourne.
        publishDownloadProgress({ label: item.name, done: 0, total: 0, percent: -1 });

        try {
            const result = kind === "peer" || kind === "disk"
                ? await importFromSource(source, section, { consoleId: item.consoleId, name: item.name })
                : await onDownload(item.consoleId, item.name);

            context.toast.open(
                result.ok
                    ? t(result.androidError ? "admin.storage.downloadedNoAndroid" : "admin.storage.downloaded",
                        { name: item.name })
                    : (result.errorMessage || t("admin.storage.notAtSource", { name: item.name })),
                { variant: result.ok ? "success" : "error" },
            );

            if (result.ok) {
                // Rangé dans le dossier de sa console : plus « non rangé ». La
                // variante Android, quand le serveur vient de la ranger aussi
                // (chantier 13, phase 5), change la ligne de la même façon.
                updateRow(item, {
                    installed: true,
                    sizeBytes: Number(result.sizeBytes) || item.sizeBytes || 0,
                    scoped: true,
                    ...(result.androidSizeBytes
                        ? { androidInstalled: true, androidSizeBytes: Number(result.androidSizeBytes) }
                        : {}),
                });
            }
        } catch (failure) {
            console.error("Téléchargement d'un élément requis", failure);
            context.toast.open(t("app.error"), { variant: "error" });
        } finally {
            // La cloche est GLOBALE : sans cette remise à zéro, un rejet la
            // laisse tourner sur tous les écrans jusqu'au rechargement.
            publishDownloadProgress(null);
            setPending(null);
        }
    };

    // UN CORE PRÉSENT N'EST PAS UN CORE À JOUR.
    //
    // ⚠️ CE COMMENTAIRE AFFIRMAIT LE CONTRAIRE DU CODE, et il a piégé deux
    // lectures (corrigé le 2026-09-27). Il promettait « la coche dit "il est
    // là", à côté d'un bouton qui reste » — or #154 a retiré ce bouton EN
    // SOURCE LOCALE le 2026-09-17 : la coche y voisine une corbeille, rien de
    // plus. Le bouton « Retélécharger » n'existe que sur une source qui
    // annonce un fichier — l'officielle ou un pair — c'est-à-dire la branche
    // plus bas, pas celle-ci.
    //
    // Ce qui reste vrai, et qui est la raison d'être de ce bouton là-bas :
    // reprendre une version plus récente doit être possible sans aller
    // supprimer le fichier à la main. Il est secondaire plutôt que primaire,
    // parce que retélécharger est un geste qu'on choisit, pas celui que
    // l'écran propose en premier.
    //
    // Un firmware n'a pas de bouton : personne ne les distribue. Absent, on
    // le DIT plutôt que de laisser la ligne muette — « il manque » est
    // l'information utile ici.
    // LE BOUTON DE BUILDBOT NE VAUT QUE POUR NOTRE PROPRE DISQUE.
    //
    // En local comme à la source officielle, il descend le cœur ICI. Chez un
    // pair il n'aurait aucun sens : sa route de téléchargement est réservée à
    // SES administrateurs, et on ne remplit pas la bibliothèque d'autrui.
    //
    // L'autre sens — rapatrier chez nous ce qu'un pair possède — passe par sa
    // propre route (`importFromSource`), d'où l'aiguillage dans `download`.
    const { kind: sourceKind } = parseSource(source);

    // CE QUE LA LIGNE OFFRE DÉPEND DE LA SOURCE (#154, tranché le 2026-09-17).
    //
    //   local      → l'état de NOTRE disque : la coche et la corbeille pour ce
    //                qui est là ; pour un cœur absent, le bouton de la source
    //                officielle (rétabli le 2026-10-10), et pour un BIOS absent
    //                le seul badge.
    //   officielle → un bouton, toujours actif : l'inventaire de buildbot n'est
    //                pas connaissable d'avance, donc c'est la réponse qui dira
    //                si l'élément y est.
    //   un pair    → un bouton, actif seulement si SON inventaire le porte —
    //                là, on sait, puisqu'on vient de le lister.
    // Retirer un élément installé ici : la ligne repasse à « absent », sur
    // place — sinon rien ne dit que le geste a eu lieu.
    const remove = async (item) => {
        setPending(`${item.consoleId}/${item.name}`);

        try {
            const result = await deleteInstalled(section, item.consoleId, item.name);

            context.toast.open(
                result.ok
                    ? t("admin.library.removed", { name: item.name })
                    : (result.errorMessage || t("admin.library.removeFailed")),
                { variant: result.ok ? "success" : "error" },
            );

            if (result.ok && item.package) {
                // Un fichier d'émulateur n'est attendu par personne : parti, il
                // quitte la liste au lieu d'y rester « absent ».
                setRows((current) => current.filter((row) => !(row.consoleId === item.consoleId
                    && row.name === item.name)));
            } else if (result.ok) {
                updateRow(item, { installed: false, sizeBytes: 0 });
            }
        } catch (failure) {
            console.error("Suppression d'un élément installé", failure);
            context.toast.open(t("app.error"), { variant: "error" });
        } finally {
            setPending(null);
        }
    };

    const actionFor = (item) => {
        const key = `${item.consoleId}/${item.name}`;

        // L'ANNEAU PREND LA PLACE DU BOUTON pendant le transfert, pour que la
        // ligne ne bouge pas. Et `percent: null` le fait TOURNER plutôt que se
        // remplir : c'est le serveur qui va chercher le fichier, la page ne
        // compte aucun octet, et un anneau figé à 0 % mentirait davantage
        // qu'une rotation (voir DownloadRing, et le défaut corrigé le
        // 2026-09-08 dans Games.jsx).
        if (pending === key) {
            return <DownloadRing percent={null} label={t("admin.storage.download")} />;
        }

        if (sourceKind === "local") {
            if (!item.installed) {
                // ABSENT ICI : LE BADGE, ET LE BOUTON DE LA SOURCE OFFICIELLE.
                //
                // Demandé par le porteur le 2026-10-10, en deux temps : « remets
                // comme c'était avant avec le bouton download depuis la source
                // officielle », puis, du texte qui renvoyait au sélecteur
                // (#209a) : « enlève ce texte […] juste missing suffit ». Le
                // badge dit l'état, le bouton le geste ; une phrase répétée sur
                // chaque ligne n'apprenait rien. `download` va à la source
                // officielle, puisque la vue n'est pas celle d'un pair.
                //
                // Le bouton est GARDÉ PAR `onDownload` : l'écran des BIOS n'en
                // fournit pas — personne ne distribue de BIOS.
                return (
                    <div className="d-flex align-items-center gap-2">
                        <bs.Badge bg="secondary">{t("admin.storage.missing")}</bs.Badge>
                        {onDownload && (
                            <bs.Button
                                variant="primary"
                                disabled={pending !== null}
                                title={t("admin.storage.download")}
                                onClick={(event) => {
                                    event.preventDefault();
                                    event.stopPropagation();
                                    download(item);
                                }}
                            >
                                <i className="bi bi-download"></i>
                            </bs.Button>
                        )}
                    </div>
                );
            }

            // LA COCHE DIT « IL EST LÀ » ; LA CORBEILLE PERMET D'EN SORTIR.
            //
            // Sans elle, revenir à une version de cœur qui marche demandait un
            // accès shell — le noyau tourne en root, donc les dossiers qu'il a
            // créés ne se laissaient plus gérer (#146). Et le téléchargement
            // refuse d'écraser un cœur en place (#143) en disant « le
            // supprimer d'abord » : la consigne existait sans le geste.
            //
            // Administrateurs seulement : le disque est celui de tout le monde.
            // LA VARIANTE ANDROID QUI MANQUE (chantier 13, phase 5) : les
            // téléphones de ce serveur viennent la prendre ici. `false` et non
            // « absent » : seul le serveur la tient — le téléphone, dont les
            // cœurs SONT la version Android, n'en dit rien. Même bouton que
            // l'absent, en secondaire : c'est un complément, pas le cœur.
            return (
                <div className="d-flex align-items-center gap-2">
                    <i className="bi bi-check-circle-fill text-success fs-4"></i>
                    {isAdmin && onDownload && item.androidInstalled === false && (
                        <bs.Button
                            variant="secondary"
                            disabled={pending !== null}
                            title={t("admin.storage.downloadAndroid")}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                download(item);
                            }}
                        >
                            <i className="bi bi-phone"></i>
                        </bs.Button>
                    )}
                    {isAdmin && (
                        <bs.Button
                            variant="danger"
                            disabled={pending !== null}
                            title={t("app.delete")}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                context.modal.open(
                                    <div className="text-center">
                                        {t("admin.library.removeConfirm")} <br />
                                        <strong>{item.name}</strong>
                                    </div>,
                                    {
                                        variant: "confirm",
                                        onConfirm: () => remove(item),
                                        confirmLabel: t("app.delete"),
                                        cancelLabel: t("app.cancel"),
                                    },
                                );
                            }}
                        >
                            <i className="bi bi-trash"></i>
                        </bs.Button>
                    )}
                </div>
            );
        }

        const absent = available !== null && !available.has(key);

        return (
            <div className="d-flex align-items-center gap-2">
                {item.installed && (
                    <i className="bi bi-check-circle-fill text-success fs-4"></i>
                )}
                <bs.Button
                    variant={item.installed ? "secondary" : "primary"}
                    disabled={pending !== null || absent}
                    title={absent
                        ? t("admin.storage.notAtSource", { name: item.name })
                        : (item.installed
                            ? t("admin.storage.redownload")
                            : t("admin.storage.download"))}
                    onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        download(item);
                    }}
                >
                    <i className={item.installed ? "bi bi-arrow-clockwise" : "bi bi-download"}></i>
                </bs.Button>
            </div>
        );
    };

    const searchContext = { values, setValues };
    const consoleOptions = useMemo(
        () => sections.map((section) => ({ value: section.consoleId, label: section.console })),
        [sections],
    );

    // Les options, dans l'ordre déclaré par l'appelant. « peers » se déplie en
    // une entrée par serveur ; une nature sans rien à proposer disparaît.
    //
    // LE SÉLECTEUR SE VOIT TOUJOURS, même avec une seule option (demandé le
    // 2026-09-17). Je l'avais masqué dans ce cas — « un choix unique n'est pas
    // un choix » — mais il dit AUSSI d'où vient ce qu'on regarde, et cette
    // information vaut d'être lue même quand il n'y a rien d'autre à choisir.
    const sourceOptions = sources.flatMap((kind) => {
        if (kind === "local") {
            return [{ value: "local", label: t("admin.storage.sourceHere") }];
        }
        if (kind === "official") {
            return [{ value: "official", label: t("admin.storage.sourceOfficial") }];
        }
        if (kind === "peers") {
            return peers.map((peer) => ({
                value: `peer:${peer.id}`,
                label: peer.hostname || peer.link,
            }));
        }
        if (kind === "disks") {
            return diskSources;
        }

        return [];
    });

    return (
        <SectionPanel>
            {/* Le sélecteur et sa remise en cache vivent dans UN composant
                partagé : les trois écrans posent la même question, et trois
                copies finiraient par ne plus se ressembler. */}
            <SourceSelector
                t={t}
                value={source}
                onChange={setSource}
                options={sourceOptions}
                onRefreshed={reload}
                section={section}
                helpText={t("admin.storage.sourceHelp")} />
            <ListingElement
                callback={(item) => (
                    <ListElement
                        key={`${item.consoleId}::${item.name}`}
                        icon={item.package ? "bi bi-box-seam" : (onDownload ? "bi bi-cpu" : "bi bi-fingerprint")}
                        label={item.name}
                        // La console, puis la taille quand le fichier est là.
                        // Absent, la taille n'existe pas — écrire « 0 o »
                        // ferait croire à un fichier vide. « non rangé »
                        // n'apparaît que pour un fichier trouvé HORS du
                        // dossier de sa console : il se charge quand même,
                        // mais il vient de la pile où deux cents fichiers se
                        // ressemblent, et le dire est le seul moyen qu'elle
                        // se résorbe.
                        helpText={[
                            item.consoleName,
                            item.package === "server" ? t("admin.storage.packageServer") : null,
                            item.package === "phone" ? t("admin.storage.packagePhone") : null,
                            // Une console récente prend le dernier fichier déposé ;
                            // la liste des versions éprouvées informe sans bloquer
                            // (chantier 29).
                            item.inUse ? t("admin.storage.emulatorInUse") : null,
                            item.tested === true ? t("admin.storage.emulatorTested") : null,
                            item.tested === false ? t("admin.storage.emulatorUntested") : null,
                            item.installed || item.package ? formatSize(item.sizeBytes) : null,
                            item.installed && item.scoped === false
                                ? t("admin.storage.unfiled")
                                : null,
                            item.androidInstalled ? t("admin.storage.android") : null,
                        ].filter(Boolean).join(" · ")}
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
                    placeholder: texts.search,
                    filters: {
                        Component: () => (
                            <div>
                                <Form.Group
                                    label={t("admin.library.filterConsole")}
                                    helpText={t("admin.library.filterConsoleHelp")}
                                >
                                    <Form.Multicheckbox
                                        name="consoles"
                                        context={searchContext}
                                        options={consoleOptions} />
                                </Form.Group>
                                <Form.Group
                                    label={t("admin.storage.filterState")}
                                    helpText={t("admin.storage.filterStateHelp")}
                                >
                                    <Form.Multicheckbox
                                        name="states"
                                        context={searchContext}
                                        options={[
                                            { value: STATE_INSTALLED, label: t("admin.storage.installed") },
                                            { value: STATE_MISSING, label: t("admin.storage.missing") },
                                        ]} />
                                </Form.Group>
                            </div>
                        ),
                    },
                }}
                texts={{ itemsFound: texts.found, noItemsFound: texts.empty }}
            />
        </SectionPanel>
    );
}
