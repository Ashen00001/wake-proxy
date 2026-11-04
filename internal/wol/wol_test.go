package wol

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestParseMAC(t *testing.T) {
	want := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	for _, in := range []string{
		"AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", "aabbccddeeff", "  AA:BB:CC:DD:EE:FF\n",
	} {
		got, err := ParseMAC(in)
		if err != nil {
			t.Errorf("ParseMAC(%q) error: %v", in, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("ParseMAC(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseMACErrors(t *testing.T) {
	for _, in := range []string{
		"", "   ", "AA:BB:CC:DD:EE", "AA:BB:CC:DD:EE:GG", "not a mac", "aabbccddee",
		"AA:BB:CC:DD:EE:FF:00:11",
	} {
		if _, err := ParseMAC(in); err == nil {
			t.Errorf("ParseMAC(%q) expected error", in)
		}
	}
}

func TestMagicPacket(t *testing.T) {
	mac := net.HardwareAddr{1, 2, 3, 4, 5, 6}
	pkt, err := MagicPacket(mac)
	if err != nil {
		t.Fatal(err)
	}
	if PacketSize != 102 || len(pkt) != 102 {
		t.Fatalf("len = %d, want 102", len(pkt))
	}
	if !bytes.Equal(pkt[:6], bytes.Repeat([]byte{0xFF}, 6)) {
		t.Errorf("bad header: %x", pkt[:6])
	}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(pkt[6+i*6:12+i*6], mac) {
			t.Errorf("repetition %d wrong: %x", i, pkt[6+i*6:12+i*6])
		}
	}
}

func TestMagicPacketBadMAC(t *testing.T) {
	if _, err := MagicPacket(net.HardwareAddr{1, 2, 3}); err == nil {
		t.Error("expected error for short MAC")
	}
}

func TestSendLoopback(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("udp loopback unavailable:", err)
	}
	defer pc.Close()
	mac := net.HardwareAddr{1, 2, 3, 4, 5, 6}
	if err := Send(pc.LocalAddr().String(), mac); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256)
	pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := MagicPacket(mac)
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("received packet differs from magic packet")
	}
}
