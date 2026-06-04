package ipparse

import "testing"

// Минимальный IPv4 + TCP + TLS ClientHello-байт.
func TestParseIPv4TCPTLS(t *testing.T) {
	// IPv4 заголовок 20 байт + TCP 20 + payload 6 = 46.
	pkt := make([]byte, 46)
	pkt[0] = 0x45        // version=4, ihl=5
	pkt[2], pkt[3] = 0, 46
	pkt[8] = 64          // TTL
	pkt[9] = ProtoTCP
	// IPs (mock)
	pkt[12] = 10
	pkt[16] = 8
	pkt[16+3] = 8
	// TCP
	tcp := pkt[20:]
	tcp[0], tcp[1] = 0xc0, 0x00 // src 49152
	tcp[2], tcp[3] = 0x01, 0xbb // dst 443
	tcp[12] = 5 << 4            // data offset 20
	tcp[13] = TCPFlagPSH | TCPFlagACK
	// payload — TLS ClientHello signature
	payload := pkt[40:]
	payload[0] = 0x16
	payload[1] = 0x03
	payload[2] = 0x01
	payload[5] = 0x01

	var p Packet
	if !Parse(pkt, &p) {
		t.Fatal("parse failed")
	}
	if p.Proto != ProtoTCP {
		t.Errorf("proto=%d", p.Proto)
	}
	if p.DstPort != 443 {
		t.Errorf("dst=%d", p.DstPort)
	}
	if p.PayloadLn != 6 {
		t.Errorf("payload len=%d", p.PayloadLn)
	}
	if !IsTLSClientHello(p.Payload()) {
		t.Error("ClientHello not detected")
	}
}

func TestChecksumIPv4(t *testing.T) {
	// Минимальный IP-заголовок без опций.
	hdr := []byte{
		0x45, 0x00, 0x00, 0x28,
		0x00, 0x00, 0x40, 0x00,
		0x40, 0x06, 0x00, 0x00, // chksum=0
		192, 168, 0, 1,
		8, 8, 8, 8,
	}
	cs := checksum16(hdr)
	if cs == 0 {
		t.Error("zero checksum suspicious")
	}
}
