import React, { useState } from "react";
import SettingsPage, { useSearchParam } from "../SettingsPage.jsx";
import { Form, formContext, FormButton, helpOrReason, Ruleset } from "../../ui/Forms.jsx";
import { AccountName } from "../../bricks/AccountName.jsx";
import { AdminMark } from "../../bricks/AdminMark.jsx";
import { Avatar } from "../../bricks/Avatar.jsx";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { IconAddon } from "../../bricks/IconAddon.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { QuickTiles } from "../../bricks/QuickTiles.jsx";
import { RecentNotices, RequestRow } from "../../bricks/RecentNotices.jsx";
import { DownloadRow } from "../../bricks/DownloadRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";

// LES MAQUETTES DU CHANTIER 81 (le porteur, 2026-11-25 : « Si tu peux faire des screenshots avant de lancer les vraies
// modifications ce serait mieux ») : les règles du §0 de docs/roadmap/chantier-81-refonte-des-pages.md dessinées avec
// les briques d'aujourd'hui, sans toucher aux vraies pages. Un outil de la dev, comme le reste des Modèles, écrit en
// français ; rien ici n'agit (aucun appel au serveur). `?m=` choisit la maquette.
//
// LA COULEUR D'UNE SECTION EST SON SENS (§4) : les pastilles de ses lignes la prennent, une ligne ne choisit plus la
// sienne. Le prototype de globals/sections.js.
const SENSES = {
    danger: "rose", // détruire, retirer
    modify: "amber", // modifier ce qui existe
    add: "indigo", // ajouter, créer (primary)
    info: "sky", // informer
    optional: "slate", // facultatif (secondary)
    play: "green", // jouer, ouvrir, télécharger (proposé)
};

const noop = () => {};

// Une section : son titre (en minuscules, sans icône, la direction C), la couleur de son sens sur ses lignes.
function Sense({ sense, title, help = null, admin = false, children }) {
    return <Section title={title} help={help} badge={admin ? <AdminMark label="Administrateur" /> : null}><Rows>{React.Children.map(children, (row) => row && React.cloneElement(row, { tone: SENSES[sense] }))}</Rows></Section>;
}

// UN ÉLÉMENT DE LISTING : la vraie ligne (NavRow) — ses états (`checked`, `disabled`, `obsolete`) et son action au bord
// droit (`opens` : page, confirm, action — rien —, delete).
const OPENS = { page: "page", confirm: "confirm", none: "action", delete: "delete" };
function Item({ id, icon, label, help = null, checked = false, disabled = false, obsolete = false, action = "page" }) {
    return <NavRow id={id} icon={icon} label={label} help={help} checked={checked} disabled={disabled} obsolete={obsolete} opens={OPENS[action]} onOpen={noop} />;
}

// UN LISTING : toujours en premier, toujours sa recherche, un filtre quand il y a lieu.
function MockListing({ items, placeholder, filters = true }) {
    const [search, setSearch] = useState("");
    const shown = items.filter((item) => item.label.toLowerCase().includes(search.toLowerCase()));

    return (
        <Listing items={shown} render={(item) => <Item key={item.id} {...item} />} search={search} onSearch={setSearch} placeholder={placeholder}
            filters={filters ? [{ key: "state", title: "État", rows: [] }] : null} filterCount={0} pageSize={10} />
    );
}

// DEUX BOUTONS CÔTE À CÔTE (25/75, 50/50, 75/25), la couleur de chacun dit sa gravité.
function Buttons({ split, left, right }) {
    const cols = { "25/75": ["col-3", "col-9"], "50/50": ["col-6", "col-6"], "75/25": ["col-9", "col-3"] }[split];

    return (
        <div className="gf-form-button border-top pt-3">
            <div className="row g-2">
                <div className={`${cols[0]} d-grid`}><button type="button" className={`btn btn-${left[1]}`}>{left[0]}</button></div>
                <div className={`${cols[1]} d-grid`}><button type="button" className={`btn btn-${right[1]}`}>{right[0]}</button></div>
            </div>
        </div>
    );
}

function Footer() {
    return <p className="text-center small text-body-secondary mt-5 mb-4">GameFlex (0.17.0) · GitHub · Buy me a coffee</p>;
}

function End({ guides }) {
    return <><GuideSection guides={guides} /><Footer /></>;
}

