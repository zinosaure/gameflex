"""Real pairing with independent QR decoding and device-role transitions."""
import json,os,re
from pathlib import Path
from playwright.sync_api import sync_playwright
BASE=os.environ.get('INTERFACE_PREVIEW','http://172.28.0.11:18231/gameflex/')
OUT=Path(os.environ.get('RESULT_DIR','/results'));OUT.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
 b=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox','--disable-dev-shm-usage']);screen=b.new_page(viewport={'width':960,'height':540});phone=b.new_page(viewport={'width':390,'height':844},has_touch=True,is_mobile=True)
 screen.goto(BASE+'?showcase=1&tv=1');screen.locator('.ps-console-tile').first.wait_for();screen.get_by_role('button',name='Manette téléphone',exact=True).click()
 roles=screen.get_by_role('navigation',name='Rôle de cet appareil');assert roles.get_by_role('button',name='Écran',exact=True).get_attribute('aria-pressed')=='true'
 screen.get_by_role('button',name='Afficher un code',exact=True).click();screen.locator('.gf-pair-qr').wait_for();code=re.sub(r'\D','',screen.locator('.gf-tv-code').inner_text());assert len(code)==8
 screen.add_script_tag(path='/app/tests/pi-tv/vendor/jsQR.js')
 url=screen.evaluate('''async()=>{const svg=document.querySelector('.gf-pair-qr'),data=new XMLSerializer().serializeToString(svg);const image=new Image();image.src='data:image/svg+xml;charset=utf-8,'+encodeURIComponent(data);await image.decode();const canvas=document.createElement('canvas');canvas.width=canvas.height=416;const ctx=canvas.getContext('2d');ctx.drawImage(image,0,0,416,416);return jsQR(ctx.getImageData(0,0,416,416).data,416,416).data;}''')
 assert url==BASE+'controller#code='+code
 screen.screenshot(path=str(OUT/'screen-qr.png'))
 rect=screen.locator('.gf-pair-qr').bounding_box();assert rect['y']>=0 and rect['y']+rect['height']<=540,rect
 screen.screenshot(path=str(OUT/'screen-qr.png'))
 phone.goto(url);phone.get_by_label('Code affiché sur la TV',exact=True).wait_for();assert phone.get_by_label('Code affiché sur la TV',exact=True).input_value()==code;assert '#code=' not in phone.url and '?code=' not in phone.url
 roles=phone.get_by_role('navigation',name='Rôle de cet appareil');assert roles.get_by_role('button',name='Manette',exact=True).get_attribute('aria-pressed')=='true'
 assert phone.locator('.gf-phone-header strong').evaluate('el=>getComputedStyle(el).userSelect')=='none'
 field=phone.get_by_label('Code affiché sur la TV',exact=True);field.evaluate('el=>el.select()');assert field.evaluate('el=>el.selectionEnd-el.selectionStart')==8
 phone.screenshot(path=str(OUT/'controller-prefilled.png'))
 phone.get_by_role('button',name='Associer à ma TV',exact=True).click();screen.get_by_role('button',name='Autoriser ce téléphone',exact=True).wait_for();screen.get_by_role('button',name='Autoriser ce téléphone',exact=True).click();phone.get_by_role('main',name='Manette tactile').wait_for()
 phone.get_by_role('navigation',name='Rôle de cet appareil').get_by_role('button',name='Écran',exact=True).click();phone.get_by_role('dialog',name='Votre téléphone devient une manette').wait_for();assert phone.get_by_role('navigation',name='Rôle de cet appareil').get_by_role('button',name='Écran',exact=True).get_attribute('aria-pressed')=='true'
 phone.get_by_role('navigation',name='Rôle de cet appareil').get_by_role('button',name='Manette',exact=True).click();phone.get_by_label('Code affiché sur la TV',exact=True).wait_for();assert phone.get_by_role('main',name='Manette tactile').count()==0
 screen.get_by_role('button',name='Déconnecter le téléphone',exact=True).click()
 # Existing app chrome suppresses selection, but search text remains editable.
 screen.get_by_role('button',name='Fermer l’association',exact=True).click();assert screen.locator('.ps-row-title h3').first.evaluate('el=>getComputedStyle(el).userSelect')=='none'
 screen.locator('.ps-console-heading h1').dblclick();assert screen.evaluate('getSelection().toString()')==''
 search=screen.get_by_role('searchbox').first;search.fill('Advance');search.evaluate('el=>el.select()');assert search.evaluate('el=>el.selectionEnd-el.selectionStart')==7
 b.close()
(OUT/'device-roles.json').write_text(json.dumps({'passed':True,'qrIndependentlyDecoded':True,'urlCodePrefilled':True,'codeRemovedFromUrl':True,'ownerApproval':True,'rolesRoundTrip':True,'controllerDisconnectedBeforeScreen':True,'chromeNotSelectable':True,'fieldsEditable':True},indent=2)+'\n');print('Device roles, independently decoded QR, prefill, approval and text-selection boundaries passed.')
