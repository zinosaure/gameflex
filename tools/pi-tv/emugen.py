"""EmuGen public metadata. Chromium is imported only by the explicit sources action."""
import hashlib
from html import unescape
from html.parser import HTMLParser
import http.client
import json
from pathlib import Path
import re
import ssl
import threading
import time
import unicodedata
from urllib.parse import urlsplit,urljoin,urlencode,unquote
from romspure import PublicHTTPS,read_cache,save_cache

ORIGIN='https://www.emugen.net'
HOSTS={'www.emugen.net','emugen.net'}
SYSTEMS={'nes':('nes',),'snes':('snes',),'gbc':('gameboy','gbc'),'gba':('gba',),'n64':('n64',),'nds':('nds',),'gc':('game-cube',),'wii':('wii',),'ps1':('ps1',),'ps2':('ps2',),'psp':('psp',),'sms':('master-system',),'megadrive':('mega-drive',),'saturn':('sega-saturn',),'dreamcast':('dreamcast',),'atari2600':('atari-2600',),'atari7800':('atari-7800',),'jaguar':('jaguar',),'lynx':('lynx',),'neogeo':('neo-geo',),'ngp':('neo-geo-pocket-color',),'pce':('pc-engine',),'coleco':('colecovision',)}
FIELDS=('title','description','attribution','consoleId','consoleName','coverFront','coverBack','image','images','screenshots','trailers','releaseDate','genres','languages','version','region')
NETWORK=threading.BoundedSemaphore(2);RATE=threading.Lock();LAST=0.0
LOCKS=[threading.RLock() for _ in range(64)]
class EmugenError(ValueError):
    def __init__(self,status='error',message='EmuGen est momentanément inaccessible.'):
        super().__init__(message);self.status=status

def valid_url(url,kind='page'):
    try:u=urlsplit(url);port=u.port
    except (ValueError,TypeError):raise EmugenError('blocked_navigation','Adresse EmuGen refusée.')
    if len(url)>4096 or u.scheme!='https' or u.hostname not in HOSTS or port not in (None,443) or u.username or u.password or u.fragment or re.search(r'[\x00-\x20\x7f]',url) or '..' in unquote(u.path).split('/'):
        raise EmugenError('blocked_navigation','Adresse EmuGen refusée.')
    if kind=='game' and not re.fullmatch(r'/jeux/[a-z0-9-]{1,240}/?',u.path):raise EmugenError('blocked_navigation','Fiche EmuGen invalide.')
    if kind=='image' and not re.fullmatch(r'/wp-content/uploads/[A-Za-z0-9_./%-]+\.(?:png|jpe?g|webp)',u.path,re.I):raise EmugenError('blocked_navigation','Image EmuGen refusée.')
    if kind=='page' and not (re.fullmatch(r'/jeux/[a-z0-9-]{1,240}/?',u.path) or re.fullmatch(r'/wp-json/wp/v2/(?:jeux(?:/\d+)?|console)',u.path)):
        raise EmugenError('blocked_navigation','Route EmuGen refusée.')
    return u

def game_url(slug):
    if not isinstance(slug,str) or not re.fullmatch('[a-z0-9-]{1,240}',slug):raise EmugenError('not_found','Fiche EmuGen introuvable.')
    return ORIGIN+'/jeux/'+slug+'/'

