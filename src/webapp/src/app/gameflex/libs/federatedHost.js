// Où se joue une chose, et avec quoi s'y présenter — chantier 9, C9-P4.
//
// Une console fédérée et une session fédérée ont le même problème : elles ne
// vivent pas ici. Pour les toucher il faut deux choses, et deux seulement —
// l'ADRESSE du serveur qui les tient, et une SESSION chez lui (celle que notre
// kernel échange contre le jeton qu'il nous a prêté, chantier 20).
//
// LA RÈGLE DE CE FICHIER : l'hôte se résout par son IDENTIFIANT, jamais par
// une adresse qu'on nous tend. Une URL de streaming peut être fabriquée par
// n'importe qui ; si la page acceptait l'adresse qui s'y trouve, elle
// enverrait le jeton de compte du serveur 4 à l'adresse écrite à côté, et une
// adresse forgée n'aurait plus qu'à le ramasser. L'identifiant, lui, ne se
// résout que par notre propre kernel, qui seul tient le registre.
//
// C'est la même discipline que pour les jeux fédérés (`srv<id>:<référence>`,
// chantier 6) : le préfixe dit CHEZ QUI, et rien de plus. Ce qu'il faut pour
// y aller se demande, il ne se transporte pas.
import { apiUrl } from "./serverBase.js";
import { federationSession } from "./federationSession.js";
import { getStoredToken } from "../profile/auth/AuthContext.jsx";

// Le préfixe qui dit « ceci vit chez le serveur numéro N ».
//
// UNE SEULE ÉCRITURE DEPUIS LE 2026-09-17 : `srv<id>:<référence>`, élue forme
// canonique (#163). Les deux kernels l'écrivent maintenant — celui du serveur
// l'a toujours fait (federation_catalog.go), et celui du téléphone est passé
// de `srv:<id>:` à celle-ci (federatedReference, mobilekernel.go), où elle
// était déjà employée pour les appareils fédérés.
//
// Deux écritures, c'était deux vérités : chaque hôte ne savait défaire que la
// sienne (le parseur du serveur refuse `srv:3:x`, son premier « : » étant à
// l'index 0 ; celui du téléphone refusait `srv3:x`), et un lecteur de la page
// avait fini par ne reconnaître que celle du téléphone — donc à rendre `null`
// sur chaque référence de notre propre serveur.
//
// CE LECTEUR RESTE INDULGENT MALGRÉ TOUT, et le `:?` ci-dessous n'est pas un
// oubli : un historique déjà rangé sur un appareil porte
// l'ancienne forme. On LIT les deux, on n'en ÉCRIT qu'une (federatedId, plus
// bas) — le lecteur est indulgent, l'écrivain ne l'est pas.
//
// Ce que coûtait l'inverse, vécu le 2026-09-07 : sur le téléphone, la fiche
// d'un jeu Xbox d'un serveur fédéré n'offrait que « télécharger », sans jamais
// proposer de le lancer là-bas, parce que l'expression ne reconnaissait pas sa
// référence.
const PREFIX = /^srv:?(\d+):(.+)$/;

/**
 * Défaire `srv4:quelque-chose`. Rend null pour tout le reste, ce qui est le
 * cas normal et pas une erreur : la plupart des identifiants sont d'ici.
 */
export function parseFederatedId(value) {
    const found = PREFIX.exec(String(value || ""));
    if (!found) {
        return null;
    }

    return { serverId: Number(found[1]), id: found[2] };
}

/** Refaire l'identifiant préfixé — un seul endroit sait comment il s'écrit. */
export function federatedId(serverId, id) {
    return `srv${serverId}:${id}`;
}

// Le registre, gardé en mémoire. Il change quand on ajoute un serveur, ce qui
// arrive une fois par mois ; le redemander à chaque appel ferait un
// aller-retour avant chaque bouton.
let directory = null;

