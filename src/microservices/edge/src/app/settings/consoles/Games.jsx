// LES JEUX D'UNE CONSOLE (Réglages › Consoles › une console › « Jeux de la console ») — chantier 53, P8 (#677), sur les
// modèles 21 à 29 validés par le porteur le 2026-11-13 (docs/reports/2026-11-13-launcher-modeles/) :
//
//   …/<console>/games                          les SOURCES : cette machine, chaque disque interne, chaque serveur
//                                              fédéré, chaque ressource externe (`#808`, gameflex-federation intégré
//                                              à GameFlex : Samba, WebDAV, kDrive, FTP, SFTP ; ses jeux s'importent) ;
//   …/games/<source>                           ses jeux, cherchés (`?q=`) et feuilletés (`?page=`) ;
//   …/games/<source>/game?ref=                 un jeu : d'ici, ses informations, puis « Plus » (son correctif, son
//                                              apparence, les corrections de sa fiche) et le retirer (« Danger ») ;
//                                              d'un serveur fédéré, « Télécharger dans la bibliothèque » (qui prend
//                                              la file des jeux) et « Télécharger les métadonnées dans la
//                                              bibliothèque » ;
//   …/games/<source>/patch?ref=                son correctif, comme un émulateur : installé, ses détails et le
//                                              supprimer ; absent, « Le récupérer » d'une source qui l'a ;
//   …/games/<source>/appearance?ref=           son apparence : ses informations, ses images une ligne par sorte
//                                              (chantier 53, P9) ;
//   …/games/<source>/image?ref=&kind=          une image : la remplacer (elle ne se supprime pas seule) ;
//   …/games/<source>/image-search?ref=&kind=   en ajouter une, ou la remplacer : la recherche chez les fournisseurs
//                                              de métadonnées (le porteur, 2026-11-14, remarque d) ;
//   …/games/<source>/infos|match|forget?ref=   corriger les informations, « Ce n'est pas le bon jeu ? », oublier une
//                                              source (GameMetadata.jsx).
//
// LA SOURCE EST UN SEGMENT DE L'ADRESSE : `here` (la bibliothèque de cette machine), `disk-<nom>`, `server-<id>`,
// `resource-<id>`. LA
// RÉFÉRENCE D'UN JEU VA DANS LA REQUÊTE, pas dans le chemin : c'est souvent un nom de fichier (« Zelda.zip »).
//
// L'ANCIENNE PAGE « JEUX » DES RÉGLAGES EST PARTIE (AGENTS.md : aucune rétro compatibilité) : son choix d'une console et
// d'une source, ses ressources distantes (Samba, WebDAV, kDrive), et le téléchargement qui attendait la réponse de son
// POST. Un jeu fédéré et un correctif passent par la commande `download` du canal du kernel, qui dit sa progression
// (libs/downloads.js).
//
// Une ligne ouvre une page, elle n'agit pas ; chaque section a un titre ; un geste qui n'agit sur aucun groupe est une
// ligne — ce qui détruit dans « Danger », en bas, le reste dans « Plus » ; toute confirmation est la fenêtre de
// Skeleton (useConfirm).
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les disques et les serveurs fédérés des sources (leur page dans l'adresse, `?disksPage=`,
// `?serversPage=`), les jeux d'une source (`?page=`), les sources d'un correctif.

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams, useSearchParams } from "react-router-dom";
import { Listing, useListSearch } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Section } from "../../bricks/Section.jsx";
import { listResources, listRoms } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { counted, fact, formatSize } from "../../libs/format.js";
import { useGameMetadata } from "../../libs/metadata.js";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";

export const consolePath = (consoleId) => `/settings/consoles/${encodeURIComponent(consoleId)}`;
export const gamesPath = (consoleId) => `${consolePath(consoleId)}/games`;
export const sourcePath = (consoleId, slug) => `${gamesPath(consoleId)}/${encodeURIComponent(slug)}`;
// Une page d'un jeu : la sienne (`game`), ou une des siennes (`patch`, `appearance`, `image`, `infos`, `match`, `forget`).
export const gamePagePath = (consoleId, slug, reference, page = "game", extra = {}) => `${sourcePath(consoleId, slug)}/${page}?${new URLSearchParams({ ref: reference, ...extra })}`;

