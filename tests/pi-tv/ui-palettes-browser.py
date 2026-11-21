"""Compare the original home CSS with shared surfaces, across all 22 modes."""
import json,os,sys
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://127.0.0.1:18197/gameflex/')
MODES=['station','crt','shelf','orbit','arcade','minimal','covers','consoles','dashboard','hub-canape','ligne-cinema','mosaique-joy','roue-retro','terminal-poche','cartouches','carte-epoques','double-ecran','club-local','quai-systemes','magazine-retro','bureau-collectionneur','grand-ecran-accessible']
FORM=os.environ.get('INTERFACE_FORMAT','pc');VIEWPORT={'pc':{'width':1440,'height':1000},'mobile':{'width':390,'height':844},'tv':{'width':1920,'height':1080}}[FORM]
def navigate(page,path):
 page.evaluate('(path)=>{history.pushState({},"",path);dispatchEvent(new PopStateEvent("popstate"))}',path)
rows=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 context=browser.new_context(viewport=VIEWPORT);page=context.new_page();page.set_default_timeout(12000)
 page.goto(BASE+'settings?saved=station',wait_until='domcontentloaded');page.locator('.gx-settings').wait_for()
 for mode in MODES:
  page.locator('.gx-settings').get_by_role('button',name='Changer d’interface',exact=True).click()
  page.locator(f'button.gx-choice.gf-choice-{mode},button.gx-choice.gx-{mode}').click()
  navigate(page,'/gameflex?section=home');page.locator('.gx-settings').wait_for(state='detached');page.locator(f'[data-interface="{mode}"]').first.wait_for()
  original=page.locator(f'[data-interface="{mode}"]').first.evaluate('''el=>{
   const styles=[...document.querySelectorAll('style')].filter(s=>s.textContent.includes('Palette and screen composition are independent'));
   const actualImage=getComputedStyle(el).backgroundImage;
   styles.forEach(s=>s.sheet.disabled=true);
   const style=getComputedStyle(el);const result={color:style.color,background:style.backgroundColor,image:style.backgroundImage,actualImage,base:style.getPropertyValue('--bg').trim()};
   styles.forEach(s=>s.sheet.disabled=false);return result;
  }''')
  canonical=page.locator('[data-appearance]').evaluate('el=>({color:getComputedStyle(el).color,background:getComputedStyle(el).backgroundColor,palette:getComputedStyle(el).getPropertyValue("--dialog-bg").trim()})')
  assert original['image']==original['actualImage'],(mode,'home gradient changed',original)
  assert original['color']==canonical['color'],(mode,'home foreground',original,canonical)
  if original['base']:assert original['base'].lower()==canonical['palette'].lower(),(mode,'home base',original,canonical)
  if original['background']!='rgba(0, 0, 0, 0)':assert original['background']==canonical['background'],(mode,'home background',original,canonical)
  navigate(page,'/gameflex?section=library');page.locator('.em-installed-total').wait_for()
  shared=page.locator('.gx-app').evaluate('el=>({color:getComputedStyle(el).color,background:getComputedStyle(el).backgroundColor})')
  assert shared['color']==canonical['color'] and shared['background']==canonical['background'],(mode,shared,canonical)
  page.locator('[data-focus-key="gba:gba-0"]').click();sheet=page.locator('.gf-catalogue-dialog');sheet.wait_for()
  dialog=sheet.evaluate('el=>({color:getComputedStyle(el).color,background:getComputedStyle(el).backgroundColor})')
  assert dialog==shared,(mode,'sheet',dialog,shared)
  page.get_by_role('dialog').get_by_role('button',name='Retour',exact=True).click()
  navigate(page,'/gameflex/settings');page.locator('.gx-settings').wait_for()
  settings=page.locator(f'[data-interface="{mode}"]').first.evaluate('el=>getComputedStyle(el).color')
  assert settings==canonical['color'],(mode,'settings',settings,canonical)
  rows.append({'id':mode,'format':FORM,'originalHome':original,'palette':canonical['palette'],'foreground':canonical['color'],'shared':shared,'sheet':dialog,'settingsForeground':settings,'passed':True})
  print(FORM+' palette passed '+mode,file=sys.stderr,flush=True)
 context.close();browser.close()
print(json.dumps(rows,ensure_ascii=False,indent=2))
