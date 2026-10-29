// Command fake_child simulates a session container pushing encoded frames to
// gatewaywebrtc over the shared internal protocol — it validates the Phase-2
// gateway surface end-to-end (child registration, frame fan-out, browser
// viewers, input roundtrip) before any real child is migrated. Run it
// against a live gateway (GAMEFLEX_INTERNAL_SECRET must match).
package main

import (
	_ "embed"
	"flag"
	"log/slog"
	"os"
	"time"

	"gameflex/gatewayclient"
)

//go:embed assets/testpattern.h264
var testPattern []byte

// splitAnnexBAccessUnits groups Annex-B NAL units into access units with
// non-VCL NALs (SPS/PPS/SEI/AUD) prepended to the next slice — same logic as
// gatewaywebrtc's test harness, because a decoder fed SPS/PPS as their own
// frames stays black.
func splitAnnexBAccessUnits(data []byte) [][]byte {
	var nals [][]byte
	start := -1
	for i := 0; i < len(data); {
		if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			if start >= 0 && i > start {
				nals = append(nals, data[start:i])
			}
			start = i
			i += 4
			continue
		}
		if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			if start >= 0 && i > start {
				nals = append(nals, data[start:i])
			}
			start = i
			i += 3
			continue
		}
		i++
	}
	if start >= 0 && start < len(data) {
		nals = append(nals, data[start:])
	}

	var units [][]byte
	var pending []byte
	for _, nal := range nals {
		nalType := byte(0)
		i := 3
		if len(nal) > 3 && nal[2] == 0 {
			i = 4
		}
		if i < len(nal) {
			nalType = nal[i] & 0x1F
		}
		switch nalType {
		case 6, 7, 8, 9: // SEI, SPS, PPS, AUD — prepend to the next slice
			pending = append(append([]byte{}, pending...), nal...)
		default:
			units = append(units, append(pending, nal...))
			pending = nil
		}
	}
	return units
}

func main() {
	url := flag.String("url", os.Getenv("GAMEFLEX_GATEWAY_URL"), "gateway URL (http://host:port)")
	session := flag.String("session", "fake1", "session id to register")
	platform := flag.String("platform", "fake", "platform label")
	flag.Parse()
	if *url == "" {
		slog.Error("fake_child: -url or GAMEFLEX_GATEWAY_URL required")
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	frames := splitAnnexBAccessUnits(testPattern)
	slog.Info("fake_child: pattern loaded", "access_units", len(frames))

	handlers := gatewayclient.Handlers{
		OnInput: func(viewerID string, data []byte) {
			slog.Info("fake_child: input", "viewer", viewerID, "bytes", len(data))
		},
		OnViewerJoined: func(viewerID, username string, hasInput bool) {
			slog.Info("fake_child: viewer joined", "viewer", viewerID, "username", username, "has_input", hasInput)
		},
		OnViewerLeft: func(viewerID string) {
			slog.Info("fake_child: viewer left", "viewer", viewerID)
		},
		OnStatus: func(connected bool, err error) {
			slog.Info("fake_child: gateway status", "connected", connected, "error", err)
		},
	}

	client, err := gatewayclient.Dial(*url, *session, *platform, os.Getenv("GAMEFLEX_INTERNAL_SECRET"),
		gatewayclient.MediaInfo{Width: 640, Height: 360, FPS: 30, Bitrate: 800000}, handlers)
	if err != nil {
		slog.Error("fake_child: dial failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	slog.Info("fake_child: registered, pushing frames")

	ticker := time.NewTicker(33 * time.Millisecond) // ~30fps, matches the clip
	defer ticker.Stop()
	i := 0
	for range ticker.C {
		if err := client.PushVideo(frames[i%len(frames)], 33*time.Millisecond); err != nil {
			slog.Warn("fake_child: push failed", "error", err)
		}
		i++
	}
}
