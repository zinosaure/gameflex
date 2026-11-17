import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { dropMetadataSource, fetchGameStatus, fetchMetadataSource, metadataCandidates, putManualImage, reportGameStatus, saveManualMetadata, searchGameImages } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { GAME_STATUSES } from "../../../libs/format.js";
import { forgetGameImages, gamePath, useImage } from "../../../libs/images.js";
import { forgetGameMetadata, setGameMetadata, useGameMetadata } from "../../../libs/metadata.js";
import { toast } from "../../../shell/Shell.jsx";
import { Form, FormButton, Ruleset, useCalendarDays } from "../../../ui/Forms.jsx";
import { ImageSearch, imageOrigin } from "./ImageSearch.jsx";
import NavRow, { BlockRow, ChoiceRow, DangerRow, EmptyRow, IconAddon, InfoRow, Listing, Note, PlaceholderRow, Rows, SubmitRow, Section, SelectRow, TextRow, Thumb, useConfirm } from "../../../ui/Bricks.jsx";

// LES MÉTADONNÉES D'UN JEU, CORRIGÉES À LA MAIN (les gestes du kernel, 869923ad) : les pages du « Plus » d'un jeu
// d'ici (Réglages › Consoles › une console › Jeux, Games.jsx ; chantier 53, P8, modèles 23 à 25), une page
// chacune :
//
//   « Corriger les informations » (GameInfoForm) — chaque champ fusionné avec sa source ; le modifier (une liste :
//     séparée par des virgules ; la sortie : sa précision, puis l'année ou le jour) l'enregistre comme correction
//     manuelle, qui gagne ; l'effacer (l'icône accolée) l'efface. Puis, sous son propre formulaire, le statut du jeu
//     (GameStatusForm) ;
//   « Apparence » (GameAppearance ; chantier 53, P9, le modèle 31) — ses informations, une ligne par sorte d'image, en
//     ajouter une, les remettre par défaut ; chacune ouvre sa page (GameImage), où elle se remplace par une image
//     CHOISIE dans une recherche (GameImageSearch ; le porteur, 2026-11-14, remarque d : plus d'envoi de fichier). Elle
//     ne se supprime pas seule (le porteur, 2026-11-14 : « On ne peut que la remplacer. ») : seule « Réinitialiser les
//     images par défaut » rend celles des services ;
//   « Ce n'est pas le bon jeu ? » (GameMatch) — par service : ses candidats (nom, année, plateformes, jaquette s'il en
//     a), en désigner un (gardé d'une relecture à l'autre), ou refaire la recherche par le titre ; puis chaque serveur
//     fédéré (chantier 57, P3) : ses jeux du même identifiant puis du titre, leur jaquette par le kernel — en désigner
//     un copie sa fiche ici (la source `server`), qui reste quand il s'éteint ;
//   « Oublier une source » (GameForget) — choisie dans un sélecteur : son cache, images comprises ; relue à la
//     lecture suivante (sauf la main) ; la fiche copiée d'un serveur fédéré part, et ne revient que désignée.
//
// Chaque groupe a son titre, et son bouton dans son encadré (le porteur, 2026-11-13 : « le bouton doit être dans la
// section card avec les autres form controls ») ; toute confirmation est la fenêtre de Skeleton (useConfirm).
//
// Les erreurs des services se disent telles quelles : pas de compte (409), service injoignable (502).

const TEXT_FIELDS = [["title", false], ["releaseDate", false], ["publishers", true], ["developers", true], ["genres", true], ["series", false]];
export const IMAGE_KINDS = ["cover", "background", "logo", "title", "screenshot", "icon"];
const SERVICES = ["igdb", "steamgriddb"];
// Les sources qu'on oublie : leur cache se refait à la lecture suivante — la fiche copiée d'un serveur fédéré
// (`server`), elle, part. Pas la correction à la main (le porteur, 2026-11-14 : « on n'a plus d'option "la correction
// à la main". On ne fait pas de upload directement ») : ses champs s'effacent un à un (GameInfoForm, la gomme), ses
// images se remettent d'un geste (GameAppearance, « Danger »).
const SOURCES = ["igdb", "steamgriddb", "libretro", "server"];

const asText = (value, list) => (list ? (Array.isArray(value) ? value.join(", ") : value || "") : Array.isArray(value) ? value.join(", ") : String(value ?? ""));
const asValue = (text, list) => (list ? text.split(",").map((item) => item.trim()).filter(Boolean) : text.trim());

