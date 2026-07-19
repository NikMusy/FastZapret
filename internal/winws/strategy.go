// Package winws строит командную строку для движка winws.exe (bol-van/zapret).
//
// FastZapret не реализует обход DPI сам — он управляет проверенным движком
// winws.exe, собирая для него набор стратегий (--dpi-desync ...), разбитых
// на группы через --new. Наборы стратегий взяты из
// github.com/Flowseal/zapret-discord-youtube и дополнены профилем под
// Le Mans Ultimate (Cloudflare + Hetzner, мягкий cutoff-desync).
package winws

import (
	"path/filepath"
	"strings"
)

// Profile описывает, что именно обходить.
type Profile struct {
	// Strategy — базовый набор стратегий: "default" | "alt" | "alt2" | "alt3".
	// Разные варианты по-разному пробивают разных провайдеров — если один
	// не работает, пробуют следующий.
	Strategy string `json:"strategy"`
	// LeMans включает выделенную группу стратегий под Le Mans Ultimate.
	LeMans bool `json:"lemans"`
	// LeMansWide — ловить широкий диапазон портов (1024-65535) вместо точных
	// портов LMU. Медленнее (больше трафика через user-space, возможны лаги),
	// но покрывает сервера с нестандартными портами. По умолчанию выключено.
	LeMansWide bool `json:"lemans_wide"`
}

// DefaultProfile — профиль по умолчанию.
func DefaultProfile() Profile {
	return Profile{Strategy: "default", LeMans: false}
}

// Strategies — список доступных базовых наборов (для UI).
var Strategies = []string{"default", "alt", "alt2", "alt3"}

// Порты игрового фильтра LMU. Desync срабатывает ТОЛЬКО для IP из ipset-lmu.txt,
// но перехват (divert) в winws идёт по портам — поэтому узкий набор = меньше
// лишнего трафика через user-space = меньше лагов.
//
// Точные порты LMU/rFactor2: 54297 (симуляция TCP+UDP), 64297 (HTTP TCP),
// 64298/64299 (UDP). Их и ловим по умолчанию.
const (
	lmuNarrowTCP = "54297,64297"
	lmuNarrowUDP = "54297,64298,64299"
	lmuWidePorts = "1024-65535"
)

func lmuPortsTCP(wide bool) string {
	if wide {
		return lmuWidePorts
	}
	return lmuNarrowTCP
}

func lmuPortsUDP(wide bool) string {
	if wide {
		return lmuWidePorts
	}
	return lmuNarrowUDP
}

