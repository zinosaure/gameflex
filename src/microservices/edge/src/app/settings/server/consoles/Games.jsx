import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams, useSearchParams } from "react-router-dom";
import { copyFederatedSheet, deleteInstalled, deleteRom, gamePatch, listRoms } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { downloadOutcome, downloadState, followDownload, queuedNotice, runDownload, useDownloads } from "../../../libs/downloads.js";
import { formatMegabytes, formatSize } from "../../../libs/format.js";
import { gameMetadataStatus, useGameMetadata } from "../../../libs/metadata.js";
import { toast } from "../../../shell/Shell.jsx";
import { GameCover } from "../../../screens/library/HeroArt.jsx";
import { GameAppearance, GameForget, GameImage, GameImageSearch, GameInfoForm, GameMatch, GameReset, IMAGE_KINDS } from "./GameMetadata.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import NavRow, { DangerRow, DangerSection, DownloadRow, EmptyRow, InfoRow, Listing, Note, PlaceholderRow, RadioRow, RangeRow, Rows, Section, SwitchRow, useConfirm } from "../../../ui/Bricks.jsx";

// LES JEUX D'UNE CONSOLE (Réglages › Consoles › une console › « Jeux de la console ») — chantier 53, P8 (#677), sur les
// modèles 21 à 29 validés par le porteur le 2026-11-13 (docs/reports/2026-11-13-launcher-modeles/) :
//
//   …/<console>/games                          les SOURCES : cette machine, chaque disque interne, chaque serveur
//                                              fédéré — plus de ressource distante (le porteur : « GameFlex ne
//                                              connaîtra que la fédération et les disques internes ») ;
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
// LA SOURCE EST UN SEGMENT DE L'ADRESSE : `here` (la bibliothèque de cette machine), `disk-<nom>`, `server-<id>`. LA
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

const consolePath = (consoleId) => `/settings/consoles/${encodeURIComponent(consoleId)}`;
const gamesPath = (consoleId) => `${consolePath(consoleId)}/games`;
const sourcePath = (consoleId, slug) => `${gamesPath(consoleId)}/${encodeURIComponent(slug)}`;
// Une page d'un jeu : la sienne (`game`), ou une des siennes (`patch`, `appearance`, `image`, `infos`, `match`, `forget`).
const gamePagePath = (consoleId, slug, reference, page = "game", extra = {}) => `${sourcePath(consoleId, slug)}/${page}?${new URLSearchParams({ ref: reference, ...extra })}`;

const HERE = "here";
// `{kind: "here" | "disk" | "server", id}`, ou null pour un segment qui ne désigne rien.
const sourceOf = (slug) => {
    const match = /^(disk|server)-(.+)$/.exec(slug || "");

    return slug === HERE ? { kind: "here", id: null } : match ? { kind: match[1], id: match[2] } : null;
};

// Ce que la source donne à la liste des jeux (listRoms) et aux appels qui visent un jeu (sa fiche, ses images, son
// correctif ; romSource) : un jeu d'un disque porte son disque, un jeu d'un serveur fédéré son serveur — sa fiche et
// ses images se lisent chez lui, par le relais du kernel (chantier 57, P2), et ne se corrigent pas d'ici.
const listOptions = (source) => ({ disk: source.kind === "disk" ? source.id : null, server: source.kind === "server" ? source.id : null });
const gameSourceOf = (source) => (source?.kind === "disk" ? { disk: source.id } : source?.kind === "server" ? { server: source.id } : {});
const isLocal = (source) => Boolean(source) && source.kind !== "server";

// LA RÉFÉRENCE D'UN JEU CHEZ LE SERVEUR FÉDÉRÉ QUI LE PRÊTE : la liste d'ici la préfixe de son serveur (`srv<id>:`,
// remoteReference, federation_catalog.go) ; l'import la lui transmet telle qu'il la connaît (handleRemoteStorageImport,
// client.OpenRom).
const peerReference = (reference) => String(reference).replace(/^srv\d+:/, "");

// Le refus d'une source, dit à l'utilisateur : notre jeton refusé, la source muette, ou ce qu'en dit le serveur.
const refusalOf = (answer, t) => (answer.status === 403 ? t("consoles.sourceDenied")
    : answer.status === 502 || answer.status === 0 ? t("consoles.sourceUnreachable") : answer.errorMessage || t("settings.actionFailed"));

// POURQUOI LA FICHE D'UN JEU FÉDÉRÉ NE SE LIT PAS (le relais du kernel, chantier 57) : notre jeton refusé (403), le
// serveur muet (502, ou rien), ou un serveur qui ne prête pas cette fiche (404 : une version qui n'a pas encore les
// fiches, ou un jeu qu'il n'a plus).
const sheetRefusalOf = (status, server, t) => (status === 403 ? t("gameAdmin.sheetDenied", { server })
    : status === 404 ? t("gameAdmin.sheetNotLent", { server }) : t("gameAdmin.sheetUnreachable", { server }));