// L'erreur d'un service, dite clairement — d'un serveur fédéré aussi : il refuse notre jeton, ou ne prête pas encore
// les fiches de ses jeux (`name` : son nom).
const serviceError = (answer, t, name) => (answer.errorCode === "peer_denied" ? t("gameAdmin.meta.peerDenied", { service: name })
    : answer.errorCode === "peer_not_lending" ? t("gameAdmin.meta.peerNotLending", { service: name })
        : answer.errorCode === "service_not_configured" || answer.status === 409 ? t("gameAdmin.meta.notConfigured", { service: name })
            : answer.status === 502 ? t("gameAdmin.meta.unreachable", { service: name }) : answer.errorMessage || t("settings.actionFailed"));

// La jaquette d'un candidat : chez son hôte (un fournisseur), ou par le kernel avec le jeton (un jeu d'un serveur fédéré).
function CandidateCover({ item }) {
    const relayed = useImage(item.coverPath || null);
    const url = item.coverPath ? relayed?.url : item.coverUrl;

    return url ? <img src={url} alt="" className="gf-cover rounded object-fit-cover" /> : null;
}

// Comment la vignette d'une sorte tient : un fond, une capture, un écran-titre la remplissent ; une jaquette, un logo,
// une icône y tiennent entiers.
const fitOf = (kind) => (kind === "background" || kind === "screenshot" || kind === "title" ? "cover" : "contain");

// UNE SORTE D'IMAGE, SA LIGNE : la vignette telle que la fiche la montre ; elle ouvre sa page. Elle ne dit pas d'où vient
// l'image (le porteur, 2026-11-14 : « Les corrections des textes et images remplacer n'est pas considéré comme à la
// main » — un administrateur peut changer le dossier metadata sans passer par l'interface) ; seulement qu'il n'y en a pas.
function KindRow({ consoleId, reference, game, kind, onOpen }) {
    const { t } = useTranslation();
    const image = useImage(gamePath(consoleId, reference, kind, 256, game));

    return (
        <NavRow id={`ap-kind-${kind}`} lead={<Thumb url={image?.url || null} fit={fitOf(kind)} />} label={t(`gameAdmin.meta.kinds.${kind}`)}
            help={image === null ? t("appearance.noImage") : null} onOpen={onOpen} />
    );
}

// LES IMAGES D'UN JEU (Games.jsx, …/games/<source>/appearance?ref= ; le modèle 31, la décision 11) : ses
// informations (le titre, la sortie) ; ses images, une ligne par sorte (`onOpenImage(kind)`), chacune se remplace depuis
// sa page. Plus d'« Ajouter une image » (le porteur, 2026-11-14 : « enleve le lien "Add an image" ») : chaque sorte a
// déjà sa ligne. Pas de « Danger » : la page du jeu a « Réinitialiser la fiche » (GameReset), qui remet ses textes ET
// ses images.
export function GameAppearance({ consoleId, reference, game = {}, title, onOpenImage }) {
    const { t } = useTranslation();
    const metadata = useGameMetadata(consoleId, reference, game);
    const name = metadata?.title || title;

    return (
        <>
            <Section title={t("appearance.gameTitle")}>
                {metadata === undefined ? <Note className="mt-0">{t("settings.loading")}</Note> : (
                    <Rows>
                        <InfoRow icon="bi-fonts" label={t("gameAdmin.fields.title")} value={name} />
                        <InfoRow icon="bi-calendar3" label={t("gameAdmin.fields.releaseDate")} value={metadata?.releaseDate} />
                    </Rows>
                )}
            </Section>
            <Section title={t("appearance.imagesTitle")} help={t("appearance.gameImagesHelp")}>
                <Listing items={IMAGE_KINDS} loading={metadata === undefined} placeholderRow={() => <PlaceholderRow lead="thumb" />}
                    render={(kind) => <KindRow key={kind} consoleId={consoleId} reference={reference} game={game} kind={kind} onOpen={() => onOpenImage(kind)} />} />
            </Section>
        </>
    );
}

// RÉINITIALISER LA FICHE D'UN JEU (le porteur, 2026-11-14 : « Le bouton réinitialiser permet de tout remettre comme
// avant. C'est suffisant ») : une ligne de « Danger » de la page du jeu (Games.jsx, LocalGame), toujours active —
// la fiche ne dit plus ce qui a été corrigé : toutes les corrections du jeu, textes et images, partent, et elle revient à
// ce que donnent les services (`DELETE …/metadata?source=manual`). Sa fenêtre de confirmation, celle de Skeleton.
export function GameReset({ consoleId, reference, game = {}, title }) {
    const { t } = useTranslation();

    return (
        <DangerRow id="gm-reset" icon="bi-arrow-counterclockwise" label={t("gameAdmin.resetSheet")} help={t("gameAdmin.resetSheetHelp")}
            question={t("gameAdmin.resetSheetConfirm", { name: title })} confirmLabel={t("gameAdmin.resetSheetAction")} onConfirm={async () => {
                const answer = await dropMetadataSource(consoleId, reference, game, "manual");
                toast(answer.ok ? t("gameAdmin.resetSheetDone", { name: title }) : answer.errorMessage || t("settings.actionFailed"));

                if (answer.ok) {
                    reread(consoleId, reference, game);
                }
            }} />
    );
}

