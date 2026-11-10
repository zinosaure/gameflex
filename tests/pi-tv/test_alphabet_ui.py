import shutil,threading,unittest
from pathlib import Path
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
ROOT=Path(__file__).resolve().parents[2]
class AlphabetUiTests(unittest.TestCase):
 def test_initial_all_sorted_select_and_deselect_same_letter(self):
  if not shutil.which('chromium') or not (ROOT/'.local/alphabet-preview/app.js').exists():self.skipTest('Build alphabet-view.jsx and run in the experience image.')
  class Handler(BaseHTTPRequestHandler):
   def log_message(self,*args):pass
   def do_GET(self):
    name=self.path.strip('/');file=ROOT/'.local/alphabet-preview'/name
    self.send_response(200);self.send_header('Content-Type','text/css' if name.endswith('.css') else 'application/javascript' if name.endswith('.js') else 'text/html');self.end_headers()
    self.wfile.write(file.read_bytes() if name in ('app.js','app.css') else b'<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/app.css"><div id="root"></div><script src="/app.js"></script>')
  server=ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
  from playwright.sync_api import sync_playwright
  try:
   with sync_playwright() as driver:
    browser=driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage']);page=browser.new_page(viewport={'width':390,'height':844},has_touch=True)
    page.goto('http://127.0.0.1:'+str(server.server_port));page.locator('.em-poster strong').first.wait_for()
    titles=lambda:page.locator('.em-poster strong').all_text_contents()
    expected=['12 fixture','Alpha fixture','Éclair fixture','Golf fixture','Zeta fixture']
    self.assertEqual(titles(),expected);self.assertEqual(page.locator('.gx-alphabet [aria-pressed=true]').count(),0)
    button=page.get_by_role('button',name='G',exact=True);button.tap();self.assertEqual(titles(),['Golf fixture']);self.assertEqual(button.get_attribute('aria-pressed'),'true')
    button.tap();self.assertEqual(titles(),expected);self.assertEqual(page.locator('.gx-alphabet [aria-pressed=true]').count(),0)
    number=page.get_by_role('button',name='0–9',exact=True);number.tap();self.assertEqual(titles(),['12 fixture']);number.tap();self.assertEqual(titles(),expected)
    browser.close()
  finally:server.shutdown();server.server_close()
