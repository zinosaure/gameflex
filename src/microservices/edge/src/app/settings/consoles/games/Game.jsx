// Route : /settings/consoles/<console>/games/<source>/game?ref= — un jeu : d'ici, d'un serveur fédéré, d'une ressource.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { DangerRow } from "../../../bricks/DangerRow.jsx";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { DownloadRow } from "../../../bricks/DownloadRow.jsx";
import { GameFacts } from "../../../bricks/GameFacts.jsx";
import { GameImages } from "../../../bricks/GameImages.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { RadioRow } from "../../../bricks/RadioRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { useConfirm } from "../../../bricks/useConfirm.jsx";
import { copyFederatedSheet, deleteRom, gamePatch } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { downloadOutcome, followDownload, queuedNotice, runDownload, useDownloads } from "../../../libs/downloads.js";
import { isFavorite, setFavorite, useFavorites } from "../../../libs/favorites.js";
import { fact, formatSize, statusOf } from "../../../libs/format.js";
import { gameMetadataStatus, useGameMetadata } from "../../../libs/metadata.js";
import { gameScreenPath } from "../../../libs/tabs.js";
import { splitTitle } from "../../../libs/visuals.js";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { useSavesLink } from "../../catalog/Games.jsx";
import { GameReset } from "../../server/consoles/GameMetadata.jsx";
import { gamePagePath, gameSourceOf, peerReference, sheetRefusalOf, SOURCE_ICONS, sourcePath, useAddressedGame, useGameTitle, useLevels, useSourceNames } from "../Games.jsx";

// LES SECTIONS D'UN JEU, LES MÊMES SUR SES TROIS PAGES (la mise en forme du porteur, 2026-11-24 ; le 2026-11-25 : « les
// sections dépendent du fait qu'on est dans catalogue/jeux ou consoles/jeux. Par exemple le download c'est différent. Dans
// consoles/jeux ça devient add to library ») : ses images, ses détails, puis ce qui suit — d'ici (LocalGame), d'un serveur
// fédéré (PeerGame), d'une ressource externe (ResourceGame). Ce qu'une page ne peut pas faire reste là, désactivé, avec sa
// raison.
//
// « JOUER MAINTENANT OU PLUS TARD » : le jouer — sa fiche à l'écran, pour un jeu d'ici seulement (`here`) — et mes favoris
// (un jeu d'un serveur fédéré aussi : `serverId`).
function PlaySection({ console, game, serverId = null, here }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const favorites = useFavorites();
    const favorite = isFavorite(favorites, console?.id, game.reference);
    const [pending, setPending] = useState(false);

    const toggleFavorite = async () => {
        setPending(true);
        const answer = await setFavorite({ consoleId: console.id, reference: game.reference, serverId, title: game.title }, !favorite);
        setPending(false);
        toast(answer.ok ? t(favorite ? "favorites.removed" : "favorites.added", { name: splitTitle(game.title).main }) : answer.detail || t("favorites.failed"));
    };

    return (
        <Section title={t("gameAdmin.sections.play")} help={t("gameAdmin.sections.playHelp")} sense="play">
            <Rows>
                <NavRow id="gm-library" icon="bi-play" label={t("gameAdmin.play")} help={here ? t("gameAdmin.openInLibraryHelp") : t("gameAdmin.playNotHere")}
                    disabled={!here || !console} onOpen={() => navigate(gameScreenPath(console, game.reference))} />
                <NavRow id="gm-favorite" icon={favorite ? "bi-star-fill" : "bi-star"} label={t(favorite ? "favorites.remove" : "favorites.add")} help={t("favorites.help")}
                    pending={pending || !favorites.loaded} disabled={!console} opens="action" onOpen={toggleFavorite} />
            </Rows>
        </Section>
    );
}

