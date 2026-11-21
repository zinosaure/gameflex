import sys,unittest,socket,threading,base64,time
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from private_vnc_http import HttpViewers
from emugen_manual import ManualError
class TunnelTests(unittest.TestCase):
 def test_owner_session_binary_and_expiration(self):
  listener=socket.socket();listener.bind(('127.0.0.1',0));listener.listen();expired=False
  class Manager:
   def viewer_endpoint(self,owner,session):
    if owner!='alice':raise ManualError('forbidden','Forbidden',403)
    if expired:raise ManualError('session_expired','Expired',410)
    return listener.getsockname()
  viewers=HttpViewers();m=Manager();opened=viewers.request(m,'alice','first',{'op':'open'});key=opened['viewer'];peer,_=listener.accept();peer.settimeout(2)
  try:
   peer.sendall(b'RFB 003.008\n');r=viewers.request(m,'alice','first',{'op':'read','viewer':key});self.assertEqual(base64.b64decode(r['data']),b'RFB 003.008\n')
   viewers.request(m,'alice','first',{'op':'write','viewer':key,'data':base64.b64encode(b'input').decode()});self.assertEqual(peer.recv(5),b'input')
   for owner,session in [('bob','first'),('alice','other')]:
    with self.assertRaises(ManualError):viewers.request(m,owner,session,{'op':'read','viewer':key})
   expired=True
   with self.assertRaises(ManualError):viewers.request(m,'alice','first',{'op':'read','viewer':key})
  finally:
   with viewers.lock:viewers._close(key)
   peer.close();listener.close()

class GuardTests(unittest.TestCase):
 def test_http_auth_and_origin_before_socket(self):
  from private_vnc_http import serve_http
  class Handler:
   server=type('Server',(),{'auth_base':'fixture'})()
   headers={'Host':'gameflex.test','Origin':'https://attacker.test'}
   def reply(self,code,value):self.code=code
  h=Handler();serve_http(h,None,lambda *_:None,'0'*32);self.assertEqual(h.code,403)
  h.headers={'Host':'gameflex.test','Origin':'https://gameflex.test'}
  serve_http(h,None,lambda *_:None,'0'*32);self.assertEqual(h.code,401)

if __name__=='__main__':unittest.main()
