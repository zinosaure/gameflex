import json
from pathlib import Path
import sys,tempfile,unittest
from unittest.mock import Mock,patch
from types import SimpleNamespace
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import source_settings as settings
import test_api as endpoint
class ProviderSettingsTests(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();self.s=SimpleNamespace(data=Path(self.temp.name),auth_base='http://127.0.0.1:10200',library=Mock())
 def tearDown(self):self.temp.cleanup()
 request=endpoint.ApiTests.request
 def test_defaults_priority_persistence_and_empty_selection(self):
  state=settings.listing(self.s.data);self.assertNotIn('emugen',[item['id'] for item in state['items']]);self.assertEqual(state['enabledIds'],['planetemu','freeroms','romspure'])
  self.assertTrue(all(x['enabled'] for x in state['items']))
  settings.save(self.s.data,['planetemu','freeroms']);self.assertEqual(settings.enabled_ids(self.s.data),['planetemu','freeroms'])
  settings.save(self.s.data,[]);self.assertEqual(settings.enabled_ids(self.s.data),[])
  self.assertEqual((self.s.data/'provider-settings.json').stat().st_mode&0o777,0o600)
 def test_legacy_emugen_only_selection_migrates(self):
  (self.s.data/'provider-settings.json').write_text(json.dumps({'enabledIds':['emugen']}))
  self.assertEqual(settings.enabled_ids(self.s.data),['planetemu','freeroms','romspure'])
 def test_invalid_updates_do_not_erase_state(self):
  for value in [None,'emugen',['unknown'],['emugen','emugen'],[{}]]:
   with self.assertRaises(ValueError):settings.save(self.s.data,value)
  self.assertEqual(settings.enabled_ids(self.s.data),['planetemu','freeroms','romspure'])
 def test_api_is_authenticated_and_only_admin_can_toggle(self):
  api=endpoint.api
  for method in ['GET','PUT']:self.assertEqual(self.request('/v1/game-sources',method,{'enabledIds':[]})[0],401)
  with patch.object(api,'verified_identity',return_value={'username':'member','role_level':1}):
   self.assertFalse(self.request('/v1/game-sources',token='test')[1]['canConfigure'])
   self.assertEqual(self.request('/v1/game-sources','PUT',{'enabledIds':[]},'test')[0],403)
  with patch.object(api,'verified_identity',return_value={'username':'admin','role_level':0}):
   self.assertEqual(self.request('/v1/game-sources','PUT',{'enabledIds':['planetemu','freeroms']},'test')[0],200)
   self.assertEqual(self.request('/v1/game-sources',token='test')[1]['enabledIds'],['planetemu','freeroms'])
 def test_disabled_catalogue_and_download_have_no_provider_call(self):
  settings.save(self.s.data,[])
  import planetemu
  with patch.object(endpoint.api,'verified_username',return_value='alice'),patch.object(planetemu,'catalogue') as catalogue:
   code,value=self.request('/v1/planetemu/gba',token='test');self.assertEqual(code,200);self.assertTrue(value['disabled']);catalogue.assert_not_called()
  with patch.object(endpoint.api,'verified_identity',return_value={'username':'alice','role_level':0}):
   self.assertEqual(self.request('/v1/downloads','POST',{'gameId':'freeroms:nes:test'},'test')[0],409)
   self.s.library.start.assert_not_called()
 def test_disabled_emugen_sources_and_refresh_do_not_launch_chromium(self):
  settings.save(self.s.data,[])
  import emugen_browser
  with patch.object(endpoint.api,'verified_username',return_value='alice'),patch.object(emugen_browser.RESOLVER,'resolve') as resolver:
   for path in ['/v1/emugen/sources','/v1/emugen/verification','/v1/metadata/refresh']:
    code,value=self.request(path,'POST',{'slug':'fixture'},'test');self.assertEqual(code,409);self.assertEqual(value['status'],'disabled')
   resolver.assert_not_called()
 def test_disabled_secondary_has_no_network_and_primary_manual_are_retained(self):
  settings.save(self.s.data,[])
  from metadata_enrichment import enrich
  with patch('metadata_enrichment.provider') as provider:
   result=enrich(self.s.data,{'consoleId':'nes','title':'Fixture','manualMetadata':{'description':'My edit'}},{'consoleId':'nes','title':'Fixture','description':'Primary'},[{'consoleId':'nes','title':'Fixture','downloadId':'freeroms:nes:fixture'}])
   provider.assert_not_called();self.assertEqual(result['description'],'My edit')
 def test_disabled_queued_fallback_cannot_transfer_bytes(self):
  settings.save(self.s.data,[])
  import providers
  with patch.object(providers,'provider') as provider:
   with self.assertRaises(ValueError):providers.payload(self.s.data,'freeroms:nes:fixture')
   provider.assert_not_called()

 def test_disabled_artwork_uses_cache_only(self):
  settings.save(self.s.data,[])
  import freeroms
  with patch.object(freeroms,'artwork') as artwork:
   self.assertEqual(self.request('/v1/freeroms-art/nes/fixture')[0],404);artwork.assert_not_called()
  folder=self.s.data/'freeroms';folder.mkdir();media=self.s.data/'media';media.mkdir();identifier='a'*64
  (folder/'nes-fixture.art.json').write_text(json.dumps({'id':identifier,'reviewAfter':0}));(media/(identifier+'.img')).write_bytes(b'fixture')
  self.assertEqual(settings.cached_art(self.s.data,'freeroms','nes/fixture')['id'],identifier)
  self.assertIsNone(settings.cached_art(self.s.data,'freeroms','nes/../../anything'))
