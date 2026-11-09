import base64
import json
from pathlib import Path
import sys
import tempfile
import unittest
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import covers
class CoversTests(unittest.TestCase):
    def test_exact_and_edition_match_without_wrong_game(self):
        with tempfile.TemporaryDirectory() as folder:
            index=Path(folder)/'cover-index';index.mkdir()
            (index/'nes.json').write_text(json.dumps(['Mario Bros. (USA).png','Mario Bros. (Japan).png','Super Mario Bros. (USA).png','../Mario Bros.png']))
            self.assertEqual(covers.filename(folder,'nes','Mario Bros. (Japan)'), 'Mario Bros. (Japan).png')
            self.assertEqual(covers.filename(folder,'nes','Mario Bros. (USA) (Beta)'), 'Mario Bros. (USA).png')
            self.assertIsNone(covers.filename(folder,'nes','Mario Kart (USA)'))
    def test_no_arbitrary_url_or_file_fetch(self):
        for url in ['http://raw.githubusercontent.com/test','https://evil.test/cover.png','https://raw.githubusercontent.com/other/rom.zip']:
            with self.assertRaises(ValueError):covers.fetch(url,100)
        for system,encoded in [('other','AAAA'),('nes','../'),('nes',base64.urlsafe_b64encode(b'\x00').decode().rstrip('='))]:
            with self.assertRaises(ValueError):covers.resolve('/unused',system,encoded)
    def test_artwork_system_is_specific(self):
        item={'title':'Metroid (Japan)','path':'/rom/nintendo-family-computer-disk-system-fds/metroid'}
        self.assertIn('/covers/fds/',covers.image_url('nes',item))
        self.assertIn('/covers/gbc-color/',covers.image_url('gbc',{'title':'Test','path':'/rom/nintendo-game-boy-color/test'}))

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
