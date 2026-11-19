"""Exercise every selectable GameFlex appearance in the isolated mobile preview."""
import json
import os
from pathlib import Path

from playwright.sync_api import sync_playwright


BASE = os.environ.get("INTERFACE_PREVIEW", "http://127.0.0.1:18196/gameflex/")
OUTPUT = Path(os.environ.get("INTERFACE_SCREENSHOTS", "/tmp/gameflex-interface-audit"))
INTERFACES = [
    "station", "crt", "shelf", "orbit", "arcade", "minimal", "covers",
    "consoles", "dashboard", "hub-canape", "ligne-cinema", "mosaique-joy",
    "roue-retro", "terminal-poche", "cartouches", "carte-epoques",
    "double-ecran", "club-local", "quai-systemes", "magazine-retro",
    "bureau-collectionneur", "grand-ecran-accessible",
]
ONLY = os.environ.get("INTERFACE_ONLY")


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    results = []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(
            executable_path="/usr/bin/chromium",
            headless=True,
            args=["--no-sandbox", "--disable-dev-shm-usage"],
        )
        selected = [item for item in INTERFACES if not ONLY or item == ONLY]
        for index, interface in enumerate(selected, 1):
            context = browser.new_context(
                viewport={"width": 390, "height": 844},
                device_scale_factor=1,
                locale="fr-FR",
            )
            page = context.new_page()
            errors = []
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.goto(f"{BASE}?saved={interface}", wait_until="domcontentloaded")
            page.locator(f'[data-interface="{interface}"]').wait_for(timeout=15_000)
            page.wait_for_timeout(900)
            interrupted = page.get_by_text("Affichage interrompu", exact=True).count()
            overflow = page.evaluate("document.documentElement.scrollWidth - innerWidth")
            screenshot = OUTPUT / f"{index:02d}-{interface}.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "interface": interface,
                "errors": errors,
                "interrupted": interrupted,
                "overflow": overflow,
                "screenshot": str(screenshot),
            }
            results.append(result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
            context.close()
        browser.close()
    failures = [
        result for result in results
        if result["errors"] or result["interrupted"] or result["overflow"] > 2
    ]
    print(json.dumps({"interfaces": len(results), "failures": failures}, ensure_ascii=False))
    raise SystemExit(1 if failures else 0)


if __name__ == "__main__":
    main()