// LES RÈGLES, EN UNE PAGE : les sens, les états et les actions d'un élément, les boutons.
function Rules() {
    return (
        <>
            <Section title="Les éléments" help="Un listing, toujours en tête : sa recherche, son filtre, ses éléments.">
                <MockListing placeholder="Rechercher un élément…" items={[
                    { id: "r-1", icon: "bi-cpu", label: "Un élément choisi", help: "Choisi • une validation verte", checked: true },
                    { id: "r-2", icon: "bi-cpu", label: "Un élément ordinaire", help: "Une route • le chevron" },
                    { id: "r-3", icon: "bi-cpu", label: "Une confirmation", help: "Une question • le point d'interrogation", action: "confirm" },
                    { id: "r-4", icon: "bi-cpu", label: "Sans action", help: "Rien à faire • pas d'icône", action: "none" },
                    { id: "r-5", icon: "bi-cpu", label: "À supprimer", help: "Supprimer • la poubelle rouge", action: "delete" },
                    { id: "r-6", icon: "bi-cpu", label: "Un élément désactivé", help: "Désactivé • sa raison", disabled: true },
                    { id: "r-7", icon: "bi-cpu", label: "Un élément obsolète", help: "Obsolète • grisé", obsolete: true },
                ]} />
            </Section>
            <Section title="Une liste vide" help="Sa boîte reste : une ligne grisée dit pourquoi (EmptyRow).">
                <Listing items={[]} render={() => null} search="" onSearch={noop} placeholder="Rechercher un jeu…" empty="Aucun jeu." />
            </Section>
            <Section title="Une liste qui charge" help="Des lignes fantômes, autant qu'une page en tient : la hauteur ne saute pas quand la liste arrive.">
                <Listing items={[]} render={() => null} loading search="" onSearch={noop} placeholder="Rechercher un jeu…" pageSize={4} />
            </Section>
            {Object.entries({ info: "Informer", modify: "Modifier", add: "Ajouter", play: "Jouer, ouvrir, télécharger", optional: "Facultatif", danger: "Danger" }).map(([sense, title]) => (
                <Sense key={sense} sense={sense} title={title} help={`Une section « ${title.toLowerCase()} » : la couleur de ses pastilles dit ce qu'elle fait.`}>
                    <NavRow id={`r-s-${sense}`} icon="bi-circle" label={`Une ligne « ${title.toLowerCase()} »`} help={`${sense} • ${SENSES[sense]}`} onOpen={noop} />
                </Sense>
            ))}
            <Section title="Les dernières notifications" help="Les dix dernières à l'accueil, les cinq dernières dans le menu d'une partie ; une notification finie part au bout d'une minute et demie.">
                <RecentNotices limit={10} notices={[
                    { key: "a", changedAt: 4, endedAt: null, row: <RequestRow id="r-n-request" label="Gixi vous invite à regarder sa partie" help="Jeu : Astro Bot" acceptLabel="Regarder" onIgnore={noop} onAccept={noop} /> },
                    { key: "b", changedAt: 3, endedAt: null, row: <DownloadRow entry={{ id: "mock", downloadId: "mock", state: "running", phase: "download", received: 3 * 2 ** 30, total: 8 * 2 ** 30 }} id="r-n-download" label="Final Fantasy VII Rebirth" /> },
                    { key: "c", changedAt: 2, endedAt: Date.now(), row: <InfoRow icon="bi-download" label="Hogwarts Legacy" value="Terminé" pill="success" /> },
                    { key: "d", changedAt: 1, endedAt: Date.now(), row: <InfoRow icon="bi-controller" label="Demande de manette" value={["Claude", "refusée"]} pill="secondary" /> },
                ]} />
            </Section>
            <Section title="Deux boutons, côte à côte" help="Jamais l'un sur l'autre : 25/75, 50/50 ou 75/25, la couleur dit la gravité.">
                <Rows>
                    <div className="list-group-item gf-row d-block">
                        <div className="gf-form-button border-top pt-3 d-grid"><button type="button" className="btn btn-primary">Enregistrer — un formulaire : un seul bouton, toute la ligne</button></div>
                        <Buttons split="50/50" left={["Modifier", "warning"]} right={["Supprimer", "danger"]} />
                        <Buttons split="75/25" left={["Ajouter une console", "primary"]} right={["Aide", "info"]} />
                    </div>
                </Rows>
            </Section>
            <End guides={[["library", "1-add-a-console"]]} />
        </>
    );
}

// UNE CONSOLE : ses émulateurs en listing d'abord (celui qui sert est choisi, les anciens sont obsolètes, le paquet du
// téléphone ne s'applique pas ici), puis les sections par leur sens.
function Console() {
    return (
        <>
            <Section title="Émulateurs" help="Celui qui sert est choisi ; les anciennes versions se suppriment.">
                <MockListing placeholder="Rechercher un émulateur…" items={[
                    { id: "c-1", icon: "bi-cpu", label: "KytyPS5", help: "2026-11-22 • 52 Mo", checked: true },
                    { id: "c-2", icon: "bi-cpu", label: "KytyPS5", help: "2026-11-04 • 51 Mo", obsolete: true, action: "delete" },
                    { id: "c-3", icon: "bi-cpu", label: "KytyPS5", help: "2026-10-29 • 50 Mo", obsolete: true, action: "delete" },
                    { id: "c-4", icon: "bi-phone", label: "KytyPS5 du téléphone", help: "aarch64 • r2", disabled: true },
                ]} />
            </Section>
            <Sense sense="info" title="Détails" help="La console, et ce qu'elle a de jeux.">
                <InfoRow icon="bi-joystick" label="Console" value="PlayStation 5" />
                <InfoRow icon="bi-collection" label="Jeux" value="13 jeux • 1,2 To" />
            </Sense>
            <Sense sense="play" title="Jeux" help="Les jeux de cette console, par source.">
                <NavRow id="c-games" icon="bi-collection-play" label="Ses jeux" help="13 jeux • 3 sources" onOpen={noop} />
            </Sense>
            <Sense sense="modify" title="Réglages" help="Ce qui change le comportement de la console.">
                <NavRow id="c-avail" icon="bi-toggle-on" label="Disponibilité" help="Activée" onOpen={noop} />
                <NavRow id="c-params" icon="bi-sliders2" label="Paramètres de KytyPS5" help="1080p • Auto" onOpen={noop} />
                <NavRow id="c-look" icon="bi-image" label="Images et statut" help="Sony • Jouable" onOpen={noop} />
            </Sense>
            <Sense sense="add" title="Ajouter" help="Un émulateur de plus pour cette console.">
                <NavRow id="c-add" icon="bi-download" label="Télécharger un émulateur" help="Le paquet officiel ou un ami" onOpen={noop} />
            </Sense>
            <Sense sense="optional" title="Plus" help="Ce qui sert rarement.">
                <NavRow id="c-orphans" icon="bi-file-earmark-x" label="Métadonnées sans jeu" help="4 fiches • 12 Mo" onOpen={noop} />
            </Sense>
            <End guides={[["library", "1-add-a-console"]]} />
        </>
    );
}

