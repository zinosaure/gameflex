import React from "react";
import { useTranslation } from "react-i18next";
import { counted, formatSize, STATUS_VARIANT } from "../libs/format.js";
import { InSettings } from "../globals/backoffice.js";
import { EmptyRow } from "./EmptyRow.jsx";
import { InfoRow } from "./InfoRow.jsx";
import { Note } from "./Note.jsx";
import { PlaceholderRow } from "./PlaceholderRow.jsx";
import { Rows } from "./Rows.jsx";
import { CONSOLE_ICON } from "../globals/icons.js";

// LES INFORMATIONS D'UN JEU, UNE SEULE FORME (le porteur, 2026-11-23 : « tout doit rester cohérent », « le côté front doit
// être comme un framework ») : l'engrenage d'un jeu (GameSettings.jsx), sa fiche du Catalogue et de Mes favoris, sa page
// des Réglages (d'ici, d'un ami, d'une ressource externe) — chaque renseignement à sa place et avec son icône. TOUS LES
// CHAMPS D'UN JEU, TOUJOURS, « - » quand on ne sait pas (le porteur, 2026-11-25 : « Il faut tous les éléments par défaut et
// si pas de valeur alors c'est un "-" qu'on met ») : la console, la source, le statut, la sortie, l'éditeur, le
// développeur, les genres, la série, les joueurs, l'identifiant, la taille (le nombre de fichiers est parti : « la
// ligne file qui ne sert à rien en vrai », le porteur, 2026-11-25). Ce qui ne vaut que pour une page
// (le chemin, le moteur, la date d'une sauvegarde, « déjà ici ») n'y est que quand elle le donne ; les détails d'une
// sauvegarde (`complete` à faux) ne montrent que ce que la page donne. Une seule couleur, celle
// de la section. `facts` : ce que la page sait du jeu (`console`, `origin`, `here` — déjà dans la
// bibliothèque —, `status` et `statusNote`, `series`, `players`, `gameId`, `path` — son chemin depuis le dossier des jeux —,
// `engine`, `date` — celle d'une sauvegarde —, `sizeBytes`) ;
// `metadata` : sa fiche (undefined : elle se lit, une ligne fantôme) ; `refusal` : pourquoi elle ne se lit pas ; `sources` :
// d'où vient la fiche (sourcesOf, libs/metadata.js), cités sous les lignes ; `children` : des lignes en tête (pourquoi le jeu
// ne partirait pas d'ici).
export function GameFacts({ facts = {}, metadata = null, refusal = null, sources = [], complete = true, children = null }) {
    const { t } = useTranslation();
    const joined = (values) => (values || []).join(", ");
    const status = facts.status || null;
    // `complete` à faux : seulement ce que la page donne (les détails d'une sauvegarde, Mes sauvegardes).
    const dash = (value) => (complete && (value === null || value === undefined || value === "") ? "-" : value);

    // EN TEXTE, MÊME DANS LES RÉGLAGES (le porteur, 2026-11-25 : « rajouter l'exception dans pastilles pour la fiche des
    // jeux dans settings ») : la fiche d'un jeu se lit comme hors des Réglages — ses valeurs en texte, son statut en
    // pastille (globals/backoffice.js).
    return (
        <InSettings.Provider value={false}>
            <Rows>
                {children}
                <InfoRow icon={CONSOLE_ICON} label={t("catalog.console")} value={dash(facts.console)} />
                <InfoRow icon="bi-arrow-left-right" label={t("game.origin")} value={dash(facts.origin)} />
                {facts.here && <InfoRow icon="bi-check2-circle" label={t("games.status")} value={t("gameAdmin.alreadyHere")} />}
                <InfoRow icon="bi-patch-check" label={t("games.status")} value={!status ? dash(null) : (
                    <>
                        <span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>
                        {facts.statusNote && <span className="d-block small text-body-secondary text-wrap mt-1">{facts.statusNote}</span>}
                    </>
                )} />
                {metadata === undefined && <PlaceholderRow help={false} chevron={false} />}
                {refusal && <EmptyRow icon="bi-journal-x" text={refusal} />}
                <InfoRow icon="bi-calendar3" label={t("gameAdmin.fields.releaseDate")} value={dash(metadata?.releaseDate)} />
                <InfoRow icon="bi-building" label={t("gameAdmin.fields.publishers")} value={dash(joined(metadata?.publishers))} />
                <InfoRow icon="bi-code-slash" label={t("gameAdmin.fields.developers")} value={dash(joined(metadata?.developers))} />
                <InfoRow icon="bi-tags" label={t("gameAdmin.fields.genres")} value={dash(joined(metadata?.genres))} />
                <InfoRow icon="bi-collection" label={t("gameAdmin.fields.series")} value={dash(metadata?.series || facts.series)} />
                <InfoRow icon="bi-people" label={t("game.players")} value={Number(metadata?.players || facts.players) > 0 ? counted(t, "players", metadata?.players || facts.players) : dash(null)} />
                <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={dash(facts.gameId || metadata?.gameId)} />
                <InfoRow icon="bi-folder2" label={t("resources.filePath")} value={facts.path} mono />
                <InfoRow icon="bi-boxes" label={t("gear.engine")} value={facts.engine} />
                <InfoRow icon="bi-clock-history" label={t("facts.date")} value={facts.date} />
                <InfoRow icon="bi-hdd" label={t("game.size")} value={dash(formatSize(facts.sizeBytes))} />
            </Rows>
            {sources.length > 0 && (
                <Note>
                    <b className="text-body">{t("game.sources")}</b>{" "}
                    {sources.map((source, index) => (
                        <React.Fragment key={source.label}>
                            {index > 0 && ", "}
                            {source.href ? <a href={source.href} target="_blank" rel="noopener noreferrer" data-nav>{source.label}</a> : source.label}
                        </React.Fragment>
                    ))}
                </Note>
            )}
        </InSettings.Provider>
    );
}
