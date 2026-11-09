"""Personal-use boxart metadata from Libretro's documented thumbnail service.
Only Named_Boxarts PNGs are fetched. No ROM, firmware, screenshot or game endpoint.
"""
from collections import OrderedDict
import base64
from functools import lru_cache
import hashlib
import json
from pathlib import Path
import re
import threading
import time
import unicodedata
import urllib.parse
import urllib.request
from artwork import cache_bytes

SYSTEMS = {'nes':'Nintendo - Nintendo Entertainment System','fds':'Nintendo - Family Computer Disk System','gbc':'Nintendo - Game Boy','gbc-color':'Nintendo - Game Boy Color','gba':'Nintendo - Game Boy Advance','snes':'Nintendo - Super Nintendo Entertainment System','n64':'Nintendo - Nintendo 64','nds':'Nintendo - Nintendo DS','gc':'Nintendo - GameCube','wii':'Nintendo - Wii','vb':'Nintendo - Virtual Boy','sms':'Sega - Master System - Mark III','megadrive':'Sega - Mega Drive - Genesis','saturn':'Sega - Saturn','dreamcast':'Sega - Dreamcast','ps1':'Sony - PlayStation','ps2':'Sony - PlayStation 2','psp':'Sony - PlayStation Portable','pce':'NEC - PC Engine - TurboGrafx 16','pcfx':'NEC - PC-FX','atari2600':'Atari - 2600','atari7800':'Atari - 7800','jaguar':'Atari - Jaguar','lynx':'Atari - Lynx','coleco':'Coleco - ColecoVision','c64':'Commodore - 64','amstrad':'Amstrad - CPC','ngp':'SNK - Neo Geo Pocket','wswan':'Bandai - WonderSwan','vectrex':'GCE - Vectrex','zxspectrum':'Sinclair - ZX Spectrum'}
INDEX_LOCKS={key:threading.RLock() for key in SYSTEMS}
IMAGE_LOCKS=[threading.Lock() for _ in range(64)]
NETWORK_SLOTS=threading.BoundedSemaphore(4)
ENCODE_SLOTS=threading.BoundedSemaphore(2)
RATE_LOCK=threading.Lock()
WRITE_LOCK=threading.Lock()
LAST=0.0
COVER_VERSION=3
MISSES=OrderedDict()
MISS_LOCK=threading.Lock()
def missed(key):
    with MISS_LOCK:
        deadline=MISSES.get(key,0)
        if deadline>time.monotonic():return True
        MISSES.pop(key,None);return False

def remember_miss(key,seconds):
    with MISS_LOCK:
        MISSES.pop(key,None);MISSES[key]=time.monotonic()+seconds
        while len(MISSES)>2048:MISSES.popitem(last=False)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs):return None

def fetch(url,limit):
    global LAST
    u=urllib.parse.urlsplit(url)
    if u.scheme!='https' or u.hostname not in ('api.github.com','raw.githubusercontent.com') or u.username or u.password or u.port not in (None,443):raise ValueError('provider refused')
    prefix='/repos/libretro-thumbnails/' if u.hostname=='api.github.com' else '/libretro-thumbnails/'
    if not u.path.startswith(prefix):raise ValueError('provider path refused')
    with NETWORK_SLOTS:
        with RATE_LOCK:
            start=max(time.monotonic(),LAST+.12);LAST=start
        time.sleep(max(0,start-time.monotonic()))
        request=urllib.request.Request(url,headers={'User-Agent':'GameFlex-Boxart/2.0','Accept':'application/vnd.github+json' if u.hostname=='api.github.com' else 'image/png'})
        with urllib.request.build_opener(NoRedirect).open(request,timeout=6) as r:
            body=r.read(limit+1)
            if len(body)>limit:raise ValueError('response too large')
            return body,r.headers.get_content_type()

def clean_title(title):
    # Availability labels belong to the provider UI, not the game title.
    return re.sub(r'(?<=[)\]])\s+(?:Retirée?|Removed|Withdrawn|Unavailable)\s*$','',title,flags=re.I).strip()

