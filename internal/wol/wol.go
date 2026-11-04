// Package wol builds and sends Wake-on-LAN magic packets.
package wol

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// PacketSize is the length of a magic packet: 6 bytes of 0xFF plus 16 copies of the MAC.
const PacketSize = 6 + 16*6

// ParseMAC parses a 48-bit MAC address. It accepts ':' or '-' separators
// (AA:BB:CC:DD:EE:FF, aa-bb-cc-dd-ee-ff) or no separators at all.
func ParseMAC(s string) (net.HardwareAddr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("wol: empty MAC address")
	}
	if !strings.ContainsAny(s, ":-.") && len(s) == 12 {
		var b strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(s[i : i+2])
		}
		s = b.String()
	}
	hw, err := net.ParseMAC(s)
	if err != nil {
		return nil, fmt.Errorf("wol: invalid MAC %q: %w", s, err)
	}
	if len(hw) != 6 {
		return nil, fmt.Errorf("wol: MAC %q must be 6 bytes, got %d", s, len(hw))
	}
	return hw, nil
}

// MagicPacket returns the 102-byte magic packet for mac.
func MagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("wol: MAC must be 6 bytes, got %d", len(mac))
	}
	pkt := make([]byte, 0, PacketSize)
	for i := 0; i < 6; i++ {
		pkt = append(pkt, 0xFF)
	}
	for i := 0; i < 16; i++ {
		pkt = append(pkt, mac...)
	}
	return pkt, nil
}

// Send broadcasts a magic packet for mac to addr (host:port, e.g. 192.0.2.255:9).
func Send(addr string, mac net.HardwareAddr) error {
	pkt, err := MagicPacket(mac)
	if err != nil {
		return err
	}
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("wol: dial %s: %w", addr, err)
	}
	defer conn.Close()
	n, err := conn.Write(pkt)
	if err != nil {
		return fmt.Errorf("wol: send: %w", err)
	}
	if n != len(pkt) {
		return fmt.Errorf("wol: short write: %d of %d bytes", n, len(pkt))
	}
	return nil
}
