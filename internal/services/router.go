// Package services — настройка стратегий по сервису (Discord/YouTube/Telegram/Roblox).
package services

import (
	"strings"

	"github.com/phantom/fastzapret/internal/ipparse"
	"github.com/phantom/fastzapret/internal/strategy"
)

// Profile — какие сервисы включить и какую агрессивность применить.
type Profile struct {
	Discord  bool
	YouTube  bool
	Telegram bool
	Roblox   bool
	// Tune — общий уровень: "fast" | "balanced" | "aggressive" | "max"
	Tune string
}

// Router реализует divert.Router.
type Router struct {
	prof Profile
	// Общий fallback по протоколу (если SNI не распознан).
	tcpPipe strategy.Pipeline
	udpPipe strategy.Pipeline
	// Per-service пайплайны — выбираются по SNI.
	pipeYouTube  strategy.Pipeline
	pipeDiscord  strategy.Pipeline
	pipeTelegram strategy.Pipeline
	pipeRoblox   strategy.Pipeline
}

// NewRouter собирает пайплайны под профиль.
func NewRouter(p Profile) *Router {
	r := &Router{prof: p}
	switch strings.ToLower(p.Tune) {
	case "fast":
		r.tcpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.TCPSplit{Position: 1}}}
		r.udpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.UDPFake{TTL: 3, FakeData: quicFake[:]}}, EmitOriginal: true}
	case "aggressive":
		r.tcpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.TCPFake{TTL: 3}, strategy.TCPSplit{Position: 3, Disorder: true}}}
		r.udpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.UDPFake{TTL: 2, FakeData: quicFake[:]}, strategy.UDPFake{TTL: 4, FakeData: quicFake[:]}}, EmitOriginal: true}
	case "max":
		// Самый сильный: multi-fake + bad-checksum + split-disorder.
		r.tcpPipe = strategy.Pipeline{Ops: []strategy.Op{
			strategy.TCPMultiFake{TTLs: []uint8{2, 4, 6}},
			strategy.TCPFakeBadChecksum{TTL: 0},
			strategy.TCPSplit{Position: 3, Disorder: true},
		}}
		r.udpPipe = strategy.Pipeline{Ops: []strategy.Op{
			strategy.UDPFake{TTL: 2, FakeData: quicFake[:]},
			strategy.UDPFake{TTL: 4, FakeData: quicFake[:]},
			strategy.UDPFake{TTL: 6, FakeData: quicFake[:]},
		}, EmitOriginal: true}
	default: // balanced
		r.tcpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.TCPFake{TTL: 4}, strategy.TCPSplit{Position: 2}}}
		r.udpPipe = strategy.Pipeline{Ops: []strategy.Op{strategy.UDPFake{TTL: 3, FakeData: quicFake[:]}}, EmitOriginal: true}
	}

	// Per-service пайплайны (если ниже не переопределим — используется общий tcpPipe).
	r.pipeYouTube = strategy.Pipeline{Ops: []strategy.Op{
		strategy.TCPMultiFake{TTLs: []uint8{3, 5}},
		strategy.TCPSplit{Position: 2, Disorder: true},
	}}
	r.pipeDiscord = strategy.Pipeline{Ops: []strategy.Op{
		strategy.TCPFake{TTL: 4},
		strategy.TCPSplit{Position: 1},
	}}
	r.pipeTelegram = strategy.Pipeline{Ops: []strategy.Op{
		strategy.TCPFake{TTL: 4},
		strategy.TCPSplit{Position: 4},
	}}
	r.pipeRoblox = strategy.Pipeline{Ops: []strategy.Op{
		strategy.TCPSplit{Position: 1},
	}}
	return r
}

// Pick — выбор стратегии. Сначала пробуем по SNI, иначе общий пайп.
func (r *Router) Pick(p *ipparse.Packet) strategy.Pipeline {
	if p.Proto == ipparse.ProtoTCP {
		if pipe, ok := r.pickByHost(p); ok {
			return pipe
		}
		return r.tcpPipe
	}
	return r.udpPipe
}

// PassThrough — отсев пакетов, которые не надо обрабатывать (быстрый путь).
func (r *Router) PassThrough(p *ipparse.Packet) bool {
	if p.Proto == ipparse.ProtoTCP {
		if p.PayloadLn < 5 {
			return true
		}
		if ipparse.IsTLSClientHello(p.Payload()) {
			return false
		}
		return true
	}
	if p.Proto == ipparse.ProtoUDP {
		if p.DstPort == 443 && ipparse.IsQUICInitial(p.Payload()) {
			return false
		}
		if r.prof.Discord && p.DstPort >= 50000 {
			return false
		}
		if r.prof.Roblox && p.DstPort >= 30000 && p.DstPort < 65535 {
			return false
		}
		return true
	}
	return true
}

// quicFake — фейковый QUIC Initial с минимально валидным заголовком.
var quicFake = [64]byte{
	0xc0,                   // long header, type=Initial
	0x00, 0x00, 0x00, 0x01, // version: 1
	0x08,                   // dcid len
	1, 2, 3, 4, 5, 6, 7, 8, // dcid
	0x00,       // scid len
	0x00,       // token len
	0x40, 0x32, // packet length (50)
}
