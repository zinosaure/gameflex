import base64,json,shutil,threading,unittest
from pathlib import Path
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
ROOT=Path(__file__).resolve().parents[2];ASSETS=ROOT/'deploy/pi-tv/android/app/src/main/assets'
class TvShellTests(unittest.TestCase):
 def test_library_console_details_favorites_and_focus_restore(self):
  if not shutil.which('chromium'):self.skipTest('Run in experience image')
  prefs={'interface':'covers','favorites':[],'history':['nes:fixture'],'collections':[]}
  class Handler(BaseHTTPRequestHandler):
   def log_message(self,*args):pass
   def do_GET(self):
    path=self.path.split('?')[0]
    data={'/api/v1/auth/me':{'item':{'username':'fixture'}},'/api/v1/consoles':{'data':[{'id':'nes','name':'Nintendo NES','method':'retrogaming'}]},'/experience-api/v1/preferences':prefs,'/experience-api/v1/media':{'items':[]},'/experience-api/v1/imports':{'items':[]},'/experience-api/v1/metadata':{'items':[]},'/api/v1/retrogaming/nes/roms':{'data':[{'reference':'fixture','title':'Démo libre','files':['demo.nes']}]},'/api/v1/retrogaming/nes/cores':{'data':[{'name':'quicknes','installed':True}]}}.get(path)
    self.send_response(200);self.send_header('Content-Type','application/json' if data is not None else 'text/html');self.end_headers()
    if data is not None:self.wfile.write(json.dumps(data).encode());return
    origin='http://127.0.0.1:'+str(self.server.server_port);html=(ASSETS/'tv.html').read_text().replace('__SERVER__',json.dumps(origin)).replace('__PAIR_SECRET__',json.dumps('a'*64)).replace('__PAIR_CHALLENGE__',json.dumps('b'*64)).replace('__APP_SCRIPT__',(ASSETS/'tv.js').read_text()).replace('__LOGO__',base64.b64encode((ASSETS/'logo.png').read_bytes()).decode());self.wfile.write(html.encode())
   def do_PUT(self):
    prefs.update(json.loads(self.rfile.read(int(self.headers['Content-Length']))));self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(prefs).encode())
  server=ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
  from playwright.sync_api import sync_playwright
  try:
   with sync_playwright() as driver:
    browser=driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage']);page=browser.new_page(viewport={'width':1920,'height':1080});page.add_init_script("localStorage.setItem('retrogaming-users-token','fixture-only')")
    page.goto('http://127.0.0.1:'+str(server.server_port));card=page.get_by_role('button',name='Démo libre Nintendo NES');card.wait_for();page.wait_for_function("document.activeElement.id==='game-fixture'")
    page.keyboard.press('Enter');page.get_by_role('button',name='Jouer',exact=True).wait_for();page.wait_for_function("document.activeElement.dataset.action==='play'")
    page.keyboard.press('ArrowRight');self.assertEqual(page.locator(':focus').get_attribute('data-action'),'favorite');page.keyboard.press('Enter');page.get_by_role('button',name='Retirer des favoris').wait_for();self.assertEqual(prefs['favorites'],['nes:fixture'])
    page.keyboard.press('Escape');page.wait_for_function("document.activeElement.id==='game-fixture'")
    page.get_by_role('button',name='Consoles',exact=True).click();page.get_by_role('button',name='Nintendo NES 1 jeu(x)').click();page.get_by_role('button',name='Démo libre Nintendo NES').wait_for()
    self.assertEqual(page.get_by_role('heading',name='Nintendo NES',exact=True).count(),1)
    page.set_viewport_size({'width':960,'height':540});page.get_by_role('button',name='Bibliothèque',exact=True).click();card=page.get_by_role('button',name='Démo libre Nintendo NES');card.wait_for();page.wait_for_timeout(100)
    self.assertTrue(page.evaluate("document.documentElement.scrollWidth<=innerWidth"))
    rect=card.bounding_box();self.assertLess(rect['y']+rect['height'],510)
    browser.close()
  finally:server.shutdown();server.server_close()
