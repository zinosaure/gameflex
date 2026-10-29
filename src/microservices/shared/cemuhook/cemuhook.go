// Package cemuhook implements the SERVER side of the DSU (cemuhook) UDP
// protocol: the emulator (e.g. Eden/yuzu's "UDP Controller") connects as a
// client to 127.0.0.1:<port> and we push pad data back to it. The port is
// the session's own (bound on ":0", see Port) — never a fixed one, since
// session containers share the host's network.
//
// SHARED WITH THE PHONE (#441, 2026-10-23): streamex serves its uinput pads'
// state here, and the phone's kernel serves the gestures of its players — up
// to four, where its X keyboard only had keys for two.
//
// Wire format (see eden src/input_common/helpers/udp_protocol.h):
//
//	header 20B: magic "DSUS" | u16 proto=1001 | u16 payload_len (=data+4) |
//	            u32 crc32(IEEE, over the whole message with crc=0) |
//	            u32 client_id (echoed) | u32 type
//	Version   data: u16 version
//	PortInfo  data: u8 id | u8 state(2=connected) | u8 model(4=generic) |
//	                 u8 conn(1=usb) | mac[6] | u8 battery(5=full) | u8 active
//	PadData   data: PortInfo | u32 seq | u16 buttons | u8 home | u8 touch |
//	                 4×u8 sticks | 12×u8 analog | 2×TouchPad(6B) |
//	                 u64 motion_ts | 3×f32 accel | 3×f32 gyro   (80 bytes)
package cemuhook

import (
	"encoding/binary"
	"hash/crc32"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	magicServer = "DSUS"
	magicClient = "DSUC"
	protoVer    = 1001

	typeVersion  = 0x00100000
	typePortInfo = 0x00100001
	typePadData  = 0x00100002

	// DSU digital button bits (eden's 16-entry table order).
	bitShare    = 1 << 0
	bitL3       = 1 << 1
	bitR3       = 1 << 2
	bitOptions  = 1 << 3
	bitUp       = 1 << 4
	bitRight    = 1 << 5
	bitDown     = 1 << 6
	bitLeft     = 1 << 7
	bitL2       = 1 << 8
	bitR2       = 1 << 9
	bitL1       = 1 << 10
	bitR1       = 1 << 11
	bitTriangle = 1 << 12
	bitCircle   = 1 << 13
	bitCross    = 1 << 14
	bitSquare   = 1 << 15
)

// Pad is one controller's state — the same fields as streamex's
// gamepad.State, which converts to it as is.
type Pad struct {
	A, B, X, Y            bool // face buttons, by position (A east, B south)
	L1, R1, L3, R3        bool // shoulders / stick clicks
	Select, Start, PS     bool
	Up, Down, Left, Right bool  // d-pad
	LX, LY, RX, RY        int32 // sticks, -32767..32767 (Y positive = down)
	L2, R2                int32 // triggers, 0..255
}

// Snapshots supplies the pad states (index = player slot).
type Snapshots func(pad int) (Pad, bool)

// Server is the DSU UDP server. Eden polls it every 3 s; the Run loop also
// pushes pad data every 8 ms for low-latency analog updates.
type Server struct {
	conn      *net.UDPConn
	done      chan struct{}
	closeOnce sync.Once

	// Logf receives the traffic, every 2 s: log.Printf for streamex (set by
	// Listen), nil for silence — the phone asks Client instead.
	Logf func(format string, args ...any)

	mu       sync.Mutex
	client   *net.UDPAddr
	clientID uint32
	seq      uint32

	pushes atomic.Uint64 // pad messages sent (debug)
	polls  atomic.Uint64 // client requests received (debug)
}

// Listen binds the DSU server socket.
func Listen(addr string) (*Server, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		return nil, err
	}
	return &Server{conn: conn, done: make(chan struct{}), Logf: log.Printf}, nil
}

