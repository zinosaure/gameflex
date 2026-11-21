"""Streamed personal-file imports. Never fetches a ROM from the network."""
import hashlib
import re
import unicodedata
from pathlib import Path
from library import LibraryError, atomic_json

FORMATS={
 'nes':{'nes','unf'},'snes':{'sfc','smc'},'gbc':{'gb','gbc'},'gba':{'gba'},
 'n64':{'n64','z64','v64'},'nds':{'nds'},'megadrive':{'md','gen','bin'},
 'sms':{'sms'},'pce':{'pce'},'atari2600':{'a26','bin'},'atari7800':{'a78'},
 'jaguar':{'j64','jag','rom'},'lynx':{'lnx'},'coleco':{'col','rom'},
 'ngp':{'ngp','ngc','npc'},'wswan':{'ws','wsc'},'vb':{'vb'},
 'vectrex':{'vec'},'c64':{'d64','t64','prg'},'amstrad':{'dsk','cdt'},
 'zxspectrum':{'z80','sna','tap','tzx'},'neogeo':{'zip'},'cps1':{'zip'},'cps2':{'zip'},
 'ps1':{'chd','iso','pbp'},'ps2':{'chd','iso'},'psp':{'iso','cso','pbp'},
 'dreamcast':{'chd','cdi'},'saturn':{'chd'},'gc':{'iso','rvz'},'wii':{'iso','wbfs','rvz'},
 'pcfx':{'chd'},'doom':{'wad'}
}
def validate(console,name):
    if console not in FORMATS:raise LibraryError('Console non prise en charge pour cet import.')
    if not isinstance(name,str) or not 1<=len(name)<=200 or name.startswith('.') or re.search(r'[/\\\x00-\x1f\x7f]',name) or '..' in name:
        raise LibraryError('Nom de fichier refusé.')
    if Path(name).suffix.lower().lstrip('.') not in FORMATS[console]:
        raise LibraryError('Format incompatible avec cette console. Sélectionnez un fichier de jeu décompressé (archive ZIP uniquement pour l’arcade).')
    return unicodedata.normalize('NFC',name)

def describe(console,name,body):
    digest=hashlib.sha256(body).hexdigest()
    title=Path(name).stem.replace('_',' ').strip()
    internal=''
    bounds={'gba':(0xa0,0xac),'gbc':(0x134,0x143),'nds':(0,12)}.get(console)
    if bounds and len(body)>=bounds[1]:
        raw=body[bounds[0]:bounds[1]].split(b'\0')[0]
        if raw and all(32<=x<127 for x in raw):internal=raw.decode('ascii').strip()
    # Filename is generally a fuller regional title than a short cartridge header.
    basis='filename'
    if internal and re.fullmatch(r'(?:rom|game|dump|unknown|[0-9a-f]{8,64})(?:[- _]\d+)?',title,re.I):
        title=internal;basis='header'
    from covers import image_url
    return {'consoleId':console,'filename':name,'title':title or internal or 'Jeu importé',
            'internalTitle':internal,'identification':basis,'sha256':digest,'size':len(body),
            'image':image_url(console,{'title':title}) or '',
            'attribution':'Libretro Thumbnails · Ayants droit du jeu'}

def validate_content(console,name,prefix):
    if prefix[:2] in (b'MZ',b'#!') or prefix.startswith(b'\x7fELF') or prefix[:256].lstrip()[:32].lower().startswith((b'<!doctype html',b'<html')):
        raise LibraryError('Ce fichier n’est pas un fichier de jeu compatible.')
    if console=='nes' and Path(name).suffix.lower()=='.nes' and not prefix.startswith(b'NES\x1a'):
        raise LibraryError('En-tête NES invalide.')

def enrich(library,console,name,body):
    item=describe(console,name,body)
    # Cover lookup uses only metadata and image endpoints; never game files.
    from covers import peek,normalized
    if item['image']:
        system,encoded=item['image'].split('/')[-2:]
        ready=peek(library.data,system,encoded)
        if ready:item['image']=ready['url']
    import json
    for letter in '0ABCDEFGHIJKLMNOPQRSTUVWXYZ':
        path=library.data/'freeroms'/(console+'-'+letter+'.json')
        try:entries=json.loads(path.read_text()).get('items',[])
        except (OSError,ValueError):continue
        match=next((entry for entry in entries if normalized(entry.get('title',''))==normalized(item['title'])),None)
        if match:
            image=match.get('image','')
            if re.fullmatch(r'/experience-api/v1/freeroms-art/[a-z0-9_-]+/[a-z0-9_-]{1,200}',image):
                item.update(fallbackImage=image,fallbackAttribution=match.get('attribution','Ayants droit du jeu'))
            break
    # ROMsPure catalogue metadata already on disk is an additional artwork source.
    for path in sorted((library.data/'romspure').glob('page-*.json'),key=lambda p:p.stat().st_mtime,reverse=True)[:150]:
        try:entries=json.loads(path.read_text()).get('items',[])
        except (OSError,ValueError):continue
        match=next((entry for entry in entries if entry.get('consoleId')==console and normalized(entry.get('title',''))==normalized(item['title'])),None)
        if match and re.fullmatch(r'/experience-api/v1/romspure-art/[a-z0-9_-]+/[a-z0-9~-]{1,301}',match.get('image','')):
            item.update(fallbackImage=item['image'],fallbackAttribution=item['attribution'],image=match['image'],attribution=match.get('attribution','Ayants droit du jeu'))
            break
    return item

def record_import(library,console,name,item):
    key=hashlib.sha256((console+':'+name).encode()).hexdigest()
    atomic_json(library.data/'imports'/(key+'.json'),item)
    return item

def install_stream(library,console,name,source,size):
    name=validate(console,name)
    if size<=0:raise LibraryError('Le fichier est vide.')
    if not library.status()['configured']:raise LibraryError('Le dossier de jeux est indisponible.')
    header=b''
    while len(header)<min(size,512):
        chunk=source.read(min(size,512)-len(header))
        if not chunk:raise LibraryError('Transfert incomplet. Aucun fichier installé.')
        header+=chunk
    validate_content(console,name,header)
    class Replay:
        def __init__(self):self.prefix=header
        def read(self,count):
            first=self.prefix[:count];self.prefix=self.prefix[len(first):]
            return first+(source.read(count-len(first)) if len(first)<count else b'')
    stream=Replay()
    def prepare(prefix,digest,count):
        item=enrich(library,console,name,prefix)
        item.update(sha256=digest,size=count)
        return item
    item=library.receive_game(console,name,stream,size,prepare,lambda prefix:validate_content(console,name,prefix))
    return record_import(library,console,name,item)

def install(library,console,name,body):
    from io import BytesIO
    if not body:raise LibraryError('Le fichier est vide.')
    return install_stream(library,console,name,BytesIO(body),len(body))

def metadata(library):
    result=[]
    for path in (library.data/'imports').glob('*.json'):
        try:
            import json
            result.append(json.loads(path.read_text()))
        except (OSError,ValueError):pass
        if len(result)>=2000:break
    return result
