import { fileURLToPath } from 'url';

import fs from 'fs';
import path from 'path';
import webpack from 'webpack';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import CopyWebpackPlugin from 'copy-webpack-plugin';

const FILENAME = fileURLToPath(import.meta.url);
const DIRNAME = path.dirname(FILENAME);

// LA VERSION ET LE COMMIT, LUS DANS LE DÉPÔT (chantier 56, P3 ; la règle de la webapp). Le compose monte le dépôt en
// lecture seule sur /repo : webpack y lit VERSION et le commit de HEAD à chaque compilation. Pris dans l'environnement,
// ils restaient ceux du démarrage du conteneur — « v0.2.0 » partait dans l'APK 0.4.0 (2026-10-24). L'image de la
// release n'a pas de dépôt : elle retombe sur les arguments de sa construction (GAMEFLEX_VERSION, GITHUB_COMMIT_SHA).
const REPO = '/repo';

function readRepo(file) {
    try {
        return fs.readFileSync(path.join(REPO, file), 'utf8').trim();
    } catch {
        return '';
    }
}

// Le commit de HEAD, sans git (l'image n'en a pas) : la référence de HEAD, en fichier ou dans packed-refs ; ou HEAD
// lui-même, détaché.
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

// `build-info.json`, à côté du bundle : ce que le build de l'APK compare à la version qu'il fabrique
// (src/mobileapp/docker-entrypoint.sh) — une APK refuse une interface d'une autre version.
const emitBuildInfo = {
    apply(compiler) {
        compiler.hooks.thisCompilation.tap('BuildInfo', (compilation) => {
            compilation.hooks.processAssets.tap(
                { name: 'BuildInfo', stage: webpack.Compilation.PROCESS_ASSETS_STAGE_ADDITIONAL },
                () => compilation.emitAsset('build-info.json', new webpack.sources.RawSource(JSON.stringify(buildInfo()) + '\n')),
            );
        });
    },
};

// EDGE, L'INTERFACE DE GAMEFLEX (chantier 56) — le launcher des chantiers 51 à 53, la seule
// interface depuis la bascule (P5), au port de la page (GAMEFLEX_PORT).
//
// webpack 5, babel, React 18 ; de Skeleton, ses seules libs (Form, Table, Pagination, sur
// react-bootstrap) ; Bootstrap compilé depuis ses sources SCSS (app/styles/bootstrap.scss), et
// bootstrap-icons.
// Deux usages : `npm start`, le serveur de développement (le conteneur gameflex-edge), et
// `npm run build`, la construction de production dans dist/ — celle que sert le frontal de la
// release (l'étape `page` du Dockerfile d'edge) et qu'embarque l'APK (chantier 56, P3).
//
// LA TABLE DES DOMAINES DE L'API (api-domains.json, à côté) : la seule, que lisent ce serveur de
// développement ET le frontal de la release (cmd/edge, /app/api-domains.json) — jamais recopiée :
// deux copies de la liste finiraient par diverger (règle des images de production,
// CODE_CONVENTIONS.md).

