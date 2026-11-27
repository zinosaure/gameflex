import React, { useState } from "react";
import { fact } from "../../libs/format.js";
import { AccountName } from "../../bricks/AccountName.jsx";
import { AddressRow } from "../../bricks/AddressRow.jsx";
import { Avatar } from "../../bricks/Avatar.jsx";
import { BlockRow } from "../../bricks/BlockRow.jsx";
import { Chip } from "../../bricks/Chip.jsx";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { CopyField } from "../../bricks/CopyField.jsx";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { DownloadRow } from "../../bricks/DownloadRow.jsx";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import { EmptyState } from "../../bricks/EmptyState.jsx";
import { FileRow } from "../../bricks/FileRow.jsx";
import { FilterGroups, switchGroup } from "../../bricks/FilterGroups.jsx";
import { GameFacts } from "../../bricks/GameFacts.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { IconAddon } from "../../bricks/IconAddon.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { QuickTiles } from "../../bricks/QuickTiles.jsx";
import { RadioRow } from "../../bricks/RadioRow.jsx";
import { RangeRow } from "../../bricks/RangeRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SelectRow } from "../../bricks/SelectRow.jsx";
import { SplitRow } from "../../bricks/SplitRow.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { TextSection } from "../../bricks/TextSection.jsx";
import { Thumb } from "../../bricks/Thumb.jsx";
import { formContext, FormButton } from "../../ui/Forms.jsx";

// LE CATALOGUE DES MODÈLES (le porteur, 2026-11-23 : « Fais en sorte que toutes les pages admin suivent un template très
// strict […] dans le dev tu crées des pages templates avec les composants pour qu'on valide le design ensemble et après on
// dit où les appliquer. Si possible tu donnes à chaque composant un nom et code unique pour se repérer et rajoute un
// tableau pour savoir où le template est appliqué dans les settings. Liste aussi les arguments. ») — un outil de la dev
// (capacité `staging`), écrit en français : chaque COMPOSANT (C…) et chaque GABARIT DE PAGE (T…) a son code, son rôle, ses
// arguments et un exemple vivant ; PAGES dit quel gabarit chaque page des Réglages applique, et si elle y est conforme.
//
// Un argument : [nom, type, défaut, rôle]. Les codes ne changent jamais : on s'y réfère dans les échanges.

const noop = () => {};

function SwitchExample() {
    const [on, setOn] = useState(true);

    return <Rows><SwitchRow id="tpl-switch" label="Afficher les consoles sans jeu" help="Elles sont masquées par défaut." checked={on} onChange={setOn} /></Rows>;
}

function RangeExample() {
    const [value, setValue] = useState(100);

    return <Rows><RangeRow id="tpl-range" label="Taille minimum (Mo)" help="Seulement les jeux d'au moins cette taille ; 0 : tous." value={value} min={0} max={1000} step={10} onChange={setValue} /></Rows>;
}

function SelectExample() {
    const [value, setValue] = useState("auto");

    return (
        <Rows>
            <SelectRow id="tpl-select" label="Débit" help="Auto suit le réseau du moment." value={value} onChange={setValue}
                options={[{ value: "auto", label: "Auto" }, { value: "4000", label: "4 Mbps" }, { value: "8000", label: "8 Mbps" }]} />
        </Rows>
    );
}

function RadioExample() {
    const [value, setValue] = useState("a");

    return (
        <Rows>
            <RadioRow id="tpl-radio" label="Source" help="Où le jeu se télécharge." value={value} onChange={setValue}
                options={[{ value: "a", icon: "bi-hdd", label: "Gamebox", help: [fact("Type", "Disque interne"), fact("Taille", "1.2 GB")] },
                    { value: "b", icon: "bi-cloud", label: "kDrive", help: [fact("Type", "kDrive"), fact("Taille", "1.2 GB")] }]} />
        </Rows>
    );
}

function FormExample() {
    const [name, setName] = useState("");

    return (
        <Section help="Le dossier doit contenir un dossier « roms », rangé par console.">
            <form onSubmit={(event) => event.preventDefault()}>
                <Rows>
                    <TextRow id="tpl-name" label="Nom" help="Le nom affiché pour cette connexion." value={name} onChange={setName} />
                    <SubmitRow><FormButton id="tpl-save" label="Enregistrer" isReplaced={name.trim() !== ""} /></SubmitRow>
                </Rows>
            </form>
        </Section>
    );
}

function ChoiceExample() {
    const [value, setValue] = useState("ps5");

    return <Rows><ChoiceRow id="tpl-choice" label="Type" value={value} onChange={setValue} options={[["ps4", "PS4"], ["ps5", "PS5"]]} /></Rows>;
}

function AddonExample() {
    const [value, setValue] = useState("");

    return (
        <Rows>
            <TextRow id="tpl-addon" label="Adresse IP" help="L'adresse de la console sur votre réseau, ou laissez la recherche la trouver." value={value} onChange={setValue}
                props={{ placeholder: "192.168.x.x" }} addon={<IconAddon id="tpl-addon-search" icon="bi-search" label="Rechercher" kind="main" onClick={noop} />} />
        </Rows>
    );
}

function AddressExample() {
    const [values, setValues] = useState({ "tpl-host": "", "tpl-port": "" });
    const [scheme, setScheme] = useState("smb");

    return (
        <Rows>
            <AddressRow id="tpl-host" portId="tpl-port" label="Adresse" help="Le nom ou l'adresse IP du serveur ; le port, à droite, s'il n'est pas celui par défaut."
                context={formContext(values, setValues)} schemes={["smb", "webdav"]} scheme={scheme} onScheme={setScheme} portLabel="Port" portPlaceholder="445" />
        </Rows>
    );
}

function FileExample() {
    const [values, setValues] = useState({});

    return <Rows><FileRow id="tpl-file" label="Image" help="Une image de votre appareil." upload={{ context: formContext(values, setValues) }} /></Rows>;
}

