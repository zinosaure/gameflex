import { useEffect, useRef, useState } from "react";
import { serverOrigin } from "./serverBase.js";
import { can } from "./capabilities.js";
import { federatedServerName } from "./federatedHost.js";
import { forgetFederationToken } from "./federationSession.js";
import { invalidateAuth } from "../profile/auth/auth-invalidation.js";
import {
    channelRefused, connectionLost, getTranslatedErrorMessage, serverUnreachable, sessionRefused,
} from "../shared/utils/error-code-translator.js";

// LE CENTRE DE MESSAGES — UN WebSocket par onglet et par kernel.
//
// La page ouvrait jusqu'à cinq canaux vers le même kernel (la liste des
// sessions, le mode « session » de la façade, le suivi d'un lancement, l'état
// de la partie), et lançait ou arrêtait par autant de requêtes HTTP. Le kernel
// sert maintenant UN canal, `/api/v1/facade/ws`, où le type du message dit à
// qui il s'adresse (voir `shared/facade` côté Go) :
//
//   - `subscribe {topic}` / `unsubscribe {topic}` — le kernel pousse tout de
//     suite ce qu'il sait, puis chaque changement, sous le même `topic` ;
//   - les commandes (`launch`, `session.stop`) — une réponse `reply` porte le
//     même `id`, le statut HTTP et le corps qu'aurait rendus l'ancienne route.
//
// CE QUI N'Y PASSE PAS : ce qui se décide DANS une session (rejoindre,
// demander ou céder la manette) appartient au child qui la tient, et la page
// le lui dit sur SON canal (`use-child-session-ws.js`). Le kernel surveille
// les childs ; il ne s'interpose pas entre eux et leurs joueurs.
//
// UNE CONNEXION PAR KERNEL, PARTAGÉE. La clé est l'origine ET le jeton : notre
// kernel avec notre jeton, un serveur fédéré avec celui du compte qu'on y a,
// l'appareil d'un hôte avec le jeton d'invité qu'il nous a donné. Deux usages
// qui visent le même kernel avec le même jeton partagent le même socket ; un
// jeton différent est une autre identité, donc une autre connexion.
//
// L'ADRESSE ET LE JETON SONT DONNÉS, JAMAIS LUS DANS UN ÉTAT GLOBAL. Le défaut
// qui a fait naître `homeOrigin` (2026-09-07) : un abonnement qui reconstruisait
// son URL à chaque reconnexion se rebranchait sur l'hôte d'une page fédérée
// ouverte entre-temps — avec notre jeton, que celui-ci refuse. Une connexion
// d'ici garde l'adresse qu'on lui a donnée en la créant.

// Reprise après une coupure : vite d'abord, puis sans s'acharner.
const RECONNECT_FIRST_MS = 1000;
const RECONNECT_MAX_MS = 10000;
// Le dernier usage relâché, on garde le socket un instant : React monte,
// démonte et remonte un composant en développement, et une page qui en
// remplace une autre reprend souvent les mêmes abonnements.
const RELEASE_GRACE_MS = 2000;

const channels = new Map();

function socketUrl(origin, token) {
    const base = origin
        ? origin.replace(/^http/, "ws")
        : `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}`;

    return `${base}/api/v1/facade/ws${token ? `?token=${encodeURIComponent(token)}` : ""}`;
}

// La réponse d'une commande, sous la forme que rendaient les appels HTTP
// qu'elle remplace (`api()` de spawnSessions.js) : le corps, plus `status` et
// `ok`. Les appelants lisent donc un 409 « tu joues déjà ailleurs » comme
// avant.
function asResult(frame) {
    const body = frame.data && typeof frame.data === "object" ? frame.data : {};
    const status = frame.status || 0;

    return {
        ...body,
        // UN REFUS CODÉ SE DIT DANS LA LANGUE DE LA PAGE (chantier 20) : le
        // kernel rend un code (`consoleNotGranted`), l'écran lit le message.
        ...(body.errorCode ? { errorMessage: getTranslatedErrorMessage(body) } : {}),
        status,
        ok: status >= 200 && status < 300 && body.success !== false,
        ...(frame.launchId ? { launchId: frame.launchId } : {}),
    };
}

