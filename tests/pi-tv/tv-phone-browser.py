"""Two independent browsers, real HTTP/WebSocket pairing, simulated game sink."""
import json,os,re
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18230/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
 launch={'executable_path':'/usr/bin/chromium','headless':True,'args':['--no-sandbox','--disable-dev-shm-usage']}
 tvbrowser=p.chromium.launch(**launch);phonebrowser=p.chromium.launch(**launch)
 tv=tvbrowser.new_page(viewport={'width':1920,'height':1080});phone=phonebrowser.new_page(viewport={'width':844,'height':390},has_touch=True,is_mobile=True)
 requests=[]
 tv.on('request',lambda request:requests.append(request.url));phone.on('request',lambda request:requests.append(request.url))
 errors=[];tv.on('pageerror',lambda error:errors.append(str(error)));phone.on('pageerror',lambda error:errors.append(str(error)))
 tv.goto(BASE+'?showcase=1&tv=1');tv.locator('.ps-console-tile').first.wait_for();tv.wait_for_function('document.body.classList.contains("gf-tv-mode")')
 assert tv.evaluate('getComputedStyle(document.querySelector(".ps-row-title h3")).fontSize')=='25px'
 tv.screenshot(path=str(OUT/'tv-library.png'),full_page=True)
 tv.get_by_role('button',name='Manette téléphone',exact=True).click();tv.get_by_role('button',name='Afficher un code',exact=True).click();tv.locator('.gf-tv-code').wait_for()
 code=re.sub(r'\D','',tv.locator('.gf-tv-code').inner_text());assert len(code)==8
 tv.screenshot(path=str(OUT/'tv-pairing.png'))
 phone.goto(BASE+'controller');phone.get_by_label('Code affiché sur la TV',exact=True).fill(code)
 phone.get_by_role('button',name='Associer à ma TV',exact=True).click();phone.get_by_text('Regardez la télévision',exact=True).wait_for()
 assert phone.get_by_role('main',name='Manette tactile').count()==0
 tv.get_by_role('button',name='Autoriser ce téléphone',exact=True).wait_for();tv.get_by_role('button',name='Autoriser ce téléphone',exact=True).click()
 phone.get_by_role('main',name='Manette tactile').wait_for();tv.get_by_role('button',name='Continuer sur l’écran',exact=True).click()
 tv.locator('[data-system="nds"]').focus()
 right=phone.get_by_role('button',name='→',exact=True);right.hover();phone.mouse.down();tv.wait_for_function('document.querySelector("[data-system=gba]").getAttribute("aria-pressed")==="true"');phone.mouse.up()
 tv.get_by_role('button',name='Paramètres',exact=True).click();phone.get_by_text('Réglages du compte : utilisez la télécommande TV. B pour revenir.',exact=True).wait_for()
 tv.get_by_role('button',name='Désactiver le mode TV',exact=True).focus();phone.get_by_role('button',name='A',exact=True).click();phone.wait_for_timeout(250);assert tv.evaluate('document.body.classList.contains("gf-tv-mode")')
 phone.get_by_role('button',name='B',exact=True).click();tv.locator('.ps-console-tile').first.wait_for()
 tv.get_by_role('button',name='Lancer Advance Wars',exact=True).click();tv.locator('[data-testid="stream-route"]').wait_for()
 phone.get_by_text('La partie est sur votre télévision',exact=True).wait_for()
 phone.screenshot(path=str(OUT/'phone-landscape.png'))
 phone.set_viewport_size({'width':390,'height':844});assert phone.evaluate('document.documentElement.scrollWidth<=innerWidth+1');phone.screenshot(path=str(OUT/'phone-portrait.png'),full_page=True)
 phone.set_viewport_size({'width':844,'height':390});phone.reload();phone.get_by_text('La partie est sur votre télévision',exact=True).wait_for()
 # Real multitouch sends both button edges through the shared game input hook.
 cdp=phone.context.new_cdp_session(phone)
 a=phone.get_by_role('button',name='A',exact=True).bounding_box();b=phone.get_by_role('button',name='B',exact=True).bounding_box()
 points=[{'x':box['x']+box['width']/2,'y':box['y']+box['height']/2,'id':index+1} for index,box in enumerate([a,b])]
 cdp.send('Input.dispatchTouchEvent',{'type':'touchStart','touchPoints':points})
 tv.wait_for_function('(window.__phoneGameEvents||[]).some(e=>e.index===0&&e.down)&&(window.__phoneGameEvents||[]).some(e=>e.index===1&&e.down)')
 cdp.send('Input.dispatchTouchEvent',{'type':'touchEnd','touchPoints':[]})
 tv.wait_for_function('(window.__phoneGameEvents||[]).some(e=>e.index===0&&e.down===false)&&(window.__phoneGameEvents||[]).some(e=>e.index===1&&e.down===false)')
 stick=phone.get_by_role('application',name='Stick gauche',exact=True).bounding_box();x=stick['x']+stick['width']*.8;y=stick['y']+stick['height']/2
 cdp.send('Input.dispatchTouchEvent',{'type':'touchStart','touchPoints':[{'x':x,'y':y,'id':3}]})
 tv.wait_for_function('(window.__phoneGameEvents||[]).some(e=>e.stick===0&&e.x>.4)')
 cdp.send('Input.dispatchTouchEvent',{'type':'touchEnd','touchPoints':[]})
 phone.get_by_role('button',name='L3',exact=True).click();phone.get_by_role('button',name='R3',exact=True).click()
 tv.wait_for_function('(window.__phoneGameEvents||[]).some(e=>e.index===10&&e.down)&&(window.__phoneGameEvents||[]).some(e=>e.index===11&&e.down)')
 # A frozen phone tab cannot leave a game button held.
 phone.get_by_role('button',name='A',exact=True).hover();phone.mouse.down();tv.wait_for_function('(window.__phoneGameEvents||[]).filter(e=>e.index===0).at(-1)?.down===true')
 cdp.send('Emulation.setScriptExecutionDisabled',{'value':True})
 tv.wait_for_function('(window.__phoneGameEvents||[]).filter(e=>e.index===0).at(-1)?.down===false',timeout=4000)
 tv.get_by_role('button',name=re.compile('^Manette téléphone')).click();tv.get_by_role('button',name='Déconnecter le téléphone',exact=True).click()
 cdp.send('Emulation.setScriptExecutionDisabled',{'value':False});phone.mouse.up();
 try:phone.get_by_role('button',name='Associer à ma TV',exact=True).wait_for(timeout=20000)
 except Exception:
  phone.screenshot(path=str(OUT/'phone-failure.png'));print('Phone state:',phone.locator('body').inner_text(),flush=True);print('TV requests:',tv.evaluate('window.__requests.slice(-8)'),flush=True);raise
 phone.set_viewport_size({'width':390,'height':844});phone.screenshot(path=str(OUT/'phone-join.png'),full_page=True)
 assert not errors,errors
 expected=os.environ.get('EXPECTED_PAD_TRANSPORT','ws');used_http=any('/channel/send' in url for url in requests)
 assert used_http==(expected=='http'),{'expected':expected,'httpChannelUsed':used_http}
 report={'tvViewport':'1920x1080','phoneViewport':'844x390 + 390x844','actualPairingTransport':'HTTPS-compatible channel' if used_http else 'HTTP signaling and WebSocket','explicitTvApproval':True,'navigation':True,'multitouch':True,'analog':True,'watchdogRelease':True,'revocation':True,'gameSink':'shared input hook, simulated session','errors':errors,'passed':True}
 (OUT/'tv-phone-browser.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
 tvbrowser.close();phonebrowser.close()
