"""Isolated UI fixtures. Only a small local artwork selection is served; no proxy."""
from http.server import SimpleHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
import re
ROOT=Path(__file__).resolve().parents[2]/'.local/interface-cleanup'
class Handler(SimpleHTTPRequestHandler):
 def do_GET(self):
  url=self.path.split('?')[0]
  image=re.fullmatch(r'/gameflex/experience-api/v1/media/([a-f0-9]{64})',url)
  if image:
   file=ROOT/'media'/(image[1]+'.webp')
   if not file.is_file():self.send_error(404);return
   body=file.read_bytes();mime='image/webp'
  elif url.endswith('/app.js'):
   body=(ROOT/'after/app.js').read_bytes();mime='text/javascript'
  elif url.endswith('/app.css'):
   body=b'';mime='text/css'
  elif '/experience-api/' in url:
   self.send_error(404);return
  else:
   body=b'<html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="root"></div><script defer src="/app.js"></script></html>';mime='text/html'
  self.send_response(200);self.send_header('Content-Type',mime);self.send_header('Cache-Control','no-store');self.end_headers();self.wfile.write(body)
 def log_message(self,*args):pass
if __name__=='__main__':
 import argparse
 p=argparse.ArgumentParser();p.add_argument('--bind',default='127.0.0.1');p.add_argument('--port',type=int,default=18228);a=p.parse_args()
 ThreadingHTTPServer((a.bind,a.port),Handler).serve_forever()