// UNE COMMANDE SANS RÉPONSE DIT POURQUOI (le porteur, 2026-10-21).
//
// Elle rendait « Backend injoignable », quoi qu'il fût arrivé. Le S25 lançait
// ainsi chez « Gameflex Private » avec un compte expiré : ce serveur refusait
// le canal (401) toutes les 10 s, et la page n'en disait pas plus. Un
// navigateur ne montre pas le statut d'une poignée de main WebSocket refusée :
// un canal qui ne s'ouvre pas le demande donc à côté, à `/api/v1/auth/me`
// (ouverte aux autres origines, crossOriginAuthPaths). Pas de réponse : le
// serveur est muet. 401 : le jeton est refusé — le canal cesse de se rouvrir
// avec, et l'écran redemande la connexion. Sinon : le canal seul ne passe pas.
// Un canal qui s'était ouvert, lui, s'est coupé en route.

class KernelChannel {
    constructor(key, origin, token) {
        this.key = key;
        this.origin = origin;
        this.token = token;
        this.socket = null;
        this.isOpen = false;
        this.closed = false;
        this.retryMs = RECONNECT_FIRST_MS;
        this.retryTimer = null;
        this.releaseTimer = null;
        this.users = 0;
        this.nextId = 1;
        // topic → { params, handlers:Set }
        this.topics = new Map();
        // id → { resolve, onProgress }
        this.pending = new Map();
        // Les commandes posées avant l'ouverture partent à l'ouverture.
        this.outbox = [];
        this.stateListeners = new Set();
        // Un canal qui ne s'ouvre pas se diagnostique une fois — pas à chaque
        // nouvel essai, toutes les 10 s ; et à chaque commande restée sans
        // réponse.
        this.diagnosed = false;
        // Pourquoi il est fermé pour de bon, quand c'est un refus.
        this.refusal = null;
        this.connect();
    }

    connect() {
        if (this.closed) {
            return;
        }

        const socket = new WebSocket(socketUrl(this.origin, this.token));
        this.socket = socket;

        socket.onopen = () => {
            if (this.socket !== socket) {
                return;
            }
            this.isOpen = true;
            this.diagnosed = false;
            this.retryMs = RECONNECT_FIRST_MS;
            // RIEN NE SE RATTRAPE, TOUT SE RENVOIE : chaque abonnement repart
            // d'un instantané frais.
            for (const [topic, entry] of this.topics) {
                this.write({ type: "subscribe", data: { topic, ...(entry.params || {}) } });
            }
            for (const message of this.outbox.splice(0)) {
                this.write(message);
            }
            this.stateListeners.forEach((listener) => listener(true));
        };

        socket.onmessage = (event) => {
            let frame;
            try {
                frame = JSON.parse(event.data);
            } catch {
                return;
            }
            this.route(frame);
        };

        socket.onclose = () => {
            if (this.socket !== socket) {
                return;
            }
            this.socket = null;
            const wasOpen = this.isOpen;
            this.isOpen = false;
            const pending = [...this.pending.values()];
            this.pending.clear();
            if (wasOpen) {
                // Une commande en vol n'aura pas de réponse : le dire plutôt
                // que la laisser attendre. Elle a pu s'exécuter — un lancement,
                // par exemple, va au bout côté kernel — et la liste des
                // sessions le montrera.
                pending.forEach((entry) => entry.resolve(connectionLost()));
            } else if (pending.length > 0 || !this.diagnosed) {
                this.diagnosed = true;
                this.diagnose().then((result) => pending.forEach((entry) => entry.resolve(result)));
            }
            // ET ELLE NE PART PLUS. Une commande annoncée « injoignable » à son
            // appelant restait dans la file d'attente, et partait à la
            // prochaine ouverture : trois lancements d'un coup, des minutes
            // plus tard, le jour où le relais a enfin laissé passer les
            // WebSockets — trois parties pour un seul joueur (2026-10-09).
            // L'appelant a eu sa réponse ; s'il veut réessayer, il réessaiera.
            this.outbox.length = 0;
            if (wasOpen) {
                this.stateListeners.forEach((listener) => listener(false));
            }
            if (!this.closed) {
                this.retryTimer = setTimeout(() => this.connect(), this.retryMs);
                this.retryMs = Math.min(this.retryMs * 2, RECONNECT_MAX_MS);
            }
        };
    }

