import io,json,os,sys
from pathlib import Path
from playwright.sync_api import sync_playwright
from PIL import Image
BASE=os.environ.get('INTERFACE_PREVIEW','http://127.0.0.1:18197/gameflex/')
OUT=Path(os.environ.get('INTERFACE_SCREENSHOTS','/screens'));OUT.mkdir(parents=True,exist_ok=True)
results=[]
def choose(page,name):
 page.get_by_role('button',name='Changer d’interface',exact=True).first.click()
 page.locator('.gx-choice').filter(has=page.get_by_text(name,exact=True)).click()
def shot(page,name):
 image=Image.open(io.BytesIO(page.screenshot())).convert('RGB');image.thumbnail((1280,2000));image.save(OUT/name,format='WEBP',quality=82)
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 # Reproduce the reported switching order with real kernel-shaped statuses.
 context=browser.new_context(viewport={'width':1440,'height':1000});page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 page.goto(BASE+'?saved=station&section=library&objectStatus=1',wait_until='domcontentloaded')
 page.locator('[data-focus-key="gba:gba-0"]').wait_for()
 for mode,name in [('shelf','La collection'),('crt','Signal CRT'),('minimal','Minimal')]:
  choose(page,name);page.locator(f'[data-appearance="{mode}"]').wait_for();page.locator('[data-focus-key="gba:gba-0"]').wait_for()
  assert 'section=library' in page.url
  assert page.get_by_text('Affichage interrompu',exact=True).count()==0
 page.get_by_role('button',name='Favoris',exact=True).first.click();page.get_by_role('button',name='Tous les menus',exact=True).click();page.get_by_role('dialog').get_by_role('button',name='Gestion / Archives',exact=True).click()
 page.get_by_role('heading',name='Gestion / Archives',exact=True,level=1).wait_for();page.reload();page.get_by_role('heading',name='Gestion / Archives',exact=True,level=1).wait_for()
 assert not errors,errors
 results.append('Horizon → La collection → Signal CRT → Minimal in Mes jeux; Favorites → Archives; deep-link reload')
 context.close()
 # Reference route uses 432 synthetic references; never read or write real ROMs.
 context=browser.new_context(viewport={'width':1440,'height':1000});page=context.new_page()
 page.goto(BASE+'retrogaming/gba?saved=shelf&scenario=large&objectStatus=1',wait_until='domcontentloaded')
 page.get_by_text('432 références installées',exact=True).wait_for();search=page.get_by_label('Rechercher dans mes jeux',exact=True);search.fill('Advance Wars')
 page.get_by_text('2 résultats',exact=True).wait_for();assert page.get_by_text('432 références installées',exact=True).count()==1
 card=page.locator('[data-focus-key="gba:gba-0"]');card.click();dialog=page.get_by_role('dialog');dialog.get_by_role('heading',name='Advance Wars',exact=True).wait_for()
 assert dialog.get_by_text('Europe',exact=True).count()==1 and dialog.get_by_text('Anglais / Français',exact=True).count()==1
 dialog.get_by_role('button',name='Retour',exact=True).click();page.wait_for_function('document.activeElement.dataset.focusKey==="gba:gba-0"');assert search.input_value()=='Advance Wars'
 shot(page,'la-collection-gba.webp')
 opener=page.get_by_role('button',name='Changer d’interface',exact=True);opener.click();page.keyboard.press('Escape');assert search.input_value()=='Advance Wars'
 assert page.evaluate('document.activeElement.getAttribute("aria-label")')=='Changer d’interface'
 choose(page,'Signal CRT');page.locator('[data-appearance="crt"]').wait_for();assert page.locator('input').first.input_value()=='Advance Wars'
 choose(page,'Minimal');page.locator('[data-appearance="minimal"]').wait_for();assert page.locator('input').first.input_value()=='Advance Wars'
 results.append('La collection → GBA → Advance Wars → sheet → return: 432 installed / 2 results, short title, edition metadata, filter/focus, cancel and explicit theme switches')
 context.close()
 # Empty/loading/error are separate states even before the console registry loads.
 for mode in ['hub-canape','ligne-cinema','mosaique-joy']:
  context=browser.new_context();context.add_init_script("window.__prematureEmpty=false;new MutationObserver(()=>{if(document.body?.innerText.includes('Votre bibliothèque est vide'))window.__prematureEmpty=true}).observe(document,{childList:true,subtree:true});")
  page=context.new_page();page.goto(BASE+f'?saved={mode}&romDelay=900&consoleDelay=700',wait_until='domcontentloaded')
  page.get_by_text('Chargement de la bibliothèque…',exact=True).wait_for();page.locator(f'[data-interface="{mode}"]').wait_for()
  assert not page.evaluate('window.__prematureEmpty'),mode
  context.close();results.append('no premature empty: '+mode)
 context=browser.new_context();page=context.new_page();page.goto(BASE+'?saved=hub-canape&libraryError=1',wait_until='domcontentloaded')
 page.get_by_role('alert').wait_for();assert page.get_by_text('Votre bibliothèque est vide',exact=True).count()==0
 page.get_by_role('button',name='Menu principal',exact=True).click();page.get_by_role('dialog').get_by_role('button',name='Mes jeux',exact=True).click();page.get_by_text('Total indisponible',exact=True).wait_for();page.get_by_role('button',name='Accueil',exact=True).first.click()
 page.evaluate('window.__previewConfig.libraryError=false');page.get_by_role('button',name='Réessayer',exact=True).click();page.locator('.gf-hub-hero').wait_for();context.close()
 context=browser.new_context();page=context.new_page();page.goto(BASE+'?saved=hub-canape&empty=1',wait_until='domcontentloaded');page.get_by_text('Votre bibliothèque est vide',exact=True).wait_for();context.close()
 results.append('library error/retry and genuinely empty library')
 # Fault only a rendered cell, not the parent shell; retry obtains a repaired fixture.
 context=browser.new_context();page=context.new_page();page.goto(BASE+'?saved=minimal&section=library&faultOnce=1',wait_until='domcontentloaded')
 page.get_by_text('Affichage interrompu',exact=True).wait_for();assert page.get_by_role('button',name='Tous les menus',exact=True).is_visible();page.get_by_role('button',name='Tous les menus',exact=True).click();assert page.get_by_role('dialog').get_by_role('button',name='Gestion / Archives',exact=True).is_visible();page.keyboard.press('Escape')
 page.get_by_role('button',name='Réessayer sur place',exact=True).click();page.locator('[data-focus-key="gba:gba-0"]').wait_for();assert page.get_by_text('Affichage interrompu',exact=True).count()==0
 context.close();results.append('section error retains main menu and retries in place')
 # Last explicit selection persists synchronously; stale responses and offline reloads cannot win.
 context=browser.new_context();page=context.new_page();page.goto(BASE+'settings?saved=station',wait_until='domcontentloaded');page.locator('.gx-settings').wait_for();choose(page,'La collection')
 page.evaluate("localStorage.setItem('preview:preferences',JSON.stringify({...JSON.parse(localStorage.getItem('preview:preferences')||'{}'),interface:'crt'}))")
 page.goto(BASE+'settings?preferenceDelay=1800',wait_until='domcontentloaded');page.locator('[data-appearance="shelf"]').wait_for();choose(page,'Minimal');page.wait_for_timeout(2600)
 assert page.locator('[data-appearance="minimal"]').count()==1
 page.goto(BASE+'settings?preferenceOffline=1',wait_until='domcontentloaded');page.locator('[data-appearance="minimal"]').wait_for();choose(page,'La collection');page.reload(wait_until='domcontentloaded');page.locator('[data-appearance="shelf"]').wait_for();context.close()
 results.append('slow/out-of-order preferences; explicit local choice; unavailable sync and offline reload')
 # Touch + keyboard/directional access through the Joy Plus menu.
 context=browser.new_context(viewport={'width':390,'height':844},has_touch=True);page=context.new_page();page.goto(BASE+'?saved=mosaique-joy',wait_until='domcontentloaded')
 plus=page.get_by_role('button',name='Plus',exact=True);plus.wait_for();plus.tap();menu=page.get_by_role('dialog',name='Menu principal');menu.wait_for()
 assert menu.get_by_role('button',name='Paramètres',exact=True).count()==1 and menu.get_by_role('button',name='Changer d’interface',exact=True).count()==1
 page.keyboard.press('ArrowDown');assert menu.locator(':focus').count()==1
 shot(page,'mosaique-plus-mobile.webp');page.keyboard.press('Escape');assert page.evaluate('document.activeElement.textContent.includes("Plus")')
 assert 'section=collections' not in page.url
 context.close();results.append('Joy Plus is an actual touch/keyboard/directional menu; close restores focus')
 context=browser.new_context(viewport={'width':1920,'height':1080});page=context.new_page();page.goto(BASE+'?saved=crt&section=management&objectStatus=1',wait_until='domcontentloaded');page.get_by_role('heading',name='Archives',exact=True).wait_for();shot(page,'archives-crt-tv.webp');context.close()
 browser.close()
print(json.dumps({'passed':results,'physicalPhilipsTest':False,'previewOnly':True},ensure_ascii=False,indent=2))
