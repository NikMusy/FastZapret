package services

import (
	"bytes"

	"github.com/phantom/fastzapret/internal/ipparse"
	"github.com/phantom/fastzapret/internal/strategy"
)

// hostKey — служебный SNI-домен, по которому выбираем стратегию.
// Чем точнее матч — тем избирательнее обход.
var (
	youtubeHosts = [][]byte{
		[]byte("youtube.com"),
		[]byte("googlevideo.com"),
		[]byte("ytimg.com"),
		[]byte("youtu.be"),
		[]byte("ggpht.com"),
	}
	discordHosts = [][]byte{
		[]byte("discord.com"),
		[]byte("discord.gg"),
		[]byte("discordapp.com"),
		[]byte("discordapp.net"),
		[]byte("discord.media"),
	}
	telegramHosts = [][]byte{
		[]byte("telegram.org"),
		[]byte("t.me"),
		[]byte("cdn-telegram.org"),
		[]byte("telegram-cdn.org"),
	}
	robloxHosts = [][]byte{
		[]byte("roblox.com"),
		[]byte("rbxcdn.com"),
		[]byte("robloxlabs.com"),
	}
)

// detectService по SNI определяет, к какому сервису относится TLS.
func detectService(sni []byte) string {
	for _, h := range youtubeHosts {
		if hasSuffix(sni, h) {
			return "youtube"
		}
	}
	for _, h := range discordHosts {
		if hasSuffix(sni, h) {
			return "discord"
		}
	}
	for _, h := range telegramHosts {
		if hasSuffix(sni, h) {
			return "telegram"
		}
	}
	for _, h := range robloxHosts {
		if hasSuffix(sni, h) {
			return "roblox"
		}
	}
	return ""
}

func hasSuffix(sni, host []byte) bool {
	if len(sni) < len(host) {
		return false
	}
	if !bytes.EqualFold(sni[len(sni)-len(host):], host) {
		return false
	}
	if len(sni) == len(host) {
		return true
	}
	return sni[len(sni)-len(host)-1] == '.'
}

// pickByHost возвращает целевой pipeline для SNI; "" если матч не найден.
func (r *Router) pickByHost(p *ipparse.Packet) (strategy.Pipeline, bool) {
	if p.Proto != ipparse.ProtoTCP || p.PayloadLn < 43 {
		return strategy.Pipeline{}, false
	}
	sni, ok := ipparse.ExtractSNI(p.Payload())
	if !ok {
		return strategy.Pipeline{}, false
	}
	svc := detectService(sni)
	switch svc {
	case "youtube":
		if !r.prof.YouTube {
			return strategy.Pipeline{}, false
		}
		return r.pipeYouTube, true
	case "discord":
		if !r.prof.Discord {
			return strategy.Pipeline{}, false
		}
		return r.pipeDiscord, true
	case "telegram":
		if !r.prof.Telegram {
			return strategy.Pipeline{}, false
		}
		return r.pipeTelegram, true
	case "roblox":
		if !r.prof.Roblox {
			return strategy.Pipeline{}, false
		}
		return r.pipeRoblox, true
	}
	return strategy.Pipeline{}, false
}