// L'image d'une sorte, en grand : sa page (« Aperçu », qui dit « Aucune image ») et sa recherche (« L'image actuelle »).
function KindPreview({ image }) {
    const { t } = useTranslation();

    return (
        <div className="ratio ratio-4x3 rounded-4 overflow-hidden bg-body-secondary">
            {image ? <img className="object-fit-contain" src={image.url} alt="" /> : <span className="d-flex align-items-center justify-content-center small text-body-tertiary">{image === null ? t("appearance.noImage") : ""}</span>}
        </div>
    );
}

// UNE IMAGE DE LA FICHE, SA PAGE (Games.jsx, …/games/<source>/image?ref=&kind= ; le modèle 33) : l'actuelle en
// grand, sa sorte ; « Remplacer cette image » (`onReplace` : la recherche). Pas d'« Origine » : la fiche ne dit plus
// qu'une image a été remplacée (le porteur, 2026-11-14). Pas de « Supprimer » ni de section « Danger » : une image ne se
// supprime pas seule, elle se remplace ; « Réinitialiser la fiche », sur la page du jeu, rend celles des services.
export function GameImage({ consoleId, reference, game = {}, kind, onReplace }) {
    const { t } = useTranslation();
    const image = useImage(gamePath(consoleId, reference, kind, 1024, game));

    return (
        <>
            <Section title={t("appearance.preview")}>
                <KindPreview image={image} />
            </Section>
            <Section title={t("details")}>
                <Rows>
                    <InfoRow icon="bi-image" label={t("appearance.kind")} value={t(`gameAdmin.meta.kinds.${kind}`)} />
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="ap-replace" icon="bi-search" tone="indigo" label={t(image === null ? "appearance.choose" : "appearance.replace")} help={t("appearance.replaceGameHelp")}
                        onOpen={onReplace} />
                </Rows>
            </Section>
        </>
    );
}

// REMPLACER UNE IMAGE D'UN JEU (Games.jsx, …/games/<source>/image-search?ref=&kind= ; le modèle 34, la recherche
// à la place de l'envoi) : « L'image actuelle » d'abord (le porteur, 2026-11-14 : « rajoute l'image de base en haut pour
// savoir quelle image on est en train de remplacer » ; une sorte sans image n'a rien à montrer) ; puis la recherche,
// préremplie du titre du jeu ; une image choisie, la fenêtre, puis le serveur la récupère : elle passe devant celles des
// services (`onDone` : retour d'où l'on venait).
export function GameImageSearch({ consoleId, reference, game = {}, kind, title, onDone }) {
    const { t } = useTranslation();
    const kindName = t(`gameAdmin.meta.kinds.${kind}`);
    // La même adresse que la page de l'image (GameImage) : en cache, elle paraît tout de suite.
    const current = useImage(gamePath(consoleId, reference, kind, 1024, game));

    return (
        <>
            {current !== null && (
                <Section title={t("appearance.current")}>
                    <KindPreview image={current} />
                </Section>
            )}
            <Section title={t("appearance.searchTitle")} help={t("appearance.searchGameHelp", { kind: kindName })}>
                <ImageSearch key={kind} initial={searchTitleOf(title)} fit={fitOf(kind)} search={(query) => searchGameImages(consoleId, reference, game, kind, query)}
                    confirmOf={(image) => ({
                        title: t("appearance.setKindConfirmTitle", { kind: kindName }),
                        message: t("appearance.chosenConfirm", { game: image.game, source: imageOrigin(image, t) }),
                        confirmLabel: t("appearance.setAction"),
                    })}
                    onChoose={async (image) => {
                        const answer = await putManualImage(consoleId, reference, game, kind, image);

                        if (answer.ok) {
                            reread(consoleId, reference, game);
                            toast(t("appearance.imageSet", { kind: kindName }));
                            onDone();
                        }

                        return answer;
                    }} />
            </Section>
        </>
    );
}

// Le titre à chercher : celui du jeu sans ses mentions entre parenthèses ou crochets (la région, la version), comme le
// kernel le cherche (searchTitle).
const searchTitleOf = (title) => String(title || "").replace(/\s*[([][^)\]]*[)\]]/g, " ").replace(/\s+/g, " ").trim();

// Après un changement d'image, de jeu désigné ou de source : la fiche et ses images se relisent, ici et sur la page
// du jeu.
const reread = (consoleId, reference, game) => {
    forgetGameMetadata(consoleId, reference, game);
    forgetGameImages(consoleId, reference);
};

