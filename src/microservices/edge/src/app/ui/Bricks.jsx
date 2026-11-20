import React, { useCallback, useContext, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { cancelDownload } from "../libs/downloads.js";
import { formatSize } from "../libs/format.js";
import { EXTERNAL, guideUrl } from "../libs/links.js";
import { copyTextToClipboard } from "../libs/share-link.js";
import { focusElement, pushScreenInput } from "../libs/input.js";
import { choiceButton, GROUP_HEADING, ICON_BUTTON, IN_SERVICE, MUTED, SQUARE_ICON_BUTTON, useButton } from "../libs/ui.js";
import Skeleton from "../skeleton/skeleton.js";
import { fieldContext, Form, FormButton, NewPassword, PadReach, usePagination } from "./Forms.jsx";
import { toast } from "../shell/Shell.jsx";
import SidePanel from "../shell/SidePanel.jsx";
import { setInvisibleSticks, useGamepadPreferences } from "../engine/libs/gamepadPreferences.js";
import { usePhysicalGamepad } from "../engine/libs/gamepadPresence.js";
import { HALF_SCREEN_MAX, HALF_SCREEN_MIN, SCREEN_ORIENTATIONS, getHalfScreenHeight, setHalfScreenHeight, useScreenOrientation } from "../libs/screenOrientation.js";
import { canLockOrientation, chooseOrientation } from "../libs/orientation.js";

// LES BRIQUES DU LAUNCHER — la direction C (« tuiles et groupes »), choisie et validée par le porteur le
// 2026-11-13 (docs/reports/2026-11-13-launcher-modeles/composants.html). Définies UNE fois, ici ; le menu de la
// partie, les réglages d'un jeu et chaque page des Réglages s'en composent, et rien d'autre ne dessine un groupe,
// une ligne, une tuile ou un geste destructif à sa façon.
//
// LES RÈGLES, mesurées sur les modèles :
//   - un seul bord gauche : le rond de Retour, les onglets, la recherche, les tuiles, les intitulés, les groupes,
//     les textes et les boutons commencent sur la même verticale ; le titre suit le rond sur sa ligne, le fil
//     d'Ariane sous le titre, plus petit (PageHeader) ;
//   - `mb-3` sous chaque intitulé de groupe (sous son aide quand il en a une), `mt-4` entre deux groupes ;
//   - un groupe est un conteneur arrondi (`Rows`), et il a TOUJOURS un titre quand il en existe un honnête (le
//     porteur, 2026-11-13 : « chaque section doit toujours avoir un titre si possible ») ; ses lignes s'ouvrent
//     (NavRow ; SplitRow quand un geste se pose à leur bord droit), se règlent sur place (SwitchRow, SelectRow, RangeRow, ChoiceRow, TextRow) ou renseignent (InfoRow : le
//     libellé, sa valeur dessous, à gauche — le porteur, 2026-11-16, « Au lieu de les mettre en 2 colonnes ») ;
//   - LE BORD DROIT D'UNE LIGNE DIT CE QU'ELLE FAIT (NavRow, `opens`) : le chevron pour une page, « ? » pour une
//     question, « i » pour un renseignement, l'icône de son geste ou rien (le porteur, 2026-11-14) ;
//   - L'ORDRE D'UNE PAGE : ses listes d'abord, puis « Plus », puis « Danger », puis « Bon à savoir » et « Guide », TOUJOURS
//     en dernier (le porteur, 2026-11-14 : « Les sections sont toujours après les listings » ; 2026-11-16 : « la section
//     bon à savoir et guide sont toujours en dernier. Même après danger. ») ; « Guide » renvoie aux documents du dépôt public, en section
//     de texte (titres en lien, texte dessous) et non en lignes (GuideSection, TextSection) ;
//   - PAS D'ALERTE (le porteur, 2026-11-16 : « retirer les alerts ») : un renseignement est une ligne d'une section
//     d'informations (InfoRow) ou l'aide d'un groupe (`help`), une erreur une ligne atténuée (Note) ;
//   - UN GROUPE QUI SERT DE FORMULAIRE N'A PAS DE TRAITS, ses lignes espacées d'un mb-3, et ses interrupteurs n'y ont
//     pas d'icône ; un groupe de liens a ses traits d'un bord à l'autre (le porteur, 2026-11-14 ; Rows) ;
//   - LE BOUTON D'UN GROUPE EST DANS SON ENCADRÉ, après son dernier contrôle (SubmitRow ; le porteur, 2026-11-13 :
//     « le bouton doit être dans la section card avec les autres form controls ») ;
//   - UN GESTE QUI N'APPARTIENT À AUCUN GROUPE EST UNE LIGNE, pas un bouton (« Les boutons orphelins […] ne doivent
//     pas être des boutons mais des liens ») : ce qui détruit dans la section « Danger » (DangerSection, DangerRow),
//     le reste dans le groupe « Plus » ; grisée quand le geste n'est pas disponible, sa raison en sous-titre ;
//   - TOUTE CONFIRMATION EST LA FENÊTRE DE SKELETON (useConfirm ; « utilise skeleton.Feedback pour les fenêtres de
//     confirmation partout dans le launcher ») ;
//   - LES BOUTONS SONT PLEINS, et leur couleur vient de leur sorte et de leur lieu : primary pour valider et danger
//     dans les Réglages ; sur les écrans, success pour le geste principal et les ronds, les autres couleurs gardées
//     (secondary, warning, danger) ; un choix segmenté primary et outline-primary partout (libs/ui.js, `useButton()` ;
//     le porteur, 2026-11-14) ;
//   - LE BOUTON D'UN FORMULAIRE EST FormButton (Forms.jsx) : « ••• » tant que rien n'a changé, une ligne au-dessus ;
//   - UN BOUTON PORTE SOIT UN TEXTE, SOIT UNE ICÔNE, JAMAIS LES DEUX (le porteur, 2026-11-13) : les gestes sont
//     des boutons de texte ; un bouton accolé à un champ n'est qu'une icône (IconAddon), son nom en aria-label et
//     en title. Les lignes et les tuiles, qui ne sont pas des boutons, gardent icône et libellé ;
//   - une ligne EMPILÉE (un sélecteur, un champ, une glissière, un choix) n'a pas d'icône : son contrôle prend la
//     largeur ;
//   - boutons et champs à 6 px (bootstrap.scss, 60fd09c5), tuiles comprises — une tuile est un bouton ; les
//     conteneurs de groupe sont arrondis à 16 px (launcher.css, `.gf-list`).

// Les couleurs des pastilles d'icône, par thème (launcher.css, `.tone-*`).
export const TONES = ["indigo", "teal", "amber", "rose", "green", "slate", "violet", "sky"];

// LE FIL D'ARIANE (le porteur, 2026-11-13 : « rajouter des fils d'Ariane dans les titres pour ne pas se perdre ») :
// le `breadcrumb` de Bootstrap, SOUS LE TITRE ET PLUS PETIT (le porteur, 2026-11-14, chantier 53, P13 : « pour
// essayer, peut-on mettre la fil d'Ariane en dessous du titre et en plus petit ? Voir ce que ça fait ? »), ses niveaux
// séparés par « / » — « Réglages / Consoles / PlayStation 4 ». Ce sont les pages AU-DESSUS de celle-ci, chacune un
// lien : la page elle-même est le titre, le fil ne la répète pas. Quand il ne tient pas sur la ligne, il se REPLIE au
// milieu, mesuré : la racine, « … », puis les deux derniers niveaux, ou le dernier seul ; et quand même le dernier seul
// ne tient pas (le long titre d'un jeu, au-dessus de sa page « Corriger les informations »), son nom PASSE À LA LIGNE
// (`gf-crumbs-wrap`, launcher.css) : un niveau n'est jamais coupé. `trail` : [{ label, to }], le plus haut d'abord. À la
// manette, B remonte d'un niveau comme toujours ; la croix atteint les niveaux du fil comme n'importe quel lien (BAS
// depuis Retour).
function Crumbs({ trail }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const list = useRef(null);
    const [fold, setFold] = useState(0);
    // Les replis possibles : deux au-delà de deux niveaux, aucun sinon ; un de plus, et le dernier niveau passe à la ligne.
    const folds = trail.length >= 3 ? 2 : 0;
    const wrapped = fold > folds;
    const level = Math.min(fold, folds);
    const shown = level === 0 ? trail
        : level === 1 ? [trail[0], null, ...trail.slice(-2)]
            : [trail[0], null, trail[trail.length - 1]];

    // Replier tant que ça déborde ; tout redéplier quand la place change. Un niveau se réduit avant que la liste ne
    // déborde (`min-width: 0`, launcher.css) : un niveau réduit, c'est déjà trop long — sans ce second regard, le fil ne
    // se repliait jamais, il tronquait (vu au chantier 53, P8, sur les cinq niveaux d'un jeu).
    useLayoutEffect(() => {
        const node = list.current;
        const overflowing = node && (node.scrollWidth > node.clientWidth + 1 || Array.from(node.children).some((item) => item.scrollWidth > item.clientWidth + 1));

        if (overflowing && !wrapped) {
            setFold(fold + 1);
        }
    });
    // La place, c'est l'en-tête de la page (`.gf-page-header`), pas le fil : replié, le fil rétrécit, et le redéplier à
    // chaque repli le ferait battre.
    useEffect(() => {
        const node = list.current?.closest(".gf-page-header") || list.current;

        if (!node || typeof ResizeObserver === "undefined") {
            return undefined;
        }
        let width = node.clientWidth;
        const observer = new ResizeObserver(() => {
            if (Math.abs(node.clientWidth - width) > 1) {
                width = node.clientWidth;
                setFold(0);
            }
        });
        observer.observe(node);

        return () => observer.disconnect();
    }, []);

    return (
        <nav className={`gf-crumbs ${wrapped ? "gf-crumbs-wrap" : ""}`} aria-label={t("breadcrumb")}>
            <ol ref={list} className="breadcrumb">
                {shown.map((crumb, index) => (crumb === null
                    ? <li key="more" className="breadcrumb-item gf-crumb-more" aria-hidden="true">…</li>
                    : (
                        <li key={crumb.to} className="breadcrumb-item">
                            <a href={crumb.to} data-nav data-key={`crumb-${index}`} onClick={(event) => {
                                event.preventDefault();
                                navigate(crumb.to);
                            }}>{crumb.label}</a>
                        </li>
                    )))}
            </ol>
        </nav>
    );
}

// L'EN-TÊTE D'UNE PAGE DES RÉGLAGES, COMME CELUI DE LA FICHE DES JEUX (le porteur, 2026-11-14 : « faire comme dans la
// fiche des jeux, avoir le bouton "(<) Le titre" et ensuite en deuxième ligne le fil d'Ariane. Il faut que le titre
// soit intégré dans la page et non pas prendre le titre des offcanvas. Pas de border bottom sous le titre. ») : le rond
// de Retour (ICON_BUTTON, la sorte `round` : primary dans les Réglages) et le titre à côté, de la taille et du style
// de celui de la bibliothèque (`.ltitle`, Library.jsx) ; le fil d'Ariane SOUS LE TITRE, plus petit, dans la colonne
// du titre (le porteur, 2026-11-14, chantier 53, P13 : « pour essayer, peut-on mettre la fil d'Ariane en dessous du
// titre et en plus petit ? ») — le rond, au milieu de leur hauteur, à gauche des deux. Il était sur sa propre ligne,
// au bord gauche, sous le rond (P11). Aucun trait dessous.
//
// RIEN D'AUTRE (le porteur, 2026-11-14 : « le fil d'Ariane affiche "Streamex" après un retour à la ligne ») : PAS DE
// SOUS-TITRE. Ce qu'une page disait sous son fil (la façon dont une console se joue, la sorte d'une source, le nombre de
// jeux d'un disque) devient une ligne de sa section « Détails » quand elle en a une, ou disparaît.
//
// `enter` : le data-key de la commande où BAS mène depuis Retour (un saut déclaré, `data-down`, libs/input.js), quand
// la croix ne la trouverait pas d'elle-même — un interrupteur au bord droit sous des renseignements, qu'elle saute pour
// la ligne pleine largeur du dessous (sa règle : la même colonne d'abord).
// `remote` : le nom du serveur fédéré qu'on administre d'ici (chantier 73) — une pastille orange à côté du titre, pour
// ne jamais confondre ses réglages avec les nôtres.
export function PageHeader({ title, onBack, trail = [], enter = null, remote = "" }) {
    const { t } = useTranslation();
    const button = useButton();

    return (
        <header className="gf-page-header d-flex align-items-center gap-3">
            <button type="button" className={`${ICON_BUTTON} ${button("round")}`} data-nav data-key="sp-back" data-sound="back" data-down={enter || undefined}
                aria-label={t("back")} title={t("back")} onClick={onBack}>
                <i className="bi bi-chevron-left"></i>
            </button>
            <div className="flex-grow-1 min-w-0">
                <h1 className="ltitle m-0 text-break">{title}{remote && <span className="badge rounded-pill text-bg-warning ms-2 align-middle fs-6" style={{ textShadow: "none", WebkitTextStroke: 0 }}>{remote}</span>}</h1>
                {trail.length > 0 && <Crumbs trail={trail} />}
            </div>
        </header>
    );
}

// UN GROUPE : son intitulé (mb-3 sous lui, ou sous son aide), son aide, puis son contenu — des lignes dans
// `Rows`, ou ce qui s'y pose seul (un texte, des boutons, des vignettes). `badge` : ce qui suit l'intitulé.
// NI TRAIT SOUS LA SECTION, NI INTITULÉ EN CAPITALES : le commit 8cf5e34d les avait posés avant le choix de la
// direction C ; le porteur a validé C telle que dessinée le 2026-11-13 (« très bien comme ça ») — les groupes
// sont des conteneurs arrondis, leur intitulé est en minuscules.
export function Section({ title = null, help = null, badge = null, className = "", children }) {
    return (
        <section className={`mt-4 ${className}`}>
            {title && <h2 className={GROUP_HEADING}>{title}{badge && <span className="ms-2 align-middle">{badge}</span>}</h2>}
            {help && <p className="gf-group-help mb-3">{help}</p>}
            {children}
        </section>
    );
}

// LE CONTENEUR ARRONDI D'UN GROUPE : une list-group de Bootstrap. Ce qu'il est se lit à ce qu'il contient, une fois,
// dans launcher.css (`.gf-list`), et aucune page ne le déclare (le porteur, 2026-11-14 : « quand une section est utilisée
// comme étant un formulaire alors on n'affiche pas les lignes de division mais on garde un mb-3. Et quand c'est pour les
// liens, les divisions doivent être remplies de gauche à droite. » ; puis « Certains formulaires ont toujours la
// séparation horizontale entre les form groups. Il faut faire comme les autres. » et « Dans manettes aussi il y a des
// traits dans le formulaire. ») :
//   - UN FORMULAIRE — tout groupe qui tient un contrôle (un interrupteur, un sélecteur, un champ, une glissière, un choix
//     segmenté ou des cases : un `input`, un `select`, un `textarea`) ou le bouton d'un formulaire (SubmitRow), qu'il
//     s'enregistre par un bouton ou qu'il s'applique sur place : ni trait entre ses lignes, un mb-3 de l'une à l'autre ;
//     ses interrupteurs sans icône (« évite de mettre l'icône à gauche quand c'est un Switch dans un formulaire ») ;
//   - UN GROUPE DE LIENS — fait seulement de liens (NavRow) et de renseignements (InfoRow), ou de leurs lignes fantômes et
//     de la ligne d'une liste vide : un trait entre deux lignes, d'un bord à l'autre ; ses lignes gardent leur icône.
// Un groupe qui mêle les deux est un formulaire (les candidats sous la recherche d'un service, le menu de la partie) : une
// page qui veut des traits à ses liens les pose dans un groupe à part (les touches d'une manette, sous le choix de
// l'appareil).
export function Rows({ className = "", children }) {
    return <div className={`list-group gf-list ${className}`}>{children}</div>;
}

// LE BOUTON D'UN GROUPE, dans son encadré, après son dernier contrôle (le porteur, 2026-11-13) : la dernière ligne de
// `Rows`, qui porte le FormButton du groupe — et fait de ce groupe un formulaire (Rows).
export function SubmitRow({ children }) {
    return <div className="list-group-item gf-row gf-row-submit">{children}</div>;
}

// La pastille d'une ligne : l'icône dans un carré de la couleur du thème.
export function Chip({ icon, tone = "slate" }) {
    return <span className={`gf-chip tone-${tone}`} aria-hidden="true"><i className={`bi ${icon}`}></i></span>;
}

// Le texte d'une ligne : son libellé, sa valeur DESSOUS (comme une ligne d'informations, InfoRow ; le porteur, 2026-11-16 :
// « Le lien Vidéo n'est pas affiché comme sur 2 lignes comme on s'est dit »), puis son aide.
function RowText({ label, value = null, help, emphasis = false, children = null }) {
    return (
        <span className="gf-row-text">
            <span className={`gf-row-label navrow-title ${emphasis ? IN_SERVICE : ""}`}>{label}</span>
            {value !== null && value !== undefined && value !== "" && <span className="gf-row-value text-wrap text-break">{value}</span>}
            {help && <small className="gf-row-help">{help}</small>}
            {children}
        </span>
    );
}

// CE QU'UNE LIGNE FAIT, DIT À SON BORD DROIT (le porteur, 2026-11-14 : « liens qui vont nulle part ou qui ouvrent des
// modal ne doivent pas avoir la flèche droite ">". Remplace par un icône "?" si c'est une question ou "i" si c'est une
// information, ou une icône adéquate à la demande, ou sinon rien. ») — défini ICI une fois, par ce que la ligne fait
// (`opens`), jamais page par page :
//   page     elle ouvre une page ou un sous-panneau : le chevron ;
//   confirm  elle pose une question, la fenêtre de confirmation (DangerRow, useConfirm) : « ? » ;
//   info     elle montre un renseignement : « i » ;
//   link     elle ouvre une adresse d'un autre site, dans un autre onglet : la flèche qui sort ;
//   action   elle agit sur place (copier, télécharger, relire, basculer, lancer) : rien.
const ROW_ENDS = { page: "bi-chevron-right", confirm: "bi-question-circle", info: "bi-info-circle", link: "bi-box-arrow-up-right", action: null };

// UNE LIGNE QUI OUVRE (une page, un sous-panneau) — ou, pour un geste qui n'appartient à aucun groupe, qui AGIT (le
// porteur, 2026-11-13 : un bouton orphelin est une ligne) : toute la ligne se touche, ce qu'elle fait au bord droit
// (`opens`, ROW_ENDS), la valeur actuelle sous son libellé (`value`, comme InfoRow). `lead` : une vignette ou un avatar à la place de la
// pastille. `pending` : le geste est en cours — le tourniquet au bord droit, la ligne ne se touche plus. `disabled` :
// grisée (sa raison en `help`). `emphasis` : ce que la console prend sans choix (IN_SERVICE, libs/ui.js).
//
// `progress` (0 à 100) : UN TÉLÉCHARGEMENT EN COURS depuis cette ligne (le porteur, 2026-11-13 : « Les
// téléchargements doivent aussi afficher la progression, pas juste le spinner ») — le pourcentage au bord droit, une
// fine barre `progress` de Bootstrap sous le texte, « reçu sur total » en `help` ; -1 tant que le total n'est pas
// connu, le tourniquet. La ligne ne se touche plus, mais garde la lumière de la croix (elle n'est pas grisée : c'est
// elle qui travaille).
export default function NavRow({ id, icon = "bi-chevron-right", tone = "slate", label, help = null, value = null, lead = null, danger = false, disabled = false, pending = false, progress = null, emphasis = false, opens = "page", onOpen }) {
    const busy = progress !== null;
    const known = busy && progress >= 0;
    const end = ROW_ENDS[opens];

    return (
        <button type="button" className={`list-group-item list-group-item-action gf-row navrow ${danger ? "danger" : ""}`} data-nav data-key={id} data-opens={opens} disabled={disabled || pending}
            aria-busy={pending || busy || undefined} aria-disabled={busy || undefined} onClick={busy ? undefined : onOpen}>
            {/* LE GESTE EN COURS (`pending`) : la pastille de gauche devient la roue (le porteur, 2026-11-17, un rescan :
                « l'icône de gauche se change en spinner. Au lieu de mettre le spinner en sous-titre »). */}
            {pending && !lead ? <span className={`gf-chip tone-${tone}`} aria-hidden="true"><span className="spinner-border spinner-border-sm" role="status"></span></span>
                : lead || <Chip icon={icon} tone={tone} />}
            <RowText label={label} value={value} help={help} emphasis={emphasis}>
                {known && (
                    <span className="progress gf-progress" role="progressbar" aria-label={label} aria-valuenow={progress} aria-valuemin="0" aria-valuemax="100">
                        <span className="progress-bar" style={{ width: `${progress}%` }}></span>
                    </span>
                )}
            </RowText>
            {known ? <span className="gf-row-value fw-semibold">{`${progress} %`}</span>
                : (pending && lead) || busy
                    ? <span className="spinner-border spinner-border-sm text-primary flex-shrink-0" role="status"></span>
                    : !pending && end && <i className={`bi ${end} gf-row-chevron`} aria-hidden="true"></i>}
        </button>
    );
}

// UNE LIGNE QUI OUVRE, ET UN GESTE À SON BORD DROIT (le porteur, 2026-11-16, « Ma partie en cours » d'En direct : « Juste
// un élément avec le titre, le sous-titre […] Et à droite le bouton power off ») : la ligne d'une NavRow — ses classes, sa
// tête (`lead` ou la pastille), son texte, la lumière de la croix — sauf son bord droit, où se pose un bouton carré à icône
// seule (SQUARE_ICON_BUTTON, la règle des boutons : son nom en aria-label et en title). Un bouton dans un bouton n'existe
// pas : la ligne est une boîte de Bootstrap qui tient DEUX boutons côte à côte, la NavRow d'abord, qui prend la place et
// toute la hauteur (le voile de la manette la couvre de haut en bas, jusqu'au geste), puis le geste. Toucher la ligne fait `onOpen` ;
// toucher le bouton, `action.onClick`. À LA MANETTE : la croix atteint la ligne (A l'ouvre), DROITE le bouton (A agit),
// GAUCHE revient. Le bord droit étant le bouton, la ligne n'y dit rien (ROW_ENDS). `action` : { id, icon, label, kind
// (la sorte du bouton, useButton), pending (le tourniquet à sa place), disabled, onClick }.
export function SplitRow({ id, icon = "bi-chevron-right", tone = "slate", lead = null, label, help = null, onOpen, action }) {
    const button = useButton();

    return (
        <div className="list-group-item gf-row p-0 pe-3">
            <button type="button" className="list-group-item list-group-item-action gf-row navrow align-self-stretch min-w-0" data-nav data-key={id} data-opens="action" onClick={onOpen}>
                {lead || <Chip icon={icon} tone={tone} />}
                <RowText label={label} help={help} />
            </button>
            <button type="button" className={`${SQUARE_ICON_BUTTON} ${button(action.kind || "other")}`} data-nav data-key={action.id} disabled={action.disabled || action.pending}
                aria-label={action.label} title={action.label} onClick={action.onClick}>
                {action.pending ? <span className="spinner-border spinner-border-sm" role="status"></span> : <i className={`bi ${action.icon}`}></i>}
            </button>
        </div>
    );
}

// UNE LIGNE LIBRE dans un groupe (une scène, un calendrier) : le contenu sur toute la largeur, sans pastille.
export function BlockRow({ children }) {
    return <div className="list-group-item gf-row gf-row-stack">{children}</div>;
}

// UNE LIGNE QUI SE RÈGLE SUR PLACE : l'interrupteur de Skeleton (Form.Switch) au bord droit ; toucher la ligne le
// bascule aussi. À la manette, la lumière est sur l'interrupteur, A le bascule. PAS D'ICÔNE : un interrupteur est un
// contrôle, son groupe est donc toujours un formulaire (Rows), et « évite de mettre l'icône à gauche quand c'est un
// Switch dans un formulaire » (le porteur, 2026-11-14).
export function SwitchRow({ id, label, help = null, checked, disabled = false, onChange }) {
    const flip = (event) => {
        if (!disabled && !event.target.closest(".form-check, label")) {
            onChange(!checked);
        }
    };

    return (
        <div className={`list-group-item gf-row gf-row-switch ${disabled ? "opacity-50" : ""}`} onClick={flip}>
            <RowText label={label} help={help} />
            <Form.Switch name={id} context={fieldContext(id, Boolean(checked), (on) => onChange(Boolean(on)), disabled)}
                props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
        </div>
    );
}

// UNE LIGNE DE RENSEIGNEMENT, comme « À propos du téléphone » d'Android (le porteur, 2026-11-16 : « tu peux remplacer le
// style des fiches pour les mettre comme sur l'image. Au lieu de les mettre en 2 colonnes. » ; « on garde le même ui on a
// déjà, c'est juste la disposition qui change ») : sa pastille, son libellé, et SA VALEUR SOUS LE LIBELLÉ, à gauche, sur
// toute la largeur qui reste — une valeur longue (un chemin, une adresse, l'éditeur d'un jeu) passe à la ligne sans rien
// pousser, un badge (« Jouable ») ou des touches (les raccourcis) s'y rangent de même. Aucun geste. Une seule disposition :
// aucune page ne remet la valeur au bord droit. `mono` : la valeur en chasse fixe, plus petite (un chemin, un fichier,
// l'heure d'une ligne de journal). `help` : ce que la ligne désigne, sous sa valeur, plus petit, comme l'aide d'une NavRow — un service de métadonnées sans compte,
// qui n'a pas de page à ouvrir (chantier 53, P10), un statut et qui l'a posé.
export function InfoRow({ icon = null, tone = "slate", lead = null, label, value, help = null, mono = false }) {
    if (value === null || value === undefined || value === "") {
        return null;
    }

    return (
        <div className="list-group-item gf-row">
            {lead || (icon && <Chip icon={icon} tone={tone} />)}
            <span className="gf-row-text">
                <span className="gf-row-label">{label}</span>
                <span className={`gf-row-value text-wrap text-break ${mono ? "font-monospace small" : ""}`}>{value}</span>
                {help && <small className="gf-row-help">{help}</small>}
            </span>
        </div>
    );
}

// UNE LIGNE EMPILÉE : Form.Group de Skeleton (le libellé, l'aide) puis le contrôle sur toute la largeur, et son
// bouton accolé (`addon`, un IconAddon) à côté. Pas d'icône.
function StackRow({ label, help, addon = null, children }) {
    return (
        <div className="list-group-item gf-row gf-row-stack">
            <Form.Group label={label} helpText={help} props={{ className: "mb-0 w-100" }}>
                {addon ? <div className="d-flex gap-2 align-items-start"><div className="flex-grow-1 min-w-0">{children}</div>{addon}</div> : children}
            </Form.Group>
        </div>
    );
}

// Un sélecteur (Form.Select) : un choix parmi plusieurs, un filtre aussi. À la manette, GAUCHE et DROITE passent à
// l'option suivante. `options` : [{ value, label, attributes }] comme Skeleton les lit.
export function SelectRow({ id, label, help = null, value, options, onChange, disabled = false, addon = null }) {
    return (
        <StackRow label={label} help={help} addon={addon}>
            <Form.Select name={id} context={fieldContext(id, value, onChange, disabled)} options={options}
                props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
        </StackRow>
    );
}

// Une glissière (Form.Range), sa valeur lue à côté ; GAUCHE et DROITE la déplacent d'un pas.
export function RangeRow({ id, label, help = null, value, min, max, step = 1, onChange, disabled = false }) {
    return (
        <StackRow label={label} help={help}>
            <Form.Range name={id} context={fieldContext(id, value, onChange, disabled)} min={min} max={max} step={step}
                props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
        </StackRow>
    );
}

// Un choix de deux à quatre options, toutes visibles : le motif de Bootstrap, `btn-check` et ses étiquettes en
// boutons — la choisie pleine, les autres à contour, partout (`choiceButton`, libs/ui.js). Au-delà de quatre, un
// sélecteur. `options` : [[valeur, libellé], …] ; `isDisabled(valeur)`. La croix s'arrête sur chaque case ; A la
// choisit.
export function ChoiceRow({ id, label, help = null, value, options, onChange, isDisabled = () => false }) {
    return (
        <StackRow label={label} help={help}>
            <div className="btn-group w-100" role="group" aria-label={label}>
                {options.map(([option, text]) => {
                    const key = `${id}-${option || "default"}`;

                    return (
                        <React.Fragment key={option || "default"}>
                            <input type="radio" className="btn-check" name={id} id={key} autoComplete="off" checked={option === value}
                                disabled={isDisabled(option)} onChange={() => onChange(option)} />
                            <label className={`${choiceButton(option === value)} btn-sm`} htmlFor={key} data-nav data-key={key} tabIndex={0}
                                aria-pressed={option === value} onClick={(event) => {
                                    // A à la manette « clique » l'étiquette : la case suit, comme au doigt.
                                    if (event.detail === 0 && !isDisabled(option)) {
                                        event.preventDefault();
                                        onChange(option);
                                    }
                                }}>
                                {text}
                            </label>
                        </React.Fragment>
                    );
                })}
            </div>
        </StackRow>
    );
}

// Un choix dont les options se lisent (une phrase, une aide sous chacune) : LES CARTES D'OPTION DE SKELETON
// (Form.SkeletonRadio — le porteur, 2026-11-16 : « dans choix des flux, change le radio button en skeleton.SkeletonRadio »),
// l'une sous l'autre, chacune son icône, son libellé et son aide. Skeleton tient l'option choisie par son libellé : la
// valeur s'y traduit ici, une fois. La croix les parcourt (PadReach), A en choisit une ; chaque carte reçoit sa clé
// (`<id>-<rang>`), pour que la lumière arrive sur celle qui est choisie (SidePanel, `firstKey`). `options` : [{ value,
// label, help, icon }].
export function RadioRow({ id, label = null, help = null, value, options, onChange, disabled = false }) {
    const box = useRef(null);
    const chosen = options.find((option) => option.value === value);
    const pick = (title) => {
        const picked = options.find((option) => option.label === title);

        if (picked) {
            onChange(picked.value);
        }
    };

    useEffect(() => {
        box.current?.querySelectorAll("button").forEach((card, index) => card.setAttribute("data-key", `${id}-${index}`));
    });

    return (
        <StackRow label={label} help={help}>
            <PadReach>
                <div ref={box}>
                    <Form.SkeletonRadio name={id} context={fieldContext(id, chosen ? chosen.label : null, pick, disabled)}
                        options={options.map((option) => ({ icon: `bi ${option.icon || "bi-circle"}`, title: option.label, subtitle: option.help || "" }))} />
                </div>
            </PadReach>
        </StackRow>
    );
}

// Un champ de formulaire (Form.Text, Form.Password, Form.IntFloat, Form.Longtext) dans le contexte d'un formulaire de
// Skeleton (`context`, formContext) ou seul (`value` + `onChange`). `kind` : text, password, new-password (un mot de passe
// choisi, sa jauge et la règle : NewPassword, chantier 55), number, longtext.
export function TextRow({ id, label, help = null, context = null, value = "", onChange = () => {}, kind = "text", props = {}, addon = null, min, max, step }) {
    const own = context || fieldContext(id, value, onChange);
    const field = { "data-nav": true, "data-key": id, autoComplete: "off", "aria-label": label, ...props };

    return (
        <StackRow label={label} help={help} addon={addon}>
            {kind === "password" ? <Form.Password name={id} context={own} props={field} />
                : kind === "new-password" ? <NewPassword name={id} context={own} props={field} />
                    : kind === "number" ? <Form.IntFloat name={id} context={own} min={min} max={max} step={step} props={field} />
                        : kind === "longtext" ? <Form.Longtext name={id} context={own} props={field} />
                            : <Form.Text name={id} context={own} props={field} />}
        </StackRow>
    );
}

// UN BOUTON ACCOLÉ À UN CHAMP : l'icône seule, son nom en aria-label et en title (la règle des boutons).
export function IconAddon({ id, icon, label, kind = "other", disabled = false, onClick }) {
    const button = useButton();

    return (
        <button type="button" className={`${button(kind)} flex-shrink-0`} data-nav data-key={id} aria-label={label} title={label} disabled={disabled} onClick={onClick}>
            <i className={`bi ${icon}`}></i>
        </button>
    );
}

// UNE VALEUR À RECOPIER : le champ en lecture seule, en chasse fixe, et l'icône qui copie. EN HTTP AUSSI : sur une adresse
// du réseau local (http://192.168.…), le navigateur n'a pas de presse-papiers (`navigator.clipboard` n'existe qu'en
// contexte sûr) ; `copyTextToClipboard` (libs/share-link.js) prend alors l'ancien chemin. Si rien n'a copié, le dire :
// le champ reste là, sélectionné au toucher, pour copier à la main.
export function CopyField({ id, label = null, help = null, value }) {
    const { t } = useTranslation();
    const copy = async () => toast(t((await copyTextToClipboard(value)) ? "settings.copied" : "settings.copyFailed"));

    return (
        <TextRow id={id} label={label} help={help} value={value} props={{ readOnly: true, className: "font-monospace", onFocus: (event) => event.target.select() }}
            addon={<IconAddon id={`${id}-copy`} icon="bi-clipboard" label={t("settings.copy")} onClick={copy} />} />
    );
}

// L'AVATAR d'un compte, à la place de la pastille d'une ligne : ses initiales dans un rond.
export function Avatar({ name }) {
    const initials = (name || "").split(/\s+/).filter(Boolean).slice(0, 2).map((word) => word[0].toUpperCase()).join("") || "?";

    return <span className="gf-avatar" aria-hidden="true">{initials}</span>;
}

// LE NOM D'UN COMPTE, ET SON RÔLE QUAND IL ADMINISTRE (le porteur, 2026-11-14 : « quand quelqu'un est administrateur (ou
// root) mettre un icône vert de shield à côté de son nom » ; « Pour les administrateurs on rajoute le shield dans son
// nom ») : le badge vert, défini ICI une fois — la liste des membres, la carte « Mon compte », partout où un nom de
// compte s'affiche. `admin` : son rôle est root ou administrateur (`role_level` 0 ou 1, comme le serveur le juge). Son
// identifiant va dans le sous-titre, seul, un @ devant (`atUsername`).
//
// LE BADGE EST LA COCHE DENTELÉE DE BOOTSTRAP ICONS (le porteur, 2026-11-14, chantier 53, P13 : « Remplace le bouclier
// par un check avec patch-check-fill de bootstrap ») : `bi-patch-check-fill`, vert (`text-success`), la police d'icônes
// du launcher — plus de bouclier.
export function AccountName({ name, admin = false }) {
    const { t } = useTranslation();

    return (
        <>
            {name}
            {admin && <AdminMark label={t("members.roleAdmin")} className="ms-2" />}
        </>
    );
}

// L'INSIGNE DE L'ADMINISTRATION, défini ICI une fois : à côté du nom d'un administrateur (AccountName), et à côté du
// titre d'un groupe des Réglages qu'un administrateur seul voit — « Le serveur », « Les personnes et le partage » (le
// porteur, 2026-11-16 : « rajouter Icône patch fill à côté du texte le serveur et personnes et partage pour faire savoir
// que c'est de l'admin »). `label` : ce qu'il dit à un lecteur d'écran, et dans sa bulle.
export function AdminMark({ label, className = "" }) {
    return <i className={`bi bi-patch-check-fill text-success ${className}`} role="img" aria-label={label} title={label}></i>;
}

export const isAdministrator = (roleLevel) => Number(roleLevel) <= 1;

export const atUsername = (username) => `@${username}`;

// UNE VIGNETTE EN TÊTE D'UNE LIGNE, à la place de la pastille (une image d'une console ou d'un jeu, une image proposée
// par une recherche ; chantier 53, P9) : 56 px au format 4:3, la vignette d'un état (`.thumb`, Mes sauvegardes) ; sans
// adresse, l'icône d'une image. `fit` : `cover` pour un fond, `contain` pour une jaquette ou un logo. Une image d'un
// fournisseur se charge chez lui, sans dire d'où vient la page (`referrerPolicy`).
export function Thumb({ url = null, fit = "contain" }) {
    return (
        <span className="thumb ratio ratio-4x3 rounded overflow-hidden bg-body-secondary flex-shrink-0">
            {url ? <img className={`object-fit-${fit}`} src={url} alt="" loading="lazy" referrerPolicy="no-referrer" />
                : <span className="d-flex align-items-center justify-content-center text-body-tertiary"><i className="bi bi-image" aria-hidden="true"></i></span>}
        </span>
    );
}

// UN CHOIX DE FICHIER : Form.File de Skeleton (sa copie), dans une ligne empilée de son groupe — sa zone de choix, que
// Skeleton dessine seul. `upload.context` : le contexte de formulaire où il range le fichier (Invitation.jsx, l'image
// d'un code QR). Les images ne s'envoient plus : elles se choisissent dans une recherche (chantier 53, P9).
export function FileRow({ id, label, help = null, upload, accept = "image/*", maxFileSize = 16 * 1024 * 1024 }) {
    return (
        <StackRow label={label} help={help}>
            <PadReach>
                <Form.File name={id} context={upload.context} accept={accept} maxFileSize={maxFileSize} />
            </PadReach>
        </StackRow>
    );
}

// ANNULER UN TÉLÉCHARGEMENT EN COURS (le porteur, 2026-11-14, « 3. A »), OU LE RETIRER DE LA FILE (2026-11-16) : une
// ligne juste sous la ligne de ce qui télécharge, la même partout où l'on télécharge (un émulateur, un BIOS, un
// correctif, un jeu d'un serveur fédéré, En direct). Pas de fenêtre de confirmation : annuler ne détruit rien de ce qui
// était là. La commande `download.cancel` part (cancelDownload, libs/downloads.js) ; le kernel arrête la copie, ou sort
// le jeu de la file, le sujet finit « annulé », et la ligne de la source redevient elle-même — l'avis le dit
// (downloadOutcome). Un jeu qui attend son tour (`state: queued`) dit « Retirer de la file ». Partie avec lui, elle rend
// la lumière à la ligne de la source. `id` : sa clé, quand une page en montre plusieurs (En direct).
// Le geste « Annuler » / « Retirer de la file » d'un téléchargement (DownloadRow). `id` : la clé du bouton qui le porte.
function useDownloadCancel(running, id) {
    const { t } = useTranslation();
    const [pending, setPending] = useState(false);
    const touched = useRef(false);

    // Touchée, elle se grise le temps de la réponse, et un bouton grisé perd la lumière : on retient qu'elle l'avait.
    useLayoutEffect(() => {
        const row = document.querySelector(`[data-key="${id}"]`);

        return () => {
            const source = row?.previousElementSibling;

            if (row && source && (row === document.activeElement || (touched.current && document.activeElement === document.body))) {
                requestAnimationFrame(() => source.isConnected && focusElement(source, true));
            }
        };
    }, [id]);

    const cancel = async () => {
        touched.current = document.activeElement?.dataset?.key === id;
        setPending(true);
        const answer = await cancelDownload(running);

        if (!answer.ok) {
            setPending(false);
            toast(answer.status === 404 ? t("downloads.alreadyOver") : answer.errorMessage || t("settings.actionFailed"));
        }
    };

    return { pending, cancel };
}



// UN TÉLÉCHARGEMENT SUR UNE LIGNE (En direct ; le porteur, 2026-11-17 : « la progression à gauche à la place de l'icône et
// à droite le bouton annuler […] actuellement sur 2 lignes ça fait très moche », et « pareil pour les téléchargements en
// attente ») : la pastille de gauche est sa progression (le pourcentage dans un anneau qui se remplit ; le tourniquet
// tant que le total n'est pas connu ; le sablier d'un jeu de la file), le sous-titre son ÉTAPE et « reçu sur total »
// (« Téléchargement », puis « Décompression » pour un jeu reçu en archive), et le bouton du bord droit « Annuler » ou
// « Retirer de la file ». La ligne elle-même n'ouvre rien. LA SEULE LIGNE D'UN TÉLÉCHARGEMENT, partout où il se voit (En
// direct, la page d'un jeu, d'un correctif, d'un émulateur) : un écart se définit une fois. `help` remplace le sous-titre.
const PHASE_TEXTS = { download: "downloads.phaseDownload", unpack: "downloads.phaseUnpack", sheet: "downloads.phaseSheet", library: "downloads.phaseLibrary", remove: "downloads.phaseRemove" };

export function DownloadRow({ entry, id, label, help = null }) {
    const { t } = useTranslation();
    const queued = entry.state === "queued";
    const received = formatSize(entry.received) || "0 B";
    const subtitle = help ?? (queued ? t("downloads.queuedAt", { position: entry.position })
        : `${t(PHASE_TEXTS[entry.phase] || "downloads.phaseDownload")} · ${entry.total
            ? t("consoles.receivedOf", { received, total: formatSize(entry.total) }) : t("consoles.received", { received })}`);
    const { pending, cancel } = useDownloadCancel(entry, `${id}-cancel`);
    const percent = !queued && entry.total ? Math.min(100, Math.round((entry.received / entry.total) * 100)) : null;
    const lead = queued ? <Chip icon="bi-hourglass-split" tone="slate" />
        : percent === null ? <span className="gf-chip tone-teal" aria-hidden="true"><span className="spinner-border spinner-border-sm" role="status"></span></span>
            : <span className="gf-chip gf-chip-progress tone-teal" style={{ "--p": percent }} role="progressbar" aria-label={label}
                aria-valuenow={percent} aria-valuemin="0" aria-valuemax="100"><span>{percent}</span></span>;

    // UNE SUPPRESSION (`kind: remove`) ne s'annule pas : un jeu à moitié effacé ne servirait plus. Sa ligne n'a pas de bouton.
    if (entry.kind === "remove") {
        return <NavRow id={id} lead={lead} label={label} help={subtitle} opens="action" />;
    }

    return (
        <SplitRow id={id} lead={lead} label={label} help={subtitle} onOpen={() => { }}
            // Annuler, c'est arrêter : la sorte « danger » (le porteur, 2026-11-17 : « Le bouton annuler du téléchargement n'est
            // pas danger mais primary »).
            action={{ id: `${id}-cancel`, kind: "danger", icon: queued ? "bi-dash-lg" : "bi-x-lg", label: t(queued ? "downloads.dequeue" : "downloads.cancel"),
                // Le téléchargement fini, on ne revient plus en arrière : la décompression et la fiche ne s'annulent pas (le
                // porteur, 2026-11-17 : « Il faut aussi grisé le bouton annuler après le téléchargement »).
                pending, disabled: !entry.downloadId || (entry.phase && entry.phase !== "download"), onClick: cancel }} />
    );
}

// LES STICKS INVISIBLES (#758) : la même ligne dans Réglages › Manettes et dans le menu d'une partie — un seul texte, une
// seule valeur (engine/libs/gamepadPreferences.js), gardée dans ce navigateur. La manette à l'écran la suit aussitôt, partie
// en cours comprise.
export function InvisibleSticksRow({ id }) {
    const { t } = useTranslation();
    const { invisibleSticks } = useGamepadPreferences();

    return <SwitchRow id={id} label={t("gamepads.invisibleSticks")} help={t("gamepads.invisibleSticksHelp")} checked={invisibleSticks} onChange={setInvisibleSticks} />;
}

// L'OPACITÉ DE LA MANETTE À L'ÉCRAN (#758) : la même glissière dans l'éditeur de disposition (Réglages › Manettes ›
// Modifier la disposition) et dans le menu d'une partie — l'opacité de la disposition, de 1 à 100 % par pas de 1 % (le
// porteur, 2026-11-17 : « la limite de l'opacité des boutons soit de 1% à 100% dans les glisseurs » ; 25 % au plus bas
// jusque-là), son pourcentage dans le libellé. Ce qu'elle règle (un emplacement en cours d'édition, ou celui en jeu) est
// l'affaire de la page.
export const PAD_OPACITY_MIN = 0.01;

export function PadOpacityRow({ id, value, onChange }) {
    const { t } = useTranslation();

    return (
        <RangeRow id={id} label={`${t("layout.opacity")} — ${Math.round(value * 100)} %`} help={t("layout.opacityHelp")} value={value} min={PAD_OPACITY_MIN} max={1} step={0.01}
            onChange={(next) => onChange(Number(next))} />
    );
}

// L'ORIENTATION DE L'ÉCRAN (#406, #410 ; #758) : les mêmes lignes dans Réglages › Application et dans le menu d'une partie —
// UNE valeur, gardée dans ce navigateur (libs/screenOrientation.js), que lit la page de jeu. Dans l'application Android
// (`turnsTheApp`), l'activité pivote tout ; dans un navigateur, choisir un paysage allume le plein écran et verrouille
// l'écran (libs/orientation.js) — un navigateur qui ne sait pas, ou qui refuse, le dit (`onFailed`) : seule l'image d'une
// partie tourne alors (use-screen-rotation.js). Choisi pendant une partie, il s'applique tout de suite, sans la couper.
// « Portrait, demi-écran » range l'interface dans le haut de l'écran dès qu'une manette est branchée, à la hauteur réglée
// sous lui. SANS MANETTE, LE DEMI-ÉCRAN NE SE CHOISIT PAS (le porteur, 2026-11-16) : son option est grisée, et l'aide dit
// d'en brancher une ; déjà choisi, il reste choisi (la manette peut revenir).
const ORIENTATION_LABELS = { auto: "auto", "landscape-reverse": "clockwise", landscape: "anticlockwise", "portrait-half": "portraitHalf" };

// Qui a une orientation à choisir : l'application, dont l'activité tourne tout ; ailleurs, un appareil qu'on tient en main
// (un pointeur grossier : le doigt).
export const offersOrientation = (turnsTheApp) => turnsTheApp || matchMedia("(pointer: coarse)").matches;

export function OrientationRows({ idPrefix, turnsTheApp, onFailed }) {
    const { t } = useTranslation();
    const orientation = useScreenOrientation();
    const gamepad = usePhysicalGamepad();
    const [height, setHeight] = useState(getHalfScreenHeight);
    const help = orientation === "portrait-half" ? (gamepad ? "portraitHalf" : "portraitHalfWaiting") : !gamepad ? "noGamepad"
        : turnsTheApp ? "app" : canLockOrientation() ? "browser" : "browserCannot";
    const choose = (mode) => {
        onFailed(false);
        chooseOrientation(mode, turnsTheApp).then((applied) => onFailed(!applied));
    };
    const resize = (value) => {
        // Une valeur hors bornes (un chiffre en cours de frappe) n'est ni gardée ni appliquée.
        if (setHalfScreenHeight(value)) {
            setHeight(Number(value));
        }
    };

    return (
        <>
            <SelectRow id={`${idPrefix}-orientation`} label={t("settings.items.orientation.label")} help={t(`settings.orientationHelp.${help}`)} value={orientation} onChange={choose}
                options={SCREEN_ORIENTATIONS.map((mode) => ({ value: mode, label: t(`settings.orientations.${ORIENTATION_LABELS[mode]}`),
                    attributes: mode === "portrait-half" && !gamepad && orientation !== mode ? { disabled: true } : {} }))} />
            {orientation === "portrait-half" && (
                <RangeRow id={`${idPrefix}-half-height`} label={t("settings.items.halfScreenHeight.label")} help={t("settings.items.halfScreenHeight.help", { min: HALF_SCREEN_MIN, max: HALF_SCREEN_MAX })}
                    value={height} min={HALF_SCREEN_MIN} max={HALF_SCREEN_MAX} onChange={resize} />
            )}
        </>
    );
}

// L'avis d'une orientation que l'écran n'a pas prise, sous ses lignes.
export function OrientationFailed({ turnsTheApp }) {
    const { t } = useTranslation();

    return <Note>{t(turnsTheApp ? "settings.orientationFailed" : "settings.orientationRefused")}</Note>;
}

// LES TUILES : les gestes les plus fréquents en tête d'un panneau ou d'une page, deux par rangée ; un nombre impair
// de tuiles, et la dernière prend la rangée (`wide` la force). Une tuile pleine est un réglage allumé (`on`, annoncé
// par aria-pressed) ou le geste mis en avant (`primary`, Reprendre, Nouveau membre). Ce sont des boutons : 3 px, la
// croix y circule.
export function QuickTiles({ items }) {
    const shown = items.filter(Boolean);

    return (
        <div className="gf-tiles mt-3">
            {shown.map((item, index) => (
                <button key={item.id} type="button"
                    className={`btn gf-tile tone-${item.tone || "slate"} ${item.on || item.primary ? "on" : ""} ${item.wide || (shown.length % 2 === 1 && index === shown.length - 1) ? "wide" : ""}`}
                    data-nav data-key={item.id} aria-pressed={item.on === undefined ? undefined : Boolean(item.on)} disabled={item.disabled} onClick={item.onClick}>
                    <i className={`bi ${item.icon}`} aria-hidden="true"></i>
                    <span className="min-w-0">
                        <span className="gf-tile-label d-block">{item.label}</span>
                        {item.value && <span className="gf-tile-value d-block">{item.value}</span>}
                    </span>
                </button>
            ))}
        </div>
    );
}

// UNE LISTE VIDE GARDE SA BOÎTE (le porteur, 2026-11-14 : « Pour toutes les listes vides, il faut afficher au moins un
// lien (un placeholder) grisé avec le message. Sinon ça fait trop moche comme ça. ») : UNE ligne grisée, la forme d'une
// ligne de la liste — sa pastille, le message —, désactivée : ni bouton, ni `data-nav` (la lumière de la croix ne s'y
// pose pas), rien à son bord droit. « Aucun jeu. », ou ce qu'une recherche n'a pas trouvé. Définie ICI une fois : la
// dernière ligne d'une Listing vide, ou EmptyState pour une liste qui ne se feuillette pas.
export function EmptyRow({ icon = "bi-inbox", text }) {
    return (
        <div className="list-group-item gf-row gf-row-empty" aria-disabled="true">
            <Chip icon={icon} tone="slate" />
            <RowText label={text} />
        </div>
    );
}

// RIEN À MONTRER, dans une liste qui n'est pas une Listing (la liste des sauvegardes d'un jeu, les personnes d'une
// partie) : sa boîte et sa ligne grisée (EmptyRow). Le geste qui y remédie est une ligne de son groupe, pas un bouton.
export function EmptyState({ icon = "bi-inbox", text }) {
    return <Rows><EmptyRow icon={icon} text={text} /></Rows>;
}

// Le texte d'une fenêtre de Skeleton : son titre et ses boutons s'écrivent en HTML dans la page (skeleton.js) ;
// un nom de jeu ou de membre n'y doit jamais devenir du balisage.
const escapeHtml = (text) => String(text ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char]));