def normalized(title):
    base=re.sub(r'\([^)]*\)|\[[^]]*\]','',clean_title(title))
    base=unicodedata.normalize('NFKD',base).casefold()
    return ''.join(c for c in base if c.isalnum() and not unicodedata.combining(c))

@lru_cache(maxsize=4)
def groups(path,mtime):
    result={}
    for name in json.loads(Path(path).read_text()):
        if isinstance(name,str) and len(name)<500 and name.endswith('.png') and not re.search(r'[/\\\x00-\x1f]',name) and '..' not in name:
            result.setdefault(normalized(name[:-4]),[]).append(name)
    return result

def filename(folder,system,title,refresh=True):
    folder=Path(folder)/'cover-index';folder.mkdir(exist_ok=True)
    index=folder/(system+'.json')
    if not refresh and not index.exists():return None
    if refresh and (not index.exists() or time.time()-index.stat().st_mtime>7*86400):
        try:
            repo=SYSTEMS[system].replace(' ','_');base='https://api.github.com/repos/libretro-thumbnails/'+repo+'/git/trees/'
            root=json.loads(fetch(base+'master',1024*1024)[0]);sha=next(x['sha'] for x in root['tree'] if x['path']=='Named_Boxarts')
            if not re.fullmatch('[a-f0-9]{40}',sha):raise ValueError('tree refused')
            tree=json.loads(fetch(base+sha,8*1024*1024)[0])
            if tree.get('truncated'):raise ValueError('incomplete cover index')
            names=[x['path'] for x in tree['tree'] if x.get('type')=='blob' and x['path'].endswith('.png')]
            temp=index.with_suffix('.tmp');temp.write_text(json.dumps(names));temp.replace(index)
        except (OSError,ValueError,KeyError,StopIteration):
            if not index.exists():raise
    matches=groups(str(index),index.stat().st_mtime_ns).get(normalized(title),[])
    title=clean_title(title)
    exact=re.sub(r'[&*/:`<>?\\|"*]','_',title)+'.png'
    if exact in matches:return exact
    regions=re.findall(r'\((USA|Europe|Japan|World)[^)]*\)',title)
    def rank(name):
        return (0 if any('('+r in name for r in regions) else 1, bool(re.search(r'\b(Beta|Proto|Hack|Unl)\b',name)),len(name),name)
    return min(matches,key=rank) if matches else None

def image_url(console,item):
    system='fds' if '/nintendo-family-computer-disk-system' in item.get('path','') else 'gbc-color' if '/nintendo-game-boy-color/' in item.get('path','') else console
    if system not in SYSTEMS:return None
    title=item.get('title','')
    if not isinstance(title,str) or not 0<len(title)<=400:return None
    encoded=base64.urlsafe_b64encode(title.encode()).decode().rstrip('=')
    return '/experience-api/v1/covers/'+system+'/'+encoded

def decode(system,encoded):
    if system not in SYSTEMS or not re.fullmatch('[A-Za-z0-9_-]{1,800}',encoded):raise ValueError('cover identifier refused')
    try:title=base64.urlsafe_b64decode(encoded+'='*((-len(encoded))%4)).decode('utf-8')
    except (ValueError,UnicodeError):raise ValueError('cover title refused')
    if not 0<len(title)<=400 or re.search(r'[\x00-\x1f]',title):raise ValueError('cover title refused')
    return clean_title(title)

def mapping_path(folder,system,name):
    return Path(folder)/'cover-index'/(hashlib.sha256((system+'\0'+name).encode()).hexdigest()+'.cover.json')

def mapped_record(folder,mapping):
    try:
        record=json.loads(mapping.read_text())
        if record.get('reviewAfter',0)>time.time() and (Path(folder)/'media'/(record['id']+'.img')).exists():return record
    except (OSError,ValueError,KeyError):pass
    return None

def peek(folder,system,encoded):
    """Disk/memory only: catalogue requests never wait on artwork downloads."""
    title=decode(system,encoded)
    try:
        name=filename(folder,system,title,refresh=False)
        record=mapped_record(folder,mapping_path(folder,system,name)) if name else None
        return record if record and record.get('coverVersion')==COVER_VERSION else None
    except (OSError,ValueError,KeyError):return None

