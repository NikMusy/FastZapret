package strategy

import (
	"github.com/phantom/fastzapret/internal/ipparse"
)

// UDPFake — фейковый UDP-пакет с TTL=N перед оригиналом.
// Используется для QUIC (YouTube/Google), голосового Discord (RTC), Roblox UDP.
type UDPFake struct {
	TTL      uint8
	FakeData []byte // если пусто — заполним мусором длиной как у оригинала
}

func (s UDPFake) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoUDP {
		return
	}
	if out.used >= len(out.Pkts) {
		return
	}
	dst := out.Bufs[out.used]
	hdrLen := p.PayloadOf
	// Размер фейкового payload
	payload := s.FakeData
	if len(payload) == 0 {
		// заполним нулями ровно той же длины — minimum disruption
		if p.PayloadLn < cap(dst)-hdrLen {
			payload = make([]byte, p.PayloadLn) // одна аллокация при первом запуске на воркере недопустима, но FakeData обычно пресет
		} else {
			return
		}
	}
	totalLen := hdrLen + len(payload)
	if totalLen > cap(dst) {
		return
	}
	dst = dst[:totalLen]
	copy(dst, p.Buf[:hdrLen])
	copy(dst[hdrLen:], payload)
	// Длины
	if p.Version == 4 {
		ipparse.PutIPv4TotalLen(dst, totalLen)
		dst[10] = 0
		dst[11] = 0
	} else {
		ipparse.PutIPv6PayloadLen(dst, totalLen-40)
	}
	ipparse.PutUDPLen(dst, p.L4Off, len(payload)+8)
	// TTL
	if s.TTL > 0 {
		if p.Version == 4 {
			dst[8] = s.TTL
		} else {
			dst[7] = s.TTL
		}
	}
	ipparse.RecalcChecksums(dst)
	out.Pkts[out.used].Data = dst
	out.Pkts[out.used].TTL = s.TTL
	out.used++
}
