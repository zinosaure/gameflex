import sys,unittest,socket,threading,time
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import socket_bridge as b
from wsproto import WSConnection,ConnectionType
from wsproto.events import Request,AcceptConnection,TextMessage
class BridgeTests(unittest.TestCase):
 def test_paths(self):
  self.assertEqual(b.safe_path('/api/v1/facade/ws'),'/api/v1/facade/ws')
  fallback='/api/v1/gateway-webrtc/streams/'+'a'*32+'/viewer-fallback?viewerId='+'b'*32
  self.assertEqual(b.safe_path(fallback),fallback)
  for path in ['https://example.org/api/v1/facade/ws','//localhost/api/v1/facade/ws','/api/v1/facade/ws?token=secret','/api/v1/../auth','/api/v1/facade/ws#x','/api/v1/%66acade/ws']:
   with self.subTest(path=path),self.assertRaises(b.BridgeError):b.safe_path(path)
 def test_owner_and_expiry(self):
  registry=b.Bridges()
  class C:
   owner='alice';touched=0
   def close(self):pass
  registry.channels['key']=C()
  with self.assertRaises(b.BridgeError) as denied:registry.owned('bob','key')
  self.assertEqual(denied.exception.code,403)
  registry.close('alice','key')
  with self.assertRaises(b.BridgeError) as expired:registry.owned('alice','key')
  self.assertEqual(expired.exception.code,410)
 def test_real_websocket_fragmented_echo(self):
  client,server=socket.socketpair();seen=[]
  def fixture():
   ws=WSConnection(ConnectionType.SERVER)
   try:
    while True:
     data=server.recv(65536)
     if not data:break
     ws.receive_data(data)
     for event in ws.events():
      if isinstance(event,Request):
       seen.extend(event.extra_headers);server.sendall(ws.send(AcceptConnection()))
      elif isinstance(event,TextMessage):
       server.sendall(ws.send(TextMessage(data='echo:',message_finished=False)))
       server.sendall(ws.send(TextMessage(data=event.data,message_finished=True)))
   finally:server.close()
  t=threading.Thread(target=fixture,daemon=True);t.start()
  with patch.object(b.socket,'create_connection',return_value=client):channel=b.Channel('alice','fixture-token','/api/v1/facade/ws')
  try:
   channel.send({'text':'fixture'})
   self.assertEqual(channel.read()['frames'],[{'text':'echo:fixture'}])
   self.assertIn((b'authorization',b'Bearer fixture-token'),seen)
   with self.assertRaises(b.BridgeError):channel.send({'text':'a'*(1024*1024+1)})
  finally:channel.close();t.join(2)
if __name__=='__main__':unittest.main()
