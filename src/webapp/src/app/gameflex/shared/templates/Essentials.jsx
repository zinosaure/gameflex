import React, { useState, useContext, useRef } from "react";
import { createPortal } from "react-dom";
import * as bs from "react-bootstrap";
import { useTranslation } from "react-i18next";
import Skeleton from "../../../skeleton/skeleton.js";
import { useDownloadProgress } from "../../libs/deviceStream.js";

const Form = Skeleton.Form;

// Full-viewport spinner — the screen every async page entry shows before
// its data resolves (was duplicated in index.js and the streaming pages).
export function LoadingScreen({ label = null, bgColor = "black", textOnFailure = null, onFailure = null }) {
    const { t } = useTranslation();

    return (
        <div
            className="d-flex flex-column align-items-center justify-content-center text-white gap-3"
            style={{
                position: "fixed",
                inset: 0,
                width: "100vw",
                height: "var(--gameflex-visible-height)",
                zIndex: 3005,
                background: bgColor,
            }}
        >
            {textOnFailure ? (
                <div className="text-center">
                    <p className="text-danger mb-3">{textOnFailure}</p>
                    <bs.Button
                        variant="danger"
                        onClick={onFailure}
                    >
                        {t("app.returnBack")}
                    </bs.Button>
                </div>
            ) : (
                <bs.Spinner animation="border" role="status">
                    <span className="visually-hidden">{label || t("app.loading")}</span>
                </bs.Spinner>
            )}
        </div>
    );
}

// Standard page scaffold — the py-2 container + single page label (with
// optional helpText and right-aligned helpButton) every routed page shares.
// `props` lets a page keep its own container constraints.
export function Page({ label, helpText = null, helpButton = null, props = {}, children }) {
    return (
        <bs.Container className="py-2" {...props}>
            <div className="d-flex align-items-start justify-content-between gap-3">
                <div className="flex-grow-1 mb-2">
                    <h1 className="mb-1 fw-bold">{label}</h1>
                    {helpText ? <p className="text-muted small pb-0">{helpText}</p> : null}
                </div>
                {helpButton}
            </div>
            {children}
        </bs.Container>
    );
}

// Card frame for a settings form — label row + body. Extracted from
// admin/Members.jsx's create/edit card so TurnConfig/Storage keep the
// same look. `label`, `helpText` and `helpButton` are both optional (the Storage/Turn
// tabs already carry their own section labels).
export function SectionPanel({ label = null, helpText = null, helpButton = null, collapsible = false, borderless = true, children }) {
    const [isCollapsed, setCollapsed] = useState(collapsible);
    const toggleCollapse = () => setCollapsed(!isCollapsed);

    return (
        <bs.Card className={`section-panel ${borderless ? "section-panel-borderless p-0 border-0" : "section-panel-bordered p-3 mb-3 bg-body-secondary bg-opacity-25 border"}`}>
            <bs.Card.Body className="p-0">
                {(label || helpButton) && (
                    <div
                        className={`d-flex align-items-center justify-content-between ${isCollapsed ? "mb-0 pb-0" : "mb-3 pb-2"}`}
                        onClick={collapsible ? toggleCollapse : null}
                        // Repliable, il se replie aussi à la croix et au clavier
                        // (2026-10-21) : sans focus, la croix passait par-dessus.
                        tabIndex={collapsible ? 0 : undefined}
                        role={collapsible ? "button" : undefined}
                        onKeyDown={collapsible ? (event) => event.target === event.currentTarget && Skeleton.keyActivate(toggleCollapse)(event) : undefined}
                        style={{ cursor: collapsible ? "pointer" : "default" }}
                    >
                        <div className="flex-grow-1">
                            <h4 className="fw-bold mb-2">{label}</h4>
                            {helpText && <p className="text-muted small mb-0">{helpText}</p>}
                        </div>
                        {helpButton}
                        {collapsible && (
                            <i className={`bi ${isCollapsed ? "bi-chevron-down" : "bi-chevron-up"}`}></i>
                        )}
                    </div>
                )}
                {!isCollapsed && children}
            </bs.Card.Body>
        </bs.Card>
    );
}

