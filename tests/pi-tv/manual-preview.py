"""Private local fixture server, never use production credentials or external games."""
from pathlib import Path
import json,sys,threading,time
from http.server import ThreadingHTTPServer,BaseHTTPRequestHandler
from urllib.parse import urlsplit
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import emugen_manual
from private_vnc import serve
ROOT=Path(__file__).resolve().parents[2]
class FixtureHandler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def reply(self,code,value):
  self.send_response(code);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(value).encode())
 def do_GET(self):
  path=urlsplit(self.path).path
  if path.endswith('/socket'):
   if getattr(self.server,'block_websocket',False):return self.reply(400,{'error':'WebSocket blocked by fixture proxy'})
   return serve(self,self.server.manager,lambda token,_:{'username':'alice'} if token=='Bearer fixture-alice' else None,path.split('/')[-2])
  if '/verification/' in path:
   if self.headers.get('Authorization')!='Bearer fixture-alice':return self.reply(401,{'error':'fixture auth'})
   try:return self.reply(200,{'item':self.server.manager.status('alice',path.split('/')[-1])})
   except emugen_manual.ManualError as e:return self.reply(e.code,{'status':e.status,'error':str(e)})
  if path in ('/viewer.js','/viewer.css'):
   data=(ROOT/'.local/manual-preview'/path[1:]).read_bytes();self.send_response(200);self.send_header('Content-Type','application/javascript' if path.endswith('.js') else 'text/css');self.end_headers();return self.wfile.write(data)
  self.send_response(200);self.send_header('Content-Type','text/html; charset=utf-8');self.end_headers();self.wfile.write(b'<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/viewer.css"><div id="root"></div><script type="module" src="/viewer.js"></script>')
 def read_small_json(self,limit=65536):
  size=int(self.headers.get('Content-Length','0'))
  if size>limit:raise ValueError('large')
  return json.loads(self.rfile.read(size))
 def do_POST(self):
  if urlsplit(self.path).path.endswith('/transport'):
   from private_vnc_http import serve_http
   return serve_http(self,self.server.manager,lambda token,_:{'username':'alice'} if token=='Bearer fixture-alice' else None,urlsplit(self.path).path.split('/')[-2])
  if self.headers.get('Authorization')!='Bearer fixture-alice':return self.reply(401,{'error':'fixture auth'})
  self.rfile.read(int(self.headers.get('Content-Length','0')))
  return self.reply(200,self.server.manager.resume('alice',urlsplit(self.path).path.split('/')[-2]))
def create_server(bind='127.0.0.1',port=0):
 manager=emugen_manual.ManualVerification();manager.fixture=(ROOT/'tests/pi-tv/fixtures/emugen/manual.html').read_text()
 session=manager.start('alice','free-demo-wii','wii',ROOT/'.local/manual-preview')
 server=ThreadingHTTPServer((bind,port),FixtureHandler);server.manager=manager;server.auth_base='fixture'
 threading.Thread(target=server.serve_forever,daemon=True).start()
 return manager,server,session
if __name__=='__main__':
 manager,server,session=create_server('0.0.0.0',18189)
 (ROOT/'.local/manual-preview/session.json').write_text(json.dumps(session));print('Fixture ready',flush=True)
 try:threading.Event().wait()
 finally:server.shutdown();manager.close()
