"""Supplied console artwork: path, full aspect ratio and navigation, PC/mobile/TV."""
import json,os
from pathlib import Path
from urllib.parse import urljoin
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18232/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
reports=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for name,width,height in [('pc',1440,1000),('mobile',390,844),('tv',1920,1080),('tv4k',3840,2160)]:
  page=browser.new_page(viewport={'width':width,'height':height},has_touch=name=='mobile');errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(BASE+'?showcase=1'+('&tv=1' if name.startswith('tv') else ''))
  page.locator('.ps-console-logo-tile').first.wait_for()
  page.wait_for_function('Array.from(document.querySelectorAll(".ps-console-tile .gx-console-icon img")).length===2&&Array.from(document.querySelectorAll(".ps-console-tile .gx-console-icon img")).every(i=>i.complete&&i.naturalWidth>0)')
  assert page.locator('.ps-console-tile').count()==2
  values=page.locator('.ps-console-tile .gx-console-icon img').evaluate_all('images=>images.map(i=>{const r=i.getBoundingClientRect();return {src:i.getAttribute("src"),ratio:r.width/r.height,natural:i.naturalWidth/i.naturalHeight,fit:getComputedStyle(i).objectFit}})')
  for v in values:
   assert v['src'].startswith('/gameflex/images/console-icons/'),v
   assert abs(v['ratio']-v['natural'])<.02,v
   assert v['fit']=='contain',v
   response=page.request.get(urljoin(BASE,v['src'].replace('/gameflex/','/',1)));assert response.ok and response.headers['content-type']=='image/webp'
  ds=page.locator('[data-system="nds"]');ds.focus();ds.press('ArrowRight');page.get_by_role('heading',name='Game Boy Advance',exact=True).wait_for()
  assert page.locator('[data-system="gba"]').get_attribute('aria-pressed')=='true'
  assert page.locator('.ps-game-row').count()==4
  assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+1')
  if name in ['pc','mobile']:page.screenshot(path=str(OUT/f'console-icons-{name}.png'),full_page=True)
  assert not errors,errors
  reports.append({'viewport':name,'dimensions':[width,height],'completeArtwork':True,'navigation':True,'gameRowsPreserved':True,'errors':errors})
  page.close()
 browser.close()
(OUT/'console-icons-browser.json').write_text(json.dumps({'passed':True,'formats':reports},indent=2)+'\n');print(json.dumps({'passed':True,'formats':[r['viewport'] for r in reports]}))