// « PLUS » : ses sauvegardes (chez sa source, Mes sauvegardes ; désactivée, sa raison dessous, quand il n'en a pas) et « Ce
// n'est pas le bon jeu ? » (`onMatch` ; désactivé pour un jeu d'ailleurs : sa fiche se corrige chez lui).
function MoreSection({ console, game, serverId = null, metadata, onMatch = null }) {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const saves = useSavesLink(console, { ...game, peer: serverId }, metadata);

    return (
        <Section title={t("more")} help={t("gameAdmin.sections.moreHelp")} sense="optional">
            <Rows>
                <NavRow id="gm-saves" icon="bi-shield-check" label={t("catalog.saves.label")}
                    help={saves === undefined ? t("settings.loading") : saves.path ? saves.help : saves.reason} pending={saves === undefined} disabled={!saves?.path}
                    onOpen={() => open(saves.path, "gm-saves")} />
                <NavRow id="gm-match" icon="bi-search" label={t("gameAdmin.wrongGame")} help={onMatch ? t("gameAdmin.matchHelp") : t("gameAdmin.matchElsewhere")}
                    disabled={!onMatch} onOpen={onMatch || undefined} />
            </Rows>
        </Section>
    );
}

// UN JEU D'ICI (le modèle 23) : ses informations (sa fiche, son identifiant, sa taille) ; « Plus » — son correctif (lu à
// part, `…/patch` : installé ou non ; grisé sans identifiant de jeu), son apparence, les corrections de sa fiche ; le
// retirer de son disque ou de la bibliothèque, dans « Danger » (ses sauvegardes restent : deleteRom n'y touche pas).
function LocalGame({ consoleId, slug, source, game }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { refreshCounts, consoles } = useData();
    const console = consoles.find((entry) => entry.id === consoleId);
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

    return (
        <>
            <GameImages consoleId={consoleId} reference={game.reference} game={gameSource} />
            <Section title={t("details")} help={t("gameAdmin.sections.detailsHelp")} sense="info">
                <GameFacts facts={{ console: console?.name, origin: disk || t("consoles.sourceHere"), status: statusOf(game) || "untested", gameId: patch?.gameId, sizeBytes: game.sizeBytes }} metadata={metadata} />
            </Section>
            <Section title={t("gameAdmin.sections.appearance")} help={t("gameAdmin.sections.appearanceHelp")} sense="modify">
                <Rows>
                    <NavRow id="gm-patch" icon="bi-bandaid" label={t("gameAdmin.patch")} help={t(missingId ? "gameAdmin.noGameId" : "gameAdmin.patchHelp")}
                        value={patch?.gameId ? t(patch.installed ? "consoles.installed" : "consoles.missing") : null} valueTone={patch?.installed ? "success" : "danger"} disabled={missingId} onOpen={page("patch", "gm-patch")} />
                    <NavRow id="gm-appearance" icon="bi-palette" label={t("consoles.appearance")} help={t("gameAdmin.appearanceHelp")} onOpen={page("appearance", "gm-appearance")} />
                    <NavRow id="gm-infos" icon="bi-pencil" label={t("gameAdmin.fixInfos")} help={t("gameAdmin.fixInfosHelp")} onOpen={page("infos", "gm-infos")} />
                </Rows>
            </Section>
            <PlaySection console={console} game={game} here />
            {/* « AJOUTER À LA BIBLIOTHÈQUE » (au lieu du « Télécharger » du Catalogue) : un jeu d'ici y est déjà. */}
            <Section title={t("gameAdmin.sections.library")} help={t("gameAdmin.sections.libraryHelp")} sense="add">
                <Rows>
                    <NavRow id="gm-download" icon="bi-download" label={t("gameAdmin.download")} help={t("gameAdmin.alreadyHereHelp")} disabled onOpen={() => {}} />
                </Rows>
            </Section>
            <MoreSection console={console} game={game} metadata={metadata} onMatch={page("match", "gm-match")} />
            {/* Réinitialiser la fiche couvre ses textes et ses images (les pages « Corriger les informations » et
                « Apparence ») : elle est ici, sur la page qui les ouvre toutes deux. */}
            <DangerSection>
                <NavRow id="gm-forget" icon="bi-eraser" label={t("gameAdmin.meta.forget")} help={t("gameAdmin.cacheHelp")} onOpen={page("forget", "gm-forget")} />
                <GameReset consoleId={consoleId} reference={game.reference} game={gameSource} title={metadata?.title || game.title} />
                <DangerRow id="gm-remove" label={t(disk ? "gameAdmin.removeFromDisk" : "gameAdmin.remove")} help={disk ? t("gameAdmin.removeDiskHelp", { name: disk }) : t("gameAdmin.removeHelp")}
                    question={t("gameAdmin.removeConfirm", { name: game.title })} onConfirm={async () => {
                        // LA SUPPRESSION EST UNE TÂCHE SUIVIE (le porteur, 2026-11-17 : « La suppression des gros jeux
                        // c'est long ») : acceptée, la page revient à la liste et le dit ; sa progression est dans
                        // En direct › Notifications ; sa fin, un avis, et les compteurs relus.
                        const answer = await deleteRom(consoleId, game.reference, disk);

                        if (!answer.ok || !answer.data?.downloadId) {
                            feedback.failed(answer.errorMessage || t("consoles.removeFailed"));

                            return;
                        }
                        toast(t("consoles.removing", { name: game.title }));
                        back(sourcePath(consoleId, slug));
                        followDownload(answer.data.downloadId).then((done) => {
                            feedback.result(done.ok, t("consoles.removed", { name: game.title }), done.errorMessage || t("consoles.removeFailed"));
                            refreshCounts();
                        });
                    }} />
            </DangerSection>
        </>
    );
}


