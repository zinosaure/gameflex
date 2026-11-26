import React from "react";
import { useTranslation } from "react-i18next";
import { AddressRow } from "../../bricks/AddressRow.jsx";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { formContext, Ruleset } from "../../ui/Forms.jsx";

// LES RESSOURCES EXTERNES (Samba, WebDAV, kDrive, FTP, SFTP) : leurs genres et le formulaire d'une connexion, partagés
// par gameflex-federation et par Réglages › Ressources de GameFlex, où ce service s'intègre (le porteur, 2026-11-20 :
// « Un serveur Gameflex peut faire office de serveur fédéré seul » — l'interface d'abord, `#808`).

// Les genres, dans l'ordre que l'on montre : par leur nom, dans la langue de la page.
export const KINDS = [
    { kind: "smb", icon: "bi-hdd-network", tone: "indigo" },
    { kind: "webdav", icon: "bi-cloud", tone: "sky" },
    { kind: "kdrive", icon: "bi-cloud-fill", tone: "teal" },
    { kind: "ftp", icon: "bi-folder-symlink", tone: "amber" },
    { kind: "sftp", icon: "bi-shield-lock", tone: "violet" },
];
export const sortedKinds = (t, language) => [...KINDS].sort((a, b) => t(`fedService.kinds.${a.kind}.label`).localeCompare(t(`fedService.kinds.${b.kind}.label`), language));

// L'ADRESSE EN TROIS PIÈCES (le porteur, 2026-11-20 : « pour les address, tu peux utiliser des input group. Et flex les
// 2 inputs sur la même ligne comme : [protocol | input field] [port num] ») : le protocole devant (WebDAV le choisit,
// https ou http), le serveur — suivi du partage pour Samba, d'un chemin pour WebDAV —, et le port à part, vide pour
// celui du protocole. kDrive n'a qu'un numéro. L'adresse que le service reçoit se recompose à l'envoi (composeAddress).
export const SCHEMES = { smb: ["smb"], webdav: ["https", "http"], ftp: ["ftp"], sftp: ["sftp"] };
export const DEFAULT_PORTS = { smb: "445", https: "443", http: "80", ftp: "21", sftp: "22" };
export const emptyResource = (kind) => ({ name: "", scheme: (SCHEMES[kind] || [""])[0], host: "", port: "", folder: "", access: "account", username: "", password: "" });

// Le serveur et ce qui le suit : `nas/jeux` → ["nas", "/jeux"] ; une IPv6 garde ses crochets.
const splitHost = (value) => {
    const found = /^(\[[^\]]*\]|[^/]*)(.*)$/.exec(value);

    return [found[1], found[2]];
};

export const composeAddress = (kind, values) => {
    if (kind === "kdrive") {
        return values.host.trim();
    }
    const [server, rest] = splitHost(values.host.trim());

    return `${values.scheme}://${server}${values.port.trim() ? `:${values.port.trim()}` : ""}${rest}`;
};