// « CE N'EST PAS LE BON JEU ? » chez un service, son groupe, titré du nom du service (le modèle 25) : le champ de
// recherche (l'icône qui cherche accolée), ses candidats en lignes — en toucher un le désigne —, refaire la recherche
// par le titre (son bouton, dans l'encadré). Un serveur fédéré (`service` « server:<id> », `name` son nom) : ses
// candidats sont ses jeux, leur jaquette par le kernel, et les jeux de ses fournisseurs s'il les partage ; en désigner
// un copie sa fiche ici.
function WrongGame({ consoleId, reference, game, service, name, help = null }) {
    const { t } = useTranslation();
    const peer = service.startsWith("server:");
    const [query, setQuery] = useState("");
    const [items, setItems] = useState(null);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);

    const search = async () => {
        setBusy(true);
        setError("");
        const answer = await metadataCandidates(consoleId, reference, game, service, query.trim());
        setBusy(false);

        if (answer.ok) {
            setItems(answer.data?.items || []);
        } else {
            setItems(null);
            setError(serviceError(answer, t, name));
        }
    };

    const fetchFrom = async (externalId) => {
        setBusy(true);
        const answer = await fetchMetadataSource(consoleId, reference, game, service, externalId);
        setBusy(false);

        if (!answer.ok) {
            toast(answer.status === 404 && !answer.errorCode ? t("gameAdmin.meta.unknownId") : serviceError(answer, t, name));

            return;
        }
        toast(answer.data?.error ? t("gameAdmin.meta.fetchError", { error: answer.data.error }) : t(externalId ? "gameAdmin.meta.pinned" : "gameAdmin.meta.refetched", { name: answer.data?.name || "" }));
        setItems(null);
        reread(consoleId, reference, game);
    };

    return (
        <Section title={name} help={help}>
            <Rows>
                <TextRow id={`mw-q-${service}`} label={t("gameAdmin.meta.candidates")} help={t("gameAdmin.meta.searchHelp")} value={query} onChange={(value) => setQuery(value || "")}
                    props={{ placeholder: t("gameAdmin.meta.searchPlaceholder") }}
                    addon={<IconAddon id={`mw-search-${service}`} icon="bi-search" label={t("gameAdmin.meta.candidates")} disabled={busy} onClick={search} />} />
                {(items || []).map((item, index) => (
                    <NavRow key={item.externalId} id={`mw-pick-${peer ? service.replace(":", "-") : service}-${peer ? index : item.externalId}`} icon="bi-controller" label={item.name} disabled={busy}
                        opens="action" onOpen={() => fetchFrom(item.externalId)} lead={item.coverPath || item.coverUrl ? <CandidateCover item={item} /> : null}
                        help={[item.year, (item.platforms || []).join(", "), item.provider ? t(`gameAdmin.meta.services.${item.provider}`) : peer ? null : `#${item.externalId}`].filter(Boolean).join(" · ")}
                        value={<span className="badge text-bg-primary">{t("gameAdmin.meta.pin")}</span>} />
                ))}
                {items && !items.length && <EmptyRow icon="bi-search" text={t("gameAdmin.meta.noCandidate")} />}
                <SubmitRow>
                    {/* Chez un serveur fédéré, rien n'est automatique : sans candidat choisi, la copie prend le même jeu (son
                        identifiant, puis son titre). */}
                    <FormButton id={`mw-refetch-${service}`} type="button" kind="other" isReplaced label={t(peer ? "gameAdmin.meta.serverCopyBest" : "gameAdmin.meta.searchAgain")} pending={busy} onClick={() => fetchFrom("")} />
                </SubmitRow>
            </Rows>
            {/* LE REFUS D'UN SERVICE, une ligne sans encadré (le porteur, 2026-11-16 : « retirer les alerts ») — comme les refus
                de la connexion (screens/access/Login.jsx). */}
            {error && <p className="small text-danger mt-3 mb-0" role="alert">{error}</p>}
        </Section>
    );
}

// `game`, dans chaque morceau exporté — la source du jeu (libs/api.js, romSource) : `disk=<nom>` pour un jeu d'un disque
// interne, dont la fiche vit sur ce disque ; elle part sur chaque appel.

// « Ce n'est pas le bon jeu ? » : un groupe par service, puis un par serveur fédéré, titré de son nom — les
// fournisseurs d'abord (le porteur, 2026-11-15).
export function GameMatch({ consoleId, reference, game = {} }) {
    const { t } = useTranslation();
    const { servers } = useData();

    return [
        ...SERVICES.map((service, index) => (
            <WrongGame key={service} consoleId={consoleId} reference={reference} game={game} service={service} name={t(`gameAdmin.meta.services.${service}`)}
                help={index === 0 ? t("gameAdmin.meta.wrongGameHelp") : null} />
        )),
        ...(servers || []).map((server, index) => (
            <WrongGame key={`server:${server.id}`} consoleId={consoleId} reference={reference} game={game} service={`server:${server.id}`}
                name={server.hostname || server.link || t("gameAdmin.meta.services.server")} help={index === 0 ? t("gameAdmin.meta.serverHelp") : null} />
        )),
    ];
}

