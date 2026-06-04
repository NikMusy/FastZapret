// Worker — горячий цикл одного потока: RecvBatch → парсинг → стратегия → SendBatch.
// На каждый CPU-ядро запускаем отдельный Worker со своим хэндлом,
// что позволяет распараллелить обработку и обойти Windows IO bottleneck.
package divert

import (
	"context"
	"sync/atomic"

	"github.com/phantom/fastzapret/internal/ipparse"
	"github.com/phantom/fastzapret/internal/strategy"
)

const (
	BatchSize    = 64    // пакетов за один RecvEx — компромисс между задержкой и пропускной
	PacketMaxLen = 1600  // под MTU
	RingBytes    = BatchSize * 4096
)

// Router выбирает стратегию по пакету (например, по DstPort или SNI).
type Router interface {
	Pick(p *ipparse.Packet) strategy.Pipeline
	// PassThrough возвращает true для пакетов которые не надо трогать
	// (пусть улетают как есть). Это даёт максимум скорости для нерелевантного трафика.
	PassThrough(p *ipparse.Packet) bool
}

// Stats — простые счётчики, безопасны для атомарного доступа.
type Stats struct {
	RxPackets uint64
	TxPackets uint64
	Modified  uint64
	Errors    uint64
}

// Worker — один поток.
type Worker struct {
	ID     int
	Handle *Handle
	Router Router
	Stats  *Stats

	rxBuf   []byte
	rxAddrs []Address
	batch   *strategy.Batch
	txBuf   []byte
	txAddrs []Address
}

// NewWorker создаёт воркер.
func NewWorker(id int, h *Handle, r Router, st *Stats) *Worker {
	return &Worker{
		ID:      id,
		Handle:  h,
		Router:  r,
		Stats:   st,
		rxBuf:   make([]byte, RingBytes),
		rxAddrs: make([]Address, BatchSize),
		batch:   strategy.NewBatch(BatchSize * 4),
		txBuf:   make([]byte, RingBytes*4),
		txAddrs: make([]Address, BatchSize*4),
	}
}

// Run — горячий цикл. Завершается при отмене ctx или ошибке.
func (w *Worker) Run(ctx context.Context) error {
	var pkt ipparse.Packet
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		nBytes, nPkts, err := w.Handle.RecvBatch(w.rxBuf, w.rxAddrs)
		if err != nil {
			atomic.AddUint64(&w.Stats.Errors, 1)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if nPkts == 0 {
			continue
		}
		atomic.AddUint64(&w.Stats.RxPackets, uint64(nPkts))

		// Разбираем батч на отдельные пакеты по адресам
		// WinDivert упаковывает пакеты подряд, длина каждого — в IP-заголовке
		txOff := 0
		txCount := 0
		off := 0
		for i := 0; i < nPkts && off < nBytes; i++ {
			addr := &w.rxAddrs[i]
			rem := w.rxBuf[off:nBytes]
			ok := ipparse.Parse(rem, &pkt)
			if !ok {
				// Не разобрали — пропускаем как есть.
				// Считаем длину по IPv4 total / IPv6 payload+40.
				plen := guessLen(rem)
				if plen == 0 || plen > len(rem) {
					break
				}
				copy(w.txBuf[txOff:], rem[:plen])
				w.txAddrs[txCount] = *addr
				txOff += plen
				txCount++
				off += plen
				continue
			}
			plen := pkt.TotalLen
			if plen > len(rem) {
				plen = len(rem)
			}

			if w.Router.PassThrough(&pkt) {
				copy(w.txBuf[txOff:], rem[:plen])
				w.txAddrs[txCount] = *addr
				txOff += plen
				txCount++
				off += plen
				continue
			}

			pipeline := w.Router.Pick(&pkt)
			pipeline.Apply(&pkt, w.batch)
			outs := w.batch.Out()
			if len(outs) == 0 {
				// стратегия ничего не выдала → пропускаем оригинал
				copy(w.txBuf[txOff:], rem[:plen])
				w.txAddrs[txCount] = *addr
				txOff += plen
				txCount++
			} else {
				atomic.AddUint64(&w.Stats.Modified, 1)
				for _, o := range outs {
					n := copy(w.txBuf[txOff:], o.Data)
					w.txAddrs[txCount] = *addr
					// Помечаем что чексумму считали сами — драйвер не пересчитывает.
					w.txAddrs[txCount].SetChecksumValid()
					txOff += n
					txCount++
					if txCount >= len(w.txAddrs) {
						break
					}
				}
			}
			off += plen
		}

		if txCount > 0 {
			err := w.Handle.SendBatch(w.txBuf[:txOff], w.txAddrs[:txCount])
			if err != nil {
				atomic.AddUint64(&w.Stats.Errors, 1)
			} else {
				atomic.AddUint64(&w.Stats.TxPackets, uint64(txCount))
			}
		}
	}
}

func guessLen(b []byte) int {
	if len(b) < 4 {
		return 0
	}
	v := b[0] >> 4
	switch v {
	case 4:
		return int(uint16(b[2])<<8 | uint16(b[3]))
	case 6:
		if len(b) < 6 {
			return 0
		}
		return 40 + int(uint16(b[4])<<8|uint16(b[5]))
	}
	return 0
}
