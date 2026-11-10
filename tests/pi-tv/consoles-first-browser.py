"""Installed consoles, native artwork, direct launch and emulator onboarding."""
import json,os
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18229/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
results=[]
def go(page,label):
 page.get_by_role('button',name='Ouvrir le centre de contrôle',exact=True).click()
 page.get_by_role('dialog',name='Centre de contrôle').get_by_role('button',name=label,exact=False).click()
def ready(page):
 page.locator('.ps-console-tile').first.wait_for();page.wait_for_function('!document.querySelector(".ps-loading")')
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for form,width,height in [('pc',1440,1000),('mobile',390,844),('tv',1920,1080)]:
  context=browser.new_context(viewport={'width':width,'height':height},has_touch=form=='mobile',locale='fr-FR');page=context.new_page();errors=[]
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(BASE+'?showcase=1');ready(page)
  assert page.locator('.ps-console-tile').count()==2
  assert page.locator('[data-system="nes"],[data-system="n64"]').count()==0,'Empty consoles must be hidden'
  page.get_by_role('button',name='Sélectionner Nintendo DS',exact=True).click()
  page.locator('.ps-row-art img').wait_for();page.wait_for_function('document.querySelector(".ps-row-art img").naturalWidth>0')
  dimensions=page.locator('.ps-row-art img').first.evaluate('image=>({natural:image.naturalWidth/image.naturalHeight,rendered:image.getBoundingClientRect().width/image.getBoundingClientRect().height,fit:getComputedStyle(image).objectFit})')
  assert abs(dimensions['natural']-dimensions['rendered'])<.02,dimensions
  assert dimensions['fit']=='contain'
  page.screenshot(path=str(OUT/f'consoles-ds-{form}.png'),full_page=True)
  tile=page.get_by_role('button',name='Sélectionner Game Boy Advance',exact=True);tile.focus();tile.press('Enter')
  page.get_by_role('heading',name='Game Boy Advance',exact=True).wait_for();assert page.locator('.ps-game-row').count()==4
  assert page.get_by_role('button',name='Lancer Advance Wars',exact=True).count()==1
  page.locator('.ps-favorite').first.click()
  more=page.get_by_role('button',name='Paramètres de Advance Wars',exact=True);more.click()
  dialog=page.get_by_role('dialog');dialog.get_by_role('heading',name='Advance Wars',exact=True).wait_for()
  dialog.get_by_label('Émulateur utilisé').select_option('fixture-alternative');dialog.get_by_role('button',name='Enregistrer',exact=True).click()
  page.wait_for_function('document.activeElement.dataset.focusKey==="settings:gba:gba-0"')
  search=page.get_by_role('searchbox');search.fill('Advance Wars');assert page.locator('.ps-game-row').count()==1
  page.get_by_role('button',name='Fiche : Advance Wars',exact=True).click();page.get_by_role('dialog').get_by_role('button',name='Retour',exact=True).click()
  page.wait_for_function('document.activeElement.dataset.focusKey==="gba:gba-0"');assert search.input_value()=='Advance Wars'
  search.fill('');page.locator('.ps-game-row').last.scroll_into_view_if_needed();page.wait_for_function('Array.from(document.querySelectorAll(".ps-row-art img")).length===4&&Array.from(document.querySelectorAll(".ps-row-art img")).every(image=>image.complete&&image.naturalWidth>0)');page.locator('.ps-console-heading h1').click();page.evaluate('window.scrollTo(0,0)');page.screenshot(path=str(OUT/f'consoles-gba-{form}.png'),full_page=True)
  launch=page.get_by_role('button',name='Lancer Advance Wars',exact=True)
  launch.evaluate('button=>{button.click();button.click()}')
  assert page.locator('.gx-launch-dialog').count()==0,'Direct launch must not open confirmation'
  page.locator('[data-testid="stream-route"]').wait_for()
  assert page.evaluate('window.__sessionCalls')==1,'Double click must not create two sessions'
  args=page.evaluate('window.__launchArgs');assert args[0]=='gba' and args[1]=='gba-0' and args[3]=='fixture-alternative',args
  page.goto(BASE+'?showcase=1');ready(page);assert page.locator('[data-system="gba"]').get_attribute('aria-pressed')=='true'
  go(page,'Bibliothèque');page.get_by_role('heading',name='Votre bibliothèque',exact=True).wait_for()
  total=page.locator('.em-installed-total').inner_text();page.get_by_role('searchbox').fill('Advance Wars');page.get_by_text('2 résultats',exact=True).wait_for();assert page.locator('.em-installed-total').inner_text()==total
  for label in ['Favoris','Récents','Collections','Consoles','Multijoueur','Gestion / Archives','Paramètres']:
   go(page,label);page.locator('.ps-page-heading h1').wait_for();assert 'Affichage interrompu' not in page.locator('body').inner_text()
   assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),label
  assert not errors,errors
  results.append({'format':form,'nativeCover':dimensions,'ownedConsolesOnly':True,'rowActions':True,'optionsPersisted':True,'focusRestored':True,'directLaunch':True,'doubleClickGuard':True,'commonSections':True,'passed':True});context.close()
 # An absent emulator remains actionable above existing games, not an empty library.
 for scenario in ['missingEmulator=1','missingEmulator=1&readOnly=1','launchFailure=1','emulatorOffline=1']:
  context=browser.new_context();page=context.new_page();page.goto(BASE+'?showcase=1&'+scenario);ready(page)
  page.get_by_role('button',name='Sélectionner Game Boy Advance',exact=True).click();assert page.locator('.ps-game-row').count()==4
  if scenario.startswith('missingEmulator'):
   banner=page.get_by_role('complementary',name='Émulateur manquant');banner.wait_for();button=banner.get_by_role('button',name='Télécharger mon émulateur',exact=True)
   assert page.evaluate('window.__requests.filter(request=>request.path.endsWith("/emulators/gba/install")).length')==0,'Never install on navigation'
   if 'readOnly' in scenario:assert button.is_disabled();assert 'administrateur' in banner.inner_text()
   else:
    page.screenshot(path=str(OUT/'emulator-banner.png'),full_page=True);button.click();page.wait_for_function('window.__emuInstalled===true');banner.wait_for(state='detached')
    assert page.evaluate('window.__requests.filter(request=>request.path.endsWith("/emulators/gba/install")).length')==1
  elif scenario=='launchFailure=1':
   page.get_by_role('button',name='Lancer Advance Wars',exact=True).click();page.get_by_role('alert').filter(has_text='Lancement refusé').wait_for();assert page.locator('.gx-launch-dialog').count()==0
   page.wait_for_timeout(500);assert not page.evaluate('(JSON.parse(localStorage.getItem("preview:preferences")||"{}").history||[]).includes("gba:gba-0")')
  else:
   page.get_by_role('alert').filter(has_text='Vérification de l’émulateur indisponible').wait_for();assert page.get_by_role('complementary',name='Émulateur manquant').count()==0
  results.append({'scenario':scenario,'passed':True});context.close()
 browser.close()
(OUT/'consoles-first-results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'passed':len(results),'formats':['pc','mobile','tv'],'directLaunchTransport':'simulated','emulatorInstallation':'simulated'}))
