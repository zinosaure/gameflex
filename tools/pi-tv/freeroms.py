"""Bounded public FreeROMs metadata; user-requested downloads from direct file links only.
Never executes page scripts, follows advertisements or disables TLS verification.
"""
import base64
import hashlib
from html.parser import HTMLParser
import io
import ipaddress
import json
from pathlib import Path
import re
import socket
import ssl
import stat
import threading
import time
from urllib.parse import urlsplit,urljoin
import urllib.request
import zipfile

ROOT=Path(__file__).resolve().parents[2]
ORIGIN='https://www.freeroms.com'
# Category names read from the provider homepage, not guessed download endpoints.
SYSTEMS={'gba':('gba_roms','gba',{'gba'}),'nes':('nes','nes',{'nes','fds','unf'}),'snes':('snes_rom','snes',{'sfc','smc'}),'gbc':('gameboy_color','gameboy_color',{'gb','gbc'}),'n64':('n64_roms','n64',{'n64','z64','v64'}),'nds':('nds','nds',{'nds'}),'megadrive':('genesis','genesis',{'md','gen','bin'}),'sms':('sega_master_system','sega_master_system',{'sms'}),'pce':('tg16','tg16',{'pce'}),'atari2600':('atari','atari',{'a26','bin'}),'jaguar':('atari_jaguar','atari_jaguar',{'j64','jag','rom'}),'lynx':('atari_lynx','atari_lynx',{'lnx'}),'coleco':('colecovision','colecovision',{'col','rom'}),'ngp':('neogeo_pocket','neogeo_pocket',{'ngp','ngc','npc'}),'wswan':('wonderswan','wonderswan',{'ws','wsc'})}
NETWORK=threading.BoundedSemaphore(3)
ART_LOCKS=[threading.Lock() for _ in range(64)]
MAX_FILE=64*1024*1024
class ProviderError(ValueError):pass
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*a,**kw):return None

def context():
    ctx=ssl.create_default_context()
    ctx.load_verify_locations(cafile=str(ROOT/'deploy/pi-tv/certs/freeroms-chain.pem'))
    # Intermediate certificates must still chain to a system-trusted root.
    ctx.verify_flags &= ~ssl.VERIFY_X509_PARTIAL_CHAIN
    return ctx

def valid_url(url,kind):
    u=urlsplit(url)
    if u.scheme!='https' or u.username or u.password or u.port not in (None,443) or u.fragment:raise ProviderError('Adresse du fournisseur refusée.')
    if kind in ('html','image') and u.hostname not in ('www.freeroms.com','freeroms.com'):raise ProviderError('Source extérieure refusée.')
    if kind=='file' and not re.fullmatch(r'(?:www\.|download\.|dl[0-9]+\.)?freeroms\.com',u.hostname or ''):raise ProviderError('Hôte de téléchargement non pris en charge.')
    if kind=='html' and not (u.path=='/robots.txt' or re.fullmatch(r'/[a-z0-9_]+_(?:[A-Z]|NUM)\.htm',u.path) or re.fullmatch(r'/roms/[a-z0-9_]+/[a-z0-9_-]+\.htm',u.path)):raise ProviderError('Page du fournisseur refusée.')
    if kind=='image' and not re.fullmatch(r'/roms_screenshot/[a-z0-9_]+/[A-Za-z0-9_.-]+\.(?:jpg|jpeg|png|webp)',u.path,re.I):raise ProviderError('Image refusée.')
    if kind=='file' and not (re.search(r'\.(?:zip|nes|fds|unf|sfc|smc|gb|gbc|gba|nds|n64|z64|v64|md|gen|bin|sms|pce|a26|j64|jag|rom|lnx|col|ngp|ngc|npc|ws|wsc)$',u.path,re.I) or u.path in ('/download.php','/dm_download.php')):raise ProviderError('Aucun fichier direct pris en charge.')
    return u