def fetch(url,kind='page',redirects=0):
    global LAST
    u=valid_url(url,kind)
    with NETWORK:
        with RATE:start=max(time.monotonic(),LAST+(.15 if kind=='image' else .35));LAST=start
        time.sleep(max(0,start-time.monotonic()))
        conn=PublicHTTPS(u.hostname,timeout=12,context=ssl.create_default_context())
        try:
            conn.request('GET',u.path+('?' +u.query if u.query else ''),headers={'User-Agent':'GameFlex-Metadata/1.0 (personal library)','Accept':'image/*' if kind=='image' else 'application/json,text/html','Accept-Encoding':'identity'})
            r=conn.getresponse()
            if r.status in (301,302,303,307,308):
                target=urljoin(url,r.getheader('Location',''));valid_url(target,kind)
                if redirects>=3:raise EmugenError('blocked_navigation','Trop de redirections EmuGen.')
            else:
                target=None
                if r.status==404:raise EmugenError('not_found','Fiche EmuGen introuvable.')
                if r.status==400 and u.path=='/wp-json/wp/v2/jeux':
                    try:payload=json.loads(r.read(65537))
                    except (ValueError,UnicodeError):payload={}
                    if isinstance(payload,dict) and payload.get('code')=='rest_post_invalid_page_number':raise EmugenError('end_of_pages','Fin du catalogue EmuGen.')
                if r.status!=200:raise EmugenError()
                mime=r.getheader('Content-Type','').split(';')[0].strip().lower()
                allowed=('image/png','image/jpeg','image/webp') if kind=='image' else ('text/html','application/json')
                if mime not in allowed:raise EmugenError()
                limit=4*1024*1024 if kind=='image' else 3*1024*1024
                size=r.getheader('Content-Length')
                if size and size.isdigit() and int(size)>limit:raise EmugenError('error','Réponse EmuGen trop volumineuse.')
                body=r.read(limit+1)
                if len(body)>limit:raise EmugenError('error','Réponse EmuGen trop volumineuse.')
                return body,mime
        except (OSError,http.client.HTTPException) as e:raise EmugenError() from e
        finally:conn.close()
    return fetch(target,kind,redirects+1)

class Node:
    def __init__(self,tag='',attrs=None):self.tag=tag;self.attrs=attrs or {};self.children=[]
    def all(self,predicate):
        for item in self.children:
            if isinstance(item,Node):
                if predicate(item):yield item
                yield from item.all(predicate)
    def has(self,word):return word in self.attrs.get('class','').split()
    def text(self):
        if self.tag in ('script','style'):return ''
        return ' '.join(' '.join(x.text() if isinstance(x,Node) else x for x in self.children).split())
class DOM(HTMLParser):
    VOID={'img','input','br','hr','meta','link','source','area','base','embed','param','track','wbr'}
    def __init__(self):super().__init__();self.root=Node();self.stack=[self.root]
    def handle_starttag(self,t,a):
        n=Node(t,dict(a));self.stack[-1].children.append(n)
        if t not in self.VOID:self.stack.append(n)
    def handle_startendtag(self,t,a):self.handle_starttag(t,a);self.handle_endtag(t)
    def handle_endtag(self,t):
        for i in range(len(self.stack)-1,0,-1):
            if self.stack[i].tag==t:self.stack=self.stack[:i];break
    def handle_data(self,d):
        if self.stack[-1].tag not in ('script','style'):self.stack[-1].children.append(d)

def plain(value):
    d=DOM();d.feed(str(value));return unescape(d.root.text())
def compact(value):
    value=re.sub(r'\.(?:nes|sfc|smc|gba|gbc?|nds|iso|chd|zip|7z)$','',str(value),flags=re.I)
    value=re.sub(r'\([^)]*\)|\[[^]]*\]','',value)
    return ''.join(c for c in unicodedata.normalize('NFKD',value).casefold() if c.isalnum() and not unicodedata.combining(c))
def present(v):return v is not None and v!='' and v!=[] and v!={}
def merge_metadata(base,emugen=None,secondary=(),manual=None):
    result=dict(base);sources=dict(base.get('fieldSources',{}));manual={**base.get('overrides',{}),**base.get('manualMetadata',{}),**(manual or {})}
    if emugen and emugen.get('coverFront') and not emugen.get('image'):emugen={**emugen,'image':emugen['coverFront']}
    for field in base.get('manualFields',[]):
        if field in base:manual.setdefault(field,base[field])
    for field,source in base.get('fieldSources',{}).items():
        if source=='manual' and field in base:manual.setdefault(field,base[field])
    if 'coverFront' in manual and 'image' not in manual:manual['image']=manual['coverFront']
    if emugen and emugen.get('consoleId')!=base.get('consoleId'):emugen=None
    for field in FIELDS:
        if field=='consoleId':continue
        if field in manual:result[field]=manual[field];sources[field]='manual';continue
        if emugen and present(emugen.get(field)):result[field]=emugen[field];sources[field]='EmuGen';continue
        if present(base.get(field)):continue
        for record in secondary:
            if record.get('consoleId',base.get('consoleId'))!=base.get('consoleId'):continue
            if present(record.get(field)):result[field]=record[field];sources[field]=record.get('provider','Catalogue');break
    if emugen:result.update(emugen=emugen,emugenSlug=emugen.get('emugenSlug'),emugenUrl=emugen.get('emugenUrl'))
    result['fieldSources']=sources;return result

