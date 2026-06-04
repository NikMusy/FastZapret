package strategy

import (
	"github.com/phantom/fastzapret/internal/ipparse"
)

// TCPFake — отправляет fake TCP-пакет с TTL=N перед оригиналом.
// Fake-пакет содержит фейковый TLS ClientHello (или произвольные байты),
// которые DPI обработает раньше реального запроса.
// Реальный пакет уходит после.
type TCPFake struct {
	TTL      uint8  // 1..8
	FakeData []byte // что положить в payload фейка
}

// fakeTLS — минимальный фейковый TLS ClientHello с SNI=www.google.com.
// DPI его прожуёт и решит что это легитимное соединение к google.
var fakeTLS = []byte{
	0x16, 0x03, 0x01, 0x00, 0x75, // record: handshake, TLS 1.0, len=117
	0x01, 0x00, 0x00, 0x71, // handshake: client_hello, len=113
	0x03, 0x03, // legacy_version TLS 1.2
	// random 32 bytes
	0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11,
	0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
	0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11,
	0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
	0x00,             // session_id_length
	0x00, 0x02, 0x13, 0x01, // cipher_suites: TLS_AES_128_GCM_SHA256
	0x01, 0x00, // compression_methods: null
	0x00, 0x46, // extensions length=70
	// server_name extension
	0x00, 0x00, 0x00, 0x12, 0x00, 0x10, 0x00, 0x00, 0x0d,
	'w', 'w', 'w', '.', 'g', 'o', 'o', 'g', 'l', 'e', '.', 'c', 'o', 'm',
	// supported_versions: TLS 1.3
	0x00, 0x2b, 0x00, 0x03, 0x02, 0x03, 0x04,
	// supported_groups
	0x00, 0x0a, 0x00, 0x04, 0x00, 0x02, 0x00, 0x1d,
	// signature_algorithms
	0x00, 0x0d, 0x00, 0x04, 0x00, 0x02, 0x04, 0x03,
	// key_share
	0x00, 0x33, 0x00, 0x26, 0x00, 0x24, 0x00, 0x1d, 0x00, 0x20,
	0x35, 0x80, 0x72, 0xd6, 0x36, 0x58, 0x80, 0xd1, 0xae, 0xea,
	0x32, 0x9a, 0xdf, 0x91, 0x21, 0x38, 0x38, 0x51, 0xed, 0x21,
	0xa2, 0x8e, 0x3b, 0x75, 0xe9, 0x65, 0xd0, 0xd2, 0xcd, 0x16,
	0x62, 0x54,
}

func (s TCPFake) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoTCP {
		return
	}
	data := s.FakeData
	if len(data) == 0 {
		data = fakeTLS
	}
	// Берём буфер из пула батча, копируем заголовок + подменяем payload.
	if out.used >= len(out.Pkts) {
		return
	}
	dst := out.Bufs[out.used]
	hdrLen := p.PayloadOf
	totalLen := hdrLen + len(data)
	if totalLen > cap(dst) {
		return
	}
	dst = dst[:totalLen]
	copy(dst, p.Buf[:hdrLen])
	copy(dst[hdrLen:], data)
	// Правим длины
	if p.Version == 4 {
		ipparse.PutIPv4TotalLen(dst, totalLen)
		// обнуляем IP-чек
		dst[10] = 0
		dst[11] = 0
	} else {
		ipparse.PutIPv6PayloadLen(dst, totalLen-40)
	}
	// TTL
	if s.TTL > 0 {
		if p.Version == 4 {
			dst[8] = s.TTL
		} else {
			dst[7] = s.TTL
		}
	}
	// Пересчёт чек-сумм
	ipparse.RecalcChecksums(dst)
	out.Pkts[out.used].Data = dst
	out.Pkts[out.used].TTL = s.TTL
	out.used++
}