// « CORRIGER LES INFORMATIONS » (le modèle 24 ; la page « Informations » d'un jeu) : le formulaire du texte, puis celui
// du statut (le porteur, 2026-11-14 : « dans Game / information on n'a pas mis le changement de statut ») — deux
// formulaires, chacun son bouton dans son encadré : ils n'enregistrent pas au même endroit.
export function GameInfoForm({ consoleId, reference, game = {}, fallbackTitle }) {
    return (
        <>
            <GameTextForm consoleId={consoleId} reference={reference} game={game} fallbackTitle={fallbackTitle} />
            <GameStatusForm consoleId={consoleId} reference={reference} />
        </>
    );
}

// LA SORTIE, SA PRÉCISION (le porteur, 2026-11-14 : « le champ release date n'est pas un skeleton.Date » ; « Si les jeux
// ne prennent que des années de sortie alors on utilisera des selectbox pour les années. On choisit si on met une date
// complète ou juste l'année. C'est facile avec skeleton.Ruleset ») : « L'année seulement », un sélecteur des années, ou
// « La date complète », le calendrier de Skeleton (Form.Date). Les deux champs sont dans le formulaire ; le Ruleset cache
// celui que la précision n'utilise pas. La précision part de la valeur : « AAAA-MM-JJ », la date complète ; une année,
// ou rien, l'année. Enregistrée, la sortie est « AAAA » ou « AAAA-MM-JJ » (manualReleaseDate, metadata/game_admin.go,
// les deux) : passer de la date à l'année n'envoie que l'année.
const FULL_DATE = /^\d{4}-\d{2}-\d{2}$/;
// Les trois valeurs de la sortie dans le formulaire : sa précision, son année, son jour.
const RELEASE_FIELDS = ["releasePrecision", "releaseYear", "releaseDay"];
// Le texte qui s'écrit dans un champ : tout sauf la sortie, qui a ses propres contrôles.
const WRITTEN_FIELDS = TEXT_FIELDS.filter(([field]) => field !== "releaseDate");
// JAMAIS AVANT 1950 — et pas avant la plus ancienne machine du registre (1977, l'Atari 2600 ; les fiches des disques ne
// remontent pas avant 1984, relevé le 2026-11-14) : la date d'un jeu est sa première sortie, toutes machines confondues
// (igdb.go), et sa fiche peut donc la dire plus ancienne que sa console — son année reste alors dans la liste.
const OLDEST_YEAR = 1950;

// Les bornes des années : de l'an prochain (une sortie annoncée) à la plus ancienne des machines du registre, ou à
// l'année de la fiche si elle est hors de ces bornes.
const releaseBounds = (consoles, saved) => {
    const own = Number(saved.slice(0, 4)) || 0;
    const machines = (consoles || []).map((entry) => Number(entry.year) || 0).filter((year) => year >= OLDEST_YEAR);
    const bottom = Math.max(OLDEST_YEAR, Math.min(...(machines.length ? machines : [OLDEST_YEAR]), own || Infinity));

    return { bottom, top: Math.max(new Date().getFullYear() + 1, own) };
};

// LA SORTIE, SES DEUX LIGNES : sa précision (le choix segmenté), puis son champ — le sélecteur des années ou le
// calendrier, l'un caché par le Ruleset du formulaire (`context`) —, la gomme accolée (`addon`). La ligne du champ a la
// forme d'une ligne empilée (BlockRow, son Form.Group), comme les bornes du journal (settings/server/Logs.jsx) ; la croix va
// jusqu'aux jours du calendrier (useCalendarDays).
function ReleaseDateRows({ context, help, bounds, addon }) {
    const { t } = useTranslation();
    const days = useRef(null);
    const label = t("gameAdmin.fields.releaseDate");
    const years = Array.from({ length: bounds.top - bounds.bottom + 1 }, (_, index) => String(bounds.top - index));
    useCalendarDays(days);

    return (
        <>
            <ChoiceRow id="mm-precision" label={t("gameAdmin.meta.precision")} help={t("gameAdmin.meta.precisionHelp")} value={context.values.releasePrecision}
                options={[["year", t("gameAdmin.meta.yearOnly")], ["full", t("gameAdmin.meta.fullDate")]]} onChange={(value) => Form.set(context, "releasePrecision", value)} />
            <BlockRow>
                <Form.Group label={label} helpText={help} props={{ className: "mb-0 w-100" }}>
                    <div className="d-flex gap-2 align-items-start">
                        <div ref={days} className="flex-grow-1 min-w-0">
                            <Form.Select name="releaseYear" context={context} options={[{ value: "", label: t("gameAdmin.meta.pickYear") }, ...years.map((year) => ({ value: year, label: year }))]}
                                props={{ "data-nav": true, "data-key": "mm-releaseYear", "aria-label": label }} />
                            <Form.Date name="releaseDay" context={context} minDate={`${bounds.bottom}-01-01`} maxDate={`${bounds.top}-12-31`} />
                        </div>
                        {addon}
                    </div>
                </Form.Group>
            </BlockRow>
        </>
    );
}