// LA CONFIRMATION : la fenêtre de Skeleton, `context.modal.open(…, { variant: "confirm" })` (sa copie,
// skeleton/skeleton.js) — TOUTE confirmation du launcher (le porteur, 2026-11-13 : « utilise skeleton.Feedback pour
// les fenêtres de confirmation partout dans le launcher »). `confirm({ title, message, confirmLabel, onConfirm,
// onCancel })`. À LA MANETTE : la fenêtre est le calque du dessus, la lumière y entre sur Annuler, la croix passe
// d'un bouton à l'autre, A active, B annule ; refermée, la lumière revient sur ce qui l'a ouverte. Les sons sont
// ceux de la croix (libs/input.js).
//
// UNE CONFIRMATION EN DEUX TEMPS : `then`, les mêmes champs pour une seconde fenêtre, plus explicite — arrêter la partie
// d'un autre (Live.jsx ; le porteur, 2026-10-25 : deux confirmations « pour éviter les erreurs »). La seconde s'ouvre une
// fois la première refermée, jamais par-dessus elle : la lumière ne se perd pas entre les deux, et revient à la fin sur
// ce qui avait ouvert la première. Seul le `onConfirm` de la dernière agit.
export function useConfirm() {
    const skeleton = useContext(Skeleton.context);
    const { t } = useTranslation();

    // `body` : un contenu à la place du message — un champ à remplir avant de confirmer (la note d'un backup ; le porteur,
    // 2026-11-16 : « mets le champ note dans un modal skeleton au lieu de l'afficher plus bas »), un choix (quitter une
    // partie, Menu.jsx). Il vit dans le portail de Skeleton, sous son fournisseur : son propre état, et ce qu'il garde se
    // lit à la confirmation.
    //
    // `whenClosed` : le geste attend que la fenêtre soit refermée — quitter une partie démonte la page de jeu, et la
    // fenêtre posée dans son élément en plein écran (skeleton.js) partirait avec elle avant d'avoir fini de se refermer.
    // `cancelLabel` : le mot du refus quand « Annuler » ne dit pas ce qu'il fait (une demande de manette : « Garder »).
    return useCallback(function confirm({ title, message = null, body = null, confirmLabel, cancelLabel = null, onConfirm, onCancel = null, then = null, whenClosed = false }, opener = document.activeElement) {
        let release = () => {};
        let confirmed = false;
        const id = skeleton.modal.open(body || <span>{message || title}</span>, {
            variant: "confirm",
            title: message || body ? escapeHtml(title) : null,
            confirmLabel: escapeHtml(confirmLabel || title),
            cancelLabel: escapeHtml(cancelLabel || t("settings.cancel")),
            onConfirm: then || whenClosed ? () => { confirmed = true; } : onConfirm,
            onCancel,
            onClose: () => {
                release();

                if (confirmed && then) {
                    confirm(then, opener);

                    return;
                }
                if (confirmed) {
                    onConfirm();

                    return;
                }
                requestAnimationFrame(() => opener?.isConnected && focusElement(opener, true));
            },
        });
        const node = id && document.getElementById(id);

        if (!node) {
            return;
        }
        node.setAttribute("data-nav-layer", "");
        node.querySelectorAll("button, textarea, input, select").forEach((control) => control.setAttribute("data-nav", ""));
        const cancel = node.querySelector('[data-role="cancel"]');
        release = pushScreenInput({ back: () => cancel?.click() });
        // La fenêtre de Bootstrap prend le focus à la fin de son entrée : la lumière va sur Annuler à ce moment-là.
        node.addEventListener("shown.bs.modal", () => focusElement(cancel, true), { once: true });
    }, [skeleton, t]);
}