// LA FICHE GÉNÉRIQUE D'UN JEU (§6), sans listing : Détails d'abord.
function Game() {
    return (
        <>
            <Sense sense="info" title="Détails" help="Ce que GameFlex sait de ce jeu ; « - » quand il ne le sait pas.">
                {[["bi-joystick", "Console", "PlayStation 5"], ["bi-calendar", "Date de sortie", "6 sept. 2024"], ["bi-code-slash", "Développeur", "Team Asobi"],
                    ["bi-people", "Joueurs", "1"], ["bi-globe", "Région", "-"], ["bi-upc", "Identifiant du jeu", "PPSA21567"], ["bi-hdd", "Taille", "52,3 Go"]]
                    .map(([icon, label, value]) => <InfoRow key={label} icon={icon} label={label} value={value} />)}
            </Sense>
            <Sense sense="play" title="Jouer maintenant ou plus tard" help="Le lancer ici, ou le garder pour plus tard.">
                <NavRow id="g-play" icon="bi-play" label="Jouer à ce jeu" onOpen={noop} />
                <NavRow id="g-dl" icon="bi-download" label="Télécharger le jeu" help="Déjà ici" disabled onOpen={noop} />
            </Sense>
            <Sense sense="modify" title="Correctifs, apparence et statut" help="Ce qui change le jeu ou sa fiche.">
                <NavRow id="g-patch" icon="bi-bandaid" label="Correctif" help="PPSA21567 • 12 réglages" onOpen={noop} />
                <NavRow id="g-infos" icon="bi-pencil" label="Corriger les informations" onOpen={noop} />
            </Sense>
            <Sense sense="add" title="Ajouter" help="Le garder parmi vos jeux.">
                <NavRow id="g-fav" icon="bi-heart" label="Ajouter à mes favoris" onOpen={noop} />
            </Sense>
            <Sense sense="optional" title="Plus" help="Ce qui sert rarement.">
                <NavRow id="g-saves" icon="bi-shield-check" label="Ses sauvegardes" help="2 emplacements • 1 backup" onOpen={noop} />
                <NavRow id="g-wrong" icon="bi-search" label="Ce n'est pas le bon jeu" onOpen={noop} />
            </Sense>
            <Sense sense="danger" title="Danger" help="Ce qui ne se défait pas.">
                <NavRow id="g-forget" icon="bi-eraser" label="Oublier les métadonnées" opens="confirm" danger onOpen={noop} />
                <NavRow id="g-remove" icon="bi-trash" label="Retirer le jeu" opens="confirm" danger onOpen={noop} />
            </Sense>
            <End guides={[["library", "1-add-a-console"]]} />
        </>
    );
}

// MON COMPTE (Profil), d'après la mise en forme du porteur : Informations personnelles, Jeux, Sessions et sécurité.
function Account() {
    return (
        <>
            <Sense sense="info" title="Informations personnelles" help="Ce que les autres voient de vous.">
                <NavRow id="a-profile" icon="bi-person" label={<AccountName name="Claude" admin />} help="@claude" onOpen={noop} />
            </Sense>
            <Sense sense="play" title="Jeux" help="Vos parties et vos jeux préférés.">
                <NavRow id="a-saves" icon="bi-shield-check" label="Mes sauvegardes" help="21 jeux • 3 sources" onOpen={noop} />
                <NavRow id="a-favorites" icon="bi-star" label="Mes favoris" help="8 jeux • 4 consoles" onOpen={noop} />
            </Sense>
            <Sense sense="modify" title="Sessions et sécurité" help="Ce qui protège votre compte.">
                <NavRow id="a-federation" icon="bi-arrow-left-right" label="Fédération" help="2 serveurs • 1 jeton" onOpen={noop} />
                <NavRow id="a-password" icon="bi-lock" label="Mot de passe" onOpen={noop} />
                <NavRow id="a-mfa" icon="bi-shield-lock" label="Double authentification" help="Désactivée" onOpen={noop} />
            </Sense>
            <End guides={[["library", "browse-the-catalog-and-keep-favorites"]]} />
        </>
    );
}