// LE TEXTE D'UN JEU : chaque champ, Enregistrer dans son encadré.
function GameTextForm({ consoleId, reference, game = {}, fallbackTitle }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const metadata = useGameMetadata(consoleId, reference, game);
    const [drafts, setDrafts] = useState({});
    const [pending, setPending] = useState(false);

    if (metadata === undefined) {
        return <Section title={t("gameAdmin.infos")}><Note className="mt-0">{t("settings.loading")}</Note></Section>;
    }
    const sheet = metadata || {};
    const sources = sheet.sources || {};
    const saved = asText(sheet.releaseDate, false);
    const savedFull = FULL_DATE.test(saved);
    // Ce qu'affiche chaque champ : la correction en cours, sinon la fiche ; la sortie en trois valeurs tirées d'elle.
    const shown = {
        ...Object.fromEntries(WRITTEN_FIELDS.map(([field, list]) => [field, drafts[field] ?? asText(sheet[field], list)])),
        releasePrecision: drafts.releasePrecision ?? (savedFull ? "full" : "year"),
        releaseYear: drafts.releaseYear ?? saved.slice(0, 4),
        releaseDay: drafts.releaseDay ?? (savedFull ? saved : ""),
    };
    // LA SORTIE QU'ENREGISTRERAIT LE BOUTON : l'année, ou le jour choisi. Rien tant que le champ de la précision choisie
    // est vide — l'effacer est le geste de la gomme.
    const releaseDate = shown.releasePrecision === "full" ? shown.releaseDay || "" : shown.releaseYear || "";
    // Les corrections : les champs dont le texte n'est plus celui de la fiche (isReplaced), et la sortie si elle change.
    const changes = {
        ...Object.fromEntries(WRITTEN_FIELDS.filter(([field, list]) => field in drafts && drafts[field] !== asText(sheet[field], list)).map(([field, list]) => [field, asValue(drafts[field], list)])),
        ...(releaseDate && releaseDate !== saved ? { releaseDate } : {}),
    };
    const changed = Object.keys(changes);

    // Enregistrer : seulement les champs touchés ; la réponse est la fiche fusionnée.
    const save = async (patch) => {
        setPending(true);
        const answer = await saveManualMetadata(consoleId, reference, game, patch);
        setPending(false);

        if (!answer.ok) {
            toast(answer.errorMessage || t("settings.actionFailed"));

            return;
        }
        toast(t("gameAdmin.meta.saved"));
        setDrafts({});

        if (answer.data && typeof answer.data === "object" && Object.keys(answer.data).length) {
            setGameMetadata(consoleId, reference, game, answer.data);
        } else {
            forgetGameMetadata(consoleId, reference, game);
        }
    };

    // Le texte, un contexte de Skeleton : ce qu'affiche chaque champ ; un champ qu'on touche entre dans les brouillons. La
    // précision choisie cache le champ de la sortie qu'elle n'utilise pas (le Ruleset de Skeleton).
    const textForm = {
        values: { ...shown },
        setValues: (next) => setDrafts((previous) => ({
            ...previous,
            ...Object.fromEntries(Object.keys(shown).filter((field) => field in next && next[field] !== shown[field]).map((field) => [field, next[field] ?? ""])),
        })),
        files: {},
        setFiles: () => {},
        rulesets: {
            releaseYear: (data) => (data.releasePrecision === "full" ? Ruleset.isHidden() : null),
            releaseDay: (data) => (data.releasePrecision === "full" ? null : Ruleset.isHidden()),
        },
    };

    return (
        <Section title={t("gameAdmin.infos")} help={t("gameAdmin.meta.textHelp")}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (changed.length && !pending) {
                    save(changes);
                }
            } }}>
                <Rows>
                    {TEXT_FIELDS.map(([field, list]) => {
                        const current = asText(sheet[field], list);
                        const source = [].concat(sources[field] || []).join(", ");
                        const help = [source ? t("gameAdmin.from", { source }) : current ? null : t("gameAdmin.meta.noSource"), list && t("gameAdmin.meta.listHelp")].filter(Boolean).join(" · ");
                        const touched = field === "releaseDate" ? RELEASE_FIELDS.some((name) => name in drafts) : Boolean(drafts[field]);
                        const eraser = <IconAddon id={`mm-clear-${field}`} icon="bi-eraser" label={t("gameAdmin.meta.clear")} disabled={pending || (!current && !touched)} onClick={() => save({ [field]: list ? [] : "" })} />;

                        if (field === "releaseDate") {
                            return <ReleaseDateRows key={field} context={textForm} help={help} bounds={releaseBounds(consoles, saved)} addon={eraser} />;
                        }

                        return (
                            <TextRow key={field} id={field} label={t(`gameAdmin.fields.${field}`)} context={textForm} help={help}
                                props={{ placeholder: field === "title" ? fallbackTitle : "", "data-key": `mm-${field}` }} addon={eraser} />
                        );
                    })}
                    <SubmitRow>
                        <FormButton id="mm-save" label={t("settings.save")} isReplaced={changed.length > 0} pending={pending} />
                    </SubmitRow>
                </Rows>
            </Form>
        </Section>
    );
}

