"""Serve only the local interface fixture bundles, without upstream proxying."""
from http.server import SimpleHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]/'.local/interface-cleanup'
class Handler(SimpleHTTPRequestHandler):
 def do_GET(self):
  url=self.path.split('?')[0]
  folder='before' if url.startswith('/before') else 'after'
  file=ROOT/folder/url.rsplit('/',1)[-1]
  if url.endswith('.css') and not file.is_file():
   body=b'';mime='text/css'
  elif url.endswith(('.js','.css')) and file.is_file():
   body=file.read_bytes();mime='text/css' if url.endswith('.css') else 'text/javascript'
  else:
   body=f'<html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/{folder}/app.css"><div id="root"></div><script defer src="/{folder}/app.js"></script></html>'.encode();mime='text/html'
  self.send_response(200);self.send_header('Content-Type',mime);self.end_headers();self.wfile.write(body)
 def log_message(self,*args):pass
if __name__=='__main__':
 import argparse
 parser=argparse.ArgumentParser();parser.add_argument('--bind',default='127.0.0.1');parser.add_argument('--port',type=int,default=18196);args=parser.parse_args()
 ThreadingHTTPServer((args.bind,args.port),Handler).serve_forever()
