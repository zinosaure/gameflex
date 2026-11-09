// Build the isolated fixture preview using the project's existing dependencies.
const path = require('path');
const root = path.resolve(__dirname, '../..');
const modules = path.join(root, 'src/webapp/src/node_modules');
const webpack = require(path.join(modules, 'webpack'));
webpack({
    mode: 'development',
    devtool: false,
    entry: path.join(root, 'tests/pi-tv/interface-cleanup-preview.jsx'),
    output: { path: path.join(root, '.local/interface-cleanup/after'), filename: 'app.js' },
    plugins: [
        new webpack.NormalModuleReplacementPlugin(/profile\/auth\/AuthContext\.jsx$/, resource => {
            resource.request = path.join(root, 'tests/pi-tv/fixtures/interfaces/auth.jsx');
        }),
        new webpack.NormalModuleReplacementPlugin(/api\/retrogaming\.js$/, resource => {
            if (resource.context.endsWith('/experience')) {
                resource.request = path.join(root, 'tests/pi-tv/fixtures/interfaces/retrogaming.js');
            }
        }),
    ],
    resolve: {
        modules: [modules, 'node_modules'],
        extensions: ['.js', '.jsx'],
        alias: { '@': path.join(root, 'src/webapp/src/app') },
    },
    module: { rules: [
        { test: /\.jsx?$/, exclude: /node_modules/, use: {
            loader: path.join(modules, 'babel-loader'),
            options: { presets: [path.join(modules, '@babel/preset-env'),
                [path.join(modules, '@babel/preset-react'), { runtime: 'automatic' }]] },
        } },
        { test: /\.css$/, use: [path.join(modules, 'style-loader'), path.join(modules, 'css-loader')] },
        { test: /\.(png|svg|jpg|webp)$/, type: 'asset/resource' },
    ] },
}, (error, stats) => {
    if (error || stats.hasErrors()) {
        console.error(error || stats.toString({ all: false, errors: true }));
        process.exitCode = 1;
    } else console.log('Isolated preview built');
});