// UN GESTE QUI DÉTRUIT : une ligne rouge (une NavRow) de la section « Danger », qui ouvre la fenêtre de confirmation
// de Skeleton — « ? » à son bord droit (`opens="confirm"`) ; grisée quand le geste n'est pas disponible, sa raison en
// `help`. `question` : le texte de la
// fenêtre, son titre étant le geste lui-même (`label`).
export function DangerRow({ id, icon = "bi-trash", label, help = null, question, confirmLabel = null, disabled = false, onConfirm }) {
    const confirm = useConfirm();

    return (
        <NavRow id={`${id}-ask`} icon={icon} tone={disabled ? "slate" : "rose"} label={label} help={help} danger={!disabled} disabled={disabled} opens="confirm"
            onOpen={() => confirm({ title: label, message: question, confirmLabel: confirmLabel || label, onConfirm })} />
    );
}

// LA SECTION « DANGER » (le porteur, 2026-11-13 : « des liens dans une section qu'on va nommer "Danger" »), en bas
// de la page : ses gestes destructifs en lignes. `children` : des DangerRow ; ou les propriétés d'une seule.
export function DangerSection({ children = null, ...row }) {
    const { t } = useTranslation();

    return (
        <Section title={t("danger")}>
            <Rows>{children || <DangerRow {...row} />}</Rows>
        </Section>
    );
}

