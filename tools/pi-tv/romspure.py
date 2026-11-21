"""ROMsPure public metadata and user-requested file transport.
No JavaScript, advertising, CAPTCHA solving, arbitrary client URLs or ROM prefetch.
"""
import base64
from collections import OrderedDict
import hashlib
from html.parser import HTMLParser
import http.client
import ipaddress
import json
from pathlib import Path
import re
import socket
import ssl
import threading
import time
import unicodedata
from urllib.parse import urlsplit,urljoin,urlencode,unquote
from library import atomic_json
from freeroms import ProviderError,MAX_FILE

ORIGIN='https://romspure.cc'
# Categories observed in the provider's /roms/ directory, 2026-11-08.
SYSTEMS={'nes':('nintendo-entertainment-system',),'snes':('super-nintendo-entertainment-system',),
 'gbc':('nintendo-game-boy','nintendo-game-boy-color'),'gba':('nintendo-game-boy-advance',),
 'n64':('nintendo-64',),'nds':('nintendo-ds',),'gc':('nintendo-gamecube',),'wii':('nintendo-wii',),
 'ps1':('sony-playstation',),'ps2':('sony-playstation-2',),'psp':('sony-psp',),
 'megadrive':('sega-genesis',),'saturn':('sega-saturn',),'dreamcast':('sega-dreamcast',),
 'neogeo':('snk-neo-geo-aes',)}
EXTENSIONS={'nes':{'nes','unf','fds'},'snes':{'sfc','smc'},'gbc':{'gb','gbc'},'gba':{'gba'},
 'n64':{'n64','z64','v64'},'nds':{'nds'},'gc':{'iso','rvz'},'wii':{'iso','wbfs','rvz'},
 'ps1':{'chd','iso','pbp'},'ps2':{'chd','iso'},'psp':{'iso','cso','pbp'},
 'megadrive':{'md','gen','bin'},'saturn':{'chd','iso'},'dreamcast':{'chd','cdi'},'neogeo':{'zip'}}
NETWORK=threading.BoundedSemaphore(2)
RATE=threading.Lock();LAST=0.0
LOCKS=[threading.RLock() for _ in range(64)]
FAILURES=OrderedDict();FAIL_LOCK=threading.Lock()
CACHE_LOCK=threading.Lock();CACHE_WRITES=0

def fail_cache(key):
    with FAIL_LOCK:
        FAILURES[key]=time.monotonic()+60
        while len(FAILURES)>256:FAILURES.popitem(last=False)

def folded(text):
    return ''.join(c for c in unicodedata.normalize('NFKD',text).casefold() if not unicodedata.combining(c))
def letter_of(title):
    first=re.search('[a-z0-9]',folded(title))
    return first[0].upper() if first and not first[0].isdigit() else '0'
def token_for(category,slug):return category+'~'+slug

def decode(identifier):
    m=re.fullmatch(r'romspure:([a-z0-9_-]+):([a-z0-9-]{1,80})~([a-z0-9-]{1,220})',identifier or '')
    if not m or m[1] not in SYSTEMS or m[2] not in SYSTEMS[m[1]] or re.search(r'(?:^|-)(?:bios|firmware|boot-rom)(?:-|$)',m[3]):raise ProviderError('Identifiant du catalogue invalide.')
    return m[1],m[2],m[3]

def valid_url(url,kind):
    try:u=urlsplit(url);port=u.port
    except ValueError:raise ProviderError('Adresse de transfert invalide.')
    if len(url)>8192 or re.search(r'[\x00-\x20\x7f]',url) or u.scheme!='https' or not u.hostname or u.username or u.password or port not in (None,443) or u.fragment:raise ProviderError('Adresse de transfert refusée.')
    if kind!='file' and u.hostname!='romspure.cc':raise ProviderError('Source de métadonnées refusée.')
    if kind=='image' and '..' in unquote(u.path).split('/'):raise ProviderError('Chemin image refusé.')
    if kind=='image' and not re.fullmatch(r'/wp-content/uploads/[A-Za-z0-9_./%-]+\.(?:png|jpe?g|webp)',u.path,re.I):raise ProviderError('Image extérieure refusée.')
    if kind in ('html','json') and not (u.path=='/wp-admin/admin-ajax.php' or re.fullmatch(r'/roms/[a-z0-9-]+(?:/[a-z0-9-]+|/page/[0-9]+)?/?',u.path) or re.fullmatch(r'/download/[a-z0-9-]+(?:/[0-9]+)?/?',u.path)):raise ProviderError('Page de métadonnées refusée.')
    return u

