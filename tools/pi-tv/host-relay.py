#!/usr/bin/env python3
"""GameFlex-only TCP relay over a project-local Unix socket; no firewall changes."""
import argparse
import os
from pathlib import Path
import select
import socket
import socketserver
import struct
import threading

PORTS = (10202, *range(9001, 9011))
LIMIT = threading.BoundedSemaphore(32)

def tunnel(a,b):
    a.setblocking(False); b.setblocking(False)
    while True:
        readable,_,_=select.select([a,b],[],[],300)
        if not readable:return
        for source in readable:
            data=source.recv(65536)
            if not data:return
            target=b if source is a else a
            target.setblocking(True);target.settimeout(10);target.sendall(data);target.setblocking(False)

class Bounded:
    daemon_threads=True
    allow_reuse_address=True
    def process_request(self,request,address):
        if not LIMIT.acquire(False):self.shutdown_request(request);return
        try:super().process_request(request,address)
        except Exception:LIMIT.release();raise
    def process_request_thread(self,request,address):
        try:super().process_request_thread(request,address)
        finally:LIMIT.release()
    def handle_error(self,*args):pass  # Never log forwarded bytes, URLs or credentials.

class HostServer(Bounded,socketserver.ThreadingUnixStreamServer):pass
class BridgeServer(Bounded,socketserver.ThreadingTCPServer):pass

class HostHandler(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.settimeout(5)
        raw=b''
        while len(raw)<2:
            chunk=self.request.recv(2-len(raw))
            if not chunk:return
            raw+=chunk
        port=struct.unpack('!H',raw)[0]
        if port not in PORTS:return
        with socket.create_connection(('127.0.0.1',port),timeout=5) as destination:
            self.request.sendall(b'\x01');tunnel(self.request,destination)

class BridgeHandler(socketserver.BaseRequestHandler):
    def handle(self):
        with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as destination:
            destination.settimeout(10);destination.connect(self.server.unix_path)
            destination.sendall(struct.pack('!H',self.server.server_address[1]))
            if destination.recv(1)!=b'\x01':return
            tunnel(self.request,destination)

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=['host','bridge']);p.add_argument('--socket',default='/relay/control.sock');a=p.parse_args()
    if a.mode=='host':
        path=Path(a.socket)
        if path.exists():
            if not path.is_socket():p.error('Existing path is not a Unix socket')
            path.unlink()
        server=HostServer(a.socket,HostHandler);os.chmod(a.socket,0o660);server.serve_forever()
    else:
        servers=[]
        for port in PORTS:
            server=BridgeServer(('0.0.0.0',port),BridgeHandler);server.unix_path=a.socket;servers.append(server)
        for server in servers[:-1]:threading.Thread(target=server.serve_forever,daemon=True).start()
        servers[-1].serve_forever()
if __name__=='__main__':main()
