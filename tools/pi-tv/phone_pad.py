"""TV-approved, input-only phone capability. No account token leaves the TV."""
import hashlib,hmac,json,math,queue,re,secrets,select,threading,time
from wsproto import WSConnection,ConnectionType
from wsproto.events import AcceptConnection,TextMessage,Ping,Pong,CloseConnection
from private_vnc import valid_origin

class PadError(ValueError):pass
class Pairings:
 def __init__(self,clock=time.monotonic):self.clock=clock;self.lock=threading.RLock();self.records={};self.rates={}
 def _hash(self,value):return hashlib.sha256(value.encode()).hexdigest()
 def _limit(self,key,maximum):
  now=self.clock();at,count=self.rates.get(key,(now,0))
  if now-at>60:at,count=now,0
  if count>=maximum:raise PadError('Trop de tentatives. Réessayez dans une minute.')
  self.rates[key]=(at,count+1)
 def _emit(self,r,role,event):
  target=r['connections'].get(role)
  if not target:return
  q=target[1]
  if q.full():
   try:q.get_nowait()
   except queue.Empty:pass
  q.put_nowait(event)
 def _status(self,r):
  value={'type':'status','approved':r['approved'],'pending':r['phoneHash'] is not None and not r['approved'],'name':r['name'],'hostConnected':'host' in r['connections'],'phoneConnected':'phone' in r['connections'],'epoch':r['epoch'],'screen':r['screen']}
  for role in ['host','phone']:self._emit(r,role,value)
 def _neutral(self,r):
  r['buttons']=[];r['axes']=[0,0,0,0]
  self._emit(r,'host',{'type':'pad','buttons':[],'axes':[0,0,0,0],'epoch':r['epoch']})
 def _remove(self,id):
  r=self.records.pop(id,None)
  if r:
   self._neutral(r)
   for role in ['host','phone']:self._emit(r,role,{'type':'closed'})
 def _cleanup(self):
  now=self.clock()
  for id,r in list(self.records.items()):
   if now>r['expires'] or (not r['connections'] and now-r['activity']>60):self._remove(id)
   elif r['buttons'] or any(r['axes']):
    if now-r['lastInput']>1.2:self._neutral(r)
  for key,(at,_) in list(self.rates.items()):
   if now-at>120:self.rates.pop(key,None)
 def create(self,owner):
  with self.lock:
   self._cleanup();self._limit('create:'+owner,6)
   if len(self.records)>=4 or sum(r['owner']==owner for r in self.records.values())>=2:raise PadError('Fermez une association existante pour continuer.')
   id=secrets.token_hex(16);secret=secrets.token_urlsafe(32);code=''.join(secrets.choice('0123456789') for _ in range(8));now=self.clock()
   while any(r['codeHash']==self._hash(code) for r in self.records.values()):code=''.join(secrets.choice('0123456789') for _ in range(8))
   self.records[id]={'owner':owner,'hostHash':self._hash(secret),'phoneHash':None,'codeHash':self._hash(code),'approved':False,'name':'','expires':now+120,'activity':now,'lastInput':now,'connections':{},'epoch':0,'screen':'library','buttons':[],'axes':[0,0,0,0],'sequence':-1}
   return {'id':id,'hostToken':secret,'code':code,'expiresIn':120}
 def join(self,code,name,client):
  with self.lock:
   self._cleanup();self._limit('join:'+client,20)
   if not isinstance(code,str) or not re.fullmatch('[0-9]{8}',code):raise PadError('Code invalide ou expiré.')
   if not isinstance(name,str) or not 1<=len(name.strip())<=40 or any(ord(c)<32 for c in name):raise PadError('Nom de téléphone invalide.')
   entry=next(((id,r) for id,r in self.records.items() if r['codeHash'] and hmac.compare_digest(r['codeHash'],self._hash(code))),None)
   if not entry:raise PadError('Code invalide ou expiré.')
   id,r=entry;secret=secrets.token_urlsafe(32);r.update(phoneHash=self._hash(secret),codeHash=None,name=name.strip());self._status(r)
   return {'id':id,'phoneToken':secret}
 def _get(self,id,role,secret,owner=None):
  self._cleanup();r=self.records.get(id)
  if role not in ('host','phone') or not r or not isinstance(secret,str) or not 32<=len(secret)<=128:raise PadError('Association terminée.')
  expected=r.get(role+'Hash')
  if not expected or not hmac.compare_digest(expected,self._hash(secret)) or (role=='host' and owner!=r['owner']):raise PadError('Association refusée.')
  return r
 def action(self,id,secret,owner,action):
  with self.lock:
   r=self._get(id,'host',secret,owner)
   if action=='approve':
    if not r['phoneHash']:raise PadError('Aucun téléphone en attente.')
    r['approved']=True;r['expires']=self.clock()+4*3600;self._status(r)
   elif action=='revoke':self._remove(id)
   else:raise PadError('Action refusée.')
   return {'ok':True}
 def attach(self,id,role,secret,owner=None):
  with self.lock:
   r=self._get(id,role,secret,owner);old=r['connections'].get(role)
   if old:self._emit(r,role,{'type':'closed'})
   r['epoch']+=1;r['sequence']=-1
   identity=secrets.token_hex(8);q=queue.Queue(maxsize=4);r['connections'][role]=(identity,q);r['activity']=self.clock();
   if role=='host' and r['approved']:r['expires']=self.clock()+4*3600
   self._neutral(r);self._status(r)
   return identity,q
 def detach(self,id,role,identity):
  with self.lock:
   r=self.records.get(id)
   if r and r['connections'].get(role,(None,))[0]==identity:
    r['connections'].pop(role,None);r['activity']=self.clock()
    if role=='host':r['expires']=min(r['expires'],self.clock()+60)
    self._neutral(r);self._status(r)
 def message(self,id,role,identity,data):
  with self.lock:
   self._cleanup();r=self.records.get(id)
   if not r or r['connections'].get(role,(None,))[0]!=identity:raise PadError('Association terminée.')
   r['activity']=self.clock()
   if not isinstance(data,dict):raise PadError('Message invalide.')
   if data.get('type')=='ping':return
   if role=='host' and data.get('type')=='context':
    if data.get('screen') not in ('library','game','unsupported'):raise PadError('Contexte invalide.')
    r['epoch']+=1;r['sequence']=-1;r['screen']=data['screen'];self._neutral(r);self._status(r);return
   if role!='phone' or not r['approved'] or 'host' not in r['connections'] or data.get('type')!='pad':raise PadError('Commande refusée.')
   buttons,axes,seq,epoch=data.get('buttons'),data.get('axes'),data.get('seq'),data.get('epoch')
   if not isinstance(buttons,list) or len(buttons)>17 or any(type(b)!=int or not 0<=b<=16 for b in buttons):raise PadError('Boutons invalides.')
   if not isinstance(axes,list) or len(axes)!=4 or any(type(v) not in (float,int) or not math.isfinite(v) or not -1<=v<=1 for v in axes):raise PadError('Sticks invalides.')
   if type(seq)!=int or not 0<=seq<=2147483647 or type(epoch)!=int:raise PadError('Séquence invalide.')
   if epoch!=r['epoch'] or seq<=r['sequence']:return
   r.update(sequence=seq,buttons=sorted(set(buttons)),axes=axes,lastInput=self.clock())
   self._emit(r,'host',{'type':'pad','buttons':r['buttons'],'axes':axes,'epoch':epoch})
 def tick(self):
  with self.lock:self._cleanup()
