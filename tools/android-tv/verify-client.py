#!/usr/bin/env python3
"""Container-side TV shell integration check; called only by verify-live.py."""
import base64,hashlib,json,sys,time
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path('/app');ASSETS=ROOT/'deploy/pi-tv/android/app/src/main/assets';OUT=ROOT/'output/android-tv';base='http://192.168.1.63:2000'
admin=json.load(sys.stdin)['token'];secret=__import__('secrets').token_hex(32);challenge=hashlib.sha256(secret.encode()).hexdigest()
html=(ASSETS/'tv.html').read_text().replace('__SERVER__',json.dumps(base)).replace('__PAIR_SECRET__',json.dumps(secret)).replace('__PAIR_CHALLENGE__',json.dumps(challenge)).replace('__APP_SCRIPT__',(ASSETS/'tv.js').read_text()).replace('__LOGO__',base64.b64encode((ASSETS/'logo.png').read_bytes()).decode())
report={};sid=None
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage','--autoplay-policy=no-user-gesture-required','--no-sandbox'])
 context=browser.new_context(viewport={'width':1920,'height':1080});page=context.new_page()
 context.add_init_script((ASSETS/'stream-tv.js').read_text())
 try:
  page.goto(base+'/experience-api/v1/tv/link');page.set_content(html);page.wait_for_function("/^\\d{6}$/.test(document.querySelector('#pair-code')?.textContent||'')")
  code=page.locator('#pair-code').inner_text()
  denied=context.request.post(base+'/experience-api/v1/tv/pairing/approve',data={'code':code});assert denied.status==401
  r=context.request.post(base+'/experience-api/v1/tv/pairing/approve',data={'code':code},headers={'Authorization':'Bearer '+admin});assert r.ok
  card=page.get_by_role('button',name='palette-libre Nintendo Ent. System (NES)',exact=True);card.wait_for(timeout=60000);report['pairing']='approved via authenticated existing account';report['initialFocus']=page.locator(':focus').get_attribute('id');page.screenshot(path=str(OUT/'local-library.png'))
  card.press('Enter');page.get_by_role('button',name='Jouer',exact=True).wait_for();page.wait_for_function("document.activeElement.dataset.action==='play'");page.screenshot(path=str(OUT/'local-details.png'))
  page.get_by_role('button',name='Jouer',exact=True).press('Enter');page.wait_for_url('**/streaming?**',timeout=20000);sid=page.evaluate("new URL(location.href).searchParams.get('sessionId')");
  for attempt in range(18):
   page.wait_for_timeout(5000)
   state=page.evaluate('GameFlexTvStats?GameFlexTvStats():({})');report['lastPlayback']=state
   (OUT/'local-progress.json').write_text(json.dumps(state,indent=2))
   page.screenshot(path=str(OUT/'local-progress.png'))
   if state.get('width',0)>0:break
  assert state.get('width',0)>0,'Video did not start'

  page.wait_for_timeout(5000);page.keyboard.press('ArrowRight');page.keyboard.press('z');page.keyboard.press('Enter');page.wait_for_timeout(5000);report['playback']=page.evaluate('GameFlexTvStats()');page.screenshot(path=str(OUT/'local-game.png'));assert report['playback']['width']>0 and report['playback']['decoded']>0 and report['playback']['inputMessages']>0
  page.goto(base+'/experience-api/v1/tv/link');page.set_content(html)
  page.get_by_role('button',name='palette-libre Nintendo Ent. System (NES)',exact=True).wait_for(timeout=60000)
  page.wait_for_function("document.activeElement.id==='game-e4a6c62f-3e9e-5234-b356-68b7047386b6'")
  report['returnToPreviousGame']=True
  report['ownTestSessionStopped']=page.evaluate("sessionStorage.getItem('tv-current-session')===null")
  assert report['ownTestSessionStopped'];sid=None

 except Exception:
  report['errorState']=page.locator('#pair-state').inner_text() if page.locator('#pair-state').count() else 'Pair UI absent'
  report['title']=page.title();report['screenState']=page.locator('#message').inner_text() if page.locator('#message').count() else 'no detail message';report['path']=page.evaluate('location.pathname');
  if report.get('pairing'):page.screenshot(path=str(OUT/'local-failure.png'))
  raise
 finally:
  if sid:
   stop=page.evaluate('''async ({base,token,id})=>new Promise(resolve=>{const ws=new WebSocket(base.replace(/^http/,'ws')+'/api/v1/facade/ws?token='+encodeURIComponent(token));const timer=setTimeout(()=>{ws.close();resolve(false)},10000);ws.onopen=()=>ws.send(JSON.stringify({type:'session.stop',id:'test-stop',data:{platform:'retrogaming',sessionId:id}}));ws.onmessage=e=>{const x=JSON.parse(e.data);if(x.id==='test-stop'){clearTimeout(timer);ws.close();resolve(x.status>=200&&x.status<300)}}})''',{'base':base,'token':admin,'id':sid});report['ownTestSessionStopped']=stop
  browser.close();(OUT/'local-integration.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