    write(message) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(message));

            return true;
        }

        return false;
    }

    route(frame) {
        if (frame.type === "reply") {
            const entry = this.pending.get(frame.id);
            if (entry) {
                this.pending.delete(frame.id);
                entry.resolve(asResult(frame));
            }

            return;
        }
        if (frame.type === "progress") {
            // La trame entière suit l'étape : elle porte l'identifiant du
            // lancement, que le premier message donne seul (chantier 35).
            this.pending.get(frame.id)?.onProgress?.(frame.data, frame);

            return;
        }
        if (frame.topic) {
            const entry = this.topics.get(frame.topic);
            if (entry) {
                entry.last = { data: frame.data, frame };
                entry.handlers.forEach((handler) => handler(frame.data, frame));
            }
        }
    }

    // subscribe — un abonnement, partagé entre ceux qui veulent le même
    // sujet. Rend de quoi s'en défaire.
    subscribe(topic, params, handler) {
        let entry = this.topics.get(topic);
        if (!entry) {
            entry = { params, handlers: new Set(), last: null };
            this.topics.set(topic, entry);
            this.write({ type: "subscribe", data: { topic, ...(params || {}) } });
        } else if (entry.last) {
            // UN ABONNÉ TARDIF REÇOIT L'ÉTAT DU MOMENT. Un sujet ne pousse
            // qu'au changement : qui rejoignait un abonnement déjà ouvert
            // attendait le prochain changement, qui pouvait ne jamais venir.
            // La page de jeu d'une partie fédérée arrivait ainsi juste après la
            // fiche du jeu, sur le même canal, rataient la liste qui contenait
            // sa session — et concluait « introuvable » au bout de cinq
            // secondes (2026-10-09, un serveur lançant chez un autre).
            const { data, frame } = entry.last;
            queueMicrotask(() => {
                if (entry.handlers.has(handler)) {
                    handler(data, frame);
                }
            });
        }
        entry.handlers.add(handler);

        return () => {
            const current = this.topics.get(topic);
            if (!current) {
                return;
            }
            current.handlers.delete(handler);
            if (current.handlers.size === 0) {
                this.topics.delete(topic);
                this.write({ type: "unsubscribe", data: { topic } });
            }
        };
    }

    // request — une commande. Rend une promesse de sa réponse ; `onProgress`
    // reçoit les étapes d'un lancement à mesure qu'elles arrivent.
    request(type, data, { onProgress } = {}) {
        if (this.closed) {
            return Promise.resolve(this.refusal || connectionLost());
        }
        const id = String(this.nextId++);

        return new Promise((resolve) => {
            this.pending.set(id, { resolve, onProgress });
            const message = { type, id, data };
            if (!this.write(message)) {
                this.outbox.push(message);
            }
        });
    }

    // onState — `true` à l'ouverture, `false` à la coupure. Appelé tout de
    // suite avec l'état présent.
    onState(listener) {
        this.stateListeners.add(listener);
        listener(this.isOpen);

        return () => this.stateListeners.delete(listener);
    }

    retain() {
        this.users += 1;
        clearTimeout(this.releaseTimer);
    }

    release() {
        this.users -= 1;
        if (this.users > 0) {
            return;
        }
        clearTimeout(this.releaseTimer);
        this.releaseTimer = setTimeout(() => {
            if (this.users > 0) {
                return;
            }
            for (const [, entry] of this.pending) {
                entry.resolve(connectionLost());
            }
            this.pending.clear();
            this.stop(null);
        }, RELEASE_GRACE_MS);
    }

    // Fermer pour de bon : plus de reconnexion. `refusal` dit pourquoi aux
    // commandes qui arriveraient encore.
    stop(refusal) {
        this.refusal = refusal;
        this.closed = true;
        clearTimeout(this.retryTimer);
        if (channels.get(this.key) === this) {
            channels.delete(this.key);
        }
        this.socket?.close(1000);
        this.socket = null;
    }

    // Pourquoi le canal ne s'ouvre pas — voir « UNE COMMANDE SANS RÉPONSE DIT
    // POURQUOI », en tête de fichier.
    async diagnose() {
        const own = !this.origin || this.origin === serverOrigin();
        const server = own ? "" : federatedServerName(this.origin);
        let status;
        try {
            const response = await fetch(`${this.origin}/api/v1/auth/me`, {
                headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
                cache: "no-store",
            });
            status = response.status;
        } catch {
            return serverUnreachable(server);
        }
        if (this.token && status === 401) {
            const refused = sessionRefused(server);
            this.stop(refused);
            if (own) {
                invalidateAuth("401 on the facade channel");
            } else {
                forgetFederationToken(this.token);
            }

            return refused;
        }

        return channelRefused(server);
    }
}