// Standard async section states — loading spinner, error with retry, empty
// hint, or the caller's content. Extracted from SaveStates/Games' repeated
// tri-state; the error branch is generic
// (RemotePlay-style banners will adopt it when they get a retry).
export function AsyncSectionPanel({ isLoading = false, isEmpty = false, emptyText = null, error = false, errorText = null, onRetry = null, children }) {
    const { t } = useTranslation();

    if (isLoading) {
        return (
            <div className="d-flex justify-content-center py-5">
                <bs.Spinner animation="border" role="status">
                    <span className="visually-hidden">{t("app.loading")}</span>
                </bs.Spinner>
            </div>
        );
    }

    if (error) {
        return (
            <div className="text-center py-4">
                <p className="text-danger mb-3">{errorText || t("app.error")}</p>
                {onRetry && (
                    <bs.Button variant="primary" onClick={onRetry}>
                        {t("app.retry")}
                    </bs.Button>
                )}
            </div>
        );
    }

    if (isEmpty) {
        return <p className="text-muted">{emptyText}</p>;
    }

    return children;
}

// Off-canvas panel — the right-side drawer every page can open for a
// secondary action (cheats, save/load, invite, etc.). The label row is
// sticky and shows the icon, label, and help text. The bottom row is also
// sticky and shows the main action button (if any) and a close button.
// progress — un pourcentage (0-100) qui REMPLIT le bouton d'action, ou null.
//
// Pour un travail qu'on a déclenché depuis ce panneau et qu'il faut attendre :
// récupérer un jeu de 300 Mo avant de le lancer, par exemple. Un bouton grisé
// avec « ••• » dit « occupé » sans dire combien de temps, et sur un gros
// fichier ça se lit comme un blocage (signalé le 2026-09-02).
//
// Le remplissage est DANS le bouton, pas au-dessus : il reste à sa place, et
// l'attente se lit là où l'on vient de cliquer.
export function OffCanvas({
    icon,
    label = null,
    helpText = null,
    callback = null,
    buttonLabel = null,
    isDisabled = false,
    isPending = false,
    progress = null,
    children = null }) {
    const context = useContext(Skeleton.context);

    return (
        <div className="d-flex flex-column h-100 position-relative">
            <div className="position-sticky top-0 end-0 text-end" style={{ zIndex: 100 }}>
                <bs.Button
                    variant="secondary"
                    tabIndex={0}
                    title="Close"
                    onClick={() => context.offcanvas.close()}
                    onKeyDown={Skeleton.keyActivate(() => context.offcanvas.close())}
                    className="text-decoration-none text-reset px-5 mt-3"
                    disabled={isPending}
                >
                    <i className="bi bi-chevron-right"></i>
                </bs.Button>
            </div>
            {icon && label && (
                <div className="text-center mt-3 mb-3 border-bottom pb-5" style={{ zIndex: 20 }}>
                    <i className={`${icon} d-block mb-3`} style={{ fontSize: "5rem" }}></i>
                    <h2 className="fw-bold mb-1">{label}</h2>
                    {helpText && <p className="text-muted fs-6 mb-0">{helpText}</p>}
                </div>
            )}
            {children}
            {callback && buttonLabel ? (
                <div className="mt-3 pt-3 border-top border-top position-sticky bottom-0 bg-body" style={{ zIndex: 20 }}>
                    <div className="d-flex align-items-center justify-content-between gap-3 pb-3">
                        <bs.Button
                            variant="primary"
                            className="w-100 fw-bold text-uppercase flex-grow-1 position-relative overflow-hidden"
                            disabled={isDisabled || isPending || progress !== null}
                            onClick={callback}>
                            {progress !== null && (
                                <span
                                    className="position-absolute top-0 start-0 h-100"
                                    style={{
                                        // La part reçue, en clair sur le fond
                                        // du bouton. Une transition courte
                                        // pour que la barre glisse au lieu de
                                        // sauter d'un pourcent à l'autre.
                                        width: `${Math.max(0, Math.min(100, progress))}%`,
                                        background: "rgba(255, 255, 255, 0.25)",
                                        transition: "width 200ms linear",
                                    }}
                                />
                            )}
                            <span className="position-relative">
                                {progress !== null
                                    ? buttonLabel
                                    : (isPending || isDisabled ? "•••" : buttonLabel)}
                            </span>
                        </bs.Button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

// Form button row — the bottom row of a settings form, with a single
// right-aligned button. The button is disabled when the form is invalid or
// pending, and shows a spinner when pending.
export function FormButton({ callback, label, isDisabled = false, isPending = false, variant = "primary" }) {
    return (
        <div className="mt-4 pt-3 border-top text-end">
            <bs.Button
                variant={isDisabled || isPending ? "danger" : variant}
                className="w-50 fw-bold text-uppercase"
                type="button"
                onClick={callback}
                disabled={isDisabled || isPending}>
                {isPending || isDisabled ? "•••" : label}
            </bs.Button>
        </div>
    )
}

// Card row of a list — icon/tile + bold label + muted helpText + optional
// end content (badge/actions) + trailing chevron. Extracted from Games.jsx's
// GameCard; RemotePlay's device rows and the invite list share the same
// skeleton. Callers keep their own card/body
// classes (game-card, device-card…) via cardClassName/bodyClassName.

// The row itself is always its own inner flex div (not Card.Body directly) —
// it's what lets `children` (e.g. SaveStatesPanel's expand-in-place save/load
// buttons, shown only for the selected slot) stack BELOW the row instead of
// getting flexed inline with icon/label/end/chevron. w-100 on the row is
// load-bearing, not decorative: Card.Body's own flex row (the default
// bodyClassName) does NOT stretch a lone flex child to fill the available
// width (that's cross-axis stretch, which only applies to height in a row
// container) — without it the row shrinks to its content's natural size and
// label's flex-grow-1 has no slack left to grow into, so the chevron ends up
// glued right next to the label instead of pushed to the far right (this
// broke exactly that way before w-100 was added, confirmed in Games.jsx's
// list). cardStyle covers the one thing cardClassName can't (a selected-row
// background/border that isn't a static Bootstrap class, see SaveStatesPanel).
export function ListElement({
    onClick = null,
    icon = null,
    label,
    helpText = null,
    next = null,
    light = false,
    children = null
}) {
    const [isHovered, setIsHovered] = useState(false);
    const [isSelected, setIsSelected] = useState(false);
    const activate = () => {
        if (onClick) {
            onClick();
        }
        setIsSelected(!isSelected);
    };

    return (
        <>
            {/* UNE LIGNE QU'ON TOUCHE SE RÉVÈLE AU CLAVIER ET À LA MANETTE
                (#407) : sans tabIndex ni rôle, la croix ne s'y arrêtait pas,
                et Entrée ou A ne la déclenchaient pas — toutes les actions du
                menu d'une partie (prendre la manette, sauvegardes, quitter…)
                étaient hors d'atteinte. */}
            <bs.Card
                className={`list-element border-1 h-100 ${light ? "bg-secondary text-dark border-secondary" : "bg-body-secondary"} ${isHovered ? "bg-opacity-50 border-primary" : ""}`}
                onMouseEnter={() => setIsHovered(true)}
                onMouseLeave={() => setIsHovered(false)}
                style={{ cursor: onClick ? "pointer" : "default" }}
                tabIndex={onClick ? 0 : undefined}
                role={onClick ? "button" : undefined}
                // La ligne elle-même seulement : un bouton QU'ELLE CONTIENT
                // (les sauvegardes s'ouvrent sur place) garde son Entrée.
                onKeyDown={onClick ? (event) => event.target === event.currentTarget && Skeleton.keyActivate(activate)(event) : undefined}
                onClick={activate}
            >
                <bs.Card.Body className="p-2">
                    <div className="d-flex align-items-center gap-3 w-100">
                        {icon && <i className={`${icon} flex-shrink-0`} style={{
                            width: 45,
                            height: 45,
                            borderRadius: "5px",
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            fontSize: "1.5rem",
                            flexShrink: 0,
                            backgroundColor: light ? "#495057" : "#a1a1a1",
                            color: light ? "white" : "black",
                            border: light ? "1px solid #a1a1a1" : "1px solid #495057",
                        }}></i>}
                        <div className="flex-grow-1" style={{ minWidth: 0 }}>
                            <div className="fw-semibold text-body mb-0">{label}</div>
                            {helpText ? <div className="text-muted small">{helpText}</div> : null}
                        </div>
                        {onClick && !next ? (<i className="bi bi-chevron-right text-muted fs-4 flex-shrink-0"></i>) : next ? next : null}
                    </div>
                    {children}
                </bs.Card.Body>
            </bs.Card>
            {light && <hr className="mb-4" />}
        </>
    );
}

// LE CONTEXTE DE SKELETON À JOUR, SANS QU'UNE LECTURE EN DÉPENDE.
//
// Skeleton fournit `value={{ ...app }}` : un objet NEUF à chaque rendu de son
// fournisseur — ouvrir une fenêtre de confirmation, afficher un toast suffit.
// Une liste dont la lecture gardait `context` dans ses dépendances, pour son
// seul toast d'erreur, se relisait donc à chaque geste : la confirmation d'une
// suppression la rechargeait en s'ouvrant, puis en se fermant (relevé par le
// porteur le 2026-10-10 sur Émulateurs et Firmware ; Journaux et Sauvegardes
// avaient le même défaut). La référence donne le contexte du moment où l'on
// s'en sert, sans rien relancer.
export function useLatestContext() {
    const context = useContext(Skeleton.context);
    const latest = useRef(context);
    latest.current = context;

    return latest;
}

export function ListingElement({
    callback,
    items,
    itemCount = 0,
    itemsPerPage = 15,
    isLoading = false,
    currentPage = 1,
    onPositionChange = () => { },
    search = {
        context: null,
        placeholder: "Search...",
        filters: {
            icon: "bi bi-funnel-fill",
            Component: null,
        },
    },
    texts = {
        itemsFound: "item(s) found",
        noItemsFound: "No items found",
    }
}) {
    const context = useContext(Skeleton.context);
    const searchParams = new URLSearchParams(window.location.search);
    const pagination = Skeleton.usePagination({
        isPending: isLoading,
        currentPosition: currentPage,
        itemCount: itemCount || items.length,
        itemsPerPage: itemsPerPage,
        onPositionChange: onPositionChange,
        maxButtons: 5,
    });
    const FilterComponent = search?.filters?.Component || null;

    // LE PANNEAU DE FILTRES VIT DANS CET ARBRE-CI, pas dans celui du panneau.
    //
    // `offcanvas.open` range ce qu'on lui donne dans un état et le rend par
    // portail depuis le fournisseur Skeleton : c'est un INSTANTANÉ, qui ne
    // re-rend jamais avec la page. Les cases du filtre ne bougeaient donc
    // plus, alors que les valeurs changeaient bien derrière — le `setValues`
    // capturé est stable, le `values` relu ne l'est pas (signalé le
    // 2026-09-02).
    //
    // On n'ouvre donc qu'un panneau VIDE, et on porte notre propre sous-arbre
    // dans son corps. Il redevient un enfant ordinaire de cette page, et voit
    // les valeurs vivantes comme n'importe quel autre.
    const [filterNode, setFilterNode] = useState(null);

    const openFilters = () => {
        const node = context.offcanvas.open(<div />, {
            size: "450px",
            onClose: () => setFilterNode(null),
        });
        setFilterNode(node || null);
    };

    return (
        <>
            {(search?.context) && (
                <div className="position-relative mb-3">
                    {search?.filters?.Component && (
                        <bs.Button
                            variant="secondary"
                            style={{
                                position: "absolute",
                                top: 0,
                                left: 0,
                                height: "100%",
                                borderTopRightRadius: 0,
                                borderBottomRightRadius: 0,
                                padding: "0.5rem 1rem",
                                zIndex: 10
                            }}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                openFilters();
                            }}
                        >
                            <i className={search?.filters?.icon || "bi bi-funnel-fill"}></i>
                        </bs.Button>
                    )}
                    {filterNode && FilterComponent && createPortal(
                        <OffCanvas
                            icon={search?.filters?.icon || "bi bi-funnel-fill"}
                            label={search?.filters?.label || null}
                            helpText={search?.filters?.helpText || null}
                        >
                            <FilterComponent />
                        </OffCanvas>,
                        filterNode,
                    )}
                    <Form.SearchText
                        name="q"
                        context={search.context}
                        isDisabled={isLoading || !search.context}
                        props={{
                            placeholder: searchParams.get("q") || search?.placeholder || null,
                            style: { zIndex: 1, padding: search?.filters?.Component ? "0.5rem 1rem 0.5rem 4rem" : "0.5rem 1rem", },
                        }}
                    />

                </div>
            )}
            <p className="text-muted mb-3">
                <b>{itemCount}</b> {texts?.itemsFound || null}
            </p>

            <AsyncSectionPanel
                isLoading={isLoading}
                isEmpty={itemCount === 0}
                emptyText={texts?.noItemsFound || null}
            >
                {items.map(callback)}
                <div className="mt-4">
                    <pagination.PrevNext>
                        <pagination.Selectbox />
                    </pagination.PrevNext>
                </div>
            </AsyncSectionPanel>
        </>
    )
}

// The download progress surface, fed by the native bridge's onProgress
// channel (libs/deviceStream.js). It lives here because two very different
// places need the same truth:
//
//   - the streaming page's connecting overlay, while a remote game is
//     fetched as part of STARTING it (the wait for the video IS the
//     download, and a silent spinner would look like a crash);
//   - the app shell, so an import keeps its progress visible after the
//     game panel that started it has been closed — the download itself
//     already runs in a native thread, this is only about visibility.
//
// The store only ever holds what is still MOVING (percent < 100), so both
// surfaces disappear by themselves the moment a download completes.

function formatProgressBytes(bytes) {
    if (!bytes) {
        return null;
    }

    const units = ["B", "KB", "MB", "GB"];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }

    return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function ProgressContent({ t, progress, className = "" }) {
    if (!progress) {
        return null;
    }

    const done = formatProgressBytes(progress.done);
    const total = formatProgressBytes(progress.total);
    // POIDS INCONNU : l'hôte envoie -1 plutôt que d'inventer un total. Il
    // fallait le lire — la barre affichait « -1% », ce qui est pire que rien
    // (signalé le 2026-09-08). Sans total on montre une barre en mouvement et
    // la quantité reçue : « ça travaille », au lieu d'un chiffre faux.
    const known = progress.percent >= 0 && progress.total > 0;

    return (
        <div className={`text-start w-100 ${className}`} style={{ maxWidth: 420 }}>
            <div className="d-flex align-items-center justify-content-between small mb-1">
                <span className="text-truncate me-2">
                    <i className="bi bi-download me-1"></i>
                    {progress.label || t("app.download")}
                </span>
                <span className="text-nowrap">
                    {known && done && total ? `${done} / ${total}` : done}
                </span>
            </div>
            <bs.ProgressBar
                now={known ? progress.percent : 100}
                label={known ? `${progress.percent}%` : ""}
                animated
                striped
            />
        </div>
    );
}

// Inline block — drop it inside an overlay or a panel.
export function ProgressBar() {
    const { t } = useTranslation();
    const progress = useDownloadProgress();

    return <ProgressContent t={t} progress={progress} />;
}

/**
 * UN TÉLÉCHARGEMENT QUI SE VOIT.
 *
 * Un bouton qui disparaît ou se grise pendant qu'on attend se lit comme une
 * panne : le transfert d'un élément distant peut mettre plusieurs secondes à
 * seulement DÉMARRER, et pendant ce temps rien ne disait ce qui se passait
 * (signalé le 2026-09-02).
 *
 * L'anneau prend la place exacte du bouton, pour que la ligne ne bouge pas —
 * une liste qui se réorganise pendant qu'on la regarde est pire que
 * l'attente elle-même.
 *
 * Deux états, et la distinction compte : avec un `percent`, l'anneau se
 * remplit ; sans — le serveur qui zippe à la volée n'annonce pas de taille —
 * il TOURNE. Une barre qui resterait à zéro pendant une minute mentirait,
 * alors qu'une rotation ne promet rien d'autre que « ça travaille ».
 */
export function DownloadRing({ percent = null, size = 38, label = "" }) {
    const stroke = 3;
    const radius = (size - stroke) / 2;
    const circumference = 2 * Math.PI * radius;
    const known = typeof percent === "number" && percent >= 0;

    return (
        <div
            className="d-inline-flex align-items-center justify-content-center position-relative"
            style={{ width: size, height: size }}
            role="progressbar"
            aria-label={label}
            aria-valuenow={known ? percent : undefined}
            aria-valuemin={0}
            aria-valuemax={100}
        >
            <svg
                width={size}
                height={size}
                className={known ? undefined : "gameflex-ring-spin"}
                style={{ transform: known ? "rotate(-90deg)" : undefined }}
            >
                <circle
                    cx={size / 2}
                    cy={size / 2}
                    r={radius}
                    fill="none"
                    stroke="currentColor"
                    strokeOpacity={0.2}
                    strokeWidth={stroke}
                />
                <circle
                    cx={size / 2}
                    cy={size / 2}
                    r={radius}
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={stroke}
                    strokeLinecap="round"
                    strokeDasharray={circumference}
                    // Sans pourcentage : un arc court qui tourne. Avec : la
                    // part réellement reçue.
                    strokeDashoffset={known
                        ? circumference * (1 - percent / 100)
                        : circumference * 0.75}
                />
            </svg>
            {known && (
                <span
                    className="position-absolute fw-semibold"
                    style={{ fontSize: size <= 40 ? "0.6rem" : "0.75rem" }}
                >
                    {percent}
                </span>
            )}
        </div>
    );
}