import importlib.util
import json
from pathlib import Path
import tempfile
import threading
import unittest
import urllib.request
import urllib.error
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
def module(name, file):
    spec = importlib.util.spec_from_file_location(name, ROOT / file)
    m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m); return m
api = module('experience', 'tools/pi-tv/server.py')
art = module('artwork', 'tools/pi-tv/artwork.py')

class ApiTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=ROOT / 'output')
        from types import SimpleNamespace
        self.s = SimpleNamespace(data=Path(self.temp.name), auth_base='http://127.0.0.1:10200')
        from source_settings import save,registry
        save(self.s.data,[item['id'] for item in registry()])
    def tearDown(self): self.temp.cleanup()
    def request(self, path, method='GET', data=None, token=None):
        # Exercise the actual HTTP handlers in memory: this sandbox forbids sockets.
        from io import BytesIO
        h = object.__new__(api.Handler)
        h.server = self.s; h.path = path; h.command = method; h.client_address=('127.0.0.1',12345)
        raw = json.dumps(data).encode() if data is not None else b''
        h.headers = {'Content-Type':'application/json', 'Content-Length':str(len(raw))}
        if token: h.headers['Authorization'] = 'Bearer ' + token
        h.rfile = BytesIO(raw); h.wfile = BytesIO()
        status=[]
        h.send_response=lambda code: status.append(code)
        h.send_header=lambda *args: None
        h.end_headers=lambda: None
        getattr(h,'do_'+method)()
        return status[0], json.loads(h.wfile.getvalue())
    def test_tv_pairing_routes_require_verified_approval_and_bound_secret(self):
        import hashlib,tv_pairing
        tv_pairing.RECORDS.clear();tv_pairing.RATE.clear()
        secret='c'*64
        code,pair=self.request('/v1/tv/pairing','POST',{'challenge':hashlib.sha256(secret.encode()).hexdigest()})
        self.assertEqual(code,201)
        poll='/v1/tv/pairing/'+pair['id']+'/poll'
        self.assertEqual(self.request(poll,'POST',{'secret':'d'*64})[0],400)
        self.assertEqual(self.request(poll,'POST',{'secret':secret})[1]['status'],'pending')
        with patch.object(api,'verified_identity',return_value=None):
            self.assertEqual(self.request('/v1/tv/pairing/approve','POST',{'code':pair['code']})[0],401)
        with patch.object(api,'verified_identity',return_value={'username':'fixture','role_level':1}):
            self.assertEqual(self.request('/v1/tv/pairing/approve','POST',{'code':pair['code']},'verified-test-token')[0],200)
        self.assertEqual(self.request(poll,'POST',{'secret':secret})[1]['accessToken'],'verified-test-token')
        self.assertEqual(self.request(poll,'POST',{'secret':secret})[1]['status'],'expired')
        tv_pairing.RECORDS.clear();tv_pairing.RATE.clear()

    def test_manual_verification_http_requires_authentication(self):
        session='a'*32
        for path,method in [('/v1/emugen/verification','POST'),('/v1/emugen/verification/current?slug=fixture','GET'),('/v1/emugen/verification/'+session,'GET'),('/v1/emugen/verification/'+session+'/resume','POST'),('/v1/emugen/verification/'+session,'DELETE')]:
            self.assertEqual(self.request(path,method,{'slug':'fixture','consoleId':'wii'})[0],401)
        self.assertEqual(self.request('/v1/emugen/verification/'+session,'PATCH')[0],405)

    def test_library_routes_require_verified_identity_and_admin_setup(self):
        from types import SimpleNamespace
        from unittest.mock import Mock
        self.s.library=Mock()
        for path,method in [('/v1/library','GET'),('/v1/library/folders','GET'),('/v1/downloads','GET'),('/v1/library','PUT'),('/v1/downloads','POST')]:
            self.assertEqual(self.request(path,method,{'folder':'/tmp'})[0],401)
        with patch.object(api,'verified_identity',return_value={'username':'member','role_level':1}):
            self.assertEqual(self.request('/v1/library','PUT',{'folder':'/tmp'},'valid')[0],403)
            self.assertEqual(self.request('/v1/library/folders',token='valid')[0],403)
        self.s.library.configure.assert_not_called()
        self.s.library.jobs.return_value=[]
        with patch.object(api,'verified_identity',return_value={'username':'alice','role_level':0}):
            self.assertEqual(self.request('/v1/downloads',token='valid')[0],200)
            self.s.library.jobs.assert_called_once_with('alice')
    def test_external_sources_are_authenticated_and_report_capabilities(self):
        self.assertEqual(self.request('/v1/game-sources')[0],401)
        with patch.object(api,'verified_identity',return_value={'username':'alice','role_level':0}):
            code,data=self.request('/v1/game-sources',token='valid')
            self.assertEqual(code,200)
            sources={x['id']:x for x in data['items']}
            self.assertEqual(sources['freeroms']['url'],'https://www.freeroms.com/')
            self.assertTrue(sources['freeroms']['downloads'])
            self.assertFalse(sources['planetemu']['downloads'])
    def test_catalogue_read_does_not_prefetch_unseen_artwork(self):
        import covers,planetemu
        with patch.object(api,'verified_username',return_value='alice'),patch.object(planetemu,'catalogue',return_value={'items':[{'title':'Demo (USA)','path':'/rom/nes/demo'}]}),patch.object(covers,'prefetch') as prefetch:
            code,data=self.request('/v1/planetemu/nes',token='valid')
            self.assertEqual(code,200);self.assertEqual(len(data['items']),1);prefetch.assert_not_called()
    def test_auth_is_required_and_not_jwt_payload_trusted(self):
        self.assertEqual(self.request('/v1/preferences')[0],401)
        with patch.object(api, 'verified_username', return_value=None):
            self.assertEqual(self.request('/v1/preferences','PUT',{'interface':'minimal'},'forged')[0],401)
    def test_preferences_are_separate_per_verified_account(self):
        pref = {'interface':'consoles','favorites':['nes:free'],'history':[],'collections':[]}
        with patch.object(api, 'verified_username', return_value='alice'):
            self.assertEqual(self.request('/v1/preferences','PUT',pref,'a')[0],200)
            self.assertEqual(self.request('/v1/preferences',token='a')[1],pref)
        with patch.object(api, 'verified_username', return_value='bob'):
            self.assertIsNone(self.request('/v1/preferences',token='b')[1]['interface'])
        file = next((self.s.data/'preferences').glob('*.json'))
        self.assertEqual(file.stat().st_mode & 0o777, 0o600)
        self.assertNotIn('alice',file.name)
    def test_invalid_interface_and_oversized_collections(self):
        with patch.object(api, 'verified_username', return_value='alice'):
            self.assertEqual(self.request('/v1/preferences','PUT',{'interface':'evil'},'a')[0],400)
            self.assertEqual(self.request('/v1/preferences','PUT',{'interface':'minimal','collections':[{}]},'a')[0],400)
            self.assertEqual(self.request('/v1/preferences','PUT',{'interface':'minimal','history':['x']*41},'a')[0],400)
    def test_paths_and_methods_do_not_expose_files(self):
        for path in ['/v1/media/../../AGENTS.md','/.env','/v1/media/'+'g'*64]: self.assertEqual(self.request(path)[0],404)
        self.assertEqual(self.request('/v1/preferences','POST',{})[0],405)
    def test_manifest_and_digest_must_match(self):
        folder=self.s.data/'media';folder.mkdir()
        digest='a'*64
        (folder/(digest+'.img')).write_bytes(b'not its hash')
        (folder/(digest+'.json')).write_text(json.dumps({'mime':'image/png'}))
        self.assertEqual(self.request('/v1/media/'+digest)[0],404)
    def test_provider_blocks_arbitrary_hosts_and_credentials(self):
        for url in ['http://upload.wikimedia.org/a','https://localhost/a','https://upload.wikimedia.org.evil/a','https://upload.wikimedia.org:444/a','https://secret@upload.wikimedia.org/a']:
            with self.assertRaises(ValueError): art.check_url(url)
        art.check_url('https://upload.wikimedia.org/wikipedia/commons/a.jpg')
    def test_catalog_covers_33_unique_systems(self):
        code, body = self.request('/v1/compatibility')
        self.assertEqual(code,200);self.assertEqual(len(body['consoles']),33)
        self.assertEqual(len({x['id'] for x in body['consoles']}),33)
        self.assertEqual(self.request('/v1/interfaces')[1]['interfaces'],api.MODES)
