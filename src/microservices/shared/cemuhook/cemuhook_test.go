package cemuhook

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// clientRequest builds a DSU client request (the header parseRequest reads:
// magic, protocol version, client id, message type).
func clientRequest(messageType uint32, clientID uint32) []byte {
	packet := make([]byte, 20)
	copy(packet[0:4], magicClient)
	binary.LittleEndian.PutUint16(packet[4:6], protoVer)
	binary.LittleEndian.PutUint32(packet[12:16], clientID)
	binary.LittleEndian.PutUint32(packet[16:20], messageType)
	return packet
}

// TestServe_DataRequestRegistersWithoutReplying: a data request must only
// register the client. Replying to it fed a loop with Cemu, which requests
// again after every message it receives.
func TestServe_DataRequestRegistersWithoutReplying(t *testing.T) {
	server, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.serve(func(pad int) (Pad, bool) { return Pad{}, pad == 0 })

	client, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: server.Port()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	reply := make([]byte, 128)

	// The version request still gets its answer: the protocol handshake.
	if _, err := client.Write(clientRequest(typeVersion, 7)); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(reply); err != nil {
		t.Fatalf("no version reply: %v", err)
	}

	// The data request gets nothing back, but registers the client.
	if _, err := client.Write(clientRequest(typePadData, 7)); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if n, err := client.Read(reply); err == nil {
		t.Fatalf("data request was answered with %d bytes", n)
	}
	server.mu.Lock()
	registered := server.client != nil && server.clientID == 7
	server.mu.Unlock()
	if !registered {
		t.Fatal("data request did not register the client")
	}
}

// TestRun_EndsOnClose: the phone opens a server per game and closes it at
// the stop — its push loop must not outlive the socket.
func TestRun_EndsOnClose(t *testing.T) {
	server, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Logf = nil
	ended := make(chan struct{})
	go func() {
		server.Run(func(pad int) (Pad, bool) { return Pad{}, true })
		close(ended)
	}()
	server.Close()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("Run still pushing after Close")
	}
}