def art_url(url):
    valid_url(url,'image')
    import base64
    return '/experience-api/v1/emugen-art/'+base64.urlsafe_b64encode(url.encode()).decode().rstrip('=')
def cached_art_url(value):
    try:return art_url(value)
    except (ValueError,TypeError):return None

def parse_game(body,url,expected_console=None):
    valid_url(url,'game');dom=DOM();dom.feed(body.decode('utf8','replace'));root=dom.root
    h1=next(root.all(lambda n:n.tag=='h1' and n.has('post_title')),None) or next(root.all(lambda n:n.tag=='h1'),None)
    if not h1 or not h1.text():raise EmugenError('error','Fiche EmuGen illisible.')
    taxonomy=next(root.all(lambda n:n.has('post_taxonomy') and n.has('style_badge')),None)
    console_link=next(taxonomy.all(lambda n:n.tag=='a' and '/console/' in n.attrs.get('href','')),None) if taxonomy else None
    console_slug=urlsplit(console_link.attrs['href']).path.strip('/').split('/')[-1] if console_link else ''
    console=next((key for key,values in SYSTEMS.items() if console_slug in values),None)
    if not console or expected_console and console!=expected_console:raise EmugenError('mismatch','La fiche EmuGen concerne une autre console.')
    record={'provider':'EmuGen','source':url,'emugenSlug':urlsplit(url).path.strip('/').split('/')[-1],'emugenUrl':url,'title':h1.text(),'consoleId':console,'consoleName':console_link.text(),'images':[],'screenshots':[],'trailers':[]}
    fields={'jeux_description':'description','jeux_sortie_france':'releaseDate','jeux_genres':'genres','jeux_langage_texte':'languages','jeux_version_du_jeu':'version','jeux_region':'region'}
    flags={'🇫🇷':'fr','🇬🇧':'en','🇺🇸':'en','🇩🇪':'de','🇪🇸':'es','🇮🇹':'it','🇯🇵':'ja','🇵🇹':'pt','🇳🇱':'nl'}
    for node in root.all(lambda n:any(n.has(key) for key in fields)):
        name=next(fields[key] for key in fields if node.has(key));v=next(node.all(lambda n:n.has('w-post-elm-value')),None);value=(v or node).text()
        if name=='genres':value=[x.strip() for x in re.split('[,;]',value) if x.strip()]
        if name=='languages':value=list(dict.fromkeys(code for flag,code in flags.items() if flag in value)) or [x.strip() for x in re.split('[,;]',value) if x.strip()]
        if name=='releaseDate' and re.fullmatch(r'\d{2}/\d{2}/\d{4}',value):value='-'.join(reversed(value.split('/')))
        if present(value) and name not in record:record[name]=value
    meta={n.attrs.get('property'):n.attrs.get('content') for n in root.all(lambda n:n.tag=='meta')}
    cover=cached_art_url(meta.get('og:image'))
    if cover:record.update(coverFront=cover,image=cover,attribution='EmuGen · Ayants droit du jeu')
    # Only assets belonging to this title or explicit game-media custom fields.
    own=compact(record['title'])
    asset_root=next(root.all(lambda n:n.tag=='main'),root)
    explicit={}
    for container in asset_root.all(lambda n:any(n.has(k) for k in ('jeux_screenshots','jeux_captures','jeux_galerie','jeux_images','jeux_jaquette_arriere'))):
        role='screenshots' if container.has('jeux_screenshots') or container.has('jeux_captures') else 'images'
        for asset in container.all(lambda n:n.tag in ('img','a')):explicit[id(asset)]=role
    for n in asset_root.all(lambda n:n.tag in ('a','img','iframe','video','source')):
        raw=n.attrs.get('href') if n.tag=='a' else n.attrs.get('data-src') or n.attrs.get('data-lazy-src') or n.attrs.get('src')
        if not raw:continue
        candidate=urljoin(url,raw);label=(n.attrs.get('alt','')+' '+n.attrs.get('title','')+' '+candidate).lower()
        if n.tag=='iframe' and urlsplit(candidate).hostname in ('www.youtube.com','www.youtube-nocookie.com','player.vimeo.com'):
            record['trailers'].append(candidate);continue
        if id(n) not in explicit and own not in compact(urlsplit(candidate).path):continue
        image=cached_art_url(candidate)
        if not image:continue
        if re.search(r'arri[eè]re|back[-_. ]?cover|cover[-_. ]?back',label):record.setdefault('coverBack',image)
        elif explicit.get(id(n))=='screenshots' or re.search(r'screenshot|capture|gameplay',label):record['screenshots'].append(image)
        elif image!=cover:record['images'].append(image)
    for field in ('images','screenshots','trailers'):record[field]=list(dict.fromkeys(record[field]))[:16]
    return record

