package ipparse

import "encoding/binary"

// RecalcIPv4Header пересчитывает только header checksum IPv4 (для случаев
// когда меняем только TTL/длину, без правки L4).
// Поле checksum уже должно быть обнулено.
func RecalcIPv4Header(hdr []byte) uint16 {
	return checksum16(hdr)
}

// PutSeq записывает TCP seq в буфер пакета.
// l4Off — смещение L4 в исходном буфере.
func PutSeq(buf []byte, l4Off int, seq uint32) {
	binary.BigEndian.PutUint32(buf[l4Off+4:], seq)
}

// PutLengths правит IPv4 total_len / IPv6 payload_len и UDP len.
// Используется при ресайзе payload (split).
func PutIPv4TotalLen(buf []byte, total int) {
	binary.BigEndian.PutUint16(buf[2:4], uint16(total))
}

func PutIPv6PayloadLen(buf []byte, l int) {
	binary.BigEndian.PutUint16(buf[4:6], uint16(l))
}

func PutUDPLen(buf []byte, l4Off, l int) {
	binary.BigEndian.PutUint16(buf[l4Off+4:l4Off+6], uint16(l))
}
