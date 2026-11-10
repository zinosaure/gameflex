import json,threading,unittest,urllib.request,urllib.error
from http.server import ThreadingHTTPServer
from unittest.mock import patch
import server,phone_pad
class PhonePadHTTPTests(unittest.TestCase):
 def setUp(self):
  self.pairings=patch('phone_pad.PAIRS',phone_pad.Pairings());self.pairings.start()
  self.identity=patch('server.verified_identity',side_effect=lambda auth,base:{'username':'alice' if auth=='Bearer owner' else 'bob'} if auth in ('Bearer owner','Bearer other') else None);self.identity.start()
  self.http=ThreadingHTTPServer(('127.0.0.1',0),server.Handler);self.http.auth_base='fixture';self.thread=threading.Thread(target=self.http.serve_forever,daemon=True);self.thread.start();self.base='http://127.0.0.1:'+str(self.http.server_port)
 def tearDown(self):self.http.shutdown();self.http.server_close();self.thread.join();self.identity.stop();self.pairings.stop()
 def request(self,path,body,token=None):
  headers={'Content-Type':'application/json'}
  if token:headers['Authorization']='Bearer '+token
  try:
   with urllib.request.urlopen(urllib.request.Request(self.base+'/v1/phone-pad'+path,data=json.dumps(body).encode(),headers=headers),timeout=3) as response:return response.status,json.load(response)
  except urllib.error.HTTPError as e:return e.code,json.load(e)
 def test_account_is_required_only_on_tv_and_owner_approves(self):
  self.assertEqual(self.request('/create',{})[0],401)
  status,tv=self.request('/create',{},'owner');self.assertEqual(status,200)
  status,phone=self.request('/join',{'code':tv['code'],'name':'Phone'});self.assertEqual(status,200);self.assertNotIn('accessToken',phone)
  path='/'+tv['id']+'/approve';body={'hostToken':tv['hostToken']}
  self.assertEqual(self.request(path,body)[0],401);self.assertEqual(self.request(path,body,'other')[0],400)
  self.assertEqual(self.request(path,body,'owner')[0],200)
  self.assertEqual(self.request('/'+tv['id']+'/revoke',body,'owner')[0],200)
 def test_cross_origin_websocket_upgrade_is_refused(self):
  request=urllib.request.Request(self.base+'/v1/phone-pad/'+'a'*32+'/ws',headers={'Origin':'https://other.example','Upgrade':'websocket','Connection':'Upgrade','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':'dGhlIHNhbXBsZSBub25jZQ=='})
  with self.assertRaises(urllib.error.HTTPError) as error:urllib.request.urlopen(request,timeout=3)
  self.assertEqual(error.exception.code,403)
if __name__=='__main__':unittest.main()