def folder(data):
    p=Path(data)/'emugen';p.mkdir(parents=True,exist_ok=True);return p

def detail(data,slug,console,refresh=False):
    if console not in SYSTEMS:raise EmugenError('mismatch','Console EmuGen non prise en charge.')
    url=game_url(slug);key=hashlib.sha256((console+':'+slug).encode()).hexdigest();path=folder(data)/('game-'+key+'.json')
    with LOCKS[int(key[:8],16)%64]:
        ready=read_cache(path,7*86400)
        if ready and not refresh:return ready
        record=parse_game(fetch(url)[0],url,console);save_cache(path,record);return record

def rest(data,path,params):
    key=hashlib.sha256((path+urlencode(params)).encode()).hexdigest();cache=folder(data)/('rest-'+key+'.json')
    ready=read_cache(cache,86400)
    if ready is not None:return ready
    body,mime=fetch(ORIGIN+'/wp-json/wp/v2/'+path+'?'+urlencode(params))
    if mime!='application/json':raise EmugenError()
    try:result=json.loads(body)
    except ValueError:raise EmugenError()
    if not isinstance(result,list) or any(not isinstance(item,dict) for item in result):raise EmugenError()
    save_cache(cache,result);return result

def rest_game(item,console):
    groups=item.get('_embedded',{}).get('wp:term',[])
    if not isinstance(groups,list) or any(not isinstance(group,list) for group in groups):raise EmugenError('error','Console EmuGen non vérifiable.')
    terms=[t for group in groups for t in group if isinstance(t,dict) and t.get('taxonomy')=='console']
    if not any(t.get('slug') in SYSTEMS[console] for t in terms):return None
    url=item.get('link','');valid_url(url,'game');slug=urlsplit(url).path.strip('/').split('/')[-1]
    title=plain(item.get('title',{}).get('rendered',''))
    if not title:return None
    result={'provider':'EmuGen','source':url,'title':title,'consoleId':console,'consoleName':next(t['name'] for t in terms if t.get('slug') in SYSTEMS[console]),'emugenSlug':slug,'emugenUrl':url}
    yoast=item.get('yoast_head_json',{});description=yoast.get('description')
    if description:result['description']=plain(description)
    media=item.get('_embedded',{}).get('wp:featuredmedia',[])
    image=media[0].get('source_url') if media and isinstance(media[0],dict) else None
    if not image:image=next(iter(yoast.get('og_image',[])),{}).get('url')
    cover=cached_art_url(image)
    if cover:result.update(image=cover,coverFront=cover,attribution='EmuGen · Ayants droit du jeu')
    return result

