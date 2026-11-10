from pathlib import Path
import unittest


class LiveNginxWebSocketTest(unittest.TestCase):
    def test_kernel_api_proxy_uses_http_11_for_websocket_upgrade(self):
        config = (
            Path(__file__).resolve().parents[2] / "deploy/pi-tv/live-nginx.conf"
        ).read_text(encoding="utf-8")
        start = config.index("    location /api/ {")
        end = config.index("\n    }", start)
        api_proxy = config[start:end]

        self.assertIn("proxy_http_version 1.1;", api_proxy)
        self.assertIn("proxy_set_header Upgrade $http_upgrade;", api_proxy)
        self.assertIn(
            "proxy_set_header Connection $gameflex_connection;", api_proxy
        )


if __name__ == "__main__":
    unittest.main()