// LES JEUX D'UNE CONSOLE CHEZ UNE SOURCE — la même lecture pour la liste et pour une page d'un jeu ouverte par un lien
// direct (rechargée, elle n'a plus le jeu que la liste lui avait passé) : `{games}`, ou `{failure}` (la réponse).
const readGames = (consoleId, source, query = "") => listRoms(consoleId, { query, ...listOptions(source) })
    .then((answer) => (answer.ok ? { games: answer.data || [] } : { games: null, failure: answer }));

// Le nom d'un serveur fédéré.
const serverName = (server, id, t) => server?.hostname || server?.link || t("consoles.serverNamed", { id });

// LE NOM D'UNE SOURCE. Ce qu'elle est (la bibliothèque, un disque, un serveur fédéré) se lit sur la ligne qui l'ouvre :
// l'en-tête d'une page n'a pas de sous-titre (PageHeader).
function useSourceNames(slug) {
    const { t } = useTranslation();
    const { servers } = useData();
    const source = sourceOf(slug);

    if (!source) {
        return { name: slug || "" };
    }

    if (source.kind === "here") {
        return { name: t("consoles.sourceHere") };
    }

    if (source.kind === "disk") {
        return { name: source.id };
    }

    return { name: serverName((servers || []).find((server) => String(server.id) === source.id), source.id, t) };
}