function PeerGame({ consoleId, source, from, game, reload }) {
    const { t } = useTranslation();
    const { refreshCounts, consoles } = useData();
    const console = (consoles || []).find((entry) => entry.id === consoleId);
    const confirm = useConfirm();
    const gameSource = gameSourceOf(source);
    const metadata = useGameMetadata(consoleId, game.reference, gameSource);
    const failure = metadata === null ? gameMetadataStatus(consoleId, game.reference, gameSource) : null;
    const key = `roms/${consoleId}/${game.reference}`;
    const running = useDownloads().find((entry) => entry.key === key) || null;
    const wasRunning = useRef(Boolean(running));
    const size = formatSize(game.sizeBytes);
    // LES SOURCES D'UN JEU (chantier 61, `#808`) : un serveur prête aussi ses ressources externes, et un jeu peut
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
        const answer = await runDownload({
            key, label: game.title, source: `server:${source.id}`, onQueued: (position) => toast(queuedNotice(game.title, position, t)), command: {
                route: "server", server: source.id, section: "roms",
                body: {
                    consoleId, reference: peerReference(game.reference), title: game.title, files: game.files || [], folder: Boolean(game.folder),
                    ...(sources.length > 1 && chosen ? { source: chosen } : {})
                },
                ...(game.sizeBytes ? { total: game.sizeBytes } : {}),
            }
        });
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
            <GameImages consoleId={consoleId} reference={game.reference} game={gameSource} />
            <Section title={t("details")} help={t("gameAdmin.sections.detailsHelp")} sense="info">
                <GameFacts facts={{ console: console?.name, here: game.here, origin: from, sizeBytes: game.sizeBytes }} metadata={metadata} refusal={failure !== null ? sheetRefusalOf(failure, from, t) : null} />
            </Section>
            {sources.length > 1 && (
                <Section title={t("gameAdmin.sourcesTitle")} help={t("gameAdmin.sourcesHelp")} sense="modify">
                    <Rows>
                        <RadioRow id="gm-source" value={chosen} onChange={setChosen} options={sources.map((option, index) => ({
                            value: option.id,
                            icon: SOURCE_ICONS[option.kind] || "bi-hdd-network",
                            label: option.name,
                            help: [fact(t("facts.type"), t(`gameAdmin.sourceKinds.${option.kind}`, { defaultValue: option.kind })), fact(t("game.size"), option.sizeBytes ? formatSize(option.sizeBytes) : ""),
                            option.unavailable ? t("gameAdmin.sourceUnavailable") : option.pinned ? t("gameAdmin.sourcePinned") : index === 0 ? t("gameAdmin.sourceBest") : ""],
                        }))} />
                    </Rows>
                </Section>
            )}
            <PlaySection console={console} game={game} serverId={source.id} here={false} />
            <Section title={t("gameAdmin.sections.library")} help={t("gameAdmin.sections.libraryHelp")} sense="add">
                <Rows>
                    {running ? <DownloadRow entry={running} id="gm-download" label={t("gameAdmin.download")} />
                        : <NavRow id="gm-download" icon="bi-download" label={t("gameAdmin.download")}
                            help={game.here ? t("gameAdmin.alreadyHereHelp") : t(size ? "gameAdmin.downloadHelp" : "gameAdmin.downloadHelpUnsized", { size, source: from })}
                            disabled={Boolean(game.here)} opens="confirm" onOpen={ask} />}
                    <NavRow id="gm-copy-sheet" icon="bi-journal-arrow-down" label={t("gameAdmin.copySheet")}
                        help={game.here ? t("gameAdmin.copySheetHere") : failure !== null ? sheetRefusalOf(failure, from, t) : t("gameAdmin.copySheetHelp", { source: from })}
                        pending={copying} disabled={Boolean(game.here) || failure !== null || metadata === undefined} opens="confirm" onOpen={askSheet} />
                </Rows>
            </Section>
            <MoreSection console={console} game={game} serverId={source.id} metadata={metadata} />
        </>
    );
}

