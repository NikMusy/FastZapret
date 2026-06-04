package services

import (
	"strings"
)

// BuildFilter — текст фильтра для WinDivert на уровне ядра.
// Чем уже фильтр — тем меньше пакетов идёт в user-space.
//
// Что мы хотим перехватывать:
//   - исходящий TCP на 443 (HTTPS/TLS ClientHello)
//   - исходящий UDP на 443 (QUIC)
//   - исходящий UDP к Discord/Roblox диапазонам (опционально)
//   - Telegram MTProto: TCP на 80/443/5222/443/2087 + UDP — но фактически
//     Telegram работает через 443, поэтому отдельных правил не нужно.
func BuildFilter(p Profile) string {
	parts := []string{
		// TLS на 443
		"(outbound and tcp.DstPort == 443 and tcp.PayloadLength > 0)",
		// QUIC на 443
		"(outbound and udp.DstPort == 443)",
	}
	if p.Discord {
		// Голосовые UDP-диапазоны Discord
		parts = append(parts,
			"(outbound and udp.DstPort >= 50000 and udp.DstPort <= 65535)",
		)
	}
	if p.Roblox {
		// Roblox использует UDP в диапазоне 49152-65535
		parts = append(parts,
			"(outbound and udp.DstPort >= 30000 and udp.DstPort <= 65535)",
		)
	}
	return strings.Join(parts, " or ")
}
