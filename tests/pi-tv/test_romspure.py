"""ROMsPure connector: generated metadata, original CC0 bytes, no remote ROM requests."""
import hashlib
from io import BytesIO
import json
from pathlib import Path
import socket
import sys
import tempfile
import unittest
from unittest.mock import patch,Mock
import zipfile
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import romspure as rp
import providers
from library import Library
ROOT=Path(__file__).resolve().parents[2]
TOKEN='nintendo-entertainment-system~free-demo'
ID='romspure:nes:'+TOKEN

def card(title='Free Demo',slug='free-demo',image=True):
 return '<a href="https://romspure.cc/roms/nintendo-entertainment-system/'+slug+'/"><div>'+('<img src="https://romspure.cc/wp-content/uploads/2026/01/demo.png" alt="Wrong alt">' if image else '')+'</div><h3>'+title+'</h3><span>10 downloads</span></a>'
def html(body):return body.encode(),'text/html'

class RomspureTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.data=Path(self.tmp.name);rp.FAILURES.clear()
 def tearDown(self):self.tmp.cleanup()
 def test_fixed_console_identifier_and_route_boundaries(self):
  self.assertEqual(providers.decode(ID),('nes','free-demo'))
  for identifier in ['romspure:gba:'+TOKEN,'romspure:nes:nintendo-entertainment-system~../demo','romspure:nes:nintendo-entertainment-system~bios','https://evil.test']:
   with self.assertRaises(ValueError):providers.decode(identifier)
  for url,kind in [('http://romspure.cc/roms/nes/','html'),('https://evil.test/roms/x/','html'),('https://romspure.cc/wp-admin/foo','html'),('https://romspure.cc/wp-content/uploads/../../x.png','image'),('https://romspure.cc@127.0.0.1/a.zip','file'),('https://example.test:22/a.zip','file')]:
   with self.assertRaises(ValueError):rp.valid_url(url,kind)
 def test_parse_cards_uses_title_not_badges_or_image_alt_and_caches(self):
  document=card()+'<a href="https://romspure.cc/roms/nintendo-entertainment-system/page/12/">12</a>'+card('Firmware','firmware')
  with patch.object(rp,'fetch',return_value=html(document)) as fetch:
   page=rp.page_data(self.data,'nes','nintendo-entertainment-system');again=rp.page_data(self.data,'nes','nintendo-entertainment-system')
  self.assertEqual(fetch.call_count,1);self.assertEqual(page,again);self.assertEqual(page['pages'],12)
  self.assertEqual(len(page['items']),1);item=page['items'][0];self.assertEqual(item['title'],'Free Demo');self.assertEqual(item['downloadId'],ID);self.assertIn('/romspure-art/',item['image']);self.assertIn('/covers/',item['fallbackImage'])
 def test_missing_provider_image_uses_cover_metadata(self):
  with patch.object(rp,'fetch',return_value=html(card(image=False))):result=rp.page_data(self.data,'nes','nintendo-entertainment-system')
  self.assertIn('/covers/nes/',result['items'][0]['image'])
 def test_binary_letter_lookup_does_not_crawl_every_page(self):
  calls=[]
  def page(data,console,category,number=1,query=''):
   calls.append(number);letter=chr(64+number)
   return {'pages':26,'items':[{'title':letter+' Demo','downloadId':str(number)}]}
  with patch.object(rp,'page_data',side_effect=page):
   found=rp.catalogue(self.data,'nes','P');cached=rp.catalogue(self.data,'nes','P')
  self.assertEqual([x['title'] for x in found['items']],['P Demo']);self.assertEqual(found,cached);self.assertLess(len(calls),10)
 def test_full_text_query_cached_once_across_letters(self):
  with patch.object(rp,'page_data',return_value={'pages':1,'items':[{'title':'Free Demo','downloadId':ID}]}) as page:
   found=rp.catalogue(self.data,'nes','F','demo');empty=rp.catalogue(self.data,'nes','D','demo')
  self.assertEqual(page.call_count,1);self.assertEqual(len(found['items']),1);self.assertEqual(empty['items'],[])
 def test_failures_back_off_and_unsupported_console_never_calls_network(self):
  with patch.object(rp,'fetch',side_effect=OSError('offline')) as fetch:
   self.assertIn('error',rp.catalogue(self.data,'nes','F'));self.assertIn('error',rp.catalogue(self.data,'nes','F'));self.assertEqual(rp.catalogue(self.data,'vectrex','F')['items'],[])
  self.assertEqual(fetch.call_count,1)
 def test_soft_404_is_an_error_instead_of_permanent_empty_cache(self):
  with patch.object(rp,'fetch',return_value=html('<body class="error404">')):
   self.assertIn('error',rp.catalogue(self.data,'nes','A'))
 def test_private_dns_cannot_open_a_socket(self):
  conn=rp.PublicHTTPS('example.test')
  for address in ['127.0.0.1','192.168.1.63','169.254.169.254','::1','224.0.0.1','240.0.0.1']:
   with patch.object(socket,'getaddrinfo',return_value=[(socket.AF_INET,socket.SOCK_STREAM,6,'',(address,443))]),patch.object(socket,'socket') as sock:
    with self.assertRaises(ValueError):conn.connect()
    sock.assert_not_called()
 def test_public_ajax_protocol_extracts_only_file_metadata(self):
  selected='<a href="https://romspure.cc/download/free-demo-123/1">Original homebrew</a><a href="https://ad.test/download/free-demo-123/9">ad</a>'
  def fetch(url,kind='html',limit=None,progress=None,form=None):
   if kind=='html':return html(selected)
   if form['action']=='romspure_get_nonce':return json.dumps({'success':True,'data':{'nonce':'fixture_nonce'}}).encode(),'application/json'
   self.assertEqual(form['action'],'app_get_download_link');self.assertEqual(form['post_id'],'123');self.assertEqual(form['index'],'1')
   return json.dumps({'success':True,'data':{'url':'https://files.example.test/demo.zip','name':'Demo.zip'}}).encode(),'application/json'
  with patch.object(rp,'detail',return_value={'selection':['https://romspure.cc/download/free-demo-123']}),patch.object(rp,'fetch',side_effect=fetch) as request,patch.object(rp.time,'sleep'):
   files=rp.resolved_files(self.data,ID)
  self.assertEqual(files[0]['name'],'Demo.zip');self.assertEqual(request.call_count,3)
  self.assertTrue(all(call.args[1]!='file' for call in request.call_args_list if len(call.args)>1))
 def test_bad_nonce_or_missing_links_report_failure(self):
  with patch.object(rp,'detail',return_value={'selection':[]}):
   with self.assertRaises(ValueError):rp.resolved_files(self.data,ID)
  with patch.object(rp,'detail',return_value={'selection':['https://romspure.cc/download/free-demo-123']}),patch.object(rp,'fetch',side_effect=[html('<div data-download-ajax data-post-id="123" data-index="1"></div>'),(b'[]','application/json')]),patch.object(rp.time,'sleep'):
   with self.assertRaises(ValueError):rp.resolved_files(self.data,ID)
 def test_transfer_uses_original_cc0_fixture_and_actual_byte_progress(self):
  body=(ROOT/'output/nes-demo/palette-libre.nes').read_bytes();events=[]
  def transfer(url,kind,limit,progress):progress(0,len(body));progress(len(body),len(body));return body,'application/octet-stream'
  with patch.object(rp,'resolved_files',return_value=[{'name':'Demo.nes','url':'https://files.example.test/Demo.nes'}]),patch.object(rp,'fetch',side_effect=transfer):
   game,result=rp.payload(self.data,ID,lambda **values:events.append(values))
  self.assertEqual(result,body);self.assertEqual(game['sha256'],hashlib.sha256(body).hexdigest());self.assertEqual(events[-1]['bytesReceived'],len(body));self.assertNotIn(100,[e.get('progress') for e in events])
 def test_archive_traversal_and_executable_are_rejected(self):
  from freeroms import unpack_payload
  buf=BytesIO()
  with zipfile.ZipFile(buf,'w') as archive:archive.writestr('../demo.nes',b'NES\x1a')
  for body,name in [(buf.getvalue(),'demo.zip'),(b'MZ executable','demo.nes')]:
   with self.assertRaises(ValueError):unpack_payload('nes','demo',body,name,extensions={'nes'})
 def test_cross_provider_retry_produces_one_success_after_atomic_install(self):
  mount=self.data/'library';mount.mkdir();lib=Library(self.data/'jobs','/library',mount);lib.configure('/library')
  from source_settings import save
  save(lib.data,['romspure','freeroms'])
  body=(ROOT/'output/nes-demo/palette-libre.nes').read_bytes();game={'consoleId':'nes','filename':'fixture.nes','size':len(body),'sha256':hashlib.sha256(body).hexdigest()}
  import time
  try:
   with patch('freeroms.payload',side_effect=ValueError('no file')),patch.object(rp,'payload',return_value=(game,body)) as second:
    lib.start('alice','freeroms:nes:demo',{'candidateIds':[ID]})
    for _ in range(100):
     jobs=lib.jobs('alice')
     if jobs[0]['status'] in ('failed','succeeded'):break
     time.sleep(.01)
   self.assertEqual(jobs[0]['status'],'succeeded');self.assertEqual(len(jobs),1);self.assertEqual(second.call_count,1);self.assertEqual((mount/'roms/nes/fixture.nes').read_bytes(),body)
  finally:lib.pool.shutdown()
 def test_arcade_zip_remains_an_archive_and_never_extracts(self):
  from freeroms import unpack_payload
  buf=BytesIO()
  with zipfile.ZipFile(buf,'w') as archive:
   archive.writestr('free-demo.001',b'original fixture');archive.writestr('free-demo.002',b'another fixture')
  game,body=unpack_payload('neogeo','free-demo',buf.getvalue(),'demo.zip',extensions={'zip'})
  self.assertEqual(game['filename'],'free-demo.zip');self.assertEqual(body,buf.getvalue())
 def test_import_reuses_romspure_metadata_without_fetching_a_rom(self):
  from rom_import import install
  mount=self.data/'library';mount.mkdir();lib=Library(self.data/'metadata','/library',mount);lib.configure('/library')
  try:
   with patch.object(rp,'fetch',return_value=html(card())):rp.page_data(lib.data,'nes','nintendo-entertainment-system')
   item=install(lib,'nes','Free Demo.nes',(ROOT/'output/nes-demo/palette-libre.nes').read_bytes())
   self.assertIn('/romspure-art/',item['image']);self.assertIn('/covers/',item['fallbackImage'])
  finally:lib.pool.shutdown()
 def test_http_catalogue_requires_authentication(self):
  import server
  from types import SimpleNamespace
  handler=object.__new__(server.Handler);handler.path='/v1/romspure/nes?letter=A&q=demo';handler.server=SimpleNamespace(data=self.data,auth_base='http://127.0.0.1:10200');handler.headers={};handler.wfile=BytesIO();codes=[];handler.send_response=codes.append;handler.send_header=lambda *args:None;handler.end_headers=lambda:None
  with patch.object(server,'verified_username',return_value=None):handler.do_GET()
  self.assertEqual(codes[0],401)
  with patch.object(server,'verified_username',return_value='alice'),patch.object(rp,'catalogue',return_value={'items':[]}) as catalogue:
   from source_settings import save
   save(self.data,['romspure'])
   handler.do_GET();catalogue.assert_called_once_with(self.data,'nes','A','demo')
 def test_unexpected_transport_error_cannot_leave_a_job_running(self):
  import time,http.client
  mount=self.data/'lib';mount.mkdir();lib=Library(self.data/'records','/lib',mount);lib.configure('/lib')
  try:
   with patch.object(rp,'payload',side_effect=http.client.BadStatusLine('fixture')):
    lib.start('alice',ID)
    for _ in range(100):
     jobs=lib.jobs('alice')
     if jobs[0]['status']=='failed':break
     time.sleep(.01)
   self.assertEqual(jobs[0]['status'],'failed');self.assertNotEqual(jobs[0]['progress'],100)
  finally:lib.pool.shutdown()

 def test_numeric_query_cache_does_not_replace_numeric_alphabet_cache(self):
  with patch.object(rp,'page_data',return_value={'pages':1,'items':[{'title':'0 Demo','downloadId':'zero'}]}) as page:
   rp.catalogue(self.data,'nes','0','0');rp.catalogue(self.data,'nes','0')
  self.assertEqual(page.call_count,2)
