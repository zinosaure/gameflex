import json
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest.mock import patch,Mock
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import emugen as eg
import game_metadata as gm
from emugen_browser import BrowserResolver
ROOT=Path(__file__).resolve().parents[2];FIX=ROOT/'tests/pi-tv/fixtures/emugen'
URL=eg.game_url('free-demo-wii')
class MetadataTests(unittest.TestCase):
 def setUp(self):self.tmp=tempfile.TemporaryDirectory();self.data=Path(self.tmp.name)
 def tearDown(self):self.tmp.cleanup()
 def test_public_html_is_independent_of_downloads(self):
  item=eg.parse_game((FIX/'game.html').read_bytes(),URL,'wii')
  self.assertEqual(item['title'],'Free Demo');self.assertEqual(item['releaseDate'],'2020-03-02');self.assertEqual(item['languages'],['fr','en']);self.assertEqual(len(item['screenshots']),1);self.assertTrue(item['coverFront']);self.assertTrue(item['coverBack']);self.assertTrue(item['description']);self.assertEqual(len(item['trailers']),1)
 def test_console_and_navigation_are_verified(self):
  with self.assertRaises(eg.EmugenError):eg.parse_game((FIX/'game.html').read_bytes(),URL,'nes')
  for url in ['https://emugen.net.evil.test/jeux/a/','https://127.0.0.1/jeux/a/','https://www.emugen.net:22/jeux/a/','http://www.emugen.net/jeux/a/','https://www.emugen.net/wp-admin/','https://user:pass@www.emugen.net/jeux/a/']:
   with self.assertRaises(eg.EmugenError):eg.valid_url(url,'game')
 def test_priority_one_missing_field_and_manual_correction(self):
  base={'consoleId':'wii','title':'Old','image':'old','description':'old','screenshots':['old screenshot'],'manualMetadata':{'title':'My correction'}}
  primary={'consoleId':'wii','title':'EmuGen title','image':'primary cover','description':'primary description','screenshots':[]}
  result=eg.merge_metadata(base,primary,[{'provider':'other','screenshots':['secondary screenshot'],'image':'secondary cover'}])
  self.assertEqual(result['title'],'My correction');self.assertEqual(result['image'],'primary cover');self.assertEqual(result['description'],'primary description');self.assertEqual(result['screenshots'],['old screenshot'])
  result=eg.merge_metadata({'consoleId':'wii'},primary,[{'provider':'other','screenshots':['secondary screenshot'],'image':'secondary cover'}]);self.assertEqual(result['screenshots'],['secondary screenshot']);self.assertEqual(result['image'],'primary cover')
  self.assertNotIn('image',eg.merge_metadata({'consoleId':'nes'},primary))
 def test_ambiguous_results_are_not_automatically_selected(self):
  items=[{'title':'Free Demo','consoleId':'wii','emugenSlug':'a','match':{'exact':True}},{'title':'Free Demo','consoleId':'wii','emugenSlug':'b','match':{'exact':True}}]
  with patch.object(eg,'search',return_value={'items':items}),patch.object(eg,'detail') as detail:
   result=eg.identify(self.data,{'consoleId':'wii','title':'Free Demo'});self.assertEqual(result['status'],'choice');detail.assert_not_called()
 def test_region_and_edition_mismatches_need_a_choice(self):
  items=[{'title':'Free Demo','emugenSlug':'a','match':{'exact':True}}]
  with patch.object(eg,'search',return_value={'items':items}),patch.object(eg,'detail',return_value={'consoleId':'wii','region':'Japan','version':'Beta'}):
   result=eg.identify(self.data,{'consoleId':'wii','title':'Free Demo','region':'Europe','version':'Original'});self.assertEqual(result['status'],'choice')
 def test_network_failure_is_not_cached_as_an_empty_search(self):
  with patch.object(eg,'fetch',side_effect=eg.EmugenError()) as fetch:
   for _ in range(2):
    with self.assertRaises(eg.EmugenError):eg.rest(self.data,'console',{'slug':'wii'})
  self.assertEqual(fetch.call_count,2);self.assertEqual(list((self.data/'emugen').glob('rest-*.json')),[])
 def test_links_errors_are_not_cached_as_absence(self):
  resolver=BrowserResolver()
  try:
   with patch.object(resolver,'_resolve',side_effect=[{'status':'timeout','links':[]},{'status':'empty','links':[]}]) as task:
    self.assertEqual(resolver.resolve(self.data,'fixture')['status'],'timeout');self.assertEqual(resolver.resolve(self.data,'fixture')['status'],'empty');self.assertTrue(resolver.resolve(self.data,'fixture')['cached']);self.assertEqual(task.call_count,2)
  finally:resolver.close()
 def test_manual_edits_during_refresh_are_kept(self):
  base={'consoleId':'wii','reference':'uuid','title':'Free Demo'};primary={'consoleId':'wii','title':'EmuGen title','emugenSlug':'fixture'}
  previous=gm.store(self.data,base,{'status':'matched','item':primary});gm.correct(self.data,'wii','uuid',{'title':'Manual title','description':'Manual description','screenshots':[]})
  saved=gm.store(self.data,base,{'status':'matched','item':{**primary,'description':'Fresh EmuGen description'}},previous)
  self.assertEqual(saved['item']['title'],'Manual title');self.assertEqual(saved['item']['description'],'Manual description');self.assertEqual(saved['item']['screenshots'],[])
 def test_library_read_has_no_network_or_startup_reindex(self):
  with patch.object(eg,'fetch') as fetch:self.assertEqual(gm.listing(self.data),[]);fetch.assert_not_called()

 def test_region_and_beta_tags_in_filenames_are_not_silently_discarded(self):
  items=[{'title':'Free Demo','emugenSlug':'a','match':{'exact':True}}]
  with patch.object(eg,'search',return_value={'items':items}),patch.object(eg,'detail',return_value={'consoleId':'wii','title':'Free Demo','version':'Original'}):
   for title in ['Free Demo (Beta)','Free Demo (Europe)']:
    self.assertEqual(eg.identify(self.data,{'consoleId':'wii','title':title})['status'],'choice')

