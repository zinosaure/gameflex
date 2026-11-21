package config

import "testing"

func TestControlRouteDoesNotChangePublicAddress(t *testing.T) {
	cfg := Config{PublicHost: "192.168.1.63", InternalHost: "gameflex-host-relay"}
	if cfg.ReachableHost() != "gameflex-host-relay" || cfg.PublicHost != "192.168.1.63" {
		t.Fatal("internal routing must preserve the public LAN address")
	}
	cfg.InternalHost = ""
	if cfg.ReachableHost() != cfg.PublicHost {
		t.Fatal("installations without a relay must keep their original route")
	}
}
