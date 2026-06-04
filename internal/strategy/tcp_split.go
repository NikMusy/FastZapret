package strategy

import (
	"github.com/phantom/fastzapret/internal/ipparse"
)

// TCPSplit разбивает payload на 2 части в позиции Position.
// Position=1 — известный обход «split2» в zapret: 1 байт + остальное.
// Position может быть > 0; если payload меньше — split не применяется.
// Disorder=true — отправить вторую часть РАНЬШЕ первой (разупорядочивание).
type TCPSplit struct {
	Position int
	Disorder bool
}

func (s TCPSplit) Apply(p *ipparse.Packet, out *Batch) {
	if p.Proto != ipparse.ProtoTCP || p.PayloadLn <= s.Position || s.Position <= 0 {
		// нечего делить — добавим оригинал, чтобы пакет не потерялся
		out.Add(p.Buf[:p.TotalLen], 0)
		return
	}
	// Часть 1: исходный seq, payload[:Position]
	// Часть 2: seq+Position, payload[Position:]
	mk := func(start, end int, seqOff uint32) []byte {
		if out.used >= len(out.Pkts) {
			return nil
		}
		dst := out.Bufs[out.used]
		hdrLen := p.PayloadOf
		size := end - start
		totalLen := hdrLen + size
		if totalLen > cap(dst) {
			return nil
		}
		dst = dst[:totalLen]
		copy(dst, p.Buf[:hdrLen])
		copy(dst[hdrLen:], p.Buf[p.PayloadOf+start:p.PayloadOf+end])
		// Длины
		if p.Version == 4 {
			ipparse.PutIPv4TotalLen(dst, totalLen)
			dst[10] = 0
			dst[11] = 0
		} else {
			ipparse.PutIPv6PayloadLen(dst, totalLen-40)
		}
		// Seq
		ipparse.PutSeq(dst, p.L4Off, p.TCPSeq+seqOff)
		ipparse.RecalcChecksums(dst)
		out.Pkts[out.used].Data = dst
		out.Pkts[out.used].TTL = 0
		out.used++
		return dst
	}

	if s.Disorder {
		mk(s.Position, p.PayloadLn, uint32(s.Position))
		mk(0, s.Position, 0)
	} else {
		mk(0, s.Position, 0)
		mk(s.Position, p.PayloadLn, uint32(s.Position))
	}
}