const LIST_ITEMS = [
    { id: "gbc", name: "Game Boy / Game Boy Color", kind: "Retrogaming", games: 494 },
    { id: "n64", name: "Nintendo 64", kind: "Retrogaming", games: 23 },
    { id: "ps5", name: "PlayStation 5", kind: "Streamex", games: 5 },
];

function ListingExample() {
    const [search, setSearch] = useState("");
    const [hidden, setHidden] = useState([]);
    const shown = LIST_ITEMS.filter((item) => item.name.toLowerCase().includes(search.toLowerCase()) && !hidden.includes(item.kind));

    return (
        <Listing items={shown} search={search} onSearch={setSearch} placeholder="Rechercher une console…" searchKey="tpl-search" found={`${shown.length} consoles`} empty="Aucune console."
            filterCount={hidden.length} filters={[switchGroup({ key: "kinds", title: "Type", id: "tpl-kind", hidden,
                onChange: (value, on) => setHidden(on ? hidden.filter((entry) => entry !== value) : [...hidden, value]),
                choices: [{ value: "Retrogaming", label: "Retrogaming" }, { value: "Streamex", label: "Streamex" }] })]}
            render={(item) => (
                <NavRow key={item.id} id={`tpl-row-${item.id}`} icon={item.kind === "Streamex" ? "bi-display" : "bi-joystick"} tone={item.kind === "Streamex" ? "rose" : "indigo"}
                    label={item.name} checked help={[fact("Type", item.kind), fact("Nombre de jeux", `${item.games} JEUX`)]} onOpen={noop} />
            )} />
    );
}

