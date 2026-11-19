import { fileURLToPath } from 'url';

import fs from 'fs';
import path from 'path';
import webpack from 'webpack';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import CopyWebpackPlugin from 'copy-webpack-plugin';

const FILENAME = fileURLToPath(import.meta.url);
const DIRNAME = path.dirname(FILENAME);

// LA VERSION ET LE COMMIT DU PIED DE PAGE, LUS DANS LE DÉPÔT À CHAQUE
// COMPILATION (2026-10-24).
//
// Ils venaient de l'environnement du conteneur, figé à son démarrage : la
// webapp de dev, lancée le 2026-10-22, affichait encore « v0.2.0 (0f889a5e) »
// deux versions plus tard — et le bundle de production qu'elle construit pour
// l'APK aussi, jusque dans la Release 0.4.0. Le dépôt est monté en lecture
// seule sur /repo (docker-compose.yaml). L'image de production
// (microservices/edge) n'en a pas : elle garde ses arguments de build.
const REPO = '/repo';

function readRepo(file) {
    try {
        return fs.readFileSync(path.join(REPO, file), 'utf8').trim();
    } catch {
        return '';
    }
}

// Le commit de HEAD, sans git (l'image n'en a pas) : la référence de HEAD, en
// fichier ou dans packed-refs ; ou HEAD lui-même, détaché.
function repoCommit() {
    const head = readRepo('.git/HEAD');
    if (!head.startsWith('ref: ')) {
        return head.slice(0, 8);
    }
    const ref = head.slice('ref: '.length);
    const loose = readRepo(path.join('.git', ref));
    if (loose) {
        return loose.slice(0, 8);
    }
    const packed = readRepo('.git/packed-refs').split('\n').find((line) => line.endsWith(' ' + ref));

    return packed ? packed.slice(0, 8) : '';
}

function buildInfo() {
    return {
        version: readRepo('VERSION') || process.env.GAMEFLEX_VERSION || '0.0.0',
        commit: repoCommit() || process.env.GITHUB_COMMIT_SHA || 'dev',
        builtAt: new Date().toISOString().slice(0, 16).replace('T', ' '),
    };
}

// `build-info.json`, à côté du bundle : ce que le build de l'APK compare à la
// version qu'il fabrique (src/mobileapp/docker-entrypoint.sh).
const emitBuildInfo = {
    apply(compiler) {
        compiler.hooks.thisCompilation.tap('BuildInfo', (compilation) => {
            compilation.hooks.processAssets.tap(
                { name: 'BuildInfo', stage: webpack.Compilation.PROCESS_ASSETS_STAGE_ADDITIONAL },
                () => compilation.emitAsset('build-info.json',
                    new webpack.sources.RawSource(JSON.stringify(buildInfo()) + '\n')),
            );
        });
    },
};