def search(data,console,query='',letter='A',region='',edition='',page=None):
    if console not in SYSTEMS:return {'items':[],'unsupported':True}
    if not isinstance(query,str) or len(query)>160 or re.search(r'[\x00-\x1f]',query) or not isinstance(letter,str) or not re.fullmatch('[A-Z0]?',letter):raise EmugenError('invalid','Recherche invalide.')
    paginated=page is not None
    if paginated:
        if not re.fullmatch('[1-9][0-9]{0,3}',str(page)) or int(page)>1000:raise EmugenError('invalid','Page invalide.')
        requested_page=int(page)
    ids=[]
    for category in SYSTEMS[console]:
        ids.extend(term['id'] for term in rest(data,'console',{'slug':category,'per_page':10}) if term.get('slug')==category)
    if not ids:return {'items':[]}
    results=[];truncated=False;next_page=None
    for page in ([requested_page] if paginated else range(1,3 if query else 4)):
        params={'console':','.join(map(str,ids)),'per_page':30 if query else 100,'page':page,'_embed':'wp:featuredmedia,wp:term','orderby':'title','order':'asc'}
        if query:params['search']=query
        try:items=rest(data,'jeux',params)
        except EmugenError as error:
            if error.status=='end_of_pages' and page>1:truncated=False;break
            if not paginated and page>1 and results:truncated=True;break
            raise
        for item in items:
            record=rest_game(item,console)
            if record:
                initial=re.search('[A-Z0-9]',unicodedata.normalize('NFKD',record['title']).upper())
                initial=initial[0] if initial else '';initial='0' if initial.isdigit() else initial
                if query or not letter or initial==letter:results.append(record)
        if paginated:next_page=page+1 if len(items)==params['per_page'] else None
        if len(items)<params['per_page']:truncated=False;break
        truncated=page==(2 if query else 3)
    # Exact matches may be auto-bound; ambiguous or underspecified editions require a choice.
    target=compact(query);ranked=[]
    for record in results:
        exact=bool(target) and compact(record['title'])==target
        compatible=not region and not edition
        record['match']={'exact':exact,'consoleVerified':True,'needsChoice':not exact or not compatible}
        ranked.append(record)
    return {'items':ranked,'provider':'EmuGen','truncated':truncated,**({'nextPage':next_page,'page':requested_page} if paginated else {})}

def identify(data,base,refresh=False):
    console=base['consoleId'];slug=base.get('emugenSlug') or (base.get('emugen') or {}).get('emugenSlug')
    if slug:return {'status':'matched','item':detail(data,slug,console,refresh)}
    title=base.get('title','');tags=re.findall(r'\(([^)]+)\)|\[([^]]+)\]',title)
    qualifiers=[a or b for a,b in tags]
    region=base.get('region') or next((tag for tag in qualifiers if re.fullmatch(r'(?:USA|Europe|France|Japan|World|Australia|Germany|Spain|Italy|Korea|China)',tag,re.I)), '')
    edition=base.get('version') or next((tag for tag in qualifiers if re.match(r'^(?:beta|proto(?:type)?|demo|rev(?:ision)?\s*\d|v\d)',tag,re.I)), '')
    result=search(data,console,title,region=region,edition=edition)
    exact=[r for r in result['items'] if r['match']['exact']]
    if len(exact)==1:
        record=detail(data,exact[0]['emugenSlug'],console,refresh)
        if region and compact(region)!=compact(record.get('region','')) or edition and compact(edition)!=compact(record.get('version','')):
            return {'status':'choice','candidates':result['items']}
        return {'status':'matched','item':record}
    return {'status':'choice' if result['items'] else 'not_found','candidates':result['items']}

def artwork(data,encoded):
    import base64
    if not re.fullmatch('[A-Za-z0-9_-]{1,2500}',encoded):raise EmugenError('invalid')
    try:url=base64.urlsafe_b64decode(encoded+'='*((-len(encoded))%4)).decode()
    except (ValueError,UnicodeError):raise EmugenError('invalid')
    valid_url(url,'image');key=hashlib.sha256(url.encode()).hexdigest();cache=folder(data)/('art-'+key+'.json')
    with LOCKS[int(key[:8],16)%64]:
        ready=read_cache(cache,7*86400)
        if ready and (Path(data)/'media'/(ready['id']+'.img')).is_file():return ready
        from covers import optimize_image,WRITE_LOCK
        from artwork import cache_bytes
        body,mime=fetch(url,'image');image=optimize_image(body,mime)
        with WRITE_LOCK:record=cache_bytes(Path(data)/'media',image,'image/webp',{'provider':'EmuGen','author':'Ayants droit du jeu','title':'Visuel du jeu','source':url,'sourceImage':url,'license':'Illustration protégée · consultation personnelle','licenseUrl':ORIGIN+'/','approved':True,'kind':'game-cover','modifications':'Miniature WebP sans recadrage.'})
        save_cache(cache,record);return record
