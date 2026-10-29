// Reusable Playwright driver for verifying frontend changes visually — see
// SKILLS.md (harnais de test) for the full workflow this belongs to.
//
// Usage (always via `docker exec skeleton-nodejs ...`, never on the host —
// this repo's dev server and browser only exist inside the container):
//
//   docker exec skeleton-nodejs node testing/my-check.js
//
// where my-check.js does:
//
//   const { launchPage } = require('./browser.js');
//   (async () => {
//       const { browser, page, errors } = await launchPage();
//       await page.goto('http://localhost:3000/live/games');
//       await page.screenshot({ path: '/tmp/check.png' });
//       console.log('console errors:', errors);
//       await browser.close();
//   })();
//
// then `docker cp skeleton-nodejs:/tmp/check.png <scratchpad>/check.png` to
// actually look at it — a screenshot nobody views proves nothing.
const { chromium } = require('playwright-core');

const CHROMIUM_PATH = process.env.CHROMIUM_PATH || '/usr/bin/chromium';

async function launchPage(options = {}) {
    const browser = await chromium.launch({
        executablePath: CHROMIUM_PATH,
        headless: true,
        args: ['--no-sandbox', '--disable-gpu'],
        ...options,
    });

    const page = await browser.newPage();
    const errors = [];

    page.on('pageerror', (err) => errors.push(err.message));
    page.on('console', (msg) => {
        if (msg.type() === 'error') {
            errors.push(msg.text());
        }
    });

    return { browser, page, errors };
}

module.exports = { launchPage };
