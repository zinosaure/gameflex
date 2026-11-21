"""One lazy, reusable ARM64 Chromium worker. Only the website creates its Magic Cookie."""
from concurrent.futures import ThreadPoolExecutor,TimeoutError as FutureTimeout
import hashlib
import ipaddress
import os
import socket
import threading
import time
from urllib.parse import urlsplit
from emugen import HOSTS,ORIGIN,EmugenError,valid_url,game_url,folder
from romspure import read_cache,save_cache

FINAL_STATE_JS=r'''() => {
 const section=document.querySelector('#emugen-download-section');
 if(!section || !section.getClientRects().length || getComputedStyle(section).visibility==='hidden') return false;
 if(/no downloads available at this time|aucun (?:lien|téléchargement)(?: n.est)? disponible|pas de téléchargement disponible/i.test(section.innerText))return 'empty';
 const links=[...section.querySelectorAll('a[href]')].filter(a=>a.getClientRects().length&&getComputedStyle(a).visibility!=='hidden'&&['https:','http:'].includes(a.protocol)&&!a.username&&!a.password&&(!a.port||['80','443'].includes(a.port)));
 if(links.length)return 'links';
 return false;
}'''
LINKS_JS=r'''links => links.filter(a=>a.getClientRects().length&&getComputedStyle(a).visibility!=='hidden'&&['https:','http:'].includes(a.protocol)).map(a=>({label:a.innerText.trim()||a.hostname,url:a.href}))'''

def read_page(page,url,timeout=15000):
    valid_url(url,'game')
    response=page.goto(url,wait_until='domcontentloaded',timeout=20000)
    valid_url(page.url,'game')
    if response and response.status==404 or page.locator('body.error404').count():return {'status':'not_found','links':[],'message':'Fiche EmuGen introuvable.'}
    if response and response.status in (403,503) and page.locator('.cf-turnstile, #challenge-running, #challenge-stage, iframe[src*="challenges.cloudflare.com"]').count():return {'status':'challenge_required','links':[],'message':'EmuGen demande une vérification manuelle.'}
    if response and response.status>=400:return {'status':'error','links':[],'message':'Impossible de charger la fiche EmuGen.'}
    section=page.locator('#emugen-download-section')
    if not section.is_visible():
        button=page.locator('#emugen-cookie-btn')
        if button.count()==0:return {'status':'error','links':[],'message':'La fiche ne contient pas la section attendue.'}
        if page.locator('.cf-turnstile').count():
            try:page.wait_for_function("() => !!document.querySelector('[name=cf-turnstile-response]')?.value",timeout=min(10000,timeout))
            except Exception:return {'status':'challenge_required','links':[],'message':'EmuGen demande une vérification dans son navigateur. Ouvrez sa fiche pour continuer.'}
        # A real Playwright click creates a trusted DOM event. Never set cookies/tokens ourselves.
        try:button.click(timeout=min(10000,timeout))
        except Exception:
            if page.locator('.cf-turnstile,iframe[src*="challenges.cloudflare.com"]').count():
                return {'status':'challenge_required','links':[],'message':'EmuGen demande une vérification dans son navigateur. Ouvrez sa fiche pour continuer.'}
            raise
    return extract_current_links(page,url,timeout)

def extract_current_links(page,url,timeout=15000):
    valid_url(page.url,'game')
    section=page.locator('#emugen-download-section')
    final_state=page.wait_for_function(FINAL_STATE_JS,timeout=timeout).json_value()
    valid_url(page.url,'game')
    if final_state=='empty':return {'status':'empty','links':[],'message':'Aucun téléchargement disponible sur cette fiche EmuGen.'}
    links=section.locator('a[href]:visible').evaluate_all(LINKS_JS)
    safe=[]
    for link in links[:100]:
        try:u=urlsplit(link.get('url',''));port=u.port
        except (ValueError,TypeError):continue
        if u.scheme not in ('http','https') or not u.hostname or u.username or u.password or port not in (None,80,443):continue
        safe.append({'label':str(link.get('label') or u.hostname)[:200],'url':link['url'],'provider':'EmuGen'})
    return {'status':'links' if safe else 'error','links':safe,'message':'' if safe else 'Les liens EmuGen ne sont pas exploitables. Réessayez ; aucune absence de téléchargement n’a été enregistrée.'}