// UN JEU D'UNE RESSOURCE EXTERNE (`#808`) : d'où il vient, sa taille, s'il est déjà ici ; « Télécharger dans la
// bibliothèque », par la file des jeux (la commande `download`, route `resource`), comme le jeu d'un ami. Pas de fiche à
// copier : une ressource n'en a pas — la bibliothèque fera la sienne.
function ResourceGame({ consoleId, source, from, game, reload }) {
    const { t } = useTranslation();
    const { refreshCounts, consoles } = useData();
    const console = (consoles || []).find((entry) => entry.id === consoleId);
    const confirm = useConfirm();
    const key = `roms/${consoleId}/${game.reference}`;
    const running = useDownloads().find((entry) => entry.key === key) || null;
    const wasRunning = useRef(Boolean(running));
    const size = formatSize(game.sizeBytes);

    useEffect(() => {
        if (wasRunning.current && !running) {
            reload();
            refreshCounts();
        }
        wasRunning.current = Boolean(running);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [running]);

    const start = async () => {
        const answer = await runDownload({
            key, label: game.title, source: `resource:${source.id}`, onQueued: (position) => toast(queuedNotice(game.title, position, t)), command: {
                route: "resource", resource: source.id, section: "roms",
                body: { consoleId, reference: game.reference, title: game.title, files: game.files || [], folder: Boolean(game.folder) },
                ...(game.sizeBytes ? { total: game.sizeBytes } : {}),
            }
        });
        toast(downloadOutcome(answer, game.title, t));
    };

    const ask = () => confirm({
        title: t("consoles.fetchFrom", { source: from }),
        message: t(size ? "gameAdmin.downloadConfirmSized" : "gameAdmin.downloadConfirm", { name: game.title, size }),
        confirmLabel: t("consoles.download"),
        onConfirm: start,
    });

    return (
        <>
            <Section title={t("details")} help={t("gameAdmin.sections.detailsHelp")} sense="info">
                <GameFacts facts={{ console: console?.name, here: game.here, origin: from, sizeBytes: game.sizeBytes }} metadata={null} />
            </Section>
            <PlaySection console={console} game={game} here={false} />
            <Section title={t("gameAdmin.sections.library")} help={t("gameAdmin.sections.libraryHelp")} sense="add">
                <Rows>
                    {running ? <DownloadRow entry={running} id="gm-download" label={t("gameAdmin.download")} />
                        : <NavRow id="gm-download" icon="bi-download" label={t("gameAdmin.download")}
                            help={game.here ? t("gameAdmin.alreadyHereHelp") : t(size ? "gameAdmin.downloadHelp" : "gameAdmin.downloadHelpUnsized", { size, source: from })}
                            disabled={Boolean(game.here)} opens="confirm" onOpen={ask} />}
                </Rows>
            </Section>
        </>
    );
}

// /…/games/<source>/game?ref= : UN JEU — d'ici (LocalGame), d'un serveur fédéré (PeerGame), d'une ressource externe
// (ResourceGame).
export default function Game() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game, reload } = useAddressedGame();
    const { name } = useSourceNames(slug);
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug);

    return (
        <SettingsPage title={title || reference} parent={sourcePath(consoleId, slug)} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {game === undefined ? <Section title={t("details")} sense="info"><Loading /></Section>
                : game === null ? <NotFound />
                    : source.kind === "server" ? <PeerGame consoleId={consoleId} source={source} from={name} game={game} reload={reload} />
                        : source.kind === "resource" ? <ResourceGame consoleId={consoleId} source={source} from={name} game={game} reload={reload} />
                            : <LocalGame consoleId={consoleId} slug={slug} source={source} game={game} />}
        </SettingsPage>
    );
}