// Une adresse enregistrée, rendue en pièces pour la modifier (la page d'une connexion de gameflex-federation).
export const splitAddress = (kind, address) => {
    if (kind === "kdrive") {
        return { scheme: "", host: address || "", port: "" };
    }
    const found = /^([a-z]+):\/\/(\[[^\]]*\]|[^/:?#]*)(?::(\d*))?(.*)$/i.exec((address || "").trim());

    return found ? { scheme: found[1].toLowerCase(), host: `${found[2]}${found[4]}`, port: found[3] || "" } : { scheme: SCHEMES[kind][0], host: address || "", port: "" };
};

// LE BON TYPAGE (le porteur, 2026-11-20 : « les vérifications skeleton.Ruleset du bon typage d'adresse/port ») — ce que
// le service accepte déjà (federation/src/internal/sources) : un nom de serveur ou une adresse IP (IPv6 entre crochets),
// un partage pour Samba, ni chemin pour FTP et SFTP (le dossier a son champ), des chiffres pour kDrive ; un port de 1 à
// 65535. Rend la clé du message, ou null ; un champ vide n'est pas faux, il manque (le bouton reste grisé).
const HOST = /^(?:\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)$/;
const validHost = (host) => {
    if (!HOST.test(host)) {
        return false;
    }
    // Que des chiffres et des points : une adresse IPv4, quatre nombres de 0 à 255.
    if (/^[\d.]+$/.test(host)) {
        const parts = host.split(".");

        return parts.length === 4 && parts.every((part) => part !== "" && Number(part) <= 255);
    }

    return true;
};

export function hostProblem(kind, host) {
    const value = (host || "").trim();
    if (!value) {
        return null;
    }
    if (kind === "kdrive") {
        return /^\d+$/.test(value) ? null : "resources.external.rules.kdrive";
    }
    if (value.includes("://")) {
        return "resources.external.rules.noScheme";
    }
    const [server, rest] = splitHost(value);
    if (/^:/.test(rest) || (!server.startsWith("[") && server.includes(":"))) {
        return "resources.external.rules.portApart";
    }
    if (!validHost(server)) {
        return "resources.external.rules.host";
    }
    if (kind === "smb" && !rest.replace(/^\/+/, "").split("/")[0]) {
        return "resources.external.rules.share";
    }
    if ((kind === "ftp" || kind === "sftp") && rest.replace(/\/+$/, "") !== "") {
        return "resources.external.rules.noPath";
    }

    return null;
}

export function portProblem(port) {
    const value = (port || "").trim();

    return !value || (/^\d{1,5}$/.test(value) && Number(value) >= 1 && Number(value) <= 65535) ? null : "resources.external.rules.port";
}

// ANONYME OU AVEC UN COMPTE (le porteur, 2026-11-20 : « mon serveur SFTP peut être en local et j'ai pas besoin de mot de
// passe ») — un choix segmenté, sauf kDrive (un jeton seul). Anonyme, la connexion part sans nom ni mot de passe : le
// service présente ce que chaque protocole attend d'un anonyme.
export const anonymousAllowed = (kind) => kind !== "kdrive";
export const accessOf = (resource) => (anonymousAllowed(resource.kind) && !resource.username ? "anonymous" : "account");
// Ce que le service reçoit : l'adresse recomposée, et ni nom ni mot de passe pour une connexion anonyme.
export const sentValues = (values, kind) => {
    const { scheme, host, port, ...rest } = values;
    const sent = { ...rest, address: composeAddress(kind, values) };

    return anonymousAllowed(kind) && values.access === "anonymous" ? { ...sent, username: "", password: "" } : sent;
};
// Les champs obligatoires remplis (le nom, le serveur, le compte s'il y en a un) : le `isReplaced` du bouton. Une règle qui
// échoue le grise par `Ruleset.hasFeedbacks(form)`, comme l'exemple de Skeleton et Profil.
export const resourceReady = (values, kind) => Boolean(values.name.trim() && values.host.trim()
    && (!anonymousAllowed(kind) || values.access === "anonymous" || values.username.trim()));

// Les champs d'une connexion, selon son genre : l'adresse, le dossier et le compte n'y disent pas la même chose.
export function ResourceFields({ kind, form, values, setValues, editing = false }) {
    const { t } = useTranslation();
    const key = (field) => `fedService.kinds.${kind}.${field}`;
    const withAccount = !anonymousAllowed(kind) || values.access === "account";

    return (
        <>
            <TextRow id="fr-name" label={t("fedService.fields.name")} help={t("fedService.fields.nameHelp")} context={form} />
            {kind === "kdrive"
                ? <TextRow id="fr-host" label={t(key("address"))} help={t(key("addressHelp"))} context={form} props={{ inputMode: "numeric", "data-key": "fr-host" }} />
                : <AddressRow id="fr-host" portId="fr-port" label={t(key("address"))} help={t(key("addressHelp"))} context={form} schemes={SCHEMES[kind]}
                    scheme={values.scheme} onScheme={(scheme) => setValues((previous) => ({ ...previous, scheme }))}
                    portLabel={t("resources.external.port")} portPlaceholder={DEFAULT_PORTS[values.scheme] || ""} />}
            <TextRow id="fr-folder" label={t("fedService.fields.folder")} help={t("fedService.fields.folderHelp")} context={form} />
            {anonymousAllowed(kind) && (
                <ChoiceRow id="fr-access" label={t("fedService.fields.access")} help={t("fedService.fields.accessHelp")} value={values.access}
                    options={[["anonymous", t("fedService.fields.anonymous")], ["account", t("fedService.fields.withAccount")]]}
                    onChange={(access) => setValues((previous) => ({ ...previous, access }))} />
            )}
            {kind !== "kdrive" && withAccount && <TextRow id="fr-username" label={t("fedService.fields.username")} help={t(key("usernameHelp"))} context={form} />}
            {withAccount && <TextRow id="fr-password" kind="password" label={t(kind === "kdrive" ? "fedService.kinds.kdrive.token" : "fedService.fields.password")}
                help={editing ? t("fedService.fields.passwordKeep") : t(key("passwordHelp"))} context={form} />}
        </>
    );
}

// Les champs d'un formulaire de connexion portent le nom de leur clé : TextRow prend l'identifiant pour nom.
// `kind` et `t` : les règles du serveur et du port (hostProblem, portProblem), montrées sur leur champ.
export const resourceForm = (values, setValues, kind, t) => {
    const shown = (problem) => (problem ? Ruleset.isInvalid(t(problem)) : null);
    const base = formContext(values, setValues, {
        "fr-host": (data) => shown(hostProblem(kind, data["fr-host"])),
        "fr-port": (data) => shown(portProblem(data["fr-port"])),
    });
    const names = { "fr-name": "name", "fr-host": "host", "fr-port": "port", "fr-folder": "folder", "fr-username": "username", "fr-password": "password" };

    return {
        ...base,
        values: Object.fromEntries(Object.entries(names).map(([id, field]) => [id, values[field]])),
        setValues: (patch) => setValues((previous) => {
            const next = { ...previous };
            for (const [id, value] of Object.entries(patch)) {
                next[names[id] || id] = value;
            }

            return next;
        }),
    };
};
