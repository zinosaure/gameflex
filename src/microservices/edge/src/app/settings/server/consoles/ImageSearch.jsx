import React, { useEffect, useRef, useState } from "react";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { useImage } from "../../../libs/images.js";
import { useQueryState } from "../../SettingsPage.jsx";
import { Listing } from "../../../bricks/Listing.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { PlaceholderRow } from "../../../bricks/PlaceholderRow.jsx";
import { Thumb } from "../../../bricks/Thumb.jsx";
import { useConfirm } from "../../../bricks/useConfirm.jsx";
import { useDebounced } from "../../../libs/ui.js";
import { fact } from "../../../libs/format.js";

// CHOISIR UNE IMAGE (chantier 53, P9 ; le porteur, 2026-11-14, remarque d : « au lieu de mettre upload file pour
// l'image, le mieux ce serait de mettre une search bar qui va taper sur les API metadata pour récupérer plus d'images et
// ensuite les proposer en format listing. Comme ça on choisit celle qu'on veut. Pareil quand on souhaite remplacer une
// image. ») — la recherche des pages qui posent une image : un emplacement du fond d'une console (Consoles.jsx),
// une sorte d'image d'un jeu (Games.jsx). Une seule, pour que les deux se ressemblent :
//   - LA RECHERCHE (Listing) : ce qu'on tape part un instant après la dernière lettre chez les fournisseurs de
//     métadonnées que le kernel connaît (IGDB, SteamGridDB, libretro-thumbnails ; metadata/image_search.go). `initial`
//     la préremplit (le titre du jeu ; rien pour une console). Ce qu'on a tapé reste dans l'adresse (`?q=`) ;
//   - LES IMAGES PROPOSÉES, en lignes feuilletées (des fantômes pendant la lecture) : la vignette, le jeu d'où elle
//     vient, sa source et sa taille. Les lignes n'ouvrent pas de page : la liste tient sa page elle-même ;
//   - EN TOUCHER UNE ouvre la fenêtre de Skeleton (`confirmOf(image)` : son titre, son texte, son geste), puis
//     `onChoose(image)` la pose — le serveur la récupère chez l'hôte d'images de sa source, et seulement là. Sa ligne
//     travaille le temps qu'il le fait, les autres sont grisées ;
//   - SOUS LA LISTE, les fournisseurs qui n'ont pas cherché : sans compte (il se saisit dans Réglages › Métadonnées), ou
//     muets.
// `search(query)` : la lecture, qui rend la forme de `kernel` ; `fit` : comment la vignette tient (un fond la remplit).
//
// MES SERVEURS FÉDÉRÉS, APRÈS LES FOURNISSEURS (chantier 57, P3 ; le porteur, 2026-11-15 : « je peux récupérer les
// images de celui pour populer mes metadata de mes propres jeux ») : l'image d'un de leurs jeux est une ligne comme les
// autres — sa vignette se charge par le kernel, avec le jeton (`thumbPath`, useImage), comme les images d'un jeu ; son
// aide dit « <nom du serveur> · L × H ». Une image d'un de ses fournisseurs, s'il les partage, dit le fournisseur et le
// serveur. Un serveur muet se nomme dans la note des sources muettes ; un serveur qui lit encore ses jeux le dit.

// Le refus d'une image choisie, dit à l'utilisateur (writeChosenImageError, côté kernel).
const refusalOf = (answer, t) => (answer.status === 502 || answer.status === 0 ? t("images.unreachable")
    : answer.status === 415 ? t("images.unreadable") : answer.status === 413 ? t("images.tooLarge") : answer.errorMessage || t("settings.actionFailed"));

// La vignette d'une image proposée : son adresse chez son hôte, ou, pour un jeu d'un serveur fédéré, le relais du kernel.
function ResultThumb({ image, fit }) {
    const relayed = useImage(image.thumbPath || null);

    return <Thumb url={image.thumbPath ? relayed?.url || null : image.thumbUrl} fit={fit} />;
}