class BrowserResolver:
    def __init__(self):
        self.pool=ThreadPoolExecutor(max_workers=1,thread_name_prefix='emugen-chromium');self.capacity=threading.BoundedSemaphore(3)
        self.driver=None;self.browser=None;self.context=None;self.pinned={}
    def _start(self):
        from playwright.sync_api import sync_playwright
        if self.browser and self.browser.is_connected():return
        if self.driver:
            try:self.driver.stop()
            except Exception:pass
        hosts=HOSTS|{'challenges.cloudflare.com'};rules=[];self.pinned={}
        for host in hosts:
            rows=socket.getaddrinfo(host,443,type=socket.SOCK_STREAM)
            if not rows or any(not ipaddress.ip_address(r[4][0]).is_global or ipaddress.ip_address(r[4][0]).is_multicast or ipaddress.ip_address(r[4][0]).is_reserved for r in rows):raise EmugenError('blocked_navigation','Destination du navigateur refusée.')
            addresses=[r[4][0] for r in rows if r[0]==socket.AF_INET] or [rows[0][4][0]]
            self.pinned[host]=addresses[0];rules.append('MAP '+host+' '+addresses[0])
        self.driver=sync_playwright().start()
        self.browser=self.driver.chromium.launch(executable_path=os.environ.get('GAMEFLEX_CHROMIUM_EXECUTABLE','/usr/bin/chromium'),headless=True,args=['--disable-dev-shm-usage','--disable-background-networking','--disable-gpu','--renderer-process-limit=2','--host-resolver-rules='+','.join(rules)])
        self.context=self.browser.new_context(accept_downloads=False,service_workers='block',locale='fr-FR')
        self.context.route('**/*',self._route)
    def _route(self,route):
        request=route.request
        try:
            u=urlsplit(request.url)
            if u.scheme not in ('https',) or u.username or u.password or u.port not in (None,443):return route.abort()
            is_main=request.is_navigation_request() and request.frame==request.frame.page.main_frame
            if is_main:
                valid_url(request.url,'game')
            elif u.hostname not in self.pinned:return route.abort()
            if request.resource_type in ('image','font','media') or any(u.path.lower().endswith('.'+s) for s in ('zip','7z','rar','iso','wbfs','nes','gba','gbc','bin','exe')):return route.abort()
            return route.continue_()
        except Exception:return route.abort()
    def _resolve(self,url):
        from playwright.sync_api import TimeoutError as PlaywrightTimeout
        page=None
        try:
            self._start();page=self.context.new_page();page.on('dialog',lambda dialog:dialog.dismiss());page.on('popup',lambda popup:popup.close())
            result=read_page(page,url)
            return {**result,'source':url,'provider':'EmuGen'}
        except PlaywrightTimeout:return {'status':'timeout','links':[],'source':url,'provider':'EmuGen','message':'Délai EmuGen dépassé. Réessayez ; aucune absence de lien n’a été enregistrée.'}
        except EmugenError as e:return {'status':e.status,'links':[],'source':url,'provider':'EmuGen','message':str(e)}
        except Exception:return {'status':'error','links':[],'source':url,'provider':'EmuGen','message':'Chargement EmuGen impossible. Réessayez.'}
        finally:
            if page:
                try:page.close()
                except Exception:pass
    def resolve(self,data,slug):
        url=game_url(slug);key=hashlib.sha256(url.encode()).hexdigest();path=folder(data)/('links-'+key+'.json')
        ready=read_cache(path,300)
        if ready and ready.get('status') in ('links','empty'):return {**ready,'cached':True}
        if not self.capacity.acquire(blocking=False):return {'status':'busy','links':[],'source':url,'message':'Une résolution est en cours. Réessayez dans un instant.'}
        future=self.pool.submit(self._resolve,url)
        # Release when the actual browser task ends, even if an HTTP caller times out first.
        future.add_done_callback(lambda _:self.capacity.release())
        try:result=future.result(timeout=55)
        except FutureTimeout:return {'status':'timeout','links':[],'source':url,'message':'Résolution EmuGen trop longue ; réessayez.'}
        if result['status'] in ('links','empty'):save_cache(path,result)
        return result
    def release_browser(self):
        def release():
            if self.context:self.context.close()
            if self.browser:self.browser.close()
            if self.driver:self.driver.stop()
            self.context=self.browser=self.driver=None
        self.pool.submit(release).result(timeout=120)
    def close(self):
        def stop():
            if self.context:self.context.close()
            if self.browser:self.browser.close()
            if self.driver:self.driver.stop()
            self.context=self.browser=self.driver=None
        self.pool.submit(stop).result(timeout=15);self.pool.shutdown(wait=True)

RESOLVER=BrowserResolver()