// LA SECTION « GUIDE » (le porteur, 2026-11-16 : « Rajouter des sections guide avec des liens vers le docs sur le repos »
// ; « Toujours proposer des liens vers la documentation si possible ») : une page qui explique quelque chose renvoie au
// guide du dépôt public qui en parle (libs/links.js, GUIDES), au titre qui en parle quand il y en a un. Sa place : la
// DERNIÈRE de la page, après « Danger » (le porteur, 2026-11-16 : « toujours en dernier. Même après danger. »). `guides` : [[clé, ancre], …], l'ancre facultative.
// UNE SECTION DE TEXTE, PAS UN GROUPE DE LIGNES (TextSection ; le porteur, 2026-11-16 : « C'est pour faire une
// différence. Montrer que c'est un lien de guide. », puis « Enleve l'icône link et enleve les puces. Juste titre et texte
// aussi ») : le titre du groupe avec une icône de documents ; chaque guide, son nom en lien (un autre onglet, EXTERNAL ;
// dans l'application Android, le navigateur du téléphone), son aide dessous (ceux de la page « GameFlex »). La croix
// atteint chaque lien (`data-nav`), A le suit, la lumière se pose sur lui (`is-focus`).
export function GuideSection({ guides }) {
    const { t } = useTranslation();

    return (
        <TextSection title={t("guide.title")} icon="bi-journal-text" help={t("guide.help")}
            items={guides.map(([key, anchor = null]) => ({
                key: `guide-${key}${anchor ? `-${anchor}` : ""}`, title: t(`project.${key}`), text: t(`project.${key}Help`), href: guideUrl(key, anchor),
            }))} />
    );
}

