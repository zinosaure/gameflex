"""Run in the existing Chromium test image, against the isolated fixture preview."""
import json,os,sys
from urllib.parse import urlparse,parse_qs
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.10:18196/gameflex/')
results=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for old,section in [('favorites','favorites'),('resume','resume'),('collections','collections'),('detail','library'),('archives','management'),('multiplayer','multiplayer')]:
  context=browser.new_context(viewport={'width':1280,'height':900});page=context.new_page();errors=[]
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(BASE+'?saved='+old)
  page.wait_for_url(lambda url:parse_qs(urlparse(url).query).get('section')==[section])
  if old=='detail':page.get_by_role('dialog').wait_for();page.keyboard.press('Escape')
  page.wait_for_function("JSON.parse(localStorage.getItem('preview:preferences')||'{}').interfaceDestination === null")
  saved=page.evaluate("JSON.parse(localStorage.getItem('preview:preferences'))")
  assert saved['interface']=='station' and saved['previousInterface']==old and saved['interfaceDestination'] is None,saved
  assert len(saved['collections'])==1 and len(saved['favorites'])==1
  page.reload();page.wait_for_timeout(1200)
  assert page.locator('[aria-modal="true"]').count()==0,'Migration replay reopened a sheet'
  assert not errors,errors
  results.append('migration/reload '+old);print('passed '+old,file=sys.stderr,flush=True);context.close()
 context=browser.new_context(viewport={'width':1280,'height':900});page=context.new_page();errors=[]
 page.on('pageerror',lambda e:errors.append(str(e)))
 page.goto(BASE+'?section=library');card=page.locator('[data-focus-key="nes:palette"]').first;card.wait_for();card.click()
 page.get_by_role('dialog').wait_for();page.keyboard.press('Escape')
 assert page.evaluate("document.activeElement.dataset.focusKey")=='nes:palette'
 # Search, favorites and collection membership use the actual provider state.
 search=page.get_by_role('searchbox');search.fill('clavier');assert page.locator('[data-focus-key="nes:palette"]').count()==0
 search.fill('');card.click();page.get_by_role('button',name='Favori',exact=True).click();page.get_by_role('button',name='✓ Jeux de test',exact=True).click();page.keyboard.press('Escape')
 page.get_by_role('button',name='Favoris',exact=True).first.click();page.get_by_text('Aucun jeu ici',exact=True).wait_for()
 page.get_by_role('button',name='Collections',exact=True).first.click();page.get_by_role('button',name='＋ Créer',exact=True).wait_for()
 page.once('dialog',lambda d:d.accept('Nouvelle collection'));page.get_by_role('button',name='＋ Créer',exact=True).click();page.get_by_role('heading',name='Nouvelle collection',exact=True).wait_for()
 page.get_by_role('button',name='Gestion / Archives',exact=True).first.click();page.get_by_role('combobox',name='Filtrer par état').select_option('Unknown')
 assert page.locator('[data-focus-key="nes:palette"]').count()==0;assert page.locator('[data-focus-key="nes:test"]').count()==1
 page.get_by_role('combobox',name='Filtrer par état').select_option('');page.locator('[data-focus-key="nes:palette"]').click();page.get_by_role('button',name='Jouer',exact=True).click()
 page.get_by_role('heading',name='Palette libre',exact=True).wait_for();page.get_by_role('button',name='Retour',exact=True).click()
 assert page.evaluate("document.activeElement.dataset.focusKey")=='nes:palette','Sheet → launch → return lost original focus'
 page.locator('[data-focus-key="nes:palette"]').click();page.get_by_role('button',name='Jouer',exact=True).click();page.get_by_role('button',name='Jouer',exact=True).click()
 page.locator('[data-testid="stream-route"]').wait_for();assert 'preview-session' in page.url
 assert page.evaluate('window.__launchArgs[0]')=='nes';assert page.evaluate('window.__launchArgs[1]')=='palette'
 results.append('search/favorites/collections/archive filters/shared sheet/focus/launch contract')
 assert not errors,errors
 # Explicit routes take precedence over pending destinations; old links remain usable.
 context.close();context=browser.new_context(viewport={'width':390,'height':844});page=context.new_page()
 page.goto(BASE+'?saved=resume&section=favorites');page.get_by_role('heading',name='Épinglés').wait_for();assert 'section=favorites' in page.url
 page.goto(BASE+'?interface=archives');page.get_by_role('heading',name='Gestion / Archives',exact=True,level=1).wait_for()
 assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+2')
 page.goto(BASE+'?pack=1');page.locator('.gx-choice').nth(21).wait_for();assert page.locator('.gx-choice').count()==22
 assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+2')
 page.get_by_role('searchbox').fill('Grand écran accessible');assert page.locator('.gx-choice').count()==1
 results.append('deep links/subpath/mobile/22-entry selector search')
 browser.close()
report={'passed':results,'publicWrites':0,'launch':'simulated transport; actual emulator tested separately'}
print(json.dumps(report,ensure_ascii=False))
