import React from "react";
import { DEFAULT_PILL } from "./Pill.jsx";
import { MAX_LINES, SUBTITLES } from "../globals/subtitles.jsx";
import { Chip } from "./Chip.jsx";
import { RowText } from "./RowText.jsx";
import { useInListing } from "../globals/listing.js";
import { SENSES, SenseTone, useSenseTone } from "../globals/sections.js";

// CE QU'UNE LIGNE FAIT, DIT À SON BORD DROIT (le porteur, 2026-11-14 : « liens qui vont nulle part ou qui ouvrent des
// modal ne doivent pas avoir la flèche droite ">". Remplace par un icône "?" si c'est une question ou "i" si c'est une
// information, ou une icône adéquate à la demande, ou sinon rien. ») — défini ICI une fois, par ce que la ligne fait
// (`opens`), jamais page par page :
//   page     elle ouvre une page ou un sous-panneau : le chevron ;
//   confirm  elle pose une question, la fenêtre de confirmation (DangerRow, useConfirm) : « ? » ;
//   info     elle montre un renseignement : « i » ;
//   link     elle ouvre une adresse d'un autre site, dans un autre onglet : la flèche qui sort ;
//   action   elle agit sur place (copier, télécharger, relire, basculer, lancer) : rien.
//   delete   elle supprime (après sa confirmation) : la poubelle rouge (le porteur, 2026-11-25).
export const ROW_ENDS = { page: "bi-chevron-right", confirm: "bi-question-circle", info: "bi-info-circle", link: "bi-box-arrow-up-right", action: null, delete: "bi-trash text-danger" };

// UNE ACTION QUI NE PEUT PAS SE FAIRE (le porteur, 2026-11-25 : « pour jouer quand on ne peut pas lancer la partie par
// exemple à cause d'une connexion requise ou autre, ça doit être de couleur orange ou rouge. Pareil pour les autres si
// l'action n'est pas possible ») : `blocked` remplace la couleur de la section sur la pastille — orange quand il lui
// manque ce qu'on peut donner (`warning`), rouge quand elle est impossible (`danger`). Dans un listing, sans pastille
// (globals/listing.js), c'est son titre qui prend la couleur.
export const BLOCKED_TONES = { warning: "amber", danger: "rose" };

// UNE LIGNE QUI OUVRE (une page, un sous-panneau) — ou, pour un geste qui n'appartient à aucun groupe, qui AGIT (le
// porteur, 2026-11-13 : un bouton orphelin est une ligne) : toute la ligne se touche, ce qu'elle fait au bord droit
// (`opens`, ROW_ENDS), la valeur actuelle sous son libellé (`value`, comme InfoRow). `lead` : une vignette ou un avatar à la place de la
// pastille. `pending` : le geste est en cours — le tourniquet au bord droit, la ligne ne se touche plus. `disabled` :
// grisée (sa raison en `help`), sans rien à son bord droit. `checked` : la validation verte à côté du titre — ce que la console prend sans
// choix, une console activée, un jeu d'ailleurs déjà dans la bibliothèque (le porteur, 2026-11-23 : « au lieu d'écrire
// Already here mets le titre avec couleur primary »).
//
// `progress` (0 à 100) : UN TÉLÉCHARGEMENT EN COURS depuis cette ligne (le porteur, 2026-11-13 : « Les
// téléchargements doivent aussi afficher la progression, pas juste le spinner ») — le pourcentage au bord droit, une
// fine barre `progress` de Bootstrap sous le texte, « reçu sur total » en `help` ; -1 tant que le total n'est pas
// connu, le tourniquet. La ligne ne se touche plus, mais garde la lumière de la croix (elle n'est pas grisée : c'est
// elle qui travaille).
// `obsolete` : une ancienne version, toute la ligne grisée (elle se touche encore : la supprimer).
// `valueTone` : la couleur de la pastille de sa valeur (Pill : success, danger ; par défaut, celle de sa place).
export default function NavRow({ id, icon = "bi-chevron-right", tone = null, label, help = null, value = null, valueTone = DEFAULT_PILL, lead = null, danger = false, disabled = false, pending = false, progress = null, checked = false, obsolete = false, blocked = null, opens = "page", separator = SUBTITLES.separator, noKey = SUBTITLES.noKey, onOpen }) {
    // DANS « DANGER », TOUTE LIGNE EST ROUGE (le porteur, 2026-11-25 : « Dans danger le lien Forget a source doit être en
    // rouge ») : le sens de la section le dit, une fois pour toutes.
    danger = danger || React.useContext(SenseTone) === SENSES.danger;
    tone = useSenseTone(tone);
    const busy = progress !== null;
    const known = busy && progress >= 0;
    const end = disabled ? null : ROW_ENDS[opens];
    tone = BLOCKED_TONES[blocked] || tone;
    // DANS UN LISTING (globals/listing.js) : pas de pastille ; une image (`lead`) reste, avec le texte, dans la colonne de
    // gauche ; le geste en cours tourne au bord droit.
    const listed = useInListing();
    const spinsAtEnd = pending && (lead || listed);
    const text = (
        <RowText label={blocked && listed ? <span className={`text-${blocked}`}>{label}</span> : label} value={value} help={help} checked={checked} pill={valueTone}
            max={opens === "page" ? MAX_LINES : Infinity} separator={separator} noKey={noKey}>
            {known && (
                <span className="progress gf-progress" role="progressbar" aria-label={label} aria-valuenow={progress} aria-valuemin="0" aria-valuemax="100">
                    <span className="progress-bar" style={{ width: `${progress}%` }}></span>
                </span>
            )}
        </RowText>
    );

    return (
        <button type="button" className={`list-group-item list-group-item-action gf-row navrow ${danger ? "danger" : ""} ${obsolete ? "gf-row-obsolete" : ""}`} data-nav data-key={id} data-opens={opens} disabled={disabled || pending}
            aria-busy={pending || busy || undefined} aria-disabled={busy || undefined} onClick={busy ? undefined : onOpen}>
            {/* LE GESTE EN COURS (`pending`) : la pastille de gauche devient la roue (le porteur, 2026-11-17, un rescan :
                « l'icône de gauche se change en spinner. Au lieu de mettre le spinner en sous-titre »). */}
            {listed ? (lead ? <span className="gf-row-main">{lead}{text}</span> : text) : (
                <>
                    {pending && !lead ? <span className={`gf-chip tone-${tone}`} aria-hidden="true"><span className="spinner-border spinner-border-sm" role="status"></span></span>
                        : lead || <Chip icon={icon} tone={tone} />}
                    {text}
                </>
            )}
            {known ? <span className="gf-row-value fw-semibold">{`${progress} %`}</span>
                : spinsAtEnd || busy
                    ? <span className="spinner-border spinner-border-sm text-primary flex-shrink-0" role="status"></span>
                    : !pending && end && <i className={`bi ${end} gf-row-chevron`} aria-hidden="true"></i>}
        </button>
    );
}
