#!/usr/bin/env python3
"""Import reviewed console photographs, retaining individual licenses and credits."""
import argparse
import importlib.util
import io
import json
from pathlib import Path
import re
import urllib.parse

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('artwork',Path(__file__).with_name('artwork.py'))
art=importlib.util.module_from_spec(spec);spec.loader.exec_module(art)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--selection',type=Path,default=ROOT/'deploy/pi-tv/console-photos.json')
    p.add_argument('--cache',type=Path,default=ROOT/'.local/pi-tv/media')
    p.add_argument('--output',type=Path,default=ROOT/'output/console-photos')
    p.add_argument('--allow-noncommercial',action='store_true',help='Allow the explicitly listed CC BY-NC-SA photo for this personal deployment')
    a=p.parse_args()
    from PIL import Image,ImageOps
    selection=json.loads(a.selection.read_text())
    mapping=selection['consoles']
    if selection.get('external') and not a.allow_noncommercial:p.error('The CPS-2 photograph requires non-commercial use; read its license and pass --allow-noncommercial.')
    params={'action':'query','format':'json','titles':'|'.join('File:'+f for f in mapping.values()),'prop':'imageinfo','iiprop':'url|extmetadata','iiurlwidth':800,'maxlag':5}
    data=art.get_json('https://commons.wikimedia.org/w/api.php?'+urllib.parse.urlencode(params))
    if 'error' in data:raise ValueError('Wikimedia unavailable')
    pages={p['title']:p for p in data.get('query',{}).get('pages',{}).values()}
    a.output.mkdir(parents=True,exist_ok=True)
    results=[]
    for target,file in mapping.items():
        page=pages.get('File:'+file)
        if not page or not page.get('imageinfo'):raise ValueError('Image absent: '+target)
        info=page['imageinfo'][0];meta=info.get('extmetadata',{})
        get=lambda key:art.text(meta.get(key,{}).get('value',''))
        license_name=get('LicenseShortName')
        if not re.fullmatch(r'(CC BY(?:-SA)? (?:1\.0|2\.0|2\.5|3\.0|4\.0)|CC0|Public domain)',license_name):raise ValueError('License not approved: '+target)
        if not get('Artist'):raise ValueError('Missing author: '+target)
        if license_name.startswith('CC BY') and not get('LicenseUrl'):raise ValueError('Missing license URL: '+target)
        existing=a.output/(target+'.json')
        if existing.exists():
            cached=json.loads(existing.read_text())
            if cached.get('source')==info['descriptionurl'] and cached.get('license')==license_name and (a.cache/(cached['id']+'.img')).is_file():
                results.append(cached);print(target+': cached',flush=True);continue
        url=info.get('thumburl') or info['url']
        body,mime=art.fetch(url)
        if mime not in ['image/jpeg','image/png','image/webp']:raise ValueError('Not a raster photograph: '+target)
        with Image.open(io.BytesIO(body)) as original:
            image=ImageOps.exif_transpose(original).convert('RGB');image.thumbnail((960,960))
            out=io.BytesIO();image.save(out,format='WEBP',quality=86,method=6)
        record={'provider':'Wikimedia Commons','title':page['title'],'author':get('Artist'),'license':license_name,'licenseUrl':get('LicenseUrl') or 'https://creativecommons.org/publicdomain/mark/1.0/','source':info['descriptionurl'],'sourceImage':url,'attribution':get('Artist'),'kind':'console-photo','approved':True,'target':target,'modifications':'Redimensionnement et conversion WebP ; aucun recadrage.'}
        record=art.cache_bytes(a.cache,out.getvalue(),'image/webp',record)
        (a.output/(target+'.webp')).write_bytes(out.getvalue())
        (a.output/(target+'.json')).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n')
        results.append(record);print(target+': '+license_name,flush=True)
    for external in selection.get('external',[]):
        if external.get('license')!='CC BY-NC-SA 2.0' or not external.get('target'):raise ValueError('Unapproved external license')
        html,mime=art.fetch(external['source'])
        if external['licenseUrl'].encode() not in html:raise ValueError('Original source no longer confirms the license')
        body,mime=art.fetch(external['sourceImage'])
        if mime not in ['image/jpeg','image/png','image/webp']:raise ValueError('Not a photograph')
        with Image.open(io.BytesIO(body)) as original:
            image=ImageOps.exif_transpose(original).convert('RGB');image.thumbnail((960,960))
            out=io.BytesIO();image.save(out,format='WEBP',quality=86,method=6)
        record=art.cache_bytes(a.cache,out.getvalue(),'image/webp',{**external,'approved':True,'kind':'console-photo','attribution':external['author'],'modifications':'Redimensionnement et conversion WebP ; aucun recadrage.'})
        target=external['target'];(a.output/(target+'.webp')).write_bytes(out.getvalue())
        (a.output/(target+'.json')).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n')
        results.append(record);print(target+': '+record['license'],flush=True)
    (a.output/'credits.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n')
    print(str(len(results))+' photos imported',flush=True)
if __name__=='__main__':main()