// UNE SECTION DE TEXTE (le porteur, 2026-11-16 : « les lignes de bon à savoir peuvent être que du texte avec titres.
// Comme pour Guide » ; « Et pour les guides. Enleve l'icône link et enleve les puces. Juste titre et texte aussi. Pas de
// différence avec bon à savoir, juste que le titre est un lien ») : le titre du groupe et son icône, puis chaque élément,
// son titre en gras (jamais en italique) et son texte dessous, au bord gauche, sans encadré ni puce. Un élément qui a `href` a son titre en
// lien (un guide, `GuideSection`) ; les autres sont du texte (« Bon à savoir »). `items` : [{ key, title, text, href }].
export function TextSection({ title, icon, help = null, items }) {
    return (
        <Section title={title} help={help} badge={<i className={`bi ${icon}`} aria-hidden="true"></i>}>
            {items.map((item, index) => (
                <div key={item.key} className={index ? "mt-3" : ""}>
                    {item.href ? (
                        <a href={item.href} {...EXTERNAL} className="fw-bold d-inline-block" data-nav data-key={item.key}>{item.title}</a>
                    ) : (
                        <span className="fw-bold d-block">{item.title}</span>
                    )}
                    <span className={`d-block ${MUTED}`}>{item.text}</span>
                </div>
            ))}
        </Section>
    );
}