// L'ACCUEIL DE L'APPLICATION, LE MÊME AU TÉLÉPHONE ET AU PC (§7) : l'écran d'edge du PC (screens/desktop/routes.jsx)
// devient celui des deux ; une capacité dit ce qui diffère (le disque externe du téléphone, le port du PC).
function Home() {
    return (
        <>
            <div className="rounded-4 d-flex flex-column align-items-center justify-content-center mt-3 text-center"
                style={{ height: 180, background: "radial-gradient(circle at 50% 40%, rgba(47,134,207,.35), transparent 60%), #10141c" }}>
                <i className="bi bi-controller text-primary" style={{ fontSize: 48 }}></i>
                <div className="fs-4 fw-semibold">GameFlex</div>
                <div className="small text-body-secondary">github.com/zinosaure/gameflex</div>
            </div>
            <QuickTiles items={[
                { id: "h-open", icon: "bi-play-fill", label: "Ouvrir", value: "La bibliothèque, les parties", tone: "green", primary: true },
                { id: "h-stop", icon: "bi-stop-circle", label: "Arrêter le serveur", value: "En marche • port 10200", tone: "rose" },
                { id: "h-restart", icon: "bi-arrow-repeat", label: "Redémarrer", value: "Arrêter, puis relancer", tone: "amber" },
                { id: "h-invite", icon: "bi-qr-code", label: "Inviter à rejoindre", value: "Le QR code et le lien", tone: "indigo" },
                { id: "h-join", icon: "bi-box-arrow-in-right", label: "Rejoindre un serveur", value: "Un autre GameFlex", tone: "indigo" },
            ]} />
            <Sense sense="modify" title="Serveur" help="Ce qui change la façon dont ce serveur répond.">
                <NavRow id="h-port" icon="bi-ethernet" label="Port" help="10200" onOpen={noop} />
                <NavRow id="h-root" icon="bi-key" label="Mot de passe root" help="@root • 1 administrateur" onOpen={noop} />
            </Sense>
            <Sense sense="info" title="Stockage" help="Où GameFlex range les jeux.">
                <NavRow id="h-place" icon="bi-folder2" label="Emplacement" help="/storage/GameFlex • 963 Mo libres" onOpen={noop} />
                <NavRow id="h-disk" icon="bi-sd-card" label="Disque externe" help="Aucun disque" onOpen={noop} />
            </Sense>
            <Sense sense="optional" title="À propos" help="Cet appareil et sa version.">
                <NavRow id="h-device" icon="bi-phone" label="Cet appareil" help="Galaxy S25 • Android 15" opens="info" onOpen={noop} />
                <NavRow id="h-update" icon="bi-cloud-arrow-down" label="Mise à jour" help="0.17.0 • à jour" onOpen={noop} />
            </Sense>
            <End guides={[["android", "install-the-app"]]} />
        </>
    );
}

// LA LISTE DES RÉGLAGES, SANS TUILES (le porteur, 2026-11-25 : « On remplacera les tuiles par des sections dans
// settings ») : Manettes, Application, Jeux et Inviter rejoignent des sections ; l'insigne d'administration reste le seul
// signe à côté d'un titre.
function SettingsHome() {
    const row = (id, icon, label, help) => <NavRow key={id} id={id} icon={icon} label={label} help={help} onOpen={noop} />;

    return (
        <>
            <Section title="Mon compte" help="Votre profil, vos sauvegardes, votre sécurité.">
                <Rows><NavRow id="s-me" lead={<Avatar name="Claude" />} label={<AccountName name="Claude" admin />} help="@claude" onOpen={noop} /></Rows>
            </Section>
            <Sense sense="modify" title="Réglages" help="Ce qui ne vaut que pour cet appareil.">
                {row("s-pads", "bi-controller", "Manettes", "Touches • manette à l'écran")}
                {row("s-app", "bi-sliders", "Application", "Langue • sons")}
            </Sense>
            <Sense sense="play" title="Ce serveur" help="Les jeux, et y inviter quelqu'un.">
                {row("s-games", "bi-collection-play", "Jeux", "Ici • serveurs fédérés")}
                {row("s-invite", "bi-qr-code", "Inviter", "Le QR code • le lien")}
            </Sense>
            <Sense sense="modify" title="Contenu" admin help="Les consoles, leurs émulateurs et leurs images.">
                {row("s-consoles", "bi-joystick", "Consoles", "42 consoles • 1 200 jeux")}
                {row("s-rp", "bi-playstation", "Remote Play", "2 PlayStation")}
                {row("s-meta", "bi-image", "Fournisseur de métadonnées", "3 services")}
                {row("s-cast", "bi-cast", "Google Cast", "Activé")}
            </Sense>
            <Sense sense="modify" title="Partage et stockage" admin help="Ce qui se partage, et où sont les jeux.">
                {row("s-fed", "bi-arrow-left-right", "Fédération", "2 serveurs • 3 jetons")}
                {row("s-res", "bi-hdd-network", "Ressources internes/externes", "2 disques • 2 partages")}
            </Sense>
            <Sense sense="modify" title="Personnes et serveur" admin help="Les comptes, et la marche du serveur.">
                {row("s-members", "bi-people", "Membres", "5 comptes • 1 administrateur")}
                {row("s-config", "bi-gear-wide-connected", "Configuration du serveur", "Gameflex • 4 joueurs")}
                {row("s-logs", "bi-journal-text", "Journaux", "12 fichiers • 40 Mo")}
                {row("s-cache", "bi-archive", "Cache", "3 Go")}
                {row("s-rescan", "bi-arrow-repeat", "Tout rescanner", "Il y a 2 jours")}
            </Sense>
            <Sense sense="optional" title="À propos" help="Le projet, ses auteurs, l'application.">
                {row("s-gameflex", "bi-github", "GameFlex", "Le code • les guides")}
                {row("s-credits", "bi-c-circle", "Crédits", "Émulateurs • licences")}
                {row("s-android", "bi-android2", "Application Android", "0.17.0")}
            </Sense>
            <Sense sense="danger" title="Session" help="Quitter GameFlex sur cet appareil.">
                <NavRow id="s-logout" icon="bi-box-arrow-right" label="Se déconnecter" onOpen={noop} />
            </Sense>
            <End guides={[["library", "1-add-a-console"]]} />
        </>
    );
}