// Client returns the emulator's address once it has asked for pads, nil
// before.
func (s *Server) Client() *net.UDPAddr {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.client
}

// Port returns the UDP port actually bound — the one the kernel picked when
// Listen was given ":0".
func (s *Server) Port() int {
	return s.conn.LocalAddr().(*net.UDPAddr).Port
}

// Run serves requests and pushes pad data. Blocking, until Close.
func (s *Server) Run(snap Snapshots) {
	go s.serve(snap)
	tick := time.NewTicker(8 * time.Millisecond)
	defer tick.Stop()
	// Periodic diagnostics: pushes/polls per second (helps spot stalls).
	var lastPush, lastPoll uint64
	stats := time.NewTicker(2 * time.Second)
	defer stats.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-stats.C:
			if s.Logf == nil {
				continue
			}
			pushes := s.pushes.Load()
			polls := s.polls.Load()
			s.mu.Lock()
			client := s.client
			seq := s.seq
			s.mu.Unlock()
			l := Pad{}
			if st, ok := snap(0); ok {
				l = st
			}
			s.Logf("[cemuhook] pushes=%d/s polls=%d/s client=%v seq=%d P1=(%d,%d)/(%d,%d)",
				(pushes-lastPush)/2, (polls-lastPoll)/2, client, seq,
				l.LX, l.LY, l.RX, l.RY)
			lastPush, lastPoll = pushes, polls
		case <-tick.C:
			s.mu.Lock()
			client := s.client
			clientID := s.clientID
			s.mu.Unlock()
			if client == nil {
				continue
			}
			s.mu.Lock()
			s.seq++
			seq := s.seq
			s.mu.Unlock()
			for i := 0; i < 4; i++ {
				st, ok := snap(i)
				if !ok {
					continue
				}
				msg := padDataMessage(i, seq, clientID, st)
				if _, err := s.conn.WriteToUDP(msg, client); err == nil {
					s.pushes.Add(1)
				}
			}
		}
	}
}

func (s *Server) serve(snap Snapshots) {
	buf := make([]byte, 128)
	for {
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		reqType, clientID, ok := parseRequest(buf[:n])
		if !ok {
			continue
		}
		s.polls.Add(1)
		s.mu.Lock()
		s.client = from
		s.clientID = clientID
		s.mu.Unlock()

		switch reqType {
		case typeVersion:
			_, _ = s.conn.WriteToUDP(versionMessage(clientID), from)
		case typePortInfo:
			for i := 0; i < 4; i++ {
				_, _ = s.conn.WriteToUDP(portInfoMessage(i, clientID), from)
			}
		case typePadData:
			// A data request only (re)registers the client above — no reply.
			// Run already pushes every pad at 125 Hz. Answering each request
			// with four packets made a feedback loop with Cemu, whose reader
			// sends a new request after EVERY message it receives: 47 000
			// requests per second, Cemu at 358 % of a core and streamex at
			// 233 % on an idle Wii U session (2026-09-11). Eden polls every
			// 3 s and lives on the pushes, so it never needed the replies.
		}
	}
}

// Close releases the socket and ends Run.
func (s *Server) Close() error {
	s.closeOnce.Do(func() { close(s.done) })

	return s.conn.Close()
}

// parseRequest validates a client request and returns (type, clientID).
func parseRequest(pkt []byte) (uint32, uint32, bool) {
	if len(pkt) < 20 || string(pkt[:4]) != magicClient {
		return 0, 0, false
	}
	if binary.LittleEndian.Uint16(pkt[4:6]) != protoVer {
		return 0, 0, false
	}
	typ := binary.LittleEndian.Uint32(pkt[16:20])
	if typ < typeVersion || typ > typePadData {
		return 0, 0, false
	}
	return typ, binary.LittleEndian.Uint32(pkt[12:16]), true
}