class PublicHTTPS(http.client.HTTPSConnection):
    def connect(self):
        # Pin the validated DNS result to the TCP socket; preserve hostname TLS verification.
        records=socket.getaddrinfo(self.host,self.port,type=socket.SOCK_STREAM)
        if not records or any(not ipaddress.ip_address(row[4][0]).is_global or ipaddress.ip_address(row[4][0]).is_multicast or ipaddress.ip_address(row[4][0]).is_reserved for row in records):raise ProviderError('Destination réseau refusée.')
        error=None
        for family,kind,proto,_,address in records:
            sock=socket.socket(family,kind,proto);sock.settimeout(self.timeout)
            try:
                sock.connect(address);self.sock=self._context.wrap_socket(sock,server_hostname=self.host);return
            except OSError as e:error=e;sock.close()
        raise error or ProviderError('Connexion impossible.')

def fetch(url,kind='html',limit=2*1024*1024,progress=None,form=None,redirects=0):
    global LAST
    u=valid_url(url,kind)
    with NETWORK:
        with RATE:
            start=max(time.monotonic(),LAST+(.25 if kind=='image' else 1));LAST=start
        time.sleep(max(0,start-time.monotonic()))
        conn=PublicHTTPS(u.hostname,timeout=15,context=ssl.create_default_context())
        try:
            headers={'User-Agent':'GameFlex-Metadata/1.0 (personal library)','Accept':'image/*' if kind=='image' else 'application/json' if kind=='json' else 'application/octet-stream' if kind=='file' else 'text/html','Accept-Encoding':'identity'}
            body=urlencode(form).encode() if form else None
            if form:headers['Content-Type']='application/x-www-form-urlencoded';headers['Origin']=ORIGIN;headers['Referer']=ORIGIN+'/'
            conn.request('POST' if form else 'GET',u.path+('?' +u.query if u.query else ''),body,headers)
            response=conn.getresponse()
            if response.status in (301,302,303,307,308):
                target=urljoin(url,response.getheader('Location',''))
                valid_url(target,kind)
                if redirects>=3 or form:raise ProviderError('Redirection non prise en charge.')
                # Release the connection and semaphore before a bounded, revalidated redirect.
                redirect=target
            else:
                redirect=None
                if response.status!=200:raise ProviderError('Accès au fournisseur indisponible (HTTP '+str(response.status)+').')
                mime=response.getheader('Content-Type','').split(';')[0].strip().lower()
                if kind=='html' and mime!='text/html' or kind=='json' and mime!='application/json' or kind=='image' and mime not in ('image/png','image/jpeg','image/webp'):raise ProviderError('Réponse du fournisseur incompatible.')
                if kind=='file' and (mime.startswith(('text/','image/')) or mime in ('application/json','application/javascript','application/x-msdownload','application/x-executable')):raise ProviderError('La source a renvoyé une page ou un fichier incompatible.')
                length=response.getheader('Content-Length');total=int(length) if length and length.isdigit() else None
                if total is not None and total>limit:raise ProviderError('Fichier trop volumineux (64 Mio maximum).')
                chunks=[];received=0
                if progress:progress(0,total)
                while True:
                    chunk=response.read(min(65536,limit+1-received))
                    if not chunk:break
                    chunks.append(chunk);received+=len(chunk)
                    if received>limit:raise ProviderError('Réponse trop volumineuse.')
                    if progress:progress(received,total)
                if total is not None and received!=total:raise ProviderError('Transfert incomplet.')
                return b''.join(chunks),mime
        finally:conn.close()
    return fetch(redirect,kind,limit,progress=progress,redirects=redirects+1)