// LES COMPOSANTS. `example` : ce qu'il donne, vivant (une fonction : chaque rendu le reconstruit).
export const COMPONENTS = [
    {
        code: "C01", name: "SettingsPage", file: "settings/SettingsPage.jsx",
        role: "La page des Réglages : l'en-tête (Retour, titre, fil d'Ariane), puis ses sections. Toute page des Réglages en est une.",
        args: [["title", "texte", "—", "Le titre de la page."], ["parent", "chemin", "/settings", "Où mène Retour quand on arrive par un lien direct."],
            ["levels", "{ chemin: nom }", "{}", "Les noms du fil d'Ariane qu'une adresse ne dit pas."], ["enter", "fonction", "null", "Le geste du bouton A de la manette sur la page."], ["children", "sections", "—", "Le corps : des Section."]],
    },
    {
        code: "C02", name: "Section", file: "bricks/Section.jsx",
        role: "Un groupe titré : son titre au-dessus, son aide dessous (seulement si elle apprend quelque chose), puis son contenu.",
        args: [["title", "texte", "null", "Le titre du groupe (« Détails », « Plus », « Danger », « Guide »…)."], ["help", "texte", "null", "Une phrase sous le titre ; jamais une redite."],
            ["badge", "élément", "null", "Une pastille à côté du titre (l'icône du Guide, « Administrateur »)."], ["children", "élément", "—", "Rows, Listing, un formulaire…"]],
        example: () => <Section title="Détails" help="Une phrase seulement si elle apprend quelque chose."><Rows><InfoRow icon="bi-joystick" label="Console" value="PlayStation 5" /></Rows></Section>,
    },
    {
        code: "C03", name: "Rows", file: "bricks/Rows.jsx",
        role: "L'encadré arrondi qui tient les lignes d'un groupe. Une ligne est TOUJOURS dans un Rows, jamais seule.",
        args: [["children", "lignes", "—", "NavRow, InfoRow, SwitchRow…"], ["className", "classes", "\"\"", "Rarement : un écart vertical."]],
        example: () => <Rows><InfoRow icon="bi-joystick" label="Console" value="Nintendo 64" /><InfoRow icon="bi-hdd" label="Taille" value="12.0 MB" /></Rows>,
    },
    {
        code: "C04", name: "NavRow", file: "bricks/NavRow.jsx",
        role: "La ligne qui fait quelque chose : ouvrir une page (chevron), poser une question (?), montrer une information (i), ouvrir un lien (flèche), ou agir sur place (rien). Toute la ligne se touche.",
        args: [["id", "texte", "—", "La clé de la ligne (la lumière de la manette y revient)."], ["icon", "bi-…", "bi-chevron-right", "L'icône de la pastille."], ["tone", "teinte", "slate", "indigo, teal, green, amber, rose, violet, sky, slate."],
            ["label", "texte", "—", "Le titre de la ligne."], ["help", "texte ou liste", "null", "Les sous-titres : une phrase, ou des fact() ; deux au plus si la ligne ouvre une page."],
            ["value", "texte ou fact", "null", "La valeur sous le titre (un réglage, un compte)."], ["lead", "élément", "null", "Une jaquette ou un avatar à la place de la pastille."],
            ["opens", "page | confirm | info | link | action | delete", "page", "Ce que la ligne fait : dit au bord droit."], ["checked", "booléen", "false", "La validation verte à côté du titre : ce qui est en service, activé, déjà ici."], ["obsolete", "booléen", "false", "Une ancienne version : toute la ligne grisée."], ["blocked", "warning | danger", "null", "Une action qui ne peut pas se faire : orange (il lui manque quelque chose), rouge (impossible)."],
            ["pending", "booléen", "false", "Le geste est en cours : la pastille devient la roue, la ligne ne se touche plus."], ["disabled", "booléen", "false", "Grisée ; sa raison va dans help."],
            ["danger", "booléen", "false", "Le titre en rouge (ligne de Danger)."], ["progress", "0-100 | -1", "null", "Un téléchargement suivi depuis la ligne."],
            ["separator", "texte | null", "SUBTITLES", "Tous les sous-titres sur une ligne, joints par lui."], ["noKey", "booléen", "SUBTITLES", "Les valeurs seules, sans leur clé."], ["onOpen", "fonction", "—", "Le geste."]],
        example: () => (
            <Rows>
                <NavRow id="tpl-nav-page" icon="bi-collection-play" tone="indigo" label="Jeux" help={[fact("Nombre de jeux", "494 JEUX"), fact("Type", "Bibliothèque")]} onOpen={noop} />
                <NavRow id="tpl-nav-confirm" icon="bi-arrow-repeat" tone="indigo" label="Relire" help="Relit le partage pour y trouver les jeux ajoutés ou retirés." opens="confirm" onOpen={noop} />
                <NavRow id="tpl-nav-pending" icon="bi-archive" tone="indigo" label="Backup" help="Une copie mise de côté, dans vos backups." pending opens="confirm" onOpen={noop} />
                <NavRow id="tpl-nav-off" icon="bi-collection-play" tone="slate" label="Voir sa fiche dans la bibliothèque" help="Aucun jeu d'ici ne porte cette sauvegarde." disabled onOpen={noop} />
            </Rows>
        ),
    },
    {
        code: "C05", name: "InfoRow", file: "bricks/InfoRow.jsx",
        role: "La ligne de renseignement d'une fiche : son libellé (la clé), sa valeur dessous. Aucun geste. Sans valeur, elle n'apparaît pas.",
        args: [["icon", "bi-…", "null", "L'icône ; une fiche en met sur toutes ses lignes."], ["tone", "teinte", "slate", "La teinte de la pastille."], ["label", "texte", "—", "La clé (« Console », « Taille »)."],
            ["value", "texte, élément, fact ou liste", "—", "La valeur."], ["help", "texte", "null", "Une précision sous la valeur."], ["mono", "booléen", "false", "Un chemin, un fichier : en chasse fixe."], ["lead", "élément", "null", "Une vignette à la place de la pastille."],
            ["separator", "texte | null", "SUBTITLES", "Comme NavRow."], ["noKey", "booléen", "SUBTITLES", "Comme NavRow."]],
        example: () => (
            <Rows>
                <InfoRow icon="bi-joystick" label="Console" value="PlayStation 4" />
                <InfoRow icon="bi-arrow-left-right" tone="green" label="Provenance" value="Gamebox" />
                <InfoRow icon="bi-folder2" label="Dossier" value="/mnt/gamebox/roms/ps4/Kingdom Hearts III" mono />
            </Rows>
        ),
    },
    {
        code: "C06", name: "SplitRow", file: "bricks/SplitRow.jsx",
        role: "Une ligne et un bouton carré à son bord droit (reprendre une partie et l'arrêter, un téléchargement et l'annuler).",
        args: [["id", "texte", "—", "La clé de la ligne."], ["label", "texte", "—", "Le titre."], ["help", "texte ou liste", "null", "Les sous-titres."], ["lead", "élément", "null", "Une jaquette."],
            ["onOpen", "fonction", "—", "Le geste de la ligne."], ["action", "{ id, kind, icon, label, disabled, pending, onClick }", "—", "Le bouton : une icône, jamais un texte."]],
        example: () => (
            <Rows>
                <SplitRow id="tpl-split" icon="bi-controller" tone="green" label="New Super Mario Bros. Wii" help={[fact("Durée", "00:00:43"), fact("Temps pour revenir", "04:43")]} onOpen={noop}
                    action={{ id: "tpl-split-stop", kind: "danger", icon: "bi-power", label: "Arrêter", onClick: noop }} />
            </Rows>
        ),
    },
    {
        code: "C07", name: "SwitchRow", file: "bricks/SwitchRow.jsx",
        role: "Un interrupteur qui s'applique aussitôt (un réglage, un filtre). Pas d'icône à gauche.",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "Ce qu'il active."], ["help", "texte", "null", "Ce que ça change."], ["checked", "booléen", "—", "Son état."], ["disabled", "booléen", "false", "Grisé."], ["onChange", "fonction(on)", "—", "Le basculement."]],
        example: () => <SwitchExample />,
    },
    {
        code: "C08", name: "SelectRow", file: "bricks/SelectRow.jsx",
        role: "Une liste déroulante (un réglage parmi plusieurs).",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "Le réglage."], ["help", "texte", "null", "Ce qu'il fait."], ["value", "texte", "—", "La valeur choisie."], ["options", "[{ value, label }]", "—", "Les choix."], ["onChange", "fonction(value)", "—", "Le choix."], ["disabled", "booléen", "false", "Grisé."], ["addon", "élément", "null", "Un bouton accolé (IconAddon)."]],
        example: () => <SelectExample />,
    },
    {
        code: "C09", name: "RangeRow", file: "bricks/RangeRow.jsx",
        role: "Une glissière et sa case (une taille, un pourcentage).",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "Le réglage."], ["help", "texte", "null", "Ce qu'il fait."], ["value", "nombre", "—", "La valeur."], ["min", "nombre", "—", "Le minimum."], ["max", "nombre", "—", "Le maximum."], ["step", "nombre", "1", "Le pas."], ["onChange", "fonction(value)", "—", "Le réglage."]],
        example: () => <RangeExample />,
    },
    {
        code: "C10", name: "TextRow", file: "bricks/TextRow.jsx",
        role: "Un champ de saisie d'un formulaire (texte, nombre, mot de passe, adresse).",
        args: [["id", "texte", "—", "Le nom du champ."], ["label", "texte", "—", "Ce qu'on saisit."], ["help", "texte ou liste", "null", "Comment le remplir."], ["value", "texte", "\"\"", "La valeur (sans context)."], ["onChange", "fonction(value)", "—", "La saisie."],
            ["context", "formContext", "null", "Le formulaire Skeleton qui le valide."], ["kind", "text | number | password…", "text", "La sorte de champ."], ["addon", "élément", "null", "Un bouton accolé."], ["props", "objet", "{}", "Le placeholder, data-key…"]],
        example: () => <FormExample />,
    },
    {
        code: "C11", name: "RadioRow", file: "bricks/RadioRow.jsx",
        role: "Un choix en cartes (une source, une manette), chaque carte avec son icône et ses sous-titres.",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "null", "Le titre du choix."], ["help", "texte", "null", "Ce que le choix fait."], ["value", "texte", "—", "La carte choisie."], ["options", "[{ value, icon, label, help }]", "—", "Les cartes ; help suit les règles des sous-titres."], ["onChange", "fonction(value)", "—", "Le choix."]],
        example: () => <RadioExample />,
    },
    {
        code: "C12", name: "SubmitRow + FormButton", file: "bricks/, ui/Forms.jsx",
        role: "La dernière ligne d'un formulaire et son bouton : « ••• » et rouge tant que rien n'a changé, puis primary.",
        args: [["id", "texte", "—", "La clé du bouton."], ["label", "texte", "—", "« Enregistrer », « Ajouter »…"], ["isReplaced", "booléen", "—", "Quelque chose a changé : le bouton s'allume."], ["pending", "booléen", "false", "L'envoi est en cours."], ["disabled", "booléen", "false", "Un champ refuse sa valeur."], ["type", "submit | button", "submit", "Le geste du formulaire."]],
        example: () => <FormExample />,
    },
    {
        code: "C13", name: "DownloadRow", file: "bricks/DownloadRow.jsx",
        role: "Un téléchargement en cours : l'anneau du pourcentage (ou la roue), « reçu sur total », Annuler. Le même partout (bibliothèque, sauvegardes).",
        args: [["entry", "{ state, phase, received, total, position, downloadId, kind }", "—", "Où il en est (la file des jeux, ou un transfert du navigateur)."], ["id", "texte", "—", "La clé."], ["label", "texte", "—", "Ce qui se télécharge."],
            ["help", "texte", "null", "Remplace le sous-titre."], ["onCancel", "fonction", "null", "Un transfert du navigateur : son annulation à lui."]],
        example: () => <Rows><DownloadRow id="tpl-download" label="Télécharger" entry={{ state: "running", phase: "download", received: 48 * 1024 * 1024, total: 120 * 1024 * 1024 }} onCancel={noop} /></Rows>,
    },
    {
        code: "C14", name: "DangerSection / DangerRow", file: "bricks/",
        role: "Ce qui ne revient pas (supprimer, oublier, réinitialiser) : seul, en bas de la page, dans « Danger », confirmé par une fenêtre.",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "« Supprimer »…"], ["help", "texte", "null", "Ce qui part, ce qui reste."], ["question", "texte", "—", "La question de la fenêtre."], ["confirmLabel", "texte", "label", "Le bouton de la fenêtre."],
            ["disabled", "booléen", "false", "Grisé (sa raison dans help)."], ["pending", "booléen", "false", "Le geste confirmé est en cours."], ["icon", "bi-…", "bi-trash", "L'icône."], ["onConfirm", "fonction", "—", "Le geste."]],
        example: () => <DangerSection id="tpl-danger" label="Supprimer" help="Sa sauvegarde part ; les backups restent." question="Supprimer la sauvegarde de « Tekken 8 » ?" onConfirm={noop} />,
    },
    {
        code: "C15", name: "Listing", file: "bricks/Listing.jsx",
        role: "TOUTE liste : la recherche et son bouton Filtres, « N trouvés », un trait, les lignes, la pagination (10 par page), les lignes fantômes pendant la lecture.",
        args: [["items", "liste", "—", "Ce qui est montré (déjà filtré)."], ["render", "fonction(item)", "—", "La ligne d'un élément (NavRow, InfoRow)."], ["search", "texte", "\"\"", "La recherche (dans l'adresse)."], ["onSearch", "fonction(q)", "null", "Sans elle, pas de recherche."],
            ["placeholder", "texte", "\"\"", "« Rechercher… »."], ["found", "texte", "null", "« 24 consoles »."], ["empty", "texte", "null", "Rien à montrer."], ["filters", "[groupe]", "null", "Les groupes de filtres (C16) ; aucun, pas de bouton."], ["filterCount", "nombre", "0", "Combien de filtres sont posés (la pastille)."],
            ["head", "élément", "null", "Une ligne en tête (rare : préférer « Plus »)."], ["page", "nombre", "null", "La page (dans l'adresse)."], ["onPage", "fonction", "null", "Changer de page."], ["pageSize", "nombre", "10", "Lignes par page."], ["loading", "booléen", "false", "Les fantômes."], ["placeholderRow", "fonction", "null", "La forme d'un fantôme (C21)."], ["searchKey", "texte", "sc-search", "La clé de la recherche."]],
        example: () => <ListingExample />,
    },
    {
        code: "C16", name: "FilterGroups (switchGroup, statusGroup, sizeGroup)", file: "bricks/",
        role: "Le panneau « Filtres » : chaque groupe sa section titrée. Les groupes d'interrupteurs (tous allumés au départ) se construisent avec switchGroup ; un groupe d'un seul choix n'existe pas.",
        args: [["groups", "[{ key, title, help, rows }]", "—", "Les groupes (FilterGroups)."], ["switchGroup({ key, title, id, choices, hidden, onChange })", "groupe", "—", "Un interrupteur par choix ; éteindre l'écarte."],
            ["statusGroup({ t, id, hidden, onChange })", "groupe", "—", "Les statuts d'un jeu."], ["sizeGroup({ t, id, minSize, onMinSize, heaviest, onHeaviest })", "groupe", "—", "La taille minimum et « les plus lourds d'abord »."],
            ["hiddenOf(texte) / toggleHidden(liste, valeur, on)", "aide", "—", "La liste écartée, dans l'adresse (?nostatus=a,b)."]],
        example: () => <FilterGroups groups={[switchGroup({ key: "kinds", title: "Type", id: "tpl-fg", hidden: [], onChange: noop, choices: [{ value: "r", label: "Retrogaming" }, { value: "s", label: "Streamex" }] })]} />,
    },
    {
        code: "C17", name: "GameFacts", file: "bricks/GameFacts.jsx",
        role: "Les informations d'un jeu (ou d'une sauvegarde) : les mêmes lignes, le même ordre, les mêmes icônes dans toute fiche.",
        args: [["facts", "{ console, origin, here, status, statusNote, series, players, gameId, engine, date, sizeBytes }", "{}", "Ce que la page sait."], ["metadata", "fiche | undefined", "null", "La fiche du jeu (undefined : elle se lit)."],
            ["refusal", "texte", "null", "Pourquoi la fiche ne se lit pas."], ["sources", "[{ label, href }]", "[]", "D'où vient la fiche."], ["children", "lignes", "null", "Des lignes en tête (pourquoi le jeu ne partirait pas)."]],
        example: () => <Section title="Détails"><GameFacts facts={{ console: "PlayStation 5", origin: "Gamebox", status: "playable", gameId: "PPSA10594", sizeBytes: 230 * 1024 * 1024 }} metadata={{ releaseDate: "2024", publishers: ["Bandai Namco"], genres: ["Combat"] }} /></Section>,
    },
    {
        code: "C18", name: "Sous-titres (fact, counted, helpLines, SUBTITLES)", file: "libs/format.js, bricks/",
        role: "Les règles des sous-titres : une information par ligne ; « Clé : valeur » par fact() ; un compte avec son unité en majuscules (counted) ; deux lignes au plus pour une ligne qui ouvre une page ; une phrase garde sa ligne. SUBTITLES fixe le séparateur et l'absence de clés pour tout le frontend.",
        args: [["fact(clé, valeur)", "info", "—", "Une information ; rien si la valeur est vide."], ["counted(t, unité, nombre)", "texte", "—", "« 999 JEUX » (units.*)."], ["helpLines(help, max, separator, noKey)", "lignes", "∞, SUBTITLES", "Le rendu commun."],
            ["SUBTITLES", "{ separator, noKey }", "{ \"•\", true }", "Les défauts de toutes les briques."], ["MAX_LINES", "nombre", "2", "Pour une ligne qui ouvre une page."]],
        example: () => <Rows><NavRow id="tpl-sub" icon="bi-joystick" tone="indigo" label="Nintendo 64" help={[fact("Type", "Retrogaming"), fact("Nombre de jeux", "23 JEUX")]} onOpen={noop} /></Rows>,
    },
    {
        code: "C19", name: "EmptyRow / EmptyState", file: "bricks/",
        role: "Rien à montrer : une ligne grisée dans un groupe (EmptyRow), ou l'encadré seul (EmptyState).",
        args: [["icon", "bi-…", "bi-inbox", "L'icône."], ["text", "texte", "—", "« Aucun serveur fédéré. »"]],
        example: () => <><Rows><EmptyRow icon="bi-inbox" text="Aucun serveur fédéré." /></Rows><EmptyState icon="bi-sd-card" text="Aucune sauvegarde." /></>,
    },
    {
        code: "C20", name: "PlaceholderRow", file: "bricks/PlaceholderRow.jsx",
        role: "La ligne fantôme pendant une lecture, de la forme des vraies lignes.",
        args: [["lead", "chip | cover | thumb | avatar | null", "chip", "La forme de la tête."], ["help", "booléen", "true", "Un sous-titre fantôme."], ["chevron", "booléen", "true", "Le chevron."]],
        example: () => <Rows><PlaceholderRow /><PlaceholderRow lead="cover" /></Rows>,
    },
    {
        code: "C21", name: "Note / Loading", file: "bricks/",
        role: "Une phrase atténuée DANS une section (une raison, un renvoi) ; Loading : « Chargement… ». Jamais un geste en cours : il va sur sa ligne (pending).",
        args: [["children", "texte", "—", "La phrase (Note)."], ["className", "classes", "mt-3", "L'écart (Note)."]],
        example: () => <Section title="Informations"><Loading /><Note>La fiche vient de libretro-database.</Note></Section>,
    },
    {
        code: "C22", name: "GuideSection", file: "bricks/GuideSection.jsx",
        role: "« Guide » en bas de page : les liens vers la documentation de GameFlex.",
        args: [["guides", "[[clé, ancre]]", "—", "Les guides et leur partie (libs/links.js)."]],
        example: () => <GuideSection guides={[["storage", "games-on-a-network-share"]]} />,
    },
    {
        code: "C23", name: "QuickTiles", file: "bricks/QuickTiles.jsx",
        role: "Des tuiles en grille, deux par rangée (l'accueil des Réglages, l'engrenage d'un jeu).",
        args: [["items", "[{ id, icon, tone, label, value, on, primary, wide, disabled, onClick }]", "—", "Les tuiles ; value suit les règles des sous-titres."]],
        example: () => <QuickTiles items={[{ id: "tpl-t1", icon: "bi-sliders", tone: "indigo", label: "Préréglage", value: fact("Émulateur", "fceumm"), onClick: noop }, { id: "tpl-t2", icon: "bi-broadcast", tone: "sky", label: "Flux", value: [fact("Résolution", "540p"), fact("FPS", "30 fps")], onClick: noop }]} />,
    },
    {
        code: "C24", name: "TextSection (Bon à savoir)", file: "bricks/",
        role: "Des renseignements titrés sans geste, en bas d'une page d'ajout.",
        args: [["title", "texte", "—", "« Bon à savoir »."], ["icon", "bi-…", "—", "L'icône du titre."], ["items", "[{ key, title, text, href }]", "—", "Les textes (un lien si href)."]],
        example: () => <TextSection title="Bon à savoir" icon="bi-info-circle" items={[{ key: "a", title: "Vos sauvegardes", text: "Elles restent sur ce serveur." }]} />,
    },
    {
        code: "C25", name: "ChoiceRow", file: "bricks/ChoiceRow.jsx",
        role: "Un choix de deux à quatre options, toutes visibles, en boutons accolés : la choisie pleine, les autres à contour. Au-delà de quatre : SelectRow (C08) ; des options qui se lisent (une aide sous chacune) : RadioRow (C11).",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "Le titre du choix."], ["help", "texte", "null", "Ce que le choix fait."], ["value", "texte", "—", "L'option choisie."],
            ["options", "[[valeur, libellé]]", "—", "Deux à quatre options."], ["onChange", "fonction(valeur)", "—", "Le choix."], ["isDisabled", "fonction(valeur)", "() => false", "Une option grisée."]],
        example: () => <ChoiceExample />,
    },
    {
        code: "C26", name: "IconAddon", file: "bricks/IconAddon.jsx",
        role: "Le bouton à icône accolé à un champ (rechercher, effacer, copier), passé en `addon` de TextRow ou SelectRow. Une icône, jamais un texte.",
        args: [["id", "texte", "—", "La clé."], ["icon", "bi-…", "—", "L'icône."], ["label", "texte", "—", "Ce qu'il fait (lu à voix haute, en infobulle)."], ["kind", "main | other | danger…", "other", "La sorte du bouton (libs/ui.js)."],
            ["disabled", "booléen", "false", "Grisé."], ["onClick", "fonction", "—", "Le geste."]],
        example: () => <AddonExample />,
    },
    {
        code: "C27", name: "AddressRow", file: "bricks/AddressRow.jsx",
        role: "Une adresse réseau sur une ligne : [protocole | serveur] puis [port], chaque champ avec sa règle (un formulaire Skeleton).",
        args: [["id", "texte", "—", "Le champ du serveur."], ["portId", "texte", "—", "Le champ du port."], ["label", "texte", "—", "« Adresse »."], ["help", "texte", "null", "Comment la remplir."], ["context", "formContext", "—", "Le formulaire."],
            ["schemes", "[texte]", "—", "Les protocoles ; un seul : une étiquette."], ["scheme", "texte", "—", "Le protocole choisi."], ["onScheme", "fonction", "—", "Le choix du protocole."], ["portLabel", "texte", "—", "« Port »."], ["portPlaceholder", "texte", "\"\"", "Le port par défaut."]],
        example: () => <AddressExample />,
    },
    {
        code: "C28", name: "CopyField", file: "bricks/CopyField.jsx",
        role: "Une valeur à recopier : le champ en lecture seule, en chasse fixe, et l'icône qui copie (un jeton, une adresse).",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "null", "Ce que c'est."], ["help", "texte", "null", "À quoi ça sert."], ["value", "texte", "—", "La valeur."]],
        example: () => <Rows><CopyField id="tpl-copy" label="Adresse du serveur" value="https://gameflex.example.org" /></Rows>,
    },
    {
        code: "C29", name: "FileRow", file: "bricks/FileRow.jsx",
        role: "Choisir un fichier de l'appareil (une image, un QR) : le champ de fichier de Skeleton.",
        args: [["id", "texte", "—", "La clé."], ["label", "texte", "—", "Ce qu'on choisit."], ["help", "texte", "null", "Comment."], ["upload", "{ context }", "—", "Le formulaire qui reçoit le fichier."],
            ["accept", "type MIME", "image/*", "Les fichiers acceptés."], ["maxFileSize", "octets | null", "16 Mo", "La taille au plus."]],
        example: () => <FileExample />,
    },
    {
        code: "C30", name: "Avatar / AccountName / AdminMark", file: "bricks/",
        role: "Un compte : ses initiales dans un rond (Avatar, en `lead` d'une ligne), son nom suivi de la marque d'administrateur (AccountName, AdminMark).",
        args: [["Avatar.name", "texte", "—", "Le nom, dont les initiales."], ["AccountName.name / admin", "texte / booléen", "— / false", "Le nom ; la marque s'il administre."], ["AdminMark.label", "texte", "—", "Ce que dit la marque."]],
        example: () => <Rows><NavRow id="tpl-member" lead={<Avatar name="Gino Dupont" />} label={<AccountName name="Gino" admin />} help={[fact("Nom d'utilisateur", "@gino"), fact("Jeton", "Activé")]} onOpen={noop} /></Rows>,
    },
    {
        code: "C31", name: "Thumb", file: "bricks/Thumb.jsx",
        role: "Une vignette 4:3 en tête d'une ligne (une image proposée, un état sauvegardé) ; sans image, l'icône d'une image.",
        args: [["url", "adresse", "null", "L'image."], ["fit", "contain | cover", "contain", "Comment elle remplit la vignette."]],
        example: () => <Rows><NavRow id="tpl-thumb" lead={<Thumb />} label="Emplacement 1" help={fact("Date", "13 nov. 2026, 02:20")} onOpen={noop} /></Rows>,
    },
    {
        code: "C32", name: "Chip", file: "bricks/Chip.jsx",
        role: "La pastille d'une ligne : une icône dans un carré de sa teinte. Toutes les lignes la font d'elles-mêmes (`icon`, `tone`).",
        args: [["icon", "bi-…", "—", "L'icône."], ["tone", "teinte", "slate", "indigo, teal, green, amber, rose, violet, sky, slate."]],
        example: () => <div className="d-flex gap-2 flex-wrap">{["indigo", "teal", "green", "amber", "rose", "violet", "sky", "slate"].map((tone) => <Chip key={tone} icon="bi-joystick" tone={tone} />)}</div>,
    },
    {
        code: "C33", name: "BlockRow", file: "bricks/BlockRow.jsx",
        role: "Une ligne libre dans un groupe, pour ce qu'aucune autre ligne ne fait (deux calendriers, un bloc de texte long).",
        args: [["children", "élément", "—", "Le contenu."]],
        example: () => <Rows><BlockRow><span className="small">Un contenu libre : à n'utiliser que quand aucune autre ligne ne convient.</span></BlockRow></Rows>,
    },
];

