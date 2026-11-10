#!/usr/bin/env python3
"""Explicit local artwork importer. Fixed providers only; never searches ROMs/BIOS."""
import argparse
import hashlib
from html import unescape
import json
import os
from pathlib import Path
import re
import threading
import time
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
ALLOWED = {'commons.wikimedia.org', 'upload.wikimedia.org', 'thumb.wikimedia.org', 'www.flickr.com', 'live.staticflickr.com', 'api.igdb.com', 'id.twitch.tv', 'images.igdb.com'}
MAX_IMAGE = 5 * 1024 * 1024
CACHE_BUDGET = 256 * 1024 * 1024
FETCH_LOCK = threading.Lock()
LAST_REQUEST = 0.0
class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        check_url(newurl)
        # OAuth/IGDB credentials must never follow redirects.
        if req.has_header('Authorization') or req.host == 'id.twitch.tv': raise ValueError('redirection authentifiée refusée')
        return super().redirect_request(req, fp, code, msg, headers, newurl)

def check_url(url):
    u = urllib.parse.urlsplit(url)
    if u.scheme != 'https' or u.hostname not in ALLOWED or u.port not in (None, 443) or u.username or u.password:
        raise ValueError('URL de fournisseur refusée')

def fetch(url, data=None, headers=None, limit=MAX_IMAGE):
    global LAST_REQUEST
    check_url(url)
    with FETCH_LOCK:
        time.sleep(max(0, 1.05 - (time.monotonic() - LAST_REQUEST)))
        LAST_REQUEST = time.monotonic()
        request = urllib.request.Request(url, data=data, headers={'User-Agent': 'GameFlex-Artwork/1.0 (https://github.com/zinosaure/gameflex)', **(headers or {})})
        with urllib.request.build_opener(SafeRedirect).open(request, timeout=15) as r:
            if r.headers.get_content_type() == 'image/svg+xml': raise ValueError('SVG distant refusé')
            body = r.read(limit + 1)
            if len(body) > limit: raise ValueError('réponse trop volumineuse')
            return body, r.headers.get_content_type()

def get_json(url, **kwargs):
    body, _ = fetch(url, **kwargs)
    return json.loads(body)

def text(value): return unescape(re.sub('<[^>]*>', '', value or '')).strip()

def cache_image(folder, url, record):
    body, mime = fetch(url)
    return cache_bytes(folder, body, mime, {**record, "sourceImage": url})

def cache_bytes(folder, body, mime, record):
    magic = (body.startswith(b'\xff\xd8\xff') and mime == 'image/jpeg') or (body.startswith(b'\x89PNG\r\n\x1a\n') and mime == 'image/png') or (body.startswith(b'RIFF') and body[8:12] == b'WEBP' and mime == 'image/webp')
    if not magic: raise ValueError('format image refusé')
    folder.mkdir(parents=True, exist_ok=True)
    if sum(f.stat().st_size for f in folder.glob('*.img')) + len(body) > CACHE_BUDGET: raise ValueError('cache plein ; supprimer explicitement des images')
    digest = hashlib.sha256(body).hexdigest()
    record.update({'id': digest, 'url': '/experience-api/v1/media/' + digest, 'mime': mime, 'fetchedAt': int(time.time()), 'reviewAfter': int(time.time()) + 30 * 86400})
    for suffix, content in [('.img', body), ('.json', json.dumps(record, ensure_ascii=False).encode())]:
        target = folder / (digest + suffix); temp = target.with_suffix(suffix + '.tmp')
        temp.write_bytes(content); temp.replace(target)
    return record

