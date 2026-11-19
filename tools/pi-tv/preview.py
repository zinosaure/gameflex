#!/usr/bin/env python3
"""Temporary visual preview of the shared React components and approved local photos."""
import argparse
from functools import partial
from http.server import SimpleHTTPRequestHandler,ThreadingHTTPServer
import json
from pathlib import Path
import re
ROOT=Path(__file__).resolve().parents[2]
class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):
        path=self.path.split('?')[0]
        prefix='/experience-api/v1/media/'
        if path.startswith(prefix):
            digest=path[len(prefix):]
            if not re.fullmatch('[a-f0-9]{64}',digest):self.send_error(404);return
            cache=ROOT/'.local/pi-tv/media'
            try:
                meta=json.loads((cache/(digest+'.json')).read_text())
                if not meta.get('approved'):raise ValueError()
                body=(cache/(digest+'.img')).read_bytes()
            except (OSError,ValueError):self.send_error(404);return
            self.send_response(200);self.send_header('Content-Type',meta['mime']);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body);return
        super().do_GET()
    def log_message(self,*args):pass
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--bind',default='127.0.0.1');p.add_argument('--port',type=int,default=18186);a=p.parse_args()
    ThreadingHTTPServer((a.bind,a.port),partial(Handler,directory=str(ROOT/'output'))).serve_forever()
