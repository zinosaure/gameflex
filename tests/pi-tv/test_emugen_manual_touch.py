"""End-to-end noVNC touch test with fixture-only credentials and download links."""
import importlib.util,json,sys,unittest
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('manual_preview',Path(__file__).with_name('manual-preview.py'));preview=importlib.util.module_from_spec(spec);spec.loader.exec_module(preview)
class TouchTests(unittest.TestCase):
 def test_touch_same_session_resume_links_displayed(self):
  if not (ROOT/'.local/manual-preview/viewer.js').exists():self.skipTest('Bundle tests/pi-tv/manual-viewer.jsx into .local/manual-preview/viewer.js first.')
  manager,server,session=preview.create_server()
  from playwright.sync_api import sync_playwright
  try:
   with sync_playwright() as driver:
    browser=driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage'])
    context=browser.new_context(viewport={'width':390,'height':844},has_touch=True,is_mobile=True,device_scale_factor=1)
    page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.on('console',lambda e:print('Client console',e.type,e.text,flush=True) if e.type in ('error','warning') else None)
    page.goto('http://127.0.0.1:'+str(server.server_port)+'/?session='+session['id'])
    try:page.locator('.gf-manual-screen canvas').wait_for(state='visible',timeout=30000)
    except Exception:
     print('Fixture client',page.locator('body').inner_text(),errors,flush=True);page.screenshot(path=str(ROOT/'output/emugen-manual-touch-error.png'));raise
    page.wait_for_function("!document.body.innerText.includes('Connexion à la fenêtre du Pi…')",timeout=30000)
    mint="""() => {const c=document.querySelector('.gf-manual-screen canvas');if(!c||c.width<100)return null;const d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let x1=c.width,y1=c.height,x2=0,y2=0;for(let y=0;y<c.height;y+=4)for(let x=0;x<c.width;x+=4){const i=(y*c.width+x)*4;if(Math.abs(d[i]-101)<3&&Math.abs(d[i+1]-212)<3&&Math.abs(d[i+2]-189)<3){x1=Math.min(x1,x);x2=Math.max(x2,x);y1=Math.min(y1,y);y2=Math.max(y2,y);}}return x2>x1?{x:(x1+x2)/2,y:(y1+y2)/2,w:x2-x1}:null;}"""
    try:page.wait_for_function(mint,timeout=20000)
    except Exception:
     page.screenshot(path=str(ROOT/'output/emugen-manual-touch-error.png'));print(page.locator('body').inner_text(),errors,flush=True);raise
    help_section=page.locator('.gf-manual-help');self.assertFalse(help_section.evaluate('(e)=>e.open'))
    help_section.locator('summary').press('Enter');self.assertTrue(help_section.evaluate('(e)=>e.open'))
    help_section.locator('summary').press('Enter');self.assertFalse(help_section.evaluate('(e)=>e.open'))
    remote_page=manager.page;remote_context=manager.context
    def touch(selector):
     # Find the fixture's green button in actual VNC pixels, including any native browser chrome.
     width=manager.pool.submit(lambda:manager.page.locator(selector).bounding_box()['width']).result(timeout=5)
     # Wait for the VNC frame containing this button, rather than an arbitrary delay.
     page.wait_for_function('() => {const p=('+mint+')();return p&&Math.abs(p.w-'+str(width)+')<16;}',timeout=15000)
     point=page.evaluate(mint);self.assertIsNotNone(point)
     canvas=page.locator('.gf-manual-screen canvas');canvas.scroll_into_view_if_needed();box=canvas.bounding_box()
     page.touchscreen.tap(box['x']+point['x']*box['width']/1024,box['y']+point['y']*box['height']/768)
    page.screenshot(path=str(ROOT/'output/emugen-manual-touch-before.png'))
    touch('#human')
    manager.pool.submit(lambda:manager.page.locator('#emugen-cookie-btn').wait_for(state='visible',timeout=8000)).result(timeout=10)
    # Reproduce a real detail page whose Cookie button lies below the first viewport.
    manager.pool.submit(lambda:manager.page.locator('#emugen-cookie-btn').evaluate("b=>b.style.marginTop='1800px'")).result(timeout=5)
    self.assertGreater(manager.pool.submit(lambda:manager.page.locator('#emugen-cookie-btn').bounding_box()['y']).result(timeout=5),768)
    page.get_by_role('button',name='Trouver Magic Cookie',exact=True).click()
    manager.pool.submit(lambda:manager.page.wait_for_function("()=>{const b=document.querySelector('#emugen-cookie-btn').getBoundingClientRect();return b.top>=0&&b.bottom<innerHeight}",timeout=10000)).result(timeout=12)
    self.assertNotIn('fixture_magic=site',manager.pool.submit(lambda:manager.page.evaluate('document.cookie')).result(timeout=5))
    page.screenshot(path=str(ROOT/'output/emugen-cookie-found.png'))
    touch('#emugen-cookie-btn')
    manager.pool.submit(lambda:manager.page.locator('#emugen-download-section').wait_for(state='visible',timeout=8000)).result(timeout=10)
    page.get_by_role('button',name='Reprendre la recherche',exact=True).click()
    page.get_by_text('Liens disponibles.',exact=True).wait_for(timeout=12000)
    self.assertIs(manager.page,remote_page);self.assertIs(manager.context,remote_context)
    page.screenshot(path=str(ROOT/'output/emugen-manual-touch.png'))
    page.get_by_role('button',name='Revenir à la fiche',exact=True).click()
    self.assertEqual(page.get_by_role('link',name='Démo libre · lien de test').get_attribute('href'),'https://example.org/fixture-homebrew')
    self.assertEqual(errors,[])
    browser.close()
  finally:server.shutdown();server.server_close();manager.close()
if __name__=='__main__':unittest.main()
