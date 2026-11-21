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

SYSTEMS = {'nes':'Nintendo - Nintendo Entertainment System','fds':'Nintendo - Family Computer Disk System','gbc':'Nintendo - Game Boy','gbc-color':'Nintendo - Game Boy Color','gba':'Nintendo - Game Boy Advance','snes':'Nintendo - Super Nintendo Entertainment System','n64':'Nintendo - Nintendo 64','nds':'Nintendo - Nintendo DS','3ds':'Nintendo - Nintendo 3DS','gc':'Nintendo - GameCube','wii':'Nintendo - Wii','wiiu':'Nintendo - Wii U','vb':'Nintendo - Virtual Boy','sms':'Sega - Master System - Mark III','megadrive':'Sega - Mega Drive - Genesis','saturn':'Sega - Saturn','dreamcast':'Sega - Dreamcast','ps1':'Sony - PlayStation','ps2':'Sony - PlayStation 2','psp':'Sony - PlayStation Portable','pce':'NEC - PC Engine - TurboGrafx 16','pcfx':'NEC - PC-FX','atari2600':'Atari - 2600','atari7800':'Atari - 7800','jaguar':'Atari - Jaguar','lynx':'Atari - Lynx','coleco':'Coleco - ColecoVision','c64':'Commodore - 64','amstrad':'Amstrad - CPC','ngp':'SNK - Neo Geo Pocket','neogeo':'SNK - Neo Geo','cps1':'FBNeo - Arcade Games','cps2':'FBNeo - Arcade Games','doom':'DOOM','wswan':'Bandai - WonderSwan','vectrex':'GCE - Vectrex','zxspectrum':'Sinclair - ZX Spectrum'}
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
    # Libretro filenames replace a few forbidden title characters; a spaced
    # underscore commonly represents an ampersand (Nintendogs, Touch & Roll).
    base=re.sub(r'\s+_\s+',' and ',base).replace('&',' and ')
    base=unicodedata.normalize('NFKD',base).casefold()
    return ''.join(c for c in base if c.isalnum() and not unicodedata.combining(c))

def token_key(title):
    """Order-insensitive signature for harmless catalogue punctuation changes."""
    base=re.sub(r'\([^)]*\)|\[[^]]*\]','',clean_title(title))
    base=re.sub(r'\s+_\s+',' and ',base).replace('&',' and ')
    base=unicodedata.normalize('NFKD',base).casefold()
    words=re.findall(r'[a-z0-9]+',''.join(c for c in base if not unicodedata.combining(c)))
    synonyms={'brothers':'bros','islands':'island'}
    return '\0'.join(sorted(synonyms.get(word,word) for word in words))

# Localised commercial names sometimes bear no resemblance to the canonical
# No-Intro title used by Libretro. These are stable title aliases, not file or
# download locations; matching still stays inside the selected console index.
TITLE_ALIASES={
    'disneyprincessemagique':'disneyprincessmagicaljewels',
    'leapassionbebes':'imaginebabies',
    'leapassioncuisine':'imaginehappycooking',
    'leapassionmaitressedecole':'imagineteacher',
    'leapassionanimaldoctor':'imagineanimaldoctor',
    'leapassionmode':'imaginefashiondesigner',
    'leapassionpatinage':'imaginefigureskater',
    'leapassionsecretsdefilles':'mysecretworldbyimagine',
    'leapassionpresentemessecretsdefilles':'mysecretworldbyimagine',
    'leapassionstardeladanse':'imaginemoderndancer',
    'leapassionstardelapop':'imaginerockstar',
}

def title_keys(title):
    """Canonical and scene-release variants, ordered from safest to loosest."""
    title=clean_title(title)
    cleaned=re.sub(r'^\s*[\[(](?:NDS|N64|GBA|GBC|GB|SNES|NES|PS[12]|WII(?:\s*U)?|3DS|GC)[\])]\s*','',title,flags=re.I)
    cleaned=re.sub(r'[-_ ]+Jeux\s+(?:DS|N64|GBA|GBC|GB|SNES|NES|PS[12]|WII(?:\s*U)?|3DS|GC)\s*$','',cleaned,flags=re.I)
    variants=[title,cleaned]
    # Scene archives prepend a release number in several incompatible forms.
    # Keep the original candidate too, so a real title beginning with a year
    # or a number is never destroyed by the cleanup.
    for pattern in (r'^\s*\d{3,5}\s*[-_.]\s*',r'^\s*\d{3,5}\s+',r'^\s*(?:0?[1-9]|1\d|20)\s+'):
        variants.append(re.sub(pattern,'',cleaned))
    result=[]
    for variant in variants:
        key=normalized(variant)
        if key and key not in result:result.append(key)
        alias=TITLE_ALIASES.get(key)
        if alias and alias not in result:result.append(alias)
    return result

def title_variants(title):
    """Readable variants used for both compact and token-set matching."""
    title=clean_title(title)
    cleaned=re.sub(r'^\s*[\[(](?:NDS|N64|GBA|GBC|GB|SNES|NES|PS[12]|WII(?:\s*U)?|3DS|GC)[\])]\s*','',title,flags=re.I)
    cleaned=re.sub(r'[-_ ]+Jeux\s+(?:DS|N64|GBA|GBC|GB|SNES|NES|PS[12]|WII(?:\s*U)?|3DS|GC)\s*$','',cleaned,flags=re.I)
    variants=[title,cleaned]
    for pattern in (r'^\s*\d{3,5}\s*[-_.]\s*',r'^\s*\d{3,5}\s+',r'^\s*(?:0?[1-9]|1\d|20)\s+'):
        candidate=re.sub(pattern,'',cleaned)
        if candidate not in variants:variants.append(candidate)
    return variants