// LE STATUT D'UN JEU (chantier 30) : le geste de la webapp (games/GameStatus.jsx, GameStatusPanel), en formulaire — UN
// SÉLECTEUR PAR ÉMULATEUR de la console (ceux que le serveur connaît, installés ou non : sa version, ou qu'il manque ici),
// les quatre statuts du serveur, la valeur de chacun celle que la liste des jeux montre. Enregistrer envoie un
// signalement par statut choisi, pour les émulateurs qui en changent ; s'ils finissent tous au même, un seul, pour tous
// (`emulators` vide : un émulateur ajouté plus tard le suit, comme « tous cochés » dans la webapp). La vue que le serveur
// répond passe dans la liste gardée de la console (data.jsx, patchHere) : la fiche de la bibliothèque, sa recherche, ses
// sous-titres et ses filtres la montrent sans rien relire.
//
// LA NOTE ET L'HISTORIQUE (chantier 56, A1 ; le porteur, 2026-11-14 : « Oui ») — ce que GameStatus.jsx de la webapp
// avait, et que le launcher n'avait pas repris. La note (facultative, 500 caractères) part avec le signalement ; une
// note seule, sans statut changé, part avec les statuts tels qu'ils sont, un signalement par statut (rien ne change de
// statut). L'historique, dans sa section : chaque signalement, le plus récent d'abord — son statut, les émulateurs qu'il
// concerne, sa note, qui, quand, et la version de chacun à ce moment (`versions`, que seul l'administrateur lit).
function GameStatusForm({ consoleId, reference }) {
    const { t, i18n } = useTranslation();
    const { patchHere } = useData();
    const [answer, setAnswer] = useState(undefined);
    const [drafts, setDrafts] = useState({});
    const [note, setNote] = useState("");
    const [pending, setPending] = useState(false);

    useEffect(() => {
        let cancelled = false;
        setAnswer(undefined);
        setDrafts({});
        setNote("");
        fetchGameStatus(consoleId, reference).then((read) => !cancelled && setAnswer(read));

        return () => {
            cancelled = true;
        };
    }, [consoleId, reference]);

    if (!answer?.ok) {
        return (
            <Section title={t("games.status")}>
                <Note className="mt-0">{answer === undefined ? t("settings.loading") : answer.errorMessage || t("gameAdmin.statusForm.unreadable")}</Note>
            </Section>
        );
    }
    const view = answer.data?.view || {};
    const emulators = answer.data?.emulators || [];
    const savedOf = (name) => view.emulators?.[name] || view.status || "untested";
    const chosenOf = (name) => drafts[name] ?? savedOf(name);
    const changed = emulators.filter(({ name }) => chosenOf(name) !== savedOf(name));
    const written = note.trim();
    const history = [...(answer.data?.reports || [])].reverse();

    const save = async () => {
        const finals = new Set(emulators.map(({ name }) => chosenOf(name)));
        const groups = {};

        // Une note seule : chaque émulateur à son statut d'aujourd'hui, regroupés par statut.
        for (const { name } of changed.length ? changed : emulators) {
            groups[chosenOf(name)] = [...(groups[chosenOf(name)] || []), name];
        }
        const reports = (finals.size === 1 ? [{ status: [...finals][0], emulators: [] }] : Object.entries(groups).map(([status, names]) => ({ status, emulators: names })))
            .map((report) => ({ ...report, note: written }));
        setPending(true);
        let last = null;

        for (const report of reports) {
            last = await reportGameStatus(consoleId, reference, report);

            if (!last.ok) {
                break;
            }
        }
        // Un signalement refusé en route : ce qui est passé avant lui est écrit — la page relit le statut.
        const settled = last.ok ? last : await fetchGameStatus(consoleId, reference);
        setPending(false);
        toast(last.ok ? t("gameAdmin.statusForm.saved") : last.errorMessage || t("settings.actionFailed"));

        if (settled.ok) {
            setAnswer(settled);
            setDrafts({});
            setNote("");
            patchHere(consoleId, reference, { status: settled.data?.view || null });
        }
    };
    const dated = (at) => (at ? new Date(at).toLocaleString(i18n.language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

    return (
        <>
        <Section title={t("games.status")} help={t("gameAdmin.statusForm.help")}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if ((changed.length || written) && emulators.length && !pending) {
                    save();
                }
            } }}>
                <Rows>
                    {emulators.length ? emulators.map(({ name, version, installed }, index) => (
                        <SelectRow key={name} id={`ms-emulator-${index}`} label={name} help={installed ? t("gameAdmin.statusForm.version", { version }) : t("gameAdmin.statusForm.notInstalled")}
                            value={chosenOf(name)} disabled={pending} onChange={(value) => setDrafts((previous) => ({ ...previous, [name]: value }))}
                            options={GAME_STATUSES.map((status) => ({ value: status, label: t(`status.${status}`) }))} />
                    )) : <EmptyRow icon="bi-cpu" text={t("gameAdmin.statusForm.none")} />}
                    {emulators.length > 0 && (
                        <TextRow id="ms-note" kind="longtext" label={t("gameAdmin.statusForm.note")} help={t("gameAdmin.statusForm.noteHelp")} value={note}
                            onChange={(value) => setNote(String(value || "").slice(0, 500))} props={{ maxLength: 500, rows: 3, disabled: pending }} />
                    )}
                    <SubmitRow>
                        <FormButton id="ms-save" label={t("settings.save")} isReplaced={changed.length > 0 || written.length > 0} pending={pending} />
                    </SubmitRow>
                </Rows>
            </Form>
        </Section>
        <Section title={t("gameAdmin.statusForm.history")} help={t("gameAdmin.statusForm.historyHelp")}>
            <Listing items={history} empty={t("gameAdmin.statusForm.historyEmpty")} render={(report, index) => {
                const versions = Object.entries(report.versions || {}).map(([name, version]) => `${name} ${version}`).join(", ");

                return (
                    <InfoRow key={`${report.at}-${index}`} icon="bi-clock-history" tone={STATUS_TONE[report.status] || "slate"}
                        label={`${t(`status.${report.status}`)} · ${report.emulators?.length ? report.emulators.join(", ") : t("gameAdmin.statusForm.allEmulators")}`}
                        value={[report.note, t("gameAdmin.statusForm.reportedBy", { by: report.by, at: dated(report.at) }), versions].filter(Boolean).join(" — ")} />
                );
            }} />
        </Section>
        </>
    );
}