def fetch(url,kind='html',limit=2*1024*1024,progress=None):
    u=valid_url(url,kind)
    # Fixed provider hosts only; reject private, loopback and link-local addresses.
    addresses=socket.getaddrinfo(u.hostname,443,type=socket.SOCK_STREAM)
    if not addresses or any(not ipaddress.ip_address(x[4][0]).is_global for x in addresses):raise ProviderError('Adresse réseau du fournisseur refusée.')
    with NETWORK:
        req=urllib.request.Request(url,headers={'User-Agent':'GameFlex/1.0 (user-requested catalogue)','Accept':'text/html,text/plain' if kind=='html' else 'image/*' if kind=='image' else 'application/octet-stream,application/zip'})
        with urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=context())).open(req,timeout=12) as response:
            mime=response.headers.get_content_type()
            expected=response.headers.get('Content-Length')
            if expected and int(expected)>limit:raise ProviderError('Fichier trop volumineux pour cet import (64 Mio maximum).')
            if kind=='html' and mime not in ('text/html','text/plain'):raise ProviderError('La source ne renvoie pas une page de catalogue.')
            if kind=='file' and (mime.startswith('text/') or mime in ('application/json','application/javascript')):raise ProviderError('Le site a renvoyé une page, pas un fichier de jeu.')
            if progress:
                total=int(expected) if expected and int(expected)>0 else None
                chunks=[];received=0;progress(0,total)
                while True:
                    chunk=response.read(min(65536,limit+1-received))
                    if not chunk:break
                    chunks.append(chunk);received+=len(chunk)
                    if received>limit:raise ProviderError('Réponse trop volumineuse.')
                    progress(received,total)
                if total is not None and received!=total:raise ProviderError('Le transfert a été interrompu avant la fin du fichier.')
                body=b''.join(chunks)
            else:body=response.read(limit+1)
            if len(body)>limit:raise ProviderError('Réponse trop volumineuse.')
            return body,mime

class Page(HTMLParser):
    def __init__(self):super().__init__();self.links=[];self.images=[];self.link=None
    def handle_starttag(self,tag,attrs):
        a=dict(attrs)
        if tag=='a':self.link={'href':a.get('href',''),'title':'','rel':a.get('rel','')}
        if tag=='img':self.images.append(a)
    def handle_data(self,text):
        if self.link is not None:self.link['title']+=text
    def handle_endtag(self,tag):
        if tag=='a' and self.link is not None:
            self.link['title']=' '.join(self.link['title'].split());self.links.append(self.link);self.link=None

def folder(data):
    path=Path(data)/'freeroms';path.mkdir(exist_ok=True);return path

def save(path,obj):
    from library import atomic_json
    atomic_json(path,obj)

def catalogue(data,console,letter):
    if console not in SYSTEMS:return {'items':[],'source':ORIGIN,'message':'Console non prise en charge par cet import FreeROMs.'}
    if not re.fullmatch('[A-Z0]',letter):raise ProviderError('Lettre refusée.')
    path=folder(data)/(console+'-'+letter+'.json')
    try:
        cached=json.loads(path.read_text())
        if time.time()-path.stat().st_mtime<86400:return cached
    except (OSError,ValueError):cached=None
    category,system,_=SYSTEMS[console]
    try:
        # Read only the requested public letter, never search/download/ad scripts.
        url=ORIGIN+'/'+category+'_'+('NUM' if letter=='0' else letter)+'.htm'
        parser=Page();parser.feed(fetch(url)[0].decode('utf-8','replace'));items=[];seen=set()
        for link in parser.links:
            u=urlsplit(urljoin(ORIGIN,link['href']))
            if u.hostname not in ('www.freeroms.com','freeroms.com') or u.query:continue
            match=re.fullmatch('/roms/'+re.escape(system)+r'/([a-z0-9_-]+)\.htm',u.path)
            if not match or u.path in seen:continue
            slug=match[1];title=link['title']
            if not title or re.search(r'\b(?:bios|firmware|boot rom)\b',title,re.I):continue
            seen.add(u.path);items.append({'title':title,'path':u.path,'source':ORIGIN+u.path,'provider':'FreeROMs','consoleId':console,'downloadId':'freeroms:'+console+':'+slug,'image':'/experience-api/v1/freeroms-art/'+console+'/'+slug,'attribution':'FreeROMs · Ayants droit du jeu'})
        result={'items':items[:1000],'source':url,'provider':'FreeROMs'};save(path,result);return result
    except (OSError,ValueError):
        if cached:return {**cached,'stale':True}
        return {'items':[],'source':ORIGIN,'error':'Catalogue FreeROMs momentanément inaccessible.'}