// LES FORMULAIRES (le porteur, 2026-11-25 : « Et fait les maquettes pour les formulaires aussi ? » ; « on ne fait pas
// de hide/show mais que du enabled/disabled, en utilisant skeleton.Ruleset tout le temps ») : les champs de Skeleton
// (Form.Text, Form.Select, Form.Longtext, Form.SkeletonSwitch), et TOUT ce qui ne s'applique pas est désactivé par un Ruleset — avec
// sa raison —, jamais caché. UN SEUL BOUTON, sur toute la ligne (FormButton :
// « ••• » tant que rien n'a changé) ; les boutons côte à côte restent aux gestes qui ne valident pas un formulaire.

// La raison d'un champ désactivé, à la place de son aide : helpOrReason (ui/Forms.jsx).
const reason = helpOrReason;

// Un interrupteur de Skeleton, dans sa ligne, à droite comme ceux d'aujourd'hui (`reverse`).
// Sa paire de textes est la sienne (`texts` : Oui/Non par défaut, Activé/Désactivé, ou une autre paire).
function SwitchLine({ name, context, label, help, texts = ["Oui", "Non"] }) {
    return <div className="list-group-item gf-row d-block"><Form.SkeletonSwitch name={name} context={context} label={label} helpText={reason(context, name, help)} reverse variant="md" onText={texts[0]} offText={texts[1]} /></div>;
}

// Le formulaire de son profil (/settings/profile/account) : l'identifiant ne change pas (désactivé, pas caché), le nom
// affiché se vérifie à mesure.
function ProfileForm() {
    const saved = { username: "claude", display_name: "Claude" };
    const [values, setValues] = useState(saved);
    const form = formContext(values, setValues, {
        username: () => Ruleset.isDisabled("Il ne change plus une fois le compte créé."),
        display_name: (data) => (String(data.display_name || "").trim().length < 3 ? Ruleset.isInvalid("Trois caractères au moins.") : null),
    });
    const changed = values.display_name !== saved.display_name;

    return (
        <>
            <Section title="Informations personnelles" help="Ce que les autres voient de vous, dans GameFlex et dans les parties.">
                <Form props={{ onSubmit: (event) => event.preventDefault() }}>
                    <Rows>
                        <InputField name="username" label="Identifiant" help="Votre nom de connexion." context={form} />
                        <InputField name="display_name" label="Nom affiché" help="Le nom que les autres voient." context={form} />
                        <div className="list-group-item gf-row gf-row-submit">
                            <FormButton id="m-profile-save" label="Enregistrer" isReplaced={changed} disabled={Boolean(Ruleset.hasFeedbacks(form))} />
                        </div>
                    </Rows>
                </Form>
            </Section>
            <End guides={[["remoteAccess"]]} />
        </>
    );
}

// Les réglages des Journaux, sur leur propre route (/settings/logs/settings) : ils s'appliquent sur place. Sans le
// journal du serveur, les signalements et les niveaux n'ont rien où s'écrire : ils restent là, désactivés, et disent
// pourquoi.
function LogsForm() {
    // `?server=off` : la maquette s'ouvre journal du serveur éteint, pour voir les lignes désactivées.
    const [server] = useSearchParam("server", "on");
    const [values, setValues] = useState({ server: server !== "off", sessions: true, reports: true, debug: false, info: true, warn: true, error: true, http: false });
    const off = "Le journal du serveur est éteint.";
    const needsServer = (data) => (data.server ? null : Ruleset.isDisabled(off));
    const form = formContext(values, setValues, { reports: needsServer, debug: needsServer, info: needsServer, warn: needsServer, error: needsServer,
        http: (data) => (!data.server ? Ruleset.isDisabled(off) : !data.info ? Ruleset.isDisabled("Il faut le niveau Info.") : null) });

    return (
        <>
            <Section title="Écriture" help="Ce que le serveur écrit à partir de maintenant ; les journaux déjà écrits ne changent pas.">
                <Rows>
                    <SwitchLine name="server" context={form} label="Journal du serveur" help="Parties lancées • erreurs" texts={["Activé", "Désactivé"]} />
                    <SwitchLine name="sessions" context={form} label="Journaux des parties" help="Un fichier par partie" texts={["Activé", "Désactivé"]} />
                    <SwitchLine name="reports" context={form} label="Signalements" help="« Signaler l'erreur » des joueurs" />
                </Rows>
            </Section>
            <Section title="Niveaux à écrire" help="Debug dit tout et grossit vite ; Warn et Error, ce qui se passe mal.">
                <Rows>
                    <SwitchLine name="debug" context={form} label="Debug" help="Tout, pour chercher un défaut" />
                    <SwitchLine name="info" context={form} label="Info" help="Ce qui se passe" />
                    <SwitchLine name="warn" context={form} label="Warn" help="Ce qui pourrait mal tourner" />
                    <SwitchLine name="error" context={form} label="Error" help="Ce qui a échoué" />
                    <SwitchLine name="http" context={form} label="Requêtes HTTP" help="Une ligne par requête" texts={["Écrites", "Non"]} />
                </Rows>
            </Section>
            <End guides={[["storage", "more-disks-of-games"]]} />
        </>
    );
}