def optimize_image(body,mime):
    from io import BytesIO
    from PIL import Image,ImageOps
    if mime not in ('image/png','image/jpeg','image/webp'):raise ValueError('image MIME refused')
    with ENCODE_SLOTS:
        with Image.open(BytesIO(body)) as source:
            if source.width*source.height>4_000_000:raise ValueError('image dimensions refused')
            image=ImageOps.exif_transpose(source).convert('RGB')
            image.thumbnail((384,512),Image.Resampling.LANCZOS)
            output=BytesIO();image.save(output,'WEBP',quality=82,method=4)
            return output.getvalue()

def resolve(folder,system,encoded):
    title=decode(system,encoded)
    # Fast cached hits never wait for an index refresh or another download.
    ready=peek(folder,system,encoded)
    if ready:return ready
    miss_key=(str(folder),system,encoded)
    if missed(miss_key):return None
    # Only the index of this system is serialized; cached images are independent.
    with INDEX_LOCKS[system]:name=filename(folder,system,title)
    if not name:
        remember_miss(miss_key,900);return None
    mapping=mapping_path(folder,system,name)
    stripe=int(hashlib.sha256((system+'\0'+name).encode()).hexdigest()[:8],16)%len(IMAGE_LOCKS)
    with IMAGE_LOCKS[stripe]:
        record=mapped_record(folder,mapping)
        if record and record.get('coverVersion')==COVER_VERSION:return record
        if record:
            body=(Path(folder)/'media'/(record['id']+'.img')).read_bytes();mime=record['mime']
        else:
            repo=SYSTEMS[system].replace(' ','_')
            url='https://raw.githubusercontent.com/libretro-thumbnails/'+repo+'/master/Named_Boxarts/'+urllib.parse.quote(name,safe='')
            if missed(miss_key):return None
            try:body,mime=fetch(url,2*1024*1024)
            except (OSError,ValueError):
                remember_miss(miss_key,30);raise
            record={'provider':'Libretro Thumbnails','title':name[:-4],'author':'Ayants droit du jeu · collection Libretro','license':'Illustration protégée · consultation personnelle','licenseUrl':'https://docs.libretro.com/guides/roms-playlists-thumbnails/#thumbnails','source':'https://github.com/libretro-thumbnails/'+repo+'/blob/master/Named_Boxarts/'+urllib.parse.quote(name,safe=''),'approved':True,'kind':'game-cover','target':'libretro:'+system+':'+name}
        thumbnail=optimize_image(body,mime)
        record={k:v for k,v in record.items() if k not in ('id','url','mime','fetchedAt','reviewAfter')}
        record.update(coverVersion=COVER_VERSION,modifications='Miniature WebP, maximum 384 × 512 pixels ; sans recadrage.')
        with WRITE_LOCK:record=cache_bytes(Path(folder)/'media',thumbnail,'image/webp',record)
        temp=mapping.with_suffix('.tmp');temp.write_text(json.dumps(record));temp.replace(mapping)
        return record

# Only the requested first page is prefetched, never an entire system or site.
from concurrent.futures import ThreadPoolExecutor
PREFETCH_POOL=ThreadPoolExecutor(max_workers=3,thread_name_prefix='boxart')
PENDING=set()
PENDING_LOCK=threading.Lock()
def prefetch(folder,items):
    for item in items[:24]:
        url=item.get('image','')
        if not url.startswith('/experience-api/v1/covers/'):continue
        system,encoded=url.split('/')[-2:];key=(system,encoded)
        with PENDING_LOCK:
            if key in PENDING or len(PENDING)>=32:continue
            PENDING.add(key)
        def job(system=system,encoded=encoded,key=key):
            try:resolve(folder,system,encoded)
            except (OSError,ValueError,KeyError,StopIteration):pass
            finally:
                with PENDING_LOCK:PENDING.discard(key)
        PREFETCH_POOL.submit(job)