def decode(identifier):
    if not isinstance(identifier,str):raise ProviderError('Identifiant du jeu invalide.')
    m=re.fullmatch(r'freeroms:([a-z0-9_-]+):([a-z0-9_-]{1,200})',identifier)
    if not m or m[1] not in SYSTEMS or re.search(r'(?:^|_)(?:bios|firmware|boot_rom)(?:_|$)',m[2]):raise ProviderError('Identifiant du fournisseur refusé.')
    return m[1],m[2]

def detail(data,console,slug,refresh=False):
    decode('freeroms:'+console+':'+slug)
    path=folder(data)/(console+'-'+slug+'.detail.json')
    if not refresh:
        try:
            if time.time()-path.stat().st_mtime<7*86400:return json.loads(path.read_text())
        except (OSError,ValueError):pass
    url=ORIGIN+'/roms/'+SYSTEMS[console][1]+'/'+slug+'.htm'
    parser=Page();parser.feed(fetch(url)[0].decode('utf-8','replace'));images=[];direct=[]
    for image in parser.images:
        candidate=urljoin(ORIGIN,image.get('src',''))
        try:valid_url(candidate,'image');images.append(candidate)
        except ProviderError:pass
    for link in parser.links:
        if 'sponsored' in link['rel'].lower().split():continue
        candidate=urljoin(ORIGIN,link['href'])
        try:valid_url(candidate,'file');direct.append(candidate)
        except ProviderError:pass
    result={'source':url,'image':images[0] if images else None,'direct':direct[0] if direct else None};save(path,result);return result

def artwork(data,console,slug):
    from artwork import cache_bytes
    from covers import optimize_image,WRITE_LOCK
    path=folder(data)/(console+'-'+slug+'.art.json')
    with ART_LOCKS[int(hashlib.sha256((console+':'+slug).encode()).hexdigest()[:8],16)%64]:
        try:
            record=json.loads(path.read_text())
            if record.get('reviewAfter',0)>time.time() and (Path(data)/'media'/(record['id']+'.img')).exists():return record
        except (OSError,ValueError):pass
        page=detail(data,console,slug)
        if not page['image']:return None
        body,mime=fetch(page['image'],'image',4*1024*1024);thumbnail=optimize_image(body,mime)
        with WRITE_LOCK:
            record=cache_bytes(Path(data)/'media',thumbnail,'image/webp',{'provider':'FreeROMs','author':'Ayants droit du jeu · FreeROMs','title':slug,'source':page['source'],'sourceImage':page['image'],'license':'Illustration protégée · consultation personnelle','licenseUrl':page['source'],'approved':True,'kind':'game-cover','target':'freeroms:'+console+':'+slug,'modifications':'Miniature WebP sans recadrage ; jaquette et capture de la source conservées.'})
        save(path,record);return record

