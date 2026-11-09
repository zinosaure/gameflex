"""Ask secondary adapters only for missing fields they actually expose."""
from emugen import FIELDS,compact,present,merge_metadata
from providers import provider,decode
from source_settings import enabled

# These existing adapters currently expose boxart, not complete game metadata.
REMOTE_FIELDS={'freeroms':{'image','coverFront'},'romspure':{'image','coverFront'}}
def enrich(data,base,primary,variants=()):
    manual={**base.get('overrides',{}),**base.get('manualMetadata',{})}
    if primary.get('coverFront') and not primary.get('image'):primary={**primary,'image':primary['coverFront']}
    if 'coverFront' in manual and 'image' not in manual:manual['image']=manual['coverFront']
    missing={key for key in FIELDS if key!='consoleId' and key not in manual and not present(primary.get(key)) and not present(base.get(key))}
    secondary=[]
    for variant in variants[:20]:
        if not isinstance(variant,dict) or variant.get('consoleId',base['consoleId'])!=base['consoleId']:continue
        if compact(variant.get('title','')) not in {compact(base.get('title','')),compact(primary.get('title',''))}:continue
        # Reuse already available metadata before making any external request.
        record={key:variant[key] for key in missing if present(variant.get(key))}
        identifier=variant.get('downloadId','');family=identifier.split(':',1)[0] if isinstance(identifier,str) else ''
        needed=(missing-set(record))&REMOTE_FIELDS.get(family,set())
        if needed and enabled(data,family):
            try:
                adapter=provider(identifier);console,_=decode(identifier)
                if console!=base['consoleId']:continue
                parts=adapter.decode(identifier)
                if family=='freeroms':page=adapter.detail(data,parts[0],parts[1]);url='/experience-api/v1/freeroms-art/'+parts[0]+'/'+parts[1]
                else:page=adapter.detail(data,identifier);url='/experience-api/v1/romspure-art/'+parts[0]+'/'+parts[1]+'~'+parts[2]
                if page.get('image'):
                    for field in needed:record[field]=url
            except Exception:pass
        if record:
            record.update(provider=variant.get('provider') or family or 'Catalogue',consoleId=base['consoleId']);secondary.append(record);missing-=set(record)
        if not missing:break
    return merge_metadata(base,primary,secondary,manual)
