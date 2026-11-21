"""Verify that the shared game sheet follows the selected interface theme."""
import json
import os
from pathlib import Path

from playwright.sync_api import sync_playwright


BASE=os.environ.get('INTERFACE_PREVIEW','http://127.0.0.1:18196/gameflex/')
OUTPUT=Path(os.environ.get('THEMED_DIALOG_SCREENSHOTS','/tmp/gameflex-themed-dialogs'))


with sync_playwright() as playwright:
    browser=playwright.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
    results=[];OUTPUT.mkdir(parents=True,exist_ok=True)
    for mode in ('crt','consoles'):
        context=browser.new_context(viewport={'width':390,'height':844},locale='fr-FR')
        page=context.new_page();errors=[];page.on('pageerror',lambda error:errors.append(str(error)))
        page.goto(BASE+f'retrogaming/n64?saved={mode}',wait_until='domcontentloaded')
        game=page.locator('[data-game="fixture-1"], [data-focus-key="n64:fixture-1"]').first
        game.wait_for(timeout=15_000)
        page.wait_for_function("window.__requests.some(item => item.path.endsWith('/covers/search'))")
        game.click();dialog=page.get_by_role('dialog');dialog.wait_for()
        assert page.locator(f'.gf-catalogue-backdrop.gf-theme-{mode}').count()==1
        style=dialog.evaluate("node => ({background:getComputedStyle(node).backgroundColor,color:getComputedStyle(node).color,font:getComputedStyle(node).fontFamily})")
        assert not errors,errors
        page.screenshot(path=str(OUTPUT/f'{mode}.png'))
        results.append({'mode':mode,**style})
        context.close()
    assert results[0]['background']!=results[1]['background']
    assert results[0]['font']!=results[1]['font']
    print(json.dumps(results,ensure_ascii=False))
    browser.close()
