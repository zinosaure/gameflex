"""Read-only, bounded Planetemu title catalogue. Never opens ROM/download URLs."""
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import threading
import unicodedata
import time
import urllib.request
from urllib.parse import urljoin, urlsplit, parse_qs
from urllib.robotparser import RobotFileParser

ORIGIN = 'https://www.planetemu.net'
MACHINES = {'amstrad':'amstrad-cpc','atari2600':'atari-2600','atari7800':'atari-7800','jaguar':'jaguar','lynx':'lynx','coleco':'colecovision','c64':'commodore-64','gbc':'game-boy','gba':'gba','ngp':'neo-geo-pocket','n64':'nintendo-64','nds':'nintendo-ds','nes':'nes','gc':'gamecube','pce':'pc-engine','pcfx':'pc-fx','ps1':'playstation','dreamcast':'dreamcast','sms':'master-system','megadrive':'mega-drive','saturn':'saturn','zxspectrum':'zx-spectrum','snes':'super-nes','vb':'virtual-boy','wswan':'wonderswan'}
FETCH_LOCK = threading.Lock()
LAST_FETCH = 0

class Links(HTMLParser):
    def __init__(self):
        super().__init__(); self.items=[]; self.current=None
    def handle_starttag(self, tag, attrs):
        attrs=dict(attrs)
        if tag=='a': self.current={'href':attrs.get('href',''),'title':''}
    def handle_data(self, data):
        if self.current is not None: self.current['title']+=data
    def handle_endtag(self, tag):
        if tag=='a' and self.current is not None:
            self.current['title']=' '.join(self.current['title'].split()); self.items.append(self.current);self.current=None

def read_html(path):
    # Only public machine/list HTML. Detail, archive, search and download endpoints cannot pass.
    if not re.fullmatch(r'/(?:machine|roms)/[a-z0-9-]+(?:\?page=[A-Z0])?|/robots.txt', path): raise ValueError('path refused')
    request=urllib.request.Request(ORIGIN+path,headers={'User-Agent':'GameFlex/1.0 (read-only metadata catalogue)','Accept':'text/html,text/plain'})
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self,*args,**kwargs): return None
    with urllib.request.build_opener(NoRedirect).open(request,timeout=5) as response:
        if response.headers.get_content_type() not in ['text/html','text/plain']: raise ValueError('not HTML')
        body=response.read(2_000_001)
        if len(body)>2_000_000: raise ValueError('page too large')
        return body.decode('utf-8')

def matches_letter(title,letter):
    text=unicodedata.normalize('NFKD', title).upper()
    first=next((c for c in text if c.isalnum()),'')
    return first.isdigit() if letter=='0' else first==letter

def catalogue(data, console, letter='A'):
    global LAST_FETCH
    if console not in MACHINES: return {'items':[],'message':'Cette console n’a pas de catalogue associé sur Planetemu.','source':ORIGIN}
    if not re.fullmatch('[A-Z0]',letter): raise ValueError('letter refused')
    folder=Path(data)/'planetemu';folder.mkdir(exist_ok=True)
    cache=folder/(console+'-'+letter+'-v3.json')
    def cached():
        try:return json.loads(cache.read_text())
        except (OSError,ValueError):return None
    existing=cached()
    if existing and time.time()-cache.stat().st_mtime<86400:return existing
    with FETCH_LOCK:
        if time.time()-LAST_FETCH<1.1: time.sleep(1.1-(time.time()-LAST_FETCH))
        try:
            robot=RobotFileParser(); robot.parse(read_html('/robots.txt').splitlines())
            machine='/machine/'+MACHINES[console]
            if not robot.can_fetch('GameFlex',ORIGIN+machine):raise ValueError('robots denied')
            machine_cache=folder/(console+'-sets.json')
            if machine_cache.exists() and time.time()-machine_cache.stat().st_mtime<604800: sets=json.loads(machine_cache.read_text())
            else:
                parser=Links();parser.feed(read_html(machine))
                sets=list(dict.fromkeys(x['href'] for x in parser.items if re.fullmatch('/roms/[a-z0-9-]+',x['href']) and not re.search('bios|firmware|translation|hack|applications|operating-systems|compilations|demos|diskmags|educational',x['href'],re.I)))[:4]
                machine_cache.write_text(json.dumps(sets))
            sets=[p for p in sets if '-e-cards' not in p]
            items=[]
            for path in sets:
                time.sleep(1.1); page=path+'?page='+letter
                if not robot.can_fetch('GameFlex',ORIGIN+page):continue
                parser=Links();parser.feed(read_html(page))
                for x in parser.items:
                    title=x['title'];href=x['href']
                    if not re.fullmatch('/rom/[a-z0-9-]+/[a-z0-9-]+',href) or re.search(r'\b(bios|firmware|boot rom)\b',title,re.I):continue
                    if title and matches_letter(title,letter) and href not in {g['path'] for g in items}:
                        from covers import clean_title
                        cleaned=clean_title(title)
                        items.append({'title':cleaned,'path':href,'source':ORIGIN+href,**({'availability':'withdrawn'} if cleaned!=title else {})})
            items.sort(key=lambda x:x['title'].casefold())
            result={'items':items[:1000],'source':ORIGIN+machine,'letter':letter,'fetchedAt':int(time.time()),'metadataOnly':True}
            temp=cache.with_suffix('.tmp');temp.write_text(json.dumps(result,ensure_ascii=False));temp.replace(cache)
            return result
        except (OSError,ValueError):
            if existing:return {**existing,'stale':True}
            return {'items':[],'source':ORIGIN+'/machine/'+MACHINES[console],'error':'Catalogue temporairement indisponible. Ouvrez la source pour le consulter.'}
        finally:LAST_FETCH=time.time()