def payload(data,identifier,progress=None):
    console,slug=decode(identifier);page=detail(data,console,slug,refresh=True)
    if not page['direct']:raise ProviderError('Aucun fichier reçu : FreeROMs ne fournit pas de lien direct compatible pour cette édition. Aucun jeu n’a été installé.')
    body,mime=fetch(page['direct'],'file',MAX_FILE,progress=(lambda received,total:progress(phase='downloading',bytesReceived=received,totalBytes=total,progress=min(99,int(received*100/total)) if total else None,message='Téléchargement du fichier…')) if progress else None)
    if progress:progress(phase='validating',bytesReceived=len(body),totalBytes=len(body),progress=99,message='Vérification du fichier reçu…')
    return unpack_payload(console,slug,body,page['direct'],progress)

def unpack_payload(console,slug,body,filename_source,progress=None,extensions=None):
    extensions=extensions if extensions is not None else SYSTEMS[console][2]
    if body.startswith(b'PK\x03\x04') and extensions=={'zip'}:
        try:
            with zipfile.ZipFile(io.BytesIO(body)) as archive:
                entries=archive.infolist()
                if not entries or len(entries)>300 or sum(entry.file_size for entry in entries)>MAX_FILE:raise ProviderError('Archive arcade trop volumineuse.')
                for entry in entries:
                    path=Path(entry.filename)
                    if path.is_absolute() or '..' in path.parts or '\\' in entry.filename or entry.flag_bits&1 or stat.S_ISLNK(entry.external_attr>>16):raise ProviderError('Archive arcade refusée.')
        except zipfile.BadZipFile:raise ProviderError('Archive arcade illisible.')
        filename=slug+'.zip'
    elif body.startswith(b'PK\x03\x04'):
        if progress:progress(phase='extracting',progress=99,message='Extraction de l’archive…')
        try:
            with zipfile.ZipFile(io.BytesIO(body)) as archive:
                if len(archive.infolist())>300:raise ProviderError('Archive trop complexe.')
                candidates=[]
                for entry in archive.infolist():
                    path=Path(entry.filename)
                    if path.is_absolute() or '..' in path.parts or '\\' in entry.filename or stat.S_ISLNK(entry.external_attr>>16):raise ProviderError('Chemin dangereux dans l’archive.')
                    if not entry.is_dir() and path.suffix.lower().lstrip('.') in extensions:candidates.append(entry)
                if len(candidates)!=1:raise ProviderError('L’archive doit contenir un seul fichier de jeu compatible avec cette console.')
                entry=candidates[0]
                if not 0<entry.file_size<=MAX_FILE or entry.flag_bits&1:raise ProviderError('Archive chiffrée ou jeu trop volumineux.')
                with archive.open(entry) as file:body=file.read(MAX_FILE+1)
                filename=Path(entry.filename).name
        except (zipfile.BadZipFile,RuntimeError,NotImplementedError):raise ProviderError('Archive ZIP illisible ou méthode non prise en charge.')
    else:
        suffix=Path(urlsplit(filename_source).path).suffix.lower()
        if suffix.lstrip('.') not in extensions:raise ProviderError('Format du fichier téléchargé non pris en charge pour cette console.')
        filename=slug+suffix
    if filename.startswith('.') or len(filename)>220 or any(ord(c)<32 for c in filename):raise ProviderError('Nom du fichier refusé.')
    if not body or len(body)>MAX_FILE:raise ProviderError('Taille du jeu refusée.')
    if body[:256].lstrip().lower().startswith((b'<!doctype',b'<html',b'<script',b'mz',b'\x7felf')):raise ProviderError('Le fichier n’est pas un jeu pour émulateur.')
    if console=='gba' and (len(body)<192 or body[0xB2]!=0x96 or ((-sum(body[0xA0:0xBD])-0x19)&255)!=body[0xBD]):raise ProviderError('En-tête GBA invalide : ce fichier ne peut pas être importé comme jeu.')
    if console=='nes' and not body.startswith(b'NES\x1a'):raise ProviderError('En-tête NES invalide.')
    return {'consoleId':console,'filename':filename,'size':len(body),'sha256':hashlib.sha256(body).hexdigest()},body
