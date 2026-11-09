"""Modal appears while Chromium is preparing; no automatic headless resolution."""
import json,shutil,threading,unittest
from pathlib import Path
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
ROOT=Path(__file__).resolve().parents[2]
class ManualOpeningUiTests(unittest.TestCase):
 def test_modal_opens_before_browser_ready_without_headless_wait(self):
  if not shutil.which('chromium') or not (ROOT/'.local/manual-opening-preview/app.js').exists():self.skipTest('Build manual-opening-view.jsx and run in the experience image.')
  calls=[];record={'id':'a'*32,'slug':'free-demo-wii','consoleId':'wii','status':'starting','expiresAt':9999999999,'width':1024,'height':768}
  class Handler(BaseHTTPRequestHandler):
   def log_message(self,*args):pass
   def reply(self,code,value):
    self.send_response(code);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(value).encode())
   def do_GET(self):
    calls.append(self.path)
    if '/verification/current?' in self.path:return self.reply(200,{'item':None})
    if '/verification/' in self.path:return self.reply(200,{'item':record})
    name=self.path.strip('/');self.send_response(200);self.send_header('Content-Type','text/css' if name=='app.css' else 'application/javascript' if name=='app.js' else 'text/html');self.end_headers()
    self.wfile.write((ROOT/'.local/manual-opening-preview'/name).read_bytes() if name in ('app.js','app.css') else b'<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/app.css"><div id="root"></div><script type="module" src="/app.js"></script>')
   def do_POST(self):
    calls.append(self.path);self.rfile.read(int(self.headers.get('Content-Length','0')))
    if self.path=='/experience-api/v1/emugen/verification':return self.reply(202,{'item':record})
    return self.reply(500,{'error':'Unexpected automatic browser call'})
  server=ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
  from playwright.sync_api import sync_playwright
  try:
   with sync_playwright() as driver:
    browser=driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage']);page=browser.new_page(viewport={'width':390,'height':844},has_touch=True)
    page.goto('http://127.0.0.1:'+str(server.server_port))
    page.get_by_role('button',name='Sources de téléchargement',exact=True).click()
    page.get_by_role('button',name='Vérifier EmuGen',exact=True).click()
    page.get_by_role('dialog',name='Vérifier EmuGen').wait_for()
    page.get_by_text('Ouverture du navigateur…',exact=True).wait_for()
    self.assertNotIn('/experience-api/v1/emugen/sources',calls)
    self.assertEqual(calls.count('/experience-api/v1/emugen/verification'),1)
    self.assertFalse(page.get_by_role('button',name='Reprendre la recherche',exact=True).is_enabled())
    browser.close()
  finally:server.shutdown();server.server_close()