def commons(query, folder):
    params = {'action':'query','format':'json','generator':'search','gsrsearch':query + ' filetype:bitmap','gsrnamespace':6,'gsrlimit':5,'prop':'imageinfo','iiprop':'url|extmetadata','iiurlwidth':800,'maxlag':5}
    obj = get_json('https://commons.wikimedia.org/w/api.php?' + urllib.parse.urlencode(params))
    if 'error' in obj: raise ValueError('API indisponible')
    for page in obj.get('query', {}).get('pages', {}).values():
        info = page.get('imageinfo', [{}])[0]; meta = info.get('extmetadata', {})
        get = lambda k: text(meta.get(k, {}).get('value', ''))
        # Strict redistribution gate; no fair-use or unidentified rights cached.
        license_name = get('LicenseShortName')
        if not re.match(r'^(CC BY(?:-SA)?(?: (?:1\.0|2\.0|2\.5|3\.0|4\.0))?|CC0|Public domain)$', license_name, re.I): continue
        if license_name.startswith('CC BY') and (not get('Artist') or not get('LicenseUrl')): continue
        yield cache_image(folder, info.get('thumburl') or info['url'], {'provider':'Wikimedia Commons','title':page['title'],'author':get('Artist'),'license':license_name,'licenseUrl':get('LicenseUrl'),'source':info.get('descriptionurl'),'attribution':get('Attribution') or get('Artist'),'query':query,'kind':'console-photo'})

def igdb(query, folder):
    client = os.environ.get('IGDB_CLIENT_ID'); secret = os.environ.get('IGDB_CLIENT_SECRET')
    if not client or not secret: raise ValueError('identifiants IGDB requis dans l’environnement')
    # Secret travels in HTTPS POST body, never in command arguments or a URL.
    token = get_json('https://id.twitch.tv/oauth2/token', data=urllib.parse.urlencode({'client_id':client,'client_secret':secret,'grant_type':'client_credentials'}).encode(), headers={'Content-Type':'application/x-www-form-urlencoded'})['access_token']
    escaped = query.replace('\\', '\\\\').replace('"', '\\"').replace('\n', ' ').replace('\r', ' ')
    games = get_json('https://api.igdb.com/v4/games', data=f'search "{escaped}"; fields name,url,cover.image_id; limit 5;'.encode(), headers={'Client-ID':client,'Authorization':'Bearer ' + token,'Content-Type':'text/plain'})
    for game in games:
        image_id = game.get('cover', {}).get('image_id', '')
        if not re.fullmatch('[a-zA-Z0-9_]+', image_id): continue
        yield cache_image(folder, f'https://images.igdb.com/igdb/image/upload/t_cover_big/{image_id}.jpg', {'provider':'IGDB','title':game['name'],'author':'Ayants droit du jeu','license':'Twitch Developer Service Agreement — usage non commercial ; illustration protégée','licenseUrl':'https://www.twitch.tv/p/en/legal/developer-agreement/','source':game.get('url'),'attribution':'Game data and covers: IGDB','query':query,'kind':'game-cover'})

def main():
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('provider', choices=['commons','igdb']); p.add_argument('query'); p.add_argument('--cache', type=Path, default=ROOT / '.local/pi-tv/media'); p.add_argument('--accept-igdb-terms', action='store_true'); p.add_argument('--target', help='Identifiant console ou console:reference de jeu'); p.add_argument('--approve-index', type=int, help='Index candidat à approuver après vérification des métadonnées')
    a = p.parse_args()
    if a.provider == 'igdb' and not a.accept_igdb_terms: p.error('lire les conditions IGDB puis ajouter --accept-igdb-terms')
    try:
        records = list((commons if a.provider == 'commons' else igdb)(a.query, a.cache))
        if a.approve_index is not None:
            if not a.target or not 0 <= a.approve_index < len(records): raise ValueError('cible ou index invalide')
            record = records[a.approve_index]
            record.update({'approved': True, 'target': a.target})
            (a.cache / (record['id'] + '.json')).write_text(json.dumps(record, ensure_ascii=False))
        print(json.dumps({'items': records}, ensure_ascii=False, indent=2))
    except Exception:
        # Upstream errors can include credentials; never echo them.
        p.exit(1, 'Import indisponible : réseau, quota, droits ou configuration à vérifier. Aucun secret affiché.\n')
if __name__ == '__main__': main()