// Ajouter un membre : le mot de passe d'un compte créé par un lien d'invitation ne se donne pas — désactivé, avec sa
// raison, et non caché.
function MemberForm() {
    // `?username=…` : la maquette s'ouvre avec cet identifiant, pour voir ses règles.
    const [given] = useSearchParam("username", "");
    const [values, setValues] = useState({ username: given, display_name: "", admin: false, link: true, password: "" });
    const form = formContext(values, setValues, {
        username: (data) => {
            const name = String(data.username || "");
            if (!name) {
                return null;
            }

            return /\s/.test(name) ? Ruleset.isInvalid("Pas d'espace.") : name.length < 3 ? Ruleset.isInvalid("3 caractères au moins.")
                : name.length > 20 ? Ruleset.isInvalid("20 caractères au plus.") : null;
        },
        password: (data) => (data.link ? Ruleset.isDisabled("Il le choisira lui-même, par le lien.") : null),
    });
    const ready = values.username.length >= 3 && values.display_name.length >= 3 && !Ruleset.hasFeedbacks(form);

    return (
        <>
            <Section title="Le compte" help="Son nom de connexion, et le nom que les autres verront.">
                <Form props={{ onSubmit: (event) => event.preventDefault() }}>
                    <Rows>
                        <InputField name="username" label="Identifiant" help="3 à 20 caractères, sans espace ; il ne change plus ensuite." context={form} />
                        <InputField name="display_name" label="Nom affiché" help="Le nom que les autres verront." context={form} />
                        <SwitchLine name="admin" context={form} label="Administrateur" help="Il règle le serveur, ses consoles et ses membres." />
                        <SwitchLine name="link" context={form} label="Par un lien d'invitation" help="Il choisit son mot de passe en ouvrant le lien ; sinon, donnez-le ici." />
                        <InputField name="password" label="Mot de passe" help="Celui qu'il donnera pour se connecter." type="password" context={form} />
                        <div className="list-group-item gf-row gf-row-submit">
                            <FormButton id="m-member-add" label="Ajouter le membre" isReplaced={ready} />
                        </div>
                    </Rows>
                </Form>
            </Section>
            <End guides={[["remoteAccess"]]} />
        </>
    );
}

// UN IDENTIFIANT SECRET DÉJÀ ENREGISTRÉ (le porteur, 2026-11-25, à propos des clés des fournisseurs de métadonnées :
// « C'est possible de mettre le message […] pour chaque champ avec credentials ? ») : le champ reste vide, son texte
// d'attente dit qu'une valeur est enregistrée et que le laisser vide la garde (`settings.stored`, Metadata.jsx) ; on ne
// réaffiche jamais un secret. Un champ générique pour tous les champs dont on ne peut pas afficher la valeur (le porteur :
// « Les champs dont on ne [peut] pas afficher les infos ») : mots de passe, clés, jetons. Le texte, court, tient dans le
// champ d'un téléphone : « Enregistré — pas de changement ».
function ResourceForm() {
    const [values, setValues] = useState({ host: "nas.local", user: "gino", password: "", key: "" });
    const form = formContext(values, setValues);
    const changed = values.host !== "nas.local" || values.user !== "gino" || values.password !== "" || values.key !== "";
    const stored = "Enregistré — pas de changement";

    return (
        <>
            <Section title="Connexion" help="Où trouver le partage, et comment s'y connecter.">
                <Form props={{ onSubmit: (event) => event.preventDefault() }}>
                    <Rows>
                        <InputField name="host" label="Adresse" help="Le nom ou l'adresse de la machine." context={form} />
                        <InputField name="user" label="Utilisateur" help="Le compte sur cette machine." context={form} />
                        <InputField name="password" label="Mot de passe" help="Jamais réaffiché ; vide, il ne change pas." type="password" context={form} placeholder={stored} />
                        <InputField name="key" label="Clé SSH" help="Facultative ; vide, elle ne change pas." context={form} placeholder={stored} />
                        <div className="list-group-item gf-row gf-row-submit">
                            <FormButton id="m-resource-save" label="Enregistrer" isReplaced={changed} />
                        </div>
                    </Rows>
                </Form>
            </Section>
            <End guides={[["storage", "more-disks-of-games"]]} />
        </>
    );
}

