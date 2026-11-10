import io,json,ssl,tempfile,time,unittest,zipfile
from pathlib import Path
from unittest.mock import patch
import sys
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import freeroms
from library import Library
ROOT=Path(__file__).resolve().parents[2]
class FreeRomsTests(unittest.TestCase):
 def test_tls_remains_verified_and_urls_are_provider_only(self):
  ctx=freeroms.context();self.assertTrue(ctx.check_hostname);self.assertEqual(ctx.verify_mode,ssl.CERT_REQUIRED);self.assertFalse(ctx.verify_flags&ssl.VERIFY_X509_PARTIAL_CHAIN)
  for url in ['http://www.freeroms.com/file.zip','https://evil.test/file.zip','https://www.freeroms.com@evil.test/file.zip','https://filedm.com/test','https://www.freeroms.com:8080/a.zip']:
   with self.assertRaises(ValueError):freeroms.valid_url(url,'file')
  for identifier in ['freeroms:nes:../test','freeroms:gba:bios','freeroms:gba:firmware','unknown:nes:demo']:
   with self.assertRaises(ValueError):freeroms.decode(identifier)
 def test_catalogue_items_are_real_source_metadata(self):
  html=b'<a href="https://www.freeroms.com/roms/gba/example.htm">Example (USA)</a><a href="https://evil.test/roms/gba/other.htm">Other</a><a href="/roms/gba/bios.htm">BIOS</a>'
  with tempfile.TemporaryDirectory() as data,patch.object(freeroms,'fetch',return_value=(html,'text/html')):
   result=freeroms.catalogue(data,'gba','E');self.assertEqual(len(result['items']),1);self.assertEqual(result['items'][0]['provider'],'FreeROMs');self.assertEqual(result['items'][0]['downloadId'],'freeroms:gba:example')
 def test_ads_are_not_treated_as_downloads(self):
  html=b'<script>window.location="https://evil.test/file.zip"</script><a rel="sponsored" href="https://www.freeroms.com/ad.zip">Ad</a><a href="https://filedm.com/test">Download</a><img src="/roms_screenshot/gba/example.jpg">'
  with tempfile.TemporaryDirectory() as data,patch.object(freeroms,'fetch',return_value=(html,'text/html')):
   detail=freeroms.detail(data,'gba','example');self.assertIsNone(detail['direct']);self.assertTrue(detail['image'].endswith('example.jpg'))
   with self.assertRaisesRegex(ValueError,'lien direct'):freeroms.payload(data,'freeroms:gba:example')
 def archive(self,name='demo.nes',body=None):
  stream=io.BytesIO()
  with zipfile.ZipFile(stream,'w',zipfile.ZIP_DEFLATED) as z:z.writestr(name,body or (ROOT/'output/nes-demo/palette-libre.nes').read_bytes())
  return stream.getvalue()
 def test_direct_download_installs_only_the_free_generated_fixture(self):
  archive=self.archive()
  with tempfile.TemporaryDirectory() as data:
   mount=Path(data)/'library';mount.mkdir();lib=Library(Path(data)/'data','/chosen',mount);lib.configure('/chosen')
   from source_settings import save
   save(lib.data,['freeroms'])
   try:
    with patch.object(freeroms,'detail',return_value={'direct':'https://www.freeroms.com/files/test.zip'}),patch.object(freeroms,'fetch',return_value=(archive,'application/zip')):
     job=lib.start('alice','freeroms:nes:test')
     for _ in range(150):
      status=lib.jobs('alice')[0]
      if status['status'] in ('succeeded','failed'):break
      time.sleep(.01)
     self.assertEqual(status['status'],'succeeded',status.get('message'));self.assertEqual((mount/'roms/nes/demo.nes').read_bytes(),(ROOT/'output/nes-demo/palette-libre.nes').read_bytes())
   finally:lib.pool.shutdown()
 def test_zip_traversal_or_fake_game_is_rejected(self):
  for archive in [self.archive('../escape.nes'),self.archive('ad.nes',b'<html>ad</html>')]:
   with patch.object(freeroms,'detail',return_value={'direct':'https://www.freeroms.com/files/test.zip'}),patch.object(freeroms,'fetch',return_value=(archive,'application/zip')):
    with self.assertRaises(ValueError):freeroms.payload('/unused','freeroms:nes:test')
 def test_private_network_provider_address_is_refused(self):
  addresses=[(2,1,6,'',('127.0.0.1',443))]
  with patch.object(freeroms.socket,'getaddrinfo',return_value=addresses):
   with self.assertRaisesRegex(ValueError,'réseau'):freeroms.fetch('https://www.freeroms.com/files/test.zip','file')
 def test_gba_header_is_checked_with_synthetic_public_test_data(self):
  body=bytearray(192);body[0xA0:0xAC]=b'GAMEFLEXTEST';body[0xB2]=0x96;body[0xBD]=(-sum(body[0xA0:0xBD])-0x19)&255
  with patch.object(freeroms,'detail',return_value={'direct':'https://www.freeroms.com/files/test.gba'}),patch.object(freeroms,'fetch',return_value=(bytes(body),'application/octet-stream')):
   game,result=freeroms.payload('/unused','freeroms:gba:test');self.assertEqual(result,bytes(body));self.assertEqual(game['filename'],'test.gba')
  with patch.object(freeroms,'detail',return_value={'direct':'https://www.freeroms.com/files/test.gba'}),patch.object(freeroms,'fetch',return_value=(b'not-a-game','application/octet-stream')):
   with self.assertRaisesRegex(ValueError,'GBA'):freeroms.payload('/unused','freeroms:gba:test')

 def test_real_nofollow_file_link_is_accepted(self):
  html=b'<a rel="nofollow" href="https://www.freeroms.com/files/test.zip">Download</a>'
  with tempfile.TemporaryDirectory() as data,patch.object(freeroms,'fetch',return_value=(html,'text/html')):
   self.assertEqual(freeroms.detail(data,'nes','test')['direct'],'https://www.freeroms.com/files/test.zip')
 def test_stream_reports_actual_received_bytes(self):
  from email.message import Message
  body=b'test-data'*20000
  class Response:
   def __init__(self):self.file=io.BytesIO(body);self.headers=Message();self.headers['Content-Type']='application/octet-stream';self.headers['Content-Length']=str(len(body))
   def __enter__(self):return self
   def __exit__(self,*args):pass
   def read(self,n):return self.file.read(n)
  class Opener:
   def open(self,*args,**kwargs):return Response()
  events=[]
  with patch.object(freeroms.socket,'getaddrinfo',return_value=[(2,1,6,'',('93.184.216.34',443))]),patch.object(freeroms.urllib.request,'build_opener',return_value=Opener()):
   result,_=freeroms.fetch('https://www.freeroms.com/files/test.zip','file',progress=lambda received,total:events.append((received,total)))
  self.assertEqual(result,body);self.assertEqual(events[0],(0,len(body)));self.assertEqual(events[-1],(len(body),len(body)));self.assertGreater(len(events),2);self.assertEqual([r for r,t in events],sorted(r for r,t in events))
 def test_download_progress_is_not_success_until_file_exists(self):
  import threading,hashlib
  body=(ROOT/'output/nes-demo/palette-libre.nes').read_bytes();started=threading.Event();release=threading.Event()
  def payload(data,identifier,progress):
   progress(phase='downloading',progress=42,bytesReceived=420,totalBytes=1000,message='Téléchargement…');started.set();release.wait(2)
   return {'consoleId':'nes','filename':'test.nes','size':len(body),'sha256':hashlib.sha256(body).hexdigest()},body
  with tempfile.TemporaryDirectory() as data:
   mount=Path(data)/'library';mount.mkdir();lib=Library(Path(data)/'data','/chosen',mount);lib.configure('/chosen')
   from source_settings import save
   save(lib.data,['freeroms'])
   try:
    with patch.object(freeroms,'payload',side_effect=payload):
     accepted=lib.start('alice','freeroms:nes:test',{'title':'Démo libre','image':'https://evil.test/image'})
     self.assertEqual(accepted['status'],'queued');self.assertEqual(accepted['progress'],0);self.assertEqual(accepted['image'],'')
     self.assertTrue(started.wait(1));current=lib.jobs('alice')[0];self.assertEqual(current['status'],'running');self.assertEqual(current['progress'],42);self.assertFalse((mount/'roms/nes/test.nes').exists())
     release.set()
     for _ in range(100):
      done=lib.jobs('alice')[0]
      if done['status'] in ('failed','succeeded'):break
      time.sleep(.01)
     self.assertEqual(done['status'],'succeeded');self.assertEqual(done['progress'],100);self.assertTrue((mount/'roms/nes/test.nes').exists())
   finally:release.set();lib.pool.shutdown()
 def test_no_direct_link_fails_at_zero_without_installing(self):
  with tempfile.TemporaryDirectory() as data:
   mount=Path(data)/'library';mount.mkdir();lib=Library(Path(data)/'data','/chosen',mount);lib.configure('/chosen')
   from source_settings import save
   save(lib.data,['freeroms'])
   try:
    with patch.object(freeroms,'detail',return_value={'direct':None}):
     lib.start('alice','freeroms:nes:test')
     for _ in range(100):
      done=lib.jobs('alice')[0]
      if done['status']=='failed':break
      time.sleep(.01)
     self.assertEqual(done['status'],'failed');self.assertEqual(done['progress'],0);self.assertEqual(done['bytesReceived'],0);self.assertFalse((mount/'roms').exists())
   finally:lib.pool.shutdown()
