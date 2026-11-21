"""Authenticated WebSocket-to-private-loopback VNC bridge.
GameFlex authorization is received in the first frame, never in a URL or log.
No VNC connection or frame is exposed before owner authorization and readiness.
"""
import json
import select
import socket
import threading
import time
from urllib.parse import urlsplit
from wsproto import WSConnection,ConnectionType
from wsproto.events import AcceptConnection,BytesMessage,TextMessage,Ping,CloseConnection
from emugen_manual import ManualError

AUTH_SLOTS=threading.BoundedSemaphore(4)
VIEWERS=threading.BoundedSemaphore(2)

def valid_origin(headers):
    try:
        origin=urlsplit(headers.get('Origin',''))
        return origin.scheme in ('http','https') and bool(origin.netloc) and not origin.username and not origin.password and origin.netloc.lower()==headers.get('Host','').lower()
    except ValueError:return False

def serve(handler,manager,verify_identity,session_id):
    if not valid_origin(handler.headers):return handler.reply(403,{'error':'Origine de connexion refusée.'})
    if handler.headers.get('Upgrade','').lower()!='websocket' or handler.headers.get('Sec-WebSocket-Version')!='13':return handler.reply(400,{'error':'WebSocket requis.'})
    if not AUTH_SLOTS.acquire(blocking=False):return handler.reply(429,{'error':'Connexion occupée. Réessayez.'})
    protocol=WSConnection(ConnectionType.SERVER);upstream=None;viewer=False;upgraded=False
    handler.close_connection=True
    def send(event):handler.connection.sendall(protocol.send(event))
    def text_message():
        text='';deadline=time.monotonic()+5
        while time.monotonic()<deadline:
            handler.connection.settimeout(max(.1,deadline-time.monotonic()))
            raw=handler.connection.recv(4096)
            if not raw:raise ValueError('closed')
            protocol.receive_data(raw)
            for event in protocol.events():
                if isinstance(event,Ping):send(event.response());continue
                if not isinstance(event,TextMessage):raise ValueError('text required')
                text+=event.data
                if len(text)>16384:raise ValueError('large')
                if event.message_finished:return json.loads(text)
        raise ValueError('timeout')
    try:
        headers=[(k.lower().encode('ascii'),v.encode('latin1')) for k,v in handler.headers.items()]
        protocol.initiate_upgrade_connection(headers,handler.path)
        list(protocol.events())
        offered=handler.headers.get('Sec-WebSocket-Protocol','').split(',')
        data=protocol.send(AcceptConnection(subprotocol='binary' if 'binary' in [x.strip() for x in offered] else None))
        handler.wfile.write(data);handler.wfile.flush();upgraded=True
        auth=text_message()
        token=auth.get('token') if isinstance(auth,dict) and auth.get('type')=='authenticate' else None
        if not isinstance(token,str) or not 1<=len(token)<=8192:raise ValueError('authentication')
        identity=verify_identity('Bearer '+token,handler.server.auth_base)
        if not identity:raise ValueError('authentication')
        username=identity['username'];endpoint=manager.viewer_endpoint(username,session_id)
        if not VIEWERS.acquire(blocking=False):raise ManualError('busy','Connexion occupée.')
        viewer=True;send(TextMessage(data='{"type":"authorized"}'))
        ready=text_message()
        if not isinstance(ready,dict) or ready.get('type')!='viewer_ready':raise ValueError('readiness')
        # The browser is authorized before a connection to the private VNC listener exists.
        upstream=socket.create_connection(endpoint,timeout=3);upstream.settimeout(3);handler.connection.settimeout(3)
        checked=time.monotonic()
        while True:
            manager.viewer_endpoint(username,session_id)
            if time.monotonic()-checked>30:
                identity=verify_identity('Bearer '+token,handler.server.auth_base)
                if not identity or identity['username']!=username:raise ValueError('revoked')
                checked=time.monotonic()
            readable,_,_=select.select([upstream,handler.connection],[],[],1)
            if upstream in readable:
                body=upstream.recv(65536)
                if not body:break
                send(BytesMessage(data=body))
            if handler.connection in readable:
                body=handler.connection.recv(65536)
                if not body:break
                protocol.receive_data(body)
                for event in protocol.events():
                    if isinstance(event,BytesMessage):
                        if len(event.data)>1024*1024:raise ValueError('large')
                        upstream.sendall(event.data)
                    elif isinstance(event,Ping):send(event.response())
                    elif isinstance(event,CloseConnection):return
                    else:raise ValueError('binary required')
    except ManualError as error:
        if upgraded:
            try:send(CloseConnection(code=1008,reason=error.status))
            except Exception:pass
    except Exception:
        if upgraded:
            try:send(CloseConnection(code=1008,reason='connection_refused'))
            except Exception:pass
    finally:
        if upstream:upstream.close()
        if viewer:VIEWERS.release()
        AUTH_SLOTS.release()