// La couleur d'un signalement dans l'historique : celle du badge de son statut (STATUS_VARIANT), en pastille.
const STATUS_TONE = { playable: "green", crashes: "amber", unplayable: "rose", untested: "slate" };

// « OUBLIER UNE SOURCE » : la source choisie, puis le geste, son bouton dans l'encadré, qui demande d'abord (la fenêtre
// de Skeleton). AUCUNE SOURCE N'EST CHOISIE D'AVANCE (le porteur, 2026-11-14 : « l'option par défaut doit être vide et
// button disabled ») : « ••• » tant qu'on n'en a pas pris une. Il garde la clé de l'ancienne confirmation sur place
// (`mf-forget-ask`).
export function GameForget({ consoleId, reference, game = {} }) {
    const { t } = useTranslation();
    const [forgotten, setForgotten] = useState("");
    const confirm = useConfirm();

    const ask = () => {
        const name = t(`gameAdmin.meta.services.${forgotten}`);
        const chosen = forgotten;

        confirm({ title: t("gameAdmin.meta.forgetSource"), message: t("gameAdmin.meta.forgetConfirm", { source: name }), confirmLabel: t("gameAdmin.meta.forgetSource"), onConfirm: async () => {
            const answer = await dropMetadataSource(consoleId, reference, game, chosen);
            toast(answer.ok ? t("gameAdmin.meta.forgotten", { source: name }) : answer.errorMessage || t("settings.actionFailed"));

            if (answer.ok) {
                reread(consoleId, reference, game);
            }
        } });
    };

    return (
        <Section title={t("gameAdmin.meta.sources")} help={t("gameAdmin.meta.forgetHelp")}>
            <Rows>
                <SelectRow id="mf-source" label={t("gameAdmin.meta.forgetWhich")} value={forgotten} onChange={(value) => setForgotten(value || "")}
                    options={[{ value: "", label: t("gameAdmin.meta.forgetPick") }, ...SOURCES.map((source) => ({ value: source, label: t(`gameAdmin.meta.services.${source}`) }))]} />
                <SubmitRow>
                    <FormButton id="mf-forget-ask" type="button" kind="danger" isReplaced={Boolean(forgotten)} label={t("gameAdmin.meta.forgetSource")} onClick={ask} />
                </SubmitRow>
            </Rows>
        </Section>
    );
}
