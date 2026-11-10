"""Complete 22-appearance flows against the isolated fixture transport only."""
import hashlib,json,os,sys,re
from urllib.parse import urlparse,parse_qs
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://127.0.0.1:18197/gameflex/')
FORM=os.environ.get('INTERFACE_FORMAT','pc')
MODES=[('station','Horizon'),('crt','Signal CRT'),('shelf','La collection'),('orbit','Orbitale'),('arcade','Borne Arcade'),('minimal','Minimal'),('covers','Jaquettes'),('consoles','Consoles'),('dashboard','Tableau de bord'),('hub-canape','Hub Canapé'),('ligne-cinema','Ligne Cinéma'),('mosaique-joy','Mosaïque Joy'),('roue-retro','Roue Retro'),('terminal-poche','Terminal de poche'),('cartouches','Cartouches'),('carte-epoques','Carte des époques'),('double-ecran','Double écran'),('club-local','Club Local'),('quai-systemes','Quai des systèmes'),('magazine-retro','Magazine rétro'),('bureau-collectionneur','Bureau du collectionneur'),('grand-ecran-accessible','Grand écran accessible')]
ONLY=os.environ.get('INTERFACE_ONLY')
VIEWPORT={'pc':{'width':1440,'height':1000},'mobile':{'width':390,'height':844},'tv':{'width':1920,'height':1080}}[FORM]

def action(node):
    if FORM=='mobile':node.tap()
    else:node.focus();node.press('Enter')

def go(page,destination):
    names={'home':['Accueil','Accueil des consoles'],'library':['Mes jeux'],'management':['Gestion / Archives'],'settings':['Paramètres'],'favorites':['Favoris']}.get(destination,[destination])
    for label in names:
        nodes=page.get_by_role('button',name=label,exact=True)
        if nodes.count():action(nodes.first);return
    menu=page.get_by_role('button',name='Menu principal',exact=True)
    if not menu.count():menu=page.get_by_role('button',name=re.compile(r'^(Tous les menus|Plus de menus|Plus)$'))
    action(menu.first)
    action(page.get_by_role('dialog').get_by_role('button',name=names[0],exact=True))

def ready(page,mode):
    page.locator(f'[data-appearance="{mode}"]').wait_for()
    assert page.get_by_text('Affichage interrompu',exact=True).count()==0

def theme(page):
    return page.locator('[data-appearance]').evaluate('el=>({palette:getComputedStyle(el).getPropertyValue("--dialog-bg").trim(),background:getComputedStyle(el).backgroundColor,common:document.querySelector(".gx-app")?getComputedStyle(document.querySelector(".gx-app")).backgroundColor:null})')

rows=[]
with sync_playwright() as p:
    browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
    for mode,name in MODES:
        if ONLY and mode not in ONLY.split(','):continue
        context=browser.new_context(viewport=VIEWPORT,has_touch=FORM=='mobile',locale='fr-FR');page=context.new_page();errors=[]
        page.set_default_timeout(12000)
        page.on('pageerror',lambda e:errors.append(str(e)))
        try:
            page.goto(BASE+'settings?saved=station&objectStatus=1',wait_until='domcontentloaded')
            opener=page.locator('.gx-settings').get_by_role('button',name='Changer d’interface',exact=True)
            opener.wait_for();opener.evaluate('el=>el.dataset.testid="selector-opener"');action(opener)
            page.get_by_role('dialog',name='Choix de l’interface').wait_for();page.keyboard.press('ArrowRight');page.keyboard.press('Escape')
            page.wait_for_function('document.activeElement.dataset.testid==="selector-opener"')
            assert urlparse(page.url).path.endswith('/settings'),'Cancel changed the destination'
            action(opener)
            action(page.locator('.gx-choice').filter(has=page.get_by_text(name,exact=True)))
            ready(page,mode);assert urlparse(page.url).path.endswith('/settings'),'Selection changed the destination'
            go(page,'home');ready(page,mode)
            page.locator(f'[data-interface="{mode}"]').first.wait_for()
            page.wait_for_timeout(600)
            signature=page.locator(f'[data-interface="{mode}"]').first.evaluate('el=>{const shape=n=>[n.tagName,...[...n.children].filter(c=>c.tagName.toLowerCase()!=="svg").map(shape)];return JSON.stringify(shape(el))}')
            go(page,'library');page.wait_for_function('document.querySelector(".em-installed-total")&&!document.querySelector(".em-installed-total").textContent.includes("Chargement")')
            ready(page,mode);colors=theme(page);assert colors['common']==colors['background'],colors
            search=page.get_by_role('searchbox');search.fill('Advance Wars')
            page.get_by_text('2 résultats',exact=False).first.wait_for()
            card=page.locator('[data-focus-key="gba:gba-0"]').first;action(card)
            dialog=page.get_by_role('dialog');dialog.get_by_role('heading',name='Advance Wars',exact=True).wait_for()
            assert dialog.get_by_text('Anglais / Français',exact=True).count()==1
            assert dialog.locator('select').count()==0,'Single edition must be simple'
            assert page.locator(f'[data-interface-theme="{mode}"]').count()==1
            action(dialog.get_by_role('button',name='Retour',exact=True))
            page.wait_for_function('document.activeElement.dataset.focusKey==="gba:gba-0"')
            assert search.input_value()=='Advance Wars','Sheet return lost the filter'
            search.fill('');go(page,'management');page.get_by_role('heading',name='Gestion / Archives',exact=True,level=1).wait_for();ready(page,mode)
            assert page.get_by_role('option',name='Jouable',exact=True).count()==1
            go(page,'settings');ready(page,mode);page.reload(wait_until='domcontentloaded');ready(page,mode)
            page.locator('.gx-settings').wait_for()
            assert theme(page)['palette']==colors['palette'],'Reload changed palette'
            # The actual advanced Settings component is rendered in the fixture.
            page.get_by_role('link',name='Compte et bibliothèque ↗',exact=True).click()
            page.locator('.gf-advanced-content').wait_for();ready(page,mode)
            assert urlparse(page.url).path.endswith('/settings') and parse_qs(urlparse(page.url).query)['advanced']==['1']
            assert page.get_by_role('link',name='Compte et bibliothèque ↗',exact=True).count()==0
            assert page.get_by_text('Paramètres généraux',exact=True).count()>0
            assert theme(page)['common']==theme(page)['background']
            overflow=page.evaluate('document.documentElement.scrollWidth-innerWidth')
            assert overflow<=2,f'Overflow {overflow}'
            assert not errors,errors
            row={'id':mode,'name':name,'format':FORM,'flow':'settings → activation → home → library/search → sheet → return/filter/focus → archives → settings → reload → advanced settings','passed':True,'theme':colors['palette'],'overflow':overflow,'compositionFingerprint':hashlib.sha256(signature.encode()).hexdigest()[:16]}
            rows.append(row);print(f'{FORM} passed {mode}',file=sys.stderr,flush=True)
        except Exception as e:
            print(json.dumps({'failedMode':mode,'format':FORM,'url':page.url,'error':str(e),'text':page.locator('body').inner_text()[:1400],'errors':errors},ensure_ascii=False),file=sys.stderr,flush=True)
            raise
        finally:context.close()
    browser.close()
assert len({row['compositionFingerprint'] for row in rows})==len(rows),'Home compositions have converged'
print(json.dumps(rows,ensure_ascii=False,indent=2))