class Page(HTMLParser):
    def __init__(self):super().__init__();self.links=[];self.current=None;self.depth=0;self.title_depth=0;self.meta_image='';self.widget=None
    def handle_starttag(self,tag,attrs):
        a=dict(attrs)
        if tag=='a':self.current={'href':a.get('href',''),'title':'','text':'','image':'','rel':a.get('rel','')};self.depth=1
        elif self.current:
            if tag not in ('img','br','input','path','meta','link','source','hr'):self.depth+=1
            if tag=='h3':self.title_depth=self.depth
            if tag=='img':self.current['image']=a.get('data-src') or a.get('src','')
        if tag=='meta' and a.get('property')=='og:image':self.meta_image=a.get('content','')
        if 'data-download-ajax' in a:self.widget={k:a.get('data-'+k,'') for k in ('post-id','index')}
    def handle_data(self,text):
        if self.current:
            self.current['text']+=text
            if self.title_depth:self.current['title']+=text
    def handle_endtag(self,tag):
        if not self.current:return
        if tag=='a':
            for key in ('title','text'):self.current[key]=' '.join(self.current[key].split())
            self.links.append(self.current);self.current=None;self.depth=0;self.title_depth=0
        else:
            if tag=='h3':self.title_depth=0
            self.depth=max(1,self.depth-1)

def parse(body):
    page=Page();page.feed(body.decode('utf8','replace'))
    return page

def cache_folder(data):
    folder=Path(data)/'romspure';folder.mkdir(parents=True,exist_ok=True);return folder

def save_cache(path,data):
    global CACHE_WRITES
    atomic_json(path,data)
    with CACHE_LOCK:
        CACHE_WRITES+=1
        if CACHE_WRITES%32:return
        files=list(path.parent.glob('*.json'))
        if len(files)>1500:
            for old in sorted(files,key=lambda p:p.stat().st_mtime)[:len(files)-1200]:old.unlink(missing_ok=True)

def read_cache(path,ttl):
    try:
        if time.time()-path.stat().st_mtime<ttl:return json.loads(path.read_text())
    except (OSError,ValueError):pass
    return None

def page_data(data,console,category,number=1,query=''):
    key=hashlib.sha256((category+':'+str(number)+':'+query).encode()).hexdigest()
    folder=cache_folder(data);path=folder/('page-'+key+'.json')
    ready=read_cache(path,86400)
    if ready:return ready
    url=ORIGIN+'/roms/'+category+('/page/'+str(number) if number>1 else '')+'/?'+urlencode({'sort':'alphabet',**({'q':query} if query else {})})
    body=fetch(url)[0]
    if b'error404' in body or b'<title>Just a moment' in body:raise ProviderError('Catalogue momentanément inaccessible.')
    page=parse(body);items=[];seen=set();last=number
    for link in page.links:
        u=urlsplit(urljoin(ORIGIN,link['href']))
        if u.hostname!='romspure.cc':continue
        match=re.fullmatch('/roms/'+re.escape(category)+r'/([a-z0-9-]{1,220})/?',u.path)
        pagination=re.fullmatch('/roms/'+re.escape(category)+r'/page/(\d+)/?',u.path)
        if pagination:last=max(last,min(2000,int(pagination[1])))
        if not match or not link['title'] or match[1] in seen or re.search(r'\b(?:bios|firmware|boot rom)\b',link['title'],re.I):continue
        slug=match[1];token=token_for(category,slug)
        try:decode('romspure:'+console+':'+token)
        except ValueError:continue
        seen.add(slug);image=urljoin(ORIGIN,link['image']) if link['image'] else ''
        try:valid_url(image,'image')
        except ValueError:image=''
        from covers import image_url
        cover='/experience-api/v1/romspure-art/'+console+'/'+token if image else image_url(console,link) or ''
        item={'title':link['title'][:400],'consoleId':console,'path':u.path,'source':ORIGIN+u.path,'provider':'ROMsPure','downloadId':'romspure:'+console+':'+token,'image':cover,'attribution':'Ayants droit du jeu · ROMsPure'}
        fallback=image_url(console,item)
        if image and fallback:item.update(fallbackImage=fallback,fallbackAttribution='Libretro Thumbnails · Ayants droit du jeu')
        if image:save_cache(folder/('image-'+hashlib.sha256((console+':'+token).encode()).hexdigest()+'.json'),{'source':item['source'],'image':image})
        items.append(item)
    result={'items':items[:100],'pages':last}
    save_cache(path,result);return result