// TOUS LES CHAMPS QUE L'APP EMPLOIE (le porteur, 2026-11-25 : « tu peux rajouter les champs comme calendrier,
// SkeletonRange, skeleton File/upload, input group button, skeletonRadio, etc… tous les champs qu'on utilise pour avoir
// une idée. Et même avoir les rulesets pour être sûr qu'on fait bien les choses ») : UN SEUL contexte de formulaire, et
// chaque champ passe par son Ruleset — invalide (le message de Skeleton sous le champ) ou désactivé (sa raison à la
// place de sa description), jamais caché. Les champs : Form.Text, Form.Password, Form.IntFloat, Form.Longtext,
// Form.Select, Form.SearchText, Form.Range, Form.SkeletonRadio, Form.SkeletonSwitch, Form.Date, Form.DateRange,
// Form.File, un champ et son bouton accolé (IconAddon).

// UN CHAMP DE SAISIE : Form.Text de Skeleton (un Form.Input de type texte : son retour, le contour rouge et le message,
// vient de Form.Input par Form.Feedback), ou Form.Password, dans sa ligne ; son bouton accolé à côté, le champ prenant
// le reste de la largeur. Une liste : Form.Select ; un long texte : Form.Longtext.
function InputField({ name, context, label, help, type = "text", placeholder = undefined, addon = null }) {
    const props = { autoComplete: "off", placeholder, "data-nav": true, "data-key": name, "aria-label": label };
    const input = type === "password" ? <Form.Password name={name} context={context} props={props} /> : <Form.Text name={name} context={context} props={props} />;

    return (
        <Field name={name} context={context} label={label} help={help}>
            {addon ? <div className="d-flex gap-2 align-items-start"><div className="flex-grow-1 min-w-0">{input}</div>{addon}</div> : input}
        </Field>
    );
}

// Un champ dans sa ligne : son libellé, sa description (ou la raison qui le désactive), le champ de Skeleton.
function Field({ name, context, label, help, children }) {
    return (
        <div className="list-group-item gf-row d-block">
            <Form.Group label={label} helpText={reason(context, name, help)}>{children}</Form.Group>
        </div>
    );
}

// La période : les deux calendriers de Form.DateRange l'un sous l'autre, comme la page des Journaux (DateRangeRow) — côte
// à côte, le second déborde d'un téléphone.
function PeriodCalendars({ context }) {
    const { Left, Right } = Form.DateRange.components({ name: "period", context });

    return (
        <div className="d-grid gap-2">
            <div className="border rounded p-2"><Left /></div>
            <div className="border rounded p-2"><Right /></div>
        </div>
    );
}