export const HERE = "here";

// LA PAGE D'UN JEU DANS LES RÉGLAGES, depuis sa fiche de la bibliothèque (chantier 68 ; le porteur, 2026-11-19 : « ça
// permettra de faire les allers-retours plus facilement ») : sa source d'après le jeu (un disque, un serveur fédéré, ou
// cette machine).
export const gameSettingsPath = (consoleId, { disk = null, peer = null } = {}, reference) => gamePagePath(consoleId, peer ? `server-${peer}` : disk ? `disk-${disk}` : HERE, reference);
// `{kind: "here" | "disk" | "server" | "resource", id}`, ou null pour un segment qui ne désigne rien.
export const sourceOf = (slug) => {
    const match = /^(disk|server|resource)-(.+)$/.exec(slug || "");

    return slug === HERE ? { kind: "here", id: null } : match ? { kind: match[1], id: match[2] } : null;
};

// Ce que la source donne à la liste des jeux (listRoms) et aux appels qui visent un jeu (sa fiche, ses images, son
// correctif ; romSource) : un jeu d'un disque porte son disque, un jeu d'un serveur fédéré son serveur — sa fiche et
// ses images se lisent chez lui, par le relais du kernel (chantier 57, P2), et ne se corrigent pas d'ici.
export const listOptions = (source) => ({
    disk: source.kind === "disk" ? source.id : null, server: source.kind === "server" ? source.id : null,
    resource: source.kind === "resource" ? source.id : null
});
export const gameSourceOf = (source) => (source?.kind === "disk" ? { disk: source.id } : source?.kind === "server" ? { server: source.id } : {});
export const isLocal = (source) => Boolean(source) && (source.kind === "here" || source.kind === "disk");
// Une source d'où un jeu se télécharge dans la bibliothèque : un serveur fédéré, une ressource externe.
export const isRemote = (source) => source?.kind === "server" || source?.kind === "resource";

// LES RESSOURCES EXTERNES (`#808`) : leur liste, lue une fois par page (administrateur) — leur nom, leur genre, leurs
// jeux par console (`consoles`).
export function useResources() {
    const [resources, setResources] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        listResources().then((answer) => !cancelled && setResources(answer.ok ? answer.data || [] : []));

        return () => {
            cancelled = true;
        };
    }, []);

    return resources;
}

// LA RÉFÉRENCE D'UN JEU CHEZ LE SERVEUR FÉDÉRÉ QUI LE PRÊTE : la liste d'ici la préfixe de son serveur (`srv<id>:`,
// remoteReference, federation_catalog.go) ; l'import la lui transmet telle qu'il la connaît (handleRemoteStorageImport,
// client.OpenRom).
export const peerReference = (reference) => String(reference).replace(/^srv\d+:/, "");

// Le refus d'une source, dit à l'utilisateur : notre jeton refusé, la source muette, ou ce qu'en dit le serveur.
export const refusalOf = (answer, t) => (answer.status === 403 ? t("consoles.sourceDenied")
    : answer.status === 502 || answer.status === 0 ? t("consoles.sourceUnreachable") : answer.errorMessage || t("settings.actionFailed"));

// POURQUOI LA FICHE D'UN JEU FÉDÉRÉ NE SE LIT PAS (le relais du kernel, chantier 57) : notre jeton refusé (403), le
// serveur muet (502, ou rien), ou un serveur qui ne prête pas cette fiche (404 : une version qui n'a pas encore les
// fiches, ou un jeu qu'il n'a plus).
export const sheetRefusalOf = (status, server, t) => (status === 403 ? t("gameAdmin.sheetDenied", { server })
    : status === 404 ? t("gameAdmin.sheetNotLent", { server }) : t("gameAdmin.sheetUnreachable", { server }));

// LES JEUX D'UNE CONSOLE CHEZ UNE SOURCE — la même lecture pour la liste et pour une page d'un jeu ouverte par un lien
// direct (rechargée, elle n'a plus le jeu que la liste lui avait passé) : `{games}`, ou `{failure}` (la réponse).
export const readGames = (consoleId, source, query = "") => listRoms(consoleId, { query, ...listOptions(source) })
    .then((answer) => (answer.ok ? { games: answer.data || [] } : { games: null, failure: answer }));