class ChromiumFixtureTests(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  import shutil
  if not shutil.which('chromium'):raise unittest.SkipTest('Native Chromium absent; run in the experience Docker image.')
  from playwright.sync_api import sync_playwright
  cls.driver=sync_playwright().start();cls.browser=cls.driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage'])
 @classmethod
 def tearDownClass(cls):cls.browser.close();cls.driver.stop()
 def page(self,name,status=200):
  context=self.browser.new_context();body=(FIX/name).read_text();context.route('https://www.emugen.net/**',lambda route:route.fulfill(status=status,content_type='text/html',body=body));return context,context.new_page()
 def test_magic_cookie_is_created_by_the_site_and_links_keep_their_url(self):
  from emugen_browser import read_page
  context,page=self.page('links.html')
  try:
   result=read_page(page,URL,timeout=2500);self.assertEqual(result['status'],'links');self.assertEqual(len(result['links']),1);self.assertEqual(result['links'][0]['url'],'https://host.example.test/intermediate?id=42&region=eu');self.assertTrue(any(c['name']=='fixture_magic' for c in context.cookies()))
  finally:page.close();context.close()
 def test_empty_waits_for_final_message_instead_of_loading(self):
  from emugen_browser import read_page
  context,page=self.page('empty.html')
  try:self.assertEqual(read_page(page,URL,timeout=2500)['status'],'empty')
  finally:page.close();context.close()
 def test_timeout_and_not_found_are_distinct(self):
  from emugen_browser import read_page
  from playwright.sync_api import TimeoutError
  context,page=self.page('timeout.html')
  try:
   with self.assertRaises(TimeoutError):read_page(page,URL,timeout=200)
  finally:page.close();context.close()
  context,page=self.page('empty.html',404)
  try:self.assertEqual(read_page(page,URL,timeout=200)['status'],'not_found')
  finally:page.close();context.close()

 def test_explicit_empty_message_is_not_replaced_by_a_help_link(self):
  from emugen_browser import read_page
  context=self.browser.new_context()
  context.route('https://www.emugen.net/**',lambda route:route.fulfill(content_type='text/html',body='<section id="emugen-download-section">No downloads available at this time! <a href="https://www.emugen.net/faq/">Help</a></section>'))
  page=context.new_page()
  try:
   result=read_page(page,URL,timeout=1000);self.assertEqual(result['status'],'empty');self.assertEqual(result['links'],[])
  finally:page.close();context.close()

class EnrichmentTests(unittest.TestCase):
 def test_only_missing_fields_are_requested_and_primary_cover_is_kept(self):
  from metadata_enrichment import enrich
  primary={'consoleId':'wii','title':'Free Demo','image':'primary','coverFront':'primary','description':'Primary description'}
  variants=[{'title':'Free Demo','consoleId':'wii','provider':'FreeROMs','downloadId':'freeroms:wii:fixture','screenshots':['existing screenshot']}]
  with patch('metadata_enrichment.provider') as provider:
   item=enrich('/unused',{'consoleId':'wii','title':'Free Demo'},primary,variants)
  provider.assert_not_called();self.assertEqual(item['image'],'primary');self.assertEqual(item['screenshots'],['existing screenshot'])
 def test_missing_cover_queries_the_cover_adapter_only(self):
  from metadata_enrichment import enrich
  with patch('metadata_enrichment.enabled',return_value=True),patch('metadata_enrichment.provider') as provider,patch('metadata_enrichment.decode',return_value=('nes','fixture')):
   provider.return_value.decode.return_value=('nes','fixture');provider.return_value.detail.return_value={'image':'https://provider.test/image.png'}
   item=enrich('/data',{'consoleId':'nes','title':'Free Demo'},{'consoleId':'nes','title':'Free Demo','description':'Primary description'},[{'consoleId':'nes','title':'Free Demo','provider':'FreeROMs','downloadId':'freeroms:nes:fixture'}])
  self.assertIn('/freeroms-art/',item['image']);self.assertEqual(item['description'],'Primary description');provider.return_value.detail.assert_called_once()

class ResolverSerialTests(unittest.TestCase):
 def test_one_browser_serial_worker_and_pages_closed(self):
  import shutil
  if not shutil.which('chromium'):self.skipTest('Run in the native Chromium image.')
  from concurrent.futures import ThreadPoolExecutor
  import emugen_browser as browser_module
  from playwright.sync_api import sync_playwright
  resolver=BrowserResolver();starts=[];running=[0];maximum=[0];original=browser_module.read_page
  def start():
   if resolver.browser and resolver.browser.is_connected():return
   starts.append(1);resolver.driver=sync_playwright().start();resolver.browser=resolver.driver.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--disable-dev-shm-usage']);resolver.context=resolver.browser.new_context()
   resolver.context.route('https://www.emugen.net/**',lambda route:route.fulfill(content_type='text/html',body=(FIX/'empty.html').read_text()))
  def read(*args,**kwargs):
   running[0]+=1;maximum[0]=max(maximum[0],running[0])
   try:return original(*args,**kwargs)
   finally:running[0]-=1
  resolver._start=start
  try:
   with tempfile.TemporaryDirectory() as data,patch.object(browser_module,'read_page',side_effect=read),ThreadPoolExecutor(max_workers=2) as callers:
    futures=[callers.submit(resolver.resolve,data,slug) for slug in ('fixture-one','fixture-two')]
    self.assertEqual([f.result()['status'] for f in futures],['empty','empty'])
   self.assertEqual(len(starts),1);self.assertEqual(maximum[0],1);self.assertEqual(resolver.pool.submit(lambda:len(resolver.context.pages)).result(),0)
  finally:resolver.close()

class EndpointTests(unittest.TestCase):
 def request(self,path,method='GET',data=None,identity=None):
  from io import BytesIO
  from types import SimpleNamespace
  import server
  handler=object.__new__(server.Handler);handler.path=path;handler.server=SimpleNamespace(data=Path('/unused'),auth_base='http://127.0.0.1:10200');raw=json.dumps(data).encode() if data is not None else b'';handler.headers={'Content-Length':str(len(raw))};handler.rfile=BytesIO(raw);handler.wfile=BytesIO();codes=[];handler.send_response=codes.append;handler.send_header=lambda *args:None;handler.end_headers=lambda:None
  with patch.object(server,'verified_identity',return_value=identity):getattr(handler,'do_'+method)()
  return codes[0],json.loads(handler.wfile.getvalue())
 def test_removed_sources_do_not_launch_browser(self):
  import emugen_browser
  with patch.object(emugen_browser.RESOLVER,'resolve',return_value={'status':'empty','links':[]}) as resolve,patch.object(eg,'detail',return_value={'title':'Fixture'}):
   code,value=self.request('/v1/emugen/games/fixture?console=wii',identity={'username':'member'})
   self.assertEqual(code,410);self.assertEqual(value['status'],'removed')
   resolve.assert_not_called()
 def test_corrections_and_refresh_are_admin_only(self):
  for path,method in [('/v1/metadata/corrections','PUT'),('/v1/metadata/refresh','POST'),('/v1/metadata/bind','POST')]:
   self.assertEqual(self.request(path,method,{},identity={'username':'member','role_level':1})[0],403)

class CataloguePaginationTests(unittest.TestCase):
 def setUp(self):self.tmp=tempfile.TemporaryDirectory();self.data=Path(self.tmp.name)
 def tearDown(self):self.tmp.cleanup()
 def pages(self,_data,path,params):
  if path=='console':return [{'slug':'gba','id':1}]
  return [{'title':'Alpha '+str(i),'consoleId':'gba'} for i in range(100)] if params['page']<4 else [{'title':'Golf fixture','consoleId':'gba'},{'title':'Zeta fixture','consoleId':'gba'}]
 def test_no_letter_returns_all_titles_and_a_next_page(self):
  with patch.object(eg,'rest',side_effect=self.pages),patch.object(eg,'rest_game',side_effect=lambda item,_:dict(item)):
   first=eg.search(self.data,'gba',letter='',page=1);last=eg.search(self.data,'gba',letter='',page=4)
  self.assertEqual(len(first['items']),100);self.assertEqual(first['nextPage'],2)
  self.assertEqual(len(last['items']),2);self.assertIsNone(last['nextPage'])
 def test_letter_can_match_beyond_the_old_300_title_limit(self):
  with patch.object(eg,'rest',side_effect=self.pages),patch.object(eg,'rest_game',side_effect=lambda item,_:dict(item)):
   self.assertEqual(eg.search(self.data,'gba',letter='G',page=1)['items'],[])
   last=eg.search(self.data,'gba',letter='G',page=4)
  self.assertEqual(last['items'][0]['title'],'Golf fixture');self.assertIsNone(last['nextPage'])
 def test_explicit_end_is_distinct_from_network_failure(self):
  def end(_data,path,params):
   if path=='console':return [{'slug':'gba','id':1}]
   raise eg.EmugenError('end_of_pages')
  with patch.object(eg,'rest',side_effect=end):self.assertIsNone(eg.search(self.data,'gba',letter='',page=5)['nextPage'])
  with patch.object(eg,'rest',side_effect=eg.EmugenError()):
   with self.assertRaises(eg.EmugenError):eg.search(self.data,'gba',letter='',page=5)
 def test_invalid_page_and_letter_are_rejected(self):
  for value in ['0','-1','1.5','1001','bad']:
   with self.assertRaises(eg.EmugenError):eg.search(self.data,'gba',letter='',page=value)
  with self.assertRaises(eg.EmugenError):eg.search(self.data,'gba',letter='GG',page=1)

class AsyncEndpointTests(unittest.TestCase):
 def test_create_returns_accepted_before_browser_is_ready(self):
  import emugen_manual
  with patch.object(emugen_manual.MANUAL,'start',return_value={'id':'a'*32,'status':'starting'}) as start:
   code,value=EndpointTests().request('/v1/emugen/verification','POST',{'slug':'fixture','consoleId':'wii'},identity={'username':'alice'})
   self.assertEqual(code,202);self.assertEqual(value['item']['status'],'starting')
   self.assertTrue(start.call_args.kwargs['background'])