// BuildArgs собирает полный argv для winws.exe.
// binDir — папка bin (winws.exe, *.bin, WinDivert.dll), listsDir — папка lists.
func BuildArgs(p Profile, binDir, listsDir string) []string {
	b := func(name string) string { return filepath.Join(binDir, name) }
	l := func(name string) string { return filepath.Join(listsDir, name) }

	// --- глобальный фильтр WinDivert (какие порты вообще перехватывать) ---
	wfTCP := "80,443,2053,2083,2087,2096,8443"
	wfUDP := "443,19294-19344,50000-50100"
	if p.LeMans {
		wfTCP += "," + lmuPortsTCP(p.LeMansWide)
		wfUDP += "," + lmuPortsUDP(p.LeMansWide)
	}

	var groups [][]string

	// общие exclude-параметры, повторяемые во многих группах
	excl := []string{
		"--hostlist-exclude=" + l("list-exclude.txt"),
		"--hostlist-exclude=" + l("list-exclude-user.txt"),
		"--ipset-exclude=" + l("ipset-exclude.txt"),
		"--ipset-exclude=" + l("ipset-exclude-user.txt"),
	}

	// ---- базовые группы (Discord / YouTube / общий список) ----
	// Значения seqovl/pos/repeats подобраны в проекте Flowseal и меняются
	// в зависимости от выбранного варианта стратегии.
	tlsSeqovlBig := "681"
	tlsSeqovlSmall := "568"
	quicRepeats := "6"
	splitPos := "1"

	switch strings.ToLower(p.Strategy) {
	case "alt":
		tlsSeqovlBig, tlsSeqovlSmall, quicRepeats = "652", "336", "8"
	case "alt2":
		tlsSeqovlBig, tlsSeqovlSmall, quicRepeats, splitPos = "1", "1", "10", "2"
	case "alt3":
		tlsSeqovlBig, tlsSeqovlSmall, quicRepeats = "211", "211", "6"
	}

	// 1) QUIC для общего списка (Discord и пр.)
	g1 := []string{
		"--filter-udp=443",
		"--hostlist=" + l("list-general.txt"),
		"--hostlist=" + l("list-general-user.txt"),
	}
	g1 = append(g1, excl...)
	g1 = append(g1,
		"--dpi-desync=fake", "--dpi-desync-repeats="+quicRepeats,
		"--dpi-desync-fake-quic="+b("quic_initial_www_google_com.bin"),
	)
	groups = append(groups, g1)

	// 2) Discord голос / STUN (UDP)
	groups = append(groups, []string{
		"--filter-udp=19294-19344,50000-50100", "--filter-l7=discord,stun",
		"--dpi-desync=fake",
		"--dpi-desync-fake-discord=" + b("quic_initial_dbankcloud_ru.bin"),
		"--dpi-desync-fake-stun=" + b("quic_initial_dbankcloud_ru.bin"),
		"--dpi-desync-repeats=" + quicRepeats,
	})

	// 3) discord.media (TCP alt-порты)
	groups = append(groups, []string{
		"--filter-tcp=2053,2083,2087,2096,8443", "--hostlist-domains=discord.media",
		"--dpi-desync=multisplit", "--dpi-desync-split-seqovl=" + tlsSeqovlBig,
		"--dpi-desync-split-pos=" + splitPos,
		"--dpi-desync-split-seqovl-pattern=" + b("tls_clienthello_www_google_com.bin"),
	})

	// 4) YouTube / Google (TCP 443)
	groups = append(groups, []string{
		"--filter-tcp=443", "--hostlist=" + l("list-google.txt"), "--ip-id=zero",
		"--dpi-desync=multisplit", "--dpi-desync-split-seqovl=" + tlsSeqovlBig,
		"--dpi-desync-split-pos=" + splitPos,
		"--dpi-desync-split-seqovl-pattern=" + b("tls_clienthello_www_google_com.bin"),
	})

	// 5) общий список (TCP 80/443)
	g5 := []string{
		"--filter-tcp=80,443",
		"--hostlist=" + l("list-general.txt"),
		"--hostlist=" + l("list-general-user.txt"),
	}
	g5 = append(g5, excl...)
	g5 = append(g5,
		"--dpi-desync=multisplit", "--dpi-desync-split-seqovl="+tlsSeqovlSmall,
		"--dpi-desync-split-pos="+splitPos,
		"--dpi-desync-split-seqovl-pattern="+b("tls_clienthello_4pda_to.bin"),
	)
	groups = append(groups, g5)

	// 6) QUIC по ipset (заблокированные IP)
	g6 := []string{"--filter-udp=443", "--ipset=" + l("ipset-all.txt")}
	g6 = append(g6, excl...)
	g6 = append(g6,
		"--dpi-desync=fake", "--dpi-desync-repeats="+quicRepeats,
		"--dpi-desync-fake-quic="+b("quic_initial_www_google_com.bin"),
	)
	groups = append(groups, g6)

	// 7) TCP по ipset (заблокированные IP)
	g7 := []string{"--filter-tcp=80,443,8443", "--ipset=" + l("ipset-all.txt")}
	g7 = append(g7, excl...)
	g7 = append(g7,
		"--dpi-desync=multisplit", "--dpi-desync-split-seqovl="+tlsSeqovlSmall,
		"--dpi-desync-split-pos="+splitPos,
		"--dpi-desync-split-seqovl-pattern="+b("tls_clienthello_4pda_to.bin"),
	)
	groups = append(groups, g7)

	// ---- Le Mans Ultimate ----
	if p.LeMans {
		groups = append(groups, lmuGroups(b, l, excl, tlsSeqovlBig, tlsSeqovlSmall, splitPos, p.LeMansWide)...)
	}

	// ---- склейка: --wf-* ... group1 --new group2 --new ... ----
	out := []string{
		"--wf-tcp=" + wfTCP,
		"--wf-udp=" + wfUDP,
	}
	for i, g := range groups {
		if i > 0 {
			out = append(out, "--new")
		}
		out = append(out, g...)
	}
	return out
}

