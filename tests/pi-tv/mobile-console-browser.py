"""Regression for the mobile console page: installed games must be visible."""
import json
import os
from pathlib import Path

from playwright.sync_api import sync_playwright


BASE = os.environ.get("INTERFACE_PREVIEW", "http://127.0.0.1:18196/gameflex/")
SCREENSHOT = Path(os.environ.get("MOBILE_CONSOLE_SCREENSHOT", "/tmp/gameflex-mobile-console.png"))


with sync_playwright() as playwright:
    browser = playwright.chromium.launch(
        executable_path="/usr/bin/chromium",
        headless=True,
        args=["--no-sandbox", "--disable-dev-shm-usage"],
    )
    context = browser.new_context(viewport={"width":390,"height":844},locale="fr-FR")
    page = context.new_page()
    errors=[]
    page.on("pageerror",lambda error:errors.append(str(error)))
    page.goto(BASE+"retrogaming/n64?saved=consoles",wait_until="domcontentloaded")
    cards=page.locator(".em-cartridge-shelf>article")
    cards.nth(23).wait_for(timeout=15_000)
    page.wait_for_timeout(500)
    first=cards.first.bounding_box()
    dock=page.locator(".em-dock").bounding_box()
    filters=page.locator(".gf-catalogue-tabs").bounding_box()
    assert cards.count()==24,cards.count()
    assert first and dock and filters
    assert first["y"]+first["height"]<dock["y"],(first,dock)
    assert filters["y"]<first["y"],(first,filters)
    assert page.evaluate("document.documentElement.scrollWidth<=innerWidth+2")
    assert not errors,errors
    SCREENSHOT.parent.mkdir(parents=True,exist_ok=True)
    page.screenshot(path=str(SCREENSHOT),full_page=False)
    cards.last.scroll_into_view_if_needed()
    page.wait_for_timeout(150)
    last=cards.last.bounding_box()
    dock=page.locator(".em-dock").bounding_box()
    assert last and dock and last["y"]+last["height"]<dock["y"],(last,dock)
    print(json.dumps({"games":cards.count(),"firstGameTop":first["y"],"dockTop":dock["y"],"filtersTop":filters["y"],"lastGameBottom":last["y"]+last["height"],"errors":errors},ensure_ascii=False))
    browser.close()