// LES NIVEAUX AU-DESSUS D'UNE PAGE DES JEUX, nommés (SettingsPage, `levels`) : la console, « Jeux », la source, le jeu —
// le fil d'Ariane les dit même par un lien direct.
function useLevels(consoleId, slug = null, gameTitle = null) {
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
function useAddressedGame() {
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

        if (given && epoch === 0 && source.kind !== "server") {
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
function useGameTitle(consoleId, source, game) {
    const metadata = useGameMetadata(source && game ? consoleId : null, game?.reference, gameSourceOf(source));

    return metadata?.title || game?.title || "";
}

// /settings/consoles/<console>/games : LES SOURCES (le modèle 21) — « Sur ce serveur » : cette machine (sa
// bibliothèque) et chaque disque interne, avec leur nombre de jeux de la console ; « Serveurs fédérés » : chacun, le
// nombre qu'il prête, lu dans la copie de son listing — grisé avec sa raison quand il refuse ou se tait. Les disques et
// les serveurs se feuillettent, cette machine en tête de chaque page des disques.
function SourcesPage() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { consoles, disks, servers } = useData();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ disksPage: "1", serversPage: "1" });
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

    return (
        <SettingsPage title={t("gameAdmin.title")} parent={consolePath(consoleId)} levels={levels}>
            {consoles && !entry ? <NotFound /> : (
                <>
                    <Section title={t("gameAdmin.hereTitle")}>
                        <Listing items={disks || []} page={read("disksPage")} onPage={(page) => update({ disksPage: page })}
                            head={<NavRow id={`gs-${HERE}`} icon="bi-pc-display" tone="indigo" label={t("consoles.sourceHere")} help={t("gameAdmin.library")} value={here ?? null} onOpen={opener(HERE)} />}
                            render={(disk) => (
                                <NavRow key={disk.name} id={`gs-disk-${disk.name}`} icon="bi-hdd" tone="indigo" label={disk.name} help={t("gameAdmin.diskKind")}
                                    value={disk.consoles?.[consoleId] || 0} onOpen={opener(`disk-${disk.name}`)} />
                            )} />
                    </Section>
                    <Section title={t("gameAdmin.serversTitle")} help={(servers || []).length ? t("gameAdmin.serversHelp") : null}>
                        <Listing items={servers || []} empty={t("gameAdmin.noServer")} page={read("serversPage")} onPage={(page) => update({ serversPage: page })}
                            render={(server) => {
                                const answer = lent[server.id];
                                const refused = Boolean(answer) && !answer.ok;

                                return (
                                    <NavRow key={server.id} id={`gs-server-${server.id}`} icon="bi-arrow-left-right" tone={refused ? "slate" : "green"} label={serverName(server, server.id, t)}
                                        help={refused ? refusalOf(answer, t) : null} value={answer?.ok ? (answer.data || []).length : null} disabled={refused} onOpen={opener(`server-${server.id}`)} />
                                );
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
const MEGABYTE = 1024 * 1024;

function SourcePage() {
    const { t } = useTranslation();
    const { consoleId, source: slug } = useParams();
    const { open } = useSettingsNavigation();
    const source = sourceOf(slug);
    const { name } = useSourceNames(slug);
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const [query, setQuery] = useState(search.trim());
    const [answer, setAnswer] = useState(null);
    const levels = useLevels(consoleId, slug);
    const peer = source?.kind === "server";
    // Les jeux de cette console qui attendent leur tour ou descendent (la file des jeux) : leur ligne le dit.
    const downloads = useDownloads();
    const minSize = Math.max(0, Number(read("minSize")) || 0);
    const heaviest = read("sort") === "size";
    const games = (answer?.games || []).filter((game) => !minSize || (game.sizeBytes || 0) >= minSize * MEGABYTE);

    if (heaviest) {
        games.sort((left, right) => (right.sizeBytes || 0) - (left.sizeBytes || 0));
    }

    // La recherche part un instant après la dernière lettre.
    useEffect(() => {
        const timer = setTimeout(() => setQuery(search.trim()), 300);

        return () => clearTimeout(timer);
    }, [search]);

    useEffect(() => {
        if (!source) {
            return undefined;
        }
        let cancelled = false;
        setAnswer(null);
        readGames(consoleId, source, query).then((result) => !cancelled && setAnswer(result));

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, query]);

    return (
        <SettingsPage title={name} parent={gamesPath(consoleId)} levels={levels}>
            {!source ? <NotFound /> : (
                <Section title={t("gameAdmin.listTitle")}>
                    {answer && !answer.games ? <Note className="mt-0">{refusalOf(answer.failure, t)}</Note> : (
                        <Listing items={games} loading={answer === null} search={search} onSearch={(q) => update({ q })} page={read("page")} onPage={(page) => update({ page })}
                            placeholder={t("gameAdmin.search")} found={t("gameAdmin.found", { count: games.length })} empty={t(minSize ? "gameAdmin.noneHeavier" : "gameAdmin.none", { size: minSize })}
                            filterCount={(minSize ? 1 : 0) + (heaviest ? 1 : 0)} filterTitle={t("gameAdmin.filterTitle")} filterHelp={t("gameAdmin.filterHelp")} filters={(
                                <>
                                    <RangeRow id="gs-min-size" min={0} max={1000} step={10} label={t("gameAdmin.minSize")} help={t("gameAdmin.minSizeHelp")}
                                        value={minSize} onChange={(value) => update({ minSize: Number(value) > 0 ? Math.round(Number(value)) : null, page: null })} />
                                    <SwitchRow id="gs-heaviest" label={t("gameAdmin.heaviest")} help={t("gameAdmin.heaviestHelp")} checked={heaviest}
                                        onChange={(on) => update({ sort: on ? "size" : null, page: null })} />
                                </>
                            )}
                            placeholderRow={() => <PlaceholderRow lead="cover" />}
                            render={(game) => {
                                const key = `gl-${game.reference}`;
                                const downloading = peer ? downloads.find((entry) => entry.key === `roms/${consoleId}/${game.reference}`) : null;

                                return (
                                    <NavRow key={game.reference} id={key} icon="bi-disc" tone={peer && game.here ? "green" : "slate"} label={game.title}
                                        lead={<GameCover consoleId={consoleId} reference={game.reference} game={gameSourceOf(source)} />}
                                        help={[peer && game.here ? t("gameAdmin.alreadyHere") : null, downloadState(downloading, t), formatSize(game.sizeBytes)].filter(Boolean).join(" · ")}
                                        onOpen={() => open(gamePagePath(consoleId, slug, game.reference), key, { game })} />
                                );
                            }} />
                    )}
                </Section>
            )}
        </SettingsPage>
    );
}

// UN JEU D'ICI (le modèle 23) : ses informations (sa fiche, son identifiant, sa taille) ; « Plus » — son correctif (lu à
// part, `…/patch` : installé ou non ; grisé sans identifiant de jeu), son apparence, les corrections de sa fiche ; le
// retirer de son disque ou de la bibliothèque, dans « Danger » (ses sauvegardes restent : deleteRom n'y touche pas).
function LocalGame({ consoleId, slug, source, game }) {
    const { t } = useTranslation();
    const { refreshCounts } = useData();
    const { open, back } = useSettingsNavigation();
    const gameSource = gameSourceOf(source);
    const metadata = useGameMetadata(consoleId, game.reference, gameSource);
    const [patch, setPatch] = useState(undefined);
    const disk = source.kind === "disk" ? source.id : null;
    const missingId = Boolean(patch) && !patch.gameId;

    useEffect(() => {
        let cancelled = false;
        gamePatch(consoleId, game.reference, gameSource).then((answer) => !cancelled && setPatch(answer.ok ? answer.data : null));

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, game.reference]);

    const page = (name, key) => () => open(gamePagePath(consoleId, slug, game.reference, name), key, { game });
    const joined = (values) => (values || []).join(", ");

    return (
        <>
            <Section title={t("gameAdmin.infos")}>
                {metadata === undefined ? <Note className="mt-0">{t("settings.loading")}</Note> : (
                    <Rows>
                        <InfoRow icon="bi-calendar3" label={t("gameAdmin.fields.releaseDate")} value={metadata?.releaseDate} />
                        <InfoRow icon="bi-building" label={t("gameAdmin.fields.publishers")} value={joined(metadata?.publishers)} />
                        <InfoRow icon="bi-code-slash" label={t("gameAdmin.fields.developers")} value={joined(metadata?.developers)} />
                        <InfoRow icon="bi-tags" label={t("gameAdmin.fields.genres")} value={joined(metadata?.genres)} />
                        <InfoRow icon="bi-collection" label={t("gameAdmin.fields.series")} value={metadata?.series} />
                        <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={patch?.gameId} />
                        <InfoRow icon="bi-hdd" label={t("game.size")} value={formatSize(game.sizeBytes)} />
                    </Rows>
                )}
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="gm-patch" icon="bi-bandaid" tone={missingId ? "slate" : "amber"} label={t("gameAdmin.patch")} help={t(missingId ? "gameAdmin.noGameId" : "gameAdmin.patchHelp")}
                        value={patch?.gameId ? t(patch.installed ? "consoles.installed" : "consoles.missing") : null} disabled={missingId} onOpen={page("patch", "gm-patch")} />
                    <NavRow id="gm-appearance" icon="bi-palette" tone="violet" label={t("consoles.appearance")} help={t("gameAdmin.appearanceHelp")} onOpen={page("appearance", "gm-appearance")} />
                    <NavRow id="gm-infos" icon="bi-pencil" label={t("gameAdmin.fixInfos")} help={t("gameAdmin.fixInfosHelp")} onOpen={page("infos", "gm-infos")} />
                    <NavRow id="gm-match" icon="bi-search" label={t("gameAdmin.wrongGame")} help={t("gameAdmin.matchHelp")} onOpen={page("match", "gm-match")} />
                    <NavRow id="gm-forget" icon="bi-eraser" label={t("gameAdmin.meta.forget")} help={t("gameAdmin.cacheHelp")} onOpen={page("forget", "gm-forget")} />
                </Rows>
            </Section>
            {/* Réinitialiser la fiche couvre ses textes et ses images (les pages « Corriger les informations » et
                « Apparence ») : elle est ici, sur la page qui les ouvre toutes deux. */}
            <DangerSection>
                <GameReset consoleId={consoleId} reference={game.reference} game={gameSource} title={metadata?.title || game.title} />
                <DangerRow id="gm-remove" label={t(disk ? "gameAdmin.removeFromDisk" : "gameAdmin.remove")} help={disk ? t("gameAdmin.removeDiskHelp", { name: disk }) : t("gameAdmin.removeHelp")}
                    question={t("gameAdmin.removeConfirm", { name: game.title })} onConfirm={async () => {
                        // LA SUPPRESSION EST UNE TÂCHE SUIVIE (le porteur, 2026-11-17 : « La suppression des gros jeux
                        // c'est long ») : acceptée, la page revient à la liste et le dit ; sa progression est dans
                        // En direct › Notifications ; sa fin, un avis, et les compteurs relus.
                        const answer = await deleteRom(consoleId, game.reference, disk);

                        if (!answer.ok || !answer.data?.downloadId) {
                            toast(answer.errorMessage || t("consoles.removeFailed"));

                            return;
                        }
                        toast(t("consoles.removing", { name: game.title }));
                        back(sourcePath(consoleId, slug));
                        followDownload(answer.data.downloadId).then((done) => {
                            toast(done.ok ? t("consoles.removed", { name: game.title }) : done.errorMessage || t("consoles.removeFailed"));
                            refreshCounts();
                        });
                    }} />
            </DangerSection>
        </>
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
// L'icône du genre d'une source (chantier 61), celles de gameflex-federation.
const SOURCE_ICONS = { disk: "bi-device-hdd", smb: "bi-hdd-network", webdav: "bi-cloud", kdrive: "bi-cloud-fill", ftp: "bi-folder-symlink", sftp: "bi-shield-lock" };

function PeerGame({ consoleId, source, from, game, reload }) {
    const { t } = useTranslation();
    const { refreshCounts } = useData();
    const confirm = useConfirm();
    const gameSource = gameSourceOf(source);
    const metadata = useGameMetadata(consoleId, game.reference, gameSource);
    const failure = metadata === null ? gameMetadataStatus(consoleId, game.reference, gameSource) : null;
    const joined = (values) => (values || []).join(", ");
    const key = `roms/${consoleId}/${game.reference}`;
    const running = useDownloads().find((entry) => entry.key === key) || null;
    const wasRunning = useRef(Boolean(running));
    const size = formatSize(game.sizeBytes);
    // LES SOURCES D'UN JEU (chantier 61) : un serveur gameflex-federation rassemble plusieurs partages, et un jeu peut
    // être dans plusieurs — ses sources arrivent de la meilleure à la moins bonne (disponibilité, adéquation, débit).
    // La première est choisie ; la page en prend une autre, que le téléchargement demande (`source`).
    const sources = game.sources || [];
    const [chosen, setChosen] = useState(sources[0]?.id || "");
    const [copying, setCopying] = useState(false);

    useEffect(() => {
        if (wasRunning.current && !running) {
            reload();
            refreshCounts();
        }
        wasRunning.current = Boolean(running);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [running]);

    const start = async () => {
        const answer = await runDownload({ key, label: game.title, source: `server:${source.id}`, onQueued: (position) => toast(queuedNotice(game.title, position, t)), command: {
            route: "server", server: source.id, section: "roms",
            body: { consoleId, reference: peerReference(game.reference), title: game.title, files: game.files || [], folder: Boolean(game.folder),
                ...(sources.length > 1 && chosen ? { source: chosen } : {}) },
            ...(game.sizeBytes ? { total: game.sizeBytes } : {}),
        } });
        toast(downloadOutcome(answer, game.title, t));
    };

    const copySheet = async () => {
        setCopying(true);
        const answer = await copyFederatedSheet(consoleId, game.reference, gameSource);
        setCopying(false);
        toast(answer.ok ? t("gameAdmin.sheetCopied", { name: metadata?.title || game.title }) : answer.errorMessage || t("settings.actionFailed"));
    };

    const askSheet = () => confirm({
        title: t("gameAdmin.copySheet"),
        message: t("gameAdmin.copySheetConfirm", { name: metadata?.title || game.title, source: from }),
        confirmLabel: t("gameAdmin.copySheetDo"),
        onConfirm: copySheet,
    });

    const ask = () => confirm({
        title: t("consoles.fetchFrom", { source: from }),
        message: t(size ? "gameAdmin.downloadConfirmSized" : "gameAdmin.downloadConfirm", { name: game.title, size }),
        confirmLabel: t("consoles.download"),
        onConfirm: start,
    });

    return (
        <>
            <Section title={t("gameAdmin.infos")}>
                <Rows>
                    {game.here && <InfoRow icon="bi-check2-circle" tone="green" label={t("games.status")} value={t("gameAdmin.alreadyHere")} />}
                    <InfoRow icon="bi-arrow-left-right" tone="green" label={t("game.origin")} value={from} />
                    <InfoRow icon="bi-hdd" label={t("game.size")} value={size} />
                    {metadata === undefined && <PlaceholderRow help={false} chevron={false} />}
                    {failure !== null && <EmptyRow icon="bi-journal-x" text={sheetRefusalOf(failure, from, t)} />}
                    <InfoRow icon="bi-calendar3" label={t("gameAdmin.fields.releaseDate")} value={metadata?.releaseDate} />
                    <InfoRow icon="bi-building" label={t("gameAdmin.fields.publishers")} value={joined(metadata?.publishers)} />
                    <InfoRow icon="bi-code-slash" label={t("gameAdmin.fields.developers")} value={joined(metadata?.developers)} />
                    <InfoRow icon="bi-tags" label={t("gameAdmin.fields.genres")} value={joined(metadata?.genres)} />
                    <InfoRow icon="bi-collection" label={t("gameAdmin.fields.series")} value={metadata?.series} />
                    <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={metadata?.gameId} />
                </Rows>
            </Section>
            {sources.length > 1 && (
                <Section title={t("gameAdmin.sourcesTitle")} help={t("gameAdmin.sourcesHelp")}>
                    <Rows>
                        <RadioRow id="gm-source" value={chosen} onChange={setChosen} options={sources.map((option, index) => ({
                            value: option.id,
                            icon: SOURCE_ICONS[option.kind] || "bi-hdd-network",
                            label: option.name,
                            help: [t(`gameAdmin.sourceKinds.${option.kind}`, { defaultValue: option.kind }), option.sizeBytes ? formatSize(option.sizeBytes) : "",
                                index === 0 ? t("gameAdmin.sourceBest") : "", option.pinned ? t("gameAdmin.sourcePinned") : "",
                                option.unavailable ? t("gameAdmin.sourceUnavailable") : ""].filter(Boolean).join(" · "),
                        }))} />
                    </Rows>
                </Section>
            )}
            <Section title={t("more")}>
                <Rows>
                    {running ? <DownloadRow entry={running} id="gm-download" label={t("gameAdmin.download")} />
                        : <NavRow id="gm-download" icon="bi-download" tone={game.here ? "slate" : "indigo"} label={t("gameAdmin.download")}
                            help={game.here ? t("gameAdmin.alreadyHereHelp") : t(size ? "gameAdmin.downloadHelp" : "gameAdmin.downloadHelpUnsized", { size, source: from })}
                            disabled={Boolean(game.here)} opens="confirm" onOpen={ask} />}
                    <NavRow id="gm-copy-sheet" icon="bi-journal-arrow-down" tone={game.here || failure !== null || metadata === undefined ? "slate" : "teal"} label={t("gameAdmin.copySheet")}
                        help={game.here ? t("gameAdmin.copySheetHere") : failure !== null ? sheetRefusalOf(failure, from, t) : t("gameAdmin.copySheetHelp", { source: from })}
                        pending={copying} disabled={Boolean(game.here) || failure !== null || metadata === undefined} opens="confirm" onOpen={askSheet} />
                </Rows>
            </Section>
        </>
    );
}

// /…/games/<source>/game?ref= : UN JEU — d'ici (LocalGame) ou d'un serveur fédéré (PeerGame).
function GamePage() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game, reload } = useAddressedGame();
    const { name } = useSourceNames(slug);
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug);

    return (
        <SettingsPage title={title || reference} parent={sourcePath(consoleId, slug)} levels={levels}>
            {game === undefined ? <Section title={t("gameAdmin.infos")}><Note className="mt-0">{t("settings.loading")}</Note></Section>
                : game === null ? <NotFound />
                    : source.kind === "server" ? <PeerGame consoleId={consoleId} source={source} from={name} game={game} reload={reload} />
                        : <LocalGame consoleId={consoleId} slug={slug} source={source} game={game} />}
        </SettingsPage>
    );
}

// « LE RÉCUPÉRER » pour un correctif absent (le modèle 26) : chaque disque interne, puis chaque serveur fédéré, comme le
// serveur les rend (storage_patches_api.go) — grisés quand ils ne l'ont pas, avec leur raison. Toucher une source ouvre
// la fenêtre, puis la commande `download` (route `disk` ou `server`, section `patches`) ; sa ligne montre la
// progression, les autres sont grisées le temps qu'il dure. Elles se feuillettent, la page tenue par la liste.
function PatchSources({ consoleId, view, gameTitle, running }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    // LA SOURCE OFFICIELLE (chantier 63, P6) : le paquet des correctifs de l'index des paquets streamex.
    const keyOf = (source) => (source.source === "official" ? "official" : source.source === "disk" ? `disk:${source.disk}` : `server:${source.serverId}`);
    const labelOf = (source) => (source.source === "official" ? t("consoles.sourceStreamex")
        : source.source === "disk" ? source.disk : serverName({ hostname: source.name }, source.serverId, t));
    // La raison d'un serveur qui ne répond pas comme il faut est le message de son refus (remote.Refusal) : notre jeton
    // refusé (« does not allow access »), ou le serveur muet.
    const reasonOf = (source) => (!source.reason ? t("consoles.sourceLacks")
        : t(source.source !== "official" && /allow/i.test(source.reason) ? "consoles.sourceDenied" : "consoles.sourceUnreachable"));

    const start = async (source) => {
        const body = { consoleId, name: view.gameId };
        const command = source.source === "official" ? { route: "official", section: "patches", body }
            : source.source === "disk" ? { route: "disk", disk: source.disk, section: "patches", body }
                : { route: "server", server: String(source.serverId), section: "patches", body };
        const answer = await runDownload({ key: `patches/${consoleId}/${view.gameId}`, label: gameTitle || view.gameId, source: keyOf(source),
            command: { ...command, ...(source.sizeBytes ? { total: source.sizeBytes } : {}) } });
        toast(downloadOutcome(answer, view.gameId, t));
    };

    const ask = (source) => {
        const size = formatMegabytes(source.sizeBytes);
        const from = labelOf(source);

        confirm({
            title: t("consoles.fetchFrom", { source: from }),
            message: t(size ? "gameAdmin.patchConfirmSized" : "gameAdmin.patchConfirm", { name: gameTitle || view.gameId, size, source: from }),
            confirmLabel: t("consoles.download"),
            onConfirm: () => start(source),
        });
    };

    const row = (source) => {
        const own = running?.source === keyOf(source);
        const disk = source.source === "disk";
        const official = source.source === "official";
        const facts = [t(official ? "gameAdmin.officialKind" : disk ? "gameAdmin.diskKind" : "consoles.federatedServer"),
            source.available ? formatMegabytes(source.sizeBytes) : reasonOf(source)];

        return (
            own ? <DownloadRow key={keyOf(source)} entry={running} id={`gp-source-${keyOf(source)}`} label={labelOf(source)} />
                : <NavRow key={keyOf(source)} id={`gp-source-${keyOf(source)}`} icon={official ? "bi-patch-check" : disk ? "bi-hdd" : "bi-arrow-left-right"}
                    tone={!source.available || running ? "slate" : official ? "teal" : disk ? "indigo" : "green"} label={labelOf(source)}
                    help={facts.filter(Boolean).join(" · ")}
                    disabled={!source.available || Boolean(running)} opens="confirm" onOpen={() => ask(source)} />
        );
    };

    return (
        <Section title={t("consoles.fetchTitle")} help={view.sources.length ? t("gameAdmin.patchFetchHelp") : null}>
            <Listing items={view.sources} empty={t("gameAdmin.patchNoSource")} render={row} />
        </Section>
    );
}

// /…/games/<source>/patch?ref= : LE CORRECTIF D'UN JEU, comme un émulateur (le modèle 26) — `patches/<console>/<id>/`,
// lu par `…/roms/{ref}/patch` (storage_patches_api.go, handleGamePatch).
//   - Détails : l'identifiant du jeu, son statut, son dossier ; installé, ses fichiers et sa taille ;
//   - installé : le supprimer (« Danger »), puis retour au jeu ;
//   - absent : « Le récupérer » (PatchSources). Fini, la page se relit.
function PatchPage() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const { back } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);
    const [view, setView] = useState(undefined);
    // UNE ROUTE QUE L'HÔTE NE SERT PAS (le téléphone, libs/api.js) : sa raison, au lieu d'un « Introuvable » qui mentirait.
    const [unserved, setUnserved] = useState("");
    const [epoch, setEpoch] = useState(0);
    const running = useDownloads().find((entry) => entry.key === `patches/${consoleId}/${view?.gameId || ""}`) || null;
    const wasRunning = useRef(false);
    const parent = gamePagePath(consoleId, slug, reference);
    const installed = view?.installed || null;

    useEffect(() => {
        if (!isLocal(source)) {
            setView(null);

            return undefined;
        }
        let cancelled = false;
        gamePatch(consoleId, reference, gameSourceOf(source)).then((answer) => {
            if (!cancelled) {
                setView(answer.ok ? answer.data : null);
                setUnserved(answer.errorCode === "not_served" ? answer.errorMessage : "");
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, reference, epoch]);

    useEffect(() => {
        if (wasRunning.current && !running) {
            setEpoch((value) => value + 1);
        }
        wasRunning.current = Boolean(running);
    }, [running]);

    return (
        <SettingsPage title={t("gameAdmin.patch")} parent={parent} levels={levels}>
            {view === undefined ? <Section title={t("details")}><Note className="mt-0">{t("settings.loading")}</Note></Section>
                : unserved ? <Section title={t("details")}><Note className="mt-0">{unserved}</Note></Section>
                    : view === null || game === null ? <NotFound />
                    : !view.gameId ? <Section title={t("details")}><Note className="mt-0">{t("gameAdmin.noGameId")}</Note></Section> : (
                        <>
                            <Section title={t("details")}>
                                <Rows>
                                    <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={view.gameId} />
                                    <InfoRow icon={installed ? "bi-check2-circle" : "bi-x-circle"} tone={installed ? "green" : "slate"} label={t("games.status")}
                                        value={t(installed ? "consoles.installed" : "consoles.missing")} />
                                    <InfoRow icon="bi-folder2" label={t("gameAdmin.patchFolder")} value={view.folder} mono />
                                    {installed && <InfoRow icon="bi-file-earmark-code" label={t("game.files")} mono
                                        value={(installed.files || []).length ? installed.files.map((file) => <span key={file} className="d-block">{file}</span>) : null} />}
                                    {installed && <InfoRow icon="bi-hdd" label={t("game.size")} value={formatMegabytes(installed.sizeBytes)} />}
                                </Rows>
                            </Section>
                            {installed ? (
                                <DangerSection>
                                    <DangerRow id="gm-patch-remove" label={t("gameAdmin.patchRemove")} help={t("gameAdmin.patchRemoveHelp")} question={t("gameAdmin.patchRemoveConfirm", { name: view.gameId })}
                                        onConfirm={async () => {
                                            const answer = await deleteInstalled("patches", consoleId, view.gameId);
                                            toast(answer.ok ? t("gameAdmin.patchRemoved", { name: view.gameId }) : answer.errorMessage || t("settings.actionFailed"));

                                            if (answer.ok) {
                                                back(parent);
                                            }
                                        }} />
                                </DangerSection>
                            ) : <PatchSources consoleId={consoleId} view={view} gameTitle={title} running={running} />}
                        </>
                    )}
        </SettingsPage>
    );
}

// /…/games/<source>/appearance?ref= : LES IMAGES D'UN JEU (une ligne de son « Plus » ; chantier 53, P9, le modèle 31) —
// ses informations, ses images une ligne par sorte (GameAppearance, GameMetadata.jsx).
function AppearancePage() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const { open } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);

    return (
        <SettingsPage title={t("consoles.appearance")} parent={gamePagePath(consoleId, slug, reference)} levels={levels}>
            {!isLocal(source) || game === null ? <NotFound /> : (
                <GameAppearance consoleId={consoleId} reference={reference} game={gameSourceOf(source)} title={title}
                    onOpenImage={(kind) => open(gamePagePath(consoleId, slug, reference, "image", { kind }), `ap-kind-${kind}`, { game })} />
            )}
        </SettingsPage>
    );
}

// Les niveaux au-dessus d'une page sous les images d'un jeu : ceux du jeu, puis « Images ».
const useAppearanceLevels = (consoleId, slug, title) => {
    const { t } = useTranslation();

    return { ...useLevels(consoleId, slug, title), [`${sourcePath(consoleId, slug)}/appearance`]: t("consoles.appearance") };
};

// /…/games/<source>/image?ref=&kind= : une image de la fiche d'un jeu (le modèle 33) — en grand, d'où elle vient ; la
// remplacer (la recherche ; GameImage, GameMetadata.jsx).
function ImagePage() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, params, game } = useAddressedGame();
    const { open } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useAppearanceLevels(consoleId, slug, title);
    const kind = params.get("kind") || "";
    const known = IMAGE_KINDS.includes(kind);

    return (
        <SettingsPage title={known ? t(`gameAdmin.meta.kinds.${kind}`) : kind} parent={gamePagePath(consoleId, slug, reference, "appearance")} levels={levels}>
            {known && isLocal(source) && game !== null ? (
                <GameImage consoleId={consoleId} reference={reference} game={gameSourceOf(source)} kind={kind}
                    onReplace={() => open(gamePagePath(consoleId, slug, reference, "image-search", { kind }), "ap-replace", { game })} />
            ) : <NotFound />}
        </SettingsPage>
    );
}

// /…/games/<source>/image-search?ref=&kind= : REMPLACER L'IMAGE D'UNE SORTE (depuis sa page ; l'image actuelle
// au-dessus) — la recherche, préremplie du titre du jeu (GameImageSearch, GameMetadata.jsx). Posée, retour à la
// page de l'image.
function ImageSearchPage() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, params, game } = useAddressedGame();
    const { back } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const kind = params.get("kind") || "";
    const known = IMAGE_KINDS.includes(kind);
    const levels = { ...useAppearanceLevels(consoleId, slug, title), ...(known ? { [`${sourcePath(consoleId, slug)}/image`]: t(`gameAdmin.meta.kinds.${kind}`) } : {}) };
    const parent = gamePagePath(consoleId, slug, reference, "image", { kind });

    return (
        <SettingsPage title={t("appearance.replaceTitle")} parent={parent} levels={levels}>
            {known && isLocal(source) && game !== null ? (
                <GameImageSearch consoleId={consoleId} reference={reference} game={gameSourceOf(source)} kind={kind} title={title} onDone={() => back(parent)} />
            ) : <NotFound />}
        </SettingsPage>
    );
}

// /…/games/<source>/infos|match|forget?ref= : UNE PAGE DE LA FICHE D'UN JEU D'ICI (GameMetadata.jsx) — corriger
// ses informations (le modèle 24), « Ce n'est pas le bon jeu ? » (25), oublier une source.
function SheetPage({ titleKey, part: Part }) {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);

    return (
        <SettingsPage title={t(titleKey)} parent={gamePagePath(consoleId, slug, reference)} levels={levels}>
            {!isLocal(source) || game === null ? <NotFound /> : <Part consoleId={consoleId} reference={reference} game={gameSourceOf(source)} fallbackTitle={game?.title || ""} />}
        </SettingsPage>
    );
}

// Une adresse sous les jeux qui ne désigne rien : les sources.
function ToSources() {
    const { consoleId } = useParams();

    return <Navigate to={gamesPath(consoleId)} replace />;
}

// /settings/consoles/<console>/games/* (Consoles.jsx, ConsolesRoutes).
export function GamesRoutes() {
    return (
        <Routes>
            <Route index element={<SourcesPage />} />
            <Route path=":source" element={<SourcePage />} />
            <Route path=":source/game" element={<GamePage />} />
            <Route path=":source/patch" element={<PatchPage />} />
            <Route path=":source/appearance" element={<AppearancePage />} />
            <Route path=":source/image" element={<ImagePage />} />
            <Route path=":source/image-search" element={<ImageSearchPage />} />
            <Route path=":source/infos" element={<SheetPage key="infos" titleKey="gameAdmin.fixInfos" part={GameInfoForm} />} />
            <Route path=":source/match" element={<SheetPage key="match" titleKey="gameAdmin.wrongGame" part={GameMatch} />} />
            <Route path=":source/forget" element={<SheetPage key="forget" titleKey="gameAdmin.meta.forget" part={GameForget} />} />
            <Route path="*" element={<ToSources />} />
        </Routes>
    );
}
