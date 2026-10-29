package gatewayclient

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// sealGateway — une passerelle qui répond à la poignée de main avec `prove`,
// puis rend ce qu'elle reçoit sur `frames`. Elle envoie `input` scellé dès
// qu'elle a répondu, quand elle a un scellement.
type sealGateway struct {
	prove  func(sessionID, partyKey string) (*Seal, string)
	frames chan []byte
	input  []byte
}

func (gateway *sealGateway) serve(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var register struct {
			SessionID string `json:"sessionId"`
			Seal      string `json:"seal"`
		}
		_ = json.Unmarshal(data, &register)
		seal, proof := gateway.prove(register.SessionID, register.Seal)
		answer, _ := json.Marshal(map[string]string{"type": "registered", "proof": proof})
		_ = conn.WriteMessage(websocket.TextMessage, answer)
		if seal != nil && gateway.input != nil {
			_ = conn.WriteMessage(websocket.TextMessage, seal.SealText(gateway.input))
		}
		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				return
			}
			gateway.frames <- frame
		}
	}))
	t.Cleanup(server.Close)

	return server.URL
}

func gatewayKeys(t *testing.T) (*ecdh.PrivateKey, string) {
	t.Helper()
	private, public, err := NewGatewayKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ParseGatewayPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}

	return key, public
}

// LA BONNE PASSERELLE : elle prouve, elle lit les images, la partie lit ses
// entrées.
func TestASealedChildStreamsToOurGateway(t *testing.T) {
	private, public := gatewayKeys(t)
	var gatewaySeal *Seal
	gateway := &sealGateway{
		frames: make(chan []byte, 4),
		input:  []byte(`{"type":"input","viewerId":"v1","data":"` + base64.StdEncoding.EncodeToString([]byte("A")) + `"}`),
		prove: func(sessionID, partyKey string) (*Seal, string) {
			seal, proof, err := AcceptSeal(private, sessionID, partyKey)
			if err != nil {
				t.Errorf("AcceptSeal : %v", err)
			}
			gatewaySeal = seal

			return seal, proof
		},
	}
	inputs := make(chan string, 1)
	client, err := DialSealed(gateway.serve(t), "streamex-1", "streamex", "", public, MediaInfo{}, Handlers{
		OnInput: func(viewerID string, data []byte) { inputs <- viewerID + ":" + string(data) },
	})
	if err != nil {
		t.Fatalf("DialSealed : %v", err)
	}
	defer client.Close()

	if err := client.PushVideo([]byte("une image H264"), 16*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	frame := <-gateway.frames
	if bytes.Contains(frame, []byte("une image H264")) {
		t.Fatal("l'image est partie en clair")
	}
	kind, duration, payload, err := gatewaySeal.OpenFrame(frame)
	if err != nil || kind != kindVideo || duration != 16*time.Millisecond || string(payload) != "une image H264" {
		t.Fatalf("OpenFrame = %d %v %q %v", kind, duration, payload, err)
	}

	select {
	case got := <-inputs:
		if got != "v1:A" {
			t.Fatalf("entrée = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("l'entrée scellée n'est pas arrivée")
	}
}

// UNE AUTRE PASSERELLE : sans preuve, ou avec une autre clé, rien ne part.
func TestASealedChildRefusesAnotherGateway(t *testing.T) {
	_, public := gatewayKeys(t)
	other, _ := gatewayKeys(t)
	for name, prove := range map[string]func(string, string) (*Seal, string){
		"sans preuve": func(string, string) (*Seal, string) { return nil, "" },
		"preuve inventée": func(string, string) (*Seal, string) {
			return nil, base64.StdEncoding.EncodeToString([]byte("registered"))
		},
		"autre clé": func(sessionID, partyKey string) (*Seal, string) {
			seal, proof, _ := AcceptSeal(other, sessionID, partyKey)
			return seal, proof
		},
	} {
		gateway := &sealGateway{prove: prove, frames: make(chan []byte, 1)}
		_, err := DialSealed(gateway.serve(t), "streamex-1", "streamex", "", public, MediaInfo{}, Handlers{})
		if !errors.Is(err, ErrGatewayNotRecognized) {
			t.Errorf("%s : %v, attendu ErrGatewayNotRecognized", name, err)
		}
	}
}

// LE RELAIS : une passerelle tierce fait prouver une vraie à sa place. La
// partie l'accepte — mais ce qu'elle envoie ensuite ne s'ouvre pas sans la
// clé privée, que le tiers n'a pas.
func TestARelayingGatewayOnlyGetsCiphertext(t *testing.T) {
	real, public := gatewayKeys(t)
	impostor, _ := gatewayKeys(t)
	var impostorSeal *Seal
	gateway := &sealGateway{
		frames: make(chan []byte, 1),
		prove: func(sessionID, partyKey string) (*Seal, string) {
			_, proof, _ := AcceptSeal(real, sessionID, partyKey) // la vraie prouve
			impostorSeal, _, _ = AcceptSeal(impostor, sessionID, partyKey)

			return nil, proof
		},
	}
	client, err := DialSealed(gateway.serve(t), "streamex-1", "streamex", "", public, MediaInfo{}, Handlers{})
	if err != nil {
		t.Fatalf("la preuve relayée est valable, la poignée de main passe : %v", err)
	}
	defer client.Close()
	_ = client.PushVideo([]byte("une image H264"), time.Millisecond)
	frame := <-gateway.frames
	if bytes.Contains(frame, []byte("une image H264")) {
		t.Fatal("l'image est partie en clair")
	}
	if _, _, _, err := impostorSeal.OpenFrame(frame); err == nil {
		t.Fatal("le tiers a ouvert l'image")
	}
}

// LE RÉTRO NE SCELLE PAS : Dial parle à n'importe quelle passerelle, comme
// avant.
func TestAPlainChildStillStreamsInTheClear(t *testing.T) {
	gateway := &sealGateway{frames: make(chan []byte, 1), prove: func(string, string) (*Seal, string) { return nil, "" }}
	client, err := Dial(gateway.serve(t), "retro-1", "retrogaming", "", MediaInfo{}, Handlers{})
	if err != nil {
		t.Fatalf("Dial : %v", err)
	}
	defer client.Close()
	_ = client.PushVideo([]byte("une image H264"), time.Millisecond)
	if frame := <-gateway.frames; !strings.HasSuffix(string(frame), "une image H264") {
		t.Fatalf("image = %q", frame)
	}
}

func TestAGatewayKeyMustBeOne(t *testing.T) {
	if _, err := DialSealed("http://127.0.0.1:1", "s", "streamex", "", "pas-une-clé", MediaInfo{}, Handlers{}); err == nil {
		t.Fatal("une clé illisible a été acceptée")
	}
	if _, err := ParseGatewayPublicKey(DevGatewayKey); err != nil {
		t.Fatalf("DevGatewayKey : %v", err)
	}
}