class DemoTests(unittest.TestCase):
    def test_original_cartridge_structure_and_reset_vector(self):
        demo=module('nes_demo','tools/pi-tv/nes_demo.py')
        rom=demo.cartridge()
        self.assertEqual(rom[:4],b'NES\x1a')
        self.assertEqual(rom[4:8],bytes([1,1,0,0]))
        self.assertEqual(len(rom),16+16384+8192)
        self.assertEqual(int.from_bytes(rom[16+16384-4:16+16384-2],'little'),0x8000)
        self.assertEqual(rom,demo.cartridge())


class HostRelayTests(unittest.TestCase):
    def test_unknown_port_is_rejected_before_dial(self):
        relay=module('host_relay','tools/pi-tv/host-relay.py')
        import socket
        from types import SimpleNamespace
        class Request:
            def __init__(self): self.raw=b'\x00\x01'
            def settimeout(self,n): pass
            def recv(self,n): result=self.raw[:n];self.raw=self.raw[n:];return result
            def sendall(self,b): raise AssertionError('port 1 must never be accepted')
        with patch.object(socket,'create_connection',side_effect=AssertionError('unexpected dial')):
            relay.HostHandler(Request(),None,SimpleNamespace())
    def test_original_bytes_cross_the_unix_tunnel(self):
        relay=module('host_relay_live','tools/pi-tv/host-relay.py')
        import socket,socketserver
        class Echo(socketserver.BaseRequestHandler):
            def handle(self): self.request.sendall(self.request.recv(1024).upper())
        with tempfile.TemporaryDirectory(dir=ROOT/'output') as folder:
            echo=socketserver.TCPServer(('127.0.0.1',0),Echo)
            port=echo.server_address[1]
            relay.PORTS=(port,)
            unix=relay.HostServer(folder+'/relay.sock',relay.HostHandler)
            bridge=relay.BridgeServer(('127.0.0.2',port),relay.BridgeHandler);bridge.unix_path=folder+'/relay.sock'
            servers=[echo,unix,bridge];threads=[]
            try:
                for server in servers:
                    thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start();threads.append(thread)
                with socket.create_connection(('127.0.0.2',port),timeout=3) as client:
                    client.sendall(b'gameflex-free-demo');self.assertEqual(client.recv(1024),b'GAMEFLEX-FREE-DEMO')
            finally:
                for server in servers: server.shutdown();server.server_close()
                for thread in threads: thread.join()

if __name__=='__main__': unittest.main()
