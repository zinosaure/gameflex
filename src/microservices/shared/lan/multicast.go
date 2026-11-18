package lan

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"

	"github.com/wlynxg/anet"
	"golang.org/x/net/ipv4"
)

// mdnsGroup — le groupe et le port de mDNS en IPv4.
var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// multicastSocket — le socket de 5353, partagé avec qui le tient déjà (avahi,
// le système du téléphone), abonné au groupe sur chaque interface du réseau
// local.
type multicastSocket struct {
	conn       net.PacketConn
	packet     *ipv4.PacketConn
	interfaces []net.Interface
}

// lanInterfaces — les interfaces montées qui font du multicast, sans le
// bouclage ni les ponts et tunnels des conteneurs et des VPN : le même tri que
// la découverte des PlayStation (playstation/discovery).
func lanInterfaces() ([]net.Interface, error) {
	all, err := anet.Interfaces()
	if err != nil {
		return nil, err
	}
	kept := []net.Interface{}
	for _, candidate := range all {
		if candidate.Flags&net.FlagUp == 0 || candidate.Flags&net.FlagLoopback != 0 || candidate.Flags&net.FlagMulticast == 0 {
			continue
		}
		name := strings.ToLower(candidate.Name)
		skipped := false
		for _, virtual := range []string{"docker", "veth", "br-", "virbr", "vmnet", "vboxnet", "tun", "tap", "wg"} {
			skipped = skipped || strings.Contains(name, virtual)
		}
		if !skipped {
			kept = append(kept, candidate)
		}
	}

	return kept, nil
}

// reuse — SO_REUSEADDR et SO_REUSEPORT : 5353 se partage, chaque socket abonné
// reçoit chaque paquet du groupe.
func reuse(_, _ string, raw syscall.RawConn) error {
	var failure error
	err := raw.Control(func(descriptor uintptr) {
		if failure = syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); failure != nil {
			return
		}
		failure = syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, soReusePort, 1)
	})
	if err != nil {
		return err
	}

	return failure
}

// openMulticast — le socket de 5353, abonné au groupe.
func openMulticast() (*multicastSocket, error) {
	interfaces, err := lanInterfaces()
	if err != nil {
		return nil, err
	}
	config := net.ListenConfig{Control: reuse}
	conn, err := config.ListenPacket(context.Background(), "udp4", "0.0.0.0:5353")
	if err != nil {
		return nil, err
	}
	packet := ipv4.NewPacketConn(conn)
	joined := []net.Interface{}
	for _, candidate := range interfaces {
		if packet.JoinGroup(&candidate, &net.UDPAddr{IP: mdnsGroup.IP}) == nil {
			joined = append(joined, candidate)
		}
	}
	if len(joined) == 0 {
		conn.Close()

		return nil, errors.New("lan: no network interface joins the mDNS group")
	}
	_ = packet.SetMulticastTTL(255)
	_ = packet.SetMulticastLoopback(true)

	return &multicastSocket{conn: conn, packet: packet, interfaces: joined}, nil
}

// Send — au groupe, sur chaque interface abonnée.
func (socket *multicastSocket) Send(payload []byte) error {
	var last error
	sent := false
	for index := range socket.interfaces {
		if err := socket.packet.SetMulticastInterface(&socket.interfaces[index]); err != nil {
			last = err

			continue
		}
		if _, err := socket.conn.WriteTo(payload, mdnsGroup); err != nil {
			last = err

			continue
		}
		sent = true
	}
	if sent {
		return nil
	}

	return last
}

// Receive — le prochain paquet du groupe, et son adresse d'origine.
func (socket *multicastSocket) Receive() ([]byte, net.IP, error) {
	buffer := make([]byte, 9000)
	for {
		read, from, err := socket.conn.ReadFrom(buffer)
		if err != nil {
			return nil, nil, err
		}
		if address, ok := from.(*net.UDPAddr); ok {
			return append([]byte(nil), buffer[:read]...), address.IP, nil
		}
	}
}

// Close — le socket.
func (socket *multicastSocket) Close() error {
	return socket.conn.Close()
}
