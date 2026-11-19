const { chromium } = require('playwright-core');
const CHROMIUM_PATH = process.env.CHROMIUM_PATH || '/usr/bin/chromium';
async function launchPage(options = {}) {
    const browser = await chromium.launch({
        executablePath: CHROMIUM_PATH,
        headless: true,
        args: ['--no-sandbox', '--use-gl=swiftshader', '--enable-webgl', '--ignore-gpu-blocklist', '--enable-unsafe-swiftshader'],
        ...options,
    });
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', (err) => errors.push(err.message));
    page.on('console', (msg) => { if (msg.type() === 'error') errors.push(msg.text()); });
    return { browser, page, errors };
}
module.exports = { launchPage };
