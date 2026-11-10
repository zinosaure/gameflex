import json,tempfile,threading,unittest,urllib.request
from pathlib import Path
from http.server import ThreadingHTTPServer
from unittest.mock import patch
import server
class CoverSearchHTTPTests(unittest.TestCase):
 def test_real_handler_returns_installed_cover_urls(self):
  with tempfile.TemporaryDirectory() as directory,patch('server.verified_identity',return_value={'username':'fixture'}),patch('covers.prefetch'):
   http=ThreadingHTTPServer(('127.0.0.1',0),server.Handler);http.data=Path(directory);http.auth_base='http://127.0.0.1';http.library=None
   thread=threading.Thread(target=http.serve_forever,daemon=True);thread.start()
   try:
    data=json.dumps({'items':[{'consoleId':'gba','reference':'a','title':'Advance Wars'}]}).encode()
    req=urllib.request.Request('http://127.0.0.1:'+str(http.server_port)+'/v1/covers/search',data=data,headers={'Content-Type':'application/json','Authorization':'Bearer fixture'},method='POST')
    with urllib.request.urlopen(req,timeout=3) as response:
     value=json.load(response);self.assertEqual(response.status,200);self.assertEqual(len(value['items']),1)
     self.assertTrue(value['items'][0]['image'].startswith('/experience-api/v1/covers/gba/'))
   finally:http.shutdown();http.server_close();thread.join()
if __name__=='__main__':unittest.main()