// LES GABARITS DE PAGE. `structure` : l'ordre strict des sections, chacune avec ses composants.
export const TEMPLATES = [
    {
        code: "T01", name: "Menu",
        role: "Une page qui mène à d'autres : des groupes de liens, puis « Plus » (les gestes), puis « Guide ».",
        structure: [["Groupes titrés", "C02 Section + C03 Rows + C04 NavRow (opens page)"], ["Plus", "C04 NavRow : ajouter, créer, un geste"], ["Guide", "C22 GuideSection"]],
        example: () => (
            <>
                <Section title="Sauvegardes en service"><Rows><NavRow id="tpl-t01-a" icon="bi-pc-display" tone="indigo" label="Ce serveur" value={fact("Nombre de jeux", "15 JEUX")} help="Les sauvegardes de vos parties." onOpen={noop} /></Rows></Section>
                <Section title="Plus"><Rows><NavRow id="tpl-t01-b" icon="bi-plus-lg" tone="indigo" label="Ajouter une ressource externe" help="Un partage réseau ou un service en ligne." onOpen={noop} /></Rows></Section>
            </>
        ),
    },
    {
        code: "T02", name: "Liste",
        role: "Une liste d'éléments, chacun vers sa fiche : un Listing (recherche, nombre, filtres, pages), puis « Plus », puis « Guide ».",
        structure: [["La liste", "C02 Section (titre de la liste) + C15 Listing (recherche, found, C16 filtres) + C04 NavRow (2 sous-titres au plus)"], ["Plus", "C04 NavRow : « Toute la console », « Ajouter »…"], ["Guide", "C22 GuideSection"]],
        example: () => <Section title="Consoles"><ListingExample /></Section>,
    },
    {
        code: "T03", name: "Fiche",
        role: "Un élément : « Détails » (des InfoRow, une icône sur chaque), « Plus » (pages et gestes, le lien vers la fiche liée en dernier), « Danger », « Guide ».",
        structure: [["Détails", "C05 InfoRow (clé : valeur) ou C17 GameFacts"], ["Plus", "C04 NavRow ; un geste en cours : pending ; en dernier, le lien vers la fiche liée"], ["Danger", "C14 DangerSection"], ["Guide", "C22 GuideSection"]],
        example: () => (
            <>
                <Section title="Détails"><GameFacts facts={{ console: "PlayStation 5", origin: "Gamebox", gameId: "PPSA10594", date: "7 nov. 2026, 22:57", sizeBytes: 230 * 1024 * 1024 }} /></Section>
                <Section title="Plus"><Rows><NavRow id="tpl-t03-b" icon="bi-archive" tone="indigo" label="Backup" help="Une copie mise de côté, dans vos backups." opens="confirm" onOpen={noop} /><NavRow id="tpl-t03-c" icon="bi-collection-play" tone="green" label="Voir sa fiche dans la bibliothèque" help="La fiche du jeu, là où on le lance." onOpen={noop} /></Rows></Section>
                <DangerSection id="tpl-t03-d" label="Supprimer" help="Sa sauvegarde part ; les backups restent." question="Supprimer ?" onConfirm={noop} />
            </>
        ),
    },
    {
        code: "T04", name: "Formulaire",
        role: "Saisir puis enregistrer : une section (son aide dit comment remplir), ses champs, la ligne du bouton ; enregistré, un toast et le retour à la page d'avant ; puis « Guide ».",
        structure: [["Le formulaire", "C02 Section (help) + C03 Rows + C10 TextRow (C26 IconAddon) / C27 AddressRow / C08 SelectRow / C25 ChoiceRow / C11 RadioRow / C29 FileRow + C12 SubmitRow"], ["Guide", "C22 GuideSection"]],
        example: () => <FormExample />,
    },
    {
        code: "T05", name: "Réglages immédiats",
        role: "Des réglages qui s'appliquent aussitôt, sans bouton : des groupes d'interrupteurs, de listes, de glissières ; puis « Guide ».",
        structure: [["Groupes titrés", "C02 Section + C03 Rows + C07 SwitchRow / C08 SelectRow / C09 RangeRow"], ["Guide", "C22 GuideSection"]],
        example: () => <><Section title="Affichage"><SwitchExample /></Section><Section title="Flux"><SelectExample /></Section></>,
    },
    {
        code: "T06", name: "Panneau latéral",
        role: "Ce qui s'ouvre sur le côté sans quitter la page : les filtres d'une liste (C16), l'engrenage d'un jeu. Son titre est celui d'une page, sans sous-titre.",
        structure: [["En-tête", "SidePanel : le rond de fermeture, le titre"], ["Groupes", "C16 FilterGroups, ou C23 QuickTiles + sections"]],
        example: () => <FilterGroups groups={[switchGroup({ key: "s", title: "Statut", id: "tpl-t06", hidden: [], onChange: noop, choices: [{ value: "a", label: "Jouable" }, { value: "b", label: "Non testé" }] })]} />,
    },
];