export default (env = {}, argv = {}) => ({
    // `--mode production` de `npm run build` décide ; sinon l'environnement du conteneur.
    mode: argv.mode || process.env.NODE_ENV || 'development',
    entry: './app/index.js',
    output: {
        path: path.resolve(DIRNAME, 'dist'),
        filename: 'bundle.js',
        publicPath: '/',
        chunkFilename: '[name].[contenthash].bundle.js',
        clean: true,
        // TOUJOURS RÉÉCRIT : la construction de l'APK refuse un bundle plus vieux que le code (docker-entrypoint.sh).
        // Une modification sans effet sur le paquet (un import retiré) le laissait à son ancienne date, et bloquait l'APK.
        compareBeforeEmit: false,
    },
    devServer: {
        static: [{ directory: path.join(DIRNAME, 'public'), publicPath: '/' }],
        host: '0.0.0.0',
        port: 3000,
        // Les mêmes raisons que la webapp : pas de rechargement automatique
        // en boucle derrière un proxy, rien de gardé en cache.
        hot: 'only',
        liveReload: false,
        // Toute adresse de l'application se recharge, même quand son dernier segment a un point (le nom de
        // fichier d'un jeu, le chemin d'un journal) : sans `disableDotRule`, elle passait pour un fichier, et
        // le rechargement répondait 404. Les fichiers eux-mêmes ne demandent pas de HTML : jamais réécrits.
        historyApiFallback: { disableDotRule: true },
        compress: true,
        allowedHosts: 'all',
        headers: { 'Cache-Control': 'no-store' },
        client: { webSocketURL: 'auto://0.0.0.0:0/ws', reconnect: 5 },
        webSocketServer: { options: { path: '/ws' } },
        // Le relais de /api/v1/<domaine>, d'après la table (api-domains.json) : le
        // même que celui du frontal de la release.
        proxy: !env.WEBPACK_SERVE ? [] : (() => {
            const SERVICES = JSON.parse(fs.readFileSync(path.join(DIRNAME, 'api-domains.json'), 'utf8'));

            return SERVICES.map((service) => {
                const target = process.env[service.target];

                if (!target) {
                    throw new Error(`proxy : ${service.target} manque — l'adresse du service ${service.service}`);
                }

                return {
                    context: service.domains.map((domain) => `/api/v1/${domain}`),
                    target,
                    changeOrigin: true,
                    secure: false,
                    ws: Boolean(service.websocket),
                    xfwd: Boolean(service.forwardClientAddress),
                };
            });
        })(),
    },
    module: {
        rules: [
            {
                test: /\.(js|jsx)$/,
                exclude: /node_modules/,
                use: {
                    loader: 'babel-loader',
                    options: { presets: ['@babel/preset-env', '@babel/preset-react'] },
                },
            },
            { test: /\.css$/, use: ['style-loader', 'css-loader'] },
            // BOOTSTRAP DEPUIS SES SOURCES (styles/bootstrap.scss) : le thème par ses variables.
            // `quietDeps` tait les avertissements de dépréciation de Bootstrap lui-même.
            { test: /\.scss$/, use: ['style-loader', 'css-loader', { loader: 'sass-loader', options: { sassOptions: { quietDeps: true } } }] },
            { test: /\.(woff|woff2|ttf|eot)$/i, type: 'asset/resource', generator: { filename: 'fonts/[name][ext]' } },
            // LA LICENCE DE GAMEFLEX (chantier 53, P13) : le fichier LICENSE du dépôt, monté en lecture seule à /src/LICENSE
            // (le compose), importé comme texte et inscrit dans le bundle — une seule source, lue au build : une
            // construction qui ne l'a pas échoue au lieu d'afficher une copie périmée.
            { test: path.join(DIRNAME, 'LICENSE'), type: 'asset/source' },
        ],
    },
    resolve: { extensions: ['.js', '.jsx'] },
    plugins: [
        new HtmlWebpackPlugin({ template: './public/index.html' }),
        // LES FICHIERS DE public/ À LA RACINE DE dist/, comme le serveur de développement les sert : les icônes que
        // demandent la page, le manifeste et le service worker ; le service worker lui-même (le push) ; le manifeste
        // de l'installation en application (chantier 56, A8) ; le récepteur Chromecast, une page à part, hors du
        // bundle. `public/downloads` n'y entre JAMAIS : l'APK et les paquets du téléphone ne se servent qu'à la dev,
        // depuis public/ — une page qui s'embarque dans l'APK ne doit pas l'embarquer elle-même
        // (src/mobileapp/docker-entrypoint.sh).
        new CopyWebpackPlugin({
            patterns: [
                { from: 'public/images', to: 'images' },
                { from: 'public/service-worker.js', to: 'service-worker.js' },
                { from: 'public/manifest.json', to: 'manifest.json' },
                { from: 'public/cast-receiver.html', to: 'cast-receiver.html' },
            ],
        }),
        emitBuildInfo,
        new webpack.DefinePlugin({
            // La version de l'en-tête : celle du dépôt, relue à chaque compilation (`runtimeValue`, sans cache) — le
            // serveur de développement suit le dépôt sans redémarrer, et la construction écrit la même dans
            // build-info.json.
            'process.env.GAMEFLEX_VERSION': webpack.DefinePlugin.runtimeValue(() => JSON.stringify(buildInfo().version), true),
        }),
    ],
});
