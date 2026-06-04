// Package ipparse — нулевые аллокации, прямая работа с байтами пакета.
// Никаких структур и копий: возвращаем индексы в исходный буфер.
package ipparse

import "encoding/binary"

const (
	IPv4 = 4
	IPv6 = 6

	ProtoICMP = 1
	ProtoTCP  = 6
	ProtoUDP  = 17
)

// Packet — карта разобранного пакета. Все поля — индексы внутри buf.
type Packet struct {
	Buf       []byte
	Version   uint8 // 4 или 6
	IPHdrLen  int   // длина IP-заголовка
	Proto     uint8 // TCP/UDP/...
	TotalLen  int
	L4Off     int // смещение L4-заголовка
	L4HdrLen  int // длина L4-заголовка (TCP может быть >20)
	PayloadOf int
	PayloadLn int
	SrcPort   uint16
	DstPort   uint16
	TTL       uint8
	TCPFlags  uint8 // только для TCP
	TCPSeq    uint32
	TCPAck    uint32
	WindowSz  uint16
}

const (
	TCPFlagFIN = 1 << 0
	TCPFlagSYN = 1 << 1
	TCPFlagRST = 1 << 2
	TCPFlagPSH = 1 << 3
	TCPFlagACK = 1 << 4
)

// Parse разбирает IPv4/IPv6 пакет с TCP/UDP. Возвращает false, если пакет
// не интересен (битый, не TCP/UDP, фрагмент).
func Parse(buf []byte, p *Packet) bool {
	if len(buf) < 20 {
		return false
	}
	p.Buf = buf
	v := buf[0] >> 4
	p.Version = v
	switch v {
	case IPv4:
		ihl := int(buf[0]&0x0F) * 4
		if ihl < 20 || ihl > len(buf) {
			return false
		}
		// фрагменты не трогаем — DPI всё равно их собирает
		fragOff := binary.BigEndian.Uint16(buf[6:8]) & 0x3FFF
		moreFrag := buf[6]&0x20 != 0
		if fragOff != 0 || moreFrag {
			return false
		}
		p.IPHdrLen = ihl
		p.Proto = buf[9]
		p.TotalLen = int(binary.BigEndian.Uint16(buf[2:4]))
		p.TTL = buf[8]
		p.L4Off = ihl
	case IPv6:
		if len(buf) < 40 {
			return false
		}
		p.IPHdrLen = 40
		p.Proto = buf[6]
		p.TotalLen = 40 + int(binary.BigEndian.Uint16(buf[4:6]))
		p.TTL = buf[7] // Hop Limit
		p.L4Off = 40
	default:
		return false
	}

	if p.L4Off+8 > len(buf) {
		return false
	}
	l4 := buf[p.L4Off:]
	switch p.Proto {
	case ProtoTCP:
		if len(l4) < 20 {
			return false
		}
		dataOff := int(l4[12]>>4) * 4
		if dataOff < 20 || p.L4Off+dataOff > len(buf) {
			return false
		}
		p.L4HdrLen = dataOff
		p.SrcPort = binary.BigEndian.Uint16(l4[0:2])
		p.DstPort = binary.BigEndian.Uint16(l4[2:4])
		p.TCPSeq = binary.BigEndian.Uint32(l4[4:8])
		p.TCPAck = binary.BigEndian.Uint32(l4[8:12])
		p.TCPFlags = l4[13]
		p.WindowSz = binary.BigEndian.Uint16(l4[14:16])
		p.PayloadOf = p.L4Off + dataOff
		p.PayloadLn = p.TotalLen - p.PayloadOf
	case ProtoUDP:
		p.L4HdrLen = 8
		p.SrcPort = binary.BigEndian.Uint16(l4[0:2])
		p.DstPort = binary.BigEndian.Uint16(l4[2:4])
		p.PayloadOf = p.L4Off + 8
		p.PayloadLn = int(binary.BigEndian.Uint16(l4[4:6])) - 8
	default:
		return false
	}
	if p.PayloadLn < 0 || p.PayloadOf+p.PayloadLn > len(buf) {
		return false
	}
	return true
}

// Payload — срез по полезной нагрузке L4.
func (p *Packet) Payload() []byte {
	return p.Buf[p.PayloadOf : p.PayloadOf+p.PayloadLn]
}

// IsTLSClientHello проверяет, что payload — TLS ClientHello (тип 22, версия 3.x, handshake type 1).
func IsTLSClientHello(payload []byte) bool {
	if len(payload) < 6 {
		return false
	}
	// TLS record header: type=22 (handshake), version 0x03**, length
	if payload[0] != 0x16 || payload[1] != 0x03 {
		return false
	}
	// Handshake type = 1 (ClientHello)
	return payload[5] == 0x01
}

// IsQUICInitial — детектор QUIC Initial Packet (Long Header, Type=Initial).
func IsQUICInitial(payload []byte) bool {
	if len(payload) < 7 {
		return false
	}
	// Long header bit set
	if payload[0]&0x80 == 0 {
		return false
	}
	// Long Packet Type Initial = 0b00 → bits 5-4 = 00
	if payload[0]&0x30 != 0x00 {
		return false
	}
	// Version != 0 (version negotiation)
	ver := binary.BigEndian.Uint32(payload[1:5])
	return ver != 0
}

// ExtractSNI вытаскивает SNI из TLS ClientHello. Возвращает срез без копирования.
func ExtractSNI(payload []byte) ([]byte, bool) {
	// TLS record: 5 байт. Далее Handshake: 4 байта. ClientHello body начинается с offset 9.
	if len(payload) < 43 {
		return nil, false
	}
	pos := 5 + 4              // skip record + handshake hdr
	pos += 2                  // legacy_version
	pos += 32                 // random
	if pos >= len(payload) {
		return nil, false
	}
	sidLen := int(payload[pos])
	pos += 1 + sidLen
	if pos+2 > len(payload) {
		return nil, false
	}
	csLen := int(binary.BigEndian.Uint16(payload[pos:]))
	pos += 2 + csLen
	if pos+1 > len(payload) {
		return nil, false
	}
	cmLen := int(payload[pos])
	pos += 1 + cmLen
	if pos+2 > len(payload) {
		return nil, false
	}
	extTotal := int(binary.BigEndian.Uint16(payload[pos:]))
	pos += 2
	end := pos + extTotal
	if end > len(payload) {
		end = len(payload)
	}
	for pos+4 <= end {
		extType := binary.BigEndian.Uint16(payload[pos:])
		extLen := int(binary.BigEndian.Uint16(payload[pos+2:]))
		pos += 4
		if pos+extLen > end {
			return nil, false
		}
		if extType == 0 { // server_name
			if extLen < 5 {
				return nil, false
			}
			// list len (2), name type (1), name len (2), name
			nameLen := int(binary.BigEndian.Uint16(payload[pos+3:]))
			start := pos + 5
			if start+nameLen > pos+extLen {
				return nil, false
			}
			return payload[start : start+nameLen], true
		}
		pos += extLen
	}
	return nil, false
}
