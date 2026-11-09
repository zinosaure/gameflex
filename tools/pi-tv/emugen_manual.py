"""Owner-bound, single headed Chromium session for human EmuGen verification.
Only the human operates Cloudflare and Magic Cookie; no cookie/token is exported.
"""
from collections import OrderedDict
from concurrent.futures import ThreadPoolExecutor,TimeoutError as FutureTimeout
import hashlib
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import uuid
from urllib.parse import urlsplit
from emugen import HOSTS,EmugenError,game_url,valid_url,detail,SYSTEMS
from emugen_browser import extract_current_links

TTL=1200
WIDTH,HEIGHT=1024,768
DISPLAY_NUMBER=98
VNC_PORT=5999

def owner_id(username):return hashlib.sha256(username.encode()).hexdigest()
class ManualError(ValueError):
    def __init__(self,status,message,code=409):super().__init__(message);self.status=status;self.code=code

class ManualVerification:
    def __init__(self,ttl=TTL,reaper=True):
        self.ttl=ttl;self.lock=threading.RLock();self.pool=ThreadPoolExecutor(max_workers=1,thread_name_prefix='emugen-manual')
        self.records=OrderedDict();self.active=None;self.page=None;self.browser=None;self.context=None;self.driver=None;self.desktop=None;self.vnc=None;self.temp=None
        self.pinned={};self.stop=threading.Event();self.reap_future=None;self.fixture=None
        if reaper:threading.Thread(target=self._reaper,name='emugen-manual-expiration',daemon=True).start()
    def _public(self,record):
        return {key:record[key] for key in ('id','slug','consoleId','source','createdAt','expiresAt','status','width','height','result','message') if key in record}
    def _owned(self,username,session_id):
        if not isinstance(session_id,str) or not re.fullmatch('[a-f0-9]{32}',session_id):raise ManualError('not_found','Session de vérification introuvable.',404)
        record=self.records.get(session_id)
        if not record:raise ManualError('session_expired','Cette session a expiré. Relancez la vérification.',410)
        if record['owner']!=owner_id(username):raise ManualError('forbidden','Cette session appartient à un autre compte.',403)
        if record['status'] not in ('expired','closed') and time.time()>=record['expiresAt']:record['status']='expired';record.pop('result',None)
        return record
    def status(self,username,session_id):
        with self.lock:return self._public(self._owned(username,session_id))
    def has_active(self):
        with self.lock:
            record=self.records.get(self.active)
            return bool(record and record['status'] not in ('expired','closed','error') and time.time()<record['expiresAt'])
    def current(self,username,slug):
        game_url(slug)
        with self.lock:
            matches=[r for r in self.records.values() if r['owner']==owner_id(username) and r['slug']==slug]
            if not matches:return None
            return self._public(self._owned(username,matches[-1]['id']))
    def start(self,username,slug,console_id,data,background=False):
        url=game_url(slug)
        # Reuse the public HTTP metadata check, never browser cookies, for console identity.
        if not isinstance(console_id,str) or console_id not in SYSTEMS:raise EmugenError('mismatch','Console EmuGen non prise en charge.')
        if not background and self.fixture is None:detail(data,slug,console_id)
        with self.lock:
            if self.active:
                existing=self.records[self.active]
                if time.time()<existing['expiresAt'] and existing['status'] not in ('expired','closed','error'):
                    if existing['owner']==owner_id(username) and existing['slug']==slug:return self._public(existing)
                    raise ManualError('busy','Une vérification est déjà ouverte. Terminez-la ou attendez son expiration.')
            record={'id':uuid.uuid4().hex,'slug':slug,'consoleId':console_id,'source':url,'owner':owner_id(username),'createdAt':time.time(),'expiresAt':time.time()+self.ttl,'status':'starting','width':WIDTH,'height':HEIGHT}
            previous=self.active;self.active=record['id'];self.records[record['id']]=record
            while len(self.records)>64:self.records.popitem(last=False)
        if background:
            self.pool.submit(self._open_background,record,previous,data)
            with self.lock:return self._public(record)
        try:return self.pool.submit(self._open,record,previous).result(timeout=45)
        except FutureTimeout:
            # The pending task is kept on the same worker; the client can reconnect via current().
            raise ManualError('starting','Le navigateur démarre. Réessayez dans quelques secondes.')
    def _open_background(self,record,previous,data):
        try:
            with self.lock:
                if self.active!=record['id'] or record['status']!='starting':return
            if self.fixture is None:detail(data,record['slug'],record['consoleId'])
            with self.lock:
                if self.active!=record['id'] or record['status']!='starting':return
            self._open(record,previous)
        except Exception as error:
            with self.lock:
                record['status']='error'
                record['message']=str(error) if isinstance(error,(ManualError,EmugenError)) else 'Le navigateur ne peut pas être ouvert. Réessayez.'
            self._cleanup_resources()
    def _open(self,record,previous):
        self._cleanup_resources()
        from playwright.sync_api import sync_playwright
        try:
            if self.fixture is None:
                from emugen_browser import RESOLVER
                RESOLVER.release_browser()
            self.temp=tempfile.mkdtemp(prefix='gameflex-emugen-manual-')
            display=':'+str(DISPLAY_NUMBER)
            self.desktop=subprocess.Popen(['Xvfb',display,'-screen','0',f'{WIDTH}x{HEIGHT}x24','-nolisten','tcp','-noreset','-ac'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            for _ in range(60):
                if self.desktop.poll() is not None:raise RuntimeError('display')
                if Path('/tmp/.X11-unix/X'+str(DISPLAY_NUMBER)).exists():break
                time.sleep(.05)
            self.driver=sync_playwright().start()
            # Reuse the connector's validated/pinned public host resolution.
            import ipaddress
            rules=[]
            for host in HOSTS|{'challenges.cloudflare.com'}:
                rows=socket.getaddrinfo(host,443,type=socket.SOCK_STREAM) if self.fixture is None else [(socket.AF_INET,socket.SOCK_STREAM,6,'',('93.184.216.34',443))]
                if not rows or any(not ipaddress.ip_address(row[4][0]).is_global or ipaddress.ip_address(row[4][0]).is_multicast or ipaddress.ip_address(row[4][0]).is_reserved for row in rows):raise EmugenError('blocked_navigation','Destination du navigateur refusée.')
                addr=next((r[4][0] for r in rows if r[0]==socket.AF_INET),rows[0][4][0]);self.pinned[host]=addr;rules.append('MAP '+host+' '+addr)
            self.context=self.driver.chromium.launch_persistent_context(self.temp+'/profile',executable_path=os.environ.get('GAMEFLEX_CHROMIUM_EXECUTABLE','/usr/bin/chromium'),headless=False,env={**os.environ,'DISPLAY':display},args=['--kiosk','--window-position=0,0',f'--window-size={WIDTH},{HEIGHT}','--disable-dev-shm-usage','--disable-background-networking','--disable-gpu','--disable-features=Translate','--renderer-process-limit=2','--host-resolver-rules='+','.join(rules)],accept_downloads=False,service_workers='block',locale='fr-FR',viewport={'width':WIDTH,'height':HEIGHT})
            self.browser=self.context.browser
            self.context.route('**/*',lambda route:self._route(route,record))
            self.context.route_web_socket('**/*',lambda ws:ws.close())
            self.page=self.context.pages[0] if self.context.pages else self.context.new_page()
            self.context.on('page',lambda page:page.close() if page!=self.page else None)
            self.page.on('popup',lambda popup:popup.close());self.page.on('download',lambda download:download.cancel())
            response=self.page.goto(record['source'],wait_until='domcontentloaded',timeout=20000)
            self._same_game(record)
            self.page.bring_to_front()
            if response and response.status==404 or self.page.locator('body.error404').count():raise ManualError('not_found','Fiche EmuGen introuvable.',404)
            if response and response.status>=400 and response.status not in (403,503):raise ManualError('error','Impossible de charger la fiche EmuGen.',502)
            self.vnc=subprocess.Popen(['x11vnc','-display',display,'-listen','127.0.0.1','-localhost','-rfbport',str(VNC_PORT),'-forever','-shared','-nopw','-noxdamage','-nosel','-noremote','-desktop','EmuGen GameFlex','-quiet'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            for _ in range(60):
                if self.vnc.poll() is not None:raise RuntimeError('vnc')
                try:
                    with socket.create_connection(('127.0.0.1',VNC_PORT),timeout=.1):break
                except OSError:time.sleep(.05)
            else:raise RuntimeError('vnc unavailable')
            with self.lock:
                cancelled=self.active!=record['id'] or record['status'] in ('closed','expired')
            if cancelled:
                self._cleanup_resources();return self._public(record)
            with self.lock:
                record['status']='ready';record['expiresAt']=time.time()+self.ttl
                if previous and previous in self.records:self.records[previous]['status']='closed'
            return self._public(record)
        except ManualError:
            with self.lock:record['status']='error'
            self._cleanup_resources()
            raise
        except Exception:
            with self.lock:record['status']='error'
            self._cleanup_resources()
            raise ManualError('error','Le navigateur interactif n’a pas pu charger cette fiche. Réessayez.',502)
    def _route(self,route,record):
        request=route.request
        try:
            u=urlsplit(request.url)
            if u.scheme!='https' or u.username or u.password or u.port not in (None,443):return route.abort()
            main=request.is_navigation_request() and request.frame==request.frame.page.main_frame
            if main:
                valid_url(request.url,'game')
                if u.path.rstrip('/')!=urlsplit(record['source']).path.rstrip('/'):return route.abort()
            elif u.hostname not in self.pinned:return route.abort()
            if request.resource_type=='media' or re.search(r'\.(zip|7z|rar|iso|wbfs|nes|gba|gbc|bin|exe)$',u.path,re.I):return route.abort()
            if self.fixture is not None and main:return route.fulfill(content_type='text/html',body=self.fixture)
            return route.continue_()
        except Exception:return route.abort()
    def _same_game(self,record):
        if not self.page or self.page.is_closed():raise ManualError('error','La fenêtre du navigateur a été fermée.',502)
        current=valid_url(self.page.url,'game');expected=urlsplit(record['source'])
        if current.path.rstrip('/')!=expected.path.rstrip('/'):raise ManualError('error','La fenêtre ne correspond plus au jeu demandé.',502)
    def resume(self,username,session_id):
        with self.lock:
            record=self._owned(username,session_id)
            if record['status'] in ('expired','closed'):return {'status':'session_expired','links':[],'message':'La session a expiré. Relancez la vérification.'}
            if record['id']!=self.active:raise ManualError('session_expired','Cette session n’est plus active.',410)
        try:return self.pool.submit(self._read_current,record).result(timeout=12)
        except FutureTimeout:return {'status':'error','links':[],'message':'La page met trop de temps à répondre. Réessayez.'}
    def _read_current(self,record):
        from playwright.sync_api import TimeoutError as PlaywrightTimeout
        with self.lock:
            if time.time()>=record['expiresAt']:return {'status':'session_expired','links':[],'message':'La session a expiré. Relancez la vérification.'}
        try:
            self._same_game(record)
            # This parser reads the existing page. It neither navigates nor clicks Magic Cookie.
            result=extract_current_links(self.page,record['source'],timeout=3000)
            result.update(source=record['source'],provider='EmuGen',slug=record['slug'],sessionId=record['id'])
            with self.lock:record['result']=result
            return result
        except PlaywrightTimeout:
            visible=self.page.locator('#emugen-download-section').is_visible()
            pending=not visible and (self.page.locator('#emugen-cookie-btn,.cf-turnstile,#challenge-running,#challenge-stage,iframe[src*="challenges.cloudflare.com"]').count()>0 or any(word in self.page.title().lower() for word in ('just a moment','verify','vérifi','cloudflare')))
            return {'status':'challenge_required' if pending else 'error','links':[],'source':record['source'],'message':'La vérification ou le clic Magic Cookie reste à terminer dans cette fenêtre.' if pending else 'La section Téléchargements ne finit pas de charger. Réessayez.'}
        except Exception:return {'status':'error','links':[],'source':record['source'],'message':'Impossible de lire la page ouverte. Réessayez ou relancez la vérification.'}
    def viewer_endpoint(self,username,session_id):
        with self.lock:
            record=self._owned(username,session_id)
            if record['status'] in ('expired','closed'):raise ManualError('session_expired','Cette session a expiré.',410)
            if record['status']!='ready' or record['id']!=self.active:raise ManualError('error','Le navigateur n’est pas prêt.',409)
            return ('127.0.0.1',VNC_PORT)
    def deactivate(self):
        # Only called by the authenticated administrator provider-settings route.
        with self.lock:
            record=self.records.get(self.active)
            if not record:return
            record['status']='closed';record.pop('result',None)
        self.pool.submit(self._close_record,record['id'])
    def close_session(self,username,session_id):
        with self.lock:
            record=self._owned(username,session_id);starting=record['status']=='starting';record['status']='closed';record.pop('result',None)
        future=self.pool.submit(self._close_record,record['id'])
        if not starting:
            try:future.result(timeout=15)
            except FutureTimeout:pass
        return {'status':'closed'}
    def _close_record(self,session_id):
        with self.lock:
            if self.active!=session_id:return
            self.records[session_id]['status']='expired' if time.time()>=self.records[session_id]['expiresAt'] else 'closed'
            self.active=None
        self._cleanup_resources()
    def _cleanup_resources(self):
        for obj in (self.context,self.browser,self.driver):
            if obj:
                try:obj.stop() if obj is self.driver else obj.close()
                except Exception:pass
        self.page=self.context=self.browser=self.driver=None
        for proc in (self.vnc,self.desktop):
            if proc:
                try:proc.terminate();proc.wait(timeout=3)
                except Exception:
                    try:proc.kill();proc.wait(timeout=2)
                    except Exception:pass
        self.vnc=self.desktop=None
        if self.temp:shutil.rmtree(self.temp,ignore_errors=True);self.temp=None
    def _reaper(self):
        while not self.stop.wait(1):
            with self.lock:
                record=self.records.get(self.active)
                expired=record and (time.time()>=record['expiresAt'] or record['status'] in ('closed','expired','error'))
                if expired and (self.reap_future is None or self.reap_future.done()):self.reap_future=self.pool.submit(self._close_record,record['id'])
    def close(self):
        self.stop.set();self.pool.submit(self._cleanup_resources).result(timeout=20);self.pool.shutdown(wait=True)

MANUAL=ManualVerification()
