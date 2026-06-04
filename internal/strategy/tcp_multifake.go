package strategy

import (
	"github.com/phantom/fastzapret/internal/ipparse"
)

// TCPMultiFake — серия фейковых TLS ClientHello'в подряд с разными TTL.
// Каждый фейк ловится DPI на разном hop'е сети, что максимизирует
// шанс пройти даже многоуровневую инспекцию.
type TCPMultiFake struct {
	TTLs []uint8 // напр. {2, 4, 6}
}

func (s TCPMultiFake) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoTCP {
		return
	}
	for _, ttl := range s.TTLs {
		TCPFake{TTL: ttl}.Apply(p, out)
	}
}

// TCPFakeBadChecksum — фейк с заведомо невалидным TCP checksum.
// Реальный получатель (сервер) такой пакет дропнет, а DPI на пути
// обычно не проверяет L4 checksum и принимает за легитимный поток.
// Это эффективно против современных DPI с stateful TCP reassembly.
type TCPFakeBadChecksum struct {
	TTL uint8 // если > 0 — комбинируем с TTL-trick'ом
}

func (s TCPFakeBadChecksum) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoTCP {
		return
	}
	if out.used >= len(out.Pkts) {
		return
	}
	dst := out.Bufs[out.used]
	hdrLen := p.PayloadOf
	totalLen := hdrLen + len(fakeTLS)
	if totalLen > cap(dst) {
		return
	}
	dst = dst[:totalLen]
	copy(dst, p.Buf[:hdrLen])
	copy(dst[hdrLen:], fakeTLS)
	if p.Version == 4 {
		ipparse.PutIPv4TotalLen(dst, totalLen)
		dst[10] = 0
		dst[11] = 0
	} else {
		ipparse.PutIPv6PayloadLen(dst, totalLen-40)
	}
	if s.TTL > 0 {
		if p.Version == 4 {
			dst[8] = s.TTL
		} else {
			dst[7] = s.TTL
		}
	}
	ipparse.RecalcChecksums(dst)
	// Намеренно ломаем TCP checksum (XOR двух байт).
	dst[p.L4Off+16] ^= 0xFF
	dst[p.L4Off+17] ^= 0xFF
	out.Pkts[out.used].Data = dst
	out.Pkts[out.used].TTL = s.TTL
	out.used++
}

// TLSRecordSplit — разрезает TLS на уровне TLS Record:
// первая часть содержит ClientHello header (5 байт), вторая — тело.
// DPI без полной сборки TLS не увидит SNI.
type TLSRecordSplit struct{}

func (s TLSRecordSplit) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoTCP || p.PayloadLn < 6 {
		out.Add(p.Buf[:p.TotalLen], 0)
		return
	}
	// Режем по позиции 5 (за TLS Record Header).
	TCPSplit{Position: 5}.Apply(p, out)
}