// kernelChannel — LA connexion vers ce kernel, avec ce jeton.
//
// `origin` vide : la même origine que la page. `token` vide : la page de
// l'appareil lui-même, sur un téléphone sans comptes — le seul cas où un
// kernel accepte un canal sans jeton.
export function kernelChannel({ origin = "", token = "" } = {}) {
    const key = `${origin}|${token}`;
    let channel = channels.get(key);
    if (!channel) {
        channel = new KernelChannel(key, origin, token);
        channels.set(key, channel);
    }

    return channel;
}

// kernelRequest — une commande, envoyée sur la connexion de ce kernel le
// temps qu'elle dure. Pour les appelants qui ne sont pas des composants (les
// modules d'API) : la connexion est gardée ouverte jusqu'à la réponse, puis
// rendue.
export async function kernelRequest(target, type, data, options) {
    const channel = kernelChannel(target);
    channel.retain();
    try {
        return await channel.request(type, data, options);
    } finally {
        channel.release();
    }
}

// useKernelChannel — la connexion vers un kernel, tant que le composant vit.
// `target` nul : aucune (pas de jeton là où il en faut un, par exemple).
export function useKernelChannel(target) {
    const origin = target?.origin ?? "";
    const token = target?.token ?? "";
    const enabled = Boolean(target);
    const [channel, setChannel] = useState(null);

    useEffect(() => {
        if (!enabled) {
            setChannel(null);

            return undefined;
        }
        const next = kernelChannel({ origin, token });
        next.retain();
        setChannel(next);

        return () => next.release();
    }, [enabled, origin, token]);

    return channel;
}

// useKernelTopic — un abonnement, tant que le composant vit et que `topic`
// est donné. `onData` peut changer à chaque rendu : on lit toujours le
// dernier, sans se réabonner.
export function useKernelTopic(channel, topic, params, onData) {
    const onDataRef = useRef(onData);
    onDataRef.current = onData;
    const paramsKey = params ? JSON.stringify(params) : "";

    useEffect(() => {
        if (!channel || !topic) {
            return undefined;
        }

        return channel.subscribe(topic, paramsKey ? JSON.parse(paramsKey) : null, (data, frame) => {
            onDataRef.current?.(data, frame);
        });
    }, [channel, topic, paramsKey]);
}


// kernelSnapshot — ce qu'un sujet dit MAINTENANT, une fois : on s'abonne, on
// prend le premier instantané, on se désabonne. Pour une question ponctuelle
// (« ai-je déjà une session de test ? ») là où un abonnement durable n'aurait
// personne pour l'écouter. Rend `null` si le kernel ne répond pas.
export function kernelSnapshot(target, topic, timeoutMs = 10000) {
    const channel = kernelChannel(target);
    channel.retain();

    return new Promise((resolve) => {
        let settled = false;
        let unsubscribe = () => {};
        const settle = (value) => {
            if (settled) {
                return;
            }
            settled = true;
            clearTimeout(timer);
            unsubscribe();
            channel.release();
            resolve(value);
        };
        const timer = setTimeout(() => settle(null), timeoutMs);
        unsubscribe = channel.subscribe(topic, null, (data) => settle(data));
    });
}

// kernelTarget — à quel kernel parler, et avec quel jeton : un hôte quand on
// le vise (`host` porte son adresse ET le jeton du compte qu'on y a — les deux
// n'ont de sens qu'ensemble), sinon celui que cette page vise
// (`serverOrigin`, qui suit la surcharge d'une page de streaming fédérée).
export function kernelTarget(host, token) {
    if (host?.origin) {
        return { origin: host.origin, token: host.token || "" };
    }

    return { origin: serverOrigin(), token: token || "" };
}

// kernelReachable — peut-on ouvrir un canal avec ce jeton ? Toujours quand il
// y en a un ; sans, seulement la page d'un appareil sans comptes, que son
// propre kernel reconnaît (boucle locale, son origine). Une seule règle, ici,
// plutôt qu'un `token &&` recopié à chaque appel — qui fermait le canal du
// téléphone sans comptes.
export function kernelReachable(token) {
    return Boolean(token) || !can("accounts");
}
