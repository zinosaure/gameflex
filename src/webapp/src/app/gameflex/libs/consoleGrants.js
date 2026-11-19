// CE QU'UN COMPTE PEUT LANCER (chantier 20) — la même règle que
// `gameflex/consoles` (grants.go), lue par la page pour ne proposer que ce qui
// partira. Le serveur, lui, refuse de toute façon un lancement qui n'est pas
// accordé (`consoleNotGranted`) : cette copie ne décide de rien, elle évite
// seulement d'offrir un bouton qui échouerait.
//
// Un droit s'écrit `*` (tout), `retro` (toutes les consoles émulées par un
// cœur), ou l'identifiant d'une console (`ps3`, `rp-ps5`). Aucun : le
// catalogue seulement — voir et télécharger, sans jouer.

export const GRANT_ALL = "*";
export const GRANT_RETRO = "retro";

/**
 * Ces droits ouvrent-ils cette console ? `console` est l'entrée du registre
 * (`useConsoles()`), pour savoir si elle est du rétro.
 */
export function allows(grants, console) {
    if (!Array.isArray(grants) || !console) {
        return false;
    }

    return grants.some((grant) => grant === GRANT_ALL
        || grant === console.id
        || (grant === GRANT_RETRO && console.method === "retrogaming"));
}

/** Ces droits ouvrent-ils au moins une console ? */
export function allowsAny(grants) {
    return Array.isArray(grants) && grants.length > 0;
}

/**
 * Ces droits, en clair — « toutes les consoles », « tout le rétro », le nom
 * de chaque console. `consoles` est le registre (`useConsoles()`).
 */
export function describeGrants(grants, consoles, t) {
    const granted = Array.isArray(grants) ? grants : [];
    if (granted.includes(GRANT_ALL)) {
        return t("common.consoles.all");
    }
    if (granted.length === 0) {
        return t("common.consoles.none");
    }

    return granted.map((id) => {
        if (id === GRANT_RETRO) {
            return t("common.consoles.retro");
        }
        const found = (consoles || []).find((c) => c.id === id);
        if (!found) {
            return id;
        }

        return found.method === "remoteplay" ? t("common.consoles.remotePlay", { name: found.name }) : found.name;
    }).join(" · ");
}
