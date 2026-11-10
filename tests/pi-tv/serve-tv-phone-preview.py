"""Local test server: real pairing transport, test-only identity, no production proxy."""
import sys,importlib.util
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT/'tools/pi-tv'))
import server
spec=importlib.util.spec_from_file_location('static_preview',Path(__file__).with_name('serve-playstation-preview.py'));static=importlib.util.module_from_spec(spec);spec.loader.exec_module(static)
server.verified_identity=lambda auth,base:{'username':'preview','role_level':0} if auth=='Bearer fixture-only' else None
class Handler(server.Handler):
 def do_GET(self):
  if self.path.startswith('/gameflex/experience-api/v1/phone-pad/'):
   self.path=self.path.removeprefix('/gameflex/experience-api');return super().do_GET()
  return static.Handler.do_GET(self)
 def do_POST(self):
  if self.path.startswith('/gameflex/experience-api/v1/phone-pad/'):
   self.path=self.path.removeprefix('/gameflex/experience-api');return super().do_POST()
  return self.reply(404,{'error':'Fixture only'})
if __name__=='__main__':
 import argparse
 p=argparse.ArgumentParser();p.add_argument('--port',type=int,default=18230);a=p.parse_args()
 http=server.BoundedHTTPServer(('0.0.0.0',a.port),Handler);http.auth_base='fixture';http.serve_forever()