// message builds a DSU message (header + data) with the proper CRC.
func message(typ uint32, clientID uint32, data []byte) []byte {
	payloadLen := len(data) + 4
	msg := make([]byte, 20+len(data))
	copy(msg[0:4], magicServer)
	binary.LittleEndian.PutUint16(msg[4:6], protoVer)
	binary.LittleEndian.PutUint16(msg[6:8], uint16(payloadLen))
	// msg[8:12] crc left zero for the computation
	binary.LittleEndian.PutUint32(msg[12:16], clientID)
	binary.LittleEndian.PutUint32(msg[16:20], typ)
	copy(msg[20:], data)
	crc := crc32.ChecksumIEEE(msg)
	binary.LittleEndian.PutUint32(msg[8:12], crc)
	return msg
}

func versionMessage(clientID uint32) []byte {
	data := make([]byte, 2)
	binary.LittleEndian.PutUint16(data, protoVer)
	return message(typeVersion, clientID, data)
}

func portInfoMessage(id int, clientID uint32) []byte {
	data := make([]byte, 12)
	data[0] = byte(id)
	data[1] = 2 // state: connected
	data[2] = 4 // model: generic
	data[3] = 1 // connection type: USB
	data[8] = 5 // battery: full
	data[9] = 1 // active
	return message(typePortInfo, clientID, data)
}

// padDataMessage serializes one controller update. The response PadData struct
// is 80 bytes (PortInfo 12 + packet_counter 4 + buttons/home 4 + sticks 4 +
// analog buttons 12 + 2 touchpads 12 + motion timestamp 8 + accel 12 + gyro
// 12); payload_length must be 80 + sizeof(Type) = 84.
func padDataMessage(id int, seq uint32, clientID uint32, st Pad) []byte {
	data := make([]byte, 80)
	data[0] = byte(id)
	data[1] = 2 // state: connected
	data[2] = 4 // model: generic
	data[3] = 1 // connection type: USB
	data[8] = 5 // battery: full
	data[9] = 1 // active
	binary.LittleEndian.PutUint32(data[12:16], seq)

	var buttons uint16
	set := func(bit uint16, on bool) {
		if on {
			buttons |= bit
		}
	}
	set(bitShare, st.Select)
	set(bitL3, st.L3)
	set(bitR3, st.R3)
	set(bitOptions, st.Start)
	set(bitUp, st.Up)
	set(bitRight, st.Right)
	set(bitDown, st.Down)
	set(bitLeft, st.Left)
	set(bitL2, st.L2 >= 128)
	set(bitR2, st.R2 >= 128)
	set(bitL1, st.L1)
	set(bitR1, st.R1)
	set(bitTriangle, st.X)
	set(bitCircle, st.A)
	set(bitCross, st.B)
	set(bitSquare, st.Y)
	binary.LittleEndian.PutUint16(data[16:18], buttons)

	if st.PS {
		data[18] = 1
	}

	// DSU Y axes point up = 255 (screen coords): the pad delivers
	// up = negative, so negate before encoding.
	data[20] = stickByte(st.LX)
	data[21] = stickByte(-st.LY)
	data[22] = stickByte(st.RX)
	data[23] = stickByte(-st.RY)

	// analog buttons: dpad_left/down/right/up, square, cross, circle,
	// triangle, r1, l1, r2, l2
	put255 := func(off int, on bool) {
		if on {
			data[off] = 255
		}
	}
	put255(24, st.Left)
	put255(25, st.Down)
	put255(26, st.Right)
	put255(27, st.Up)
	put255(28, st.Y)
	put255(29, st.B)
	put255(30, st.A)
	put255(31, st.X)
	put255(32, st.R1)
	put255(33, st.L1)
	data[34] = byte(clamp(st.R2, 0, 255))
	data[35] = byte(clamp(st.L2, 0, 255))

	return message(typePadData, clientID, data)
}

// stickByte converts a signed stick value to the DSU byte range (127 center).
func stickByte(v int32) byte {
	x := v * 127 / 32767
	return byte(clamp(x+127, 0, 255))
}

func clamp(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