// lmuGroups — выделенные группы под Le Mans Ultimate.
//
// Логика: LMU-трафик идёт в две стороны —
//   - API/логин/лобби через Cloudflare (обычный TLS-desync);
//   - гоночные/выделенные сервера на Hetzner (сырой игровой протокол).
//
// Для игрового трафика используется --dpi-desync-cutoff — desync применяется
// только к первым пакетам соединения (пробить DPI на коннекте), а сама сессия
// идёт нетронутой. Это чинит «выкидывает в лобби после загрузки карты»:
// раньше агрессивный desync курочил сессионные пакеты.
func lmuGroups(b, l func(string) string, excl []string, seqBig, seqSmall, pos string, wide bool) [][]string {
	var g [][]string
	gameTCP := lmuPortsTCP(wide)
	gameUDP := lmuPortsUDP(wide)

	// A) TLS/API LMU (Cloudflare) — по доменам и по ipset-lmu.
	a := []string{
		"--filter-tcp=443",
		"--hostlist=" + l("list-lmu.txt"),
		"--ipset=" + l("ipset-lmu.txt"),
	}
	a = append(a, excl...)
	a = append(a,
		"--dpi-desync=multisplit", "--dpi-desync-split-seqovl="+seqBig,
		"--dpi-desync-split-pos="+pos,
		"--dpi-desync-split-seqovl-pattern="+b("tls_clienthello_www_google_com.bin"),
	)
	g = append(g, a)

	// B) игровой TCP (Hetzner/CF) — мягко, только первые пакеты.
	c := []string{
		"--filter-tcp=" + gameTCP,
		"--ipset=" + l("ipset-lmu.txt"),
		"--ipset-exclude=" + l("ipset-exclude.txt"),
		"--ipset-exclude=" + l("ipset-exclude-user.txt"),
		"--dpi-desync=multisplit", "--dpi-desync-any-protocol=1",
		"--dpi-desync-cutoff=n3",
		"--dpi-desync-split-seqovl=" + seqSmall, "--dpi-desync-split-pos=" + pos,
		"--dpi-desync-split-seqovl-pattern=" + b("tls_clienthello_4pda_to.bin"),
	}
	g = append(g, c)

	// C) игровой UDP (Hetzner/CF) — мягкий fake, только первые пакеты.
	d := []string{
		"--filter-udp=" + gameUDP,
		"--ipset=" + l("ipset-lmu.txt"),
		"--ipset-exclude=" + l("ipset-exclude.txt"),
		"--ipset-exclude=" + l("ipset-exclude-user.txt"),
		"--dpi-desync=fake", "--dpi-desync-repeats=6", "--dpi-desync-any-protocol=1",
		"--dpi-desync-fake-unknown-udp=" + b("quic_initial_dbankcloud_ru.bin"),
		"--dpi-desync-cutoff=n2",
	}
	g = append(g, d)

	return g
}

// CommandLine возвращает argv как строку для показа/логов (с кавычками вокруг
// аргументов с пробелами).
func CommandLine(exe string, args []string) string {
	var sb strings.Builder
	sb.WriteString(quote(exe))
	for _, a := range args {
		sb.WriteByte(' ')
		sb.WriteString(quote(a))
	}
	return sb.String()
}

func quote(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