function AllFields() {
    // `?state=off` : la maquette s'ouvre avec ce qui désactive les autres champs (un kDrive, sans compte, tous les jeux,
    // l'image du service) ; `?state=bad`, avec des valeurs invalides.
    const [state] = useSearchParam("state", "");
    const [values, setValues] = useState({
        name: state === "bad" ? "Mo" : "Mon NAS", anonymous: state === "off", password: "", port: state === "bad" ? 70000 : 22,
        note: state === "bad" ? "x".repeat(130) : "", source: state === "off" ? "kdrive" : "sftp", console: "", minimum: 100, all: state === "off",
        region: "Europe", since: null, period: [null, null], art: state === "off" ? "Celle du service" : "Une image à moi", address: "nas.local",
    });
    const [files, setFiles] = useState({});
    const context = {
        values,
        setValues: (patch) => setValues((previous) => ({ ...previous, ...patch })),
        files,
        setFiles: (patch) => setFiles((previous) => ({ ...previous, ...patch })),
        rulesets: {
            name: (data) => (String(data.name || "").trim().length < 3 ? Ruleset.isInvalid("Trois caractères au moins.") : null),
            password: (data) => (data.anonymous ? Ruleset.isDisabled("La connexion est anonyme : pas de mot de passe.") : null),
            port: (data) => (data.source === "kdrive" ? Ruleset.isDisabled("Un kDrive n'a pas de port.")
                : Number(data.port) < 1 || Number(data.port) > 65535 ? Ruleset.isInvalid("Entre 1 et 65535.") : null),
            address: (data) => (data.source === "kdrive" ? Ruleset.isDisabled("Un kDrive se trouve par son compte.") : null),
            note: (data) => (String(data.note || "").length > 120 ? Ruleset.isInvalid("120 caractères au plus.") : null),
            minimum: (data) => (data.all ? Ruleset.isDisabled("Tous les jeux sont montrés.") : null),
            period: (data) => (!data.since ? Ruleset.isDisabled("Choisissez d'abord le premier jour.") : null),
            art: (data) => (data.source === "kdrive" ? Ruleset.isDisabled("Un kDrive n'a pas d'image à lui.") : null),
            upload: () => (values.art !== "Une image à moi" ? Ruleset.isDisabled("L'image vient du service.") : null),
        },
    };
    const consoles = ["PlayStation 5", "PlayStation 4", "PlayStation 3", "Nintendo Switch", "Xbox", "Wii U"];

    return (
        <>
            <Section title="Les champs de texte" help="Une ligne, un mot de passe, un nombre, un long texte.">
                <Rows>
                    <InputField name="name" context={context} label="Nom (Form.Text)" help="Trois caractères au moins : sinon, invalide." />
                    <SwitchLine name="anonymous" context={context} label="Connexion anonyme (Form.SkeletonSwitch)" help="Allumée, elle désactive le mot de passe." />
                    <Field name="password" context={context} label="Mot de passe (Form.Password)" help="Désactivé quand la connexion est anonyme.">
                        <Form.Password name="password" context={context} />
                    </Field>
                    <Field name="port" context={context} label="Port (Form.IntFloat)" help="Entre 1 et 65535 ; désactivé pour un kDrive.">
                        <Form.IntFloat name="port" context={context} min={1} max={65535} />
                    </Field>
                    <Field name="note" context={context} label="Note (Form.Longtext, counter)" help="120 caractères au plus : au-delà, invalide ; le compte sous le champ.">
                        <Form.Longtext name="note" context={context} counter={120} />
                    </Field>
                </Rows>
            </Section>
            <Section title="Les choix" help="Une liste, une recherche, des cartes, une glissière.">
                <Rows>
                    <Field name="source" context={context} label="Source (Form.Select)" help="Un kDrive désactive l'adresse, le port et l'image.">
                        <Form.Select name="source" context={context} options={[{ value: "sftp", label: "SFTP" }, { value: "webdav", label: "WebDAV" }, { value: "kdrive", label: "kDrive" }]} />
                    </Field>
                    <InputField name="address" context={context} label="Adresse, et son bouton (Form.Text + IconAddon)" help="Le bouton accolé teste la connexion."
                        addon={<IconAddon id="fields-test" icon="bi-plug" label="Tester la connexion" disabled={values.source === "kdrive"} onClick={() => {}} />} />
                    <Field name="console" context={context} label="Console (Form.SearchText)" help="Ce qu'on tape propose les consoles qui y ressemblent.">
                        <Form.SearchText name="console" context={context} callback={(term) => consoles.filter((item) => item.toLowerCase().includes(String(term).toLowerCase()))} />
                    </Field>
                    <SwitchLine name="all" context={context} label="Tous les jeux (Form.SkeletonSwitch)" help="Allumé, il désactive la taille minimum." texts={["Tous", "Filtrés"]} />
                    <Field name="minimum" context={context} label="Taille minimum (Form.Range)" help="0 à 1 000 Mo ; désactivée quand tous les jeux sont montrés.">
                        <Form.Range name="minimum" context={context} min={0} max={1000} step={10} />
                    </Field>
                    <Field name="region" context={context} label="Région (Form.SkeletonRadio)" help="Des cartes, une seule choisie.">
                        <Form.SkeletonRadio name="region" context={context} options={[
                            { icon: "bi bi-globe-europe-africa", title: "Europe", subtitle: "PAL • 50 Hz" },
                            { icon: "bi bi-globe-americas", title: "Amérique", subtitle: "NTSC • 60 Hz" },
                            { icon: "bi bi-globe-asia-australia", title: "Japon", subtitle: "NTSC-J • 60 Hz" }]} />
                    </Field>
                </Rows>
            </Section>
            <Section title="Les dates et les fichiers" help="Un jour, une période, une image à envoyer.">
                <Rows>
                    <Field name="since" context={context} label="Premier jour (Form.Date)" help="Il ouvre la période.">
                        <Form.Date name="since" context={context} />
                    </Field>
                    <Field name="period" context={context} label="Période (Form.DateRange)" help="Ses deux calendriers l'un sous l'autre (Journaux) ; désactivée tant que le premier jour manque.">
                        <PeriodCalendars context={context} />
                    </Field>
                    <Field name="art" context={context} label="L'image (Form.SkeletonRadio)" help="La sienne, ou celle du service.">
                        <Form.SkeletonRadio name="art" context={context} options={[
                            { icon: "bi bi-upload", title: "Une image à moi", subtitle: "Envoyée d'ici" },
                            { icon: "bi bi-cloud", title: "Celle du service", subtitle: "IGDB • SteamGridDB" }]} />
                    </Field>
                    <Field name="upload" context={context} label="Envoyer une image (Form.File)" help="PNG ou JPEG, 16 Mo au plus.">
                        <Form.File name="upload" context={context} accept="image/*" maxFileSize={16 * 1024 * 1024} />
                    </Field>
                    <div className="list-group-item gf-row gf-row-submit">
                        <FormButton id="fields-save" label="Enregistrer" isReplaced disabled={Boolean(Ruleset.hasFeedbacks(context))} />
                    </div>
                </Rows>
            </Section>
            <End guides={[["storage", "more-disks-of-games"]]} />
        </>
    );
}

function Empty() {
    return <Rows><EmptyRow text="Maquette inconnue." /></Rows>;
}

const MOCKS = {
    rules: ["Les règles", Rules],
    settings: ["Réglages", SettingsHome],
    profile: ["Mon compte", ProfileForm],
    logs: ["Réglages des journaux", LogsForm],
    member: ["Nouveau membre", MemberForm],
    resource: ["Mon NAS", ResourceForm],
    fields: ["Tous les champs", AllFields],
    home: ["Accueil", Home],
    console: ["PlayStation 5", Console],
    account: ["Profil", Account],
    game: ["Astro Bot", Game],
};

export default function MockupsPage() {
    const [mock] = useSearchParam("m", "rules");
    const [title, Body] = MOCKS[mock] || ["Maquettes", Empty];

    return (
        <SettingsPage title={title} parent="/settings/templates">
            <Body />
        </SettingsPage>
    );
}
