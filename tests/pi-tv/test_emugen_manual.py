"""Run in the experience ARM64 image. Fixtures never contact a ROM source."""
import json
from pathlib import Path
import socket
import sys
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from types import SimpleNamespace
from wsproto import WSConnection,ConnectionType
from wsproto.events import Request,AcceptConnection,TextMessage,BytesMessage,CloseConnection
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from emugen_manual import ManualVerification,ManualError
from private_vnc import serve,valid_origin

FIX=Path(__file__).parent/'fixtures/emugen'
class ManualTests(unittest.TestCase):
 def setUp(self):
  self.manager=ManualVerification(reaper=False);self.manager.fixture=(FIX/'manual.html').read_text()
  self.session=self.manager.start('alice','free-demo-wii','wii',Path('/tmp'))
 def tearDown(self):self.manager.close()
 def act(self,fn):return self.manager.pool.submit(fn).result(timeout=10)
 def test_same_page_context_pending_human_magic_links(self):
  manager=self.manager;page=manager.page;context=manager.context
  self.assertEqual(manager.resume('alice',self.session['id'])['status'],'challenge_required')
  self.act(lambda:page.locator('#human').click())
  self.assertEqual(manager.resume('alice',self.session['id'])['status'],'challenge_required')
  self.act(lambda:page.locator('#emugen-cookie-btn').click())
  result=manager.resume('alice',self.session['id'])
  self.assertEqual(result['status'],'links');self.assertEqual(result['links'][0]['url'],'https://example.org/fixture-homebrew')
  self.assertIs(manager.page,page);self.assertIs(manager.context,context)
  self.assertEqual(self.act(lambda:page.evaluate('document.cookie')).count('fixture_'),2)
  public=json.dumps(manager.status('alice',self.session['id']))
  self.assertNotIn('fixture_verified',public);self.assertNotIn('owner',public)
  self.assertEqual(manager.start('alice','free-demo-wii','wii',Path('/tmp'))['id'],self.session['id'])
 def test_owner_expiration_and_capacity(self):
  for action in (self.manager.status,self.manager.resume,self.manager.viewer_endpoint,self.manager.close_session):
   with self.assertRaises(ManualError) as error:action('bob',self.session['id'])
   self.assertEqual(error.exception.code,403)
  with self.assertRaises(ManualError) as error:self.manager.start('alice','other-game','wii',Path('/tmp'))
  self.assertEqual(error.exception.status,'busy')
  with self.manager.lock:self.manager.records[self.session['id']]['expiresAt']=time.time()-1
  self.assertEqual(self.manager.resume('alice',self.session['id'])['status'],'session_expired')
  with self.assertRaises(ManualError):self.manager.viewer_endpoint('alice',self.session['id'])
  self.manager.pool.submit(self.manager._close_record,self.session['id']).result()
  self.assertIsNone(self.manager.browser);self.assertIsNone(self.manager.vnc);self.assertIsNone(self.manager.desktop)
 def test_explicit_empty_and_loading_error(self):
  self.act(lambda:self.manager.page.locator('#human').click());self.act(lambda:self.manager.page.locator('#emugen-cookie-btn').click())
  self.act(lambda:self.manager.page.locator('#emugen-download-section').evaluate("s=>s.innerText='No downloads available at this time!'"))
  self.assertEqual(self.manager.resume('alice',self.session['id'])['status'],'empty')
  self.act(lambda:self.manager.page.locator('#emugen-download-section').evaluate("s=>s.innerText='Loading…'"))
  self.assertEqual(self.manager.resume('alice',self.session['id'])['status'],'error')
 def test_disabling_provider_closes_private_session(self):
  self.manager.deactivate();self.manager.pool.submit(lambda:None).result(timeout=15)
  self.assertEqual(self.manager.status('alice',self.session['id'])['status'],'closed')
  self.assertIsNone(self.manager.browser);self.assertIsNone(self.manager.vnc)
 def test_websocket_owner_origin_and_no_vnc_before_auth(self):
  manager=self.manager
  class Handler(BaseHTTPRequestHandler):
   def log_message(self,*args):pass
   def reply(self,code,value):self.send_response(code);self.end_headers();self.wfile.write(json.dumps(value).encode())
   def do_GET(self):serve(self,manager,lambda auth,_: {'username':auth.removeprefix('Bearer ')} if auth in ('Bearer alice','Bearer bob') else None,self.path.strip('/'))
  server=ThreadingHTTPServer(('127.0.0.1',0),Handler);server.auth_base='fixture';threading.Thread(target=server.serve_forever,daemon=True).start()
  host='127.0.0.1:'+str(server.server_port)
  def connect(who):
   connection=socket.create_connection(('127.0.0.1',server.server_port));connection.settimeout(6)
   ws=WSConnection(ConnectionType.CLIENT);connection.sendall(ws.send(Request(host=host,target='/'+self.session['id'],subprotocols=['binary'],extra_headers=[(b'origin',('http://'+host).encode())])))
   ws.receive_data(connection.recv(4096));self.assertIsInstance(next(ws.events()),AcceptConnection)
   connection.sendall(ws.send(TextMessage(data=json.dumps({'type':'authenticate','token':who}))))
   ws.receive_data(connection.recv(4096));return connection,ws,list(ws.events())
  try:
   self.assertFalse(valid_origin({'Host':host,'Origin':'http://attacker.test'}));self.assertFalse(valid_origin({'Host':host}))
   conn,ws,events=connect('bob');self.assertIsInstance(events[0],CloseConnection);conn.close()
   conn,ws,events=connect('invalid');self.assertIsInstance(events[0],CloseConnection);conn.close()
   conn,ws,events=connect('alice');self.assertEqual(json.loads(events[0].data)['type'],'authorized')
   conn.settimeout(.2)
   with self.assertRaises(socket.timeout):conn.recv(4096)
   conn.settimeout(6);conn.sendall(ws.send(TextMessage(data='{"type":"viewer_ready"}')))
   ws.receive_data(conn.recv(4096));self.assertTrue(next(ws.events()).data.startswith(b'RFB '));conn.close()
  finally:server.shutdown();server.server_close()

