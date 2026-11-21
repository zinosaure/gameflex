"""Regression for the Office focus timer, and the unchanged launch contract."""
import json,os
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://127.0.0.1:18197/gameflex/')
results=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 # A background source refresh must not steal the menu's focus in Office.
 context=browser.new_context(viewport={'width':1920,'height':1080});page=context.new_page();page.goto(BASE+'?saved=bureau-collectionneur',wait_until='domcontentloaded');page.locator('.gf-office-table').wait_for()
 page.get_by_role('button',name='Menu principal',exact=True).click();page.get_by_role('dialog',name='Menu principal').wait_for();page.evaluate('window.dispatchEvent(new Event("focus"))');page.wait_for_timeout(500)
 assert page.evaluate('Boolean(document.activeElement.closest("[aria-modal=true]"))'),'Office refresh stole the menu focus'
 page.keyboard.press('Escape');context.close();results.append('Office TV: source refresh keeps focus inside the menu')
 # The launch callback still sends the original reference and opens streaming.
 context=browser.new_context();page=context.new_page();page.goto(BASE+'?saved=station&section=library',wait_until='domcontentloaded');page.locator('[data-focus-key="gba:gba-0"]').click()
 page.get_by_role('dialog').get_by_role('button',name='Jouer',exact=True).click();page.get_by_role('dialog').get_by_role('button',name='Jouer',exact=True).click()
 page.locator('[data-testid="stream-route"]').wait_for();assert 'preview-session' in page.url
 assert page.evaluate('window.__launchArgs[0]')=='gba' and page.evaluate('window.__launchArgs[1]')=='gba-0'
 context.close();results.append('Launch contract retains original GBA reference and opens streaming; transport is simulated')
 context=browser.new_context();page=context.new_page();page.goto(BASE+'?saved=orbit&section=library',wait_until='domcontentloaded');page.locator('[data-focus-key="gba:gba-0"]').click();page.get_by_role('dialog').get_by_role('button',name='Jouer',exact=True).click()
 launch=page.locator('.gx-launch-dialog');launch.wait_for();assert not launch.evaluate('el=>el.classList.contains("gx-orbit")');box=launch.bounding_box();assert box['x']>=0 and box['width']<800
 page.get_by_role('dialog').get_by_role('button',name='Retour',exact=True).click();page.wait_for_function('document.activeElement.dataset.focusKey==="gba:gba-0"');context.close();results.append('Orbit launch confirmation keeps dialog geometry and returns focus')
 browser.close()
print(json.dumps({'passed':results,'previewOnly':True},ensure_ascii=False,indent=2))