// Le nom d'un serveur fédéré.
export const serverName = (server, id, t) => server?.hostname || server?.link || t("consoles.serverNamed", { id });

// LE NOM D'UNE SOURCE. Ce qu'elle est (la bibliothèque, un disque, un serveur fédéré) se lit sur la ligne qui l'ouvre :
// l'en-tête d'une page n'a pas de sous-titre (PageHeader).
export function useSourceNames(slug) {
    const { t } = useTranslation();
    const { servers } = useData();
    const source = sourceOf(slug);
    const resources = useResources();

    if (!source) {
        return { name: slug || "" };
    }

    if (source.kind === "here") {
        return { name: t("consoles.sourceHere") };
    }

    if (source.kind === "disk") {
        return { name: source.id };
    }

    if (source.kind === "resource") {
        return { name: (resources || []).find((resource) => String(resource.id) === source.id)?.name || "…" };
    }

    return { name: serverName((servers || []).find((server) => String(server.id) === source.id), source.id, t) };
}

// LES NIVEAUX AU-DESSUS D'UNE PAGE DES JEUX, nommés (SettingsPage, `levels`) : la console, « Jeux », la source, le jeu —
// le fil d'Ariane les dit même par un lien direct.
export function useLevels(consoleId, slug = null, gameTitle = null) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const { name } = useSourceNames(slug);

    return {
        [consolePath(consoleId)]: (consoles || []).find((entry) => entry.id === consoleId)?.name,
        [gamesPath(consoleId)]: t("gameAdmin.title"),
        ...(slug ? { [sourcePath(consoleId, slug)]: name } : {}),
        ...(slug && gameTitle ? { [`${sourcePath(consoleId, slug)}/game`]: gameTitle } : {}),
    };
}

// LE JEU QUE DÉSIGNE L'ADRESSE D'UNE PAGE (`…/games/<source>/<page>?ref=`) : celui que la page d'avant a passé
// (l'historique), sinon — un lien direct, un rechargement, `reload()` — relu chez sa source. Un jeu d'un serveur fédéré
// se relit toujours : « déjà ici » a pu changer depuis la liste (un téléchargement fini). `game` : undefined en
// lecture, null quand la source ne l'a plus.
export function useAddressedGame() {
    const { consoleId, source: slug } = useParams();
    const [params] = useSearchParams();
    const location = useLocation();
    const reference = params.get("ref") || "";
    const source = sourceOf(slug);
    const given = location.state?.game?.reference === reference ? location.state.game : null;
    const [game, setGame] = useState(given || undefined);
    const [epoch, setEpoch] = useState(0);

    useEffect(() => {
        if (!source) {
            setGame(null);

            return undefined;
        }

        if (given && epoch === 0 && !isRemote(source)) {
            setGame(given);

            return undefined;
        }
        let cancelled = false;
        readGames(consoleId, source).then(({ games }) => !cancelled && setGame((games || []).find((item) => item.reference === reference) || null));

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, reference, epoch]);

    return { consoleId, slug, source, reference, params, game, reload: () => setEpoch((value) => value + 1) };
}

// Le titre d'un jeu : celui corrigé dans sa fiche (chez son serveur pour un jeu fédéré), sinon celui de la liste.
export function useGameTitle(consoleId, source, game) {
    const metadata = useGameMetadata(source && game ? consoleId : null, game?.reference, gameSourceOf(source));

    return metadata?.title || game?.title || "";
}

