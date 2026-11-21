import base64
import json
from pathlib import Path
import sys
import tempfile
import unittest
from io import BytesIO
from types import SimpleNamespace
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import covers
import server
class CoversTests(unittest.TestCase):
    def test_exact_and_edition_match_without_wrong_game(self):
        with tempfile.TemporaryDirectory() as folder:
            index=Path(folder)/'cover-index';index.mkdir()
            (index/'nes.json').write_text(json.dumps(['Mario Bros. (USA).png','Mario Bros. (Japan).png','Super Mario Bros. (USA).png','../Mario Bros.png']))
            self.assertEqual(covers.filename(folder,'nes','Mario Bros. (Japan)'), 'Mario Bros. (Japan).png')
            self.assertEqual(covers.filename(folder,'nes','Mario Bros. (USA) (Beta)'), 'Mario Bros. (USA).png')
            self.assertIsNone(covers.filename(folder,'nes','Mario Kart (USA)'))

    def test_ds_scene_names_and_localised_titles_find_canonical_covers(self):
        with tempfile.TemporaryDirectory() as folder:
            index=Path(folder)/'cover-index';index.mkdir()
            names=[
                'N (Japan).png',
                'WarioWare - Touched! (Europe) (En,Fr,De,Es,It).png',
                'Super Mario 64 DS (Europe) (En,Fr,De,Es,It).png',
                'Nintendo Touch Golf - Birdie Challenge! (Europe) (En,Fr,De,Es,It).png',
                'Disney Princess - Magical Jewels (Europe) (En,Fr,De,Es,It,Nl).png',
                'Imagine - Babies (Europe) (En,Fr,De,Es,It,Nl).png',
                'Nintendogs - Labrador _ Friends (Europe) (En,Fr,De,Es,It).png',
                'Rub Rabbits!, The (Europe) (En,Ja,Fr,De,Es,It).png',
                'New Super Mario Bros. (Europe) (En,Fr,De,Es,It).png',
                'Rainbow Islands Revolution (Europe).png',
            ]
            (index/'nds.json').write_text(json.dumps(names))
            self.assertEqual(covers.filename(folder,'nds',"0018 - WarioWare - Touched! (E)(Wet 'N' Wild)"),names[1])
            self.assertEqual(covers.filename(folder,'nds','0022 Super Mario 64 DS (EU)(M5)'),names[2])
            self.assertEqual(covers.filename(folder,'nds','0203 - Nintendo Touch Golf (E)'),names[3])
            self.assertEqual(covers.filename(folder,'nds','(NDS)Disney Princesse Magique-Jeux Ds'),names[4])
            self.assertEqual(covers.filename(folder,'nds','01 Léa Passion Bébés'),names[5])
            self.assertEqual(covers.filename(folder,'nds','0102 - Nintendogs - Labrador & Friends (E)'),names[6])
            self.assertEqual(covers.filename(folder,'nds','0328 _ The Rub Rabbits (E)'),names[7])
            self.assertEqual(covers.filename(folder,'nds','0476 - New Super Mario Brothers (E)(Supremacy)'),names[8])
            self.assertEqual(covers.filename(folder,'nds','0393_  Rainbow Island Revolution (E)'),names[9])
    def test_no_arbitrary_url_or_file_fetch(self):
        for url in ['http://raw.githubusercontent.com/test','https://evil.test/cover.png','https://raw.githubusercontent.com/other/rom.zip']:
            with self.assertRaises(ValueError):covers.fetch(url,100)
        for system,encoded in [('other','AAAA'),('nes','../'),('nes',base64.urlsafe_b64encode(b'\x00').decode().rstrip('='))]:
            with self.assertRaises(ValueError):covers.resolve('/unused',system,encoded)
    def test_artwork_system_is_specific(self):
        item={'title':'Metroid (Japan)','path':'/rom/nintendo-family-computer-disk-system-fds/metroid'}
        self.assertIn('/covers/fds/',covers.image_url('nes',item))
        self.assertIn('/covers/gbc-color/',covers.image_url('gbc',{'title':'Test','path':'/rom/nintendo-game-boy-color/test'}))

    def test_search_covers_every_configured_console_without_fetching(self):
        items=[
            {'consoleId':'n64','reference':'banjo','title':'Banjo-Tooie (Europe) (En,Fr,De,Es)'},
            {'consoleId':'gbc','reference':'zelda','title':'Zelda','files':['zelda.gbc']},
            {'consoleId':'cps1','reference':'sf2','title':'Street Fighter II.zip'},
            {'consoleId':'doom','reference':'doom','title':'Doom II.wad'},
            {'consoleId':'unknown','reference':'nope','title':'Nope'},
        ]
        with tempfile.TemporaryDirectory() as folder:
            result=covers.search(folder,items,prefetch_images=False)
        self.assertEqual([item['reference'] for item in result['items']],['banjo','zelda','sf2','doom'])
        self.assertIn('/covers/n64/',result['items'][0]['image'])
        self.assertIn('/covers/gbc-color/',result['items'][1]['image'])
        self.assertIn('/covers/cps1/',result['items'][2]['image'])
        self.assertIn('/covers/doom/',result['items'][3]['image'])
        self.assertTrue({'cps1','cps2','doom','neogeo'}.issubset(result['supportedSystems']))

    def test_search_rejects_oversized_or_malformed_payload(self):
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(ValueError):covers.search(folder,[{}]*2001,prefetch_images=False)
            with self.assertRaises(ValueError):covers.search(folder,'not a list',prefetch_images=False)

    def request_search(self,identity,payload):
        raw=json.dumps(payload).encode()
        handler=object.__new__(server.Handler);handler.path='/v1/covers/search'
        handler.server=SimpleNamespace(data=Path('/covers'),auth_base='http://auth')
        handler.headers={'Content-Length':str(len(raw))};handler.rfile=BytesIO(raw);handler.wfile=BytesIO()
        codes=[];handler.send_response=codes.append;handler.send_header=lambda *args:None;handler.end_headers=lambda:None
        with patch.object(server,'verified_identity',return_value=identity):handler.do_POST()
        return codes[0],json.loads(handler.wfile.getvalue())

    def test_search_endpoint_requires_authentication_and_validates_payload(self):
        item={'consoleId':'n64','reference':'banjo','title':'Banjo-Tooie (Europe)'}
        self.assertEqual(self.request_search(None,{'items':[item]})[0],401)
        expected={'items':[dict(item,image='/experience-api/v1/covers/n64/example')],'supportedSystems':['n64']}
        with patch.object(covers,'search',return_value=expected) as search:
            code,result=self.request_search({'username':'member','role_level':1},{'items':[item]})
        self.assertEqual((code,result),(200,expected));search.assert_called_once_with(Path('/covers'),[item])
        self.assertEqual(self.request_search({'username':'member'},{'items':'invalid'})[0],400)

    def test_provider_status_does_not_break_exact_cover(self):
        with tempfile.TemporaryDirectory() as folder:
            index=Path(folder)/'cover-index';index.mkdir()
            (index/'gba.json').write_text(json.dumps(['Pac-Man Collection (Europe).png']))
            self.assertEqual(covers.filename(folder,'gba','Pac-Man Collection (Europe) (Virtual Console) Retirée'),'Pac-Man Collection (Europe).png')
            self.assertEqual(covers.clean_title('Retired Games'),'Retired Games')
    def test_peek_never_fetches_missing_index(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as folder,patch.object(covers,'fetch',side_effect=AssertionError('Network forbidden')):
            title=base64.urlsafe_b64encode(b'Pac-Man Collection').decode().rstrip('=')
            self.assertIsNone(covers.peek(folder,'gba',title))
    def test_thumbnail_is_small_decodable_webp(self):
        from io import BytesIO
        from PIL import Image
        source=BytesIO();Image.new('RGB',(1000,1500),'red').save(source,'PNG')
        body=covers.optimize_image(source.getvalue(),'image/png')
        with Image.open(BytesIO(body)) as image:
            self.assertEqual(image.format,'WEBP');self.assertLessEqual(image.width,384);self.assertLessEqual(image.height,512)
        self.assertLess(len(body),10000)
    def test_cached_cover_does_not_wait_for_stale_index_refresh(self):
        import hashlib,threading,time,os
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder);index=root/'cover-index';index.mkdir();media=root/'media';media.mkdir()
            name='Demo (USA).png';source=index/'nes.json';source.write_text(json.dumps([name]));os.utime(source,(time.time()-8*86400,)*2)
            digest=hashlib.sha256(b'cached image').hexdigest();(media/(digest+'.img')).write_bytes(b'cached image')
            record={'id':digest,'url':'/experience-api/v1/media/'+digest,'reviewAfter':time.time()+3600,'coverVersion':covers.COVER_VERSION}
            covers.mapping_path(folder,'nes',name).write_text(json.dumps(record))
            acquired=threading.Event();release=threading.Event()
            def hold():
                with covers.INDEX_LOCKS['nes']:acquired.set();release.wait(1)
            thread=threading.Thread(target=hold);thread.start();acquired.wait(1)
            try:
                with patch.object(covers,'fetch',side_effect=AssertionError('Cache must not fetch')):
                    start=time.monotonic();result=covers.resolve(folder,'nes',base64.urlsafe_b64encode(b'Demo (USA)').decode().rstrip('='));elapsed=time.monotonic()-start
                self.assertEqual(result['id'],digest);self.assertLess(elapsed,.4)
            finally:release.set();thread.join()
    def test_failed_source_is_not_requested_repeatedly(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as folder:
            index=Path(folder)/'cover-index';index.mkdir();(index/'nes.json').write_text(json.dumps(['Demo (USA).png']))
            encoded=base64.urlsafe_b64encode(b'Demo (USA)').decode().rstrip('=')
            with patch.object(covers,'fetch',side_effect=OSError('offline')) as fetch:
                with self.assertRaises(OSError):covers.resolve(folder,'nes',encoded)
                self.assertIsNone(covers.resolve(folder,'nes',encoded));self.assertEqual(fetch.call_count,1)
