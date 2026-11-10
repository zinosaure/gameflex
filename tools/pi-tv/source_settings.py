"""Server-wide provider switches. EmuGen first; other providers start muted."""
import json,os,threading
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
LOCK=threading.RLock()
def registry():return json.loads((ROOT/'src/webapp/src/app/gameflex/experience/game-sources.json').read_text())['items']
def enabled_ids(data):
    try:
        value=json.loads((Path(data)/'provider-settings.json').read_text())['enabledIds']
        if not isinstance(value,list) or any(not isinstance(x,str) for x in value):return []
        return [item['id'] for item in registry() if item['id'] in value] or (['planetemu','freeroms','romspure'] if 'emugen' in value else [])
    except FileNotFoundError:return ['planetemu','freeroms','romspure']
    except (OSError,ValueError,KeyError,TypeError):return []
def enabled(data,identifier):return identifier in enabled_ids(data)
def listing(data):
    active=enabled_ids(data)
    return {'items':[{**item,'enabled':item['id'] in active} for item in registry()],'enabledIds':active}
def save(data,value):
    known={item['id'] for item in registry()}
    if not isinstance(value,list) or any(not isinstance(x,str) or x not in known for x in value) or len(value)!=len(set(value)):raise ValueError('Fournisseurs invalides.')
    folder=Path(data);folder.mkdir(parents=True,exist_ok=True)
    with LOCK:
        temporary=folder/'provider-settings.tmp'
        fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
        with os.fdopen(fd,'w') as file:json.dump({'enabledIds':value},file)
        temporary.replace(folder/'provider-settings.json')
    return listing(data)

def cached_art(data,family,token):
    """Keep existing covers visible when muted, without contacting that provider."""
    import base64,hashlib,re
    data=Path(data)
    try:
        if family=='emugen':
            if not re.fullmatch('[A-Za-z0-9_-]{1,2500}',token):return None
            url=base64.urlsafe_b64decode(token+'='*((-len(token))%4)).decode()
            mapping=data/'emugen'/('art-'+hashlib.sha256(url.encode()).hexdigest()+'.json')
        else:
            console,slug=token.split('/',1)
            if not re.fullmatch('[a-z0-9_-]{1,40}',console) or not re.fullmatch(r'[a-z0-9_~-]{1,301}',slug):return None
            mapping=data/'freeroms'/(console+'-'+slug+'.art.json') if family=='freeroms' else data/'romspure'/('art-'+hashlib.sha256(('romspure:'+console+':'+slug).encode()).hexdigest()+'.json')
        record=json.loads(mapping.read_text());identifier=record.get('id','')
        return record if re.fullmatch('[a-f0-9]{64}',identifier) and (data/'media'/(identifier+'.img')).is_file() else None
    except (OSError,ValueError,KeyError,TypeError,UnicodeError):return None
