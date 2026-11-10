"""Regression journeys for the single experience. Fixture transport only."""
import json,os
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18228/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
rows=[]
def go(page,label):
 page.get_by_role('button',name='Ouvrir le centre de contrôle',exact=True).click()
 page.get_by_role('dialog',name='Centre de contrôle').get_by_role('button',name=label,exact=False).click()
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for form,width,height in [('pc',1440,1000),('mobile',390,844),('tv',1920,1080)]:
  context=browser.new_context(viewport={'width':width,'height':height},has_touch=form=='mobile',locale='fr-FR')
  page=context.new_page();errors=[];page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(BASE+'?saved=shelf');page.locator('.ps-game-tile').first.wait_for();page.wait_for_function('!document.querySelector(".ps-loading")')
  assert page.locator('[data-appearance="ps-home"]').count()==1
  opener=page.get_by_role('button',name='Ouvrir le centre de contrôle');opener.focus();opener.press('Enter')
  page.get_by_role('dialog').wait_for();page.keyboard.press('Escape');page.wait_for_function('document.activeElement.getAttribute("aria-label")==="Ouvrir le centre de contrôle"')
  tile=page.locator('.ps-game-tile').first;tile.focus();tile.press('ArrowRight');assert page.locator('.ps-game-tile[aria-pressed=true]').count()==1
  go(page,'Bibliothèque');page.get_by_role('heading',name='Votre bibliothèque',exact=True).wait_for()
  total=page.locator('.em-installed-total').inner_text();search=page.get_by_role('searchbox');search.fill('Advance Wars')
  page.get_by_text('2 résultats',exact=True).wait_for();assert page.locator('.em-installed-total').inner_text()==total
  page.get_by_label('Filtrer par console',exact=True).select_option('gba')
  card=page.locator('[data-focus-key="gba:gba-0"]');card.click();sheet=page.get_by_role('dialog');sheet.get_by_role('heading',name='Advance Wars',exact=True).wait_for()
  assert page.locator('[data-interface-theme="ps-home"]').count()==1
  sheet.get_by_role('button',name='Retour',exact=True).click();page.wait_for_function('document.activeElement.dataset.focusKey==="gba:gba-0"')
  assert search.input_value()=='Advance Wars';assert page.get_by_label('Filtrer par console',exact=True).input_value()=='gba'
  page.locator('.ps-favorite').first.click();go(page,'Favoris');assert page.locator('[data-focus-key="gba:gba-0"]').count()==1
  for label in ['Récents','Collections','Consoles','Multijoueur','Gestion / Archives','Paramètres']:
   go(page,label);page.locator('.ps-page-heading h1').wait_for()
   assert 'Affichage interrompu' not in page.locator('body').inner_text()
   assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+1')
   rows.append({'format':form,'section':label,'passed':True})
  go(page,'Collections');page.get_by_role('button',name='＋ Créer une collection',exact=True).click()
  page.get_by_role('dialog',name='Nouvelle collection').get_by_role('textbox').fill('Soirées rétro')
  page.get_by_role('dialog',name='Nouvelle collection').get_by_role('button',name='Créer',exact=True).click()
  page.get_by_role('heading',name='Soirées rétro',exact=True).wait_for()
  go(page,'Paramètres')
  page.get_by_text('Interfaces archivées · 22' ,exact=True).click();assert page.get_by_text('Mosaïque Joy',exact=True).count()==1
  assert page.get_by_role('button',name='Changer d’interface',exact=True).count()==0
  page.reload();page.locator('[data-appearance="ps-home"]').wait_for()
  page.get_by_role('link',name='Compte et bibliothèque ↗',exact=True).click();page.locator('.gf-advanced-content').wait_for();assert 'advanced=1' in page.url
  assert page.locator('.ps-topbar').count()==1
  page.goto(BASE+'?section=management');page.get_by_role('heading',name='Gestion / Archives',exact=True).wait_for();page.reload();page.get_by_role('heading',name='Gestion / Archives',exact=True).wait_for()
  # Launch stays on the shared sheet and emulator pipeline, with original reference.
  page.goto(BASE+'?section=library');page.locator('[data-focus-key="gba:gba-0"]').click()
  page.get_by_role('dialog').get_by_role('button',name='Jouer',exact=True).click()
  page.get_by_role('dialog').get_by_role('button',name='Jouer',exact=True).click()
  page.locator('[data-testid="stream-route"]').wait_for()
  assert 'preview-session' in page.url
  assert page.evaluate('window.__launchArgs[0]')=='gba'
  assert not errors,errors
  # Visual fixtures use only four already-cached covers, with their credits.
  if os.environ.get('PREVIEW_HAS_CACHED_ART')!='1':
   rows.append({'format':form,'functionalJourney':True,'passed':True});context.close();continue
  page.goto(BASE+'?showcase=1');page.locator('.ps-feature-cover img').wait_for();page.wait_for_function('document.querySelector(".ps-feature-cover img").naturalWidth>0');page.wait_for_timeout(300)
  assert page.locator('.ps-feature-cover .gx-credit').count()==1
  page.screenshot(path=str(OUT/f'home-{form}.png'),full_page=True)
  go(page,'Bibliothèque');page.locator('.ps-library-cover img').first.wait_for();page.wait_for_timeout(500);page.screenshot(path=str(OUT/f'library-{form}.png'),full_page=True)
  assert not errors,errors
  rows.append({'format':form,'journey':'home / arrows / menu focus / library / search / filters / sheet / return focus / favorite / common sections / settings archives / advanced settings / reload / credited images','passed':True})
  context.close()
 # Loading, empty and unavailable states must stay distinct.
 for scenario in ['romDelay=1800','empty=1','libraryError=1']:
  context=browser.new_context();page=context.new_page();page.goto(BASE+'?'+scenario)
  if scenario.startswith('romDelay'):
   page.get_by_role('status',name='').filter(has_text='Chargement').first.wait_for();assert page.get_by_text('Votre bibliothèque prend vie ici',exact=True).count()==0
  elif scenario=='empty=1':page.get_by_text('Votre bibliothèque prend vie ici',exact=True).wait_for()
  else:page.get_by_role('alert').first.wait_for();assert page.get_by_role('button',name='Réessayer',exact=True).count()>0
  rows.append({'scenario':scenario,'passed':True});context.close()
 browser.close()
(OUT/'browser-results.json').write_text(json.dumps(rows,ensure_ascii=False,indent=2))
print(json.dumps({'passed':len(rows),'formats':['pc','mobile','tv'],'publicWrites':False}))