if __name__=='__main__':unittest.main()

class AsyncOpeningTests(unittest.TestCase):
 def test_background_open_returns_starting_and_reuses_same_request(self):
  from unittest.mock import patch
  manager=ManualVerification(reaper=False);manager.fixture='fixture';gate=threading.Event()
  def opening(record,previous,data):
   gate.wait(3)
   with manager.lock:record['status']='ready'
  try:
   with patch.object(manager,'_open_background',side_effect=opening) as task:
    first=manager.start('alice','free-demo-wii','wii',Path('/tmp'),background=True)
    self.assertEqual(first['status'],'starting');self.assertFalse(gate.is_set())
    second=manager.start('alice','free-demo-wii','wii',Path('/tmp'),background=True)
    self.assertEqual(second['id'],first['id']);gate.set();manager.pool.submit(lambda:None).result(timeout=5)
    self.assertEqual(manager.status('alice',first['id'])['status'],'ready');self.assertEqual(task.call_count,1)
  finally:gate.set();manager.close()
 def test_starting_session_can_be_cancelled_without_waiting_for_browser(self):
  from unittest.mock import patch
  manager=ManualVerification(reaper=False);manager.fixture='fixture';gate=threading.Event()
  try:
   with patch.object(manager,'_open_background',side_effect=lambda *_:gate.wait(3)):
    record=manager.start('alice','free-demo-wii','wii',Path('/tmp'),background=True)
    self.assertEqual(manager.close_session('alice',record['id'])['status'],'closed');self.assertFalse(gate.is_set())
    gate.set();manager.pool.submit(lambda:None).result(timeout=5)
  finally:gate.set();manager.close()
