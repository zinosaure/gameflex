"""Compact phone regressions. Pair layout uses a simulated input-only socket;
real pairing/approval remains covered by tv-phone-browser.py."""
import json,os
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18232/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
mock="""sessionStorage.setItem('gameflex:phone-pad',JSON.stringify({id:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',phoneToken:'layout-fixture-capability-not-real'}));window.WebSocket=class {constructor(){this.readyState=1;setTimeout(()=>this.onopen?.({}),30);}send(text){if(JSON.parse(text).role)setTimeout(()=>this.onmessage?.({data:JSON.stringify({type:'status',approved:true,hostConnected:true,epoch:1,screen:'library'})}),30);}close(){this.readyState=3;}};"""
geometry="""selector=>[...document.querySelectorAll(selector)].filter(e=>e.getClientRects().length).map(e=>{const r=e.getBoundingClientRect();const hit=document.elementFromPoint(r.x+r.width/2,r.y+r.height/2);return {label:e.getAttribute('aria-label')||e.innerText,x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom,hit:e===hit||e.contains(hit)}})"""
reports=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for width,height in [(320,740),(360,800),(390,844),(430,932),(568,320),(667,375),(844,390),(932,430)]:
  ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=True,is_mobile=True);ctx.add_init_script("localStorage.setItem('gameflex:tv-mode','1')")
  page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(BASE+'?showcase=1');page.locator('.ps-console-tile').first.wait_for()
  assert not page.evaluate('document.body.classList.contains("gf-tv-mode")'),(width,height,'TV styles on phone')
  assert page.evaluate('localStorage.getItem("gameflex:tv-mode")')=='1'
  values=page.evaluate(geometry,".ps-utilities button,.ps-tabs button,.ps-row-actions button")
  assert all(v['x']>=0 and v['right']<=width+1 for v in values),(width,values)
  page.locator('.ps-avatar').click();page.get_by_role('dialog',name='Centre de contrôle').wait_for()
  values=page.evaluate(geometry,'.ps-control button');assert all(v['x']>=0 and v['right']<=width+1 for v in values),(width,values)
  page.get_by_role('button',name='Fermer le centre de contrôle',exact=True).click()
  if width==390:page.screenshot(path=str(OUT/'mobile-library-fixed.png'),full_page=True)
  page.set_viewport_size({'width':1920,'height':1080});page.wait_for_function('document.body.classList.contains("gf-tv-mode")')
  page.set_viewport_size({'width':width,'height':height});page.wait_for_function('!document.body.classList.contains("gf-tv-mode")')
  assert not errors,errors;ctx.close()
  ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=True,is_mobile=True);ctx.add_init_script(mock);page=ctx.new_page()
  page.goto(BASE+'controller');page.get_by_role('main',name='Manette tactile').wait_for();page.add_style_tag(content='#root>aside{display:none}')
  values=page.evaluate(geometry,'.gf-pad-key,.gf-pad-stick,.gf-phone-header>button,.gf-pad-fullscreen,.gf-device-roles button')
  assert all(v['x']>=0 and v['right']<=width+1 and v['y']>=0 and v['bottom']<=height+1 and v['hit'] for v in values),(width,height,values)
  for i,a in enumerate(values):
   for b in values[i+1:]:
    overlap=min(a['right'],b['right'])-max(a['x'],b['x']);vertical=min(a['bottom'],b['bottom'])-max(a['y'],b['y'])
    assert overlap<1 or vertical<1,(width,height,'overlap',a,b)
  assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+1')
  if (width,height) in [(390,844),(568,320)]:page.screenshot(path=str(OUT/f'phone-pad-fixed-{width}.png'),full_page=True)
  reports.append({'viewport':[width,height],'storedTvPreferencePreserved':True,'compactLibrary':True,'controllerTargetsVisible':True,'controllerTargetsDoNotOverlap':True});ctx.close()
 browser.close()
(OUT/'mobile-interface-results.json').write_text(json.dumps({'passed':True,'socket':'simulated for layout only','formats':reports},indent=2)+'\n');print(json.dumps({'passed':True,'viewports':len(reports)}))
