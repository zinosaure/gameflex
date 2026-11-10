#!/usr/bin/env python3
"""Optional GameFlex experience service. No system, emulator, or TV commands."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import threading
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = Path(__file__).resolve().parents[2]
MODES = ['minimal', 'covers', 'consoles', 'favorites', 'resume', 'collections', 'detail', 'archives', 'multiplayer', 'dashboard', 'station', 'crt', 'shelf', 'orbit', 'arcade']
MODES += ['hub-canape', 'ligne-cinema', 'mosaique-joy', 'roue-retro', 'terminal-poche',
          'cartouches', 'carte-epoques', 'double-ecran', 'club-local', 'quai-systemes',
          'magazine-retro', 'bureau-collectionneur', 'grand-ecran-accessible']
LOCK = threading.Lock()
UPLOAD_SLOT = threading.BoundedSemaphore(1)
from upload_sessions import UploadSessions
UPLOADS = UploadSessions(UPLOAD_SLOT)

def validate_preferences(p):
    if not isinstance(p, dict) or p.get('interface') not in MODES:
        raise ValueError('interface invalide')
    result = {'interface': p['interface']}
    if 'interfaceUpdatedAt' in p:
        stamp=p['interfaceUpdatedAt']
        if isinstance(stamp,bool) or not isinstance(stamp,int) or not 0 <= stamp <= 9007199254740991: raise ValueError('révision interface invalide')
        result['interfaceUpdatedAt']=stamp
    moved = {'favorites', 'resume', 'collections', 'detail', 'archives', 'multiplayer'}
    for field in ('previousInterface', 'interfaceDestination'):
        value = p.get(field)
        if value is not None and (not isinstance(value, str) or value not in moved): raise ValueError('migration invalide')
        if field in p: result[field] = value
    for field, maximum in [('favorites', 2000), ('history', 40)]:
        values = p.get(field, [])
        if not isinstance(values, list) or len(values) > maximum or any(not isinstance(x, str) or len(x) > 512 for x in values):
            raise ValueError('liste invalide')
        result[field] = list(dict.fromkeys(values))
    cs = p.get('collections', [])
    if not isinstance(cs, list) or len(cs) > 100: raise ValueError('collections invalides')
    result['collections'] = []
    seen = set()
    for c in cs:
        if not isinstance(c, dict) or not isinstance(c.get('id'), str) or not re.fullmatch(r'[\w-]{1,80}', c['id']) or c['id'] in seen:
            raise ValueError('identifiant collection invalide')
        seen.add(c['id'])
        if not isinstance(c.get('name'), str) or not 1 <= len(c['name'].strip()) <= 80: raise ValueError('nom collection invalide')
        gs = c.get('games', [])
        if not isinstance(gs, list) or len(gs) > 2000 or any(not isinstance(g, str) or len(g) > 512 for g in gs): raise ValueError('jeux invalides')
        result['collections'].append({'id': c['id'], 'name': c['name'].strip(), 'games': list(dict.fromkeys(gs))})
    return result

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

def verified_identity(auth, auth_base):
    if not auth or not auth.startswith('Bearer ') or len(auth) > 8192: return None
    # Authoritative upstream check every time. Never decode an unverified JWT for authorization.
    request = urllib.request.Request(auth_base.rstrip('/') + '/api/v1/auth/me', headers={'Authorization': auth})
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=3) as r:
            p = json.loads(r.read(65537))
            item = p.get('item')
            if r.status == 200 and isinstance(item, dict) and isinstance(item.get('username'), str) and item['username']:
                return item
    except (OSError, ValueError, urllib.error.HTTPError):
        pass
    return None

def verified_username(auth, auth_base):
    identity=verified_identity(auth,auth_base)
    return identity['username'] if identity else None

class Handler(BaseHTTPRequestHandler):
    server_version = 'GameFlexExperience/1'
    def setup(self):
        super().setup()
        self.connection.settimeout(10)
    def log_message(self, fmt, *args):
        # Do not log URLs, tokens, paths or response bodies.
        pass
    def reply(self, status, obj):
        body = json.dumps(obj, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('Cache-Control', 'no-store')
        self.send_header('X-Content-Type-Options', 'nosniff')
        self.end_headers(); self.wfile.write(body)
    def serve_image(self, digest):
        if not re.fullmatch('[a-f0-9]{64}', digest): return self.reply(404, {'error': 'absent'})
        image = self.server.data / 'media' / (digest + '.img')
        meta = image.with_suffix('.json')
        if not image.is_file() or not meta.is_file(): return self.reply(404, {'error': 'absent'})
        try:
            info = json.loads(meta.read_text()); body = image.read_bytes()
            if info['mime'] not in ['image/jpeg', 'image/png', 'image/webp'] or hashlib.sha256(body).hexdigest() != digest: raise ValueError()
        except (OSError, ValueError, KeyError): return self.reply(404, {'error': 'absent'})
        self.send_response(200); self.send_header('Content-Type', info['mime']); self.send_header('Content-Length', str(len(body)))
        self.send_header('Cache-Control', 'public, max-age=86400'); self.send_header('X-Content-Type-Options', 'nosniff'); self.end_headers(); self.wfile.write(body); return
    def do_GET(self):
        path = self.path.split('?')[0]
        if path.startswith('/v1/emugen/'):
            return self.reply(410,{'error':'EmuGen a été retiré.','status':'removed'})
        if path=='/v1/tv/link':
            from tv_pairing import PAGE
            body=PAGE.encode();self.send_response(200);self.send_header('Content-Type','text/html; charset=utf-8');self.send_header('Content-Length',str(len(body)));self.send_header('Cache-Control','no-store');self.send_header('Referrer-Policy','no-referrer');self.end_headers();self.wfile.write(body);return
        from source_settings import enabled as source_enabled
        artwork=re.fullmatch(r'/v1/(emugen|freeroms|romspure)-art/(.+)',path)
        if artwork and not source_enabled(self.server.data,artwork[1]):
            from source_settings import cached_art
            record=cached_art(self.server.data,artwork[1],artwork[2])
            return self.serve_image(record['id']) if record else self.reply(404,{'error':'Visuel absent du cache ; fournisseur en sourdine.'})
        family=next((name for name in ('emugen','planetemu','freeroms','romspure') if path.startswith('/v1/'+name+'/')),None)
        if family and not path.startswith('/v1/emugen/verification/') and not source_enabled(self.server.data,family):
            if not verified_username(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'authentification requise'})
            if '/games/' in path:return self.reply(409,{'error':'Ce fournisseur est désactivé dans les paramètres.','status':'disabled'})
            return self.reply(200,{'items':[],'disabled':True})
        manual_socket=re.fullmatch(r'/v1/emugen/verification/([a-f0-9]{32})/socket',self.path)
        if manual_socket:
            from emugen_manual import MANUAL
            from private_vnc import serve
            return serve(self,MANUAL,verified_identity,manual_socket[1])
        if path.startswith('/v1/emugen/verification/'):
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            from emugen_manual import MANUAL,ManualError
            from urllib.parse import urlsplit,parse_qs
            try:
                value=MANUAL.current(identity['username'],parse_qs(urlsplit(self.path).query).get('slug',[''])[0]) if path=='/v1/emugen/verification/current' else MANUAL.status(identity['username'],path.removeprefix('/v1/emugen/verification/'))
                return self.reply(200,{'item':value})
            except ManualError as error:return self.reply(error.code,{'error':str(error),'status':error.status})
            except (ValueError,OSError):return self.reply(400,{'error':'Session invalide.'})
        if path in ('/v1/library','/v1/library/folders','/v1/downloads','/v1/download-catalogue'):
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            if path=='/v1/library/folders':
                if identity.get('role_level')!=0:return self.reply(403,{'error':'Navigation réservée à l’administrateur.'})
                from urllib.parse import urlsplit,parse_qs
                folder=parse_qs(urlsplit(self.path).query).get('folder',[''])[0]
                try:return self.reply(200,self.server.library.browse(folder))
                except (ValueError,OSError):return self.reply(400,{'error':'Dossier inaccessible. Revenez au dossier parent.'})
            if path=='/v1/library':return self.reply(200,{**self.server.library.status(),'canConfigure':identity.get('role_level')==0})
            if path=='/v1/downloads':return self.reply(200,{'items':[job for job in self.server.library.jobs(identity['username']) if job.get('gameId','').startswith('personal:')]})
            from urllib.parse import urlsplit,parse_qs
            console=parse_qs(urlsplit(self.path).query).get('console',[''])[0]
            return self.reply(200,{'items':self.server.library.catalogue(console) if source_enabled(self.server.data,'gameflex-free') else []})
        if path == '/v1/emulators':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'Connexion requise.'})
            from emulator_assistant import inventory
            return self.reply(200,{**inventory(self.server.library),'canInstall':identity.get('role_level')==0})
        if path == '/v1/imports':
            if not verified_username(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'authentification requise'})
            from rom_import import metadata,FORMATS
            return self.reply(200,{'items':metadata(self.server.library),'formats':{k:sorted(v) for k,v in FORMATS.items()},'maxBytes':None})
        if path == '/v1/game-sources':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            from source_settings import listing
            return self.reply(200,{**listing(self.server.data),'canConfigure':identity.get('role_level')==0})
        if path == '/health': return self.reply(200, {'ok': True, 'scope': 'process-only'})
        if path.startswith('/v1/emugen-art/'):
            from emugen import artwork
            try:record=artwork(self.server.data,path.removeprefix('/v1/emugen-art/'))
            except (ValueError,OSError):return self.reply(404,{'error':'Image EmuGen indisponible.'})
            return self.serve_image(record['id'])
        if path=='/v1/metadata' or path.startswith('/v1/metadata/jobs/') or path.startswith('/v1/emugen/'):
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            from urllib.parse import urlsplit,parse_qs
            from emugen import search,detail,EmugenError
            query=parse_qs(urlsplit(self.path).query,keep_blank_values=True)
            try:
                if path=='/v1/metadata':
                    from game_metadata import listing
                    return self.reply(200,{'items':listing(self.server.data,query.get('console',[None])[0])})
                if path.startswith('/v1/metadata/jobs/'):
                    from game_metadata import job
                    return self.reply(200,job(self.server.data,identity['username'],path.removeprefix('/v1/metadata/jobs/')))
                if path.startswith('/v1/emugen/games/'):
                    return self.reply(200,{'item':detail(self.server.data,path.removeprefix('/v1/emugen/games/'),query.get('console',[''])[0])})
                return self.reply(200,search(self.server.data,path.removeprefix('/v1/emugen/'),query.get('q',[''])[0],query.get('letter',['A'])[0],query.get('region',[''])[0],query.get('edition',[''])[0],page=query.get('page',[None])[0]))
            except EmugenError as e:return self.reply(404 if e.status=='not_found' else 502,{'error':str(e),'status':e.status})
            except (ValueError,OSError):return self.reply(502,{'error':'Métadonnées indisponibles. Réessayez.','status':'error'})
        if path.startswith('/v1/romspure-art/'):
            from romspure import artwork
            import http.client
            parts=path.split('/')
            if len(parts)!=5:return self.reply(404,{'error':'image absente'})
            try:record=artwork(self.server.data,parts[3],parts[4])
            except (OSError,ValueError,KeyError,http.client.HTTPException):return self.reply(404,{'error':'image indisponible'})
            return self.serve_image(record['id']) if record else self.reply(404,{'error':'image absente'})
        if path.startswith('/v1/romspure/'):
            if not verified_username(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'authentification requise'})
            from romspure import catalogue
            from urllib.parse import urlsplit,parse_qs
            query=parse_qs(urlsplit(self.path).query)
            try:return self.reply(200,catalogue(self.server.data,path.removeprefix('/v1/romspure/'),query.get('letter',['A'])[0],query.get('q',[''])[0]))
            except (OSError,ValueError):return self.reply(400,{'error':'catalogue invalide'})
        if path.startswith('/v1/freeroms-art/'):
            from freeroms import artwork
            parts=path.split('/')
            if len(parts)!=5:return self.reply(404,{'error':'image absente'})
            try:record=artwork(self.server.data,parts[3],parts[4])
            except (OSError,ValueError,KeyError):return self.reply(404,{'error':'image indisponible'})
            return self.serve_image(record['id']) if record else self.reply(404,{'error':'image absente'})
        if path.startswith('/v1/freeroms/'):
            if not verified_username(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'authentification requise'})
            from freeroms import catalogue
            from urllib.parse import urlsplit,parse_qs
            try:return self.reply(200,catalogue(self.server.data,path.removeprefix('/v1/freeroms/'),parse_qs(urlsplit(self.path).query).get('letter',['A'])[0]))
            except (OSError,ValueError):return self.reply(400,{'error':'catalogue invalide'})
        if path.startswith('/v1/covers/'):
            from covers import resolve
            parts=path.split('/')
            if len(parts)!=5:return self.reply(404, {'error':'jaquette absente'})
            try: record=resolve(self.server.data,parts[3],parts[4])
            except (OSError,ValueError,KeyError,StopIteration):return self.reply(404, {'error':'jaquette indisponible'})
            if not record:return self.reply(404, {'error':'jaquette absente'})
            return self.serve_image(record['id'])
        if path.startswith('/v1/planetemu/'):
            from planetemu import catalogue
            from urllib.parse import urlsplit, parse_qs
            if not verified_username(self.headers.get('Authorization'), self.server.auth_base): return self.reply(401, {'error': 'authentification requise'})
            console = path.removeprefix('/v1/planetemu/')
            letter = parse_qs(urlsplit(self.path).query).get('letter', ['A'])[0]
            try:
                result = catalogue(self.server.data, console, letter)
                # Explicitly approved cover-to-catalogue mapping; never guess a protected poster.
                covers = {}
                for meta in (self.server.data / 'media').glob('*.json'):
                    try:
                        record = json.loads(meta.read_text())
                        if record.get('approved') and record.get('kind') == 'game-cover': covers[record.get('target')] = record
                    except (OSError, ValueError): pass
                result = {**result, 'items': [dict(item) for item in result.get('items', [])]}
                from covers import image_url, peek
                for item in result['items']:
                    generated=image_url(console,item)
                    if generated:
                        system,encoded=generated.split('/')[-2:]
                        cached=peek(self.server.data,system,encoded)
                        item.update(image=cached['url'] if cached else generated,attribution='Libretro Thumbnails · Ayants droit du jeu')
                    cover = covers.get('planetemu:' + item.get('path', ''))
                    if cover:
                        item.update(image=cover['url'], attribution=cover.get('author', '') + ' · ' + cover.get('license', ''))
                return self.reply(200, result)
            except ValueError: return self.reply(400, {'error': 'catalogue invalide'})
        if path == '/v1/interfaces': return self.reply(200, {'interfaces': MODES})
        if path == '/v1/compatibility':
            return self.reply(200, json.loads((ROOT / 'deploy/pi-tv/compatibility.json').read_text()))
        if path.startswith('/v1/media/'):
            return self.serve_image(path.removeprefix('/v1/media/'))
        if path == '/v1/media':
            records = []
            for p in sorted((self.server.data / 'media').glob('*.json')):
                try: records.append(json.loads(p.read_text()))
                except (OSError, ValueError): pass
            return self.reply(200, {'items': records})
        if path != '/v1/preferences': return self.reply(404, {'error': 'route inconnue'})
        username = verified_username(self.headers.get('Authorization'), self.server.auth_base)
        if not username: return self.reply(401, {'error': 'authentification requise'})
        file = self.server.data / 'preferences' / (hashlib.sha256(username.encode()).hexdigest() + '.json')
        with LOCK:
            if not file.is_file(): return self.reply(200, {'interface': None, 'favorites': [], 'history': [], 'collections': []})
            try: result = json.loads(file.read_text())
            except (OSError, ValueError): return self.reply(503, {'error': 'préférences indisponibles'})
        return self.reply(200, result)
    def read_small_json(self,limit=4096):
        n=int(self.headers.get('Content-Length','-1'))
        if not 0<n<=limit:raise ValueError('taille refusée')
        obj=json.loads(self.rfile.read(n))
        if not isinstance(obj,dict):raise ValueError('objet requis')
        return obj
    def do_PUT(self):
        if self.path=='/v1/game-sources':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            if identity.get('role_level')!=0:return self.reply(403,{'error':'Réglage réservé à l’administrateur.'})
            from source_settings import save
            try:
                value=save(self.server.data,self.read_small_json().get('enabledIds'))
                if 'emugen' not in value['enabledIds']:
                    from emugen_manual import MANUAL
                    MANUAL.deactivate()
                return self.reply(200,{**value,'canConfigure':True})
            except ValueError:return self.reply(400,{'error':'Liste des fournisseurs invalide.'})
            except OSError:return self.reply(503,{'error':'Enregistrement indisponible.'})
        if self.path=='/v1/metadata/corrections':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            if identity.get('role_level')!=0:return self.reply(403,{'error':'Correction réservée à l’administrateur.'})
            from game_metadata import correct
            try:
                data=self.read_small_json(65536)
                return self.reply(200,{'item':correct(self.server.data,data.get('consoleId'),data.get('reference'),data.get('fields'))})
            except (ValueError,OSError):return self.reply(400,{'error':'Correction invalide.'})
        if self.path=='/v1/library':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            if identity.get('role_level')!=0:return self.reply(403,{'error':'Seul l’administrateur peut choisir le dossier du serveur.'})
            try:return self.reply(200,{**self.server.library.configure(self.read_small_json().get('folder')),'canConfigure':True})
            except ValueError as e:return self.reply(400,{'error':str(e) if e.__class__.__name__=='LibraryError' else 'Dossier invalide.'})
            except OSError:return self.reply(503,{'error':'Configuration indisponible.'})
        if self.path != '/v1/preferences': return self.reply(404, {'error': 'route inconnue'})
        username = verified_username(self.headers.get('Authorization'), self.server.auth_base)
        if not username: return self.reply(401, {'error': 'authentification requise'})
        try:
            n = int(self.headers.get('Content-Length', '-1'))
            if not 0 < n <= 524288: return self.reply(413, {'error': 'taille refusée'})
            result = validate_preferences(json.loads(self.rfile.read(n)))
        except (ValueError, TypeError): return self.reply(400, {'error': 'préférences invalides'})
        folder = self.server.data / 'preferences'; folder.mkdir(parents=True, exist_ok=True, mode=0o700)
        target = folder / (hashlib.sha256(username.encode()).hexdigest() + '.json')
        try:
            with LOCK:
                temp = target.with_suffix('.tmp')
                fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
                with os.fdopen(fd, 'w') as f: json.dump(result, f, ensure_ascii=False)
                temp.replace(target)
        except OSError:
            return self.reply(503, {'error': 'enregistrement indisponible'})
        return self.reply(200, result)
    def upload_session(self,match):
        identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
        if not identity:return self.reply(401,{'error':'Connexion requise.'})
        if identity.get('role_level')!=0:return self.reply(403,{'error':'Import réservé à l’administrateur.'})
        from urllib.parse import urlsplit,parse_qs
        try:
            if self.command=='DELETE':UPLOADS.cancel(identity['username'],match[1]);return self.reply(200,{'cancelled':True})
            if self.path=='/v1/imports/sessions':return self.reply(201,UPLOADS.start(self.server.library,identity['username'],self.read_small_json()))
            if match[2]=='complete':
                self.read_small_json();return self.reply(201,UPLOADS.complete(identity['username'],match[1]))
            size=int(self.headers.get('Content-Length','-1'))
            offset=int(parse_qs(urlsplit(self.path).query).get('offset',['-1'])[0])
            self.connection.settimeout(60)
            return self.reply(200,UPLOADS.chunk(identity['username'],match[1],offset,self.rfile,size))
        except PermissionError:return self.reply(403,{'error':'Import non autorisé.'})
        except (ValueError,OSError) as e:return self.reply(409,{'error':str(e) if isinstance(e,ValueError) else 'Écriture impossible.'})
    def do_POST(self):
        bridge=re.fullmatch(r'/v1/socket-bridge/([a-f0-9]{32})/(poll|send|close)',self.path)
        if self.path=='/v1/socket-bridge' or bridge:
            from socket_bridge import serve
            return serve(self,bridge,verified_identity)
        if self.path=='/v1/covers/search':
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'Connexion requise.'})
            from covers import search
            try:return self.reply(200,search(self.server.data,self.read_small_json(524288).get('items')))
            except ValueError:return self.reply(400,{'error':'Recherche de jaquettes invalide.'})
            except OSError:return self.reply(503,{'error':'Recherche de jaquettes indisponible.'})
        emulator=re.fullmatch(r'/v1/emulators/([a-z0-9]+)/install',self.path)
        if emulator:
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'Connexion requise.'})
            if identity.get('role_level')!=0:return self.reply(403,{'error':'Installation réservée à l’administrateur.'})
            from emulator_assistant import start,EmulatorError
            try:return self.reply(202,{'item':start(self.server.library,emulator[1])})
            except EmulatorError as e:return self.reply(409,{'error':str(e)})
        if self.path=='/v1/downloads':
            if not verified_identity(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'Connexion requise.'})
            return self.reply(409,{'error':'Les fournisseurs servent au catalogue. Importez votre fichier personnel.'})
        upload=re.fullmatch(r'/v1/imports/sessions/([a-f0-9]{32})/(chunk|complete)(?:\?offset=[0-9]+)?',self.path)
        if self.path=='/v1/imports/sessions' or upload:return self.upload_session(upload)
        if self.path.startswith('/v1/tv/pairing'):
            from tv_pairing import create,approve,poll,PairError
            try:
                data=self.read_small_json()
                if self.path=='/v1/tv/pairing':return self.reply(201,create(data.get('challenge'),self.client_address[0]))
                if self.path=='/v1/tv/pairing/approve':
                    identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
                    if not identity:return self.reply(401,{'error':'Connexion GameFlex requise.'})
                    return self.reply(200,approve(data.get('code'),self.headers['Authorization'].removeprefix('Bearer '),identity['username']))
                match=re.fullmatch('/v1/tv/pairing/([a-f0-9]{32})/poll',self.path)
                if match:return self.reply(200,poll(match[1],data.get('secret')))
                return self.reply(404,{'error':'Association introuvable.'})
            except PairError as error:return self.reply(400,{'error':str(error)})
            except (ValueError,OSError):return self.reply(400,{'error':'Association invalide.'})
        if self.path.startswith('/v1/emugen/') or self.path in ('/v1/metadata/enrich','/v1/metadata/bind','/v1/metadata/refresh'):
            from source_settings import enabled
            if not enabled(self.server.data,'emugen'):
                if not verified_username(self.headers.get('Authorization'),self.server.auth_base):return self.reply(401,{'error':'authentification requise'})
                return self.reply(409,{'error':'EmuGen est désactivé dans les paramètres.','status':'disabled'})
        manual_transport=re.fullmatch(r'/v1/emugen/verification/([a-f0-9]{32})/transport',self.path)
        if manual_transport:
            from emugen_manual import MANUAL
            from private_vnc_http import serve_http
            return serve_http(self,MANUAL,verified_identity,manual_transport[1])
        manual_start=self.path=='/v1/emugen/verification'
        manual_resume=re.fullmatch(r'/v1/emugen/verification/([a-f0-9]{32})/resume',self.path)
        if manual_start or manual_resume:
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            from emugen_manual import MANUAL,ManualError
            from emugen import EmugenError
            try:
                if manual_resume:return self.reply(200,MANUAL.resume(identity['username'],manual_resume[1]))
                data=self.read_small_json()
                item=MANUAL.start(identity['username'],data.get('slug'),data.get('consoleId'),self.server.data,background=True)
                return self.reply(202 if item['status']=='starting' else 201,{'item':item})
            except ManualError as error:return self.reply(error.code,{'error':str(error),'status':error.status})
            except EmugenError as error:return self.reply(404 if error.status=='not_found' else 502,{'error':str(error),'status':error.status})
            except (ValueError,OSError):return self.reply(400,{'error':'Demande de vérification invalide.'})
        if self.path in ('/v1/emugen/sources','/v1/metadata/enrich','/v1/metadata/bind','/v1/metadata/refresh'):
            identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
            if not identity:return self.reply(401,{'error':'authentification requise'})
            from emugen import EmugenError
            try:
                data=self.read_small_json(524288 if self.path.endswith('/refresh') else 65536)
                if self.path=='/v1/emugen/sources':
                    from emugen_browser import RESOLVER
                    from emugen_manual import MANUAL
                    if MANUAL.has_active():
                        current=MANUAL.current(identity['username'],data.get('slug'))
                        if current and current.get('status')=='ready' and current.get('result'):return self.reply(200,current['result'])
                        return self.reply(200,{'status':'challenge_required','links':[],'message':'Utilisez la fenêtre de vérification, puis Reprendre la recherche.'})
                    return self.reply(200,RESOLVER.resolve(self.server.data,data.get('slug')))
                if self.path=='/v1/metadata/enrich':
                    from emugen import detail
                    from game_metadata import clean_fields
                    from metadata_enrichment import enrich
                    game=clean_fields(data.get('game',{}))
                    if not game.get('consoleId'):raise EmugenError('invalid','Console requise.')
                    primary=detail(self.server.data,data.get('slug'),game['consoleId'])
                    variants=data.get('variants',[])
                    if not isinstance(variants,list):raise EmugenError('invalid','Éditions invalides.')
                    game['manualMetadata']=clean_fields(data.get('game',{}).get('manualMetadata',{}))
                    return self.reply(200,{'item':enrich(self.server.data,game,primary,variants)})
                if identity.get('role_level')!=0:return self.reply(403,{'error':'Métadonnées du serveur réservées à l’administrateur.'})
                from game_metadata import bind,refresh
                if self.path.endswith('/bind'):return self.reply(200,{'item':bind(self.server.data,data.get('game',{}),data.get('slug'))})
                return self.reply(202,refresh(self.server.data,identity['username'],data.get('items')))
            except EmugenError as e:return self.reply(409 if e.status=='busy' else 400,{'error':str(e),'status':e.status})
            except (ValueError,OSError):return self.reply(400,{'error':'Demande de métadonnées invalide.'})
        if self.path.split('?')[0]=='/v1/imports':return self.import_rom()
        if self.path!='/v1/downloads':return self.reply(405,{'error':'méthode refusée'})
        identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
        if not identity:return self.reply(401,{'error':'authentification requise'})
        try:
            data=self.read_small_json()
            from source_settings import enabled
            game_id=data.get('gameId');family=game_id.split(':',1)[0] if isinstance(game_id,str) and ':' in game_id else 'gameflex-free'
            if not enabled(self.server.data,family):return self.reply(409,{'error':'Ce fournisseur est désactivé dans les paramètres.'})
            presentation=data.get('presentation')
            if isinstance(presentation,dict):
                candidates=presentation.get('candidateIds',[])
                if not isinstance(candidates,list) or len(candidates)>4 or any(not isinstance(candidate,str) for candidate in candidates):raise ValueError('Sources invalides.')
                presentation={**presentation,'candidateIds':[candidate for candidate in candidates if enabled(self.server.data,candidate.split(':',1)[0])]}
            return self.reply(202,{'item':self.server.library.start(identity['username'],game_id,presentation)})
        except ValueError as e:return self.reply(409,{'error':str(e) if e.__class__.__name__=='LibraryError' else 'Demande invalide.'})
        except OSError:return self.reply(503,{'error':'Installation indisponible.'})
    def import_rom(self):
        self.close_connection=True
        identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
        if not identity:return self.reply(401,{'error':'authentification requise'})
        if identity.get('role_level')!=0:return self.reply(403,{'error':'Import réservé à l’administrateur.'})
        from rom_import import validate,install_stream
        from urllib.parse import urlsplit,parse_qs,unquote
        if not UPLOAD_SLOT.acquire(blocking=False):return self.reply(409,{'error':'Un import est déjà en cours. Réessayez dans un instant.'})
        try:
            query=parse_qs(urlsplit(self.path).query)
            console=query.get('console',[''])[0]
            name=validate(console,unquote(self.headers.get('X-GameFlex-Filename','')))
            size=int(self.headers.get('Content-Length','-1'))
            if self.headers.get('Transfer-Encoding') or size<=0:return self.reply(411,{'error':'La taille du fichier doit être indiquée et supérieure à zéro.'})
            self.connection.settimeout(300)
            item=install_stream(self.server.library,console,name,self.rfile,size)
            return self.reply(201,{'item':item})
        except ValueError as e:return self.reply(400,{'error':str(e) if e.__class__.__name__=='LibraryError' else 'Fichier incomplet ou demande invalide.'})
        except OSError:return self.reply(503,{'error':'Import impossible. Vérifiez le dossier et l’espace disponible.'})
        finally:UPLOAD_SLOT.release()
    def do_DELETE(self):
        upload=re.fullmatch(r'/v1/imports/sessions/([a-f0-9]{32})',self.path)
        if upload:return self.upload_session(upload)
        match=re.fullmatch(r'/v1/emugen/verification/([a-f0-9]{32})',self.path)
        if not match:return self.reply(405,{'error':'méthode refusée'})
        identity=verified_identity(self.headers.get('Authorization'),self.server.auth_base)
        if not identity:return self.reply(401,{'error':'authentification requise'})
        from emugen_manual import MANUAL,ManualError
        try:return self.reply(200,MANUAL.close_session(identity['username'],match[1]))
        except ManualError as error:return self.reply(error.code,{'error':str(error),'status':error.status})
    def do_PATCH(self):return self.reply(405,{'error':'méthode refusée'})

class BoundedHTTPServer(ThreadingHTTPServer):
    request_queue_size = 16
    daemon_threads = True
    def __init__(self, *args, **kwargs):
        self.slots = threading.BoundedSemaphore(16)
        super().__init__(*args, **kwargs)
    def process_request(self, request, client_address):
        if not self.slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        try: super().process_request(request, client_address)
        except Exception:
            self.slots.release()
            raise
    def process_request_thread(self, request, client_address):
        try: super().process_request_thread(request, client_address)
        finally: self.slots.release()
    def handle_error(self, request, client_address):
        # Never dump request details or credentials on unexpected errors.
        pass

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bind', default='127.0.0.1'); p.add_argument('--port', type=int, default=10211)
    p.add_argument('--data', type=Path, default=ROOT / '.local/pi-tv'); p.add_argument('--auth-base', default='http://127.0.0.1:10200')
    p.add_argument('--default-roms',action='store_true');p.add_argument('--library-host',default='');p.add_argument('--library-mount',type=Path,default=Path('/library'))
    a = p.parse_args()
    if not re.fullmatch(r'http://(127\.0\.0\.1|localhost|gameflex-edge|gameflex-credential)(:\d{1,5})?', a.auth_base): p.error('auth-base doit être un service local reconnu')
    server = BoundedHTTPServer((a.bind, a.port), Handler)
    server.data = a.data.resolve(); server.auth_base = a.auth_base
    from library import Library
    server.library=Library(server.data,a.library_host,a.library_mount,a.default_roms)
    from game_metadata import recover_jobs
    recover_jobs(server.data)
    server.serve_forever()
if __name__ == '__main__': main()
