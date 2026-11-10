"""Shared TV app at the Philips CSS viewport, with the APK remote translation."""
import os,json
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18232/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
 b=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage']);page=b.new_page(viewport={'width':960,'height':540})
 page.goto(BASE+'?showcase=1&tv=1');page.locator('.ps-console-tile').first.wait_for();page.wait_for_function('document.body.classList.contains("gf-tv-mode")')
 page.add_script_tag(path='/app/deploy/pi-tv/android/app/src/main/assets/tv-navigation.js');page.evaluate("window.__outerNav=0;window.addEventListener('keydown',()=>window.__outerNav++)");page.evaluate("document.getElementById('root').tabIndex=-1;document.getElementById('root').focus()");page.evaluate("GameFlexTv.move('right')")
 page.get_by_role('heading',name='Game Boy Advance',exact=True).wait_for();assert page.evaluate('window.__outerNav')==0,'Remote must not trigger a second global navigator'
 assert page.locator('.ps-game-row').first.bounding_box()['y']<540,'Game list must be visible on the Philips viewport'
 page.get_by_role('button',name='Lancer Advance Wars',exact=True).focus();page.evaluate('GameFlexTv.ok()');page.locator('[data-testid="stream-route"]').wait_for();page.evaluate('GameFlexTv.returnToLibrary()');page.locator('.ps-console-tile').first.wait_for()
 assert page.locator('[data-system="gba"]').get_attribute('aria-pressed')=='true'
 page.get_by_role('button',name='Paramètres',exact=True).focus();page.evaluate('GameFlexTv.ok()');page.get_by_role('button',name='Désactiver le mode TV',exact=True).wait_for()
 assert 'Affichage interrompu' not in page.locator('body').inner_text();b.close()
(OUT/'tv-native-navigation.json').write_text(json.dumps({'passed':True,'cssViewport':[960,540],'remoteNavigation':True,'gameListVisible':True,'launch':'simulated','backRestoresConsole':True,'settings':True},indent=2)+'\n');print('Native remote bridge: console, launch, return and settings passed at 960×540')