// Une ligne de texte atténuée, au bord gauche : le nombre trouvé, un renseignement — et ce qui n'a pas pu se lire (une
// erreur se dit là, sans encadré : plus d'`alert` dans le launcher, le porteur, 2026-11-16 : « retirer les alerts »).
export function Note({ className = "mt-3", children }) {
    return <p className={`${MUTED} gf-note mb-0 ${className}`}>{children}</p>;
}

// UNE LIGNE FANTÔME, le temps qu'une liste se lise (Listing) : la forme d'une ligne — la pastille grise, le libellé,
// une aide plus courte, le chevron —, en `placeholder` de Bootstrap dans une ligne `placeholder-glow`, comme la Table de
// Skeleton dessine les siennes (skeleton/libs/Table.jsx) : la hauteur ne saute pas quand la liste arrive. `lead` : la
// tête de la vraie ligne — "chip" (une pastille), "cover" (une jaquette), "thumb" (une vignette d'état), "avatar", ou
// null ; `help` : la ligne d'aide ; `chevron` : false pour une ligne de renseignement (InfoRow). RIEN NE S'Y TOUCHE : ni
// bouton, ni `data-nav` — la lumière de la croix ne s'y pose pas.
export function PlaceholderRow({ lead = "chip", help = true, chevron = true }) {
    return (
        <div className="list-group-item gf-row gf-row-placeholder placeholder-glow" aria-hidden="true">
            {lead === "cover" ? <span className="gf-cover rounded placeholder"></span>
                : lead === "thumb" ? <span className="thumb ratio ratio-4x3 rounded placeholder"></span>
                    : lead === "avatar" ? <span className="gf-avatar placeholder"></span>
                        : lead ? <span className="gf-chip tone-slate placeholder"></span> : null}
            <span className="gf-row-text">
                <span className="gf-row-label"><span className="placeholder col-7"></span></span>
                {help && <small className="gf-row-help"><span className="placeholder col-4"></span></small>}
            </span>
            {chevron && <i className="bi bi-chevron-right gf-row-chevron" aria-hidden="true"></i>}
        </div>
    );
}

