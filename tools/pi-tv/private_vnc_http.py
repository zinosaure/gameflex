"""Owner-authenticated HTTP fallback for proxies which strip WebSocket upgrades.
Only a private loopback VNC endpoint is reachable; no URL carries credentials.
"""
import base64
import secrets
import socket
import threading
import time
from emugen_manual import ManualError
from private_vnc import valid_origin

class HttpViewers:
    def __init__(self):
        self.lock=threading.RLock();self.records={}
        threading.Thread(target=self._reap,daemon=True).start()
    def _close(self,key):
        record=self.records.pop(key,None)
        if record:
            try:record['socket'].shutdown(socket.SHUT_RDWR)
            except OSError:pass
            record['socket'].close()
    def _reap(self):
        while True:
            time.sleep(5)
            with self.lock:
                for key,r in list(self.records.items()):
                    try:r['manager'].viewer_endpoint(r['owner'],r['session'])
                    except ManualError:self._close(key);continue
                    if time.monotonic()-r['touched']>30:self._close(key)
    def request(self,manager,owner,session,data):
        endpoint=manager.viewer_endpoint(owner,session)
        op=data.get('op');key=data.get('viewer')
        with self.lock:
            if op=='open':
                for k,r in list(self.records.items()):
                    if r['owner']==owner and r['session']==session:self._close(k)
                if len(self.records)>=2:raise ManualError('busy','Fenêtre occupée.',429)
                sock=socket.create_connection(endpoint,timeout=3);sock.settimeout(8)
                key=secrets.token_hex(16)
                self.records[key]={'owner':owner,'session':session,'socket':sock,'manager':manager,'touched':time.monotonic(),'read':threading.Lock(),'write':threading.Lock()}
                return {'viewer':key}
            r=self.records.get(key) if isinstance(key,str) else None
            if not r or r['owner']!=owner or r['session']!=session:raise ManualError('session_expired','Fenêtre déconnectée.',410)
            r['touched']=time.monotonic()
            if op=='close':self._close(key);return {'closed':True}
        if op=='read':
            if not r['read'].acquire(blocking=False):raise ValueError('read pending')
            try:
                try:body=r['socket'].recv(262144)
                except socket.timeout:body=None
                if body==b'':
                    with self.lock:self._close(key)
                    raise ManualError('session_expired','Fenêtre déconnectée.',410)
                return {'data':base64.b64encode(body or b'').decode()}
            finally:r['read'].release()
        if op=='write':
            encoded=data.get('data','')
            if not isinstance(encoded,str) or len(encoded)>400000:raise ValueError('large')
            body=base64.b64decode(encoded,validate=True)
            with r['write']:r['socket'].sendall(body)
            return {'written':len(body)}
        raise ValueError('operation')

VIEWERS=HttpViewers()
def serve_http(handler,manager,verify_identity,session):
    if not valid_origin(handler.headers):return handler.reply(403,{'error':'Origine de connexion refusée.'})
    identity=verify_identity(handler.headers.get('Authorization'),handler.server.auth_base)
    if not identity:return handler.reply(401,{'error':'Connexion GameFlex requise.'})
    try:
        data=handler.read_small_json(524288)
        return handler.reply(200,VIEWERS.request(manager,identity['username'],session,data))
    except ManualError as e:return handler.reply(e.code,{'error':str(e),'status':e.status})
    except (ValueError,OSError):return handler.reply(400,{'error':'Fenêtre distante indisponible.'})
