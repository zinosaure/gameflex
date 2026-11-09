"""Authenticated, bounded HTTP relay for GameFlex command/signaling WebSockets.
Fixed project upstream only. Original services still authorize every command.
"""
import base64,json,re,secrets,socket,threading,time
from collections import deque
from urllib.parse import urlsplit,parse_qsl,urlencode
from wsproto import WSConnection,ConnectionType
from wsproto.events import Request,AcceptConnection,RejectConnection,TextMessage,BytesMessage,Ping,CloseConnection
class BridgeError(ValueError):
 def __init__(self,message,code=409):super().__init__(message);self.code=code

def safe_path(value):
 if not isinstance(value,str) or len(value)>2048:raise BridgeError('Canal refusé.',400)
 u=urlsplit(value)
 if u.scheme or u.netloc or u.fragment or '%' in u.path:raise BridgeError('Canal refusé.',400)
 allowed=u.path in ['/api/v1/facade/ws','/api/v1/retrogaming/stream/ws','/api/v1/presence/ws'] or re.fullmatch(r'/api/v1/sessions/[A-Za-z0-9_-]{1,128}/child/v4/ws',u.path) or re.fullmatch(r'/api/v1/gateway-webrtc/streams/[a-f0-9]{16,128}/viewer-ws',u.path)
 if not allowed:raise BridgeError('Canal refusé.',400)
 params=parse_qsl(u.query,keep_blank_values=True)
 if any(k!='viewerId' or not re.fullmatch('[a-f0-9]{16,128}',v) for k,v in params) or len(params)>1:raise BridgeError('Paramètres du canal refusés.',400)
 return u.path+('?' +urlencode(params) if params else '')

class Channel:
 def __init__(self,owner,token,path):
  self.owner=owner;self.touched=time.monotonic();self.created=self.touched;self.condition=threading.Condition();self.write_lock=threading.Lock();self.queue=deque();self.size=0;self.closed=False;self.token=token;self.path=path
  self.socket=socket.create_connection(('gameflex-web',8080),timeout=8);self.socket.settimeout(8);self.ws=WSConnection(ConnectionType.CLIENT)
  self.socket.sendall(self.ws.send(Request(host='gameflex-web:8080',target=(path+'?token='+token if (path in ('/api/v1/presence/ws','/api/v1/retrogaming/stream/ws') or '/child/v4/ws' in path) else path),extra_headers=[(b'origin',b'http://gameflex-web'),(b'authorization',('Bearer '+token).encode())])))
  deadline=time.monotonic()+12;accepted=False
  try:
   while not accepted:
    if time.monotonic()>deadline:raise BridgeError('Ouverture du canal trop lente.',504)
    body=self.socket.recv(65536)
    if not body:raise BridgeError('Canal indisponible.',502)
    self.ws.receive_data(body)
    for e in self.ws.events():
     if isinstance(e,RejectConnection):raise BridgeError('Le service refuse ce canal.',e.status_code if e.status_code in (401,403,404) else 502)
     if isinstance(e,AcceptConnection):accepted=True
     else:self.event(e)
   self.socket.settimeout(2);threading.Thread(target=self.reader,daemon=True).start()
  except Exception:self.socket.close();raise
 def event(self,e):
  if isinstance(e,(TextMessage,BytesMessage)):
   # wsproto may split a single WebSocket message over several network reads.
   if not hasattr(self,'fragment'):self.fragment=[];self.fragment_size=0
   self.fragment.append(e.data);self.fragment_size+=len(e.data)
   if self.fragment_size>1024*1024:raise BridgeError('Message trop volumineux.')
   if e.message_finished:
    value=(''.join(self.fragment) if isinstance(e,TextMessage) else b''.join(self.fragment));self.fragment=[];self.fragment_size=0
    encoded={'text':value} if isinstance(value,str) else {'binary':base64.b64encode(value).decode()}
    with self.condition:
     self.size+=len(value)
     if self.size>8*1024*1024:raise BridgeError('Canal saturé.')
     self.queue.append((encoded,len(value)));self.condition.notify_all()
  elif isinstance(e,Ping):
   with self.write_lock:self.socket.sendall(self.ws.send(e.response()))
  elif isinstance(e,CloseConnection):self.close()
 def reader(self):
  try:
   while not self.closed:
    try:data=self.socket.recv(65536)
    except socket.timeout:continue
    if not data:break
    with self.write_lock:
     self.ws.receive_data(data);events=list(self.ws.events())
    for e in events:self.event(e)
  except Exception:pass
  finally:self.close()
 def send(self,data):
  if self.closed:raise BridgeError('Canal déconnecté.',410)
  if isinstance(data.get('text'),str) and len(data['text'].encode())<=1024*1024:event=TextMessage(data=data['text'])
  elif isinstance(data.get('binary'),str) and len(data['binary'])<=1400000:
   try:event=BytesMessage(data=base64.b64decode(data['binary'],validate=True))
   except ValueError:raise BridgeError('Message invalide.',400)
  else:raise BridgeError('Message invalide.',400)
  with self.write_lock:self.socket.sendall(self.ws.send(event))
 def read(self):
  with self.condition:
   self.condition.wait_for(lambda:self.queue or self.closed,timeout=8)
   frames=[];size=0
   while self.queue and len(frames)<64 and size<512*1024:
    value,n=self.queue.popleft();frames.append(value);self.size-=n;size+=n
   return {'frames':frames,'closed':self.closed and not self.queue}
 def close(self):
  with self.condition:
   self.closed=True
   try:self.socket.shutdown(socket.SHUT_RDWR)
   except OSError:pass
   self.socket.close();self.condition.notify_all()