def catalogue(data,console,letter,query=''):
    if console not in SYSTEMS:return {'items':[],'provider':'ROMsPure','unsupported':True}
    if not re.fullmatch('[A-Z0]',letter) or not isinstance(query,str) or len(query)>100 or re.search(r'[\x00-\x1f]',query):raise ProviderError('Recherche invalide.')
    query=' '.join(query.split());folder=cache_folder(data)
    scope=console+(':query:'+query if query else ':letter:'+letter);key=hashlib.sha256(scope.encode()).hexdigest();path=folder/('catalogue-'+key+'.json')
    ready=read_cache(path,86400)
    if ready:return {**ready,'items':[item for item in ready['items'] if letter_of(item['title'])==letter]}
    with FAIL_LOCK:failed=FAILURES.get(scope,0)>time.monotonic()
    if failed:
        stale=read_cache(path,7*86400)
        return {**stale,'items':[item for item in stale['items'] if letter_of(item['title'])==letter],'stale':True} if stale else {'items':[],'error':'Une source est momentanément inaccessible.'}
    with LOCKS[int(key[:8],16)%64]:
        ready=read_cache(path,86400)
        if ready:return {**ready,'items':[item for item in ready['items'] if letter_of(item['title'])==letter]}
        items=[];truncated=False
        try:
            for category in SYSTEMS[console]:
                first=page_data(data,console,category,query=query);pages=first['pages']
                if query:
                    for number in range(1,min(pages,5)+1):items.extend((first if number==1 else page_data(data,console,category,number,query))['items'])
                    truncated|=pages>5
                else:
                    # Alphabetical lower-bound search avoids crawling every provider page per letter.
                    low,high=1,pages
                    while low<high:
                        mid=(low+high)//2;part=page_data(data,console,category,mid)
                        if not part['items'] or letter_of(part['items'][-1]['title'])>=letter:high=mid
                        else:low=mid+1
                    for number in range(low,min(pages,low+9)+1):
                        part=first if number==1 else page_data(data,console,category,number)
                        items.extend(item for item in part['items'] if letter_of(item['title'])==letter)
                        if not part['items'] or letter_of(part['items'][-1]['title'])>letter:break
                    else:truncated=number<pages
            result={'items':list({item['downloadId']:item for item in items}.values()),'provider':'ROMsPure','truncated':truncated}
            save_cache(path,result)
            return {**result,'items':[item for item in result['items'] if letter_of(item['title'])==letter]}
        except (OSError,ValueError,http.client.HTTPException):
            fail_cache(scope);stale=read_cache(path,7*86400)
            return {**stale,'items':[item for item in stale['items'] if letter_of(item['title'])==letter],'stale':True} if stale else {'items':[],'error':'Une source est momentanément inaccessible.'}

def detail(data,identifier):
    console,category,slug=decode(identifier);key=hashlib.sha256(identifier.encode()).hexdigest();path=cache_folder(data)/('detail-'+key+'.json')
    ready=read_cache(path,86400)
    if ready:return ready
    source=ORIGIN+'/roms/'+category+'/'+slug+'/'
    page=parse(fetch(source)[0]);selection=[]
    for link in page.links:
        url=urljoin(source,link['href']);u=urlsplit(url)
        if u.hostname=='romspure.cc' and not u.query and re.fullmatch(r'/download/'+re.escape(slug)+r'-\d+(?:/\d+)?/?',u.path):selection.append(url)
    result={'source':source,'selection':list(dict.fromkeys(selection))[:8],'image':page.meta_image}
    save_cache(path,result);return result

def artwork(data,console,token):
    identifier='romspure:'+console+':'+token;decode(identifier)
    key=hashlib.sha256(identifier.encode()).hexdigest();folder=cache_folder(data);mapping=folder/('art-'+key+'.json')
    with FAIL_LOCK:failed=FAILURES.get('art:'+key,0)>time.monotonic()
    if failed:return None
    with LOCKS[int(key[:8],16)%64]:
        ready=read_cache(mapping,7*86400)
        if ready and (Path(data)/'media'/(ready['id']+'.img')).is_file():return ready
        image_mapping=folder/('image-'+hashlib.sha256((console+':'+token).encode()).hexdigest()+'.json')
        info=read_cache(image_mapping,7*86400) or detail(data,identifier)
        image=info.get('image','')
        if not image:fail_cache('art:'+key);return None
        try:body,mime=fetch(image,'image',4*1024*1024)
        except (ValueError,OSError,http.client.HTTPException):fail_cache('art:'+key);raise
        from covers import optimize_image,WRITE_LOCK
        from artwork import cache_bytes
        thumbnail=optimize_image(body,mime)
        with WRITE_LOCK:
            record=cache_bytes(Path(data)/'media',thumbnail,'image/webp',{'provider':'ROMsPure','author':'Ayants droit du jeu','title':token.split('~',1)[1],'source':info['source'],'sourceImage':image,'license':'Illustration protégée · consultation personnelle','licenseUrl':ORIGIN+'/terms-of-use/','approved':True,'kind':'game-cover','target':identifier,'modifications':'Miniature WebP sans recadrage.'})
        save_cache(mapping,record);return record

