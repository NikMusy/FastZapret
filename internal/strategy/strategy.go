// Package strategy — DPI-bypass стратегии. Каждая стратегия получает один
// разобранный пакет и возвращает 1 или более пакетов на отправку.
// Контракт: стратегия НЕ должна аллоцировать в горячем пути — она пишет
// в pre-allocated batch buffer.
package strategy

import (
	"encoding/binary"

	"github.com/phantom/fastzapret/internal/ipparse"
)

// OutPacket — описание одного исходящего пакета в батче.
type OutPacket struct {
	Data []byte
	TTL  uint8 // 0 = оригинал
}

// Batch — приёмник результатов. Стратегия пишет в Pkts[:n].
type Batch struct {
	Pkts  []OutPacket
	Bufs  [][]byte // pre-allocated буферы для копий пакета
	used  int
}

// NewBatch создаёт батч с N пред-выделенными буферами по 1600 байт.
func NewBatch(maxPkts int) *Batch {
	b := &Batch{
		Pkts: make([]OutPacket, maxPkts),
		Bufs: make([][]byte, maxPkts),
	}
	for i := range b.Bufs {
		b.Bufs[i] = make([]byte, 1600)
	}
	return b
}

// Reset сбрасывает счётчик; буферы не освобождает.
func (b *Batch) Reset() { b.used = 0 }

// Add копирует пакет в свой буфер и регистрирует его.
// ttl: 0 — оставить как есть, иначе перезаписать поле TTL.
func (b *Batch) Add(src []byte, ttl uint8) {
	if b.used >= len(b.Pkts) {
		return
	}
	dst := b.Bufs[b.used][:len(src)]
	copy(dst, src)
	if ttl != 0 {
		setTTL(dst, ttl)
	}
	b.Pkts[b.used].Data = dst
	b.Pkts[b.used].TTL = ttl
	b.used++
}

// Out возвращает использованные пакеты.
func (b *Batch) Out() []OutPacket { return b.Pkts[:b.used] }

func setTTL(buf []byte, ttl uint8) {
	if len(buf) < 20 {
		return
	}
	v := buf[0] >> 4
	switch v {
	case 4:
		buf[8] = ttl
		// IP-checksum нужно пересчитать
		buf[10] = 0
		buf[11] = 0
		ihl := int(buf[0]&0x0F) * 4
		if ihl <= len(buf) {
			binary.BigEndian.PutUint16(buf[10:12], ipparse.RecalcIPv4Header(buf[:ihl]))
		}
	case 6:
		buf[7] = ttl
	}
}

// Op — единичная операция в pipeline стратегии.
type Op interface {
	Apply(p *ipparse.Packet, out *Batch)
}

// Pipeline — последовательная композиция стратегий.
// Поведение: каждая Op пишет в общий батч. Финальный батч и есть результат.
// Если пакет надо отбросить (uplink заменён фейками+сплитами), оригинал не добавляется.
type Pipeline struct {
	Ops []Op
	// EmitOriginal — добавлять ли оригинал в конец батча. False, если pipeline
	// сам генерирует замену (fake+split разбивают пакет, оригинал не нужен).
	EmitOriginal bool
}

// Apply применяет все Op по очереди.
func (pl Pipeline) Apply(p *ipparse.Packet, out *Batch) {
	out.Reset()
	for _, op := range pl.Ops {
		op.Apply(p, out)
	}
	if pl.EmitOriginal {
		out.Add(p.Buf[:p.TotalLen], 0)
	}
}