// LES PAGES DES RÉGLAGES ET LEUR GABARIT. `status` : conforme (vérifiée le 2026-11-23), à vérifier, ou à revoir (écart connu, `note`).
export const PAGES = [
    ["/settings", "Réglages", "T01", "à vérifier"],
    ["/settings/application", "Application", "T05", "à vérifier"],
    ["/settings/gamepads", "Manettes", "T05", "à vérifier"],
    ["/settings/gamepads/layout/:controller", "Disposition d'une manette", "T05", "à revoir", "Un éditeur à part, hors gabarit."],
    ["/settings/gamepads/connected", "Manettes connectées", "T02", "à vérifier"],
    ["/settings/federation/servers/:serverId/relayed", "Serveurs relayés d'un serveur rejoint", "T02", "à vérifier"],
    ["/settings/gamepads/connected/:key", "Une manette connectée", "T03", "à vérifier"],
    ["/settings/invite", "Inviter", "T03", "à vérifier"],
    ["/settings/catalog/games", "Catalogue", "T02", "conforme"],
    ["/settings/catalog/games/game", "Jeu du Catalogue", "T03", "conforme"],
    ["/settings/consoles", "Consoles", "T02", "conforme"],
    ["/settings/consoles/:id", "Une console", "T03", "à vérifier"],
    ["/settings/consoles/:id/games", "Jeux d'une console (sources)", "T01", "conforme"],
    ["/settings/consoles/:id/games/:source", "Jeux d'une source", "T02", "conforme"],
    ["/settings/consoles/:id/games/:source/game", "Un jeu (d'ici, d'un ami, d'une ressource)", "T03", "conforme"],
    ["/settings/consoles/:id/games/:source/patch", "Correctif d'un jeu", "T03", "à vérifier"],
    ["/settings/consoles/:id/games/:source/appearance", "Apparence d'un jeu", "T02", "à vérifier"],
    ["/settings/consoles/:id/games/:source/infos", "Corriger les informations", "T04", "à vérifier"],
    ["/settings/consoles/:id/params/:emulator", "Paramètres d'un émulateur", "T04", "à vérifier"],
    ["/settings/consoles/:id/cores/:name", "Un émulateur", "T03", "à vérifier"],
    ["/settings/consoles/:id/status", "Statut d'une console", "T04", "à vérifier"],
    ["/settings/consoles/:id/metadata-orphans", "Métadonnées sans jeu", "T02", "à vérifier"],
    ["/settings/remote-play", "Remote Play", "T02", "à vérifier"],
    ["/settings/remote-play/:deviceId", "Une PlayStation", "T03", "à vérifier"],
    ["/settings/remote-play/new", "Ajouter une PlayStation", "T04", "à vérifier"],
    ["/settings/metadata", "Fournisseur de métadonnées", "T01", "à vérifier"],
    ["/settings/metadata/:service", "Un service de métadonnées", "T04", "à vérifier"],
    ["/settings/federation", "Fédération", "T01", "conforme"],
    ["/settings/federation/access", "Accès délivrés", "T02", "à vérifier"],
    ["/settings/federation/tokens/:id", "Un accès délivré", "T03", "à vérifier"],
    ["/settings/federation/pending", "Accès en attente", "T02", "conforme"],
    ["/settings/federation/invitations/:id", "Une invitation", "T03", "à vérifier"],
    ["/settings/federation/servers", "Serveurs rejoints", "T02", "à vérifier"],
    ["/settings/federation/servers/:id", "Un serveur rejoint", "T03", "à vérifier"],
    ["/settings/federation/new-token", "Nouvel accès", "T04", "à vérifier"],
    ["/settings/federation/new-pairing", "Code d'appairage", "T04", "à vérifier"],
    ["/settings/federation/add", "Rejoindre un serveur", "T04", "à vérifier"],
    ["/settings/resources", "Ressources internes/externes", "T01", "conforme"],
    ["/settings/resources/disks/:name", "Un disque interne", "T02", "conforme"],
    ["/settings/resources/external/:id", "Une ressource externe", "T03", "conforme"],
    ["/settings/resources/external/:id/games", "Jeux d'une ressource", "T02", "conforme"],
    ["/settings/resources/external/:id/edit", "Modifier la connexion", "T04", "conforme"],
    ["/settings/resources/add", "Ajouter une ressource externe", "T01", "conforme"],
    ["/settings/resources/add/:kind", "Ajouter une connexion", "T04", "conforme"],
    ["/settings/members", "Membres", "T02", "conforme"],
    ["/settings/members/:id", "Un membre", "T03", "à vérifier"],
    ["/settings/members/new", "Nouveau membre", "T04", "à vérifier"],
    ["/settings/logs", "Journaux", "T05", "à vérifier"],
    ["/settings/logs/list", "Liste des journaux", "T02", "conforme"],
    ["/settings/logs/view/*", "Un journal", "T02", "à vérifier"],
    ["/settings/cache", "Cache", "T02", "conforme"],
    ["/settings/cache/:console/:name", "Une entrée du cache", "T03", "conforme"],
    ["/settings/rescan", "Tout rescanner", "T01", "à vérifier"],
    ["/settings/profile", "Profil", "T01", "à vérifier"],
    ["/settings/profile/password", "Mot de passe", "T04", "à vérifier"],
    ["/settings/profile/favorites", "Mes favoris", "T02", "à vérifier"],
    ["/settings/profile/saves", "Mes sauvegardes", "T01", "conforme"],
    ["/settings/profile/saves/:source", "Une source de sauvegardes", "T02", "conforme"],
    ["/settings/profile/saves/:source/console/:id", "Sauvegardes d'une console", "T02", "conforme"],
    ["/settings/profile/saves/:source/console/:id/game/:game", "Une sauvegarde", "T03", "conforme"],
    ["/settings/profile/saves/:source/console/:id/all", "Toute la console", "T02", "à revoir", "Pas de recherche : la page sert le geste de toute la console."],
    ["/settings/profile/saves/:source/game/:console/:ref", "Emplacements d'un jeu", "T02", "conforme"],
    ["/settings/profile/saves/:source/game/:console/:ref/slot/:slot", "Un emplacement", "T03", "conforme"],
    ["/settings/profile/saves/…/note", "La note d'un backup", "T04", "à vérifier"],
    ["/settings/profile/federation", "Fédération (profil)", "T01", "à vérifier"],
    ["/settings/profile/federation/connections", "Connexions avec mon jeton", "T02", "conforme"],
    ["/settings/profile/federation/connections/:key", "Une connexion", "T03", "conforme"],
    ["/settings/profile/federation/servers", "Mes comptes chez les serveurs fédérés", "T02", "à vérifier"],
    ["/settings/gameflex", "GameFlex", "T01", "à vérifier"],
    ["/settings/credits", "Crédits", "T02", "à vérifier"],
    ["/settings/download", "Application Android", "T01", "à vérifier"],
    ["/settings/logout", "Se déconnecter", "T01", "à vérifier"],
].map(([path, name, template, status, note = ""]) => ({ path, name, template, status, note }));