// UNE FONCTION, et plus un objet (2026-09-15) : webpack-cli ne dit qu'à une
// fonction s'il sert la page (`env.WEBPACK_SERVE`) ou s'il la construit. Le
// relais de l'API n'existe qu'au premier cas — la construction de l'image de
// production ne connaît aucune adresse de service, et ne doit pas en exiger.
export default (env = {}) => ({
    mode: process.env.NODE_ENV || 'development',
    entry: './app/index.js',
    output: {
        path: path.resolve(DIRNAME, 'dist'),
        filename: 'bundle.js',
        publicPath: '/',
        // LES CHUNKS PORTENT LEUR EMPREINTE.
        //
        // Par défaut webpack les nomme `[id].bundle.js` — « 265.bundle.js » —
        // et RÉUTILISE le même identifiant d'un build à l'autre avec un
        // contenu différent. Le service worker, lui, sert les statiques en
        // cache-first en supposant l'inverse (son propre commentaire dit
        // « leur nom change avec leur contenu »). Résultat : le bundle
        // principal se rafraîchissait bien, network-first, mais chaque écran
        // chargé paresseusement — la fiche d'un jeu, par exemple — restait
        // servi depuis le cache d'une version précédente, indéfiniment. On
        // corrigeait, on reconstruisait, on réinstallait, et l'appareil
        // continuait d'afficher l'ancien écran (constaté le 2026-09-01, après
        // une demi-journée à chercher le bug ailleurs).
        //
        // Avec l'empreinte, un chunk modifié change d'URL : le cache-first ne
        // peut plus le manquer, et l'ancien fichier n'est simplement plus
        // demandé.
        chunkFilename: '[name].[contenthash].bundle.js',
        // Et on repart d'un dossier propre : sans ça, chaque build laisse ses
        // anciens chunks derrière lui. Ils ne servent plus à rien, mais l'APK
        // les embarque tous — elle était passée de 67 à 90 Mo.
        clean: true,
    },
    devServer: {
        static: [
            {
                directory: path.join(DIRNAME, 'dist'),
            },
            {
                directory: path.join(DIRNAME, 'public'),
                publicPath: '/',
            }
        ],
        host: '0.0.0.0',
        port: 3000,
        // 'only', not `true` — `true` still auto-reloads the whole page as
        // its OWN fallback whenever a module update can't hot-apply, a
        // behavior baked into webpack's HMR runtime itself and NOT gated by
        // `liveReload` below (that option only controls a separate
        // static-file-watch reload path — turned out not to be the actual
        // mechanism looping through a remote proxy). This app has no
        // explicit `module.hot.accept()` boundaries, so effectively every
        // change hits that fallback. 'only' removes it — HMR applies when it
        // can, otherwise nothing happens (needs a manual refresh) instead of
        // an automatic reload that a WS reconnect (proxy dropping the idle
        // connection) can retrigger forever.
        hot: 'only',
        historyApiFallback: true,
        compress: true,
        // Dev only (this whole devServer block never runs in production —
        // prod serves the built dist/ some other way). Without this the dev
        // server's default `Cache-Control: max-age=0` still lets browsers
        // (and anything in front of them, e.g. a Cloudflare tunnel) store the
        // response and only conditionally revalidate — usually fine, but
        // means every bundle/CSS change needs a manual cache clear on both
        // ends to actually show up. `no-store` tells them not to keep a copy
        // at all. Note: this only controls what *this origin* sends —
        // Cloudflare's own cache rules for static extensions (.js/.css) can
        // still override it unless the zone has "Origin Cache Control"
        // enabled or a page rule bypassing cache for this host; that part
        // isn't controllable from here.
        headers: { 'Cache-Control': 'no-store' },
        // Accessed through a remote reverse proxy (NPM/Cloudflare tunnel),
        // the dev server's own HMR socket (this `/ws`, separate from the Go
        // backend's input WebSocket) gets dropped whenever the
        // proxy's own WebSocket idle-timeout fires — most reverse proxies
        // recycle long-lived WS connections well before a dev session's
        // actual idle time. Each reconnect re-triggers the `hot: 'only'`
        // fallback situation above, and/or this separate static-file-watch
        // reload path — belt-and-suspenders, disabled too.
        liveReload: false,
        client: {
            webSocketURL: 'auto://0.0.0.0:0/ws',
            reconnect: 5,
        },
        // Without an explicit `path`, webpack-dev-server's own internal HMR
        // WebSocket server (ws.Server#shouldHandle) claims EVERY upgrade
        // request server-wide, not just its own '/ws'. Scoping it defensively
        // — the actual cause of backend WS connections dying (see the
        // `proxy` array below) turned out to be elsewhere, but there's no
        // reason to leave this one also matching paths it doesn't own.
        webSocketServer: {
            options: {
                path: '/ws',
            },
        },
        // LE PROXY : /api/v1/<domaine>/…, et un domaine n'appartient qu'à UN
        // service. On aiguille sur ce seul segment, donc aucune entrée n'en
        // recouvre une autre et l'ordre de la liste ne compte pas.
        //
        // C'est ce qui manquait avant le 2026-09-15 : des préfixes partagés
        // (/api/remoteplay entre le kernel et remoteplay-bin, /api/admin entre
        // trois services) demandaient des listes de sous-chemins, une fonction
        // de correspondance à part, et un préfixe inventé (/api-ws) pour qu'une
        // poignée de main WebSocket arrive au bon service — cassé deux fois le
        // 2026-08-25, une partie remote play sans manette.
        //
        // http-proxy-middleware compare les contextes par PRÉFIXE de chaîne :
        // `/api/v1/mount` attraperait aussi `/api/v1/mount-registry`. D'où la
        // vérification ci-dessous, qui refuse de démarrer plutôt que de laisser
        // deux entrées se disputer une requête. /internal, le service à
        // service, n'est jamais relayé.
        //
        // LA TABLE VIT DANS api-domains.json, depuis le 2026-09-15 : le frontal
        // d'une installation publique (microservices/edge) la lit aussi, et
        // deux copies de la liste des domaines finiraient par diverger. Chaque
        // entrée nomme la VARIABLE qui porte l'adresse du service — plus aucune
        // adresse par défaut ici : une variable absente arrête le serveur au
        // lieu de viser en silence une adresse privée écrite en dur.
        //
        // Les services : le kernel (parties, façade, bibliothèque, fédération,
        // serveurs amis, ressources distantes), credential (comptes,
        // préférences, présence, et les registres qui détiennent des secrets),
        // storage, la passerelle gatewaywebrtc, et remoteplay-bin (consoles
        // PlayStation, compte PSN, configuration TURN — le flux d'une partie
        // remote play, lui, est au kernel).
        proxy: !env.WEBPACK_SERVE ? [] : (() => {
            const SERVICES = JSON.parse(fs.readFileSync(path.join(DIRNAME, 'api-domains.json'), 'utf8'));
            const contexts = SERVICES.flatMap((service) => service.domains.map((domain) => `/api/v1/${domain}`));
            if (new Set(contexts).size !== contexts.length) {
                throw new Error(`proxy : un domaine est déclaré deux fois (${contexts.join(', ')})`);
            }
            contexts.forEach((shorter) => contexts.forEach((longer) => {
                if (shorter !== longer && longer.startsWith(shorter)) {
                    throw new Error(`proxy : ${shorter} recouvre ${longer} — renommer l'un des deux domaines`);
                }
            }));
            return SERVICES.map((service) => {
                const target = process.env[service.target];
                if (!target) {
                    throw new Error(`proxy : ${service.target} manque — l'adresse du service ${service.service}`);
                }
                const entry = {
                    context: service.domains.map((domain) => `/api/v1/${domain}`),
                    target,
                    changeOrigin: true,
                    secure: false,
                    ws: Boolean(service.websocket),
                    // QUI A ESSAYÉ DE SE CONNECTER, et pas seulement « le
                    // proxy » (forwardClientAddress, le service credential).
                    // Sans xfwd, toute tentative arrive au service d'identité
                    // depuis l'adresse de ce conteneur : dix essais de dix
                    // appareils s'y confondent en un seul, et le journal des
                    // connexions perd la moitié de ce qui le rend utile
                    // (2026-09-07).
                    //
                    // L'en-tête reste DÉCLARATIF — voir claimedClientAddress
                    // côté Go : bon pour reconnaître, jamais pour autoriser.
                    xfwd: Boolean(service.forwardClientAddress),
                };
                if (service.service === 'remoteplay') {
                    // The admin TURN page's credentials are stored encrypted
                    // under PASSPHRASE_SALT, and both reading and saving them
                    // require presenting it. Attaching it HERE, in the proxy,
                    // is the whole point: this runs in the container, so the
                    // secret never reaches the browser and never gets built
                    // into the bundle — the page just makes ordinary requests
                    // and the passphrase is added on the way through. Scoped
                    // to /api/v1/turn-config only (nothing else needs it), and
                    // silently skipped when unset, which the service handles.
                    entry.onProxyReq = (proxyReq, req) => {
                        if (process.env.PASSPHRASE_SALT && req.url.indexOf('/api/v1/turn-config') === 0) {
                            proxyReq.setHeader('X-Turn-Passphrase', process.env.PASSPHRASE_SALT);
                        }
                    };
                }
                return entry;
            });
        })(),
        // ── /dev/checklist — outil de triage, DÉV UNIQUEMENT ─────────────
        //
        // Une page qui liste les items encore ouverts de
        // docs/roadmap/reste-a-faire.md et écrit les coches dans un JSON, pour
        // que le tri « déjà fait / reste à faire » se fasse en cochant plutôt
        // qu'en récitant une liste dans une conversation.
        //
        // Pourquoi ici et pas une route React : ça ne demande ni composant, ni
        // entrée de menu, ni clés i18n dans les deux langues — donc zéro ligne
        // de code produit pour un outil interne. Et la page vit dans testing/,
        // hors de public/, donc elle n'est jamais copiée dans dist/ ni
        // embarquée dans l'APK.
        //
        // `unshift` : sans ça `historyApiFallback` ci-dessus répond l'index de
        // l'application à /dev/checklist et la page ne se voit jamais.
        //
        // Comme tout ce bloc devServer, ceci n'est lu qu'au DÉMARRAGE du
        // serveur : après modification, recréer le conteneur.
        setupMiddlewares: (middlewares) => {
            const PAGE = path.join(DIRNAME, 'testing', 'dev-checklist.html');
            // LA LISTE ELLE-MÊME, et plus une copie. docs/roadmap est monté sous
            // /roadmap (docker-compose.yaml) : la page lit le markdown à chaque
            // requête, et valider, refuser ou rouvrir le réécrivent. La copie
            // JSON qu'on régénérait à la main montrait encore comme ouverts des
            // items fermés, et la régénérer renumérotait tout (2026-09-14).
            const LIST = '/roadmap/reste-a-faire.md';
            // testing/checkings/ est déjà git-ignoré : les signalements et les
            // notes sont transitoires, ils n'ont pas à entrer dans l'historique.
            const STATE = path.join(DIRNAME, 'testing', 'checkings', 'dev-checklist-state.json');
            const RECETTE = ' *(en recette)*';

            // LE RÉSEAU LOCAL SEULEMENT (2026-09-15). Ces pages n'ont aucune
            // authentification, et la checklist réécrit un fichier du dépôt :
            // servies par le domaine public, n'importe qui sur Internet les
            // lisait et pouvait écrire. Une requête passée par un proxy
            // (en-têtes X-Forwarded-* ou Forwarded), ou adressée à un nom
            // d'hôte plutôt qu'à une adresse locale, est refusée.
            const fromLocalNetwork = (req) => {
                if (req.headers['x-forwarded-for'] || req.headers['x-forwarded-host'] || req.headers.forwarded) {
                    return false;
                }
                const host = String(req.headers.host || '').replace(/:\d+$/, '').replace(/^\[(.*)\]$/, '$1');

                return host === 'localhost' || host === '::1'
                    || /^(127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(host);
            };

            const send = (res, code, type, body) => {
                res.statusCode = code;
                res.setHeader('Content-Type', type);
                res.setHeader('Cache-Control', 'no-store');
                res.end(body);
            };
            const sendJson = (res, code, value) => send(res, code, 'application/json; charset=utf-8', JSON.stringify(value));
            const readBody = (req, done) => {
                let raw = '';
                req.setEncoding('utf8');
                req.on('data', (chunk) => {
                    raw += chunk;
                    // Un corps sans borne sur un serveur de dev reste un corps
                    // sans borne.
                    if (raw.length > 262144) { req.destroy(); }
                });
                req.on('end', () => {
                    let body;
                    try {
                        body = JSON.parse(raw);
                    } catch (err) {
                        return done(err);
                    }
                    return done(null, body);
                });
            };

            // Le format de la liste — le contrat est écrit dans son en-tête :
            //   - [ ] `#12` …                   à faire
            //   - [x] `#12` *(en recette)* …    fini par Claude, à valider
            //   - [x] `#12` …                   validé
            // Un item continue sur les lignes indentées qui le suivent.
            const parseList = (text) => {
                const lines = text.split('\n');
                const items = [];
                let section = '';
                let sous = '';
                for (let i = 0; i < lines.length; i += 1) {
                    const line = lines[i];
                    if (line.startsWith('## ')) {
                        section = line.slice(3).trim();
                        sous = '';
                    } else if (line.startsWith('### ')) {
                        sous = line.slice(4).trim();
                    } else {
                        const m = /^- \[([ x])\] (?:`#(\d+)` )?(\*\(en recette\)\* )?(.*)$/.exec(line);
                        if (m) {
                            const body = [m[4]];
                            let j = i + 1;
                            while (j < lines.length && lines[j].startsWith('  ')) {
                                body.push(lines[j].trim());
                                j += 1;
                            }
                            const complet = body.join(' ');
                            const lead = /^\*\*(.+?)\*\*/.exec(complet);
                            let statut = 'fait';
                            if (m[1] === ' ') { statut = 'todo'; } else if (m[3]) { statut = 'recette'; }
                            items.push({
                                id: m[2] ? Number(m[2]) : null,
                                statut,
                                section,
                                sous,
                                court: lead ? `**${lead[1]}**` : complet.split(/(?<=\.)\s/)[0].slice(0, 160),
                                complet,
                                ligne: i + 1,
                            });
                            i = j - 1;
                        }
                    }
                }
                return items;
            };

            // action → [état requis, réécriture de SA ligne]
            const TRANSITIONS = {
                valider: ['recette', (line) => line.replace(RECETTE, '')],
                refuser: ['recette', (line) => line.replace('- [x] ', '- [ ] ').replace(RECETTE, '')],
                rouvrir: ['fait', (line) => line.replace('- [x] ', '- [ ] ')],
            };

            middlewares.unshift({
                name: 'dev-checklist',
                path: '/dev/checklist',
                middleware: (req, res, next) => {
                    if (!fromLocalNetwork(req)) {
                        return send(res, 404, 'text/plain; charset=utf-8', 'Not found');
                    }
                    const route = (req.url || '/').split('?')[0];
                    if (req.method === 'GET' && (route === '/' || route === '')) {
                        return fs.readFile(PAGE, (err, buf) => (err
                            ? send(res, 404, 'text/plain; charset=utf-8', 'absent : dev-checklist.html')
                            : send(res, 200, 'text/html; charset=utf-8', buf)));
                    }
                    if (req.method === 'GET' && route === '/items') {
                        return fs.readFile(LIST, 'utf8', (err, text) => (err
                            ? sendJson(res, 500, { ok: false, error: `liste illisible (${LIST}) : ${err.message} — le conteneur a-t-il été recréé avec le montage ?` })
                            : sendJson(res, 200, parseList(text))));
                    }
                    if (req.method === 'GET' && route === '/state') {
                        return fs.readFile(STATE, 'utf8', (err, text) => {
                            let saved = {};
                            try {
                                saved = err ? {} : JSON.parse(text);
                            } catch (parseErr) {
                                saved = {};
                            }
                            // `faits` était le nom d'avant : une coche posée par
                            // l'utilisateur sur un item OUVERT, donc un signalement.
                            // Relu tel quel, sans réécrire son fichier.
                            return sendJson(res, 200, { signales: saved.signales || saved.faits || {}, notes: saved.notes || {} });
                        });
                    }
                    if (req.method === 'POST' && route === '/state') {
                        return readBody(req, (err, body) => {
                            if (err) { return sendJson(res, 400, { ok: false, error: 'json invalide' }); }
                            const value = { signales: body.signales || {}, notes: body.notes || {}, maj: new Date().toISOString() };
                            return fs.mkdir(path.dirname(STATE), { recursive: true }, () => {
                                fs.writeFile(STATE, `${JSON.stringify(value, null, 1)}\n`, (werr) => (werr
                                    ? sendJson(res, 500, { ok: false, error: werr.message })
                                    : sendJson(res, 200, { ok: true })));
                            });
                        });
                    }
                    if (req.method === 'POST' && route === '/transition') {
                        return readBody(req, (err, body) => {
                            if (err) { return sendJson(res, 400, { ok: false, error: 'json invalide' }); }
                            const rule = TRANSITIONS[body.action];
                            if (!rule) { return sendJson(res, 400, { ok: false, error: `action inconnue : ${body.action}` }); }
                            return fs.readFile(LIST, 'utf8', (rerr, text) => {
                                if (rerr) { return sendJson(res, 500, { ok: false, error: rerr.message }); }
                                const found = parseList(text).filter((it) => it.id === Number(body.id));
                                if (found.length !== 1) {
                                    return sendJson(res, 404, { ok: false, error: `#${body.id} : ${found.length} item(s) portent ce numéro` });
                                }
                                const [item] = found;
                                if (item.statut !== rule[0] || (body.expect && body.expect !== item.statut)) {
                                    // Changé entre l'affichage et le clic — par Claude,
                                    // ou dans un autre onglet.
                                    return sendJson(res, 409, { ok: false, statut: item.statut, error: `#${item.id} est « ${item.statut} » : recharge la page` });
                                }
                                const lines = text.split('\n');
                                const before = lines[item.ligne - 1];
                                const after = rule[1](before);
                                if (after === before) { return sendJson(res, 500, { ok: false, error: 'rien à réécrire' }); }
                                lines[item.ligne - 1] = after;
                                const rewritten = lines.join('\n');
                                // En place, jamais par renommage : un fichier remplacé
                                // depuis le conteneur changerait de propriétaire sur l'hôte.
                                return fs.writeFile(LIST, rewritten, (werr) => (werr
                                    ? sendJson(res, 500, { ok: false, error: werr.message })
                                    : sendJson(res, 200, { ok: true, id: item.id, statut: parseList(rewritten).find((it) => it.id === item.id).statut })));
                            });
                        });
                    }
                    return next();
                },
            });

            // LES GRAPHES D'ARCHITECTURE (docs/graphs, monté sous /graphs en
            // lecture seule). Le markdown reste la source — lisible tel quel sur
            // GitHub — et la page le rend avec Mermaid. Un nom de fichier, jamais
            // un chemin : rien ne sort du dossier.
            const GRAPHS = '/graphs';
            const GRAPHS_PAGE = path.join(DIRNAME, 'testing', 'dev-graphs.html');
            const GRAPH_FILE = /^[\w.-]+\.md$/;
            middlewares.unshift({
                name: 'dev-graphs',
                path: '/dev/graphs',
                middleware: (req, res, next) => {
                    if (!fromLocalNetwork(req)) {
                        return send(res, 404, 'text/plain; charset=utf-8', 'Not found');
                    }
                    if (req.method !== 'GET') { return next(); }
                    const url = new URL(req.url || '/', 'http://dev.local');
                    const route = url.pathname;
                    if (route === '/' || route === '') {
                        return fs.readFile(GRAPHS_PAGE, (err, buf) => (err
                            ? send(res, 404, 'text/plain; charset=utf-8', 'absent : dev-graphs.html')
                            : send(res, 200, 'text/html; charset=utf-8', buf)));
                    }
                    if (route === '/list') {
                        return fs.readdir(GRAPHS, (err, names) => {
                            if (err) {
                                return sendJson(res, 500, { ok: false, error: `${GRAPHS} illisible : ${err.message} — le conteneur a-t-il été recréé avec le montage ?` });
                            }
                            // L'index d'abord : c'est la porte d'entrée, et la vue d'ensemble.
                            const docs = names.filter((name) => GRAPH_FILE.test(name))
                                .sort((a, b) => (a === 'README.md' ? -1 : b === 'README.md' ? 1 : a.localeCompare(b)))
                                .map((file) => {
                                let title = file;
                                try {
                                    const first = fs.readFileSync(path.join(GRAPHS, file), 'utf8').split('\n').find((line) => line.startsWith('# '));
                                    if (first) { title = first.slice(2).trim(); }
                                } catch (readErr) {
                                    title = file;
                                }
                                return { file, title };
                            });
                            return sendJson(res, 200, docs);
                        });
                    }
                    if (route === '/doc') {
                        const file = url.searchParams.get('file') || '';
                        if (!GRAPH_FILE.test(file)) { return sendJson(res, 400, { ok: false, error: 'nom de fichier invalide' }); }
                        return fs.readFile(path.join(GRAPHS, file), 'utf8', (err, text) => (err
                            ? sendJson(res, 404, { ok: false, error: `absent : ${file}` })
                            : send(res, 200, 'text/markdown; charset=utf-8', text)));
                    }
                    return next();
                },
            });

            return middlewares;
        },
        allowedHosts: 'all',
        // bundle.js is un-hashed in dev, and the service worker's cache-first
        // asset strategy would otherwise pin an OLD bundle forever (the SW
        // only reinstalls when service-worker.js itself changes). no-store on
        // every response is what actually lets a reload pick up new frontend
        // code without clearing site data by hand.
        headers: { 'Cache-Control': 'no-store' },
    },
    module: {
        rules: [
            {
                test: /\.(js|jsx)$/,
                exclude: /node_modules/,
                use: {
                    loader: 'babel-loader',
                    options: {
                        presets: ['@babel/preset-env', '@babel/preset-react'],
                    }
                },
            },
            {
                test: /\.css$/,
                use: ['style-loader', 'css-loader'],
            },
            {
                test: /\.(png|svg|jpg|jpeg|gif)$/,
                type: 'asset/resource'
            },
            {
                test: /\.(woff|woff2|ttf|eot)$/i,
                type: 'asset/resource',
                generator: {
                    filename: 'fonts/[name][ext]',
                },
            },
        ],
    },
    resolve: {
        extensions: ['.js', '.jsx'],
        modules: [
            path.resolve(DIRNAME, 'node_modules'),
            'node_modules'
        ],
        alias: {
            '@': path.resolve(DIRNAME, 'app/'),
        }
    },
    plugins: [
        new HtmlWebpackPlugin({
            template: './public/index.html'
        }),
        new CopyWebpackPlugin({
            patterns: [
                // À LA RACINE, comme le serveur de dev : index.html, le
                // manifeste et le service worker demandent `/images/…`. Copiées
                // sous `public/images`, les icônes répondaient 404 en
                // production — onglet sans icône, notifications aussi
                // (relevé le 2026-10-09).
                {
                    to: 'images',
                    from: 'public/images',
                },
                {
                    to: 'manifest.json',
                    from: 'public/manifest.json',
                },
                {
                    to: 'service-worker.js',
                    from: 'public/service-worker.js',
                },
                // Chromecast custom receiver (Phase B1 of the "Mode Console
                // + Chromecast" plan) — deliberately outside the React
                // bundle (see the file's own top-of-file doc), just a
                // static asset dev's `static` serving already exposes for
                // free; production needs this explicit copy like the other
                // public/ files above.
                {
                    to: 'cast-receiver.html',
                    from: 'public/cast-receiver.html',
                },
                // L'APK Android, déposée là par le build de src/mobileapp.
                // noErrorOnMissing : un dépôt frais n'en a pas, et le
                // frontend doit se construire quand même — c'est un binaire
                // produit, pas une source (il est git-ignoré).
                {
                    to: 'downloads',
                    from: 'public/downloads',
                    noErrorOnMissing: true,
                },
            ]
        }),
        emitBuildInfo,
        new webpack.DefinePlugin({
            'process.env.PUID': JSON.stringify(process.env.PUID || '1000'),
            'process.env.PGID': JSON.stringify(process.env.PGID || '1000'),
            'process.env.TIMEZONE': JSON.stringify(process.env.TZ || 'UTC'),
            // Relues à chaque compilation (`runtimeValue`, sans cache) : le
            // serveur de développement suit le dépôt sans redémarrer.
            'process.env.GITHUB_COMMIT_SHA': webpack.DefinePlugin.runtimeValue(
                () => JSON.stringify(buildInfo().commit), true),
            'process.env.GAMEFLEX_VERSION': webpack.DefinePlugin.runtimeValue(
                () => JSON.stringify(buildInfo().version), true),
            // LA DATE DE CE BUNDLE-CI.
            //
            // GITHUB_COMMIT_SHA est figé au DÉMARRAGE DU CONTENEUR : il ne
            // bouge pas quand on reconstruit le bundle, ce qui en fait une
            // marque inutile pour la seule question qui se pose vraiment —
            // « l'application que je regarde contient-elle mes
            // modifications ? ». On a passé trois échanges à en débattre
            // faute de pouvoir y répondre d'un coup d'œil (2026-09-01).
            //
            // Celle-ci change à chaque compilation, et se lit dans le pied
            // de page comme dans l'application.
            'process.env.BUILD_STAMP': webpack.DefinePlugin.runtimeValue(
                () => JSON.stringify(buildInfo().builtAt), true),
            // Chromecast receiver App ID (see docker-compose.yaml's
            // gameflex-webapp service / credentials.sh) — the env var is
            // GAMEFLEX_GOOGLECAST_APPLICATION_ID; cast.js's RECEIVER_APP_ID
            // reads it at build time through this define.
            'process.env.GOOGLECAST_APPLICATION_ID': JSON.stringify(process.env.GAMEFLEX_GOOGLECAST_APPLICATION_ID || ''),
        }),
    ],
});