// Route : /settings/consoles/<console>/games — les sources de ses jeux.
// /settings/consoles/<console>/games : LES SOURCES (le modèle 21), en un seul listing — cette machine (sa bibliothèque),
// chaque disque interne, chaque serveur fédéré (le nombre qu'il prête, lu dans la copie de son listing — grisé avec sa
// raison quand il refuse ou se tait) et chaque ressource externe, avec leur nombre de jeux de la console et leur sorte.
export default function ConsoleGames() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { consoles, disks, servers } = useData();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1" });
    const resources = useResources();
    const entry = (consoles || []).find((item) => item.id === consoleId && item.method !== "remoteplay");
    const [here, setHere] = useState(undefined);
    const [lent, setLent] = useState({});
    const levels = useLevels(consoleId);

    useEffect(() => {
        let cancelled = false;
        listRoms(consoleId).then((answer) => !cancelled && setHere(answer.ok ? (answer.data || []).length : null));

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    useEffect(() => {
        let cancelled = false;

        for (const server of servers || []) {
            listRoms(consoleId, { server: server.id }).then((answer) => !cancelled && setLent((current) => ({ ...current, [server.id]: answer })));
        }

        return () => {
            cancelled = true;
        };
    }, [consoleId, servers]);

    const opener = (slug) => () => open(sourcePath(consoleId, slug), `gs-${slug}`);
    const sources = useListSearch([
        { kind: "here", name: t("consoles.sourceHere"), type: t("gameAdmin.library") },
        ...(disks || []).map((disk) => ({ kind: "disk", disk, name: disk.name, type: t("gameAdmin.diskKind") })),
        ...(servers || []).map((server) => ({ kind: "server", server, name: serverName(server, server.id, t), type: t("gameAdmin.serverKind") })),
        ...(resources || []).map((resource) => ({ kind: "resource", resource, name: resource.name, type: t(`fedService.kinds.${resource.kind}.label`) })),
    ], (source) => `${source.name} ${source.type}`);

    return (
        <SettingsPage title={t("gameAdmin.title")} parent={consolePath(consoleId)} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {consoles && !entry ? <NotFound /> : (
                <>
                    {/* UN SEUL LISTING (le porteur, 2026-11-25 : « faudra mettre en listing car de 1. ça ne respecte pas la règle 1
                        liste par page. Et de 2. il se pourrait qu'on ait plus de 10 fédérations ou même de disques internes ») :
                        ce serveur, ses disques, les serveurs fédérés et les ressources externes, ensemble — chaque ligne dit ce
                        qu'elle est —, qui se cherchent et se feuillettent. */}
                    <Section title={t("gameAdmin.sourcesTitle")} help={t("gameAdmin.sourcesHelp")} sense="modify">
                        <Listing items={sources.found} loading={resources === undefined} page={read("page")} onPage={(page) => update({ page })}
                            search={sources.search} onSearch={(q) => { sources.onSearch(q); update({ page: null }); }} placeholder={t("gameAdmin.searchSource")} searchKey="gs-search"
                            render={(source) => {
                                if (source.kind === "here") {
                                    return <NavRow key="here" id={`gs-${HERE}`} icon="bi-pc-display" label={t("consoles.sourceHere")}
                                        value={[fact(t("facts.games"), counted(t, "games", here ?? null)), source.type]} onOpen={opener(HERE)} />;
                                }
                                if (source.kind === "disk") {
                                    return <NavRow key={`disk-${source.disk.name}`} id={`gs-disk-${source.disk.name}`} icon="bi-hdd" label={source.disk.name}
                                        value={[fact(t("facts.games"), counted(t, "games", source.disk.consoles?.[consoleId] || 0)), source.type]} onOpen={opener(`disk-${source.disk.name}`)} />;
                                }
                                if (source.kind === "server") {
                                    const answer = lent[source.server.id];
                                    const refused = Boolean(answer) && !answer.ok;

                                    return <NavRow key={`server-${source.server.id}`} id={`gs-server-${source.server.id}`} icon="bi-arrow-left-right" label={source.name}
                                        help={refused ? refusalOf(answer, t) : null} value={[fact(t("facts.games"), counted(t, "games", answer?.ok ? (answer.data || []).length : null)), source.type]}
                                        disabled={refused} onOpen={opener(`server-${source.server.id}`)} />;
                                }

                                return <NavRow key={`resource-${source.resource.id}`} id={`gs-resource-${source.resource.id}`} icon={SOURCE_ICONS[source.resource.kind] || "bi-hdd-network"}
                                    label={source.name} help={source.resource.state?.error ? t("gameAdmin.resourceUnreachable") : null}
                                    value={[fact(t("facts.games"), counted(t, "games", source.resource.consoles?.[consoleId] || 0)), source.type]} onOpen={opener(`resource-${source.resource.id}`)} />;
                            }} />
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}

// /settings/consoles/<console>/games/<source> : SES JEUX (le modèle 22), cherchés chez la source (`?q=`), une page à la
// fois (`?page=`). Une ligne : la jaquette du jeu (celle de son serveur pour un jeu fédéré, chantier 57), son titre, sa
// taille ; d'un serveur fédéré, « Déjà ici » quand il l'est.
//
// LE FILTRE PAR TAILLE (chantier 56, A6 ; le porteur, 2026-11-14 : « Rajoute aussi le filtre par taille, c'est important
// quand on veut supprimer des jeux trop lourd ») — celui de la bibliothèque de la webapp (admin/ROMs.jsx), dans le panneau
// des filtres de la liste : une taille minimum, en Mo (`?minSize=`, la même unité que la taille d'une ligne,
// formatSize), sur les jeux lus. La glissière de la webapp, réglée comme elle (Form.Range, 0 à 1000 Mo par pas de
// 10, sa case à côté pour taper une autre valeur ; le porteur, 2026-11-14 : « sur Webapp c'était une skeletonRange où
// on pouvait choisir la taille qu'on veut en mb »). À côté, PROPOSÉ ET PAS IMPOSÉ : « Les plus lourds d'abord » (`?sort=size`), éteint par
// défaut — l'ordre reste celui de la source.
export const MEGABYTE = 1024 * 1024;

// Les niveaux au-dessus d'une page sous les images d'un jeu : ceux du jeu, puis « Images ».
export const useAppearanceLevels = (consoleId, slug, title) => {
    const { t } = useTranslation();

    return { ...useLevels(consoleId, slug, title), [`${sourcePath(consoleId, slug)}/appearance`]: t("consoles.appearance") };
};

// /…/games/<source>/infos|match|forget?ref= : UNE PAGE DE LA FICHE D'UN JEU D'ICI (GameMetadata.jsx) — corriger
// ses informations (le modèle 24), « Ce n'est pas le bon jeu ? » (25), oublier une source.
export function SheetPage({ titleKey, part: Part }) {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);

    return (
        <SettingsPage title={t(titleKey)} parent={gamePagePath(consoleId, slug, reference)} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {!isLocal(source) || game === null ? <NotFound /> : <Part consoleId={consoleId} reference={reference} game={gameSourceOf(source)} fallbackTitle={game?.title || ""} />}
        </SettingsPage>
    );
}

// UN JEU D'UN SERVEUR FÉDÉRÉ (les modèles 27 à 29) : ce que sa liste en dit — d'où il vient, sa taille, « Déjà ici » —,
// puis SA FICHE, LUE CHEZ SON SERVEUR (chantier 57, P2 ; le porteur, 2026-11-15 : « si je partage mes jeux, je partage
// aussi mes métadonnées ») : la sortie, les éditeurs, les développeurs, les genres, la série, l'identifiant, en lecture
// seule — elle se corrige chez lui. Quand il ne la prête pas, une ligne grisée dit pourquoi : notre jeton refusé, le
// serveur muet, ou une version qui ne prête pas encore les fiches (`gameMetadataStatus`). « Télécharger dans la
// bibliothèque », une ligne de « Plus » : la fenêtre, puis la commande `download` (la route `server`, la section `roms`,
// le corps du POST d'avant) — LE JEU PREND LA FILE (le porteur, 2026-11-16 : « c'est possible de mettre les jeux à
// télécharger en queue ? ») : derrière d'autres, sa ligne dit son rang, et « Retirer de la file » le sort ; parti, sa
// progression ; le jeu arrivé emporte sa fiche (P3). Un jeu déjà ici le dit, grisé. Fini — ici ou pendant que la page
// était ailleurs —, le jeu se relit.
// « TÉLÉCHARGER LES MÉTADONNÉES DANS LA BIBLIOTHÈQUE » (2.4.1) : sa fiche seule, copiée dans les miennes sous son
// `gameId` (copyFederatedSheet) — le jeu téléchargé plus tard la retrouve ; d'ici là, elle est une des « Métadonnées
// sans jeu » de la console. Grisée pour un jeu déjà ici (sa fiche se règle sur sa page) ou quand le serveur ne la prête
// pas.
// L'icône du genre d'une source : une ressource externe du serveur (`#808`, Samba, WebDAV, kDrive, FTP, SFTP).
export const SOURCE_ICONS = { disk: "bi-device-hdd", smb: "bi-hdd-network", webdav: "bi-cloud", kdrive: "bi-cloud-fill", ftp: "bi-folder-symlink", sftp: "bi-shield-lock" };