@lru_cache(maxsize=4)
def groups(path,mtime):
    result={}
    for name in json.loads(Path(path).read_text()):
        if isinstance(name,str) and len(name)<500 and name.endswith('.png') and not re.search(r'[/\\\x00-\x1f]',name) and '..' not in name:
            result.setdefault(normalized(name[:-4]),[]).append(name)
    return result

@lru_cache(maxsize=4)
def token_groups(path,mtime):
    result={}
    for name in json.loads(Path(path).read_text()):
        if isinstance(name,str) and len(name)<500 and name.endswith('.png') and not re.search(r'[/\\\x00-\x1f]',name) and '..' not in name:
            result.setdefault(token_key(name[:-4]),[]).append(name)
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
    indexed=groups(str(index),index.stat().st_mtime_ns);matches=[]
    for key in title_keys(title):
        matches=indexed.get(key,[])
        if matches:break
    if not matches:
        token_index=token_groups(str(index),index.stat().st_mtime_ns)
        for variant in title_variants(title):
            matches=token_index.get(token_key(variant),[])
            if matches:break
    if not matches:
        # A dump often omits a subtitle ("Nintendo Touch Golf" versus
        # "Nintendo Touch Golf - Birdie Challenge"). Accept containment only
        # when it identifies exactly one canonical title in this console.
        candidate_keys=set()
        for key in title_keys(title):
            if len(key)<8:continue
            candidate_keys.update(name for name in indexed if len(name)>=8 and (key in name or name in key))
        if len(candidate_keys)==1:matches=indexed[next(iter(candidate_keys))]
    title=clean_title(title)
    exact=re.sub(r'[&*/:`<>?\\|"*]','_',title)+'.png'
    if exact in matches:return exact
    regions=re.findall(r'\((USA|Europe|Japan|World)[^)]*\)',title)
    if not regions:
        shorthand=re.findall(r'\((E|EU|U|J)\)',title,re.I)
        regions=[{'e':'Europe','eu':'Europe','u':'USA','j':'Japan'}[value.casefold()] for value in shorthand]
    def rank(name):
        return (0 if any('('+r in name for r in regions) else 1, bool(re.search(r'\b(Beta|Proto|Hack|Unl)\b',name)),len(name),name)
    return min(matches,key=rank) if matches else None

def artwork_system(console,item):
    path=item.get('path','') if isinstance(item.get('path',''),str) else ''
    files=item.get('files',[])
    if not isinstance(files,list):files=[]
    if console=='nes' and '/nintendo-family-computer-disk-system' in path:return 'fds'
    if console=='gbc' and ('/nintendo-game-boy-color/' in path or any(isinstance(name,str) and name.lower().endswith('.gbc') for name in files)):return 'gbc-color'
    return console

def image_url(console,item):
    system=artwork_system(console,item)
    if system not in SYSTEMS:return None
    title=item.get('title','')
    if not isinstance(title,str) or not 0<len(title)<=400:return None
    encoded=base64.urlsafe_b64encode(title.encode()).decode().rstrip('=')
    return '/experience-api/v1/covers/'+system+'/'+encoded

def search(folder,items,prefetch_images=True):
    """Build safe, lazy cover URLs for installed games from every known system."""
    if not isinstance(items,list) or len(items)>2000:raise ValueError('invalid cover search')
    result=[];seen=set()
    for item in items:
        if not isinstance(item,dict):raise ValueError('invalid cover item')
        console=item.get('consoleId');reference=item.get('reference');title=item.get('title')
        if not isinstance(console,str) or artwork_system(console,item) not in SYSTEMS:continue
        if not isinstance(reference,str) or not 0<len(reference)<=512 or re.search(r'[\x00-\x1f]',reference):continue
        if not isinstance(title,str) or not 0<len(title)<=400:continue
        key=(console,reference)
        if key in seen:continue
        seen.add(key);image=image_url(console,item)
        if image:result.append({'consoleId':console,'reference':reference,'image':image,'attribution':'Libretro Thumbnails · Ayants droit du jeu'})
    if prefetch_images:prefetch(folder,result)
    return {'items':result,'supportedSystems':sorted(SYSTEMS)}

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


def search(folder, items, prefetch_images=True):
    """Resolve safe lazy artwork URLs for installed references; no game downloads."""
    if not isinstance(items, list) or len(items) > 2000:
        raise ValueError('invalid cover search')
    result, seen = [], set()
    for item in items:
        if not isinstance(item, dict): raise ValueError('invalid cover item')
        console, reference, title = item.get('consoleId'), item.get('reference'), item.get('title')
        if not isinstance(console, str) or console not in SYSTEMS: continue
        if not isinstance(reference, str) or not 0 < len(reference) <= 512 or re.search(r'[\x00-\x1f]', reference): continue
        if not isinstance(title, str) or not 0 < len(title) <= 400: continue
        if (console, reference) in seen: continue
        seen.add((console, reference))
        cover_item = {**item, 'title': re.sub(r'\.(?:gba|gbc|gb|nes|sfc|smc|z64|n64|nds|iso|bin|chd|zip|7z)$', '', title, flags=re.I)}
        image = image_url(console, cover_item)
        if image: result.append({'consoleId': console, 'reference': reference, 'image': image,
                                 'attribution': 'Libretro Thumbnails · Ayants droit du jeu'})
    if prefetch_images: prefetch(folder, result)
    return {'items': result, 'supportedSystems': sorted(SYSTEMS)}