def resolved_files(data,identifier):
    page=detail(data,identifier);choices=[]
    for source in page['selection']:
        document=parse(fetch(source)[0])
        if document.widget:choices.append(document.widget)
        for link in document.links:
            url=urljoin(ORIGIN,link['href']);u=urlsplit(url)
            m=re.fullmatch(r'/download/[a-z0-9-]+-(\d+)/(\d+)/?',u.path)
            if u.hostname=='romspure.cc' and not u.query and m:choices.append({'post-id':m[1],'index':m[2]})
    unique=list({(item['post-id'],item['index']):item for item in choices if re.fullmatch(r'\d{1,12}',item.get('post-id','')) and re.fullmatch(r'\d{1,4}',item.get('index',''))}.values())[:4]
    if not unique:raise ProviderError('Aucun lien compatible trouvé. Aucun jeu n’a été installé.')
    # Same public AJAX protocol as the site's download.js; no ad scripts or challenge bypass.
    started=time.monotonic();time.sleep(3)
    nonce=json.loads(fetch(ORIGIN+'/wp-admin/admin-ajax.php','json',65536,form={'action':'romspure_get_nonce'})[0])
    value=nonce.get('data',{}).get('nonce') if isinstance(nonce,dict) and nonce.get('success') and isinstance(nonce.get('data'),dict) else None
    if not isinstance(value,str) or not re.fullmatch('[A-Za-z0-9_-]{1,100}',value):raise ProviderError('Le fournisseur refuse temporairement la préparation du lien.')
    results=[]
    for choice in unique:
        response=json.loads(fetch(ORIGIN+'/wp-admin/admin-ajax.php','json',65536,form={'action':'app_get_download_link','post_id':choice['post-id'],'index':choice['index'],'nonce':value})[0])
        obj=response.get('data',{}) if isinstance(response,dict) and response.get('success') else {}
        if isinstance(obj,dict) and isinstance(obj.get('url'),str):
            valid_url(obj['url'],'file');name=obj.get('name','')
            if isinstance(name,str) and 0<len(name)<=220 and not re.search(r'[/\\\x00-\x1f]',name):results.append({'url':obj['url'],'name':name})
    time.sleep(max(0,7-(time.monotonic()-started)))
    if not results:raise ProviderError('Le fournisseur n’a pas fourni de fichier compatible. Aucun jeu n’a été installé.')
    return results

def payload(data,identifier,progress=None):
    console,category,slug=decode(identifier)
    candidates=resolved_files(data,identifier)
    from freeroms import unpack_payload
    last=None
    for candidate in candidates:
        try:
            suffix=Path(candidate['name']).suffix.lower().lstrip('.')
            if suffix not in EXTENSIONS[console]|{'zip'}:raise ProviderError('Format du fichier incompatible avec cette console.')
            if progress:progress(phase='checking',progress=0,bytesReceived=0,totalBytes=None,message='Préparation d’un accès compatible…')
            body,mime=fetch(candidate['url'],'file',MAX_FILE,progress=(lambda received,total:progress(phase='downloading',bytesReceived=received,totalBytes=total,progress=min(99,int(received*100/total)) if total else None,message='Téléchargement du fichier…')) if progress else None)
            return unpack_payload(console,slug,body,candidate['name'],progress,EXTENSIONS[console])
        except (ValueError,OSError,http.client.HTTPException) as error:last=error
    raise ProviderError(str(last) if isinstance(last,ProviderError) else 'Transfert impossible. Aucun succès confirmé.')