// UNE LISTE QU'ON CHERCHE ET QU'ON FEUILLETTE (10 par page, `pageSize`), comme ListingElement de la webapp : la
// recherche de Skeleton (Form.SearchText) au bord gauche, les filtres (des lignes, dans leur groupe), le nombre
// trouvé, les lignes dans leur groupe, puis la pagination de Skeleton (usePagination). `page` et `onPage` : la page
// tenue par l'appelant (dans l'adresse), pour la retrouver au retour d'une ligne ouverte — la lumière y revient ;
// sinon la liste la tient. `total` : une liste que le SERVEUR feuillette (les membres) — `items` est alors la page
// lue, et `total` le nombre de tous.
//
// TOUTE LISTE LUE EN ASYNCHRONE DES RÉGLAGES EN EST UNE (le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») — les émulateurs et les BIOS d'une console, les sources, les sauvegardes, les disques, les
// consoles Remote Play… Celle qu'on ne cherche pas n'a pas de recherche (`onSearch` absent), et celle qui ne se compte
// pas, pas de nombre (`found` absent) : elle n'est que ses lignes, et sa pagination dès qu'elle dépasse une page.
// `head` : des lignes en tête de chaque page, dans le même groupe, qui ne comptent pas dans la page (« Toutes ses
// sauvegardes », « Cette machine »).
//
// PENDANT QU'ELLE SE LIT (`loading`), DES LIGNES FANTÔMES (le porteur, 2026-11-14 : « il faut aussi utiliser ces
// placeholder pendant que ça charge. Un placeholder du même nombre d'éléments du ITEM PER PAGE. Skeleton.Table propose
// déjà le placeholder. ») — la règle de la Table de Skeleton (handlePendingState), écrite ICI une fois :
//   - autant que les lignes qu'elle montrait (elles se changent en fantômes : une page relue, une recherche refaite),
//     sinon autant qu'une page (la première lecture) ;
//   - de la forme de ses vraies lignes : PlaceholderRow, ou celle que la liste donne (`placeholderRow`, une fonction
//     de l'index : des jaquettes, des vignettes, des renseignements) ;
//   - la pagination reste à l'écran, inerte (`isPending`, et `inert` : la croix ne s'y pose pas, ni sur un fantôme).
//
// VIDE, ELLE GARDE SA BOÎTE (le porteur, 2026-11-14 : « Pour toutes les listes vides, il faut afficher au moins un lien
// (un placeholder) grisé avec le message ») : une ligne grisée (EmptyRow) — sous ses lignes de tête s'il y en a — qui dit
// `empty` (« Aucun jeu. ») ; quand une recherche n'a rien trouvé, la loupe et `noMatch`, sinon « Rien ne correspond à
// « … ». » (`null` : aucune ligne, la liste dit autrement pourquoi). Le nombre trouvé ne se dit pas : la ligne le dit.
//
// SES FILTRES (`filters` : leurs lignes, des contrôles — interrupteurs, sélecteurs, la période d'un journal) NE SONT
// JAMAIS SOUS LA RECHERCHE (le porteur, 2026-11-14 : « Remplace le modal filtre par un offcanvas, c'est plus simple pour
// la navigation » ; « Dans logs, rajoute le bouton filtre avec le search bar et met les calendriers dans le offcanvas du
// filtre » ; « Oui, c'est mieux de faire partout comme ça ») : comme ListingElement de la webapp, un bouton à icône
// accolé à la recherche, dans son input-group, ouvre le panneau latéral (SidePanel, l'offcanvas du launcher) qui les
// tient — un formulaire, sans traits ; plusieurs groupes de filtres (`filters` : `[{ key, title, help?, rows }]`) y ont
// chacun leur section, sous leur titre et avec leur aide (le Catalogue des Réglages, 2026-11-16). La pastille du bouton dit combien sont posés (`filterCount`) : un bouton enfoncé ne
// change pas de couleur (libs/ui.js). `filterTitle` : le titre du panneau (« Filtres ») ; `filterHelp` : son aide. Ils
// s'appliquent à mesure, et chaque page les garde dans son adresse. À la manette, la lumière entre dans le panneau sur
// son premier filtre, B le referme et la rend au bouton ; au doigt, son fond le referme. Le panneau n'est pas une page :
// un filtre posé sur la liste.
export function Listing({ items, render, loading = false, search = "", onSearch = null, placeholder = "", found = null, empty = null, noMatch = undefined, filters = null, filterCount = 0, filterTitle = null, filterHelp = null, head = null, pageSize = 10, page = null, onPage = null, total = null, placeholderRow = null, searchKey = "sc-search" }) {
    const { t } = useTranslation();
    const [ownPage, setOwnPage] = useState(1);
    const pager = useRef(null);
    const pressed = useRef(-1);
    const lastShown = useRef(0);
    const served = total !== null;
    const pages = Math.max(1, Math.ceil((served ? total : items.length) / pageSize));
    const shown = Math.min(Math.max(1, Number(page ?? ownPage) || 1), pages);
    const setPage = (next) => (onPage || setOwnPage)(next);
    const rows = served ? items : items.slice((shown - 1) * pageSize, shown * pageSize);
    const ghosts = !loading ? 0 : rows.length > 0 ? rows.length : lastShown.current > 0 ? lastShown.current : pageSize;
    // La liste suit la recherche et sa ligne (puis un trait), ou elle est le premier contenu de son groupe : pas d'écart
    // au-dessus dans les deux cas.
    const searching = Boolean(onSearch || filters);
    const first = "mt-0";
    const blank = !loading && !rows.length && !items.length;
    const searched = (search || "").trim();
    const blankText = !searched ? empty : noMatch === undefined ? t("settings.noMatch", { search: searched }) : noMatch;
    const note = loading || blank ? null : found;
    const button = useButton();
    const [filtering, setFiltering] = useState(false);
    const filterButton = useRef(null);
    const closeFilters = () => {
        setFiltering(false);
        requestAnimationFrame(() => filterButton.current?.isConnected && focusElement(filterButton.current, true));
    };
    const searchForm = {
        values: { q: search },
        setValues: (next) => {
            setPage(1);
            onSearch(next.q || "");
        },
        files: {},
        setFiles: () => {},
        rulesets: {},
    };
    const pagination = usePagination({
        isPending: loading,
        currentPosition: shown,
        itemCount: served ? total : items.length,
        itemsPerPage: pageSize,
        maxButtons: 5,
        // Skeleton redessine sa pagination à chaque page : la lumière revient au bouton qu'on vient d'appuyer.
        onPositionChange: (next) => {
            pressed.current = Array.from(pager.current?.querySelectorAll("button, select") || []).indexOf(document.activeElement);
            setPage(next);
        },
    });

    // Le panneau ouvert, la lumière va sur son premier filtre (le panneau la pose d'abord sur sa croix de fermeture).
    useEffect(() => {
        if (filtering) {
            requestAnimationFrame(() => focusElement(document.querySelector(".gf-filters .offcanvas-body [data-nav]"), true));
        }
    }, [filtering]);

    // Les lignes montrées, retenues : relue, la liste en fait autant de fantômes.
    useEffect(() => {
        if (!loading) {
            lastShown.current = rows.length;
        }
    });

    // La lumière rendue au bouton de la pagination qu'on vient d'appuyer. Une page que le serveur donne se LIT d'abord
    // (la pagination est inerte entre-temps, la lumière la quitte) : rendue quand la lecture finit.
    const wasLoading = useRef(loading);
    useEffect(() => {
        const settled = wasLoading.current && !loading;
        wasLoading.current = loading;

        if (loading || (served && !settled)) {
            return;
        }
        const controls = Array.from(pager.current?.querySelectorAll("button, select") || []);
        const target = controls[pressed.current];
        pressed.current = -1;

        if (target) {
            focusElement(target.disabled ? controls.find((control) => control.tagName === "SELECT") || target : target);
        }
    }, [shown, loading]);

    // LA RECHERCHE ET SON RÉSULTAT, PUIS UN TRAIT, PUIS LA LISTE (le porteur, 2026-11-14, chantier 53, P12 : « les search
    // bar rattaché à des listings doit être dans une section. La section aura donc le search bar (et des fois le bouton
    // filtre) et en dessous du search bar, le résultat de la recherche. Puis entre les 2 section, search bar et listing,
    // rajouter un hr pour séparer. ») — une fois, ici, pour toute liste qui se cherche ou se filtre : LA BOÎTE de la
    // recherche (`.gf-listing-search`, le conteneur arrondi des groupes : le champ, le bouton des filtres accolé, la ligne
    // « N trouvés » dessous — le porteur : « Le search bar n'a pas encore de section... »), un `hr`, puis la boîte des
    // lignes et leur pagination. Le titre du groupe (la Section de l'appelant) reste au-dessus de tout : il nomme la liste,
    // sa recherche comprise. Une liste sans recherche ne change pas : sa ligne de nombre, puis ses lignes.
    return (
        <>
            {searching && (
                <Rows className="gf-listing-search">
                    <BlockRow>
                        <div className={`gf-search ${filters && onSearch ? "input-group flex-nowrap" : ""}`}>
                            {filters && (
                                <button ref={filterButton} type="button" className={`${button("other")} position-relative flex-shrink-0`} data-nav data-key={`${searchKey}-filter`}
                                    aria-label={filterTitle || t("settings.filters")} title={filterTitle || t("settings.filters")} aria-pressed={filterCount > 0} aria-expanded={filtering}
                                    onClick={() => setFiltering(true)}>
                                    <i className="bi bi-funnel"></i>
                                    {filterCount > 0 && <span className="position-absolute top-0 start-100 translate-middle badge rounded-pill text-bg-danger">{filterCount}</span>}
                                </button>
                            )}
                            {onSearch && <Form.SearchText name="q" context={searchForm} props={{ placeholder, autoComplete: "off", "data-nav": true, "data-key": searchKey, "aria-label": placeholder }} />}
                        </div>
                        {note && <Note className="mt-2">{note}</Note>}
                    </BlockRow>
                </Rows>
            )}
            {searching && <hr className="gf-listing-rule my-3" />}
            {filters && filtering && (
                <SidePanel label={filterTitle || t("settings.filters")} className="gf-filters" onClose={closeFilters}>
                    {/* DES GROUPES DE FILTRES, chacun sa section titrée (le Catalogue : ses favoris, ses sources, ses statuts, sa
                        taille, ses consoles), comme le panneau de recherche de la bibliothèque ; sinon une seule. */}
                    {Array.isArray(filters) ? filters.map((group) => (
                        <Section key={group.key} title={group.title} help={group.help || null}>
                            <Rows>{group.rows}</Rows>
                        </Section>
                    )) : (
                        <Section help={filterHelp}>
                            <Rows>{filters}</Rows>
                        </Section>
                    )}
                </SidePanel>
            )}
            {!searching && note && <Note className={first}>{note}</Note>}
            {(rows.length > 0 || head || ghosts > 0 || (blank && blankText)) && (
                <Rows className={!searching && note ? "mt-3" : first}>
                    {head}
                    {loading
                        ? Array.from({ length: ghosts }, (_, index) => <React.Fragment key={`ghost-${index}`}>{placeholderRow ? placeholderRow(index) : <PlaceholderRow />}</React.Fragment>)
                        : rows.length ? rows.map(render) : blankText && <EmptyRow icon={searched ? "bi-search" : "bi-inbox"} text={blankText} />}
                </Rows>
            )}
            {pages > 1 && (
                <div ref={pager} className="gf-pager mt-3" inert={loading ? "" : undefined} aria-busy={loading || undefined}>
                    {/* LA MANETTE JUSQU'À LA PAGINATION, partout où une liste se feuillette (le porteur, 2026-11-16 : « Avec une
                        manette, la pagination dans recherche ne fonctionne pas ») : Skeleton dessine ses boutons sans
                        `data-nav` ; une page des Réglages les marquait (SettingsPage, usePadReach), pas un panneau latéral
                        (la recherche d'une bibliothèque) — la liste les marque donc elle-même. */}
                    <PadReach>
                        <pagination.PrevNext>
                            <pagination.Selectbox />
                        </pagination.PrevNext>
                    </PadReach>
                </div>
            )}
        </>
    );
}