class Bridges:
 def __init__(self):
  self.lock=threading.RLock();self.channels={};threading.Thread(target=self.reap,daemon=True).start()
 def reap(self):
  while True:
   time.sleep(5)
   with self.lock:
    for key,c in list(self.channels.items()):
     if time.monotonic()-c.touched>35 or time.monotonic()-c.created>4*3600:c.close();self.channels.pop(key,None)
 def open(self,owner,token,path):
  path=safe_path(path)
  with self.lock:
   if len(self.channels)>=16 or sum(c.owner==owner for c in self.channels.values())>=6:raise BridgeError('Trop de canaux ouverts. Fermez les anciens onglets.',429)
   # Reserve while connecting; a failed handshake never exposes a channel.
   channel=Channel(owner,token,path);key=secrets.token_hex(16);self.channels[key]=channel
   return {'id':key}
 def owned(self,owner,key):
  with self.lock:
   c=self.channels.get(key)
   if not c:raise BridgeError('Canal expiré.',410)
   if c.owner!=owner:raise BridgeError('Canal non autorisé.',403)
   c.touched=time.monotonic();return c
 def close(self,owner,key):
  c=self.owned(owner,key);c.close()
  with self.lock:self.channels.pop(key,None)
BRIDGES=Bridges()
def serve(handler,match,verify):
 identity=verify(handler.headers.get('Authorization'),handler.server.auth_base)
 if not identity:return handler.reply(401,{'error':'Connexion GameFlex requise.'})
 try:
  data=handler.read_small_json(1500000)
  if not isinstance(data,dict):raise BridgeError('Demande invalide.',400)
  owner=identity['username'];token=handler.headers['Authorization'].removeprefix('Bearer ')
  if not match:return handler.reply(201,BRIDGES.open(owner,token,data.get('path')))
  channel=BRIDGES.owned(owner,match[1]);op=match[2]
  if op=='poll':return handler.reply(200,channel.read())
  if op=='send':channel.send(data);return handler.reply(200,{'sent':True})
  BRIDGES.close(owner,match[1]);return handler.reply(200,{'closed':True})
 except BridgeError as e:return handler.reply(e.code,{'error':str(e)})
 except (ValueError,OSError):return handler.reply(502,{'error':'Connexion au canal interrompue.'})
