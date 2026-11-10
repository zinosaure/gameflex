"""Persistent metadata bindings, manual corrections and explicit background refreshes.
Reading the library never triggers a provider search or a complete reindex.
"""
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
from pathlib import Path
import re
import threading
import time
import uuid
from emugen import FIELDS,SYSTEMS,EmugenError,detail,identify,merge_metadata
from library import atomic_json

POOL=ThreadPoolExecutor(max_workers=1,thread_name_prefix='metadata-refresh');SLOT=threading.BoundedSemaphore(1)
LOCK=threading.RLock()

def key(console,reference):
    if not isinstance(console,str) or console not in SYSTEMS or not isinstance(reference,str) or not 0<len(reference)<=512 or re.search(r'[\x00-\x1f]',reference):raise EmugenError('invalid','Référence de jeu invalide.')
    return hashlib.sha256((console+':'+reference).encode()).hexdigest()

def clean_fields(source):
    if not isinstance(source,dict):raise EmugenError('invalid','Métadonnées invalides.')
    result={}
    arrays={'images','screenshots','trailers','genres','languages'}
    for field in FIELDS:
        if field not in source:continue
        value=source[field]
        if field in arrays:
            if not isinstance(value,list) or len(value)>24 or any(not isinstance(v,str) or len(v)>3000 for v in value):raise EmugenError('invalid','Liste de métadonnées invalide.')
        elif not isinstance(value,str) or len(value)>(16000 if field=='description' else 3000):raise EmugenError('invalid','Champ de métadonnées invalide.')
        result[field]=value
    return result

def record_path(data,console,reference):return Path(data)/'game-metadata'/(key(console,reference)+'.json')
def load(data,console,reference):
    try:return json.loads(record_path(data,console,reference).read_text())
    except (OSError,ValueError):return None

def listing(data,console=None):
    items=[]
    for path in (Path(data)/'game-metadata').glob('*.json'):
        try:item=json.loads(path.read_text())
        except (OSError,ValueError):continue
        if not console or item.get('consoleId')==console:items.append(item)
        if len(items)>=2000:break
    return items

def store(data,base,identified,previous=None):
    console=base['consoleId'];reference=base['reference'];path=record_path(data,console,reference)
    # Re-read under the same lock as corrections: a correction made during HTTP work wins.
    with LOCK:
        previous={**(previous or {}),**(load(data,console,reference) or {})}
        manual={**base.get('manualMetadata',{}),**previous.get('manualMetadata',{})}
        saved={**previous,'reference':reference,'consoleId':console,'base':{**previous.get('base',{}),**clean_fields(base),'consoleId':console},'manualMetadata':manual,'updatedAt':time.time()}
        if identified['status']=='matched':saved.update(emugen=identified['item'],status='matched',candidates=[])
        else:saved.update(status=identified['status'],candidates=identified.get('candidates',[])[:30])
        saved['item']=merge_metadata({**saved['base'],'reference':reference},saved.get('emugen'),manual=manual)
        atomic_json(path,saved)
    return saved

def bind(data,base,slug):
    if not isinstance(base,dict):raise EmugenError('invalid','Jeu invalide.')
    key(base.get('consoleId'),base.get('reference'));base={**base,**clean_fields(base)}
    if not isinstance(base.get('title'),str) or not base['title']:raise EmugenError('invalid','Titre requis.')
    return store(data,base,{'status':'matched','item':detail(data,slug,base['consoleId'])})

def correct(data,console,reference,values):
    clean=clean_fields(values);clean.pop('consoleId',None)
    with LOCK:
        saved=load(data,console,reference)
        if not saved:raise EmugenError('not_found','Métadonnées du jeu introuvables.')
        manual={**saved.get('manualMetadata',{}),**clean};saved['manualMetadata']=manual
        saved['item']=merge_metadata({**saved['base'],'reference':reference},saved.get('emugen'),manual=manual)
        atomic_json(record_path(data,console,reference),saved)
    return saved

def user_key(user):return hashlib.sha256(user.encode()).hexdigest()
def job_path(data,job):return Path(data)/'metadata-jobs'/(job+'.json')
def job(data,user,job_id):
    if not re.fullmatch('[a-f0-9]{32}',job_id):raise EmugenError('not_found','Rafraîchissement introuvable.')
    try:value=json.loads(job_path(data,job_id).read_text())
    except (OSError,ValueError):raise EmugenError('not_found','Rafraîchissement introuvable.')
    if value['owner']!=user_key(user):raise EmugenError('not_found','Rafraîchissement introuvable.')
    return {k:v for k,v in value.items() if k!='owner'}

def refresh(data,user,items):
    if not isinstance(items,list) or not 0<len(items)<=2000:raise EmugenError('invalid','Sélectionnez entre 1 et 2 000 jeux.')
    valid=[]
    for item in items:
        if not isinstance(item,dict):raise EmugenError('invalid','Jeu invalide.')
        if item.get('consoleId') not in SYSTEMS:continue
        key(item['consoleId'],item.get('reference'));base={**clean_fields(item),'reference':item['reference']}
        if not base.get('title'):raise EmugenError('invalid','Titre requis.')
        base['manualMetadata']=clean_fields(item.get('manualMetadata',{}));valid.append(base)
    if not SLOT.acquire(blocking=False):raise EmugenError('busy','Un rafraîchissement des métadonnées est déjà en cours.')
    value={'id':uuid.uuid4().hex,'owner':user_key(user),'status':'running','total':len(valid),'completed':0,'matched':0,'choices':0,'failed':0,'createdAt':time.time()}
    try:atomic_json(job_path(data,value['id']),value)
    except Exception:SLOT.release();raise
    POOL.submit(_run,data,valid,value)
    return {k:v for k,v in value.items() if k!='owner'}

def _run(data,items,value):
    path=job_path(data,value['id'])
    try:
        for base in items:
            try:
                previous=load(data,base['consoleId'],base['reference'])
                if previous and previous.get('emugen'):base={**base,'emugenSlug':previous['emugen']['emugenSlug']}
                result=identify(data,base,refresh=True);store(data,base,result,previous)
                if result['status']=='matched':value['matched']+=1
                if result['status']=='choice':value['choices']+=1
            except Exception:
                # Preserve saved metadata/corrections on all provider failures.
                value['failed']+=1
            value['completed']+=1;atomic_json(path,value)
        value.update(status='completed',updatedAt=time.time());atomic_json(path,value)
    except Exception:value.update(status='failed',updatedAt=time.time());atomic_json(path,value)
    finally:SLOT.release()

def recover_jobs(data):
    # Local job recovery only; never enumerate or reindex games at startup.
    for path in (Path(data)/'metadata-jobs').glob('*.json'):
        try:
            value=json.loads(path.read_text())
            if value.get('status')=='running':
                value.update(status='failed',message='Actualisation interrompue ; les métadonnées précédentes sont conservées.',updatedAt=time.time());atomic_json(path,value)
        except (OSError,ValueError):pass