// Ce qui distingue une image proposée : son adresse, ou le jeu du serveur fédéré qui la prête.
export const imageKey = (image) => `${image.source}:${image.server || ""}:${image.url || image.reference}`;

// D'où vient une image proposée : le fournisseur, le serveur fédéré, ou les deux (un fournisseur qu'il partage).
export const imageOrigin = (image, t) => (image.source === "server" ? image.serverName || t("images.services.server")
    : [t(`images.services.${image.source}`), image.serverName].filter(Boolean).join(", "));

export function ImageSearch({ initial = "", search, confirmOf, onChoose, fit = "contain" }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const confirm = useConfirm();
    const [params] = useSearchParams();
    const [, update] = useQueryState({});
    const typed = useRef(params.has("q"));
    const [text, setText] = useState(() => params.get("q") ?? initial);
    // La recherche part un instant après la dernière lettre : chaque recherche interroge les fournisseurs.
    const query = useDebounced(text.trim(), 500);
    const [answer, setAnswer] = useState(undefined);
    const [busy, setBusy] = useState(null);
    const service = (source) => t(`images.services.${source}`);

    // Le titre arrive après la page (la fiche d'un jeu se lit) : il préremplit tant qu'on n'a rien tapé.
    useEffect(() => {
        if (!typed.current) {
            setText(initial);
        }
    }, [initial]);

    useEffect(() => {
        if (!query) {
            setAnswer({ items: [], sources: [] });

            return undefined;
        }
        let cancelled = false;
        setAnswer(undefined);
        search(query).then((result) => !cancelled && setAnswer(result.ok ? result.data || { items: [], sources: [] } : { failure: result }));

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [query]);

    const choose = (image) => confirm({
        ...confirmOf(image),
        onConfirm: async () => {
            setBusy(imageKey(image));
            const result = await onChoose(image);
            setBusy(null);

            if (!result.ok) {
                feedback.failed(refusalOf(result, t));
            }
        },
    });

    const items = answer?.items || [];
    const silent = (answer?.sources || []).filter((source) => source.state === "noAccount");
    const failed = (answer?.sources || []).filter((source) => source.state === "error");
    const reading = (answer?.sources || []).filter((source) => source.reading);
    const sourceName = (source) => (source.source === "server" ? source.serverName || t("images.services.server") : service(source.source));

    return (
        <>
            <Listing items={items} loading={answer === undefined} search={text} searchKey="is-search" placeholder={t("images.searchPlaceholder")}
                onSearch={(value) => {
                    typed.current = true;
                    setText(value);
                    update({ q: value });
                }}
                found={t("images.found", { count: items.length })} empty={answer?.failure ? null : query ? t("images.none") : t("images.typeTitle")} noMatch={answer?.failure ? null : t("images.none")}
                placeholderRow={() => <PlaceholderRow lead="thumb" />}
                render={(image, index) => (
                    <NavRow key={imageKey(image)} id={`is-${index}`} lead={<ResultThumb image={image} fit={fit} />} label={image.game || imageOrigin(image, t)}
                        help={[fact(t("game.origin"), imageOrigin(image, t)), fact(t("facts.dimensions"), image.width && image.height ? `${image.width} × ${image.height}` : null)]}
                        pending={busy === imageKey(image)} disabled={Boolean(busy) && busy !== imageKey(image)} opens="confirm" onOpen={() => choose(image)} />
                )} />
            {answer?.failure && <Note>{answer.failure.errorMessage || t("settings.actionFailed")}</Note>}
            {silent.length > 0 && <Note>{t("images.noAccount", { list: silent.map(sourceName).join(", ") })}</Note>}
            {failed.length > 0 && <Note>{t("images.sourceSilent", { list: failed.map(sourceName).join(", ") })}</Note>}
            {reading.length > 0 && <Note>{t("images.serverReading", { list: reading.map(sourceName).join(", ") })}</Note>}
        </>
    );
}