/**
 * Les serveurs fédérés enregistrés — `[{id, link, hostname, listing}]`, où
 * `listing` dit ce que la copie de son listing dit de lui (sa date, sa
 * version, sa machine, les consoles que le compte de notre jeton peut lancer
 * chez lui).
 *
 * La liste ENTIÈRE, pas seulement celui d'une chose donnée : une fiche de jeu
 * propose d'émuler chez n'importe lequel d'entre eux (C9-P5), et c'est
 * ensuite la disponibilité du jeu là-bas qui décide si on peut lancer.
 *
 * `fresh` la relit au lieu de la garder : la fiche d'un jeu fédéré décide de
 * Play sur ce que le pair nous ouvre, et une console accordée entre-temps doit
 * s'y voir (chantier 15).
 */
export async function federatedServers({ fresh = false } = {}) {
    if (fresh) {
        directory = null;
    }

    return serverDirectory();
}

/**
 * CE serveur, vérifié chez lui à l'instant (chantier 15) — `{id, link,
 * hostname, listing, online}` : les consoles que le compte de notre jeton peut
 * y lancer (`listing.consoles`) et s'il répond. La fiche d'un de ses jeux décide de Play
 * là-dessus ; la liste ci-dessus, elle, ne demande rien à personne. Null quand
 * notre propre kernel ne répond pas.
 */
export async function federatedServerNow(serverId) {
    const token = getStoredToken();
    try {
        const response = await fetch(apiUrl(`/api/v1/servers/${encodeURIComponent(serverId)}`), {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (!response.ok) {
            return null;
        }

        return (await response.json())?.data || null;
    } catch {
        return null;
    }
}

async function serverDirectory() {
    // UNE LISTE VIDE NE SE MET PAS EN CACHE.
    //
    // `if (directory)` suffisait, et `[]` est VRAI en JavaScript : un écran qui
    // interrogeait cette route avant qu'un pair ne soit enregistré figeait le
    // vide pour toute la vie de la page. Le sélecteur de source des écrans
    // d'administration (#148) restait alors masqué, et il fallait recharger
    // pour le voir apparaître — signalé le 2026-09-17, après l'ajout d'un pair
    // sur la pile publique.
    //
    // Redemander quand on n'a rien ne coûte rien : la route est locale, et
    // c'est notre propre kernel qui répond.
    if (directory && directory.length > 0) {
        return directory;
    }

    const token = getStoredToken();
    try {
        const response = await fetch(apiUrl("/api/v1/servers"), {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (!response.ok) {
            return [];
        }
        directory = (await response.json())?.data || [];
    } catch {
        // Notre propre kernel ne répond pas : il n'y a rien à faire depuis
        // cette page, et rendre une liste vide donne le même écran qu'un
        // appareil sans amis.
        return [];
    }

    return directory;
}

/**
 * Le nom d'un serveur fédéré, pour le dire dans un message (« Gameflex Private
 * ne répond pas ») : celui du registre quand il est déjà lu, sinon l'adresse.
 */
export function federatedServerName(origin) {
    const base = String(origin || "").replace(/\/+$/, "");
    const entry = (directory || []).find((server) => String(server.link || "").replace(/\/+$/, "") === base);
    if (entry?.hostname) {
        return entry.hostname;
    }
    try {
        return new URL(base).host;
    } catch {
        return base;
    }
}

/**
 * Tout ce qu'il faut pour parler à l'hôte d'une chose fédérée.
 *
 * Rend `{serverId, link, hostname, token, username, id, refusal}` — ou null
 * quand la chose est d'ici (le cas normal). `token` est vide quand l'hôte ne
 * nous ouvre pas de session, et `refusal` dit pourquoi (un code que la page
 * traduit, voir federationSession) : on y regarde, sans y jouer.
 */
export async function federatedHostOf(value) {
    const parsed = parseFederatedId(value);
    if (!parsed) {
        return null;
    }

    const server = (await serverDirectory()).find((entry) => entry.id === parsed.serverId);
    if (!server) {
        // Le serveur a été retiré du registre pendant qu'un lien traînait.
        // Rien à joindre, et surtout pas l'adresse que le lien proposerait.
        return null;
    }
    const opened = await federationSession(parsed.serverId);

    return {
        serverId: parsed.serverId,
        link: server.link,
        hostname: server.hostname,
        token: opened.ok ? opened.session.token : "",
        username: opened.ok ? opened.session.username : "",
        id: parsed.id,
        refusal: opened.ok ? null : opened.code,
    };
}
