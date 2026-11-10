// Probe only the generated demo: private owner control is provided by exercise-stream.py.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"net/http"
	"os"
	"regexp"
	"time"
)

func main() {
	if len(os.Args) != 3 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(os.Args[1]) {
		os.Exit(2)
	}
	id := os.Args[1]
	base := "http://127.0.0.1:10202/api/v1/gateway-webrtc/streams/" + id
	client := http.Client{Timeout: 8 * time.Second}
	response, e := client.Post(base+"/viewer-offer", "application/json", bytes.NewBufferString(`{"hasInput":false}`))
	if e != nil || response.StatusCode != 200 {
		fmt.Println("Offer unavailable")
		os.Exit(1)
	}
	var offer struct{ ViewerID, SDP string }
	if json.NewDecoder(response.Body).Decode(&offer) != nil {
		os.Exit(1)
	}
	response.Body.Close()
	defer func() {
		request, _ := http.NewRequest("DELETE", base+"/viewer/"+offer.ViewerID, nil)
		r, e := client.Do(request)
		if e == nil {
			r.Body.Close()
		}
	}()
	engine := webrtc.SettingEngine{}
	engine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	pc, e := webrtc.NewAPI(webrtc.WithSettingEngine(engine)).NewPeerConnection(webrtc.Configuration{})
	if e != nil {
		os.Exit(1)
	}
	defer pc.Close()
	done := make(chan bool, 1)
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) { fmt.Println("Peer:", s.String()) })
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		file, e := os.Create(os.Args[2])
		if e != nil {
			done <- false
			return
		}
		defer file.Close()
		builder := samplebuilder.New(512, &codecs.H264Packet{}, 90000)
		count := 0
		for {
			packet, _, e := track.ReadRTP()
			if e != nil {
				done <- false
				return
			}
			builder.Push(packet)
			for sample := builder.Pop(); sample != nil; sample = builder.Pop() {
				file.Write(sample.Data)
				count++
				if count >= 60 {
					fmt.Println("Video access units received:", count)
					done <- true
					return
				}
			}
		}
	})
	if pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}) != nil {
		os.Exit(1)
	}
	answer, e := pc.CreateAnswer(nil)
	if e != nil {
		os.Exit(1)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	pc.SetLocalDescription(answer)
	select {
	case <-gathered:
	case <-time.After(8 * time.Second):
		fmt.Println("ICE gathering timeout")
		os.Exit(1)
	}
	signal, _, e := websocket.DefaultDialer.Dial("ws://127.0.0.1:10202/api/v1/gateway-webrtc/streams/"+id+"/viewer-ws?viewerId="+offer.ViewerID, nil)
	if e != nil {
		fmt.Println("Signaling unavailable")
		os.Exit(1)
	}
	defer signal.Close()
	signal.WriteJSON(map[string]any{"type": "answer", "sdp": pc.LocalDescription().SDP})
	select {
	case ok := <-done:
		if !ok {
			os.Exit(1)
		}
	case <-time.After(35 * time.Second):
		fmt.Println("No video received")
		os.Exit(1)
	}
}