PAIRS=Pairings();AUTH_SLOTS=threading.BoundedSemaphore(4)

def http(handler,verify):
 path=handler.path.removeprefix('/v1/phone-pad')
 try:
  body=handler.read_small_json(2048)
  if path=='/join':return handler.reply(200,PAIRS.join(body.get('code'),body.get('name','Téléphone'),handler.client_address[0]))
  identity=verify(handler.headers.get('Authorization'),handler.server.auth_base)
  if not identity:return handler.reply(401,{'error':'Connexion requise sur la TV.'})
  if path=='/create':return handler.reply(200,PAIRS.create(identity['username']))
  match=re.fullmatch('/([a-f0-9]{32})/(approve|revoke)',path)
  if not match:raise PadError('Action inconnue.')
  return handler.reply(200,PAIRS.action(match[1],body.get('hostToken'),identity['username'],match[2]))
 except PadError as e:return handler.reply(400,{'error':str(e)})
 except (ValueError,TypeError):return handler.reply(400,{'error':'Demande invalide.'})

def websocket(handler,verify,id):
 if not valid_origin(handler.headers):return handler.reply(403,{'error':'Origine refusée.'})
 if handler.headers.get('Upgrade','').lower()!='websocket' or handler.headers.get('Sec-WebSocket-Version')!='13':return handler.reply(400,{'error':'WebSocket requis.'})
 if not AUTH_SLOTS.acquire(False):return handler.reply(429,{'error':'Réessayez.'})
 ws=WSConnection(ConnectionType.SERVER);role=identity=None;upgraded=False;handshake=True;handler.close_connection=True
 def send(event):handler.connection.sendall(ws.send(event))
 try:
  headers=[(k.lower().encode('ascii'),v.encode('latin1')) for k,v in handler.headers.items()];ws.initiate_upgrade_connection(headers,handler.path);list(ws.events())
  handler.wfile.write(ws.send(AcceptConnection()));handler.wfile.flush();upgraded=True;handler.connection.settimeout(5)
  message='';deadline=time.monotonic()+5;owner=None;access_token=None
  while identity is None:
   if time.monotonic()>deadline:raise PadError('Délai dépassé.')
   raw=handler.connection.recv(4096)
   if not raw:raise PadError('Connexion fermée.')
   ws.receive_data(raw)
   for event in ws.events():
    if isinstance(event,Ping):send(event.response());continue
    if not isinstance(event,TextMessage):raise PadError('Authentification requise.')
    message+=event.data
    if len(message)>12288:raise PadError('Message trop long.')
    if event.message_finished:
     auth=json.loads(message);role=auth.get('role')
     if role=='host':
      access_token=auth.get('accessToken');user=verify('Bearer '+access_token,handler.server.auth_base) if isinstance(access_token,str) else None
      if not user:raise PadError('Connexion TV refusée.')
      owner=user['username']
     identity,inbox=PAIRS.attach(id,role,auth.get('token'),owner);message='';break
  AUTH_SLOTS.release();handshake=False;handler.connection.settimeout(3);checked=time.monotonic();rates=0;rate_at=checked
  while True:
   PAIRS.tick()
   while True:
    try:out=inbox.get_nowait()
    except queue.Empty:break
    send(TextMessage(data=json.dumps(out)))
    if out.get('type')=='closed':return
   now=time.monotonic()
   if role=='host' and now-checked>30:
    user=verify('Bearer '+access_token,handler.server.auth_base)
    if not user or user['username']!=owner:PAIRS.action(id,auth['token'],owner,'revoke');return
    checked=now
   if not select.select([handler.connection],[],[],.05)[0]:continue
   raw=handler.connection.recv(4096)
   if not raw:break
   ws.receive_data(raw)
   for event in ws.events():
    if isinstance(event,Ping):send(event.response())
    elif isinstance(event,Pong):pass
    elif isinstance(event,CloseConnection):return
    elif isinstance(event,TextMessage):
     message+=event.data
     if len(message)>2048:raise PadError('Message trop long.')
     if event.message_finished:
      if now-rate_at>=1:rate_at=now;rates=0
      rates+=1
      if rates>240:raise PadError('Trop de commandes.')
      PAIRS.message(id,role,identity,json.loads(message));message=''
    else:raise PadError('Message refusé.')
 except Exception:
  if upgraded:
   try:send(CloseConnection(code=1008,reason='association_closed'))
   except Exception:pass
 finally:
  if handshake:AUTH_SLOTS.release()
  if identity:PAIRS.detach(id,role,identity)
