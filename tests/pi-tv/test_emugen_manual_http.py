"""The same touch / same-context test with WebSockets deliberately blocked."""
from unittest.mock import patch
import test_emugen_manual_touch as touch
class HttpFallbackTests(touch.TouchTests):
 def test_touch_same_session_resume_links_displayed(self):
  original=touch.preview.create_server
  def blocked(*args,**kwargs):
   manager,server,session=original(*args,**kwargs);server.block_websocket=True
   return manager,server,session
  with patch.object(touch.preview,'create_server',blocked):
   super().test_touch_same_session_resume_links_displayed()
