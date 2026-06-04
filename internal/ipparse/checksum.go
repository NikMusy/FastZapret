package ipparse

import "encoding/binary"

// RecalcChecksums пересчитывает IP+L4 checksum после модификаций.
// Реализация инлайн, без вызова WinDivertHelperCalcChecksums (тот делает то же,
// но через syscall в драйвер — это медленно в горячем цикле).
func RecalcChecksums(buf []byte) {
	if len(buf) < 20 {
		return
	}
	v := buf[0] >> 4
	switch v {
	case 4:
		recalcIPv4(buf)
	case 6:
		recalcIPv6(buf)
	}
}

func recalcIPv4(buf []byte) {
	ihl := int(buf[0]&0x0F) * 4
	if ihl < 20 || ihl > len(buf) {
		return
	}
	// IP checksum
	buf[10] = 0
	buf[11] = 0
	csum := checksum16(buf[:ihl])
	binary.BigEndian.PutUint16(buf[10:12], csum)

	proto := buf[9]
	totalLen := int(binary.BigEndian.Uint16(buf[2:4]))
	if totalLen > len(buf) {
		totalLen = len(buf)
	}
	l4 := buf[ihl:totalLen]
	switch proto {
	case ProtoTCP:
		recalcTCPv4(buf, l4)
	case ProtoUDP:
		recalcUDPv4(buf, l4)
	}
}

func recalcIPv6(buf []byte) {
	if len(buf) < 40 {
		return
	}
	proto := buf[6]
	plLen := int(binary.BigEndian.Uint16(buf[4:6]))
	end := 40 + plLen
	if end > len(buf) {
		end = len(buf)
	}
	l4 := buf[40:end]
	switch proto {
	case ProtoTCP:
		recalcTCPv6(buf, l4)
	case ProtoUDP:
		recalcUDPv6(buf, l4)
	}
}

func recalcTCPv4(ip, l4 []byte) {
	if len(l4) < 20 {
		return
	}
	l4[16] = 0
	l4[17] = 0
	sum := pseudoSumV4(ip, ProtoTCP, uint32(len(l4)))
	sum = sum16Add(sum, l4)
	cs := ^foldSum(sum)
	binary.BigEndian.PutUint16(l4[16:18], cs)
}

func recalcUDPv4(ip, l4 []byte) {
	if len(l4) < 8 {
		return
	}
	l4[6] = 0
	l4[7] = 0
	sum := pseudoSumV4(ip, ProtoUDP, uint32(len(l4)))
	sum = sum16Add(sum, l4)
	cs := ^foldSum(sum)
	if cs == 0 {
		cs = 0xFFFF
	}
	binary.BigEndian.PutUint16(l4[6:8], cs)
}

func recalcTCPv6(ip, l4 []byte) {
	if len(l4) < 20 {
		return
	}
	l4[16] = 0
	l4[17] = 0
	sum := pseudoSumV6(ip, ProtoTCP, uint32(len(l4)))
	sum = sum16Add(sum, l4)
	cs := ^foldSum(sum)
	binary.BigEndian.PutUint16(l4[16:18], cs)
}

func recalcUDPv6(ip, l4 []byte) {
	if len(l4) < 8 {
		return
	}
	l4[6] = 0
	l4[7] = 0
	sum := pseudoSumV6(ip, ProtoUDP, uint32(len(l4)))
	sum = sum16Add(sum, l4)
	cs := ^foldSum(sum)
	if cs == 0 {
		cs = 0xFFFF
	}
	binary.BigEndian.PutUint16(l4[6:8], cs)
}

func pseudoSumV4(ip []byte, proto uint8, l4len uint32) uint32 {
	var sum uint32
	// src + dst
	sum += uint32(binary.BigEndian.Uint16(ip[12:14]))
	sum += uint32(binary.BigEndian.Uint16(ip[14:16]))
	sum += uint32(binary.BigEndian.Uint16(ip[16:18]))
	sum += uint32(binary.BigEndian.Uint16(ip[18:20]))
	sum += uint32(proto)
	sum += l4len
	return sum
}

func pseudoSumV6(ip []byte, proto uint8, l4len uint32) uint32 {
	var sum uint32
	for i := 8; i < 40; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ip[i : i+2]))
	}
	sum += l4len & 0xFFFF
	sum += l4len >> 16
	sum += uint32(proto)
	return sum
}

func sum16Add(sum uint32, b []byte) uint32 {
	n := len(b)
	i := 0
	for ; i+1 < n; i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if i < n {
		sum += uint32(b[i]) << 8
	}
	return sum
}

func foldSum(sum uint32) uint16 {
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return uint16(sum)
}

func checksum16(b []byte) uint16 {
	return ^foldSum(sum16Add(0, b))
}
